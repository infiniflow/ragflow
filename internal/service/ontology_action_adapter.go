package service

import (
	"context"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ontology/contract"
)

// OntologyActionAdapter exposes the action layer to the agent.
//
// The agent cannot import this package (the chat pipeline imports the agent), so
// the dependency is inverted here, exactly like the ontology retrieval adapter:
// the agent declares what it needs, the service implements it at boot.
//
// Everything the adapter returns is already judged. The agent receives a verdict
// and its reasons, never the inputs to re-derive it — a prompt can be argued out
// of a rule, a returned verdict cannot.
type OntologyActionAdapter struct {
	svc *ActionBindingService
}

// NewOntologyActionAdapter creates the adapter.
func NewOntologyActionAdapter(svc *ActionBindingService) *OntologyActionAdapter {
	if svc == nil {
		svc = NewActionBindingService()
	}
	return &OntologyActionAdapter{svc: svc}
}

// tenantFor resolves the tenant an agent turn acts in.
//
// A turn carries the user, not the tenant, and the two coincide only for a user
// whose own id is their tenant (the default for a personal account). Falling back
// to the user id is therefore the same default the REST handlers use, kept in one
// place so the two paths cannot scope differently.
func tenantFor(userID string) string {
	return userID
}

// ActionContract reads and judges one action.
func (a *OntologyActionAdapter) ActionContract(ctx context.Context, req agentic_rag.ActionContractRequest) (*agentic_rag.ActionContractResult, error) {
	tenantID := tenantFor(req.UserID)
	template, err := a.svc.templateDAO.GetTemplate(ctx, dao.DB, tenantID, req.ModelID)
	if err != nil {
		return nil, err
	}
	datasets := req.DatasetIDs
	if len(datasets) == 0 {
		if _, _, scope, err := reviewScopeDatasets(ctx, dao.DB, tenantID, template, nil); err == nil {
			datasets = scope
		}
	}
	action, err := LoadCompiledAction(ctx, dao.DB, tenantID, req.ModelID, datasets, req.ActionName)
	if err != nil {
		return nil, err
	}
	if action == nil {
		return nil, nil
	}
	binding, err := a.svc.bindingDAO.FindByAction(ctx, dao.DB, tenantID, req.ModelID, req.ActionName)
	if err != nil {
		return nil, err
	}
	toolAvailable := false
	if binding != nil {
		if tools, err := a.svc.serverTools(ctx, dao.DB, tenantID, binding.MCPServer); err == nil {
			_, toolAvailable = tools[binding.MCPTool]
		}
	}
	gate := contract.ActionGateInput{
		ActionExists:      true,
		TemplatePublished: template.Version > 0,
		Bound:             binding != nil,
		BindingEnabled:    binding != nil && binding.Enabled,
		ToolAvailable:     toolAvailable,
		QACallable:        binding != nil && binding.QACallable,
		ReadOnly:          binding != nil && binding.ReadOnly,
		ExecEnabled:       ActionExecEnabled(),
		Contract:          action.Contract,
	}
	verdict := contract.EvaluateActionGates(gate)
	view := action.Contract.View()
	fields := make(map[string]agentic_rag.ActionContractField, len(view.Fields))
	for name, field := range view.Fields {
		fields[name] = agentic_rag.ActionContractField{
			Status:     string(field.Status),
			Value:      field.Value,
			Quote:      field.Quote,
			Provenance: string(field.Provenance),
		}
	}
	result := &agentic_rag.ActionContractResult{
		ActionName: action.Name,
		Verdict:    string(verdict.Verdict),
		Reasons:    verdict.Reasons,
		Fields:     fields,
		Missing:    view.Missing,
		Required:   view.Required,
		Declared:   view.Declared,
		Bound:      binding != nil,
	}
	if binding != nil {
		result.BindingEnabled = binding.Enabled
		result.ReadOnly = binding.ReadOnly
	}
	return result, nil
}

// InvokeAction runs the action through the query-mode gates.
//
// A refusal is returned as a result rather than an error: "it cannot run, here is
// why" is a fact about the model the agent must be able to relay. Only a genuine
// failure (engine down, audit write impossible) becomes an error.
func (a *OntologyActionAdapter) InvokeAction(ctx context.Context, req agentic_rag.ActionInvokeRequest) (*agentic_rag.ActionInvokeResult, error) {
	result, err := a.svc.RunAction(ctx, dao.DB, ActionCallInput{
		TenantID:   tenantFor(req.UserID),
		TemplateID: req.ModelID,
		ActionName: req.ActionName,
		Mode:       entity.ActionExecutionModeQuery,
		OperatorID: req.UserID,
		Arguments:  req.Arguments,
	})
	if result == nil {
		return nil, err
	}
	out := &agentic_rag.ActionInvokeResult{
		Verdict:    result.Verdict,
		Reasons:    result.Reasons,
		Status:     result.Status,
		Output:     result.Output,
		Error:      result.Error,
		DurationMS: result.DurationMS,
		Executed:   result.Status == entity.ActionExecutionStatusSucceeded,
	}
	return out, nil
}

var _ agentic_rag.ActionService = (*OntologyActionAdapter)(nil)
