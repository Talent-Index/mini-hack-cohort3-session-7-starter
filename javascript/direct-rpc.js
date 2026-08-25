// Method 1: Direct RPC via ethers.js
//
// The simplest way to ask Avalanche a question: no SDK, no API key, just
// a JSON-RPC call straight to a public endpoint. Best for simple,
// real-time, one-off lookups. For anything involving transaction
// history across many blocks, see chainkit-fetch.js instead, direct RPC
// does not paginate or index for you.

import "dotenv/config";
import { ethers } from "ethers";

const FUJI_RPC_URL = "https://api.avax-test.network/ext/bc/C/rpc";

async function main() {
  const walletAddress = process.env.WALLET_ADDRESS;
  if (!walletAddress) {
    throw new Error("Set WALLET_ADDRESS in your .env first.");
  }

  const provider = new ethers.JsonRpcProvider(FUJI_RPC_URL);

  const balance = await provider.getBalance(walletAddress);
  console.log(ethers.formatEther(balance), "AVAX");

  const block = await provider.getBlock("latest");
  console.log("Latest block:", block.number, "at", new Date(block.timestamp * 1000).toISOString());

  const txCount = await provider.getTransactionCount(walletAddress);
  console.log("Transactions sent from this address:", txCount);
}

main().catch((err) => {
  console.error("Direct RPC error:", err.message);
  process.exit(1);
});
