---
sidebar_position: 4
title: RAGFlow MCP Tools
sidebar_label: RAGFlow MCP Tools
slug: /mcp_tools
sidebar_custom_props: {
  categoryIcon: LucideToolCase
}
---

# RAGFlow MCP Tools

The Go MCP runtime registers three tools. Calls run as the user selected by the endpoint's authentication mode and have a 60-second server-side timeout.

## `ragflow_retrieval`

Retrieves relevant chunks from datasets available to the authenticated user. When `dataset_ids` is omitted or empty, RAGFlow searches all datasets that user can access.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `question` | string | Yes | — | Query used for retrieval. |
| `dataset_ids` | array of strings | No | All accessible datasets | Limits retrieval to the specified datasets. |
| `document_ids` | array of strings | No | All documents in the selected datasets | Limits retrieval to the specified documents. |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `10` | From `1` through `100`; `50` or fewer is recommended to limit response size. |
| `similarity_threshold` | number | No | `0.2` | From `0.0` through `1.0`. |
| `vector_similarity_weight` | number | No | `0.3` | From `0.0` through `1.0`; controls the vector-similarity contribution relative to term similarity. |
| `keyword` | boolean | No | `false` | Enables keyword-based search when true. |
| `top_k` | integer | No | `1024` | From `1` through `1024`; maximum candidate count considered before ranking. |
| `rerank_id` | string | No | Empty | Optional reranking model ID. |
| `force_refresh` | boolean | No | `false` | Passes a metadata refresh request to the retrieval service. |

`page × page_size` must not exceed the fixed window of 512 rerank candidates. Requests beyond that window return an error.

The tool returns one text content item. Its text is a JSON object with:

- `chunks`: the retrieved chunks, supplemented with dataset names, document names, and available document metadata.
- `pagination`: `page`, `page_size`, `total_chunks`, and `total_pages`.
- `query_info`: the question, similarity threshold, vector weight, keyword-search setting, and number of searched datasets.

## `ragflow_list_datasets`

Lists datasets available to the authenticated user in descending creation-time order.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `100` | Schema accepts `1` through `1000`; the Go connector caps an individual request at 100 results. |

The tool returns one text content item containing one JSON object per line. Each object contains `id`, `name`, and `description`. The text is empty when no datasets are available.

## `ragflow_list_chats`

Lists chat assistants available to the authenticated user in descending creation-time order.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `30` | From `1` through `100`. |

The tool returns one text content item containing one JSON object per line. Each object contains `id`, `name`, and `description`. The text is empty when no chat assistants are available.

## Tool discovery

`tools/list` returns the schemas above. RAGFlow also appends the authenticated user's accessible dataset information to the retrieval and dataset-listing descriptions, and accessible chat information to the chat-listing description.

For the authoritative implementation, see the [Go tool schemas](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/tools.json), [tool registration](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/server.go), [service connector](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/connector.go), [MCP protocol handler](https://github.com/infiniflow/ragflow/blob/main/internal/handler/mcp_server.go), and [retrieval result handler](https://github.com/infiniflow/ragflow/blob/main/internal/handler/mcp_retrieval.go).
