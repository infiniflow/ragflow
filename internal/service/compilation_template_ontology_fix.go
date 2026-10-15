package service

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// OntologyFixRequest is one edit a reader confirmed on the compile-quality
// ledger (ontology.md §2.7). It is deliberately a small, closed vocabulary:
// the ledger can only propose edits this file knows how to make, so a client
// cannot talk the template into an arbitrary rewrite.
//
// Two operations carry every ledger row:
//
//	widen   — a rejected assertion's endpoint class is missing from the
//	          property's declaration, so the declaration is what changes.
//	declare — a property observed in the data has no declaration at all.
type OntologyFixRequest struct {
	// Op is "widen" or "declare".
	Op string `json:"op"`
	// Property is the property whose declaration changes.
	Property string `json:"property"`
	// Side is which endpoint of a widened property changes: "domain" or "range".
	Side string `json:"side,omitempty"`
	// Class is the endpoint class a widened declaration must admit.
	Class string `json:"class,omitempty"`
	// Domain / Range / Datatype seed a newly declared property.
	Domain   string `json:"domain,omitempty"`
	Range    string `json:"range,omitempty"`
	Datatype string `json:"datatype,omitempty"`
}

const (
	ontologyFixWiden   = "widen"
	ontologyFixDeclare = "declare"
)

// ApplyOntologyFix applies one confirmed edit to an ontology template and saves
// it through the ordinary group-update path, so validation, tenant scoping and
// sibling reconciliation are the ones a manual save in the editor gets. It
// returns the patched template so the caller can refresh what it displays.
//
// Nothing here reasons about the documents: re-compiling the affected ones is
// the caller's next step, and doing it is what makes the ledger's numbers move.
func (s *CompilationTemplateGroupService) ApplyOntologyFix(
	ctx context.Context, tenantID, templateID string, fix OntologyFixRequest,
) (*GroupTemplate, error) {
	return s.ApplyOntologyFixes(ctx, tenantID, templateID, []OntologyFixRequest{fix})
}

// ApplyOntologyFixes applies the edits a reader selected, as ONE write.
//
// The reader selects several lines of the ledger and then compiles once: each
// line is an edit to the same template, so writing them one by one would save
// the template several times and re-compile several times, and re-compiling is
// the expensive step (it calls the model). One payload, one validate, one save.
//
// The batch is all-or-nothing: an edit that the template cannot take fails the
// whole batch and nothing is written, so a half-applied selection can never be
// what the next compile runs against.
func (s *CompilationTemplateGroupService) ApplyOntologyFixes(
	ctx context.Context, tenantID, templateID string, fixes []OntologyFixRequest,
) (*GroupTemplate, error) {
	if len(fixes) == 0 {
		return nil, fmt.Errorf("no fixes were selected.")
	}
	if strings.TrimSpace(templateID) == "" {
		return nil, fmt.Errorf("a template id is required.")
	}
	tpl, err := s.templateDAO.GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	if tpl == nil || tpl.GroupID == nil || strings.TrimSpace(*tpl.GroupID) == "" {
		return nil, fmt.Errorf("cannot find template %s.", templateID)
	}
	if kind := strings.TrimSpace(tpl.Kind); !kccommon.IsOntologyKind(kind) {
		return nil, fmt.Errorf("template %s is a %s template; declarations are edited here only for ontology templates.", templateID, kind)
	}

	patched, err := ApplyOntologyFixesToConfig(map[string]any(tpl.Config), fixes)
	if err != nil {
		return nil, err
	}
	// Same gate the editor save passes through: a fix that would make the
	// template invalid is refused instead of written.
	if err := ValidateTemplatePayload(map[string]any{
		"name": tpl.Name, "kind": tpl.Kind, "config": patched,
	}, true); err != nil {
		return nil, err
	}

	children, err := s.templateDAO.ListByGroup(ctx, dao.DB, *tpl.GroupID)
	if err != nil {
		return nil, err
	}
	// The sibling templates travel back untouched: reconcileChildren removes
	// what the payload does not carry, so a partial payload would delete them.
	req := &GroupRequest{Templates: make([]*GroupTemplate, 0, len(children))}
	for _, child := range children {
		config := child.Config
		if child.ID == tpl.ID {
			config = entity.JSONMap(patched)
		}
		req.Templates = append(req.Templates, &GroupTemplate{
			ID:          child.ID,
			Name:        child.Name,
			Description: derefString(child.Description),
			Kind:        child.Kind,
			Config:      config,
		})
	}
	if _, err := s.UpdateGroup(ctx, tenantID, *tpl.GroupID, req); err != nil {
		return nil, err
	}
	return &GroupTemplate{
		ID:          tpl.ID,
		Name:        tpl.Name,
		Description: derefString(tpl.Description),
		Kind:        tpl.Kind,
		Config:      entity.JSONMap(patched),
	}, nil
}

// ApplyOntologyFixesToConfig applies the selected edits in order to one copy of
// the config, so the caller can validate and save once. All-or-nothing: the
// first edit the template cannot take stops the batch and the caller's config is
// left untouched, so a partially applied selection can never be saved.
func ApplyOntologyFixesToConfig(config map[string]any, fixes []OntologyFixRequest) (map[string]any, error) {
	if len(fixes) == 0 {
		return nil, fmt.Errorf("no fixes were selected.")
	}
	patched := config
	for index, fix := range fixes {
		next, err := ApplyOntologyFixToConfig(patched, fix)
		if err != nil {
			// Name the offending edit: the reader selected it, so the message has
			// to say which one the template refused.
			return nil, fmt.Errorf("edit %d (%s): %w", index+1, strings.TrimSpace(fix.Property), err)
		}
		patched = next
	}
	return patched, nil
}

// ApplyOntologyFixToConfig is the whole edit, as a pure function over the
// template's config: it copies, changes the one declaration and returns. The
// input map is not mutated, so a rejected fix cannot leave a half-edited config
// behind for the next reader.
func ApplyOntologyFixToConfig(config map[string]any, fix OntologyFixRequest) (map[string]any, error) {
	property := strings.TrimSpace(fix.Property)
	if property == "" {
		return nil, fmt.Errorf("the fix names no property.")
	}
	switch strings.ToLower(strings.TrimSpace(fix.Op)) {
	case ontologyFixWiden:
		side := strings.ToLower(strings.TrimSpace(fix.Side))
		if side != "domain" && side != "range" {
			return nil, fmt.Errorf("widening %s needs a side of domain or range.", property)
		}
		class := strings.TrimSpace(fix.Class)
		if class == "" {
			return nil, fmt.Errorf("widening %s names no class to admit.", property)
		}
		out, section, fields, err := copyConfigWithRelationFields(config)
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			if strings.TrimSpace(stringFieldValue(field, "type")) != property {
				continue
			}
			field[side] = appendPipeValue(stringFieldValue(field, side), class)
			setRelationFields(section, fields)
			return out, nil
		}
		return nil, fmt.Errorf("the template does not declare %s, so there is nothing to widen.", property)

	case ontologyFixDeclare:
		domain := strings.TrimSpace(fix.Domain)
		target := strings.TrimSpace(fix.Range)
		if domain == "" && target == "" {
			return nil, fmt.Errorf("declaring %s needs a domain or a range.", property)
		}
		for _, field := range relationFields(config) {
			if strings.TrimSpace(stringFieldValue(field, "type")) == property {
				return nil, fmt.Errorf("the template already declares %s.", property)
			}
		}
		out, section, fields, err := copyConfigWithRelationFields(config)
		if err != nil {
			return nil, err
		}
		// Only the keys the reader actually knows are written, so an inferred
		// endpoint is not turned into an invented one.
		declared := map[string]any{"type": property}
		if domain != "" {
			declared["domain"] = domain
		}
		if target != "" {
			declared["range"] = target
		}
		if datatype := strings.TrimSpace(fix.Datatype); datatype != "" {
			declared["datatype"] = datatype
		}
		switch {
		case strings.TrimSpace(fix.Datatype) != "":
			declared["kind"] = "datatype"
		case target != "":
			declared["kind"] = "object"
		}
		setRelationFields(section, append(fields, declared))
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported fix %q.", fix.Op)
	}
}

// appendPipeValue adds a value to a `|`-separated declaration, keeping the
// existing order and refusing to add what is already there: the editor shows
// these values verbatim, and a duplicate would read as two declarations.
func appendPipeValue(current, add string) string {
	add = strings.TrimSpace(add)
	if add == "" {
		return current
	}
	var values []string
	for _, part := range strings.Split(current, "|") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	for _, value := range values {
		if value == add {
			return current
		}
	}
	values = append(values, add)
	return strings.Join(values, "|")
}

// relationFields returns the config's relation.fields entries, or nil.
func relationFields(config map[string]any) []map[string]any {
	sections := configSection(config, "relation")
	if sections == nil {
		return nil
	}
	raw, ok := sections["fields"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		if field, ok := entry.(map[string]any); ok {
			out = append(out, field)
		}
	}
	return out
}

// configSection returns the config's section map, or nil.
func configSection(config map[string]any, name string) map[string]any {
	if config == nil {
		return nil
	}
	if section, ok := config[name].(map[string]any); ok {
		return section
	}
	return nil
}

// copyConfigWithRelationFields shallow-copies the config and deep-copies the
// relation section down to its field maps, which is exactly the depth an edit
// touches. Everything else stays shared with the caller's map on purpose: this
// only has to keep a rejected fix from mutating the row it read.
func copyConfigWithRelationFields(config map[string]any) (map[string]any, map[string]any, []map[string]any, error) {
	out := make(map[string]any, len(config)+1)
	for key, value := range config {
		out[key] = value
	}
	section := configSection(config, "relation")
	if section == nil {
		return nil, nil, nil, fmt.Errorf("the template has no relation section to edit.")
	}
	sectionCopy := make(map[string]any, len(section))
	for key, value := range section {
		sectionCopy[key] = value
	}
	fields := make([]map[string]any, 0, len(relationFields(config)))
	raw, _ := section["fields"].([]any)
	for _, entry := range raw {
		field, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		fieldCopy := make(map[string]any, len(field))
		for key, value := range field {
			fieldCopy[key] = value
		}
		fields = append(fields, fieldCopy)
	}
	sectionCopy["fields"] = fieldsAsAny(fields)
	out["relation"] = sectionCopy
	// The copy's section (not the caller's) is what an edit writes into, so a
	// config read from a row is never mutated on the way to a failed fix.
	return out, sectionCopy, fields, nil
}

// setRelationFields writes the edited fields back into the copied section.
func setRelationFields(section map[string]any, fields []map[string]any) {
	if section == nil {
		return
	}
	section["fields"] = fieldsAsAny(fields)
}

func fieldsAsAny(fields []map[string]any) []any {
	out := make([]any, 0, len(fields))
	for _, field := range fields {
		out = append(out, field)
	}
	return out
}

// stringFieldValue reads a declaration field as a string.
func stringFieldValue(field map[string]any, key string) string {
	if field == nil {
		return ""
	}
	value, _ := field[key].(string)
	return value
}
