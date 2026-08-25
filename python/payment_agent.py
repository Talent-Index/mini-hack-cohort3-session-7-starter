"""
The Invoice Payment Agent, end to end, with full safety architecture.

Five steps: define the condition (invoice overdue, unpaid), evaluate it
against a mock invoice database, present the reasoning and get human
approval, execute the payment as a real USDC transfer on Fuji, and log
the outcome, approved or not. Wrapped in four required safety pillars:
pre-flight checks, a spending limit, an idempotency check, and an audit
log entry for every decision.

See kill_switch.sol in this folder for the fourth pillar, the on-chain
kill switch, that one lives in the smart contract, not this script.
"""

import json
import os
from datetime import datetime, timedelta, timezone

from dotenv import load_dotenv
from web3 import Web3

load_dotenv()

FUJI_RPC = "https://api.avax-test.network/ext/bc/C/rpc"
MAX_PAYMENT_USDC = float(os.environ.get("MAX_PAYMENT_USDC", "500"))  # spending limit, per transaction
SENT_PAYMENTS_LOG = set()  # idempotency, in memory for this demo

USDC_ABI = [
    {
        "name": "transfer",
        "type": "function",
        "stateMutability": "nonpayable",
        "inputs": [{"name": "to", "type": "address"}, {"name": "amount", "type": "uint256"}],
        "outputs": [{"type": "bool"}],
    }
]

# Mock invoice database, standing in for a real one, same shape you'd
# get back from a real accounts-payable system or database query.
def _days_ago(n: int) -> str:
    """A date string n days before whenever this script actually runs.

    Using fixed calendar dates here would mean this mock data silently
    becomes "more overdue" every day real time moves forward, exactly
    the kind of thing that quietly breaks a demo months after it was
    written. Computing relative to now keeps the mock data's meaning
    stable regardless of when this runs.
    """
    return (datetime.now(timezone.utc) - timedelta(days=n)).date().isoformat()


INVOICES = [
    {"id": "INV-042", "supplier": "Supplier X", "amount_usdc": 50, "due_date": _days_ago(4), "paid": False, "recipient": "0xAB12cd34Ef56aB12Cd34Ef56ab12Cd34Ef56aB12"},
    {"id": "INV-043", "supplier": "Supplier Y", "amount_usdc": 120, "due_date": _days_ago(1), "paid": False, "recipient": "0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34"},
    {"id": "INV-044", "supplier": "Supplier Z", "amount_usdc": 30, "due_date": _days_ago(0), "paid": True, "recipient": "0xeF56ab12CD34Ef56ab12cD34Ef56AB12cD34ef56"},
]


def days_overdue(due_date: str) -> int:
    due = datetime.fromisoformat(due_date).replace(tzinfo=timezone.utc)
    now = datetime.now(timezone.utc)
    return (now - due).days


# Step 1 and 2: define the condition, evaluate it against the mock database.
def find_overdue_invoices() -> list:
    return [inv for inv in INVOICES if not inv["paid"] and days_overdue(inv["due_date"]) > 3]


# Safety pillar 1: pre-flight checks, run before any payment goes out,
# no exceptions. Safety pillar 2 (spending limit) and the idempotency
# half of pillar 1 are both checked here too.
def preflight_checks(invoice: dict) -> list:
    errors = []
    if not invoice["recipient"] or invoice["recipient"] == "0x0000000000000000000000000000000000000000":
        errors.append("invalid recipient address")
    if invoice["amount_usdc"] > MAX_PAYMENT_USDC:
        errors.append(f"amount exceeds spending limit of {MAX_PAYMENT_USDC} USDC")
    if invoice["id"] in SENT_PAYMENTS_LOG:
        errors.append("payment already sent for this invoice, idempotency check failed")
    return errors


# Step 3: present reasoning, get human approval before anything executes.
def confirm_payment(invoice: dict, overdue_days: int) -> bool:
    print(f"\nInvoice {invoice['id']} from {invoice['supplier']} is {overdue_days} days overdue.")
    print(f"I intend to send {invoice['amount_usdc']} USDC to {invoice['recipient']}.")
    answer = input("Do you approve? (y/n): ")
    return answer.strip().lower() == "y"


# Step 4: on approval, actually execute the payment on Fuji.
def send_payment(invoice: dict) -> str:
    w3 = Web3(Web3.HTTPProvider(FUJI_RPC))
    account = w3.eth.account.from_key(os.environ["AGENT_PRIVATE_KEY"])

    usdc = w3.eth.contract(address=os.environ["FUJI_USDC_ADDRESS"], abi=USDC_ABI)

    # USDC on Fuji is an ERC-20, transfer() takes the recipient and an
    # amount in the token's smallest unit, 6 decimals for USDC, not 18
    # like AVAX, this trips people up constantly.
    amount = int(invoice["amount_usdc"] * 10**6)

    nonce = w3.eth.get_transaction_count(account.address)
    built_tx = usdc.functions.transfer(invoice["recipient"], amount).build_transaction({
        "from": account.address,
        "nonce": nonce,
        "gas": 100000,
        "gasPrice": w3.eth.gas_price,
        "chainId": 43113,  # Fuji
    })

    signed = account.sign_transaction(built_tx)
    tx_hash = w3.eth.send_raw_transaction(signed.raw_transaction)
    w3.eth.wait_for_transaction_receipt(tx_hash)
    return tx_hash.hex()


# Safety pillar 4 (the software half, the kill switch itself lives on
# chain, see kill_switch.sol): log every decision, approved or not.
def log_decision(invoice_id: str, reasoning: str, approved: bool, tx_hash: str = None) -> None:
    entry = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "invoice_id": invoice_id,
        "reasoning": reasoning,
        "approved": approved,
        "tx_hash": tx_hash,
    }
    print(json.dumps(entry))
    # production: append to a durable log store, not just stdout


def main():
    overdue = find_overdue_invoices()

    for invoice in overdue:
        overdue_days = days_overdue(invoice["due_date"])
        reasoning = f"{invoice['id']} is {overdue_days} days overdue, condition met (overdue > 3 days)"

        preflight_errors = preflight_checks(invoice)
        if preflight_errors:
            log_decision(invoice["id"], f"Pre-flight failed: {', '.join(preflight_errors)}", approved=False)
            continue

        approved = confirm_payment(invoice, overdue_days)
        if not approved:
            log_decision(invoice["id"], reasoning, approved=False)
            continue

        tx_hash = send_payment(invoice)
        SENT_PAYMENTS_LOG.add(invoice["id"])
        log_decision(invoice["id"], reasoning, approved=True, tx_hash=tx_hash)
        print(f"Payment sent, transaction hash: {tx_hash}")


if __name__ == "__main__":
    main()
