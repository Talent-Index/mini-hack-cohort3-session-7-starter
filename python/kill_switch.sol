// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

// The fourth safety pillar: an on-chain kill switch. Everything else in
// this repo (pre-flight checks, spending limits, the audit log) lives
// in application code, which means it only protects you if that code is
// actually running. This contract is the backstop: even if your agent's
// process is compromised, misbehaving, or just running code you no
// longer trust, one transaction from the owner's wallet stops every
// agent-initiated payment cold, on chain, immediately.
//
// This file was written using standard, well-established Solidity
// patterns (an onlyOwner modifier, a paused boolean guard), the same
// shape used across countless production contracts. There was no
// Solidity compiler available in the environment this was built in, so
// unlike the rest of this repo's code, this file was not compiled or
// deployed during development, only carefully hand-checked against
// well-known syntax. Compile and test this yourself, on Remix or with
// Hardhat/Foundry locally, before you deploy it, same as you should for
// any contract that will hold real authority over real payments.
contract PaymentAgentController {
    address public owner;
    bool public paused;

    event PaymentAgentPaused(address indexed by);
    event PaymentAgentUnpaused(address indexed by);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    modifier whenNotPaused() {
        require(!paused, "agent payments are paused");
        _;
    }

    constructor() {
        owner = msg.sender;
    }

    // The kill switch itself. Only the owner can call this, and once
    // paused, every function guarded by whenNotPaused reverts, no
    // exceptions, until unpauseAgentPayments is called.
    function pauseAgentPayments() external onlyOwner {
        paused = true;
        emit PaymentAgentPaused(msg.sender);
    }

    function unpauseAgentPayments() external onlyOwner {
        paused = false;
        emit PaymentAgentUnpaused(msg.sender);
    }

    // Transfers ownership, e.g. to a multisig, so the kill switch itself
    // isn't a single point of failure tied to one private key.
    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "new owner cannot be the zero address");
        owner = newOwner;
    }

    // Example of how a real payment-triggering function would be
    // guarded. Your actual agent-facing entry point (however you build
    // it, a relayer, a backend service, a separate contract that calls
    // into this one) should be gated by whenNotPaused exactly like this.
    function agentInitiatedPayment(address token, address to, uint256 amount) external whenNotPaused {
        // ... actual payment logic, e.g. IERC20(token).transfer(to, amount)
        // left as an integration point, this contract's job is the
        // safety gate, not the payment logic itself.
    }
}
