# Agent Canvas Go Port — Design Document

> **Last cross-checked against code:** 2026-06-17
> **Source of truth:** `internal/agent/` (canvas, component, tool, runtime, workflowx, dsl, audio, sandbox, observability/otel/) + `internal/service/` (agent.go, canvas_decode.go) + `internal/handler/` (agent.go) + `tools/` (gen-component-parity, migrate-canvas)

---

## 0. How to read this document

- **Sections 1–13** describe the current design.
- **Section 14** lists the actionable backlog for the next iteration.
- **Appendices** preserve the per-component / per-tool inventories and corner-case catalogues.

---

## 1. 概述 / Overview

### 1.1 目标

RAGFlow 的 Agent Canvas（编排 22 个 component + 21 个 tool 的 DSL 执行器）从 Python 移植到 Go。Python 端位于 `agent/canvas.py`（`Graph` / `Canvas`）+ `agent/component/base.py`（`ComponentBase` / `ComponentParamBase`）+ `agent/tools/`。Go 端独立实现于 `internal/agent/`，与 Python 端通过共享 DSL JSON schema 兼容（v1↔v2 双向转换器在 `internal/agent/dsl/`，已收敛为单一 wire 形态）。

### 1.2 核心架构决策

**State + Workflow 混血**：eino 的 `compose.Workflow` 提供声明式拓扑（节点 + exec 边）+ 并发调度；`compose.WithGenLocalState` + `WithStatePreHandler/WithStatePostHandler` 提供任意节点读任意节点输出的"状态变量"能力。State 解决 `{{cpn_id@param}}` 任意交叉引用问题，Workflow 解决执行拓扑 + cancel + checkpoint 问题。

**5-tier 移植策略**：T1（直接复用 eino 内置）→ T2（薄包装）→ T3（Lambda + State）→ T4（嵌套 Workflow 子图）→ T5（重 I/O + 第三方 lib）。判定原则：功能相当 → 优先 eino 内置，禁止复制 Python 端的黑魔法（`_feeded_deprecated_params`、partial hack、`thread_pool_exec` 异步伪装等）。

**Checkpoint 存 Redis**：eino `compose.CheckPointStore` 是纯 KV 接口，Redis String + EXPIRE 是天然 fit。业务元数据（status / canvas_id / parent_run_id）走独立 Redis Hash（**由应用层显式控制**，不依赖 eino 自动写）。

**Observability 走 OpenTelemetry**：用 OTLP HTTP exporter + eino `callbacks.Handler` 注入 span。

**AGPL-3 零容忍**：T5 DOCX 库穷举后全部 AGPL-3/维护停滞，**自实现 OOXML writer**（`archive/zip` stdlib + `text/template`）；PDF 选 `signintech/gopdf` (MIT)；Excel 选 `xuri/excelize/v2` (BSD-3)；Markdown 选 `yuin/goldmark` (MIT)。

**Wait-for-User 用 eino 原生 interrupt**：废除自实现 sentinel 链路（`__wait_for_user__` / `_user_input_provided` / synthetic Loop / `cycle_wrap.go` / `wait_for_user.go`）；改用 `compose.Interrupt` + `compose.ResumeWithData` 一等 API，节点内 `compose.GetResumeContext[T](ctx)` 读用户输入。

**Real Compile/Invoke 接入生产链**：`buildRunFunc` 驱动真正的 `canvas.Compile` → `CompiledCanvas.Invoke` 流程。

### 1.3 Reuse-First Principle

Before adding any new component, runtime abstraction, or third-party dependency, every phase must check whether the capability already exists elsewhere in the codebase or its declared dependencies.

**Decision order** (apply in sequence; first match wins):

1. **Reuse the existing RAGFlow model/service capability as-is.** If `internal/entity/models/anthropic.go`, `internal/handler/chat_session.go`, or similar already has the capability, just wire it through — don't reimplement.
2. **Wrap an existing eino / workflowx / MCP-client primitive.** If eino's `compose.NewGraphMultiBranch` or `workflowx.AddLoopNode` or `internal/utility/mcp_client.go` already provides the mechanism, build a thin adapter.
3. **Promote an already-declared-but-indirect dependency.** If the dependency is already in `go.mod` (even as `// indirect`), the work is to import it directly and use it.
4. **Add a registry alias only (no new body)** when an existing engine-level mechanism already handles the semantics.
5. **Only as a last resort** — add a new component, a new interface method, or a new third-party dependency. Each such addition must come with a written justification explaining why steps 1-4 don't apply.

**Anti-patterns** explicitly rejected: ❌ Adding `InvokeAsync` to the `Component` interface (would compete with eino `compose.Parallel`); ❌ Registering `LoopItem` / `ExitLoop` as components; ❌ Reimplementing Python's runtime path extension in Go; ❌ Building a new MCP subsystem; ❌ "Introducing" gonja (it's already a declared dep).

---

## 2. 顶层模块布局 / Module Layout

```
internal/agent/
├── canvas/              # 画布执行器（eino 编译、状态调度、checkpoint、cancel、stream、interrupt）
│   ├── canvas.go        # Canvas struct, BuildWorkflow, Run/Stream
│   ├── runner.go        # canvas.Runner; SSE event emission + interrupt catch
│   ├── scheduler.go     # State pre/post handler + 节点 lambda + legacyNoOpNames
│   ├── node_body.go     # 单节点 lambda 体 (per-class timeout via resolveTimeout)
│   ├── timeout.go       # componentDefaults map; 4-level resolver (per-class env → per-class table → uniform env → 600s fallback)
│   ├── loop_subgraph.go # Loop 宏展开 (buildSubWorkflow + translateLoopCondition)
│   ├── interrupt_resume.go # eino interrupt 封装: UserFillUpNodeBody / IsInterruptError / ExtractInterruptContexts
│   ├── multibranch.go   # Switch / Categorize 路由的 eino MultiBranch 集成
│   ├── cancel.go        # Redis cancel 协议 (watchCancel goroutine)
│   ├── stream.go        # SSE 通道
│   ├── compile.go       # eino 编译 + WithCheckPointStore + checkPointAdapter (不覆盖 InternalSerializer)
│   ├── checkpoint_store.go  # RedisCheckPointStore (Get/Set/Delete) — interface 包含 Delete
│   ├── run_tracker.go   # RunTracker (Start/MarkSucceeded/MarkFailed/MarkCancelled/AttachCheckpoint)
│   ├── state_serializer.go  # CanvasStateSerializer (encoding/json)
│   └── state_export.go  # WithState / GetStateFromContext 薄重导出
│
├── component/           # 19 components + 6 helpers (含 fixture_stubs.go + universe_a_wrappers.go)
│   ├── base.go          # Component interface + ParamError + ErrNotImplemented
│   ├── registry.go      # name → factory 映射 (auto-init)
│   ├── runtime_wire.go  # 组件与 runtime 包的桥接
│   ├── io_init.go       # T5 组件初始化
│   ├── fixture_stubs.go # IterationStub / IterationItemStub / RetrievalStub / SearchMyDataset alias / ExeSQLStub
│   ├── universe_a_wrappers.go # newRetrievalComponent / newExeSQLComponent / newTavilySearchComponent — Universe A → Universe B 委派
│   ├── production_chain_fixes_test.go   # 生产链回归 pin 测试
│   ├── agent.go         # T1 — react.NewAgent + tool artifact capture + maybeAppendCitation + Reset() interface-assert
│   ├── llm.go           # T1 — EinoChatModel 薄包装; VisualFiles / Cite / MessageHistoryWindowSize / ChatTemplateKwargs / OutputStructure / JSONOutput / TopP / MaxRetries / DelayAfterError
│   ├── llm_retry.go     # retryInvoker + Unwrap(); unwrapChatInvoker 辅助
│   ├── switch.go        # T2 — 12 of 12 operators (==/!=/contains/not contains/start with/end with/empty/not empty/>/</>=/<=)
│   ├── begin.go / message.go / categorize.go / invoke.go / browser.go
│   ├── data_operations.go / list_operations.go / string_transform.go
│   ├── variable_aggregator.go / variable_assigner.go
│   ├── fillup.go / userfillup.go
│   ├── loop.go          # T4 — no-op marker, 实际工作由 loop_subgraph 接管
│   ├── parallel.go      # T4 — workflowx.AddParallelNode 包装
│   ├── docs_generator.go / excel_processor.go   # T5
│   └── render.go        # output_format HTML/Markdown/plain renderer
│
├── tool/                # 21 tools (统一 eino tool.InvokableTool)
│   ├── registry.go      # BuildAll / BuildByName (alias: exesql=execute_sql, retrieval=search_my_dateset=search_my_dataset)
│   ├── http_helper.go   # 共用 HTTP client (context + retry + backoff)
│   ├── ssrf.go          # SSRF 防护
│   ├── mcp.go           # MCPToolAdapter — InvokableRun 调 mcpclient.CallTool over streamable-HTTP
│   ├── retrieval.go / retrieval_service.go / retrieval_nlp.go / retrieval_kg.go  # RetrievalService 双 registry: nlp + kg
│   ├── sandbox_bridge.go # CodeExec sandbox providers 桥接
│   └── akshare.go / arxiv.go / code_exec.go / code_exec_client.go / crawler.go / deepl.go
│       / duckduckgo.go / email.go / exesql.go / github.go / google.go
│       / google_scholar.go / jin10.go / pubmed.go / qweather.go
│       / searxng.go / tavily.go / tushare.go / wencai.go / wikipedia.go / yahoo_finance.go
│
├── runtime/             # canvas + component 共享的运行时契约（无 cycle）
│   ├── component.go     # Component interface (从 component/base.go 提取)
│   ├── context.go       # GetStateFromContext / withState
│   ├── state.go         # CanvasState + NewCanvasState + GetVar/SetVar/ReadVars + MarshalJSON/UnmarshalJSON + compose.RegisterSerializableType
│   ├── template.go      # ResolveTemplate (regex 快速路径)
│   ├── template_jinja.go # gonja 兜底
│   ├── selector.go      # component selector 辅助
│   └── metrics.go       # runtime metrics + Prometheus counters
│
├── workflowx/           # eino 扩展（零侵入，外部 helper）
│   ├── loop.go          # AddLoopNode[T] — 通用 do-while 循环节点
│   ├── parallel.go      # AddParallelNode[I,O] — 通用 bounded-concurrency 节点
│   └── *_test.go        # 单元 + 集成测试
│
├── sandbox/             # CodeExec 沙箱 providers
│   ├── provider.go / manager.go / http.go / result_protocol.go / artifacts.go
│   ├── self_managed.go / aliyun.go / e2b.go / local.go / ssh.go
│   └── e2b_test.go / local_test.go / manager_test.go / result_protocol_test.go / self_managed_test.go / ssh_test.go
│
├── audio/               # TTS
│   ├── tts.go                # Synthesizer interface + 错误哨兵 + 默认 stub
│   ├── model_provider_synthesizer.go  # calls models.BaseModel.AudioSpeech (60+ driver impls)
│   ├── tts_dispatch.go       # TTSDispatcher interface + NewTTSDispatchFunc
│   └── *_test.go
│
├── observability/otel/  # OTel SDK + eino callbacks.Handler
│   ├── provider.go      # TracerProvider 工厂
│   └── handler.go       # eino callbacks.Handler → OTel span
│
└── dsl/                 # DSL normalize
    ├── normalize.go     # NormalizeForCanvas (enforceHandleIds / buildGraphFromComponents / foldLegacyLoopVariants)
    ├── normalize_test.go
    └── testdata/        # 7 fixtures (all / browser / dfx_picture_parser / questions_category / resume / subaget / switch)

internal/handler/
├── agent.go             # HTTP API (RunAgent SSE with RunEvent.Type dispatch)
├── agent_wait_for_user_test.go  # 4 e2e tests pinning wait-for-user orchestrator side
└── admin_runtime.go     # POST /api/v1/admin/canvas-runtime/:tenant_id

internal/service/
├── agent.go             # AgentService.RunAgent / buildRunFunc / NewAgentService[WithOptions] / option injection
├── canvas_decode.go     # decodeCanvasFromDSL
├── canvas_decode_test.go
├── agent_run_e2e_test.go  # 4 e2e tests
└── agent_sessions.go    # session CRUD

cmd/server_main.go       # Redis CheckPointStore + RunTracker + TTS service wire-up

internal/observability/otel/
├── provider.go          # TracerProvider 工厂 (读 OTEL_EXPORTER_OTLP_ENDPOINT)
├── handler.go           # eino callbacks.Handler → OTel span
└── handler_test.go      # tracetest.SpanRecorder
```

**实际文件计数**：

- Components: **19 个** — 见 §4.2
- Tools: **21 个** — 见 §4.5
- Sandbox providers: **5 个** (self_managed, aliyun, e2b, local, ssh)
- Test files: 60+ (canvas 17, component 50+, tool 30+, runtime 4, workflowx 8, sandbox 6, audio 3, service 8+, handler 10+)

---

## 3. 架构 / Architecture

### 3.1 State + Workflow 混血

eino `compose.Workflow` 本身只支持 DAG（节点间数据通过 declared predecessor 输出传递），没有"任意节点读任意节点输出"的现成 API。RAGFlow Python 端用 `self._canvas.get_variable_value("cpn_id@param")` 实现 `{{cpn_id@param}}` 任意交叉引用。

**Go 端方案**：

1. **State 承载变量**：每个 canvas run 创建 `*CanvasState`，挂在 `context.Value` 上。所有节点通过 `runtime.GetStateFromContext(ctx)` 读写。
2. **State pre-handler**：在 `wf.AddLambdaNode(...)` 时挂 `compose.WithStatePreHandler[map[string]any, *runtime.CanvasState](canvasPre)`，从 State 提取节点输入。
3. **State post-handler**：挂 `compose.WithStatePostHandler`，把节点输出回写 State。
4. **Workflow 承载拓扑**：节点按 `downstream` / `upstream` 加 exec 边，**数据流走 State 不走边**。

```go
// internal/agent/canvas/scheduler.go
node := wf.AddLambdaNode(cpnID, nodeBody,
    compose.WithStatePreHandler[map[string]any, *runtime.CanvasState](canvasPre),
    compose.WithStatePostHandler[map[string]any, *runtime.CanvasState](canvasPost),
)
for _, upID := range comp.Upstream {
    node.AddInput(upID)
}
```

**CanvasState 序列化**：

`CanvasState` 结构包含 `sync.RWMutex`，原生无法被 `encoding/json` 序列化（`Marshaler` 接口与 mutex 不兼容）。通过：

- `MarshalJSON` / `UnmarshalJSON` 方法 — 输出/读取 `canvasStateJSON` 内部结构（不暴露 mutex）
- `compose.RegisterSerializableType[CanvasState]` — 让 eino `StatePre/PostHandler` 在 interrupt path 能 marshal/unmarshal state

eino `InternalSerializer` 是另一个独立的序列化机制（eino 内部 checkpoint payload），**不**与 `WithStateSerializer`/`compose.Serializer` 共享。生产代码只 wire `WithCheckPointStore` (保留 eino `InternalSerializer` 默认值) + CanvasState 自带 `MarshalJSON`。

### 3.2 `runtime` 包：消除 `canvas <-> component` cycle

**问题**：`component/` 大量文件（Begin/Message/Switch/Browser/...）需要调 `canvas.CanvasState` / `canvas.GetStateFromContext` / `canvas.ResolveTemplate` / `canvas.SetDefaultFactory`；同时 `canvas` 通过 `ComponentFactory` 间接依赖 `component` 的具体实现。强行 `canvas -> component` 形成 Go import cycle。

**方案**：把"运行时共用契约"提取到 `internal/agent/runtime/`，**canvas 和 component 都依赖 runtime，但不互相依赖**。

| 提取到 runtime | 留在 canvas | 留在 component |
|---------------|-------------|----------------|
| `Component` interface | DSL graph types (`Canvas`, `CanvasComponent`, `CanvasComponentObj`) | component registry + factory |
| `CanvasState` + `GetVar/SetVar/ReadVars` + MarshalJSON | 拓扑构建 (`BuildWorkflow`, `buildLoopExpansion`, scheduler wiring) | 具体 component 实现 |
| `GetStateFromContext` / `withState` / `WithState` | checkpoint / workflow 编译 orchestration | `NewBeginComponent`, `NewMessageComponent`, ... |
| `ResolveTemplate` + `template_jinja` (gonja fallback) | Loop 宏展开 logic | |
| `ParamError`, `ErrNotImplemented` | | |

### 3.3 eino interrupt 路径

```
UserFillUp 节点 → compose.Interrupt(ctx, inputSpec)
                          ↓
            返回 *InterruptSignal (实现 error 接口)
                          ↓
            图引擎捕获 → 自动 checkpoint → 向上传播
                          ↓
       Runner.Run 捕获 → SSE "waiting_for_user" + 保存 interrupt id
                          ↓
        用户提交 → Runner.Run 注入 __resume_interrupt_id__ + __resume_data__
                          ↓
            buildRunFunc 消费 → compose.ResumeWithData(ctx2, id, data)
                          ↓
          节点重入 → 顶部 compose.GetResumeContext[T](ctx) → 返回用户输入
```

**核心实现**：

```go
// internal/agent/canvas/interrupt_resume.go
func UserFillUpNodeBody(cpnID string, params map[string]any) func(ctx context.Context, input map[string]any) (map[string]any, error) {
    inputSpec := buildInputSpec(params)
    return func(ctx context.Context, input map[string]any) (map[string]any, error) {
        // Resume path: 节点重入时, 顶部检查 resume context
        if isResume, hasData, data := compose.GetResumeContext[any](ctx); isResume && hasData {
            return map[string]any{
                "user_input": data,
                cpnID:        data,
            }, nil
        }
        // 首次执行: 调 Interrupt 暂停图
        if err := compose.Interrupt(ctx, inputSpec); err != nil {
            return nil, err
        }
        return nil, errors.New("UserFillUp: interrupt did not halt execution")
    }
}
```

**Runner.Run interrupt catch**（`internal/agent/canvas/runner.go`）：

```go
if info, ok := compose.ExtractInterruptInfo(runErr); ok {
    ctxs := info.InterruptContexts // []*compose.InterruptCtx
    if len(ctxs) > 0 {
        d.saveInterruptID(canvasID, sessionID, ctxs[0].ID)
        payload, _ := json.Marshal(WaitingForUserEvent{CpnID: ctxs[0].ID})
        push(out, RunEvent{Type: "waiting_for_user", Data: string(payload)})
        return
    }
}
```

**Resume 传参**（`buildRunFunc`）：

```go
if resumeID, ok := root["__resume_interrupt_id__"].(string); ok && resumeID != "" {
    resumeData := root["__resume_data__"]
    delete(root, "__resume_interrupt_id__")
    delete(root, "__resume_data__")
    ctx2 = compose.ResumeWithData(ctx2, resumeID, resumeData)
}
```

**Cycle 处理**：前端契约保证生产画布无环（`hasCanvasCycle` 阻止保存），eino 的 DAG 检查在 `Compile()` 时自动拒绝有环图，无需额外防御。

### 3.4 真实 Compile/Invoke 接入生产链

```go
// internal/service/agent.go — buildRunFunc

func (s *AgentService) buildRunFunc(canvasID string, versionRow *entity.UserCanvasVersion, dsl map[string]any) canvas.RunFunc {
    return func(ctx context.Context, root map[string]any) (*canvas.CanvasState, error) {
        if err := ctx.Err(); err != nil {
            return nil, err
        }

        taskID := ""
        if versionRow != nil {
            taskID = versionRow.ID
        }

        c, err := decodeCanvasFromDSL(dsl)
        if err != nil {
            return nil, err
        }

        runID := canvasID
        if sessionID, ok := root["session_id"].(string); ok && sessionID != "" {
            runID = runID + "-" + sessionID
        }
        state := canvas.NewCanvasState(runID, taskID)

        userInput, _ := root["user_input"].(string)
        state.Sys["query"] = userInput
        ctx2 := runtime.WithState(ctx, state)

        if resumeID, ok := root["__resume_interrupt_id__"].(string); ok && resumeID != "" {
            resumeData := root["__resume_data__"]
            delete(root, "__resume_interrupt_id__")
            delete(root, "__resume_data__")
            ctx2 = compose.ResumeWithData(ctx2, resumeID, resumeData)
        }

        if s.runTracker != nil {
            _ = s.runTracker.Start(ctx2, runID, canvasID, tenantIDFromRoot(root), userInput)
        }

        var cc *canvas.CompiledCanvas
        if s.checkpointStore != nil && s.stateSerializer != nil {
            cc, err = canvas.Compile(ctx2, c,
                canvas.WithCheckPointStore(s.checkpointStore),
                canvas.WithStateSerializer(s.stateSerializer),
            )
        } else {
            cc, err = canvas.Compile(ctx2, c)
        }
        if err != nil {
            s.markRunFailed(ctx2, runID, "compile: "+err.Error())
            return nil, fmt.Errorf("canvas compile: %w: %w", err, ErrAgentStorageError)
        }

        if s.runTracker != nil {
            _ = s.runTracker.AttachCheckpoint(ctx2, runID, runID)
        }

        _, err = cc.Workflow.Invoke(ctx2, map[string]any{"query": userInput})
        if err != nil {
            if canvas.IsInterruptError(err) || canvas.ExtractInterruptContexts(err) != nil {
                s.markRunFailed(ctx2, runID, "interrupt: "+err.Error())
                return state, err
            }
            s.markRunFailed(ctx2, runID, "invoke: "+err.Error())
            return nil, fmt.Errorf("canvas invoke: %w: %w", err, ErrAgentStorageError)
        }

        s.markRunSucceeded(ctx2, runID)
        return state, nil
    }
}
```

**AgentService option injection**（`internal/service/agent.go`）：

```go
type AgentService struct {
    // ... existing fields
    checkpointStore canvas.CheckPointStore  // nil = in-memory (test path)
    stateSerializer canvas.StateSerializer  // nil = eino default
    runTracker      *canvas.RunTracker      // nil = best-effort no-tracking
    runner          *canvas.Runner
}

func NewAgentService() *AgentService {
    return NewAgentServiceWithOptions(nil, nil, nil)
}

func NewAgentServiceWithOptions(
    cp canvas.CheckPointStore,
    ser canvas.StateSerializer,
    rt *canvas.RunTracker,
) *AgentService {
    return &AgentService{...}
}
```

**Production boot wiring**（`cmd/server_main.go`）：

```go
// SetRedisCheckPointStore + CanvasStateSerializer + RunTracker → NewAgentServiceWithOptions
// + configureTTSSynthesizer (audio.SetModelProviderSynthesizer)
// Redis 不可达时 graceful degradation: 退化为 in-memory (nil options)
```

**DSL decoder**（`internal/service/canvas_decode.go`）：

`decodeCanvasFromDSL` 接受两种形态：
1. **IMPORT shape**: `obj.component_name` / `obj.params` (Python v1 DSL 直接写入)
2. **NormalizeForCanvas output shape**: 扁平 `name` / `params` (生产路径走 NormalizeForCanvas)

不采用 JSON round-trip — 直接 map walking 更清晰，因为生产路径已通过 `NormalizeForCanvas` 扁平化。所有失败模式 wrap `ErrAgentStorageError`。

---

## 4. Component 库 / Component Library

### 4.1 5-tier 移植策略

| Tier | 含义 | 验收 |
|------|------|------|
| **T1** | 直接用 eino 已有类型/接口，零代码 | eino 单元测试覆盖 |
| **T2** | 薄包装 1 struct + factory，对齐 Python 行为参数 | 跨 eino/RAGFlow 边界 + 1 e2e |
| **T3** | `compose.Lambda` + `StatePre/PostHandler` | 1 单测 + 1 e2e |
| **T4** | 嵌套 `compose.Workflow` + `getState[CanvasState](ctx)` | 子图单测 + 完整 e2e |
| **T5** | 重 I/O + 第三方 lib | 单测 + e2e + 失败注入 |

**判定原则**：T1 > T2 > T3 > T4 > T5 时**禁止跳级**。

### 4.2 Component 现状（19 个 .go 文件）

| Component | Python 行为 | Tier | Go 实现 | 状态 |
|-----------|------------|------|---------|------|
| **LLM** | `LLMBundle` 单轮 chat + JSON output + cite + stream | T1 | `EinoChatModel` 薄包装 `internal/entity/models/<provider>.go`；实现 `model.ToolCallingChatModel`；`retryInvoker.Unwrap()` + `unwrapChatInvoker` 实现 normal-absolute-count retry 语义 | ✅ |
| **Agent** | ReAct + tool/MCP + 多轮 stream | T1 | `react.NewAgent` + `compose.ToolsNodeConfig{Tools: tools}` + 22 tool 全注册；citation 中间件 + tool artifact 收集已实现；`Reset()` 走 `interface{ Reset() }` 类型断言 | ✅ |
| **Switch** | 多条件 (and/or) → 多 downstream + ELSE | T2 | `compose.NewGraphMultiBranch` 路由；12 of 12 operators (`==`/`!=`/`contains`/`not contains`/`start with`/`end with`/`empty`/`not empty`/`>`/`<`/`>=`/`<=`) + case-insensitive string ops | ✅ |
| **Categorize** | LLM 分类 + 路由 | T3 | Lambda 调 LLM + `compose.NewGraphMultiBranch` | ✅ |
| **Begin** | DSL 入口 + 注入 inputs + 文件 inputs | T3 | Lambda + `StatePreHandler`；文件走 `internal/service/file_service.go` | ✅ |
| **UserFillUp / Fillup** | Jinja2 + file inputs + **wait-for-user interrupt** | T3 | `text/template` 替代 Jinja2 + eino interrupt via `interrupt_resume.go` | ✅ |
| **Message** | 最终输出（jinja2 + stream + downloads + filegen + TTS + memory） | T3 | Lambda + `schema.StreamReader` + `text/template` + MinIO + TTS dispatch + MemorySaver | 🟡 真实 TTS binary + MemorySaver completion deferred |
| **Invoke** | HTTP 客户端 + HTML 清洗 + JSON | T3 | `net/http` + `golang.org/x/net/html` | ✅ |
| **Browser** | LLM + HTTP + 文件下载 + MinIO | T3 | 复用 Invoke + LLM + storage | ✅ |
| **DataOperations** | dict 7 类操作 | T3 | Lambda + `encoding/json` + `go/ast` | ✅ |
| **ListOperations** | slice 6 类操作 | T3 | Lambda + `slices` (Go 1.21+ stdlib) | ✅ |
| **StringTransform** | split/merge + Jinja2 | T3 | Lambda + `strings.Split` + `text/template` | ✅ |
| **VariableAggregator** | 多 group，first-non-empty | T3 | Lambda + State 读 | ✅ |
| **VariableAssigner** | 11 个算子原地改 State | T3 | Lambda + State 写 | ✅ |
| **Loop** | 条件循环 + `loop_variables` 初始化 + 终止评估 | T4 | `compose.NewWorkflow` + `workflowx.AddLoopNode`（loop.go 自身变为 no-op marker；实际工作由 `canvas/loop_subgraph.go` 宏展开接管） | ✅ |
| **Parallel** | 数组并行处理 | T4 | `workflowx.AddParallelNode` 包装 | ✅ |
| **DocsGenerator** | pdf/docx/txt/md/html 生成 | T5 | `signintech/gopdf` (PDF) + 自实现 OOXML writer (DOCX) + `yuin/goldmark` (MD)；`render.go` 提供 HTML/Markdown/plain rendering | 🟡 txt/md/html writers 部分缺失 |
| **ExcelProcessor** | pandas 读/合并/转换 Excel | T5 | `xuri/excelize/v2` (BSD-3) | ✅ |
| **Retrieval (Universe A)** | canvas DAG node | T2 | `newRetrievalComponent` — 委派给 Universe B `RetrievalTool` | ✅ |

### 4.3 不移植的 Python 端"遗产" / Iteration LoopItem ExitLoop 重分类

| Python 端 | 不移植原因 |
|----------|-----------|
| `_feeded_deprecated_params` / `_deprecated_params` / `_user_feeded_params` 三层装饰 | DSL v2 已去除；Go `ComponentParamBase` 不引入 |
| `ComponentParamBase.validate()` + `param_validation/*.json` 96 文件 | Go struct tag + `go-playground/validator/v10` 替代 |
| `ComponentBase.thread_limiter = asyncio.Semaphore(...)` | Go `errgroup.SetLimit(MAX_CONCURRENT_CHATS)` (stdlib x/sync) |
| `partial` 流式 hack | eino `schema.StreamReader` 原生流式 |
| `thread_pool_exec(self._invoke, **kwargs)` 异步伪装 | Go 全程 goroutine |
| `set_output("_ERROR", ...)` + `set_exception_default_value()` 双轨 | Go `error` 单一返回 + eino `OnError` callback |
| `ExitLoop` no-op 节点 | DSL v1 compat 通过 `legacyNoOpNames` 在 canvas 层吸收，**不注册 component** |
| `LoopItem` 组件 | LoopItem 角色由 `workflowx.AddLoopNode` 内部 machinery 取代，**不注册 component**；`TestLoop_Registered` enforces absence |
| `Iteration` / `IterationItem` 组件 | `IterationStub` + `IterationItemStub` 注册为 compat stubs（DSL round-trip） |

### 4.4 Two Registry Universes (Universe A vs Universe B)

```
┌──────────────────────────────────────────────────────────────┐
│ Universe A — Canvas DAG Components                            │
│   Registry: internal/agent/component/registry.go (auto-init) │
│   Interface: Component { Invoke, Stream, Inputs, Outputs }   │
│   Output: map[string]any                                     │
│   Names: PascalCase — Retrieval, TavilySearch, ExeSQL,       │
│          Answer, Generate, Begin, LLM, Switch, …              │
│   Used by: Canvas DAG nodes (placed on the canvas directly)  │
├──────────────────────────────────────────────────────────────┤
│ Universe B — Agent ReAct Tools                               │
│   Registry: internal/agent/tool/registry.go                  │
│   Interface: einotool.BaseTool { Info, InvokableRun }        │
│   Output: JSON string (envelope)                             │
│   Names: snake_case — retrieval, tavily, execute_sql, …      │
│   Used by: Agent component's tools=["…"] list, called via    │
│            eino ReAct loop                                   │
└──────────────────────────────────────────────────────────────┘
```

**Mapping table**：

| Universe A (PascalCase) | Universe B (snake_case) | 当前状态 |
|---|---|---|
| Retrieval | `retrieval` / `search_my_dateset` / `search_my_dataset` | 委派到 Universe B real (nlp + kg 双 backend) |
| ExeSQL | `execute_sql` / `exesql` | 委派到 Universe B real (mysql/pg/mssql/oceanbase/trino) |
| TavilySearch | `tavily` | 委派到 Universe B real |
| Answer | — | 需要 orchestrator-side pause/resume（已通过 eino interrupt 实现） |
| Generate | — | alias to LLM component |
| SearchMyDataset | — | 注册为 Retrieval alias (4 spellings: PascalCase + snake_case + Python-typo) |

### 4.5 Tool 实现统一模式

```go
// internal/agent/tool/registry.go
type Tool interface {
    einotool.InvokableTool  // eino 协议：Info() / InvokableRun(ctx, args, opts)
}

func BuildAll(names []string, params map[string]map[string]any) ([]einotool.BaseTool, error)
func BuildByName(name string, params map[string]any) (einotool.BaseTool, error)
```

**21 tool 表** (alias 不算新 tool): akshare, arxiv, code_exec, crawler, deepl, duckduckgo, email, exesql(=execute_sql), github, google, google_scholar, jin10, pubmed, qweather, retrieval(=search_my_dateset=search_my_dataset), searxng, tavily, tushare, wencai, wikipedia, yahoo_finance。

**Retrieval 双 registry**：
- `internal/agent/tool/retrieval_nlp.go` — `NLPRetrievalAdapter` 桥接 `nlp.RetrievalService`
- `internal/agent/tool/retrieval_kg.go` — `KGRetrievalAdapter` 桥接 `kg.Retrieval(...)` (GraphRAG, `use_kg=true`)
- `internal/agent/tool/retrieval_service.go` — 两个独立 `SetRetrievalService` / `SetKGRetrievalService` registry; un-wired 返回 `ErrRetrievalServiceMissing` / `ErrKGRetrievalServiceMissing`

**MCP tools**：`internal/agent/tool/mcp.go` — `MCPToolAdapter.InvokableRun` 通过 `mcpclient.CallTool` over streamable-HTTP dispatch。

**Tool 通用模式**：HTTP 类 tool 走 `http_helper.go` (context + retry + 指数 backoff)；ExeSQL 走 stdlib `database/sql` + 各 driver (mysql / pg / mssql / oceanbase / trino)；CodeExec 走 `internal/agent/sandbox/` 5 providers (`self_managed` / `aliyun` / `e2b` / `local` / `ssh`) + `tool/sandbox_bridge.go` 桥接；Retrieval 走进程内 `internal/service/nlp/retrieval.go` (Dealer 后端已 Go 化)。

### 4.6 Component & Tool Inventory

Parity legend: ✅ implemented & tested · 🟡 scaffolded (loud-fail sentinel) · ⚠️ implemented with a known gap vs Python.

#### Universe A — Canvas DAG components (24)

| Name | Source | Status |
|------|--------|--------|
| Agent | `internal/agent/component/agent.go` | ✅ |
| Begin | `internal/agent/component/begin.go` | ✅ |
| Browser | `internal/agent/component/browser.go` | ✅ |
| Categorize | `internal/agent/component/categorize.go` | ✅ |
| DataOperations | `internal/agent/component/data_operations.go` | ✅ |
| DocsGenerator | `internal/agent/component/docs_generator.go` | ✅ |
| ExcelProcessor | `internal/agent/component/excel_processor.go` | ✅ |
| ExeSQL | `internal/agent/component/universe_a_wrappers.go` | ⚠️ Wrapper exists; registry primary still stub |
| Fillup | `internal/agent/component/fillup.go` | ✅ |
| Generate | `internal/agent/component/fixture_stubs.go` | ✅ Legacy alias for DSL round-trip |
| Invoke | `internal/agent/component/invoke.go` | ✅ |
| Iteration | `internal/agent/component/fixture_stubs.go` | ✅ Legacy alias; compat stub |
| IterationItem | `internal/agent/component/fixture_stubs.go` | ✅ Legacy alias; compat stub |
| ListOperations | `internal/agent/component/list_operations.go` | ✅ |
| LLM | `internal/agent/component/llm.go` | ✅ |
| Loop | `internal/agent/component/loop.go` | ✅ Engine-level macro (`LoopItem`/`ExitLoop` deliberately not registered) |
| Message | `internal/agent/component/message.go` | 🟡 TTS real engine + MemorySaver completion still deferred |
| Parallel | `internal/agent/component/parallel.go` | ✅ |
| Retrieval | `internal/agent/component/universe_a_wrappers.go` | ⚠️ Wrapper exists; registry primary still stub (also covers `SearchMyDataset` alias) |
| StringTransform | `internal/agent/component/string_transform.go` | ✅ |
| Switch | `internal/agent/component/switch.go` | ✅ All 12 operators with case-folded string ops |
| TavilySearch | `internal/agent/component/universe_a_wrappers.go` | ⚠️ Wrapper exists; registry primary still stub |
| UserFillUp | `internal/agent/component/userfillup.go` | ✅ |
| VariableAggregator | `internal/agent/component/variable_aggregator.go` | ✅ |
| VariableAssigner | `internal/agent/component/variable_assigner.go` | ✅ |
| Answer | `internal/agent/component/fixture_stubs.go` | 🟡 Compat stub; canvas pause/resume is real but the Answer node is still a placeholder |

> **Stub vs wrapper**: `Retrieval` / `TavilySearch` / `ExeSQL` have real delegation wrappers in `universe_a_wrappers.go`; the registry still maps them to stubs in `fixture_stubs.go`. Tracked in §14.

#### Universe B — eino ReAct tools (25 = 23 standalone + 2 aliases)

| Name | Source | Status |
|------|--------|--------|
| akshare | `internal/agent/tool/akshare.go` | ✅ |
| arxiv | `internal/agent/tool/arxiv.go` | ✅ |
| code_exec | `internal/agent/tool/code_exec.go` + `code_exec_client.go` | ✅ All 5 sandbox providers |
| crawler | `internal/agent/tool/crawler.go` | ✅ |
| deepl | `internal/agent/tool/deepl.go` | ✅ |
| duckduckgo | `internal/agent/tool/duckduckgo.go` | ✅ |
| email | `internal/agent/tool/email.go` | ✅ |
| execute_sql | `internal/agent/tool/exesql.go` | ⚠️ SELECT-only; rejects Trino/DB2 (`ErrExeSQLUnsupportedDB`) |
| exesql | `internal/agent/tool/exesql.go` | ⚠️ Alias of `execute_sql` |
| github | `internal/agent/tool/github.go` | ✅ |
| google | `internal/agent/tool/google.go` | ✅ |
| google_scholar | `internal/agent/tool/google_scholar.go` | ✅ |
| jin10 | `internal/agent/tool/jin10.go` | ✅ |
| mcp | `internal/agent/tool/mcp.go` | 🟡 `MCPToolAdapter` wraps `mcpclient.Tool`; `InvokableRun` returns "not yet implemented" until `mcpclient.CallTools` lands |
| pubmed | `internal/agent/tool/pubmed.go` | ✅ |
| qweather | `internal/agent/tool/qweather.go` | ✅ |
| retrieval | `internal/agent/tool/retrieval.go` | ✅ Adapter + boot wiring (`cmd/server_main.go`) |
| search_my_dataset | `internal/agent/tool/registry.go` | ✅ Alias of `retrieval` |
| search_my_dateset | `internal/agent/tool/registry.go` | ✅ Python-typo alias of `retrieval` |
| searxng | `internal/agent/tool/searxng.go` | ✅ |
| tavily | `internal/agent/tool/tavily.go` | ✅ |
| tushare | `internal/agent/tool/tushare.go` | ✅ |
| wencai | `internal/agent/tool/wencai.go` | ✅ |
| wikipedia | `internal/agent/tool/wikipedia.go` | ✅ |
| yahoo_finance | `internal/agent/tool/yahoo_finance.go` | ✅ |

**Total**: 49 named entities (24 components + 25 tools).

---

## 5. DSL 单一形态

RAGFlow agent DSL 现在只有**一种** wire 形态（之前 v1/v2 双轨已删）：

```jsonc
{
  "globals":    {...},                                                  // sys.query / sys.user_id / ...
  "graph":      { "nodes": [...], "edges": [...] },                     // React-Flow 布局
  "variables":  {...},                                                  // 用户级变量
  "components": { "<Name>:<UUID>": {                                    // 执行拓扑
    "downstream": [...], "upstream": [...],
    "obj": { "component_name": "Name", "params": {...} }
  }},
  "path": [...], "retrieval": {...}, "history": [...]                    // 运行时状态
}
```

**单一 wire 的硬性保证**：

1. **后端 GET/PUT 收到的 DSL 必定同时含 `graph` + `components`**。前端 `use-build-dsl.ts` 在 PUT 时一并填充两个块，back-end 不依赖 `graph`。
2. **Go 端的唯一入口是 `dsl.NormalizeForCanvas`**（`internal/handler/agent.go:226`、`internal/service/agent.go:217,273`）。所有 Python ↔ Go 路径的 dsl 都在解码边界过一次。
3. **`internal/agent/dsl/` 包当前仅 `normalize.go` + `normalize_test.go` + `testdata/`**（v1↔v2 转换器与 `v2.go`/`loader.go`/`converter_v1_to_v2.go`/`converter_v2_to_v1.go` 已 `git rm`）。

### 5.1 NormalizeForCanvas：解码边界的三步流水线

`internal/agent/dsl/normalize.go` 的 `NormalizeForCanvas(dsl map[string]any) map[string]any`：

1. **`enforceHandleIds(dsl)`** — 把 `graph.edges[*].sourceHandle` / `targetHandle` 规约为 React-Flow 约定。
2. **`buildGraphFromComponents(components)`** — 若 `graph.nodes` 缺失，从 `components` 派生默认布局。
3. **`foldLegacyLoopVariants(dsl)`** — 把 `Loop+LoopItem` / `Iteration+IterationItem` 折叠成单个 `Loop` / `Parallel` 节点。

### 5.2 Loop / Iteration 折叠语义

- **Python 端保留** `Loop+LoopItem` / `Iteration+IterationItem` 旧类名（stable server，本次不动）。
- **Go 端** `Loop` 已经是单节点（`internal/agent/component/loop.go`），`Parallel` 已经是单节点。`Iteration` / `IterationItem` 仅作为 alias 留在 `internal/agent/component/fixture_stubs.go`，stub 体内**委托给 Parallel factory**。
- **前端** `Operator` 枚举里 `Iteration` / `IterationStart` / `LoopStart` 保留。

### 5.3 Compile 入口的兼容兜底

`canvas.Compile(ctx, c *Canvas, opts...)` 接收的 `*Canvas` **预期已经过 `NormalizeForCanvas`**。如果某条路径直接 unmarshal dsl 后丢给 Compile 而没走 decoder，`Compile` 入口会 `log.Printf` 一行 stderr warning。

### 5.4 7 个 testdata 顶层结构

`internal/agent/dsl/testdata/{all, browser, dfx_picture_parser, questions_category, resume, subaget, switch}.json` 顶层都是 `{globals, graph, variables}`（`graph.nodes` / `graph.edges` 完整）。**没有 `components` 顶层 key**。这是 import / export 文件的形态。

### 5.5 前端 dsl-bridge：单一 import 路径

`web/src/pages/agent/utils/dsl-bridge.ts` 重写为单一模式：

- 删除 `DSL_MODE` / `DslMode` / `if (DSL_MODE === 'v1')` / `if (DSL_MODE === 'v2')` 编译期分支
- `importDsl(rawParsed, isAgent)` 单一优先级：`raw.graph.nodes` 非空 → 用之；否则 fallback 到 empty seed
- `dslToGraph(dsl)` 同样只读 `dsl.graph.nodes`

---

## 6. workflowx 扩展 / workflowx Extensions

`internal/agent/workflowx/` 提供**零侵入 eino 扩展**——不修改 eino 源码，只提供外部 helper。

### 6.1 AddLoopNode[T] — 通用循环节点

**API**：
```go
func AddLoopNode[T any](
    ctx context.Context,
    wf *compose.Workflow[T, T],
    key string,
    sub *compose.Workflow[T, T],
    shouldQuit LoopCondition[T],
    opts ...LoopOption,
) (*compose.WorkflowNode, error)
```

**执行模型**（do-while 语义）：

1. 接收 `current`
2. 跑一次 sub-workflow 拿 `next`
3. `shouldQuit(ctx, iteration, current, next)` — `iteration` 从 1 开始
4. 满足 quit → 返回 `next`；否则 `current = next` 继续
5. 必须至少执行一次

**实现要点**：

- `compose.AnyLambda[T, T, struct{}](...)` 包裹 invoke + stream 双路径
- `WithLoopMaxIterations(n)` 强建议（防意外死循环）
- `WithLoopStream(mode)` — `LoopStreamFinalOnly` (默认) / `LoopStreamEveryIteration`
- 错误处理：`ErrLoopMaxIterationsExceeded` / `ErrLoopSubGraphInterrupted` / `ErrLoopResumeStateInvalid` / `ErrLoopQuitConditionFailed`
- 嵌套子 workflow 走 `compose.Runnable[T,T]` + sub-checkpoint 通过 loop-owned bridge store

**Checkpoint/Resume 合约**：

- Invoke path 嵌套 interrupt → 通过 `compose.CompositeInterrupt` 向上传播；resume 从中断的 iteration 继续（不重头）
- Stream path 走 **iteration-granular** 恢复合约：已完整发到下游的 iteration 不重放
- 稳定 child checkpoint ID 通过 `WithLoopCheckpointIDBuilder(nodeKey, iteration)`；默认 `workflowx-loop:<nodeKey>:<iteration>` 命名空间

**Loop 在 canvas 中的应用**：

- `Loop` 在 Go 端是**单节点**：registry 注册 + 工厂，但 `LoopComponent.Invoke` 是 no-op
- `BuildWorkflow` 看到名为 `Loop` 的 cpn 时：调用 `expandLoopSubgraph` 收集下游、构建 sub-`compose.Workflow[map[string]any, map[string]any]`、调 `workflowx.AddLoopNode` 把结果作为单节点插入外图
- `LoopItem` / `ExitLoop` **已删除**（v1 compat 通过 `legacyNoOpNames` 在 canvas 层吸收）

### 6.2 AddParallelNode[I, O] — 通用并发节点

**API**：
```go
func AddParallelNode[I, O any](
    ctx context.Context,
    wf *compose.Workflow[[]I, []O],
    key string,
    sub Compilable[I, O],
    opts ...ParallelOption,
) (*compose.WorkflowNode, error)
```

**实现要点**：

- 外层 invoke-only；内层 sub workflow 可 stream-capable
- `WithParallelMaxConcurrency(n int)`：0 / 1 = 顺序执行；> 1 = 信号量并发
- **顺序保持不变量**：`outputs[i]` 永远对应 `inputs[i]`
- 错误处理：`ErrParallelCompileFailed` / `ErrParallelResumeStateInvalid`；per-item 错误用 `fmt.Errorf("item %d: %w", idx, err)` 包装
- 嵌套 interrupt：累积到 `compose.CompositeInterrupt(ctx, nil, state, interruptErrs...)`
- 恢复不变量：`CompletedResults ∪ InterruptedIndices = 0..TotalCount-1`（partition 完整）

**Parallel 在 canvas 中的应用**：

- `Parallel` component 走 T4 薄包装：注册时传 `agenttool.BuildByName("parallel", params)`（实际是 `internal/agent/component/parallel.go` 的 `ParallelComponent`），内部用 `workflowx.AddParallelNode` 把 sub-workflow 插入外图

### 6.3 Canvas parallel batch (eino intrinsic, NOT workflowx parallel)

**关键发现**：Phase 4.1 "Canvas parallel batch execution" 不需要额外实现 — **eino `compose.Workflow.Run` 本身就在每个 topological wave 内 spawn 一个 `go t.execute()` per ready node**。

- `canvas/parallel_batch_test.go::TestBuildWorkflow_ParallelBatchStructure` pin 4-node sibling compile
- `canvas/parallel_timing_test.go::TestCanvas_ParallelExecution_StaticAnalysis` pin 5-node DAG compile 静态分析

`workflowx/parallel.go` 仍存在，但仅用于 `Parallel` component (Loop/Iteration 风格的 array parallel)，**不是** canvas 层的 ready-node 调度。

---

## 7. Checkpoint + Run Tracker / Persistence

### 7.1 双 key 设计

**Key 1：`agent:cp:{check_point_id}`** — eino payload 存储

- 类型：String (直接存 `[]byte`，**不走 JSON** — eino Serializer 已负责序列化)
- TTL：30 天，Set 时 `EXPIRE 30*24*3600` 一次设置
- eino `CheckPointStore` 是**纯 KV 接口** — `Get(ctx, id) ([]byte, bool, error)` / `Set(ctx, id, []byte) error`
- eino **不会**自动写入 status / canvas_id / tenant_id / run_id / parent_id / expires_at 等业务字段

**Key 2：`agent:run:{run_id}`** — 业务元数据存储 (Redis Hash)

| 字段 | 类型 | 含义 |
|------|------|------|
| `canvas_id` | string | `user_canvas.id` |
| `tenant_id` | string | 从 user-tenant lookup |
| `checkpoint_id` | string | 当前 run 的最新 checkpoint (指向 key 1) |
| `parent_run_id` | string | resume_from 源 run (续跑链)，可空 |
| `status` | int (0/1/2/3) | 0=running 1=succeeded 2=failed 3=cancelled |
| `failure_reason` | string | 失败原因 (err.Error()) |
| `cancel_requested` | int (0/1) | 1=用户/admin 已请求 cancel |
| `started_at` | int (epoch ms) | |
| `finished_at` | int (epoch ms) | 退出时填写 |

- TTL：30 天 (与 key 1 同步)
- `RunTracker.Start/MarkSucceeded/MarkFailed/MarkCancelled/AttachCheckpoint` 显式调用
- **不依赖 eino 自动写** — cancel/fail 后的 `status=failed` 由应用层自己写

### 7.2 4 个 eino payload 写入触发 (写 `agent:cp:*`)

| # | 触发点 | eino 源码 | 用途 |
|---|--------|-----------|------|
| **W1** | 节点显式 `compose.Interrupt(ctx, info)` / `StatefulInterrupt(ctx, info, state)` | `compose/interrupt.go` | human-in-the-loop、外部 API 回调、限流暂停 |
| **W2** | `compose.WithInterruptBeforeNodes([]string)` / `WithInterruptAfterNodes([]string)` 编译期拦截点 | `compose/interrupt.go` | 命中后**写盘 + 终止 run** (与 W1 共用 `handleInterrupt` 路径)；**默认开 0 个** |
| **W3** | 子 graph interrupt 向上传播 | `subGraphInterruptError` | 嵌套 subgraph / ToolsNode / agentic 抛 interrupt 时，父 graph 同步落盘 |
| **W4** | 运行退出 | `WithCheckPointID` + `WithWriteToCheckPointID` | run 退出时最后一次落盘 |

### 7.3 4 个业务元数据写入 + 1 个恢复触发

| # | 触发点 | 写入函数 |
|---|--------|---------|
| **B1** | Canvas run 启动 | `RunTracker.Start(runID, canvasID, tenantID, parentRunID)` |
| **B2** | Run 正常完成 | `RunTracker.MarkSucceeded(runID)` |
| **B3** | Run 失败 | `RunTracker.MarkFailed(runID, err.Error())` |
| **B4** | Run 被 cancel | `RunTracker.MarkCancelled(runID)` |
| **B5** | Compile 成功后 | `RunTracker.AttachCheckpoint(runID, cpID)` |
| **R1** | HTTP `POST /run?resume_from=run_xxx` | handler: `HGetAll("agent:run:run_xxx")` → `checkpoint_id` → `WithCheckPointID(cpID)` + `WithWriteToCheckPointID(newCP)` + `RunTracker.Start(newRunID, canvas, tenant, "run_xxx")` |

### 7.4 CheckPointStore / StateSerializer 接口设计

**`internal/agent/canvas/checkpoint_store.go`**：

```go
type CheckPointStore interface {
    Get(ctx context.Context, id string) ([]byte, bool, error)
    Set(ctx context.Context, id string, data []byte) error
    Delete(ctx context.Context, id string) error  // 自定义扩展, eino compose.CheckPointStore 无此方法
}
```

**`internal/agent/canvas/state_serializer.go`**：

```go
type StateSerializer interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}

// CanvasStateSerializer — encoding/json
type CanvasStateSerializer struct{}
func (CanvasStateSerializer) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (CanvasStateSerializer) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
```

**`internal/agent/canvas/compile.go`** — 关键修正：

```go
// 注意: 不能用 compose.WithSerializer 覆盖 eino 的 InternalSerializer!
// eino 的 compose.Serializer 同时控制 (a) 用户提供的 state 序列化 AND (b) eino 内部
// graph state 序列化。覆盖会破坏 eino graph 内部 marshal/unmarshal 逻辑。
//
// 正确做法: 仅 wire WithCheckPointStore (custom KV 接口), 让 eino 内部
// InternalSerializer 保留默认值。同时 CanvasState 自带 MarshalJSON 让
// eino StatePre/PostHandler 能序列化 state。
func Compile(ctx context.Context, c *Canvas, opts ...CompileOption) (*CompiledCanvas, error) {
    cfg := CompileOptions{}
    for _, o := range opts { o(&cfg) }
    
    compileOpts := []compose.GraphCompileOption{
        compose.WithCheckPointStore(checkPointAdapter{cfg.Store}),  // 适配 Delete
    }
    // 显式 NOT 调用 compose.WithSerializer
    return wf.Compile(ctx, compileOpts...)
}

// checkPointAdapter drops the Delete method that compose.CheckPointStore does not declare.
type checkPointAdapter struct{ inner CheckPointStore }
func (a checkPointAdapter) Get(ctx context.Context, id string) ([]byte, bool, error) {
    return a.inner.Get(ctx, id)
}
func (a checkPointAdapter) Set(ctx context.Context, id string, data []byte) error {
    return a.inner.Set(ctx, id, data)
}
```

**CompiledCanvas struct**：

```go
type CompiledCanvas struct {
    Workflow     compose.Runnable
    CheckPointID string  // 暂时空字符串; V2.1 从 eino Runnable 表面化
}
```

### 7.5 Cancel 协议 (两段式)

```go
// internal/agent/canvas/cancel.go
func Run(ctx context.Context, taskID string, compiled compose.Runnable[...]) error {
    einoCtx, interrupt := compose.WithGraphInterrupt(ctx)
    defer close(stopCh)

    go watchCancel(taskID, func() {
        interrupt(compose.WithGraphInterruptTimeout(30 * time.Second))
    })

    return compiled.Invoke(einoCtx, input,
        compose.WithCheckPointID(genID(taskID)),
        compose.WithWriteToCheckPointID(genID(taskID)),
    )
}

func watchCancel(taskID string, onCancel func()) {
    ticker := time.NewTicker(500 * time.Millisecond)  // 500ms 轮询
    defer ticker.Stop()
    for {
        select {
        case <-stopCh: return
        case <-ticker.C:
            v, _ := redis.Get(context.Background(), fmt.Sprintf("%s-cancel", taskID))
            if v != "" { onCancel(); return }
        }
    }
}
```

**Python 兼容**：`{task_id}-cancel` Redis key 命名与 Python 端 task_service.py 协议**完全一致**。

---

## 8. OpenTelemetry 可观测性 / Observability

### 8.1 总体设计

```
Canvas run goroutine (Go)
   ↓
eino Graph Engine
   ↓ (OnStart / OnEnd / OnError auto-injected)
callbacks.Handler (业务实现)
   ├─ OTelHandler
   │   └─ 开始 span → 注入 attributes → 结束 span
   │       └─ otlphttpexporter → OTel Collector (外部)
   │           ├─ Jaeger / Tempo (trace UI)
   │           ├─ Langfuse (LLM 专门)
   │           └─ Prometheus / Grafana
   └─ SSEHandler (业务事件流) → admin UI
```

### 8.2 双通道分离

| 通道 | 用途 | 协议 | 消费者 |
|------|------|------|--------|
| **SSE** | 业务事件（"node 开始/结束/消息"） | `text/event-stream` HTTP | admin UI |
| **OTel span** | 系统可观测性（节点耗时/错误/token） | OTLP HTTP | 运维/APM |

### 8.3 eino callback → OTel 映射

| eino 时机 | OTel 行为 | Span attribute |
|-----------|-----------|----------------|
| `OnStart(ctx, info, input)` | `tracer.Start(ctx, info.Name)` → 写入 `ctx` | `eino.component.name`, `eino.component.type`, `eino.input.size` |
| `OnEnd(ctx, info, output)` | `span.End()` | `eino.output.size` |
| `OnError(ctx, info, err)` | `span.RecordError(err)` + `span.SetStatus(codes.Error, ...)` | `eino.error.message` |

### 8.4 启动配置

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318"
export OTEL_SERVICE_NAME="ragflow-agent"
export OTEL_RESOURCE_ATTRIBUTES="service.namespace=ragflow,deployment.environment=production"
export OTEL_TRACES_SAMPLER="parentbased_traceidratio"
export OTEL_TRACES_SAMPLER_ARG="0.1"  # 10% 采样
```

**降级**：未配置 `OTEL_EXPORTER_OTLP_ENDPOINT` → handler 退化为 noop，不影响业务。

---

## 9. 多版本 Agent 管理 / Multi-version Agents

**Go 端支持多版本并存**（**永不覆盖**）：

| 场景 | 行为 |
|------|------|
| 编辑器保存草稿 | `UPDATE user_canvas SET dsl=? WHERE id=?` (**不创建 version**) |
| 点击"发布" | `INSERT user_canvas_version(...)` 新行；`UPDATE user_canvas SET release=true, dsl=?, update_at=NOW()` |
| Run 不带 version | 拉取**最新** `user_canvas_version` (`create_time DESC LIMIT 1`) |
| Run `?version=v_xxx` | 拉取**指定** `user_canvas_version` |
| Run `?version=draft` | 拉取 `user_canvas.dsl` (编辑器未发布状态) |

**API 端**：

- `GET /api/v1/agents/{id}/versions` — 列表
- `POST /api/v1/agents/{id}/versions` — 显式发布
- `DELETE /api/v1/agents/{id}/versions/{version_id}` — 删除
- `POST /api/v1/agents/{id}/run?version=xxx` — 指定版本运行

---

## 10. 第三方库选型 / Third-party Libraries (License Gate)

### 10.1 决策结论

| 用途 | 选 | License | 备注 |
|------|-----|---------|------|
| **PDF 生成** | `signintech/gopdf` | MIT | 主选；TTF 字体注册 + CJK + header/footer 内置 |
| **DOCX 生成** | **自实现** OOXML writer | — | Go `archive/zip` stdlib + `text/template` + `//go:embed` |
| **Excel 读写** | `xuri/excelize/v2` | BSD-3 | 无 license 风险 |
| **Markdown 解析** | `yuin/goldmark` | MIT | CommonMark 标准 |
| **HTML 解析** | `golang.org/x/net/html` | BSD-3 | stdlib 旁路 |
| **OpenTelemetry SDK** | `go.opentelemetry.io/otel` v1.44.0 | Apache-2.0 | |
| **MySQL driver** | `go-sql-driver/mysql` | MPL-2.0 | ExeSQL 走 stdlib `database/sql` |
| **PG driver** | `lib/pq` | MIT | |
| **MSSQL driver** | `denisenkom/go-mssqldb` | BSD-3 | |
| **Trino driver** | `trinodb/trino-go-client v0.333.0` | Apache-2.0 | ExeSQL Trino dialect |
| **Jinja2 模板** | `nikolalohinski/gonja v1.5.3` | MIT | Phase 8a — 直接 import (from indirect) |
| **Test SQL mock** | `DATA-DOG/go-sqlmock` | MIT | ExeSQL 注入测试 |

### 10.2 AGPL-3 零容忍

RAGFlow 是 Apache-2.0；AGPL-3 强传染会让整个 RAGFlow Go 二进制被迫 AGPL-3 化。所有候选 AGPL-3 库 (unipdf / unioffice / fumiama-go-docx / baliance-gooxml) **全部排除**。

**AGPL-3 预筛规则**：
- README header 含 "AGPL" 或 "Affero" → 直接拒绝
- LICENSE 文件首行含 "Affero General Public License" → 拒绝
- GitHub license badge 显示 AGPL-3.0 / SSPL-1.0 → 拒绝
- CI 中 `go-licenses check` 命中 AGPL → 构建失败

---

## 11. HTTP 接口 / HTTP API

| Method | Path | Handler | 说明 |
|--------|------|---------|------|
| `GET`  | `/api/v1/agents` | `ListAgents` | 已存在 |
| `POST` | `/api/v1/agents` | `CreateAgent` | |
| `GET`  | `/api/v1/agents/{id}` | `GetAgent` | |
| `PATCH`| `/api/v1/agents/{id}` | `UpdateAgent` | |
| `DELETE`| `/api/v1/agents/{id}` | `DeleteAgent` | 级联删除所有 version |
| `POST` | `/api/v1/agents/{id}/run` | `RunAgent` | 同步; `?version=v_xxx` 缺省=最新 |
| `POST` | `/api/v1/agents/{id}/stream` | `StreamAgent` | SSE; emits `message` / `waiting_for_user` / `error` / `done` events |
| `POST` | `/api/v1/agents/{id}/cancel` | `CancelAgent` | 写 Redis cancel key |
| `GET`  | `/api/v1/agents/{id}/versions` | `ListVersions` | |
| `POST` | `/api/v1/agents/{id}/versions` | `PublishVersion` | |
| `GET`  | `/api/v1/agents/{id}/versions/{vid}` | `GetVersion` | |
| `DELETE`| `/api/v1/agents/{id}/versions/{vid}` | `DeleteVersion` | |
| `POST` | `/api/v1/admin/canvas-runtime/:tenant_id` | `AdminRuntime` | 翻转租户 override |

**SSE 事件 payload**：

```text
event: message
data: {"answer": "...", "reference": [...]}

event: waiting_for_user
data: {"cpn_id": "node:userfillup_1"}

event: error
data: {"error": "..."}

event: done
data: [DONE]
```

---

## 12. 验收标准 / Acceptance Criteria

| 类别 | 标准 |
|------|------|
| **功能** | 19 component × ≥3 单测 = ≥57 个 component 单测 |
| **功能** | 21 tool × ≥2 单测 = ≥42 个 tool 单测 |
| **eino 复用** | T1 组件 (LLM/Agent) 回归：跑 eino 自带 react_test.go / chatmodel_test.go / compose_test.go 不退化 |
| **功能** | `{{cpn_id@param}}` 任意节点读任意节点, 单测覆盖 |
| **功能** | SSE 事件序列与 Python `agent_api.py` 一致: `message` / `waiting_for_user` / `error` / `done` |
| **wait-for-user** | Canvas 含 UserFillUp 节点 → 首次运行到 UserFillUp 暂停 → SSE `waiting_for_user` → 用户提交后恢复运行 → 最终输出 `message` + `done` 事件 |
| **RunAgent e2e** | 4 e2e sub-tests: `TestRunAgent_RealCanvas_BeginMessage` / `_CompileFails` / `_InvokeFails` / `_WaitForUserResume` |
| **RunTracker** | miniredis-backed e2e pinning Start → AttachCheckpoint → MarkSucceeded sequence |
| **TTS dispatch** | model-provider integration wired (`audio.NewTTSDispatchFunc`) |
| **per-class timeout** | ExeSQL→3s, TavilySearch→12s, uniform fallback, env override |
| **LLM retry** | MaxRetries=5 → exactly 6 invoker calls (absolute count) |
| **可靠** | Redis 取消协议：cancel → 5s 内节点 stop (500ms 轮询下 p99 ≤ 500ms) |
| **可观测性** | OTel handler P99 overhead < 2% (100 节点) |
| **checkpoint** | Redis `RedisCheckPointStore` Get/Set/Delete 通过 eino 集成测试 |
| **代码质量** | 公共 API 100% godoc 注释；`>=80% test coverage on internal/agent/canvas` |

---

## 13. 风险 & 缓解 / Risks

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| **eino State 在高并发下 mutex 竞争** | 中 | Phase 1 末 benchmark；若 > 5% 调度开销，引入分片 mutex |
| **v1 DSL 100% 兼容不可能** | 中 | 不兼容的旧 DSL 走"自动转换 + 提示"路径 |
| **Tool 外部 HTTP 失败** | 中 | 复用 `http_helper.go` 的 retry |
| **前端 DSL 编辑器只懂 v1** | 中 | Phase 5 维持 v1 写出能力 |
| **测试环境无 LLM key** | 低 | 所有 LLM 组件测试走 mock provider driver |
| **LLM retry multiplicative stacking** | 中 | `retryInvoker.Unwrap()` + `unwrapChatInvoker` 让 MaxRetries = absolute count |
| **CodeExec feature gap vs Python** | 中 | 5 sandbox providers 已 ported；`docs/develop/sandbox-python-go-diff.md` 详细记录 per-provider diff |
| **real TTS binary shape TBD** | 中 | model-provider 60+ driver 路由；real binary 由 model provider 决定 |
| **real MemorySaver 端口 partial** | 低 | partial port；user-deferred |

---

## 14. Future Work

可操作的下一轮跟进项 (按优先级)：

1. **Compile LRU cache** — LRU 按 `(canvasID, versionID, DSL-hash)` 缓存编译产物；仅在 profiling 显示 `Compile` 主导热路径时启动。1-2 周。
2. **Browser Playwright parity** — Python `browser.py` 29.4K vs Go 8.9K，差 3.3×。需要 scope 决策：完整 Playwright 移植 vs 缩减到核心场景。1 周。
3. **ExcelProcessor pandas-fidelity audit** — Python 端 15.5K vs Go 当前 happy-path 覆盖。1 天 audit + 修补。
4. **Phase 8b real MemorySaver completion** — 端口 `internal/service/memory_message_service.go` 完整实现。1-2 周，user-deferred。
5. **Phase 5c DB2 support** — CGO + `github.com/ibmdb/go_ibmdb` + native client lib。仅在 e2e 需求浮现时启动。0.5-1 周。
6. **Phase 5d CodeExec 完整对等** — 5 sandbox providers + artifacts/args/timeout/per-language base image 已 ported；file output collection paths, GraphRAG adapter 仍剩余。1-2 周。
7. **Phase 6 gray + Phase 7 cleanup** — per-tenant runtime 灰度切换；`agent_api.py` 标 `@deprecated` + 兼容 proxy shim。2-4 周。
8. **DSL v3** — 类型化表达式 (编译期校验 `{{cpn_id@param}}`)。
9. **eino 生态对齐** — `AddAgenticModelNode` 替换 LLM component; `AddRetrieverNode` 替换 Retrieval component。
10. **GraphRAG component Go 化** (独立项目排期)。
11. **WebSocket 流支持** (pending demand)。
12. **Checkpoint 增强** — 跨 canvas run 复用、增量 checkpoint (仅写 diff channel)。

### Sandbox provider gaps (consolidated from the port diff)

The five Python sandbox providers are ported to Go with functional parity (self_managed, aliyun, local, ssh) and one strict superset (e2b — Go is real, Python is a stub). Admin-panel settings reader lands in `ProviderManager.LoadFromSettings` (see commit history). The remaining 7 gaps are intentional and tracked here:

- **Aliyun Go SDK gaps (v1.1.0)** — ⏸️ **blocked on upstream aliyun SDK.** Two related gaps to revisit when the SDK catches up: (1) `TemplateName` not sent on `CreateCodeInterpreter` (operators must pre-create non-default templates via Python or the aliyun console, then reference by name in metadata); (2) execute uses raw HTTP because the SDK has no execute method (the wire format was reverse-engineered from the Python SDK). Swap to the SDK calls when both APIs land. (1-2 days once the SDK releases; no in-house workaround)
- **`LocalProvider` rlimits not applied** — Go `os/exec` has no portable pre-start hook; rlimits (RLIMIT_AS/CPU/FSIZE/NOFILE) are not enforced. The Go `LocalProvider` is **not a security boundary** — for adversarial code, use `SelfManagedProvider` (executor_manager + gVisor) or `AliyunCodeInterpreterProvider` (cloud microVM). This matches the Python note that "local" is "for development / trusted environments". (no fix planned — by design)
- **`SSHProvider` uses SSH exec, not SFTP** — avoids the `github.com/pkg/sftp` dependency. For workloads with many large artifacts, switch to pkg/sftp if profiling shows exec overhead. (1 day, deferred until profiling shows it matters)
- **Windows build of `LocalProvider`** — `syscall.Setpgid` is POSIX-only. The Go side is `//go:build !windows`; the Python side runs on Windows via `process.kill()`. Tracked; not blocking because RAGFlow production is Linux. (1-2 days, deferred)
- **e2b community Go SDK is a single-maintainer port** — `github.com/eric642/e2b-go-sdk` v0.1.3 (Apache-2.0). Re-evaluate quarterly; fork to `github.com/infiniflow/e2b-go-sdk` if maintenance lags. (1 day fork if needed)
- **OTel spans on provider ops** — providers are log-free; OTel span propagation is on the HTTP client only (via `otelhttp.NewTransport`). Providers themselves do not emit OTel spans. (1 day)

---

## 15. Operations Guide

### 15.1 Boot wiring

`cmd/server_main.go` registers the runtime in three layers:

1. **ProviderManager** (`internal/agent/sandbox/manager.go`) — chooses which sandbox provider backs CodeExec. Default `self_managed`; override via `SANDBOX_PROVIDER_TYPE`. Falls back to env-driven init when the admin-panel settings table is empty/malformed.
2. **RetrievalService** (`internal/agent/tool/retrieval_service.go`) — `nlp.NewRetrievalService(docEngine, docDAO)` and `kg.NewRetrieval(...)` are wired via `tool.SetRetrievalService(...)` / `tool.SetKGRetrievalService(...)` at boot. The first backs `use_kg=false`; the second backs `use_kg=true`.
3. **AgentService** (`internal/service/agent.go`) — accepts optional Redis-backed CheckPointStore / StateSerializer / RunTracker via `NewAgentServiceWithOptions(...)`. Boot installs these when Redis is up; otherwise the fields stay nil and the service falls back to in-memory mode (transparent to callers).

Any layer that is not wired at boot produces a loud-fail sentinel (see §15.3) — stubs never silently return empty results.

### 15.2 Feature flags

| Env var | Default | Effect |
|---------|---------|--------|
| `SANDBOX_PROVIDER_TYPE` | `self_managed` | One of `self_managed` / `aliyun_codeinterpreter` / `e2b` / `local` / `ssh` |
| `SANDBOX_EXECUTOR_MANAGER_URL` | `http://sandbox-executor-manager:9385` | self-managed endpoint |
| `SANDBOX_EXECUTOR_MANAGER_TIMEOUT` | `30` (s) | self-managed per-call timeout |
| `AGENTRUN_*` (5 vars) | n/a | aliyun code interpreter |
| `E2B_API_KEY` / `E2B_ACCESS_TOKEN` | n/a | e2b (one required) |
| `E2B_TEMPLATE` | `base` | e2b sandbox template |
| `LOCAL_*` (8 vars) | n/a | local subprocess |
| `SSH_HOST` / `SSH_PORT` / `SSH_USERNAME` / `SSH_PASSWORD` / `SSH_PRIVATE_KEY` / `SSH_PRIVATE_KEY_PATH` | n/a | SSH provider |
| `COMPONENT_EXEC_TIMEOUT` | `600` (s) | canvas-level per-invocation timeout; per-class overrides via env-derived map (see `canvas/timeout.go`) |

### 15.3 Known deferred items (loud-fail sentinels)

| Sentinel | Cause | Fix |
|----------|-------|-----|
| `ErrRetrievalServiceMissing` | `tool.SetRetrievalService(...)` not called at boot | Wire `nlp.NewRetrievalService` at boot (default in `cmd/server_main.go`) |
| `ErrKGRetrievalServiceMissing` | Canvas uses `use_kg=true` and `tool.SetKGRetrievalService(...)` not called | Wire `kg.NewRetrieval(...)` at boot (default in `cmd/server_main.go`) |
| `ErrMemoryServiceMissing` | `component.SetMemorySaver(...)` not called at boot | Wire `NewMemoryMessageService(...)` (default in `cmd/server_main.go`) |
| `ErrEmbedderNotWired` | MemorySaver reached but no embedder configured | Port the embedding model — see §14 |
| `ErrSandboxNotConfigured` | `SANDBOX_PROVIDER_TYPE` set to unknown value | Set to one of the 5 supported values |
| `ErrE2BProviderNotImplemented` | `SANDBOX_PROVIDER_TYPE=e2b` and no `E2B_API_KEY`/`E2B_ACCESS_TOKEN` | Provide one of the two env vars |
| `ErrTTSEngineNotConfigured` | Message runs with `auto_play=true` and no `audio.SetSynthesizer(...)` | Wire a TTS engine at boot — see §14 |
| `ErrExeSQLUnsupportedDB` | `db_type` is `trino` or `ibm db2` | Add the driver registration — see §14 |

### 15.4 Canvas migration (Python → Go)

`tools/migrate-canvas` cross-validates Python's `normalize_chunker_dsl` against Go's `NormalizeForCanvas`. Manual equivalent until the tool ships:

1. Export canvas JSON from Python: `GET /api/v1/canvas/<id>/export`.
2. Validate Python normalizer: `uv run python -c "from agent.canvas import normalize_chunker_dsl; print(normalize_chunker_dsl(json.load(open('canvas.json'))))"`.
3. Validate Go normalizer: `go test ./internal/agent/dsl/ -run TestNormalize -v` (uses fixtures in `internal/agent/dsl/testdata/`).
4. Diff the two normalized forms. If structurally identical, the canvas is Go-portable.

### 15.5 Testing

```sh
go test -count=1 ./internal/agent/...           # all agent tests
go test -count=1 ./internal/agent/component/   # component tests
go test -count=1 ./internal/agent/tool/         # tool tests + retrieval + sandbox providers
go test -count=1 ./internal/agent/sandbox/     # 5 sandbox providers + manager
go test -count=1 ./internal/agent/canvas/      # canvas engine, parallel, interrupt/resume
go test -count=1 ./internal/agent/runtime/     # state, template, history window
```

Fixtures: `internal/agent/dsl/testdata/` (7 JSONs) drive the e2e suite and match the input corpus Python's `normalize_chunker_dsl` accepts.

---

## 附录 A · 关键文件 / Key Files

| 设计点 | 关键文件 |
|--------|---------|
| **State 模式** | `internal/agent/canvas/{state.go, scheduler.go}` + `internal/agent/runtime/{state.go, context.go, template.go, template_jinja.go}` |
| **CanvasState MarshalJSON** | `internal/agent/runtime/state.go` |
| **runtime 提取** | `internal/agent/runtime/*.go` (8 文件) + `internal/agent/canvas/state_export.go` |
| **Loop 宏展开** | `internal/agent/canvas/loop_subgraph.go` + `internal/agent/component/loop.go` (no-op marker) |
| **Parallel** | `internal/agent/component/parallel.go` + `internal/agent/workflowx/parallel.go` |
| **Loop 通用节点** | `internal/agent/workflowx/loop.go` + `loop_*_test.go` |
| **Interrupt 路径** | `internal/agent/canvas/interrupt_resume.go` + `internal/agent/canvas/runner.go` |
| **Checkpoint** | `internal/agent/canvas/{checkpoint_store.go, run_tracker.go, state_serializer.go, compile.go}` |
| **Compile 适配** | `internal/agent/canvas/compile.go` (checkPointAdapter) |
| **Per-class timeout** | `internal/agent/canvas/timeout.go` + `node_body.go` |
| **Cancel 协议** | `internal/agent/canvas/cancel.go` |
| **OTel** | `internal/observability/otel/{provider.go, handler.go, handler_test.go}` |
| **DSL normalize** | `internal/agent/dsl/{normalize.go, normalize_test.go}` + `testdata/` |
| **Tool registry** | `internal/agent/tool/{registry.go, http_helper.go, ssrf.go, mcp.go, retrieval*.go}` |
| **Component 5-tier** | `internal/agent/component/{base.go, registry.go, runtime_wire.go, fixture_stubs.go, universe_a_wrappers.go}` + 19 component .go |
| **AgentService V2** | `internal/service/agent.go` (buildRunFunc) + `internal/service/canvas_decode.go` + `internal/service/agent_run_e2e_test.go` |
| **Sandbox providers** | `internal/agent/sandbox/{self_managed.go, aliyun.go, e2b.go, local.go, ssh.go, manager.go}` + `tool/sandbox_bridge.go` |
| **TTS dispatch** | `internal/agent/audio/{tts.go, tts_dispatch.go, model_provider_synthesizer.go}` |

## 附录 B · 测试覆盖 / Test Coverage

| 包 | 测试文件数 | 覆盖点 |
|----|-----------|--------|
| `internal/agent/canvas` | 17 | `canvas_test.go, scheduler_test.go, state_test.go, variable_test.go, state_bench_test.go, state_serializer_test.go, checkpoint_store_test.go, run_tracker_test.go, cancel_test.go, stream_test.go, loop_subgraph_test.go, loop_semantics_test.go, dsl_examples_e2e_test.go, interrupt_resume_test.go, multibranch_test.go, node_body_timeout_test.go, node_body_per_class_timeout_integration_test.go, parallel_batch_test.go, parallel_timing_test.go` |
| `internal/agent/component` | 50+ | 各 component `_test.go` + `verify_p1_test.go` + `production_chain_fixes_test.go` |
| `internal/agent/tool` | 30+ | 各 tool `_test.go` + `registry_test.go` + `retrieval_nlp_test.go` + `retrieval_kg_test.go` + `exesql_trino_test.go` + `exesql_unsupported_test.go` + `http_helper_test.go` + `ssrf_test.go` + `mcp_test.go` |
| `internal/agent/runtime` | 4 | `metrics_test.go, selector_test.go, state_test.go, template_jinja_test.go` |
| `internal/agent/workflowx` | 8 | `loop_test.go, loop_options_test.go, loop_integration_test.go, loop_example_test.go, parallel_test.go, parallel_options_test.go, parallel_integration_test.go, parallel_helpers_test.go` |
| `internal/agent/dsl` | 1 | `normalize_test.go` |
| `internal/agent/audio` | 3 | `model_provider_synthesizer_test.go, tts_dispatch_test.go, tts_test.go` |
| `internal/agent/sandbox` | 6 | `e2b_test.go, local_test.go, manager_test.go, result_protocol_test.go, self_managed_test.go, ssh_test.go` |
| `internal/observability/otel` | 1 | `handler_test.go` (tracetest.SpanRecorder) |
| `internal/service` | 8+ | `canvas_decode_test.go, agent_run_e2e_test.go, agent_test.go, agent_sessions_test.go, chat_session_test.go, ...` |
| `internal/handler` | 10+ | `agent_test.go, agent_wait_for_user_test.go, admin_runtime_test.go, ...` |

## 附录 C · Deepdoc Service Endpoints (DLA/OCR/TSR)

### C.1 Endpoint summary

| Endpoint | URL | Status | Go port need |
|----------|-----|--------|--------------|
| DLA (Document Layout Analysis) | `POST {DEEPDOC_URL}/predict` | Remote HTTP (via `dla_cli.py`) | Go client with 3-retry + 18s timeout |
| OCR | **No remote endpoint** | Local ONNX only | None — `ErrNotImplemented` stub |
| TSR (Table Structure Recognition) | **No remote endpoint** | Local ONNX only | None — `ErrNotImplemented` stub |

Single toggle: `DEEPDOC_URL` (preferred) or `TENSORRT_DLA_SVR` (legacy).

### C.2 DLA HTTP contract

- **Method**: `POST {DEEPDOC_URL}/predict`
- **Body**: `multipart/form-data`, field name `request`, raw JPEG bytes
- **Response**: `{"bboxes": [[left, top, right, bottom, score, type_idx], ...]}`
- **Timeout**: 18s per request; **3 retries** per image
- **Failure sentinel**: empty list `[]`

## 附录 D · DSL v1 Corner Cases Inventory

### D.1 Top-level DSL shape

```json
{
  "components": {
    "<cpn_id>": {
      "obj": {"component_name": "Retrieval", "params": {...}},
      "downstream": ["generate_0"],
      "upstream": ["answer_0"]
    }
  },
  "path": ["begin"],
  "history": [],
  "retrieval": {"chunks": [], "doc_aggs": []},
  "globals": {"sys.query": "", "sys.user_id": "...", "sys.conversation_turns": 0,
              "sys.files": [], "sys.history": [], "sys.date": "..."},
  "variables": {},
  "memory": []
}
```

### D.2 Variable reference syntax

Two regexes:
```
variable_ref_patt    = r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z0-9_.-]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+)\} *\}*"
iteration_alias_patt = r"\{* *\{(item|index|result)\} *\}*"
```

Key behaviors the Go port must mirror:
- **Brace tolerance**: `{{var}}`, `{{ var }}`, `{{{var}}}` are all valid
- **`sys.*`/`env.*`**: namespace-only (no `@`), read from `State` flat namespace
- **`cpn_id@param.nested.path`**: dot-path traversal with `json.loads` on strings, `dict.get`, `list[int]` index
- **Empty `{{...}}`**: resolves to `""`, never crashes
- **`is_reff`**: returns `True` only if `cpn_id@param` resolves to a known component

### D.3 Component-name case-insensitivity

All comparisons use `.lower()`. Stored cpn_ids may be any case. Go port must NOT key component map by case-sensitive `cpn_id`.

## 附录 E · Component & Tool Interface Inventory

### E.1 Component inventory (22 → 19 active)

| # | Component | File | `component_name` | Tier | Key behavior |
|---|-----------|------|-----------------|------|--------------|
| 1 | Begin | `begin.py` | `Begin` | T3 | Consumes `kwargs["inputs"]`, resolves file inputs via FileService |
| 2 | UserFillUp | `fillup.py` | `UserFillUp` | T3 | Renders `tips` with variable interpolation; eino interrupt |
| 3 | Fillup | (alias) | `Fillup` | T3 | Thin alias of UserFillUp (disable `enable_tips`) |
| 4 | Message | `message.py` | `Message` | T3 | jinja2 prompt + stream + TTS + filegen + memory save |
| 5 | LLM | `llm.py` | `LLM` | T1 | Sync + async paths; chatModel.Generate / Stream; structured JSON output |
| 6 | Categorize | `categorize.py` | `Categorize` | T3 | LLM one-shot classification → `_next` (routing list) + `category_name` |
| 7 | Switch | `switch.py` | `Switch` | T2 | 12 operators; `_next` = matching downstream(s) |
| 8 | Agent | `agent_with_tools.py` | `Agent` | T1 | ReAct loop with LLMBundle + tool binding + citations |
| 9 | Iteration | `iteration.py` | `Iteration` | T4 | Compat stub → Parallel (Go) |
| 10 | IterationItem | `iterationitem.py` | `IterationItem` | T4 | Compat stub |
| 11 | Loop | `loop.py` | `Loop` | T4 | workflowx.AddLoopNode (Go) |
| 12 | LoopItem | `loopitem.py` | `LoopItem` | (none) | Engine-handled, not registered |
| 13 | ExitLoop | `exit_loop.py` | `ExitLoop` | (none) | `legacyNoOpNames` (Go) |
| 14 | Invoke | `invoke.py` | `Invoke` | T3 | HTTP GET/POST/PUT/PATCH/DELETE + headers/proxy/timeout |
| 15 | Browser | `browser.py` | `Browser` | T3 | LLM-driven browsing |
| 16 | DataOperations | `data_operations.py` | `DataOperations` | T3 | 7 ops: select_keys/literal_eval/combine/filter/append_or_update/remove/rename |
| 17 | ListOperations | `list_operations.py` | `ListOperations` | T3 | 6 ops: nth/head/tail/filter/sort/drop_duplicates |
| 18 | StringTransform | `string_transform.py` | `StringTransform` | T3 | split/merge/jinja2 template ops |
| 19 | VariableAggregator | `variable_aggregator.py` | `VariableAggregator` | T3 | Returns first non-empty in each variable group |
| 20 | VariableAssigner | `variable_assigner.py` | `VariableAssigner` | T3 | 11 ops |
| 21 | DocsGenerator | `docs_generator.py` | `DocGenerator` | T5 | MD → PDF/DOCX/TXT/MD/HTML |
| 22 | ExcelProcessor | `excel_processor.py` | `ExcelProcessor` | T5 | pandas read/write/merge/convert |

### E.2 Tool inventory (21)

All tools extend `ToolBase`, expose `get_meta()` (OpenAI function-call schema), `_invoke`/`_invoke_async`.

| # | Tool | `component_name` | Behavior |
|---|------|-----------------|----------|
| 1 | AkShare | `AkShare` | Chinese financial data (HTTP) |
| 2 | ArXiv | `ArXiv` | `export.arxiv.org/api/query` search |
| 3 | CodeExec | `CodeExec` | gRPC client to Python sandbox; **5 sandbox providers** in `internal/agent/sandbox/` |
| 4 | Crawler | `Crawler` | Generic HTML scraper |
| 5 | DeepL | `DeepL` | DeepL Translate API (HTTP) |
| 6 | DuckDuckGo | `DuckDuckGo` | `html.duckduckgo.com/html` search |
| 7 | Email | `Email` | SMTP send via `smtplib` |
| 8 | ExeSQL | `ExeSQL` | MySQL/PG/MSSQL/Trino/OceanBase via stdlib `database/sql` |
| 9 | GitHub | `GitHub` | GitHub REST API search |
| 10 | Google | `Google` | SerpAPI / Google CSE search |
| 11 | GoogleScholar | `GoogleScholar` | Scholar via SerpAPI |
| 12 | Jin10 | `Jin10` | Chinese financial news feed |
| 13 | PubMed | `PubMed` | NCBI E-utilities |
| 14 | QWeather | `QWeather` | HeFeng weather API |
| 15 | Retrieval | `Retrieval` | nlp.Dealer + kg.Retrieval (Go dual-registry) |
| 16 | SearXNG | `SearXNG` | Meta-search |
| 17 | TavilySearch | `TavilySearch` | Tavily search API |
| 18 | TavilyExtract | `TavilyExtract` | Tavily extract API |
| 19 | TuShare | `TuShare` | Tushare Chinese financial data |
| 20 | WenCai | `WenCai` | 同花顺 问财 stock Q&A |
| 21 | Wikipedia | `Wikipedia` | Wikipedia REST API |
| 22 | YahooFinance | `YahooFinance` | Yahoo Finance unofficial API |
| — | MCP | (server_id) | `MCPToolAdapter` over streamable-HTTP |

## 附录 F · Open Questions (actionable)

| ID | Question | Action | Effort |
|----|----------|--------|--------|
| OQ #1 | Iteration semantic preservation | ✅ Done — engine design | — |
| OQ #2 | MCP tool priority | ✅ Done — thin wrapper | — |
| OQ #3 | DSL normalization | ✅ Done — Go-side + `tools/migrate-canvas` built | — |
| OQ #4 | History window behavior | ✅ Done — canvas-level session | — |
| OQ #5 | Citation injection scope | ✅ Done — LLM + Agent | — |
| OQ #6 | Component timeout granularity | ✅ Done — per-class table is a Go enhancement over Python's uniform 600s | — |
| OQ #7 | Universe A/B naming asymmetry | ✅ Done — keep dual-naming convention | — |
| OQ #8 | GraphRAG scope | ✅ Done — KGRetrievalAdapter wired | — |
| OQ #9 | `generate` legacy alias | ⏸️ Deferred | — |
| OQ #10 | Phase 5a vs 5b ordering | ✅ Done — single Retrieval milestone | — |
| OQ #11 | Per-component env-driven timeout | ✅ Done — canvas-level uniform 600s | — |
| OQ #12 | Embedding model port | ✅ Done — model provider architecture | — |
| OQ #13 | Switch operator coverage | ✅ Done — 12/12 | — |
| OQ #14 | Universe A `SearchMyDataset` alias | ✅ Done — 4 spellings | — |
| OQ #15 | LLM `max_retries` / `delay_after_error` | ✅ Done — `retryInvoker.Unwrap()` normal-absolute-count | — |
| OQ #16 | Phase 4.4 orchestrator side | ✅ Done — Runner.Run catches interrupt | — |
| OQ #17 | Phase 5d CodeExec full feature parity | ⏸️ Partial — 5 providers + artifacts/args/timeout/per-language base image done; GraphRAG adapter remains | 1-2 weeks |
| OQ #18 | Phase 8b real TTS engine | ✅ Done — dispatcher routes through 60+ model drivers, no shell-out needed | — |
| OQ #19 | Phase 8b real MemorySaver completion | ⏸️ Open | 1-2 weeks |
| OQ #20 | Phase 5c DB2 e2e demand | ⏸️ Open (CGO + native lib) | 0.5-1 week if needed |
| OQ #21 | Compile LRU cache | ⏸️ Open — defer until profiling | 1-2 weeks |
| OQ #22 | Phase 6 component hardening | ⏸️ Open — Browser Playwright parity + ExcelProcessor audit | 1-2 weeks |
| OQ #23 | `tools/gen-component-parity` script | ✅ Done | — |

---

> **Last verified**: 2026-06-17
