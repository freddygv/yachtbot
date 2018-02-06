YachtBot
=======
[![Go Report Card](https://goreportcard.com/badge/github.com/freddygv/cryptoslack)](https://goreportcard.com/report/github.com/freddygv/cryptoslack) 
[![license](https://img.shields.io/github/license/mashape/apistatus.svg)]()

### What?
YachtBot provides cryptocurrency price information from [CoinMarketCap](https://coinmarketcap.com/)

### Components
* [Updater](https://github.com/freddygv/yachtbot/tree/master/updater): Daily cron job. Populates AWS RDS with ticker to ID mappings.
* [Slackbot](https://github.com/freddygv/yachtbot/tree/master/slackbot): Handler function for bot mention invocations. Queries DB to get CoinMarketCap ID for a given ticker then fetches and returns price data.

### Deployment
Both the [updater](https://github.com/freddygv/yachtbot/tree/master/updater) and [slackbot](https://github.com/freddygv/yachtbot/tree/master/slackbot) are running on AWS Lambda.
