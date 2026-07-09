//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// loop_subgraph.go — Loop macro expansion for BuildWorkflow.
//
// The RAGFlow DSL expresses a loop as a parent Loop component with a
// chain of downstream body components. In the Go port we collapse this
// to a SINGLE eino node by:
//  1. Collecting the Loop's downstream descendants into a sub-graph
//     (a *compose.Workflow[map[string]any, map[string]any]).
//  2. Prepending a synthetic "LoopInit" lambda that resolves the DSL's
//     `loop_variables` and writes them into the per-run CanvasState
//     under `state.Outputs[loopID][name]`, then passes the outer input
//     through.
//  3. Translating the DSL's `loop_termination_condition` list into a
//     `workflowx.LoopCondition[map[string]any]` closure that reads the
//     same state slots via `state.GetVar` on every iteration.
//
// The actual installation into the outer graph is done by BuildWorkflow
// (canvas.go) via workflowx.AddLoopNode, which registers the resulting
// *WorkflowNode inside the outer *compose.Workflow.
package canvas

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"ragflow/internal/agent/workflowx"

	"github.com/cloudwego/eino/compose"
)

// loopExpansion holds the two artefacts produced by buildLoopExpansion
// and consumed by BuildWorkflow to install the loop node.
type loopExpansion struct {
	Sub        *compose.Workflow[map[string]any, map[string]any]
	ShouldQuit workflowx.LoopCondition[map[string]any]
	MaxIters   int
	Members    map[string]bool // cpn_ids consumed by the sub-graph; caller skips these in the main pass.
}

// buildLoopExpansion constructs the sub-workflow + termination condition
// for the given Loop cpn. It does NOT touch the outer workflow — the
// caller is responsible for installing the result via
// workflowx.AddLoopNode and for skipping the members in the main
// BuildWorkflow pass.
//
// Parameters:
//
//	c      — the parent Canvas (DSL representation).
//	loopID — the cpn_id of the Loop component being expanded.
//
// The returned `Members` is the set of cpn_ids that the expansion
// consumed as body nodes. BuildWorkflow must skip these when iterating
// `c.Components` in the main pass (they will be wired inside the
// sub-graph, not the outer graph).
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

	members := collectLoopMembers(c, loopID)

	initValues, err := resolveInitialVariables(loopComp.Obj.Params)
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q: %w", loopID, err)
	}

	shouldQuit, err := translateLoopCondition(loopID, loopComp.Obj.Params)
	if err != nil {
		return nil, fmt.Errorf("canvas: loop %q: %w", loopID, err)
	}

	maxIters := readMaxLoopCount(loopComp.Obj.Params)

	sub, err := buildSubWorkflow(ctx, c, members, loopID, initValues)
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
// downstream edges, NOT including root itself. The BFS stops at the
// back-edge to root (i.e. a node whose Downstream contains root). This
// prevents infinite recursion on cyclic graphs.
func collectLoopMembers(c *Canvas, loopID string) map[string]bool {
	members := collectGroupedMembers(c, loopID)
	if len(members) > 0 {
		return members
	}
	return collectDescendants(c, loopID)
}

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

// buildSubWorkflow constructs a fresh *compose.Workflow[map[string]any,
// map[string]any] containing one node per member cpn, plus a synthetic
// "LoopInit" entry node that seeds the loop variables into the per-run
// state. Edges within the sub-graph mirror the canvas's Downstream
// relations. The sub-workflow's START wires to LoopInit; the END wires
// to whichever member has no downstream within the sub-graph (the
// "tail" of the body).
//
// Body nodes are built through buildNodeBody so they share the same
// legacy-no-op / factory / placeholder routing as the outer graph,
// and receive the same statePre / statePost handlers so loop body
// outputs land in CanvasState.Outputs alongside outer-node outputs.
func buildSubWorkflow(
	ctx context.Context,
	c *Canvas,
	members map[string]bool,
	loopID string,
	initValues map[string]initVarSpec,
) (*compose.Workflow[map[string]any, map[string]any], error) {
	_ = ctx
	sub := compose.NewWorkflow[map[string]any, map[string]any]()
	nodes := make(map[string]*compose.WorkflowNode, len(members)+1)

	// Synthetic entry: writes loop variables into the per-run state
	// the FIRST TIME the sub-workflow runs, then returns the input
	// map unchanged. Subsequent iterations skip the seeding so the
	// body's mutations accumulate across iterations — otherwise a
	// VariableAssigner that increments `counter` would be clobbered
	// back to its initial value at the top of every iteration and
	// the loop could never terminate on a condition that watches the
	// counter.
	//
	// "First time" is detected by checking whether the loop's state
	// bucket already holds the variable: a missing bucket entry
	// (GetVar returns nil with no error) means the loop has not yet
	// seeded; any non-nil value means the body already wrote it on
	// a prior iteration. This is safe even for "zero-init" loop
	// variables (number→0, string→"") because Go's typed zero
	// values are non-nil when stored back through SetVar.
	//
	// input_mode dispatch (per agent/component/loop.py:60-77):
	//   "constant"  → use the literal value from the DSL
	//   "variable"  → dereference the value as a state ref via
	//                 state.GetVar; store the resolved value
	//                 (or nil if the ref is unresolvable — mirrors
	//                 Python's "treat as literal" fallback)
	//   "" (zero)   → use the type-derived zero value (resolved at
	//                 build time by resolveLoopVarValue)
	initNode := sub.AddLambdaNode(loopInitKey,
		compose.InvokableLambda(func(ctx context.Context, in map[string]any) (map[string]any, error) {
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
		}),
	)
	nodes[loopInitKey] = initNode

	// Body nodes: each member becomes a real factory-built (or
	// placeholder, when no factory is registered) component invoke
	// wrapped by withStateBracket so it shares the same state
	// snapshot / result-persistence contract as outer-graph nodes.
	// We do NOT use eino's StatePreHandler / StatePostHandler here
	// because the sub-workflow has no WithGenLocalState of its own:
	// state flows in through ctx (runtime.WithState) attached by
	// the caller, and is read back via runtime.GetStateFromContext
	// inside withStateBracket. This is what lets a Loop body
	// actually mutate CanvasState (e.g. VariableAssigner
	// incrementing the loop counter) so the LoopCondition closure
	// can observe the change on the next iteration.
	for cpnID := range members {
		name := c.Components[cpnID].Obj.ComponentName
		if name == "" {
			return nil, fmt.Errorf("canvas: loop %q member %q has empty component_name", loopID, cpnID)
		}
		body, err := buildNodeBody(cpnID, name, c.Components[cpnID].Obj.Params)
		if err != nil {
			return nil, err
		}
		nodes[cpnID] = sub.AddLambdaNode(cpnID,
			compose.InvokableLambda[map[string]any, map[string]any](withStateBracket(body)),
			compose.WithNodeName(cpnID),
		)
	}

	// Wire edges. The synthetic init node connects to every body node
	// that has no upstream within the sub-graph (the body's "entry"
	// nodes). For diamond / merge topologies within the body, we use
	// the same eino one-data-input rule as BuildWorkflow: the first
	// upstream carries data, the rest are exec-only AddDependency.
	for cpnID := range members {
		upstreams := c.Components[cpnID].Upstream
		first := true
		for _, up := range upstreams {
			if up == loopID {
				// Upstream is the parent Loop; in the sub-graph the
				// data source is the synthetic init node.
				if first {
					nodes[cpnID].AddInput(loopInitKey)
					first = false
				} else {
					nodes[cpnID].AddDependency(loopInitKey)
				}
				continue
			}
			if !members[up] {
				continue
			}
			if first {
				nodes[cpnID].AddInput(up)
				first = false
			} else {
				nodes[cpnID].AddDependency(up)
			}
		}
		if first {
			// No in-subgraph upstream: wire from init (this happens
			// for body entries whose only upstream in the DSL is the
			// Loop itself).
			nodes[cpnID].AddInput(loopInitKey)
		}
	}

	// Loop body sub-graphs need the same runtime MultiBranch wiring as the
	// outer workflow, otherwise in-body Switch/Categorize nodes fan out to
	// every declared child and loop exit/continue semantics diverge.
	wireMultiBranches(sub, subCanvasForMembers(c, members), nil)

	// Wire END: every member that has no downstream within the
	// sub-graph is a sub-graph terminal. Multi-terminal loop bodies
	// need the same merge-node treatment as outer workflows; otherwise
	// eino's END node rejects repeated output mappings during compile.
	hasDownstream := make(map[string]bool, len(members))
	for cpnID := range members {
		for _, down := range c.Components[cpnID].Downstream {
			if members[down] {
				hasDownstream[cpnID] = true
				break
			}
		}
	}
	terminals := make([]string, 0, len(members))
	for cpnID := range members {
		if hasDownstream[cpnID] {
			continue
		}
		if isLegacyNoOp(c.Components[cpnID].Obj.ComponentName) {
			if strings.EqualFold(c.Components[cpnID].Obj.ComponentName, "ExitLoop") {
				terminals = append(terminals, cpnID)
			}
			continue
		}
		terminals = append(terminals, cpnID)
	}
	if err := wireWorkflowTerminals(sub, terminals, loopInitKey, false); err != nil {
		return nil, err
	}

	// Wire START. The synthetic init node is the sub-workflow's
	// entry; eino's Workflow requires every start node to be wired
	// from compose.START explicitly. The init node takes the
	// sub-workflow's input (the per-iteration `prev`) and seeds the
	// loop variables into state.
	initNode.AddInput(compose.START)

	return sub, nil
}

func subCanvasForMembers(c *Canvas, members map[string]bool) *Canvas {
	if c == nil {
		return nil
	}
	sub := &Canvas{
		Components: make(map[string]CanvasComponent, len(members)),
	}
	for id := range members {
		comp, ok := c.Components[id]
		if !ok {
			continue
		}
		sub.Components[id] = comp
	}
	return sub
}

// loopInitKey is the synthetic cpn_id used for the LoopInit entry node
// inside the sub-workflow. Using a reserved key avoids collisions with
// user-defined cpn_ids.
const loopInitKey = "__loop_init__"

// initVarSpec carries the per-variable info the init lambda needs to
// decide how to seed the loop variable into the per-run state.
//
// For input_mode == "variable", Value is the ref string to dereference
// at init time via state.GetVar; for "constant", Value is used as-is;
// for "" (zero-init), Value is the type-derived zero (resolved at build
// time by resolveLoopVarValue) and the init lambda stores it directly.
type initVarSpec struct {
	Value     any
	InputMode string
}

// resolveInitialVariables applies the input_mode dispatch from
// agent/component/loop.py:60-77 to a list of loop_variable entries.
//
//	input_mode == "variable"  → returns the ref string in Value
//	                            (the init lambda dereferences it at
//	                            runtime via state.GetVar; resolution
//	                            is deferred because this helper is
//	                            state-free).
//	input_mode == "constant"  → Value is the literal value.
//	otherwise (zero-init)     → Value is the type-based zero value.
//
// The init lambda (buildSubWorkflow) iterates the returned map and
// writes each Value into the per-run state under
// `state.Outputs[loopID][name]`. The "variable" dereference happens
// there, in the lambda body, where the live CanvasState is available.
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
		// The "variable" path is handled at init time inside
		// buildSubWorkflow's init lambda, where the state is
		// available. Here we just return the ref string.
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

// translateLoopCondition converts the DSL's loop_termination_condition
// list into a workflowx.LoopCondition[map[string]any] closure.
//
// The closure reads each condition's variable via
// `state.GetVar(loopID + "." + variable)` on every iteration, applies
// the operator, and combines results via the configured logical
// operator ("and" by default, "or" otherwise).
//
// The closure's per-iteration cost is one state lookup per condition —
// no allocations once the conditions slice is captured.
func translateLoopCondition(loopID string, params map[string]any) (workflowx.LoopCondition[map[string]any], error) {
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

	return func(ctx context.Context, _ int, _, _ map[string]any) (bool, error) {
		// The condition is evaluated at the end of each iteration.
		// We need access to the per-run state to read loop variables
		// and other DSL variables. The workflowx lambda passes the
		// loop's outer context into this closure, so
		// canvas.GetStateFromContext works.
		state, _, err := GetStateFromContext[*CanvasState](ctx)
		if err != nil || state == nil {
			return false, fmt.Errorf("loop %q: condition eval: no canvas state in context", loopID)
		}
		if len(conditions) == 0 {
			// No conditions means the loop only stops at max count
			// — never quit on conditions. Mirrors Python fallback.
			return false, nil
		}
		// Vacuous starting value: true for AND, false for OR.
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

type loopConditionSpec struct {
	Variable  string
	Operator  string
	Value     any
	InputMode string // "constant" or "variable"
}

// evalOneLoopCondition resolves a single condition entry. Mirrors
// loopitem.py:128-142. Variable lookup is by full cpn_id path
// ("loopID.varName" for loop variables, or whatever ref the DSL
// supplies for state-level refs).
func evalOneLoopCondition(state *CanvasState, loopID string, spec loopConditionSpec) (bool, error) {
	// Resolve the right-hand side value.
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
	// Resolve the variable being tested. The DSL stores either a bare
	// variable name (loop variable) or a full cpn_id@param ref. For
	// loop variables written by the init lambda, the bucket key is
	// "loopID" so the ref is "loopID@name". For arbitrary state refs,
	// the DSL passes the full path.
	ref := spec.Variable
	if !strings.Contains(ref, ".") && !strings.Contains(ref, "@") {
		// Bare name — assume it's a loop variable.
		ref = loopID + "@" + ref
	}
	got, err := state.GetVar(ref)
	if err != nil {
		return false, fmt.Errorf("loop %q: condition lhs ref %q: %w", loopID, ref, err)
	}
	return evaluateCondition(got, spec.Operator, rhs)
}

// evaluateCondition is the type-dispatched operator logic that mirrors
// loopitem.py:48-122. The operator set is the union of operators used
// across all type branches — at runtime only the branches matching
// the dynamic type of `var` are reachable.
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
		// mirrors `var is None` for booleans
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
// Loop. 0 means "infinite" (no cap, only condition-driven termination).
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
