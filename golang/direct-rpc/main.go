// Method 1: Direct RPC, no SDK at all.
//
// The simplest way to ask Avalanche a question: a raw JSON-RPC call
// straight to a public endpoint over plain HTTP. Best for simple,
// real-time, one-off lookups. For anything involving transaction
// history across many blocks, see chainkit-fetch instead, direct RPC
// does not paginate or index for you, and you would have to walk every
// block yourself.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

const fujiRPCURL = "https://api.avax-test.network/ext/bc/C/rpc"

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func rpcCall(method string, params []any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(fujiRPCURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", parsed.Error.Message)
	}
	return parsed.Result, nil
}

func hexToWei(hexStr string) *big.Int {
	value := new(big.Int)
	value.SetString(hexStr[2:], 16) // strip "0x"
	return value
}

func weiToAvax(wei *big.Int) *big.Float {
	f := new(big.Float).SetInt(wei)
	return new(big.Float).Quo(f, big.NewFloat(1e18))
}

func main() {
	_ = godotenv.Load()

	walletAddress := os.Getenv("WALLET_ADDRESS")
	if walletAddress == "" {
		log.Fatal("Set WALLET_ADDRESS in your .env first.")
	}

	// eth_getBalance
	balanceResult, err := rpcCall("eth_getBalance", []any{walletAddress, "latest"})
	if err != nil {
		log.Fatal("Direct RPC error:", err)
	}
	var balanceHex string
	_ = json.Unmarshal(balanceResult, &balanceHex)
	fmt.Println(weiToAvax(hexToWei(balanceHex)).Text('f', 6), "AVAX")

	// eth_getBlockByNumber
	blockResult, err := rpcCall("eth_getBlockByNumber", []any{"latest", false})
	if err != nil {
		log.Fatal("Direct RPC error:", err)
	}
	var block struct {
		Number    string `json:"number"`
		Timestamp string `json:"timestamp"`
	}
	_ = json.Unmarshal(blockResult, &block)
	blockNum := hexToWei(block.Number)
	blockTimeUnix := hexToWei(block.Timestamp)
	blockTime := time.Unix(blockTimeUnix.Int64(), 0).UTC()
	fmt.Println("Latest block:", blockNum, "at", blockTime.Format(time.RFC3339))

	// eth_getTransactionCount
	countResult, err := rpcCall("eth_getTransactionCount", []any{walletAddress, "latest"})
	if err != nil {
		log.Fatal("Direct RPC error:", err)
	}
	var countHex string
	_ = json.Unmarshal(countResult, &countHex)
	fmt.Println("Transactions sent from this address:", hexToWei(countHex))
}
