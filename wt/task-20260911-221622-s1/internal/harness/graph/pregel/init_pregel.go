// Package pregel wires itself via types.SetPregelRunFunc so that
// types.CompiledGraph.Invoke uses the Pregel execution engine.
package pregel

import (
	"context"

	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/types"
)

func init() {
	types.SetPregelRunFunc(runCompiledGraph)
}

func runCompiledGraph(
	ctx context.Context,
	cg types.CompiledGraph,
	input interface{},
	config *types.RunnableConfig,
	streamMode types.StreamMode,
) (interface{}, error) {
	interruptKeys := make([]string, 0, len(cg.GetInterrupts()))
	for k := range cg.GetInterrupts() {
		interruptKeys = append(interruptKeys, k)
	}
	interruptAfterKeys := make([]string, 0, len(cg.GetInterruptsAfter()))
	for k := range cg.GetInterruptsAfter() {
		interruptAfterKeys = append(interruptAfterKeys, k)
	}

	cp, _ := cg.GetCheckpointer().(checkpoint.BaseCheckpointer)
	engine := NewEngine(cg.GetGraph(),
		WithCheckpointer(cp),
		WithInterrupts(interruptKeys...),
		WithInterruptsAfter(interruptAfterKeys...),
		WithRecursionLimit(cg.GetRecursionLimit()),
		WithDebug(cg.IsDebug()),
		WithConfig(config),
	)
	return engine.RunSync(ctx, input)
}
