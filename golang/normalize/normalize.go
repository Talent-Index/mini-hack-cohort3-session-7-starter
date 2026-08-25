// Package normalize turns raw on-chain transaction data into something a
// model, or a human, can actually read. Whichever method you used to get
// the data, direct RPC, ChainKit, the Graph, the raw result is still
// wei, hex, and Unix timestamps. Shared by direct-rpc, chainkit-fetch,
// and chainkit-mcp-agent so they all use the exact same logic.
package normalize

import (
	"math/big"
	"time"
)

// RawTx is the common shape this package expects, in wei and Unix time.
// Adapt whatever your data source returns into this shape before
// calling Normalize.
type RawTx struct {
	Hash        string
	ValueWei    *big.Int
	TokenSymbol string
	From        string
	To          string
	Timestamp   int64
	Status      int
}

// Tx is the normalized, human and model-readable shape.
type Tx struct {
	Hash       string
	AmountAvax float64
	Token      string
	From       string
	To         string
	Timestamp  string
	Status     string
}

func Normalize(raw RawTx) Tx {
	token := raw.TokenSymbol
	if token == "" {
		token = "AVAX"
	}

	status := "failed"
	if raw.Status == 1 {
		status = "success"
	}

	amount := weiToAvax(raw.ValueWei)

	return Tx{
		Hash:       raw.Hash,
		AmountAvax: amount,
		Token:      token,
		From:       raw.From,
		To:         raw.To,
		Timestamp:  time.Unix(raw.Timestamp, 0).UTC().Format(time.RFC3339),
		Status:     status,
	}
}

func NormalizeMany(raws []RawTx) []Tx {
	out := make([]Tx, 0, len(raws))
	for _, r := range raws {
		out = append(out, Normalize(r))
	}
	return out
}

func weiToAvax(wei *big.Int) float64 {
	if wei == nil {
		return 0
	}
	f := new(big.Float).SetInt(wei)
	divisor := new(big.Float).SetFloat64(1e18)
	result := new(big.Float).Quo(f, divisor)
	avax, _ := result.Float64()
	return avax
}
