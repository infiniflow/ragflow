// Package graph provides CompiledStateGraph implementation for subgraph support.
package graph

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// CompiledStateGraph represents a compiled state graph with full subgraph support.
// This corresponds to Python's CompiledStateGraph in graph/state.py
type CompiledStateGraph struct {
	*CompiledGraph
	
	// subgraphs maps subgraph names to their compiled graphs
	subgraphs map[string]*CompiledStateGraph
	
	// parent is the parent graph (nil for root graph)
	parent *CompiledStateGraph
	
	// namespace is the checkpoint namespace for this graph
	namespace string
	
	// checkpointMap maps parent checkpoint IDs to child checkpoint IDs
	checkpointMap map[string]string
	
	mu sync.RWMutex
}

// NewCompiledStateGraph creates a new compiled state graph.
func NewCompiledStateGraph(base *CompiledGraph) *CompiledStateGraph {
	return &CompiledStateGraph{
		CompiledGraph:   base,
		subgraphs:       make(map[string]*CompiledStateGraph),
		parent:          nil,
		namespace:       "",
		checkpointMap:   make(map[string]string),
	}
}

// AddSubgraph adds a subgraph to this compiled graph.
func (c *CompiledStateGraph) AddSubgraph(name string, subgraph *StateGraph) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.subgraphs[name]; exists {
		return fmt.Errorf("subgraph '%s' already exists", name)
	}

	// Compile the subgraph
	compiled, err := subgraph.Compile()
	if err != nil {
		return fmt.Errorf("failed to compile subgraph '%s': %w", name, err)
	}

	// Wrap in CompiledStateGraph
	subgraphCSG := &CompiledStateGraph{
		CompiledGraph: compiled,
		subgraphs:     make(map[string]*CompiledStateGraph),
		parent:        c,
		namespace:     buildSubgraphNamespace(c.namespace, name),
		checkpointMap: make(map[string]string),
	}

	c.subgraphs[name] = subgraphCSG
	return nil
}

// GetSubgraph retrieves a subgraph by name.
func (c *CompiledStateGraph) GetSubgraph(name string) (*CompiledStateGraph, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	subgraph, exists := c.subgraphs[name]
	return subgraph, exists
}

// GetSubgraphs returns all subgraphs.
func (c *CompiledStateGraph) GetSubgraphs() map[string]*CompiledStateGraph {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	result := make(map[string]*CompiledStateGraph, len(c.subgraphs))
	for name, subgraph := range c.subgraphs {
		result[name] = subgraph
	}
	return result
}

// Invoke executes the graph with subgraph support.
func (c *CompiledStateGraph) Invoke(ctx context.Context, input interface{}, config ...*types.RunnableConfig) (interface{}, error) {
	// Set up namespace in config
	rc := &types.RunnableConfig{}
	if len(config) > 0 && config[0] != nil {
		rc = config[0]
	}

	// Add checkpoint namespace
	if rc.Configurable == nil {
		rc.Configurable = make(map[string]interface{})
	}
	rc.Configurable[constants.ConfigKeyCheckpointNS] = c.namespace

	// Invoke base graph
	return c.CompiledGraph.Invoke(ctx, input, rc)
}

// Stream executes the graph with streaming and subgraph support.
func (c *CompiledStateGraph) Stream(ctx context.Context, input interface{}, mode types.StreamMode, config ...*types.RunnableConfig) (<-chan interface{}, <-chan error) {
	// Set up namespace in config
	rc := &types.RunnableConfig{}
	if len(config) > 0 && config[0] != nil {
		rc = config[0]
	}
	if rc.Configurable == nil {
		rc.Configurable = make(map[string]interface{})
	}
	rc.Configurable[constants.ConfigKeyCheckpointNS] = c.namespace

	// Stream from base graph
	return c.CompiledGraph.Stream(ctx, input, mode, rc)
}

// MigrateCheckpoint migrates a checkpoint from parent to subgraph or vice versa.
func (c *CompiledStateGraph) MigrateCheckpoint(
	ctx context.Context,
	threadID string,
	checkpointID string,
	toSubgraph string,
) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if toSubgraph == "" {
		// Migrate to parent
		if c.parent == nil {
			return "", fmt.Errorf("no parent graph to migrate to")
		}
		
		// Get parent checkpoint ID from map
		parentCheckpointID, exists := c.checkpointMap[checkpointID]
		if !exists {
			return "", fmt.Errorf("no parent checkpoint mapping found for %s", checkpointID)
		}
		
		return parentCheckpointID, nil
	}

	// Migrate to subgraph
	subgraph, exists := c.subgraphs[toSubgraph]
	if !exists {
		return "", fmt.Errorf("subgraph '%s' not found", toSubgraph)
	}

	// Create new checkpoint ID for subgraph
	newCheckpointID := generateCheckpointID()
	
	// Store mapping
	subgraph.checkpointMap[newCheckpointID] = checkpointID
	c.checkpointMap[checkpointID] = newCheckpointID

	return newCheckpointID, nil
}

// GetNamespace returns the checkpoint namespace for this graph.
func (c *CompiledStateGraph) GetNamespace() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.namespace
}

// GetParent returns the parent graph.
func (c *CompiledStateGraph) GetParent() *CompiledStateGraph {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.parent
}

// IsRoot returns true if this is the root graph (no parent).
func (c *CompiledStateGraph) IsRoot() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.parent == nil
}

// GetCheckpointMap returns the checkpoint mapping.
func (c *CompiledStateGraph) GetCheckpointMap() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	result := make(map[string]string, len(c.checkpointMap))
	for k, v := range c.checkpointMap {
		result[k] = v
	}
	return result
}

// buildSubgraphNamespace builds the namespace for a subgraph.
func buildSubgraphNamespace(parentNS, subgraphName string) string {
	if parentNS == "" {
		return subgraphName
	}
	return parentNS + constants.NSSep + subgraphName
}

// buildTaskPath builds the task path for checkpoint migration.
func buildTaskPath(namespace, subgraphName string) string {
	if namespace == "" {
		return subgraphName + string(constants.NSEnd)
	}
	return namespace + string(constants.NSSep) + subgraphName + string(constants.NSEnd)
}

// generateCheckpointID generates a new checkpoint ID.
func generateCheckpointID() string {
	return "cp_" + uuid.New().String()
}
