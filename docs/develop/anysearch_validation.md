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
