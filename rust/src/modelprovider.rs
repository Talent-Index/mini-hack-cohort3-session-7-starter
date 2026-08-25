//! One factory, one interface, four providers. `new_model_client` gives
//! you back a [`Client`] with a `provider` name and a `generate_text`
//! method. Every provider implements the same interface so your agent
//! code never needs to know which model is actually running underneath.
//!
//! There is no official Rust SDK from Anthropic, OpenAI, or Google at
//! the time this was written (unlike Python, JS, and Go, which all have
//! first-party SDKs). Rather than depend on an unofficial, possibly
//! unmaintained crate, every provider here talks to its REST API
//! directly with `reqwest`. This is also a genuinely good way to see
//! what these SDKs are actually doing underneath, in any language.
//!
//! Provider selection: pass a name explicitly, `new_model_client(Some("openai"))`,
//! or pass `None` and it reads `MODEL_PROVIDER` from the environment,
//! falling back to `"anthropic"` if that is not set either.
//!
//! `generate_text` always returns a [`Response`] with:
//! - `text`: the assistant's reply text (empty if it only called tools)
//! - `tool_calls`: a `Vec<ToolCall>`, normalized regardless of provider.
//!   Empty if the model did not call a tool, OR if this provider does
//!   not support tool calling yet.
//! - `stop_reason`: the provider's own reason string, kept as is
//! - `raw`: the full untouched JSON response, for provider-specific detail
//!
//! # Tool-calling support, read this before you build an agent
//!
//! Only the Anthropic client below implements tools. The other three
//! accept a `tools` argument without erroring, but ignore it and always
//! return an empty `tool_calls` vector. Each provider's function-calling
//! API shape is different enough that fully normalizing all four is
//! real work, not a quick add. If your agent needs tools, build it on
//! `"anthropic"` until the others catch up.

use serde_json::{json, Value};
use std::env;

#[derive(Debug, Clone)]
pub struct Message {
    pub role: String,
    pub content: String,
}

#[derive(Debug, Clone)]
pub struct ToolCall {
    pub id: String,
    pub name: String,
    pub input: Value,
}

#[derive(Debug, Clone)]
pub struct Tool {
    pub name: String,
    pub description: String,
    pub input_schema: Value,
}

#[derive(Debug, Clone)]
pub struct Response {
    pub text: String,
    pub tool_calls: Vec<ToolCall>,
    pub stop_reason: String,
    pub raw: Value,
}

pub const SUPPORTED_PROVIDERS: [&str; 4] = ["anthropic", "openai", "gemini", "ollama"];

pub struct Client {
    pub provider: String,
    http: reqwest::Client,
    api_key: String,
    base_url: String,
    model: String,
}

fn env_or(key: &str, fallback: &str) -> String {
    env::var(key).unwrap_or_else(|_| fallback.to_string())
}

fn configured_provider() -> Result<String, String> {
    let provider = env::var("MODEL_PROVIDER")
        .unwrap_or_default()
        .trim()
        .to_lowercase();
    let provider = if provider.is_empty() { "anthropic".to_string() } else { provider };

    if !SUPPORTED_PROVIDERS.contains(&provider.as_str()) {
        return Err(format!(
            "Unsupported MODEL_PROVIDER \"{provider}\". Use one of: {}",
            SUPPORTED_PROVIDERS.join(", ")
        ));
    }
    Ok(provider)
}

pub fn new_model_client(provider: Option<&str>) -> Result<Client, String> {
    let resolved = match provider {
        Some(p) if !p.trim().is_empty() => p.trim().to_lowercase(),
        _ => configured_provider()?,
    };

    let http = reqwest::Client::new();

    match resolved.as_str() {
        "anthropic" => {
            let api_key = env::var("ANTHROPIC_API_KEY").map_err(|_| "ANTHROPIC_API_KEY is not set.".to_string())?;
            Ok(Client {
                provider: "anthropic".to_string(),
                http,
                api_key,
                base_url: "https://api.anthropic.com/v1/messages".to_string(),
                model: env_or("ANTHROPIC_MODEL", "claude-sonnet-4-6"),
            })
        }
        "openai" => {
            let api_key = env::var("OPENAI_API_KEY").map_err(|_| "OPENAI_API_KEY is not set.".to_string())?;
            Ok(Client {
                provider: "openai".to_string(),
                http,
                api_key,
                base_url: "https://api.openai.com/v1/chat/completions".to_string(),
                model: env_or("OPENAI_MODEL", "gpt-4.1"),
            })
        }
        "gemini" => {
            let api_key = env::var("GEMINI_API_KEY").map_err(|_| "GEMINI_API_KEY is not set.".to_string())?;
            Ok(Client {
                provider: "gemini".to_string(),
                http,
                api_key,
                base_url: "https://generativelanguage.googleapis.com/v1beta/models".to_string(),
                model: env_or("GEMINI_MODEL", "gemini-2.5-flash"),
            })
        }
        "ollama" => Ok(Client {
            provider: "ollama".to_string(),
            http,
            api_key: String::new(),
            base_url: env_or("OLLAMA_BASE_URL", "http://localhost:11434"),
            model: env_or("OLLAMA_MODEL", "llama3.1"),
        }),
        other => Err(format!("Unsupported provider: {other}")),
    }
}

impl Client {
    pub async fn generate_text(
        &self,
        system_prompt: &str,
        messages: &[Message],
        tools: &[Tool],
    ) -> Result<Response, Box<dyn std::error::Error>> {
        match self.provider.as_str() {
            "anthropic" => self.generate_text_anthropic(system_prompt, messages, tools).await,
            "openai" => self.generate_text_openai(system_prompt, messages).await,
            "gemini" => self.generate_text_gemini(system_prompt, messages).await,
            "ollama" => self.generate_text_ollama(system_prompt, messages).await,
            other => Err(format!("Unsupported provider: {other}").into()),
        }
    }

    async fn generate_text_anthropic(
        &self,
        system_prompt: &str,
        messages: &[Message],
        tools: &[Tool],
    ) -> Result<Response, Box<dyn std::error::Error>> {
        let anthropic_messages: Vec<Value> = messages
            .iter()
            .map(|m| json!({ "role": m.role, "content": m.content }))
            .collect();

        let anthropic_tools: Vec<Value> = tools
            .iter()
            .map(|t| {
                json!({
                    "name": t.name,
                    "description": t.description,
                    "input_schema": t.input_schema,
                })
            })
            .collect();

        let body = json!({
            "model": self.model,
            "max_tokens": env_or("MAX_TOKENS", "1024").parse::<u32>().unwrap_or(1024),
            "system": system_prompt,
            "messages": anthropic_messages,
            "tools": anthropic_tools,
        });

        let resp = self
            .http
            .post(&self.base_url)
            .header("x-api-key", &self.api_key)
            .header("anthropic-version", "2023-06-01")
            .header("content-type", "application/json")
            .json(&body)
            .send()
            .await?;

        let raw: Value = resp.json().await?;

        let mut text = String::new();
        let mut tool_calls = Vec::new();
        if let Some(content) = raw.get("content").and_then(|c| c.as_array()) {
            for block in content {
                match block.get("type").and_then(|t| t.as_str()) {
                    Some("text") => {
                        if let Some(t) = block.get("text").and_then(|t| t.as_str()) {
                            text = t.to_string();
                        }
                    }
                    Some("tool_use") => {
                        tool_calls.push(ToolCall {
                            id: block.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                            name: block.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                            input: block.get("input").cloned().unwrap_or(json!({})),
                        });
                    }
                    _ => {}
                }
            }
        }

        let stop_reason = raw
            .get("stop_reason")
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();

        Ok(Response { text, tool_calls, stop_reason, raw })
    }

    async fn generate_text_openai(
        &self,
        system_prompt: &str,
        messages: &[Message],
    ) -> Result<Response, Box<dyn std::error::Error>> {
        // Tool calling not yet implemented for this provider, see the
        // module doc comment. Plain text chat only, for now.
        let mut openai_messages = vec![json!({ "role": "system", "content": system_prompt })];
        for m in messages {
            openai_messages.push(json!({ "role": m.role, "content": m.content }));
        }

        let body = json!({
            "model": self.model,
            "max_tokens": env_or("MAX_TOKENS", "1024").parse::<u32>().unwrap_or(1024),
            "messages": openai_messages,
        });

        let resp = self
            .http
            .post(&self.base_url)
            .bearer_auth(&self.api_key)
            .json(&body)
            .send()
            .await?;

        let raw: Value = resp.json().await?;

        let choice = raw.get("choices").and_then(|c| c.get(0));
        let text = choice
            .and_then(|c| c.get("message"))
            .and_then(|m| m.get("content"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();
        let stop_reason = choice
            .and_then(|c| c.get("finish_reason"))
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();

        Ok(Response { text, tool_calls: vec![], stop_reason, raw })
    }

    async fn generate_text_gemini(
        &self,
        system_prompt: &str,
        messages: &[Message],
    ) -> Result<Response, Box<dyn std::error::Error>> {
        // Tool calling not yet implemented for this provider, see the
        // module doc comment. Plain text chat only, for now.
        let contents: Vec<Value> = messages
            .iter()
            .map(|m| {
                let role = if m.role == "assistant" { "model" } else { "user" };
                json!({ "role": role, "parts": [{ "text": m.content }] })
            })
            .collect();

        let body = json!({
            "system_instruction": { "parts": [{ "text": system_prompt }] },
            "contents": contents,
        });

        let url = format!("{}/{}:generateContent?key={}", self.base_url, self.model, self.api_key);

        let resp = self.http.post(&url).json(&body).send().await?;
        let raw: Value = resp.json().await?;

        let candidate = raw.get("candidates").and_then(|c| c.get(0));
        let text = candidate
            .and_then(|c| c.get("content"))
            .and_then(|c| c.get("parts"))
            .and_then(|p| p.get(0))
            .and_then(|p| p.get("text"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();
        let stop_reason = candidate
            .and_then(|c| c.get("finishReason"))
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();

        Ok(Response { text, tool_calls: vec![], stop_reason, raw })
    }

    async fn generate_text_ollama(
        &self,
        system_prompt: &str,
        messages: &[Message],
    ) -> Result<Response, Box<dyn std::error::Error>> {
        // Tool calling not yet implemented for this provider, see the
        // module doc comment. Plain text chat only, for now.
        let mut ollama_messages = vec![json!({ "role": "system", "content": system_prompt })];
        for m in messages {
            ollama_messages.push(json!({ "role": m.role, "content": m.content }));
        }

        let body = json!({
            "model": self.model,
            "stream": false,
            "messages": ollama_messages,
        });

        let url = format!("{}/api/chat", self.base_url);
        let resp = self.http.post(&url).json(&body).send().await?;

        if !resp.status().is_success() {
            return Err(format!(
                "Ollama request failed with status {}. Is \"ollama serve\" running, and have you run \"ollama pull {}\"?",
                resp.status(),
                self.model
            )
            .into());
        }

        let raw: Value = resp.json().await?;
        let text = raw
            .get("message")
            .and_then(|m| m.get("content"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();
        let stop_reason = raw
            .get("done_reason")
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();

        Ok(Response { text, tool_calls: vec![], stop_reason, raw })
    }
}
