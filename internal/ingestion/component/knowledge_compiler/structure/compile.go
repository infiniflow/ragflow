package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// CompileConfig carries the per-run identity and extraction settings.
type CompileConfig struct {
	LLMID        string
	Type         Type // inferred compile kind (list / set / hypergraph)
	TenantID     string
	DocID        string
	Variant      common.Variant
	Lang         string
	ParserConfig map[string]any
	// TemplateID is mixed into stable row ids so two templates sharing a
	// compile kind don't collide on identical payloads (mirrors Python's
	// row_seed_extras in _struct_to_doc_storage_doc).
	TemplateID string
}

// extractionTemperature mirrors Python's gen_conf for structure extraction
// (_struct_extract_hypergraph uses temperature 0.1).
var extractionTemperature = 0.1

// extractHypergraph mirrors _struct_extract_hypergraph: stage 1 extracts node
// (entity) items, stage 2 extracts edge (relation) items constrained to the
// stage-1 entities via the {known_nodes} placeholder. The two stages within
// one batch are strictly sequential; batches parallelise at the caller.
func extractHypergraph(ctx context.Context, deps common.Deps, cfg CompileConfig, nodePrompt, edgePromptTmpl, packedText string) (nodes, edges []map[string]any, err error) {
	user := UserPrompt(packedText)
	nodeRaw, err := common.GenJSON(ctx, deps.Chat, common.ChatRequest{
		LLMID: cfg.LLMID, SystemPrompt: nodePrompt, UserPrompt: user, Temperature: &extractionTemperature,
	})
	if err != nil {
		return nil, nil, err
	}
	nodes = unwrapItems(nodeRaw)

	// Known entities: unique values of the config's entity id field, in
	// first-seen order (mirrors _struct_extract_hypergraph's known_keys).
	idField := EntityIDField(cfg.ParserConfig)
	var known []string
	for _, n := range nodes {
		v := strings.TrimSpace(stringOf(n[idField]))
		if v == "" || containsString(known, v) {
			continue
		}
		known = append(known, v)
	}

	if strings.TrimSpace(edgePromptTmpl) == "" {
		return nodes, nil, nil
	}
	edgeRaw, err := common.GenJSON(ctx, deps.Chat, common.ChatRequest{
		LLMID: cfg.LLMID, SystemPrompt: fillKnownNodes(edgePromptTmpl, known), UserPrompt: user, Temperature: &extractionTemperature,
	})
	if err != nil {
		return nil, nil, err
	}
	return nodes, unwrapItems(edgeRaw), nil
}

// unwrapItems mirrors _struct_unwrap_items: the contract is
// {"items": [{...}, ...]}; a top-level array is tolerated defensively.
// GenJSON already guarantees a JSON object, so the list form only appears
// under "items". Non-object entries are dropped.
func unwrapItems(raw map[string]any) []map[string]any {
	if raw == nil {
		return nil
	}
	arr, ok := raw["items"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// payloadChunkIDs mirrors _struct_payload_chunk_ids: keep only model-selected
// chunk IDs that belong to the current batch; fall back to all batch ids when
// the model returned none that qualify.
func payloadChunkIDs(payload map[string]any, batchIDs []string) []string {
	var rawIDs []string
	switch v := payload["source_chunk_ids"].(type) {
	case string:
		rawIDs = []string{v}
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				rawIDs = append(rawIDs, s)
			}
		}
	case []string:
		rawIDs = v
	}
	allowed := make(map[string]bool, len(batchIDs))
	for _, id := range batchIDs {
		allowed[id] = true
	}
	var selected []string
	seen := map[string]bool{}
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if allowed[id] && !seen[id] {
			selected = append(selected, id)
			seen[id] = true
		}
	}
	if len(selected) == 0 {
		return append([]string{}, batchIDs...)
	}
	return selected
}

// payloadDescription mirrors _struct_payload_description: concat the string
// values of every field (lists flattened) with single spaces. Go maps lose
// JSON insertion order, so keys are sorted to keep the embedding input (and
// therefore the row vector) deterministic across runs.
func payloadDescription(payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	appendValue := func(v any) {
		s := strings.TrimSpace(stringOf(v))
		if s != "" {
			parts = append(parts, s)
		}
	}
	for _, k := range keys {
		switch v := payload[k].(type) {
		case []any:
			for _, item := range v {
				appendValue(item)
			}
		case []string:
			for _, item := range v {
				appendValue(item)
			}
		default:
			appendValue(v)
		}
	}
	return strings.Join(parts, " ")
}

// payloadJSON serialises a payload the way Python's json.dumps(ensure_ascii=
// False) does: Go's json.Marshal escapes <, > and & to < etc., which
// would corrupt entity names containing those characters, so HTML escaping is
// disabled. Keys are alphabetically sorted (map marshal), giving a canonical,
// hash-stable form.
func payloadJSON(payload map[string]any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "{}"
	}
	return strings.TrimSpace(b.String())
}

// parsePayload is the inverse of payloadJSON (nil on malformed content).
func parsePayload(content string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return nil
	}
	return m
}

// buildRows converts extracted node/edge payloads into entity/relation
// products, mirroring _struct_process_batch: embedding input is
// payloadDescription(payload); content is the payload JSON (Python's
// content_with_weight); source_chunk_ids are the model's picks filtered to
// the batch; the stable row id hashes (content, doc_id, template_id).
func buildRows(ctx context.Context, deps common.Deps, cfg CompileConfig, nodes, edges []map[string]any, batchIDs []string) ([]common.Product, error) {
	srcField, tgtField := RelationMemberFields(cfg.ParserConfig)

	type spec struct {
		kind    string // "entity" | "relation"
		payload map[string]any
	}
	specs := make([]spec, 0, len(nodes)+len(edges))
	for _, p := range nodes {
		specs = append(specs, spec{kind: "entity", payload: p})
	}
	for _, p := range edges {
		specs = append(specs, spec{kind: "relation", payload: p})
	}
	if len(specs) == 0 {
		return nil, nil
	}

	texts := make([]string, len(specs))
	for i, s := range specs {
		texts[i] = payloadDescription(s.payload)
	}
	vectors, err := deps.Embed.Encode(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(specs) {
		return nil, fmt.Errorf("knowledge_compiler: embedding count mismatch (%d vs %d)", len(vectors), len(specs))
	}

	rows := make([]common.Product, 0, len(specs))
	for i, s := range specs {
		content := payloadJSON(s.payload)
		idParts := []string{content, cfg.DocID}
		if cfg.TemplateID != "" {
			idParts = append(idParts, cfg.TemplateID)
		}
		meta := map[string]any{
			"kind":             s.kind,
			"source_chunk_ids": payloadChunkIDs(s.payload, batchIDs),
			"mention_count":    1,
		}
		if s.kind == "entity" {
			if name := entityName(s.payload); name != "" {
				meta["name"] = name
			}
			if typ := strings.TrimSpace(stringOf(s.payload["type"])); typ != "" {
				meta["entity_type"] = typ
			} else {
				meta["entity_type"] = "other"
			}
			if desc := strings.TrimSpace(stringOf(s.payload["description"])); desc != "" {
				meta["description"] = desc
			}
		} else {
			if from := relationEndpoint(s.payload, srcField, "source", "src", "from"); from != "" {
				meta["from"] = from
			}
			if to := relationEndpoint(s.payload, tgtField, "target", "tgt", "to"); to != "" {
				meta["to"] = to
			}
			if typ := strings.TrimSpace(stringOf(s.payload["type"])); typ != "" {
				meta["relation_type"] = typ
			}
		}
		rows = append(rows, common.Product{
			ID:       common.StableRowID(idParts...),
			DocID:    cfg.DocID,
			TenantID: cfg.TenantID,
			Variant:  cfg.Variant,
			Content:  content,
			Vector:   vectors[i],
			Meta:     meta,
		})
	}
	return rows, nil
}

// entityName mirrors _struct_graph_entity's name resolution
// (name → text → term → title, "-1" sentinel rejected).
func entityName(payload map[string]any) string {
	for _, k := range []string{"name", "text", "term", "title"} {
		if s := strings.TrimSpace(stringOf(payload[k])); s != "" && s != "-1" {
			return s
		}
	}
	return ""
}

// relationEndpoint resolves a relation's endpoint: the config-declared member
// field first, then the conventional aliases (mirrors _struct_graph_relation
// combined with _struct_relation_member_fields).
func relationEndpoint(payload map[string]any, declared string, aliases ...string) string {
	if declared != "" {
		if s := strings.TrimSpace(stringOf(payload[declared])); s != "" && s != "-1" {
			return s
		}
	}
	for _, k := range aliases {
		if s := strings.TrimSpace(stringOf(payload[k])); s != "" && s != "-1" {
			return s
		}
	}
	return ""
}

// stringOf renders a scalar payload value; maps/slices render as "" (they are
// not scalar text).
func stringOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%v", x), ".0")
	case bool:
		return fmt.Sprintf("%v", x)
	default:
		return ""
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
