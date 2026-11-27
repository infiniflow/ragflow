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

The Go MCP runtime registers five tools. Calls run as the user selected by the endpoint's authentication mode and have a 60-second server-side timeout.

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
| `page_size` | integer | No | `100` | From `1` through `100`; a single call returns at most 100 datasets. |

The tool returns one text content item containing one JSON object per line. Each object contains `id`, `name`, and `description`. The text is empty when no datasets are available.

## `ragflow_list_chats`

Lists chat assistants available to the authenticated user in descending creation-time order.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `30` | From `1` through `100`. |

The tool returns one text content item containing one JSON object per line. Each object contains `id`, `name`, and `description`. The text is empty when no chat assistants are available.

## `ragflow_list_documents`

Lists readable documents in an accessible dataset in descending creation-time order. Use the returned document IDs with `ragflow_retrieval` for scoped search or `ragflow_list_chunks` to read a whole document.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `dataset_id` | string | Yes | — | Non-empty ID of an accessible dataset. |
| `keywords` | string | No | Empty | Filters document names by keyword. |
| `name` | string | No | Empty | Filters by exact document name; combines with `keywords` when both are supplied. |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `30` | From `1` through `100`. |

The tool returns one text content item containing a JSON object with `documents`, `total`, `page`, and `page_size`. Each document contains `id`, `name`, `dataset_id`, `type`, `size`, `chunk_count`, `token_count`, `ingestion_status`, `create_date`, `update_date`, and `meta_fields`. `ingestion_status` is the Go API's parse status (for example, `UNSTART`, `RUNNING`, `COMPLETED`, or `FAILED`). `total` counts matching readable documents before pagination; an empty page returns `documents: []`.

Missing IDs, invalid pagination, and inaccessible datasets return errors. Document visibility is applied before counting and pagination, using the same permission scope as the HTTP document-list API.

## `ragflow_list_chunks`

Reads chunks of one fully parsed document in document order. No question, similarity threshold, or relevance ranking is applied, and the retrieval tool's 512-candidate window does not apply. Page through the results to read the whole document.

| Argument | Type | Required | Default | Valid values and behavior |
| --- | --- | --- | --- | --- |
| `dataset_id` | string | Yes | — | Non-empty ID of an accessible dataset. |
| `document_id` | string | Yes | — | Non-empty ID of a readable document belonging to `dataset_id`. |
| `page` | integer | No | `1` | Minimum `1`. |
| `page_size` | integer | No | `30` | From `1` through `100`. |

The tool returns one text content item containing a JSON object with `chunks`, `doc`, `total`, `page`, and `page_size`. It preserves the HTTP chunk-list fields, including `id`, `content`, available `positions`, and `important_keywords`. `doc` includes the document's `name` and `chunk_count`. `total` is the chunk-list total before pagination. Disabled chunks are included, with their `available` flag.

Ordering follows the existing chunk-list service: ascending `chunk_order_int`, then page number and vertical position, then descending creation timestamp. For older chunks without an explicit chunk order, the position fields provide the reading order. As with HTTP pagination, changes to a document during reading can shift page boundaries.

Missing IDs, invalid pagination, inaccessible resources, and a document that belongs to another dataset return errors. A document whose `ingestion_status` is not `COMPLETED` returns an explicit “not fully parsed” error with its current status.

## Tool discovery

`tools/list` returns the schemas above. RAGFlow also appends the authenticated user's accessible dataset information to the retrieval, dataset-listing, document-listing, and chunk-listing descriptions, and accessible chat information to the chat-listing description.

For the authoritative implementation, see the [Go tool schemas](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/tools.json), [tool registration](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/server.go), [service connector](https://github.com/infiniflow/ragflow/blob/main/internal/mcp/connector.go), [MCP protocol handler](https://github.com/infiniflow/ragflow/blob/main/internal/handler/mcp_server.go), and [retrieval result handler](https://github.com/infiniflow/ragflow/blob/main/internal/handler/mcp_retrieval.go).
