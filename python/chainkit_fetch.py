"""
Method 2: On-chain data via the Glacier REST API.

There is no official ChainKit Python package at the time this was
written, ChainKit itself is a JavaScript/TypeScript SDK. What ChainKit
actually calls under the hood is Avalanche's Glacier REST API, so this
file talks to that API directly over HTTP. Same structured, paginated
data, same free API key from avacloud.io, just without the JS wrapper.

NOTE: verify the exact endpoint path and header name against the current
Glacier API docs (build.avax.network/docs/tooling/glacier-api) before
you rely on this, REST APIs do change their paths over versions, and
this was written from the documented shape at the time, not tested
against a live key.
"""

import asyncio
import os

import httpx
from dotenv import load_dotenv

from normalize import normalize_many

load_dotenv()

GLACIER_BASE_URL = "https://glacier-api.avax.network/v1"
FUJI_CHAIN_ID = "43113"


async def main():
    wallet_address = os.environ.get("WALLET_ADDRESS")
    api_key = os.environ.get("GLACIER_API_KEY")
    if not wallet_address:
        raise ValueError("Set WALLET_ADDRESS in your .env first.")
    if not api_key:
        raise ValueError("Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.")

    url = f"{GLACIER_BASE_URL}/chains/{FUJI_CHAIN_ID}/addresses/{wallet_address}/transactions"

    async with httpx.AsyncClient() as client:
        response = await client.get(
            url,
            params={"pageSize": 10},
            headers={"x-glacier-api-key": api_key},
            timeout=30.0,
        )

    if response.status_code != 200:
        raise RuntimeError(f"Glacier API request failed with status {response.status_code}: {response.text}")

    data = response.json()
    transactions = data.get("transactions", [])
    print(f"Found {len(transactions)} transactions\n")

    for tx in normalize_many(transactions):
        status_label = "OK" if tx["status"] == "success" else "FAILED"
        print(f'{status_label}  {tx["amount"]} {tx["token"]}  {tx["timestamp"]}  {tx["hash"]}')


if __name__ == "__main__":
    asyncio.run(main())
