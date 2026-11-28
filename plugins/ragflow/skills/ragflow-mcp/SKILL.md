---
name: ragflow-mcp
description: >-
  Connect an agent to RAGFlow over MCP and use its tools: ragflow_retrieval,
  ragflow_list_datasets and ragflow_list_chats. Covers the /api/v1/mcp
  endpoint vs the optional MCP listener, host vs self-host authentication,
  Docker exposure, and retrieval parameters. Use when an agent should search
  RAGFlow datasets for grounded context.
---

# RAGFlow over MCP

References: `docs/develop/mcp/use_ragflow_as_mcp_server.md`,
`docs/develop/mcp/mcp_tools.md`, `docs/develop/mcp/mcp_client_example.md`.

## Endpoints

| Endpoint | Notes |
|----------|-------|
| `POST http://<host>:9380/api/v1/mcp` | Always on with the Go API. Streamable HTTP. Each client sends `Authorization: Bearer <RAGFlow API key>`. No SSE. |
| `http://<host>:9382/mcp` (optional listener) | Enable with `mcp.enabled` / `RAGFLOW_MCP_ENABLED=true`. Legacy SSE at `/sse` + `/messages/` if enabled. |

The plugin's `.mcp.json` points at `http://127.0.0.1:9380/api/v1/mcp` and
reads the key from `RAGFLOW_API_KEY`. Change the URL for a remote server.

## Listener auth modes

- `host`: every request authenticates as its own client (Bearer,
  `X-API-Key`, `Api-Key` or `api_key` headers). Prefer this when several
  people connect.
- `self-host` (default): all clients act as the user of
  `RAGFLOW_MCP_HOST_API_KEY`; startup fails if it is empty. Only for
  trusted clients.

In Docker, set `RAGFLOW_MCP_HOST=0.0.0.0` and connect to
`http://<docker-host>:${SVR_MCP_PORT}/mcp`. That exposes the port to the
network, so firewall it. Keep keys out of command lines and committed files.

## Tools

- `ragflow_list_datasets(page=1, page_size<=100)`: one JSON object per line
  with `id`, `name`, `description`.
- `ragflow_list_chats(page=1, page_size<=100)`: same shape for chat
  assistants.
- `ragflow_retrieval(question, dataset_ids?, document_ids?, page,
  page_size, similarity_threshold=0.2, vector_similarity_weight=0.3,
  keyword=false, top_k=1024, rerank_id?)`: omitting `dataset_ids` searches
  every accessible dataset. Returns JSON with `chunks`, `pagination` and
  `query_info`.

## Using retrieval well

1. List datasets first and pass `dataset_ids` to keep results focused.
2. Keep `page_size` at 50 or less; `page × page_size` must stay within the
   512-candidate rerank window.
3. Raise `similarity_threshold` if results are noisy; turn on `keyword` for
   exact identifiers.
4. Cite the document names from each chunk when answering.

Calls time out after 60 seconds on the server.
