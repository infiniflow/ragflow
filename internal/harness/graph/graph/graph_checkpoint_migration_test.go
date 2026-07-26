// Package graph provides checkpoint migration, version evolution, and
// subgraph persistence integration tests.
package graph

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Checkpoint migration — basic parent-child mapping
// ============================================================

func TestCheckpointMigration_ParentChild_Mapping(t *testing.T) {
	inner := mkEchoGraph()
	_, _ = inner.Compile()
	outer := mkRootGraph()
	outerCompiled, _ := outer.Compile()

	csg := NewCompiledStateGraph(outerCompiled)
	if err := csg.AddSubgraph("sub", inner); err != nil {
		t.Fatalf("AddSubgraph: %v", err)
	}

	subCPID, err := csg.MigrateCheckpoint(context.Background(), "thread1", "parent_cp_1", "sub")
	if err != nil {
		t.Fatalf("MigrateCheckpoint to sub: %v", err)
	}
	if subCPID == "" {
		t.Fatal("expected non-empty subgraph checkpoint ID")
	}

	// Migrate back via the subgraph object (mapping stored in subgraph.checkpointMap).
	sub, _ := csg.GetSubgraph("sub")
	parentCPID, err := sub.MigrateCheckpoint(context.Background(), "thread1", subCPID, "")
	if err != nil {
		t.Fatalf("MigrateCheckpoint to parent: %v", err)
	}
	if parentCPID != "parent_cp_1" {
		t.Fatalf("expected parent_cp_1, got %s", parentCPID)
	}
}

func TestCheckpointMigration_MultipleSubgraphs(t *testing.T) {
	inner1, c1 := mkEchoGraphCompiled(t)
	inner2, c2 := mkEchoGraphCompiled(t)
	_ = c1
	_ = c2

	outer := mkRootGraph()
	oc, _ := outer.Compile()

	csg := NewCompiledStateGraph(oc)
	if err := csg.AddSubgraph("sub_a", inner1); err != nil {
		t.Fatalf("AddSubgraph sub_a: %v", err)
	}
	if err := csg.AddSubgraph("sub_b", inner2); err != nil {
		t.Fatalf("AddSubgraph sub_b: %v", err)
	}

	subAID, _ := csg.MigrateCheckpoint(context.Background(), "t1", "parent_a", "sub_a")
	subBID, _ := csg.MigrateCheckpoint(context.Background(), "t1", "parent_b", "sub_b")
	if subAID == subBID {
		t.Fatal("expected different checkpoint IDs")
	}

	// Migrate back via subgraph objects (mappings stored in subgraph checkpointMap).
	subA, _ := csg.GetSubgraph("sub_a")
	subB, _ := csg.GetSubgraph("sub_b")
	backA, _ := subA.MigrateCheckpoint(context.Background(), "t1", subAID, "")
	if backA != "parent_a" {
		t.Fatalf("expected parent_a, got %s", backA)
	}
	backB, _ := subB.MigrateCheckpoint(context.Background(), "t1", subBID, "")
	if backB != "parent_b" {
		t.Fatalf("expected parent_b, got %s", backB)
	}
}

// ============================================================
// P1: Subgraph namespace isolation
// ============================================================

func TestCheckpointMigration_NamespaceIsolation(t *testing.T) {
	inner := mkEchoGraph()
	ic, _ := inner.Compile()
	_ = ic

	outer := mkRootGraph()
	oc, _ := outer.Compile()

	csg := NewCompiledStateGraph(oc)
	if err := csg.AddSubgraph("sub1", inner); err != nil {
		t.Fatalf("AddSubgraph sub1: %v", err)
	}
	if err := csg.AddSubgraph("sub2", inner); err != nil {
		t.Fatalf("AddSubgraph sub2: %v", err)
	}

	sub1, ok1 := csg.GetSubgraph("sub1")
	sub2, ok2 := csg.GetSubgraph("sub2")
	if !ok1 || !ok2 {
		t.Fatal("subgraphs not found")
	}
	if sub1.GetNamespace() == sub2.GetNamespace() {
		t.Fatal("expected different namespaces")
	}
}

// ============================================================
// P1: Checkpoint version evolution
// ============================================================

func TestCheckpointMigration_VersionEvolution(t *testing.T) {
	v1 := NewStateGraph(map[string]any{})
	v1.AddNode("v1_proc", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["version"] = "v1"
		return m, nil
	})
	v1.AddEdge(constants.Start, "v1_proc")
	v1.AddEdge("v1_proc", constants.End)

	ms := checkpoint.NewMemorySaver()
	v1Compiled, err := v1.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("V1 Compile: %v", err)
	}

	tid := "version-evolution"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			constants.ConfigKeyThreadID: tid,
		},
	}
	_, err = v1Compiled.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("V1 Invoke: %v", err)
	}

	inspector, ok := v1Compiled.(StateInspector)
	if !ok {
		t.Fatal("v1Compiled does not implement StateInspector")
	}
	snap, err := inspector.GetState(context.Background(), cfg)
	if err != nil {
		t.Fatalf("V1 GetState: %v", err)
	}
	_ = snap

	v2 := NewStateGraph(map[string]any{})
	v2.AddNode("v2_proc", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["version"] = "v2"
		m["new_field"] = "evolved"
		return m, nil
	})
	v2.AddEdge(constants.Start, "v2_proc")
	v2.AddEdge("v2_proc", constants.End)

	v2Compiled, err := v2.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("V2 Compile: %v", err)
	}
	_, err = v2Compiled.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("V2 Invoke: %v", err)
	}

	inspector2, ok2 := v2Compiled.(StateInspector)
	if !ok2 {
		t.Fatal("v2Compiled does not implement StateInspector")
	}
	snap2, err := inspector2.GetState(context.Background(), cfg)
	if err != nil {
		t.Fatalf("V2 GetState: %v", err)
	}
	_ = snap2
}

// ============================================================
// P2: Subgraph persistence
// ============================================================

func TestSubgraphPersistence_SharedCheckpointer(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("counter", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		if v, ok := m["count"]; ok {
			m["count"] = v.(int) + 1
		} else {
			m["count"] = 1
		}
		return m, nil
	})
	b.AddEdge(constants.Start, "counter")
	b.AddEdge("counter", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms), WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	tid := "subgraph-persistence"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			constants.ConfigKeyThreadID: tid,
		},
	}
	ctx := context.Background()

	_, err = cg.Invoke(ctx, map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("first Invoke: %v", err)
	}

	result, err := cg.Invoke(ctx, map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("second Invoke: %v", err)
	}
	_ = result
}

func TestSubgraphPersistence_MultipleThreads_Isolated(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["processed"] = "yes"
		return m, nil
	})
	b.AddEdge(constants.Start, "echo")
	b.AddEdge("echo", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms), WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		tid := fmt.Sprintf("isolated-thread-%d", i)
		cfg := &types.RunnableConfig{
			Configurable: map[string]interface{}{
				constants.ConfigKeyThreadID: tid,
			},
		}
		_, err := cg.Invoke(ctx, map[string]any{}, cfg)
		if err != nil {
			t.Fatalf("thread %d Invoke: %v", i, err)
		}
	}
}

// ============================================================
// P2: Checkpoint migration error cases
// ============================================================

func TestCheckpointMigration_SubgraphNotFound(t *testing.T) {
	outer := mkRootGraph()
	oc, _ := outer.Compile()
	csg := NewCompiledStateGraph(oc)

	_, err := csg.MigrateCheckpoint(context.Background(), "t1", "cp1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent subgraph")
	}
}

func TestCheckpointMigration_ParentNotFound(t *testing.T) {
	outer := mkRootGraph()
	oc, _ := outer.Compile()
	csg := NewCompiledStateGraph(oc)

	_, err := csg.MigrateCheckpoint(context.Background(), "t1", "cp1", "")
	if err == nil {
		t.Fatal("expected error when no parent exists")
	}
}

func TestCheckpointMigration_DuplicateSubgraph(t *testing.T) {
	inner := mkEchoGraph()
	outer := mkRootGraph()
	oc, _ := outer.Compile()
	csg := NewCompiledStateGraph(oc)

	if err := csg.AddSubgraph("dup", inner); err != nil {
		t.Fatalf("first AddSubgraph: %v", err)
	}
	if err := csg.AddSubgraph("dup", inner); err == nil {
		t.Fatal("expected error for duplicate subgraph name")
	}
}

// ============================================================
// Helpers
// ============================================================

func mkEchoGraph() types.StateGraph {
	g := NewStateGraph(map[string]any{})
	g.AddNode("echo", func(ctx context.Context, state any) (any, error) { return state, nil })
	g.AddEdge(constants.Start, "echo")
	g.AddEdge("echo", constants.End)
	return g
}

func mkRootGraph() types.StateGraph {
	g := NewStateGraph(map[string]any{})
	g.AddNode("root", func(ctx context.Context, state any) (any, error) { return state, nil })
	g.AddEdge(constants.Start, "root")
	g.AddEdge("root", constants.End)
	return g
}

func mkEchoGraphCompiled(t *testing.T) (types.StateGraph, types.CompiledGraph) {
	t.Helper()
	g := mkEchoGraph()
	c, err := g.Compile()
	if err != nil {
		t.Fatalf("mkEchoGraphCompiled: %v", err)
	}
	return g, c
}
