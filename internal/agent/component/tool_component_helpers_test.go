package component

import (
	"context"
	"encoding/json"
	"fmt"

	"ragflow/internal/agent/tool"
)

// fakeTool implements tool.Tool for use in component tests.
type fakeTool struct {
	name  string
	runFn func(ctx context.Context, argsJSON string) (string, error)
}

func (f *fakeTool) ToolMeta() tool.ToolMeta {
	return tool.ToolMeta{Name: f.name}
}

func (f *fakeTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	if f.runFn != nil {
		return f.runFn(ctx, argsJSON)
	}
	return "{}", nil
}

// simpleToolComponent wraps a tool.Tool as a Component (mirrors
// the production fixture_stubs.go simpleToolDelegate but is
// self-contained for tests).
func simpleToolComponent(name string, t tool.Tool) Component {
	return &testToolDelegate{name: name, inner: t}
}

type testToolDelegate struct {
	name  string
	inner tool.Tool
}

func (d *testToolDelegate) Name() string { return d.name }

func (d *testToolDelegate) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	argsJSON, _ := json.Marshal(inputs)
	out, err := d.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: %s: %w", d.name, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("canvas: %s: decode result: %w", d.name, err)
	}
	return result, nil
}

func (d *testToolDelegate) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (d *testToolDelegate) Inputs() map[string]string  { return nil }
func (d *testToolDelegate) Outputs() map[string]string { return nil }
func (d *testToolDelegate) Parallelism() int           { return 1 }
func (d *testToolDelegate) GetInputForm() map[string]any {
	return map[string]any{"query": map[string]any{"type": "line"}}
}

// newBGPTComponentWithInvoker creates a BGPT component backed by the given
// tool.Tool. The second parameter is the stored API key (optional).
func newBGPTComponentWithInvoker(tool tool.Tool, storedKey ...string) Component {
	setups := map[string]any{}
	if len(storedKey) > 0 && storedKey[0] != "" {
		setups["api_key"] = storedKey[0]
	}
	return &testToolDelegateWithSetup{
		name:   "BGPT",
		inner:  tool,
		setups: setups,
	}
}

// newDuckDuckGoComponentWithInvoker creates a DuckDuckGo component.
func newDuckDuckGoComponentWithInvoker(tool tool.Tool) Component {
	return simpleToolComponent("DuckDuckGo", tool)
}

// newGoogleScholarComponentWithInvoker creates a GoogleScholar component.
func newGoogleScholarComponentWithInvoker(tool tool.Tool, params map[string]any) Component {
	return &testToolDelegateWithSetup{
		name:   "GoogleScholar",
		inner:  tool,
		setups: params,
	}
}

// testToolDelegateWithSetup is like testToolDelegate but merges
// stored setup values into the input before passing to the tool.
type testToolDelegateWithSetup struct {
	name   string
	inner  tool.Tool
	setups map[string]any
}

func (d *testToolDelegateWithSetup) Name() string { return d.name }

func (d *testToolDelegateWithSetup) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(inputs)+len(d.setups))
	for k, v := range d.setups {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}
	argsJSON, _ := json.Marshal(merged)
	out, err := d.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: %s: %w", d.name, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("canvas: %s: decode result: %w", d.name, err)
	}
	return result, nil
}

func (d *testToolDelegateWithSetup) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}
func (d *testToolDelegateWithSetup) Inputs() map[string]string  { return nil }
func (d *testToolDelegateWithSetup) Outputs() map[string]string { return nil }
func (d *testToolDelegateWithSetup) Parallelism() int           { return 1 }
func (d *testToolDelegateWithSetup) GetInputForm() map[string]any {
	return map[string]any{"query": map[string]any{"type": "line"}}
}

// anySlice attempts a type assertion to []any and returns the result
// or nil on failure.
func anySlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
