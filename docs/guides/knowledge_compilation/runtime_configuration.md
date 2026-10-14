---
sidebar_position: 6
title: Knowledge Compilation Runtime Configuration
sidebar_label: Runtime Configuration
slug: /knowledge_compilation/runtime_configuration
sidebar_custom_props: {
  categoryIcon: LucideWandSparkles
}
---

# Knowledge Compilation Runtime Configuration

Knowledge compilation runtime parameters can be configured with environment
variables. In the Docker deployment, set them in `docker/.env` and restart the
RAGFlow service for the changes to take effect.

The values below are the defaults. If an environment variable is not set, its
default value is used. Invalid values are replaced with the default value.
Values below a configured minimum or above a configured maximum are clamped to
the valid range and logged.

## Wiki Configuration

| Environment variable | Default | Description |
| --- | ---: | --- |
| `WIKI_MAP_LLM_POOL_SIZE` | `20` | Maximum number of concurrent LLM calls allowed by the Wiki task's shared LLM pool. |
| `WIKI_MAP_MAX_PENDING` | `25` | Maximum number of active and waiting calls admitted by the Wiki LLM pool. The effective value is never lower than `WIKI_MAP_LLM_POOL_SIZE`. |
| `WIKI_REFINE_WORKERS` | `4` | Number of page-refinement workers. Actual LLM concurrency is still limited by the shared pool. |
| `WIKI_MAP_WORKERS` | `20` | Default worker count used by direct Wiki MAP calls. |
| `WIKI_MAP_TIMEOUT` | `600` seconds | Timeout for one Wiki MAP extraction call. |
| `WIKI_PLAN_TIMEOUT` | `600` seconds | Timeout for Wiki PLAN calls, including page planning and MAYBE resolution. |
| `WIKI_REFINE_TIMEOUT` | `300` seconds | Timeout for one Wiki page-writing call. |
| `WIKI_MERGE_TIMEOUT` | `600` seconds | Timeout for merging an existing Wiki page with newly generated content. |

`WIKI_MAP_LLM_POOL_SIZE` is a hard ceiling for the task. The pool can reduce
the effective concurrency for a model after rate limiting, so setting this
value does not force every provider to receive that many concurrent requests.

## Structure Compile Configuration

| Environment variable | Default | Description |
| --- | ---: | --- |
| `DOC_STRUCTURE_LLM_POOL_SIZE` | `20` | Maximum number of concurrent LLM calls for a document Structure Compile task. |
| `DOC_STRUCTURE_COMPILE_MAX_IN_FLIGHT` | `15` | Maximum number of structure batch/template operations in flight. |
| `DOC_STRUCTURE_COMPILE_BATCH_CHUNKS` | `4` | Number of source chunks passed to one outer Structure Compile batch. |
| `STRUCTURE_CONTEXT_FRACTION` | `0.5` | Fraction of the model context window used when packing structure batches, clamped to `(0, 1]`. |
| `STRUCTURE_DEFAULT_CONTEXT` | `100000` tokens | Fallback model context size when the model does not provide one. |
| `KNOWLEDGE_GRAPH_CONTEXT_FRACTION` | `0.1` | Fraction of the model context window used for Knowledge Graph batches, clamped to `(0, 1]`. |
| `KNOWLEDGE_GRAPH_MIN_BATCH_TOKENS` | `2048` tokens | Minimum Knowledge Graph batch size. |
| `KNOWLEDGE_GRAPH_MAX_BATCH_TOKENS` | `4096` tokens | Maximum Knowledge Graph batch size. The effective value is never lower than `KNOWLEDGE_GRAPH_MIN_BATCH_TOKENS`. |
| `STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S` | `120` seconds | Time limit for the Structure Compile chain-correction LLM step. |

The regular Structure Compile entity/relation extraction path does not define
an independent application-level timeout. Its request timeout is determined by
the configured LLM provider/client.

## LLM Pool Rate-Limit Handling

These variables apply to the shared adaptive LLM pool:

| Environment variable | Default | Description |
| --- | ---: | --- |
| `LLM_POOL_RATE_LIMIT_RETRIES` | `3` | Number of retries after a rate-limit response. |
| `LLM_POOL_RATE_LIMIT_RETRY_BASE_DELAY` | `1.0` second | Initial delay used by exponential backoff. |
| `LLM_POOL_RATE_LIMIT_RETRY_MAX_DELAY` | `30.0` seconds | Maximum delay between rate-limit retries. |

After a rate-limit response, the pool lowers the effective concurrency for the
affected model and retries the request. Successful calls gradually restore
the model's concurrency up to the configured pool limit.

The pool normalizes `max_pending` to at least the configured pool size, and
normalizes the retry maximum delay to at least the retry base delay.

When all retries are exhausted, the backend records the detailed failure in
its service log and the frontend task log receives only the stage, context, and
error type. Provider response bodies are not forwarded to the frontend.

## Wiki Page Writer Recovery

Wiki REFINE can retry a page's LLM writing request after the provider and
adaptive pool have exhausted their retries. This recovery is limited to the
LLM call, before embedding and persistence; successful pages, database writes,
page versions, and edit-history records are not replayed.

| Environment variable | Default | Description |
| --- | ---: | --- |
| `WIKI_REFINE_LLM_RETRIES` | `2` | Additional page-writer attempts after a transient failure. Set to `0` to disable recovery. |
| `WIKI_REFINE_LLM_RETRY_BASE_DELAY` | `30` seconds | Initial recovery delay, doubled for each subsequent attempt, with up to 25% positive jitter. Must be positive for recovery to run. |
| `WIKI_REFINE_LLM_RETRY_MAX_DELAY` | `120` seconds | Maximum recovery delay, including jitter. Must be positive for recovery to run. |

Setting either delay bound to zero disables additional recovery instead of
retrying without a cooldown.

Explicit rate limits, timeouts, connection failures, and retryable server
errors can recover. Authentication, invalid requests, content filtering,
quota/billing failures (including permanent 429 responses), and unknown errors
are not retried. When an exception retains a `Retry-After` header, recovery
waits at least that long; a delay exceeding the configured maximum fails
without retrying early. Providers that return error strings lose HTTP headers,
so those calls use the configured backoff instead.

The attempts stay in the running task and use its existing adaptive LLM pool.
Cancellation interrupts the recovery wait. Exhausted failures keep the existing
failure marker and `Wiki incomplete ... retry required` behavior for a later
manual Update. This does not add durable task requeueing or recovery after a
worker restart. Provider and pool retries still apply inside each attempt, so
increase these settings cautiously to avoid amplifying requests and cost.

## Example

The following configuration lowers concurrency for providers with a smaller
request limit and shortens the Wiki MAP timeout:

```dotenv
WIKI_MAP_LLM_POOL_SIZE=8
WIKI_MAP_MAX_PENDING=10
DOC_STRUCTURE_LLM_POOL_SIZE=8
LLM_POOL_RATE_LIMIT_RETRIES=5
WIKI_MAP_TIMEOUT=300
```

Restart the RAGFlow backend after changing `docker/.env`. The settings are
read when the Python modules are loaded and are not changed for already-running
tasks.
