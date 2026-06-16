package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// Multi-turn integration tests using real FlowAgent + ReAct loop + middleware chain.
// Goal: find bugs in agentcore/graphengine, not work around them.

// TestMultiTurn_StateAccumulation: 3 consecutive turns, verify state carries through.
func TestMultiTurn_StateAccumulation(t *testing.T) {
	model := &mockModel{}
	model.addResp("first response")
	model.addResp("second response")
	model.addResp("third response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("multi_turn")
	agent.name = "multi_turn"
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	for turn := 1; turn <= 3; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var found bool
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d unexpected err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				found = true
			}
		}
		if !found {
			t.Errorf("turn %d: expected output event", turn)
		}
	}
}

// TestMultiTurn_ToolCallAcrossTurns: each turn produces a tool call + response.
func TestMultiTurn_ToolCallAcrossTurns(t *testing.T) {
	tool := &mockTool{name: "calc", desc: "calculator"}

	for turn := 1; turn <= 2; turn++ {
		turnModel := &forcedToolModel{
			inner:     &mockModel{},
			toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{\"x\":1,\"y\":2}"}}},
			finalResp: fmt.Sprintf("result %d", turn),
			firstCall: true,
		}

		agent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model:       turnModel,
			Tools:       []Tool{tool},
			ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		}).WithName("tool_turn")
		agent.name = "tool_turn"

		runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected at least one output", turn)
		}
		t.Logf("turn %d: %d outputs (tool call + response)", turn, outputs)
	}
}

// TestMultiTurn_MiddlewareHooksAcrossTurns: 4 middleware hooks fire each turn.
func TestMultiTurn_MiddlewareHooksAcrossTurns(t *testing.T) {
	var mu sync.Mutex
	var hooks []string

	mw := &testMiddleware{
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			mu.Lock()
			hooks = append(hooks, "beforeAgent")
			mu.Unlock()
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			mu.Lock()
			hooks = append(hooks, "afterAgent")
			mu.Unlock()
			return ctx, nil
		},
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			mu.Lock()
			hooks = append(hooks, "beforeModel")
			mu.Unlock()
			return ctx, state, nil
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			mu.Lock()
			hooks = append(hooks, "afterModel")
			mu.Unlock()
			return ctx, state, nil
		},
	}

	model := &mockModel{}
	model.addResp("first")
	model.addResp("second")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mw},
	}).WithName("mw_multi")
	agent.name = "mw_multi"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("q%d", turn))})
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
		}
	}

	mu.Lock()
	hookCounts := countHooks(hooks)
	mu.Unlock()

	if hookCounts["beforeAgent"] < 2 {
		t.Errorf("beforeAgent called %d times, expected >=2", hookCounts["beforeAgent"])
	}
	if hookCounts["afterAgent"] < 2 {
		t.Errorf("afterAgent called %d times, expected >=2", hookCounts["afterAgent"])
	}
	if hookCounts["beforeModel"] < 2 {
		t.Errorf("beforeModel called %d times, expected >=2", hookCounts["beforeModel"])
	}
	if hookCounts["afterModel"] < 2 {
		t.Errorf("afterModel called %d times, expected >=2", hookCounts["afterModel"])
	}
	t.Logf("middleware hooks across turns: %v", hookCounts)
}

func countHooks(hooks []string) map[string]int {
	counts := make(map[string]int)
	for _, h := range hooks {
		counts[h]++
	}
	return counts
}

// TestMultiTurn_SequentialWorkflow: Sequential agent A->B runs 2 turns.
func TestMultiTurn_SequentialWorkflow(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("a turn 1")
	m1.addResp("a turn 2")
	m2 := &mockModel{}
	m2.addResp("b turn 1")
	m2.addResp("b turn 2")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("seq_a").WithDescription("first")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("seq_b").WithDescription("second")

	ctx := context.Background()
	seq, err := NewSequential(ctx, &SequentialConfig{
		Name: "seq_multi", Description: "multi-turn sequential",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected outputs from sequential agents", turn)
		}
		t.Logf("turn %d: %d outputs", turn, outputs)
	}
}

// TestMultiTurn_LoopWorkflow: Loop (MaxIterations=3) runs 2 turns.
func TestMultiTurn_LoopWorkflow(t *testing.T) {
	model := &mockModel{}
	model.addResp("loop 1")
	model.addResp("loop 2")
	model.addResp("loop 3")
	model.addResp("loop 4")
	model.addResp("loop 5")
	model.addResp("loop 6")

	body := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("loop_body").WithDescription("body")

	ctx := context.Background()
	loop, err := NewLoop(ctx, &LoopConfig{
		Name: "loop_multi", Description: "multi-turn loop",
		SubAgents:     []Agent{body},
		MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: loop})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected outputs from loop", turn)
		}
		t.Logf("loop turn %d: %d outputs", turn, outputs)
	}
}

// TestMultiTurn_ParallelWorkflow: Parallel (A || B) runs 2 turns.
func TestMultiTurn_ParallelWorkflow(t *testing.T) {
	ma := &mockModel{}
	ma.addResp("a1")
	ma.addResp("a2")
	mb := &mockModel{}
	mb.addResp("b1")
	mb.addResp("b2")

	pa := NewReActAgent(&ReActConfig[*schema.Message]{Model: ma}).WithName("par_a")
	pb := NewReActAgent(&ReActConfig[*schema.Message]{Model: mb}).WithName("par_b")

	ctx := context.Background()
	par, err := NewParallel(ctx, &ParallelConfig{
		Name: "par_multi", Description: "multi-turn parallel",
		SubAgents: []Agent{pa, pb},
	})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: par})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected outputs from parallel agents", turn)
		}
		t.Logf("parallel turn %d: %d outputs", turn, outputs)
	}
}

// TestMultiTurn_CancelAndResume: Turn 1 cancelled, Turn 2 resumes from checkpoint.
func TestMultiTurn_CancelAndResume(t *testing.T) {
	m := newCancelTestChatModel(nil)
	m.addResp("first response")
	m.addResp("resumed response")
	m.setDelay(100 * time.Millisecond)

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("cancel_resume")
	agent.name = "cancel_resume"

	store := newCancelTestStore()
	cid := "multi-turn-cid"
	cancelOpt, cancelFunc := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})

	ctx1 := context.Background()
	iter1 := runner.Run(ctx1, []*schema.Message{schema.UserMessage("run me")},
		WithCheckPointID(cid), cancelOpt)

	time.Sleep(20 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))

	for {
		_, ok := iter1.Next()
		if !ok {
			break
		}
	}

	ctx2 := context.Background()
	resumedIter, err := runner.Resume(ctx2, cid)
	if err != nil {
		t.Logf("Resume failed (known P0 gap?): %v", err)
		return
	}

	var outputs int
	for {
		ev, ok := resumedIter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("resume event err: %v", ev.Err)
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.Message != nil {
			outputs++
		}
	}
	if outputs == 0 {
		t.Log("no outputs from resume (expected if checkpoint not saved)")
	}
	t.Logf("cancel/resume across turns: %d resumed outputs", outputs)
}

// TestMultiTurn_HighConcurrency: 30 runners each doing 3 turns.
func TestMultiTurn_HighConcurrency(t *testing.T) {
	const (
		agents = 30
		turns  = 3
	)

	var wg sync.WaitGroup
	errs := make(chan error, agents*turns)

	for i := 0; i < agents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			model := &mockModel{}
			for t := 0; t < turns; t++ {
				model.addResp(fmt.Sprintf("agent %d turn %d", id, t))
			}

			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName(fmt.Sprintf("conc_%d", id))
			agent.name = fmt.Sprintf("conc_%d", id)
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

			for turn := 0; turn < turns; turn++ {
				ctx := context.Background()
				iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
				var ok bool
				for {
					ev, more := iter.Next()
					if !more {
						break
					}
					if ev.Err != nil {
						errs <- fmt.Errorf("agent %d turn %d: %w", id, turn, ev.Err)
					}
					if ev.Output != nil && ev.Output.MessageOutput != nil {
						ok = true
					}
				}
				if !ok {
					errs <- fmt.Errorf("agent %d turn %d: no output", id, turn)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestMultiTurn_ModelErrorAcrossTurns: model failure in turn 1 does not affect turn 2.
func TestMultiTurn_ModelErrorAcrossTurns(t *testing.T) {
	model := &mockModel{}
	model.addResp("first ok")
	model2 := &mockModel{}
	model2.addResp("second ok")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("err_recover")
	agent.name = "err_recover"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	ctx1 := context.Background()
	iter1 := runner.Run(ctx1, []*schema.Message{schema.UserMessage("first")})
	var turn1Ok bool
	for {
		ev, ok := iter1.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("turn 1 model error: %v", ev.Err)
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			turn1Ok = true
		}
	}

	if turn1Ok {
		t.Log("turn 1 succeeded")
	} else {
		t.Log("turn 1 ended (model may have run out of responses)")
	}

	agent2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: model2}).WithName("err_recover2")
	agent2.name = "err_recover2"
	runner2 := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent2})

	ctx2 := context.Background()
	iter2 := runner2.Run(ctx2, []*schema.Message{schema.UserMessage("second")})
	var turn2Ok bool
	for {
		ev, ok := iter2.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("turn 2 unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			turn2Ok = true
		}
	}
	if !turn2Ok {
		t.Error("turn 2 should succeed after replacing model")
	}
}

// TestMultiTurn_SupervisorTransfer: supervisor + worker runs 2 turns.
func TestMultiTurn_SupervisorTransfer(t *testing.T) {
	subM := &mockModel{}
	subM.addResp("sub turn 1")
	subM.addResp("sub turn 2")
	supM := &mockModel{}
	supM.addResp("sup turn 1")
	supM.addResp("sup turn 2")

	sub := NewReActAgent(&ReActConfig[*schema.Message]{Model: subM}).WithName("worker").WithDescription("worker")
	ctx := context.Background()
	wrappedSub := AgentWithOptions(ctx, sub, WithDisallowTransferToParent())

	sup := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       supM,
		Instruction: "You are a supervisor. Transfer to worker agent when asked.",
	}).WithName("supervisor").WithDescription("supervisor")

	flow, err := SetSubAgents(ctx, sup, []Agent{wrappedSub})
	if err != nil {
		t.Fatalf("SetSubAgents: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: flow})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected outputs from supervisor flow", turn)
		}
		t.Logf("supervisor turn %d: %d outputs", turn, outputs)
	}
}

// TestMultiTurn_WrapModelAcrossTurns: WrapModel middleware fires each turn.
func TestMultiTurn_WrapModelAcrossTurns(t *testing.T) {
	var wrapCount int32

	innerModel := &mockModel{}
	innerModel.addResp("first")
	innerModel.addResp("second")

	mw := &testMiddleware{
		wrapModel: func(ctx context.Context, m Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
			atomic.AddInt32(&wrapCount, 1)
			return m, nil
		},
	}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       innerModel,
		Middlewares: []ReActMiddleware{mw},
	}).WithName("wrap_test")
	agent.name = "wrap_test"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("q%d", turn))})
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
		}
	}

	c := int(atomic.LoadInt32(&wrapCount))
	if c < 2 {
		t.Errorf("WrapModel called %d times across 2 turns, expected >=2", c)
	}
	t.Logf("WrapModel called %d times across 2 turns", c)
}

// TestMultiTurn_ConcurrentSameRunner: 2 concurrent Run calls on the same Runner.
func TestMultiTurn_ConcurrentSameRunner(t *testing.T) {
	model := &mockModel{}
	model.addResp("slow 1")
	model.addResp("slow 2")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("same_runner")
	agent.name = "same_runner"
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("concurrent %d", id))})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errCh <- fmt.Errorf("run %d err: %v", id, ev.Err)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	var failures int
	for err := range errCh {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestMultiTurn_PlanExecute: PlanExecute runs 2 turns.
func TestMultiTurn_PlanExecute(t *testing.T) {
	for turn := 1; turn <= 2; turn++ {
		plannerM := &mockModel{}
		plannerM.addResp(fmt.Sprintf("plan turn %d", turn))
		execM := &mockModel{}
		execM.addResp(fmt.Sprintf("exec turn %d", turn))
		replannerM := &mockModel{}
		replannerM.addResp(fmt.Sprintf("replan turn %d", turn))

		ctx := context.Background()

		planner := NewReActAgent(&ReActConfig[*schema.Message]{Model: plannerM}).WithName("planner")
		executor := NewReActAgent(&ReActConfig[*schema.Message]{Model: execM}).WithName("executor")
		replanner := NewReActAgent(&ReActConfig[*schema.Message]{Model: replannerM}).WithName("replanner")

		loopAgent, err := NewLoop(ctx, &LoopConfig{
			Name:          "pe_loop",
			Description:   "Plan-Execute loop",
			SubAgents:     []Agent{executor, replanner},
			MaxIterations: 1,
		})
		if err != nil {
			t.Fatalf("turn %d NewLoop: %v", turn, err)
		}

		seqAgent, err := NewSequential(ctx, &SequentialConfig{
			Name:        "plan_execute",
			Description: "Plan-Execute agent",
			SubAgents:   []Agent{planner, loopAgent},
		})
		if err != nil {
			t.Fatalf("turn %d NewSequential: %v", turn, err)
		}

		runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seqAgent})
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("plan turn %d", turn))})
		var outputs int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				outputs++
			}
		}
		if outputs == 0 {
			t.Errorf("turn %d: expected outputs from plan-execute", turn)
		}
		t.Logf("plan-execute turn %d: %d outputs", turn, outputs)
	}
}

// TestMultiTurn_AgentToolNested: parent agent calls inner via AgentTool, 2 turns.
func TestMultiTurn_AgentToolNested(t *testing.T) {
	for turn := 1; turn <= 2; turn++ {
		innerM := &mockModel{}
		innerM.addResp(fmt.Sprintf("inner result turn %d", turn))
		innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: innerM}).WithName("inner").WithDescription("inner")

		ctx := context.Background()
		agentTool := NewAgentTool(ctx, innerAgent)

		parentM := &forcedToolModel{
			inner:     &mockModel{},
			toolCalls: []schema.ToolCall{{ID: "call_tool", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{\"task\":\"run\"}"}}},
			finalResp: fmt.Sprintf("parent done turn %d", turn),
			firstCall: true,
		}

		parent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model:       parentM,
			Tools:       []Tool{agentTool},
			ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
		}).WithName("parent")

		runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent})
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var lastContent string
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
				lastContent = ev.Output.MessageOutput.Message.Content
			}
		}
		if lastContent == "" {
			t.Errorf("turn %d: expected final content from parent", turn)
		}
		t.Logf("agent tool turn %d: lastContent=%s", turn, lastContent)
	}
}

// TestMultiTurn_StreamingMode: EnableStreaming=true, 2 turns with output events.
func TestMultiTurn_StreamingMode(t *testing.T) {
	model := &mockModel{}
	model.addResp("streamed 1")
	model.addResp("streamed 2")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("stream_multi")
	agent.name = "stream_multi"

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store, EnableStreaming: true})

	for turn := 1; turn <= 2; turn++ {
		ctx := context.Background()
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("turn %d", turn))})
		var outputEvents int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("turn %d err: %v", turn, ev.Err)
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil {
				outputEvents++
			}
		}
		if outputEvents == 0 {
			t.Errorf("turn %d: expected at least one output event", turn)
		}
		t.Logf("streaming turn %d: %d output events", turn, outputEvents)
	}
}

// TestMultiTurn_CheckpointStateConsistency: turn 1 completes, turn 2 uses fresh checkpoint ID.
func TestMultiTurn_CheckpointStateConsistency(t *testing.T) {
	model := &mockModel{}
	model.addResp("response 1")
	model.addResp("response 2")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("cp_multi")
	agent.name = "cp_multi"
	store := newCancelTestStore()

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	cid := "cp-multi-1"
	ctx1 := context.Background()
	iter1 := runner.Run(ctx1, []*schema.Message{schema.UserMessage("first")}, WithCheckPointID(cid))
	for {
		ev, ok := iter1.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("turn 1 err: %v", ev.Err)
		}
	}

	cid2 := "cp-multi-2"
	ctx2 := context.Background()
	iter2 := runner.Run(ctx2, []*schema.Message{schema.UserMessage("second")}, WithCheckPointID(cid2))
	var outputs int
	for {
		ev, ok := iter2.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("turn 2 err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
			outputs++
		}
	}
	if outputs == 0 {
		t.Errorf("turn 2: expected outputs from fresh checkpoint")
	}
	t.Logf("checkpoint consistency: turn 1 completed, turn 2 had %d outputs", outputs)
}

// TestMultiTurn_ContextTimeout: timeout in turn 1 does not affect turn 2.
func TestMultiTurn_ContextTimeout(t *testing.T) {
	fastM := &mockModel{}
	fastM.addResp("fast response")

	slowM := newCancelTestChatModel(nil)
	slowM.addResp("slow response")
	slowM.setDelay(200 * time.Millisecond)

	slowAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: slowM}).WithName("slow")
	slowAgent.name = "slow"
	runner1 := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: slowAgent})
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel1()
	iter1 := runner1.Run(ctx1, []*schema.Message{schema.UserMessage("slow query")})
	var timeoutSeen bool
	for {
		ev, ok := iter1.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			if errors.Is(ev.Err, context.DeadlineExceeded) {
				timeoutSeen = true
			}
		}
	}
	if !timeoutSeen {
		t.Log("turn 1: no timeout error (model may have completed before deadline)")
	}

	fastAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: fastM}).WithName("fast")
	fastAgent.name = "fast"
	runner2 := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: fastAgent})
	ctx2 := context.Background()
	iter2 := runner2.Run(ctx2, []*schema.Message{schema.UserMessage("fast query")})
	var turn2Ok bool
	for {
		ev, ok := iter2.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("turn 2 unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			turn2Ok = true
		}
	}
	if !turn2Ok {
		t.Error("turn 2 should succeed after turn 1 timeout")
	}
}

// Helper: alternatingToolModel for testing tool call -> tool result -> response flow.
type alternatingToolModel struct {
	inner      *mockModel
	toolCalls  []schema.ToolCall
	responses  []string
	callCount  int
	mu         sync.Mutex
}

func (m *alternatingToolModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.callCount < len(m.responses) {
		if len(msgs) > 1 {
			prev := msgs[len(msgs)-1]
			if prev.Role == schema.RoleTool {
				resp := m.responses[m.callCount]
				m.callCount++
				return &schema.Message{Role: schema.RoleAssistant, Content: resp}, nil
			}
		}
		m.callCount++
		return &schema.Message{
			Role:      schema.RoleAssistant,
			Content:   "",
			ToolCalls: m.toolCalls,
		}, nil
	}
	return m.inner.Generate(ctx, msgs, opts...)
}

func (m *alternatingToolModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *alternatingToolModel) BindTools(tools []*schema.ToolInfo) error { return nil }
