// The agent's API route, step one of two. Wraps Session 6's invoice
// payment agent logic and streams its progress with Server-Sent
// Events. Stops at approval_required rather than executing anything,
// the second half of the human-in-the-loop pattern lives in
// api/agent/confirm/route.ts, this route never touches a private key.

import { findOverdueInvoices, preflightChecks } from "@/lib/invoices";
import { createRun, logDecision } from "@/lib/runs";

export async function POST() {
  const stream = new ReadableStream({
    async start(controller) {
      const encoder = new TextEncoder();

      function send(event: string, data: unknown) {
        controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
      }

      // Steps 1 and 2 from Session 6: define the condition, evaluate it.
      send("step", { label: "Checking overdue invoices" });
      const overdue = findOverdueInvoices();

      if (overdue.length === 0) {
        send("final", { text: "No overdue invoices right now." });
        controller.close();
        return;
      }

      const invoice = overdue[0];
      const overdueDays = Math.floor((Date.now() - new Date(invoice.dueDate).getTime()) / (1000 * 60 * 60 * 24));
      const reasoning = `${invoice.id} is ${overdueDays} days overdue, condition met (overdue > 3 days)`;

      send("step", { label: "Found an overdue invoice", detail: invoice.id });

      // Safety pillar 1: pre-flight checks, same as the CLI version.
      const errors = preflightChecks(invoice);
      if (errors.length > 0) {
        logDecision({ invoiceId: invoice.id, reasoning: `Pre-flight failed: ${errors.join(", ")}`, approved: false });
        send("final", { text: `Blocked: ${errors.join(", ")}` });
        controller.close();
        return;
      }

      // Step 3: present reasoning, then pause and wait for a real human
      // decision, this is the actual pause, not a demo, the process
      // does not know yet whether it should send anything.
      const runId = createRun(invoice, reasoning);
      send("approval_required", {
        runId,
        reasoning: `Invoice ${invoice.id} from ${invoice.supplier} is ${overdueDays} days overdue.`,
        action: `Send ${invoice.amountUsdc} USDC to ${invoice.recipient}`,
      });

      controller.close();
    },
  });

  return new Response(stream, {
    headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-cache", Connection: "keep-alive" },
  });
}
