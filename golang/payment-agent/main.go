// The Invoice Payment Agent, CLI version, end to end, with full safety
// architecture. All the shared logic, invoice checking, pre-flight
// checks, and the actual Fuji signing, lives in the paymentagent
// package, this file is just the CLI-specific pieces: the terminal
// approval prompt and the run loop. See api-server/main.go for the
// same logic wrapped in an HTTP + SSE API instead.
//
// See kill_switch.sol in this folder for the fourth safety pillar, the
// on-chain kill switch, that one lives in the smart contract, not this
// program.
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/joho/godotenv"
	"mini-hack-cohort3-session7-golang/paymentagent"
)

// Step 3: present reasoning, get human approval before anything executes.
func confirmPayment(inv paymentagent.Invoice, overdueDays int) bool {
	fmt.Printf("\nInvoice %s from %s is %d days overdue.\n", inv.ID, inv.Supplier, overdueDays)
	fmt.Printf("I intend to send %.0f USDC to %s.\n", inv.AmountUSDC, inv.Recipient)
	fmt.Print("Do you approve? (y/n): ")
	var answer string
	fmt.Scanln(&answer)
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

func main() {
	_ = godotenv.Load()

	overdue := paymentagent.FindOverdueInvoices()

	for _, inv := range overdue {
		overdueDays := paymentagent.DaysOverdue(inv.DueDate)
		reasoning := fmt.Sprintf("%s is %d days overdue, condition met (overdue > 3 days)", inv.ID, overdueDays)

		if errs := paymentagent.PreflightChecks(inv); len(errs) > 0 {
			paymentagent.LogDecision(inv.ID, fmt.Sprintf("Pre-flight failed: %s", strings.Join(errs, ", ")), false, "")
			continue
		}

		if !confirmPayment(inv, overdueDays) {
			paymentagent.LogDecision(inv.ID, reasoning, false, "")
			continue
		}

		txHash, err := paymentagent.SendPayment(inv)
		if err != nil {
			log.Fatal("Payment agent error:", err)
		}
		paymentagent.MarkSent(inv.ID)
		paymentagent.LogDecision(inv.ID, reasoning, true, txHash)
		fmt.Printf("Payment sent, transaction hash: %s\n", txHash)
	}
}
