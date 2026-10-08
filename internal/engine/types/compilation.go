package types

import "strings"

// CanonicalCompilationKind returns the persisted knowledge compilation kind.
func CanonicalCompilationKind(kind string) string {
	kind = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(kind), "-", "_"))
	switch kind {
	case "knowledge_graph", "knowledgegraph", "structure", "hypergraph", "list", "set":
		return "graph"
	case "mindmap":
		return "mind_map"
	case "pageindex":
		return "page_index"
	case "wiki_page", "wiki_section", "wiki_entity", "wiki_relation", "wiki_contribution_state", "wiki_map_active", "wiki_map_extract":
		return "wiki"
	}
	return kind
}

func compilationString(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case []string:
		if len(value) > 0 {
			return strings.TrimSpace(value[0])
		}
	case []any:
		if len(value) > 0 {
			return compilationString(value[0])
		}
	}
	return ""
}

func isCompilationKind(kind string) bool {
	switch kind {
	case "graph", "mind_map", "page_index", "tree", "timeline", "wiki", "session_graph", "session_essence":
		return true
	}
	return false
}

// CompilationKind reads the canonical field before the stored template kind.
func CompilationKind(row map[string]any) string {
	kind := compilationString(row["compile_kwd"])
	template := compilationString(row["compilation_template_kind_kwd"])
	if isCompilationKind(kind) && template == "" {
		return kind
	}
	if template != "" {
		return CanonicalCompilationKind(template)
	}
	return CanonicalCompilationKind(kind)
}

func CompilationRowType(row map[string]any) string {
	kind := compilationString(row["compile_kwd"])
	if kind == "wiki" {
		return compilationString(row["type_kwd"])
	}
	if strings.HasPrefix(kind, "wiki_") {
		return kind
	}
	return compilationString(row["type_kwd"])
}

func WikiPageCategory(row map[string]any) string {
	if compilationString(row["compile_kwd"]) == "wiki" {
		if category := compilationString(row["entity_type_kwd"]); category != "" {
			return category
		}
	}
	return compilationString(row["page_type_kwd"])
}

func IsNavigationRow(row map[string]any) bool {
	role := compilationString(row["type_kwd"])
	return role == "nav_doc" || role == "nav_cluster" || compilationString(row["compile_kwd"]) == "dataset_nav"
}

func compilationValues(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case []any:
		var out []string
		for _, v := range value {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// CompilationFilter builds predicates for mixed stored generations in one query.
// Each old-field branch is guarded so it cannot override a canonical value.
func CompilationFilter(filter map[string]any) map[string]any {
	out := make(map[string]any, len(filter))
	var clauses []map[string]any
	for field, value := range filter {
		var alternatives []map[string]any
		switch field {
		case "compilation_template_kind_kwd":
			for _, kind := range compilationValues(value) {
				alternatives = append(alternatives, compilationKindFilter(kind))
			}
			if len(alternatives) == 0 {
				out[field] = value
			}
		case "compile_kwd":
			changed := false
			for _, kind := range compilationValues(value) {
				switch {
				case kind == "dataset_nav":
					changed = true
					alternatives = append(alternatives, map[string]any{"type_kwd": []string{"nav_doc", "nav_cluster"}})
				case strings.HasPrefix(kind, "wiki_") && CanonicalCompilationKind(kind) == "wiki":
					changed = true
					alternatives = append(alternatives, wikiRoleFilter(kind))
				case isCompilationKind(CanonicalCompilationKind(kind)):
					changed = true
					alternatives = append(alternatives, compilationKindFilter(kind))
				default:
					alternatives = append(alternatives, map[string]any{field: kind})
				}
			}
			if !changed {
				alternatives = nil
				out[field] = value
			}
		case "type_kwd":
			changed := false
			for _, role := range compilationValues(value) {
				if strings.HasPrefix(role, "wiki_") && CanonicalCompilationKind(role) == "wiki" {
					changed = true
					alternatives = append(alternatives, wikiRoleFilter(role))
				} else {
					alternatives = append(alternatives, map[string]any{field: role})
				}
			}
			if !changed {
				alternatives = nil
				out[field] = value
			}
		case "entity_type_kwd":
			out[field] = value
			for _, entityType := range compilationValues(value) {
				if entityType == "mind_map" || entityType == "mindmap" {
					entityTypes := append([]string(nil), compilationValues(value)...)
					entityTypes = append(entityTypes, "mind_map", "mindmap")
					out[field] = entityTypes
					break
				}
			}
		case "page_type_kwd":
			alternatives = append(alternatives,
				map[string]any{"compile_kwd": "wiki", "entity_type_kwd": value},
				map[string]any{"page_type_kwd": value, "must_not": map[string]any{"compile_kwd": "wiki", "exists": "entity_type_kwd"}})
		case "and", "or":
			var children []map[string]any
			for _, child := range FilterClauses(value) {
				children = append(children, CompilationFilter(child))
			}
			out[field] = children
		default:
			out[field] = value
		}
		if len(alternatives) > 0 {
			clauses = append(clauses, map[string]any{"or": alternatives})
		}
	}
	if len(clauses) > 0 {
		if len(out) > 0 {
			clauses = append(clauses, out)
		}
		return map[string]any{"and": clauses}
	}
	return out
}

func wikiRoleFilter(role string) map[string]any {
	return map[string]any{"or": []map[string]any{
		{"compile_kwd": "wiki", "type_kwd": role},
		{"compile_kwd": role},
	}}
}

func compilationKindFilter(kind string) map[string]any {
	canonical := CanonicalCompilationKind(kind)
	aliases := []string{canonical}
	switch canonical {
	case "graph":
		aliases = []string{"graph", "knowledge_graph", "knowledgegraph", "structure"}
	case "mind_map":
		aliases = []string{"mind_map", "mindmap"}
	case "page_index":
		aliases = []string{"page_index", "pageindex"}
	}
	oldKeywords := append([]string(nil), aliases...)
	if canonical == "graph" {
		oldKeywords = append(oldKeywords, "hypergraph", "list", "set")
	}
	if canonical == "wiki" {
		oldKeywords = append(oldKeywords, "wiki_page", "wiki_section", "wiki_entity", "wiki_relation", "wiki_contribution_state", "wiki_map_active", "wiki_map_extract")
	}
	missingTemplateKind := []map[string]any{
		{"must_not": map[string]any{"exists": "compilation_template_kind_kwd"}},
		{"compilation_template_kind_kwd": []string{""}},
	}
	return map[string]any{"or": []map[string]any{
		{"compile_kwd": canonical, "or": missingTemplateKind},
		{"compilation_template_kind_kwd": aliases},
		{"compile_kwd": oldKeywords, "or": missingTemplateKind},
	}}
}

func FilterClauses(value any) []map[string]any {
	switch value := value.(type) {
	case []map[string]any:
		return value
	case []any:
		var out []map[string]any
		for _, child := range value {
			if child, ok := child.(map[string]any); ok {
				out = append(out, child)
			}
		}
		return out
	}
	return nil
}
