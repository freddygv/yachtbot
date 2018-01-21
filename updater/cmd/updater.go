package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	_ "github.com/lib/pq"
)

const (
	apiEndpoint = "https://api.coinmarketcap.com/v1/ticker/"
)

var (
	client  *http.Client
	db      *sql.DB
	dbURL   = os.Getenv("DB_URL")
	dbPort  = os.Getenv("DB_PORT")
	dbName  = os.Getenv("DB_NAME")
	dbTable = os.Getenv("DB_TABLE")
	dbUser  = os.Getenv("DB_USER")
	dbPW    = os.Getenv("DB_PW")
)

func main() {
	lambda.Start(lambdaHandler)
}

func lambdaHandler() {
	err := getAll()
	if err != nil {
		panic(err)
	}
	db.Close()
}

func init() {
	client = &http.Client{Timeout: time.Second * 10}

	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		dbUser, dbPW, dbName, dbURL, dbPort)

	var err error

	db, err = sql.Open("postgres", dbinfo)
	if err != nil {
		panic(err)
	}
}

func getAll() error {
	// Gets current data for all coins/tokens, don't know another way to get the IDs on demand
	target := apiEndpoint + "?limit=0"

	resp, err := makeRequest(target)
	if err != nil {
		return fmt.Errorf("\n getAll: %v", err)
	}

	tickerMap, err := responseToDict(resp)
	if err != nil {
		return fmt.Errorf("\n getAll: %v", err)
	}

	if err := updateDB(tickerMap); err != nil {
		return fmt.Errorf("\n getAll: %v", err)
	}

	return nil
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

func responseToDict(resp *http.Response) (map[string]string, error) {
	payload := make([]Response, 0)
	err := json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return nil, fmt.Errorf("\n responseToDict NewDecoder: %v", err)
	}

	tickerMap := make(map[string]string)
	for _, v := range payload {
		tickerMap[v.Symbol] = v.ID
	}

	return tickerMap, nil
}

func updateDB(tickerMap map[string]string) error {
	tableName := dbTable

	// Truncating the table then inserting row by row, simplest solution
	if _, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s;", tableName)); err != nil {
		return fmt.Errorf("\n getAll truncate: %v", err)
	}

	for k, v := range tickerMap {
		stmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s(ticker, id) VALUES($1, $2);", tableName))
		if err != nil {
			return fmt.Errorf("\n updateDB db.Prepare: %v", err)
		}

		_, err = stmt.Exec(k, v)
		if err != nil {
			return fmt.Errorf("\n updateDB insert exec: %v", err)
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
