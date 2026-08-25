"use client";

import { useState } from "react";

type Step = { label: string; detail?: string };
type Approval = { runId: string; reasoning: string; action: string } | null;
type Outcome =
  | { kind: "success"; text: string; txHash?: string }
  | { kind: "rejected"; text: string }
  | { kind: "info"; text: string }
  | { kind: "error"; text: string }
  | null;

const FUJI_EXPLORER = "https://testnet.snowtrace.io/tx/";

export default function Home() {
  const [steps, setSteps] = useState<Step[]>([]);
  const [approval, setApproval] = useState<Approval>(null);
  const [result, setResult] = useState<Outcome>(null);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  function copyHash(hash: string) {
    navigator.clipboard.writeText(hash);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  function shorten(hash: string) {
    return `${hash.slice(0, 10)}…${hash.slice(-8)}`;
  }

  async function checkInvoices() {
    setSteps([]);
    setApproval(null);
    setResult(null);
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
        if (event === "final") setResult({ kind: "info", text: data.text });
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

    if (data.error) {
      setResult({ kind: "error", text: data.error });
    } else if (!approved) {
      setResult({ kind: "rejected", text: data.text });
    } else {
      setResult({ kind: "success", text: data.text, txHash: data.txHash });
    }
    setLoading(false);
  }

  return (
    <main className="max-w-xl w-full mx-auto px-6 py-16">
      <header className="mb-10">
        <div className="flex items-center gap-2 mb-3">
          <span className="h-2 w-2 rounded-full bg-accent shadow-[0_0_12px_2px] shadow-accent/60" />
          <span className="text-xs uppercase tracking-[0.2em] text-muted">
            Avalanche Fuji · Human-in-the-loop
          </span>
        </div>
        <h1 className="text-3xl font-bold tracking-tight">
          Invoice Payment <span className="text-accent">Agent</span>
        </h1>
        <p className="mt-2 text-sm text-muted">
          The agent finds overdue invoices and pauses for your approval before
          anything ever touches the chain.
        </p>
      </header>

      <button
        className="inline-flex items-center gap-2 rounded-xl bg-accent px-5 py-2.5 text-sm font-semibold text-background transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
        disabled={loading}
        onClick={checkInvoices}
      >
        {loading && (
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-background/40 border-t-background" />
        )}
        {loading ? "Working…" : "Check overdue invoices"}
      </button>

      {steps.length > 0 && (
        <ol className="mt-6 space-y-2">
          {steps.map((s, i) => (
            <li
              key={i}
              className="flex items-start gap-3 rounded-xl border border-border bg-card px-4 py-3 text-sm"
            >
              <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent/15 text-[11px] font-semibold text-accent">
                {i + 1}
              </span>
              <span>
                <span className="text-foreground">{s.label}</span>
                {s.detail ? (
                  <span className="text-muted"> — {s.detail}</span>
                ) : null}
              </span>
            </li>
          ))}
        </ol>
      )}

      {approval && (
        <div className="mt-6 rounded-2xl border border-accent/30 bg-card p-5 shadow-[0_0_30px_-12px] shadow-accent/40">
          <p className="text-xs font-semibold uppercase tracking-wider text-accent">
            Approval required
          </p>
          <p className="mt-2 font-semibold text-foreground">
            {approval.reasoning}
          </p>
          <p className="mt-1 text-sm text-muted">{approval.action}</p>
          <div className="mt-5 flex gap-3">
            <button
              className="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-background transition hover:brightness-110"
              onClick={() => decide(approval.runId, true)}
            >
              Approve &amp; send
            </button>
            <button
              className="rounded-lg border border-border px-4 py-2 text-sm font-semibold text-muted transition hover:text-foreground hover:border-foreground/30"
              onClick={() => decide(approval.runId, false)}
            >
              Reject
            </button>
          </div>
        </div>
      )}

      {result?.kind === "success" && (
        <div className="mt-6 rounded-2xl border border-emerald-500/30 bg-emerald-500/[0.06] p-5">
          <div className="flex items-center gap-2">
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-500 text-sm text-background">
              ✓
            </span>
            <p className="font-semibold text-emerald-400">Payment sent</p>
          </div>
          <p className="mt-2 text-sm text-muted">
            The transaction was submitted to Avalanche Fuji.
          </p>
          {result.txHash && (
            <div className="mt-4 rounded-xl border border-border bg-background/60 p-3">
              <p className="text-xs uppercase tracking-wider text-muted">
                Transaction hash
              </p>
              <div className="mt-1 flex flex-wrap items-center gap-3">
                <code className="font-mono text-sm text-foreground">
                  {shorten(result.txHash)}
                </code>
                <button
                  className="text-xs font-semibold text-accent hover:underline"
                  onClick={() => copyHash(result.txHash!)}
                >
                  {copied ? "Copied!" : "Copy"}
                </button>
                <a
                  className="text-xs font-semibold text-accent hover:underline"
                  href={`${FUJI_EXPLORER}${result.txHash}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  View on Snowtrace ↗
                </a>
              </div>
            </div>
          )}
        </div>
      )}

      {result?.kind === "rejected" && (
        <div className="mt-6 rounded-2xl border border-border bg-card p-5">
          <p className="font-semibold text-foreground">Payment rejected</p>
          <p className="mt-1 text-sm text-muted">{result.text}</p>
        </div>
      )}

      {result?.kind === "error" && (
        <div className="mt-6 rounded-2xl border border-red-500/30 bg-red-500/[0.06] p-5">
          <p className="font-semibold text-red-400">Something went wrong</p>
          <p className="mt-1 text-sm text-muted">{result.text}</p>
        </div>
      )}

      {result?.kind === "info" && (
        <div className="mt-6 rounded-2xl border border-border bg-card p-5">
          <p className="text-sm text-muted">{result.text}</p>
        </div>
      )}
    </main>
  );
}
