---
sidebar_position: 1
slug: /deepwiki
sidebar_custom_props: {
  categoryIcon: LucideBookOpen
}
---

# Explore RAGFlow on DeepWiki

An AI-generated, always-up-to-date knowledge base for understanding RAGFlow's codebase — designed for developers doing secondary development or deep-diving into RAGFlow's internals.

---

## What is DeepWiki?

[DeepWiki](https://deepwiki.com) is an AI-powered tool that automatically reads a GitHub repository's source code, tests, and documentation to produce a structured, interactive wiki. It maps out architecture diagrams, module relationships, data flows, and design rationale — all without requiring manual documentation work.

## The RAGFlow DeepWiki page

The RAGFlow project is indexed at:

**[https://deepwiki.com/infiniflow/ragflow](https://deepwiki.com/infiniflow/ragflow)**

## Target audience

This resource is primarily intended for:

- **Secondary developers** who want to extend or customize RAGFlow (e.g., add a new document parser, integrate a new LLM provider, or modify the retrieval pipeline).
- **Contributors** who need to understand how a specific module fits into the overall architecture before filing a PR.
- **Researchers and engineers** who want to study RAGFlow's internal design principles — chunking strategies, embedding pipelines, graph-based retrieval, and agent orchestration.

:::tip NOTE
For general usage of RAGFlow (configuring knowledge bases, running chat, etc.), the [Guides](../guides/) section is a better starting point.
:::

## What you can find on DeepWiki

| Topic | What to look for |
|---|---|
| **Overall architecture** | High-level component diagram showing how `api/`, `rag/`, `deepdoc/`, `agent/`, and `web/` relate to each other |
| **Document ingestion pipeline** | How files flow from upload → parsing (`deepdoc/`) → chunking → embedding → storage |
| **Retrieval pipeline** | How queries are processed, how hybrid search (keyword + vector) works, and how reranking is applied |
| **Agent framework** | How `agent/` orchestrates multi-step reasoning, tool calling, and memory |
| **LLM / Embedding abstractions** | How `rag/llm/` wraps different model providers behind a unified interface |
| **API layer** | How `api/apps/` Blueprint routes map to internal service calls |

## Using DeepWiki alongside local development

When you are making changes to the codebase, DeepWiki can help you quickly answer questions such as:

- *"Where is the entry point for task execution?"*
- *"Which class handles PDF page segmentation?"*
- *"How does the knowledge graph retrieval differ from the dense vector path?"*

You can also ask DeepWiki questions in natural language using its built-in chat interface — it will ground its answers in the actual source code.

## Keeping the wiki current

DeepWiki re-indexes the repository automatically when the upstream `main` branch is updated. If you notice the indexed content lagging behind a recent release, you can trigger a manual re-index from the DeepWiki page.

## Related resources

- [Launch service from source](./launch_ragflow_from_source.md) — set up a local RAGFlow development environment.
- [Build RAGFlow Docker image](./build_docker_image.mdx) — build a custom image after code changes.
- [Contribution guidelines](./contributing.md) — how to file a PR once you understand the codebase.
