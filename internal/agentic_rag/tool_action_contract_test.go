package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// fakeActionService stands in for the server-side action layer: it records what
// the tools asked and answers with whatever verdict the test wants to prove the
// rendering of.
type fakeActionService struct {
	contractReq ActionContractRequest
	invokeReq   ActionInvokeRequest
	contract    *ActionContractResult
	invoke      *ActionInvokeResult
	err         error
}

func (f *fakeActionService) ActionContract(_ context.Context, req ActionContractRequest) (*ActionContractResult, error) {
	f.contractReq = req
	return f.contract, f.err
}

func (f *fakeActionService) InvokeAction(_ context.Context, req ActionInvokeRequest) (*ActionInvokeResult, error) {
	f.invokeReq = req
	return f.invoke, f.err
}

// actionTestCtx is a chat turn with a knowledge layer bound, which is the state
// the action tools require.
func actionTestCtx() context.Context {
	return WithOntologyBinding(context.Background(), ontologyBinding{
		UserID:   "u-1",
		TenantID: "u-1",
		Config: OntologyRunConfig{
			ModelID:             "model-1",
			KnowledgeDatasetIDs: []string{"kb-1"},
		},
	})
}

func toolNames(t *testing.T, tools []tool.BaseTool) map[string]bool {
	t.Helper()
	out := make(map[string]bool, len(tools))
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		out[info.Name] = true
	}
	return out
}

// The action tools are part of the retrieval turn's tool set, and whether the
// model may execute is a deployment decision: reading a contract costs one query,
// offering a tool that can only ever answer "refused" invites a wasted round.
func TestActionToolsAreOfferedToTheRetrievalLoop(t *testing.T) {
	ctx := actionTestCtx()

	t.Setenv("RAGFLOW_ONTOLOGY_ACTION_EXEC", "0")
	names := toolNames(t, ontologyToolsFor(ctx, nil, ontologyModeGuide))
	if !names[actionContractToolName] {
		t.Fatalf("get_action_contract must be offered whenever a knowledge layer is bound, got %v", names)
	}
	if names[invokeActionToolName] {
		t.Fatalf("invoke_action must not be offered while execution is off, got %v", names)
	}

	t.Setenv("RAGFLOW_ONTOLOGY_ACTION_EXEC", "true")
	names = toolNames(t, ontologyToolsFor(ctx, nil, ontologyModeGuide))
	if !names[invokeActionToolName] {
		t.Fatalf("invoke_action must be offered once execution is on, got %v", names)
	}

	// The knowledge-only mode drops the corpus tools; the action tools stay
	// because a contract is knowledge, not a corpus search.
	names = toolNames(t, ontologyToolsFor(ctx, nil, ontologyModeDirect))
	if !names[actionContractToolName] || !names[invokeActionToolName] {
		t.Fatalf("a knowledge-only turn still needs the action tools, got %v", names)
	}

	// No knowledge layer, no action tools: an action without a model has no
	// contract to read and belongs to nobody's binding.
	if names := toolNames(t, ontologyToolsFor(ctx, nil, ontologyModeOff)); len(names) != 0 {
		t.Fatalf("an unbound turn must not carry action tools, got %v", names)
	}
}

// The contract tool must ask about the run's own model and scope — not a model
// the model itself names — and render the verdict and provenance the server sent.
func TestActionContractToolPassesTheRunScopeAndRendersTheVerdict(t *testing.T) {
	svc := &fakeActionService{contract: &ActionContractResult{
		ActionName: "refund_order",
		Verdict:    "confirm",
		Reasons:    []string{"authorization_present"},
		Fields: map[string]ActionContractField{
			"authorization": {
				Status:     "present",
				Value:      "manager",
				Quote:      "requires manager approval",
				Provenance: "document",
			},
			"preconditions": {Status: "missing"},
		},
		Missing:        []string{"preconditions"},
		Required:       actionContractFieldOrder,
		Declared:       1,
		Bound:          true,
		BindingEnabled: true,
	}}
	SetActionService(svc)
	t.Cleanup(func() { SetActionService(nil) })

	out, err := NewActionContractTool().InvokableRun(actionTestCtx(), `{"action":"refund_order"}`)
	if err != nil {
		t.Fatalf("contract tool failed: %v", err)
	}
	if svc.contractReq.ModelID != "model-1" || svc.contractReq.UserID != "u-1" ||
		svc.contractReq.ActionName != "refund_order" ||
		len(svc.contractReq.DatasetIDs) != 1 || svc.contractReq.DatasetIDs[0] != "kb-1" {
		t.Fatalf("request lost the run scope: %#v", svc.contractReq)
	}
	for _, want := range []string{
		`verdict="confirm"`,
		`bound="true"`,
		`declared="1/6"`,
		`provenance="document"`,
		`<quote>requires manager approval</quote>`,
		`<verdict_reasons>authorization_present</verdict_reasons>`,
		"a human must approve before it runs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("contract output is missing %q:\n%s", want, out)
		}
	}
	// A missing field must not carry a value: an empty <value> is exactly what
	// the model would quote as "the document says nothing is required".
	if strings.Contains(out, `<field name="preconditions" status="missing" provenance=""><value>`) {
		t.Fatalf("a missing field rendered a value:\n%s", out)
	}
}

// A refusal is an answer about the world, so it reaches the model as content it
// can relay — not as a tool error the loop would retry.
func TestInvokeActionToolReturnsRefusalAsModelReadableContent(t *testing.T) {
	svc := &fakeActionService{invoke: &ActionInvokeResult{
		Verdict: "confirm",
		Reasons: []string{"authorization_present"},
		Status:  "refused",
	}}
	SetActionService(svc)
	t.Cleanup(func() { SetActionService(nil) })

	out, err := NewInvokeActionTool().InvokableRun(actionTestCtx(),
		`{"action":"refund_order","arguments":{"order_id":"A-1"}}`)
	if err != nil {
		t.Fatalf("a refusal must not surface as a tool error: %v", err)
	}
	if svc.invokeReq.Arguments["order_id"] != "A-1" {
		t.Fatalf("operator-supplied arguments were dropped: %#v", svc.invokeReq)
	}
	for _, want := range []string{
		`verdict="confirm"`,
		`executed="false"`,
		`<reasons>authorization_present</reasons>`,
		"<notice>nothing ran. Report this outcome as it is.</notice>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("invocation output is missing %q:\n%s", want, out)
		}
	}
}

// Without a bound knowledge layer there is no model to read a contract from, and
// the tool says so instead of failing the turn.
func TestActionToolsExplainAMissingBinding(t *testing.T) {
	out, err := NewActionContractTool().InvokableRun(context.Background(), `{"action":"refund_order"}`)
	if err != nil {
		t.Fatalf("unbound contract tool failed: %v", err)
	}
	if !strings.Contains(out, "no_ontology_bound") {
		t.Fatalf("expected the no_ontology_bound notice, got:\n%s", out)
	}
}
