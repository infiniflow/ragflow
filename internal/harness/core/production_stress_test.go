package core

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================================
// Production-Scale Stress Tests
//
// These tests are designed to verify that both agentcore and graphengine can
// handle production-level loads: massive concurrency, long soaks, error storms,
// cancellation pressure, and large-state graphs.
//
// Run with: go test -race -timeout 120s ./agentcore/ -run "TestProduction_"
// ============================================================================

// ---- helpers ----

// makeWorkflowGraphAgents creates N ReAct agents with mock tools and forcedToolModel.
func makeWorkflowGraphAgents(n int, prefix string) ([]Agent, []Tool) {
	agents := make([]Agent, n)
	tools := make([]Tool, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s_%d", prefix, i)
		tools[i] = &mockTool{name: "tool_" + name, desc: name}
		agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{
			Model: &forcedToolModel{
				toolCalls: []schema.ToolCall{{
					ID:       fmt.Sprintf("c%d", i),
					Function: schema.ToolCallFunction{Name: tools[i].Name(), Arguments: "{}"},
				}},
				finalResp: fmt.Sprintf("done from %s", name),
			},
			Tools: []Tool{tools[i]},
		}).WithName(name)
	}
	return agents, tools
}

// runGraphAndCollect drains all events from a graph execution.
func runGraphAndCollect(t testing.TB, wfg *WorkflowGraph, input *AgentInput) (msgCount int, hasError bool) {
	t.Helper()
	ctx := context.Background()
	s, err := wfg.Invoke(ctx, input)
	if err != nil {
		return 0, true
	}
	if s == nil {
		return 0, false
	}
	return len(s.Messages), false
}

// ============================================================================
// Test 1: Massive Concurrent StateGraphs
// 100 concurrent StateGraphs with 15 nodes each. Run under -race.
// ============================================================================

func TestProduction_MassiveConcurrentGraphs(t *testing.T) {
	const numGraphs = 100
	const nodesPerGraph = 15

	var wg sync.WaitGroup
	errCh := make(chan error, numGraphs)
	msgCounts := make([]int32, numGraphs)

	for i := 0; i < numGraphs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("graph %d panic: %v", id, r)
				}
			}()

			// Build a custom StateGraph.
			sg := graph.NewStateGraph(&dagState{})
			for j := 0; j < nodesPerGraph; j++ {
				idx := j
				name := fmt.Sprintf("g%d_n%d", id, j)
				sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
					s := state.(*dagState)
					s.Messages = append(s.Messages, name)
					s.Step = idx
					return s, nil
				})
			}
			sg.AddEdge(constants.Start, fmt.Sprintf("g%d_n0", id))
			for j := 1; j < nodesPerGraph; j++ {
				sg.AddEdge(fmt.Sprintf("g%d_n%d", id, j-1), fmt.Sprintf("g%d_n%d", id, j))
			}
			sg.AddEdge(fmt.Sprintf("g%d_n%d", id, nodesPerGraph-1), constants.End)

			compiled, compileErr := sg.Compile(graph.WithRecursionLimit(nodesPerGraph + 5))
			if compileErr != nil {
				errCh <- fmt.Errorf("graph %d compile: %w", id, compileErr)
				return
			}

			stateIf, invokeErr := compiled.Invoke(context.Background(), &dagState{})
			if invokeErr != nil {
				errCh <- fmt.Errorf("graph %d invoke: %w", id, invokeErr)
				return
			}
			if s, ok := stateIf.(*dagState); ok {
				atomic.StoreInt32(&msgCounts[id], int32(len(s.Messages)))
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		t.Fatalf("%d/%d graphs failed. First error: %v", len(errs), numGraphs, errs[0])
	}
	t.Logf("Massive concurrent: %d graphs x %d nodes = %d total nodes, all OK", numGraphs, nodesPerGraph, numGraphs*nodesPerGraph)
}

// ============================================================================
// Test 2: Mixed Workload High Concurrency
// SequentialGraph + ParallelGraph + LoopGraph + custom StateGraph, all running together.
// ============================================================================

func TestProduction_MixedWorkloadHighConcurrency(t *testing.T) {
	const seqCount = 10
	const parCount = 10
	const loopCount = 5
	const customCount = 10
	total := seqCount + parCount + loopCount + customCount

	type result struct {
		id   string
		err  error
		msgs int
	}
	results := make(chan result, total)
	var wg sync.WaitGroup

	// Sequential graphs.
	for i := 0; i < seqCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- result{fmt.Sprintf("seq_%d", id), fmt.Errorf("panic: %v", r), 0}
				}
			}()
			agents, _ := makeWorkflowGraphAgents(5, fmt.Sprintf("seq_%d", id))
			wfg, err := NewSequentialGraph(context.Background(), &SequentialConfig{
				Name: fmt.Sprintf("seq_%d", id), Description: "sequential", SubAgents: agents,
			}, nil)
			if err != nil {
				results <- result{fmt.Sprintf("seq_%d", id), err, 0}
				return
			}
			msgs, hasErr := runGraphAndCollect(t, wfg, &AgentInput{
				Messages: []Message{schema.UserMessage(fmt.Sprintf("seq %d", id))},
			})
			if hasErr {
				results <- result{fmt.Sprintf("seq_%d", id), fmt.Errorf("failed"), 0}
				return
			}
			results <- result{fmt.Sprintf("seq_%d", id), nil, msgs}
		}(i)
	}

	// Parallel graphs.
	for i := 0; i < parCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- result{fmt.Sprintf("par_%d", id), fmt.Errorf("panic: %v", r), 0}
				}
			}()
			agents, _ := makeWorkflowGraphAgents(3, fmt.Sprintf("par_%d", id))
			wfg, err := NewParallelGraph(context.Background(), &ParallelConfig{
				Name: fmt.Sprintf("par_%d", id), Description: "parallel", SubAgents: agents,
			}, nil)
			if err != nil {
				results <- result{fmt.Sprintf("par_%d", id), err, 0}
				return
			}
			msgs, hasErr := runGraphAndCollect(t, wfg, &AgentInput{
				Messages: []Message{schema.UserMessage(fmt.Sprintf("par %d", id))},
			})
			if hasErr {
				results <- result{fmt.Sprintf("par_%d", id), fmt.Errorf("failed"), 0}
				return
			}
			results <- result{fmt.Sprintf("par_%d", id), nil, msgs}
		}(i)
	}

	// Loop graphs.
	for i := 0; i < loopCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- result{fmt.Sprintf("loop_%d", id), fmt.Errorf("panic: %v", r), 0}
				}
			}()
			agents, _ := makeWorkflowGraphAgents(3, fmt.Sprintf("loop_%d", id))
			wfg, err := NewLoopGraph(context.Background(), &LoopConfig{
				Name: fmt.Sprintf("loop_%d", id), Description: "loop", SubAgents: agents,
			}, nil)
			if err != nil {
				results <- result{fmt.Sprintf("loop_%d", id), err, 0}
				return
			}
			msgs, hasErr := runGraphAndCollect(t, wfg, &AgentInput{
				Messages: []Message{schema.UserMessage(fmt.Sprintf("loop %d", id))},
			})
			if hasErr {
				results <- result{fmt.Sprintf("loop_%d", id), fmt.Errorf("failed"), 0}
				return
			}
			results <- result{fmt.Sprintf("loop_%d", id), nil, msgs}
		}(i)
	}

	// Custom StateGraphs (DAG fan-in).
	for i := 0; i < customCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- result{fmt.Sprintf("dag_%d", id), fmt.Errorf("panic: %v", r), 0}
				}
			}()
			sg := graph.NewStateGraph(&dagState{})
			sg.NodeTriggerMode = types.NodeTriggerAllPredecessor
			branchCount := 5

			sg.AddNode(fmt.Sprintf("s_%d", id), func(ctx context.Context, state interface{}) (interface{}, error) {
				return state, nil
			})
			sg.AddEdge(constants.Start, fmt.Sprintf("s_%d", id))

			for b := 0; b < branchCount; b++ {
				bName := fmt.Sprintf("b%d_%d", id, b)
				sg.AddNode(bName, func(ctx context.Context, state interface{}) (interface{}, error) {
					s := state.(*dagState)
					s.Messages = append(s.Messages, bName)
					return s, nil
				})
				sg.AddEdge(fmt.Sprintf("s_%d", id), bName)
			}

			mName := fmt.Sprintf("m_%d", id)
			sg.AddNode(mName, func(ctx context.Context, state interface{}) (interface{}, error) {
				return state, nil
			})
			for b := 0; b < branchCount; b++ {
				sg.AddEdge(fmt.Sprintf("b%d_%d", id, b), mName)
			}
			sg.AddEdge(mName, constants.End)

			compiled, err := sg.Compile(
				graph.WithRecursionLimit(branchCount+5),
				graph.WithNodeTriggerMode(types.NodeTriggerAllPredecessor),
			)
			if err != nil {
				results <- result{fmt.Sprintf("dag_%d", id), err, 0}
				return
			}
			_, invokeErr := compiled.Invoke(context.Background(), &dagState{})
			if invokeErr != nil {
				results <- result{fmt.Sprintf("dag_%d", id), invokeErr, 0}
				return
			}
			results <- result{fmt.Sprintf("dag_%d", id), nil, 1}
		}(i)
	}

	wg.Wait()
	close(results)

	var errs []error
	totalMsgs := 0
	for r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.id, r.err))
		}
		totalMsgs += r.msgs
	}
	if len(errs) > 0 {
		t.Fatalf("%d/%d workloads failed: %v", len(errs), total, errs[0])
	}
	t.Logf("Mixed workload: %d graphs (seq=%d, par=%d, loop=%d, dag=%d), total msgs=%d",
		total, seqCount, parCount, loopCount, customCount, totalMsgs)
}

// ============================================================================
// Test 3: Soak — 1000 sequential graph executions
// Detect goroutine leaks after repeated execution.
// ============================================================================

func TestProduction_Soak_1000Executions(t *testing.T) {
	agents, _ := makeWorkflowGraphAgents(5, "soak")
	wfg, err := NewSequentialGraph(context.Background(), &SequentialConfig{
		Name: "soak", Description: "soak test", SubAgents: agents,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	startGoroutines := runtime.NumGoroutine()
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		_, hasErr := runGraphAndCollect(t, wfg, &AgentInput{
			Messages: []Message{schema.UserMessage(fmt.Sprintf("soak %d", i))},
		})
		if hasErr {
			t.Fatalf("iteration %d failed", i)
		}
		if i%200 == 199 {
			runtime.GC()
		}
	}

	endGoroutines := runtime.NumGoroutine()
	leaked := endGoroutines - startGoroutines
	if leaked > 10 {
		t.Errorf("Potential goroutine leak: %d -> %d goroutines (delta=%d)", startGoroutines, endGoroutines, leaked)
	} else {
		t.Logf("Soak 1000: %d iterations, goroutines %d -> %d (delta=%d), no leak", iterations, startGoroutines, endGoroutines, leaked)
	}
}

// ============================================================================
// Test 4: Error Recovery Under Load
// 50% of graphs have failing nodes, 50% normal. All run concurrently.
// ============================================================================

func TestProduction_ErrorRecoveryUnderLoad(t *testing.T) {
	const totalGraphs = 50

	type graphResult struct {
		id     int
		hasErr bool
		panic  bool
	}
	results := make(chan graphResult, totalGraphs)
	var wg sync.WaitGroup

	for i := 0; i < totalGraphs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			shouldFail := id%2 == 0

			defer func() {
				if r := recover(); r != nil {
					results <- graphResult{id: id, panic: true}
				}
			}()

			sg := graph.NewStateGraph(&dagState{})
			sg.AddNode("pre", func(ctx context.Context, state interface{}) (interface{}, error) {
				return state, nil
			})
			sg.AddEdge(constants.Start, "pre")

			if shouldFail {
				sg.AddNode("failer", func(ctx context.Context, state interface{}) (interface{}, error) {
					return nil, fmt.Errorf("injected failure in graph %d", id)
				})
				sg.AddEdge("pre", "failer")
				sg.AddEdge("failer", constants.End)
			} else {
				sg.AddNode("worker", func(ctx context.Context, state interface{}) (interface{}, error) {
					s := state.(*dagState)
					s.Messages = append(s.Messages, "ok")
					return s, nil
				})
				sg.AddEdge("pre", "worker")
				sg.AddEdge("worker", constants.End)
			}

			compiled, err := sg.Compile(graph.WithRecursionLimit(10))
			if err != nil {
				results <- graphResult{id: id, hasErr: true}
				return
			}
			_, invokeErr := compiled.Invoke(context.Background(), &dagState{})
			results <- graphResult{id: id, hasErr: invokeErr != nil, panic: false}
		}(i)
	}
	wg.Wait()
	close(results)

	var normalOK, failedOK, normalErr, failedErr int
	for r := range results {
		isFailer := r.id%2 == 0
		if r.hasErr || r.panic {
			if isFailer {
				failedOK++ // expected
			} else {
				normalErr++ // unexpected
			}
		} else {
			if isFailer {
				failedErr++ // unexpected (failer should have errored)
			} else {
				normalOK++ // expected
			}
		}
	}
	if normalErr > 0 {
		t.Errorf("%d normal graphs unexpectedly errored", normalErr)
	}
	if failedErr > 0 {
		t.Errorf("%d failer graphs unexpectedly succeeded", failedErr)
	}
	t.Logf("Error recovery: normalOK=%d, failerOK(exp)=%d, unexpected normalErr=%d, unexpected failerOK=%d",
		normalOK, failedOK, normalErr, failedErr)
}

// ============================================================================
// Test 5: Cancel Storm — Rapid create/cancel of many graphs
// ============================================================================

func TestProduction_CancelStorm(t *testing.T) {
	const totalOps = 500

	agents, _ := makeWorkflowGraphAgents(10, "storm")

	for i := 0; i < totalOps; i++ {
		func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			wfg, err := NewSequentialGraph(ctx, &SequentialConfig{
				Name: fmt.Sprintf("storm_%d", i), Description: "cancel storm", SubAgents: agents,
			}, nil)
			if err != nil {
				t.Logf("Op %d: compile error (expected under pressure): %v", i, err)
				return
			}

			// Cancel immediately.
			cancel()
			_, invokeErr := wfg.Invoke(ctx, &AgentInput{
				Messages: []Message{schema.UserMessage(fmt.Sprintf("storm %d", i))},
			})

			// Both success and error (due to cancellation) are acceptable.
			_ = invokeErr
		}()
	}
	t.Logf("Cancel storm: %d rapid create/cancel ops completed without crash", totalOps)
}

// ============================================================================
// Test 6: Large State Graph — verify engine handles expanding state gracefully
// ============================================================================

func TestProduction_LargeStateGraph(t *testing.T) {
	const numMessages = 100
	const numNodes = 20

	sg := graph.NewStateGraph(&dagState{})

	sg.AddNode("source", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(*dagState)
		for i := 0; i < numMessages; i++ {
			s.Messages = append(s.Messages, fmt.Sprintf("msg_%d", i))
		}
		return s, nil
	})
	sg.AddEdge(constants.Start, "source")

	for i := 0; i < numNodes; i++ {
		idx := i
		name := fmt.Sprintf("node_%d", i)
		sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*dagState)
			s.Step = idx
			return s, nil
		})
		if i == 0 {
			sg.AddEdge("source", name)
		} else {
			sg.AddEdge(fmt.Sprintf("node_%d", i-1), name)
		}
	}
	sg.AddEdge(fmt.Sprintf("node_%d", numNodes-1), constants.End)

	compiled, err := sg.Compile(graph.WithRecursionLimit(numNodes + 10))
	if err != nil {
		t.Fatal(err)
	}

	stateIf, err := compiled.Invoke(context.Background(), &dagState{})
	if err != nil {
		t.Fatalf("Large state graph failed: %v", err)
	}

	t.Logf("Large state: %d messages + %d nodes, state type=%T, result=%+v", numMessages, numNodes, stateIf, stateIf)
}

// ============================================================================
// Test 7: Checkpoint Pressure — 50 concurrent graphs with checkpointing
// ============================================================================

func TestProduction_CheckpointPressure(t *testing.T) {
	const numGraphs = 50
	const nodesPerGraph = 10

	var wg sync.WaitGroup
	errCh := make(chan error, numGraphs)

	for i := 0; i < numGraphs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("graph %d panic: %v", id, r)
				}
			}()

			memSaver := checkpoint.NewMemorySaver()
			sg := graph.NewStateGraph(&dagState{})
			for j := 0; j < nodesPerGraph; j++ {
				idx := j
				name := fmt.Sprintf("n%d_g%d", j, id)
				sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
					s := state.(*dagState)
					s.Messages = append(s.Messages, name)
					s.Step = idx
					return s, nil
				})
			}
			sg.AddEdge(constants.Start, fmt.Sprintf("n0_g%d", id))
			for j := 1; j < nodesPerGraph; j++ {
				sg.AddEdge(fmt.Sprintf("n%d_g%d", j-1, id), fmt.Sprintf("n%d_g%d", j, id))
			}
			sg.AddEdge(fmt.Sprintf("n%d_g%d", nodesPerGraph-1, id), constants.End)

			compiled, compileErr := sg.Compile(
				graph.WithRecursionLimit(nodesPerGraph+5),
				graph.WithCheckpointer(memSaver),
			)
			if compileErr != nil {
				errCh <- fmt.Errorf("graph %d compile: %w", id, compileErr)
				return
			}

			_, invokeErr := compiled.Invoke(context.Background(), &dagState{})
			if invokeErr != nil {
				errCh <- fmt.Errorf("graph %d invoke: %w", id, invokeErr)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		t.Fatalf("%d/%d checkpoint graphs failed: %v", len(errs), numGraphs, errs[0])
	}
	t.Logf("Checkpoint pressure: %d concurrent graphs with checkpointing, all OK", numGraphs)
}

// ============================================================================
// Test 8: Rapid Sequential — 1000 back-to-back graph compile + invoke
// ============================================================================

func TestProduction_RapidSequential(t *testing.T) {
	const iterations = 1000
	startGoroutines := runtime.NumGoroutine()

	for i := 0; i < iterations; i++ {
		sg := graph.NewStateGraph(&dagState{})
		sg.AddNode("a", func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(*dagState)
			s.Messages = append(s.Messages, "a")
			return s, nil
		})
		sg.AddEdge(constants.Start, "a")
		sg.AddEdge("a", constants.End)

		compiled, err := sg.Compile(graph.WithRecursionLimit(10))
		if err != nil {
			t.Fatalf("iter %d compile: %v", i, err)
		}
		_, err = compiled.Invoke(context.Background(), &dagState{})
		if err != nil {
			t.Fatalf("iter %d invoke: %v", i, err)
		}
		if i%200 == 199 {
			runtime.GC()
		}
	}

	endGoroutines := runtime.NumGoroutine()
	leaked := endGoroutines - startGoroutines
	if leaked > 10 {
		t.Errorf("Potential goroutine leak: %d -> %d (delta=%d)", startGoroutines, endGoroutines, leaked)
	}
	t.Logf("Rapid sequential: %d iterations, goroutines %d -> %d (delta=%d)", iterations, startGoroutines, endGoroutines, leaked)
}

// ============================================================================
// Test 9: Topic Channel Under Load — concurrent writes to a topic channel
// ============================================================================

func TestProduction_TopicChannelUnderLoad(t *testing.T) {
	const writerCount = 20
	const msgsPerWriter = 100

	topic := channels.NewTopic(nil, true)
	var mu sync.Mutex // Topic's Update is not goroutine-safe

	var wg sync.WaitGroup
	for i := 0; i < writerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				mu.Lock()
				topic.Update([]interface{}{fmt.Sprintf("w%d_m%d", id, j)})
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	val, err := topic.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	items, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", val)
	}
	expected := writerCount * msgsPerWriter
	if len(items) != expected {
		t.Errorf("expected %d items, got %d", expected, len(items))
	}
	t.Logf("Topic channel: %d writers x %d = %d items, OK", writerCount, msgsPerWriter, len(items))
}

// ============================================================================
// Test 10: BinaryOperator Aggregate Under Load — concurrent writes to binop channel
// ============================================================================

func TestProduction_BinOpChannelUnderLoad(t *testing.T) {
	const writerCount = 50
	const opsPerWriter = 100

	binop := channels.NewBinaryOperatorAggregate(0, func(a, b interface{}) interface{} {
		ai, aok := a.(int)
		bi, bok := b.(int)
		if aok && bok {
			return ai + bi
		}
		return a
	})
	var mu sync.Mutex // BinaryOperatorAggregate's Update is not goroutine-safe

	var wg sync.WaitGroup
	for i := 0; i < writerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				mu.Lock()
				binop.Update([]interface{}{1})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	valIf, err := binop.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	val, ok := valIf.(int)
	if !ok {
		t.Fatalf("expected int, got %T", valIf)
	}
	expected := writerCount * opsPerWriter
	if val != expected {
		t.Errorf("expected %d, got %d", expected, val)
	}
	t.Logf("BinOp channel: %d writers x %d ops = %d", writerCount, opsPerWriter, val)
}
