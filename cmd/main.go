package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	endpoint = "https://api.coinmarketcap.com/v1/ticker/"
)

var client *http.Client

func main() {
	client = &http.Client{Timeout: time.Second * 10}

	ticker := "bitcoin"
	err := getSingle(ticker)
	if err != nil {
		fmt.Printf("Error: %s", err)
	}
	getAll()
}

func getSingle(ticker string) error {
	target := endpoint + strings.Replace(ticker, "$", "", -1)

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Bad response: %s", resp.Status)
	}

	payload := make([]Response, 0)
	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return err
	}

	priceUSD, err := strconv.ParseFloat(payload[0].PriceUSD, 64)
	if err != nil {
		return err
	}

	change24h, err := strconv.ParseFloat(payload[0].Change24h, 64)
	if err != nil {
		return err
	}
	change24h = priceUSD - (priceUSD / (change24h + 1))

	change7d, err := strconv.ParseFloat(payload[0].Change7d, 64)
	if err != nil {
		return err
	}
	change7d = priceUSD - (priceUSD / (change7d + 1))

	singleAttachment := fmt.Sprintf(slackAttachment,
		payload[0].Name, payload[0].Symbol, payload[0].ID,
		fmt.Sprintf("%.2f", priceUSD), payload[0].PriceBTC,
		fmt.Sprintf("%.2f", change24h), payload[0].Change24h,
		fmt.Sprintf("%.2f", change7d), payload[0].Change7d)

	fmt.Println(singleAttachment)

	return nil
}

func getAll() error {
	target := endpoint + "?limit=0"

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Bad response: %s", resp.Status)
	}

	payload := make([]Response, 0)
	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return err
	}

	tickerMap := make(map[string]string)
	for _, v := range payload {
		tickerMap[v.Symbol] = v.ID
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
