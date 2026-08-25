//! Data normalization.
//!
//! Whichever method you used to get on-chain data, the raw result is
//! still wei, hex, and Unix timestamps. This turns one raw transaction
//! into something a model, or a human, can actually read. Shared by
//! `direct_rpc`, `chainkit_fetch`, and `chainkit_mcp_agent` so they all
//! use the exact same normalization logic.

use chrono::DateTime;
use serde::{Deserialize, Serialize};

/// The common shape this module expects, in wei and Unix time. Adapt
/// whatever your data source returns into this shape before calling
/// [`normalize`].
#[derive(Debug, Clone, Default)]
pub struct RawTx {
    pub hash: String,
    pub value_wei: u128,
    pub token_symbol: Option<String>,
    pub from: String,
    pub to: String,
    pub timestamp: i64,
    pub status: i32,
}

/// The normalized, human and model-readable shape.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Tx {
    pub hash: String,
    pub amount_avax: f64,
    pub token: String,
    pub from: String,
    pub to: String,
    pub timestamp: String,
    pub status: String,
}

pub fn normalize(raw: &RawTx) -> Tx {
    let amount_avax = raw.value_wei as f64 / 1e18;
    let token = raw.token_symbol.clone().unwrap_or_else(|| "AVAX".to_string());
    let status = if raw.status == 1 { "success" } else { "failed" };

    let timestamp = format_unix_timestamp(raw.timestamp);

    Tx {
        hash: raw.hash.clone(),
        amount_avax,
        token,
        from: raw.from.clone(),
        to: raw.to.clone(),
        timestamp,
        status: status.to_string(),
    }
}

pub fn normalize_many(raws: &[RawTx]) -> Vec<Tx> {
    raws.iter().map(normalize).collect()
}

/// Formats a Unix timestamp (seconds) as an RFC3339 / ISO8601 string.
fn format_unix_timestamp(unix_seconds: i64) -> String {
    match DateTime::from_timestamp(unix_seconds, 0) {
        Some(dt) => dt.to_rfc3339(),
        None => "1970-01-01T00:00:00+00:00".to_string(),
    }
}
