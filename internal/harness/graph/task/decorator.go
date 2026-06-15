// Package task provides function decorators for Agent Harness tasks.
package task

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// TaskDecorator wraps a function with retry, cache, and other policies.
type TaskDecorator struct {
	name        string
	retryPolicy *types.RetryPolicy
	cachePolicy *types.CachePolicy
	metadata    map[string]interface{}
	cache       sync.Map // key -> cacheEntry for Cached() support
}

type tCacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// DecoratorOption configures a TaskDecorator.
type DecoratorOption func(*TaskDecorator)

// WithName sets the task name.
func WithName(name string) DecoratorOption {
	return func(d *TaskDecorator) {
		d.name = name
	}
}

// WithRetryPolicy sets the retry policy.
func WithRetryPolicy(policy *types.RetryPolicy) DecoratorOption {
	return func(d *TaskDecorator) {
		d.retryPolicy = policy
	}
}

// WithCachePolicy sets the cache policy.
func WithCachePolicy(policy *types.CachePolicy) DecoratorOption {
	return func(d *TaskDecorator) {
		d.cachePolicy = policy
	}
}

// WithMetadata sets task metadata.
func WithMetadata(metadata map[string]interface{}) DecoratorOption {
	return func(d *TaskDecorator) {
		d.metadata = metadata
	}
}

// NewDecorator creates a new task decorator.
func NewDecorator(opts ...DecoratorOption) *TaskDecorator {
	d := &TaskDecorator{
		name:     uuid.New().String(),
		metadata: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Wrap wraps a function with the decorator's policies.
func (d *TaskDecorator) Wrap(fn types.NodeFunc) types.NodeFunc {
	return func(ctx context.Context, input interface{}) (interface{}, error) {
		// Create task context
		taskCtx := &TaskContext{
			Name:     d.name,
			ID:       uuid.New().String(),
			Input:    input,
			Metadata: d.metadata,
			Start:    time.Now(),
		}

		// Check cache if configured
		if d.cachePolicy != nil {
			if cached, ok := d.getCached(input); ok {
				return cached, nil
			}
		}

		// Execute with retry if configured
		if d.retryPolicy != nil {
			output, err := d.executeWithRetry(ctx, taskCtx, fn)
			if err == nil && d.cachePolicy != nil {
				d.setCached(input, output)
			}
			return output, err
		}

		// Execute normally
		output, err := fn(ctx, input)
		taskCtx.End = time.Now()
		taskCtx.Output = output
		taskCtx.Error = err
		if err == nil && d.cachePolicy != nil {
			d.setCached(input, output)
		}
		return output, err
	}
}

// getCached retrieves a cached value if present and not expired.
func (d *TaskDecorator) getCached(input interface{}) (interface{}, bool) {
	key := cacheKey(input)
	if v, ok := d.cache.Load(key); ok {
		if entry, ok := v.(tCacheEntry); ok {
			if d.cachePolicy.TTL == nil || time.Now().Before(entry.expiresAt) {
				return entry.value, true
			}
			d.cache.Delete(key)
		}
	}
	return nil, false
}

// setCached stores a value in the cache.
func (d *TaskDecorator) setCached(input interface{}, value interface{}) {
	key := cacheKey(input)
	var expiresAt time.Time
	if d.cachePolicy.TTL != nil {
		expiresAt = time.Now().Add(*d.cachePolicy.TTL)
	}
	d.cache.Store(key, tCacheEntry{value: value, expiresAt: expiresAt})
}

// cacheKey generates a deterministic cache key from an input value.
func cacheKey(input interface{}) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", input)))
	return fmt.Sprintf("%x", h[:])
}

// executeWithRetry executes the function with retry logic.
func (d *TaskDecorator) executeWithRetry(ctx context.Context, taskCtx *TaskContext, fn types.NodeFunc) (interface{}, error) {
	policy := d.retryPolicy
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		taskCtx.Attempt = attempt

		output, err := fn(ctx, taskCtx.Input)
		if err == nil {
			taskCtx.End = time.Now()
			taskCtx.Output = output
			return output, nil
		}

		// Check if retryable
		if policy.RetryOn != nil && !policy.RetryOn(err) {
			return nil, fmt.Errorf("task %s failed with non-retryable error: %w", d.name, err)
		}

		lastErr = err

		if attempt >= policy.MaxAttempts {
			break
		}

		// Calculate backoff
		backoff := calculateBackoff(attempt, policy)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			// Continue
		}
	}

	taskCtx.End = time.Now()
	taskCtx.Error = lastErr
	return nil, fmt.Errorf("task %s failed after %d attempts: %w", d.name, policy.MaxAttempts, lastErr)
}

// TaskContext holds context information about a task execution.
type TaskContext struct {
	Name     string
	ID       string
	Input    interface{}
	Output   interface{}
	Error    error
	Attempt  int
	Metadata map[string]interface{}
	Start    time.Time
	End      time.Time
}

// Duration returns the execution duration.
func (tc *TaskContext) Duration() time.Duration {
	return tc.End.Sub(tc.Start)
}

// calculateBackoff calculates the backoff duration.
func calculateBackoff(attempt int, policy *types.RetryPolicy) time.Duration {
	backoff := time.Duration(float64(policy.InitialInterval) * math.Pow(policy.BackoffFactor, float64(attempt-1)))
	if backoff > policy.MaxInterval {
		backoff = policy.MaxInterval
	}

	if policy.Jitter {
		backoff = addJitter(backoff)
	}

	return backoff
}

// addJitter adds ±25% random jitter to a duration.
func addJitter(d time.Duration) time.Duration {
	delta := float64(d) * 0.25
	jitter := (rand.Float64()*2 - 1) * delta
	return d + time.Duration(jitter)
}

// Task wraps a function with the given options.
func Task(fn types.NodeFunc, opts ...DecoratorOption) types.NodeFunc {
	decorator := NewDecorator(opts...)
	return decorator.Wrap(fn)
}

// Entrypoint marks a function as a graph entrypoint.
type Entrypoint struct {
	name           string
	fn             types.NodeFunc
	metadata       map[string]interface{}
	checkpointer   interface{}
	store          interface{}
	configurable   map[string]interface{}
	graph          *graph.StateGraph
	compiledGraph  *graph.CompiledGraph
	compileOnce    sync.Once
	compileErr     error
}

// NewEntrypoint creates a new entrypoint.
func NewEntrypoint(name string, fn types.NodeFunc, metadata map[string]interface{}) *Entrypoint {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	return &Entrypoint{
		name:         name,
		fn:           fn,
		metadata:     metadata,
		checkpointer: nil,
		store:        nil,
		configurable: make(map[string]interface{}),
		graph:        nil,
	}
}

// EntrypointOption configures an entrypoint.
type EntrypointOption func(*Entrypoint)

// WithEntrypointCheckpointer sets the checkpointer for the entrypoint.
func WithEntrypointCheckpointer(cp interface{}) EntrypointOption {
	return func(e *Entrypoint) {
		e.checkpointer = cp
	}
}

// WithEntrypointStore sets the store for the entrypoint.
func WithEntrypointStore(st interface{}) EntrypointOption {
	return func(e *Entrypoint) {
		e.store = st
	}
}

// WithEntrypointConfigurable sets configurable values for the entrypoint.
func WithEntrypointConfigurable(configurable map[string]interface{}) EntrypointOption {
	return func(e *Entrypoint) {
		e.configurable = configurable
	}
}

// WithEntrypointGraph sets the graph for the entrypoint.
func WithEntrypointGraph(g *graph.StateGraph) EntrypointOption {
	return func(e *Entrypoint) {
		e.graph = g
	}
}

// NewEntrypointWithOptions creates a new entrypoint with options.
func NewEntrypointWithOptions(name string, fn types.NodeFunc, metadata map[string]interface{}, opts ...EntrypointOption) *Entrypoint {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	e := &Entrypoint{
		name:         name,
		fn:           fn,
		metadata:     metadata,
		checkpointer: nil,
		store:        nil,
		configurable: make(map[string]interface{}),
		graph:        nil,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name returns the entrypoint name.
func (e *Entrypoint) Name() string {
	return e.name
}

// Execute executes the entrypoint.
func (e *Entrypoint) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	return e.fn(ctx, input)
}

// Compile compiles the graph associated with this entrypoint.
// Safe to call concurrently — only the first invocation executes compilation.
func (e *Entrypoint) Compile(ctx context.Context) error {
	e.compileOnce.Do(func() {
		if e.graph == nil {
			e.compileErr = fmt.Errorf("no graph associated with entrypoint")
			return
		}

		// Collect compile options from the checkpointer if set
		var opts []graph.CompileOption
		if cp, ok := e.checkpointer.(graph.Checkpointer); ok {
			opts = append(opts, graph.WithCheckpointer(cp))
		}

		// Actually compile the graph and cache the result
		cg, err := e.graph.Compile(opts...)
		if err != nil {
			e.compileErr = err
			return
		}
		e.compiledGraph = cg
	})
	return e.compileErr
}

// Invoke invokes the graph with the given input.
// When a graph is associated via WithEntrypointGraph, it delegates to the
// compiled graph's Invoke method instead of executing the raw function.
func (e *Entrypoint) Invoke(ctx context.Context, input interface{}, config *types.RunnableConfig) (interface{}, error) {
	// Compile once (thread-safe via sync.Once).
	if err := e.Compile(ctx); err != nil {
		return nil, err
	}

	// Merge configurable values
	if config == nil {
		config = types.NewRunnableConfig()
	}
	for k, v := range e.configurable {
		config.Set(k, v)
	}

	// Use the compiled graph when available
	if e.compiledGraph != nil {
		return e.compiledGraph.Invoke(ctx, input, config)
	}

	return e.Execute(ctx, input)
}

// InvokeAsyncResult carries the result of an asynchronous graph invocation.
type InvokeAsyncResult struct {
	Output interface{}
	Err    error
}

// AInvoke invokes the graph asynchronously with the given input.
// The returned channel carries the result (output + error) when done.
func (e *Entrypoint) AInvoke(ctx context.Context, input interface{}, config *types.RunnableConfig) <-chan InvokeAsyncResult {
	result := make(chan InvokeAsyncResult, 1)

	go func() {
		output, err := e.Invoke(ctx, input, config)
		result <- InvokeAsyncResult{Output: output, Err: err}
		close(result)
	}()

	return result
}

// Stream streams the output of the graph execution.
// When a graph is associated, delegates to the compiled graph's Stream method.
func (e *Entrypoint) Stream(ctx context.Context, input interface{}, config *types.RunnableConfig, mode types.StreamMode) (<-chan interface{}, error) {
	// Compile once (thread-safe via sync.Once)
	if err := e.Compile(ctx); err != nil {
		return nil, err
	}

	// Merge configurable values
	if config == nil {
		config = types.NewRunnableConfig()
	}
	for k, v := range e.configurable {
		config.Set(k, v)
	}

	// Use the compiled graph when available.
	// CompiledGraph.Stream returns (valueCh, errCh); merge into a single channel
	// for the Entrypoint's simpler Stream contract.
	if e.compiledGraph != nil {
		outCh, errCh := e.compiledGraph.Stream(ctx, input, mode, config)
		ch := make(chan interface{}, 1)
		go func() {
			defer close(ch)
			select {
			case v, ok := <-outCh:
				if ok {
					ch <- v
				}
			case err, ok := <-errCh:
				if ok && err != nil {
					ch <- err
				}
			case <-ctx.Done():
			}
		}()
		return ch, nil
	}

	// Fallback: execute the function directly
	output, err := e.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	ch := make(chan interface{}, 1)
	ch <- output
	close(ch)
	return ch, nil
}

// AStream streams the output of the graph execution asynchronously.
func (e *Entrypoint) AStream(ctx context.Context, input interface{}, config *types.RunnableConfig, mode types.StreamMode) (<-chan interface{}, error) {
	return e.Stream(ctx, input, config, mode)
}

// Batch invokes the graph with multiple inputs.
func (e *Entrypoint) Batch(ctx context.Context, inputs []interface{}, config *types.RunnableConfig) ([]interface{}, error) {
	results := make([]interface{}, len(inputs))
	
	for i, input := range inputs {
		output, err := e.Invoke(ctx, input, config)
		if err != nil {
			return nil, fmt.Errorf("batch invocation failed at index %d: %w", i, err)
		}
		results[i] = output
	}
	
	return results, nil
}

// BatchAsyncResult carries the result of an asynchronous batch invocation.
type BatchAsyncResult struct {
	Outputs []interface{}
	Err     error
}

// ABatch invokes the graph with multiple inputs asynchronously.
// The returned channel carries the result (outputs + error) when done.
func (e *Entrypoint) ABatch(ctx context.Context, inputs []interface{}, config *types.RunnableConfig) <-chan BatchAsyncResult {
	result := make(chan BatchAsyncResult, 1)

	go func() {
		outputs, err := e.Batch(ctx, inputs, config)
		result <- BatchAsyncResult{Outputs: outputs, Err: err}
		close(result)
	}()

	return result
}

// EntrypointDecorator creates an entrypoint decorator.
func EntrypointDecorator(name string, metadata map[string]interface{}) func(types.NodeFunc) types.NodeFunc {
	return func(fn types.NodeFunc) types.NodeFunc {
		entry := NewEntrypoint(name, fn, metadata)
		return func(ctx context.Context, input interface{}) (interface{}, error) {
			return entry.Execute(ctx, input)
		}
	}
}

// Retryable marks a function as retryable with the given policy.
func Retryable(fn types.NodeFunc, maxAttempts int, backoffFactor float64) types.NodeFunc {
	policy := types.DefaultRetryPolicy()
	policy.MaxAttempts = maxAttempts
	policy.BackoffFactor = backoffFactor

	return Task(fn, WithRetryPolicy(&policy))
}

// Cached wraps a function with caching.
func Cached(fn types.NodeFunc, ttl time.Duration) types.NodeFunc {
	policy := &types.CachePolicy{
		TTL: &ttl,
	}
	return Task(fn, WithCachePolicy(policy))
}

// Named names a task.
func Named(name string, fn types.NodeFunc) types.NodeFunc {
	return Task(fn, WithName(name))
}

// WithTimeout adds timeout to a function.
func WithTimeout(fn types.NodeFunc, timeout time.Duration) types.NodeFunc {
	return func(ctx context.Context, input interface{}) (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return fn(ctx, input)
	}
}

// Compose composes multiple decorators.
func Compose(decorators ...func(types.NodeFunc) types.NodeFunc) func(types.NodeFunc) types.NodeFunc {
	return func(fn types.NodeFunc) types.NodeFunc {
		for i := len(decorators) - 1; i >= 0; i-- {
			fn = decorators[i](fn)
		}
		return fn
	}
}

// EntrypointFinal represents a final value that should be saved to checkpoint.
// This is used to mark the final output of an entrypoint for persistence.
type EntrypointFinal struct {
	Value interface{}
	Save  bool
}

// Final creates a new EntrypointFinal with the given value.
// If save is true, the value will be persisted to the checkpointer.
func Final(value interface{}, save ...bool) *EntrypointFinal {
	shouldSave := true
	if len(save) > 0 {
		shouldSave = save[0]
	}
	return &EntrypointFinal{
		Value: value,
		Save:  shouldSave,
	}
}

// IsFinal checks if a value is an EntrypointFinal.
func IsFinal(v interface{}) (*EntrypointFinal, bool) {
	if f, ok := v.(*EntrypointFinal); ok {
		return f, true
	}
	return nil, false
}

// GetFinalValue extracts the value from a final result, handling EntrypointFinal.
func GetFinalValue(result interface{}) interface{} {
	if f, ok := IsFinal(result); ok {
		return f.Value
	}
	return result
}

// ExecutionContext provides dependency injection context for entrypoints.
type ExecutionContext struct {
	// Config is the RunnableConfig for the execution.
	Config *types.RunnableConfig
	// Previous is the result from the previous execution (for resuming).
	Previous interface{}
	// Store is the BaseStore for long-term storage.
	Store interface{}
	// Writer is the stream writer for emitting events.
	Writer interface{}
	// Runtime contains runtime-specific values.
	Runtime map[string]interface{}
}

// InjectDependencies creates a new node function with injected dependencies.
// This allows the function to access Config, Previous, Store, and Writer.
func InjectDependencies(fn types.NodeFunc, execCtx *ExecutionContext) types.NodeFunc {
	return func(ctx context.Context, input interface{}) (interface{}, error) {
		// Create an enhanced context with execution context
		enhancedCtx := context.WithValue(ctx, executionContextKey{}, execCtx)
		return fn(enhancedCtx, input)
	}
}

// executionContextKey is the key for storing ExecutionContext in context.
type executionContextKey struct{}

// GetExecutionContext retrieves the ExecutionContext from the context.
func GetExecutionContext(ctx context.Context) *ExecutionContext {
	if execCtx, ok := ctx.Value(executionContextKey{}).(*ExecutionContext); ok {
		return execCtx
	}
	return nil
}

// GetConfig retrieves the config from the execution context.
func GetConfig(ctx context.Context) *types.RunnableConfig {
	if execCtx := GetExecutionContext(ctx); execCtx != nil {
		return execCtx.Config
	}
	return nil
}

// GetPrevious retrieves the previous result from the execution context.
func GetPrevious(ctx context.Context) interface{} {
	if execCtx := GetExecutionContext(ctx); execCtx != nil {
		return execCtx.Previous
	}
	return nil
}

// GetStore retrieves the store from the execution context.
func GetStore(ctx context.Context) interface{} {
	if execCtx := GetExecutionContext(ctx); execCtx != nil {
		return execCtx.Store
	}
	return nil
}

// GetWriter retrieves the writer from the execution context.
func GetWriter(ctx context.Context) interface{} {
	if execCtx := GetExecutionContext(ctx); execCtx != nil {
		return execCtx.Writer
	}
	return nil
}

// InvokeWithDependencies invokes the entrypoint with dependency injection.
// When a graph is associated, delegates to the compiled graph's Invoke method.
func (e *Entrypoint) InvokeWithDependencies(
	ctx context.Context,
	input interface{},
	config *types.RunnableConfig,
	previous interface{},
	store interface{},
	writer interface{},
) (interface{}, error) {
	// If a graph is available, delegate to the compiled graph
	if e.graph != nil {
		return e.Invoke(ctx, input, config)
	}

	// Create execution context
	execCtx := &ExecutionContext{
		Config:   config,
		Previous: previous,
		Store:    store,
		Writer:   writer,
		Runtime:  make(map[string]interface{}),
	}

	// Add store from entrypoint if not provided
	if store == nil && e.store != nil {
		execCtx.Store = e.store
	}

	// Inject dependencies into the function
	injectedFn := InjectDependencies(e.fn, execCtx)

	// Execute with injected function
	result, err := injectedFn(ctx, input)
	if err != nil {
		return nil, err
	}

	// Handle EntrypointFinal
	if final, ok := IsFinal(result); ok {
		// Save to checkpointer if enabled
		if final.Save && e.checkpointer != nil {
			// In a full implementation, this would save to checkpointer
			// For now, we just return the value
		}
		return final.Value, nil
	}

	return result, nil
}
