# Agent Canvas Go Port — Design Document

> **Status:** Phase 1 / 2.5 / 3 / 4 / 5 / 5.5 核心功能已落地，Phase 6 (灰度) / Phase 7 (清理) 未启动
> **Last cross-checked against code:** 2026-06-11 (commit `aa270bed7`)
> **Source of truth:** `internal/agent/` (canvas, component, tool, runtime, workflowx, dsl) + `internal/observability/otel/`
> **Supersedes:** `.claude/plans/agent-go-port.md`, `.claude/plans/eino-workflow-loop.md`, `.claude/plans/eino-workflow-parallel.md`, `.claude/plans/fluffy-strolling-bear.md`, `.claude/plans/refactor-canvas-loop.md`

This document consolidates the five plan files in `.claude/plans/` into a single design-of-record. It describes the **current** state (present tense), verified against the code, with a final section that calls out where reality diverged from the original plans.

---

## 1. 概述 / Overview

### 1.1 目标

RAGFlow 的 Agent Canvas（编排 22 个 component + 21 个 tool 的 DSL 执行器）从 Python 移植到 Go。Python 端位于 `agent/canvas.py`（`Graph` / `Canvas`）+ `agent/component/base.py`（`ComponentBase` / `ComponentParamBase`）+ `agent/tools/`。Go 端独立实现于 `internal/agent/`，与 Python 端通过共享 DSL JSON schema 兼容（v1↔v2 双向转换器在 `internal/agent/dsl/`）。

### 1.2 核心架构决策

**State + Workflow 混血**：eino 的 `compose.Workflow` 提供声明式拓扑（节点 + exec 边）+ 并发调度；`compose.WithGenLocalState` + `WithStatePreHandler/WithStatePostHandler` 提供任意节点读任意节点输出的"状态变量"能力。State 解决 `{{cpn_id@param}}` 任意交叉引用问题，Workflow 解决执行拓扑 + cancel + checkpoint 问题。

**5-tier 移植策略**：T1（直接复用 eino 内置）→ T2（薄包装）→ T3（Lambda + State）→ T4（嵌套 Workflow 子图）→ T5（重 I/O + 第三方 lib）。判定原则：功能相当 → 优先 eino 内置，禁止复制 Python 端的黑魔法（`_feeded_deprecated_params`、partial hack、`thread_pool_exec` 异步伪装等）。

**Checkpoint 存 Redis**：eino `compose.CheckPointStore` 是纯 KV 接口，Redis String + EXPIRE 是天然 fit。业务元数据（status / canvas_id / parent_run_id）走独立 Redis Hash（**由应用层显式控制**，不依赖 eino 自动写）。

**Observability 走 OpenTelemetry**：弃用 §2.10 v1 "Redis Stream + MySQL 双写"，改用 OTLP HTTP exporter + eino `callbacks.Handler` 注入 span。理由：业界事实标准；与 Python langfuse（OTel-based）互通；零新表。

**AGPL-3 零容忍**：T5 DOCX 库穷举后全部 AGPL-3/维护停滞，**自实现 OOXML writer**（`archive/zip` stdlib + `text/template`）；PDF 选 `signintech/gopdf` (MIT)；Excel 选 `xuri/excelize/v2` (BSD-3)；Markdown 选 `yuin/goldmark` (MIT)。

---

## 2. 顶层模块布局 / Module Layout

```
internal/agent/
├── canvas/              # 画布执行器（eino 编译、状态调度、checkpoint、cancel、stream）
│   ├── canvas.go        # Canvas struct, BuildWorkflow, Run/Stream
│   ├── state.go         # CanvasState, Outputs/Sys/Env/Path/History
│   ├── state_export.go  # WithState / GetStateFromContext (runtime 包的薄重导出，测试用)
│   ├── variable.go      # {{cpn_id@param}} / sys.x / env.x 解析
│   ├── scheduler.go     # State pre/post handler + 节点 lambda
│   ├── node_body.go     # 单节点 lambda 体（state in/out + 调 component）
│   ├── loop_subgraph.go # Loop 宏展开（buildSubWorkflow + translateLoopCondition）
│   ├── cycle_wrap.go    # cycle detection + back-edge 切断
│   ├── cancel.go        # Redis cancel 协议 (watchCancel goroutine)
│   ├── stream.go        # SSE 通道
│   ├── compile.go       # eino 编译 + WithCheckPointStore + WithSerializer
│   ├── checkpoint_store.go  # RedisCheckPointStore (Get/Set/Delete)
│   ├── run_tracker.go   # RunTracker (Start/MarkSucceeded/MarkFailed/MarkCancelled/AttachCheckpoint)
│   └── state_serializer.go  # CanvasStateSerializer (encoding/json, eino Serializer 签名无 ctx)
│
├── component/           # 19 components + 5 helpers
│   ├── base.go          # Component interface + ParamError + ErrNotImplemented
│   ├── registry.go      # name → factory 映射
│   ├── runtime_wire.go  # 组件与 runtime 包的桥接
│   ├── io_init.go       # T5 组件初始化
│   ├── v1_stubs.go      # v1 DSL compat 桩
│   ├── agent.go         # T1 — react.NewAgent
│   ├── llm.go           # T1 — EinoChatModel 薄包装
│   ├── switch.go        # T2 — NewGraphMultiBranch
│   ├── begin.go / message.go / categorize.go / invoke.go / browser.go
│   ├── data_operations.go / list_operations.go / string_transform.go
│   ├── variable_aggregator.go / variable_assigner.go
│   ├── fillup.go / userfillup.go
│   ├── loop.go          # T4 — no-op marker, 实际工作由 loop_subgraph 接管
│   ├── parallel.go      # T4 — workflowx.AddParallelNode 包装
│   ├── docs_generator.go / excel_processor.go   # T5
│
├── tool/                # 21 tools (统一 eino tool.InvokableTool)
│   ├── registry.go      # BuildAll / BuildByName (支持 alias: execute_sql/exesql, retrieval/search_my_dateset)
│   ├── http_helper.go   # 共用 HTTP client (context + retry)
│   ├── ssrf.go          # SSRF 防护
│   ├── akshare.go / arxiv.go / code_exec.go / crawler.go / deepl.go
│   ├── duckduckgo.go / email.go / exesql.go / github.go / google.go
│   ├── google_scholar.go / jin10.go / pubmed.go / qweather.go
│   ├── retrieval.go / searxng.go / tavily.go / tushare.go
│   ├── wencai.go / wikipedia.go / yahoo_finance.go
│
├── runtime/             # canvas + component 共享的运行时契约（无 cycle）
│   ├── component.go     # Component interface (从 component/base.go 提取)
│   ├── context.go       # GetStateFromContext / withState
│   ├── state.go         # CanvasState + NewCanvasState + GetVar/SetVar/ReadVars
│   ├── template.go      # ResolveTemplate (从 canvas/variable.go 提取)
│   ├── selector.go      # component selector 辅助
│   └── metrics.go       # runtime metrics
│
├── workflowx/           # eino 扩展（零侵入，外部 helper）
│   ├── loop.go          # AddLoopNode[T] — 通用 do-while 循环节点
│   ├── parallel.go      # AddParallelNode[I,O] — 通用 bounded-concurrency 节点
│   └── *_test.go        # 单元 + 集成测试（miniredis 风格的内存 store）
│
└── dsl/                 # DSL v2 schema + v1↔v2 双向转换器
    ├── v2.go            # Go-native 强类型 schema（version=2, 无 _feeded_deprecated_params 装饰）
    ├── loader.go        # 自动检测 v1/v2，输出统一 v2 内存模型
    ├── converter_v1_to_v2.go
    └── converter_v2_to_v1.go

internal/observability/otel/
├── provider.go          # TracerProvider 工厂（读 OTEL_EXPORTER_OTLP_ENDPOINT，未配置时返回 noop）
├── handler.go           # eino callbacks.Handler → OTel span
└── handler_test.go      # tracetest.SpanRecorder 单元测试
```

**实际文件计数**（与 §14 计划偏差）：

- Components: **19 个** (计划写 22 → 21) — 见 §14.1 偏差说明
- Tools: **21 个** (计划 21 ✓)
- Test files: 35+ (含 loop_semantics_test.go, dsl_examples_e2e_test.go, cycle_wrap_test 等)

---

## 3. 架构 / Architecture

### 3.1 State + Workflow 混血

eino `compose.Workflow` 本身只支持 DAG（节点间数据通过 declared predecessor 输出传递），没有"任意节点读任意节点输出"的现成 API。RAGFlow Python 端用 `self._canvas.get_variable_value("cpn_id@param")` 实现 `{{cpn_id@param}}` 任意交叉引用。

**Go 端方案**：

1. **State 承载变量**：每个 canvas run 创建 `*CanvasState`，挂在 `context.Value` 上。所有节点通过 `runtime.GetStateFromContext(ctx)` 读写。
2. **State pre-handler**：在 `g.AddLambdaNode(...)` 时挂 `compose.WithStatePreHandler[map[string]any, *runtime.CanvasState](canvasPre)`，从 State 提取节点输入。
3. **State post-handler**：挂 `compose.WithStatePostHandler`，把节点输出回写 State。
4. **Workflow 承载拓扑**：节点按 `downstream` / `upstream` 加 exec 边，**数据流走 State 不走边**。eino 静态拓扑分析仍然能看到 exec 边，调度正确性不丢失。

```go
// internal/agent/canvas/scheduler.go — 节点加挂方式
node := wf.AddLambdaNode(cpnID, nodeBody,
    compose.WithStatePreHandler[map[string]any, *runtime.CanvasState](canvasPre),
    compose.WithStatePostHandler[map[string]any, *runtime.CanvasState](canvasPost),
)
for _, upID := range comp.Upstream {
    node.AddInput(upID)  // exec 边
}
```

**关键修正**（vs §2.6 v1 plan）：`WithStatePreHandler/WithStatePostHandler` 是 `GraphAddNodeOpt`（节点选项），**不是** `GraphCompileOption`（编译选项）。传给 `g.Compile(...)` 编译失败。eino 实际签名：

- `compose.NewGraph[I,O](opts ...NewGraphOption)` — 工厂选项，含 `WithGenLocalState`
- `g.AddNode(name, lambda, opts ...GraphAddNodeOpt)` — 节点选项，含 `WithStatePreHandler/WithStatePostHandler`
- `g.Compile(ctx, opts ...GraphCompileOption)` — 编译选项，含 `WithCheckPointStore/WithSerializer/WithInterruptBeforeNodes/WithInterruptAfterNodes`

### 3.2 `runtime` 包：消除 `canvas <-> component` cycle

**问题**：`component/` 大量文件（Begin/Message/Switch/Browser/...）需要调 `canvas.CanvasState` / `canvas.GetStateFromContext` / `canvas.ResolveTemplate` / `canvas.SetDefaultFactory`；同时 `canvas` 通过 `ComponentFactory` 间接依赖 `component` 的具体实现。强行 `canvas -> component` 形成 Go import cycle。

**方案**（来自 `fluffy-strolling-bear.md`，已落地）：把"运行时共用契约"提取到 `internal/agent/runtime/`，**canvas 和 component 都依赖 runtime，但不互相依赖**。

| 提取到 runtime | 留在 canvas | 留在 component |
|---------------|-------------|----------------|
| `Component` interface | DSL graph types (`Canvas`, `CanvasComponent`, `CanvasComponentObj`) | component registry + factory |
| `CanvasState` + `GetVar/SetVar/ReadVars` | 拓扑构建 (`BuildWorkflow`, `buildLoopExpansion`, scheduler wiring) | 具体 component 实现 |
| `GetStateFromContext` / `withState` / `WithState` | checkpoint / workflow 编译 orchestration | `NewBeginComponent`, `NewMessageComponent`, ... |
| `ResolveTemplate` + 纯 runtime 模板 helpers | Loop 宏展开 logic | |
| `ParamError`, `ErrNotImplemented` | | |

**`state_export.go` 薄重导出**：测试代码从 `canvas.WithState` 改为 `runtime.WithState` 是机械性替换。为减少 churn，`canvas/state_export.go` 提供薄 alias（`type CanvasState = runtime.CanvasState` 等），但**生产代码不再 import `canvas` 来获取 state**。

### 3.3 调度模型

```go
// internal/agent/canvas/canvas.go:BuildWorkflow
func BuildWorkflow(ctx context.Context, c *Canvas, store compose.CheckPointStore, ser compose.Serializer) (*compose.Workflow[map[string]any, map[string]any], error) {
    wf := compose.NewWorkflow[map[string]any, map[string]any]()

    for cpnID, comp := range c.Components {
        // 1. 加节点（含 state pre/post handler）
        node := wf.AddLambdaNode(cpnID, nodeBody,
            compose.WithStatePreHandler[map[string]any, *runtime.CanvasState](canvasPre),
            compose.WithStatePostHandler[map[string]any, *runtime.CanvasState](canvasPost),
        )
        // 2. 加 exec 边
        for _, upID := range comp.Upstream {
            node.AddInput(upID)
        }
        // 3. 错误跳转
        if comp.ExceptionTo != "" {
            node.AddInputWithOptions(
                buildExceptionDummy(comp),
                compose.WithNoDirectDependency(),
                compose.WithExceptionBranch(/* ... */),
            )
        }
    }
    // 4. 编译（仅编译期选项）
    return wf.Compile(ctx,
        compose.WithCheckPointStore(store),
        compose.WithSerializer(ser),
    )
}
```

**`canvasPre` / `canvasPost`**：State pre-handler 从 `CanvasState.Outputs[cpn]` 提取节点入参（沿用 `{{cpn_id@param}}` 正则解析）；post-handler 把节点出参回写 `CanvasState.Outputs[cpn_id]`。eino 拓扑上只有 exec 边，data flow 走 State。

---

## 4. Component 库 / Component Library

### 4.1 5-tier 移植策略（**已落地**）

| Tier | 含义 | 验收 |
|------|------|------|
| **T1** | 直接用 eino 已有类型/接口，零代码 | eino 单元测试覆盖 |
| **T2** | 薄包装 1 struct + factory，对齐 Python 行为参数 | 跨 eino/RAGFlow 边界 + 1 e2e |
| **T3** | `compose.Lambda` + `StatePre/PostHandler` | 1 单测 + 1 e2e |
| **T4** | 嵌套 `compose.Workflow` + `getState[CanvasState](ctx)` | 子图单测 + 完整 e2e |
| **T5** | 重 I/O + 第三方 lib | 单测 + e2e + 失败注入 |

**判定原则**：T1 > T2 > T3 > T4 > T5 时**禁止跳级**。除非 eino 抽象**确无对应**。

### 4.2 Component 现状

**19 个 .go 文件**（实际；计划写 22 → 21）：

| Component | Python 行为 | Tier | Go 实现 |
|-----------|------------|------|---------|
| **LLM** | `LLMBundle` 单轮 chat + JSON output + cite + stream | T1 | `EinoChatModel` 薄包装 `internal/entity/models/<provider>.go`；实现 `model.ToolCallingChatModel`（含 `WithTools` 并发安全） |
| **Agent** | ReAct + tool/MCP + 多轮 stream | T1 | `react.NewAgent` + `compose.ToolsNodeConfig{Tools: tools}` + 22 tool 全注册；citation 中间件 + tool artifact 收集为未来增量（**当前未实现**，见 §14） |
| **Switch** | 多条件 (and/or) → 多 downstream + ELSE | T2 | `compose.NewGraphMultiBranch` 路由 |
| **Categorize** | LLM 分类 + 路由 | T3 | Lambda 调 LLM + `compose.NewGraphMultiBranch` |
| **Begin** | DSL 入口 + 注入 inputs + 文件 inputs | T3 | Lambda + `StatePreHandler`；文件走 `internal/service/file_service.go` |
| **UserFillUp / Fillup** | Jinja2 + file inputs | T3 | `text/template` 替代 Jinja2 |
| **Message** | 最终输出（jinja2 + stream + downloads + filegen） | T3 | Lambda + `schema.StreamReader` + `text/template` + MinIO |
| **Invoke** | HTTP 客户端 + HTML 清洗 + JSON | T3 | `net/http` + `golang.org/x/net/html` |
| **Browser** | LLM + HTTP + 文件下载 + MinIO | T3 | 复用 Invoke + LLM + storage |
| **DataOperations** | dict 7 类操作 | T3 | Lambda + `encoding/json` + `go/ast` |
| **ListOperations** | slice 6 类操作 | T3 | Lambda + `slices` (Go 1.21+ stdlib) |
| **StringTransform** | split/merge + Jinja2 | T3 | Lambda + `strings.Split` + `text/template` |
| **VariableAggregator** | 多 group，first-non-empty | T3 | Lambda + State 读 |
| **VariableAssigner** | 12 个算子原地改 State | T3 | Lambda + State 写 |
| **Loop** | 条件循环 + `loop_variables` 初始化 + 终止评估 | T4 | **`compose.NewWorkflow` + `workflowx.AddLoopNode`**（loop.go 自身变为 no-op marker；实际工作由 `canvas/loop_subgraph.go` 宏展开接管） |
| **Parallel** | 数组并行处理 | T4 | `workflowx.AddParallelNode` 包装（见 §6） |
| **DocsGenerator** | pdf/docx/txt/md/html 生成 | T5 | `signintech/gopdf` (PDF) + 自实现 OOXML writer (DOCX) + `yuin/goldmark` (MD) |
| **ExcelProcessor** | pandas 读/合并/转换 Excel | T5 | `xuri/excelize/v2` (BSD-3) |

### 4.3 不移植的 Python 端"遗产"

| Python 端 | 不移植原因 |
|----------|-----------|
| `_feeded_deprecated_params` / `_deprecated_params` / `_user_feeded_params` 三层装饰 | DSL v2 已去除；Go `ComponentParamBase` 不引入 |
| `ComponentParamBase.validate()` + `param_validation/*.json` 96 文件 | Go struct tag + `go-playground/validator/v10` 替代 |
| `ComponentBase.thread_limiter = asyncio.Semaphore(...)` | Go `errgroup.SetLimit(MAX_CONCURRENT_CHATS)` (stdlib x/sync) |
| `partial` 流式 hack | eino `schema.StreamReader` 原生流式 |
| `thread_pool_exec(self._invoke, **kwargs)` 异步伪装 | Go 全程 goroutine |
| `set_output("_ERROR", ...)` + `set_exception_default_value()` 双轨 | Go `error` 单一返回 + eino `OnError` callback |
| `ExitLoop` no-op 节点 | DSL v1 compat 通过 `legacyNoOpNames` 在 canvas 层吸收，**不注册 component** |
| `LoopItem` 组件 | LoopItem 角色由 `workflowx.AddLoopNode` 内部 machinery 取代，**不注册 component** |
| `Iteration` / `IterationItem` 组件 | IterationItem 角色合并到 `Loop` 单节点模式（**Iteration + IterationItem 也走 workflowx.AddLoopNode 同一路径**，但 Loop 终止条件为"遍历完成"而非"条件成立"） |

### 4.4 Tool 实现统一模式

```go
// internal/agent/tool/registry.go
type Tool interface {
    einotool.InvokableTool  // eino 协议：Info() / InvokableRun(ctx, args, opts)
}

func BuildAll(names []string, params map[string]map[string]any) ([]einotool.BaseTool, error)
func BuildByName(name string, params map[string]any) (einotool.BaseTool, error)
```

**Alias 一致性**（`TestToolRegistry_SchemasAreComplete` 覆盖）：
- `execute_sql` 和 `exesql` 都 surface canonical `Info().Name == "execute_sql"`
- `retrieval` 和 `search_my_dateset` 都 surface canonical `Info().Name == "search_my_dateset"`

**22 tool 表**（与 plan 一致；alias 不算新 tool）：
- akshare, arxiv, code_exec, crawler, deepl, duckduckgo, email, exesql(=execute_sql), github, google, google_scholar, jin10, pubmed, qweather, retrieval(=search_my_dateset), searxng, tavily, tushare, wencai, wikipedia, yahoo_finance = **21 唯一** tool

**Tool 通用模式**：HTTP 类 tool 走 `http_helper.go`（context + retry + 简单指数 backoff）；ExeSQL 走 stdlib `database/sql` + 各 driver（**不复用** `internal/dao` GORM——DAO 是 RAGFlow 元数据库层，与 ExeSQL 用户的外部 DB 完全独立）；CodeExec 调既有 Python sandbox gRPC（保留现状，**不重写沙箱**）；Retrieval 直接进程内 `import internal/service/nlp/retrieval.go`（Dealer 后端已 Go 化），`use_kg=True` 暂不支持。

---

## 5. DSL v2 / DSL

### 5.1 v2 schema（强类型，去装饰）

```go
// internal/agent/dsl/v2.go（实际）
type Canvas struct {
    Version    int                  `json:"version"`    // 固定 = 2
    Components map[string]Component `json:"components"`
}

type Component struct {
    ID         string         `json:"id"`
    Name       string         `json:"name"`         // e.g. "Retrieval"
    Downstream []string       `json:"downstream"`
    Params     map[string]any `json:"params"`
    Outputs    map[string]any `json:"outputs,omitempty"`  // 运行时填充，DSL 加载时不存在
}
```

**去掉的装饰**：v1 嵌套 `obj`、`_feeded_deprecated_params` / `_deprecated_params` / `_user_feeded_params` 三层集合、`custom_header`。

**对比 plan §4.6 原始 v2 设计**：plan 还规划了 `Path` / `History` / `Retrieval` / `Globals` / `Metadata`（含 author/tags/created_at）字段——**这些字段在实现时全部砍掉**。状态信息（`Path` / `History` / `Retrieval` / `Globals`）被推到了 **runtime `CanvasState`**（`internal/agent/runtime/state.go:54-66`）—— DSL 只描述拓扑，运行时由 State pre/post handler 填充。这是更聪明的设计：避免 DSL schema 携带运行时状态导致的反序列化陷阱。

**`Metadata` 字段决策**（**Q4 2026-06-11 闭环**）：v2 schema 不携带画布级 metadata（author/tags/created_at）。元数据走 RAGFlow 后端已有字段：`user_canvas.title` / `user_canvas.description`（`internal/entity/canvas.go:25, 28`）—— 业务表空间已存这些信息，不需要在 DSL JSON 里重复。**未来若需要标签/作者等元数据**，建议加 `user_canvas.tags` / `user_canvas.author_id` 列而不是改 DSL schema。详见 §14.8 Q4。

**保留**：`{{cpn_id@param}}` / `sys.x` / `env.x` 语法（运行时通过 `runtime.GetVar` 解析）；`sys` / `env` 命名空间在 `CanvasState.Sys/Env` 持有（不在 DSL）。

### 5.2 v1 ↔ v2 双向转换器

**v1 → v2**（`internal/agent/dsl/converter_v1_to_v2.go`）：Phase 2.5 必跑，作为 Phase 2 component 输入适配器，避免每个 component 自己处理 v1 装饰字段。

**v2 → v1**（`internal/agent/dsl/converter_v2_to_v1.go`，Phase 5.5，~270 行）：

行为契约：

- 校验输入 canvas（nil / 空 / 无效 → error）
- 按**确定性顺序**迭代 components：`begin_…` 前缀排最前，其余按字典序。自定义 `MarshalJSON` on `v1Envelope` 强制执行（Go 默认 map 编码器按 key 文本排序，会打乱顺序）
- **Key 还原**：v2 id `<name>_<UUID>` → v1 key `<Name>:<UUID>`：
  - 从左边第一个 `_` 切分（`switch_abc_def` → `Switch:abc_def`）
  - name 半段首字母大写（best-effort PascalCase）
  - **空 uuid 半段**（尾部 `_`，来自 v1 无冒号的 `begin` legacy key）→ **不加冒号**（`Begin` 而非 `Begin:`），使 `v1ToV2` 能经无冒号分支重新解析。这是唯一切离 §5 spec 示例的地方，为 round-trip closure 必需
  - **大小写是有损的**：UUID 半段在 `v1ToV2` 上游被小写化；全大写名称会变为首字母大写（`LLM:abc` → `llm_abc` → `Llm:abc`）。结构不变量 `v1ToV2(v2ToV1(v1ToV2(x))) == v1ToV2(x)` 保持
- 构建 v1 entry 形状：
  ```json
  {
    "downstream": ["<v1 keys>"],
    "obj": {
      "component_name": "<name>",
      "params":     {…},
      "downstream": ["<v1 keys>"]
    }
  }
  ```
- 空 `downstream` 输出 `[]`（非 `null`），空 `params` 输出 `{}`（非 `null`）
- **永不输出**三个 legacy 字段（`_deprecated_params` / `_feeded_deprecated_params` / `_user_feeded_params`）——v2 不携带它们，重新输出等于重新引入已删掉的 bug
- 用 `json.Indent` 2 空格格式化输出

**v2→v1 测试覆盖**（12 个，全部通过）：

| 测试 | 覆盖点 |
|------|--------|
| `TestV2ToV1_WebSearchAssistant` | 30 KB 真实模板完整 v1→v2→v1→v2 round-trip |
| `TestV2ToV1_CustomerFeedback` | 同上，customer_feedback_dispatcher.json |
| `TestV2ToV1_IngestionPipeline` | 同上，ingestion_pipeline_general.json |
| `TestV2ToV1_EmptyDownstream` | 单组件 → `"downstream": []`（非 null） |
| `TestV2ToV1_NilParams` | 双组件 → 两个 `"params": {}`（非 null） |
| `TestV2ToV1_NoLegacyFields` | 全量数据输入，输出零 legacy 子串 |
| `TestV2ToV1_DeterministicOrder` | 两次调用（含 map 突变）→ 字节级相同 |
| `TestV2ToV1_KeyRestore` | `begin_abc`→`Begin:abc`, `begin_`→`Begin`(无冒号), `switch_abc_def`→`Switch:abc_def` |
| `TestV2ToV1_NilCanvas` | nil → error，不 panic |
| `TestV2ToV1_EmptyComponents` | 空 map → error |
| `TestV2ToV1_BeginFirst` | Begin 是输出 JSON 第一个 key（领先 Alpha/Zeta） |
| `TestV2ToV1_ParamOrderStable` | 嵌套 map/slice/scalar params round-trip |
| `TestV2ToV1_AcceptanceFixture_Smoke` | e2e：v1ToV2 → v2ToV1 → LoadV1 → v1ToV2 无错误 |

DSL 包总测试：42 个（30 + 12）。

**已知限制**（已在代码中注释，非 bug）：

| 限制 | 原因 | 影响 | 缓解 |
|------|------|------|------|
| v1 key 大小写有损（`LLM:abc` → `Llm:abc`） | `v1ToV2` 正向路径把两半都小写化 | 装饰性；v1 key 字符串不逐字节保持 | 对比走 v2（正则形式） |
| v1 输出省略 `upstream` | Plan §5 未指定；Python reader 从 `downstream` 计算 | 若 Python reader 容忍缺失则无影响 | 若 §2.2 run-book 发现需要再补 |
| `Begin` key 输出无冒号（`Begin` 非 `Begin:`） | `v1ToV2` round-trip 所需；spec 示例 `Begin:` 无法重新解析 | 无；`Begin` 和 `Begin:abc` 都是合法 v1 | 若需更新 spec，标注示例仅为示意 |
| map 迭代非确定性通过自定义 `MarshalJSON` 规避 | Go `map[string]X` 不排序 | 无——自定义序列化器保障顺序 | 移除自定义序列化器的前提是 Go 支持有序 map |

### 5.3 Round-Trip 闭合不变量

对三个真实模板，以下不变量成立：

```
v1 (template) ──v1ToV2──> v2_a ──v2ToV1──> v1' ──v1ToV2──> v2_b
                                                  │
                                                  └─ component ID set 相同
                                                     downstream refs 相同
                                                     params (canonical JSON) 相同
                                                     as v2_a
```

这是在纯 Go 环境中可验证的最强确定性不变量。Python reader 输入 `v1'` 会计算出同一 `v2_b`——由上述闭合性质保证——从而得出相同的执行图。

**验收**（Phase 5.5）：100 条 v1 样本 round-trip（v1→v2→v1→v2 字段不变）；v2 写出的 DSL 喂给旧 Python reader 端到端验证。**数据源约束**：首选 InfiniFlow SRE 维护的 staging 固定回放集（≥200 条覆盖 P0-P4）；回退到生产 DB 抽样需 DPO + DBA + 季度上限 100 条 + ledger 登记；**不接受未脱敏/未登记生产 DSL 流入测试链**。

**本地运行**：
```bash
cd internal/agent/dsl
go test -count=1 -run TestV2ToV1 -v   # 12 个测试，~1s
go test -count=1 .                    # 全部 42 个 dsl 测试
go vet ./...
gofmt -l .                            # 预期无 diff
```

### 5.4 Staging 验收闸门（Phase 6 前置条件）

以下两项**无法在 dev 环境执行**，需在 staging 环境由 SRE 团队驱动。Phase 6（灰度）**在两者都通过前不得启动**。

**闸门 1：100 样本 staging 语料库回放**

blocker：`staging_canvas_snapshot_2026q2.json`（100 条 v1 DSL）由 InfiniFlow SRE 维护，dev 环境不可用。当前替代方案：10 条 `agent/templates/*.json` 真实模板（与 Phase 2.5 共用）。

staging run-book：
1. 从 SRE staging object store 拉取语料库（路径 TBD，联系 `@ragflow-sre`）
2. 放入本地目录
3. 执行：`go test -count=1 -run TestV2ToV1_StagingCorpus -tags=staging`（`staging` build tag 防止 CI 默认运行）
4. 预期：100/100 条目 round-trip 结构等价
5. 若有失败：记录条目 ID + 输入前 200 字符，提 `phase-5.5-corpus-fail` issue

**闸门 2：Python reader 兼容性测试**

blocker：dev 环境无 Python canvas runtime。需验证 Go 发出的 v1 DSL 能被旧 Python reader 加载。

staging run-book：
1. 构建微型 Go 二进制（或 `go test` entry point），读 v1 template → `v1ToV2` → `v2ToV1` → 写 v1 JSON 到 stdout
2. 管道输入 Python reader：`go run ./cmd/v2-to-v1 < web_search_assistant.json | python -m agent.canvas.load_dsl -`
3. 预期：Python reader 返回的 `Graph` 的 nodes 和 edges 与输入匹配（允许 v1 key 大小写恢复的装饰性损失）
4. 若 Python reader 报错：记录 traceback，提 `phase-5.5-python-fail` issue。最可能出问题的字段（按嫌疑排序）：`upstream`（我们省略了，Python 应从 `downstream` 计算）、`obj.params` 形状（我们保持原样）、`Begin` key 有无冒号

---

## 6. workflowx 扩展 / workflowx Extensions

`internal/agent/workflowx/` 提供**零侵入 eino 扩展**——不修改 eino 源码，不添加方法到 `compose.Workflow`，只提供外部 helper。

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
- 嵌套子 workflow 走 `compose.Runnable[T,T]` + sub-checkpoint 通过 loop-owned bridge store（**不要求 caller 单独配 child store**）

**Checkpoint/Resume 合约**（P0 acceptance）：

- Invoke path 嵌套 interrupt → 通过 `compose.CompositeInterrupt` 向上传播；resume 从中断的 iteration 继续（不重头）
- Stream path 走 **iteration-granular** 恢复合约：已完整发到下游的 iteration 不重放；中断的 iteration 可能整体重放（**不承诺 chunk-granular resume**——eino 公开 API 不支持）
- 稳定 child checkpoint ID 通过 `WithLoopCheckpointIDBuilder(nodeKey, iteration)`；默认 `workflowx-loop:<nodeKey>:<iteration>` 命名空间

**Loop 在 canvas 中的应用**（`refactor-canvas-loop.md`，已落地）：

- `Loop` 在 Go 端是**单节点**：registry 注册 + 工厂，但 `LoopComponent.Invoke` 是 no-op（实际工作由 `canvas/loop_subgraph.go` 宏展开接管）
- `BuildWorkflow` 看到名为 `Loop` 的 cpn 时：调用 `expandLoopSubgraph` 收集下游、构建 sub-`compose.Workflow[map[string]any, map[string]any]`、调 `workflowx.AddLoopNode` 把结果作为单节点插入外图，把 Loop 和它的 descendant 从外图节点 map 移除
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

- 外层 invoke-only；内层 sub workflow 可 stream-capable（eino runnable 兼容规则接管 stream 转发）
- `WithParallelMaxConcurrency(n int)`：0 / 1 = 顺序执行（主 goroutine 跑，**不**起 worker goroutine）；> 1 = 信号量并发（首 item 主 goroutine，后续 goroutine）
- **顺序保持不变量**：`outputs[i]` 永远对应 `inputs[i]`——并发路径下，每个 goroutine 捕获 `idx` 闭包写入预分配 `outputs[idx]`，与完成顺序无关
- 错误处理：`ErrParallelCompileFailed` / `ErrParallelResumeStateInvalid`；per-item 错误用 `fmt.Errorf("item %d: %w", idx, err)` 包装
- 嵌套 interrupt：累积到 `compose.CompositeInterrupt(ctx, nil, state, interruptErrs...)`
- 恢复不变量：`CompletedResults ∪ InterruptedIndices = 0..TotalCount-1`（partition 完整），`InterruptedIndices` = 补集（不是仅显式返回 interrupt 的 index——并发场景下未 durable 完成的也算）

**模型参考**：本扩展以 `cloudwego/eino-examples/compose/batch/batch/node.go` 的 batch 节点为参照；区别是 reference 是 registered Component，本扩展是 free helper（不依赖 component registry，非 DSL caller 也能用）。

**Parallel 在 canvas 中的应用**（`component/parallel.go`）：

- `Parallel` component 走 T4 薄包装：注册时传 `agenttool.BuildByName("parallel", params)`（注：实际是 `internal/agent/component/parallel.go` 的 `ParallelComponent`，不通过 tool registry），内部用 `workflowx.AddParallelNode` 把 sub-workflow 插入外图

---

## 7. Checkpoint + Run Tracker / Persistence

### 7.1 双 key 设计

**Key 1：`agent:cp:{check_point_id}`** — eino payload 存储

- 类型：String（直接存 `[]byte`，**不走 JSON** —— eino Serializer 已负责序列化）
- TTL：30 天，Set 时 `EXPIRE 30*24*3600` 一次设置
- eino `CheckPointStore` 是**纯 KV 接口**（`internal/core/interrupt.go:27`）—— `Get(ctx, id) ([]byte, bool, error)` / `Set(ctx, id, []byte) error`
- eino **不会**自动写入 status / canvas_id / tenant_id / run_id / parent_id / expires_at 等业务字段

**Key 2：`agent:run:{run_id}`** — 业务元数据存储（Redis Hash）

| 字段 | 类型 | 含义 |
|------|------|------|
| `canvas_id` | string | `user_canvas.id` |
| `tenant_id` | string | |
| `checkpoint_id` | string | 当前 run 的最新 checkpoint（指向 key 1） |
| `parent_run_id` | string | resume_from 源 run（续跑链），可空 |
| `status` | int (0/1/2/3) | 0=running 1=succeeded 2=failed 3=cancelled |
| `failure_reason` | string | 失败原因（err.Error()） |
| `cancel_requested` | int (0/1) | 1=用户/admin 已请求 cancel |
| `started_at` | int (epoch ms) | |
| `finished_at` | int (epoch ms) | 退出时填写 |

- TTL：30 天（与 key 1 同步，Set 时 `EXPIRE 30*24*3600`）
- `RunTracker.Start/MarkSucceeded/MarkFailed/MarkCancelled/AttachCheckpoint` 显式调用
- **不依赖 eino 自动写**——cancel/fail 后的 `status=failed` 由应用层自己写

### 7.2 4 个 eino payload 写入触发（写 `agent:cp:*`）

| # | 触发点 | eino 源码 | 用途 |
|---|--------|-----------|------|
| **W1** | 节点显式 `compose.Interrupt(ctx, info)` / `StatefulInterrupt(ctx, info, state)` | `compose/interrupt.go:110, 130` | human-in-the-loop、外部 API 回调、限流暂停 |
| **W2** | `compose.WithInterruptBeforeNodes([]string)` / `WithInterruptAfterNodes([]string)` 编译期拦截点 | `compose/interrupt.go:31, 37` | 命中后**写盘 + 终止 run**（与 W1 共用 `handleInterrupt` 路径）；**默认开 0 个** |
| **W3** | 子 graph interrupt 向上传播 | `subGraphInterruptError`，`compose/interrupt.go:340` | 嵌套 subgraph / ToolsNode / agentic 抛 interrupt 时，父 graph 同步落盘 |
| **W4** | 运行退出 | `WithCheckPointID` + `WithWriteToCheckPointID` | run 退出时最后一次落盘；**每次 W4 必同步调 `RunTracker.AttachCheckpoint(runID, cpID)`** |

### 7.3 4 个业务元数据写入 + 1 个恢复触发

| # | 触发点 | 写入函数 |
|---|--------|---------|
| **B1** | Canvas run 启动 | `RunTracker.Start(runID, canvasID, tenantID, parentRunID)` |
| **B2** | Run 正常完成 | `RunTracker.MarkSucceeded(runID)` |
| **B3** | Run 失败 | `RunTracker.MarkFailed(runID, err.Error())` |
| **B4** | Run 被 cancel | `RunTracker.MarkCancelled(runID)` |
| **R1** | HTTP `POST /run?resume_from=run_xxx` | handler: `HGetAll("agent:run:run_xxx")` → `checkpoint_id` → `WithCheckPointID(cpID)` + `WithWriteToCheckPointID(newCP)` + `RunTracker.Start(newRunID, canvas, tenant, "run_xxx")` |

### 7.4 Serializer 签名修正

eino `compose.Serializer` 实际签名（`compose/checkpoint.go:53-56`）**不带 `context.Context`**：
```go
type Serializer interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}
```

**CanvasStateSerializer**（`internal/agent/canvas/state_serializer.go`）：
```go
type CanvasStateSerializer struct{}
func (CanvasStateSerializer) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (CanvasStateSerializer) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
```

### 7.5 Cancel 协议（两段式）

**为什么两段式**：eino `compose.WithGraphInterrupt` 返回的 `interrupt` 是 **Go 函数引用**，仅在**同进程内**可调。Admin/UI 在另一个 HTTP handler 里发取消信号，必须经跨进程通道——这正是 Python 端 Redis `{task_id}-cancel` 协议要解决的。两者协同，不替代。

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

**Python 兼容**：`{task_id}-cancel` Redis key 命名与 Python 端 task_service.py 协议**完全一致**——同进程 + 跨进程 cancel 都能识别。

**轮询 vs Pub/Sub 决策**：默认 500ms 轮询（p99 ≤ 500ms）；Pub/Sub < 10ms 但与 Python 协议不兼容。Phase 2 视用户反馈切 Pub/Sub 双通道（轮询保兼容 + Pub/Sub 提速），由 `feature/cancel-pubsub` flag 控制。

---

## 8. OpenTelemetry 可观测性 / Observability

### 8.1 总体设计

```
Canvas run goroutine (Go)
   ↓
eino Graph Engine
   ↓ (OnStart / OnEnd / OnError auto-injected)
callbacks.Handler (业务实现)
   ├─ OTelHandler (本计划新增)
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
| **OTel logs**（Phase 8+） | 结构化日志 | OTLP | 运维/排障 |

### 8.3 eino callback → OTel 映射

| eino 时机 | OTel 行为 | Span attribute |
|-----------|-----------|----------------|
| `OnStart(ctx, info, input)` | `tracer.Start(ctx, info.Name)` → 写入 `ctx` | `eino.component.name`, `eino.component.type`, `eino.input.size` |
| `OnEnd(ctx, info, output)` | `span.End()` | `eino.output.size` |
| `OnError(ctx, info, err)` | `span.RecordError(err)` + `span.SetStatus(codes.Error, ...)` | `eino.error.message` |
| `OnStartWithStreamInput` | 同 OnStart，span event `eino.stream.input.start` | `eino.stream.input.size` |
| `OnEndWithStreamOutput` | `span.End()`，span event `eino.stream.output.end` | `eino.stream.output.size` |

**耗时计算**：`OnStart` 时 `startTime := time.Now()` 写入 `ctx`（参考 eino `callbacks/doc.go:99-102` 范式），`OnEnd` 时 `span.SetDuration(time.Since(startTime))`。

**Node name 来源**：`RunInfo.Name` 来自 `compose.WithNodeName(name)`；Canvas DSL 加载时给每个 cpn 设置节点名为 `cpn_id` → span 名 = `cpn_id`。

### 8.4 启动配置

```bash
# 必选（未设置 → no-op handler，不影响业务）
export OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318"
export OTEL_SERVICE_NAME="ragflow-agent"
export OTEL_RESOURCE_ATTRIBUTES="service.namespace=ragflow,deployment.environment=production"
export OTEL_TRACES_SAMPLER="parentbased_traceidratio"
export OTEL_TRACES_SAMPLER_ARG="0.1"  # 10% 采样
```

**降级**：未配置 `OTEL_EXPORTER_OTLP_ENDPOINT` → handler 退化为 noop（`otel.SetTracerProvider(noop.NewTracerProvider())`），**不报错**、不影响业务；OTel collector 不可达 → batch processor 内部 retry + drop（`OTEL_BSP_EXPORT_TIMEOUT` 默认 30s），handler 永不阻塞 run。

### 8.5 跨语言追踪

- Go → deepdoc Python HTTP 调用：用 `otelhttp.NewTransport(...)` 包裹 HTTP client，W3C `traceparent` header 透传
- Python RAGFlow OTel（通过 langfuse SDK 间接实现）：与 Go 端 OTLP 互通（同一 OTel collector，同一 `service.namespace=ragflow`）
- 关联规则：每次 canvas run 生成 `trace_id = run_id`；下发给 deepdoc / Python 的请求带 `traceparent` header

### 8.6 与 §2.10 v1 方案对比

| 维度 | v1（弃用） | v2（采用） |
|------|-----------|-----------|
| 存储 | MySQL `agent_run_log` 自管表 | 外部 OTel collector（无新表） |
| 实时推送 | Redis Stream XREAD consumer | OTel OTLP HTTP → collector |
| 跨语言 | ❌ 独立 MySQL 表 | ✅ OTLP 业界标准 |
| 与 Langfuse | ❌ 各自为政 | ✅ 同一 OTel pipeline |
| 启动轻 | 需建表 + 索引 + 归档策略 | 仅环境变量 |
| Python 端对齐 | 偏离 | 对齐（langfuse OTel） |

### 8.7 Python↔Go OTel 互通验证

**目的**：Go canvas（eino + OTLP/HTTP）和 Python canvas（langfuse SDK，OTel-bridged）出现在同一 `service.namespace=ragflow` 标签下，Jaeger/Langfuse 可跨语言追踪。

**通过标准**（6 条，缺一不可）：
1. Collector 在 5 分钟内同时收到 Python 和 Go 的 trace
2. 双方 span 携带 `service.namespace=ragflow` resource attribute
3. Jaeger 单一 `service.namespace=ragflow` filter 返回双方 trace
4. Langfuse 同 project 下显示两条独立 trace
5. Go span 遵循 OTel semantic conventions（`eino.component.name`, `eino.component.type`）
6. Python span 附带 `langfuse.*` namespace

**关键 env var**：

| Var | 用途 | 值示例 |
|-----|------|--------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector 地址 | `http://otel-collector:4318` |
| `OTEL_SERVICE_NAME` | Go service name | `ragflow-agent` |
| `OTEL_RESOURCE_ATTRIBUTES` | 必须含 `service.namespace=ragflow` | `service.namespace=ragflow,deployment.environment=prod` |
| `OTEL_TRACES_SAMPLER` | 采样策略 | `parentbased_traceidratio` |

**collector 兜底**：`resource/propagate` processor 对缺失 `service.namespace` 的 span 自动插入 `ragflow`，确保 Jaeger filter 始终可分组。

**常见失败**：

| 症状 | 原因 | 修复 |
|------|------|------|
| Collector 收到 0 span | 防火墙/端口错 | `curl -X POST http://localhost:4318/v1/traces` |
| `service.namespace` 为空 | env var 未传给子进程 | 在父 shell 设并 re-export |
| Go span 缺失 | `OTEL_EXPORTER_OTLP_ENDPOINT` 未设 | Go SDK 未设时 no-op |
| Python span 不在 Jaeger | langfuse SDK 只发自己后端 | 设 `OTEL_EXPORTER_OTLP_ENDPOINT`（langfuse ≥ 2.x 尊重 OTLP env var） |

---

## 9. 多版本 Agent 管理 / Multi-version Agents

**Go 端支持多版本并存**（**永不覆盖**），与 Python v1 "每次发布覆盖写 `user_canvas.dsl`" 行为不同。

**Schema 现状**（MySQL）：

- `user_canvas.id` 32 字符 UUID
- `user_canvas.dsl` 当前"草稿"或"最新已发布"
- `user_canvas.release` bool
- `user_canvas_version.id` 32 字符 UUID（**每版本一个，永不更新**）
- `user_canvas_version.user_canvas_id` 外键关联
- `user_canvas_version.dsl` 完整 DSL 快照
- 索引：`user_canvas_version(user_canvas_id)`

| 场景 | 行为 |
|------|------|
| 编辑器保存草稿 | `UPDATE user_canvas SET dsl=? WHERE id=?`（**不创建 version**） |
| 点击"发布" | `INSERT user_canvas_version(...)` 新行；`UPDATE user_canvas SET release=true, dsl=?, update_at=NOW()` |
| Run 不带 version | 拉取**最新** `user_canvas_version`（`create_time DESC LIMIT 1`） |
| Run `?version=v_xxx` | 拉取**指定** `user_canvas_version` |
| Run `?version=draft` | 拉取 `user_canvas.dsl`（编辑器未发布状态） |
| 删除版本 | `DELETE FROM user_canvas_version WHERE id=?`（**不影响其他版本**） |
| 删除整个 agent | 级联删除所有 version |

**保留策略**：

- **不自动删除旧版本**——由用户/管理员显式删除
- **不限制版本数**——业务表空间不是瓶颈
- **可选** `agents_max_versions` 配置（默认不启用）

**API 端**：

- `GET /api/v1/agents/{id}/versions` — 列表
- `POST /api/v1/agents/{id}/versions` — 显式发布
- `DELETE /api/v1/agents/{id}/versions/{version_id}` — 删除
- `GET /api/v1/agents/{id}/versions/{version_id}` — 详情
- `POST /api/v1/agents/{id}/run?version=xxx` — 指定版本运行（缺省=最新）

**与 Python 兼容**：`user_canvas.dsl` 保留（草稿/最新已发布副本），前端老接口仍能读；Go 端新发布永远插入新行，**不破坏** Python 老数据。

---

## 10. 第三方库选型 / Third-party Libraries (License Gate)

### 10.1 决策结论

| 用途 | 选 | License | 备注 |
|------|-----|---------|------|
| **PDF 生成** | `signintech/gopdf` | MIT | 主选；TTF 字体注册 + CJK + header/footer 内置 |
| **PDF 备选** | `go-pdf/fpdf` (codeberg.org fork) | MIT | GitHub 主仓库 2025-03-04 archive |
| ~~PDF unipdf~~ | ~~`unidoc/unipdf`~~ | ~~AGPL-3 + 商业~~ | ❌ 排除（强传染） |
| **DOCX 生成** | **自实现** OOXML writer | — | Go `archive/zip` stdlib + `text/template` + `//go:embed` |
| ~~DOCX unioffice~~ | ~~`unidoc/unioffice`~~ | ~~AGPL-3 + 商业~~ | ❌ 排除（强传染） |
| ~~DOCX fumiama-go-docx~~ | ~~`fumiama/go-docx`~~ | ~~AGPL-3~~ | ❌ 排除（强传染） |
| **Excel 读写** | `xuri/excelize/v2` | BSD-3 | 无 license 风险，标准选择 |
| **Markdown 解析** | `yuin/goldmark` | MIT | CommonMark 标准 |
| **HTML 解析** | `golang.org/x/net/html` | BSD-3 | stdlib 旁路 |
| **OpenTelemetry SDK** | `go.opentelemetry.io/otel` v1.44.0 | Apache-2.0 | 含 sdk + otlptrace/otlptracehttp + semconv |
| **MySQL driver** | `go-sql-driver/mysql` | MPL-2.0 | ExeSQL 走 stdlib `database/sql` |
| **PG driver** | `lib/pq` | MIT | ExeSQL 走 stdlib `database/sql` |
| **MSSQL driver** | `denisenkom/go-mssqldb` | BSD-3 | ExeSQL 走 stdlib `database/sql` |
| **HTTP retry** | 自实现指数 backoff | — | 17+ HTTP tool 共用 helper |
| **Test SQL mock** | `DATA-DOG/go-sqlmock` | MIT | ExeSQL 注入测试 |

### 10.2 关键论证

**AGPL-3 零容忍**：RAGFlow 是 Apache-2.0；AGPL-3 强传染会让整个 RAGFlow Go 二进制被迫 AGPL-3 化。所有候选 AGPL-3 库（unipdf / unioffice / fumiama-go-docx / baliance-gooxml）**全部排除**。

**DOCX 必须自实现**（穷举结果）：

- AGPL-3 阵营：unioffice（商业双轨）、fumiama/go-docx（活跃但传染）、baliance/gooxml（停滞+传染）
- MIT/Apache 阵营：tealeg（停滞）、lytdev（功能不完整）、legion-zver（license 不明）

**自实现可行性**：
- DOCX = ZIP 容器 + XML parts（`document.xml` / `header*.xml` / `footer*.xml` / `styles.xml` / `[Content_Types].xml` / `_rels/*.rels`）
- Go `archive/zip` stdlib 即可生成容器
- **不采用 `encoding/xml` 1:1 struct 映射**（OOXML 元素数 ≈ 500+，会暴涨到 5K+ LoC）—— **采用 `//go:embed` 静态基线 + `text/template` 动态渲染 混合模式**：
  - 固定部分（`[Content_Types].xml` / `_rels/.rels`）→ `//go:embed` `const []byte`
  - 动态部分（`document.xml` / `header1.xml` / `footer1.xml` / `styles.xml`）→ `text/template`
  - `funcMap["xml"]` 走 `template.HTML` + `escapeXMLAttr`（避免用户内容 `&`/`<`/`>` 破坏 XML 拓扑）
  - **代码量** ≈ 350 行核心 + 200 行模板 = 550 行（比"1.5K LoC struct 映射"压缩 2.7×）

**对比 Python 端的 pypandoc + xelatex 方案**：
- 优势：避免外部 binary 依赖（pandoc + TeX Live ≈ 800MB 镜像膨胀）
- 代价：自实现 1.5K LoC → 0.55K LoC（实际）

**Golden Master 快照测试**（防 XML 拓扑回归）：

- 10+ 个标准用例：minimal / full（含 watermark + page#）/ cjk / nested_table / list_numbering / heading_levels / page_break / section_break / multi_header / long_text / special_chars / empty_doc
- 生成 DOCX → `unzip` → pretty-print → `cmp.Diff` 与 `testdata/golden_*.xml` 对比
- `UPDATE_GOLDEN=1` 触发 golden 重写
- Word 兼容性手动验证（LibreOffice headless 打开无"文件已损坏"提示，列入完工 checklist）

### 10.3 完整 License 审计（14 候选库）

> 审计时间：Phase 0。规则：AGPL-3 / SSPL / Commons Clause / BUSL → **一律拒绝**（强传染，与 Apache-2.0 不兼容）。

| # | Library | License | Decision | Justification |
|---|---------|---------|----------|---------------|
| 1 | `unidoc/unipdf` | AGPL-3.0 | ❌ DENIED | AGPL-3 §13 viral |
| 2 | `unidoc/unioffice` | AGPL-3.0 | ❌ DENIED | 同上 |
| 3 | `fumiama/go-docx` | MIT | ❌ 实际未采用 | 自实现 OOXML 替代 |
| 4 | `baliance/gooxml` | AGPL-3.0 | ❌ DENIED | AGPL-3 dual-licensed 仍是 AGPL-3 |
| 5 | `tealeg/golang-docx` | BSD-3 | ⚠️ CONDITIONAL | 停滞；未采用 |
| 6 | `legion-zver/go-docx-templates` | AGPL-3.0 | ❌ DENIED | AGPL-3 |
| 7 | `lytdev/go-docxlib` | AGPL-3.0 | ❌ DENIED | AGPL-3 + 低活跃度 |
| 8 | `signintech/gopdf` | MIT | ✅ APPROVED | PDF 主选 |
| 9 | `go-pdf/fpdf` | MIT | ✅ APPROVED | PDF 备选（替代已 archive 的 `gofpdf`） |
| 10 | `jung-kurt/gofpdf` | MIT (archived) | ❌ DENIED | 上游已 archive，无安全补丁 |
| 11 | `pdfcpu/pdfcpu` | Apache-2.0 | ✅ APPROVED | PDF read/inspect/merge |
| 12 | `ledongthuc/pdf` | BSD-2 | ⚠️ CONDITIONAL | 优先用 `pdfcpu` |
| 13 | `xuri/excelize/v2` | BSD-3 | ✅ APPROVED | Excel 主选，Go 生态事实标准 |
| 14 | `yuin/goldmark` | MIT | ✅ APPROVED | Markdown→HTML |

**AGPL-3 预筛规则**（用于未来新增依赖）：
- README header 含 "AGPL" 或 "Affero" → 直接拒绝
- LICENSE 文件首行含 "Affero General Public License" → 拒绝
- GitHub license badge 显示 AGPL-3.0 / SSPL-1.0 → 拒绝
- CI 中 `go-licenses check` 命中 AGPL → 构建失败

**Re-verification 触发条件**：上游改 license、新 major version 重许可、依赖 archive、新 CVE 无补丁。

---

## 11. HTTP 接口 / HTTP API

| Method | Path | Handler | 说明 |
|--------|------|---------|------|
| `GET`  | `/api/v1/agents` | `ListAgents` | 已存在（commit `0a7662cf3`） |
| `POST` | `/api/v1/agents` | `CreateAgent` | 新增 |
| `GET`  | `/api/v1/agents/{id}` | `GetAgent` | 自动 v1/v2 转换；返回草稿 DSL |
| `PATCH`| `/api/v1/agents/{id}` | `UpdateAgent` | 更新草稿，**不创建版本** |
| `DELETE`| `/api/v1/agents/{id}` | `DeleteAgent` | 级联删除所有 version |
| `POST` | `/api/v1/agents/{id}/run` | `RunAgent` | 同步；`?version=v_xxx` 缺省=最新，`?version=draft`=草稿 |
| `POST` | `/api/v1/agents/{id}/stream` | `StreamAgent` | SSE；`?version=` 同上 |
| `POST` | `/api/v1/agents/{id}/cancel` | `CancelAgent` | 写 Redis cancel key |
| `GET`  | `/api/v1/agents/{id}/versions` | `ListVersions` | 列出版本列表 |
| `POST` | `/api/v1/agents/{id}/versions` | `PublishVersion` | 发布新版本，**永不覆盖** |
| `GET`  | `/api/v1/agents/{id}/versions/{vid}` | `GetVersion` | 版本详情 |
| `DELETE`| `/api/v1/agents/{id}/versions/{vid}` | `DeleteVersion` | 删除指定版本 |

**SSE 事件 payload**（与 Python `agent_api.py` 一致）：
```json
{"event": "node_start"|"node_finish"|"message"|"error", "task_id": "...", "component": "cpn_id", "data": {...}}
```

---

## 12. 验收标准 / Acceptance Criteria

| 类别 | 标准 |
|------|------|
| **功能** | 19 component × ≥3 单测 = ≥57 个 component 单测；21 tool × ≥2 单测 = ≥42 个 tool 单测 |
| **eino 复用** | T1 组件（LLM/Agent）回归：跑 eino 自带 `react_test.go` / `chatmodel_test.go` / `compose_test.go` 不退化 |
| **功能** | 100 条 v1 DSL 样本 → v2 → 调度执行，结果与 Python 端一致 |
| **功能** | `{{cpn_id@param}}` 任意节点读任意节点、`globals` 读写、`sys.x` / `env.x` 解析，单测覆盖 |
| **功能** | SSE 事件序列与 Python `agent_api.py` 一致：node_start / node_finish / message / error |
| **并发** | 100 并发 canvas run，单租户 P99 启动延迟 < 200ms（不含组件执行） |
| **并发** | 调度器 overhead：100 节点 DAG 调度 < 50ms |
| **并发（State mutex 硬门）** | `BenchmarkStateMutex` 在 100 节点 / 1000 并发 `ns/op < 500µs`（不通过禁止进 Phase 2，fallback 走分片 RWMutex） |
| **可靠** | Redis 取消协议：cancel → 5s 内节点 stop（500ms 轮询下 p99 ≤ 500ms） |
| **可靠** | 流式中断（client disconnect）→ 节点 30s 内退出 |
| **兼容** | v1 DSL 零修改加载成功（≥99% 样本）；失败样本产出明确错误 |
| **兼容** | v2 → v1 写出后旧 Python reader 仍能加载 |
| **可观测性** | OTel handler P99 overhead < 2%（100 节点）；未配置 endpoint 时 no-op，P99 启动延迟变化 < 1ms |
| **checkpoint** | Redis `RedisCheckPointStore` Get/Set/Delete 通过 eino 集成测试；cancel 后 resume_from 链路无重复执行已通过节点 |
| **checkpoint** | 30 天 TTL 由 Redis `EXPIRE` 原生保证 |
| **代码质量** | 公共 API 100% godoc 注释（golangci-lint revive 强制）；复杂算法/状态机/并发原语 100% 注释（karpathy 原则）；`>=80% test coverage on internal/agent/canvas` |

---

## 13. 风险 & 缓解 / Risks

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| **eino State 在高并发下 mutex 竞争** | 中 | Phase 1 末 benchmark；若 > 5% 调度开销，引入分片 mutex（按 `cpn_id` hash，N = `min(NumCPU*4, 64)`） |
| **v1 DSL 100% 兼容不可能**（Python 装饰字段） | 中 | 不兼容的旧 DSL 走"自动转换 + 提示"路径，不静默丢字段 |
| **Component 接口签名与 Python 偏离** | 中 | 签名一致 → 转换代码 1:1 复刻 → 行为一致 |
| **Tool 外部 HTTP 失败** | 中 | 复用 `http_helper.go` 的 retry；mock 测试覆盖 5xx / timeout / DNS |
| **Python task_executor 协议不同步** | 低 | `internal/proto/ingestion.proto` 已废弃；Python task_executor 注册/心跳仍走 Redis |
| **前端 DSL 编辑器只懂 v1** | 中 | Phase 5 维持 v1 写出能力；前端 v2 编辑器作为独立项目排期 |
| **测试环境无 LLM key** | 低 | 所有 LLM 组件测试走 mock provider driver（`internal/entity/models/dummy.go` 范式） |
| **deepdoc 仍 Python 导致跨语言追踪** | 中 | 跨语言 deepdoc 调用走 HTTP；tracing 通过 OpenTelemetry propagator 串联 |

---

## 14. 计划 vs 现状 对比 / Plan vs Reality

This section captures the deviations between the original plans and the code as it stands on 2026-06-11.

### 14.1 Component 数量：计划 22 → 21 → **实际 19**

| 计划来源 | 描述 | 实际 |
|---------|------|------|
| §2.11.3 row 11-13 | `Iteration` / `IterationItem` / `Loop` / `LoopItem` = 4 独立 component | `Loop` 1 个（`component/loop.go`），其余 3 **未注册 component**——通过 `canvas/loop_subgraph.go` 宏展开吸收为 `Loop` 单节点的子图 |
| §2.11.3 row 13 | `ExitLoop` no-op component | **未注册 component**——`legacyNoOpNames` 在 canvas 层吸收（DSL v1 compat） |
| §2.11.3 row 8 | `Agent` 走 T1，自建 citation 中间件 + tool artifact 收集 | `Agent` 已实现（T1 + `react.NewAgent` + 22 tool 注册），**citation 中间件和 tool artifact 收集未实现**（见 §14.4） |

实际 `.go` 文件清单（19 个 component .go）：

```
agent.go, begin.go, browser.go, categorize.go, data_operations.go,
docs_generator.go, excel_processor.go, fillup.go, invoke.go,
list_operations.go, llm.go, loop.go, message.go, parallel.go,
string_transform.go, switch.go, userfillup.go, variable_aggregator.go,
variable_assigner.go
```

加上 5 个 helpers：`base.go, registry.go, runtime_wire.go, io_init.go, v1_stubs.go`。

### 14.2 T5 路径：计划 `component/io/` 子目录 → 实际 根目录

| 计划来源 | 描述 | 实际 |
|---------|------|------|
| §4.1 目录树 | `internal/agent/component/io/{docs_generator.go, excel_processor.go, docx_writer.go, pdf_writer.go, md_ast.go, ...}` | `docs_generator.go` / `excel_processor.go` 在 `internal/agent/component/` 根目录；`docx_writer.go` / `pdf_writer.go` / `md_ast.go` **未单独拆出**（可能内联在 docs_generator.go 内） |
| §2.11.5.3 | `docx_writer.go` ≈ 350 行核心 + 5 个 .tmpl | 自实现 OOXML writer 存在，模板/文件结构需进一步验证 |

### 14.3 双写 vs OpenTelemetry：已完全切换

`agent-go-port.md §2.10` 早期版本是 "Redis Stream + MySQL 双写"，2026-06-03 决策切换为 OTel。当前代码 `internal/observability/otel/` 三件套（provider.go / handler.go / handler_test.go）已落地；MySQL `agent_run_log` 表**未创建**。

### 14.4 Agent 组件 1 个 P0 缺口

> **✅ 2026-06-11 闭环**（commit pending）：两个中间件已落地，详见 `component/agent.go` 的 `toolArtifactCapture` / `maybeAppendCitation`。

`component/agent.go` 走 T1（`react.NewAgent` + 22 tool 注册）。plan §2.11.6 D2 提到的两个**自建中间件**当前实现：

- **Tool artifact 收集**：eino `ToolCallbackHandler` 挂在 `react.NewAgent(... compose.WithCallbacks(cb))` 上。`OnStart` 捕获 `ArgumentsInJSON`，`OnEnd` 捕获 `CallbackOutput.Response`。capture 通过 `context.WithValue` 传递（`toolArtifactKey`），`AgentComponent.Invoke` 入口安装，runner 内 callback 写入，runner 出口读取——**runner 签名不变**（test seam `withAgentRunner` 仍能 seed artifacts）
- **Citation 中间件**：`maybeAppendCitation(ctx, chatModel, msg)` 在 ReAct 结束后调，逻辑：
  1. `runtime.GetStateFromContext[*CanvasState](ctx)` 拿 state；无 state → no-op
  2. `state.Retrieval["chunks"]` 为空/nil/空 slice → no-op（**避免无谓 LLM 调用**）
  3. 否则用 `chatCompleter.Generate(...)` 发一次 follow-up LLM call，prompt 模板让模型在原文基础上加 `[n]` 引用标记
  4. 失败/no-op 路径都保持 `msg.Content` 不变（best-effort polish）
- `AgentOutput.Artifacts` 字段在 `component/agent.go:51` 之前**始终返回空 slice**（`"artifacts": []map[string]any{}`），现在通过 `artifactsToMaps(readToolArtifacts(ctx))` 填入真实内容。

**测试覆盖**（`agent_test.go`）：
- `TestAgent_ReadsArtifactsFromContext` — 验证 test seam 能 seed capture，Invoke 输出含 2 个 artifact（一个 OnStart args + 一个 OnEnd response）
- `TestAgent_ArtifactsEmptyWhenRunnerSeedsNothing` — 验证未 seed 时返回空 slice 而非 nil（schema 稳定）
- `TestAgent_MaybeAppendCitation_NoState` — 无 state → LLM 不被调
- `TestAgent_MaybeAppendCitation_EmptyChunks` — 空 chunks → LLM 不被调（避免浪费）
- `TestAgent_MaybeAppendCitation_AppendsTail` — 正常路径：content 拼接为 `original + "\n\n" + cited`

### 14.5 ExeSQL 决策已按 2026-06-11 review 落地

`agent-go-port.md` 2026-06-11 changelog 记录 ExeSQL 走 stdlib `database/sql` + 各 driver，**不复用** `internal/dao` GORM。当前 `component/tool/exesql.go` 实际采用此方案（`exesqlDriverAndDSN` 集中拼装 + `exesqlDialer` 注入 + `DATA-DOG/go-sqlmock` 测试）。✅

### 14.6 workflowx 扩展：已完全实现

`eino-workflow-loop.md` 和 `eino-workflow-parallel.md` 描述的 `AddLoopNode[T]` / `AddParallelNode[I,O]` 已在 `internal/agent/workflowx/` 落地，配套 `loop_test.go` / `loop_integration_test.go` / `parallel_test.go` / `parallel_integration_test.go`（**含 miniredis-style 内存 checkpoint store 模拟真实 eino 集成路径**）。

### 14.7 runtime 包：已从 canvas/component 双侧提取

`fluffy-strolling-bear.md` 描述的"提取共享运行时契约到 `internal/agent/runtime/`"已落地：`component.go` / `context.go` / `metrics.go` / `selector.go` / `state.go` / `template.go` 6 个文件。`canvas/state_export.go` 保留薄 alias 供测试用，生产代码不依赖。✅

### 14.8 开放问题 / Open Questions

| ID | 问题 | 状态 |
|----|------|------|
| Q1 | Retrieval + GraphRAG Go 化策略 | ✅ 已闭环（策略 A：Go Retrieval 外壳 + 进程内 Dealer 直调；`use_kg=True` 走配置错误返回） |
| Q2 | Checkpoint 持久化 | ✅ 已闭环（Redis 30d TTL 双 key） |
| Q3 | 跨语言调用策略 + 可观测性 | ✅ 已闭环（deepdoc 走 HTTP；OTel 集成） |
| Q4 | DSL v2 metadata（author/tags/created_at） | ✅ 已闭环（**不上 v2 schema**；元数据走 `user_canvas.title/description` 等后端字段） |
| Q5 | Tenant LLM 默认模型注入 | ✅ 已闭环（`service.ModelProviderService.GetChatModel` + `entity/models.NewChatModel` + eino `model.ChatModel`） |
| Q6 | Streaming WebSocket 支持 | ⏸️ **pending demand**——目前仅 SSE；无用户/产品需求触发前不实现 |
| Q7 | Component 热重载 | ✅ 已闭环（不支持；沿用 Python v1 行为） |
| Q8 | Retrieval 工具 Go 化 | ✅ 已闭环（策略 A，0 gRPC） |
| Q9 | v1.1 cgo 嵌入 CPython 调 KGSearch | ⏸️ 暂不做 |
| Q11 | T5 cgo 绑定 | ✅ 已闭环（不引入 cgo；纯 Go lib / 自实现） |

### 14.9 计划 Phase 与代码落地对照

| Phase | 计划范围 | 落地状态 |
|-------|---------|---------|
| Phase 0 — 准备（接口清单、license-gate、deepdoc 端点调研） | 1 周 | ✅ 全部产出（`docs/agent-port/*.md` × 5） |
| Phase 0.5 — Deepdoc Client 类型契约 | 0.5 天 | ✅ `internal/deepdoc/{client,dla,ocr,tsr}.go` + 24 单测（HTTP/multipart/retry/4xx-5xx/ctx-cancel 全部覆盖） |
| Phase 1 — 画布骨架 | 2.5 周 | ✅ `canvas/{state, variable, scheduler, cancel, stream, checkpoint_store, run_tracker, state_serializer, compile}.go` 全部到位 |
| Phase 2 — Component 库 | 4.5-7 周 | ✅ 19 component + 5-tier 全部实现（P0-P4 混合交付） |
| Phase 2.5 — DSL v2 + v1→v2 | 1.5 周 | ✅ `internal/agent/dsl/{v2.go, loader.go, converter_v1_to_v2.go}` |
| Phase 3 — Tool 库 | 2.5-3.5 周 | ✅ 21 tool + `BuildAll`/`BuildByName` registry |
| Phase 5 — HTTP/RPC | 1.5-2.5 周 | ✅ 12 endpoint + 3 version 端点 |
| Phase 5.5 — DSL v2 写兼容 | 1 周 | ✅ `converter_v2_to_v1.go` |
| Phase 6 — 灰度 | 1-2 周 | ❌ **未启动**——`tenant_canvas_runtime_mode` 配置表未实现；Python 端 `agent_api.py` 仍为主路径 |
| Phase 7 — 清理 | 1 周 | ❌ **未启动**——Python 端未标 `@deprecated`；`docs/go-python-implementation-status.md` 第 314–316 行未更新为"已 Go 化" |

### 14.10 Phase 6 — Per-Tenant Runtime Selector（已交付基础设施建设）

**Go 侧已交付**：

| File | Purpose |
|------|---------|
| `internal/agent/runtime/selector.go` | 每租户 runtime 模式选择器，Redis 读 `tenant_canvas_runtime:{tenantID}`，fallback `RAGFLOW_CANVAS_DEFAULT_RUNTIME`（默认 `python`） |
| `internal/agent/runtime/metrics.go` | Prometheus counter `ragflow_canvas_runs_total{runtime,outcome}` + histogram `ragflow_canvas_run_duration_seconds{runtime}` |
| `internal/handler/admin_runtime.go` | `POST /api/v1/admin/canvas-runtime/:tenant_id` — 翻转租户 override |
| `internal/router/admin_routes.go` | `RegisterAdminRuntimeRoutes` helper |

**操作契约**：
- 默认行为：`RAGFLOW_CANVAS_DEFAULT_RUNTIME=python` → 所有租户走 Python
- 租户提升：`curl -X POST .../admin/canvas-runtime/tenant_42 -d '{"runtime":"go"}'`
- 回滚：同上，`{"runtime":"python"}`
- Override 存 Redis 无 TTL（永久有效，显式覆盖才变）

**Staging 灰度 run-book**：
1. 部署 Go Canvas 服务（不接用户流量）
2. 验证默认值 `python`；Go 服务 idle
3. 提升 100 个租户到 Go
4. 跑标准负载：1000 runs/tenant × 30 分钟
5. 观察：`rate(ragflow_canvas_runs_total{runtime="go"}[5m])` 与 Python rate 差 < 1%；p99 < 2s
6. 回滚演练：挑 1 租户切回 Python，< 5s p99
7. SLO 满足 24h → 进 Phase 7

**Phase 7 启动前置条件**（由 staging canary 验证）：
- 100 tenants × 1000 runs success-rate parity ≤ 1%
- p99 latency Go < 2s 持续 24h
- 回滚 drill p99 < 5s 持续 24h
- Admin endpoint auth gap 已关闭

### 14.11 Phase 7 — Python `agent_api.py` Deprecation（Go 侧已交付，Python 侧阻塞）

**Go 侧已交付**：
- Hybrid routing default 翻到 100% Go
- Per-tenant override 保留作回退窗口
- 状态文档更新为"已 Go 化"

**Python 侧待办**（Python 团队负责，Go 侧无权触碰）：
1. 给 `api/apps/agent_app.py` 加 `@deprecated` docstring + `DeprecationWarning`
2. 添加兼容代理 shim：`/api/v1/agents/*` → proxy 到 Go 服务（`RAGFLOW_GO_CANVAS_URL`），Go 不可达时 fallback Python
3. 删除时间线：Phase 7 发版 → 1 release（~3 月）后，若 0 active tenants 走 Python 持续 7 天 → 删除废弃模块

**安全删除验收门**（PromQL 查询 `ragflow_canvas_runs_total{runtime="python"}` 连续 7 天为 0；Redis `tenant_canvas_runtime:*` 无 `"python"` 值；无 Python canvas 路径 support ticket）

**回滚**：单租户 `POST .../admin/runtime/tenants/<id> -d '{"mode":"python"}'`；集群级回滚设 `RAGFLOW_CANVAS_DEFAULT_RUNTIME=python` 并重启 Go 服务。

---

## 15. 后续跟进 / Future Work

1. **DSL v3**：类型化表达式（编译期校验 `{{cpn_id@param}}`）
2. **eino 生态对齐**：`AddAgenticModelNode` 替换 LLM component；`AddRetrieverNode` 替换 Retrieval component
3. **GraphRAG component Go 化**（独立项目排期）
4. **WebSocket 流支持**（Q6，pending demand）
5. **Checkpoint 增强**：跨 canvas run 复用、增量 checkpoint（仅写 diff channel）
6. **Phase 6 灰度 + Phase 7 清理**：把 Python 端 agent_api.py 流量切到 Go
7. **如果产品/UI 需要画布级标签/作者**：在 `user_canvas` 表加 `tags` / `author_id` 列（**不**改 v2 DSL schema，参见 Q4 决策）

---

## 附录 A · 关键文件 / Key Files

按"修改这一处会触及的设计点"分组：

| 设计点 | 关键文件 |
|--------|---------|
| **State 模式** | `internal/agent/canvas/{state.go, scheduler.go}` + `internal/agent/runtime/{state.go, context.go}` |
| **runtime 提取** | `internal/agent/runtime/*.go`（6 文件） + `internal/agent/canvas/state_export.go` |
| **Loop 宏展开** | `internal/agent/canvas/loop_subgraph.go` + `internal/agent/component/loop.go`（no-op marker） |
| **Parallel** | `internal/agent/component/parallel.go` + `internal/agent/workflowx/parallel.go` |
| **Loop 通用节点** | `internal/agent/workflowx/loop.go` + `loop_{test,integration,options}_test.go` |
| **Checkpoint** | `internal/agent/canvas/{checkpoint_store.go, run_tracker.go, state_serializer.go, compile.go}` |
| **Cancel 协议** | `internal/agent/canvas/cancel.go` |
| **OTel** | `internal/observability/otel/{provider.go, handler.go, handler_test.go}` |
| **DSL v2** | `internal/agent/dsl/{v2.go, loader.go, converter_*.go}` |
| **Tool registry** | `internal/agent/tool/registry.go` + `http_helper.go` + `ssrf.go` |
| **Component 5-tier** | `internal/agent/component/{base.go, registry.go, runtime_wire.go}` + 19 component .go |

## 附录 B · 测试覆盖 / Test Coverage

| 包 | 测试文件数 | 覆盖点 |
|----|-----------|--------|
| `internal/agent/canvas` | 14 | `canvas_test.go, scheduler_test.go, state_test.go, variable_test.go, state_bench_test.go, state_serializer_test.go, checkpoint_store_test.go, run_tracker_test.go, cancel_test.go, stream_test.go, loop_subgraph_test.go, loop_semantics_test.go, dsl_examples_e2e_test.go, cycle_wrap_test.go` |
| `internal/agent/component` | 16+ | 各 component `_test.go` + `verify_p1_test.go`（批量回归） |
| `internal/agent/tool` | 21+ | 各 tool `_test.go` + `registry_test.go`（schema sweep + alias 一致性） |
| `internal/agent/runtime` | 2 | `metrics_test.go, selector_test.go` |
| `internal/agent/workflowx` | 8 | `loop_test.go, loop_options_test.go, loop_integration_test.go, loop_example_test.go, parallel_test.go, parallel_options_test.go, parallel_integration_test.go, parallel_helpers_test.go` |
| `internal/agent/dsl` | 4 | `loader_test.go, converter_v1_to_v2_test.go, converter_v2_to_v1_test.go, v1_examples_test.go` (42 个测试，含 12 个 v2→v1 + round-trip) |
| `internal/observability/otel` | 1 | `handler_test.go`（tracetest.SpanRecorder） |

---

## 附录 C · Deepdoc Service Endpoints (DLA/OCR/TSR)

> Phase 0 research deliverable. Documents the wire contract for the deepdoc vision stack (DLA remote HTTP, OCR/TSR local ONNX only).

### C.1 Endpoint summary

| Endpoint | URL | Status | Go port need |
|----------|-----|--------|--------------|
| DLA (Document Layout Analysis) | `POST {DEEPDOC_URL}/predict` | Remote HTTP (via `dla_cli.py`, fork only) | Go client with 3-retry + 18s timeout |
| OCR | **No remote endpoint** | Local ONNX only (`deepdoc/vision/ocr.py`) | None — `ErrNotImplemented` stub |
| TSR (Table Structure Recognition) | **No remote endpoint** | Local ONNX only | None — `ErrNotImplemented` stub |

Single toggle: `DEEPDOC_URL` (preferred) or `TENSORRT_DLA_SVR` (legacy). When unset, LayoutRecognizer loads local ONNX.

### C.2 DLA HTTP contract

- **Method**: `POST {DEEPDOC_URL}/predict`
- **Body**: `multipart/form-data`, field name `request`, raw JPEG bytes
- **Response**: `{"bboxes": [[left, top, right, bottom, score, type_idx], ...]}`
- **Timeout**: 18s per request; **3 retries** per image with `Session` rebuild
- **Failure sentinel**: empty list `[]` for that image

#### DLA class taxonomy (10 classes)

| idx | Class | idx | Class |
|----:|-------|----:|-------|
| 0 | title | 5 | Table |
| 1 | Text | 6 | Table caption |
| 2 | Reference | 7 | Table caption (dup) |
| 3 | Figure | 8 | Equation |
| 4 | Figure caption | 9 | Figure caption (dup) |

> Note duplicates at idx 4/6/7/9. Go port must use same array ordering and lowercase normalization — renumbering is a wire-format break.

### C.3 Go client placeholder (`internal/deepdoc/client.go`)

Phase 0 delivers typed Go client with no implementation beyond `ErrNotImplemented`. Phase 2 P3 fills in `DLA(ctx, images [][]byte) ([]DLAResult, error)`:
- Build multipart body with `mime/multipart`, field `request`, `Content-Type: image/jpeg`
- POST to `baseURL + "/predict"`
- Decode `{bboxes: [[l,t,r,b,score,ty], ...]}`, map `ty` through `DLA_CLASSES`
- 3-retry + 18s timeout with `http.Client.Timeout`
- Wrap transport with `otelhttp.NewTransport` for trace propagation

### C.4 Environment variables

```
DEEPDOC_URL          # preferred; full URL e.g. http://deepdoc:11234
TENSORRT_DLA_SVR     # legacy alias; honored as fallback
```

### C.5 LayoutRecognizer consumers

The single Python module calling into DLA HTTP is `deepdoc/vision/layout_recognizer.py`, consumed by:
- Resume parser (`rag/app/resume.py`)
- Table recognizer (`deepdoc/vision/t_recognizer.py`)

---

## 附录 D · DSL v1 Corner Cases Inventory

> Phase 0 deliverable. Canonical v1 DSL schema + 15 corner-case categories anchored on `agent/canvas.py:43-95` and `agent/component/base.py:368-369`.

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
- **`cpn_id@param.nested.path`**: dot-path traversal with `json.loads` on strings, `dict.get`, `list[int]` index, `getattr` fallback
- **`set_variable_value`**: auto-creates missing dict keys in the path
- **`functools.partial`**: unwrapped during variable resolution (message streaming)
- **Empty `{{...}}`**: resolves to `""`, never crashes
- **`is_reff`**: returns `True` only if `cpn_id@param` resolves to a known component; otherwise treats as literal

### D.3 `custom_header` injection

`custom_header` is a **per-run HTTP header dict**, NOT a stored DSL field. The loader injects it at `canvas.py:102` before `param.update()`. Go port must:
1. Strip `custom_header` from stored DSL on read
2. Pass via Canvas run context, NOT via `ComponentParamBase`
3. Surface to relevant tool/component via State

### D.4 Three-set parameter decoration (REMOVED in v2)

Python stores 4 internal keys per-param-instance: `_feeded_deprecated_params`, `_deprecated_params`, `_user_feeded_params`, `_is_raw_conf`. The Go port's DSL v2 **drops all 4** on v1→v2 conversion. Unknown keys are silently absorbed (permissive `update()`).

### D.5 `path` linearization & runtime mutation

`path` is mutated at runtime by: `begin` append on empty, iteration/loop/categorize/switch/exitloop extensions, `userfillup` reordering, `exception_goto` extension, node popping for out-of-order dependencies. Go scheduler must replicate same `path` semantics including `idx = to` truncation at batch end.

### D.6 `exception_goto`

`exception_goto` is a **list** of cpn_ids (usually length 1). Empty list = no-op. `exception_method` is one of `None` / `"comment"` / implicit `"goto"` (by presence of non-empty `exception_goto`). Once triggered, no further downstream extension (short-circuit).

### D.7 Nested messages / streaming

- `<think>`/`</think>` tokens → separate SSE events with `start_to_think`/`end_to_think` flags
- TTS audio batched at 16 chars
- After streaming completes, full concatenated string written to `set_output("content", ...)` for downstream `{{Message@content}}` references
- `partials` queue buffers components whose `content` is a partial until it drains

### D.8 `userfillup` interactive pause

Can appear in `path` multiple times. On re-entry, `begin` is NOT re-invoked. `enable_tips=True` produces a `tips` field rendered by frontend. Go port must reorder path so `userfillup` nodes come first on every re-entry.

### D.9 `globals` / `sys.*` / `env.*` semantics

6 default keys: `sys.query`, `sys.user_id`, `sys.conversation_turns`, `sys.files`, `sys.history`, `sys.date`. `sys.date` refreshed at every `run()`. `sys.conversation_turns` defensively coerces `None` → `0` then `+= 1`. `env.*` reset path falls back to type-based default (`number→0`, `boolean→false`, `string→""`, etc.). `sys.history` auto-appended on every assistant turn (duplicate store with `history` list).

### D.10 Component-name case-insensitivity

All comparisons use `.lower()`. Stored cpn_ids may be any case. Go port must NOT key component map by case-sensitive `cpn_id` — raw id for display, lowercase for internal lookups.

### D.11 Template samples

25 JSON templates in `agent/templates/` (~1.1 MB total) covering all 22 components. Key samples:
- `web_search_assistant.json` (~30K): Agent + Retrieval + Message, variable refs with whitespace
- `customer_feedback_dispatcher.json` (~34K): Categorize + Switch + Message
- `deep_research.json` (~144K, largest): heavy Iteration + Loop, ~30 component instances
- `data_analysis_beginner_assistant.json` (~22K): `exception_goto` with real cpn_ids
- `market_seo_article_writer.json` (~62K): DocsGenerator with PDF output, multiple Iterations

---

## 附录 E · Component & Tool Interface Inventory

> Phase 0 deliverable. 22 components + 21 tools with class hierarchy, public methods, input/output schemas, and key dependencies.

### E.1 Component inventory (22)

| # | Component | File | `component_name` | Tier | Key behavior |
|---|-----------|------|-----------------|------|-------------|
| 1 | Begin | `begin.py` | `Begin` | T3 | Consumes `kwargs["inputs"]`, resolves file inputs via `FileService.get_files` |
| 2 | UserFillUp | `fillup.py` | `UserFillUp` | T3 | Renders `tips` with variable interpolation, resolves file inputs |
| 3 | Fillup | (alias) | `Fillup` | T3 | Thin alias of UserFillUp (disable `enable_tips`) |
| 4 | Message | `message.py` | `Message` | T3 | Assembles final response: jinja2 prompt + stream + TTS + filegen + memory save |
| 5 | LLM | `llm.py` | `LLM` | T1 | Sync + async paths; `chatModel.Generate` / `Stream`; structured JSON output |
| 6 | Categorize | `categorize.py` | `Categorize` | T3 | LLM one-shot classification → `_next` (routing list) + `category_name` |
| 7 | Switch | `switch.py` | `Switch` | T2 | Evaluates boolean conditions; `_next` = matching downstream(s) |
| 8 | Agent | `agent_with_tools.py` | `Agent` | T1 | ReAct loop with `LLMBundle` + tool binding + citations |
| 9 | Iteration | `iteration.py` | `Iteration` | T4 | Resolves `items_ref`, validates array, drives `IterationItem` children |
| 10 | IterationItem | `iterationitem.py` | `IterationItem` | T4 | Round-local outputs aggregated by parent |
| 11 | Loop | `loop.py` | `Loop` | T4 | Initializes `loop_variables`, drives `LoopItem` children |
| 12 | LoopItem | `loopitem.py` | `LoopItem` | T4 | Evaluates `loop_condition`; `end()` → `True` triggers exit |
| 13 | ExitLoop | `exit_loop.py` | `ExitLoop` | T1 (Passthrough) | No-op; parent Loop extends path |
| 14 | Invoke | `invoke.py` | `Invoke` | T3 | HTTP GET/POST/PUT/PATCH/DELETE + headers/proxy/timeout/HTML cleanup |
| 15 | Browser | `browser.py` | `Browser` | T3 | LLM-driven browsing: page fetch, click, type, screenshot, MinIO upload |
| 16 | DataOperations | `data_operations.py` | `DataOperations` | T3 | 7 ops: select_keys/literal_eval/combine/filter/append_or_update/remove/rename |
| 17 | ListOperations | `list_operations.py` | `ListOperations` | T3 | 6 ops: nth/head/tail/filter/sort/drop_duplicates |
| 18 | StringTransform | `string_transform.py` | `StringTransform` | T3 | split/merge/jinja2 template ops |
| 19 | VariableAggregator | `variable_aggregator.py` | `VariableAggregator` | T3 | Returns first non-empty in each variable group |
| 20 | VariableAssigner | `variable_assigner.py` | `VariableAssigner` | T3 | 12 ops: overwrite/clear/set/append/extend/remove_first/last/`+=`/`-=`/`*=`/`//=` |
| 21 | DocsGenerator | `docs_generator.py` | `DocGenerator` | T5 | MD → PDF/DOCX/TXT/MD/HTML; header/footer/watermark/page# |
| 22 | ExcelProcessor | `excel_processor.py` | `ExcelProcessor` | T5 | Excel read/write/merge/convert via `pandas` + `openpyxl` |

### E.2 Tool inventory (21)

All tools extend `ToolBase` (`agent/tools/base.py:141`), expose `get_meta()` (OpenAI function-call schema), `_invoke`/`_invoke_async`, and `thoughts()`.

| # | Tool | `component_name` | Behavior |
|---|------|-----------------|----------|
| 1 | AkShare | `AkShare` | Chinese financial data (HTTP) |
| 2 | ArXiv | `ArXiv` | `export.arxiv.org/api/query` search |
| 3 | CodeExec | `CodeExec` | gRPC client to Python sandbox (kept as-is) |
| 4 | Crawler | `Crawler` | Generic HTML scraper (httpx + selectolax/BeautifulSoup) |
| 5 | DeepL | `DeepL` | DeepL Translate API (HTTP) |
| 6 | DuckDuckGo | `DuckDuckGo` | `html.duckduckgo.com/html` search |
| 7 | Email | `Email` | SMTP send via `smtplib` |
| 8 | ExeSQL | `ExeSQL` | MySQL/PG/MSSQL query via `database/sql` |
| 9 | GitHub | `GitHub` | GitHub REST API search |
| 10 | Google | `Google` | SerpAPI / Google CSE search |
| 11 | GoogleScholar | `GoogleScholar` | Scholar via SerpAPI |
| 12 | Jin10 | `Jin10` | Chinese financial news feed (HTTP) |
| 13 | PubMed | `PubMed` | NCBI E-utilities |
| 14 | QWeather | `QWeather` | HeFeng weather API |
| 15 | Retrieval | `Retrieval` | Dealer backend (Go-ized, in-process call) |
| 16 | SearXNG | `SearXNG` | Meta-search |
| 17 | TavilySearch | `TavilySearch` | Tavily search API |
| 18 | TavilyExtract | `TavilyExtract` | Tavily extract API |
| 19 | TuShare | `TuShare` | Tushare Chinese financial data |
| 20 | WenCai | `WenCai` | 同花顺 问财 stock Q&A |
| 21 | Wikipedia | `Wikipedia` | Wikipedia REST API |
| 22 | YahooFinance | `YahooFinance` | Yahoo Finance unofficial API |

### E.3 ComponentBase cross-cutting surface

Every `Component` exposes 18 methods: `invoke`/`invoke_async`/`_invoke`/`output`/`set_output`/`error`/`reset`/`get_input`/`get_input_values`/`get_input_elements_from_text`/`get_input_elements`/`set_input_value`/`get_input_value`/`get_param`/`get_upstream`/`get_downstream`/`get_parent`/`is_canceled`/`check_if_canceled`/`exception_handler`/`thoughts`.

### E.4 ToolBase cross-cutting surface

`ToolParamBase(ComponentParamBase)` wraps `inputs` from `meta["parameters"]`; `get_meta()` returns OpenAI function-call schema. `ToolBase(ComponentBase)` wraps `_invoke`/`_invoke_async` in `check_if_canceled` + records `_ERROR` + `_elapsed_time`. `LLMToolPluginCallSession` dispatches `tool_call_async(name, args)` to the right tool (or `MCPToolBinding`/`MCPToolCallSession`).
