// Data normalization
//
// Whichever method you used to get on-chain data, the raw result is
// still wei, hex, and Unix timestamps. This turns one raw transaction
// object into something a model, or a human, can actually read.
// Exported so direct-rpc.js, chainkit-fetch.js, and chainkit-mcp-agent.js
// can all share the exact same normalization logic.

import { ethers } from "ethers";

export function normalize(rawTx) {
  return {
    hash: rawTx.hash,
    amount: Number(ethers.formatEther(rawTx.value ?? "0")),
    token: rawTx.tokenSymbol ?? "AVAX",
    from: rawTx.from,
    to: rawTx.to,
    timestamp: new Date((rawTx.timestamp ?? 0) * 1000).toISOString(),
    status: rawTx.status === 1 ? "success" : "failed",
  };
}

export function normalizeMany(rawTxs) {
  return rawTxs.map(normalize);
}
