package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	_ "github.com/lib/pq"
)

const (
	apiEndpoint = "https://api.coinmarketcap.com/v1/ticker/"
)

var client *http.Client
var db *sql.DB
var conf botConfig
var confPath = os.Getenv("HOME") + "/.aws_conf/yachtbot.config"

func main() {
	err := getAll()
	if err != nil {
		panic(err)
	}
	db.Close()
}

func init() {
	client = &http.Client{Timeout: time.Second * 10}

	// Decode DB connection details from local conf
	_, err := toml.DecodeFile(confPath, &conf)
	if err != nil {
		panic(err)
	}

	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		conf.Db.User, conf.Db.Pw, conf.Db.Name, conf.Db.Endpoint, conf.Db.Port)

	db, err = sql.Open("postgres", dbinfo)
	if err != nil {
		panic(err)
	}
}

func getAll() error {
	// Gets current data for all coins/tokens, don't know another way to get the IDs on demand
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

	// Truncating the table then inserting row by row, simplest solution
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
