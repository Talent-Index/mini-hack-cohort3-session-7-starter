//! The agent's HTTP API, a separate-backend equivalent to the Next.js
//! version in javascript/web/. Reuses the paymentagent module directly,
//! this file adds an HTTP and SSE layer on top with axum, the crate
//! that has become the standard choice for HTTP servers in the Rust
//! ecosystem, built on the same tokio runtime already used throughout
//! this repo.
//!
//! This is the pattern for "if you're using a separate backend" from
//! the Session 7 slides: a real API a frontend on a different origin
//! can call, which means CORS has to be configured, done below with
//! tower_http's CorsLayer.
//!
//! Run it with: cargo run --bin api_server

use axum::extract::State;
use axum::response::sse::{Event, KeepAlive, Sse};
use axum::response::IntoResponse;
use axum::routing::post;
use axum::{Json, Router};
use futures_util::stream::{self, Stream};
use mini_hack_session7::paymentagent::{
    find_overdue_invoices, log_decision, preflight_checks, send_payment, Invoice,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::collections::HashMap;
use std::convert::Infallible;
use std::sync::{Arc, Mutex};
use tower_http::cors::CorsLayer;

// Holds pending runs between the two requests that make up the
// human-in-the-loop pattern, plus the set of invoice ids already paid,
// which is what actually makes the idempotency check in
// preflight_checks meaningful across separate /api/agent calls. Both
// are plain in-memory state protected by a mutex and wrapped in Arc so
// every request handler can share them, which only works because this
// runs as one long-lived process. On a platform where each request can
// hit a different, freshly started instance, swap this for Redis or a
// database, this limitation is exactly why: in-memory state here is a
// teaching simplification, not a production pattern.
#[derive(Clone)]
struct PendingRun {
    invoice: Invoice,
    reasoning: String,
}

#[derive(Clone, Default)]
struct AppState {
    pending_runs: Arc<Mutex<HashMap<String, PendingRun>>>,
    sent_invoices: Arc<Mutex<std::collections::HashSet<String>>>,
}

async fn start_agent_run(
    State(state): State<AppState>,
) -> Sse<impl Stream<Item = Result<Event, Infallible>>> {
    let mut events: Vec<Event> = Vec::new();

    // Steps 1 and 2 from Session 6: define the condition, evaluate it.
    events.push(Event::default().event("step").data(json!({"label": "Checking overdue invoices"}).to_string()));

    let overdue = find_overdue_invoices();
    if overdue.is_empty() {
        events.push(Event::default().event("final").data(json!({"text": "No overdue invoices right now."}).to_string()));
        return Sse::new(stream::iter(events.into_iter().map(Ok))).keep_alive(KeepAlive::default());
    }

    let invoice = overdue[0].clone();
    let reasoning = format!(
        "{} is {} days overdue, condition met (overdue > 3 days)",
        invoice.id, invoice.days_overdue
    );

    events.push(Event::default().event("step").data(json!({"label": "Found an overdue invoice", "detail": invoice.id}).to_string()));

    // Safety pillar 1: pre-flight checks, same as the CLI version. Uses
    // the real shared sent_invoices set, so a repeat call for an
    // invoice already paid in an earlier run genuinely gets blocked
    // here, not just an empty set that could never catch anything.
    let errors = {
        let sent = state.sent_invoices.lock().unwrap();
        preflight_checks(&invoice, &sent)
    };
    if !errors.is_empty() {
        log_decision(invoice.id, &format!("Pre-flight failed: {}", errors.join(", ")), false, None);
        events.push(Event::default().event("final").data(json!({"text": format!("Blocked: {}", errors.join(", "))}).to_string()));
        return Sse::new(stream::iter(events.into_iter().map(Ok))).keep_alive(KeepAlive::default());
    }

    // Step 3: present reasoning, then pause and wait for a real human
    // decision, this is the actual pause, not a demo, the process does
    // not know yet whether it should send anything.
    let run_id = uuid::Uuid::new_v4().to_string();
    state.pending_runs.lock().unwrap().insert(run_id.clone(), PendingRun { invoice: invoice.clone(), reasoning });

    events.push(Event::default().event("approval_required").data(
        json!({
            "runId": run_id,
            "reasoning": format!("Invoice {} from {} is {} days overdue.", invoice.id, invoice.supplier, invoice.days_overdue),
            "action": format!("Send {} USDC to {}", invoice.amount_usdc, invoice.recipient),
        })
        .to_string(),
    ));

    Sse::new(stream::iter(events.into_iter().map(Ok))).keep_alive(KeepAlive::default())
}

#[derive(Deserialize)]
struct ConfirmBody {
    #[serde(rename = "runId")]
    run_id: String,
    approved: bool,
}

#[derive(Serialize)]
struct ConfirmResponse {
    text: String,
    #[serde(rename = "txHash", skip_serializing_if = "Option::is_none")]
    tx_hash: Option<String>,
}

async fn confirm_agent_run(
    State(state): State<AppState>,
    Json(body): Json<ConfirmBody>,
) -> impl IntoResponse {
    let run = state.pending_runs.lock().unwrap().remove(&body.run_id);

    let Some(run) = run else {
        return (axum::http::StatusCode::NOT_FOUND, Json(json!({"error": "Unknown or expired run"}))).into_response();
    };

    // Step 3, the rejection path: the human said no, nothing executes,
    // and that decision gets logged exactly like an approval would.
    if !body.approved {
        log_decision(run.invoice.id, &run.reasoning, false, None);
        return Json(ConfirmResponse { text: "Payment rejected, nothing was sent.".to_string(), tx_hash: None }).into_response();
    }

    // Step 4: only now, after a real approval, does anything touch chain.
    match send_payment(&run.invoice).await {
        Ok(tx_hash) => {
            state.sent_invoices.lock().unwrap().insert(run.invoice.id.to_string());
            // Step 5: log the outcome, same as every other agent in this repo.
            log_decision(run.invoice.id, &run.reasoning, true, Some(&tx_hash));
            Json(ConfirmResponse {
                text: format!("Payment sent, transaction hash: {tx_hash}"),
                tx_hash: Some(tx_hash),
            })
            .into_response()
        }
        Err(err) => {
            log_decision(run.invoice.id, &format!("Execution failed: {err}"), true, None);
            (axum::http::StatusCode::INTERNAL_SERVER_ERROR, Json(json!({"error": err.to_string()}))).into_response()
        }
    }
}

#[tokio::main]
async fn main() {
    let _ = dotenvy::dotenv();

    let state = AppState::default();

    let app = Router::new()
        .route("/api/agent", post(start_agent_run))
        .route("/api/agent/confirm", post(confirm_agent_run))
        .layer(CorsLayer::permissive())
        .with_state(state);

    let listener = tokio::net::TcpListener::bind("0.0.0.0:8004").await.unwrap();
    println!("Agent API listening on :8004");
    axum::serve(listener, app).await.unwrap();
}
