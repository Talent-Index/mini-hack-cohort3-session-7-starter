// The same mock invoice database and condition logic from Session 6's
// payment-agent.js, unchanged. This is deliberately not an LLM call,
// the condition evaluation is plain code so it stays fully
// deterministic and auditable, same reasoning as the CLI version.

export type Invoice = {
  id: string;
  supplier: string;
  amountUsdc: number;
  dueDate: string;
  paid: boolean;
  recipient: string;
};

// daysAgo returns an ISO date string that many days before whenever
// this app actually runs. Using fixed calendar dates here would mean
// this mock data silently becomes "more overdue" every day real time
// moves forward, exactly the kind of thing that quietly breaks a demo
// months after it was written. Computing relative to Date.now() keeps
// the mock data's meaning stable regardless of when this runs.
function daysAgo(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return d.toISOString().slice(0, 10);
}

export const invoices: Invoice[] = [
  {
    id: "INV-041",
    supplier: "Supplier X",
    amountUsdc: 2,
    dueDate: daysAgo(4),
    paid: false,
    recipient: "0xAB12cd34Ef56aB12Cd34Ef56ab12Cd34Ef56aB12",
  },
  {
    id: "INV-045",
    supplier: "Supplier A",
    amountUsdc: 2,
    dueDate: daysAgo(4),
    paid: false,
    recipient: "0xAB12cd34Ef56aB12Cd34Ef56ab12Cd34Ef56aB12",
  },
  {
    id: "INV-043",
    supplier: "Supplier Y",
    amountUsdc: 1,
    dueDate: daysAgo(1),
    paid: false,
    recipient: "0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34",
  },
  {
    id: "INV-044",
    supplier: "Supplier Z",
    amountUsdc: 3,
    dueDate: daysAgo(0),
    paid: true,
    recipient: "0xeF56ab12CD34Ef56ab12cD34Ef56AB12cD34ef56",
  },
];

export function daysOverdue(dueDate: string): number {
  const diffMs = Date.now() - new Date(dueDate).getTime();
  return Math.floor(diffMs / (1000 * 60 * 60 * 24));
}

export function findOverdueInvoices(): Invoice[] {
  return invoices.filter((inv) => !inv.paid && daysOverdue(inv.dueDate) > 3);
}

const sentPaymentsLog = new Set<string>();

export function preflightChecks(inv: Invoice): string[] {
  const errors: string[] = [];
  const maxPayment = Number(process.env.MAX_PAYMENT_USDC || 500);

  if (
    !inv.recipient ||
    inv.recipient === "0x0000000000000000000000000000000000000000"
  ) {
    errors.push("invalid recipient address");
  }
  if (inv.amountUsdc > maxPayment) {
    errors.push(`amount exceeds spending limit of ${maxPayment} USDC`);
  }
  if (sentPaymentsLog.has(inv.id)) {
    errors.push(
      "payment already sent for this invoice, idempotency check failed",
    );
  }
  return errors;
}

export function markSent(invoiceId: string) {
  sentPaymentsLog.add(invoiceId);
}
