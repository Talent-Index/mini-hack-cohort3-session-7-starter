// The Invoice Payment Agent, end to end, with full safety architecture.
//
// Five steps: define the condition (invoice overdue, unpaid), evaluate
// it against a mock invoice database, present the reasoning and get
// human approval, execute the payment as a real USDC transfer on Fuji,
// and log the outcome, approved or not. Wrapped in four required safety
// pillars: pre-flight checks, a spending limit, an idempotency check,
// and an audit log entry for every decision.
//
// See kill-switch.sol in this folder for the fourth pillar, the on-chain
// kill switch, that one lives in the smart contract, not this script.

import "dotenv/config";
import { ethers } from "ethers";
import readline from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

const rl = readline.createInterface({ input, output });

const FUJI_RPC = "https://api.avax-test.network/ext/bc/C/rpc";
const MAX_PAYMENT_USDC = Number(process.env.MAX_PAYMENT_USDC || 500); // spending limit, per transaction
const SENT_PAYMENTS_LOG = new Set(); // idempotency, in memory for this demo

// daysAgo returns an ISO date string that many days before whenever
// this script actually runs. Using fixed calendar dates here would
// mean this mock data silently becomes "more overdue" every day real
// time moves forward, exactly the kind of thing that quietly breaks a
// demo months after it was written. Computing relative to Date.now()
// keeps the mock data's meaning stable regardless of when this runs.
function daysAgo(n) {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return d.toISOString().slice(0, 10);
}

// Mock invoice database, standing in for a real one, same shape you'd
// get back from a real accounts-payable system or database query.
const invoices = [
  { id: "INV-042", supplier: "Supplier X", amountUsdc: 50, dueDate: daysAgo(4), paid: false, recipient: "0xAB12cd34Ef56aB12Cd34Ef56ab12Cd34Ef56aB12" },
  { id: "INV-043", supplier: "Supplier Y", amountUsdc: 120, dueDate: daysAgo(1), paid: false, recipient: "0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34" },
  { id: "INV-044", supplier: "Supplier Z", amountUsdc: 30, dueDate: daysAgo(0), paid: true, recipient: "0xeF56ab12CD34Ef56ab12cD34Ef56AB12cD34ef56" },
];

function daysOverdue(dueDate) {
  const diffMs = new Date() - new Date(dueDate);
  return Math.floor(diffMs / (1000 * 60 * 60 * 24));
}

// Step 1 and 2: define the condition, evaluate it against the mock database.
function findOverdueInvoices() {
  return invoices.filter((inv) => !inv.paid && daysOverdue(inv.dueDate) > 3);
}

// Safety pillar 1: pre-flight checks, run before any payment goes out,
// no exceptions. Safety pillar 2 (spending limit) and the idempotency
// half of pillar 1 are both checked here too.
function preflightChecks(invoice) {
  const errors = [];
  if (!invoice.recipient || invoice.recipient === ethers.ZeroAddress) errors.push("invalid recipient address");
  if (invoice.amountUsdc > MAX_PAYMENT_USDC) errors.push(`amount exceeds spending limit of ${MAX_PAYMENT_USDC} USDC`);
  if (SENT_PAYMENTS_LOG.has(invoice.id)) errors.push("payment already sent for this invoice, idempotency check failed");
  return errors;
}

// Step 3: present reasoning, get human approval before anything executes.
async function confirmPayment(invoice, overdueDays) {
  console.log(`\nInvoice ${invoice.id} from ${invoice.supplier} is ${overdueDays} days overdue.`);
  console.log(`I intend to send ${invoice.amountUsdc} USDC to ${invoice.recipient}.`);
  const answer = await rl.question("Do you approve? (y/n): ");
  return answer.trim().toLowerCase() === "y";
}

// Step 4: on approval, actually execute the payment on Fuji.
async function sendPayment(invoice) {
  const provider = new ethers.JsonRpcProvider(FUJI_RPC);
  const wallet = new ethers.Wallet(process.env.AGENT_PRIVATE_KEY, provider);

  // USDC on Fuji is an ERC-20, transfer() takes the recipient and an
  // amount in the token's smallest unit, 6 decimals for USDC, not 18
  // like AVAX, this trips people up constantly.
  const usdcAbi = ["function transfer(address to, uint256 amount) returns (bool)"];
  const usdc = new ethers.Contract(process.env.FUJI_USDC_ADDRESS, usdcAbi, wallet);

  const amount = ethers.parseUnits(invoice.amountUsdc.toString(), 6);
  const tx = await usdc.transfer(invoice.recipient, amount);
  await tx.wait();
  return tx.hash;
}

// Safety pillar 4 (the software half, the kill switch itself lives on
// chain, see kill-switch.sol): log every decision, approved or not.
function logDecision({ invoiceId, reasoning, approved, txHash }) {
  const entry = {
    timestamp: new Date().toISOString(),
    invoiceId,
    reasoning,
    approved,
    txHash: txHash ?? null,
  };
  console.log(JSON.stringify(entry));
  // production: append to a durable log store, not just stdout
}

async function main() {
  const overdue = findOverdueInvoices();

  for (const invoice of overdue) {
    const overdueDays = daysOverdue(invoice.dueDate);
    const reasoning = `${invoice.id} is ${overdueDays} days overdue, condition met (overdue > 3 days)`;

    const preflightErrors = preflightChecks(invoice);
    if (preflightErrors.length > 0) {
      logDecision({ invoiceId: invoice.id, reasoning: `Pre-flight failed: ${preflightErrors.join(", ")}`, approved: false });
      continue;
    }

    const approved = await confirmPayment(invoice, overdueDays);
    if (!approved) {
      logDecision({ invoiceId: invoice.id, reasoning, approved: false });
      continue;
    }

    const txHash = await sendPayment(invoice);
    SENT_PAYMENTS_LOG.add(invoice.id);
    logDecision({ invoiceId: invoice.id, reasoning, approved: true, txHash });
    console.log(`Payment sent, transaction hash: ${txHash}`);
  }

  rl.close();
}

main().catch((err) => {
  console.error("Payment agent error:", err.message);
  process.exit(1);
});
