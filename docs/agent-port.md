# Agent Go Port — Migration & Operations Guide

**Status**: Phases 1, 2, 3, 4, 5a, 5b, 5c, 5d, 5e, 6, 8a, 8b scaffolded or closed. See [§1 Status](#1-status) for the per-phase summary. **Detailed per-phase status** (test counts, file paths, deferred-state sentinels): [`docs/develop/agent-go-port-status.md`](develop/agent-go-port-status.md). **Detailed per-component parity**: [`docs/component-parity.md`](component-parity.md). **Design rationale**: [`docs/develop/agent-go-port-design.md`](develop/agent-go-port-design.md). **Functional diff vs Python**: [`docs/develop/sandbox-python-go-diff.md`](develop/sandbox-python-go-diff.md). **Gap analysis plan**: [`.claude/plans/agent-go-port-gap-analysis.md`](../.claude/plans/agent-go-port-gap-analysis.md).

> **Last verified**: 2026-06-16. Source-of-truth for status is the current Go code; status.md is a snapshot.

## 1. Status

The Go port is the runtime for the Agent canvas. **All P0 + most P1 + early P2 is shipped.** Real production gaps remain in: 5b retrieval backend depth, 5c ExeSQL Trino/DB2, 5d CodeExec sandbox scope, 8b TTS real engines, embedding model port — each with a documented deferred-state sentinel that surfaces the gap clearly when the operator hits it (no silent-empty result).

| Layer | Closed | Open |
|-------|--------|------|
| Component base + Invoke/Stream + 22 production components + 13 LLM tests + 5 Agent tests + Message + 5 helpers + 4 supplementary | ✅ | — |
| LLM advanced (vision, citation, structured output, streaming, history, sampler, chat_template) | ✅ | max_retries retry loop (one-shot only today) |
| Agent advanced (tool artifact, citation grounding, multi-turn, cancel, tool DSL, reset, meta) | ✅ | — |
| MCP tool wrap | 🟡 scaffolded (call pending) | `mcpclient.CallTools` |
| Canvas runtime (state, scheduler, loop, multibranch, cycle, checkpoint, cancel) | ✅ | Parallel batch runs in eino's `compose.Workflow.Run`: each ready node in a topological wave is dispatched via `go t.execute()` (v0.9.4 `graph_manager.go:submit`). Defense: `parallel_batch_test.go` (structural 4-node compile) + `parallel_timing_test.go` (5-node static analysis) |
| 4.4 wait-for-user cycle | 🟡 canvas side done | HTTP handler side |
| 4.3 MultiBranch | 🟡 Switch surface done | runtime branch wiring |
| Tools (21+) | ✅ | — |
| 5a Universe A wrappers | ✅ real wrapper code shipped (Tavily/Retrieval/ExeSQL) | Phase 5a close-out: replace stub registrations |
| 5b RetrievalService | ✅ nlp.NewRetrievalService + kg.NewRetrieval wired at boot | real ChunkService depth + GraphRAG `use_kg=true` path |
| 5c ExeSQL Trino/DB2 | ❌ | driver registration + DSN switch |
| 5d CodeExec | ✅ 5 providers (self_managed / aliyun / e2b / local / ssh) | — |
| 5e `SearchMyDataset` alias | ✅ | — |
| 6 io hardening | 🟡 5 writers (pdf/docx/txt/markdown/html) | per-component audit |
| 8a chunked + Jinja2 | ✅ chunked streaming + gonja direct | remaining canvas templates |
| 8b TTS / rich content / memory | 🟡 TTS scaffold + rich content renderer + MemorySaver service | TTS HTTP clients + embedding port + boot wiring |

## 2. Boot wiring (the operator's mental model)

The Go runtime registers itself into three layers, in this order:

1. **ProviderManager** ([`internal/agent/sandbox/manager.go`](internal/agent/sandbox/manager.go)) — chooses which sandbox provider backs CodeExec. Default is `self_managed`; set `SANDBOX_PROVIDER_TYPE` env var to override.
2. **RetrievalService** ([`internal/agent/tool/retrieval_service.go`](internal/agent/tool/retrieval_service.go)) — chooses which backend backs `RetrievalTool` and `RetrievalComponent`. The Go server constructs `nlp.NewRetrievalService(docEngine, docDAO)` and `kg.NewRetrieval(...)` and registers them via `tool.SetRetrievalService(...)` at boot (see `cmd/server_main.go`).
3. **MemorySaver** ([`internal/agent/component/memory_save.go`](internal/agent/component/memory_save.go)) — backs the `Message` component's `_ERROR`-routed memory writes. Default is `stubMemorySaver` (loud-fail `ErrMemoryServiceMissing`); operators that need memory writes should call `component.SetMemorySaver(NewMemoryMessageService(...))` from their boot path.

Any one of these that is not wired at boot produces a **loud-fail sentinel** (e.g. `ErrRetrievalServiceMissing`, `ErrMemoryServiceMissing`, `ErrSandboxNotConfigured`). The stubs are NOT silent — they raise with a clear "Phase 5b boot wiring missing" message so operators can see what to fix.

## 3. Feature flags

| Env var | Default | Effect |
|---------|---------|--------|
| `SANDBOX_PROVIDER_TYPE` | `self_managed` | `self_managed` / `aliyun_codeinterpreter` / `e2b` / `local` / `ssh` |
| `SANDBOX_EXECUTOR_MANAGER_URL` | `http://sandbox-executor-manager:9385` | self-managed endpoint |
| `SANDBOX_EXECUTOR_MANAGER_TIMEOUT` | `30` (s) | self-managed per-call timeout |
| `AGENTRUN_*` (5 vars) | n/a | aliyun code interpreter |
| `E2B_API_KEY` / `E2B_ACCESS_TOKEN` | n/a | e2b (one required) |
| `E2B_TEMPLATE` | `base` | e2b sandbox template |
| `LOCAL_*` (8 vars) | n/a | local subprocess |
| `SSH_HOST` / `SSH_PORT` / `SSH_USERNAME` / `SSH_PASSWORD` / `SSH_PRIVATE_KEY` / `SSH_PRIVATE_KEY_PATH` | n/a | SSH provider |
| `COMPONENT_EXEC_TIMEOUT` | `600` (s) | canvas-level per-invocation timeout (read at `node_body.go:109-115`) |

Per-component-class timeout (Python's 10min for LLM / 12s for Tavily / 3s for invoke split) is **not** done — the Go side applies the uniform `COMPONENT_EXEC_TIMEOUT` to all components. Plan v3.3.1 OQ #6.

## 4. Migration tool

`tools/migrate-canvas` is the validation harness for Python→Go canvas migration. **Not yet implemented** (plan §7 + OQ #3). The plan recommends a Python subprocess that calls Python's `normalize_chunker_dsl` and Go's `NormalizeForCanvas` on the same input, then diffs the results. Status: **partial — Go-side `normalize_test.go` covers the Go normalizer; Python cross-comparison harness is a follow-up.**

Until the tool ships, manual migration steps:

1. Export canvas JSON from Python: `GET /api/v1/canvas/<id>/export`.
2. Validate Python normalizer: `uv run python -c "from agent.canvas import normalize_chunker_dsl; print(normalize_chunker_dsl(json.load(open('canvas.json'))))"`.
3. Validate Go normalizer: `go test ./internal/agent/dsl/ -run TestNormalize -v` (uses the same fixture corpus in `internal/agent/dsl/testdata/`).
4. Diff the two normalized forms. If structurally identical, the canvas is Go-portable.

## 5. Per-component parity

See [`docs/component-parity.md`](component-parity.md) for the auto-generated table. The script `tools/gen-component-parity/main.go` (planned) produces it from the current Go code by inspecting the `Registry` + each component's `Inputs()`/`Outputs()` methods.

## 6. Known deferred items (loud-fail sentinels)

If a user-visible feature returns a sentinel error, the operator should:

| Sentinel | Cause | Fix |
|----------|-------|-----|
| `ErrRetrievalServiceMissing` | `tool.SetRetrievalService(...)` not called at boot, OR `E2B_API_KEY` is empty for an e2b canvas | Wire the production `nlp.NewRetrievalService` at boot (already done in `cmd/server_main.go`) |
| `ErrKGRetrievalServiceMissing` | Canvas uses `use_kg=true` and `tool.SetKGRetrievalService(...)` not called at boot | Wire `tool.NewKGRetrievalAdapter(docEngine, modelProviderSvc)` at boot (already done in `cmd/server_main.go`) |
| `ErrMemoryServiceMissing` | `component.SetMemorySaver(...)` not called at boot | Wire `NewMemoryMessageService(memService)` — pending follow-up (plan v3.3.1 G-c) |
| `ErrEmbedderNotWired` | MemorySaver service's `embedAndSave` was reached but `internal/rag/llm/` is empty (embedding model port missing) | Port the embedding model — pending follow-up (plan v3.3.1 OQ #12) |
| `ErrSandboxNotConfigured` | Manager has no provider (e.g. `SANDBOX_PROVIDER_TYPE` set to unknown value) | Set `SANDBOX_PROVIDER_TYPE` to one of the 5 supported values |
| `ErrE2BProviderNotImplemented` | `SANDBOX_PROVIDER_TYPE=e2b` and `E2B_API_KEY`/`E2B_ACCESS_TOKEN` not set | Provide one of the two env vars |
| `ErrTTSEngineNotConfigured` | Message component runs with `auto_play=true` and no `audio.SetSynthesizer(...)` at boot | Wire a TTS engine at boot — pending follow-up (plan v3.3.1 G-a) |
| `ErrExeSQLUnsupportedDB` | `db_type` is `trino` or `ibm db2` | Add the driver registration — pending follow-up (plan v3.3.1 D) |

## 7. Rollout plan

The runtime ships **on by default** for all canvases — there is no `FEATURE_GO_*` env var gating. The plan §7 originally envisioned a per-tenant canary, but the current code path is:

1. **Internal dogfood** (in progress): all internal canvas builders use Go runtime.
2. **Operator pre-flight**: before running a canvas in Go, ensure the sentinels above are all wired. The Python fallback (route to `agent_api.py`) is still available for any non-Go-portable canvas.
3. **Per-canvas routing**: the orchestrator (e.g. `internal/handler/agent.go`) decides Go vs Python per canvas based on the operator's deployment choice; the runtime is fully Go-capable.

## 8. Testing

```sh
go test -count=1 ./internal/agent/...   # all agent tests
go test -count=1 ./internal/agent/component/   # 234+ tests; pre-existing TestSplitSentences failures unrelated to this work
go test -count=1 ./internal/agent/tool/   # 264+ tests; tool registry, retrieval, e2b, ssh, local, aliyun, code exec
go test -count=1 ./internal/agent/sandbox/  # 90+ tests; sandbox providers
go test -count=1 ./internal/agent/canvas/   # canvas engine, parallel, multibranch
go test -count=1 ./internal/agent/runtime/  # state, template, history window
```

`internal/agent/dsl/testdata/` has 7 fixture JSONs that drive the `dsl_examples_test.go` end-to-end suite. The fixtures are the same ones Python's `normalize_chunker_dsl` accepts.

## 9. References

- [`docs/develop/agent-go-port-design.md`](develop/agent-go-port-design.md) — main design doc (2133 lines, 17 sections)
- [`docs/develop/agent-go-port-status.md`](develop/agent-go-port-status.md) — current per-phase status snapshot
- [`docs/develop/sandbox-python-go-diff.md`](develop/sandbox-python-go-diff.md) — Python vs Go functional diff for the sandbox subsystem
- [`docs/component-parity.md`](component-parity.md) — per-component parity matrix
- [`.claude/plans/agent-go-port-gap-analysis.md`](../.claude/plans/agent-go-port-gap-analysis.md) — gap analysis plan (v3.3.1)
- [`internal/agent/component/`](internal/agent/component/) — 22 production components
- [`internal/agent/tool/`](internal/agent/tool/) — 21+ eino tools
- [`internal/agent/sandbox/`](internal/agent/sandbox/) — sandbox provider subsystem (5 providers)
- [`internal/service/nlp/retrieval.go`](internal/service/nlp/retrieval.go) — KB retrieval backend (1079 lines)
- [`internal/service/kg/retrieval.go`](internal/service/kg/retrieval.go) — GraphRAG backend (264 lines)
