"use client";

import { useState } from "react";

type Step = { label: string; detail?: string };
type Approval = { runId: string; reasoning: string; action: string } | null;

export default function Home() {
  const [steps, setSteps] = useState<Step[]>([]);
  const [approval, setApproval] = useState<Approval>(null);
  const [result, setResult] = useState("");
  const [loading, setLoading] = useState(false);

  async function checkInvoices() {
    setSteps([]);
    setApproval(null);
    setResult("");
    setLoading(true);

    const response = await fetch("/api/agent", { method: "POST" });
    const reader = response.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const events = buffer.split("\n\n");
      buffer = events.pop() ?? "";

      for (const raw of events) {
        const eventMatch = raw.match(/^event: (.+)$/m);
        const dataMatch = raw.match(/^data: (.+)$/m);
        if (!eventMatch || !dataMatch) continue;

        const event = eventMatch[1];
        const data = JSON.parse(dataMatch[1]);

        if (event === "step") setSteps((prev) => [...prev, data]);
        if (event === "approval_required") setApproval(data);
        if (event === "final") setResult(data.text);
      }
    }

    setLoading(false);
  }

  // The second phase: a real decision, sent to the confirm route, which
  // is the only place in this app that ever touches a private key.
  async function decide(runId: string, approved: boolean) {
    setApproval(null);
    setLoading(true);

    const response = await fetch("/api/agent/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ runId, approved }),
    });
    const data = await response.json();
    setResult(data.text ?? data.error ?? "Something went wrong");
    setLoading(false);
  }

  return (
    <main className="max-w-xl mx-auto p-8">
      <h1 className="text-xl font-bold mb-4">Invoice Payment Agent</h1>

      <button className="border px-4 py-2 rounded" disabled={loading} onClick={checkInvoices}>
        Check overdue invoices
      </button>

      <ul className="mt-4 space-y-1 text-sm text-gray-600">
        {steps.map((s, i) => (
          <li key={i}>{s.label}{s.detail ? `: ${s.detail}` : ""}</li>
        ))}
      </ul>

      {approval && (
        <div className="mt-4 border rounded p-4 bg-yellow-50">
          <p className="font-medium">{approval.reasoning}</p>
          <p className="text-sm text-gray-700 mb-2">{approval.action}</p>
          <button className="border px-3 py-1 rounded mr-2" onClick={() => decide(approval.runId, true)}>
            Approve
          </button>
          <button className="border px-3 py-1 rounded" onClick={() => decide(approval.runId, false)}>
            Reject
          </button>
        </div>
      )}

      {result && <p className="mt-4">{result}</p>}
    </main>
  );
}
