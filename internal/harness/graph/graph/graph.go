// Package graph provides graph building capabilities for Harness-Go.
package graph

import (
	"context"
	"fmt"
	"slices"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// Interface compliance checks.
var _ types.StateGraph = (*stateGraph)(nil)

// stateGraph is a graph whose nodes communicate by reading and writing to a shared state.
type stateGraph struct {
	nodes            map[string]*types.Node
	edges            []*types.Edge
	dataEdges        []*types.DataEdge
	conditionalEdges []*types.ConditionalEdge
	branches         []*types.Branch
	entryPoint       string
	finishPoints     []string
	channels         map[string]channels.Channel
	reducers         map[string]types.ReducerFunc
	stateSchema      interface{}
	inputSchema      interface{}
	outputSchema     interface{}
	NodeTriggerMode  types.NodeTriggerMode
}

// NewStateGraph creates a new StateGraph with the given state schema.
func NewStateGraph(stateSchema interface{}) types.StateGraph {
	return &stateGraph{
		nodes:            make(map[string]*types.Node),
		edges:            make([]*types.Edge, 0),
		conditionalEdges: make([]*types.ConditionalEdge, 0),
		branches:         make([]*types.Branch, 0),
		finishPoints:     make([]string, 0),
		channels:         make(map[string]channels.Channel),
		reducers:         make(map[string]types.ReducerFunc),
		stateSchema:      stateSchema,
		inputSchema:      stateSchema,
		outputSchema:     stateSchema,
	}
}

func (g *stateGraph) WithInputSchema(schema interface{}) types.StateGraph {
	g.inputSchema = schema
	return g
}

func (g *stateGraph) WithOutputSchema(schema interface{}) types.StateGraph {
	g.outputSchema = schema
	return g
}

func (g *stateGraph) AddNode(name string, fn types.NodeFunc) *types.Node {
	node := &types.Node{
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

func (g *stateGraph) AddNodeWithOptions(name string, fn types.NodeFunc, opts types.NodeOptions) *types.Node {
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

// WithStatePreHandler wraps the node with a pre-execution state transform.
func WithStatePreHandler(fn types.NodeFunc) func(*types.NodeOptions) {
	return func(opts *types.NodeOptions) { opts.StatePre = fn }
}

// WithStatePostHandler wraps the node with a post-execution state transform.
func WithStatePostHandler(fn types.NodeFunc) func(*types.NodeOptions) {
	return func(opts *types.NodeOptions) { opts.StatePost = fn }
}

// WithFieldMapping sets field-level routing for this node's output.
func WithFieldMapping(mappings ...types.FieldMapping) func(*types.NodeOptions) {
	return func(opts *types.NodeOptions) { opts.FieldMapping = append(opts.FieldMapping, mappings...) }
}

// MapFields creates a FieldMapping from source to target path.
func MapFields(from, to string) types.FieldMapping {
	return types.FieldMapping{From: from, To: to}
}

// MapTo creates a FieldMapping that maps entire output to a target path.
func MapTo(to string) types.FieldMapping {
	return types.FieldMapping{To: to}
}

func (g *stateGraph) AddEdge(from, to string) error {
	if _, ok := g.nodes[from]; !ok && from != constants.Start {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	if _, ok := g.nodes[to]; !ok && to != constants.End {
		return &errors.NodeNotFoundError{NodeName: to}
	}
	g.edges = append(g.edges, &types.Edge{From: from, To: to})
	if from == constants.Start {
		g.entryPoint = to
	}
	if to == constants.End {
		if !slices.Contains(g.finishPoints, from) {
			g.finishPoints = append(g.finishPoints, from)
		}
	}
	return nil
}

func (g *stateGraph) AddConditionalEdges(from string, condition types.EdgeFunc, mapping map[string]string) error {
	if _, ok := g.nodes[from]; !ok {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	for _, target := range mapping {
		if _, ok := g.nodes[target]; !ok && target != constants.End {
			return &errors.NodeNotFoundError{NodeName: target}
		}
	}
	g.conditionalEdges = append(g.conditionalEdges, &types.ConditionalEdge{
		From: from, Condition: condition, Mapping: mapping,
	})
	return nil
}

func (g *stateGraph) AddBranch(from string, condition types.EdgeFunc, then func(interface{}) []string) error {
	if _, ok := g.nodes[from]; !ok {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	g.branches = append(g.branches, &types.Branch{
		From: from, Condition: condition, Then: then,
	})
	return nil
}

func (g *stateGraph) AddDataEdge(from, to string, mappings ...types.FieldMapping) error {
	if _, ok := g.nodes[from]; !ok && from != constants.Start {
		return &errors.NodeNotFoundError{NodeName: from}
	}
	if _, ok := g.nodes[to]; !ok && to != constants.End {
		return &errors.NodeNotFoundError{NodeName: to}
	}
	g.dataEdges = append(g.dataEdges, &types.DataEdge{From: from, To: to, Mapping: mappings})
	return nil
}

// --- types.StateGraph interface methods ---

func (g *stateGraph) GetChannels() map[string]interface{} {
	result := make(map[string]interface{}, len(g.channels))
	for k, v := range g.channels {
		result[k] = v
	}
	return result
}

func (g *stateGraph) GetEntryPoint() string { return g.entryPoint }
func (g *stateGraph) GetNode(name string) (*types.Node, bool) {
	n, ok := g.nodes[name]
	return n, ok
}
func (g *stateGraph) GetEdges() []*types.Edge                       { return g.edges }
func (g *stateGraph) GetConditionalEdges() []*types.ConditionalEdge { return g.conditionalEdges }
func (g *stateGraph) GetBranches() []*types.Branch                  { return g.branches }
func (g *stateGraph) GetNodes() map[string]*types.Node              { return g.nodes }
func (g *stateGraph) GetNodeTriggerMode() types.NodeTriggerMode     { return g.NodeTriggerMode }
func (g *stateGraph) SetNodeTriggerMode(mode types.NodeTriggerMode) { g.NodeTriggerMode = mode }
func (g *stateGraph) GetDataEdges() []*types.DataEdge               { return g.dataEdges }
func (g *stateGraph) GetStateSchema() interface{}                   { return g.stateSchema }

func (g *stateGraph) SetEntryPoint(node string) error {
	if _, ok := g.nodes[node]; !ok {
		return &errors.NodeNotFoundError{NodeName: node}
	}
	g.entryPoint = node
	return nil
}

func (g *stateGraph) SetFinishPoint(node string) error {
	if _, ok := g.nodes[node]; !ok {
		return &errors.NodeNotFoundError{NodeName: node}
	}
	g.finishPoints = append(g.finishPoints, node)
	return nil
}

func (g *stateGraph) AddChannel(name string, channel interface{}) {
	if ch, ok := channel.(channels.Channel); ok {
		ch.SetKey(name)
		g.channels[name] = ch
	}
}

func (g *stateGraph) SetReducer(channelName string, reducer types.ReducerFunc) {
	if channel, ok := g.channels[channelName]; ok {
		g.channels[channelName] = channels.NewReducerChannel(channel, reducer)
	}
	g.reducers[channelName] = reducer
}

func (g *stateGraph) AddChannelWithReducer(name string, channel interface{}, reducer types.ReducerFunc) {
	if ch, ok := channel.(channels.Channel); ok {
		ch.SetKey(name)
		if reducer != nil {
			g.channels[name] = channels.NewReducerChannel(ch, reducer)
			g.reducers[name] = reducer
		} else {
			g.channels[name] = ch
		}
	}
}

func (g *stateGraph) Validate() error {
	if g.entryPoint == "" {
		return fmt.Errorf("no entry point set")
	}
	if len(g.finishPoints) == 0 {
		return fmt.Errorf("no finish points set")
	}
	reachable := g.computeReachable()
	for name := range g.nodes {
		if !reachable[name] {
			return fmt.Errorf("node %s is not reachable from entry point", name)
		}
	}
	if err := g.ValidateStateSchema(); err != nil {
		return fmt.Errorf("state schema validation failed: %w", err)
	}
	return nil
}

func (g *stateGraph) computeReachable() map[string]bool {
	reachable := make(map[string]bool)
	if g.entryPoint == "" {
		return reachable
	}
	queue := []string{g.entryPoint}
	reachable[g.entryPoint] = true
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range g.edges {
			if edge.From == current && !reachable[edge.To] && edge.To != constants.End {
				reachable[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
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
	}
	return reachable
}

func (g *stateGraph) configureChannelsFromSchema() error {
	fieldInfos, err := g.GetStateSchemaInfo()
	if err != nil {
		return err
	}
	for fieldName, info := range fieldInfos {
		if _, exists := g.channels[fieldName]; !exists {
			g.channels[fieldName] = info.Channel
		}
		if info.Annotation != nil && info.Annotation.Reducer != nil {
			g.reducers[fieldName] = info.Annotation.Reducer
		}
	}
	return nil
}

// --- Compile and CompiledGraph ---

// CompileOption configures CompiledGraph behavior at compile time.
type CompileOption func(*compiledGraph)

// compiledGraph is a compiled, executable graph.
type compiledGraph struct {
	graph           *stateGraph
	checkpointer    interface{} // checkpoint.BaseCheckpointer
	interrupts      map[string]bool
	interruptsAfter map[string]bool
	recursionLimit  int
	debug           bool
	nodeTriggerMode types.NodeTriggerMode
}

func (g *stateGraph) Compile(opts ...interface{}) (types.CompiledGraph, error) {
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %w", err)
	}
	if err := g.configureChannelsFromSchema(); err != nil {
		return nil, fmt.Errorf("failed to configure channels from schema: %w", err)
	}
	cg := &compiledGraph{
		graph:           g,
		interrupts:      make(map[string]bool),
		interruptsAfter: make(map[string]bool),
		recursionLimit:  constants.DefaultRecursionLimit,
		nodeTriggerMode: types.NodeTriggerAnyPredecessor,
	}
	for _, opt := range opts {
		if fn, ok := opt.(CompileOption); ok {
			fn(cg)
		}
	}
	g.NodeTriggerMode = cg.nodeTriggerMode
	return cg, nil
}

func WithCheckpointer(checkpointer interface{}) CompileOption {
	return func(cg *compiledGraph) {
		cg.checkpointer = checkpointer
	}
}

func WithInterrupts(nodes ...string) CompileOption {
	return func(cg *compiledGraph) {
		for _, node := range nodes {
			cg.interrupts[node] = true
		}
	}
}

func WithInterruptsAfter(nodes ...string) CompileOption {
	return func(cg *compiledGraph) {
		for _, node := range nodes {
			cg.interruptsAfter[node] = true
		}
	}
}

func WithRecursionLimit(limit int) CompileOption {
	return func(cg *compiledGraph) {
		cg.recursionLimit = limit
	}
}

func WithDebug(debug bool) CompileOption {
	return func(cg *compiledGraph) {
		cg.debug = debug
	}
}

func WithNodeTriggerMode(mode types.NodeTriggerMode) CompileOption {
	return func(cg *compiledGraph) {
		cg.nodeTriggerMode = mode
	}
}

func (cg *compiledGraph) Invoke(ctx context.Context, input interface{}, config ...*types.RunnableConfig) (interface{}, error) {
	rc := &types.RunnableConfig{}
	if len(config) > 0 && config[0] != nil {
		rc = config[0]
	}
	return cg.run(ctx, input, rc, types.StreamModeValues)
}

func (cg *compiledGraph) Stream(ctx context.Context, input interface{}, mode types.StreamMode, config ...*types.RunnableConfig) (<-chan interface{}, <-chan error) {
	outputCh := make(chan interface{}, 1)
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

func (cg *compiledGraph) run(ctx context.Context, input interface{}, config *types.RunnableConfig, streamMode types.StreamMode) (interface{}, error) {
	if types.PregelRunFunc == nil {
		return nil, fmt.Errorf("graph: pregel engine not installed")
	}
	return types.PregelRunFunc(ctx, cg, input, config, streamMode)
}

// --- types.CompiledGraph interface methods ---

func (cg *compiledGraph) GetGraph() types.StateGraph          { return cg.graph }
func (cg *compiledGraph) GetCheckpointer() interface{}        { return cg.checkpointer }
func (cg *compiledGraph) GetInterrupts() map[string]bool      { return cg.interrupts }
func (cg *compiledGraph) GetInterruptsAfter() map[string]bool { return cg.interruptsAfter }
func (cg *compiledGraph) GetRecursionLimit() int              { return cg.recursionLimit }
func (cg *compiledGraph) IsDebug() bool                       { return cg.debug }
