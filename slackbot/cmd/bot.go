package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	_ "github.com/lib/pq"
	"github.com/nlopes/slack"
)

const (
	apiEndpoint = "https://api.coinmarketcap.com/v1/ticker/"
)

var (
	client   *http.Client
	db       *sql.DB
	err      error
	dbURL    = os.Getenv("DB_URL")
	dbPort   = os.Getenv("DB_PORT")
	dbName   = os.Getenv("DB_NAME")
	dbTable  = os.Getenv("DB_TABLE")
	dbUser   = os.Getenv("DB_USER")
	dbPW     = os.Getenv("DB_PW")
	botToken = os.Getenv("BOT_TOKEN")
)

// Contains DB connection details and Slack token
var confPath = os.Getenv("HOME") + "/.aws_conf/yachtbot.config"

func main() {
	lambda.Start(queryHandler)
}

func init() {
	client = &http.Client{Timeout: time.Second * 10}

	// Connect to configured AWS RDS
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		dbUser, dbPW, dbName, dbURL, dbPort)

	db, err = sql.Open("postgres", dbinfo)
	if err != nil {
		panic(err)
	}
}

func queryHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// If it's a challenge, unmarshal and reply with challenge.Challenge
	var challenge *Challenge
	if err := json.Unmarshal([]byte(request.Body), &challenge); err != nil {
		log.Fatalf("challenge decode: %v", err)
	}
	if challenge.Challenge != "" {
		return events.APIGatewayProxyResponse{Body: challenge.Challenge, StatusCode: 200}, nil
	}

	// If it's not a challenge, it should be a mention event
	var mention *Mention
	if err := json.Unmarshal([]byte(request.Body), &mention); err != nil {
		log.Fatalf("mention decode: %v", err)
	}

	// debug
	fmt.Println("Mention text:", mention.Event.Text)

	// Get the ticker and pull data
	tickerSplit := strings.Split(mention.Event.Text, " ")
	ticker := strings.ToUpper(tickerSplit[len(tickerSplit)-1])

	attachment, err := getSingle(ticker)
	if err != nil {
		log.Fatalf("queryHandler: %v", err)
	}

	// Send message as slack attachment
	params := slack.PostMessageParameters{AsUser: true}
	params.Attachments = []slack.Attachment{attachment}

	api := slack.New(botToken)
	_, _, err = api.PostMessage(mention.Event.Channel, "", params)
	if err != nil {
		log.Fatalf("queryHandler: %v", err)
		return events.APIGatewayProxyResponse{}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// getSingle returns Slack attachment with price information for a single coin/token
func getSingle(ticker string) (slack.Attachment, error) {
	// CoinMarketCap uses IDs to query the API, not ticker symbols
	id, err := getID(db, ticker)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("getSingle: %v", err)
	}

	if id == "" {
		return slack.Attachment{}, fmt.Errorf("getSingle null ID: %v", err)
	}

	target := apiEndpoint + id

	resp, err := makeRequest(target)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("getSingle: %v", err)
	}

	attachment, err := prepareAttachment(resp)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("getSingle: %s", resp.Status)
	}

	return attachment, nil
}

// Queries Postgres DB for ID that matches the incoming ticker symbol
func getID(db *sql.DB, ticker string) (string, error) {
	cleanTicker := strings.Replace(ticker, "$", "", -1)

	stmt, err := db.Prepare(fmt.Sprintf("SELECT id FROM %s WHERE ticker = $1;", dbTable))
	if err != nil {
		return "", fmt.Errorf("\n getID db.Prepare: %v", err)
	}

	var id string
	rows, err := stmt.Query(cleanTicker)
	if err != nil {
		return "", fmt.Errorf("\n getID query: %v", err)
	}

	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			return "", fmt.Errorf("\n getID scan: %v", err)
		}
	}

	return id, nil
}

func makeRequest(target string) (*http.Response, error) {
	// Prepare and make the request
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, fmt.Errorf("\n makeRequest NewRequest: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("\n makeRequest Do: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("\n makeRequest Bad response: %s", resp.Status)
	}

	return resp, nil
}

func prepareAttachment(resp *http.Response) (slack.Attachment, error) {
	payload := make([]Response, 0)
	err := json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n prepareAttachment Decode: %v", err)
	}
	resp.Body.Close()

	// No financial decisions better be made out of this, using % change to calculate $ differences
	priceUSD, err := strconv.ParseFloat(payload[0].PriceUSD, 64)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n prepareAttachment ParseFloat: %v", err)
	}

	pct24h, err := strconv.ParseFloat(payload[0].Change24h, 64)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n prepareAttachment ParseFloat: %v", err)
	}
	diff24h := priceUSD - (priceUSD / ((pct24h / 100) + 1))

	pct7d, err := strconv.ParseFloat(payload[0].Change7d, 64)
	if err != nil {
		return slack.Attachment{}, fmt.Errorf("\n prepareAttachment ParseFloat: %v", err)
	}
	diff7d := priceUSD - (priceUSD / ((pct7d / 100) + 1))

	color, emoji := getReaction(pct24h)

	// Formatted Slack attachment
	// https://api.slack.com/docs/message-attachments
	attachment := slack.Attachment{
		Title:     fmt.Sprintf("Price of %s - $%s %s", payload[0].Name, payload[0].Symbol, emoji),
		TitleLink: fmt.Sprintf("https://coinmarketcap.com/currencies/%s/", payload[0].ID),
		Fallback:  "Cryptocurrency Price",
		Color:     color,
		Fields: []slack.AttachmentField{
			{
				Title: "Price USD",
				Value: fmt.Sprintf("$%.2f", priceUSD),
				Short: true,
			},
			{
				Title: "Price BTC",
				Value: payload[0].PriceBTC,
				Short: true,
			},
			{
				Title: "24H Change",
				Value: fmt.Sprintf("%s (%s%%)", currency(diff24h), payload[0].Change24h),
				Short: true,
			},
			{
				Title: "7D Change",
				Value: fmt.Sprintf("%s (%s%%)", currency(diff7d), payload[0].Change7d),
				Short: true,
			},
		},
		Footer: "ESKETIT",
	}

	return attachment, nil

}

// Determines color and emoji for Slack attachment based on 24h performance
func getReaction(pct24h float64) (string, string) {
	switch {
	case pct24h < -50:
		return "#d7191c", ":trash::fire:"
	case pct24h < -25:
		return "#d7191c", ":smoking:"
	case pct24h < -10:
		return "#fdae61", ":thinking_face:"
	case pct24h < 0:
		return "#FAD898", ":zzz:"
	case pct24h < 25:
		return "#FAD898", ":beers:"
	case pct24h < 50:
		return "#a6d96a", ":champagne:"
	case pct24h < 100:
		return "#1a9641", ":racing_car:"
	case pct24h < 1000:
		return "#1a9641", ":motor_boat:"
	default:
		return "#000000", ":full_moon_with_face:"
	}
}

type currency float64

// Ensures that negative sign goes before dollar sign
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

// Challenge from Slack to validate my API, need to reply with the challenge in plaintext
// https://api.slack.com/events/url_verification
type Challenge struct {
	Token     string `json:"token,omitempty"`
	Challenge string `json:"challenge,omitempty"`
	Type      string `json:"type,omitempty"`
}

// Mention from Slack
// https://api.slack.com/events/app_mention#mention
type Mention struct {
	Token       string   `json:"token,omitempty"`
	TeamID      string   `json:"team_id,omitempty"`
	APIAppID    string   `json:"api_app_id,omitempty"`
	Event       Event    `json:"event,omitempty"`
	Type        string   `json:"type,omitempty"`
	EventID     string   `json:"event_id,omitempty"`
	EventTime   int      `json:"event_time,omitempty"`
	AuthedUsers []string `json:"authed_users,omitempty"`
}

// Event details corresponding to a mention
type Event struct {
	Type    string `json:"type,omitempty"`
	User    string `json:"user,omitempty"`
	Text    string `json:"text,omitempty"`
	TS      string `json:"ts,omitempty"`
	Channel string `json:"channel,omitempty"`
	EventTS string `json:"event_ts,omitempty"`
}
