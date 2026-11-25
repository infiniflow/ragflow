# Validate AnySearch

Configure [Chat](../guides/chat/chat_configuration.md) or the [Agent tools](../guides/agent/use_anysearch.md) through the normal application settings. The tests below exercise the Go implementation. Prepare native dependencies as described in [Launch RAGFlow from source](./launch_ragflow_from_source.md).

## Local contracts

```bash
bash build.sh --test ./internal/service ./internal/agent/tool ./internal/agent/component -run 'AnySearch|WebSearchRequest'
```

These tests require no external services. They cover configuration, request mapping, result preservation, errors, references and bounded diagnostics. Catalog fixtures exercise transport contracts; they do not substitute for actual search results.

## Live service contracts

```bash
RAGFLOW_TEST_ANYSEARCH=1 bash build.sh --test-integration-go ./internal/service ./internal/agent/tool -run 'AnySearch.*Live' -v -timeout 30m
```

Leave `ANYSEARCH_TEST_API_KEY` unset for anonymous access. Set it through the environment to also exercise authenticated requests. Never put a key in a test fixture, command saved to source control or captured log.

Run the larger catalog matrix separately:

```bash
RAGFLOW_TEST_ANYSEARCH_CATALOG=1 bash build.sh --test-integration-go ./internal/agent/tool -run TestAnySearchCatalogLive -v -timeout 90m
```

The catalog matrix uses real queries and documented parameter values. It fails errors and empty results. Set `RAGFLOW_ANYSEARCH_CATALOG_RECEIPTS` to a private directory to retain requests, statuses, request IDs and delivered results. Review returned identities and source scope against the current catalog; a successful HTTP response alone does not prove a filter worked. Keep vendor replies outside committed fixtures.

## Saved workflows with real inference

Run a local Go API with its database, search engine and other configured services. Use a dedicated validation account and functioning OpenAI-compatible chat and embedding servers. The chat model must support automatic tool calls. Set these environment variables:

| Variable | Value |
| --- | --- |
| `RAGFLOW_TEST_API_URL` | Local Go API origin, such as `http://127.0.0.1:9380` |
| `RAGFLOW_TEST_API_TOKEN` | The validation account's authorization token |
| `RAGFLOW_TEST_LLM_BASE_URL` | Chat server base URL, including `/v1` where required |
| `RAGFLOW_TEST_LLM_MODEL` | Chat model name exposed by that server |
| `RAGFLOW_TEST_LLM_API_KEY` | Optional chat server Bearer key, added only by the forwarding observer |
| `RAGFLOW_TEST_LLM_MAX_TOKENS` | Optional positive output-token budget; default `1024` |
| `RAGFLOW_TEST_EMBEDDING_BASE_URL` | Embedding server base URL |
| `RAGFLOW_TEST_EMBEDDING_MODEL` | A 384-dimensional embedding model name |

```bash
RAGFLOW_TEST_ANYSEARCH_E2E=1 bash build.sh --test-e2e ./internal/service -run TestAnySearchRealModelE2E -v -timeout 30m
```

The test creates model instances, a dataset and a document, then manually indexes a unique chunk through the normal chunk API. It checks saved standard and Agentic Chat, standalone search and extraction components, and an Agent's actual tool invocation. Agentic Chat saves a focused corpus-search/read tool set through `prompt_config.agentic_rag`; the runtime adds configured web search. A forwarding observer checks real inference inputs and responses; it never supplies model output. Created workflows, chats, datasets and model instances are removed during cleanup. Document ingestion is a separate path.

Set `RAGFLOW_ANYSEARCH_E2E_RECEIPTS` to a private directory to capture model exchanges, responses and persisted references for failure analysis. The test refuses to write a receipt containing its authorization token, configured AnySearch key or chat server key. The chat server key stays in the observer's upstream Authorization header; model instances are saved with an empty API key, and model request bodies are forwarded unchanged.

Run the API and test on the same host so the API can reach the loopback forwarding observer. The observer can forward to a local or hosted chat server; the API must also reach the configured embedding server. No live test runs in the default unit tier.

## Saved Canvas catalog selections

Use the same dedicated local API account to exercise explicit vertical and source selections without a model. Set `RAGFLOW_ANYSEARCH_CATALOG_REQUESTS` to a private JSON file containing cases from the current catalog:

```json
[
  {
    "name": "react-docs",
    "request": {
      "query": "React useEffect cleanup",
      "tag": "code.doc",
      "params": {"library": "react"},
      "max_results": 3
    }
  }
]
```

Each request also accepts `zone` and `language`. Supply all required and conditionally applicable parameters from the current catalog. The file contains request inputs only; keep credentials in `ANYSEARCH_TEST_API_KEY` and the API authorization token in `RAGFLOW_TEST_API_TOKEN`.

```bash
RAGFLOW_TEST_ANYSEARCH_CATALOG_E2E=1 bash build.sh --test-e2e ./internal/service -run TestAnySearchSavedCatalogE2E -v -timeout 90m
```

The test saves and reloads each Canvas's provider settings, then passes `tag`, `params` and `max_results` as named inputs through the normal Agent API. It checks that structured results and their request ID reach the user message and saved session. These runtime inputs are distinct from node configuration. Created sessions and workflows are removed afterward. `RAGFLOW_ANYSEARCH_E2E_RECEIPTS` saves private per-case receipts. All cases run independently, so one failed selection does not prevent the remaining cases from executing.

For same-run wire evidence, set `RAGFLOW_ANYSEARCH_WIRE_RECEIPTS` to a private JSONL recording from an outbound observer that forwards actual requests unchanged. Each row contains `started_utc`, `method`, full `url`, parsed `request`, HTTP `status`, `business_code` and `request_id`. Never record authorization headers. The test links each delivered request ID to exactly one observed search call and checks its full request against the saved selection. Without this recording, the test establishes saved routing and delivery only.

Review every selection's actual result scope separately. This test does not turn HTTP success, nonempty results or a successful saved workflow into proof that a source filter worked.

## Saved Agent discovery and batch

Use the API and chat-model variables above, plus the private wire recording. These cases do not need an embedding model or dataset:

```bash
RAGFLOW_TEST_ANYSEARCH_EXTENSIONS_E2E=1 bash build.sh --test-e2e ./internal/service -run TestAnySearchAgentExtensionsE2E -v -timeout 20m
```

The discovery workflow uses actual model-selected tool calls to list domains and inspect the code domain. The batch workflow asks the model to issue two independent queries in one tool call. Tests correlate the observed requests and vendor responses with model tool replies, the user answer and saved history. The forwarding observer supplies authentication to a hosted model when configured; it never supplies model output.
