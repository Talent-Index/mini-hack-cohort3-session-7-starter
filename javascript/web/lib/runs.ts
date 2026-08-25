// Holds the state of an agent run between the two HTTP requests that
// make up the human-in-the-loop pattern: the first request streams the
// agent's steps and stops at approval_required, the second request,
// after a real human decision, resumes and completes the action.
//
// This is a plain in-memory Map, which only works because this app
// runs as one long-lived Node process. On a serverless platform where
// each request can hit a different, freshly cold-started instance,
// swap this for Redis, or a database table, or anything else that
// exists outside a single process's memory. This limitation is exactly
// why: an in-memory Map here is a teaching simplification, not a
// production pattern.

import type { Invoice } from "./invoices";

type PendingRun = {
  invoice: Invoice;
  reasoning: string;
  createdAt: number;
};

const pendingRuns = new Map<string, PendingRun>();

export function createRun(invoice: Invoice, reasoning: string): string {
  const runId = crypto.randomUUID();
  pendingRuns.set(runId, { invoice, reasoning, createdAt: Date.now() });
  return runId;
}

export function getRun(runId: string): PendingRun | undefined {
  return pendingRuns.get(runId);
}

export function deleteRun(runId: string): void {
  pendingRuns.delete(runId);
}

// Same audit log shape as every CLI agent in this repo, one structured
// entry per decision, approved, rejected, or blocked, every time.
export function logDecision(entry: {
  invoiceId: string;
  reasoning: string;
  approved: boolean;
  txHash?: string | null;
}) {
  const record = {
    timestamp: new Date().toISOString(),
    ...entry,
    txHash: entry.txHash ?? null,
  };
  console.log(JSON.stringify(record));
  // production: append to a durable log store, not just stdout
}
