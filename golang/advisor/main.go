// The Smart Wallet Advisor, end to end.
//
// Five steps: fetch a wallet's transaction history via the Glacier API
// (same approach as chainkit-fetch), normalize it, build a
// summarization prompt, send it to the model, and return a structured
// plain-English summary. Includes a human-in-the-loop checkpoint before
// anything gets flagged for review, and an audit log entry for every
// decision the agent makes along the way.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"mini-hack-cohort3-session7-golang/modelprovider"
	"mini-hack-cohort3-session7-golang/normalize"
)

const glacierBaseURL = "https://glacier-api.avax.network/v1"
const fujiChainID = "43113"
const summarySystemPrompt = "You are a wallet analyst. Be precise and concise."

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

func getWalletHistory(walletAddress string) ([]normalize.Tx, error) {
	apiKey := os.Getenv("GLACIER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("set GLACIER_API_KEY in your .env first, get a free one at avacloud.io")
	}

	url := fmt.Sprintf("%s/chains/%s/addresses/%s/transactions?pageSize=50", glacierBaseURL, fujiChainID, walletAddress)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-glacier-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Glacier API request failed with status %d", resp.StatusCode)
	}

	var data glacierResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

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

	return normalize.NormalizeMany(rawTxs), nil
}

func buildSummaryPrompt(txs []normalize.Tx) string {
	data, _ := json.MarshalIndent(txs, "", "  ")
	return fmt.Sprintf(
		"Summarize this wallet's activity. Include:\n"+
			"- total AVAX in and out\n"+
			"- most frequent counterparty\n"+
			"- primary token used\n"+
			"- one notable pattern\n\n"+
			"Wallet transactions (normalized JSON):\n%s",
		string(data),
	)
}

func confirmAction(reader *bufio.Reader, reasoning, action string) bool {
	fmt.Printf("\nAgent reasoning: %s\n", reasoning)
	fmt.Printf("Proposed action: %s\n", action)
	fmt.Print("Approve? (y/n): ")
	answer, _ := reader.ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

func logDecision(reasoning, tool string, params map[string]any, result string) {
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"reasoning": reasoning,
		"tool":      tool,
		"params":    params,
		"result":    result,
	}
	encoded, _ := json.Marshal(entry)
	fmt.Println(string(encoded))
	// production: append to a durable log store, not just stdout
}

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		log.Fatal("Usage: go run ./advisor <wallet-address>")
	}
	walletAddress := os.Args[1]

	txs, err := getWalletHistory(walletAddress)
	if err != nil {
		log.Fatal("Advisor error:", err)
	}
	logDecision("Fetched wallet history", "glacier.listTransactions", map[string]any{"walletAddress": walletAddress}, fmt.Sprintf("%d transactions", len(txs)))

	client, err := modelprovider.NewModelClient("")
	if err != nil {
		log.Fatal("Advisor error:", err)
	}

	ctx := context.Background()
	resp, err := client.GenerateText(ctx, summarySystemPrompt, []modelprovider.Message{
		{Role: "user", Content: buildSummaryPrompt(txs)},
	}, nil)
	if err != nil {
		log.Fatal("Advisor error:", err)
	}
	fmt.Println(resp.Text)

	var largeTx *normalize.Tx
	for i, tx := range txs {
		if tx.AmountAvax >= 5 {
			largeTx = &txs[i]
			break
		}
	}

	if largeTx != nil {
		reader := bufio.NewReader(os.Stdin)
		approved := confirmAction(
			reader,
			fmt.Sprintf("This wallet sent %.2f AVAX to %s, unusually large for this pattern.", largeTx.AmountAvax, largeTx.To),
			"Flag transaction for manual review",
		)
		result := "rejected"
		if approved {
			result = "approved"
		}
		logDecision("Large transaction detected", "confirmAction", map[string]any{"hash": largeTx.Hash}, result)
	}
}
