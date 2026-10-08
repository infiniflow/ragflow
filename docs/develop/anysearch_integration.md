---
sidebar_position: 21
---

# AnySearch Agent integration design

This draft proposes a native Go Agent integration for early maintainer feedback.
The skeleton PR contains this design document only. No implementation or tests are
included in the proposed PR.

## Intended behavior

Users add AnySearch tools to an Agent or Canvas workflow using the existing editor.
General search works with an optional API key; anonymous requests omit Authorization.
Vertical search uses the live capability catalog and preserves provider parameters.
Parallel search accepts one to five queries, preserving input order and individual
failures. Extraction fetches a single HTTP(S) page through AnySearch.

The proposed tools are `anysearch_search`, `anysearch_get_sub_domains`,
`anysearch_batch_search` and `anysearch_extract`, backed by Canvas components
`AnySearchSearch`, `AnySearchGetSubDomains`, `AnySearchBatchSearch` and
`AnySearchExtract`.

## Ownership and data flow

`internal/anysearch` will own fixed-endpoint HTTP requests, bounded responses, optional
Bearer authentication, provider status validation and request IDs. It uses the Go
standard library and adds no dependency or database migration.

`internal/agent/tool` will own Eino argument schemas, saved node configuration and
RAGFlow result/reference mapping. Credentials belong to saved configuration, not
model-visible arguments. Tools will use the existing registry and the single
`ToolBackedComponent` Canvas adapter. The editor will provide tool/node selection,
configuration, output presentation and credential removal from exported workflows.

Execution is through the existing Agent runtime and
`POST /api/v1/agents/chat/completions`. Dedicated provider HTTP routes are outside
this change. Ordinary-chat provider support is a separate proposed PR using the
same client.

HTTP/business failures remain explicit, including authentication, quota, timeout,
cancellation and malformed responses. POST requests are not retried automatically.
Batch item failures retain their positions. Provider page content and catalog text
are external data, not instructions.

## Validation

Planned client unit tests use local HTTP servers and cover request mapping, anonymous/keyed
headers, live-schema validation, empty required parameter values, extraction,
bounded responses, invalid envelopes, cancellation and concurrent partial failure.

Integration completion requires registry/component tests, editor validation and
credential-export tests, frontend type-check/build, and real-service evidence from
saved workflows through the UI and existing Agent HTTP API. Tests contacting real
services must use the repository's integration/e2e build tags. A direct provider
probe does not establish RAGFlow integration success.

The repository's build/test wrapper remains the required native validation path.
An isolated standard-library client check does not validate the Agent runtime,
native dependencies, frontend or full server startup.
