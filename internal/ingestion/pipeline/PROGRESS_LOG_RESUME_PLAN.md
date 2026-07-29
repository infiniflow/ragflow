# Ingestion Pipeline：Progress / Log / Resume 机制计划书

> 范围：`internal/ingestion/pipeline` 的执行可观测性与断点续跑。
> 关联代码：`internal/agent/canvas/*`（框架闸口）、`internal/agent/runtime/*`（progress 回调）、`internal/ingestion/*`（ingestion 专属逻辑）。
> 状态：设计稿 v2（v1 经评审修订，修订记录见文末 §10）。结论：**续跑走 B（eino interrupt/checkpoint），观测走轻量 task_log（不存全量 output）**。

---

## 1. 背景与目标

ingestion pipeline 是把一份 DSL 编译成 eino 图后完整执行（`Pipeline.Run` → `compiled.Workflow.Invoke`）。需要三件事：

1. **Progress（进度）**：框架层对每个 component 的发令（进入/退出/失败），供 UI/API 展示。
2. **Log（日志）**：把 progress 落库成可回读的 `ingestion_task_log`，API 侧 `IngestionTaskLogDAO.ListLogsByTaskID` 读取。
3. **Resume（续跑）**：重跑同一个 task（task_id / pipeline_id 不变）时，已成功执行过的 component 不必重做。

目标约束（来自 `node_body.go:156` 的项目自身原则）：

> cross-cutting concerns belong in the framework body (`realComponentBody`), not inside each component's `Invoke`.

即 progress / resume 不应回到 `Parser` 等组件内部（否则每个组件都依赖 `dao`，且违反单一闸口原则）。

---

## 2. 现状（已落地的部分）

区分两件事：**功能是否完整**（能否在进程内 interrupt→resume）与**持久化是否到位**（崩溃/重启后能否恢复）。

| 能力 | 位置 | 状态 |
| --- | --- | --- |
| 框架级 progress 发令 | `node_body.go:187` `runtime.TrackProgress` → 从 ctx 取 `ProgressCallback` | ✅ 已落地（功能完整） |
| pipeline 写 `ingestion_task_log` | `pipeline.go:165` `taskLogProgressCallback`（`dao.DB==nil` 时 no-op） | ✅ 已落地，但 `progress` 字段语义是状态 `{0,1,-1}`，无序号、无进入/退出区分、无输出序列化 |
| eino `WithInterruptAfterNodes` 接线 | `compile.go:56,78,143` `CompileOptions.InterruptAfter` → `compose.WithInterruptAfterNodes` | ✅ 已落地（功能完整） |
| eino CheckPointStore（Redis） | `compile.go:63` + `checkpoint_store.go` `RedisCheckPointStore`，key=`agent:cp:{id}` | ✅ 已落地（功能完整） |
| 状态机（running/succeeded/failed/cancelled） | `run_tracker.go` `RunTracker`，key=`agent:run:{run_id}`，状态 `0/1/2/3` | ✅ 已落地（功能完整） |
| interrupt 捕获 / resume id 暂存 | `runner.go:163` `interruptIDs`（**内存** map） | ✅ 功能完整（agent/chat 用，进程内可用）；⚠️ **持久化待补**（跨进程重启即丢，ingestion 不能依赖） |
| `UserFillUp` interrupt 发令 | `interrupt_resume.go` `UserFillUpNodeBody`（`compose.Interrupt` / `GetResumeContext`） | ✅ 发令功能完整；⚠️ resume 侧（`ResumeWithData`）尚未接线（仅注释提及） |
| 稳定 `CheckPointID` 选项 | `compile.go` `CompileOptions` | ❌ 缺 `WithCheckPointID`，目前 CheckPointID 每编译一次生成 |

**关键结论**：续跑要用的「执行完 interrupt → 持久化 checkpoint → 后续 resume」底座在 canvas 层已经齐全（功能层面），ingestion 只需把 `Pipeline.Run` 接上，并补齐 **interrupt id 的跨进程持久化** 这一缺口。

---

## 3. 路线 A：自研 task_log 缓存 + 框架闸口跳过（先前提案，已放弃用于续跑）

### 机制
- `realComponentBody` 在 `comp.Invoke` 前，从 ctx 取 `ResumeProvider`（agent 侧为 nil），按 `cpnID` 命中则直接 `return cachedOutput`，不调 `Invoke`。
- `Pipeline.Run` 预读 `ingestion_task_log` 的 exit 行 → 构建 `map[cpnID]output`，经 ctx 注入；并注入 `cpnID→index` 映射供 progress 写 0-based 序号。
- 失效靠 `input_fingerprint = hash(in)`：exit log 记指纹，resume 时组件输入指纹不匹配则重跑。

### 优点
- 不依赖 Redis；DB 无关（测试友好）。
- 对 eino resume 语义零依赖，完全自控。

### 缺点（这是放弃它做续跑的原因）
- **重造 checkpoint**：eino 已有现成的图级 checkpoint + 序列化 + 恢复，A 等于自己再写一遍，且对 DAG / Loop / Parallel 的拓扑正确性要自己保证。
- **框架闸口 skip 是 hack**：在 `realComponentBody` 里短路 `Invoke`，与「pipeline 完整执行 DSL」的契约微妙冲突；eino 也失去了它自己对该节点执行结果的一致性保证。
- **体积（量化）**：exit log 要存全量 output（longtext）。预估：单文件 parser 输出通常 KB~MB；长文档 / 多分片可达 **十 MB 级**；若按「每 component 一条 exit 行 + 全量 output」落库，单 task 的 `ingestion_task_log` 体积 = Σ(output_size)，可能达 **数十 MB**。这正是 B 方案把全量 output 交给 eino checkpoint（Redis，带 TTL，可过期）而非 task_log（DB longtext，需永久保留）的原因。
- **指纹失效不彻底**：仅 `(cpnID, fingerprint)` 仍可能在 DSL 结构变化（节点增删、边重连）时误命中。

---

## 4. 路线 B：eino interrupt-after-node + Redis checkpoint（推荐）

> 思路：每个 component **执行完成后** 由 eino 自动 `interrupt`，把整图状态（含该节点输出）序列化进 Redis checkpoint；orchestrator 捕获 interrupt 后 `ResumeWithData` 续跑下一步。进程崩溃/重启后，下一次 `Pipeline.Run` 检测到遗留 checkpoint 即从中恢复。

### 4.1 接线（编译期）
`Pipeline.Run` 编译时传入：

```go
compiled, err := canvas.Compile(ctx, p.canvas,
    canvas.WithCheckPointStore(dao.NewRedisCheckPointStore(ttl)), // 复用 checkpoint_store.go
    canvas.WithStateSerializer(canvas.CanvasStateSerializer{}),  // state_serializer.go（JSON）
    canvas.WithInterruptAfterNonTerminalCpn(),                  // 无参；canvas.Compile 内部算出非终态节点并 interrupt
    canvas.WithCheckPointID(p.taskID),                           // 稳定 id = taskID（需新增选项，见 §6）
)
```

- `WithInterruptAfterNonTerminalCpn()`（**无参**，用户建议）：调用方不再需要计算 / 传入 `nonTerminalCpnIDs`。`canvas.Compile` 在内部直接根据 `Canvas` 结构算出「非终态节点 id」（见 §4.1.a），并对它们注册 `WithInterruptAfterNodes`。这样接线从调用方下沉到 `Compile`，避免「算错集合 / 传 allCpnIDs」类错误（见 §4.1.a），也让 `UserFillUp` 排除（§4.2.b）由同一处统一处理。

#### 4.1.a 为什么不能用 `allCpnIDs`（用户发现 2）
`WithInterruptAfter(allCpnIDs)` 会在**每个**节点（含终态/输出节点）执行后再插入一次 eino interrupt。终态节点的 `Invoke` 返回即代表整图完成；若把它纳入，eino 会在它执行后再 pause 一次，导致 orchestrator 多一轮无意义的 `ResumeWithData` 才真正结束，且可能让 `Invoke` 返回 interrupt 错误而非完成结果，打乱 §4.2 的 `err==nil → break` 判定。

正确做法：**只对「有下游」的非终态节点 interrupt**；终态节点正常退出 → `err==nil` → 循环 break，整图一次完成。该集合由 `canvas.Compile` 内部计算（出度 > 0 的中间节点 = `Canvas.Components` 中不是 `Canvas.Outputs` 的节点），调用方用无参 `WithInterruptAfterNonTerminalCpn()` 即可，无需也无法传错集合。
- `WithCheckPointID`：目前 `CompileOptions` **没有**该字段，需新增（映射 `compose.WithCheckPointID`），使 checkpoint key 稳定为 `agent:cp:{taskID}`，重跑同一个 task 才命中同一份 checkpoint。

### 4.2 运行期：进程内循环 + 跨进程恢复（统一在 `Pipeline.Run`）

```go
// Pipeline.Run 内部
for {
    // 崩溃恢复优先（用户建议）：每轮首要动作先判断是否有遗留中断。
    // 上一进程在「命中 interrupt → 持久化 interrupt id」之后、消费它之前崩溃，
    // 会在 Redis 留下未消费的 interrupt_id。本轮第一件事就是直接 resume 该中断，
    // 而不是从图入口重跑（那样会重复执行已完成节点）。
    if pending := runTracker.GetInterruptID(ctx, p.taskID); pending != "" {
        runCtx = compose.ResumeWithData(runCtx, pending, nil)
        runTracker.ClearInterruptID(ctx, p.taskID) // 已消费，避免下一轮重复 resume
    }
    out, err := compiled.Workflow.Invoke(runCtx, current)
    if err == nil {
        break // 整图完成
    }
    if !canvas.IsInterruptError(err) {
        return nil, err // 真错误
    }
    // 续跑型 interrupt（不在本轮回内联 resume，交给下一轮顶部统一 resume，
    // 避免同一 ctx 被重复 ResumeWithData）。
    // 安全前提见 §4.2.b：ingestion DSL 不得含 UserFillUp 节点，
    // 因此 IsInterruptError 命中即续跑，不会误当 wait-for-user。
    ctxs := canvas.ExtractInterruptContexts(err)
    interruptID := canvas.RootInterruptID(ctxs)
    runTracker.AttachInterrupt(ctx, p.taskID, interruptID)   // 持久化到同一 hash（见 §4.4），供崩溃后恢复
}
```

单次 `Pipeline.Run` 调用即可跑完整条 pipeline，每节点执行后把状态落 Redis；中途崩溃则下一次调用恢复。

#### 4.2.b 安全前置：`UserFillUp` 在 ingest DSL 的禁止（S3）
ingestion 当前无交互节点，但若将来 DSL 增加 `UserFillUp`（如人工确认 chunking 结果），`IsInterruptError` 命中会被误判为续跑。前置守卫二选一并写进编译契约：

- **方案 A（推荐，编译期禁止）**：`canvas.Compile` 增加 `InterruptAfterNonTerminalCpn` 与 `UserFillUp` 的互斥校验——ingestion 走该选项时，若 DSL 含 `UserFillUp` 节点直接编译失败，返回明确错误。
- **方案 B（运行期过滤）**：`canvas.Compile` 在计算 `nonTerminalCpnIDs`（供 `WithInterruptAfterNonTerminalCpn` 使用）时显式剔除 `UserFillUp` 类节点，且不对其注册 after-node interrupt；其余节点的 interrupt 才走续跑路径。

**共存原理（Question：`WithInterruptAfter` 与用户导致的 interrupt 能否共存？）**：两者都是 eino 的**同一套 interrupt 原语**，仅靠 **interrupt id** 区分，由 `IsInterruptError` 统一捕获、`ExtractInterruptContexts` / `RootInterruptID` 取出对应 id。`WithInterruptAfterNodes` 是「编排级」pause（节点跑完由框架插入，resume 传 `nil`）；`compose.Interrupt`（UserFillUp 内部调用）是「节点级」pause（节点自己发令、等待用户数据，resume 传用户数据）。它们**可以共存于同一图**——只要同一个节点不被两种机制重复注册。ingestion 通过 §4.2.b 把 UserFillUp 排除出 `nonTerminalCpnIDs`，使其仅保留自身 `compose.Interrupt` 作为该节点的唯一 interrupt 源；其余节点走 after-node interrupt。该共存行为需在 eino v0.9.12 上 spike 确认（见新增 R7），但原理上无冲突。

本期 ingestion DSL 保证无 `UserFillUp`，两条守卫任选其一落地即可消除 R6 歧义。

### 4.3 跨调用恢复（「重复运行一个 task」）
`Pipeline.Run` 开头判断：

```go
st := runTracker.Get(ctx, p.taskID) // agent:run:{taskID}，status ∈ {0,1,2,3}
switch st["status"] {
case runStatusRunning:              // 0：上次跑崩/被中断，保留 checkpoint + interrupt_id
    // 不删、不在此处 resume；由 §4.2 循环顶部读 GetInterruptID 直接恢复
case runStatusCancelled:            // 3：见 §4.3.b
    store.Delete(ctx, p.taskID)     // 清旧 checkpoint + 整跑（默认策略）
case runStatusSucceeded, runStatusFailed, "": // 1/2/""：已完成或全新 → 重跑
    store.Delete(ctx, p.taskID)     // 清旧 checkpoint（连带清 interrupt_id，循环中无 pending → 从入口跑）
}
```

- **`running` 的续跑入口在循环顶部**：状态机只决定「保不保留 checkpoint」。`running` 时不删，下一行进入 §4.2 的 `for` 循环，循环顶部 `GetInterruptID` 读到遗留 interrupt_id 即 `ResumeWithData` 恢复——所以恢复逻辑只有一处（循环顶），不在 §4.3 重复写（避免与 §4.2 两处 resume 漂移）。
- 其余状态 `Delete` 会把 checkpoint 与 `interrupt_id` 一同清掉，循环顶部 `GetInterruptID` 返回 `""` → 从图入口正常跑（eino checkpoint 已删，不会命中旧状态）。

- **失效策略=状态机**，而非 A 的指纹：`running` → 复用；其余 → 重置重跑。比指纹更可靠（天然覆盖 DSL 结构变化）。
- 这与 `run_tracker.go` 的 `Start/MarkSucceeded/MarkFailed/AttachCheckpoint` 完全对齐；`Pipeline.Run` 在开始时 `Start`、完成时 `MarkSucceeded`、失败时 `MarkFailed`。

#### 4.3.a 重跑 vs 续跑语义（S2）
ingestion **不区分**「重跑」与「续跑」两种入口——它的语义统一为：

> 按 `task_id` 重做整图；框架用 checkpoint 跳过已执行节点。状态机只决定「是否命中遗留 checkpoint」：`running` 命中复用，其余清掉重跑。

即：用户再次提交同一 `task_id`（合法重跑用例）时，行为是「先清旧 checkpoint 再整跑」——这是**正确的**（用户明确要重做）。如果将来需要「基于上次结果继续而不重做」，需新增独立的 resume 入口（不在本期范围），届时 `succeeded` 分支才需要区分「清掉重跑」与「保留续跑」。本期在 API / 用户文档中明确此语义。

#### 4.3.b `cancelled` 状态恢复策略（S5）
`RunTracker` 有 `runStatusCancelled = "3"`（`run_tracker.go:44`）。本期策略：

- 命中 `cancelled` 默认**不允许自动续跑** → 清 checkpoint + 整跑（与 `failed` 一致），避免「用户取消后悄悄继续」的语义歧义。
- 可选增强（不在本期）：暴露显式 `resume cancelled` 入口，由人工/API 触发，才走 `running` 的复用分支。

### 4.4 需要新增的持久化（M3 + S4）
`runner.go:163` 的 `interruptIDs` 是**内存** map，进程重启即丢。

- **M3 决策（不新增 key）**：复用现有 `agent:run:{taskID}` hash，新增 `interrupt_id` 字段，而非引入第二种持久化约定（`agent:cpint:{taskID}`）。给 `RunTracker` 加：
  ```go
  func (t *RunTracker) AttachInterrupt(ctx, runID, interruptID string) error // HSet "interrupt_id"
  func (t *RunTracker) GetInterruptID(ctx, runID) (string, error)            // HGet "interrupt_id"
  func (t *RunTracker) ClearInterruptID(ctx, runID) error                    // HDel "interrupt_id"（循环顶部消费后调用）
  ```
  与既有的 `AttachCheckpoint`（写 `checkpoint_id` 字段，`run_tracker.go:110`）同 hash、同生命周期、同 TTL，维护成本最低。
- **S4 边界（不动 agent/chat）**：本次改动**仅引入 ingestion 用的持久化路径**。`runner.go` 的内存 `interruptIDs` 继续服务 agent/chat 路径，不做合并——不为「统一性」强行改 agent 调用方（遵循 `CLAUDE.md`：agent 仍在用就保留）。文档明确这一点，避免后续实现者把它当 deprecated 留着或误删。

### 4.5 优点
- **复用现成底座**：eino 负责 checkpoint 序列化/恢复，对 DAG/Loop/Parallel 拓扑正确性是 eino 保证的，不自己再造。
- **组件零改动、零 `dao` 依赖**：`Parser` 等完全不知道 resume 存在，符合 `node_body.go:156` 原则。
- **用户原始意图被升级为框架级 checkpoint（澄清）**：用户最早设想「组件读自己的 exit log 跳过」，方案 B 不是让组件去读自己的 log，而是**框架统一用 eino checkpoint 在闸口处跳过**——组件仍然零改动，意图以更稳健的方式达成（等价于「框架替组件做了它想做的事」）。
- **失效干净**：状态机驱动，无指纹误命中问题。
- **天然支持用户需求**：「重跑同 task，已完成组件不重做」= checkpoint 恢复，零额外代码。

### 4.6 缺点 / 风险（见 §6 风险与 spike）
- 依赖 Redis（ingestion 已用 Redis，可接受；无 Redis 时的**可见**降级见 R5）。
- 需把 `Pipeline.Run` 从单次 `Invoke` 改为循环，且管理 interrupt id 持久化。
- eino 序列化保真度：checkpoint = `CanvasState` 整体 JSON（`state_serializer.go` 用 `json.Marshal`）。native 解析器等若产出非 JSON 类型（`[]byte`、自定义 struct）会断 resume——需 spike（R1）。
- 需确认 `WithInterruptAfterNodes` + 全图 `ResumeWithData` 在 Loop/Parallel 下的精确语义（R3，P0）。

---

## 5. Progress / Log 改进（路线无关，必做）

无论续跑走 A 还是 B，`taskLogProgressCallback` 都该升级为**结构化事件**，满足用户最初要求：

- `progress` 字段改为 `(component 序号 0-based, 相位 enter/exit/error)`。
- **不携带 component 的 `Output`**：resume 由框架 eino checkpoint 负责，progress/log 仅是观测，不需要 output（用户决定）。这同时消解了 M5「事件携带 vs sink 不落库」的紧张——两层现在一致「都不存 output」。

### 5.1 共享类型保持纯净（M1）
`runtime.ProgressEvent` **不**携带 `Index`——`Index` 是 ingestion 专有概念，放进 agent/ingestion 共享类型属于抽象泄漏。定义：

```go
// internal/agent/runtime（共享，agent 与 ingestion 共用，保持纯净）
type ProgressPhase int
const (PhaseEnter ProgressPhase = iota; PhaseExit; PhaseError)
type ProgressEvent struct {
    Phase     ProgressPhase
    Component string            // cpnID
    Err       error             // 仅 PhaseError 非空
    // 不携带 Output：resume 由 eino checkpoint 负责，progress 仅是观测（用户决定）
}
type ProgressCallback func(ProgressEvent)
```

`realComponentBody` 的 `cpnID` 是闭包参数（`node_body.go:180`），而 `node_body.go:187` 当前传给 `TrackProgress` 的**第一参数是 `componentClass`（class 名），不是节点 id**。改造时把该第一参数从 `componentClass` 改为 **`cpnID`**：`runtime.TrackProgress(cpnID, ...)`，使 `ProgressEvent.Component` 填的是**节点 id**（`ProgressEvent.Component` 用于落库定位与排序，需唯一标识节点；class 名无法区分同名多实例）。采用「只改调用处第一参数」方案，不扩 `TrackProgress` 签名（更少改动，符合用户建议）。`realComponentBody` 改为发 `ProgressEvent{Phase, Component: cpnID, ...}`（enter 时 Phase=PhaseEnter；exit 时 Phase=PhaseExit；error 时 Phase=PhaseError 且填 `Err`），**不读取、不传递组件 output**。

### 5.2 ingestion 侧注入 Index（M1 落地）
在 `pipeline` 包内定义轻量包装，Index 由 **sink 层**用 pipeline 注入 ctx 的 `cpnID→index` 映射算出，不污染共享类型：

```go
// internal/ingestion/pipeline（ingestion 专有）
type pipelineProgressEvent struct {
    Index int                // 0-based，来自 ComponentIndexMap
    Event runtime.ProgressEvent
}
```

`taskLogProgressCallback` 闭包持有 `cpnID→index` 映射，收到 `ProgressEvent` 后计算 `Index = indexMap[event.Component]`，再写库。Index 的归属与计算完全留在 ingestion 侧。

### 5.3 落库策略（M5，已消解）
`ProgressEvent` **不携带 `Output`**（用户决定：resume 不需要它，output 完整回放本就交给 eino checkpoint / Redis，progress 仅是观测）。因此 M5 原本的「事件携带 vs sink 不落库」紧张关系**不再存在**——两层一致：事件不带、sink 也不存。

**时间戳（Q1 结论：event 不携带时间戳）**：
- `ProgressEvent` 不携带时间戳。回调在 `realComponentBody` 内同步触发、sink 立即落库，emit≈落库。
- 时间戳只由 GORM `BaseModel.create_time` 自动写入（**秒级精度**），作为行级元数据；现有 `pipeline.go` 把 `create_time` 又塞进 `checkpoint` JSON 是**冗余，应删除**。
- 行内排序以自增 `id` 为准（`LatestLogByTaskID` 已依赖 `id DESC` 做确定性 tie-break，因 `create_time` 秒级会撞）——event 时间戳对排序无增量价值。
- 与移除 `Output` 一致，保持共享 `ProgressEvent` 精简（M1 纯净化）。

> `taskLogProgressCallback` 直接写独立列（`component_index` / `phase` / `component` / `message`），**不再写 `checkpoint` JSON**（不含 output、不含 create_time）；时间戳仅来自 `BaseModel.create_time`。任何地方都不在 progress 通道里携带组件全量 output（体积问题见 §3 量化，且 resume 不依赖它）。

---

## 6. 风险与待验证（Spike，附优先级）

| # | 风险 | 优先级 | 验证方式 |
| --- | --- | --- | --- |
| R1 | `CanvasState` 经 JSON 序列化/反序列化保真（native 解析器 `[]byte`/自定义类型） | P1 | 集成测试：跑含 Parser 的 DSL，中断→恢复，断言恢复后输出与一次性跑一致 |
| R2 | 稳定 `CheckPointID` 接线 | P1 | 给 `CompileOptions` 加 `CheckPointID` + `WithCheckPointID`；单测验证 key=`agent:cp:{taskID}` |
| **R3** | **`WithInterruptAfterNodes` + 全图 `ResumeWithData` 在 Loop/Parallel/DAG 下的语义** | **P0** | **✅ 已通过（2026-07-09）**：`internal/agent/workflowx/r3_interrupt_test.go` 五个集成用例全绿（DAG / 含 Loop 前置节点 / 含 Loop 节点本身 / 含 Parallel / 跨进程重编译同 CheckPointID 恢复）。结论：interrupt-after-node + `ResumeWithData` 在三类拓扑下语义正确——① 已完成节点（interrupt 之前的）resume 后**不重跑**（counter 恒为 1，无「重解析文件」风险）；② Loop/Parallel 正确重入（或已完成则跳过，counter 不增）；③ 跨进程崩溃恢复（重编译图 + 同 `WithCheckPointID`）可续跑。**无需 M2 回退**，§8 第 3 步可按原计划推进。注意：after-node interrupt 仅在**有状态图**（`WithGenLocalState`，与 `canvas.BuildWorkflow` 的 `CanvasState` 一致）下才会被包成 `*interruptError` 并被 `ExtractInterruptInfo` 识别；无状态图返回裸 `*core.InterruptSignal`（`ExtractInterruptInfo` 返回 nil）。ingestion 路径因 `CanvasState` 已注册而天然满足。 |
| R4 | interrupt id 持久化与崩溃恢复 | P1 | 进程内循环跑完正常；模拟「跑崩在节点 N」→ 新进程 `Pipeline.Run` 从该断点续跑 |
| R5 | Redis 不可用降级（**可见**，非静默） | P1 | 见 §6.a：启动探测 Redis → 拒绝启动并明确报错，或在 task_log 写 `resume_degraded` warning |
| R6 | ingestion「续跑型 interrupt」与 agent「wait-for-user interrupt」区分 | P2 | 由 §4.2.b 的 `UserFillUp` 禁止/过滤守卫消除，无需靠运行期猜测 |
| R7 | `WithInterruptAfterNodes` 与节点内 `compose.Interrupt`（UserFillUp）在同一图的共存语义 | P1 | eino v0.9.12 集成测试：构造含 UserFillUp + 非终态 after-node interrupt 的图，断言两者按 interrupt id 分别 resume，且 UserFillUp 节点不被重复 interrupt（见 §4.2.b 共存原理） |

### 6.a R5 可见降级（M4）
静默降级为「整跑」会悄悄违反用户需求「重跑同 task，已完成组件不重做」，且无上层信号。改为**可见**二选一：

- **方案 A（推荐）**：ingest job 启动时探测 Redis 不可用 → **直接拒绝启动**并返回明确错误（不在无 checkpoint 能力下假装支持 resume）。
- **方案 B**：降级路径必须在 `ingestion_task_log` 写一条 `resume_degraded` warning（含原因），使 API 侧能查询到「本次为整跑、无续跑保证」。

两种都保证**不静默**。本期选 A（强信号），B 作为可加的审计记录。

### 6.b R3 不通过的回退（M2）
若 eino v0.9.12 不支持「全图 interrupt-after-node + ResumeWithData」：
- 降级为「**只在 DAG 节点上 interrupt**」（Loop/Parallel 内部不 interrupt，整段作为原子单元），或
- 回退到路线 A 的**轻量版**（仅 progress/log，无 resume）。

> ✅ **R3 已于 2026-07-09 通过**（见 §6 R3 行 + `internal/agent/workflowx/r3_interrupt_test.go`），M2 回退路径**当前不需要**；§8 第 3 步的 resume 循环可按原计划安全推进。

---

## 7. 推荐方案

**续跑走 B，观测走轻量 task_log**：

1. `ProgressCallback` 升级为结构化事件（序号 + 进入/退出 + 退出带输出），`taskLogProgressCallback` 写轻量 `ingestion_task_log`（sink 不存全量 output，见 §5.3）。
2. `Pipeline.Run` 接 eino interrupt/checkpoint：编译期 `WithInterruptAfterNonTerminalCpn()`（无参）+ `WithCheckPointStore` + `WithCheckPointID(taskID)`；运行期内部 resume 循环（循环顶部先判断遗留中断并恢复）；`RunTracker` 状态机驱动跨调用恢复与失效。
3. 组件（`Parser` 等）**完全不改**、不依赖 `dao`；续跑由框架闸口 + Redis checkpoint 统一负责。

理由：B 复用仓库已建好的 eino 底座（功能完整，仅缺 ingest 用持久化），最省事且对图拓扑正确性最稳；A 的「自研缓存 + 闸口跳过」仅在「无 Redis、必须 DB 无关」的极端约束下才值得。

---

## 8. 实施步骤（建议顺序，含评审修订）

0. **schema 评估与迁移（S1，新增）+ 前端取数契约（Q2/Q3）** ✅ **已实现（entity/DAO/pipeline 落地，dao+pipeline 包测试通过）**：
   - **表结构（最小迁移，沿用 `ingestion_task_log`，但 `checkpoint` JSON 拆列）**：原 `checkpoint` LONGTEXT(JSON) 按 key 拆成**独立列**便于直接访问与聚合（用户决定）：`id` BIGINT 自增 PK；`task_id` VARCHAR(32) 索引；`component_index` INT（0-based 序号，key `index`）；`phase` TINYINT（key `phase`，复用 `ProgressPhase`：0=enter / 1=exit / 2=error）；`component` VARCHAR(64)（cpnID，key `component`，建议加索引便于聚合）；`message` TEXT（key `message`）；`BaseModel`（`create_time`/`update_time`/`deleted_at`，时间戳只此一处）。**不再有 `checkpoint` 列**（旧 `{progress,message,create_time}` 整体废弃）。拆列后 `AggregateProgress` 可纯 SQL `GROUP BY phase, component` 完成，无需解析 JSON（见下方端点）。
   - **老 `progress` 字段兼容**：旧编码 `{progress:0/1/-1, message, create_time}` 原本整体在 `checkpoint` JSON 列内（无独立列）。现改为**拆列**后 `checkpoint` 列直接删除，旧 `IngestionTaskLog.Create` 路径（写 JSONMap）随之废弃；`IngestionTaskLogDAO` 改用直接写列的 `Create`（实体字段即列），无需 `CreateStructured` 双写。需确认现有 API 消费者是否依赖旧 `checkpoint.progress` 编码（见下方端点兼容性），若有则端点层做字段映射兼容。
   - **总组件数 N（算百分比必需）**：在 `ingestion_task` 表**新增加法列** `component_total INT`（任务创建时由 DSL 节点数写入）。加法列迁移安全，且让"完成百分比"有服务端权威 N。
   - **前端取数端点（建议）**：
     - `GET /api/v1/ingestion_task/{task_id}/logs` → 复用 `IngestionTaskLogDAO.ListLogsByTaskID`，**改为按 `id ASC`**（时间正序，日志流用）返回 `{id, task_id, index, phase, component, message, create_time}`。前端按 phase 渲染（0 开始 / 1 完成 / 2 失败）。
     - `GET /api/v1/ingestion_task/{task_id}/progress` → 服务端聚合 `{total, done, failed, running, percent, status}`。因 `phase`/`component` 已是独立列，`AggregateProgress` 可纯 SQL 完成（无需解析 JSON）：按 `component` 取每个组件最新 `phase`（按 `id DESC` 取每组件末行），再 `CASE WHEN 末phase=1 THEN 'done' WHEN 末phase=2 THEN 'failed' ELSE 'running' END` 计数；`percent = done/total*100`；`total` 取 `ingestion_task.component_total`。前端拉进度条不必先拉全量日志。
   - 产出：migration 脚本（含 `component_total` 加法列）+ API 兼容性结论，进入第 1 步前完成。
   - **已落地（本次）**：
     - `entity.IngestionTaskLog`：`Checkpoint JSONMap` → 四列 `ComponentIndex`(INT) / `Phase`(INT) / `Component`(VARCHAR(64),index) / `Message`(TEXT)；`entity.IngestionTask` 新增 `ComponentTotal`(INT,default 0)。GORM `AutoMigrate` 自动加列（加法安全）。
     - `dao`：`IngestionTaskDAO.UpdateComponentTotal(taskID,total)`；`IngestionTaskLogDAO.ListLogsByTaskID` 改 `id ASC`；新增 `TaskProgress{Total,Done,Failed,Running,Percent}` 与 `AggregateProgress(taskID,total)`（子查询取每 component 最新 `id`→按 phase 分类计数；分类**向前兼容** 1c renumber：done=1，failed∈{-1,2}）。
     - `pipeline.Run`：编译后 best-effort `UpdateComponentTotal(taskID, len(canvas.Components))`（DB 为空则跳过）。
     - ⚠️ 依赖 1c：sink 目前仍写 `Component=""` / `ComponentIndex=0`，故 `AggregateProgress` 的 `GROUP BY component` 在 1c（cpnID 注入）前会退化为单组；method 与 schema 已就绪，待 1c 填列后生效。

1. **结构化 progress 事件（独立、低风险，但先做完影响面评估）** ✅ **已实现（runtime/canvas/pipeline 落地 + runtime/canvas/pipeline 包测试通过；agent/ingestion 整树编译通过）**：
   - **1a. 影响面清点（已核实）**：`ProgressCallback|TrackProgress|WithProgressCallback|ProgressCallbackFromContext` 全仓仅 `helpers.go`/`helpers_test.go`/`pipeline.go`/`node_body.go` 4 个 Go 文件有真实引用；`file.go`/`tokenizer.go` 经内容搜索**无** `Progress`/`TrackProgress` 引用（§9 标注为旧注释，已不实）。生产可执行调用方仅 `node_body.go`（`TrackProgress` 一处）+ `pipeline.go`（sink）；`WithProgressCallback`/`ProgressCallbackFromContext` 签名随类型别名自动迁移，零改动。**无 SDK / 外部调用方**，无需对外征询。
   - **1b. 向后兼容策略 → 改为干净硬切**：影响面仅 8 处生产 + 4 个测试函数，按 AGENTS.md「优先单路径、删废弃分支」原则**未保留**旧 `ProgressCallback func(int,string)` 适配器（渐进迁移方案被否决，避免遗留 deprecated 双轨）。一次性把 `ProgressCallback` 改为 `func(ProgressEvent)`，并同步重写 `helpers_test.go` 4 个测试。
   - **1c. 落地（已做）**：
     - `runtime/helpers.go`：新增 `ProgressPhase`（`PhaseEnter=0`/`PhaseExit=1`/`PhaseError=2`，值稳定并持久化到 `phase` 列，属数据契约）+ `ProgressEvent{Phase, Component, Err}`（`Err` 仅 error 相位非空；**不携带 Output**，见 §5.3）；`ProgressCallback` 改为 `func(ProgressEvent)`；`TrackProgress(cpnID, cb, fn)` 发 `ProgressEvent{PhaseEnter}` / `PhaseExit` / `{PhaseError, Err}`（不再拼 `compName Started/Done/:err` 字符串，消息改由 sink 派生）。
     - `node_body.go:187`：`TrackProgress(componentClass, ...)` → `TrackProgress(cpnID, ...)`，`ProgressEvent.Component` 现填**节点 id**（唯一标识，可区分同名多实例）；`componentClass` 仍用于 `resolveTimeoutFromContext`，无冗余。
     - `pipeline.go`：`taskLogProgressCallback` 改消费 `ProgressEvent`，`componentIndexMap()`（按排序 cpnID 给确定性 0-based 序号，因 map 遍历无序）算 `ComponentIndex`，并写 `Component`/`Phase`(=int(Phase))/`ComponentIndex`/`Message`（消息由相位派生：`cpnID Started`/`Done`/`:err`，保持前端兼容）。`AggregateProgress` 的 `GROUP BY component` 在 1c 后**已生效**（按 cpnID 正确分组）。
   - ⚠️ **已知不一致（非阻塞）**：`component_total` 在 step 0 取 `len(canvas.Components)`（含 no-op/UserFillUp 节点），而这些节点的 body（`legacyNoOpBody`/`UserFillUpNodeBody`）**不发** `TrackProgress`，故 `done` 计数可能永远 < `total`，进度百分比上限 < 100%。属观测精度问题，留待后续按需把分母改为「实际会发事件的组件数」。

2. **`CompileOptions` 加 `CheckPointID`（R2）** ✅ **已实现（canvas 包测试通过）**：
   - **关键 API 事实修正**：`compose.WithCheckPointID(id)` 返回的是 **运行期 `compose.Option`**（`checkpoint.go:74`），**不是** `GraphCompileOption`，无法在 `Compile`（`Workflow.Compile(...GraphCompileOption)`）里直接 `append`。因此 step 2 **不**把 `compose.WithCheckPointID` 塞进编译选项，而是：
     - `CompileOptions` 新增 `CheckPointID string` 字段 + `WithCheckPointID(id)` 选项（仅把 id 记到选项里）；
     - `Compile` 把 `cfg.CheckPointID` 存进返回的 `CompiledCanvas.CheckPointID`（该字段早已存在却从未赋值）；
     - **运行期接线（真正的 `compose.WithCheckPointID`）留到 step 3**：`pipeline.go` 在 `compiled.Workflow.Invoke(runCtx, current, compose.WithCheckPointID(compiled.CheckPointID))` 时传。service 层 `agent.go:1041` 早已自行维护 `cpID=runID` 并 `compose.WithCheckPointID(cpID)`，不受本改动影响（它不读 `compiled.CheckPointID`）。
   - **无参 `WithInterruptAfterNonTerminalCpn()`（建议1）已实现**：`CompileOptions` 新增 `InterruptAfterNonTerminal bool` + 无参 `WithInterruptAfterNonTerminalCpn()`；`Compile` 内部调用 `computeNonTerminalCpnIDs(c)` 算出「出度>0 的中间节点」（非终态），与调用方传入的 `InterruptAfter` 合并去重后统一 `compose.WithInterruptAfterNodes(...)`。选择规则：**排除终态节点**（无 `Downstream`，避免 Invoke 返回 interrupt 而非完成、并省一轮无意义 ResumeWithData）、**排除 `UserFillUp` 节点**（§4.2.b，它自带 `compose.Interrupt`，不与之重复注册）。`Canvas` 无 `Outputs` 字段，故「非终态」以 `len(Downstream)>0` 为准（与 §4.1.a 出度>0 等价）。
   - **⚠️ 接线约束**：`WithInterruptAfterNonTerminalCpn()` 一旦被 `pipeline.go` 调用而**尚无 step 3 的 resume 循环**，`Invoke` 会在第一个非终态节点暂停并返回 interrupt 错误，直接破坏 ingestion。故 step 2 **只动 `compile.go` + 单测**，`pipeline.go` 的真正接线（compile 选项 + `Invoke` 传 `compose.WithCheckPointID` + resume 循环）整体归入 **step 3**（R3 gate 之后）。
   - **单测**：`compile_test.go` 新增 `TestWithCheckPointID_OptionSetsField`、`TestWithInterruptAfterNonTerminalCpn_OptionSetsField`、`TestComputeNonTerminalCpnIDs`（断言 `n1,n2` 命中、`n3/n4` 终态排除、`uf` UserFillUp 排除）、`TestCompile_PropagatesCheckPointID`（编译成功则断言 `CompiledCanvas.CheckPointID==task-9`，编译失败则 skip，与现有 compile_test 忽略编译错误的约定一致）。

3. **`Pipeline.Run` resume 循环 ✅ 已实现（2026-07-09）**：R3 已于 2026-07-09 通过（见 §6 R3 行），无需 M2 回退。落地于 `internal/ingestion/pipeline/pipeline.go`：`Run` 先 `resolveStore()`/`resolveTracker()`（注入优先，否则按 `redis.Get()` 解析），有 store 才编译期接 `WithCheckPointStore`+`WithCheckPointID(p.taskID)`+`WithInterruptAfterNonTerminalCpn()` 进入 resumable 路径；运行期 `for` 循环（`maxResumeRounds=1000` 防呆）**顶部先恢复中断**（优先级：RunTracker `GetInterruptID` 跨进程恢复 > 进程内 `localInterruptID` 兜底），命中 interrupt 后仅 `AttachInterrupt` 持久化、下一轮顶部统一 `ResumeWithData(ctx, id, nil)`（不内联 resume，避免同 ctx 双 resume，依 §4.2 建议2）；`RunTracker.Start/MarkSucceeded/MarkFailed` 接入；`cancelled`（`context.Canceled`/`DeadlineExceeded`）分支按 §4.3.b 调 `cleanupCheckpoint`（删 eino checkpoint + `ClearInterruptID`）并 `MarkCancelled`；无 store 时降级为单次 `runPlain`（不破坏无 Redis 的现有测试）。interrupt 捕获用 `canvas.IsInterruptError`/`ExtractInterruptContexts`/`FirstInterruptID`。端到端测试 `TestPipelineRunResumableAutoResumes`（注入 in-memory store，断言每个节点恰好执行 1 次——验证 R3「resume 不重跑」在 pipeline 层成立）。

4. **interrupt id 持久化（M3）✅ 已实现（2026-07-09）**：`internal/agent/canvas/run_tracker.go` 加 `AttachInterrupt/GetInterruptID/ClearInterruptID`，写同一 hash 的 `interrupt_id` 字段（不新增 key）；agent/chat 路径不动（S4）。与 step 3 的进程内 `localInterruptID` 兜底共同构成跨/进程内双层恢复。

5. **降级与守卫（M4 + S3）✅ 已实现（2026-07-09）**：
   - **M4（§6.a 方案 A，可见拒绝，非静默）**：`Pipeline` 加 `requireResume bool` + `WithRequireResume()` 选项 + `var ErrResumeUnavailable = errors.New(...)` sentinel。`Run` 在 `resolveStore()` 之后、`Compile` 之前加检查：当 `requireResume && store==nil`（无注入 store 且无全局 Redis）时返回 `fmt.Errorf("pipeline: Run: %w", ErrResumeUnavailable)`，由调用方 `errors.Is` 识别并拒绝入队（而非静默降级 `runPlain`）。默认（不置 `requireResume`）保持原有静默降级 `runPlain`，不破坏无 Redis 的单元/headless 测试；生产 ingestion 接线时启用 `WithRequireResume()` 即「Redis 不可用 → 拒绝启动」的强信号。
   - **S3（§4.2.b 方案 A，编译期硬拒绝）**：`internal/agent/canvas/compile.go` 在 `BuildWorkflow` 之前加护栏——当 `cfg.InterruptAfterNonTerminal`（ingestion resume 模式）且 `AutoDiscoverUserFillUpIDs(c)` 非空时，`Compile` 直接返回错误 `"...WithInterruptAfterNonTerminalCpn forbids UserFillUp nodes ... would be mis-resumed with nil data"`。理由：`UserFillUp` 自身发 `compose.Interrupt`（wait-for-user），`runResumable` 的 `IsInterruptError` 全捕获并以 `nil` 自动续跑，会**静默跳过人工交互**；编译期硬拒使其永不发生。先于 step 2 已有的 `computeNonTerminalCpnIDs` 过滤（方案 B，不重复注册）构成双层守卫。
   - 测试：`pipeline_test.go` 加 `TestPipelineRun_RequireResumeRejectsWithoutStore`（无 store + `WithRequireResume` → `ErrResumeUnavailable`）；`compile_test.go` 加 `TestCompile_RejectsUserFillUpInResumeMode`（含 `UserFillUp` + `WithInterruptAfterNonTerminalCpn` → 编译错误且含 "UserFillUp"）。`bash build.sh --test ./internal/ingestion/pipeline/... ./internal/agent/canvas/...` 全绿（2026-07-09）。

6. **Spike 验证（§6 R1–R4）通过后，默认开启 checkpoint；否则回退为纯 progress/log（无 resume）**（M2 回退路径）。

---

## 9. 涉及文件与影响面

- **改**：
  - `internal/agent/runtime/helpers.go`（`ProgressCallback` / `TrackProgress` / `WithProgressCallback` / `ProgressCallbackFromContext` 签名变更）。
    **影响面（已全仓 grep 核实）**：共 **42 处匹配、6 个文件**，分布如下：

    | 文件 | 匹配数 | 性质 |
    | --- | --- | --- |
    | `internal/agent/runtime/helpers.go` | 17 | 被改文件本身（定义+实现+doc） |
    | `internal/agent/runtime/helpers_test.go` | 14 | **测试回归主成本** |
    | `internal/ingestion/pipeline/pipeline.go` | 6 | 生产调用方（sink + `WithProgressCallback` 注入） |
    | `internal/agent/canvas/node_body.go` | 2 | 生产调用方（框架发令） |
    | `internal/ingestion/component/file.go` | 2 | **仅 doc 注释**，非调用 |
    | `internal/ingestion/component/tokenizer.go` | 1 | **仅 doc 注释**，非调用 |

    - 生产**可执行**调用方只有 `pipeline.go` + `node_body.go`（共 8 处）；`file.go`/`tokenizer.go` 仅为注释，不影响编译。
    - 6 个文件全部在 `agent runtime / canvas / ingestion` 内，**无 SDK / 外部调用方** → 第 1 步无需对外征询（用户发现 2 担心的「先通知外部」仅当存在外部调用方才需，本次不存在）。
    - 真正工作量在 `helpers_test.go` 的 14 处测试回归 + 8 处生产调用方改造；采用 §8 第 1b 步的向后兼容适配器降低风险。
  - `internal/agent/canvas/node_body.go`（发结构化事件；`node_body.go:187` 把 `TrackProgress` 第一参数由 `componentClass` 改为 `cpnID`，见 §5.1）。
  - `internal/agent/canvas/compile.go`（加 `CheckPointID` 选项 + `WithCheckPointID`；新增**无参** `WithInterruptAfterNonTerminalCpn()` —— `Compile` 内部计算非终态节点 id（出度>0，§4.1.a）并对它们注册 `WithInterruptAfterNodes`，同时按 §4.2.b 排除 `UserFillUp` 节点 / 或与其互斥校验）。
  - `internal/agent/canvas/run_tracker.go`（新增 `AttachInterrupt` / `GetInterruptID` / `ClearInterruptID`，同 hash 字段）。
  - `internal/ingestion/pipeline/pipeline.go`（`Run` resume 循环——**循环顶部先 `GetInterruptID` 恢复崩溃遗留中断**、命中后 `AttachInterrupt` 持久化；升级 `taskLogProgressCallback` + `ComponentIndexMap` 注入，§4.2/§5.1；`UserFillUp` 过滤已下沉到 `compile.go`，此处不再计算 `nonTerminalCpnIDs`，§4.2.b）。
  - `internal/dao`（新增 `IngestionTaskLogDAO` 结构化写入方法 / schema 迁移，S1）。
- **复用（不改）**：`internal/agent/canvas/checkpoint_store.go`、`internal/agent/canvas/interrupt_resume.go`、`internal/agent/canvas/state_serializer.go`、`runner.go` 的 agent/chat 内存 `interruptIDs`（保留，S4）。
- **不改**：`internal/ingestion/component/*`、`internal/agent/component/*`（组件零改动）。

---

## 10. 修订记录（v1 → v2，基于评审）

| # | 项 | 优先级 | 修订 |
| --- | --- | --- | --- |
| M1 | `ProgressEvent.Index` 不应进共享 runtime 类型 | P0 | 共享类型去除 `Index`；ingestion 包内 `pipelineProgressEvent{Index, Event}` 包装，Index 由 sink 用 `cpnID→index` 映射算出（§5.1/§5.2） |
| M2 | R3（Loop/Parallel）必须前置 spike | P0 | R3 升为 P0，§8 第 3 步加 R3 gate；新增 §6.b 回退路径 |
| M3 | interrupt id 复用 RunTracker hash 字段，别加新 key | P1 | §4.4 定为 `AttachInterrupt` 写同一 hash 的 `interrupt_id`，不引入 `agent:cpint:{taskID}` |
| M4 | Redis 不可用降级要可见 | P1 | §6.a 改为拒绝启动 / 写 `resume_degraded` warning，非静默整跑 |
| M5 | §5 与 §8 第 1 步 output 携带矛盾 | P1 | 已消解：`ProgressEvent` **不携带 `Output`**（用户决定 resume 不需要，output 回放交给 eino checkpoint），事件层与 sink 层一致「都不存 output」，§5.3 重写为两层共识 |
| S1 | `ingestion_task_log` schema 兼容性 | P2 | §8 新增第 0 步 schema 评估与迁移；优先新增 DAO 方法 |
| S2 | succeeded 重跑 vs resume 语义 | P2 | §4.3.a 明确统一语义「按 task_id 重做整图，框架跳过已执行节点」 |
| S3 | `UserFillUp` 在 ingest DSL 的禁止/过滤 | P2 | §4.2.b 加编译期禁止 / 运行期过滤守卫 |
| S4 | `runner.go` 内存 `interruptIDs` 处置 | P2 | §4.4 明确 agent/chat 路径不动，本次仅引入 ingest 持久化路径 |
| S5 | `cancelled` 状态去向 | P2 | §4.3.b 明确 cancelled 默认清 checkpoint+整跑，可选暴露显式 resume 入口 |
| 排版 | §2 ⚠️/✅ 区分「功能完整」与「持久化到位」 | — | §2 表格重排，分两列表述 |
| 排版 | §3 体积量化 | — | §3 给出 ballpark（单 task 可达数十 MB） |
| 排版 | §4.5 第 2 条表述 | — | 澄清「用户意图被升级为框架级 checkpoint，非组件读自己 log」 |
| 排版 | §9 helpers.go 影响面 | — | §9 列出已知调用方 + 需 grep sweep 的说明 |
| 发现1 | `realComponentBody` 的 `cpnID` 是闭包参数，`ProgressEvent.Component` 应填 cpnID 而非 componentClass | — | §5.1 改为「调用处第一参数由 componentClass 改 cpnID」，不扩 helper 签名 |
| 发现2 | `helpers.go` 影响面实为 42 处匹配（6 文件），非「少量」 | — | §9 以全仓 grep 复核：生产可执行调用方仅 pipeline.go+node_body.go(8)；file.go/tokenizer.go 仅 doc；helpers_test.go 14 为测试回归主成本；无外部调用方；§8 第 1 步加 1a 影响面清点 + 1b 向后兼容渐进迁移 |
| 问答1 | `WithInterruptAfter` 与 UserFillUp 的 interrupt 能否共存 | — | §4.2.b 新增「共存原理」：同一 eino interrupt 原语按 id 区分，不重复注册同一节点即可共存；ingestion 排除 UserFillUp 出 nonTerminalCpnIDs；新增 R7 spike |
| 问答2 | `WithInterruptAfter` 不应传 allCpnIDs | — | §4.1 改为 `nonTerminalCpnIDs`（出度>0 的中间节点），新增 §4.1.a 解释终态节点不应 interrupt 的原因 |
| 建议1 | `WithInterruptAfter(nonTerminalCpnIDs)` 参数化 → 无参 | — | §4.1 / §4.1.a 改为无参 `WithInterruptAfterNonTerminalCpn()`；`Canvas.Components` 出度>0 的节点集合由 `canvas.Compile` **内部**计算，调用方无需也不能传错；`UserFillUp` 排除（§4.2.b）一并下沉到 `Compile`。§7/§8-3/§9 同步更新 |
| 建议2 | `Pipeline.Run` 的 for 循环应先判断 interrupt 是否存在 | — | §4.2 循环顶部新增「`GetInterruptID` 读遗留中断 → `ResumeWithData` 恢复 → `ClearInterruptID`」作为每轮首要动作；底部命中 interrupt 只 `AttachInterrupt` 持久化、不内联 resume（避免同一 ctx 重复 resume）。§4.3 状态机只做 keep/delete，`running` 的恢复入口收敛到循环顶部一处，杜绝与 §4.2 两处 resume 漂移 |
| 决策 | 选择路线 B | — | 路线 B（eino interrupt-after-node + Redis checkpoint）正式定为方案；§5/§8 的 progress 改进对 B 同样必做（原本标注 A/B 兼容，现仅服务于 B） |
| 决策 | `ProgressEvent` 不再存储 `Output` | — | §5 开头、`§5.1` 类型定义、`§5.3` 落库策略、`§8 第1c 步` 全部改为事件与 sink **均不携带/不落库 Output**；理由：resume 由 eino checkpoint 负责，progress 仅观测，不需要 output（同时彻底消解原 M5 矛盾） |
| 决策 | `ProgressEvent` 不携带时间戳（Q1） | — | §5.3 新增：event 不带时间戳；时间戳只由 `BaseModel.create_time` 自动写入（秒级），排序以自增 `id` 为准（复用 `LatestLogByTaskID` 的 `id DESC` tie-break）；删除现有 `pipeline.go` 把 create_time 塞进 checkpoint JSON 的冗余 |
| 决策 | schema 与前端取数契约（Q2/Q3） | — | §8 第0步补：沿用 `ingestion_task_log` 表、`checkpoint` JSON 改为 `{index,phase,component,message}`；`ingestion_task` 新增加法列 `component_total`；新增 `GET .../logs`（`id ASC`）+ `GET .../progress`（`{total,done,failed,running,percent,status}` 服务端聚合）端点契约 |
| 决策 | `ingestion_task_log.checkpoint` 拆成独立列 | — | §8 第0步 / §5.3 / §8-1c：`checkpoint` LONGTEXT(JSON) 按 key 拆为 `component_index`(INT) / `phase`(TINYINT) / `component`(VARCHAR,建议索引) / `message`(TEXT) 四列，删除 `checkpoint` 列；`AggregateProgress` 改为纯 SQL `GROUP BY phase,component`（无需解析 JSON）；旧 `Create`(写 JSONMap) 路径废弃，无需 `CreateStructured` 双写 |
| 决策 | §8 第1步 结构化 progress 事件（干净硬切，非渐进迁移） | — | §8 第1步 / §5.1：`ProgressCallback` 改为 `func(ProgressEvent)`（新增 `ProgressPhase` 枚举 0/1/2 + `ProgressEvent{Phase,Component,Err}`，不携带 Output/时间戳）；`TrackProgress(cpnID,cb,fn)` 发 enter/exit/error 事件；`node_body.go` 第一参数 `componentClass`→`cpnID`；`pipeline.go` sink 消费 `ProgressEvent` 并由 `componentIndexMap()`（排序 cpnID 确定性序号）填 `ComponentIndex`+写 `Component`/`Phase`；未保留 1b 适配器（按 AGENTS.md 单路径原则）；`AggregateProgress` 的 `GROUP BY component` 在 1c 后已生效 |
| 决策 | §8 第2步 `CompileOptions.CheckPointID` + 无参 `WithInterruptAfterNonTerminalCpn()` | — | §8 第2步 / §4.1.a / §4.2.b：①**API 事实修正**——`compose.WithCheckPointID` 是运行期 `compose.Option`（非 `GraphCompileOption`），无法在 `Compile` 内直接 append；故 `WithCheckPointID(id)` 仅记到 `CompileOptions.CheckPointID`，`Compile` 存进 `CompiledCanvas.CheckPointID`，**真正 `compose.WithCheckPointID` 在 `Invoke` 侧接线留 step 3**；service 层 `agent.go:1041` 自行维护 `cpID=runID`，不受影响。②无参 `WithInterruptAfterNonTerminalCpn()` → `Compile` 内部 `computeNonTerminalCpnIDs(c)` 算「出度>0 的中间节点」（排除终态与 `UserFillUp`），与调用方 `InterruptAfter` 合并去重后统一 `compose.WithInterruptAfterNodes`。③**接线约束**：该选项未接入 `pipeline.go`（无 step 3 resume 循环会破坏 ingestion），整体接线归 step 3。④单测覆盖选项 setter + `computeNonTerminalCpnIDs` + 容错传播 |
| 决策 | §6 R3 spike 结论（P0 gate 已通过） | — | R3 / DAG+Loop+Parallel / `WithInterruptAfterNodes`+`ResumeWithData`：①新增 `internal/agent/workflowx/r3_interrupt_test.go` 五个 eino 集成用例（DAG 链 A→B→C、`A→loop(B→C)→D` 中断在 A、`A→loop→D` 中断在 loop 节点本身、`A→parallel(B)→D`、以及跨进程「重编译图+同 CheckPointID」恢复），全部 `bash build.sh --test ./internal/agent/workflowx/...` 通过（2026-07-09）；②核心结论：已完成的节点 resume 后**不重跑**（节点 counter 恒为 1），Loop/Parallel 正确重入或跳过，**无需 M2 回退**，§8 第 3 步按原计划推进；③关键实现约束：after-node interrupt 仅在**有状态图**（`WithGenLocalState`，与 `canvas.BuildWorkflow`/`CanvasState` 一致）下才被包成 `*interruptError` 并被 `ExtractInterruptInfo` 识别；无状态图返回裸 `*core.InterruptSignal`（`ExtractInterruptInfo`→nil）。自定义 state 类型须 `compose.RegisterSerializableType[T]`（仿 `runtime.CanvasState`），否则 checkpoint marshal 报 `unknown type` |
| 决策 | §8 第3步+第4步（M3）实现：Pipeline.Run resume 循环 + interrupt id 持久化 | — | ①`internal/ingestion/pipeline/pipeline.go`：`Run` 改为 `resolveStore()`/`resolveTracker()`（注入优先，否则 `redis.Get()` 解析）→ 有 store 才进 resumable 路径（编译接 `WithCheckPointStore`+`WithCheckPointID(p.taskID)`+`WithInterruptAfterNonTerminalCpn()`），`for` 循环（`maxResumeRounds=1000`）顶部恢复中断（RunTracker `GetInterruptID` 跨进程 > `localInterruptID` 进程内兜底），命中仅 `AttachInterrupt` 持久化、下轮顶部 `ResumeWithData` 恢复（不内联 resume）；`RunTracker.Start/MarkSucceeded/MarkFailed/MarkCancelled` 接入；`cancelled` 分支按 §4.3.b 调 `cleanupCheckpoint` 删 eino checkpoint+`ClearInterruptID`；无 store 降级 `runPlain` 单次 Invoke；`NewPipelineFromDSL` 加 `...PipelineOption`（`WithCheckPointStore`/`WithRunTracker` 注入）。②`internal/agent/canvas/run_tracker.go` 加 `AttachInterrupt/GetInterruptID/ClearInterruptID`（同 hash `interrupt_id` 字段）。③测试：`pipeline_test.go` 加 `TestPipelineRunResumableAutoResumes`（注入 in-memory store，断言每节点恰好执行 1 次）；`bash build.sh --test ./internal/ingestion/pipeline/... ./internal/agent/canvas/...` 全绿（2026-07-09） |
| 决策 | §8 第5步（M4+S3）实现：可见降级 + UserFillUp 守卫 | — | ①**M4（§6.a 方案 A 可见拒绝）**：`internal/ingestion/pipeline/pipeline.go` 加 `requireResume bool` 字段 + `WithRequireResume()` 选项 + `var ErrResumeUnavailable = errors.New("resume unavailable: no checkpoint store (Redis down or not configured)")` sentinel；`Run` 在 `resolveStore()` 后、`Compile` 前检查 `requireResume && store==nil` → 返回 `fmt.Errorf("pipeline: Run: %w", ErrResumeUnavailable)`，由调用方 `errors.Is` 识别并拒绝入队（而非静默 `runPlain`）。默认不置 `requireResume` 保持静默降级（覆盖无 Redis 的单元/headless 测试）；生产 ingestion 接线时启用 `WithRequireResume()` 即「Redis 不可用 → 拒绝启动」强信号。②**S3（§4.2.b 方案 A 编译期硬拒）**：`internal/agent/canvas/compile.go` 在 `BuildWorkflow` 前加护栏——`cfg.InterruptAfterNonTerminal && len(AutoDiscoverUserFillUpIDs(c))>0` → 返回 `"...WithInterruptAfterNonTerminalCpn forbids UserFillUp nodes ... would be mis-resumed with nil data"`；理由：`UserFillUp` 自发包 `compose.Interrupt`，`runResumable` 全捕获并以 `nil` 续跑会静默跳过人工交互，硬拒不使其发生；与 step 2 的 `computeNonTerminalCpnIDs` 过滤（方案 B）构成双层守卫。③测试：`pipeline_test.go` 加 `TestPipelineRun_RequireResumeRejectsWithoutStore`；`compile_test.go` 加 `TestCompile_RejectsUserFillUpInResumeMode`；`bash build.sh --test ./internal/ingestion/pipeline/... ./internal/agent/canvas/...` 全绿（2026-07-09） |
