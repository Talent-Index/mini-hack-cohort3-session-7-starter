"""
Retrieval-Augmented Generation (RAG), end to end.

Five steps: chunk your documents (skipped here, our five docs are
already short enough to index whole), embed each one, store the
embeddings in Chroma, retrieve the closest matches at query time, and
generate an answer grounded in those matches, with citations.

A note on embeddings: the official Chroma client computes embeddings
locally, in this process, using a small bundled model (all-MiniLM-L6-v2).
The first time you run this, Chroma downloads that model, a few tens of
megabytes, so the very first call is slower and needs internet access.
After that it's cached and runs offline.
"""

import asyncio
import os
import sys

import chromadb
from dotenv import load_dotenv

from model_provider import create_model_client

load_dotenv()

# Five documents, five sources, matching what the curriculum asks for.
# In your own project, swap these for your actual docs, README, API
# reference, whatever your agent should be able to answer questions
# about.
DOCUMENTS = [
    {"id": "avax-cchain", "text": "The C-Chain is Avalanche's EVM-compatible smart contract chain. This is where Solidity contracts, USDC, and most DeFi activity actually run."},
    {"id": "chainkit-readme", "text": "ChainKit is Avalanche's official on-chain data SDK. It provides structured, paginated access to wallet transaction history, and can also run as its own MCP server."},
    {"id": "daraja-api", "text": "The Daraja API is Safaricom's integration platform for M-Pesa. It exposes endpoints for STK push, C2B, B2C, and transaction status queries."},
    {"id": "ethers-core", "text": "ethers.js is a library for interacting with Ethereum-compatible chains. It handles wallets, providers, contract calls, and transaction signing."},
    {"id": "project-readme", "text": "Mini Hack Cohort 3 starter repo: a provider-agnostic agent starter supporting four LLM providers through one shared interface."},
]

SUMMARY_SYSTEM_PROMPT = "You are a documentation assistant. Only answer from the provided context."


async def build_knowledge_base(client: chromadb.AsyncClientAPI):
    # Step 1 (chunk, already done above) + steps 2 and 3 (embed and
    # store). get_or_create_collection makes this safe to run more than
    # once, it won't duplicate the collection on a second run.
    collection = await client.get_or_create_collection(name="mini-hack-docs")

    await collection.add(
        ids=[d["id"] for d in DOCUMENTS],
        documents=[d["text"] for d in DOCUMENTS],
        # no embeddings= argument: we're letting Chroma's default
        # embedding function handle step 2 for us, this is the whole
        # point of using a vector database instead of writing embedding
        # code by hand.
    )

    return collection


async def query_knowledge_base(collection, question: str) -> list:
    # Step 4: retrieve. Embeds the question the same way, using the same
    # default embedding function, then finds the closest chunks by
    # cosine similarity under the hood.
    results = await collection.query(query_texts=[question], n_results=3)

    # Chroma returns parallel lists (one per query, we only sent one),
    # zip them back into a friendlier shape: one dict per chunk.
    return [
        {"text": text, "id": results["ids"][0][i], "distance": results["distances"][0][i]}
        for i, text in enumerate(results["documents"][0])
    ]


def build_grounded_prompt(question: str, chunks: list) -> str:
    # Step 5, part one: build a prompt that forces the model to answer
    # only from what we retrieved, and to cite which chunk it used.
    # This single instruction is what turns "the model made something
    # up" into "the model told me exactly where this came from."
    context = "\n\n".join(f"[{i + 1}] (source: {c['id']})\n{c['text']}" for i, c in enumerate(chunks))

    return (
        "Answer the question using only the context below. "
        "Cite which source number supports each claim. "
        "If the context does not cover the question, say you don't know.\n\n"
        f"Context:\n{context}\n\n"
        f"Question: {question}"
    )


async def main():
    question = sys.argv[1] if len(sys.argv) > 1 else "What is the C-Chain?"

    # Chroma connection, configurable so this same code works whether
    # Chroma is running locally (CHROMA_HOST defaults to localhost) or as
    # a sibling service in Docker Compose (CHROMA_HOST=chroma there).
    chroma_host = os.environ.get("CHROMA_HOST", "localhost")
    chroma_port = int(os.environ.get("CHROMA_PORT", "8000"))
    chroma_client = await chromadb.AsyncHttpClient(host=chroma_host, port=chroma_port)
    collection = await build_knowledge_base(chroma_client)
    chunks = await query_knowledge_base(collection, question)

    # Step 5, part two: call the model and print the grounded answer.
    model_client = create_model_client()
    response = await model_client.generate_text(
        SUMMARY_SYSTEM_PROMPT,
        [{"role": "user", "content": build_grounded_prompt(question, chunks)}],
    )
    print(response.text)


if __name__ == "__main__":
    asyncio.run(main())
