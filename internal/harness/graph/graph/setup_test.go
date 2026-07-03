package graph

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// isTestRunner is set to true when the graph package's own test runner is
// active (as opposed to the full pregel engine from the pregel package).
// Tests that need the real pregel engine (checkpoints, interrupts, time
// travel) should check this and skip.
var isTestRunner bool

func init() {
	isTestRunner = true
	types.SetPregelRunFunc(testPregelRun)
}

// TestMain provides package-level test filtering.
func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

// testPregelRun provides a minimal sequential executor so that graph
// package tests can compile and invoke graphs without importing the
// full pregel engine (circular dependency: pregel imports graph).
func testPregelRun(
	ctx context.Context,
	cg types.CompiledGraph,
	input interface{},
	config *types.RunnableConfig,
	streamMode types.StreamMode,
) (interface{}, error) {
	g := cg.GetGraph()
	graphChannels := g.GetChannels()

	// Build a channel registry from the graph's channels.
	reg := channels.NewRegistry()
	for name, ch := range graphChannels {
		if chImpl, ok := ch.(channels.Channel); ok {
			reg.Register(name, chImpl.Copy())
		}
	}

	// Convert input to map and apply to channels.
	inputMap, err := toMap(input)
	if err != nil {
		return nil, fmt.Errorf("apply input: %w", err)
	}
	// Auto-register channels for any keys in the input that aren't yet registered.
	for k, v := range inputMap {
		if _, ok := reg.Get(k); !ok {
			reg.Register(k, channels.NewLastValue(v))
		}
	}
	writes := make(map[string][]any, len(inputMap))
	for k, v := range inputMap {
		writes[k] = []any{v}
	}
	if err := reg.UpdateChannels(writes); err != nil {
		return nil, fmt.Errorf("apply input: %w", err)
	}

	// Simple sequential execution: follow edges from entry point.
	visited := make(map[string]bool)
	queue := []string{g.GetEntryPoint()}
	maxSteps := cg.GetRecursionLimit()
	if maxSteps <= 0 {
		maxSteps = 25
	}

	step := 0
	for len(queue) > 0 {
		if step >= maxSteps {
			return nil, fmt.Errorf("recursion limit reached (%d steps)", maxSteps)
		}
		current := queue[0]
		queue = queue[1:]

		// Allow revisiting loops (but limit by maxSteps).
		visited[current] = true
		step++

		node, ok := g.GetNode(current)
		if !ok {
			return nil, fmt.Errorf("node %s not found", current)
		}

		// Read current state from registry and invoke the node.
		currentState, _ := reg.GetValues()
		output, nodeErr := node.Function(ctx, currentState)
		if nodeErr != nil {
			return nil, fmt.Errorf("node %s error: %w", current, nodeErr)
		}

		// Write output back to channels.
		if output != nil {
			outMap, err := toMap(output)
			if err != nil {
				return nil, fmt.Errorf("output conversion at %s: %w", current, err)
			}
			chWrites := make(map[string][]any, len(outMap))
			for k, v := range outMap {
				// Auto-register channels for any new keys in the output.
				if _, ok := reg.Get(k); !ok {
					reg.Register(k, channels.NewLastValue(v))
				}
				chWrites[k] = []any{v}
			}
			if err := reg.UpdateChannels(chWrites); err != nil {
				return nil, fmt.Errorf("update channels at %s: %w", current, err)
			}
		}

		// Determine next node(s) via edges.
		edges := g.GetEdges()
		condEdges := g.GetConditionalEdges()
		for _, edge := range edges {
			if edge.From == current {
				if edge.To == constants.End {
					finalState, _ := reg.GetValues()
					return finalState, nil
				}
				queue = append(queue, edge.To)
			}
		}
		for _, ce := range condEdges {
			if ce.From == current {
				result, _ := ce.Condition(ctx, currentState)
				resultStr := fmt.Sprintf("%v", result)
				if target, ok := ce.Mapping[resultStr]; ok {
					if target == constants.End {
						finalState, _ := reg.GetValues()
						return finalState, nil
					}
					queue = append(queue, target)
				}
			}
		}
	}

	finalState, _ := reg.GetValues()
	return finalState, nil
}

// toMap converts any value to map[string]any.
func toMap(val any) (map[string]any, error) {
	if val == nil {
		return map[string]any{}, nil
	}

	if m, ok := val.(map[string]any); ok {
		return m, nil
	}

	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Map {
		return map[string]any{"__root__": val}, nil
	}

	result := make(map[string]any)
	if rv.Kind() == reflect.Map {
		for _, key := range rv.MapKeys() {
			result[fmt.Sprintf("%v", key.Interface())] = rv.MapIndex(key).Interface()
		}
		return result, nil
	}

	// Struct — use field names as-is (matches how configureChannelsFromSchema
	// registers channels via processField, which uses field.Name).
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		val := rv.Field(i).Interface()
		result[field.Name] = val
	}
	return result, nil
}
