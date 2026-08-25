//! Method 2: on-chain data via the Glacier REST API.
//!
//! There is no official ChainKit Rust crate at the time this was
//! written, ChainKit itself is a JavaScript/TypeScript SDK. What
//! ChainKit actually calls under the hood is Avalanche's Glacier REST
//! API, so this file talks to that API directly over HTTP. Same
//! structured, paginated data, same free API key from avacloud.io, just
//! without the JS wrapper.
//!
//! NOTE: verify the exact endpoint path and header name against the
//! current Glacier API docs
//! (build.avax.network/docs/tooling/glacier-api) before you rely on
//! this, REST APIs do change their paths over versions, and this was
//! written from the documented shape at the time, not tested against a
//! live key.

use mini_hack_session7::normalize::{normalize_many, RawTx};
use serde::Deserialize;
use std::env;

const GLACIER_BASE_URL: &str = "https://glacier-api.avax.network/v1";
const FUJI_CHAIN_ID: &str = "43113";

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

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _ = dotenvy::dotenv();

    let wallet_address = env::var("WALLET_ADDRESS").map_err(|_| "Set WALLET_ADDRESS in your .env first.")?;
    let api_key =
        env::var("GLACIER_API_KEY").map_err(|_| "Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.")?;

    let url = format!("{GLACIER_BASE_URL}/chains/{FUJI_CHAIN_ID}/addresses/{wallet_address}/transactions?pageSize=10");

    let client = reqwest::Client::new();
    let resp = client.get(&url).header("x-glacier-api-key", &api_key).send().await?;

    if !resp.status().is_success() {
        return Err(format!("Glacier API request failed with status {}", resp.status()).into());
    }

    let data: GlacierResponse = resp.json().await?;
    println!("Found {} transactions\n", data.transactions.len());

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

    for tx in normalize_many(&raw_txs) {
        let status_label = if tx.status == "success" { "OK" } else { "FAILED" };
        println!("{}  {:.4} {}  {}  {}", status_label, tx.amount_avax, tx.token, tx.timestamp, tx.hash);
    }

    Ok(())
}
