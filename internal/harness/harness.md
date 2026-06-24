# Harness 增强设计方案：事件回溯 + Replay + 观测性

## 一、总体架构

新增 3 个顶层包：

```
internal/harness/
├── events/          # 事件溯源层 - append-only event log
│   ├── event.go     # 核心 Event 类型 + EventLog 接口
│   ├── store.go     # EventStore 接口
│   ├── memory.go    # MemoryEventStore 实现
│   ├── localfile.go # LocalFileEventStore 实现
│   ├── nats.go      # NATSEventStore 实现
│   ├── recorder.go  # EventRecorder - 从 Engine/Callbacks 记录事件
│   └── clock.go     # LogicalClock - 全序逻辑时钟
├── replay/          # 重放引擎
│   ├── replay.go    # ReplayEngine - 从 EventLog 确定性重放
│   ├── fork.go      # Fork - 从任意事件分叉新执行
│   ├── diff.go      # Diff - 两次重放差异对比
│   └── injector.go  # 模型/工具结果注入策略
├── metrics/         # 观测性 & 指标
│   ├── metrics.go   # MetricsCollector 接口 + AutoCollector
│   ├── aggregator.go# 指标聚合 & 时序窗口
│   └── exporter.go  # Prometheus 导出
```

集成方式——所有组件通过 `CallbackManager` 注入到现有 Engine，零侵入：

```
┌──────────────────────────────────────────────────────────┐
│ Layer 4: events/replay/metrics                           │
│  EventRecorder ──▶ append-only event log                  │
│  AutoCollector ──▶ metrics (tool success, cost, etc.)    │
│  ReplayEngine ──▶ deterministic replay, fork, diff        │
└──────────────────────┬───────────────────────────────────┘
                       │ EventRecorder + AutoCollector
                       │ 都实现 GraphCallback 接口
                       ▼
Layer 1–3: Engine → AgentCore → AgentLoop
```

---

## 二、事件溯源层 (`events/`)

### 2.1 核心 Event 类型

```go
// EventID 全局唯一事件标识 (UUID v7, time-ordered)
type EventID string

// EventType 枚举 agent 执行过程中可追溯的每一种操作
type EventType string

const (
    // 图执行生命周期
    EventGraphStart    EventType = "graph.start"
    EventGraphEnd      EventType = "graph.end"
    EventStepStart     EventType = "step.start"
    EventStepEnd       EventType = "step.end"

    // 节点执行
    EventNodeStart     EventType = "node.start"
    EventNodeEnd       EventType = "node.end"

    // State 变更
    EventStateRead     EventType = "state.read"
    EventStateWrite    EventType = "state.write"

    // Tool 调用
    EventToolCallStart   EventType = "tool.call.start"
    EventToolCallResult  EventType = "tool.call.result"
    EventToolCallError   EventType = "tool.call.error"

    // LLM 调用
    EventLLMCallStart    EventType = "llm.call.start"
    EventLLMCallChunk    EventType = "llm.call.chunk"
    EventLLMCallEnd      EventType = "llm.call.end"

    // Memory 操作
    EventMemoryRead      EventType = "memory.read"
    EventMemoryWrite     EventType = "memory.write"

    // Human-in-the-loop
    EventApprovalRequest EventType = "approval.request"
    EventApprovalGranted EventType = "approval.granted"
    EventApprovalDenied  EventType = "approval.denied"

    // Checkpoint (与事件关联而非快照)
    EventCheckpointCreated EventType = "checkpoint.created"
    EventCheckpointRestored EventType = "checkpoint.restored"

    // 中断 / 恢复
    EventInterrupt      EventType = "interrupt"
    EventResume         EventType = "resume"

    // 错误 & 重试
    EventError          EventType = "error"
    EventRetry          EventType = "retry"

    // Fork (事件溯源特有: 从某个事件分叉新执行)
    EventFork           EventType = "fork"
)
```

```go
// Event 是不可变的 append-only 事件
type Event struct {
    ID         EventID          `json:"id"`
    Type       EventType        `json:"type"`
    Timestamp  time.Time        `json:"timestamp"`
    Clock      uint64           `json:"clock"`      // 全局单调递增逻辑时钟

    TraceID    string           `json:"trace_id"`   // 一次完整执行
    ParentID   EventID          `json:"parent_id"`  // 前驱事件 (linked list)
    CausedBy   []EventID        `json:"caused_by"`  // 多个前驱 (fork 场景)

    ThreadID   string           `json:"thread_id"`
    Step       int              `json:"step"`
    Node       string           `json:"node"`
    TaskID     string           `json:"task_id"`

    Payload    json.RawMessage  `json:"payload"`    // 类型化载荷
    Metadata   map[string]any   `json:"metadata"`

    Deterministic bool          `json:"deterministic"` // false = 包含非确定性
    Hash         string         `json:"hash"`          // SHA256 of payload
}
```

### 2.2 类型化 Payload

```go
type ToolCallPayload struct {
    ToolName   string         `json:"tool_name"`
    Arguments  map[string]any `json:"arguments"`
    Result     any            `json:"result,omitempty"`
    Duration   time.Duration  `json:"duration"`
    Error      string         `json:"error,omitempty"`
    RetryCount int            `json:"retry_count"`
}

type LLMCallPayload struct {
    Model      string         `json:"model"`
    Provider   string         `json:"provider"`
    Messages   []any          `json:"messages"`    // 输入消息
    Tokens     TokenUsage     `json:"tokens"`
    Content    string         `json:"content"`     // 完整输出
    Chunks     int            `json:"chunks"`      // streaming chunk 数
    Duration   time.Duration  `json:"duration"`
    Cost       float64        `json:"cost"`        // USD
}

type StateTransitionPayload struct {
    Channel    string `json:"channel"`
    OldValue   any    `json:"old_value,omitempty"`
    NewValue   any    `json:"new_value"`
    Reducer    string `json:"reducer,omitempty"` // reducer 类型
}

type MemoryWritePayload struct {
    Store      string  `json:"store"`       // short_term / long_term / vector
    Operation  string  `json:"operation"`   // insert / update / delete / search
    Key        string  `json:"key"`
    Value      any     `json:"value,omitempty"`
    Score      float64 `json:"score,omitempty"` // 检索相关度
}

type ApprovalPayload struct {
    RequestID  string        `json:"request_id"`
    Action     string        `json:"action"`
    Context    any           `json:"context"`
    Decision   string        `json:"decision,omitempty"`
    Latency    time.Duration `json:"latency"`
}

type TokenUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

### 2.3 EventLog 接口

```go
// EventLog 是 append-only 事件日志接口
type EventLog interface {
    // Append 追加事件 (不可变，只追加)
    Append(ctx context.Context, events ...*Event) error

    // Stream 按逻辑时钟顺序流式读取事件
    Stream(ctx context.Context, filter EventFilter) EventIterator

    // Get 按 ID 获取单个事件
    Get(ctx context.Context, id EventID) (*Event, error)

    // Range 范围查询 (按逻辑时钟)
    Range(ctx context.Context, from, to uint64, filter EventFilter) ([]*Event, error)

    // Seek 定位到指定逻辑时钟
    Seek(ctx context.Context, clock uint64) (EventIterator, error)

    // Length 返回总事件数
    Length(ctx context.Context) (uint64, error)
}

// EventFilter 事件过滤器
type EventFilter struct {
    TraceID   string       // 按 trace 过滤
    ThreadID  string       // 按 thread 过滤
    Types     []EventType  // 按类型过滤
    Node      string       // 按节点名过滤
    FromClock uint64       // 起始逻辑时钟
    ToClock   uint64       // 结束逻辑时钟
    FromTime  time.Time
    ToTime    time.Time
    Limit     int
}

// EventIterator 事件迭代器
type EventIterator interface {
    Next(ctx context.Context) (*Event, bool)
    Close() error
}
```

### 2.4 EventStore 接口 — 持久化后端

仅提供 **3 种**实现：

| Backend | 包路径 | 适用场景 |
|---------|--------|-----------|
| `MemoryEventStore` | `events/memory.go` | 开发/测试，进程内存储，重启丢失 |
| `LocalFileEventStore` | `events/localfile.go` | 单机持久化，文件分段 (按时间或大小轮转) |
| `NATSEventStore` | `events/nats.go` | 生产级分布式，NATS JetStream 持久化 |

```go
// EventStore 是 EventLog 的持久化实现接口
type EventStore interface {
    EventLog

    // Snapshot 创建快照 (可选优化，避免从头重放)
    CreateSnapshot(ctx context.Context, traceID string) (*Snapshot, error)

    // RestoreSnapshot 从快照恢复
    RestoreSnapshot(ctx context.Context, snapshotID string) (EventIterator, error)

    // Subscribe 订阅新事件 (实时)
    Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error)

    // GC 按策略清理旧事件
    GC(ctx context.Context, retention time.Duration) error
}
```

#### MemoryEventStore — 进程内存储

```go
type MemoryEventStore struct {
    mu     sync.RWMutex
    events []*Event
    byID   map[EventID]*Event
    clock  uint64
    subs   []chan *Event
}

func NewMemoryEventStore() *MemoryEventStore
```

#### LocalFileEventStore — 文件分段持久化

```go
type LocalFileEventStore struct {
    dir     string
    segment int           // 当前写入段编号
    maxSize int64          // 单文件最大字节 (默认 64MB)
    mu      sync.RWMutex
    events  []*Event       // 当前段内存缓存
    clock   uint64
}

func NewLocalFileEventStore(dir string) (*LocalFileEventStore, error)

// 文件命名: events_{traceID}_{segment}.jsonl
// 每行一个 JSON 序列化的 Event
// 读取时自动扫描目录加载所有段并合并
```

#### NATSEventStore — NATS JetStream 持久化

```go
type NATSEventStore struct {
    conn    *nats.Conn
    js      nats.JetStreamContext
    stream  string      // JetStream stream name
}

func NewNATSEventStore(conn *nats.Conn, stream string) (*NATSEventStore, error)

// 每条 NATS 消息 = 一个 Event (JSON)
// 使用 JetStream 的 ordered consumer 保证读取顺序
// Subscribe 基于 JetStream push consumer
```

### 2.5 EventRecorder — Engine 集成

`EventRecorder` 实现 `GraphCallback` 接口，通过 `CallbackManager` 注入到 Engine，零侵入：

```go
type EventRecorder struct {
    store    EventLog
    clock    *LogicalClock
    traceID  string
    threadID string
}

func NewEventRecorder(store EventLog, opts ...RecorderOption) *EventRecorder

// 实现 GraphCallback 的所有接口方法:
//   OnRunStart → EventGraphStart
//   OnNodeStart → EventNodeStart
//   OnNodeEnd → EventNodeEnd
//   OnCheckpointSave → EventCheckpointCreated
//   ...
// 同时通过 WrapModel 注入中间件自动记录 LLM 调用
// 通过 ToolInvokeMiddleware 自动记录 Tool 调用
```

使用示例：

```go
store := events.NewMemoryEventStore()
recorder := events.NewEventRecorder(store, events.WithTraceID(traceID))
cb := pregel.NewCallbackManager()
cb.AddCallback(recorder)  // 即插即用

engine := pregel.NewEngine(graph, pregel.WithCheckpointer(cp))
traced := pregel.NewTracedEngine(engine, pregel.WithEngineCallbacks(cb))
```

### 2.6 LogicalClock

```go
type LogicalClock struct {
    value atomic.Uint64
}

func NewLogicalClock() *LogicalClock
func (c *LogicalClock) Tick() uint64  // 原子自增，返回新值
func (c *LogicalClock) Now() uint64   // 读取当前值
```

---

## 三、Replay 引擎 (`replay/`)

### 3.1 确定性重放

```go
// ReplayConfig 重放配置
type ReplayConfig struct {
    Store    EventLog              // 事件源

    TraceID  string                // 要重放的 trace
    Start    uint64                // 起始 clock (0 = 从头)
    End      uint64                // 结束 clock (0 = 到最后)

    // 替换策略
    ModelOverride    ModelOverrideFunc    // 替换 LLM 模型
    ToolOverride     ToolOverrideFunc     // 替换工具结果
    StateOverride    StateOverrideFunc    // 替换初始状态

    OutputStore      EventLog             // 重放事件写入 (nil = 丢弃)
    DiffEnabled      bool                 // 是否与原 trace 对比
}

// ModelOverrideFunc 在重放时替换模型调用
// 返回非空 *string 表示使用替换结果；返回 nil, nil 表示使用录制结果
type ModelOverrideFunc func(messages []any, recordedResponse string) (*string, error)

// ToolOverrideFunc 在重放时替换工具执行
// 返回非 nil 表示使用替换结果；返回 nil 表示使用录制结果
type ToolOverrideFunc func(toolName string, args map[string]any, recordedResult any) (any, error)

type ReplayEngine struct {
    store EventLog
}

func (e *ReplayEngine) Replay(ctx context.Context, cfg *ReplayConfig) (*ReplayResult, error)

type ReplayResult struct {
    Events      []*Event
    OriginalLen int
    ReplayLen   int
    Divergences []EventDivergence     // 与原 trace 的差异
    Duration    time.Duration
    Metrics     ReplayMetrics
}

type EventDivergence struct {
    OriginalEvent *Event
    ReplayEvent   *Event
    Clock         uint64
    Type          DivergenceType  // Missing / Extra / Mismatch
    Description   string
}

type DivergenceType string
const (
    DivergenceMissing DivergenceType = "missing"
    DivergenceExtra   DivergenceType = "extra"
    DivergenceMismatch DivergenceType = "mismatch"
)

// 常用替换策略
func ReplayExactTools() ToolOverrideFunc     // 精确使用录制结果
func ReplayLiveTools() ToolOverrideFunc      // 重新执行真实工具
func ReplaySubstituteModel(m Model) ModelOverrideFunc // 用新模型替换
```

### 3.2 Fork — 从任意事件分叉

```go
type ForkConfig struct {
    Store    EventLog
    TraceID  string
    Point    EventID     // 从哪个事件分叉

    // 分叉后替换策略
    ModelOverride ModelOverrideFunc
    ToolOverride  ToolOverrideFunc
    NewInput      any

    // 分叉后使用实际 Engine 继续执行
    Engine  *pregel.Engine
    Store   EventLog     // 分叉事件写入
}

func (e *ReplayEngine) Fork(ctx context.Context, cfg *ForkConfig) (*ForkResult, error)

type ForkResult struct {
    ForkTraceID   string      // 分叉 trace ID
    ForkEvents    []*Event
    ParentTraceID string      // 原 trace ID
    ForkPoint     EventID
}
```

### 3.3 Diff — 两次执行差异对比

```go
type DiffResult struct {
    LeftTraceID    string
    RightTraceID   string

    MissingInRight []*Event           // 左有右无
    MissingInLeft  []*Event            // 右有左无
    Mismatched     []EventMismatch     // 同位置不同值

    StateDiff      map[string]StateDiff       // 状态差异
    ToolCallDiff   []ToolCallDiff              // 工具调用差异
    LLMResponseDiff []LLMResponseDiff          // LLM 输出差异
    FinalOutputDiff string                     // 最终输出差异
}

func Diff(left, right EventLog, leftTraceID, rightTraceID string) (*DiffResult, error)
```

### 3.4 典型场景

```go
// 1. 录制一次真实执行
store := events.NewNATSEventStore(conn, "agent-events")
recorder := events.NewEventRecorder(store)
// ... 执行 agent ...

// 2. 重放 + 替换模型
replay := replay.NewReplayEngine(store)
result, _ := replay.Replay(ctx, &ReplayConfig{
    TraceID: traceID,
    ModelOverride: replay.ReplaySubstituteModel(newModel),
    ToolOverride:  replay.ReplayExactTools(), // 工具结果用录制的
    DiffEnabled:   true,
})

// 3. 从第 5 个 tool call 分叉，换策略
forkResult, _ := replay.Fork(ctx, &ForkConfig{
    TraceID: traceID,
    Point:   toolCall5EventID,
    ToolOverride: func(name string, args map[string]any, recorded any) (any, error) {
        return betterToolImpl(name, args)
    },
    Engine: engine,
    Store:  forkStore,
})
```

---

## 四、观测性指标 (`metrics/`)

### 4.1 核心指标定义

```go
// AgentMetrics 一次 agent 执行的完整指标快照
type AgentMetrics struct {
    TraceID   string
    ThreadID  string
    Duration  time.Duration

    // Tool 指标
    ToolCalls       int                // 总调用次数
    ToolSuccesses   int                // 成功次数
    ToolFailures    int                // 失败次数
    ToolRetries     int                // 总重试次数
    ToolLatency     map[string][]time.Duration // 每个工具延迟列表
    ToolSuccessRate float64            // 成功率
    ToolRetryRate   float64            // 重试率 = Retries / (Calls + Retries)

    // LLM 指标
    LLMCalls        int
    TotalTokens     int
    PromptTokens    int
    CompletionTokens int
    LLMLatency      []time.Duration
    LLMCost         float64            // USD

    // Approval 指标
    ApprovalRequests int
    ApprovalGranted  int
    ApprovalDenied   int
    ApprovalLatency  []time.Duration
    ApprovalAvgLatency time.Duration
    ApprovalRate     float64

    // Checkpoint 指标
    CheckpointSaves     int
    CheckpointRestores  int
    CheckpointRestoreSuccess float64

    // Memory 指标
    MemoryReads      int
    MemoryWrites     int
    MemoryAvgHitScore float64

    // Replay 指标
    ForkCount          int
    ForkReplayPassRate float64

    // 图执行指标
    Steps           int
    NodesExecuted   int
    RecoveredErrors int

    // 综合指标
    CostPerTask     float64    // Cost / 完成任务数
    LatencyP50      time.Duration
    LatencyP95      time.Duration
    LatencyP99      time.Duration
}
```

### 4.2 MetricsCollector 接口

```go
type MetricsCollector interface {
    // RecordEvent 从事件中提取指标
    RecordEvent(event *Event)

    // Snapshot 获取当前指标快照
    Snapshot() *AgentMetrics

    // Reset 重置
    Reset()
}
```

`autoMetricCollector` 同样实现 `GraphCallback`，与 `EventRecorder` 并列注入：

```go
store := events.NewMemoryEventStore()
recorder := events.NewEventRecorder(store)
mcollector := metrics.NewAutoCollector()

cb := pregel.NewCallbackManager()
cb.AddCallback(recorder)
cb.AddCallback(mcollector)   // 并列

engine := pregel.NewEngine(graph, pregel.WithCheckpointer(cp))
traced := pregel.NewTracedEngine(engine, pregel.WithEngineCallbacks(cb))
```

### 4.3 Prometheus Exporter

```go
type PrometheusExporter struct {
    toolCalls       *prometheus.CounterVec
    toolLatency     *prometheus.HistogramVec
    toolSuccessRate prometheus.Gauge
    llmTokens       prometheus.Counter
    llmCost         prometheus.Counter
    approvalLatency prometheus.Histogram
    // ...
}

func NewPrometheusExporter(namespace string) *PrometheusExporter
func (e *PrometheusExporter) Export(m *AgentMetrics)
```

---

## 五、评测闭环 (`core/evals/` 增强)

### 5.1 EventLog → 回归数据集

```go
type Dataset struct {
    Name     string
    Cases    []DatasetCase
    Metadata DatasetMetadata
}

type DatasetCase struct {
    ID         string
    Query      string          // 用户输入
    SourceTrace string         // 来源 trace ID
    GoldEvents []*Event        // 标准事件序列
    Assertions []Assertion     // 自动生成的断言
    Tags       []string
}

type Assertion struct {
    Type        AssertionType
    Description string
    Check       func(replayEvents []*Event) error
}

type AssertionType string
const (
    AssertFinalOutput   AssertionType = "final_output"
    AssertToolCalled    AssertionType = "tool_called"
    AssertToolNotCalled AssertionType = "tool_not_called"
    AssertStateAt       AssertionType = "state_at"       // 在某 clock 处 state 满足条件
    AssertNoError       AssertionType = "no_error"
    AssertLatencyUnder  AssertionType = "latency_under"
)

// FromEventLog 从事件日志生成回归数据集
func FromEventLog(store EventLog, filter EventFilter) (*Dataset, error)
```

### 5.2 Replay-based Evaluation

```go
type ReplayEvalConfig struct {
    Dataset      *Dataset              // 回归数据集

    // 变化因子——每个组合都重放并对比
    Models       []ModelOverrideFunc    // 不同的模型
    Strategies   []ToolOverrideFunc     // 不同的工具策略

    ReportDir    string
    MaxConcurrency int
}

type ReplayEvalReport struct {
    Cases    []ReplayEvalCaseReport
    Summary  EvalSummary

    // 对比分析
    ModelComparison     map[string][]float64  // 模型 → 通过率
    StrategyComparison  map[string][]float64
    RegressionAlerts    []RegressionAlert     // 回归告警
}

type RegressionAlert struct {
    CaseName   string
    OldModel   string
    NewModel   string
    OldResult  bool
    NewResult  bool
    Diff       *DiffResult
}

func RunReplayEval(ctx context.Context, cfg *ReplayEvalConfig) (*ReplayEvalReport, error)
```

### 5.3 典型评测工作流

```go
// 1. 线上失败 trace → 自动生成数据集
failedTrace := "trace_abc123"
dataset, _ := FromEventLog(prodStore, EventFilter{TraceID: failedTrace})

// 2. 用新模型和旧模型分别重放对比
report, _ := RunReplayEval(ctx, &ReplayEvalConfig{
    Dataset: dataset,
    Models: []ModelOverrideFunc{
        ReplaySubstituteModel(oldModel),   // 基准
        ReplaySubstituteModel(newModel),   // 新模型
    },
})

// 3. 检查回归告警
for _, alert := range report.RegressionAlerts {
    if alert.OldResult && !alert.NewResult {
        log.Printf("回归! case=%s 模型从 %s 换到 %s 后失败", alert.CaseName, alert.OldModel, alert.NewModel)
    }
}
```

---

## 六、设计原则

1. **Event Log 与 Checkpoint 互补**：Checkpoint 用于快速恢复和性能优化（无需从头重放 10000 个事件），Event Log 用于精确审计和确定性重放。两者通过 `EventCheckpointCreated` 事件关联。

2. **零侵入集成**：EventRecorder 和 MetricCollector 都通过现有 `CallbackManager` 注入，不需要修改 Engine 核心代码。

3. **逻辑时钟保证全序**：monotonic uint64 避免 wall clock skew；Fork 场景用 `CausedBy` 字段表达多维因果关系。

4. **确定性标记**：`Event.Deterministic` 标记事件是否包含非确定性（LLM 输出、random 等），重放时对非确定性事件使用 ModelOverride/ToolOverride 注入。

5. **EventStore 仅 3 种实现**：Memory（测试/开发）、LocalFile（单机持久化）、NATS（生产分布式），覆盖全部典型部署场景。

---

## 七、文件清单

| 文件 | 职责 | 关键类型/方法 |
|------|------|---------------|
| `events/event.go` | 核心类型定义 | `Event`, `EventID`, `EventType`, `ToolCallPayload`, `LLMCallPayload`, `StateTransitionPayload`, `MemoryWritePayload`, `ApprovalPayload`, `EventFilter`, `EventIterator` |
| `events/store.go` | EventStore 接口 | `EventLog`, `EventStore`, `Snapshot` |
| `events/memory.go` | 内存实现 | `MemoryEventStore` (Append/Stream/Get/Range/Seek/Length/Subscribe) |
| `events/localfile.go` | 文件实现 | `LocalFileEventStore` (JSONL分文件，按大小/时间轮转) |
| `events/nats.go` | NATS 实现 | `NATSEventStore` (JetStream ordered consumer) |
| `events/recorder.go` | Engine 集成 | `EventRecorder` (GraphCallback), `RecorderOption` |
| `events/clock.go` | 逻辑时钟 | `LogicalClock` (Tick/Now) |
| `replay/replay.go` | 确定性重放 | `ReplayEngine`, `ReplayConfig`, `ReplayResult`, `EventDivergence`, `DivergenceType` |
| `replay/fork.go` | 分叉执行 | `ForkConfig`, `ForkResult` |
| `replay/diff.go` | 差异对比 | `DiffResult`, `Diff`, `EventMismatch`, `StateDiff`, `ToolCallDiff`, `LLMResponseDiff` |
| `replay/injector.go` | 注入策略 | `ModelOverrideFunc`, `ToolOverrideFunc`, `StateOverrideFunc`, `ReplayExactTools`, `ReplayLiveTools`, `ReplaySubstituteModel` |
| `metrics/metrics.go` | 指标收集 | `AgentMetrics`, `MetricsCollector`, `autoMetricCollector` |
| `metrics/aggregator.go` | 聚合 | `MetricsAggregator`, `MetricsWindow`, `AggregatedMetrics` |
| `metrics/exporter.go` | 导出 | `PrometheusExporter`, `Export` |
| `core/evals/dataset.go` | 回归数据集 | `Dataset`, `DatasetCase`, `Assertion`, `AssertionType`, `FromEventLog` |
| `core/evals/replay_eval.go` | 重放评测 | `ReplayEvalConfig`, `ReplayEvalReport`, `RegressionAlert`, `RunReplayEval` |
