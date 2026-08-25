// The same real Fuji payment execution from Session 6's
// payment-agent.js, unchanged. This is the actual on-chain step, only
// ever called from api/agent/confirm/route.ts, and only after a real
// human approval has been received.

import { ethers } from "ethers";
import type { Invoice } from "./invoices";

const FUJI_RPC = "https://api.avax-test.network/ext/bc/C/rpc";

export async function sendPayment(invoice: Invoice): Promise<string> {
  const provider = new ethers.JsonRpcProvider(FUJI_RPC);
  const wallet = new ethers.Wallet(process.env.AGENT_PRIVATE_KEY!, provider);

  // USDC on Fuji is an ERC-20, transfer() takes the recipient and an
  // amount in the token's smallest unit, 6 decimals for USDC, not 18
  // like AVAX, this trips people up constantly.
  const usdcAbi = ["function transfer(address to, uint256 amount) returns (bool)"];
  const usdc = new ethers.Contract(process.env.FUJI_USDC_ADDRESS!, usdcAbi, wallet);

  const amount = ethers.parseUnits(invoice.amountUsdc.toString(), 6);
  const tx = await usdc.transfer(invoice.recipient, amount);
  await tx.wait();
  return tx.hash;
}
