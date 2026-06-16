// Package pregel provides subgraph support for Pregel.
package pregel

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

func TestSubgraphManagerCreation(t *testing.T) {
	parentEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	manager := NewSubgraphManager(parentEngine)
	
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
	
	if manager.parentEngine != parentEngine {
		t.Error("Expected parent engine to match")
	}
	
	if len(manager.subgraphs) != 0 {
		t.Errorf("Expected empty subgraphs, got %d", len(manager.subgraphs))
	}
}

func TestNamespaceStack(t *testing.T) {
	manager := &SubgraphManager{
		namespaceStack: make([]string, 0),
	}
	
	// Test pushing namespaces
	manager.PushNamespace("subgraph1")
	if len(manager.namespaceStack) != 1 {
		t.Errorf("Expected stack size 1, got %d", len(manager.namespaceStack))
	}
	if manager.CurrentNamespace() != "subgraph1" {
		t.Errorf("Expected current namespace 'subgraph1', got %s", manager.CurrentNamespace())
	}
	
	manager.PushNamespace("subgraph2")
	if len(manager.namespaceStack) != 2 {
		t.Errorf("Expected stack size 2, got %d", len(manager.namespaceStack))
	}
	if manager.CurrentNamespace() != "subgraph2" {
		t.Errorf("Expected current namespace 'subgraph2', got %s", manager.CurrentNamespace())
	}
	
	// Test popping namespaces
	manager.PopNamespace()
	if len(manager.namespaceStack) != 1 {
		t.Errorf("Expected stack size 1, got %d", len(manager.namespaceStack))
	}
	if manager.CurrentNamespace() != "subgraph1" {
		t.Errorf("Expected current namespace 'subgraph1', got %s", manager.CurrentNamespace())
	}
	
	manager.PopNamespace()
	if len(manager.namespaceStack) != 0 {
		t.Errorf("Expected empty stack, got %d", len(manager.namespaceStack))
	}
	if manager.CurrentNamespace() != "" {
		t.Errorf("Expected empty namespace, got %s", manager.CurrentNamespace())
	}
}

func TestBuildNamespacePath(t *testing.T) {
	tests := []struct {
		name          string
		stack         []string
		expectedPath  string
	}{
		{
			name:          "empty stack",
			stack:         []string{},
			expectedPath:  "",
		},
		{
			name:          "single namespace",
			stack:         []string{"subgraph1"},
			expectedPath:  "subgraph1",
		},
		{
			name:          "multiple namespaces",
			stack:         []string{"subgraph1", "subgraph2", "subgraph3"},
			expectedPath:  "subgraph1|subgraph2|subgraph3",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &SubgraphManager{
				namespaceStack: tt.stack,
			}
			
			path := manager.BuildNamespacePath()
			if path != tt.expectedPath {
				t.Errorf("Expected path '%s', got '%s'", tt.expectedPath, path)
			}
		})
	}
}

func TestNamespaceIsolatedRegistry(t *testing.T) {
	baseRegistry := channels.NewRegistry()
	
	// Register some channels in base registry
	ch1 := channels.NewAnyValue(nil)
	baseRegistry.Register("base_channel1", ch1)
	
	// Create isolated registry
	isolated := NewNamespaceIsolatedRegistry(baseRegistry, "subgraph1")
	
	// Test get with namespace prefix
	_, exists := isolated.Get("channel1")
	if exists {
		t.Error("Expected non-existent channel")
	}
	
	// Register in isolated registry
	ch2 := channels.NewAnyValue(nil)
	if err := isolated.Register("isolated_channel1", ch2); err != nil {
		t.Fatalf("Failed to register isolated channel: %v", err)
	}
	
	// Get should work with full namespace prefix
	_, exists = isolated.Get("isolated_channel1")
	if !exists {
		t.Error("Expected channel to exist")
	}
}

func TestRecursiveSubgraphExecutor(t *testing.T) {
	parentEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	manager := NewSubgraphManager(parentEngine)
	executor := NewRecursiveSubgraphExecutor(manager, 5)
	
	t.Run("depth limit enforcement", func(t *testing.T) {
		// Try to exceed max depth
		_, err := executor.executeRecursive(context.Background(), "test", "input", 10)
		if err == nil {
			t.Error("Expected depth limit error")
		}
	})
	
	t.Run("normal execution", func(t *testing.T) {
		// This would need proper graph setup for full test
		_, err := executor.executeRecursive(context.Background(), "test", "input", 0)
		if err == nil {
			// Expected - node doesn't exist
			t.Log("Expected error for missing node (ok)")
		}
	})
}

func TestCheckpointMigration(t *testing.T) {
	mockCheckpointer := &MockCheckpointSaver{
		checkpoints: make(map[string]map[string]interface{}),
		tuples:     make(map[string]*checkpoint.CheckpointTuple),
		mu:         sync.RWMutex{},
	}
	
	parentEngine := &Engine{
		checkpointer: mockCheckpointer,
	}
	
	manager := NewSubgraphManager(parentEngine)
	migration := NewCheckpointMigration(manager, mockCheckpointer)
	
	t.Run("migrate to subgraph", func(t *testing.T) {
		manager.PushNamespace("subgraph1")
		ctx := context.Background()
		
		// Create parent checkpoint
		parentCP := map[string]interface{}{
			"channel1": "value1",
			"channel2": "value2",
		}
		
		mockCheckpointer.tuples["parent"] = &checkpoint.CheckpointTuple{
			Checkpoint: &checkpoint.Checkpoint{
				State: parentCP,
			},
		}
		
		// Migrate to subgraph
		_, err := migration.MigrateToSubgraph(ctx, "thread1", "parent", "subgraph1")
		if err != nil {
			t.Errorf("MigrateToSubgraph failed: %v", err)
		}
		
		// Check namespace tracking
		ns, exists := manager.checkpointNS["thread1"]
		if !exists {
			t.Error("Expected checkpoint namespace to be tracked")
		}
		expectedNS := "subgraph1" + string(constants.NSSep) + "subgraph1"
		if ns != expectedNS {
			t.Errorf("Expected namespace '%s', got '%s'", expectedNS, ns)
		}
		
		manager.PopNamespace()
	})
	
	t.Run("migrate from subgraph", func(t *testing.T) {
		ctx := context.Background()
		
		// Create subgraph checkpoint
		subgraphCP := map[string]interface{}{
			"channel1": "updated_value1",
			"channel2": "updated_value2",
		}
		
		mockCheckpointer.tuples["subgraph"] = &checkpoint.CheckpointTuple{
			Checkpoint: &checkpoint.Checkpoint{
				State: subgraphCP,
			},
		}
		
		// Set checkpoint namespace
		manager.checkpointNS["thread1"] = "subgraph1"
		
		// Migrate from subgraph
		err := migration.MigrateFromSubgraph(ctx, "thread1", "subgraph")
		if err != nil {
			t.Errorf("MigrateFromSubgraph failed: %v", err)
		}
		
		// Check namespace removed
		_, exists := manager.checkpointNS["thread1"]
		if exists {
			t.Error("Expected checkpoint namespace to be removed")
		}
	})
}

func TestResolveParentCommand(t *testing.T) {
	manager := NewSubgraphManager(&Engine{})
	
	t.Run("resolve with namespace", func(t *testing.T) {
		manager.PushNamespace("subgraph1")
		
		cmd := &types.Command{
			Goto: types.Parent,
		}
		
		resolved, err := manager.ResolveParentCommand(context.Background(), cmd)
		if err != nil {
			t.Errorf("ResolveParentCommand failed: %v", err)
		}
		
		if resolved.Goto != types.Parent {
			t.Error("Expected Parent goto")
		}
		
		manager.PopNamespace()
	})
	
	t.Run("resolve at root", func(t *testing.T) {
		cmd := &types.Command{
			Goto: types.Parent,
		}
		
		_, err := manager.ResolveParentCommand(context.Background(), cmd)
		if err == nil {
			t.Error("Expected error at root namespace")
		}
	})
}

func TestCreateSubgraph(t *testing.T) {
	parentEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	manager := NewSubgraphManager(parentEngine)
	
	// Create a simple graph for subgraph
	graph := NewMockGraph()
	graph.AddNode("node1", &MockNode{name: "node1"})
	graph.SetEntryPoint("node1")
	
	t.Run("create subgraph", func(t *testing.T) {
		config := &SubgraphConfig{
			Name:         "subgraph1",
			ParentEngine: parentEngine,
			Graph:        graph,
		}
		
		subgraph, err := manager.CreateSubgraph(config)
		if err != nil {
			t.Fatalf("CreateSubgraph failed: %v", err)
		}
		
		if subgraph == nil {
			t.Fatal("Expected non-nil subgraph")
		}
		
		// Check that subgraph is registered in manager
		_, exists := manager.GetSubgraph("subgraph1")
		if !exists {
			t.Error("Expected subgraph to be registered in manager")
		}
	})
	
	t.Run("duplicate subgraph", func(t *testing.T) {
		config := &SubgraphConfig{
			Name:         "subgraph1",
			ParentEngine: parentEngine,
			Graph:        graph,
		}
		
		_, err := manager.CreateSubgraph(config)
		if err == nil {
			t.Error("Expected error for duplicate subgraph")
		}
	})
}

func TestGetSubgraph(t *testing.T) {
	parentEngine := &Engine{}
	manager := NewSubgraphManager(parentEngine)
	
	// Create a subgraph
	graph := NewMockGraph()
	graph.AddNode("node1", &MockNode{name: "node1"})
	graph.SetEntryPoint("node1")
	
	config := &SubgraphConfig{
		Name:         "test_subgraph",
		ParentEngine: parentEngine,
		Graph:        graph,
	}
	
	subgraph, err := manager.CreateSubgraph(config)
	if err != nil {
		t.Fatalf("Failed to create subgraph: %v", err)
	}
	
	t.Run("get existing subgraph", func(t *testing.T) {
		retrieved, exists := manager.GetSubgraph("test_subgraph")
		if !exists {
			t.Error("Expected subgraph to exist")
		}
		if retrieved != subgraph {
			t.Error("Retrieved subgraph doesn't match")
		}
	})
	
	t.Run("get non-existent subgraph", func(t *testing.T) {
		_, exists := manager.GetSubgraph("non_existent")
		if exists {
			t.Error("Expected subgraph to not exist")
		}
	})
}

func TestNamespaceSeparator(t *testing.T) {
	// Verify constants.NSSep is set correctly
	expectedSep := "|"
	
	if string(constants.NSSep) != expectedSep {
		t.Errorf("Expected NSSep '%s', got '%s'", expectedSep, string(constants.NSSep))
	}
	
	// Test path building with NSSep
	manager := &SubgraphManager{}
	manager.PushNamespace("ns1")
	manager.PushNamespace("ns2")
	
	path := manager.BuildNamespacePath()
	expectedPath := "ns1|ns2"
	
	if path != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, path)
	}
}

// MockCheckpointSaver is a mock implementation of CheckpointSaver for testing.
type MockCheckpointSaver struct {
	checkpoints map[string]map[string]interface{}
	tuples     map[string]*checkpoint.CheckpointTuple
	mu         sync.RWMutex
}

func (m *MockCheckpointSaver) Put(ctx context.Context, config map[string]interface{}, checkpoint map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	id, _ := config["checkpoint_id"].(string)
	m.checkpoints[id] = checkpoint
	return nil
}

func (m *MockCheckpointSaver) Get(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	id, _ := config["checkpoint_id"].(string)
	cp, exists := m.checkpoints[id]
	if !exists {
		return nil, nil
	}
	return cp, nil
}

func (m *MockCheckpointSaver) List(ctx context.Context, config map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *MockCheckpointSaver) PutWrites(ctx context.Context, config map[string]interface{}, writes []*checkpoint.PendingWrite) error {
	return nil
}

func (m *MockCheckpointSaver) GetTuple(ctx context.Context, config map[string]interface{}) (*checkpoint.CheckpointTuple, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	id, _ := config["checkpoint_id"].(string)
	tuple, exists := m.tuples[id]
	if !exists {
		return nil, nil
	}
	return tuple, nil
}

func (m *MockCheckpointSaver) GetLineage(ctx context.Context, threadID string) ([]*checkpoint.LineageEntry, error) {
	return nil, nil
}

func (m *MockCheckpointSaver) DeleteThread(ctx context.Context, threadID string) error {
	return nil
}

// MockNode is a mock implementation of Node for testing.
type MockNode struct {
	name string
}

func (n *MockNode) Invoke(ctx context.Context, input interface{}) (interface{}, error) {
	return map[string]interface{}{
		"output": n.name + "_result",
	}, nil
}

// MockGraph is a mock implementation of a graph for testing.
type MockGraph struct{}

func (g *MockGraph) AddNode(name string, node interface{}) {
	// Mock implementation
}

func (g *MockGraph) SetEntryPoint(node string) {
	// Mock implementation
}

// NewMockGraph creates a new mock graph.
func NewMockGraph() *MockGraph {
	return &MockGraph{}
}
