package structure

import (
	"fmt"
	"strings"
	"time"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Prompt construction for the structure (graph) variant, mirroring Python
// structure.py: `_struct_infer_type`, `_struct_render_fields`,
// `_struct_render_type_fields` and `_struct_hypergraph_prompts`. All three
// compile kinds (list / set / hypergraph) share the same two-stage
// node → edge prompts and the {"items": [...]} response contract; they differ
// only in the Auto-type label and (for set) the uniqueness rule.

// Type identifies the structure compile kind.
type Type string

const (
	TypeList       Type = "list"
	TypeSet        Type = "set"
	TypeHypergraph Type = "hypergraph"
)

// typeAliases mirrors Python's _STRUCT_TYPE_ALIASES: "graph" and
// "knowledge_graph" both mean the hypergraph compile.
var typeAliases = map[string]Type{
	"graph":           TypeHypergraph,
	"knowledge_graph": TypeHypergraph,
}

// normalizeKind mirrors _struct_normalize_kind.
func normalizeKind(raw any) string {
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "_")
}

// InferType mirrors _struct_infer_type: explicit compile_type restricted to
// the known compile kinds; then kind returned VERBATIM (aliases resolved) —
// Python stamps arbitrary kinds ("timeline", "page_index") as the autotype;
// then the legacy output.entities+output.relations shape; default "list".
func InferType(parserConfig map[string]any) Type {
	if t, ok := resolveTypeAlias(normalizeKind(common.Get(parserConfig, "compile_type"))); ok {
		return t
	}
	kind := normalizeKind(common.Get(parserConfig, "kind"))
	if t, ok := typeAliases[kind]; ok {
		return t
	}
	if kind != "" {
		return Type(kind)
	}
	output := common.GetMap(parserConfig, "output")
	if common.Get(output, "entities") != nil && common.Get(output, "relations") != nil {
		return TypeHypergraph
	}
	return TypeList
}

func resolveTypeAlias(normalized string) (Type, bool) {
	if t, ok := typeAliases[normalized]; ok {
		return t, true
	}
	switch Type(normalized) {
	case TypeList, TypeSet, TypeHypergraph:
		return Type(normalized), true
	}
	return "", false
}

// renderFields mirrors _struct_render_fields (legacy output.entities /
// output.relations field shape). Returns (bulleted descriptions, JSON skeleton).
func renderFields(fields []any, lang string) (string, string) {
	var lines []string
	var skel []string
	for _, raw := range fields {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := f["name"].(string)
		ftype, _ := f["type"].(string)
		if ftype == "" {
			ftype = "str"
		}
		desc := common.Localize(f["description"], lang)
		reqLabel := "required"
		if req, ok := f["required"].(bool); ok && !req {
			reqLabel = "optional"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, %s): %s", name, ftype, reqLabel, desc))
		var placeholder string
		switch ftype {
		case "list":
			placeholder = "[<string>, ...]"
		case "int":
			placeholder = "<int>"
		case "float":
			placeholder = "<float>"
		case "bool":
			placeholder = "<true|false>"
		default:
			placeholder = "<string>"
		}
		skel = append(skel, fmt.Sprintf("%q: %s", name, placeholder))
	}
	return strings.Join(lines, "\n"), "{ " + strings.Join(skel, ", ") + " }"
}

// renderTypeFields mirrors _struct_render_type_fields (new compilation-template
// shape: allowed item `type` values with descriptions/rules).
func renderTypeFields(fields []any, lang, kind string) (string, string) {
	var lines []string
	var typeValues []string
	for _, raw := range fields {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(firstStringOf(f["type"]))
		if typ == "" {
			continue
		}
		typeValues = append(typeValues, typ)
		lines = append(lines, "- type: "+typ)
		if desc := common.Localize(f["description"], lang); desc != "" {
			lines = append(lines, "  description: "+desc)
		}
		if rule := common.Localize(f["rule"], lang); rule != "" {
			lines = append(lines, "  rule: "+rule)
		}
	}
	if len(typeValues) == 0 {
		typeValues = append(typeValues, "other")
		lines = append(lines, "- type: other")
	}
	oneOf := strings.Join(typeValues, "|")
	var skeleton string
	if kind == "relation" {
		skeleton = `{ "type": "<one of: ` + oneOf + `>", "source": "<known entity name>", "target": "<known entity name>", "description": "<evidence or relation description>", "source_chunk_ids": ["<source chunk id>", ...] }`
	} else {
		skeleton = `{ "type": "<one of: ` + oneOf + `>", "name": "<exact extracted item text>", "description": "<evidence, definition, or detail from the source>", "source_chunk_ids": ["<source chunk id>", ...] }`
	}
	return strings.Join(lines, "\n"), skeleton
}

// HypergraphPrompts mirrors _struct_hypergraph_prompts: it renders the node
// (entity) and edge (relation) system prompts from the parser_config template.
// The returned edge prompt still contains the "{known_nodes}" placeholder,
// filled per batch from the stage-1 entities (see fillKnownNodes). An empty
// edge prompt means the config declares no relations and stage 2 is skipped.
func HypergraphPrompts(parserConfig map[string]any, lang string) (nodePrompt, edgePrompt string) {
	autotype := InferType(parserConfig)
	guideline := common.GetMap(parserConfig, "guideline")
	output := common.GetMap(parserConfig, "output")
	options := common.GetMap(parserConfig, "options")

	target := common.Localize(common.Get(guideline, "target"), lang)
	rulesE := common.Localize(common.Get(guideline, "rules_for_entities"), lang)
	rulesR := common.Localize(common.Get(guideline, "rules_for_relations"), lang)
	rulesT := common.Localize(common.Get(guideline, "rules_for_time"), lang)
	globalRules := common.Localize(common.Get(parserConfig, "global_rules"), lang)

	observationTime, _ := common.Get(options, "observation_time").(string)
	if strings.TrimSpace(observationTime) == "" {
		observationTime = time.Now().Format("2006-01-02")
	}
	if strings.Contains(rulesT, "{observation_time}") {
		rulesT = strings.ReplaceAll(rulesT, "{observation_time}", observationTime)
	}

	usesTemplateShape := common.Get(parserConfig, "entity") != nil || common.Get(parserConfig, "relation") != nil
	var entitiesCfg, relationsCfg map[string]any
	if usesTemplateShape {
		entitiesCfg = common.GetMap(parserConfig, "entity")
		relationsCfg = common.GetMap(parserConfig, "relation")
	} else {
		entitiesCfg = common.GetMap(output, "entities")
		relationsCfg = common.GetMap(output, "relations")
	}
	entDesc := common.Localize(common.Get(entitiesCfg, "description"), lang)
	relDesc := common.Localize(common.Get(relationsCfg, "description"), lang)
	entFields := configFields(entitiesCfg)
	relFields := configFields(relationsCfg)

	var entFieldsText, entSkel, relFieldsText, relSkel string
	if usesTemplateShape {
		entFieldsText, entSkel = renderTypeFields(entFields, lang, "entity")
		relFieldsText, relSkel = renderTypeFields(relFields, lang, "relation")
	} else {
		entFieldsText, entSkel = renderFields(entFields, lang)
		relFieldsText, relSkel = renderFields(relFields, lang)
	}

	var nodeParts []string
	if target != "" {
		nodeParts = append(nodeParts, "# Role and Task:\n"+target)
	}
	if globalRules != "" {
		nodeParts = append(nodeParts, "## Global Rules:\n"+globalRules)
	}
	if rulesE != "" {
		nodeParts = append(nodeParts, "## Entity Extraction Rules:\n"+rulesE)
	}
	if entDesc != "" {
		nodeParts = append(nodeParts, "## Entity Description:\n"+entDesc)
	}
	nodeParts = append(nodeParts, "## Entity Fields:\n"+entFieldsText)
	unique := ""
	if autotype == TypeSet {
		unique = "Items must be unique. "
	}
	nodeParts = append(nodeParts,
		"## Response Format:\n"+
			"Reply with a single JSON object of the form: "+
			fmt.Sprintf(`{"items": [%s, ...]}.`, entSkel)+"\n"+
			fmt.Sprintf(`Auto-type: %q. `, string(autotype))+unique+"Return JSON only, no commentary.")
	nodePrompt = strings.Join(nodeParts, "\n\n")

	if len(relationsCfg) == 0 {
		return nodePrompt, ""
	}

	var edgeParts []string
	if target != "" {
		edgeParts = append(edgeParts, "# Role and Task:\n"+target)
	}
	if globalRules != "" {
		edgeParts = append(edgeParts, "## Global Rules:\n"+globalRules)
	}
	if rulesR != "" {
		edgeParts = append(edgeParts, "## Relation Extraction Rules:\n"+rulesR)
	}
	if rulesT != "" {
		edgeParts = append(edgeParts, "## Time Rules:\n"+rulesT)
	}
	if relDesc != "" {
		edgeParts = append(edgeParts, "## Relation Description:\n"+relDesc)
	}
	edgeParts = append(edgeParts, "## Relation Fields:\n"+relFieldsText)
	edgeParts = append(edgeParts, "## Known Entities:\n{known_nodes}")
	edgeParts = append(edgeParts,
		"## Response Format:\n"+
			"Reply with a single JSON object of the form: "+
			fmt.Sprintf(`{"items": [%s, ...]}.`, relSkel)+"\n"+
			"Only create relations between entities listed in 'Known Entities'. "+
			"Return JSON only, no commentary.")
	edgePrompt = strings.Join(edgeParts, "\n\n")
	return nodePrompt, edgePrompt
}

// fillKnownNodes renders the stage-2 edge prompt by substituting the
// {known_nodes} placeholder with the bulleted known-entity list ("(none)"
// when stage 1 found nothing), mirroring _struct_extract_hypergraph.
func fillKnownNodes(edgePromptTemplate string, knownKeys []string) string {
	knownStr := "(none)"
	if len(knownKeys) > 0 {
		knownStr = "- " + strings.Join(knownKeys, "\n- ")
	}
	return strings.ReplaceAll(edgePromptTemplate, "{known_nodes}", knownStr)
}

// UserPrompt wraps one batch's packed chunk text exactly as
// _struct_extract_hypergraph does; the same user prompt serves both stages.
func UserPrompt(packedText string) string {
	return "## Source Text:\n" +
		"Each source chunk is enclosed by [CHUNK_ID: ...] and [END_CHUNK]. " +
		"For every entity and relation, return source_chunk_ids containing only " +
		"the IDs of chunks that support that item.\n" +
		packedText + "\n\n## Output (JSON only):"
}

// PackBatch renders one batch's chunks as [CHUNK_ID: id]...[END_CHUNK]
// segments (mirrors _struct_process_batch) and returns the batch chunk ids in
// order. Chunks without text are skipped, matching Python's isinstance check.
func PackBatch(batch []common.Chunk) (string, []string) {
	segments := make([]string, 0, len(batch))
	ids := make([]string, 0, len(batch))
	for i, c := range batch {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i+1)
		}
		text := common.FirstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		ids = append(ids, id)
		segments = append(segments, "[CHUNK_ID: "+id+"]\n"+text+"\n[END_CHUNK]")
	}
	return strings.Join(segments, "\n\n"), ids
}

// EntityIDField mirrors _struct_entity_id_field.
func EntityIDField(parserConfig map[string]any) string {
	if common.Get(parserConfig, "entity") != nil {
		return "name"
	}
	identifiers := common.GetMap(parserConfig, "identifiers")
	if entityID, ok := common.Get(identifiers, "entity_id").(string); ok {
		if s := strings.TrimSpace(entityID); s != "" && !strings.Contains(s, "{") {
			return s
		}
	}
	output := common.GetMap(parserConfig, "output")
	entitiesCfg := common.GetMap(output, "entities")
	for _, raw := range configFields(entitiesCfg) {
		if f, ok := raw.(map[string]any); ok {
			if req, isBool := f["required"].(bool); !isBool || req {
				if name, _ := f["name"].(string); name != "" {
					return name
				}
			}
		}
	}
	return "name"
}

// RelationMemberFields mirrors _struct_relation_member_fields: it returns the
// payload keys carrying a relation's endpoints, or ("","") when the schema
// declares none.
func RelationMemberFields(parserConfig map[string]any) (string, string) {
	identifiers := common.GetMap(parserConfig, "identifiers")
	if members := common.GetMap(identifiers, "relation_members"); members != nil {
		src, _ := members["source"].(string)
		if src == "" {
			src, _ = members["src"].(string)
		}
		tgt, _ := members["target"].(string)
		if tgt == "" {
			tgt, _ = members["tgt"].(string)
		}
		if src != "" || tgt != "" {
			return src, tgt
		}
	}
	if common.Get(parserConfig, "relation") != nil {
		return "source", "target"
	}
	output := common.GetMap(parserConfig, "output")
	relationsCfg := common.GetMap(output, "relations")
	names := map[string]bool{}
	for _, raw := range configFields(relationsCfg) {
		if f, ok := raw.(map[string]any); ok {
			if name, _ := f["name"].(string); name != "" {
				names[name] = true
			}
		}
	}
	if names["source"] && names["target"] {
		return "source", "target"
	}
	return "", ""
}

// configFields extracts cfg["fields"] as a []any (nil when absent).
func configFields(cfg map[string]any) []any {
	if cfg == nil {
		return nil
	}
	switch v := cfg["fields"].(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, 0, len(v))
		for _, f := range v {
			out = append(out, f)
		}
		return out
	}
	return nil
}

func firstStringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
