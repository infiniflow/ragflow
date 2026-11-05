# REST API pagination

The Go API and Admin servers share pagination policy in
`internal/common/pagination.go`. All existing REST endpoints accepting
`page_size`, and JSON retrieval endpoints accepting `size`, reject sizes outside
**1–100**, including zero and negative values. `100` is accepted; `101` is rejected.

Pages start at **1**. Omitted fields use defaults; supplied empty, malformed,
fractional, or repeated query pagination fields are rejected. Page offsets that
would overflow an integer are rejected. Errors use each endpoint's existing
HTTP status and business error envelope. Clients must check the response body's
`code`, even when HTTP status is 200.

## Defaults and audit

Paths below are relative to `/api/v1`. All rows now enforce the shared 100-item
maximum. Existing finite defaults are preserved; formerly unpaginated defaults
now use page 1 and 30 items.

| Endpoint | Size field | Default size | Behavior before this change |
| --- | --- | ---: | --- |
| `GET /datasets` | query `page_size` | 30 | No upper bound |
| `GET /datasets/:id/documents`, `GET /documents` | query `page_size` | 10 | Oversized values replaced with 10 |
| `GET /datasets/:id/documents/:doc/chunks` | query `page_size` | 30 | No upper bound |
| `POST /datasets/search`, `POST /datasets/:id/search`, `POST /retrieval` | JSON `page_size` / `size` | 30 | No upper bound; `page_size` takes precedence |
| `GET /datasets/:id/ingestions` (including `log_type=datasource`) | query `page_size` | 30 | No handler upper bound; downstream pagination varied by log type |
| `GET /chats`, `GET /agents`, `GET /searches` | query `page_size` | 30 | Omitted or invalid pagination could return every matching row |
| `GET /chats/:id/sessions` | query `page_size` | 30 | No upper bound; negative size disabled pagination; zero returned an empty list |
| `GET /agents/:id/sessions` | query `page_size` | 30 | No upper bound |
| `GET /all-models`, `GET /admin/all-models` | query `page_size` | 30 | Omitted pagination returned all models; partial/out-of-range pages could panic or return the whole catalog |
| `GET /files` | query `page_size` | 15 | Oversized values clamped to 100 |
| `GET /folders/:id/commits`, workspace and dataset commit aliases | query `page_size` | 15 | No upper bound |
| `GET /memories` | query `page_size` | 50 | No upper bound |
| `GET /memories/:id` | query `page_size` | 50 | Already rejected values over 100 |
| `GET /connectors/:id/logs`, `GET /connectors/sync-logs` | query `page_size` | 15 | Oversized values replaced downstream; zero could return all sync logs |
| `GET /mcp/servers` | query `page_size` | 30 | Rejected values over 100, but omitted/nonpositive pagination could return all servers |
| `GET /compilation-template-groups` | query `page_size` | 30 | No upper bound |
| `GET /datasets/:id/artifacts` | query `page_size` | 30 | Oversized values clamped to 100 |
| `POST /skills/search` | JSON `page_size` | 10 | No upper bound |
| `POST /searchbots/retrieval` | JSON `size` | 30 | No upper bound |
| `GET /admin/users` | query `page_size` | 10 | No upper bound |

All model-list pages now use the same one-based page convention as other lists.
A partial final page returns the remaining models; a page past the end returns
an empty list. Filters, sorting, totals, and response shapes retain their existing
contracts. Filter/aggregation modes return their aggregation payload rather than
a page of records, while still validating supplied pagination fields.

## Other collection APIs

This cap applies to existing `page_size` / `size` contracts. It does not add
pagination fields to every array-valued response. The audit also found:

- Collections without pagination: connectors, chat channels, skill spaces,
  tenant memberships, API keys/tokens,
  and agent versions and tags. These continue returning their existing collections.
- Catalogs and structured results: providers/instances/models, model groups,
  pipeline and compilation templates, plugin tools,
  metadata, commit files/trees, artifact topics/structures/navigation/skill trees,
  service/config/environment/ingestor inventories, and admin queue messages
  retain their existing response contracts.
- Separate count/cursor contracts: ingestion messages use `limit` (default 200,
  maximum 500); document structure claims use `offset`/`limit` (default 20,
  clamped to 100); recent memory messages use `limit`; search/rerank candidate
  counts such as `top_k` remain independent of result pagination.

## Implementation

Use `common.ParsePagination(query, defaultSize)` at the REST boundary. It owns
query parsing, positive page validation, size validation, and overflow checks.
Use `common.ValidatePagination(page, pageSize, size)` for optional JSON fields.
Both size aliases are checked when both are provided, including an alias that
will not be used. Defaults and the maximum are declared only in the shared file;
endpoint-specific constants preserve established finite defaults.

Internal batch operations can still request larger batches or all records when
their own contract requires it. REST validation runs before passing pagination
to those operations.
