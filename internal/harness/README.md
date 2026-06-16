# Agent Harness Go

[![Go Reference](https://pkg.go.dev/badge/ragflow/internal/harness.svg)](https://pkg.go.dev/ragflow/internal/harness)
[![Go Report Card](https://goreportcard.com/badge/ragflow/internal/harness)](https://goreportcard.com/report/ragflow/internal/harness)

A Go framework for building **stateful, multi-agent applications** with LLMs. It provides a **graph-based execution engine** (`graphengine/`) with Pregel-style BSP execution, plus a **full Agent Development Kit** (`agentcore/`) built on top of it — supporting ReAct agents, middleware, workflows, checkpointing, human-in-the-loop, and streaming.

---

- [Quick Start](#quick-start)
- [Two-Layer Architecture](#two-layer-architecture)
- [Layer 1: Graph Engine (graphengine)](#layer-1-graph-engine-graphengine)
- [Layer 2: Agent Development Kit (agentcore)](#layer-2-agent-development-kit-agentcore)
- [Layer 3: Push-Based AgentLoop](#layer-3-push-based-agentloop)
- [Checkpoint & Resume](#checkpoint--resume)
- [Interrupts (Human-in-the-Loop)](#interrupts-human-in-the-loop)
- [Cancellation System](#cancellation-system)
- [Prebuilt Components](#prebuilt-components)
- [Observability (OpenTelemetry)](#observability-opentelemetry)
- [Project Structure](#project-structure)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

---

## Quick Start

### Minimal StateGraph

```go
package main

import (
    "context"
    "fmt"
    "log"

    "ragflow/internal/harness"
)

type State struct {
    Messages []string
    Counter  int
}

func main() {
    ctx := context.Background()

    // 1. Create a graph builder
    builder := harness.NewStateGraph(State{})

    // 2. Add nodes (functions that read/write shared state)
    builder.AddNode("agent", func(ctx context.Context, state interface{}) (interface{}, error) {
        s := state.(State)
        s.Messages = append(s.Messages, "Hello from agent")
        s.Counter++
        return s, nil
    })

    // 3. Add edges (define execution order)
    builder.AddEdge(harness.Start, "agent")
    builder.AddEdge("agent", harness.End)

    // 4. Compile the graph (validates structure)
    graph, err := builder.Compile()
    if err != nil {
        log.Fatal(err)
    }

    // 5. Run the graph
    result, err := graph.Invoke(ctx, State{
        Messages: []string{"Starting..."},
        Counter:  0,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Result: %+v\n", result)
}
```

### Minimal ReAct Agent

```go
package main

import (
    "context"
    "ragflow/internal/harness/core"
    "ragflow/internal/harness/core/schema"
)

func main() {
    model := myChatModel{} // implements agentcore.Model[*schema.Message]

    agent := agentcore.NewReActAgent(&agentcore.ReActConfig[*schema.Message]{
        Model:       model,
        Tools:       []agentcore.Tool{&myTool{}},
        Instruction: "You are a helpful assistant.",
    }).WithName("my_agent")

    runner := agentcore.NewTypedRunner(agentcore.RunnerConfig[*schema.Message]{Agent: agent})
    iter := runner.Run(context.Background(), []*schema.Message{
        schema.UserMessage("Hello!"),
    })

    for {
        ev, ok := iter.Next()
        if !ok { break }
        if ev.Err != nil { /* handle error */ }
        if ev.Output != nil && ev.Output.MessageOutput != nil {
            // consume output
        }
    }
}
```

---

## Two-Layer Architecture

The framework is organized into three logical layers:

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 3: AgentLoop (push-based execution, preempt/stop)         │
├─────────────────────────────────────────────────────────────────┤
│ Layer 2: AgentCore ADK (ReActAgent, Runner, Middleware, Tools)   │
│           ├─ ReActAgent with iterate-loop or graph-backed exec   │
│           ├─ Middleware system (9 hook points)                   │
│           ├─ Tool system (standard + enhanced + tool_registry)   │
│           ├─ flowAgent (sub-agent management, transfer routing)  │
│           └─ workflowAgent (Sequential / Parallel / Loop)       │
├─────────────────────────────────────────────────────────────────┤
│ Layer 1: Graph Engine (graphengine)                              │
│           ├─ StateGraph builder (nodes, edges, channels)         │
│           ├─ Pregel BSP execution engine (superstep loop)        │
│           ├─ Channels (LastValue, Topic, BinOp, etc.)           │
│           ├─ Checkpoint (MemorySaver, SqliteSaver, Postgres)     │
│           └─ Prebuilt components (ToolNode, ConditionalNode...)  │
└─────────────────────────────────────────────────────────────────┘
```

**Layer 1** (`graphengine/`) is the foundation — a general-purpose stateful graph execution engine using the [Pregel](https://research.google.com/pubs/pub37252.html) BSP model. It is **LLM-agnostic** and can run any kind of stateful computation.

**Layer 2** (`agentcore/`) builds on the engine to provide an Agent Development Kit: ReAct agents, middleware chains, tool abstraction, workflow orchestration, sub-agent management, and the Runner entry point.

**Layer 3** (`agentcore/agent_loop.go`) provides push-based agent execution for chat/streaming applications, with preempt/stop controllers and turn lifecycle management.

---

## Layer 1: Graph Engine (graphengine)

### Core Concepts

The engine follows a **Builder → Compile → Execute** pattern:

1. **Build Phase**: Define nodes, edges, and state channels via `StateGraph`
2. **Compile Phase**: Validate the graph (reachability, schema, cycles)
3. **Execution Phase**: Run the Pregel superstep loop

### StateGraph

`graph.StateGraph` is the main builder. Nodes communicate by reading/writing a **shared state object** (a struct or map).

```go
builder := harness.NewStateGraph(MyState{})
```

### Nodes

Nodes are functions `func(ctx, state) (updatedState, error)`:

```go
builder.AddNode("my_node", func(ctx context.Context, state interface{}) (interface{}, error) {
    s := state.(MyState)
    // read/write state...
    return s, nil
})
```

Options (retry, tags, triggers, field mappings):

```go
builder.AddNodeWithOptions("risky_node", nodeFunc, harness.NodeOptions{
    RetryPolicy: &harness.RetryPolicy{
        MaxAttempts:     3,
        InitialInterval: 500 * time.Millisecond,
        BackoffFactor:   2.0,
    },
})
```

### Edges

Edges define the control flow between nodes:

```go
// Simple edge
builder.AddEdge("node_a", "node_b")

// Conditional edges (routing based on condition function)
builder.AddConditionalEdges("router", func(ctx context.Context, state interface{}) (interface{}, error) {
    s := state.(MyState)
    if s.Value > threshold { return "high", nil }
    return "low", nil
}, map[string]string{
    "high": "high_value_node",
    "low":  "low_value_node",
})

// Branches (multi-way fan-out)
builder.AddBranch("router", conditionFunc, thenFunc)
```

**Data edges** provide field-level data routing without affecting execution order:

```go
builder.AddDataEdge("node_a", "node_b",
    harness.NewFieldMapping("result", "input"),
)
```

### Node Trigger Modes

| Mode | Description | Best For |
|---|---|---|
| `AnyPredecessor` (default) | Node triggers when **any** predecessor completes | BSP-style graphs with cycles/loops |
| `AllPredecessor` | Node triggers when **all** predecessors complete | DAG-style fan-in/aggregation |

```go
builder.WithNodeTriggerMode(harness.AllPredecessor)
```

`AnyPredecessor` supports cyclic graphs; `AllPredecessor` does not.

### Channels

Channels define **how state is stored and updated**. Every state field maps to a channel:

| Channel | Semantics | Use Case |
|---|---|---|
| `LastValue` (default) | Keeps only the last value written | Ordinary state fields |
| `AnyValue` | Accepts multiple writes, keeps last | Similar to LastValue but relaxed |
| `Topic` | PubSub mode; `accumulate` flag controls clearing | Message queues, event buses |
| `BinaryOperatorAggregate` | Reduces values via binary operator (add, append, merge) | Numerical counters, list accumulation |
| `ReducerChannel` | Decorator wrapping any channel with a reducer | Custom reduction logic |
| `NamedBarrierValue` | Waits for named nodes to write before readable | Synchronization barriers |
| `EphemeralValue` | Auto-clears after first read | One-shot signals |
| `UntrackedValue` | Not checkpointed | Temporary computation caches |

```go
builder.AddChannel("messages", harness.NewTopic(string, true))   // accumulating topic
builder.AddChannel("counter", harness.NewBinaryOperatorAggregate(0, harness.IntAdd))
```

Built-in binary operators: `IntAdd`, `ListAppend`, `StringConcat`, `MergeReducer`, `AddMessagesReducer`.

**Schema annotations** via struct tags:

```go
type State struct {
    Messages []string `harness:"reducer=append"`
    Counter  int      `harness:"reducer=add"`
}
```

### Compile

The `Compile` step validates the graph and produces an executable `CompiledGraph`:

```go
graph, err := builder.Compile(
    harness.WithCheckpointer(saver),      // enable persistence
    harness.WithInterrupts("review"),     // set interrupt points
    harness.WithRecursionLimit(25),        // max supersteps
    harness.WithDebug(true),               // enable debug logging
)
```

### Execution

```go
// Synchronous
result, err := graph.Invoke(ctx, initialState, config)

// Streaming (event-driven)
stream := graph.Stream(ctx, initialState, config, types.StreamModeUpdates)
for event := range stream {
    // handle checkpoint, task_start, task_end, update, values, interrupt, error, final
}
```

### Pregel Engine Internals

The `pregel.Engine` runs the BSP superstep loop:

```
Input Application → Checkpoint Restore (if any)
  └─> Superstep Loop:
        ├─ prepareNextTasks (which nodes are ready?)
        ├─ shouldInterrupt (check for interrupt points)
        ├─ executeTasksAsync (concurrent via AsyncPipeline)
        │    └─ each task: read channels → run node fn → return output
        ├─ applyWrites (merge outputs into channels)
        ├─ checkpoint (save state)
        ├─ stream events (via StreamManager)
        └─ repeat until: no more tasks, recursion limit, interrupt, or cancel
```

**Concurrency model**: `AsyncExecutor` uses a semaphore-based goroutine pool with configurable `maxConcurrency`. Nodes that are independent (no data/control dependencies) execute in parallel.

**Stream events**: checkpoint, task_start, task_end, update, values, interrupt, error, final, debug — filtered by `StreamMode`.

### Checkpointer Interface

```go
type BaseCheckpointer interface {
    Get(ctx, config) (Checkpoint, error)
    Put(ctx, config, Checkpoint) error
    List(ctx, config, limit) ([]Checkpoint, error)
}
```

**Implementations**:

| Saver | When to Use |
|---|---|
| `MemorySaver` | In-memory, for testing or single-instance |
| `SqliteSaver` | File-based persistence via SQLite |
| `PostgresSaver` | Production, multi-instance with shared DB |

### Errors

| Error | Cause |
|---|---|
| `GraphRecursionError` | Exceeded recursion limit |
| `GraphInterrupt` | Graph paused for human intervention |
| `InvalidUpdateError` | Channel wrote invalid state |
| `NodeNotFoundError` / `EdgeNotFoundError` | Graph validation failure |

---

## Layer 2: Agent Development Kit (agentcore)

### Architecture

AgentCore provides high-level Agent abstractions on top of the graph engine:

```
Agent interface (Run/Resume)
  └─ ReActAgent (ReAct loop with tool execution)
       ├─ Uses ToolsNode or executeInlineTools for tool dispatch
       ├─ Middleware chain (BeforeAgent → BeforeModel → AfterModel → AfterAgent)
       ├─ Model wrapper chain (EventSender → Retry → Failover → StateWrapper)
       └─ Supports both standard for-loop and graph-backed execution
  └─ flowAgent (sub-agent management & transfer routing)
  └─ workflowAgent (Sequential / Parallel / Loop orchestration)
       └─ Runner (entry point: Run/Resume/Query)
```

### Agent Interface

All agents implement `TypedAgent[M]` (M = `*schema.Message` or `*schema.AgenticMessage`):

```go
type TypedAgent[M any] interface {
    Name(ctx context.Context) string
    Description(ctx context.Context) string
    Run(ctx context.Context, input *TypedAgentInput[M], opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]]
    Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]]
}
```

### ReActAgent

Builds a ReAct (Reasoning + Acting) loop:

**Simple for-loop** (default): `buildReActRunFunc` runs the loop inline.

```
BeforeAgent (middleware)
  └─> Loop (RemainingIterations > 0):
        ├─ BeforeModelRewrite (middleware)
        ├─ StateModifier (optional)
        ├─ GenModelInput (build input messages)
        ├─ model.Generate (with wrapper chain)
        ├─ AfterModelRewrite (middleware)
        ├─ extractToolCalls
        ├─ if tool calls → ToolsNode.Execute (or executeInlineTools)
        └─ if no tool calls → break
AfterAgent (middleware)
```

**Graph-backed** (`GraphReAct=true`): each iteration becomes a StateGraph node, enabling automatic checkpoint at every step.

```go
cfg := &agentcore.ReActConfig[*schema.Message]{Model: model}
cfg.GraphReAct = true
cfg.GraphReActCheckpointer = checkpoint.NewMemorySaver()
```

The model wrapper chain layers on top of the base model:

```
base Model
  → EventSender (emits model output events)
  → Retry (backoff + ShouldRetry)
  → Failover (backup models)
  → User Middleware.WrapModel (custom)
  → StateWrapper (deep copy + ID injection + cancel check)
  → Callback Injection (tracing/monitoring)
```

### Tools

**Standard Tool** (string I/O):

```go
type WeatherTool struct{}
func (t *WeatherTool) Name() string                       { return "get_weather" }
func (t *WeatherTool) Description() string                 { return "Get weather for a city" }
func (t *WeatherTool) Invoke(ctx, args string, opts...)    (string, error)
func (t *WeatherTool) Stream(ctx, args string, opts...)    (*schema.StreamReader[string], error)
```

**Enhanced Tool** (structured I/O via `*schema.ToolResult`):

```go
type WeatherTool struct{}
// EnhancedTool embeds Tool + adds:
func (t *WeatherTool) EnhancedInvoke(ctx, args *schema.ToolArgument, opts...) (*schema.ToolResult, error)
func (t *WeatherTool) EnhancedStream(ctx, args *schema.ToolArgument, opts...) (*schema.StreamReader[*schema.ToolResult], error)
```

**Reflective Tool** (from any struct-typed function):

```go
type WeatherArgs struct {
    City string `json:"city" description:"The city name"`
}
tool, _ := harness.ReflectTool("get_weather", "Get current weather",
    func(ctx context.Context, args *WeatherArgs) (string, error) { ... })
```

**Tool Invocation Middleware Chain** (`ToolInvokeMiddleware`):

```go
tool := ToolWrapperChain(
    ToolToInvokeFn(myTool),
    NewTimeoutToolMiddleware(5*time.Second),
    NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 3}),
    NewFallbackToolMiddleware(fallbackFn),
)
```

Built-in wrappers: **Timeout**, **Retry** (exponential backoff), **Fallback**, **Approval** (human-in-the-loop for tool calls).

**ToolRegistry** — centralized tool management with aliases, categories, filtering, and merge:

```go
registry := agentcore.NewToolRegistry()
registry.Register(myTool, agentcore.WithAlias("weather"), agentcore.WithCategory("search"))
tool := registry.Lookup("get_weather")
searchTools := registry.LookupByCategory("search")
```

**LoopGuard** — detects repeated tool calls with identical arguments:

```go
ToolsConfig: &agentcore.ToolsNodeConfig{
    Tools:     tools,
    LoopGuard: agentcore.NewLoopGuard(maxSame=2, maxFails=3),
}
```

### Middleware System

**ReActMiddleware** provides 9 hook points:

| Hook | Signature | Purpose |
|---|---|---|
| `BeforeAgent` | `(ctx, *ReActAgentContext)` | Modify instruction, tools, return-directly map |
| `AfterAgent` | `(ctx, state)` | Post-execution cleanup |
| `BeforeModelRewrite` | `(ctx, state, *ModelContext)` | Transform state before model call |
| `AfterModelRewrite` | `(ctx, state, *ModelContext)` | Transform state after model call |
| `WrapModel` | `(ctx, ChatModel[M], *ModelContext)` → ChatModel[M] | Wrap the model call |
| `WrapToolInvoke` | `(ctx, InvokableToolEndpoint, *ToolContext)` | Wrap sync tool invoke |
| `WrapToolStream` | `(ctx, StreamableToolEndpoint, *ToolContext)` | Wrap streaming tool invoke |
| `WrapEnhancedInvokableToolCall` | `(ctx, EnhancedInvokableToolEndpoint, *ToolContext)` | Wrap enhanced sync tool |
| `WrapEnhancedStreamableToolCall` | `(ctx, EnhancedStreamableToolEndpoint, *ToolContext)` | Wrap enhanced streaming tool |

Embed `BaseMiddleware[*schema.Message]` and override only needed hooks:

```go
type LoggingMiddleware struct {
    agentcore.BaseMiddleware[*schema.Message]
}
func (m *LoggingMiddleware) BeforeModelRewrite(ctx, state, mc) (context.Context, *agentcore.ReActAgentState, error) {
    log.Printf("model input: %d messages", len(state.Messages))
    return ctx, state, nil
}
```

**Prebuilt middlewares** (in `agentcore/middlewares/`):

| Middleware | Purpose |
|---|---|
| `subagent` | Injects sub-agents as callable tools (LLM-driven delegation) |
| `summarization` | Auto-compresses long conversation history on token overflow |
| `reduction` | Offloads large tool results to backend storage |
| `filesystem` | Provides read/write/edit/ls/grep/execute tools |
| `skill` | Loads and executes skills from SKILL.md files |
| `patchtoolcalls` | Fixes dangling tool calls in message history |
| `plantask` | Task management CRUD for coding sessions |
| `agentsmd` | Injects AGENTS.md file contents into model input |
| `telemetry` | OpenTelemetry tracing/monitoring middleware *(removed in internal copy)* |
| `dynamictool` | Dynamic tool registration and invocation |

### Runner

Primary entry point for agent execution:

```go
runner := agentcore.NewTypedRunner(agentcore.RunnerConfig[*schema.Message]{
    Agent:            agent,
    EnableStreaming:  true,
    CheckPointStore:  store,
})

// Run
iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("Hello")})

// Convenience
iter := runner.Query(ctx, "Hello")

// Resume from checkpoint
iter, err := runner.Resume(ctx, "checkpoint-id")
```

`Runner` wraps the agent with `flowAgent` for session management, transfer routing, and checkpoint/resume.

### Workflow Agents

**Sequential** — runs sub-agents one after another:

```go
wf, _ := agentcore.NewSequential(ctx, &agentcore.SequentialConfig{
    Name: "pipeline", SubAgents: []agentcore.Agent{agentA, agentB},
})
```

**Parallel** — runs sub-agents concurrently with event isolation:

```go
wf, _ := agentcore.NewParallel(ctx, &agentcore.ParallelConfig{
    Name: "collectors", SubAgents: []agentcore.Agent{agentC, agentD},
})
```

**Loop** — repeats a sequence of sub-agents up to MaxIterations:

```go
wf, _ := agentcore.NewLoop(ctx, &agentcore.LoopConfig{
    Name: "reflection", SubAgents: []agentcore.Agent{mainAgent, critiqueAgent},
    MaxIterations: 5,
})
```

### SubAgentMiddleware

Dynamically injects sub-agents as callable tools that the parent LLM can invoke via tool calls:

```
Parent Agent (ReActAgent)
  ├─ Tools: [..., researcher_AgentTool, coder_AgentTool] ← injected by SubAgentMiddleware
  ├─ Middlewares: [SubAgentMiddleware, ...]
  └─ Tool dispatch: executeInlineTools (ToolsConfig = nil→force inline)

  When LLM calls "researcher":
    └─ researcher_AgentTool.Invoke(ctx, args)
         └─ Runner.Run(runCtx_with_depth_1)
              └─ Researcher Agent (independent ReAct loop)
```

Three ways to declare sub-agents:

```go
// 1. Pre-built Agent
spec := SubAgentSpec{
    Name: "researcher", Description: "Research",
    Agent: agentcore.NewReActAgent(cfg).WithName("researcher"),
}

// 2. Declarative AgentConfig (recommended)
spec := SubAgentSpec{
    Name: "researcher", Description: "Research",
    AgentConfig: &AgentConfig{
        Model:        claudeModel,
        Tools:        []agentcore.Tool{searchTool},
        SystemPrompt: "You are a research assistant.",
        Middlewares:  []agentcore.ReActMiddleware{ownMiddleware},
    },
    InheritParentMiddlewares: true, // inherits parent's non-subagent middlewares
    ExcludedParentMiddlewareNames: []string{
        "*filesystem.middleware[*schema.Message]",
    },
}

// 3. AgentFactory (legacy)
spec := SubAgentSpec{
    Name: "researcher", Description: "Research",
    AgentFactory: func(ctx context.Context) (agentcore.Agent, error) {
        return agentcore.NewReActAgent(cfg).WithName("researcher"), nil
    },
}
```

**Recursion depth guard** — `Config.MaxDepth` limits nesting. Depth propagated via context:

```go
mw := subagent.New(specs, &subagent.Config{
    EmitInternalEvents: true,  // forward sub-agent events to parent stream
    MaxDepth:           3,     // allow parent→child→grandchild, block deeper
})
```

**Design principle**: `BindToConfig()` sets `config.ToolsConfig = nil` to force inline tool dispatch (the only path that finds middleware-injected tools in `rc.Tools`). Both `BindToConfig` and middleware build are idempotent.

### Sub-Agent Architecture: flowAgent vs SubAgentMiddleware

| Aspect | flowAgent (deterministic) | SubAgentMiddleware (LLM-driven) |
|---|---|---|
| Invocation | Code-driven via `TransferToAgent` action | LLM-driven via tool call |
| Control flow | Pre-registered via `SetSubAgents()`, routed by `runLoop` | LLM decides when to invoke |
| Execution context | Shares parent session, events accumulated | Independent Runner, no session sharing |
| Middleware inheritance | N/A | Optional via `InheritParentMiddlewares` |
| Orchestration | Sequential/Parallel/Loop (workflowAgent) | LLM decides sequencing |
| Best for | Predictable multi-step pipelines | Dynamic task decomposition by LLM |

Both can be combined: a workflowAgent step can use SubAgentMiddleware for dynamic sub-agent delegation within a structured pipeline.

### Event System

**AsyncIterator / AsyncGenerator** — async pull/push event mechanism.

**Event types**: Model output events, tool result events, error events, action events (interrupt, transfer, exit, break-loop).

**Event constructors**: `ToolInvokeEvent`, `ToolStreamEvent`, `EnhancedToolInvokeEvent`, `EnhancedToolStreamEvent` (preserve `Extra` metadata for multimodal).

---

## Layer 3: Push-Based AgentLoop

`AgentLoop` enables push-based agent interaction where external events can be injected while the agent is running — designed for chat/streaming applications.

**Lifecycle:**

```
idle ──beginPlanningTurn──▶ planning ──beginActiveTurn──▶ active ──endActiveTurn──▶ idle
                             │                                                      ▲
                             └────────abortPlanningTurn─────────────────────────────┘
```

**Key components:**
- **preemptController** — turn-targeted preempt with snapshot/ack mechanism
- **stopController** — global terminal stop with optional active-turn cancel
- **bridgeStore** — bridges AgentLoop checkpoints with Runner checkpoints
- **TurnContext** — per-turn Preempted/Stopped channels, StopCause
- **Callbacks**: `GenInput`, `GenResume`, `PrepareAgent`, `OnAgentEvents`

**Push options:** `WithPreempt`, `WithPreemptTimeout`, `WithPreemptDelay`  
**Stop options:** `WithGraceful`, `WithImmediate`, `WithGracefulTimeout`, `UntilIdleFor`, `WithSkipCheckpoint`, `WithStopCause`

---

## Checkpoint & Resume

Checkpoints are serialized via gob encoding with type registration (`schema.RegisterType`).

**Checkpoint payload** includes: run context (run path, session values), interrupt info (state data, interrupt signal), agent state (`*ReActAgentState`).

**Resume flow:**
1. `Runner.Resume` → loads checkpoint from store
2. Reconstructs run context from checkpoint data
3. Calls `ResumableAgent.Resume` with `ResumeInfo`
4. `ReActAgent.Resume` restores state from `InterruptState` and re-enters run function
5. `ReActAgentResumeData.HistoryModifier` allows input modification on resume

**Gob encodability check** proactively validates values at `SetRunLocalValue` time, catching unregistered types early.

```go
store := &myCheckpointStore{}

// Run with checkpoint ID
iter := runner.Run(ctx, msgs, agentcore.WithCheckPointID("run-001"))

// Resume from checkpoint
iter, err := runner.Resume(ctx, "run-001")
```

The **graph engine** side provides `Durability` modes: `Sync` (blocking after each superstep), `Async` (non-blocking), `Exit` (only on graph exit). The `CheckpointManager` uses optimistic locking for version conflict detection.

---

## Interrupts (Human-in-the-Loop)

**Graph-level interrupts** pause execution at specified nodes:

```go
graph, err := builder.Compile(harness.WithInterrupts("human_review"))
```

Inside a node:

```go
func humanReviewNode(ctx context.Context, state interface{}) (interface{}, error) {
    result, err := harness.InterruptFunc("Please review and approve")
    if err != nil {
        return nil, err
    }
    return processResult(result), nil
}
```

**Resume with command:**

```go
result, err := graph.Invoke(ctx, harness.NewCommand().WithResume(approval), config)
```

**ReActGraph** (graph-backed agent): interrupt set at the `"execute_tools"` node for human-in-the-loop before tool execution. With Checkpointer, each node transition saves a checkpoint automatically.

```go
cfg := &agentcore.ReActConfig[*schema.Message]{Model: model}
cfg.GraphReAct = true
cfg.GraphReActInterruptBefore = []string{"execute_tools"}
```

---

## Cancellation System

Three cancel modes:

| Mode | Behavior |
|---|---|
| `CancelImmediate` | Stop immediately |
| `CancelAfterChatModel` | Stop after current model call completes |
| `CancelAfterToolCalls` | Stop after current tool calls complete |

**State machine:** `cancelContext` transitions `Running → Cancelling → Done/Handled`.

**Key features:**
- Children derive from parents with configurable recursive propagation
- `deriveAgentToolCancelContext` — creates child cancel context for nested agent tools
- `timeoutEscalation` — timeout triggers escalation from graceful to immediate
- `cancelMonitoredToolHandler` / `cancelMonitoredModel` — check cancel state before dispatch
- `InterruptFromGraph` — coordinates graph-level interrupts with the cancel state machine

```go
opt, cancel := agentcore.WithCancel()
defer cancel(agentcore.WithCancelMode(agentcore.CancelAfterChatModel))

iter := runner.Run(ctx, msgs, opt)

// Later, to cancel:
handle, ok := cancel(agentcore.WithCancelMode(agentcore.CancelImmediate))
if ok { handle.Wait() }
```

**Error types:** `CancelError` (with `AgentCancelInfo`), `StreamCanceledError`, `ErrCancelTimeout`, `ErrExecutionEnded`.

---

## Prebuilt Components

Package `prebuilt/` provides ready-to-use ReAct state machine and node factories for the graph engine:

### ReAct Agent

```go
agent := prebuilt.NewReactAgent(&prebuilt.ReactAgentConfig{
    Model:          myLLM,
    Tools:          []prebuilt.Tool{myTool},
    SystemPrompt:   "You are a coding assistant.",
    MaxIterations:  10,
    StopCondition:  func(state *ReActState) bool { return state.Iteration >= 5 },
})
```

The ReAct state machine runs: **Input → Model.Generate → ParseAction → (Answer: done | Tool: execute → loop)**

### Node Factories

- **`ToolNode(tool)`** — wraps a `Tool` as a graph node with standardized output format
- **`ValidationNode(func, errorMessage)`** — input validation, passes through on success
- **`ConditionalNode(condition, branches, defaultBranch)`** — conditional routing node
- **`TransformNode(func)`** — pure data transformation node

---

## Observability (OpenTelemetry)

```go
// NOTE: telemetry package is not included in this internal copy.
// RAGFlow has its own observability setup in internal/observability/.
```

---

## Project Structure

```
harness-go/
│
├── agentcore/               # Agent Development Kit (Layer 2 + 3)
│   ├── react_agent.go       # ReActAgent: ReAct loop, freeze, run/resume
│   ├── react_loop.go        # ReAct for-loop implementation
│   ├── react_graph.go       # Graph-based ReAct using StateGraph
│   ├── contracts.go         # Middleware, Tool, Model interfaces
│   ├── tools_node.go        # ToolsNode: tool dispatch, middleware chains
│   ├── tool_invoke.go       # ToolInvocationContext, middleware wrappers
│   ├── tool_registry.go     # ToolRegistry: aliases, categories, filtering
│   ├── tool_schema.go       # Reflection-based ToolInfo generation
│   ├── event_sender.go      # Event sender middlewares
│   ├── model_chain.go       # Model wrapper chain builder
│   ├── state_wrapper.go     # StateModelWrapper: deep copy, ID injection
│   ├── retry.go             # Model retry with backoff
│   ├── failover.go          # Model failover across backup models
│   ├── flow.go              # flowAgent: sub-agent management, transfer
│   ├── workflow.go          # workflowAgent: Sequential/Parallel/Loop
│   ├── runner.go            # Runner: run/resume/query entry point
│   ├── agent_loop.go        # AgentLoop: push-based execution
│   ├── cancel.go            # Cancel state machine, cancel modes
│   ├── callback.go          # Callback handler, gob encodability check
│   ├── session.go           # Session, BranchEvents, fork/join
│   ├── agent_handoff.go     # Deterministic transfer, message ID utils
│   ├── turn_buffer.go       # AgentLoop buffer implementation
│   ├── config.go            # Agent option types
│   ├── interrupt.go         # Interrupt types and signals
│   ├── resume_data.go       # Resume data types
│   ├── utils.go             # AsyncIterator, AsyncGenerator
│   ├── tool.go              # AgentTool (sub-agent as Tool), depth guard
│   ├── instruction.go       # Instruction management
│   │
│   ├── backend/             # Filesystem backend abstraction
│   ├── evals/               # Eval framework (LLM-as-judge, scorers)
│   ├── internal/            # Internal helpers (default system prompt)
│   ├── middlewares/         # 10 middleware implementations
│   │   ├── subagent/        #   SubAgentMiddleware (LLM-driven delegation)
│   │   ├── summarization/   #   Auto-summarization
│   │   ├── reduction/       #   Tool output reduction
│   │   ├── filesystem/      #   Filesystem tools
│   │   ├── skill/           #   Skill loading
│   │   ├── patchtoolcalls/  #   Dangling tool call fixer
│   │   ├── plantask/        #   Task management
│   │   ├── agentsmd/        #   Agents.md injection
│   │   ├── telemetry/       #   OpenTelemetry tracing *(removed in internal copy)*
│   │   └── dynamictool/     #   Dynamic tool registration
│   ├── prebuilt/            # Prebuilt agents (deep, supervisor, planexecute)
│   └── schema/              # Message, ToolCall, ToolResult, StreamReader
│
├── graphengine/             # Graph Engine (Layer 1)
│   ├── graph/               # StateGraph builder, CompiledGraph
│   │   ├── graph.go         #   Nodes, Edges, ConditionalEdges, Branches
│   │   ├── state.go         #   State schema validation, annotations
│   │   ├── message.go       #   MessageGraph, MessagesState
│   │   └── compiled.go      #   CompiledStateGraph, subgraph support
│   ├── channels/            # Channel implementations
│   │   ├── base.go          #   Channel interface, BaseChannel, Registry
│   │   ├── last_value.go    #   LastValue
│   │   ├── topic.go         #   Topic (PubSub)
│   │   ├── binop.go         #   BinaryOperatorAggregate
│   │   ├── reducer.go       #   ReducerChannel (decorator)
│   │   ├── barrier.go       #   NamedBarrierValue
│   │   └── ephemeral.go     #   EphemeralValue
│   ├── checkpoint/          # Checkpoint persistence
│   │   ├── memory.go        #   MemorySaver
│   │   ├── sqlite.go        #   SqliteSaver
│   │   └── postgres.go      #   PostgresSaver
│   ├── pregel/              # Pregel BSP execution engine
│   │   ├── engine.go        #   Engine: superstep loop, task scheduling
│   │   ├── async.go         #   AsyncExecutor, AsyncPipeline
│   │   ├── stream.go        #   StreamManager, StreamEvent types
│   │   ├── write.go         #   Channel writes
│   │   ├── read.go          #   Channel reads
│   │   ├── retry.go         #   Node retry
│   │   ├── subgraph.go      #   Subgraph execution
│   │   └── websocket.go     #   WebSocket streaming
│   ├── types/               # Core types
│   │   ├── types.go         #   NodeFunc, EdgeFunc, Interrupt, Command
│   │   ├── config.go        #   RunnableConfig
│   │   ├── stream.go        #   StreamProtocol, ChannelStream
│   │   └── scratchpad.go    #   Scratchpad storage
│   ├── constants/           # Reserved keys, virtual node names
│   ├── errors/              # Custom error types
│   ├── interrupt/           # Interrupt utilities
│   ├── runnable/            # Runnable abstraction layer
│   ├── task/                # Task decorators
│   ├── managed/             # Managed execution
│   ├── viemu/               # Visual emulation
│   └── visualization/       # DOT graph output
│
├── prebuilt/                # Prebuilt ReAct agent + node factories
│   ├── prebuilt.go          #   ReAct agent state machine
│   ├── tool_node.go         #   ToolNode factory
│   ├── validation_node.go   #   ValidationNode factory
│   ├── conditional_node.go  #   ConditionalNode factory
│   └── transform_node.go    #   TransformNode factory
│
├── server/                  # HTTP server *(removed in internal copy)*
├── telemetry/               # OpenTelemetry integration *(removed in internal copy)*
│
├── harness.go               # Top-level re-exports and init()
├── harness_test.go          # Integration tests
├── Makefile                 # Build, test, lint targets
└── examples/                # Example applications
    ├── workflow/            # Workflow examples (loop, sequential)
    └── open-agent-builder/  # Web-based agent builder
```

---

## Examples

### StateGraph with Conditional Routing & Retry

```go
builder.AddConditionalEdges("router", func(ctx context.Context, state interface{}) (interface{}, error) {
    s := state.(MyState)
    if s.Value > threshold {
        return "high", nil
    }
    return "low", nil
}, map[string]string{
    "high": "high_value_node",
    "low":  "low_value_node",
})

builder.AddNodeWithOptions("risky_node", nodeFunc, harness.NodeOptions{
    RetryPolicy: &harness.RetryPolicy{
        MaxAttempts:     3,
        InitialInterval: 500 * time.Millisecond,
        BackoffFactor:   2.0,
    },
})
```

### Agent with Middleware Stack

```go
import (
    "ragflow/internal/harness/core"
    "ragflow/internal/harness/core/middlewares/filesystem"
    "ragflow/internal/harness/core/middlewares/summarization"
    "ragflow/internal/harness/core/middlewares/subagent"
)

agent := agentcore.NewReActAgent(&agentcore.ReActConfig[*schema.Message]{
    Model:       model,
    Middlewares: []agentcore.ReActMiddleware{
        subAgentMW,
        filesystem.New(&filesystem.Config{Backend: fsBackend}),
        summarization.New(&summarization.Config{
            TokenLimit: 100000,
            Model:      summaryModel,
        }),
    },
    Instruction: "You are a coding assistant.",
})
```

### Agent with Sub-Agents

```go
spec := subagent.SubAgentSpec{
    Name:        "researcher",
    Description: "Research a topic using web search",
    AgentConfig: &subagent.AgentConfig{
        Model:        claudeModel,
        Tools:        []agentcore.Tool{webSearchTool},
        SystemPrompt: "You are a research assistant.",
        Middlewares:  []agentcore.ReActMiddleware{ownMiddleware},
    },
    InheritParentMiddlewares: true,
}

saMW := subagent.New([]subagent.SubAgentSpec{spec}, &subagent.Config{
    EmitInternalEvents: true,
    MaxDepth:           5,
})

cfg := &agentcore.ReActConfig[*schema.Message]{
    Model:       parentModel,
    Middlewares: []agentcore.ReActMiddleware{saMW, filesystem.New(...)},
}
saMW.BindToConfig(cfg) // mandatory: injects tools, forces inline dispatch
agent := agentcore.NewReActAgent(cfg)
```

### Full loop example

See [examples/workflow/loop/](examples/workflow/loop/).

```go
wf, err := agentcore.NewLoop(ctx, &agentcore.LoopConfig{
    Name:          "reflection_agent",
    SubAgents:     []agentcore.Agent{mainAgent, critiqueAgent},
    MaxIterations: 5,
})
runner := agentcore.NewTypedRunner(agentcore.RunnerConfig[*schema.Message]{Agent: wf})
iter := runner.Query(ctx, "briefly introduce multimodal embedding models")
```

### Custom Middleware

```go
type LoggingMiddleware struct {
    agentcore.BaseMiddleware[*schema.Message]
}
func (m *LoggingMiddleware) BeforeModelRewrite(
    ctx context.Context,
    state *agentcore.ReActAgentState,
    mc *agentcore.ModelContext,
) (context.Context, *agentcore.ReActAgentState, error) {
    log.Printf("model input: %d messages", len(state.Messages))
    return ctx, state, nil
}
func (m *LoggingMiddleware) AfterModelRewrite(
    ctx context.Context,
    state *agentcore.ReActAgentState,
    mc *agentcore.ModelContext,
) (context.Context, *agentcore.ReActAgentState, error) {
    log.Printf("model output: %d messages", len(state.Messages))
    return ctx, state, nil
}
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.
