// Retrieval-Augmented Generation (RAG), end to end.
//
// Five steps: chunk your documents (skipped here, our five docs are
// already short enough to index whole), embed each one, store the
// embeddings in Chroma, retrieve the closest matches at query time, and
// generate an answer grounded in those matches, with citations.
//
// A note on embeddings, Go specific: there is no official Chroma Go
// client, and going through Chroma's REST API directly means embeddings
// must be computed before they're sent, the server does not embed raw
// text for you over plain REST the way the JS and Python SDKs do
// locally. This file computes embeddings by calling OpenAI's
// embeddings endpoint, which means you need OPENAI_API_KEY set in your
// .env regardless of which MODEL_PROVIDER you have chat generation
// pointed at, embeddings and chat completion are separate capabilities.
//
// The Chroma REST endpoints below (tenants, databases, collections,
// add, query) were verified against a real running Chroma v2 server
// during development, not guessed from documentation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"mini-hack-cohort3-session7-golang/modelprovider"
)

const chromaTenant = "default_tenant"
const chromaDatabase = "default_database"
const openaiEmbeddingsURL = "https://api.openai.com/v1/embeddings"
const embeddingModel = "text-embedding-3-small"

// Chroma connection, configurable so this same code works whether
// Chroma is running locally (CHROMA_HOST defaults to localhost) or as
// a sibling service in Docker Compose (CHROMA_HOST=chroma there).
var chromaBaseURL string

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func init() {
	chromaBaseURL = fmt.Sprintf("http://%s:%s/api/v2", envOr("CHROMA_HOST", "localhost"), envOr("CHROMA_PORT", "8000"))
}

// Five documents, five sources, matching what the curriculum asks for.
// In your own project, swap these for your actual docs, README, API
// reference, whatever your agent should be able to answer questions
// about.
var documents = []struct{ ID, Text string }{
	{"avax-cchain", "The C-Chain is Avalanche's EVM-compatible smart contract chain. This is where Solidity contracts, USDC, and most DeFi activity actually run."},
	{"chainkit-readme", "ChainKit is Avalanche's official on-chain data SDK. It provides structured, paginated access to wallet transaction history, and can also run as its own MCP server."},
	{"daraja-api", "The Daraja API is Safaricom's integration platform for M-Pesa. It exposes endpoints for STK push, C2B, B2C, and transaction status queries."},
	{"ethers-core", "ethers.js is a library for interacting with Ethereum-compatible chains. It handles wallets, providers, contract calls, and transaction signing."},
	{"project-readme", "Mini Hack Cohort 3 starter repo: a provider-agnostic agent starter supporting four LLM providers through one shared interface."},
}

// Step 2: embed. Calls OpenAI's embeddings endpoint for one or more
// strings at once, batching is both faster and cheaper than one call
// per document.
func embedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set, embeddings need it even if your chat model is a different provider")
	}

	body, err := json.Marshal(map[string]any{
		"model": embeddingModel,
		"input": texts,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiEmbeddingsURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI embeddings request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

func chromaPost(ctx context.Context, path string, body any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chromaBaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Chroma request to %s failed with status %d", path, resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// some Chroma endpoints (like /add) return no body on success
		return map[string]any{}, nil
	}
	return result, nil
}

// Step 3: store. get_or_create semantics, if the collection already
// exists this just returns its id instead of erroring.
func getOrCreateCollection(ctx context.Context, name string) (string, error) {
	path := fmt.Sprintf("/tenants/%s/databases/%s/collections", chromaTenant, chromaDatabase)
	result, err := chromaPost(ctx, path, map[string]any{
		"name":          name,
		"get_or_create": true,
	})
	if err != nil {
		return "", err
	}
	id, _ := result["id"].(string)
	if id == "" {
		return "", fmt.Errorf("Chroma did not return a collection id")
	}
	return id, nil
}

func addDocuments(ctx context.Context, collectionID string, ids, texts []string, embeddings [][]float64) error {
	path := fmt.Sprintf("/tenants/%s/databases/%s/collections/%s/add", chromaTenant, chromaDatabase, collectionID)
	_, err := chromaPost(ctx, path, map[string]any{
		"ids":        ids,
		"documents":  texts,
		"embeddings": embeddings,
	})
	return err
}

type chunk struct {
	Text     string
	ID       string
	Distance float64
}

// Step 4: retrieve. Embeds the question the same way, then asks Chroma
// for the closest matches by cosine similarity.
func queryKnowledgeBase(ctx context.Context, collectionID, question string) ([]chunk, error) {
	questionEmbeddings, err := embedTexts(ctx, []string{question})
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/tenants/%s/databases/%s/collections/%s/query", chromaTenant, chromaDatabase, collectionID)
	result, err := chromaPost(ctx, path, map[string]any{
		"query_embeddings": questionEmbeddings,
		"n_results":        3,
	})
	if err != nil {
		return nil, err
	}

	// Chroma returns parallel arrays nested one level for "one array per
	// query", we only sent one query, so we read index 0 throughout.
	idsRaw, _ := result["ids"].([]any)
	docsRaw, _ := result["documents"].([]any)
	distRaw, _ := result["distances"].([]any)
	if len(idsRaw) == 0 {
		return nil, fmt.Errorf("no results returned")
	}

	ids := idsRaw[0].([]any)
	docs := docsRaw[0].([]any)
	dists := distRaw[0].([]any)

	chunks := make([]chunk, len(ids))
	for i := range ids {
		chunks[i] = chunk{
			ID:       ids[i].(string),
			Text:     docs[i].(string),
			Distance: dists[i].(float64),
		}
	}
	return chunks, nil
}

// Step 5, part one: build a prompt that forces the model to answer only
// from what we retrieved, and to cite which chunk it used. This single
// instruction is what turns "the model made something up" into "the
// model told me exactly where this came from."
func buildGroundedPrompt(question string, chunks []chunk) string {
	context := ""
	for i, c := range chunks {
		if i > 0 {
			context += "\n\n"
		}
		context += fmt.Sprintf("[%d] (source: %s)\n%s", i+1, c.ID, c.Text)
	}

	return fmt.Sprintf(
		"Answer the question using only the context below. Cite which source number supports each claim. If the context does not cover the question, say you don't know.\n\nContext:\n%s\n\nQuestion: %s",
		context, question,
	)
}

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	question := "What is the C-Chain?"
	if len(os.Args) > 1 {
		question = os.Args[1]
	}

	// Steps 2 and 3: embed and store all five documents.
	ids := make([]string, len(documents))
	texts := make([]string, len(documents))
	for i, d := range documents {
		ids[i] = d.ID
		texts[i] = d.Text
	}

	docEmbeddings, err := embedTexts(ctx, texts)
	if err != nil {
		log.Fatal("RAG agent error:", err)
	}

	collectionID, err := getOrCreateCollection(ctx, "mini-hack-docs")
	if err != nil {
		log.Fatal("RAG agent error:", err)
	}

	if err := addDocuments(ctx, collectionID, ids, texts, docEmbeddings); err != nil {
		log.Fatal("RAG agent error:", err)
	}

	// Step 4: retrieve.
	chunks, err := queryKnowledgeBase(ctx, collectionID, question)
	if err != nil {
		log.Fatal("RAG agent error:", err)
	}

	// Step 5: build the grounded prompt and call the model.
	client, err := modelprovider.NewModelClient("")
	if err != nil {
		log.Fatal("RAG agent error:", err)
	}

	resp, err := client.GenerateText(ctx, "You are a documentation assistant. Only answer from the provided context.", []modelprovider.Message{
		{Role: "user", Content: buildGroundedPrompt(question, chunks)},
	}, nil)
	if err != nil {
		log.Fatal("RAG agent error:", err)
	}

	fmt.Println(resp.Text)
}
