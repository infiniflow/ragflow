// Package graph provides graph building capabilities for Harness-Go.
package graph

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// Node represents a node in the graph. Each node is a callable function that
// receives the current shared state and returns a (possibly modified) state.
// Nodes are connected by edges which determine execution order.
type Node struct {
	// Name is a unique identifier for this node within the graph.
	Name string
	// Function is the node's executable body. It receives context and state,
	// and returns the new state or an error.
	Function types.NodeFunc
	// Triggers lists channel names this node reads from.
	Triggers []string
	// Writes lists channel names this node writes to.
	Writes   []string
	// RetryPolicy configures automatic retry for this node.
	RetryPolicy *types.RetryPolicy
	// Tags are opaque labels for filtering and debugging.
	Tags []string
	// Metadata holds arbitrary key-value pairs for tooling.
	Metadata map[string]interface{}
	// FieldMapping specifies field-level routing for this node's output.
	// Used by the engine to route only specific fields through data edges.
	FieldMapping []FieldMapping
}

// Edge is a directed connection between two nodes. After the From node
// completes, execution proceeds to the To node. Use constants.Start and
// constants.End for the virtual start/end nodes.
//
// Example:
//
//	sg.AddEdge("node_a", "node_b")    // node_a always flows to node_b
//	sg.AddEdge("node_b", "__end__")    // node_b is a terminal node
type Edge struct {
	From string
	To   string
}

// FieldMapping specifies how a field from a source node's output is mapped
// to a target node's input. Supports dotted paths like "a.b.c" for nested access.
//
// Example:
//
//	FieldMapping{From: "response.text", To: "input.query"}
type FieldMapping struct {
	From string // source field path (dotted notation, empty = pass entire state)
	To   string // target field path (dotted notation, empty = set at root)
}

// DataEdge is a directed data-flow connection with field-level mapping.
// It allows fine-grained control over which fields flow between nodes.
// Unlike Edge (control flow), DataEdge only routes data without affecting
// execution order. Control flow is determined by Edge/conditionalEdge alone.
type DataEdge struct {
	From    string
	To      string
	Mapping []FieldMapping
}

// ConditionalEdge allows routing to different nodes based on a condition
// function. The Condition function is evaluated after the From node completes;
// its return value is looked up in Mapping to determine the next node.
//
// Example:
//
//	sg.AddConditionalEdges("router",
//	    func(ctx context.Context, state any) (any, error) {
//	        return state.(MyState).Route, nil
//	    },
//	    map[string]string{
//	        "path_a": "node_a",
//	        "path_b": "node_b",
//	        "__end__": "__end__",
//	    },
//	)
type ConditionalEdge struct {
	From      string
	Condition types.EdgeFunc
	// Mapping from condition result to target node name.
	Mapping map[string]string
}

// Branch provides a higher-level conditional edge. The Condition function
// is evaluated, and Then receives the result to produce zero or more target
// node names. Unlike ConditionalEdge, Branch supports single-source fan-out.
type Branch struct {
	From      string
	Condition types.EdgeFunc
	// Then is called with the condition result to determine next nodes.
	Then func(interface{}) []string
}

// Send represents a dynamic node invocation. It is used with StateGraph's
// dynamic routing to invoke a named node with a specific argument, bypassing
// the normal state channel. This enables map-reduce and fan-out patterns
// where different nodes receive different subsets of the state.
type Send struct {
	Node string
	Arg  interface{}
}

// StateGraph is a graph whose nodes communicate by reading and writing to a shared state.
//
// Nodes execute sequentially or conditionally based on directed edges. Each node
// receives the current state (a map or struct matching the schema) and returns
// an updated state. The framework merges returned values into channels using
// configured reducers.
//
// Usage:
//
//	// Define state schema
//	type MyState struct { Messages []string }
//
//	builder := NewStateGraph(MyState{})
//	builder.AddNode("agent", func(ctx context.Context, state interface{}) (interface{}, error) {
//	    s := state.(MyState)
//	    s.Messages = append(s.Messages, "hello")
//	    return s, nil
//	})
//	builder.AddEdge("__start__", "agent")
//	builder.AddEdge("agent", "__end__")
//	compiled, err := builder.Compile()
type StateGraph struct {
	// Nodes in the graph
	nodes map[string]*Node
	// Edges between nodes
	edges []*Edge
	// Data edges for field-level routing
	dataEdges []*DataEdge
	// Conditional edges
	conditionalEdges []*ConditionalEdge
	// Branches
	branches []*Branch
	// Entry point of the graph
	entryPoint string
	// Finish points of the graph
	finishPoints []string
	// Channel definitions for the state schema
	channels map[string]channels.Channel
	// Reducer functions for channels
	reducers map[string]types.ReducerFunc
	// State schema type
	stateSchema interface{}
	// Input schema type
	inputSchema interface{}
	// Output schema type
	outputSchema interface{}
	// NodeTriggerMode controls how nodes are triggered for execution.
	NodeTriggerMode types.NodeTriggerMode
}

// NewStateGraph creates a new StateGraph with the given state schema.
// The stateSchema defines the structure of the shared state.
func NewStateGraph(stateSchema interface{}) *StateGraph {
	return &StateGraph{
		nodes:            make(map[string]*Node),
		edges:            make([]*Edge, 0),
		conditionalEdges: make([]*ConditionalEdge, 0),
		branches:         make([]*Branch, 0),
		finishPoints:     make([]string, 0),
		channels:         make(map[string]channels.Channel),
		reducers:         make(map[string]types.ReducerFunc),
		stateSchema:      stateSchema,
		inputSchema:      stateSchema,
		outputSchema:     stateSchema,
	}
}

// WithInputSchema sets the input schema for the graph.
func (g *StateGraph) WithInputSchema(schema interface{}) *StateGraph {
	g.inputSchema = schema
	return g
}

// WithOutputSchema sets the output schema for the graph.
func (g *StateGraph) WithOutputSchema(schema interface{}) *StateGraph {
	g.outputSchema = schema
	return g
}

// AddNode adds a node to the graph.
func (g *StateGraph) AddNode(name string, fn types.NodeFunc) *Node {
	node := &Node{
		Name:     name,
		Function: fn,
		Triggers: make([]string, 0),
		Writes:   make([]string, 0),
		Tags:     make([]string, 0),
		Metadata: make(map[string]interface{}),
	}
	g.nodes[name] = node
	return node
}

// AddNodeWithOptions adds a node with options.
func (g *StateGraph) AddNodeWithOptions(name string, fn types.NodeFunc, opts NodeOptions) *Node {
	// Apply StatePre/StatePost wrappers around the node function.
	if opts.StatePre != nil || opts.StatePost != nil {
		orig := fn
		pre := opts.StatePre
		post := opts.StatePost
		fn = func(ctx context.Context, state interface{}) (interface{}, error) {
			if pre != nil {
				var err error
				state, err = pre(ctx, state)
				if err != nil {
					return nil, fmt.Errorf("state pre-handler for '%s': %w", name, err)
				}
			}
			out, err := orig(ctx, state)
			if err != nil {
				return nil, err
			}
			if post != nil {
				out, err = post(ctx, out)
				if err != nil {
					return nil, fmt.Errorf("state post-handler for '%s': %w", name, err)
				}
			}
			return out, nil
		}
	}

	node := g.AddNode(name, fn)
	if opts.RetryPolicy != nil {
		node.RetryPolicy = opts.RetryPolicy
	}
	if len(opts.Tags) > 0 {
		node.Tags = append(node.Tags, opts.Tags...)
	}
	if len(opts.Metadata) > 0 {
		for k, v := range opts.Metadata {
			node.Metadata[k] = v
		}
	}
	if len(opts.Triggers) > 0 {
		node.Triggers = append(node.Triggers, opts.Triggers...)
	}
	if len(opts.Writes) > 0 {
		node.Writes = append(node.Writes, opts.Writes...)
	}
	if len(opts.FieldMapping) > 0 {
		node.FieldMapping = append(node.FieldMapping, opts.FieldMapping...)
	}
	return node
}

// NodeOptions contains options for adding a node.
type NodeOptions struct {
	RetryPolicy  *types.RetryPolicy
	Tags         []string
	Metadata     map[string]interface{}
	Triggers     []string
	Writes       []string
	FieldMapping []FieldMapping          // field-level routing for this node's output
	StatePre     types.NodeFunc          // transforms state BEFORE node execution
	StatePost    types.NodeFunc          // transforms state AFTER node execution
}

// WithStatePreHandler wraps the node with a pre-execution state transform.
// The handler receives the incoming state and can modify it before the node runs.
func WithStatePreHandler(fn types.NodeFunc) func(*NodeOptions) {
	return func(opts *NodeOptions) { opts.StatePre = fn }
}

// WithStatePostHandler wraps the node with a post-execution state transform.
// The handler receives the node's output state and can modify it before it flows downstream.
func WithStatePostHandler(fn types.NodeFunc) func(*NodeOptions) {
	return func(opts *NodeOptions) { opts.StatePost = fn }
}

// WithFieldMapping sets field-level routing for this node's output.
func WithFieldMapping(mappings ...FieldMapping) func(*NodeOptions) {
	return func(opts *NodeOptions) { opts.FieldMapping = append(opts.FieldMapping, mappings...) }
}

// MapFields is a convenience function to create a FieldMapping from a source path to a target path.
func MapFields(from, to string) FieldMapping {
	return FieldMapping{From: from, To: to}
}

// MapTo is a convenience function to create a FieldMapping that maps the entire output to a target path.
func MapTo(to string) FieldMapping {
	return FieldMapping{To: to}
}

// AddEdge adds an edge between two nodes.
func (g *StateGraph) AddEdge(from, to string) error {
	if _, ok := g.nodes[from]; !ok && from != constants.Start {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	if _, ok := g.nodes[to]; !ok && to != constants.End {
		return &errors.NodeNotFoundError{NodeName: to}
	}
	
	g.edges = append(g.edges, &Edge{From: from, To: to})
	
	// If this is an edge from Start, set entry point to the target node
	if from == constants.Start {
		g.entryPoint = to
	}
	
	// If this is an edge to End, set the source as a finish point
	if to == constants.End {
		found := false
		for _, fp := range g.finishPoints {
			if fp == from {
				found = true
				break
			}
		}
		if !found {
			g.finishPoints = append(g.finishPoints, from)
		}
	}
	
	return nil
}

// AddConditionalEdges adds conditional edges from a node.
func (g *StateGraph) AddConditionalEdges(from string, condition types.EdgeFunc, mapping map[string]string) error {
	if _, ok := g.nodes[from]; !ok {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	
	// Validate all targets exist
	for _, target := range mapping {
		if _, ok := g.nodes[target]; !ok && target != constants.End {
			return &errors.NodeNotFoundError{NodeName: target}
		}
	}
	
	g.conditionalEdges = append(g.conditionalEdges, &ConditionalEdge{
		From:      from,
		Condition: condition,
		Mapping:   mapping,
	})
	return nil
}

// AddBranch adds a branch from a node.
func (g *StateGraph) AddBranch(from string, condition types.EdgeFunc, then func(interface{}) []string) error {
	if _, ok := g.nodes[from]; !ok {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	
	g.branches = append(g.branches, &Branch{
		From:      from,
		Condition: condition,
		Then:      then,
	})
	return nil
}

// AddDataEdge adds a data-flow edge with optional field-level mappings between two nodes.
// Unlike AddEdge (control flow), AddDataEdge only routes data without affecting execution order.
func (g *StateGraph) AddDataEdge(from, to string, mappings ...FieldMapping) error {
	if _, ok := g.nodes[from]; !ok && from != constants.Start {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	if _, ok := g.nodes[to]; !ok && to != constants.End {
		return &errors.NodeNotFoundError{NodeName: to}
	}
	g.dataEdges = append(g.dataEdges, &DataEdge{From: from, To: to, Mapping: mappings})
	return nil
}

// GetDataEdges returns all data edges in the graph.
func (g *StateGraph) GetDataEdges() []*DataEdge {
	return g.dataEdges
}

// SetEntryPoint sets the entry point of the graph.
func (g *StateGraph) SetEntryPoint(node string) error {
	if _, ok := g.nodes[node]; !ok {
		return &errors.NodeNotFoundError{NodeName: node}
	}
	g.entryPoint = node
	return nil
}

// SetFinishPoint sets a finish point of the graph.
func (g *StateGraph) SetFinishPoint(node string) error {
	if _, ok := g.nodes[node]; !ok {
		return &errors.NodeNotFoundError{NodeName: node}
	}
	g.finishPoints = append(g.finishPoints, node)
	return nil
}

// AddChannel adds a channel definition to the state schema.
func (g *StateGraph) AddChannel(name string, channel channels.Channel) {
	channel.SetKey(name)
	g.channels[name] = channel
}

// SetReducer sets a reducer function for a channel.
// If the channel exists, it wraps it with a ReducerChannel.
func (g *StateGraph) SetReducer(channelName string, reducer types.ReducerFunc) {
	if channel, ok := g.channels[channelName]; ok {
		// Wrap existing channel with reducer
		g.channels[channelName] = channels.NewReducerChannel(channel, reducer)
	}
	g.reducers[channelName] = reducer
}

// AddChannelWithReducer adds a channel definition with a reducer function.
func (g *StateGraph) AddChannelWithReducer(name string, channel channels.Channel, reducer types.ReducerFunc) {
	channel.SetKey(name)
	if reducer != nil {
		// Wrap channel with reducer
		g.channels[name] = channels.NewReducerChannel(channel, reducer)
		g.reducers[name] = reducer
	} else {
		g.channels[name] = channel
	}
}

// GetNode returns a node by name.
func (g *StateGraph) GetNode(name string) (*Node, bool) {
	node, ok := g.nodes[name]
	return node, ok
}

// GetNodes returns all nodes.
func (g *StateGraph) GetNodes() map[string]*Node {
	return g.nodes
}

// GetEdges returns all edges.
func (g *StateGraph) GetEdges() []*Edge {
	return g.edges
}

// GetChannels returns all channels.
func (g *StateGraph) GetChannels() map[string]channels.Channel {
	return g.channels
}

// GetEntryPoint returns the entry point node name.
func (g *StateGraph) GetEntryPoint() string {
	return g.entryPoint
}

// GetConditionalEdges returns all conditional edges.
func (g *StateGraph) GetConditionalEdges() []*ConditionalEdge {
	return g.conditionalEdges
}

// GetBranches returns all branches.
func (g *StateGraph) GetBranches() []*Branch {
	return g.branches
}

// Validate validates the graph structure.
func (g *StateGraph) Validate() error {
	if g.entryPoint == "" {
		return fmt.Errorf("no entry point set")
	}
	
	if len(g.finishPoints) == 0 {
		return fmt.Errorf("no finish points set")
	}
	
	// Check that all nodes are reachable
	reachable := g.computeReachable()
	for name := range g.nodes {
		if !reachable[name] {
			return fmt.Errorf("node %s is not reachable from entry point", name)
		}
	}
	
	// Validate state schema
	if err := g.ValidateStateSchema(); err != nil {
		return fmt.Errorf("state schema validation failed: %w", err)
	}
	
	return nil
}

// computeReachable computes all reachable nodes from the entry point.
func (g *StateGraph) computeReachable() map[string]bool {
	reachable := make(map[string]bool)
	if g.entryPoint == "" {
		return reachable
	}
	
	queue := []string{g.entryPoint}
	reachable[g.entryPoint] = true
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		// Follow regular edges
		for _, edge := range g.edges {
			if edge.From == current && !reachable[edge.To] && edge.To != constants.End {
				reachable[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
		
		// Follow conditional edges - all targets are potentially reachable
		for _, condEdge := range g.conditionalEdges {
			if condEdge.From == current {
				for _, target := range condEdge.Mapping {
					if _, ok := g.nodes[target]; ok && !reachable[target] && target != constants.End {
						reachable[target] = true
						queue = append(queue, target)
					}
				}
			}
		}
		
		// Note: branches are truly dynamic and can't be statically verified
	}
	
	return reachable
}

// configureChannelsFromSchema configures channels and reducers based on state schema annotations.
func (g *StateGraph) configureChannelsFromSchema() error {
	// Get field information from schema
	fieldInfos, err := g.GetStateSchemaInfo()
	if err != nil {
		return err
	}

	// Configure channels and reducers for each field
	for fieldName, info := range fieldInfos {
		// Check if channel already exists
		if _, exists := g.channels[fieldName]; !exists {
			// Add channel
			g.channels[fieldName] = info.Channel
		}

		// Set reducer if specified in annotation
		if info.Annotation != nil && info.Annotation.Reducer != nil {
			g.reducers[fieldName] = info.Annotation.Reducer
		}
	}

	return nil
}

// Compile validates the graph structure and produces an executable CompiledGraph.
// Validation includes reachability checks (all nodes reachable from the entry point),
// state schema validation, and channel configuration from struct annotations.
//
// opts configure runtime behavior:
//   - WithCheckpointer: enable persistence for interrupt/resume
//   - WithInterrupts: set human-in-the-loop breakpoints
//   - WithRecursionLimit: cap Pregel iterations (default 25)
//   - WithDebug: enable verbose execution logging
//
// Example:
//
//	cg, err := sg.Compile(
//	    graph.WithCheckpointer(mySaver),
//	    graph.WithInterrupts("human_review"),
//	)
func (g *StateGraph) Compile(opts ...CompileOption) (*CompiledGraph, error) {
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %w", err)
	}
	
	// Configure channels and reducers from schema annotations
	if err := g.configureChannelsFromSchema(); err != nil {
		return nil, fmt.Errorf("failed to configure channels from schema: %w", err)
	}
	
	cg := &CompiledGraph{
		graph:           g,
		checkpointer:    nil,
		interrupts:      make(map[string]bool),
		recursionLimit:  constants.DefaultRecursionLimit,
		debug:           false,
		nodeTriggerMode: types.NodeTriggerAnyPredecessor,
	}
	
	for _, opt := range opts {
		opt(cg)
	}
	
	// Propagate node trigger mode to the graph for the engine to access.
	g.NodeTriggerMode = cg.nodeTriggerMode

	return cg, nil
}

// CompileOption configures CompiledGraph behavior at compile time.
type CompileOption func(*CompiledGraph)

// WithCheckpointer enables checkpoint-based persistence for interrupt/resume.
// The checkpointer is called at each Pregel step to save execution state.
// Built-in implementations: MemorySaver, SqliteSaver, PostgresSaver.
func WithCheckpointer(checkpointer Checkpointer) CompileOption {
	return func(cg *CompiledGraph) {
		cg.checkpointer = checkpointer
	}
}

// WithInterrupts marks one or more nodes as interrupt points (human-in-the-loop
// breakpoints). Execution pauses before these nodes and can be resumed later
// via the checkpointer. Use "*" to interrupt before every node.
func WithInterrupts(nodes ...string) CompileOption {
	return func(cg *CompiledGraph) {
		for _, node := range nodes {
			cg.interrupts[node] = true
		}
	}
}

// WithRecursionLimit sets the maximum number of Pregel iterations before the
// graph aborts with GraphRecursionError. The default is 25. Increase for deeply
// nested or iterative graphs, decrease to catch runaway loops early.
func WithRecursionLimit(limit int) CompileOption {
	return func(cg *CompiledGraph) {
		cg.recursionLimit = limit
	}
}

// WithDebug enables verbose execution logging for debugging node execution
// order, channel state transitions, and task scheduling.
func WithDebug(debug bool) CompileOption {
	return func(cg *CompiledGraph) {
		cg.debug = debug
	}
}

// WithNodeTriggerMode sets the node trigger mode for graph execution.
//   - NodeTriggerAnyPredecessor (default): Pregel/BSP mode, triggers when any
//     predecessor completes. Supports cycles and loops.
//   - NodeTriggerAllPredecessor: DAG mode, triggers only when ALL predecessors have
//     completed. Required for fan-in/convergence patterns. Does not support cycles.
func WithNodeTriggerMode(mode types.NodeTriggerMode) CompileOption {
	return func(cg *CompiledGraph) {
		cg.nodeTriggerMode = mode
	}
}

// Checkpointer is the interface for checkpoint persistence.
// It is a type alias for checkpoint.BaseCheckpointer.
type Checkpointer = checkpoint.BaseCheckpointer

// ---- Pregel runner bridge ----
//
// PregelRunFunc is the pluggable execution function for CompiledGraph.
// It allows the root harness package to inject a pregel.Engine-based runner
// without creating an import cycle (graph → pregel → graph).
//
// The default value (nil) falls back to the inline Pregel loop.
// Set it via SetPregelRunFunc, typically from an init() in the root harness package.
var PregelRunFunc func(ctx context.Context, cg *CompiledGraph, input interface{}, config *types.RunnableConfig, streamMode types.StreamMode) (interface{}, error)

// SetPregelRunFunc replaces the default Pregel execution function.
// Called from harness.go's init() to inject a pregel.Engine-based runner.
// External consumers should compile graphs via sg.Compile() and call Invoke/Stream;
// they do not need to call SetPregelRunFunc directly.
func SetPregelRunFunc(fn func(ctx context.Context, cg *CompiledGraph, input interface{}, config *types.RunnableConfig, streamMode types.StreamMode) (interface{}, error)) {
	PregelRunFunc = fn
}

// CompiledGraph is a compiled, executable graph produced by StateGraph.Compile().
//
// It provides two execution paths:
//   - Invoke: synchronous, returns final state
//   - Stream: asynchronous, returns channels for streaming events
//
// Execution delegates to PregelRunFunc (production) or falls back to an inline
// Pregel loop (backward compatibility).
//
// Example:
//
//	cg, err := sg.Compile(graph.WithCheckpointer(memSaver))
//	result, err := cg.Invoke(ctx, MyState{Messages: []string{"hello"}})
type CompiledGraph struct {
	graph            *StateGraph
	checkpointer     Checkpointer
	interrupts       map[string]bool
	recursionLimit   int
	debug            bool
	nodeTriggerMode  types.NodeTriggerMode
}

// Invoke executes the graph synchronously. It applies input to the state
// channels, runs the Pregel loop, and returns the final state after all nodes
// complete or an interrupt/error occurs.
//
// config is optional; when nil, a default RunnableConfig is used. For resumable
// execution, pass a config with ThreadID and a checkpointer configured during
// Compile().
func (cg *CompiledGraph) Invoke(ctx context.Context, input interface{}, config ...*types.RunnableConfig) (interface{}, error) {
	rc := &types.RunnableConfig{}
	if len(config) > 0 && config[0] != nil {
		rc = config[0]
	}
	
	result, err := cg.run(ctx, input, rc, types.StreamModeValues)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

// Stream executes the graph and returns channels for receiving streaming events.
// outputCh yields stream events (checkpoint snapshots, task start/end, value updates,
// or the final state depending on streamMode). errCh receives a single error or nil
// when execution completes.
//
// streamMode controls which events are emitted:
//   - StreamModeValues: final state only
//   - StreamModeUpdates: per-node state updates
//   - StreamModeTasks: task lifecycle events
//   - StreamModeCheckpoints: checkpoint snapshots
//   - StreamModeDebug: all event types
func (cg *CompiledGraph) Stream(ctx context.Context, input interface{}, mode types.StreamMode, config ...*types.RunnableConfig) (<-chan interface{}, <-chan error) {
	outputCh := make(chan interface{}, 1) // Buffer to reduce blocking
	errCh := make(chan error, 1)

	rc := &types.RunnableConfig{}
	if len(config) > 0 && config[0] != nil {
		rc = config[0]
	}

	go func() {
		defer close(outputCh)
		defer close(errCh)

		result, err := cg.run(ctx, input, rc, mode)
		if err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}

		select {
		case outputCh <- result:
		case <-ctx.Done():
		}
	}()

	return outputCh, errCh
}

// run delegates to the configured Pregel runner, or falls back to the inline
// Pregel loop when no external runner is set.
func (cg *CompiledGraph) run(ctx context.Context, input interface{}, config *types.RunnableConfig, streamMode types.StreamMode) (interface{}, error) {
	if PregelRunFunc != nil {
		return PregelRunFunc(ctx, cg, input, config, streamMode)
	}
	return cg.inlineRun(ctx, input, config)
}

// inlineRun is the default inline Pregel loop kept as a fallback.
// It is only used when no PregelRunFunc has been set via SetPregelRunFunc.
// For production use, the pregel.Engine (injected via harness.init()) provides
// full async pipeline, streaming, and checkpoint support.
// TODO: Consider moving this to a separate file or removing entirely once
// all consumers use the pregel engine path.
func (cg *CompiledGraph) inlineRun(ctx context.Context, input interface{}, config *types.RunnableConfig) (interface{}, error) {
	g := cg.graph
	channelRegistry := channels.NewRegistry()
	for name, ch := range g.GetChannels() {
		channelRegistry.Register(name, ch.Copy())
	}

	if input != nil {
		if err := inlineApplyInput(channelRegistry, input); err != nil {
			return nil, fmt.Errorf("failed to apply input: %w", err)
		}
	}

	if cg.checkpointer != nil {
		threadID := getThreadID(config)
		cp, err := cg.checkpointer.Get(ctx, map[string]interface{}{
			constants.ConfigKeyThreadID: threadID,
		})
		if err == nil && cp != nil {
			if err := channelRegistry.RestoreFromCheckpoint(cp); err != nil {
				return nil, fmt.Errorf("failed to restore from checkpoint: %w", err)
			}
		}
	}

	step := 0
	completedTasks := make(map[string]bool)
	lastCompletedNode := ""
	lastState := input

	for {
		if step >= cg.recursionLimit {
			return nil, &errors.GraphRecursionError{Limit: cg.recursionLimit}
		}

		tasks, err := inlineGetNextTasks(ctx, channelRegistry, completedTasks, lastCompletedNode, lastState, g)
		if err != nil {
			return nil, fmt.Errorf("failed to get next tasks: %w", err)
		}
		if len(tasks) == 0 {
			break
		}

		interrupted := inlineShouldInterrupt(tasks, cg.interrupts)
		if interrupted {
			if cg.checkpointer != nil {
				cp := channelRegistry.CreateCheckpoint()
				_ = cg.checkpointer.Put(ctx, map[string]interface{}{
					constants.ConfigKeyThreadID: getThreadID(config),
				}, cp)
			}
			return nil, &errors.GraphInterrupt{}
		}

		results, err := inlineExecuteTasks(ctx, tasks, g)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tasks: %w", err)
		}

		for _, result := range results {
			if result.err != nil {
				return nil, fmt.Errorf("node %s failed: %w", result.nodeName, result.err)
			}
			completedTasks[result.nodeName] = true
			lastCompletedNode = result.nodeName
			lastState = inlineMergeStates(lastState, result.output)
		}

		if err := inlineApplyWrites(channelRegistry, results); err != nil {
			return nil, fmt.Errorf("failed to apply writes: %w", err)
		}

		if cg.checkpointer != nil {
			cp := channelRegistry.CreateCheckpoint()
			_ = cg.checkpointer.Put(ctx, map[string]interface{}{
				constants.ConfigKeyThreadID: getThreadID(config),
				"step":      step,
			}, cp)
		}
		step++
	}

	finalState, err := inlineBuildOutput(channelRegistry, lastState)
	if err != nil {
		return nil, fmt.Errorf("failed to build output: %w", err)
	}
	return finalState, nil
}

// GetGraph returns the underlying StateGraph for read-only inspection.
func (cg *CompiledGraph) GetGraph() *StateGraph {
	return cg.graph
}

// GetCheckpointer returns the configured checkpointer, or nil if none was set.
func (cg *CompiledGraph) GetCheckpointer() Checkpointer {
	return cg.checkpointer
}

// GetInterrupts returns the set of node names that are configured to interrupt
// execution (human-in-the-loop breakpoints).
func (cg *CompiledGraph) GetInterrupts() map[string]bool {
	return cg.interrupts
}

// GetRecursionLimit returns the maximum number of Pregel steps before the
// graph aborts with a GraphRecursionError.
func (cg *CompiledGraph) GetRecursionLimit() int {
	return cg.recursionLimit
}

// IsDebug reports whether debug mode is enabled for detailed execution logging.
func (cg *CompiledGraph) IsDebug() bool {
	return cg.debug
}

// ---- Inline Pregel execution helpers (fallback when no external runner is set) ----

type inlineTask struct {
	id       string
	nodeName string
	input    interface{}
}

type inlineTaskResult struct {
	taskID   string
	nodeName string
	output   interface{}
	err      error
}

func getThreadID(config *types.RunnableConfig) string {
	if config != nil && config.Configurable != nil {
		if tid, ok := config.Configurable[constants.ConfigKeyThreadID].(string); ok {
			return tid
		}
	}
	return uuid.New().String()
}

func inlineApplyInput(registry *channels.Registry, input interface{}) error {
	inputMap, err := inlineToMap(input)
	if err != nil {
		return err
	}
	writes := make(map[string][]interface{})
	for key, value := range inputMap {
		if _, ok := registry.Get(key); ok {
			writes[key] = []interface{}{value}
		}
	}
	if len(writes) > 0 {
		return registry.UpdateChannels(writes)
	}
	return nil
}

func inlineGetNextTasks(ctx context.Context, registry *channels.Registry, completedTasks map[string]bool, lastCompletedNode string, currentState interface{}, g *StateGraph) ([]*inlineTask, error) {
	tasks := make([]*inlineTask, 0)
	if len(completedTasks) == 0 && g.entryPoint != "" {
		node, ok := g.GetNode(g.entryPoint)
		if !ok {
			return nil, &errors.NodeNotFoundError{NodeName: g.entryPoint}
		}
		tasks = append(tasks, &inlineTask{id: uuid.New().String(), nodeName: node.Name, input: currentState})
		return tasks, nil
	}
	if lastCompletedNode != "" {
		nextNodes := make(map[string]bool)
		for _, condEdge := range g.conditionalEdges {
			if condEdge.From == lastCompletedNode {
				conditionResult, err := condEdge.Condition(ctx, currentState)
				if err != nil {
					return nil, fmt.Errorf("condition evaluation failed for node %s: %w", lastCompletedNode, err)
				}
				conditionKey := fmt.Sprintf("%v", conditionResult)
				targetNode, ok := condEdge.Mapping[conditionKey]
				if !ok {
					return nil, fmt.Errorf("no mapping for condition result %s from node %s", conditionKey, lastCompletedNode)
				}
				if targetNode == constants.End {
					return tasks, nil
				}
				// BSP mode: always schedule conditional edge targets, even if previously completed.
				// The conditional router can dynamically route to different nodes each time.
				nextNodes[targetNode] = true
			}
		}
		if len(nextNodes) == 0 {
			for _, edge := range g.edges {
				if edge.From == lastCompletedNode {
					if edge.To == constants.End {
						return tasks, nil
					}
					// BSP loop edges: always schedule, even if previously completed.
					// completedTasks only prevents re-scheduling the SAME node,
					// not nodes reached via outgoing edges (support loops).
					nextNodes[edge.To] = true
				}
			}
		}
		for nodeName := range nextNodes {
			node, ok := g.GetNode(nodeName)
			if ok {
				tasks = append(tasks, &inlineTask{id: uuid.New().String(), nodeName: node.Name, input: currentState})
			}
		}
	}
	return tasks, nil
}

func inlineShouldInterrupt(tasks []*inlineTask, interrupts map[string]bool) bool {
	if len(interrupts) == 0 {
		return false
	}
	interruptAll := interrupts[types.All]
	for _, t := range tasks {
		if interruptAll || interrupts[t.nodeName] {
			return true
		}
	}
	return false
}

func inlineExecuteTasks(ctx context.Context, tasks []*inlineTask, g *StateGraph) ([]*inlineTaskResult, error) {
	results := make([]*inlineTaskResult, 0, len(tasks))
	for _, t := range tasks {
		node, ok := g.GetNode(t.nodeName)
		if !ok {
			return nil, &errors.NodeNotFoundError{NodeName: t.nodeName}
		}
		var output interface{}
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("node %s panic: %v", t.nodeName, r)
				}
			}()
			output, err = node.Function(ctx, t.input)
		}()
		results = append(results, &inlineTaskResult{taskID: t.id, nodeName: t.nodeName, output: output, err: err})
	}
	return results, nil
}

func inlineApplyWrites(registry *channels.Registry, results []*inlineTaskResult) error {
	writes := make(map[string][]interface{})
	for _, result := range results {
		if result.err != nil {
			continue
		}
		outputMap, err := inlineToMap(result.output)
		if err != nil {
			return fmt.Errorf("failed to convert output to map: %w", err)
		}
		for key, value := range outputMap {
			if _, ok := registry.Get(key); ok {
				writes[key] = append(writes[key], value)
			}
		}
	}
	if len(writes) > 0 {
		return registry.UpdateChannels(writes)
	}
	return nil
}

func inlineBuildOutput(registry *channels.Registry, lastState interface{}) (interface{}, error) {
	values, err := registry.GetValues()
	if err != nil {
		return lastState, nil
	}
	if len(values) > 0 {
		return values, nil
	}
	return lastState, nil
}

func inlineMergeStates(existing, new interface{}) interface{} {
	if existing == nil {
		return new
	}
	if new == nil {
		return existing
	}
	existingMap, ok1 := existing.(map[string]interface{})
	newMap, ok2 := new.(map[string]interface{})
	if ok1 && ok2 {
		result := make(map[string]interface{})
		for k, v := range existingMap {
			result[k] = v
		}
		for k, v := range newMap {
			result[k] = v
		}
		return result
	}
	return new
}

func inlineToMap(v interface{}) (map[string]interface{}, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Map {
		return map[string]interface{}{"__root__": v}, nil
	}
	result := make(map[string]interface{})
	if rv.Kind() == reflect.Map {
		for _, key := range rv.MapKeys() {
			result[fmt.Sprintf("%v", key.Interface())] = rv.MapIndex(key).Interface()
		}
		return result, nil
	}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		result[field.Name] = rv.Field(i).Interface()
	}
	return result, nil
}
