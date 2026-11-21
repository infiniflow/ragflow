package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ontology/contract"

	"gorm.io/gorm"
)

// Manual action editing: the operator half of the contract.
//
// Not every authorization rule lives in the corpus. Enterprises routinely write
// the procedure into documents while the permission sits in an approval system,
// so the missing fields can only be filled in by a person. That person must be
// able to fill them without editing a row by hand, and without the next compile
// silently throwing the value away.
//
// The mechanism is one rule (contract.ActionContract.SetManual): writing a value
// deletes its quote. Everything else follows — provenance becomes `manual`, the
// next extraction cannot overwrite it (MergeExtracted only overwrites fields that
// bring a quote), and the agent is forbidden from citing a chunk for it.

// ActionContractEditInput is one manual edit of one action row.
type ActionContractEditInput struct {
	TenantID   string
	TemplateID string
	ActionID   string
	ActorID    string
	// DatasetID is optional: the caller knows the row id, not where it lives, so
	// the dataset is resolved from the model's own scope when it is empty.
	DatasetID string
	// Values maps a contract field to its new value. A nil value clears the field
	// (that is how "we were wrong, the document says nothing" is expressed);
	// audit_required takes a bool, the atomic_effects/follow_ups take a list or a
	// string, the rest take a string.
	Values map[string]any
	// Description optionally replaces the action's description. The contract is
	// usually what an operator edits, but a mis-extracted action name/description
	// is the other half of the same problem.
	Description *string
	// Reason is required: a contract edit changes what the system is allowed to
	// do, so "why" is not optional.
	Reason string
}

// ActionContractEditResult is the row after the edit.
type ActionContractEditResult struct {
	ID           string                      `json:"id"`
	DatasetID    string                      `json:"dataset_id"`
	Contract     contract.ActionContractView `json:"contract"`
	ReviewStatus string                      `json:"review_status"`
	ReviewBy     string                      `json:"review_by"`
}

// ActionEditRejectedError is a caller error (bad field, missing reason, row not
// found). The handler maps it to 400 rather than 500: nothing is broken, the
// request is.
type ActionEditRejectedError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Fields lists the contract fields involved, when the rejection is about them.
	Fields []string `json:"fields,omitempty"`
}

func (e *ActionEditRejectedError) Error() string {
	if len(e.Fields) > 0 {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, strings.Join(e.Fields, ", "))
	}
	return e.Code + ": " + e.Message
}

// Rejection codes, stable because the UI renders them.
const (
	ActionEditCodeFieldNotEditable = "FIELD_NOT_EDITABLE"
	ActionEditCodeReasonRequired   = "REASON_REQUIRED"
	ActionEditCodeActionNotFound   = "ACTION_NOT_FOUND"
	ActionEditCodeNotAnAction      = "NOT_AN_ACTION"
	ActionEditCodeEmptyEdit        = "EMPTY_EDIT"
)

// EditActionContract applies one manual edit.
//
// Order matters and is the contract's own requirement: write the value, delete
// the quote, then recompute contract_missing_kwd from the result. Writing the
// missing column from anything but the final contract is how it drifts into
// "shows a value, still counts as missing".
func (s *CompilationTemplateGovernanceService) EditActionContract(ctx context.Context, db *gorm.DB, in ActionContractEditInput) (*ActionContractEditResult, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return nil, &ActionEditRejectedError{
			Code:    ActionEditCodeReasonRequired,
			Message: "a contract edit changes what the system may do; state why",
		}
	}
	if len(in.Values) == 0 && in.Description == nil {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeEmptyEdit, Message: "nothing to update"}
	}
	// Reject unknown fields instead of ignoring them: a silently dropped key looks
	// like a successful edit and leaves the operator believing a rule was changed.
	allowed := map[string]bool{}
	for _, name := range contract.ActionContractFields() {
		allowed[name] = true
	}
	unknown := make([]string, 0, len(in.Values))
	for name := range in.Values {
		if !allowed[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, &ActionEditRejectedError{
			Code:    ActionEditCodeFieldNotEditable,
			Message: "only the six contract fields and description are editable; source_chunk_ids, evidence and id may not be rewritten",
			Fields:  unknown,
		}
	}

	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, in.TenantID, template, nil)
	if err != nil {
		return nil, err
	}
	if len(datasetIDs) == 0 {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeActionNotFound, Message: "no dataset compiles this model"}
	}
	datasetID := strings.TrimSpace(in.DatasetID)
	if datasetID != "" {
		bound := false
		for _, id := range datasetIDs {
			if id == datasetID {
				bound = true
				break
			}
		}
		if !bound {
			return nil, &ActionEditRejectedError{Code: ActionEditCodeActionNotFound, Message: "dataset does not compile this model"}
		}
	}

	row, err := findActionRow(ctx, in.TenantID, template.ID, datasetID, datasetIDs, in.ActionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeActionNotFound, Message: "no such action in this model"}
	}
	rowDataset := strings.TrimSpace(firstStringValue(row["kb_id"]))
	if rowDataset == "" {
		rowDataset = datasetID
	}
	current := contract.ParseActionContract(row["contract"])
	updated := current.Clone()
	for name, value := range in.Values {
		// SetManual is the whole rule: the value goes in, the quote goes out.
		updated.SetManual(name, normalizeContractEditValue(value))
	}
	if updated.IsEmpty() && len(in.Values) > 0 {
		// An all-empty contract must remove the columns rather than write an empty
		// object plus a full missing list: a row with no contract is the honest
		// "nothing has been extracted or entered here".
		updated = contract.NewActionContract()
	}

	// Snapshot only what this call changed. The history is read to answer "who
	// widened the authorization, and from what", so a copy of all six fields
	// would bury the one that moved.
	changedFields := contractEditFieldNames(in.Values)
	before := contractFieldSnapshot(current, changedFields)
	after := contractFieldSnapshot(updated, changedFields)
	if in.Description != nil {
		changedFields = append(changedFields, "description")
		before["description"] = map[string]interface{}{
			"v": strings.TrimSpace(firstStringValue(graphLoadPayload(row)["description"])),
		}
		after["description"] = map[string]interface{}{
			"v": strings.TrimSpace(*in.Description),
		}
	}

	update := map[string]interface{}{
		"contract":             updated,
		"contract_missing_kwd": contract.ContractMissingFields(updated),
		// A human just signed this row. Marking it accepted is what keeps it out of
		// the unreviewed queue and out of a bulk decision someone else makes later.
		"review_status_kwd": string(AssertionReviewAccepted),
		"review_by_kwd":     strings.TrimSpace(in.ActorID),
		"review_at_flt":     float64(time.Now().UnixMilli()),
		"review_note":       "manual edit: " + strings.TrimSpace(in.Reason),
	}
	if in.Description != nil {
		payload := graphLoadPayload(row)
		if payload == nil {
			payload = map[string]interface{}{}
		}
		payload["description"] = strings.TrimSpace(*in.Description)
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		update["content_with_weight"] = string(encoded)
	}

	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("document engine is not initialized")
	}
	if err := docEngine.UpdateChunks(ctx,
		map[string]interface{}{"id": in.ActionID, "kb_id": rowDataset},
		update, wikiIndexName(in.TenantID), rowDataset); err != nil {
		return nil, err
	}
	// The row carries only the newest signature, so the history is written after
	// the change lands. A failure here must not undo the edit: the contract is
	// already in force, and reporting it as failed would invite a retry that
	// applies the same change twice. It is logged instead.
	history := &entity.OntologyActionContractEdit{
		ID:         newOntologyUUID(),
		TenantID:   in.TenantID,
		TemplateID: template.ID,
		ActionName: firstStringValue(row["name_kwd"]),
		ActionID:   in.ActionID,
		DatasetID:  rowDataset,
		OperatorID: strings.TrimSpace(in.ActorID),
		Reason:     strings.TrimSpace(in.Reason),
		Before:     before,
		After:      after,
	}
	if err := s.contractEditDAO.Create(ctx, db, history); err != nil {
		log.Printf("[action-contract] history write failed action=%s operator=%s: %v",
			history.ActionName, in.ActorID, err)
	}
	log.Printf("[action-contract] manual edit action=%s fields=%s operator=%s",
		firstStringValue(row["name_kwd"]), strings.Join(changedFields, ","), in.ActorID)

	return &ActionContractEditResult{
		ID:           in.ActionID,
		DatasetID:    rowDataset,
		Contract:     updated.View(),
		ReviewStatus: string(AssertionReviewAccepted),
		ReviewBy:     strings.TrimSpace(in.ActorID),
	}, nil
}

// contractFieldSnapshot serialises only the named fields, in the column's own
// {"v":value,"q":"quote"} shape.
//
// It goes through the contract's JSON form rather than the typed view because
// the history is evidence: it must store the bytes that were in force, including
// a field that was cleared (absent here, recorded as null) — a view would
// convert "nobody stated it" back into a status and lose what actually changed.
func contractFieldSnapshot(c contract.ActionContract, names []string) entity.JSONMap {
	out := entity.JSONMap{}
	if len(names) == 0 {
		return out
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return out
	}
	parsed := map[string]interface{}{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return out
	}
	for _, name := range names {
		if value, ok := parsed[name]; ok {
			out[name] = value
			continue
		}
		out[name] = nil
	}
	return out
}

// ActionContractEditView is one history row as the workbench lists it.
type ActionContractEditView struct {
	ID         string         `json:"id"`
	OperatorID string         `json:"operator_id"`
	Reason     string         `json:"reason"`
	Before     entity.JSONMap `json:"before,omitempty"`
	After      entity.JSONMap `json:"after,omitempty"`
	CreateTime int64          `json:"create_time,omitempty"`
}

// ListActionContractEdits returns the manual edits of one action, newest first.
//
// Keyed by name, not by row id: a recompile mints a new id for the same action,
// and "what may this action do" is a question that spans those recompiles.
func (s *CompilationTemplateGovernanceService) ListActionContractEdits(
	ctx context.Context, db *gorm.DB, tenantID, templateID, actionName string, offset, limit int,
) ([]ActionContractEditView, int64, error) {
	actionName = strings.TrimSpace(actionName)
	if actionName == "" {
		return nil, 0, &ActionEditRejectedError{
			Code: ActionEditCodeActionNotFound, Message: "empty action name",
		}
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.contractEditDAO.ListByAction(ctx, db, tenantID, templateID, actionName, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ActionContractEditView, 0, len(rows))
	for _, row := range rows {
		view := ActionContractEditView{
			ID:         row.ID,
			OperatorID: row.OperatorID,
			Reason:     row.Reason,
			Before:     row.Before,
			After:      row.After,
		}
		if row.CreateTime != nil {
			view.CreateTime = *row.CreateTime
		}
		out = append(out, view)
	}
	return out, total, nil
}

// findActionRow locates one compiled row by id inside the model's scope.
//
// The scope check is not redundant with the id: the engine's update only enforces
// kb_id, so a caller who guessed an id from another model's dataset would
// otherwise be able to rewrite it through this path.
func findActionRow(ctx context.Context, tenantID, templateID, datasetID string, datasetIDs []string, actionID string) (map[string]interface{}, error) {
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeActionNotFound, Message: "empty action id"}
	}
	scope := datasetIDs
	if datasetID != "" {
		scope = []string{datasetID}
	}
	rows, _, err := reviewSearch(ctx, tenantID, scope, []string{"id", "kb_id", "name_kwd", "contract", "content_with_weight", "knowledge_graph_kwd"},
		map[string]interface{}{
			"id":                       []string{actionID},
			"compilation_template_ids": []string{templateID},
			"knowledge_graph_kwd":      []string{"entity"},
		}, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// normalizeContractEditValue keeps the column's three value shapes: a trimmed
// string, a list of non-empty strings, or a bool. Anything else is stringified
// rather than stored raw, so the column cannot accumulate maps that no reader
// expects.
func normalizeContractEditValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case bool:
		return v
	case string:
		return strings.TrimSpace(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, text := range v {
			if strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func contractEditFieldNames(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
