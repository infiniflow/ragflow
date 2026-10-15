package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/dao"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// StructureFilterForTemplate reads the named ontology template and returns the
// ES filter its constraints map to, with every class expanded along the
// declared hierarchy.
//
// It fails rather than degrading: a caller who asked for structure-aware
// retrieval and gets an unreadable template would otherwise receive a search
// that silently ignores the narrowing — the filter would match nothing (or
// everything) and nothing on screen would say which.
func StructureFilterForTemplate(
	ctx context.Context,
	tenantID, templateID string,
	c OntologyStructureConstraints,
) (map[string]interface{}, error) {
	if c.IsZero() {
		return nil, nil
	}
	if strings.TrimSpace(templateID) == "" {
		return nil, fmt.Errorf("ontology_template_id is required when the search carries structure constraints")
	}
	cfg := loadOntologyTemplateConfig(ctx, tenantID, templateID)
	if cfg == nil {
		return nil, fmt.Errorf("ontology template %q could not be read as an ontology template, so its structure cannot narrow this search", strings.TrimSpace(templateID))
	}
	return OntologyStructureFilter(parseDeclaredOntology(cfg), c), nil
}

// OntologyStructureConstraints is the structural narrowing a caller asks of a
// retrieval: which classes the compiled entities must be typed with, which
// object properties the assertions must use, and which endpoint classes those
// assertions may join.
//
// Every field is explicit input on purpose. A structure filter derived from the
// question's prose would narrow retrieval to whatever the reader happened to
// say, and the project's convention forbids parsing structure out of natural
// language: the caller (a UI, a tool with typed arguments, an API client) states
// the structure, and the service only applies it.
type OntologyStructureConstraints struct {
	EntityTypes []string
	Properties  []string
	FromTypes   []string
	ToTypes     []string
}

// IsZero reports whether nothing was asked for, which is the case that must
// leave retrieval exactly as it was.
func (c OntologyStructureConstraints) IsZero() bool {
	return len(c.EntityTypes) == 0 &&
		len(c.Properties) == 0 &&
		len(c.FromTypes) == 0 &&
		len(c.ToTypes) == 0
}

// loadOntologyTemplateConfig reads one ontology template's config so the
// constraints can be expanded along the declared hierarchy. It returns nil for a
// missing template, a foreign tenant's template, or a template that is not an
// ontology: in each case the caller asked for a structure that cannot be read,
// and the caller has to say so rather than quietly searching without it.
func loadOntologyTemplateConfig(ctx context.Context, tenantID, templateID string) map[string]interface{} {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(templateID) == "" {
		return nil
	}
	tpl, err := dao.NewCompilationTemplateDAO().GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil || tpl == nil {
		return nil
	}
	if !kccommon.IsOntologyKind(tpl.Kind) {
		return nil
	}
	return map[string]interface{}(tpl.Config)
}

// declaredChildren inverts the declared parent links into a child list per
// class, in declaration order so the expansion is reproducible run to run.
func declaredChildren(declared declaredOntology) map[string][]string {
	children := make(map[string][]string, len(declared.classes))
	for _, name := range declared.classOrder {
		for _, parent := range declared.classes[name].parents {
			parent = strings.TrimSpace(parent)
			if parent == "" {
				continue
			}
			if !containsStringValue(children[parent], name) {
				children[parent] = append(children[parent], name)
			}
		}
	}
	return children
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// expandDeclaredClasses returns each requested class together with every
// declared descendant of it.
//
// This IS the ontology guiding retrieval: a reader who asks for `agent` means
// the instances typed `person` and `organization` too, and without the
// expansion the filter would return nothing at all for the abstract class —
// exactly the case that made the declared hierarchy worth writing down. A class
// the template does not declare is kept verbatim rather than dropped: the caller
// named it, and a filter for a name that matches nothing is more honest than
// silently searching for something else.
func expandDeclaredClasses(classes []string, children map[string][]string) []string {
	out := make([]string, 0, len(classes))
	seen := make(map[string]bool, len(classes))
	var walk func(string)
	walk = func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
		for _, child := range children[name] {
			walk(child)
		}
	}
	for _, name := range classes {
		walk(name)
	}
	return out
}

// dedupeStrings keeps first-seen order, which is the order the caller wrote.
func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// OntologyStructureFilter turns the constraints into the row columns the
// compiled rows already carry (applyStructureGraphColumns writes them), so the
// narrowing needs no new storage:
//
//	entity_type_kwd  the class of a compiled entity
//	prop_kwd         the object property of a compiled assertion
//	from_type_kwd    the class at the assertion's from side
//	to_type_kwd      the class at the assertion's to side
//
// It also pins the row kinds. A constraint on a class means compiled entity
// rows, a constraint on a property or an endpoint means compiled assertions —
// without that, `prop_kwd` would also match hypernode rows (a hypernode carries
// its leaf attribute in the same column) and the reader would get retrieval
// units that are not the thing they asked for.
func OntologyStructureFilter(declared declaredOntology, c OntologyStructureConstraints) map[string]interface{} {
	if c.IsZero() {
		return nil
	}
	children := declaredChildren(declared)
	filter := make(map[string]interface{}, 5)
	kinds := map[string]bool{}

	if types := dedupeStrings(c.EntityTypes); len(types) > 0 {
		filter["entity_type_kwd"] = expandDeclaredClasses(types, children)
		kinds["entity"] = true
	}
	if properties := dedupeStrings(c.Properties); len(properties) > 0 {
		filter["prop_kwd"] = properties
		kinds["relation"] = true
	}
	if from := dedupeStrings(c.FromTypes); len(from) > 0 {
		filter["from_type_kwd"] = expandDeclaredClasses(from, children)
		kinds["relation"] = true
	}
	if to := dedupeStrings(c.ToTypes); len(to) > 0 {
		filter["to_type_kwd"] = expandDeclaredClasses(to, children)
		kinds["relation"] = true
	}
	if len(kinds) > 0 {
		ordered := make([]string, 0, len(kinds))
		for kind := range kinds {
			ordered = append(ordered, kind)
		}
		sort.Strings(ordered)
		filter["knowledge_graph_kwd"] = ordered
	}
	return filter
}
