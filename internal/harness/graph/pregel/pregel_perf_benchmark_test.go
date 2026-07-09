// Package pregel provides performance benchmarks for the Pregel engine.
//
// Benchmarks cover: throughput (ops/sec), latency distribution (P50/P99),
// memory allocation, large state handling, and scalability with
// increasing node counts.
package pregel

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	graphPkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Throughput benchmarks
// ============================================================

// benchmarkSimpleGraph creates a simple 3-node graph for benchmarking.
// mirrors newSimpleGraph in engine_test.go
func benchmarkSimpleGraph() types.StateGraph {
	sg := graphPkg.NewStateGraph(map[string]any{"value": ""})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("node_a", func(ctx context.Context, state any) (any, error) {
		// Return a fresh map copy to avoid sharing mutable state across runs.
		return map[string]any{"value": "a"}, nil
	})
	sg.AddNode("node_b", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"value": "b"}, nil
	})
	_ = sg.AddEdge(constants.Start, "node_a")
	_ = sg.AddEdge("node_a", "node_b")
	_ = sg.AddEdge("node_b", constants.End)
	return sg
}

// BenchmarkEngine_SimpleChain measures throughput for a 3-node chain.
func BenchmarkEngine_SimpleChain(b *testing.B) {
	g := benchmarkSimpleGraph()
	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()
	input := map[string]any{"value": "bench"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, input)
		if err != nil {
			b.Fatalf("RunSync: %v", err)
		}
	}
}

// BenchmarkEngine_LongChain measures throughput for a 100-node chain.
func BenchmarkEngine_LongChain(b *testing.B) {
	type State struct {
		Count int
	}
	sg := graphPkg.NewStateGraph(State{})
	sg.AddChannel("count", channels.NewLastValue(0))

	prev := constants.Start
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("node_%d", i)
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			s := state.(State)
			s.Count++
			return s, nil
		})
		sg.AddEdge(prev, name)
		prev = name
	}
	sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg, WithRecursionLimit(200))
	ctx := context.Background()
	input := State{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, input)
		if err != nil {
			b.Fatalf("RunSync: %v", err)
		}
	}
}

// ============================================================
// P0: Latency benchmarks
// ============================================================

// BenchmarkEngine_WithCheckpointer measures latency when checkpoints are
// persisted to MemorySaver.
func BenchmarkEngine_WithCheckpointer(b *testing.B) {
	g := benchmarkSimpleGraph()
	ms := checkpoint.NewMemorySaver()
	engine := NewEngine(g, WithRecursionLimit(10), WithCheckpointer(ms))
	ctx := context.Background()
	input := map[string]any{"value": "bench-cp"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, input)
		if err != nil {
			b.Fatalf("RunSync with CP: %v", err)
		}
	}
}

// ============================================================
// P1: Increasing node count scaling
// ============================================================

// BenchmarkEngine_Scaling_Nodes measures how throughput scales with
// increasing node counts (10, 50, 100 nodes).
func BenchmarkEngine_Scaling_Nodes(b *testing.B) {
	for _, n := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("%d_nodes", n), func(b *testing.B) {
			type State struct{ Count int }
			sg := graphPkg.NewStateGraph(State{})
			sg.AddChannel("count", channels.NewLastValue(0))

			prev := constants.Start
			for i := 0; i < n; i++ {
				name := fmt.Sprintf("n_%d", i)
				sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
					s := state.(State)
					s.Count++
					return s, nil
				})
				sg.AddEdge(prev, name)
				prev = name
			}
			sg.AddEdge(prev, constants.End)

			engine := NewEngine(sg, WithRecursionLimit(n*2))
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := engine.RunSync(ctx, State{})
				if err != nil {
					b.Fatalf("RunSync: %v", err)
				}
			}
		})
	}
}

// ============================================================
// P1: Allocation benchmarks
// ============================================================

// BenchmarkEngine_Allocation measures per-call memory allocation overhead.
func BenchmarkEngine_Allocation(b *testing.B) {
	g := benchmarkSimpleGraph()
	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()
	input := map[string]any{"value": "bench-alloc"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, input)
		if err != nil {
			b.Fatalf("RunSync: %v", err)
		}
	}
}

// ============================================================
// P2: Large state benchmarks
// ============================================================

// BenchmarkEngine_LargeState measures performance when the state contains
// a large map (10K entries).
func BenchmarkEngine_LargeState(b *testing.B) {
	type State struct{ Data map[string]string }

	sg := graphPkg.NewStateGraph(State{})
	sg.AddNode("load", func(ctx context.Context, state any) (any, error) {
		s := state.(State)
		if s.Data == nil {
			s.Data = make(map[string]string)
		}
		for i := 0; i < 10000; i++ {
			s.Data[fmt.Sprintf("k_%d", i)] = fmt.Sprintf("v_%d", i)
		}
		return s, nil
	})
	sg.AddEdge(constants.Start, "load")
	sg.AddEdge("load", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, State{})
		if err != nil {
			b.Fatalf("RunSync: %v", err)
		}
	}
}

// ============================================================
// P2: Concurrency scaling benchmarks
// ============================================================

// BenchmarkEngine_ConcurrentCalls measures throughput under concurrent load.
func BenchmarkEngine_ConcurrentCalls(b *testing.B) {
	g := benchmarkSimpleGraph()
	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()
	input := map[string]any{"value": "bench-conc"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := engine.RunSync(ctx, input)
			if err != nil {
				b.Errorf("RunSync: %v", err)
			}
		}
	})
}

// ============================================================
// P2: Checkpoint with large state benchmarks
// ============================================================

// BenchmarkEngine_Checkpoint_LargeState measures the cost of checkpointing
// a large state (10K entries map).
func BenchmarkEngine_Checkpoint_LargeState(b *testing.B) {
	type State struct{ Data map[string]string }

	sg := graphPkg.NewStateGraph(State{})
	sg.AddNode("load", func(ctx context.Context, state any) (any, error) {
		s := state.(State)
		if s.Data == nil {
			s.Data = make(map[string]string)
		}
		for i := 0; i < 10000; i++ {
			s.Data[fmt.Sprintf("k_%d", i)] = fmt.Sprintf("v_%d", i)
		}
		return s, nil
	})
	sg.AddEdge(constants.Start, "load")
	sg.AddEdge("load", constants.End)

	ms := checkpoint.NewMemorySaver()
	engine := NewEngine(sg, WithRecursionLimit(10), WithCheckpointer(ms))
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := engine.RunSync(ctx, State{})
		if err != nil {
			b.Fatalf("RunSync: %v", err)
		}
	}
}
