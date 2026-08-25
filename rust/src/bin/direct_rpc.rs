//! Method 1: Direct RPC, no SDK at all.
//!
//! The simplest way to ask Avalanche a question: a raw JSON-RPC call
//! straight to a public endpoint over plain HTTP. Best for simple,
//! real-time, one-off lookups. For anything involving transaction
//! history across many blocks, see `chainkit_fetch` instead, direct RPC
//! does not paginate or index for you.

use serde_json::{json, Value};
use std::env;

const FUJI_RPC_URL: &str = "https://api.avax-test.network/ext/bc/C/rpc";

async fn rpc_call(client: &reqwest::Client, method: &str, params: Value) -> Result<Value, Box<dyn std::error::Error>> {
    let body = json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": method,
        "params": params,
    });

    let resp: Value = client.post(FUJI_RPC_URL).json(&body).send().await?.json().await?;

    if let Some(err) = resp.get("error") {
        return Err(format!("RPC error: {err}").into());
    }
    Ok(resp["result"].clone())
}

fn hex_to_u128(hex_str: &str) -> u128 {
    u128::from_str_radix(hex_str.trim_start_matches("0x"), 16).unwrap_or(0)
}

fn wei_to_avax(wei: u128) -> f64 {
    wei as f64 / 1e18
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _ = dotenvy::dotenv();

    let wallet_address = env::var("WALLET_ADDRESS").map_err(|_| "Set WALLET_ADDRESS in your .env first.")?;

    let client = reqwest::Client::new();

    let balance_hex = rpc_call(&client, "eth_getBalance", json!([wallet_address, "latest"])).await?;
    let balance_wei = hex_to_u128(balance_hex.as_str().unwrap_or("0x0"));
    println!("{:.6} AVAX", wei_to_avax(balance_wei));

    let block = rpc_call(&client, "eth_getBlockByNumber", json!(["latest", false])).await?;
    let block_number = hex_to_u128(block["number"].as_str().unwrap_or("0x0"));
    let block_timestamp = hex_to_u128(block["timestamp"].as_str().unwrap_or("0x0"));
    let block_time = chrono::DateTime::from_timestamp(block_timestamp as i64, 0)
        .map(|dt| dt.to_rfc3339())
        .unwrap_or_default();
    println!("Latest block: {block_number} at {block_time}");

    let count_hex = rpc_call(&client, "eth_getTransactionCount", json!([wallet_address, "latest"])).await?;
    let tx_count = hex_to_u128(count_hex.as_str().unwrap_or("0x0"));
    println!("Transactions sent from this address: {tx_count}");

    Ok(())
}
