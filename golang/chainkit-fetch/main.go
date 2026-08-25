// Method 2: on-chain data via the Glacier REST API.
//
// There is no official ChainKit Go package at the time this was written,
// ChainKit itself is a JavaScript/TypeScript SDK. What ChainKit actually
// calls under the hood is Avalanche's Glacier REST API, so this file
// talks to that API directly over HTTP. Same structured, paginated
// data, same free API key from avacloud.io, just without the JS
// wrapper.
//
// NOTE: verify the exact endpoint path and header name against the
// current Glacier API docs (build.avax.network/docs/tooling/glacier-api)
// before you rely on this, REST APIs do change their paths over
// versions, and this was written from the documented shape at the time,
// not tested against a live key.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"mini-hack-cohort3-session7-golang/normalize"
)

const glacierBaseURL = "https://glacier-api.avax.network/v1"
const fujiChainID = "43113"

type glacierTx struct {
	Hash        string `json:"txHash"`
	Value       string `json:"value"`
	TokenSymbol string `json:"tokenSymbol"`
	From        string `json:"from"`
	To          string `json:"to"`
	Timestamp   int64  `json:"blockTimestamp"`
	Status      int    `json:"status"`
}

type glacierResponse struct {
	Transactions []glacierTx `json:"transactions"`
}

func main() {
	_ = godotenv.Load()

	walletAddress := os.Getenv("WALLET_ADDRESS")
	apiKey := os.Getenv("GLACIER_API_KEY")
	if walletAddress == "" {
		log.Fatal("Set WALLET_ADDRESS in your .env first.")
	}
	if apiKey == "" {
		log.Fatal("Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.")
	}

	url := fmt.Sprintf("%s/chains/%s/addresses/%s/transactions?pageSize=10", glacierBaseURL, fujiChainID, walletAddress)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal("ChainKit fetch error:", err)
	}
	req.Header.Set("x-glacier-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal("ChainKit fetch error:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Glacier API request failed with status %d", resp.StatusCode)
	}

	var data glacierResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatal("ChainKit fetch error:", err)
	}

	fmt.Printf("Found %d transactions\n\n", len(data.Transactions))

	rawTxs := make([]normalize.RawTx, 0, len(data.Transactions))
	for _, tx := range data.Transactions {
		valueWei := new(big.Int)
		valueWei.SetString(tx.Value, 10)
		rawTxs = append(rawTxs, normalize.RawTx{
			Hash:        tx.Hash,
			ValueWei:    valueWei,
			TokenSymbol: tx.TokenSymbol,
			From:        tx.From,
			To:          tx.To,
			Timestamp:   tx.Timestamp,
			Status:      tx.Status,
		})
	}

	for _, tx := range normalize.NormalizeMany(rawTxs) {
		statusLabel := "OK"
		if tx.Status != "success" {
			statusLabel = "FAILED"
		}
		fmt.Printf("%s  %.4f %s  %s  %s\n", statusLabel, tx.AmountAvax, tx.Token, tx.Timestamp, tx.Hash)
	}
}
