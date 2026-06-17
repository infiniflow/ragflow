# Component Parity Matrix

> **Status**: this matrix is **manually curated** as of 2026-06-16. The plan §7 originally envisioned an `auto-generated` table produced by a `tools/gen-component-parity` script. That script is **not yet implemented** (plan v3.3.1 OQ #3 / §7 deliverables). The script would walk the registered component and tool factories and emit this markdown; for now, it's a hand-written snapshot.
>
> **Source-of-truth** for any single entry is the current Go code (the entry's file + test). When in doubt, grep the test file: `find internal/agent/component internal/agent/tool -name "*_test.go" | xargs grep -l "<name>"`.

## Conventions

- ✅ = implemented, tested, behaviorally compatible with Python
- 🟡 = scaffolded (loud-fail sentinel on call); real impl pending follow-up
- ⚠️ = implemented but with a known gap vs Python (e.g. operator coverage, plan-tracked deferral)
- ❌ = not implemented; placeholder or no Go file

Universe A = canvas-DAG components (PascalCase names, `Component.Invoke(ctx, inputs) → (outputs, error)`).
Universe B = eino ReAct tools (snake_case names, `InvokableRun(ctx, args) → (string, error)`).

## Universe A — Canvas DAG components (24 total)

| Name | Source file | Python parity | Sentinel / status |
|------|-------------|----------------|---------------------|
| Agent | `internal/agent/component/agent.go` | ✅ | All 3.1-3.5, 3.8 done; 3.6 max_retries retry loop ⚠️ |
| Begin | `internal/agent/component/begin.go` | ✅ | — |
| Browser | `internal/agent/component/browser.go` | ✅ | per-component `Timeout` field; `COMPONENT_EXEC_TIMEOUT` env drives canvas-level default |
| Categorize | `internal/agent/component/categorize.go` | ✅ | — |
| DataOperations | `internal/agent/component/data_operations.go` | ✅ | — |
| DocsGenerator | `internal/agent/component/docs_generator.go` | ✅ | uses `internal/agent/component/io/{pdf,docx,txt,markdown,html}_writer.go` |
| ExcelProcessor | `internal/agent/component/excel_processor.go` | ✅ | — |
| ExeSQL | `internal/agent/component/universe_a_wrappers.go` | ⚠️ | Wrapper delegates to `exesql` Universe B tool; canvas-factory plumbing still stubs (plan v3.3.1) |
| Fillup | `internal/agent/component/fillup.go` | ✅ | — |
| Generate | `internal/agent/component/fixture_stubs.go` | ✅ | Legacy v1 alias; preserved for DSL round-trip |
| Invoke | `internal/agent/component/invoke.go` | ✅ | — |
| Iteration | `internal/agent/component/fixture_stubs.go` | ✅ | Legacy alias; `IterationItemStub` per v3.1 reclassification |
| IterationItem | `internal/agent/component/fixture_stubs.go` | ✅ | Legacy alias; compat stub for DSL round-trip |
| ListOperations | `internal/agent/component/list_operations.go` | ✅ | — |
| LLM | `internal/agent/component/llm.go` | ✅ | All 2.1-2.7 done; 13 LLM tests |
| Loop | `internal/agent/component/loop.go` | ✅ | engine-level macro; `LoopItem`/`ExitLoop` intentionally absent per v3.1 reclassification |
| Message | `internal/agent/component/message.go` | ✅ | 8a chunked + 8b rich content; TTS + MemorySaver still 🟡 |
| Parallel | `internal/agent/component/parallel.go` | ✅ | — |
| Retrieval | `internal/agent/component/universe_a_wrappers.go` | ⚠️ | **stub registered** (not the delegation wrapper); `SearchMyDataset` alias also stub; both call `ErrRetrievalServiceMissing` (loud-fail). Plan v3.3.1 follow-up: replace stub with `newRetrievalComponent` + register `SearchMyDataset` to the real wrapper |
| StringTransform | `internal/agent/component/string_transform.go` | ✅ | — |
| Switch | `internal/agent/component/switch.go` | ✅ | All 12 operators as of v3.3.1 (was 6/12; now also includes `not contains` / `start with` / `end with` / `not empty` / `≥` / `≤`, with case-folded string comparison) |
| TavilySearch | `internal/agent/component/universe_a_wrappers.go` | ⚠️ | Same stub issue as `Retrieval` — registered as `NewTavilySearchStub`, not the delegation wrapper. Same follow-up applies |
| UserFillUp | `internal/agent/component/userfillup.go` | ✅ | — |
| VariableAggregator | `internal/agent/component/variable_aggregator.go` | ✅ | — |
| VariableAssigner | `internal/agent/component/variable_assigner.go` | ✅ | — |
| Answer | `internal/agent/component/fixture_stubs.go` | 🟡 | compat stub; needs 4.4 wait-for-user real impl |

**Note on stubs vs wrappers**: `Retrieval`, `TavilySearch`, `ExeSQL` have **real delegation wrappers** in `universe_a_wrappers.go` but the **registry currently maps them to stubs** in `fixture_stubs.go`. The wrappers are dead code at production runtime. Plan v3.3.1 follow-up replaces the stub registrations with the wrapper registrations (separate commit; one of the "still open" items).

## Universe B — eino ReAct tools (23+2 = 25 total)

| Name | Source file | Python parity | Notes |
|------|-------------|----------------|-------|
| akshare | `internal/agent/tool/akshare.go` | ✅ | — |
| arxiv | `internal/agent/tool/arxiv.go` | ✅ | — |
| code_exec | `internal/agent/tool/code_exec.go` + `code_exec_client.go` | ✅ | All 5 providers (self_managed / aliyun / e2b / local / ssh); see `docs/develop/sandbox-python-go-diff.md` for full diff |
| crawler | `internal/agent/tool/crawler.go` | ✅ | — |
| deepl | `internal/agent/tool/deepl.go` | ✅ | — |
| duckduckgo | `internal/agent/tool/duckduckgo.go` | ✅ | — |
| email | `internal/agent/tool/email.go` | ✅ | — |
| execute_sql | `internal/agent/tool/exesql.go` | ⚠️ | SELECT-only safety filter; rejects Trino/IBM DB2 with `ErrExeSQLUnsupportedDB` |
| exesql | `internal/agent/tool/exesql.go` | ⚠️ | Same as `execute_sql`; alias name |
| github | `internal/agent/tool/github.go` | ✅ | — |
| google | `internal/agent/tool/google.go` | ✅ | — |
| google_scholar | `internal/agent/tool/google_scholar.go` | ✅ | — |
| jin10 | `internal/agent/tool/jin10.go` | ✅ | — |
| mcp | `internal/agent/tool/mcp.go` | 🟡 | `MCPToolAdapter` wraps `mcpclient.Tool`; `InvokableRun` returns "not yet implemented" until `mcpclient.CallTools` lands |
| pubmed | `internal/agent/tool/pubmed.go` | ✅ | — |
| qweather | `internal/agent/tool/qweather.go` | ✅ | — |
| retrieval | `internal/agent/tool/retrieval.go` | ⚠️ | Adapter 落（`retrieval_nlp.go::NLPRetrievalAdapter`），boot wiring 已落（`cmd/server_main.go`），但当前 `RetrievalTool` 返回 `ErrRetrievalServiceMissing` 如果 `SetRetrievalService` 未调；**实际 production 中 `SetRetrievalService` 已在 boot 调**（v3.3.1 follow-up） |
| search_my_dataset | `internal/agent/tool/registry.go:48` | ✅ | snake_case alias of `retrieval` |
| search_my_dateset | `internal/agent/tool/registry.go:49` | ✅ | snake_case alias of `retrieval` (Python-typo compat) |
| searxng | `internal/agent/tool/searxng.go` | ✅ | — |
| tavily | `internal/agent/tool/tavily.go` | ✅ | — |
| tushare | `internal/agent/tool/tushare.go` | ✅ | — |
| wencai | `internal/agent/tool/wencai.go` | ✅ | — |
| wikipedia | `internal/agent/tool/wikipedia.go` | ✅ | — |
| yahoo_finance | `internal/agent/tool/yahoo_finance.go` | ✅ | — |

## Counts

- **Universe A**: 24 components (19 production + 5 compat stubs)
- **Universe B**: 25 tools (23 standalone + 2 aliases of `retrieval`)
- **Total**: 49 named entities

## Test coverage

| Package | Test files | Notes |
|---------|-----------|-------|
| `internal/agent/component` | 40+ | 234+ test cases; covers 22 production components; pre-existing `TestSplitSentences_*` failures unrelated |
| `internal/agent/tool` | 31 | 264+ test cases; covers all 24 tools including retrieval adapter, e2b, local, ssh, code_exec |
| `internal/agent/sandbox` | 4 | 90+ test cases; 5 providers + result_protocol + manager |
| `internal/agent/canvas` | 12+ | 19+ test cases; scheduler, loop, multibranch, interrupt_resume, checkpoint, dsl_examples |
| `internal/agent/runtime` | 3 | 3+ test cases; selector, metrics, template_jinja |
| `internal/service/nlp` | 1 | retrieval_test.go |
| `internal/service/kg` | 1 | retrieval_test.go |
| `internal/service/memory` | 1 | memory_message_service_test.go |
| `internal/agent/component/prompts` | 0 | citation template tests are in `component/citation_test.go` |
| `internal/agent/component/io` | 4 | docx / html / markdown / txt writer tests |
| `internal/agent/audio` | 1 | tts_test.go |
| `internal/agent/workflowx` | 8 | loop / parallel + integration tests |

## How to regenerate this table

```sh
# Placeholder for plan §7's auto-gen tool. Today: a small script
# (about 100 lines of Go) that walks Registry() and each tool's
# constructor would emit this markdown. Suggested locations:
#   tools/gen-component-parity/main.go
# The script reads the `componentName*` consts from
# `internal/agent/component/*.go` and the registry entries from
# `internal/agent/tool/registry.go`, then for each name runs:
#   - `go test -run TestXxx` to confirm a test exists
#   - `grep TODO\|deferred\|FIXME` to flag ⚠️ entries
# and emits the table.
```

## What changed in v3.3.1

- **RetrievalService** wired at boot in `cmd/server_main.go` (OQ #8 resolution)
- **SearchMyDataset** alias registered in `fixture_stubs.go` (OQ #14 closed)
- **Switch operators** all 12 implemented with case-folded comparisons (OQ #13 closed)
- **COMPONENT_EXEC_TIMEOUT** verified at the canvas dispatcher layer (OQ #11 closed)
- Three Universe A components (Retrieval, TavilySearch, ExeSQL) still register as stubs instead of delegation wrappers — tracked as plan v3.3.1 follow-up
- **Phase 4.1 canvas parallel** confirmed closed by eino (v3.3.1 user audit) — `compose.Workflow.Run` spawns one `go t.execute()` per ready node in each topological wave. The plan v3.3.1 §11.3 "2-3 day refactor" item was a misread of eino's executor; **no Go work needed** beyond the canvas `AddInput` edge wiring already in `scheduler.go:175,179`. Defense: [`internal/agent/canvas/parallel_batch_test.go`](internal/agent/canvas/parallel_batch_test.go) (structural 4-node sibling compile) + [`internal/agent/canvas/parallel_timing_test.go`](internal/agent/canvas/parallel_timing_test.go) (5-node DAG static analysis)
