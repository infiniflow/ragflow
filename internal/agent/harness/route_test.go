package harness

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/agent/component"
)

// fakeChatInvoker returns a fixed content for the chat call, so tests can drive
// the route/planner LLM output deterministically.
type fakeChatInvoker struct{ content string }

func (f *fakeChatInvoker) Invoke(_ context.Context, _ *gorm.DB, _ component.ChatInvokeRequest) (*component.ChatInvokeResponse, error) {
	return &component.ChatInvokeResponse{Content: f.content}, nil
}

func installChat(t *testing.T, content string) {
	t.Helper()
	component.SetDefaultChatInvoker(&fakeChatInvoker{content: content})
	t.Cleanup(func() { component.SetDefaultChatInvoker(nil) })
}

// TestRouteNode_Classifies asserts route classification drives the execution
// strategy from the mode and the LLM's question_type.
func TestRouteNode_Classifies(t *testing.T) {
	installChat(t, `{"question_type":"comparative","requires_decomposition":true,"reasoning":"cmp"}`)
	r := RouteNode(t.Context(), nil, "Compare A and B", "medium")
	if r.QuestionType != "comparative" {
		t.Errorf("question_type = %q, want comparative", r.QuestionType)
	}
	// medium mode requires decomposition AND LLM says true -> true.
	if !r.RequiresDecomposition {
		t.Errorf("requires_decomposition = false, want true")
	}
	if r.ExecutionStrategy != "decompose_and_search" {
		t.Errorf("execution_strategy = %q, want decompose_and_search", r.ExecutionStrategy)
	}
}

// TestRouteNode_LowModeDisablesDecomposition asserts low mode never decomposes
// even if the LLM requests it.
func TestRouteNode_LowModeDisablesDecomposition(t *testing.T) {
	installChat(t, `{"question_type":"analytical","requires_decomposition":true}`)
	r := RouteNode(t.Context(), nil, "Analyze X", "low")
	if r.RequiresDecomposition {
		t.Errorf("low mode must disable decomposition, got true")
	}
	if r.ExecutionStrategy != "direct_search" {
		t.Errorf("low mode execution_strategy = %q, want direct_search", r.ExecutionStrategy)
	}
}

// TestRouteNode_FencedJSON asserts think-tag/fence stripping works.
func TestRouteNode_FencedJSON(t *testing.T) {
	installChat(t, "Sure!\n```json\n{\"question_type\":\"factual\",\"requires_decomposition\":false}\n```")
	r := RouteNode(t.Context(), nil, "What is X?", "medium")
	if r.QuestionType != "factual" {
		t.Errorf("question_type = %q, want factual", r.QuestionType)
	}
	if r.RequiresDecomposition {
		t.Errorf("requires_decomposition = true, want false")
	}
}

// TestRouteNode_EmptyQuestionFallsBack asserts an empty question yields a
// direct factual decision without calling the LLM.
func TestRouteNode_EmptyQuestionFallsBack(t *testing.T) {
	r := RouteNode(t.Context(), nil, "", "medium")
	if r.QuestionType != "factual" || r.RequiresDecomposition {
		t.Errorf("empty question must fall back to direct factual, got %+v", r)
	}
}

// TestRouteNode_SuggestsCompilation asserts the route preserves a normalized
// compiled-artifact suggestion (P5) so the production runner can prefer the wiki
// tool.
func TestRouteNode_SuggestsCompilation(t *testing.T) {
	installChat(t, `{"question_type":"analytical","requires_decomposition":true,"suggests_compilation":"wiki"}`)
	r := RouteNode(t.Context(), nil, "What does the domain say about X?", "medium")
	if r.SuggestsCompilation != "wiki" {
		t.Errorf("suggests_compilation = %q, want wiki", r.SuggestsCompilation)
	}
}

// TestNormalizeCompilationSuggestion asserts free-text suggestions map to the
// canonical keys and unknown/null collapse to "".
func TestNormalizeCompilationSuggestion(t *testing.T) {
	cases := map[string]string{
		"wiki":      "wiki",
		"WIKI":      "wiki",
		"compiled":  "wiki",
		"graph":     "graph",
		"kg":        "graph",
		"toc":       "toc",
		"null":      "",
		"":          "",
		"spaghetti": "",
	}
	for in, want := range cases {
		if got := normalizeCompilationSuggestion(in); got != want {
			t.Errorf("normalizeCompilationSuggestion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRouteNode_WikiSuggestionSurvivesFence asserts a fenced JSON route still
// carries the wiki suggestion through decide().
func TestRouteNode_WikiSuggestionSurvivesFence(t *testing.T) {
	installChat(t, "```json\n{\"question_type\":\"procedural\",\"requires_decomposition\":false,\"suggests_compilation\":\"graph\"}\n```")
	r := RouteNode(t.Context(), nil, "How is this structured?", "medium")
	if r.SuggestsCompilation != "graph" {
		t.Errorf("suggests_compilation = %q, want graph", r.SuggestsCompilation)
	}
}

// TestPlannerNode_DirectMode asserts a non-decomposed route yields one coarse
// claim without calling the LLM.
func TestPlannerNode_DirectMode(t *testing.T) {
	plan := PlannerNode(t.Context(), nil, RouteDecision{
		Question: "What is X?", RequiresDecomposition: false,
	}, nil)
	if plan.PlanType != "direct" || len(plan.Claims) != 1 {
		t.Fatalf("direct plan = %+v, want 1 direct claim", plan)
	}
}

// TestPlannerNode_Decomposes asserts the planner builds claims from the LLM
// output and applies the mode's max iterations.
func TestPlannerNode_Decomposes(t *testing.T) {
	installChat(t, `{"claims":[
		{"claim_id":"c0","description":"fact one","priority":0},
		{"claim_id":"c1","description":"fact two","priority":1}
	]}`)
	plan := PlannerNode(t.Context(), nil, RouteDecision{
		Question: "Compare A and B", QuestionType: "comparative", RequiresDecomposition: true, ThinkingMode: "medium",
	}, nil)
	if plan.PlanType != "comparative_decomposition" {
		t.Errorf("plan_type = %q, want comparative_decomposition", plan.PlanType)
	}
	if len(plan.Claims) != 2 {
		t.Fatalf("claims = %d, want 2", len(plan.Claims))
	}
	if plan.Claims[1].Priority != 1 {
		t.Errorf("claim priority = %d, want 1", plan.Claims[1].Priority)
	}
	// medium maxOrchestratorCycles = 3
	if plan.MaxIterations != 3 {
		t.Errorf("max_iterations = %d, want 3", plan.MaxIterations)
	}
}

// TestPlannerNode_UnknownModeFallsBack asserts an unknown (non-empty) mode label
// falls back to medium so the planner is not driven by a zero-valued mode
// (which would produce a degenerate plan with max_claims=0).
func TestPlannerNode_UnknownModeFallsBack(t *testing.T) {
	installChat(t, `{"claims":[{"claim_id":"c0","description":"fact one","priority":0}]}`)
	plan := PlannerNode(t.Context(), nil, RouteDecision{
		Question: "Q", RequiresDecomposition: true, ThinkingMode: "turbo-unknown",
	}, nil)
	// medium maxOrchestratorCycles = 3, and claims must still be built.
	if plan.MaxIterations != 3 {
		t.Errorf("max_iterations = %d, want 3 (medium fallback)", plan.MaxIterations)
	}
	if len(plan.Claims) != 1 {
		t.Errorf("claims = %d, want 1", len(plan.Claims))
	}
}

// TestPlannerNode_BadJSONFallsBack asserts unparseable planner output falls back
// to the direct plan.
func TestPlannerNode_BadJSONFallsBack(t *testing.T) {
	installChat(t, "not json at all")
	plan := PlannerNode(t.Context(), nil, RouteDecision{
		Question: "Q", RequiresDecomposition: true, ThinkingMode: "medium",
	}, nil)
	if plan.PlanType != "direct" || len(plan.Claims) != 1 {
		t.Fatalf("fallback plan = %+v, want direct", plan)
	}
}

// TestUnmarshalModelJSON pins unmarshalModelJSON: think preamble and Markdown
// fences are stripped before JSON parsing, and an empty payload yields {}.
func TestUnmarshalModelJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]any
	}{
		{name: "plain json", in: `{"answer":"42"}`, want: map[string]any{"answer": "42"}},
		{name: "think prefix", in: `<think>reason</think>{"answer":"42"}`, want: map[string]any{"answer": "42"}},
		{name: "json fence", in: "```json\n{\"answer\":\"42\"}\n```", want: map[string]any{"answer": "42"}},
		{name: "think then fence", in: "<think>r</think>```json\n{\"answer\":\"42\"}\n```", want: map[string]any{"answer": "42"}},
		{name: "empty becomes empty object", in: "", want: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out map[string]any
			if err := unmarshalModelJSON(tt.in, &out); err != nil {
				t.Fatalf("unmarshalModelJSON(%q): %v", tt.in, err)
			}
			if !reflect.DeepEqual(out, tt.want) {
				t.Errorf("unmarshalModelJSON(%q) = %#v, want %#v", tt.in, out, tt.want)
			}
		})
	}
}
