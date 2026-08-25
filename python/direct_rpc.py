"""
Method 1: Direct RPC via web3.py.

The simplest way to ask Avalanche a question: no SDK, no API key, just a
JSON-RPC call straight to a public endpoint. Best for simple, real-time,
one-off lookups. For anything involving transaction history across many
blocks, see chainkit_fetch.py instead, direct RPC does not paginate or
index for you.
"""

import asyncio
import os
from datetime import datetime, timezone

from dotenv import load_dotenv
from web3 import Web3

load_dotenv()

FUJI_RPC_URL = "https://api.avax-test.network/ext/bc/C/rpc"


async def main():
    wallet_address = os.environ.get("WALLET_ADDRESS")
    if not wallet_address:
        raise ValueError("Set WALLET_ADDRESS in your .env first.")

    w3 = Web3(Web3.HTTPProvider(FUJI_RPC_URL))
    wallet_address = Web3.to_checksum_address(wallet_address)

    balance_wei = w3.eth.get_balance(wallet_address)
    print(Web3.from_wei(balance_wei, "ether"), "AVAX")

    block = w3.eth.get_block("latest")
    block_time = datetime.fromtimestamp(block["timestamp"], tz=timezone.utc).isoformat()
    print("Latest block:", block["number"], "at", block_time)

    tx_count = w3.eth.get_transaction_count(wallet_address)
    print("Transactions sent from this address:", tx_count)


if __name__ == "__main__":
    asyncio.run(main())
