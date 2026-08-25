//! The Smart Wallet Advisor, end to end.
//!
//! Five steps: fetch a wallet's transaction history via the Glacier API
//! (same approach as `chainkit_fetch`), normalize it, build a
//! summarization prompt, send it to the model, and return a structured
//! plain-English summary. Includes a human-in-the-loop checkpoint
//! before anything gets flagged for review, and an audit log entry for
//! every decision the agent makes along the way.

use chrono::Utc;
use mini_hack_session7::modelprovider::{new_model_client, Message};
use mini_hack_session7::normalize::{normalize_many, RawTx};
use serde::Deserialize;
use serde_json::json;
use std::env;
use std::io::{self, Write};

const GLACIER_BASE_URL: &str = "https://glacier-api.avax.network/v1";
const FUJI_CHAIN_ID: &str = "43113";
const SUMMARY_SYSTEM_PROMPT: &str = "You are a wallet analyst. Be precise and concise.";

#[derive(Debug, Deserialize)]
struct GlacierTx {
    #[serde(rename = "txHash")]
    hash: String,
    value: String,
    #[serde(rename = "tokenSymbol")]
    token_symbol: Option<String>,
    from: String,
    to: String,
    #[serde(rename = "blockTimestamp")]
    timestamp: i64,
    status: i32,
}

#[derive(Debug, Deserialize)]
struct GlacierResponse {
    transactions: Vec<GlacierTx>,
}

async fn get_wallet_history(wallet_address: &str) -> Result<Vec<mini_hack_session7::normalize::Tx>, Box<dyn std::error::Error>> {
    let api_key = env::var("GLACIER_API_KEY")
        .map_err(|_| "Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.")?;

    let url = format!("{GLACIER_BASE_URL}/chains/{FUJI_CHAIN_ID}/addresses/{wallet_address}/transactions?pageSize=50");

    let client = reqwest::Client::new();
    let resp = client.get(&url).header("x-glacier-api-key", &api_key).send().await?;

    if !resp.status().is_success() {
        return Err(format!("Glacier API request failed with status {}", resp.status()).into());
    }

    let data: GlacierResponse = resp.json().await?;

    let raw_txs: Vec<RawTx> = data
        .transactions
        .into_iter()
        .map(|tx| RawTx {
            hash: tx.hash,
            value_wei: tx.value.parse().unwrap_or(0),
            token_symbol: tx.token_symbol,
            from: tx.from,
            to: tx.to,
            timestamp: tx.timestamp,
            status: tx.status,
        })
        .collect();

    Ok(normalize_many(&raw_txs))
}

fn build_summary_prompt(txs: &[mini_hack_session7::normalize::Tx]) -> String {
    let data = serde_json::to_string_pretty(txs).unwrap_or_default();
    format!(
        "Summarize this wallet's activity. Include:\n\
         - total AVAX in and out\n\
         - most frequent counterparty\n\
         - primary token used\n\
         - one notable pattern\n\n\
         Wallet transactions (normalized JSON):\n{data}"
    )
}

fn confirm_action(reasoning: &str, action: &str) -> bool {
    println!("\nAgent reasoning: {reasoning}");
    println!("Proposed action: {action}");
    print!("Approve? (y/n): ");
    io::stdout().flush().ok();
    let mut answer = String::new();
    io::stdin().read_line(&mut answer).ok();
    answer.trim().eq_ignore_ascii_case("y")
}

fn log_decision(reasoning: &str, tool: &str, params: serde_json::Value, result: &str) {
    let entry = json!({
        "timestamp": Utc::now().to_rfc3339(),
        "reasoning": reasoning,
        "tool": tool,
        "params": params,
        "result": result,
    });
    println!("{entry}");
    // production: append to a durable log store, not just stdout
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _ = dotenvy::dotenv();

    let wallet_address = env::args().nth(1).ok_or("Usage: cargo run --bin advisor <wallet-address>")?;

    let txs = get_wallet_history(&wallet_address).await?;
    log_decision(
        "Fetched wallet history",
        "glacier.listTransactions",
        json!({ "wallet_address": wallet_address }),
        &format!("{} transactions", txs.len()),
    );

    let client = new_model_client(None).map_err(|e| e.to_string())?;
    let messages = vec![Message { role: "user".to_string(), content: build_summary_prompt(&txs) }];
    let response = client.generate_text(SUMMARY_SYSTEM_PROMPT, &messages, &[]).await?;
    println!("{}", response.text);

    if let Some(large_tx) = txs.iter().find(|tx| tx.amount_avax >= 5.0) {
        let approved = confirm_action(
            &format!(
                "This wallet sent {:.2} AVAX to {}, unusually large for this pattern.",
                large_tx.amount_avax, large_tx.to
            ),
            "Flag transaction for manual review",
        );
        log_decision(
            "Large transaction detected",
            "confirm_action",
            json!({ "hash": large_tx.hash }),
            if approved { "approved" } else { "rejected" },
        );
    }

    Ok(())
}
