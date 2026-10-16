package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/dao"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// StructureFilterForTemplate reads the named ontology model and returns the
// filter its constraints map to, with every class expanded along the declared
// hierarchy.
//
// It fails rather than degrading: a caller who asked for structure-aware
// retrieval and gets an unreadable model would otherwise receive a search that
// silently ignores the narrowing — the filter would match nothing (or
// everything) and nothing on screen would say which.
//
// version pins the snapshot to read. Zero means the model's current published
// version, or the working config when it has never been published.
func StructureFilterForTemplate(
	ctx context.Context,
	tenantID, templateID string,
	version int,
	c OntologyStructureConstraints,
) (map[string]interface{}, error) {
	if c.IsZero() {
		return nil, nil
	}
	if strings.TrimSpace(templateID) == "" {
		return nil, fmt.Errorf("ontology_model_id is required when the search carries structure constraints")
	}
	declared, _, err := loadOntologyTemplate(ctx, tenantID, templateID, version)
	if err != nil {
		return nil, err
	}
	return OntologyStructureFilter(declared, c), nil
}

// OntologyModelRef names the model a declared structure was read from, so a
// result can always say which version of the model produced it. A question
// asked either side of a publish may legitimately get different answers, and
// nothing else can explain that.
type OntologyModelRef struct {
	ID            string
	Name          string
	Version       int
	PublishStatus string
}

// OntologyAssertionKindFilter pins retrieval to compiled assertions.
//
// It is a filter in its own right because the knowledge layer's default
// request carries no type constraint: asking the knowledge scope what it says
// about a question is a complete request, and the row-kind pin is what keeps
// navigation clusters, wiki pages and build markers out of the answer.
//
// The column is knowledge_graph_kwd rather than type_kwd on purpose: the
// engines translate that key into `type_kwd IN (...) OR knowledge_graph_kwd IN
// (...)`, which is the only form that also matches rows written before the
// column was renamed.
func OntologyAssertionKindFilter() map[string]interface{} {
	return map[string]interface{}{"knowledge_graph_kwd": []string{"entity", "relation"}}
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

// loadOntologyTemplate reads one ontology model's declared structure together
// with the identity of the version it was read from.
//
// The config a published model is read through is the IMMUTABLE SNAPSHOT, never
// the working config, exactly as compilation reads it: the rows being searched
// were produced under the snapshot, so narrowing by a draft edited since would
// filter on classes the rows do not carry and the mismatch would surface only
// as a silently emptier result set. A model that has never been published has
// no snapshot, and its working config is the only structure there is.
//
// A missing model, a foreign tenant's model, a kind that is not an ontology and
// an unreadable snapshot are all errors rather than nils: the caller asked for
// a structure, and the caller has to say that it could not be had instead of
// quietly searching without it.
func loadOntologyTemplate(ctx context.Context, tenantID, templateID string, version int) (declaredOntology, OntologyModelRef, error) {
	var declared declaredOntology
	ref := OntologyModelRef{}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(templateID) == "" {
		return declared, ref, fmt.Errorf("ontology model %q could not be read: a tenant and a model id are both required", strings.TrimSpace(templateID))
	}
	tpl, err := dao.NewCompilationTemplateDAO().GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil || tpl == nil {
		return declared, ref, fmt.Errorf("ontology model %q not found", strings.TrimSpace(templateID))
	}
	if !kccommon.IsOntologyKind(tpl.Kind) {
		return declared, ref, fmt.Errorf("template %q is not an ontology model (kind %q)", strings.TrimSpace(templateID), tpl.Kind)
	}
	ref = OntologyModelRef{
		ID:            tpl.ID,
		Name:          tpl.Name,
		Version:       tpl.Version,
		PublishStatus: tpl.PublishStatus,
	}
	cfg := map[string]interface{}(tpl.Config)
	effective := tpl.Version
	if version > 0 {
		effective = version
	}
	if effective > 0 {
		snapshot, snapErr := dao.NewCompilationTemplateVersionDAO().GetByVersion(ctx, dao.DB, tenantID, templateID, effective)
		if snapErr != nil || snapshot == nil {
			return declared, ref, fmt.Errorf("ontology model %q version %d could not be read; the rows compiled under it cannot be narrowed reliably", strings.TrimSpace(templateID), effective)
		}
		if !kccommon.IsOntologyKind(snapshot.Kind) {
			return declared, ref, fmt.Errorf("ontology model %q version %d is not an ontology model (kind %q)", strings.TrimSpace(templateID), effective, snapshot.Kind)
		}
		cfg = map[string]interface{}(snapshot.Config)
		ref.Version = effective
		if strings.TrimSpace(snapshot.Name) != "" {
			ref.Name = snapshot.Name
		}
	}
	return parseDeclaredOntology(cfg), ref, nil
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
