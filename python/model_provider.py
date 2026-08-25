"""
model_provider.py

One factory, one interface, four providers. create_model_client() gives
you back an object with .provider and an async generate_text(...) method.
Every provider implements the same shape so your agent code never needs
to know which model is actually running underneath.

Provider selection: pass a name explicitly, create_model_client("openai"),
or leave it blank and it reads MODEL_PROVIDER from .env, falling back to
"anthropic" if that is not set either.

generate_text(system_prompt, messages, tools=None) always returns a
ModelResponse with:
  text        the assistant's reply text ("" if it only called tools)
  tool_calls  a list of ToolCall(id, name, input), normalized regardless
              of provider. Empty if the model did not call a tool, OR if
              this provider does not support tool calling yet.
  stop_reason the provider's own reason string, kept as is, not normalized
  raw         the full untouched response, for provider-specific detail

TOOL-CALLING SUPPORT, READ THIS BEFORE YOU BUILD AN AGENT
Only the Anthropic client below implements tools. The other three accept
a tools argument without erroring, but ignore it and always return an
empty tool_calls list. Each provider's function-calling API shape is
different enough (Anthropic's content blocks vs. OpenAI's tool_calls
array vs. Gemini's function_call parts) that fully normalizing all four
is real work, not a quick add. If your agent needs tools, build it on
"anthropic" until the others catch up.
"""

import os
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, Optional

SUPPORTED_PROVIDERS = ["anthropic", "openai", "gemini", "ollama"]


@dataclass
class ToolCall:
    id: str
    name: str
    input: dict


@dataclass
class ModelResponse:
    text: str
    tool_calls: list
    stop_reason: str
    raw: Any


@dataclass
class ModelClient:
    provider: str
    generate_text: Callable[..., Awaitable["ModelResponse"]]


def _get_configured_provider() -> str:
    provider = os.environ.get("MODEL_PROVIDER", "").strip().lower() or "anthropic"
    if provider not in SUPPORTED_PROVIDERS:
        raise ValueError(
            f'Unsupported MODEL_PROVIDER "{provider}". '
            f'Use one of: {", ".join(SUPPORTED_PROVIDERS)}'
        )
    return provider


def _create_anthropic_client() -> ModelClient:
    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        raise ValueError("ANTHROPIC_API_KEY is not set.")

    from anthropic import AsyncAnthropic

    client = AsyncAnthropic(api_key=api_key)
    model = os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-4-6")
    max_tokens = int(os.environ.get("MAX_TOKENS", "1024"))

    async def generate_text(system_prompt, messages, tools=None):
        response = await client.messages.create(
            model=model,
            max_tokens=max_tokens,
            system=system_prompt,
            tools=tools or [],
            messages=messages,
        )

        text_block = next((b for b in response.content if b.type == "text"), None)
        tool_calls = [
            ToolCall(id=b.id, name=b.name, input=b.input)
            for b in response.content
            if b.type == "tool_use"
        ]

        return ModelResponse(
            text=text_block.text if text_block else "",
            tool_calls=tool_calls,
            stop_reason=response.stop_reason,
            raw=response,
        )

    return ModelClient(provider="anthropic", generate_text=generate_text)


def _create_openai_client() -> ModelClient:
    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        raise ValueError("OPENAI_API_KEY is not set.")

    from openai import AsyncOpenAI

    client = AsyncOpenAI(api_key=api_key)
    model = os.environ.get("OPENAI_MODEL", "gpt-4.1")
    max_tokens = int(os.environ.get("MAX_TOKENS", "1024"))

    async def generate_text(system_prompt, messages, tools=None):
        # Tool calling not yet implemented for this provider, see the
        # module docstring. Plain text chat only, for now.
        response = await client.chat.completions.create(
            model=model,
            max_tokens=max_tokens,
            messages=[{"role": "system", "content": system_prompt}, *messages],
        )
        choice = response.choices[0]
        return ModelResponse(
            text=choice.message.content or "",
            tool_calls=[],
            stop_reason=choice.finish_reason,
            raw=response,
        )

    return ModelClient(provider="openai", generate_text=generate_text)


def _create_gemini_client() -> ModelClient:
    api_key = os.environ.get("GEMINI_API_KEY")
    if not api_key:
        raise ValueError("GEMINI_API_KEY is not set.")

    from google import genai

    client = genai.Client(api_key=api_key)
    model = os.environ.get("GEMINI_MODEL", "gemini-2.5-flash")

    async def generate_text(system_prompt, messages, tools=None):
        # Tool calling not yet implemented for this provider, see the
        # module docstring. Plain text chat only, for now.
        # NOTE: check the installed google-genai version's docs, the
        # async namespace (client.aio.*) has moved before across versions.
        contents = [
            {
                "role": "model" if m["role"] == "assistant" else "user",
                "parts": [{"text": m["content"]}],
            }
            for m in messages
        ]
        response = await client.aio.models.generate_content(
            model=model,
            config={"system_instruction": system_prompt},
            contents=contents,
        )
        finish_reason = "unknown"
        if response.candidates:
            finish_reason = getattr(response.candidates[0], "finish_reason", "unknown")

        return ModelResponse(
            text=response.text or "",
            tool_calls=[],
            stop_reason=str(finish_reason),
            raw=response,
        )

    return ModelClient(provider="gemini", generate_text=generate_text)


def _create_ollama_client() -> ModelClient:
    import httpx

    base_url = os.environ.get("OLLAMA_BASE_URL", "http://localhost:11434")
    model = os.environ.get("OLLAMA_MODEL", "llama3.1")

    async def generate_text(system_prompt, messages, tools=None):
        # Tool calling not yet implemented for this provider, see the
        # module docstring. Plain text chat only, for now.
        async with httpx.AsyncClient() as http_client:
            response = await http_client.post(
                f"{base_url}/api/chat",
                json={
                    "model": model,
                    "stream": False,
                    "messages": [{"role": "system", "content": system_prompt}, *messages],
                },
                timeout=60.0,
            )

        if response.status_code != 200:
            raise RuntimeError(
                f"Ollama request failed with status {response.status_code}. "
                f'Is "ollama serve" running, and have you run "ollama pull {model}"?'
            )

        data = response.json()
        return ModelResponse(
            text=data["message"]["content"],
            tool_calls=[],
            stop_reason=data.get("done_reason", "unknown"),
            raw=data,
        )

    return ModelClient(provider="ollama", generate_text=generate_text)


_FACTORIES = {
    "anthropic": _create_anthropic_client,
    "openai": _create_openai_client,
    "gemini": _create_gemini_client,
    "ollama": _create_ollama_client,
}


def create_model_client(provider: Optional[str] = None) -> ModelClient:
    resolved = (provider or "").strip().lower() or _get_configured_provider()
    factory = _FACTORIES.get(resolved)
    if factory is None:
        raise ValueError(f"Unsupported provider: {resolved}")
    return factory()
