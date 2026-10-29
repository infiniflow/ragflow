# Chunk SDK contract audit

This folder targets the checked-in Python SDK and Go API. It does not use the
legacy Python server as a contract oracle. No production code or full SDK CI is
changed. Dataset/document audit PRs #21619 and #21633 are not prerequisites.

## Running safely

Run from the repository root with **both** the local config and `--confcutdir`.
This excludes ancestor fixtures that bootstrap credentials, mutate configuration,
parse shared documents, or delete every dataset belonging to the test tenant.

```sh
CHUNKS=test/testcases/test_sdk_api/test_chunk_management_within_dataset
python -m pytest -c "$CHUNKS/pytest.ini" --confcutdir="$CHUNKS" "$CHUNKS" --collect-only -q
python -m pytest -c "$CHUNKS/pytest.ini" --confcutdir="$CHUNKS" "$CHUNKS" -m 'not integration' -q
```

Integration requires a disposable **Go backend and dedicated tenant**, an embedding
model configured for that tenant, and its search/storage dependencies. Set
`RAGFLOW_SDK_ISOLATED=1`, `RAGFLOW_SDK_BASE_URL`, and `RAGFLOW_SDK_API_KEY` only after
verifying ownership of that environment, then run the same command without `-m`.
The guard fails setup rather than disguising missing infrastructure as a defect
skip. `RAGFLOW_SDK_INDEX_TIMEOUT` controls the indexing deadline (default 60 seconds).
Request connect/read timeouts are 5/60 seconds. API errors are never retried as
indexing lag. The 1,000-chunk deletion case is intentionally substantial.

The valid reranker case also requires `RAGFLOW_SDK_RERANK_ID`; keyword expansion
requires a configured tenant chat model and `RAGFLOW_SDK_CHAT_CONFIGURED=1`.
Missing model configuration fails the relevant case; it is not a production skip.
Run against Elasticsearch and Infinity separately. Current `internal/engine/global.go`
has no OpenSearch backend; obsolete OpenSearch count branches are removed.

Fixtures create a unique dataset per case and delete only that dataset ID, including
when upload fails. Cleanup errors remain visible and deletion is verified by ID.
Chunks are created manually; there is no ingestion run or `document.run == "DONE"`
assumption. Listing and retrieval readiness are checked independently. Raw list
responses are used for persistence probes because the SDK list schema is defective;
a separate skipped test retains the correct public SDK schema assertions.

## Contract evidence

The audited call paths are:

- `sdk/python/ragflow_sdk/modules/document.py`: `add_chunk`, `delete_chunks`,
  `list_chunks`; `modules/chunk.py`: `Chunk.update`, `ChunkUpdateError`;
  `ragflow.py`: `RAGFlow.retrieve`.
- `internal/handler/chunk.go`: request binding, query validation, update type
  handling; `internal/service/chunk/chunk.go`: add/list/update/remove semantics,
  ownership, image storage, engine requests and response fields.
- `internal/handler/dataset.go`: `SearchDatasets` typed request validation;
  `internal/service/dataset/search.go` and `internal/service/nlp/`: retrieval,
  candidate selection, score ordering, document/metadata/availability filtering.
- `internal/engine/elasticsearch/chunk.go`, `internal/engine/infinity/chunk.go`:
  lexical/vector matching and fusion. `top_k` is a candidate limit, not a promised
  response count; `page_size` is a response limit.
- `docs/references/python_api_reference.md` and `http_api_reference.md`: chunk and
  retrieval contracts. Python-specific exception examples are obsolete. The old
  empty-question example also conflicts with current Go required-field validation.
- `internal/handler/chunk_test.go`, `internal/service/chunk/chunk_test.go`,
  `internal/handler/dataset_search_test.go`, and `test/api/retrieval_api_test.go`:
  existing Go validation, ownership, idempotency, schema and retrieval coverage.

These sources establish: add rejects empty/non-string content and invalid list
payloads; update accepts empty string content but must reject invalid types;
questions are trimmed and empty entries removed; duplicate adds use a stable ID;
delete is document-scoped and unknown/repeated IDs are idempotent; numeric list
query strings are accepted, whereas retrieval JSON numeric strings are rejected.
Updates return `None` and do not mutate the SDK object's cached attributes.

Retrieval tests use a small owned corpus with a shared lexical query, threshold
zero and text-only readiness checks. They assert identity, ownership, content,
finite ordered scores, pagination, filtering and response limits. They do not
require arbitrary four/five-result counts from normal hybrid retrieval. The
single-result lexical list filters use unique terms in the owned fixture.

## Original test inventory and decisions

All six original files, their fixtures/helpers, and every parameter branch were
reviewed: **36 original test methods, 134 collected parameter cases**. Classification
below is by original method: **20 UPDATE, 16 REPLACE, 0 unchanged KEEP, 0 whole-test
DELETE**. Valid checks within rewritten methods remain. Consolidated duplicates
include engine-specific lexical/top-k alternatives and repeated missing-argument
checks; missing-argument behavior now has a service-free test. SKIP is an orthogonal
case-level outcome, detailed below, rather than deletion of the original assertions.

| Original file / method | Original cases | Decision | Coverage after audit |
| --- | ---: | --- | --- |
| add / content | 6 | UPDATE | Null, empty, whitespace, integer, text and punctuation; failed requests do not add IDs. |
| add / important_keywords | 6 | UPDATE | String lists, empty members, duplicates, non-string members and scalar rejection. |
| add / questions | 6 | UPDATE | Same boundaries plus normalization of blank questions. |
| add / tag_kwd | 6 | UPDATE | String-list contract and exact Go validation messages. |
| add / repeated_add_chunk | 1 | UPDATE | Same content returns the same ID and persists once. |
| add / add_chunk_to_deleted_document | 1 | UPDATE | Current document ownership error. |
| add / concurrent_add_chunk | 1 | UPDATE | Consume all 50 futures, unique IDs and persisted ID set. |
| delete / delete_partial_invalid_id | 3 | REPLACE | Unknown IDs at three positions do not prevent owned valid deletions. |
| delete / repeated_deletion | 1 | REPLACE | Repeated deletion is idempotent. |
| delete / duplicate_deletion | 1 | REPLACE | Duplicate IDs are idempotent. |
| delete / concurrent_deletion | 1 | UPDATE | All futures succeed and persistence becomes empty. |
| delete / delete_1k | 1 | UPDATE | Preserve bulk coverage; paginated readiness and deletion probes. |
| delete / basic_scenarios | 6 | REPLACE | Unknown/single/all IDs, explicit delete_all, safe no-op and Python kwargs boundary. |
| list / page | 7 | UPDATE | Default/null, valid pages, numeric strings, zero/negative/non-numeric validation. |
| list / page_size | 7 | UPDATE | Default/null, bounds, numeric strings and invalid values. |
| list / keywords | 7 | REPLACE | Empty/default, shared term, unique term, numeric term and no-match; no engine count exceptions. |
| list / id | 4 | REPLACE | Empty IDs remain active; known/unknown filters retain correct expectations under targeted skips. |
| list / concurrent_list | 1 | UPDATE | Consume each future and validate owned fixture count. |
| list / default | 1 | UPDATE | 31 explicit chunks establish the default limit of 30; no sleep. |
| list / invalid_document_id_raises | 1 | UPDATE | Assert meaningful current Go error text. |
| retrieval / basic_scenarios | 4 | REPLACE | Owned results with/without document filter; Python argument errors tested separately. |
| retrieval / page | 7 | REPLACE | Compare slices of a controlled baseline; retain invalid inputs in shared negative matrix. |
| retrieval / page_size | 5 | REPLACE | Defaults, single result, caps and typed/range errors. |
| retrieval / vector_similarity_weight | 4 | UPDATE | Weights 0, 0.5, 1; finite ordered owned results and invalid ranges/types. |
| retrieval / top_k | 5 | REPLACE | Candidate limits 1/10/2048, response cap; invalid values, no engine result-count branches. |
| retrieval / rerank_id | 2 | REPLACE | Configured reranker and explicit unknown-model rejection; no blanket skip. |
| retrieval / keyword | 5 | REPLACE | False/null/true, configured expansion, string-bool rejection; no blanket skip. |
| retrieval / concurrent_retrieval | 1 | UPDATE | Consume all futures and verify nonempty owned ordered results. |
| update / content | 6 | UPDATE | Persist text/punctuation/empty/space; null/integer rejection remains skipped for defect. |
| update / important_keywords | 6 | REPLACE | Shared typed list matrix; seed then clear; invalid cases retain rejection assertions. |
| update / questions | 6 | REPLACE | Shared matrix with trimming/blank normalization and invalid-type skips. |
| update / tag_kwd | 6 | REPLACE | Shared matrix with active strict negative cases. |
| update / available | 6 | REPLACE | Both bool transitions persist; string/integer rejection retained under defect skip. |
| update / repeated_update_chunk | 1 | UPDATE | Every sequential/repeated write is verified in storage. |
| update / concurrent_update_chunk | 1 | UPDATE | 50 contending writes across four chunks; all futures observed and final values submitted. |
| update / update_chunk_to_deleted_document | 1 | UPDATE | `ChunkUpdateError`, current code/message rather than generic exception. |

New coverage includes add list-null/empty and image payload boundaries; delete_all
conflict/document isolation; list response schema; retrieval negative matrix,
document/metadata/availability filters and pagination uniqueness; unsupported update
fields; SDK payload/default/error contracts; polling and cleanup failure behavior.

## Skip audit

Historical references were inspected rather than carried forward based on issue
status: #6411, #6509, #6499, #6554 and #10692 concern the legacy Python backend;
PR #6376 addressed duplicate-ID deletion validation. Their old skips are removed
or replaced by the current defects below. The old `**"not json"` case never reaches
HTTP; a service-free TypeError check replaces that skip. Old list negative-input
skips now assert Go validation, and rerank/keyword groups execute with explicit
model prerequisites. No retrieval group is blanket-skipped.

**20 cases remain skipped for source-confirmed contract defects:**

| Cases | Defect and source boundary | Correct assertion retained |
| ---: | --- | --- |
| 3 | `RemoveChunks` rejects omitted/null/empty IDs without delete_all, contrary to both API references. | Safe no-op preserves IDs. |
| 2 | SDK `list_chunks(id=...)` sends `id`; Go `ListChunks` reads `chunk_ids`. | Known ID selects one; unknown ID returns none. |
| 1 | Go list returns storage names (`chunk_id`, `content_with_weight`, `important_kwd`, `question_kwd`) and omits document/dataset IDs; SDK constructs `Chunk` without mapping. | SDK identity/content/ownership/keyword/question schema. |
| 2 | Go `UpdateChunk` silently ignores null/integer content. | Typed `ChunkUpdateError` mentioning content. |
| 4 | Go update ignores invalid keyword scalar/null types or converts non-string members to empty strings. | List-type validation error. |
| 4 | Go update has the same invalid questions handling. | List-type validation error. |
| 4 | Go update silently ignores string/integer availability. | Typed `ChunkUpdateError` mentioning availability. |

No production fix is included. Defects are confirmed by current source paths, not
by integration execution in this audit environment. Correct assertions remain
inside every skipped case.

## Validation boundary

The resulting suite collects **195 cases**: add 39, delete 17, list 28, retrieval
48, update 40, and 23 service-free SDK/fixture cases. Python syntax and focused
Ruff checks pass. All **23 service-free cases pass**, including payloads, SDK error
propagation, cleanup ownership/failures, deadlines and pagination-loop detection.

A full invocation with no isolated backend configured produces **23 passed,
20 skipped, 152 setup errors, zero assertion failures**. Every setup error is the
explicit integration isolation guard; report these as **152 environment-blocked
cases**, not production defects or successful integration tests. The existing
host stack is not verified disposable and was not used. Elasticsearch/Infinity,
model-backed retrieval/reranking/keyword expansion, 1,000-chunk deletion, images
and concurrency still require execution in the isolated environment described above.
