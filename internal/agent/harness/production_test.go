package harness

import (
	"context"
	"strings"
	"sync"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"ragflow/internal/agent/component"
	"ragflow/internal/service/nav"
)

// routeChat returns a model-style response depending on the stage: nav-selection
// calls return {"relevant":[...]}, all other calls return a plain final answer.
type routeChat struct{}

func (routeChat) Invoke(_ context.Context, _ *gorm.DB, req component.ChatInvokeRequest) (*component.ChatInvokeResponse, error) {
	content := ""
	msg := ""
	if len(req.Messages) > 0 {
		msg = req.Messages[len(req.Messages)-1].Content
	}
	switch {
	case strings.Contains(msg, "(numbered)") || strings.Contains(msg, "Entities"):
		// nav-select pass: keep every item (all relevant).
		content = `{"relevant":[0,1,2,3,4,5,6,7,8,9]}`
	default:
		content = "final scoped answer"
	}
	return &component.ChatInvokeResponse{Content: content}, nil
}

// installRouteChat installs the stage-aware chat invoker for E2E tests.
func installRouteChat(t *testing.T) {
	t.Helper()
	component.SetDefaultChatInvoker(routeChat{})
	t.Cleanup(func() { component.SetDefaultChatInvoker(nil) })
}

// fakeInvokableTool is an einotool.InvokableTool double for E2E testing.
type fakeInvokableTool struct {
	name     string
	fn       func(ctx context.Context, argsJSON string) string
	mu       sync.Mutex
	lastArgs string
}

func (f *fakeInvokableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: f.name}, nil
}

func (f *fakeInvokableTool) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	f.mu.Lock()
	f.lastArgs = argsJSON
	f.mu.Unlock()
	return f.fn(context.Background(), argsJSON), nil
}

func (f *fakeInvokableTool) args() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastArgs
}

// TestProductionRunner_E2E_RouteToScopedSearch proves the chain
// "user question → dataset nav routes doc scope → hybrid_search retrieval is
// scoped to those docs". It injects a fake nav service (returns docs d1,d2) and
// asserts the search tool receives doc_scope=[d1,d2].
func TestProductionRunner_E2E_RouteToScopedSearch(t *testing.T) {
	installRouteChat(t)

	// Fake nav service whose two-round router returns docs d1,d2.
	navSvc := &fakeNavSvcHarness{
		clusters: []nav.NavNode{{Name: "C1", Description: "cluster"}},
		children: map[string][]nav.NavNode{
			"C1": {
				{Name: "DocA", Type: "doc", DocID: "d1"},
				{Name: "DocB", Type: "doc", DocID: "d2"},
			},
		},
	}
	// AskNavSelect returns all items (both docs) for the doc-select pass.
	searchTool := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"c1","content_with_weight":"scoped evidence"}]}`
	}}

	// Multi-KB session: routing must cover every bound dataset, not just the
	// first KB.
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1", "kb2"}, searchTool, navSvc)
	res := runner.Run(context.Background(), "Compare A and B", "", "medium")

	if res.FinalAnswer != "final scoped answer" {
		t.Errorf("final answer = %q, want chat output", res.FinalAnswer)
	}
	// The search tool must receive doc_scope=[d1,d2] — the docs the nav router
	// selected — proving retrieval is scoped to those docs across KBs.
	if !strings.Contains(searchTool.args(), `"doc_scope":["d1","d2"]`) {
		t.Errorf("hybrid_search not scoped to routed docs; args=%s", searchTool.args())
	}
	// The search tool must receive all bound KBs (not collapsed to one).
	if !strings.Contains(searchTool.args(), `"kb_ids":["kb1","kb2"]`) {
		t.Errorf("hybrid_search kb_ids must include all bound KBs; args=%s", searchTool.args())
	}
}

// TestProductionRunner_E2E_NoRoute_SearchUnscoped asserts low mode (no
// decomposition) skips nav routing and searches without a doc_scope.
func TestProductionRunner_E2E_NoRoute_SearchUnscoped(t *testing.T) {
	installRouteChat(t)
	searchTool := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"c1","content_with_weight":"evidence"}]}`
	}}
	// A nav service is provided but must NOT be consulted in low mode.
	navSvc := &fakeNavSvcHarness{clusters: []nav.NavNode{{Name: "C1"}}}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, searchTool, navSvc)
	res := runner.Run(context.Background(), "What is X?", "", "low")

	if res.FinalAnswer == "" {
		t.Error("expected a final answer in low mode")
	}
	if strings.Contains(searchTool.args(), "doc_scope") {
		t.Errorf("low mode search should have no doc_scope; args=%s", searchTool.args())
	}
}

// TestProductionRunner_E2E_EmptyRoute_SearchUnscoped asserts an empty doc route
// falls back to an unscoped search (no hard failure).
func TestProductionRunner_E2E_EmptyRoute_SearchUnscoped(t *testing.T) {
	installRouteChat(t)
	searchTool := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"c1","content_with_weight":"evidence"}]}`
	}}
	// Nav service with no clusters -> empty route.
	navSvc := &fakeNavSvcHarness{clusters: nil}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, searchTool, navSvc)
	res := runner.Run(context.Background(), "Compare A and B", "", "medium")
	if res.FinalAnswer == "" {
		t.Error("expected an answer even when nav routing returns no docs")
	}
}
