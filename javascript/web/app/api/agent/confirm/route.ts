// The agent's API route, step two of two. Only ever reached after a
// real human clicked Approve or Reject in the UI. This is the only
// route in the whole app that touches AGENT_PRIVATE_KEY.

import { deleteRun, getRun, logDecision } from "@/lib/runs";
import { markSent } from "@/lib/invoices";
import { sendPayment } from "@/lib/payment";

export async function POST(req: Request) {
  const { runId, approved } = await req.json();

  const run = getRun(runId);
  if (!run) {
    return Response.json({ error: "Unknown or expired run" }, { status: 404 });
  }
  deleteRun(runId);

  // Step 3, the rejection path: the human said no, nothing executes,
  // and that decision gets logged exactly like an approval would.
  if (!approved) {
    logDecision({ invoiceId: run.invoice.id, reasoning: run.reasoning, approved: false });
    return Response.json({ text: "Payment rejected, nothing was sent." });
  }

  // Step 4: only now, after a real approval, does anything touch chain.
  try {
    const txHash = await sendPayment(run.invoice);
    markSent(run.invoice.id);
    // Step 5: log the outcome, same as every other agent in this repo.
    logDecision({ invoiceId: run.invoice.id, reasoning: run.reasoning, approved: true, txHash });
    return Response.json({ text: `Payment sent, transaction hash: ${txHash}`, txHash });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    logDecision({ invoiceId: run.invoice.id, reasoning: `Execution failed: ${message}`, approved: true });
    return Response.json({ error: message }, { status: 500 });
  }
}
