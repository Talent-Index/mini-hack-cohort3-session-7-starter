// The agent's HTTP API, a separate-backend equivalent to the Next.js
// version in javascript/web/. Reuses the paymentagent package directly,
// this file adds an HTTP and SSE layer on top with Go's standard
// library only, no framework needed.
//
// This is the pattern for "if you're using a separate backend" from
// the Session 7 slides: a real API a frontend on a different origin
// can call, which means CORS has to be configured, done below by hand
// with the standard library, since a separate CORS package would be
// overkill for two headers.
//
// Run it with: go run ./api-server
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"mini-hack-cohort3-session7-golang/paymentagent"
)

// Holds pending runs between the two requests that make up the
// human-in-the-loop pattern. A plain in-memory map, protected by a
// mutex since Go's net/http serves requests concurrently, which only
// works because this runs as one long-lived process. On a platform
// where each request can hit a different, freshly started instance,
// swap this for Redis or a database, this limitation is exactly why:
// a map here is a teaching simplification, not a production pattern.
type pendingRun struct {
	invoice   paymentagent.Invoice
	reasoning string
}

var (
	pendingRunsMu sync.Mutex
	pendingRuns   = map[string]pendingRun{}
)

func newRunID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func sendEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	encoded, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
	flusher.Flush()
}

func startAgentRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Steps 1 and 2 from Session 6: define the condition, evaluate it.
	sendEvent(w, flusher, "step", map[string]any{"label": "Checking overdue invoices"})

	overdue := paymentagent.FindOverdueInvoices()
	if len(overdue) == 0 {
		sendEvent(w, flusher, "final", map[string]any{"text": "No overdue invoices right now."})
		return
	}

	inv := overdue[0]
	overdueDays := paymentagent.DaysOverdue(inv.DueDate)
	reasoning := fmt.Sprintf("%s is %d days overdue, condition met (overdue > 3 days)", inv.ID, overdueDays)

	sendEvent(w, flusher, "step", map[string]any{"label": "Found an overdue invoice", "detail": inv.ID})

	// Safety pillar 1: pre-flight checks, same as the CLI version.
	if errs := paymentagent.PreflightChecks(inv); len(errs) > 0 {
		paymentagent.LogDecision(inv.ID, fmt.Sprintf("Pre-flight failed: %v", errs), false, "")
		sendEvent(w, flusher, "final", map[string]any{"text": fmt.Sprintf("Blocked: %v", errs)})
		return
	}

	// Step 3: present reasoning, then pause and wait for a real human
	// decision, this is the actual pause, not a demo, the process does
	// not know yet whether it should send anything.
	runID := newRunID()
	pendingRunsMu.Lock()
	pendingRuns[runID] = pendingRun{invoice: inv, reasoning: reasoning}
	pendingRunsMu.Unlock()

	sendEvent(w, flusher, "approval_required", map[string]any{
		"runId":     runID,
		"reasoning": fmt.Sprintf("Invoice %s from %s is %d days overdue.", inv.ID, inv.Supplier, overdueDays),
		"action":    fmt.Sprintf("Send %.0f USDC to %s", inv.AmountUSDC, inv.Recipient),
	})
}

func confirmAgentRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID    string `json:"runId"`
		Approved bool   `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	pendingRunsMu.Lock()
	run, ok := pendingRuns[body.RunID]
	delete(pendingRuns, body.RunID)
	pendingRunsMu.Unlock()

	if !ok {
		http.Error(w, "unknown or expired run", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Step 3, the rejection path: the human said no, nothing executes,
	// and that decision gets logged exactly like an approval would.
	if !body.Approved {
		paymentagent.LogDecision(run.invoice.ID, run.reasoning, false, "")
		json.NewEncoder(w).Encode(map[string]string{"text": "Payment rejected, nothing was sent."})
		return
	}

	// Step 4: only now, after a real approval, does anything touch chain.
	txHash, err := paymentagent.SendPayment(run.invoice)
	if err != nil {
		paymentagent.LogDecision(run.invoice.ID, fmt.Sprintf("Execution failed: %s", err), true, "")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	paymentagent.MarkSent(run.invoice.ID)
	// Step 5: log the outcome, same as every other agent in this repo.
	paymentagent.LogDecision(run.invoice.ID, run.reasoning, true, txHash)
	json.NewEncoder(w).Encode(map[string]string{
		"text":   fmt.Sprintf("Payment sent, transaction hash: %s", txHash),
		"txHash": txHash,
	})
}

func main() {
	_ = godotenv.Load()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent", withCORS(startAgentRun))
	mux.HandleFunc("/api/agent/confirm", withCORS(confirmAgentRun))

	server := &http.Server{
		Addr:         ":8000",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Println("Agent API listening on :8000")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
