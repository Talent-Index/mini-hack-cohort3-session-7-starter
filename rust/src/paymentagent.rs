//! Shared invoice-checking and Fuji USDC signing logic, used by both
//! the CLI in src/bin/payment_agent.rs and the HTTP + SSE server in
//! src/bin/api_server.rs. Pulled out into its own module rather than
//! duplicated, since duplicating signing code is exactly the kind of
//! thing that quietly diverges into two subtly different, both-wrong
//! versions over time.
//!
//! Uses the `ethers` crate for signing, the same crate the entire Rust
//! Ethereum ecosystem is built on. Verified during development by
//! constructing a real wallet and signing a real transaction with a
//! test key, deriving the exact same address as the JavaScript, Python,
//! and Go versions in this repo from that same key.

use ethers::prelude::*;
use ethers::signers::{LocalWallet, Signer};
use ethers::types::transaction::eip2718::TypedTransaction;
use ethers::utils::keccak256;
use std::collections::HashSet;
use std::env;
use std::str::FromStr;

const FUJI_RPC: &str = "https://api.avax-test.network/ext/bc/C/rpc";
const FUJI_CHAIN_ID: u64 = 43113;

#[derive(Clone)]
pub struct Invoice {
    pub id: &'static str,
    pub supplier: &'static str,
    pub amount_usdc: f64,
    pub days_overdue: i64, // fixed for this demo, in a real system compute from a real due date
    pub paid: bool,
    pub recipient: &'static str,
}

// Mock invoice database, standing in for a real one, same shape you'd
// get back from a real accounts-payable system or database query.
pub fn invoices() -> Vec<Invoice> {
    vec![
        Invoice { id: "INV-042", supplier: "Supplier X", amount_usdc: 50.0, days_overdue: 4, paid: false, recipient: "0xAB12cd34Ef56aB12Cd34Ef56ab12Cd34Ef56aB12" },
        Invoice { id: "INV-043", supplier: "Supplier Y", amount_usdc: 120.0, days_overdue: 1, paid: false, recipient: "0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34" },
        Invoice { id: "INV-044", supplier: "Supplier Z", amount_usdc: 30.0, days_overdue: 0, paid: true, recipient: "0xeF56ab12CD34Ef56ab12cD34Ef56AB12cD34ef56" },
    ]
}

pub fn max_payment_usdc() -> f64 {
    env::var("MAX_PAYMENT_USDC").ok().and_then(|v| v.parse().ok()).unwrap_or(500.0) // spending limit, per transaction
}

// Step 1 and 2: define the condition, evaluate it against the mock database.
pub fn find_overdue_invoices() -> Vec<Invoice> {
    invoices().into_iter().filter(|inv| !inv.paid && inv.days_overdue > 3).collect()
}

// Safety pillar 1: pre-flight checks, run before any payment goes out,
// no exceptions. Safety pillar 2 (spending limit) and the idempotency
// half of pillar 1 are both checked here too.
pub fn preflight_checks(inv: &Invoice, sent_payments_log: &HashSet<String>) -> Vec<String> {
    let mut errors = Vec::new();
    if inv.recipient.is_empty() || inv.recipient == "0x0000000000000000000000000000000000000000" {
        errors.push("invalid recipient address".to_string());
    }
    if inv.amount_usdc > max_payment_usdc() {
        errors.push(format!("amount exceeds spending limit of {} USDC", max_payment_usdc()));
    }
    if sent_payments_log.contains(inv.id) {
        errors.push("payment already sent for this invoice, idempotency check failed".to_string());
    }
    errors
}

// erc20_transfer_data builds the calldata for transfer(address,uint256),
// the function selector 0xa9059cbb is the first four bytes of
// Keccak256("transfer(address,uint256)"), a standard, well-known ERC-20
// selector, followed by the recipient address and amount, each
// left-padded to 32 bytes per the Solidity ABI encoding rules.
pub fn erc20_transfer_data(recipient: Address, amount: U256) -> Vec<u8> {
    let selector = keccak256(b"transfer(address,uint256)")[..4].to_vec();
    let mut data = selector;
    let mut recipient_padded = [0u8; 32];
    recipient_padded[12..].copy_from_slice(recipient.as_bytes());
    data.extend_from_slice(&recipient_padded);
    let mut amount_bytes = [0u8; 32];
    amount.to_big_endian(&mut amount_bytes);
    data.extend_from_slice(&amount_bytes);
    data
}

// Step 4: on approval, actually execute the payment on Fuji. This is
// the only function in this module that ever touches AGENT_PRIVATE_KEY.
pub async fn send_payment(inv: &Invoice) -> Result<String, Box<dyn std::error::Error>> {
    let wallet: LocalWallet = env::var("AGENT_PRIVATE_KEY")?.parse()?;
    let wallet = wallet.with_chain_id(FUJI_CHAIN_ID);

    let provider = Provider::<Http>::try_from(FUJI_RPC)?;
    let nonce = provider.get_transaction_count(wallet.address(), None).await?;

    let usdc_address = Address::from_str(&env::var("FUJI_USDC_ADDRESS")?)?;
    let recipient = Address::from_str(inv.recipient)?;

    // USDC on Fuji is an ERC-20, transfer() takes the recipient and an
    // amount in the token's smallest unit, 6 decimals for USDC, not 18
    // like AVAX, this trips people up constantly.
    let amount = U256::from((inv.amount_usdc * 1_000_000.0) as u64);
    let data = erc20_transfer_data(recipient, amount);

    let tx: TypedTransaction = TransactionRequest::new()
        .to(usdc_address)
        .data(data)
        .nonce(nonce)
        .gas(100_000u64)
        .gas_price(25_000_000_000u64) // 25 gwei, illustrative fixed gas price for this demo
        .chain_id(FUJI_CHAIN_ID)
        .into();

    let signature = wallet.sign_transaction(&tx).await?;
    let raw_tx = tx.rlp_signed(&signature);

    let pending_tx = provider.send_raw_transaction(raw_tx).await?;
    let tx_hash = format!("{:?}", pending_tx.tx_hash());
    Ok(tx_hash)
}

// Safety pillar 4 (the software half, the kill switch itself lives on
// chain, see kill_switch.sol): log every decision, approved or not.
pub fn log_decision(invoice_id: &str, reasoning: &str, approved: bool, tx_hash: Option<&str>) {
    let entry = serde_json::json!({
        "timestamp": chrono::Utc::now().to_rfc3339(),
        "invoiceId": invoice_id,
        "reasoning": reasoning,
        "approved": approved,
        "txHash": tx_hash,
    });
    println!("{entry}");
    // production: append to a durable log store, not just stdout
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_find_overdue_invoices() {
        let overdue = find_overdue_invoices();
        assert_eq!(overdue.len(), 1);
        assert_eq!(overdue[0].id, "INV-042");
    }

    #[test]
    fn test_preflight_checks() {
        let sent = HashSet::new();
        let inv = &invoices()[0];
        let errors = preflight_checks(inv, &sent);
        assert!(errors.is_empty());

        let mut bad = invoices()[0].clone();
        bad.amount_usdc = 9999.0;
        let errors2 = preflight_checks(&bad, &sent);
        assert_eq!(errors2.len(), 1);
    }

    #[test]
    fn test_erc20_transfer_data() {
        let recipient = Address::from_str("0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34").unwrap();
        let data = erc20_transfer_data(recipient, U256::from(50_000_000u64));
        assert_eq!(data.len(), 4 + 32 + 32);
        assert_eq!(&data[..4], &[0xa9, 0x05, 0x9c, 0xbb]);
    }

    #[test]
    fn test_key_derivation_matches_other_languages() {
        let wallet: LocalWallet = "1111111111111111111111111111111111111111111111111111111111111111"[..64]
            .parse()
            .unwrap();
        let addr = format!("{:?}", wallet.address());
        // this should match the JS, Python, and Go versions in this repo exactly
        assert_eq!(addr.to_lowercase(), "0x19e7e376e7c213b7e7e7e46cc70a5dd086daff2a");
    }
}
