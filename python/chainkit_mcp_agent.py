"""
ChainKit as an MCP server, wired into an agent.

Before running this, start the ChainKit MCP server in another terminal:
    npx -y @avalanche-sdk/chainkit mcp-server
It will print the local URL it is running on, e.g. http://localhost:PORT/mcp
Put that URL in CHAINKIT_MCP_URL in your .env.

This agent then asks a plain-English question, the model decides to call
a ChainKit tool, and the tool call is forwarded straight to the running
MCP server, no manual SDK calls for the actual data lookup in this file
at all.

NOTE: import paths inside the official "mcp" package have moved before
between versions (SSE transport vs. the newer streamable HTTP transport).
If streamablehttp_client is not found, check the installed version's docs.
"""

import asyncio
import os

from dotenv import load_dotenv
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

from model_provider import create_model_client

load_dotenv()

SYSTEM_PROMPT = "You are Mini Hack Assistant. Use tools when they genuinely help; otherwise answer directly."


def _mcp_tools_to_anthropic_format(mcp_tools) -> list:
    # MCP's Tool type uses inputSchema (camelCase). Anthropic's tools
    # parameter expects input_schema (snake_case). This is an easy
    # mismatch to miss, converting explicitly here rather than assuming
    # the shapes line up.
    return [
        {"name": t.name, "description": t.description or "", "input_schema": t.inputSchema}
        for t in mcp_tools
    ]


async def main():
    url = os.environ.get("CHAINKIT_MCP_URL")
    if not url:
        raise ValueError("Set CHAINKIT_MCP_URL in your .env first, from the running mcp-server output.")

    client = create_model_client()
    messages = []

    async with streamable_http_client(url) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools_result = await session.list_tools()
            tools = _mcp_tools_to_anthropic_format(tools_result.tools)

            print(f"Mini Hack on-chain agent using {client.provider}, connected to ChainKit MCP. Type 'exit' to quit.\n")

            while True:
                user_input = input("You: ")
                if user_input.strip().lower() == "exit":
                    break

                messages.append({"role": "user", "content": user_input})

                response = await client.generate_text(SYSTEM_PROMPT, messages, tools=tools)

                while response.stop_reason == "tool_use":
                    messages.append({"role": "assistant", "content": response.raw.content})

                    tool_results = []
                    for call in response.tool_calls:
                        try:
                            result = await session.call_tool(call.name, call.input)
                            tool_results.append({
                                "type": "tool_result",
                                "tool_use_id": call.id,
                                "content": str(result.content),
                            })
                        except Exception as err:
                            tool_results.append({
                                "type": "tool_result",
                                "tool_use_id": call.id,
                                "content": f"Error: {err}",
                                "is_error": True,
                            })

                    messages.append({"role": "user", "content": tool_results})
                    response = await client.generate_text(SYSTEM_PROMPT, messages, tools=tools)

                print(f"\nAssistant: {response.text}\n")
                messages.append({"role": "assistant", "content": response.raw.content})


if __name__ == "__main__":
    asyncio.run(main())
