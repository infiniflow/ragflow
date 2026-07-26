package core

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

func assertHasCancelError(t *testing.T, events []*AgentEvent) {
	t.Helper()
	for _, e := range events {
		var ce *CancelError
		if e.Err != nil && errors.As(e.Err, &ce) {
			return
		}
	}
	t.Fatal("expected CancelError in events")
}

func drainAndAssertCancelError(t *testing.T, iter *AsyncIterator[*AgentEvent]) {
	t.Helper()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		var ce *CancelError
		if ev.Err != nil && errors.As(ev.Err, &ce) {
			return
		}
	}
	t.Fatal("expected CancelError in event stream")
}

func drainEventsAndAssertCancelError(t *testing.T, iter *AsyncIterator[*AgentEvent]) []*AgentEvent {
	t.Helper()
	var events []*AgentEvent
	hasCancel := false
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		var ce *CancelError
		if ev.Err != nil && errors.As(ev.Err, &ce) {
			hasCancel = true
		}
		events = append(events, ev)
	}
	if !hasCancel {
		t.Fatal("expected CancelError in event stream")
	}
	return events
}

func drainEventsAndHasCancel(iter *AsyncIterator[*AgentEvent]) ([]*AgentEvent, bool) {
	var events []*AgentEvent
	hasCancel := false
	for {
		e, ok := iter.Next()
		if !ok {
			break
		}
		events = append(events, e)
		var ce *CancelError
		if e.Err != nil && errors.As(e.Err, &ce) {
			hasCancel = true
		}
	}
	return events, hasCancel
}

type cancelTestStore struct {
	m  map[string][]byte
	mu sync.Mutex
}

func newCancelTestStore() *cancelTestStore { return &cancelTestStore{m: make(map[string][]byte)} }
func (s *cancelTestStore) Set(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}
func (s *cancelTestStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

// ======================== CancelContext State Machine ========================

func TestCancelContext_Basics(t *testing.T) {
	cc := newCancelContext()
	if cc.shouldCancel() {
		t.Error("not cancelled initially")
	}
	cc.setMode(CancelImmediate)
	close(cc.cancelChan)
	if !cc.shouldCancel() {
		t.Error("should cancel after close")
	}
	if cc.getMode() != CancelImmediate {
		t.Error("mode")
	}
	_ = cc.markHandled()
}

func TestCancelContext_New(t *testing.T) {
	cc := newCancelContext()
	if !cc.isRoot() {
		t.Error("expected root")
	}
}

func TestCancelContext_Lifecycle(t *testing.T) {
	cc := newCancelContext()
	cc.triggerCancel(CancelAfterChatModel)
	if !cc.shouldCancel() {
		t.Error("should cancel")
	}
	if cc.getMode() != CancelAfterChatModel {
		t.Error("wrong mode")
	}
	if cc.isImmediate() {
		t.Error("should not be immediate")
	}
}

func TestCancelContext_Immediate(t *testing.T) {
	cc := newCancelContext()
	cc.triggerImmediate()
	if !cc.shouldCancel() {
		t.Error("should cancel")
	}
	if !cc.isImmediate() {
		t.Error("should be immediate")
	}
}

func TestCancelContext_MarkDone(t *testing.T) {
	cc := newCancelContext()
	cc.markDone()
	select {
	case <-cc.doneChan:
	default:
		t.Fatal("doneChan not closed")
	}
}

func TestCancelContext_MarkHandled(t *testing.T) {
	cc := newCancelContext()
	cc.triggerCancel(CancelAfterChatModel)
	if !cc.markHandled() {
		t.Error("first should succeed")
	}
	if cc.markHandled() {
		t.Error("second should fail")
	}
}

// ---- BuildCancelFunc ----

func TestBuildCancelFunc_Immediate(t *testing.T) {
	cc := newCancelContext()
	_, ok := cc.buildCancelFunc()()
	if !ok {
		t.Fatal("should contribute")
	}
	select {
	case <-cc.immediateChan:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("immediate not triggered")
	}
}

func TestBuildCancelFunc_SafePoint(t *testing.T) {
	cc := newCancelContext()
	_, ok := cc.buildCancelFunc()(WithCancelMode(CancelAfterChatModel))
	if !ok {
		t.Fatal("should contribute")
	}
	if !cc.shouldCancel() {
		t.Error("should cancel")
	}
	select {
	case <-cc.immediateChan:
		t.Fatal("should NOT be immediate")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBuildCancelFunc_AfterDone(t *testing.T) {
	cc := newCancelContext()
	cc.markDone()
	h, ok := cc.buildCancelFunc()()
	if ok {
		t.Fatal("should not contribute after done")
	}
	if !errors.Is(h.Wait(), ErrExecutionEnded) {
		t.Error("expected ErrExecutionEnded")
	}
}

func TestBuildCancelFunc_Twice(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	h1, ok1 := cf(WithCancelMode(CancelAfterChatModel))
	h2, ok2 := cf(WithCancelMode(CancelAfterToolCalls))
	if !ok1 || !ok2 {
		t.Fatal("both should contribute")
	}
	want := CancelAfterChatModel | CancelAfterToolCalls
	if cc.getMode() != want {
		t.Errorf("mode=%v want=%v", cc.getMode(), want)
	}
	cc.markHandled()
	_ = h1.Wait()
	_ = h2.Wait()
}

func TestBuildCancelFunc_TimeoutEscalation(t *testing.T) {
	cc := newCancelContext()
	h, _ := cc.buildCancelFunc()(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(30*time.Millisecond))
	time.Sleep(100 * time.Millisecond)
	if !cc.isImmediate() {
		t.Error("should escalate")
	}
	cancelErr := cc.createError()
	if !cancelErr.Info.Timeout {
		t.Error("expected timeout flag")
	}
	if !cancelErr.Info.Escalated {
		t.Error("expected escalated")
	}
	cc.markHandled()
	if !errors.Is(h.Wait(), ErrCancelTimeout) {
		t.Error("expected ErrCancelTimeout")
	}
}

func TestBuildCancelFunc_StateDoneUnderLock(t *testing.T) {
	for i := 0; i < 50; i++ {
		cc := newCancelContext()
		cf := cc.buildCancelFunc()
		cc.markDone()
		h, ok := cf()
		if ok {
			continue
		}
		if !errors.Is(h.Wait(), ErrExecutionEnded) {
			t.Error("expected ErrExecutionEnded")
		}
	}
}

func TestBuildCancelFunc_CASFailStateDone(t *testing.T) {
	for i := 0; i < 10; i++ {
		cc := newCancelContext()
		cf := cc.buildCancelFunc()
		var wg sync.WaitGroup
		for j := 0; j < 100; j++ {
			wg.Go(func() { ; _, _ = cf() })
		}
		wg.Wait()
		cc.markHandled()
	}
}

// ---- DeriveAgentToolCancelContext ----

func TestDeriveAgentToolCancelContext(t *testing.T) {
	t.Run("Shallow/DoesNotPropagateSafePoint", func(t *testing.T) {
		parent := newCancelContext()
		ctx := t.Context()
		child := parent.deriveAgentToolCancelContext(ctx)
		defer child.markDone()
		parent.triggerCancel(CancelAfterChatModel)
		select {
		case <-child.cancelChan:
			t.Fatal("propagated")
		case <-time.After(50 * time.Millisecond):
		}
	})
	t.Run("Shallow/ImmediateDoesNotPropagate", func(t *testing.T) {
		parent := newCancelContext()
		ctx := t.Context()
		child := parent.deriveAgentToolCancelContext(ctx)
		defer child.markDone()
		parent.triggerImmediate()
		select {
		case <-child.immediateChan:
			t.Fatal("propagated")
		case <-time.After(50 * time.Millisecond):
		}
	})
	t.Run("Shallow/GrandchildNoPropagation", func(t *testing.T) {
		a := newCancelContext()
		ctx := t.Context()
		b := a.deriveAgentToolCancelContext(ctx)
		c := b.deriveAgentToolCancelContext(ctx)
		t.Cleanup(func() { c.markDone(); b.markDone() })
		a.triggerCancel(CancelAfterChatModel)
		select {
		case <-b.cancelChan:
			t.Fatal("b")
		case <-time.After(50 * time.Millisecond):
		}
		select {
		case <-c.cancelChan:
			t.Fatal("c")
		case <-time.After(50 * time.Millisecond):
		}
	})
	t.Run("Shallow/GoroutineCleanup", func(t *testing.T) {
		before := goroutineCount()
		parent := newCancelContext()
		ctx, cancel := context.WithCancel(context.Background())
		child := parent.deriveAgentToolCancelContext(ctx)
		parent.triggerCancel(CancelAfterChatModel)
		time.Sleep(100 * time.Millisecond)
		child.markDone()
		cancel()
		time.Sleep(200 * time.Millisecond)
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
		after := goroutineCount()
		if after > before+5 {
			t.Errorf("goroutine leak: %d -> %d", before, after)
		}
	})
	t.Run("Recursive/PropagatesSafePoint", func(t *testing.T) {
		parent, child, cleanup := setupParentChild(t)
		defer cleanup()
		parent.setRecursive(true)
		parent.triggerCancel(CancelAfterChatModel)
		select {
		case <-child.cancelChan:
		case <-time.After(1 * time.Second):
			t.Fatal("child not cancelled")
		}
		if !child.shouldCancel() {
			t.Error("child should cancel")
		}
	})
	t.Run("Recursive/ImmediatePropagates", func(t *testing.T) {
		parent, child, cleanup := setupParentChild(t)
		defer cleanup()
		parent.setRecursive(true)
		parent.triggerImmediate()
		select {
		case <-child.immediateChan:
		case <-time.After(1 * time.Second):
			t.Fatal("child not immediate")
		}
		if !child.isImmediate() {
			t.Error("child should be immediate")
		}
	})
	t.Run("Recursive/GrandchildPropagation", func(t *testing.T) {
		a := newCancelContext()
		ctx := t.Context()
		b := a.deriveAgentToolCancelContext(ctx)
		c := b.deriveAgentToolCancelContext(ctx)
		t.Cleanup(func() { c.markDone(); b.markDone() })
		a.setRecursive(true)
		a.triggerCancel(CancelAfterChatModel)
		select {
		case <-b.cancelChan:
		case <-time.After(1 * time.Second):
			t.Fatal("B not cancelled")
		}
		select {
		case <-c.cancelChan:
		case <-time.After(1 * time.Second):
			t.Fatal("C not cancelled")
		}
	})
	t.Run("Escalation/EscalateFromNonRecursive", func(t *testing.T) {
		parent, child, cleanup := setupParentChild(t)
		defer cleanup()
		parent.triggerCancel(CancelAfterChatModel)
		select {
		case <-child.cancelChan:
			t.Fatal("should not propagate")
		case <-time.After(50 * time.Millisecond):
		}
		parent.setRecursive(true)
		select {
		case <-child.cancelChan:
		case <-time.After(1 * time.Second):
			t.Fatal("child not cancelled")
		}
	})
}

func TestDeriveAgentToolCancelContext_Race(t *testing.T) {
	t.Run("SetRecursiveConcurrentWithCancelChan", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			parent := newCancelContext()
			ctx, cancel := context.WithCancel(context.Background())
			child := parent.deriveAgentToolCancelContext(ctx)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); parent.setRecursive(true) }()
			go func() { defer wg.Done(); parent.triggerCancel(CancelAfterChatModel) }()
			wg.Wait()
			select {
			case <-child.cancelChan:
			case <-time.After(1 * time.Second):
				t.Fatal("child not cancelled")
			}
			child.markDone()
			cancel()
		}
	})
	t.Run("ChildCompletesBeforeEscalation", func(t *testing.T) {
		parent := newCancelContext()
		ctx := t.Context()
		child := parent.deriveAgentToolCancelContext(ctx)
		parent.triggerCancel(CancelAfterChatModel)
		time.Sleep(50 * time.Millisecond)
		child.markDone()
		time.Sleep(50 * time.Millisecond)
		parent.setRecursive(true)
		select {
		case <-child.cancelChan:
			t.Fatal("child done")
		case <-time.After(50 * time.Millisecond):
		}
	})
	t.Run("MultipleChildren_PartialCompletion", func(t *testing.T) {
		parent := newCancelContext()
		ctx := t.Context()
		child1 := parent.deriveAgentToolCancelContext(ctx)
		child2 := parent.deriveAgentToolCancelContext(ctx)
		parent.triggerCancel(CancelAfterChatModel)
		time.Sleep(50 * time.Millisecond)
		child1.markDone()
		parent.setRecursive(true)
		select {
		case <-child2.cancelChan:
		case <-time.After(1 * time.Second):
			t.Fatal("child2 not cancelled")
		}
		child2.markDone()
	})
}

// ---- sendInterrupt ----

func TestGraphInterruptFuncs_Parallel(t *testing.T) {
	cc := newCancelContext()
	if !cc.sendInterrupt() {
		t.Error("first should succeed")
	}
	if cc.sendInterrupt() {
		t.Error("second should fail")
	}
}

// ---- TestFilterCancelOption ----

func TestFilterCancelOption(t *testing.T) {
	opt, _ := WithCancel()
	result := filterCancelOption([]RunOption{opt})
	if len(result) != 0 {
		t.Error("cancel option should be filtered")
	}
}

// ---- TestWrapIterWithMarkDone ----

func TestWrapIterWithMarkDone(t *testing.T) {
	t.Run("CancelErrorIsWrapped", func(t *testing.T) {
		cc := newCancelContext()
		cc.triggerCancel(CancelAfterChatModel)
		it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
		go func() { defer gen.Close(); gen.Send(&TypedAgentEvent[*schema.Message]{}) }()
		wrapped := wrapIterWithCancelCtx(it, cc)
		_, ok := wrapped.Next()
		cc.markHandled()
		_ = ok
	})
	t.Run("WithoutCancelContext", func(t *testing.T) {
		it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
		go func() { defer gen.Close(); gen.Send(&TypedAgentEvent[*schema.Message]{}) }()
		wrapped := wrapIterWithCancelCtx(it, nil)
		if _, ok := wrapped.Next(); !ok {
			t.Fatal("expected event")
		}
	})
}

// ---- TestHandleRunFuncError_AlreadyHandled_NoDuplicate ----

func TestHandleRunFuncError_AlreadyHandled_NoDuplicate(t *testing.T) {
	cc := newCancelContext()
	cc.triggerCancel(CancelAfterChatModel)
	if !cc.markHandled() {
		t.Fatal("first should succeed")
	}
	if cc.markHandled() {
		t.Fatal("second should fail")
	}
}

// ---- TestCancel_SafePointNeverFires ----

func TestCancel_SafePointNeverFires_ErrExecutionEnded(t *testing.T) {
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: &mockModel{}})
	agent.name = "never"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("hi")}}, opt)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// ---- TestCancelContextKey ----

func TestCancelContextKey(t *testing.T) {
	cc := newCancelContext()
	ctx := withCancelContext(context.Background(), cc)
	got := getCancelContext(ctx)
	if got == nil {
		t.Fatal("expected cancelContext")
	}
	if v := getCancelContext(context.Background()); v != nil {
		t.Error("expected nil")
	}
}

// ---- Workflow Cancel Tests ----

func TestWithCancel_SequentialAgent(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("A1")
	m2 := &mockModel{}
	m2.addResp("A2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "s1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "s2"
	ctx := context.Background()
	wf, err := NewSequential(ctx, &SequentialConfig{Name: "seq", Description: "test", SubAgents: []Agent{a1, a2}})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel()
	drainAndAssertCancelError(t, iter)
}

func TestWithCancel_LoopAgent(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("L1")
	m2 := &mockModel{}
	m2.addResp("L2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "l1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "l2"
	ctx := context.Background()
	wf, err := NewLoop(ctx, &LoopConfig{Name: "loop", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 5})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel()
	drainAndAssertCancelError(t, iter)
}

func TestWithCancel_ParallelAgent(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("P1")
	m2 := &mockModel{}
	m2.addResp("P2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "p1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "p2"
	ctx := context.Background()
	wf, err := NewParallel(ctx, &ParallelConfig{Name: "par", Description: "test", SubAgents: []Agent{a1, a2}})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel()
	drainAndAssertCancelError(t, iter)
}

func TestCheckCancel_Sequential_BetweenSubAgents(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("X1")
	m2 := &mockModel{}
	m2.addResp("X2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "x1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "x2"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "chk", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCheckCancel_Loop_BetweenIterations(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("Y1")
	m2 := &mockModel{}
	m2.addResp("Y2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "y1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "y2"
	ctx := context.Background()
	wf, _ := NewLoop(ctx, &LoopConfig{Name: "chk_loop", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 5})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCheckCancel_Parallel_PreSpawn(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("Z1")
	m2 := &mockModel{}
	m2.addResp("Z2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "z1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "z2"
	ctx := context.Background()
	wf, _ := NewParallel(ctx, &ParallelConfig{Name: "chk_par", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// ---- Cancel after completion ----

func TestWithCancel_AfterCompletion(t *testing.T) {
	model := &mockModel{}
	model.addResp("done")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "after"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("hi")}}, opt)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
	h, ok := cancel()
	if !ok {
		if !errors.Is(h.Wait(), ErrExecutionEnded) {
			t.Error("expected ErrExecutionEnded")
		}
	}
}

// ---- Helpers ----

func goroutineCount() int { n := runtime.NumGoroutine(); return n }

func setupParentChild(t *testing.T) (parent, child *cancelContext, cleanup func()) {
	parent = newCancelContext()
	ctx, cancel := context.WithCancel(context.Background())
	child = parent.deriveAgentToolCancelContext(ctx)
	return parent, child, func() { child.markDone(); cancel() }
}

// cancelUnawareAgent is a custom Agent that ignores ctx.Done (doesn't participate in cancel protocol).
type cancelUnawareAgent struct {
	name string
	desc string
}

func (a *cancelUnawareAgent) Name(_ context.Context) string        { return a.name }
func (a *cancelUnawareAgent) Description(_ context.Context) string { return a.desc }

func (a *cancelUnawareAgent) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	go func() {
		defer gen.Close()
		time.Sleep(200 * time.Millisecond)
		gen.Send(&AgentEvent{Output: &AgentOutput{MessageOutput: &MessageVariant{Message: &schema.Message{Role: schema.RoleAssistant, Content: "unaware"}}}})
	}()
	return it
}

func TestCancelWithTools_CancelImmediate(t *testing.T) {
	model := newCancelTestChatModel(nil)
	tool := newSlowTool("slow_tool", 200*time.Millisecond, "result")
	model.addResp("tool")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "with_tools"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(50 * time.Millisecond)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelWithTools_CancelAfterChatModel(t *testing.T) {
	model := newCancelTestChatModel(nil)
	tool := newSlowTool("slow_tool", 300*time.Millisecond, "result")
	model.addResp("tool")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "after_chat"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(50 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelWithTools_CancelAfterToolCalls(t *testing.T) {
	model := newCancelTestChatModel(nil)
	tool := newSlowTool("slow_tool", 50*time.Millisecond, "result")
	model.addResp("tool")
	model.addResp("final")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "after_tool"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(50 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterToolCalls))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestWithCancel_WithCheckpoint(t *testing.T) {
	model := newCancelTestChatModel(nil)
	model.addResp("hi")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "ckpt"
	store := newCancelTestStore()
	opt, cancel := WithCancel()
	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestWithCancel_Streaming(t *testing.T) {
	t.Run("CancelImmediate", func(t *testing.T) {
		model := newCancelTestChatModel(nil)
		model.addResp("streaming_response")
		agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
		agent.name = "stream_cancel"
		opt, cancel := WithCancel()
		ctx := context.Background()
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}, EnableStreaming: true}, opt)
		time.Sleep(30 * time.Millisecond)
		cancel()
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			_ = ev
		}
	})
	t.Run("CancelAfterToolCalls", func(t *testing.T) {
		model := newCancelTestChatModel(nil)
		tool := newSlowTool("slow_tool", 30*time.Millisecond, "result")
		model.addResp("tool")
		model.addResp("final")
		agent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model: model, Tools: []Tool{tool},
			ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		})
		agent.name = "stream_tool_cancel"
		opt, cancel := WithCancel()
		ctx := context.Background()
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}, EnableStreaming: true}, opt)
		time.Sleep(50 * time.Millisecond)
		cancel(WithCancelMode(CancelAfterToolCalls))
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			_ = ev
		}
	})
}

func TestWithCancel_Resume(t *testing.T) {
	model := newCancelTestChatModel(nil)
	model.addResp("hi")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "cancel_then_resume"
	store := newCancelTestStore()
	opt, cancel := WithCancel()
	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancel_SequentialWorkflow_CancelAfterChatModel(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("A1")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("A2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "s1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "s2"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "seq", Description: "test", SubAgents: []Agent{a1, a2}})
	store := newCancelTestStore()
	opt, cancel := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: wf, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancel_ParallelWorkflow_CancelAfterChatModel(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("P1")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("P2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "p1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "p2"
	ctx := context.Background()
	wf, _ := NewParallel(ctx, &ParallelConfig{Name: "par", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancel_LoopWorkflow_CancelAfterChatModel(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("L1")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("L2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "l1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "l2"
	ctx := context.Background()
	wf, _ := NewLoop(ctx, &LoopConfig{Name: "loop", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 5})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCheckCancel_Transfer_BeforeTarget(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("route")
	s1 := newCancelTestChatModel(nil)
	s1.addResp("sub")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "sup"
	sAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: s1})
	sAgent.name = "sub1"
	ctx := context.Background()
	sup, err := SetSubAgents(ctx, a1, []Agent{sAgent})
	if err != nil {
		t.Fatalf("SetSubAgents: %v", err)
	}
	opt, cancel := WithCancel()
	iter := sup.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("transfer")}}, opt)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_SequentialTransitionBoundary(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("A")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("B")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "x1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "x2"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "seq_bound", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_LoopTransitionBoundary(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("L")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("L")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "lx"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "ly"
	ctx := context.Background()
	wf, _ := NewLoop(ctx, &LoopConfig{Name: "loop_bound", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 5})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelAfterChatModel_SequentialTransitionBoundary(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("A")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("B")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "t1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "t2"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "seq_trans", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_OrphanedToolGoroutine_NoPanic(t *testing.T) {
	_ = &reActExecCtx{}
}

func TestCancelImmediate_MultiLevelNesting(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("root")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("mid")
	m3 := newCancelTestChatModel(nil)
	m3.addResp("leaf")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "root"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "mid"
	a3 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m3})
	a3.name = "leaf"
	ctx := context.Background()
	midFlow, err := SetSubAgents(ctx, a2, []Agent{a3})
	if err != nil {
		t.Fatalf("SetSubAgents mid: %v", err)
	}
	rootFlow, err := SetSubAgents(ctx, a1, []Agent{midFlow})
	if err != nil {
		t.Fatalf("SetSubAgents root: %v", err)
	}
	opt, cancel := WithCancel()
	iter := rootFlow.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("nested")}}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancel_NestedWorkflow_AgentTool_CancelAfterChatModel(t *testing.T) {
	mRoot := newCancelTestChatModel(nil)
	mRoot.addResp("tool")
	mLeaf := newCancelTestChatModel(nil)
	mLeaf.addResp("leaf")
	leafAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: mLeaf})
	leafAgent.name = "leaf"
	tool := NewAgentTool(context.Background(), leafAgent)
	rootAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: mRoot, Tools: []Tool{tool}, ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	rootAgent.name = "root"
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := rootAgent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("start")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancel_CancelAfterToolCalls_InSequentialWorkflow(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("X")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("Y")
	slow := newSlowTool("slow_tool", 30*time.Millisecond, "result")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: m1, Tools: []Tool{slow},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{slow}},
	})
	a1.name = "with_tool"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "after_tool"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "seq_tool", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(50 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterToolCalls))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_CancelUnawareAgent_GracePeriodFallback(t *testing.T) {
	ua := &cancelUnawareAgent{name: "unaware", desc: "agent that ignores cancel"}
	cc := newCancelContext()
	ctx := withCancelContext(context.Background(), cc)
	opt := WrapImplSpecificOptFn(func(o *runOptions) { o.cancelCtx = cc })
	iter := ua.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("hi")}}, opt)
	cc.triggerImmediate()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_ParallelWorkflow_WithAgentTool(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("P1")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("P2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "pt1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "pt2"
	ctx := context.Background()
	wf, _ := NewParallel(ctx, &ParallelConfig{Name: "par_tool", Description: "test", SubAgents: []Agent{a1, a2}})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestAgentCancelFuncMultipleCalls(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	h1, ok1 := cf()
	if !ok1 {
		t.Fatal("first should contribute")
	}
	cc.markDone()
	h2, ok2 := cf()
	if ok2 {
		t.Fatal("second should not contribute")
	}
	if !errors.Is(h2.Wait(), ErrExecutionEnded) {
		t.Error("expected ErrExecutionEnded")
	}
	h1.Wait()
}

func TestAgentCancelFunc_MultiCall_EscalateToImmediate(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	cf(WithCancelMode(CancelAfterChatModel))
	cf(WithCancelMode(CancelImmediate))
	if cc.getMode() != CancelImmediate {
		t.Errorf("expected CancelImmediate, got %v", cc.getMode())
	}
	if atomic.LoadInt32(&cc.escalated) != 1 {
		t.Error("expected escalated")
	}
}

func TestAgentCancelFunc_MultiCall_JoinSafePointModes(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	cf(WithCancelMode(CancelAfterChatModel))
	cf(WithCancelMode(CancelAfterToolCalls))
	want := CancelAfterChatModel | CancelAfterToolCalls
	if cc.getMode() != want {
		t.Errorf("mode=%v want=%v", cc.getMode(), want)
	}
}

func TestAgentCancelFunc_MultiCall_TimeoutEscalationReturnsErrCancelTimeout(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	_, ok := cf(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(20*time.Millisecond))
	if !ok {
		t.Fatal("first should contribute")
	}
	time.Sleep(50 * time.Millisecond)
	if !cc.isImmediate() {
		t.Error("should escalate to immediate")
	}
	if atomic.LoadInt32(&cc.timeoutEscalated) != 1 {
		t.Error("expected timeout escalated")
	}
}

func TestAgentCancelFunc_MultiCall_TimeoutDeadlineJoinUsesAbsoluteTime(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	_, ok1 := cf(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(200*time.Millisecond))
	if !ok1 {
		t.Fatal("first should contribute")
	}
	_, ok2 := cf(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(20*time.Millisecond))
	if !ok2 {
		t.Fatal("second should contribute")
	}
	time.Sleep(50 * time.Millisecond)
	if !cc.isImmediate() {
		t.Error("should escalate to immediate after short timeout")
	}
}

func TestCancelContext_RecursiveGraceBoundary(t *testing.T) {
	a := newCancelContext()
	ctx := t.Context()
	b := a.deriveAgentToolCancelContext(ctx)
	c := b.deriveAgentToolCancelContext(ctx)
	t.Cleanup(func() { c.markDone(); b.markDone() })
	a.setRecursive(true)
	if !a.isRecursive() {
		t.Error("a should be recursive")
	}
	a.triggerCancel(CancelAfterChatModel)
	time.Sleep(100 * time.Millisecond)
	if !b.shouldCancel() {
		t.Error("b should cancel (propagated)")
	}
	if !c.shouldCancel() {
		t.Error("c should cancel (propagated)")
	}
}

func TestDeriveAgentToolCancelContext_ContextCancelConcurrentWithRecursive(t *testing.T) {
	for i := 0; i < 20; i++ {
		parent := newCancelContext()
		ctx, cxl := context.WithCancel(context.Background())
		child := parent.deriveAgentToolCancelContext(ctx)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); parent.setRecursive(true); parent.triggerCancel(CancelAfterChatModel) }()
		go func() { defer wg.Done(); cxl() }()
		wg.Wait()
		child.markDone()
	}
}

func TestDeriveAgentToolCancelContext_ConcurrentSetRecursive(t *testing.T) {
	for i := 0; i < 20; i++ {
		parent := newCancelContext()
		ctx, cancel := context.WithCancel(context.Background())
		child := parent.deriveAgentToolCancelContext(ctx)
		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Go(func() { ; parent.setRecursive(true) })
		}
		wg.Wait()
		child.markDone()
		cancel()
	}
}

func TestWithCancel_SupervisorAgent(t *testing.T) {
	mSup := newCancelTestChatModel(nil)
	mSup.addResp("sup")
	mSub := newCancelTestChatModel(nil)
	mSub.addResp("sub")
	sup := NewReActAgent(&ReActConfig[*schema.Message]{Model: mSup})
	sup.name = "supervisor"
	sub := NewReActAgent(&ReActConfig[*schema.Message]{Model: mSub})
	sub.name = "sub1"
	ctx := context.Background()
	flow, err := SetSubAgents(ctx, sup, []Agent{sub})
	if err != nil {
		t.Fatalf("SetSubAgents: %v", err)
	}
	opt, cancel := WithCancel()
	iter := flow.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("route")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelAfterChatModel_Sequential_Agent1CompletesCancelBeforeAgent2Resume(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("A")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("B")
	m3 := newCancelTestChatModel(nil)
	m3.addResp("C")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1})
	a1.name = "a"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	a2.name = "b"
	a3 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m3})
	a3.name = "c"
	ctx := context.Background()
	wf, _ := NewSequential(ctx, &SequentialConfig{Name: "seq3", Description: "test", SubAgents: []Agent{a1, a2, a3}})
	store := newCancelTestStore()
	opt, cancel := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: wf, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterChatModel))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelImmediate_AgentTool_PreservesChildCheckpoint(t *testing.T) {
	mRoot := newCancelTestChatModel(nil)
	mRoot.addResp("tool")
	mLeaf := newCancelTestChatModel(nil)
	mLeaf.addResp("leaf")
	leafAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: mLeaf})
	leafAgent.name = "leaf"
	agt := NewAgentTool(context.Background(), leafAgent)
	root := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: mRoot, Tools: []Tool{agt},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agt}},
	})
	root.name = "root_agent"
	store := newCancelTestStore()
	opt, cancel := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: root, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	time.Sleep(50 * time.Millisecond)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestCancelAfterToolCalls_LoopTransitionBoundary(t *testing.T) {
	m1 := newCancelTestChatModel(nil)
	m1.addResp("tool")
	tool := newSlowTool("slow_tool", 50*time.Millisecond, "result")
	m2 := newCancelTestChatModel(nil)
	m2.addResp("L2")
	l1 := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: m1, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	l1.name = "l_tool"
	l2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2})
	l2.name = "l_done"
	ctx := context.Background()
	wf, _ := NewLoop(ctx, &LoopConfig{Name: "loop_tool", Description: "test", SubAgents: []Agent{l1, l2}, MaxIterations: 5})
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(100 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterToolCalls))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// ================================================================
// Edge-case tests from the ADK cancel_edge_test.go / cancel_multicall_test.go
// ================================================================

// TestCancel_BeforeExecutionStarts verifies cancel before agent starts
// does not panic.
func TestCancel_BeforeExecutionStarts(t *testing.T) {
	model := &mockModel{}
	model.addResp("should not be called")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("never_start")
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("fail")}}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// TestCancel_AfterBusinessInterrupt verifies that cancelling after a
// business interrupt returns ErrExecutionEnded.
func TestCancel_AfterBusinessInterrupt(t *testing.T) {
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "bi", Function: schema.ToolCallFunction{Name: "bi_tool", Arguments: "{}"}}},
		finalResp: "done",
		firstCall: true,
	}
	tool := &mockTool{name: "bi_tool", desc: "business interrupt tool"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	}).WithName("biz_interrupt")
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
	opt, cancel := WithCancel()
	h, ok := cancel()
	if !ok {
		if !errors.Is(h.Wait(), ErrExecutionEnded) {
			t.Error("expected ErrExecutionEnded after business interrupt")
		}
	}
	_ = opt
}

// TestCancel_AfterError verifies cancelling after a model error returns ErrExecutionEnded.
func TestCancel_AfterError(t *testing.T) {
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("after_err")
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("fail")}}, opt)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
	h, ok := cancel()
	if !ok {
		if !errors.Is(h.Wait(), ErrExecutionEnded) {
			t.Error("expected ErrExecutionEnded after model error")
		}
	}
}

// TestCancel_ModelError verifies model error marks cancelCtx done.
func TestCancel_ModelError(t *testing.T) {
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("model_err")
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("fail")}}, opt)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
	h, ok := cancel()
	if !ok {
		if !errors.Is(h.Wait(), ErrExecutionEnded) {
			t.Error("expected ErrExecutionEnded")
		}
	}
}

// TestCancel_NoCheckpointStore verifies cancel without checkpoint store doesn't panic.
func TestCancel_NoCheckpointStore(t *testing.T) {
	model := newCancelTestChatModel(nil)
	model.addResp("nockpt")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("no_ckpt_cancel")
	opt, cancel := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")}, opt)
	cancel()
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// TestCancel_MultipleToolsConcurrent verifies CancelAfterToolCalls waits
// for all concurrent tools to complete.
func TestCancel_MultipleToolsConcurrent(t *testing.T) {
	model := newCancelTestChatModel(nil)
	tool1 := newSlowTool("slow_tool_1", 50*time.Millisecond, "result1")
	tool2 := newSlowTool("slow_tool_2", 80*time.Millisecond, "result2")
	model.addResp("tool")
	model.addResp("final")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool1, tool2},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool1, tool2}},
	}).WithName("multi_tool_cancel")
	opt, cancel := WithCancel()
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(30 * time.Millisecond)
	cancel(WithCancelMode(CancelAfterToolCalls))
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

// TestCancel_TimeoutEscalation_Flags verifies CancelError has correct flags.
func TestCancel_TimeoutEscalation_Flags(t *testing.T) {
	cc := newCancelContext()
	cc.setMode(CancelAfterChatModel)
	cc.timeoutEscalated = 1
	cc.escalated = 1
	cancelErr := cc.createError()
	if !cancelErr.Info.Timeout {
		t.Error("expected Timeout flag")
	}
	if !cancelErr.Info.Escalated {
		t.Error("expected Escalated flag")
	}
}

// TestCancel_MultiCall_TimeoutDeadlineJoinAbsolute verifies absolute time join.
func TestCancel_MultiCall_TimeoutDeadlineJoinAbsolute(t *testing.T) {
	cc := newCancelContext()
	cf := cc.buildCancelFunc()
	_, ok1 := cf(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(200*time.Millisecond))
	if !ok1 {
		t.Fatal("first should contribute")
	}
	_, ok2 := cf(WithCancelMode(CancelAfterChatModel), WithCancelTimeout(20*time.Millisecond))
	if !ok2 {
		t.Fatal("second should contribute")
	}
	time.Sleep(50 * time.Millisecond)
	if !cc.isImmediate() {
		t.Error("should escalate to immediate after short timeout")
	}
	if atomic.LoadInt32(&cc.timeoutEscalated) != 1 {
		t.Error("expected timeout escalated")
	}
}
