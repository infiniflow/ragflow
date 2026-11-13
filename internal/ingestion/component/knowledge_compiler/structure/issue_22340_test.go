package structure

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestIssue22340_UnwrapItems verifies that unwrapItems supports LLM responses
// that place entities/relations under "relationships", "relations", "edges",
// or raw string arrays instead of strictly requiring "items".
func TestIssue22340_UnwrapItems(t *testing.T) {
	// 1. "relationships" key
	rawRel := map[string]any{
		"relationships": []any{
			map[string]any{
				"from_entity": "WO-001",
				"relation":    "repairs",
				"to_entity":   "Pump-001",
			},
			map[string]any{
				"from_entity": "Pump-001",
				"relation":    "uses",
				"to_entity":   "Bearing-001",
			},
		},
	}
	rels := unwrapItems(rawRel)
	if len(rels) != 2 {
		t.Fatalf("unwrapItems(relationships): got %d items, want 2", len(rels))
	}
	if rels[0]["from_entity"] != "WO-001" || rels[0]["to_entity"] != "Pump-001" {
		t.Errorf("unwrapItems(relationships) item 0: got %#v", rels[0])
	}

	// 2. "entities" key as string list
	rawEnt := map[string]any{
		"entities": []any{"WO-001", "Pump-001", "Bearing-001"},
	}
	ents := unwrapItems(rawEnt)
	if len(ents) != 3 {
		t.Fatalf("unwrapItems(entities string slice): got %d items, want 3", len(ents))
	}
	if ents[0]["name"] != "WO-001" || ents[1]["name"] != "Pump-001" || ents[2]["name"] != "Bearing-001" {
		t.Errorf("unwrapItems(entities string slice): got %#v", ents)
	}

	// 3. "edges" key
	rawEdges := map[string]any{
		"edges": []any{
			map[string]any{"source": "A", "target": "B", "type": "connects"},
		},
	}
	edges := unwrapItems(rawEdges)
	if len(edges) != 1 || edges[0]["source"] != "A" {
		t.Fatalf("unwrapItems(edges): got %#v", edges)
	}
}

// TestIssue22340_EndpointAndTypeAliases verifies that relationEndpoint and relationType
// resolve common NLP and LLM field names like from_entity, to_entity, relation, etc.
func TestIssue22340_EndpointAndTypeAliases(t *testing.T) {
	payload := map[string]any{
		"from_entity": "WO-001",
		"to_entity":   "Pump-001",
		"relation":    "repairs",
	}

	from := relationEndpoint(payload, "", "source", "src", "from", "from_entity", "source_entity", "head", "start", "subject", "origin")
	if from != "WO-001" {
		t.Errorf("relationEndpoint from_entity: got %q, want \"WO-001\"", from)
	}

	to := relationEndpoint(payload, "", "target", "tgt", "to", "to_entity", "target_entity", "tail", "end", "object", "destination")
	if to != "Pump-001" {
		t.Errorf("relationEndpoint to_entity: got %q, want \"Pump-001\"", to)
	}

	typ := relationType(payload)
	if typ != "repairs" {
		t.Errorf("relationType relation: got %q, want \"repairs\"", typ)
	}
}

// issue22340MockChat simulates an LLM that returns custom relationships JSON
// as reported in issue #22340.
type issue22340MockChat struct{}

func (m *issue22340MockChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	if strings.Contains(req.SystemPrompt, "## Known Entities:") {
		// Relation extraction returns "relationships" array with from_entity / to_entity
		return &common.ChatResponse{
			Content: `{
				"relationships": [
					{
						"from_entity": "WO-001",
						"relation": "repairs",
						"to_entity": "Pump-001"
					},
					{
						"from_entity": "Pump-001",
						"relation": "uses",
						"to_entity": "Bearing-001"
					}
				]
			}`,
		}, nil
	}
	// Entity extraction returns entities
	return &common.ChatResponse{
		Content: `{
			"items": [
				{"name": "WO-001", "type": "work_order"},
				{"name": "Pump-001", "type": "equipment"},
				{"name": "Bearing-001", "type": "spare_part"}
			]
		}`,
	}, nil
}

// TestIssue22340_EndToEndRelationExtraction verifies that the extraction pipeline
// extracts relations and that FilterOrphanRelations retains them rather than reporting 0 relations.
func TestIssue22340_EndToEndRelationExtraction(t *testing.T) {
	ctx := context.Background()
	deps := common.Deps{
		Chat:     &issue22340MockChat{},
		Embed:    constEmbedder{dim: 4},
		TenantID: "tenant-1",
	}
	cfg := CompileConfig{
		LLMID:    "test-llm",
		Type:     TypeHypergraph,
		TenantID: "tenant-1",
		DocID:    "doc-1",
		Variant:  common.VariantStructure,
		ParserConfig: map[string]any{
			"kind": "knowledge_graph",
			"entity": map[string]any{
				"fields": []any{
					map[string]any{"type": "equipment"},
					map[string]any{"type": "work_order"},
					map[string]any{"type": "spare_part"},
				},
			},
			"relation": map[string]any{
				"fields": []any{
					map[string]any{"type": "repairs"},
					map[string]any{"type": "uses"},
				},
			},
		},
	}

	nodePrompt, edgePromptTmpl := HypergraphPrompts(cfg.ParserConfig, "en")
	nodes, edges, err := extractHypergraph(ctx, deps, cfg, nodePrompt, edgePromptTmpl, "WO-001 repairs Pump-001 using Bearing-001")
	if err != nil {
		t.Fatalf("extractHypergraph: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("extractHypergraph nodes: got %d, want 3", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("extractHypergraph edges: got %d, want 2 (bug was returning 0)", len(edges))
	}

	rows, err := buildRows(ctx, deps, cfg, nodes, edges, []string{"c1"})
	if err != nil {
		t.Fatalf("buildRows: %v", err)
	}

	var relationRows []common.Product
	var entityRows []common.Product
	for _, r := range rows {
		switch r.Meta["kind"] {
		case "relation":
			relationRows = append(relationRows, r)
		case "entity":
			entityRows = append(entityRows, r)
		}
	}

	if len(entityRows) != 3 {
		t.Fatalf("entity rows: got %d, want 3", len(entityRows))
	}
	if len(relationRows) != 2 {
		t.Fatalf("relation rows: got %d, want 2", len(relationRows))
	}

	// Verify relation metadata endpoints are populated
	if relationRows[0].Meta["from"] != "WO-001" || relationRows[0].Meta["to"] != "Pump-001" {
		t.Errorf("relation 0 meta endpoints: got from=%v to=%v", relationRows[0].Meta["from"], relationRows[0].Meta["to"])
	}
	if relationRows[0].Meta["relation_type"] != "repairs" {
		t.Errorf("relation 0 relation_type: got %v, want repairs", relationRows[0].Meta["relation_type"])
	}

	// Verify FilterOrphanRelations preserves both relations (previously dropped to 0)
	filtered := common.FilterOrphanRelations(rows)
	var preservedRelations int
	for _, p := range filtered {
		if p.Meta["kind"] == "relation" {
			preservedRelations++
		}
	}

	if preservedRelations != 2 {
		t.Fatalf("FilterOrphanRelations preserved relations: got %d, want 2 (Issue #22340 0 relations bug)", preservedRelations)
	}
}
