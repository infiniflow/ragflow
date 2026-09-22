---
sidebar_position: 12
title: RAGFlow MCP Tools
sidebar_label: RAGFlow MCP Tools
slug: /mcp_tools
sidebar_custom_props: {
  categoryIcon: LucideToolCase
}
---

# RAGFlow MCP Tools

The Go MCP runtime registers three tools:

| Tool | Purpose | Main arguments |
| --- | --- | --- |
| `ragflow_retrieval` | Retrieve relevant chunks from accessible datasets. | Required `question`; optional `dataset_ids`, `document_ids`, and retrieval controls such as `page`, `page_size`, `similarity_threshold`, `vector_similarity_weight`, `top_k`, `rerank_id`, `keyword`, and `force_refresh`. Omitting or emptying `dataset_ids` searches all accessible datasets. |
| `ragflow_list_datasets` | List accessible datasets, including IDs, names, and descriptions. | Optional `page` and `page_size`. |
| `ragflow_list_chats` | List accessible chat assistants, including IDs, names, and descriptions. | Optional `page` and `page_size`. |

The `tools/list` response adds the current user's accessible dataset or chat information to tool descriptions. For the authoritative tool schemas, see [Go tool definitions](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/tools.json) and [registration](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/server.go). The [Go MCP handler](https://github.com/infiniflow/ragflow/blob/main/internal/handler/mcp_server.go) connects these tools to RAGFlow services, and the [router](https://github.com/infiniflow/ragflow/blob/main/internal/router/router.go) registers the API endpoint.

For `ragflow_retrieval`, `page × page_size` must not exceed the fixed window of 512 rerank candidates. Requests beyond that window return an error; narrow the page or page size. `ragflow_list_datasets` caps each requested page at 100 results.
