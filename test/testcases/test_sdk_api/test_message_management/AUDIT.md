# Message management SDK audit

Base: `97b08d6585` (`upstream/main`, refreshed 2026-10-29; intervening commits do not change the audited contracts). Branch: `test/audit-sdk-messages`.
This is the final functional-folder audit in Phase 2. Python SDK tests stay in Python; no SDK, backend, other audit folder, or shared infrastructure is changed.

## Running safely

From the repository root:

```sh
folder=test/testcases/test_sdk_api/test_message_management
python -m pytest -c "$folder/pytest.ini" --confcutdir="$folder" "$folder" -m 'not integration' -q
python -m pytest -c "$folder/pytest.ini" --confcutdir="$folder" "$folder" --collect-only -q
```

Use the same config and `--confcutdir` with `-m integration` for live tests. Both arguments are required: ancestor conftests configure live tenant models, replace marker selection, and contain obsolete class-level environmental skips. The standalone invocation excludes those hooks. Those shared files are untouched to avoid conflicts with Memory Management PR #21670; a three-way merge probe confirmed that deleting the adjacent shared message-skip entries would conflict with that PR.

Live tests require a verified disposable Go deployment and a dedicated empty tenant, declared with `RAGFLOW_SDK_ISOLATED=1`, `RAGFLOW_SDK_BASE_URL`, `RAGFLOW_SDK_API_KEY`, and `RAGFLOW_SDK_TENANT_ID`. Do not set the flag for a shared deployment. Configure usable, tenant-owned models with `RAGFLOW_SDK_MEMORY_EMBD_ID` and `RAGFLOW_SDK_MEMORY_LLM_ID`; the suite does not assume a built-in embedding endpoint or install models. Cross-tenant tests additionally require `RAGFLOW_SDK_OTHER_API_KEY` and `RAGFLOW_SDK_OTHER_TENANT_ID` for a different unprivileged tenant. They only target memories created by this suite, never resources belonging to that second tenant. A wrong or unavailable model/service is an environmental or deployment failure, not a reason to blanket-skip search tests.

The suite refuses nonempty tenants and xdist execution. Every created memory ID is tracked immediately, before checking returned tenant ownership. Cleanup is registered before creation, attempts every owned ID after partial setup failures, reports errors, and verifies removal. It never enumerates tenant memories to choose deletion targets. Initial memory listing is read-only, solely to verify an empty test tenant. HTTP requests have a 30-second timeout; eventual-state polling has a 30-second deadline and propagates API errors immediately. Function-scoped fixtures prevent status/forget mutations from leaking between tests.

## Current contract evidence

- SDK: `sdk/python/ragflow_sdk/ragflow.py:382-424` and `modules/memory.py:59-89` establish POST creation, GET search/recent/list/content, DELETE forgetting, PUT boolean status, query serialization, return shapes, and raised API errors. The wire tests use the real SDK and prepared HTTP requests; mocks establish SDK behavior only, not Go behavior.
- Routes: `internal/router/router.go:490-507` uses `/messages/{memory_id}:{message_id}` and `/content`. Handler `parseMemoryMessagePath` (`internal/handler/memory.go:668-693`) accepts nonnegative int64 message IDs and rejects malformed composite paths.
- Creation: HTTP docs at `docs/references/http_api_reference.md:6146-6228`, `MemoryHandler.AddMessage` (`internal/handler/memory.go:547-620`), and `MemoryService.AddMessage` (`internal/service/memory.go:1035`) support multiple memories. One request creates a separately addressable message per memory, with partial success possible when one memory is unavailable. The raw message is embedded/stored before success; extraction remains asynchronous (`memory_message_service.go:173-244`). Both raw and all supported extracted memory types retain coverage.
- API-key `user_id` is a required nonblank external conversation subject, trimmed before storage; it does not select the principal used for memory authorization. JWT/browser attribution instead uses the authenticated user and ignores the supplied subject. This is explicit in HTTP docs at line 6200 and the handler at lines 594-615. The SDK always sends its default empty string, so callers using an API key must explicitly supply an external subject. The old `IS_GO_PROXY` attribution conditional is removed. JWT/browser authentication itself is outside this API-key SDK audit's live coverage.
- Stored raw content is exactly `User Input: {user_input}\nAgent Response: {agent_response}`, with memory/user/agent/session metadata and initial boolean status true (`internal/service/memory_message_service.go:279-305`). Assertions verify that content and ownership after retrieval, not only the success acknowledgement.
- Listing: page defaults 1/50, page size 1..100, descending `valid_at`, raw message rows with extraction/task metadata, and total count (`handler/memory.go:494-509`, `service/memory.go:1798-1954`). `keywords` applies to session IDs, not content. The SDK docs at `docs/references/python_api_reference.md:2436` additionally promise fuzzy session matching. Equal timestamps have no guaranteed secondary order; tests compare identity sets and descending timestamps, not insertion order.
- Recent: SDK default limit 10, accepted range 1..100; agent/session and memory filters; inaccessible or unknown memories yield an empty list (`handler/memory.go:879-916`, `service/memory.go:1548-1627`). Forgotten messages are excluded, while status is not a recent-message filter.
- Search: query text plus memory/agent/session/user filters, enabled and not-forgotten messages, `top_n` 1..100; multiple memories require the same embedding model (`service/memory.go:1208-1349`). SDK default top_n is 10, HTTP default is 5. Although the HTTP reference labels query as required, blank-query filter-only retrieval is intentional in Go's `TestSearchMessageFiltersAccessibleMemoryAndDefaultsStatus` (`internal/service/memory_message_test.go:950-1015`), so it remains active. Semantic tests distinguish coriander from Everest using disjoint relevant IDs without fixing scores or ordering tied relevant matches.
- Forget is soft deletion: stamp `forget_at`, retain retrievable content, exclude the record from recent/search; repeats and missing message documents are idempotent (`service/memory.go:992-1029`). Listing is an administrative view and is not assumed to hide forgotten records. Status changes accept actual JSON booleans, persist, and do not change content (`handler/memory.go:705-748`, `service/memory.go:1143-1205`).
- Corroborating Go tests: `internal/handler/memory_message_test.go`, `internal/service/memory_message_test.go`, `internal/service/memory_message_service_test.go`, and `test/api/memory_messages_api_test.go` cover parsing, permission filtering, raw message construction, lifecycle persistence, and external-user attribution. These existing Go tests were inspected, not rerun for this Python-test-only change.

## Every original test and parameter case

The original inventory contains 36 collected cases. UPDATE retains the feature while correcting fixtures, parameters, or assertions; KEEP preserves an already-valid expectation and strengthens its evidence. No case is removed because it fails.

| Original file / test / case | Disposition | Result |
|---|---|---|
| add / auth `None` | UPDATE | Isolated URL; verified authentication error |
| add / auth invalid token | UPDATE | Same contract; independent invalid-token case |
| add / `test_add_raw_message` | UPDATE | Raw parameter of `test_add_message`; exact persisted content and ownership |
| add / `test_add_multiple_type_message` | UPDATE | All four supported types, deterministic; same content checks |
| add / `test_add_to_multiple_memory` | UPDATE | Both owned memories independently retrieved and checked |
| forget / auth `None` | UPDATE | Error plus unchanged owned message |
| forget / auth invalid token | UPDATE | Independent token case; unchanged message |
| forget / `test_forget_message` | UPDATE | Timestamp, retained content, exact recent survivors and repeat operation |
| content / auth `None` | UPDATE | Verified error on a real owned composite ID |
| content / auth invalid token | UPDATE | Independent invalid-token case |
| content / `test_get_message_content` | UPDATE | All five known conversations and metadata; vector assertion preserved separately with narrow Infinity skip |
| recent / auth `None` | UPDATE | Isolated URL and verified error |
| recent / auth invalid token | UPDATE | Independent invalid-token case |
| recent / `test_get_recent_messages` | UPDATE | Exact five identities, content, ownership and descending timestamps |
| recent / `test_filter_recent_messages_by_agent` | UPDATE | Known agent, exact three matching messages; cannot pass empty |
| recent / `test_filter_recent_messages_by_session` | UPDATE | Known session, exact single message; cannot pass empty |
| list / auth `None` | UPDATE | Isolated URL and verified error |
| list / auth invalid token | UPDATE | Independent invalid-token case |
| list / `test_params_unset` | UPDATE | Exact identity set, count, schema and ordering |
| list / `test_params_empty` | DELETE | Verified duplicate: `method(**{})` and `method()` invoke identical defaults; covered by unset and SDK wire tests |
| list / page 1, size 10 -> 5 | KEEP | Preserved size; additionally count/IDs/page coverage |
| list / page 2, size 10 -> 0 | KEEP | Preserved empty past-end page; total remains five |
| list / page 1, size 2 -> 2 | KEEP | Preserved size; known IDs and complete page coverage |
| list / page 3, size 2 -> 1 | KEEP | Preserved partial last page; known IDs and coverage |
| list / page 5, size 10 -> 0 | KEEP | Preserved far-past-end page; total remains five |
| list / `test_filter_agent_id` | UPDATE | Deterministic scalar and repeated-list agent filters; exact identities |
| list / `test_search_keyword` | REPLACE | Known exact session plus preserved substring assertion; only confirmed engine-specific fuzzy defect skipped |
| search / auth `None` | UPDATE | Isolated URL and verified error |
| search / auth invalid token | UPDATE | Independent invalid-token case |
| search / `test_query` | UPDATE | Disjoint semantic topics and known relevant IDs; content and ownership checked |
| search / `test_query_with_agent_filter` | UPDATE | Known relevant message and allowed agent identities |
| search / `test_query_with_not_default_params` | UPDATE | Preserved 0.1/0.6/4 values, relevant identity/content and maximum result count |
| status / auth `None` | UPDATE | Error plus unchanged status/content |
| status / auth invalid token | UPDATE | Independent token case; unchanged status/content |
| status / `test_update_to_false` | UPDATE | Strict false persistence, repeat and unchanged survivors |
| status / `test_update_to_true` | UPDATE | False then true persistence, repeat and retained content |

## Every original fixture/helper/skip

| Original surface | Disposition | Replacement |
|---|---|---|
| `add_empty_raw_type_memory` | REPLACE | Owned `memory_factory()` with cleanup registered before creation |
| `add_empty_multiple_type_memory` | REPLACE | Factory with all four supported types; no random duplicate type choices |
| `add_2_multiple_type_memory` | REPLACE | Two individually tracked factory-created memories |
| `add_memory_with_multiple_type_message_func` | REPLACE | Known raw conversations; multi-type creation remains independently tested |
| `add_memory_with_5_raw_message_func` | REPLACE | Function-scoped `populated_memory`, exact five payloads/IDs with two alternating agents/users and five distinct sessions |
| Five enumerate-and-delete teardown functions | DELETE | Single ownership-based client finalizer; unrelated IDs are never cleanup targets |
| `MESSAGE_USER_ID` / `IS_GO_PROXY` | REPLACE | Explicit nonempty external API-key conversation subjects, including trim/default-rejection tests |
| Random agent/session/message selection and substring slicing | DELETE | Known fixture indexes and exact expected identity sets |
| Two-second indexing sleeps | REPLACE | Deadline-based observable-state polling, with immediate API error propagation |
| Keyword `skipif(DOC_ENGINE=infinity)` and runtime “Not support” skip | REPLACE | Actual deployment engine detection; specific fuzzy-session defect skips only |
| Ancestor broad localhost:6380 skip entries | Excluded | Standalone `--confcutdir` execution does not load them; no shared-file conflict with #21670 |

## Narrow defect skips and removal conditions

1. **Ten invalid similarity/keyword-weight cases** (two fields × `-0.1`, `1.1`, invalid text, NaN, infinity): `MemoryHandler.SearchMessage` ignores parse errors and documented [0,1] ranges (`internal/handler/memory.go:833-835`, HTTP docs 6395-6405). Correct argument-error assertions remain behind method-level parametrized `pytest.mark.skip`. Remove when parsing, finite-value and range validation is implemented. Valid boundary cases and all other search tests stay active.
2. **Embedding schema, Infinity only:** `GetChunk` returns `q_<dimension>_vec` without `content_embed` normalization (`internal/engine/infinity/chunk.go:1907-1911,2177`; compare Elasticsearch at 2096-2102). Only the separate vector-schema test is skipped; exact content/identity retrieval stays active. Remove when documented vector readback is normalized.
3. **Fuzzy session substring, Infinity/OceanBase/SeekDB only:** equality filters (`internal/engine/infinity/chunk.go:3293`, `internal/engine/oceanbase/filter.go:135`) contradict the SDK fuzzy-filter promise. Elasticsearch has explicit wildcard handling (`internal/engine/elasticsearch/chunk.go:2327`). Exact-session filters remain active everywhere. Remove after adapters implement substring filtering.
4. **Missing-message status update, Infinity/OceanBase/SeekDB only:** adapters discard affected-row/update results (`internal/engine/infinity/chunk.go:743-758`, `internal/engine/oceanbase/crud.go:292-294`), so absence is reported as success despite the service's missing-message contract (`internal/service/memory.go:1165-1169`). Preserve the error assertion behind this specific test's engine check; real-message status tests remain active. Remove when absence is propagated.

Engine skips use `/system/status`, not an unexplained environment switch. These defects are source-confirmed; they are not claimed as live reproductions. No entire file or class is skipped.

## Added coverage and verification

Added creation required-field/type validation, API-key attribution/defaults, partial multi-memory failures, cross-tenant authorization for seven operations, pagination/limit validation, combined/negative filters, multiple-memory isolation, strict status booleans, malformed/nonexistent IDs, idempotency, enabled/forgotten search exclusion, and SDK transport/error contracts.

Service-free regressions exercise cleanup after partial creation, ownership-assertion failure, cleanup failure continuation, already-deleted owned IDs, unrelated survivors, nonempty tenant refusal, explicit preflight timeout, missing isolation settings, polling success/deadline, and API-error propagation. Prepared-request tests cover every audited SDK operation, defaults/options, repeated memory/agent query keys, composite paths, JSON boolean preservation, API errors and transport failures.

Validation on this branch: 60 service-free cases pass; all 225 cases collect (165 integrations); Ruff lint/format, Pyright and `git diff --check` pass. A representative integration attempt fails at the isolation guard before network access because required disposable-environment settings are absent. The ten static defect-skip cases are part of the integration inventory; engine-specific skip applicability depends on the deployment.

Not verified live: persistence/search/model execution, authorization against two real tenants, engine-specific behavior, or pagination stability when timestamps tie. No deployment, model configuration, or production code was changed, and no other functional folder was started.
