// Package graph — Parallel macro support.
//
// NewParallelNodeFunc wraps a compiled sub-graph in a NodeFunc closure
// that invokes the sub-graph once per item in the input slice, with
// bounded concurrency. The outer StateGraph sees a single node; the
// fan-out lives entirely inside the closure.
//
// Design (independent sub-graph pattern):
//
//   - Input: the node receives a []any (list of items to process).
//   - Output: the node returns []any (ordered results, one per input item).
//   - Concurrency: maxConcurrency ≤ 1 → sequential; > 1 → bounded fan-out
//     using a semaphore. The first item runs on the calling goroutine.
//   - Interrupt: if any per-item sub-graph invocation is interrupted, the
//     results for completed items are saved alongside the interrupted indices
//     and per-item checkpoint IDs, then re-thrown via interrupt.Interrupt.
//     On resume, only the interrupted items are re-processed.
//   - Error: the first non-interrupt error terminates the loop (items are
//     drained to prevent goroutine leaks, but the error is returned).
package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// ParallelOption configures NewParallelNodeFunc.
type ParallelOption func(*parallelOptions)

type parallelOptions struct {
	maxConcurrency int
}

// WithParallelMaxConcurrency caps the number of concurrent sub-graph
// invocations. ≤ 1 = sequential (no goroutines). Default 0 (sequential).
func WithParallelMaxConcurrency(maxConcurrency int) ParallelOption {
	return func(o *parallelOptions) {
		if maxConcurrency >= 0 {
			o.maxConcurrency = maxConcurrency
		}
	}
}

func getParallelOptions(opts []ParallelOption) *parallelOptions {
	o := &parallelOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Sentinel errors.
var (
	ErrParallelResumeStateInvalid = errors.New("graph: parallel resume state invalid")
	ErrParallelItemFailed         = errors.New("graph: parallel item failed")
)

// parallelInterruptState is persisted when a parallel node is interrupted.
type parallelInterruptState struct {
	OriginalInputsJSON []byte            `json:"original_inputs_json"`
	CompletedResults   map[int]any       `json:"completed_results"`
	InterruptedIndices []int             `json:"interrupted_indices"`
	TotalCount         int               `json:"total_count"`
	ItemCheckpoints    map[string][]byte `json:"item_checkpoints,omitempty"`
}

// parallelItemResult is the per-item outcome from the fan-out goroutines.
type parallelItemResult struct {
	index  int
	output interface{}
	err    error
}

// NewParallelNodeFunc wraps a compiled sub-graph into a NodeFunc that
// parallel-fanouts over its input slice.
//
// The returned NodeFunc expects input of type []any and returns []any.
// It can be added directly via sg.AddNode(key, nodeFunc).
//
// key is the node name.
// sub is the already-compiled sub-graph to invoke per item.
func NewParallelNodeFunc(
	key string,
	sub *compiledGraph,
	opts ...ParallelOption,
) (types.NodeFunc, error) {
	if key == "" {
		return nil, fmt.Errorf("graph: parallel key is empty")
	}
	if sub == nil {
		return nil, fmt.Errorf("graph: parallel sub-graph is nil")
	}
	options := getParallelOptions(opts)

	nodeFunc := func(ctx context.Context, state interface{}) (interface{}, error) {
		items, ok := state.([]interface{})
		if !ok {
			// If the state is a map with the relevant key, try to extract.
			m, mapOk := state.(map[string]interface{})
			if !mapOk {
				return nil, fmt.Errorf("graph: parallel node expects []interface{} input, got %T", state)
			}
			// Try '__input__' or 'items' key.
			if raw, found := m["__input__"]; found {
				items, ok = raw.([]interface{})
				if !ok {
					items, ok = raw.([]interface{})
				}
			}
			if raw, found := m["items"]; found && !ok {
				items, _ = raw.([]interface{})
			}
			if items == nil {
				return nil, fmt.Errorf("graph: parallel node cannot extract []interface{} from %T", state)
			}
		}
		return runParallel(ctx, key, sub, items, options)
	}
	return nodeFunc, nil
}

// runParallel implements the parallel fan-out body.
func runParallel(
	ctx context.Context,
	key string,
	sub *compiledGraph,
	items []interface{},
	options *parallelOptions,
) (interface{}, error) {
	// Check for resume.
	prev, isResume := loadParallelSnapshot(ctx)

	effectiveItems := items
	totalCount := len(effectiveItems)
	outputs := make([]interface{}, totalCount)
	indicesToProcess := make([]int, totalCount)
	for i := range effectiveItems {
		indicesToProcess[i] = i
	}
	bridgeState := newItemCheckpointStore(nil)

	if isResume && prev != nil {
		totalCount = prev.TotalCount
		outputs = make([]interface{}, totalCount)
		// Replay completed results.
		for idx, v := range prev.CompletedResults {
			if idx >= 0 && idx < totalCount {
				outputs[idx] = v
			}
		}
		// Only re-process interrupted indices.
		indicesToProcess = append([]int(nil), prev.InterruptedIndices...)
		bridgeState = newItemCheckpointStore(prev.ItemCheckpoints)
	}

	if len(indicesToProcess) == 0 {
		return outputs, nil
	}

	// Fan-out.
	results := fanOutItems(ctx, key, sub, effectiveItems, indicesToProcess, options, bridgeState)

	// Collect results.
	var normalErr error
	hasInterrupt := false
	completedResults := make(map[int]any)
	for r := range results {
		if r.err == nil {
			if r.index >= 0 && r.index < len(outputs) {
				outputs[r.index] = r.output
			}
			completedResults[r.index] = r.output
			continue
		}
		if interrupt.IsInterrupt(r.err) {
			hasInterrupt = true
			continue
		}
		if normalErr == nil {
			normalErr = fmt.Errorf("%w: item %d: %v", ErrParallelItemFailed, r.index, r.err)
		}
	}

	if normalErr != nil {
		return nil, normalErr
	}

	if hasInterrupt {
		inputsJSON, jErr := json.Marshal(effectiveItems)
		if jErr != nil {
			return nil, fmt.Errorf("graph: parallel marshal inputs: %w", jErr)
		}
		pending := make([]int, 0, totalCount)
		for idx := 0; idx < totalCount; idx++ {
			if _, done := completedResults[idx]; !done {
				pending = append(pending, idx)
			}
		}
		state := parallelInterruptState{
			OriginalInputsJSON: inputsJSON,
			CompletedResults:   completedResults,
			InterruptedIndices: pending,
			TotalCount:         totalCount,
			ItemCheckpoints:    bridgeState.snapshot(),
		}
		// Pass state directly — the checkpoint engine serializes
		// interrupt values when persisting. Avoid double-serialization
		// by not marshalling here.
		_, interruptErr := interrupt.Interrupt(ctx, state)
		return nil, interruptErr
	}

	return outputs, nil
}

// fanOutItems fans out sub-graph invocations across items.
func fanOutItems(
	ctx context.Context,
	key string,
	sub *compiledGraph,
	items []interface{},
	indices []int,
	options *parallelOptions,
	bridgeState *itemCheckpointStore,
) <-chan parallelItemResult {
	resultCh := make(chan parallelItemResult, len(indices))
	if len(indices) == 0 {
		close(resultCh)
		return resultCh
	}

	runOne := func(idx int) {
		itemCheckpointID := fmt.Sprintf("parallel:%s:item:%d", key, idx)
		cfg := &types.RunnableConfig{Configurable: make(map[string]interface{})}
		cfg.Configurable["thread_id"] = itemCheckpointID

		// Attach bridge state so the sub-graph's checkpointer can find it.
		itemCtx := withItemCheckpointCtx(ctx, bridgeState)

		var out interface{}
		var err error
		if idx < 0 || idx >= len(items) {
			err = fmt.Errorf("parallel item index %d out of bounds (len(items)=%d)", idx, len(items))
			resultCh <- parallelItemResult{index: idx, output: out, err: err}
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("parallel item %d panic: %v", idx, r)
				}
			}()
			out, err = sub.Invoke(itemCtx, items[idx], cfg)
		}()
		resultCh <- parallelItemResult{index: idx, output: out, err: err}
	}

	// Sequential path.
	if options.maxConcurrency <= 1 {
		for _, idx := range indices {
			runOne(idx)
		}
		close(resultCh)
		return resultCh
	}

	// Concurrent path: semaphore-bounded fan-out.
	sem := make(chan struct{}, options.maxConcurrency)
	var wg sync.WaitGroup
	for i, idx := range indices {
		wg.Add(1)
		idx := idx
		if i == 0 {
			// First item runs on main goroutine.
			runOne(idx)
			wg.Done()
			continue
		}
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			runOne(idx)
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	return resultCh
}

// loadParallelSnapshot reads the parallel state from interrupt resume values.
func loadParallelSnapshot(ctx context.Context) (*parallelInterruptState, bool) {
	values := interrupt.GetResumeValues(ctx)
	for _, v := range values {
		var data []byte
		switch tv := v.(type) {
		case []byte:
			data = tv
		case string:
			data = []byte(tv)
		default:
			continue
		}
		var st parallelInterruptState
		if err := json.Unmarshal(data, &st); err == nil && st.TotalCount > 0 {
			return &st, true
		}
	}
	return nil, false
}

// itemCheckpointStore provides per-item checkpoint storage for sub-graphs.
type itemCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newItemCheckpointStore(data map[string][]byte) *itemCheckpointStore {
	cloned := make(map[string][]byte)
	for k, v := range data {
		buf := make([]byte, len(v))
		copy(buf, v)
		cloned[k] = buf
	}
	if cloned == nil {
		cloned = make(map[string][]byte)
	}
	return &itemCheckpointStore{data: cloned}
}

func (s *itemCheckpointStore) get(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[id]
	if !ok {
		return nil, false
	}
	buf := make([]byte, len(v))
	copy(buf, v)
	return buf, true
}

func (s *itemCheckpointStore) set(id string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := make([]byte, len(payload))
	copy(buf, payload)
	s.data[id] = buf
}

func (s *itemCheckpointStore) snapshot() map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]byte, len(s.data))
	for k, v := range s.data {
		buf := make([]byte, len(v))
		copy(buf, v)
		result[k] = buf
	}
	return result
}

type itemCheckpointCtxKey struct{}

func withItemCheckpointCtx(ctx context.Context, store *itemCheckpointStore) context.Context {
	return context.WithValue(ctx, itemCheckpointCtxKey{}, store)
}

func getItemCheckpointStore(ctx context.Context) *itemCheckpointStore {
	if s, ok := ctx.Value(itemCheckpointCtxKey{}).(*itemCheckpointStore); ok {
		return s
	}
	return nil
}
