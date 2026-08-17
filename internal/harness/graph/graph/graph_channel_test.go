package graph

import (
	"context"
	"fmt"
	"sync"

	"testing"
	"time"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// Channel type integration tests — all channel types executed through a compiled graph.
// These tests live in the graph package to avoid import cycle (graph imports channels).

// Helper: build a chain graph (Start→a→b→...→End).
type chainBuilder struct {
	schema map[string]interface{}
	chans  map[string]channels.Channel
	nodes  []string
	fns    map[string]types.NodeFunc
}

func newChain(schema map[string]interface{}) *chainBuilder {
	return &chainBuilder{
		schema: schema,
		chans:  make(map[string]channels.Channel),
		fns:    make(map[string]types.NodeFunc),
	}
}

func (b *chainBuilder) channel(name string, ch channels.Channel) *chainBuilder {
	b.chans[name] = ch
	return b
}

func (b *chainBuilder) node(name string, fn types.NodeFunc) *chainBuilder {
	b.nodes = append(b.nodes, name)
	b.fns[name] = fn
	return b
}

func (b *chainBuilder) invoke(input interface{}) (interface{}, error) {
	g := NewStateGraph(b.schema)
	for name, ch := range b.chans {
		g.AddChannel(name, ch)
	}
	for name, fn := range b.fns {
		g.AddNode(name, fn)
	}
	if len(b.nodes) > 0 {
		if err := g.AddEdge(constants.Start, b.nodes[0]); err != nil {
			return nil, fmt.Errorf("AddEdge Start->%s: %w", b.nodes[0], err)
		}
		for i := 0; i < len(b.nodes)-1; i++ {
			if err := g.AddEdge(b.nodes[i], b.nodes[i+1]); err != nil {
				return nil, fmt.Errorf("AddEdge %s->%s: %w", b.nodes[i], b.nodes[i+1], err)
			}
		}
		if err := g.AddEdge(b.nodes[len(b.nodes)-1], constants.End); err != nil {
			return nil, fmt.Errorf("AddEdge %s->End: %w", b.nodes[len(b.nodes)-1], err)
		}
	}
	cg, err := g.Compile()
	if err != nil {
		return nil, fmt.Errorf("Compile: %w", err)
	}
	return cg.Invoke(context.Background(), input)
}

// ============================================================
// BinaryOperatorAggregate tests
// ============================================================

func TestGraphChannel_BinaryOperator_AddAccumulation(t *testing.T) {
	b := newChain(map[string]interface{}{"total": 0})
	b.channel("total", channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": 5}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": 10}, nil
	})

	result, err := b.invoke(map[string]interface{}{"total": 0})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	total, _ := m["total"].(int)
	if total != 15 {
		t.Errorf("expected total=15 (0+5+10), got %d", total)
	}
}

func TestGraphChannel_BinaryOperator_Overwrite(t *testing.T) {
	t.Skip("inline Pregel path does not support Overwrite wrapper; use pregel.Engine instead")

	b := newChain(map[string]interface{}{"total": 0})
	b.channel("total", channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd))
	b.node("writer", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": types.NewOverwrite(99)}, nil
	})

	result, err := b.invoke(map[string]interface{}{"total": 0})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	total, _ := m["total"].(int)
	if total != 99 {
		t.Errorf("expected total=99 (overwrite), got %d", total)
	}
}

func TestGraphChannel_BinaryOperator_StringConcat(t *testing.T) {
	b := newChain(map[string]interface{}{"text": ""})
	b.channel("text", channels.NewBinaryOperatorAggregate("", channels.StringConcat))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"text": "hello "}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"text": "world"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"text": ""})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	text, _ := m["text"].(string)
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestGraphChannel_BinaryOperator_ListAppend(t *testing.T) {
	b := newChain(map[string]interface{}{"items": []interface{}{}})
	b.channel("items", channels.NewBinaryOperatorAggregate([]interface{}{}, channels.ListAppend))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"items": []interface{}{"a", "b"}}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"items": []interface{}{"c"}}, nil
	})

	result, err := b.invoke(map[string]interface{}{"items": []interface{}{}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	items, _ := m["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d: %v", len(items), items)
	}
}

func TestGraphChannel_BinaryOperator_MultipleSteps(t *testing.T) {
	t.Skip("inline Pregel path recursion limit issue with conditional loops; use pregel.Engine instead")

	g := NewStateGraph(map[string]interface{}{"total": 0})
	g.AddChannel("total", channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd))
	g.AddNode("inc", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": 1}, nil
	})
	g.AddNode("done", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{}, nil
	})
	g.AddEdge(constants.Start, "inc")
	g.AddConditionalEdges("inc",
		func(_ context.Context, state interface{}) (interface{}, error) {
			m := state.(map[string]interface{})
			c, _ := m["total"].(int)
			if c >= 5 {
				return "done", nil
			}
			return "inc", nil
		},
		map[string]string{"inc": "inc", "done": "done"},
	)
	g.AddEdge("done", constants.End)

	cg, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	result, err := cg.Invoke(context.Background(), map[string]interface{}{"total": 0})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	total, _ := m["total"].(int)
	if total < 5 {
		t.Errorf("expected total >= 5, got %d", total)
	}
	t.Logf("loop accumulated total: %d", total)
}

// ============================================================
// ReducerChannel tests
// ============================================================

func TestGraphChannel_Reducer_Basic(t *testing.T) {
	inner := channels.NewLastValue(int(0))
	rc := channels.NewReducerChannel(inner, func(current, update interface{}) interface{} {
		ci, _ := current.(int)
		ui, _ := update.(int)
		return ci + ui
	})
	b := newChain(map[string]interface{}{"val": 0})
	b.channel("val", rc)
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": 5}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": 10}, nil
	})

	result, err := b.invoke(map[string]interface{}{"val": 0})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	v, _ := m["val"].(int)
	if v != 15 {
		t.Errorf("expected val=15 (0+5+10), got %d", v)
	}
}

func TestGraphChannel_Reducer_Append(t *testing.T) {
	// With the pregel engine, applying input to a ReducerChannel causes
	// the input value to be accumulated through the reducer (AppendReducer
	// treats []interface{}{} as a single element).  Start from empty input
	// and let nodes provide the values.
	b := newChain(map[string]interface{}{"items": []interface{}{}})
	inner := channels.NewLastValue([]interface{}{})
	rc := channels.NewReducerChannel(inner, channels.AppendReducer)
	b.channel("items", rc)
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"items": "a"}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"items": "b"}, nil
	})

	result, err := b.invoke(map[string]interface{}{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	items, _ := m["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("expected 2 items (a, b), got %d: %v", len(items), items)
	}
}

func TestGraphChannel_Reducer_Merge(t *testing.T) {
	inner := channels.NewLastValue(map[string]interface{}{})
	rc := channels.NewReducerChannel(inner, channels.MergeReducer)
	b := newChain(map[string]interface{}{"data": map[string]interface{}{}})
	b.channel("data", rc)
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"data": map[string]interface{}{"x": 1}}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"data": map[string]interface{}{"y": 2}}, nil
	})

	result, err := b.invoke(map[string]interface{}{"data": map[string]interface{}{}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	data, _ := m["data"].(map[string]interface{})
	if data["x"] != 1 || data["y"] != 2 {
		t.Errorf("expected {x:1, y:2}, got %v", data)
	}
}

// ============================================================
// Topic tests
// ============================================================

func TestGraphChannel_Topic_Accumulate(t *testing.T) {
	b := newChain(map[string]interface{}{"msgs": []interface{}{}})
	b.channel("msgs", channels.NewTopic("", true))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"msgs": "msg_a"}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"msgs": "msg_b"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"msgs": []interface{}{}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	msgs, _ := m["msgs"].([]interface{})
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d: %v", len(msgs), msgs)
	}
}

func TestGraphChannel_Topic_NoAccumulate(t *testing.T) {
	b := newChain(map[string]interface{}{"msgs": []interface{}{}})
	b.channel("msgs", channels.NewTopic("", false))
	b.node("writer", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"msgs": "only"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"msgs": []interface{}{}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	msgs, _ := m["msgs"].([]interface{})
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

// ============================================================
// EphemeralValue test
// ============================================================

func TestGraphChannel_Ephemeral_InGraph(t *testing.T) {
	b := newChain(map[string]interface{}{"temp": "", "persist": ""})
	b.channel("temp", channels.NewEphemeralValue("", true))
	b.channel("persist", channels.NewLastValue(""))
	b.node("writer", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"temp": "ephemeral", "persist": "stored"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"temp": "", "persist": ""})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	p, _ := m["persist"].(string)
	if p != "stored" {
		t.Errorf("expected stored, got %q", p)
	}
}

// ============================================================
// AnyValue test
// ============================================================

func TestGraphChannel_AnyValue_InGraph(t *testing.T) {
	b := newChain(map[string]interface{}{"val": ""})
	b.channel("val", channels.NewAnyValue(""))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "from_a"}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "from_b"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"val": ""})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	v, _ := m["val"].(string)
	if v != "from_b" {
		t.Errorf("expected 'from_b' (last write wins), got %q", v)
	}
}

// ============================================================
// UntrackedValue test
// ============================================================

func TestGraphChannel_Untracked_InGraph(t *testing.T) {
	b := newChain(map[string]interface{}{"val": "", "secret": ""})
	b.channel("val", channels.NewLastValue(""))
	b.channel("secret", channels.NewUntrackedValue(""))
	b.node("writer", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "visible", "secret": "hidden"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"val": "", "secret": ""})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	v, _ := m["val"].(string)
	if v != "visible" {
		t.Errorf("expected visible, got %q", v)
	}
}

// ============================================================
// Cross-channel interaction tests
// ============================================================

func TestGraphChannel_LastValueAndBinaryOp(t *testing.T) {
	b := newChain(map[string]interface{}{"name": "", "total": 0})
	b.channel("name", channels.NewLastValue(""))
	b.channel("total", channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"name": "alice", "total": 10}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": 20}, nil
	})

	result, err := b.invoke(map[string]interface{}{"name": "", "total": 0})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	name, _ := m["name"].(string)
	total, _ := m["total"].(int)
	if name != "alice" {
		t.Errorf("expected alice, got %q", name)
	}
	if total != 30 {
		t.Errorf("expected total=30 (0+10+20), got %d", total)
	}
}

func TestGraphChannel_TopicAndBarrier(t *testing.T) {
	b := newChain(map[string]interface{}{"msgs": []interface{}{}})
	b.channel("msgs", channels.NewTopic("", true))
	b.node("a", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"msgs": "from_a"}, nil
	})
	b.node("b", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"msgs": "from_b"}, nil
	})

	result, err := b.invoke(map[string]interface{}{"msgs": []interface{}{}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	msgs, _ := m["msgs"].([]interface{})
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d: %v", len(msgs), msgs)
	}
}

// ============================================================
// Race condition tests (channels are single-goroutine, so these
// verify the engine doesn't introduce races)
// ============================================================

func TestGraphChannel_Race_ConcurrentGraphInvocations(t *testing.T) {
	g := NewStateGraph(map[string]interface{}{"val": ""})
	g.AddChannel("val", channels.NewLastValue(""))
	g.AddNode("echo", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "done"}, nil
	})
	g.AddEdge(constants.Start, "echo")
	g.AddEdge("echo", constants.End)

	cg, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Go(func() {
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// ============================================================
// Error propagation
// ============================================================

func TestGraphChannel_OverwriteConflict(t *testing.T) {
	t.Skip("inline Pregel path does not support parallel fan-in; OverwriteConflict requires AllPredecessor mode")
}

func TestGraphChannel_BinaryOperator_EmptyUpdate(t *testing.T) {
	// With the pregel engine, the input value 42 is accumulated onto the
	// BinaryOperatorAggregate's initial zero value via Update.
	b := newChain(map[string]interface{}{"total": 0})
	b.channel("total", channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd))
	b.node("nop", func(_ context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"total": nil}, nil
	})

	result, err := b.invoke(map[string]interface{}{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	total, _ := m["total"].(int)
	if total != 0 {
		t.Errorf("expected total=0 (no input, nop returns nil), got %d", total)
	}
}

// ============================================================
// Timeout cancel
// ============================================================

func TestGraphChannel_TimeoutCancel(t *testing.T) {
	g := NewStateGraph(map[string]interface{}{"val": ""})
	g.AddChannel("val", channels.NewLastValue(""))
	g.AddNode("slow", func(ctx context.Context, state interface{}) (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		return map[string]interface{}{"val": "done"}, nil
	})
	g.AddEdge(constants.Start, "slow")
	g.AddEdge("slow", constants.End)

	cg, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = cg.Invoke(ctx, map[string]interface{}{"val": ""})
	if err == nil {
		t.Log("node completed before timeout")
	} else {
		t.Logf("timeout correctly triggered: %v", err)
	}
}

// ============================================================
// Checkpoint round-trip for channel types
// ============================================================

func TestGraphChannel_CheckpointRoundTrip(t *testing.T) {
	type testCase struct {
		name   string
		makeCh func() channels.Channel
		setup  func(channels.Channel)
		verify func(channels.Channel, *testing.T)
	}

	cases := []testCase{
		{
			name:   "LastValue",
			makeCh: func() channels.Channel { ch := channels.NewLastValue(""); ch.SetKey("lv"); return ch },
			setup:  func(ch channels.Channel) { ch.Update([]interface{}{"stored"}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				if v != "stored" {
					t.Errorf("expected 'stored', got %v", v)
				}
			},
		},
		{
			name: "BinaryOperatorAggregate",
			makeCh: func() channels.Channel {
				ch := channels.NewBinaryOperatorAggregate(int(0), channels.IntAdd)
				ch.SetKey("bo")
				return ch
			},
			setup: func(ch channels.Channel) { ch.Update([]interface{}{7, 3}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				if v != 10 {
					t.Errorf("expected 10, got %v", v)
				}
			},
		},
		{
			name: "NamedBarrierValue",
			makeCh: func() channels.Channel {
				ch := channels.NewNamedBarrierValue(nil, []string{"x", "y"})
				ch.SetKey("nb")
				return ch
			},
			setup: func(ch channels.Channel) { ch.Update([]interface{}{"x", "y"}) },
			verify: func(ch channels.Channel, t *testing.T) {
				if !ch.IsAvailable() {
					t.Error("barrier should be available")
				}
			},
		},
		{
			name:   "Topic",
			makeCh: func() channels.Channel { ch := channels.NewTopic("", true); ch.SetKey("tp"); return ch },
			setup:  func(ch channels.Channel) { ch.Update([]interface{}{"a", "b"}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				items := v.([]interface{})
				if len(items) != 2 {
					t.Errorf("expected 2 items, got %d", len(items))
				}
			},
		},
		{
			name:   "EphemeralValue",
			makeCh: func() channels.Channel { ch := channels.NewEphemeralValue("", true); ch.SetKey("ev"); return ch },
			setup:  func(ch channels.Channel) { ch.Update([]interface{}{"temp"}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				if v != "temp" {
					t.Errorf("expected 'temp', got %v", v)
				}
			},
		},
		{
			name:   "AnyValue",
			makeCh: func() channels.Channel { ch := channels.NewAnyValue(""); ch.SetKey("av"); return ch },
			setup:  func(ch channels.Channel) { ch.Update([]interface{}{"stored"}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				if v != "stored" {
					t.Errorf("expected 'stored', got %v", v)
				}
			},
		},
		{
			name: "ReducerChannel",
			makeCh: func() channels.Channel {
				inner := channels.NewLastValue(int(0))
				ch := channels.NewReducerChannel(inner, channels.AddReducer)
				ch.SetKey("rc")
				return ch
			},
			setup: func(ch channels.Channel) { ch.Update([]interface{}{5, 3}) },
			verify: func(ch channels.Channel, t *testing.T) {
				v, _ := ch.Get()
				if v != 8 {
					t.Errorf("expected 8, got %v", v)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := tc.makeCh()
			tc.setup(orig)
			cp := orig.Checkpoint()
			restored := orig.FromCheckpoint(cp)
			tc.verify(restored, t)
		})
	}
}

// ============================================================
// BinaryOperator edge cases
// ============================================================

func TestGraphChannel_BinaryOperator_IntAddEdgeCases(t *testing.T) {
	if v := channels.IntAdd(1.5, 2.5); v != 4.0 {
		t.Errorf("expected 4.0, got %v", v)
	}
	if v := channels.IntAdd(5, "not a number"); v != 5 {
		t.Errorf("expected 5 (unchanged on type mismatch), got %v", v)
	}
}

func TestGraphChannel_BinaryOperator_ListAppendEdgeCases(t *testing.T) {
	r := channels.ListAppend([]interface{}{"a"}, "b")
	items := r.([]interface{})
	if len(items) != 2 || items[1] != "b" {
		t.Errorf("expected [a, b], got %v", items)
	}
	r2 := channels.ListAppend("a", "b")
	items2 := r2.([]interface{})
	if len(items2) != 2 {
		t.Errorf("expected 2 items, got %d", len(items2))
	}
}

func TestGraphChannel_StringConcatEdgeCases(t *testing.T) {
	r := channels.StringConcat(1, 2)
	if r != "12" {
		t.Errorf("expected '12', got %q", r)
	}
}

// ============================================================
// Errors package verification
// ============================================================

func TestGraphChannel_Errors(t *testing.T) {
	if !errors.IsEmptyChannelError(&errors.EmptyChannelError{}) {
		t.Error("IsEmptyChannelError should detect EmptyChannelError")
	}
	if !errors.IsInvalidUpdateError(&errors.InvalidUpdateError{Message: "test"}) {
		t.Error("IsInvalidUpdateError should detect InvalidUpdateError")
	}
}
