//! The Invoice Payment Agent, CLI version, end to end, with full safety
//! architecture. All the shared logic, invoice checking, pre-flight
//! checks, and the actual Fuji signing, lives in the paymentagent
//! module, this file is just the CLI-specific pieces: the terminal
//! approval prompt and the run loop. See api_server.rs for the same
//! logic wrapped in an HTTP + SSE API instead.
//!
//! See kill_switch.sol in this folder for the fourth safety pillar, the
//! on-chain kill switch, that one lives in the smart contract, not this
//! program.

use mini_hack_session7::paymentagent::{find_overdue_invoices, log_decision, preflight_checks, send_payment, Invoice};
use std::collections::HashSet;
use std::io::{self, Write};
use std::sync::Mutex;

// Step 3: present reasoning, get human approval before anything executes.
fn confirm_payment(inv: &Invoice) -> bool {
    println!("\nInvoice {} from {} is {} days overdue.", inv.id, inv.supplier, inv.days_overdue);
    println!("I intend to send {} USDC to {}.", inv.amount_usdc, inv.recipient);
    print!("Do you approve? (y/n): ");
    io::stdout().flush().ok();
    let mut answer = String::new();
    io::stdin().read_line(&mut answer).ok();
    answer.trim().eq_ignore_ascii_case("y")
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _ = dotenvy::dotenv();

    let sent_payments_log: Mutex<HashSet<String>> = Mutex::new(HashSet::new());
    let overdue = find_overdue_invoices();

    for inv in overdue {
        let reasoning = format!("{} is {} days overdue, condition met (overdue > 3 days)", inv.id, inv.days_overdue);

        let errors = preflight_checks(&inv, &sent_payments_log.lock().unwrap());
        if !errors.is_empty() {
            log_decision(inv.id, &format!("Pre-flight failed: {}", errors.join(", ")), false, None);
            continue;
        }

        if !confirm_payment(&inv) {
            log_decision(inv.id, &reasoning, false, None);
            continue;
        }

        let tx_hash = send_payment(&inv).await?;
        sent_payments_log.lock().unwrap().insert(inv.id.to_string());
        log_decision(inv.id, &reasoning, true, Some(&tx_hash));
        println!("Payment sent, transaction hash: {tx_hash}");
    }

    Ok(())
}
