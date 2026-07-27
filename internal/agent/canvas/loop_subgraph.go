// loop_subgraph.go — Loop macro expansion for BuildWorkflow.
//
// The RAGFlow DSL expresses a loop as a parent Loop component with a
// chain of downstream body components. We collapse this to a SINGLE
// harness node by:
//
//  1. Collecting the Loop's downstream descendants into a sub-graph
//     (a *graphpkg.StateGraph).
//  2. Compiling it into a *graphpkg.CompiledGraph.
//  3. Creating a LoopCondition closure that reads CanvasState slots.
//  4. Wrapping via graph.NewLoopNodeFunc into a single NodeFunc.
//
// The returned compiled sub-graph is passed to NewLoopNodeFunc, which
// produces a NodeFunc that the outer StateGraph registers as one node.
package canvas

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"ragflow/internal/harness/graph/constants"
	graphpkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// loopExpansion holds the artefacts produced by buildLoopExpansion.
type loopExpansion struct {
	Sub        types.CompiledGraph  // compiled loop body sub-graph
	ShouldQuit LoopConditionHarness // terminal condition
	MaxIters   int
	Members    map[string]bool // cpn_ids consumed by the sub-graph
}

// LoopConditionHarness is a graph.LoopCondition alias for internal use.
// It is the same as graph.LoopCondition.
type LoopConditionHarness = graphpkg.LoopCondition

// buildLoopExpansion constructs the sub-graph + termination condition
// for the given Loop cpn. It does NOT touch the outer graph — the
// caller is responsible for installing the result via graph.NewLoopNodeFunc.
func buildLoopExpansion(ctx context.Context, c *Canvas, loopID string) (*loopExpansion, error) {
	if c == nil {
		return nil, fmt.Errorf("canvas: nil canvas")
	}
	if loopID == "" {
		return nil, fmt.Errorf("canvas: buildLoopExpansion: empty loopID")
	}
	if _, ok := c.Components[loopID]; !ok {
		return nil, fmt.Errorf("canvas: buildLoopExpansion: unknown cpn %q", loopID)
	}

	loopComp := c.Components[loopID]
	members := collectDescendants(c, loopID)

	initValues, err := resolveInitialVariables(loopComp.Obj.Params)
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q: %w", loopID, err)
	}

	shouldQuit, err := translateLoopCondition(loopID, loopComp.Obj.Params)
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q: %w", loopID, err)
	}

	maxIters := readMaxLoopCount(loopComp.Obj.Params)

	sub, err := buildSubGraph(ctx, c, members, loopID, initValues)
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q: %w", loopID, err)
	}

	return &loopExpansion{
		Sub:        sub,
		ShouldQuit: shouldQuit,
		MaxIters:   maxIters,
		Members:    members,
	}, nil
}

// collectDescendants returns the set of cpn_ids reachable from root via
// downstream edges, NOT including root itself.
func collectDescendants(c *Canvas, root string) map[string]bool {
	visited := make(map[string]bool)
	queue := []string{}
	for _, child := range c.Components[root].Downstream {
		if child == root {
			continue
		}
		if !visited[child] {
			visited[child] = true
			queue = append(queue, child)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range c.Components[cur].Downstream {
			if child == root || child == cur {
				continue
			}
			if !visited[child] {
				visited[child] = true
				queue = append(queue, child)
			}
		}
	}
	return visited
}

// buildSubGraph constructs a fresh StateGraph containing one node per
// member cpn, plus a synthetic "LoopInit" entry node. It compiles the
// graph and returns the CompiledGraph.
func buildSubGraph(
	ctx context.Context,
	c *Canvas,
	members map[string]bool,
	loopID string,
	initValues map[string]initVarSpec,
) (types.CompiledGraph, error) {
	_ = ctx
	sg := graphpkg.NewStateGraph(map[string]any{})

	// Synthetic entry node: writes loop variables into CanvasState
	// the FIRST time the sub-graph runs.
	initNode := func(ctx context.Context, in any) (any, error) {
		state, _, err := GetStateFromContext[*CanvasState](ctx)
		if err != nil || state == nil {
			return in, nil
		}
		for k, spec := range initValues {
			existing, _ := state.GetVar(loopID + "@" + k)
			if existing != nil {
				continue
			}
			v := spec.Value
			if spec.InputMode == "variable" {
				ref, _ := spec.Value.(string)
				resolved, err := state.GetVar(ref)
				if err != nil {
					return nil, fmt.Errorf("canvas: loop %q init: variable %q ref %q: %w", loopID, k, ref, err)
				}
				v = resolved
			}
			state.SetVar(loopID, k, v)
		}
		return in, nil
	}
	sg.AddNode(loopInitKey, initNode)

	// Body nodes: register each member.
	nodes := map[string]bool{loopInitKey: true}
	for cpnID := range members {
		name := c.Components[cpnID].Obj.ComponentName
		if name == "" {
			return nil, fmt.Errorf("canvas: loop %q member %q has empty component_name", loopID, cpnID)
		}
		body, err := buildNodeBody(ctx, cpnID, name, c.Components[cpnID].Obj.Params)
		if err != nil {
			return nil, err
		}
		sg.AddNode(cpnID, withStateBracket(body))
		nodes[cpnID] = true
	}

	// Wire edges. The synthetic init node connects to every body node
	// that has no upstream within the sub-graph.
	for cpnID := range members {
		upstreams := c.Components[cpnID].Upstream
		wired := false
		for _, up := range upstreams {
			if up == loopID || members[up] {
				target := up
				if up == loopID {
					target = loopInitKey
				}
				if err := sg.AddEdge(target, cpnID); err != nil {
					// Ignore errors from mid-construction edges
					_ = err
				}
				wired = true
			}
		}
		if !wired {
			// No in-subgraph upstream: wire from init.
			_ = sg.AddEdge(loopInitKey, cpnID)
		}
	}

	// Wire END: every member with no in-subgraph downstream.
	hasDownstream := make(map[string]bool, len(members))
	for cpnID := range members {
		for _, down := range c.Components[cpnID].Downstream {
			if members[down] {
				hasDownstream[cpnID] = true
				break
			}
		}
	}
	hasEnd := false
	for cpnID := range members {
		if hasDownstream[cpnID] {
			continue
		}
		_ = sg.AddEdge(cpnID, constants.End)
		hasEnd = true
	}
	if !hasEnd {
		_ = sg.AddEdge(loopInitKey, constants.End)
	}

	// Wire START to the init node.
	_ = sg.AddEdge(constants.Start, loopInitKey)

	// Compile the sub-graph.
	compiled, err := sg.Compile()
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q sub-graph compile: %w", loopID, err)
	}
	return compiled, nil
}

// loopInitKey is the synthetic cpn_id used for the LoopInit entry node.
const loopInitKey = "__loop_init__"

// translateLoopCondition converts DSL loop_termination_condition to
// a graph.LoopCondition closure. The closure reads state via ctx.
//
// The DSL `loop_termination_condition` schema is unchanged from the
// harness version; only the return type changes to graph.LoopCondition
// to graph.LoopCondition (same signature).
func translateLoopCondition(loopID string, params map[string]any) (graphpkg.LoopCondition, error) {
	rawList, _ := params["loop_termination_condition"].([]any)
	conditions := make([]loopConditionSpec, 0, len(rawList))
	for i, raw := range rawList {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("loop_termination_condition[%d]: not a map", i)
		}
		variable, hasVar := m["variable"].(string)
		operator, hasOp := m["operator"].(string)
		if !hasVar || variable == "" {
			return nil, fmt.Errorf("loop_termination_condition[%d] is incomplete (missing 'variable')", i)
		}
		if !hasOp || operator == "" {
			return nil, fmt.Errorf("loop_termination_condition[%d] is incomplete (missing 'operator')", i)
		}
		inputMode, _ := m["input_mode"].(string)
		if inputMode == "" {
			inputMode = "constant"
		}
		conditions = append(conditions, loopConditionSpec{
			Variable:  variable,
			Operator:  operator,
			Value:     m["value"],
			InputMode: inputMode,
		})
	}
	logicalOp, _ := params["logical_operator"].(string)
	if logicalOp == "" {
		logicalOp = "and"
	}
	if logicalOp != "and" && logicalOp != "or" {
		return nil, fmt.Errorf("invalid logical_operator %q (want 'and' or 'or')", logicalOp)
	}

	return func(ctx context.Context, _ int, _, _ interface{}) (bool, error) {
		state, _, err := GetStateFromContext[*CanvasState](ctx)
		if err != nil || state == nil {
			return false, fmt.Errorf("loop %q: condition eval: no canvas state in context", loopID)
		}
		if len(conditions) == 0 {
			return false, nil
		}
		combined := logicalOp == "and"
		for _, spec := range conditions {
			v, err := evalOneLoopCondition(state, loopID, spec)
			if err != nil {
				return false, err
			}
			if logicalOp == "or" {
				combined = combined || v
			} else {
				combined = combined && v
			}
		}
		return combined, nil
	}, nil
}

// initVarSpec carries the per-variable info the init lambda needs to
// decide how to seed the loop variable into the per-run state.
type initVarSpec struct {
	Value     any
	InputMode string
}

// resolveInitialVariables applies the input_mode dispatch from
// agent/component/loop.py:60-77 to a list of loop_variable entries.
func resolveInitialVariables(params map[string]any) (map[string]initVarSpec, error) {
	rawList, _ := params["loop_variables"].([]any)
	out := make(map[string]initVarSpec, len(rawList))
	for i, raw := range rawList {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("loop_variable[%d]: not a map", i)
		}
		name, inputMode, value, typ, err := readLoopVarFields(item)
		if err != nil {
			return nil, err
		}
		v, err := resolveLoopVarValue(inputMode, value, typ)
		if err != nil {
			return nil, fmt.Errorf("loop_variable[%d] %q: %w", i, name, err)
		}
		out[name] = initVarSpec{Value: v, InputMode: inputMode}
	}
	return out, nil
}

func readLoopVarFields(item map[string]any) (name, inputMode string, value, typ any, err error) {
	if item == nil {
		return "", "", nil, nil, fmt.Errorf("nil loop_variable entry")
	}
	vRaw, hasVar := item["variable"]
	imRaw, hasIM := item["input_mode"]
	valRaw, hasVal := item["value"]
	typeRaw, hasType := item["type"]

	if !hasVar || vRaw == nil {
		return "", "", nil, nil, fmt.Errorf("loop_variable is not complete (missing 'variable')")
	}
	if !hasIM || imRaw == nil {
		return "", "", nil, nil, fmt.Errorf("loop_variable is not complete (missing 'input_mode')")
	}
	if !hasVal {
		return "", "", nil, nil, fmt.Errorf("loop_variable is not complete (missing 'value')")
	}
	if !hasType || typeRaw == nil {
		return "", "", nil, nil, fmt.Errorf("loop_variable is not complete (missing 'type')")
	}

	name, _ = vRaw.(string)
	if name == "" {
		name = fmt.Sprintf("%v", vRaw)
	}
	inputMode, _ = imRaw.(string)
	return name, inputMode, valRaw, typeRaw, nil
}

func resolveLoopVarValue(inputMode string, value, typ any) (any, error) {
	switch inputMode {
	case "variable":
		return value, nil
	case "constant":
		return value, nil
	}
	return zeroValueForType(typ), nil
}

// zeroValueForType implements the type→zero mapping from
// agent/component/loop.py:65-76:
//
//	number  → 0
//	string  → ""
//	boolean → false
//	object* → map[string]any{}
//	array*  → []any{}
//	else    → ""
func zeroValueForType(typ any) any {
	s, _ := typ.(string)
	switch {
	case s == "number":
		return 0
	case s == "string":
		return ""
	case s == "boolean":
		return false
	case strings.HasPrefix(s, "object"):
		return map[string]any{}
	case strings.HasPrefix(s, "array"):
		return []any{}
	}
	return ""
}

type loopConditionSpec struct {
	Variable  string
	Operator  string
	Value     any
	InputMode string
}

// evalOneLoopCondition resolves a single condition entry.
func evalOneLoopCondition(state *CanvasState, loopID string, spec loopConditionSpec) (bool, error) {
	var rhs any
	if spec.InputMode == "variable" {
		ref, _ := spec.Value.(string)
		v, err := state.GetVar(ref)
		if err != nil {
			return false, fmt.Errorf("loop %q: condition rhs ref %q: %w", loopID, ref, err)
		}
		rhs = v
	} else if spec.InputMode != "constant" {
		return false, fmt.Errorf("loop %q: invalid input mode %q", loopID, spec.InputMode)
	} else {
		rhs = spec.Value
	}
	ref := spec.Variable
	if !strings.Contains(ref, ".") && !strings.Contains(ref, "@") {
		ref = loopID + "@" + ref
	}
	got, err := state.GetVar(ref)
	if err != nil {
		return false, fmt.Errorf("loop %q: condition lhs ref %q: %w", loopID, ref, err)
	}
	return evaluateCondition(got, spec.Operator, rhs)
}

// evaluateCondition is the type-dispatched operator logic.
func evaluateCondition(varVal any, op string, value any) (bool, error) {
	switch v := varVal.(type) {
	case nil:
		if op == "empty" {
			return true, nil
		}
		return false, nil
	case string:
		return evalStringOp(v, op, value)
	case bool:
		return evalBoolOp(v, op, value)
	case int:
		return evalNumberOp(float64(v), op, value)
	case int32:
		return evalNumberOp(float64(v), op, value)
	case int64:
		return evalNumberOp(float64(v), op, value)
	case float32:
		return evalNumberOp(float64(v), op, value)
	case float64:
		return evalNumberOp(v, op, value)
	case map[string]any:
		return evalDictOp(v, op, value)
	case []any:
		return evalListOp(v, op, value)
	}
	return false, fmt.Errorf("invalid operator: %s (variable type %T unsupported)", op, varVal)
}

func evalStringOp(s, op string, value any) (bool, error) {
	switch op {
	case "contains":
		vs, _ := value.(string)
		return strings.Contains(s, vs), nil
	case "not contains":
		vs, _ := value.(string)
		return !strings.Contains(s, vs), nil
	case "start with":
		vs, _ := value.(string)
		return strings.HasPrefix(s, vs), nil
	case "end with":
		vs, _ := value.(string)
		return strings.HasSuffix(s, vs), nil
	case "is":
		return s == value, nil
	case "is not":
		return s != value, nil
	case "empty":
		return s == "", nil
	case "not empty":
		return s != "", nil
	}
	return false, fmt.Errorf("invalid operator: %s (string variable)", op)
}

func evalBoolOp(b bool, op string, value any) (bool, error) {
	switch op {
	case "is":
		vb, _ := value.(bool)
		return b == vb, nil
	case "is not":
		vb, _ := value.(bool)
		return b != vb, nil
	case "empty":
		return b == false && value == nil, nil
	case "not empty":
		return b == true || value != nil, nil
	}
	return false, fmt.Errorf("invalid operator: %s (bool variable)", op)
}

func evalNumberOp(n float64, op string, value any) (bool, error) {
	cmp, ok := toFloat(value)
	if !ok && !isNilOp(op) {
		return false, fmt.Errorf("invalid operator: %s (number variable, non-numeric value)", op)
	}
	switch op {
	case "=":
		return n == cmp, nil
	case "≠":
		return n != cmp, nil
	case ">":
		return n > cmp, nil
	case "<":
		return n < cmp, nil
	case "≥":
		return n >= cmp, nil
	case "≤":
		return n <= cmp, nil
	case "empty":
		return value == nil, nil
	case "not empty":
		return value != nil, nil
	}
	return false, fmt.Errorf("invalid operator: %s (number variable)", op)
}

func evalDictOp(m map[string]any, op string, _ any) (bool, error) {
	switch op {
	case "empty":
		return len(m) == 0, nil
	case "not empty":
		return len(m) > 0, nil
	}
	return false, fmt.Errorf("invalid operator: %s (dict variable)", op)
}

func evalListOp(lst []any, op string, value any) (bool, error) {
	switch op {
	case "contains":
		return slices.Contains(lst, value), nil
	case "not contains":
		return !slices.Contains(lst, value), nil
	case "is":
		return listEqual(lst, value), nil
	case "is not":
		return !listEqual(lst, value), nil
	case "empty":
		return len(lst) == 0, nil
	case "not empty":
		return len(lst) > 0, nil
	}
	return false, fmt.Errorf("invalid operator: %s (list variable)", op)
}

func listEqual(lst []any, value any) bool {
	other, ok := value.([]any)
	if !ok {
		return false
	}
	if len(lst) != len(other) {
		return false
	}
	for i := range lst {
		if lst[i] != other[i] {
			return false
		}
	}
	return true
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func isNilOp(op string) bool {
	return op == "empty" || op == "not empty"
}

// readMaxLoopCount returns the configured `maximum_loop_count` for the
// Loop. 0 means "use default cap" (1024 iterations) — the scheduler only
// calls WithLoopMaxIterations when exp.MaxIters > 0, so 0 falls back to
// the harness's default recursion limit.
func readMaxLoopCount(params map[string]any) int {
	v, ok := params["maximum_loop_count"]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case int32:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	}
	return 0
}
