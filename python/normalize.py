"""
Data normalization.

Whichever method you used to get on-chain data, the raw result is still
wei, hex, and Unix timestamps. This turns one raw transaction dict into
something a model, or a human, can actually read. Shared by
direct_rpc.py, chainkit_fetch.py, and chainkit_mcp_agent.py so they all
use the exact same normalization logic.
"""

from datetime import datetime, timezone
from web3 import Web3


def normalize(raw_tx: dict) -> dict:
    value_wei = int(raw_tx.get("value", 0))
    timestamp = int(raw_tx.get("timestamp", 0))

    return {
        "hash": raw_tx.get("hash"),
        "amount": float(Web3.from_wei(value_wei, "ether")),
        "token": raw_tx.get("tokenSymbol", "AVAX"),
        "from": raw_tx.get("from"),
        "to": raw_tx.get("to"),
        "timestamp": datetime.fromtimestamp(timestamp, tz=timezone.utc).isoformat(),
        "status": "success" if raw_tx.get("status") == 1 else "failed",
    }


def normalize_many(raw_txs: list) -> list:
    return [normalize(tx) for tx in raw_txs]
