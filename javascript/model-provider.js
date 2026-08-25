// model-provider.js
//
// One factory, one interface, four providers. createModelClient() gives you
// back { provider, generateText }. Every provider implements the same
// shape so chat.js, and anything you build on top of it, never needs to
// know which model is actually running underneath.
//
// Provider selection: pass a name explicitly, createModelClient("openai"),
// or leave it blank and it reads MODEL_PROVIDER from .env, falling back
// to "anthropic" if that's not set either.
//
// generateText({ systemPrompt, messages, tools }) always returns:
//   { text, toolCalls, stopReason, raw }
//
//   text       the assistant's reply text ("" if it only called tools)
//   toolCalls  [{ id, name, input }], normalized regardless of provider.
//                Empty array if the model didn't call a tool, OR if this
//                provider doesn't support tool calling yet (see below).
//   stopReason the provider's own reason string, kept as is, not normalized
//   raw        the full untouched response, in case you need provider-specific detail
//
// TOOL-CALLING SUPPORT, READ THIS BEFORE SESSION 2
// Only the Anthropic client below implements tools. OpenAI, Gemini, and
// Ollama clients accept a `tools` argument without erroring, but ignore it
// and always return toolCalls: []. Each provider's function-calling API
// shape is different enough (Anthropic's content blocks vs. OpenAI's
// tool_calls array vs. Gemini's functionCall parts) that fully normalizing
// all four is real work, not a quick add. If your Week 1 agent needs
// tools, and it does, that's the deliverable, build it on "anthropic"
// until the others catch up.

const SUPPORTED_PROVIDERS = ["anthropic", "openai", "gemini", "ollama"];

function getConfiguredProvider() {
  const provider =
    process.env.MODEL_PROVIDER?.trim().toLowerCase() || "anthropic";

  if (!SUPPORTED_PROVIDERS.includes(provider)) {
    throw new Error(
      `Unsupported MODEL_PROVIDER "${provider}". Use one of: ${SUPPORTED_PROVIDERS.join(", ")}`,
    );
  }

  return provider;
}

// Generic text extraction for providers that don't (yet) return structured
// tool calls, tries the common shapes a chat-completion response takes.
function extractText(value) {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) {
    return value.map(extractText).filter(Boolean).join("\n");
  }

  if (!value || typeof value !== "object") return "";

  if (typeof value.text === "string") return value.text;
  if (typeof value.content === "string") return value.content;
  if (Array.isArray(value.content))
    return value.content.map(extractText).filter(Boolean).join("\n");
  if (typeof value.message?.content === "string") return value.message.content;
  if (Array.isArray(value.message?.content)) {
    return value.message.content.map(extractText).filter(Boolean).join("\n");
  }

  return "";
}

function normalizeResponse(response) {
  if (typeof response === "string") return response;

  const candidates = [
    response?.content,
    response?.choices?.[0]?.message?.content,
    response?.message?.content,
    response?.text,
    response?.result,
    response?.reply,
    response?.response,
  ];

  for (const candidate of candidates) {
    const text = extractText(candidate);
    if (text) return text;
  }

  throw new Error("Unable to extract text from model response.");
}

async function createAnthropicClient() {
  const apiKey = process.env.ANTHROPIC_API_KEY;
  if (!apiKey) {
    throw new Error("ANTHROPIC_API_KEY is not set.");
  }

  const { default: Anthropic } = await import("@anthropic-ai/sdk");
  const client = new Anthropic({ apiKey });

  return {
    provider: "anthropic",
    async generateText({ systemPrompt, messages, tools }) {
      const response = await client.messages.create({
        model: process.env.ANTHROPIC_MODEL || "claude-sonnet-4-6",
        max_tokens: Number(process.env.MAX_TOKENS || 1024),
        system: systemPrompt,
        tools: tools ?? undefined,
        messages,
      });

      // Precise extraction, not the generic guesser below, we know this
      // shape exactly, and tool_use blocks need to survive the round trip.
      const textBlock = response.content.find((b) => b.type === "text");
      const toolCalls = response.content
        .filter((b) => b.type === "tool_use")
        .map((b) => ({ id: b.id, name: b.name, input: b.input }));

      return {
        text: textBlock ? textBlock.text : "",
        toolCalls,
        stopReason: response.stop_reason,
        raw: response,
      };
    },
  };
}

async function createOpenAIClient() {
  const apiKey = process.env.OPENAI_API_KEY;
  if (!apiKey) {
    throw new Error("OPENAI_API_KEY is not set.");
  }

  const { default: OpenAI } = await import("openai");
  const client = new OpenAI({ apiKey });

  return {
    provider: "openai",
    async generateText({ systemPrompt, messages }) {
      // Tool calling not yet implemented for this provider, see the note
      // at the top of this file. Plain text chat only, for now.
      const response = await client.chat.completions.create({
        model: process.env.OPENAI_MODEL || "gpt-4.1",
        max_tokens: Number(process.env.MAX_TOKENS || 1024),
        messages: [{ role: "system", content: systemPrompt }, ...messages],
      });

      return {
        text: normalizeResponse(response),
        toolCalls: [],
        stopReason: response.choices?.[0]?.finish_reason ?? "unknown",
        raw: response,
      };
    },
  };
}

async function createGeminiClient() {
  const apiKey = process.env.GEMINI_API_KEY;
  if (!apiKey) {
    throw new Error("GEMINI_API_KEY is not set.");
  }

  const { GoogleGenAI } = await import("@google/genai");
  const client = new GoogleGenAI({ apiKey });

  return {
    provider: "gemini",
    async generateText({ systemPrompt, messages }) {
      // Tool calling not yet implemented for this provider, see the note
      // at the top of this file. Plain text chat only, for now.
      const response = await client.models.generateContent({
        model: process.env.GEMINI_MODEL || "gemini-2.5-flash",
        config: { systemInstruction: systemPrompt },
        contents: messages.map((message) => ({
          role: message.role === "assistant" ? "model" : "user",
          parts: [{ text: message.content }],
        })),
      });

      return {
        text: normalizeResponse(response),
        toolCalls: [],
        stopReason: response.candidates?.[0]?.finishReason ?? "unknown",
        raw: response,
      };
    },
  };
}

async function createOllamaClient() {
  const baseUrl = process.env.OLLAMA_BASE_URL || "http://localhost:11434";
  const model = process.env.OLLAMA_MODEL || "llama3.1";

  return {
    provider: "ollama",
    async generateText({ systemPrompt, messages }) {
      // Tool calling not yet implemented for this provider, see the note
      // at the top of this file. Plain text chat only, for now.
      const response = await fetch(`${baseUrl}/api/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model,
          stream: false,
          messages: [{ role: "system", content: systemPrompt }, ...messages],
        }),
      });

      if (!response.ok) {
        throw new Error(
          `Ollama request failed with status ${response.status}, is "ollama serve" running, and have you run "ollama pull ${model}"?`,
        );
      }

      const data = await response.json();
      return {
        text: normalizeResponse(data),
        toolCalls: [],
        stopReason: data.done_reason ?? "unknown",
        raw: data,
      };
    },
  };
}

export async function createModelClient(providerOverride) {
  const provider =
    providerOverride?.trim().toLowerCase() || getConfiguredProvider();

  switch (provider) {
    case "anthropic":
      return createAnthropicClient();
    case "openai":
      return createOpenAIClient();
    case "gemini":
      return createGeminiClient();
    case "ollama":
      return createOllamaClient();
    default:
      throw new Error(`Unsupported provider: ${provider}`);
  }
}

export { SUPPORTED_PROVIDERS, normalizeResponse, extractText };
