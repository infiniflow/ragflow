// Package types provides core types for LangGraph Go.
package types

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// NodeTriggerMode controls when a graph node is triggered for execution.
type NodeTriggerMode string

const (
	// NodeTriggerAnyPredecessor is the default Pregel/BSP mode: a node is triggered
	// when any of its incoming edges' source nodes complete. Supports cycles/loops.
	NodeTriggerAnyPredecessor NodeTriggerMode = "any"

	// NodeTriggerAllPredecessor is DAG mode: a node is triggered only when ALL of
	// its incoming edges' source nodes have completed. Does not support cycles.
	// This is the correct mode for fan-in/convergence patterns.
	NodeTriggerAllPredecessor NodeTriggerMode = "all"
)

// StreamMode defines how the stream method should emit outputs.
type StreamMode string

const (
	// StreamModeValues emits all values in the state after each step, including interrupts.
	StreamModeValues StreamMode = "values"
	// StreamModeUpdates emits only the node or task names and updates returned by the nodes or tasks after each step.
	StreamModeUpdates StreamMode = "updates"
	// StreamModeCustom emits custom data using StreamWriter from inside nodes or tasks.
	StreamModeCustom StreamMode = "custom"
	// StreamModeMessages emits LLM messages token-by-token together with metadata for any LLM invocations inside nodes or tasks.
	StreamModeMessages StreamMode = "messages"
	// StreamModeCheckpoints emits an event when a checkpoint is created.
	StreamModeCheckpoints StreamMode = "checkpoints"
	// StreamModeTasks emits events when tasks start and finish, including their results and errors.
	StreamModeTasks StreamMode = "tasks"
	// StreamModeDebug emits checkpoints and tasks events for debugging purposes.
	StreamModeDebug StreamMode = "debug"
)

// Durability mode for the graph execution.
type Durability string

const (
	// DurabilitySync persists changes synchronously before the next step starts.
	DurabilitySync Durability = "sync"
	// DurabilityAsync persists changes asynchronously while the next step executes.
	DurabilityAsync Durability = "async"
	// DurabilityExit persists changes only when the graph exits.
	DurabilityExit Durability = "exit"
)

// All is a special value to indicate that graph should interrupt on all nodes.
const All = "*"

// RetryPolicy configures retrying nodes.
type RetryPolicy struct {
	// InitialInterval is the amount of time that must elapse before the first retry occurs.
	InitialInterval time.Duration
	// BackoffFactor is the multiplier by which the interval increases after each retry.
	BackoffFactor float64
	// MaxInterval is the maximum amount of time that may elapse between retries.
	MaxInterval time.Duration
	// MaxAttempts is the maximum number of attempts to make before giving up, including the first.
	MaxAttempts int
	// Jitter indicates whether to add random jitter to the interval between retries.
	Jitter bool
	// RetryOn is a function that returns true for exceptions that should trigger a retry.
	RetryOn func(error) bool
}

// DefaultRetryPolicy returns a default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		InitialInterval: 500 * time.Millisecond,
		BackoffFactor:   2.0,
		MaxInterval:     128 * time.Second,
		MaxAttempts:     3,
		Jitter:          true,
		RetryOn:         DefaultRetryOn,
	}
}

// CalculateBackoff computes the exponential backoff duration for the given attempt number
// (1-indexed). It applies the BackoffFactor, caps at MaxInterval, and optionally adds jitter.
// This is the shared backoff calculation used by both Pregel graph-node retries and
// agent-level model-call retries.
func (p *RetryPolicy) CalculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(float64(p.InitialInterval) * powFloat(p.BackoffFactor, attempt-1))
	if backoff > p.MaxInterval {
		backoff = p.MaxInterval
	}
	if p.Jitter {
		// Subtract up to 50% to spread retry bursts.
		jitter := time.Duration(float64(backoff) * 0.5 * randFloat())
		backoff = backoff - jitter
		if backoff < 0 {
			backoff = 0
		}
	}
	return backoff
}

// powFloat computes base^exp for float base (small int exponents).
func powFloat(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// randFloat returns a random float in [0,1).
func randFloat() float64 {
	return rand.Float64()
}

// DefaultRetryOn is the default retry condition function.
func DefaultRetryOn(err error) bool {
	return true
}

// CachePolicy configures caching nodes.
type CachePolicy struct {
	// KeyFunc generates a cache key from the node's input.
	KeyFunc func(context.Context, interface{}) string
	// TTL is the time to live for the cache entry in seconds.
	// If nil, the entry never expires.
	TTL *time.Duration
}

// DefaultCacheKey generates a default cache key.
func DefaultCacheKey(ctx context.Context, input interface{}) string {
	return fmt.Sprintf("%v", input)
}

// Interrupt represents information about an interrupt that occurred in a node.
type Interrupt struct {
	// Value is the value associated with the interrupt.
	Value interface{}
	// ID is the ID of the interrupt. Can be used to resume the interrupt directly.
	ID string
}

// NewInterrupt creates a new Interrupt with the given value and ID.
func NewInterrupt(value interface{}, id string) *Interrupt {
	return &Interrupt{
		Value: value,
		ID:    id,
	}
}

// StateUpdate represents an update to the graph state.
type StateUpdate struct {
	Values map[string]interface{}
	AsNode string
	TaskID string
}

// PregelTask represents a Pregel task.
type PregelTask struct {
	ID         string
	Name       string
	Path       []interface{}
	Error      error
	Interrupts []*Interrupt
	State      interface{} // RunnableConfig or StateSnapshot
	Result     interface{}
}

// CacheKey is the cache key for a task.
type CacheKey struct {
	// Namespace for the cache entry.
	NS []string
	// Key for the cache entry.
	Key string
	// TTL is the time to live for the cache entry in seconds.
	TTL *time.Duration
}

// PregelExecutableTask represents an executable task in Pregel.
type PregelExecutableTask struct {
	Name        string
	Input       interface{}
	Proc        interface{} // Runnable
	Writes      [][2]interface{}
	Config      map[string]interface{}
	Triggers    []string
	RetryPolicy []RetryPolicy
	CacheKey    *CacheKey
	ID          string
	Path        []interface{}
	Writers     []interface{}
	Subgraphs   []interface{} // PregelProtocol
}

// StateSnapshot is a snapshot of the state of the graph at the beginning of a step.
type StateSnapshot struct {
	// Values are the current values of channels.
	Values interface{}
	// Next is the name of the node to execute in each task for this step.
	Next []string
	// Config used to fetch this snapshot.
	Config map[string]interface{}
	// Metadata associated with this snapshot.
	Metadata interface{}
	// CreatedAt is the timestamp of snapshot creation.
	CreatedAt string
	// ParentConfig is the config used to fetch the parent snapshot, if any.
	ParentConfig map[string]interface{}
	// Tasks to execute in this step. If already attempted, may contain an error.
	Tasks []*PregelTask
	// Interrupts that occurred in this step that are pending resolution.
	Interrupts []*Interrupt
}

// Send represents a message or packet to send to a specific node in the graph.
type Send struct {
	// Node is the name of the target node to send the message to.
	Node string
	// Arg is the state or message to send to the target node.
	Arg interface{}
}

// NewSend creates a new Send instance.
func NewSend(node string, arg interface{}) *Send {
	return &Send{
		Node: node,
		Arg:  arg,
	}
}

// Command represents one or more commands to update the graph's state and send messages to nodes.
type Command struct {
	// Graph is the graph to send the command to.
	// Supported values are:
	//   - nil/empty: the current graph
	//   - "__parent__": closest parent graph
	Graph string
	// Update to apply to the graph's state.
	Update interface{}
	// Resume value to resume execution with.
	Resume interface{}
	// Goto can be:
	//   - Name of the node to navigate to next
	//   - Sequence of node names to navigate to next
	//   - Send object
	//   - Sequence of Send objects
	Goto interface{}
}

// Parent is the constant for the parent graph.
const Parent = "__parent__"

// NewCommand creates a new Command.
func NewCommand() *Command {
	return &Command{}
}

// WithGraph sets the graph for the command.
func (c *Command) WithGraph(graph string) *Command {
	c.Graph = graph
	return c
}

// WithUpdate sets the update for the command.
func (c *Command) WithUpdate(update interface{}) *Command {
	c.Update = update
	return c
}

// WithResume sets the resume value for the command.
func (c *Command) WithResume(resume interface{}) *Command {
	c.Resume = resume
	return c
}

// WithGoto sets the goto value for the command.
func (c *Command) WithGoto(gotoVal interface{}) *Command {
	c.Goto = gotoVal
	return c
}

// UpdateAsTuples converts the update to tuples.
func (c *Command) UpdateAsTuples() [][2]interface{} {
	if c.Update == nil {
		return nil
	}

	switch v := c.Update.(type) {
	case map[string]interface{}:
		result := make([][2]interface{}, 0, len(v))
		for key, val := range v {
			result = append(result, [2]interface{}{key, val})
		}
		return result
	default:
		return [][2]interface{}{{"__root__", v}}
	}
}

// StreamWriter is a function that accepts a single argument and writes it to the output stream.
type StreamWriter func(interface{})

// Checkpointer represents the type of checkpointer to use for a subgraph.
type Checkpointer interface {
	// IsCheckpointer marks this as a checkpointer type.
	IsCheckpointer()
}

// CheckpointerBool is a boolean checkpointer type.
type CheckpointerBool bool

func (c CheckpointerBool) IsCheckpointer() {}

// RunnableConfig is defined in config.go

// Overwrite wraps a value to bypass a reducer and write directly to a channel.
type Overwrite struct {
	Value interface{}
}

// NewOverwrite creates a new Overwrite.
func NewOverwrite(value interface{}) *Overwrite {
	return &Overwrite{Value: value}
}

// ReducerFunc reduces multiple values into one.
type ReducerFunc func(current, update interface{}) interface{}

// NodeFunc is the signature of a node function.
type NodeFunc func(context.Context, interface{}) (interface{}, error)

// EdgeFunc is the signature of an edge/condition function.
type EdgeFunc func(context.Context, interface{}) (interface{}, error)

// ============================================================
// Graph type definitions (shared by graph/graph and pregel)
// ============================================================

// Node represents a node in a StateGraph.
type Node struct {
	Name         string
	Function     NodeFunc
	Triggers     []string
	Writes       []string
	RetryPolicy  *RetryPolicy
	Tags         []string
	Metadata     map[string]interface{}
	FieldMapping []FieldMapping
}

// Edge is a directed connection between two nodes.
type Edge struct {
	From string
	To   string
}

// FieldMapping specifies how a field from a source node's output is mapped
// to a target node's input.
type FieldMapping struct {
	From string
	To   string
}

// DataEdge is a directed data-flow connection with field-level mapping.
type DataEdge struct {
	From    string
	To      string
	Mapping []FieldMapping
}

// ConditionalEdge routes to different nodes based on a condition.
type ConditionalEdge struct {
	From      string
	Condition EdgeFunc
	Mapping   map[string]string
}

// Branch provides a higher-level conditional edge.
type Branch struct {
	From      string
	Condition EdgeFunc
	Then      func(interface{}) []string
}

// NodeOptions contains options for adding a node.
type NodeOptions struct {
	RetryPolicy  *RetryPolicy
	Tags         []string
	Metadata     map[string]interface{}
	Triggers     []string
	Writes       []string
	FieldMapping []FieldMapping
	StatePre     NodeFunc
	StatePost    NodeFunc
}

// StateGraph is the interface for graph building and inspection.
type StateGraph interface {
	AddNode(name string, fn NodeFunc) *Node
	AddNodeWithOptions(name string, fn NodeFunc, opts NodeOptions) *Node
	AddEdge(from, to string) error
	AddConditionalEdges(from string, condition EdgeFunc, mapping map[string]string) error
	AddBranch(from string, condition EdgeFunc, then func(interface{}) []string) error
	AddDataEdge(from, to string, mappings ...FieldMapping) error
	AddChannel(name string, channel interface{}) // channel must be channels.Channel
	SetReducer(channelName string, reducer ReducerFunc)
	AddChannelWithReducer(name string, channel interface{}, reducer ReducerFunc)
	SetEntryPoint(node string) error
	SetFinishPoint(node string) error
	WithInputSchema(schema interface{}) StateGraph
	WithOutputSchema(schema interface{}) StateGraph
	SetNodeTriggerMode(mode NodeTriggerMode)
	Compile(opts ...interface{}) (CompiledGraph, error)
	Validate() error

	GetChannels() map[string]interface{}
	GetEntryPoint() string
	GetNode(name string) (*Node, bool)
	GetEdges() []*Edge
	GetConditionalEdges() []*ConditionalEdge
	GetBranches() []*Branch
	GetNodes() map[string]*Node
	GetNodeTriggerMode() NodeTriggerMode
	GetDataEdges() []*DataEdge

	// GetStateSchema returns the raw state schema (struct, map, etc.)
	GetStateSchema() interface{}
}

// CompiledGraph is the interface for executing a compiled graph.
type CompiledGraph interface {
	Invoke(ctx context.Context, input interface{}, config ...*RunnableConfig) (interface{}, error)
	Stream(ctx context.Context, input interface{}, mode StreamMode, config ...*RunnableConfig) (<-chan interface{}, <-chan error)
	GetGraph() StateGraph
	GetCheckpointer() interface{} // cast to checkpoint.BaseCheckpointer
	GetInterrupts() map[string]bool
	GetInterruptsAfter() map[string]bool
	GetRecursionLimit() int
	IsDebug() bool
}

// PregelRunFunc is the pluggable pregel execution function.
// Set by pregel.init() via SetPregelRunFunc.
var PregelRunFunc func(ctx context.Context, cg CompiledGraph, input interface{}, config *RunnableConfig, streamMode StreamMode) (interface{}, error)

// SetPregelRunFunc sets the pregel execution function for compiled graphs.
func SetPregelRunFunc(fn func(ctx context.Context, cg CompiledGraph, input interface{}, config *RunnableConfig, streamMode StreamMode) (interface{}, error)) {
	PregelRunFunc = fn
}
