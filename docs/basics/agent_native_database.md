---
sidebar_position: 1
title: "What Is Agent Native Database?"
sidebar_label: "What Is Agent Native Database?"
slug: /what-is-agent-native-database
---

# What is an Agent Native Database?

An agent native database is a **context supply layer** that an agent harness can query directly. It stores, compiles, and governs context—claims with verbatim evidence, navigation structures, entity relations, timelines, ontologies, wikis, and memory—and hands the agent the smallest sufficient set of verified facts instead of a ranked list of passages.

It is not a RAG engine, not a vector database, and not a relational database. It is the layer that produces and delivers context, while the harness decides what enters the model window.

---

## Why it matters

Retrieval-augmented generation is becoming agentic. A search harness writes queries, reads what it finds, reformulates, and decides when to stop. This shift has largely solved the accuracy problem: a harness with only BM25, grep, and MiniMax M3 answers 85% of BrowseComp-Plus and reaches 91% on the multi-hop FRAMES benchmark in our runs, without any compiled index and without any vector index.

What the shift exposes is a context problem. An undisciplined loop repeats the same queries, rereads the same documents, and fills the window with passages that are never used. The process consumes tokens but does not become an answer.

The missing layer is **knowledge compilation**: a configurable ingestion pipeline that enriches source documents with context no chunk contains on its own. Compilation is not a better retriever. It is a supply layer.

The economics are asymmetric. A navigation tree whose leaves are claim–evidence pairs cuts total query tokens by 91% when the evidence suffices and adds 1.9% when it does not—a 38% average reduction. That asymmetry implies a 2% break-even sufficiency rate, while the measured rate on FRAMES is near 43%. Compilation is not a bet on hits; it is a trade that pays on most corpora.

---

## How it works

### Three layers, one division of labor

**Model layer.** The large language model that reasons and answers. It is tightly coupled with the harness and evolves with it.

**Harness layer.** The agent loop that orchestrates: it decides what to search, what to read, and when to stop. It can be RAGFlow's reference harness, an external framework such as LangGraph, or a custom loop.

**Context layer.** The agent native database. It supplies compiled, governed context through a query interface, and it never decides what the model should conclude.

The division is one sentence: **the harness orchestrates, the database supplies.**

![Model, harness, and the agent's context layer](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_native_db.svg)

### Inside the context layer

The context layer behaves like a database, not a prompt stuffer:

- **Query interface.** Tools such as search, grep, read, navigation, graph, and memory. It returns candidates and evidence, never conclusions.
- **Collections.** Evidence (claim + verbatim span), navigation trees, entity graphs, timelines, LLM wikis, and memory—each typed, versioned, and queryable.
- **Governance.** Versioned snapshots, lineage, access control, and audit trails. Every delivered claim resolves to a live source span.

### From raw data to context

Raw data—documents, tables, logs, sessions, databases, APIs—stays raw. It is never overwritten. An **ingest and compile** pipeline parses, chunks, enriches, validates, assembles, and registers typed artifacts. Those artifacts are the data the agent queries.

Two rules keep the layer trustworthy:

- **Raw and derived channels are separate.** Derived artifacts are read-only and never written back into the source. A model reading a claim can always fall back to the original text.
- **Compilation is query-independent.** Understanding moves from query time to index time. Compile once, reuse across queries, artifacts, and sessions.

---

## Key concepts

**Knowledge compilation.** A query-independent transformation that turns a corpus into typed, versioned artifacts with lineage to the exact source spans.

**Claim–evidence pair.** The unit of compilation: a normalized claim, a verbatim evidence span, source offsets, and validity metadata. A claim without a locatable span is not stored.

**Navigation tree with claim leaves.** The tree organizes; the leaves carry evidence. Structure alone adds no accuracy—under a competent harness, summary trees, PageIndex outlines, wikis, and graphs produced no detectable accuracy gain on FRAMES, and a PageIndex outline degraded at scale.

**Two version lines.** Pipeline versions govern execution (“which graph this document runs on”); compilation versions govern provenance (“which template produced this knowledge”). Both are mechanisms, not conventions.

**Evidence gate.** A decision model that answers one question before generation: is this block of evidence sufficient to send? Its value is in deciding what not to send, not in ranking what to send.

---

## How it differs from RAG and vector databases

|                  | RAG engine      | Vector database    | Agent native database              |
|------------------|-----------------|--------------------|------------------------------------|
| Primary consumer | Human user      | Retrieval pipeline | Agent harness                      |
| Delivers         | Ranked passages | Similarity hits    | Smallest sufficient context        |
| Unit of value    | Recall@k        | Embedding quality  | Token cost, provenance, governance |
| Structure        | Chunks + index  | Vectors            | Typed, versioned artifacts         |
| Query form       | Search box      | Similarity query   | Tool calls                         |

A RAG engine answers “what passages are relevant?” A vector database answers “what is nearest?” An agent native database answers “what is the smallest sufficient set of verified facts for this step, and where did each fact come from?”

---

## What it deliberately does not do

- **It does not replace search or grep.** Search finds candidates; compilation helps the harness use less context after it finds them.
- **It does not claim structure improves accuracy.** Our measurements say otherwise. Structure organizes; evidence supplies.
- **It does not default to a vector index.** Inside a harness, a dense leg added no signal the loop lacked, and in our runs it lowered accuracy.
- **It does not lock in a single harness.** RAGFlow's harness is a reference implementation. The database serves any harness that can call its tools.
- **It is not a relational database.** There is no query language and no schema enforcement. The harness, not the store, decides what a given question reads.

---

## Getting started

RAGFlow 1.0 is the first open-source implementation of an agent native database. It is written in Go, ships with a configurable ingestion pipeline and a stateful retrieval harness, and is API-compatible with 0.27.x for a smooth upgrade.

- GitHub: https://github.com/infiniflow/ragflow
- Paper: *Context as Infrastructure: Knowledge Compilation for Retrieval in Agentic Systems with Harnesses* https://arxiv.org/abs/2610.41172
