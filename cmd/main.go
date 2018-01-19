package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	_ "github.com/lib/pq"
)

const (
	apiEndpoint = "https://api.coinmarketcap.com/v1/ticker/"
)

var client *http.Client
var conf botConfig
var confPath = os.Getenv("HOME") + "/.aws_conf/yachtbot.config"

func main() {
	client = &http.Client{Timeout: time.Second * 10}

	if _, err := toml.DecodeFile(confPath, &conf); err != nil {
		panic(err)
	}

	fmt.Println(conf.Slack.Token)

	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		conf.Db.User, conf.Db.Pw, conf.Db.Name, conf.Db.Endpoint, conf.Db.Port)

	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// TODO: Uncomment later
	// ticker := "$BTC"
	// err = getSingle(db, ticker)
	// if err != nil {
	// 	panic(err)
	// }

	// TODO: Uncomment later
	// err = getAll(db)
	// if err != nil {
	// 	panic(err)
	// }
}

func getSingle(db *sql.DB, ticker string) error {
	id, err := getID(db, ticker)
	if err != nil {
		return fmt.Errorf("\n getSingle getID: %v", err)
	}

	target := apiEndpoint + id

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return fmt.Errorf("\n getSingle req: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("\n getSingle Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("\n Bad response: %s", resp.Status)
	}

	payload := make([]Response, 0)
	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return fmt.Errorf("\n getSingle Decode: %v", err)
	}

	priceUSD, err := strconv.ParseFloat(payload[0].PriceUSD, 64)
	if err != nil {
		return fmt.Errorf("\n getSingle ParseFloat: %v", err)
	}
	bigPrice := big.NewFloat(priceUSD)

	change24h, err := dollarDifference(payload[0].Change24h, bigPrice)
	if err != nil {
		return fmt.Errorf("\n getSingle: %v", err)
	}

	change7d, err := dollarDifference(payload[0].Change7d, bigPrice)
	if err != nil {
		return fmt.Errorf("\n getSingle: %v", err)
	}

	singleAttachment := fmt.Sprintf(slackAttachment,
		payload[0].Name, payload[0].Symbol, payload[0].ID,
		fmt.Sprintf("%.2f", priceUSD), payload[0].PriceBTC,
		fmt.Sprintf("%.2f", change24h), payload[0].Change24h,
		fmt.Sprintf("%.2f", change7d), payload[0].Change7d)

	fmt.Println(singleAttachment)

	return nil
}

func getAll(db *sql.DB) error {
	target := apiEndpoint + "?limit=0"

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return fmt.Errorf("\n getAll req: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("\n getAll Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("\n Bad response: %s", resp.Status)
	}

	payload := make([]Response, 0)
	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return fmt.Errorf("\n getAll NewDecoder: %v", err)
	}

	tickerMap := make(map[string]string)
	for _, v := range payload {
		tickerMap[v.Symbol] = v.ID
	}

	if _, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s;", conf.Db.Table)); err != nil {
		return fmt.Errorf("\n getAll truncate: %v", err)
	}

	for k, v := range tickerMap {
		stmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s(ticker, id) VALUES($1, $2);", conf.Db.Table))
		if err != nil {
			return fmt.Errorf("\n getAll db.Prepare: %v", err)
		}

		_, err = stmt.Exec(k, v)
		if err != nil {
			return fmt.Errorf("\n getAll insert exec: %v", err)
		}
	}

	return nil
}

func dollarDifference(percentChange string, bigPrice *big.Float) (float64, error) {
	parsedChange, err := strconv.ParseFloat(percentChange, 64)
	if err != nil {
		return 0, fmt.Errorf("\n dollarDifference: %v", err)
	}
	bigChange := new(big.Float).Quo(big.NewFloat(parsedChange), big.NewFloat(100))
	priceYesterday := new(big.Float).Quo(bigPrice, (new(big.Float).Add(bigChange, big.NewFloat(1))))
	bigDiff := new(big.Float).Sub(bigPrice, priceYesterday)
	difference, _ := bigDiff.Float64()

	return difference, nil
}

func getID(db *sql.DB, ticker string) (string, error) {
	cleanTicker := strings.Replace(ticker, "$", "", -1)

	stmt, err := db.Prepare(fmt.Sprintf("SELECT id FROM %s WHERE ticker = $1;", conf.Db.Table))
	if err != nil {
		return "", fmt.Errorf("\n getSingle db.Prepare: %v", err)
	}

	var id string
	rows, err := stmt.Query(cleanTicker)
	if err != nil {
		return "", fmt.Errorf("\n getSingle query: %v", err)
	}

	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			return "", fmt.Errorf("\n getSingle scan: %v", err)
		}
	}

	return id, nil
}

// Response from CoinMarketCap API
type Response struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name,omitempty"`
	Symbol          string `json:"symbol,omitempty"`
	Rank            string `json:"rank,omitempty"`
	PriceUSD        string `json:"price_usd,omitempty"`
	PriceBTC        string `json:"price_btc,omitempty"`
	Volume24h       string `json:"24h_volume_usd,omitempty"`
	MarketCap       string `json:"market_cap_usd,omitempty"`
	SupplyAvailable string `json:"available_supply,omitempty"`
	SupplyTotal     string `json:"total_supply,omitempty"`
	SupplyMax       string `json:"max_supply,omitempty"`
	Change1h        string `json:"percent_change_1h,omitempty"`
	Change24h       string `json:"percent_change_24h,omitempty"`
	Change7d        string `json:"percent_change_7d,omitempty"`
	Updated         string `json:"last_updated,omitempty"`
}

type botConfig struct {
	Db    dbConfig
	Slack slackConfig
}

type dbConfig struct {
	Endpoint string
	Port     string
	Name     string
	Table    string
	User     string
	Pw       string
}

type slackConfig struct {
	Token string
}

var slackAttachment = `{
		"attachments": [
			{
				"fallback": "Cryptocurrency Price",
				"color": "#36a64f",
				"title": "Price of %s - %s 🛥", 
				"title_link": "https://coinmarketcap.com/currencies/%s/",
				"fields": [
					{
						"title": "Price USD",
						"value": "$%s",
						"short": true
					},
					{
						"title": "Price BTC",
						"value": "%s",
						"short": true
					},
					{
						"title": "24H Change",
						"value": "$%s (%s%%)",
						"short": true
					},
					{
						"title": "7D Change",
						"value": "$%s (%s%%)",
						"short": true
					}
				],
				"footer": "YachtBot",
				"footer_icon": "https://emojipedia-us.s3.amazonaws.com/thumbs/160/apple/33/motor-boat_1f6e5.png",
				"ts": 123456789
			}
		]
	}`
