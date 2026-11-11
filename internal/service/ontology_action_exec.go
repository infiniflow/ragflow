package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	mcpclient "ragflow/internal/mcp/client"
	"ragflow/internal/ontology/contract"
	"ragflow/internal/server"

	"gorm.io/gorm"
)

// Action execution: from "the action exists" to "the tool ran".
//
// The design's four gates are all evaluated server-side, in one place, and the
// verdict travels with the call. The agent is never told how to decide — it is
// told the decision. That asymmetry is the whole safety model: a prompt can be
// talked out of a rule, a returned verdict cannot.

// compiledAction is an action row as the executor and the agent tools need it.
type compiledAction struct {
	ID             string
	Name           string
	Description    string
	Type           string
	DatasetID      string
	Contract       contract.ActionContract
	Attributes     map[string]any
	SourceChunkIDs []string
}

// LoadCompiledAction resolves one action by name inside a model's scope.
//
// Resolution is by name, not by row id: the id changes on every recompile (it is
// a hash of the row content), while the name is what an operator bound and what a
// question refers to.
func LoadCompiledAction(ctx context.Context, db *gorm.DB, tenantID, templateID string, datasetIDs []string, actionName string) (*compiledAction, error) {
	actionName = strings.TrimSpace(actionName)
	if actionName == "" {
		return nil, nil
	}
	rows, _, err := reviewSearch(ctx, tenantID, datasetIDs,
		[]string{"id", "kb_id", "name_kwd", "entity_type_kwd", "contract", "attr", "content_with_weight", "source_chunk_ids"},
		map[string]interface{}{
			"name_kwd":                 []string{actionName},
			"compilation_template_ids": []string{templateID},
			"knowledge_graph_kwd":      []string{"entity"},
			"entity_type_kwd":          []string{contract.ActionEntityType},
		}, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	payload := graphLoadPayload(row)
	action := &compiledAction{
		ID:             firstStringValue(row["id"]),
		Name:           firstStringValue(row["name_kwd"]),
		Type:           contract.ActionEntityType,
		DatasetID:      firstStringValue(row["kb_id"]),
		Contract:       contract.ParseActionContract(row["contract"]),
		Attributes:     ontologyAttributesOf(row["attr"]),
		SourceChunkIDs: stringSliceValue(row["source_chunk_ids"]),
	}
	if payload != nil {
		action.Description = strings.TrimSpace(firstStringValue(payload["description"]))
		if action.Name == "" {
			action.Name = strings.TrimSpace(firstStringValue(payload["name"]))
		}
	}
	return action, nil
}

// ontologyAttributesOf reads the folded datatype attributes of a row.
func ontologyAttributesOf(raw any) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		var out map[string]any
		if json.Unmarshal([]byte(v), &out) == nil {
			return out
		}
	}
	return nil
}

// ModelActionView is one compiled action as the workbench lists it.
type ModelActionView struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	DatasetID   string                      `json:"dataset_id"`
	Description string                      `json:"description,omitempty"`
	Contract    contract.ActionContractView `json:"contract"`
	// Verdict is computed with the binding's state, so the list answers "what can
	// run now" without a second call per row.
	Verdict        string             `json:"verdict"`
	VerdictReasons []string           `json:"verdict_reasons,omitempty"`
	Binding        *ActionBindingView `json:"binding,omitempty"`
	// ReviewStatus is the row's review state: an unreviewed action's contract is
	// still a proposal, which the workbench shows before an operator wires it.
	ReviewStatus string `json:"review_status,omitempty"`
}

// ListModelActions returns the model's compiled actions with their contract,
// binding and current verdict.
//
// One call, not four: the workbench renders contract, binding and verdict in the
// same row, and fetching them separately would let the three disagree on screen.
func (s *ActionBindingService) ListModelActions(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]ModelActionView, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, tenantID, template, nil)
	if err != nil {
		return nil, err
	}
	rows, _, err := reviewSearch(ctx, tenantID, datasetIDs,
		[]string{"id", "kb_id", "name_kwd", "contract", "content_with_weight", "review_status_kwd", "attr"},
		map[string]interface{}{
			"compilation_template_ids": []string{templateID},
			"knowledge_graph_kwd":      []string{"entity"},
			"entity_type_kwd":          []string{contract.ActionEntityType},
		}, nil, 0, 500)
	if err != nil {
		return nil, err
	}
	out := make([]ModelActionView, 0, len(rows))
	for _, row := range rows {
		action := &compiledAction{
			ID:        firstStringValue(row["id"]),
			Name:      firstStringValue(row["name_kwd"]),
			DatasetID: firstStringValue(row["kb_id"]),
			Contract:  contract.ParseActionContract(row["contract"]),
		}
		if payload := graphLoadPayload(row); payload != nil {
			action.Description = strings.TrimSpace(firstStringValue(payload["description"]))
		}
		view := ModelActionView{
			ID:           action.ID,
			Name:         action.Name,
			DatasetID:    action.DatasetID,
			Description:  action.Description,
			Contract:     action.Contract.View(),
			ReviewStatus: firstStringValue(row["review_status_kwd"]),
		}
		binding, err := s.bindingDAO.FindByAction(ctx, db, tenantID, templateID, action.Name)
		if err != nil {
			return nil, err
		}
		toolAvailable := false
		var tool mcpclient.Tool
		if binding != nil {
			if tools, err := s.serverTools(ctx, db, tenantID, binding.MCPServer); err == nil {
				if found, ok := tools[binding.MCPTool]; ok {
					toolAvailable = true
					tool = found
				}
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
		result := contract.EvaluateActionGates(gate)
		view.Verdict = string(result.Verdict)
		view.VerdictReasons = result.Reasons
		if binding != nil {
			bindingView := ActionBindingView{
				OntologyActionBinding: *binding,
				ToolAvailable:         toolAvailable,
				Verdict:               view.Verdict,
				VerdictReasons:        view.VerdictReasons,
			}
			if binding.ArgMapping != nil {
				bindingView.MissingRequiredArgs = missingRequiredArgs(tool, mappingKeys(binding.ArgMapping))
			}
			view.Binding = &bindingView
		}
		out = append(out, view)
	}
	return out, nil
}

// ActionCallInput is one attempt to run an action.
type ActionCallInput struct {
	TenantID   string
	TemplateID string
	ActionName string
	// Mode is query (an agent turn) or manual (an operator pressing run).
	Mode string
	// OperatorID is the user behind a manual run, or the agent's owner for a turn.
	OperatorID string
	// ConfirmedBy records who answered a `confirm` verdict. It is empty for a query
	// turn, because there is nobody to ask there.
	ConfirmedBy string
	// Arguments lets a caller override individual tool arguments after the mapping
	// is applied. It is the manual-run path's way to supply a value the corpus
	// cannot (an account number).
	Arguments map[string]any
}

// ActionCallResult is what happened.
type ActionCallResult struct {
	ActionName string         `json:"action_name"`
	Verdict    string         `json:"verdict"`
	Reasons    []string       `json:"reasons,omitempty"`
	Status     string         `json:"status"`
	Output     string         `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMS int            `json:"duration_ms"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

// ActionCallError is a refusal the caller should render, not retry.
type ActionCallError struct {
	Verdict string
	Reasons []string
}

func (e *ActionCallError) Error() string {
	return fmt.Sprintf("action refused: %s (%s)", e.Verdict, strings.Join(e.Reasons, ", "))
}

// RunAction applies the gates, assembles the arguments and calls the tool.
//
// The audit row is written for refusals too. The question after an incident is
// always "what did it try", and a trail that keeps only successful calls cannot
// answer it.
func (s *ActionBindingService) RunAction(ctx context.Context, db *gorm.DB, in ActionCallInput) (*ActionCallResult, error) {
	started := time.Now()
	mode := in.Mode
	if mode == "" {
		mode = entity.ActionExecutionModeQuery
	}

	result := &ActionCallResult{ActionName: in.ActionName, Verdict: string(contract.ContractVerdictRefuse)}
	audit := &entity.OntologyActionExecution{
		ID:          newOntologyUUID(),
		TenantID:    in.TenantID,
		TemplateID:  in.TemplateID,
		ActionName:  in.ActionName,
		Mode:        mode,
		OperatorID:  in.OperatorID,
		ConfirmedBy: in.ConfirmedBy,
	}
	finish := func(status string, errText string) {
		audit.DurationMS = int(time.Since(started).Milliseconds())
		audit.Status = status
		audit.Error = errText
		if err := s.execDAO.Create(ctx, db, audit); err != nil {
			// The audit must not mask the outcome, but a silent failure here is worse
			// than a log line: it means the trail has a hole.
			log.Printf("[action-exec] failed to write audit row action=%s: %v", in.ActionName, err)
		}
	}

	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return nil, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, in.TenantID, template, nil)
	if err != nil {
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return nil, err
	}
	action, err := LoadCompiledAction(ctx, db, in.TenantID, in.TemplateID, datasetIDs, in.ActionName)
	if err != nil {
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return nil, err
	}
	binding, err := s.bindingDAO.FindByAction(ctx, db, in.TenantID, in.TemplateID, in.ActionName)
	if err != nil {
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return nil, err
	}

	toolAvailable := false
	var tool mcpclient.Tool
	if binding != nil {
		tools, err := s.serverTools(ctx, db, in.TenantID, binding.MCPServer)
		if err != nil {
			log.Printf("[action-exec] mcp server %q unavailable: %v", binding.MCPServer, err)
		} else if found, ok := tools[binding.MCPTool]; ok {
			toolAvailable = true
			tool = found
		}
	}
	gate := contract.ActionGateInput{
		ActionExists:      action != nil,
		TemplatePublished: template.Version > 0,
		Bound:             binding != nil,
		BindingEnabled:    binding != nil && binding.Enabled,
		ToolAvailable:     toolAvailable,
		QACallable:        binding != nil && binding.QACallable,
		ReadOnly:          binding != nil && binding.ReadOnly,
		ExecEnabled:       ActionExecEnabled(),
	}
	if action != nil {
		gate.Contract = action.Contract
	}
	verdict := contract.EvaluateActionGates(gate)
	result.Verdict = string(verdict.Verdict)
	result.Reasons = verdict.Reasons
	audit.Verdict = result.Verdict
	if binding != nil {
		audit.BindingID = binding.ID
		audit.MCPTool = binding.MCPTool
	}

	callable := verdict.Verdict == contract.ContractVerdictAllow
	if mode == entity.ActionExecutionModeQuery {
		// A query turn has nobody to confirm with; a manual run does, and the
		// confirmation has to arrive as data (ConfirmedBy) rather than as a claim.
		callable = contract.ActionCallableFromQA(verdict)
	} else if verdict.Verdict == contract.ContractVerdictConfirm && strings.TrimSpace(in.ConfirmedBy) != "" {
		callable = true
	}
	if !callable {
		result.Status = entity.ActionExecutionStatusRefused
		finish(entity.ActionExecutionStatusRefused, strings.Join(verdict.Reasons, ", "))
		return result, &ActionCallError{Verdict: string(verdict.Verdict), Reasons: verdict.Reasons}
	}

	args, err := assembleActionArguments(action, binding.ArgMapping, in.Arguments)
	if err != nil {
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = err.Error()
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return result, err
	}
	if missing := missingRequiredArgs(tool, mapKeys(args)); len(missing) > 0 {
		// Caught before the call: an upstream error here reads like "the action
		// failed", while the real problem is the binding.
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = "unmapped required arguments: " + strings.Join(missing, ", ")
		finish(entity.ActionExecutionStatusFailed, result.Error)
		return result, fmt.Errorf("%s", result.Error)
	}
	result.Arguments = args
	// The audit records which arguments were sent, never their values. A tool
	// argument is routinely an account number, a customer id or an amount: an
	// audit table is read by far more people than the action itself, and a trail
	// that copies the payload turns every review of "what ran" into a disclosure.
	// The values remain in the tool call's own response, which is where an
	// incident review looks for them.
	argNames := entity.JSONMap{}
	for key := range args {
		argNames[key] = nil
	}
	audit.RequestArg = &argNames

	encoded, err := json.Marshal(args)
	if err != nil {
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = err.Error()
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return result, err
	}
	serverRow, err := s.loadMCPServer(ctx, db, in.TenantID, binding.MCPServer)
	if err != nil {
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = err.Error()
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return result, err
	}
	callResult, err := mcpclient.CallTool(ctx, mcpclient.CallOptions{
		URL:               serverRow.URL,
		ServerType:        serverRow.ServerType,
		Headers:           jsonMapStringValues(serverRow.Headers),
		Variables:         jsonMapStringValues(serverRow.Variables),
		ToolName:          binding.MCPTool,
		Arguments:         encoded,
		Timeout:           30 * time.Second,
		AllowPrivateHosts: server.MCPAllowPrivateHosts(),
	})
	result.DurationMS = int(time.Since(started).Milliseconds())
	if err != nil {
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = err.Error()
		finish(entity.ActionExecutionStatusFailed, err.Error())
		return result, err
	}
	result.Status = entity.ActionExecutionStatusSucceeded
	result.Output = callResult.Text
	audit.ResponseExcerpt = truncateRunes(callResult.Text, 2000)
	if callResult.IsError {
		// The tool answered, and the answer is "this failed". That is a real outcome,
		// not a transport error, and the audit has to say so.
		result.Status = entity.ActionExecutionStatusFailed
		result.Error = callResult.Text
	}
	finish(result.Status, result.Error)
	return result, nil
}

func (s *ActionBindingService) loadMCPServer(ctx context.Context, db *gorm.DB, tenantID, serverName string) (*entity.MCPServer, error) {
	var servers []entity.MCPServer
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND (id = ? OR name = ?)", tenantID, serverName, serverName).
		Limit(1).Find(&servers).Error; err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp server %q is not configured", serverName)
	}
	return &servers[0], nil
}

// assembleActionArguments resolves the binding's arg_mapping into a call payload.
//
// The expression language is closed and small (contract./action./attr./literal:),
// so the result is predictable: the same action and contract always produce the
// same arguments, which is what makes a dry run meaningful.
func assembleActionArguments(action *compiledAction, mapping *entity.JSONMap, overrides map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if mapping != nil {
		for arg, raw := range *mapping {
			expression, _ := raw.(string)
			value, err := resolveArgExpression(expression, action)
			if err != nil {
				return nil, fmt.Errorf("argument %q: %w", arg, err)
			}
			if value != nil {
				out[arg] = value
			}
		}
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out, nil
}

func resolveArgExpression(expression string, action *compiledAction) (any, error) {
	expression = strings.TrimSpace(expression)
	if action == nil {
		return nil, fmt.Errorf("no compiled action to read %q from", expression)
	}
	switch {
	case strings.HasPrefix(expression, ArgExpressionContract):
		field := strings.TrimPrefix(expression, ArgExpressionContract)
		value := action.Contract.Value(field)
		if value == nil {
			return nil, fmt.Errorf("contract field %q has no value (the document does not specify it)", field)
		}
		return value, nil
	case strings.HasPrefix(expression, ArgExpressionAction):
		switch strings.TrimPrefix(expression, ArgExpressionAction) {
		case "name":
			return action.Name, nil
		case "description":
			return action.Description, nil
		case "type":
			return action.Type, nil
		}
		return nil, fmt.Errorf("unsupported action expression %q", expression)
	case strings.HasPrefix(expression, ArgExpressionAttr):
		name := strings.TrimPrefix(expression, ArgExpressionAttr)
		value, ok := action.Attributes[name]
		if !ok {
			return nil, fmt.Errorf("attribute %q has no value", name)
		}
		return value, nil
	case strings.HasPrefix(expression, ArgExpressionLiteral):
		var literal any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(expression, ArgExpressionLiteral)), &literal); err != nil {
			return nil, fmt.Errorf("literal is not valid json: %w", err)
		}
		return literal, nil
	}
	return nil, fmt.Errorf("unsupported expression %q", expression)
}

// newOntologyUUID returns a 32-character id, matching the size of every primary
// key in this schema (the dashes of a canonical UUID do not fit).
func newOntologyUUID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func mapKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

// truncateRunes cuts a string on a rune boundary.
//
// The audit column holds an upstream system's text, which can be Chinese, so a
// byte-based cut can produce invalid UTF-8 that then fails to store — turning a
// successful call into an audit write error.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// ActionDryRun answers "what would happen" without calling anything.
//
// It exists because the alternative is discovering that a binding is misconfigured
// by executing it. The arguments it reports are the real ones: same assembly code
// as the real call.
func (s *ActionBindingService) ActionDryRun(ctx context.Context, db *gorm.DB, in ActionCallInput) (*ActionCallResult, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, in.TenantID, template, nil)
	if err != nil {
		return nil, err
	}
	action, err := LoadCompiledAction(ctx, db, in.TenantID, in.TemplateID, datasetIDs, in.ActionName)
	if err != nil {
		return nil, err
	}
	binding, err := s.bindingDAO.FindByAction(ctx, db, in.TenantID, in.TemplateID, in.ActionName)
	if err != nil {
		return nil, err
	}
	toolAvailable := false
	var tool mcpclient.Tool
	if binding != nil {
		if tools, err := s.serverTools(ctx, db, in.TenantID, binding.MCPServer); err == nil {
			if found, ok := tools[binding.MCPTool]; ok {
				toolAvailable = true
				tool = found
			}
		}
	}
	gate := contract.ActionGateInput{
		ActionExists:      action != nil,
		TemplatePublished: template.Version > 0,
		Bound:             binding != nil,
		BindingEnabled:    binding != nil && binding.Enabled,
		ToolAvailable:     toolAvailable,
		QACallable:        binding != nil && binding.QACallable,
		ReadOnly:          binding != nil && binding.ReadOnly,
		ExecEnabled:       ActionExecEnabled(),
	}
	if action != nil {
		gate.Contract = action.Contract
	}
	verdict := contract.EvaluateActionGates(gate)
	out := &ActionCallResult{
		ActionName: in.ActionName,
		Verdict:    string(verdict.Verdict),
		Reasons:    verdict.Reasons,
		Status:     "dry_run",
	}
	if action == nil || binding == nil {
		return out, nil
	}
	args, err := assembleActionArguments(action, binding.ArgMapping, in.Arguments)
	if err != nil {
		// A dry run reports the failure instead of returning an error: the point of
		// asking is to see the problem.
		out.Error = err.Error()
		return out, nil
	}
	if missing := missingRequiredArgs(tool, mapKeys(args)); len(missing) > 0 {
		out.Error = "unmapped required arguments: " + strings.Join(missing, ", ")
	}
	out.Arguments = args
	return out, nil
}

// BackfillActionContracts stamps the contract columns on action rows compiled
// before those columns existed.
//
// It writes only the missing projections; it never touches a contract that has
// content. That restraint is the point: a backfill that "helpfully" recomputed
// contracts would overwrite values operators typed by hand, and the rows it is
// meant to rescue are exactly the oldest, least reviewable ones.
//
// The result is idempotent: a second run finds nothing to do, which is what makes
// it safe to offer as a button instead of a script.
func (s *ActionBindingService) BackfillActionContracts(ctx context.Context, db *gorm.DB, tenantID, templateID string) (int, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return 0, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, tenantID, template, nil)
	if err != nil {
		return 0, err
	}
	rows, _, err := reviewSearch(ctx, tenantID, datasetIDs,
		[]string{"id", "kb_id", "contract", "contract_missing_kwd"},
		map[string]interface{}{
			"compilation_template_ids": []string{templateID},
			"knowledge_graph_kwd":      []string{"entity"},
			"entity_type_kwd":          []string{contract.ActionEntityType},
		}, nil, 0, 1000)
	if err != nil {
		return 0, err
	}
	docEngine := engine.Get()
	if docEngine == nil {
		return 0, fmt.Errorf("document engine is not initialized")
	}
	updated := 0
	for _, row := range rows {
		if len(stringSliceValue(row["contract_missing_kwd"])) > 0 {
			// Already stamped: an empty contract with a missing list is the honest
			// "nobody has filled this in", and re-writing it would be a no-op that
			// still costs a write per row.
			continue
		}
		action := contract.ParseActionContract(row["contract"])
		missing := contract.ContractMissingFields(action)
		if len(missing) == 0 {
			// A contract with all six fields needs no missing list.
			continue
		}
		datasetID := firstStringValue(row["kb_id"])
		if datasetID == "" {
			continue
		}
		if err := docEngine.UpdateChunks(ctx,
			map[string]interface{}{"id": firstStringValue(row["id"]), "kb_id": datasetID},
			map[string]interface{}{"contract_missing_kwd": missing},
			wikiIndexName(tenantID), datasetID); err != nil {
			return updated, err
		}
		updated++
	}
	log.Printf("[action-contract] backfilled %d/%d action row(s) template=%s", updated, len(rows), templateID)
	return updated, nil
}
