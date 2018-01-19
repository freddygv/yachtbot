package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	slackbot "github.com/adampointer/go-slackbot"
	"github.com/essentialkaos/slack"
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
	bot := slackbot.New(conf.Slack.Token)
	bot.Hear("(?i)").MessageHandler(queryHandler)
	bot.Run()
}

func init() {
	client = &http.Client{Timeout: time.Second * 10}

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

func queryHandler(ctx context.Context, bot *slackbot.Bot, evt *slack.MessageEvent) {
	tickerSplit := strings.Split(evt.Msg.Text, " ")
	fmt.Println(tickerSplit)

	ticker := tickerSplit[1]

	// Easter eggs
	switch ticker {
	case "XVG":
		bot.Reply(evt, ":joy::joy::joy:", slackbot.WithTyping)
		return
	case "USD":
		bot.Reply(evt, ":trash:", slackbot.WithTyping)
		return
	}

	attachment, err := getSingle(ticker)
	if err != nil {
		panic(err)
	}

	attachments := []slack.Attachment{attachment}
	bot.ReplyWithAttachments(evt, attachments, slackbot.WithTyping)
}

func getSingle(ticker string) (slack.Attachment, error) {
	id, err := getID(db, ticker)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle getID: %v", err)
	}

	target := apiEndpoint + id

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle req: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return slack.Attachment{}, fmt.Errorf("\n Bad response: %s", resp.Status)
	}

	payload := make([]Response, 0)
	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle Decode: %v", err)
	}

	priceUSD, err := strconv.ParseFloat(payload[0].PriceUSD, 64)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle ParseFloat: %v", err)
	}
	bigPrice := big.NewFloat(priceUSD)

	change24h, err := dollarDifference(payload[0].Change24h, bigPrice)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle: %v", err)
	}

	change7d, err := dollarDifference(payload[0].Change7d, bigPrice)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n getSingle: %v", err)
	}

	attachment := slack.Attachment{
		Title:     fmt.Sprintf("Price of %s - $%s 🛥", payload[0].Name, payload[0].Symbol),
		TitleLink: fmt.Sprintf("https://coinmarketcap.com/currencies/%s/", id),
		Fallback:  "Cryptocurrency Price",
		Color:     "#7CD197",
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: "Price USD",
				Value: fmt.Sprintf("$%.2f", priceUSD),
				Short: true,
			},
			slack.AttachmentField{
				Title: "Price BTC",
				Value: payload[0].PriceBTC,
				Short: true,
			},
			slack.AttachmentField{
				Title: "24H Change",
				Value: fmt.Sprintf("%s (%s%%)", currency(change24h), payload[0].Change24h),
				Short: true,
			},
			slack.AttachmentField{
				Title: "7D Change",
				Value: fmt.Sprintf("%s (%s%%)", currency(change7d), payload[0].Change7d),
				Short: true,
			},
		},
		Footer: "ESKETIT",
	}

	return attachment, nil
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

type currency float64

func (c currency) String() string {
	if c < 0 {
		return fmt.Sprintf("-$%.2f", math.Abs(float64(c)))
	}
	return fmt.Sprintf("$%.2f", float32(c))
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
