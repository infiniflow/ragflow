package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// ---- mocks ----

// graphChat answers the three LLM call shapes of the structure variant:
// node extraction, edge extraction, and merge judging. It emits items for
// every fixture phrase present in the user prompt so assertions hold whether
// the chunks pack into one batch or several.
type graphChat struct {
	mergeCalls int
	nodeCalls  int
	edgeCalls  int
}

var graphFixtures = []struct {
	phrase    string
	entities  []map[string]any
	relations []map[string]any
}{
	{
		phrase: "Alpha is a Beta",
		entities: []map[string]any{
			{"type": "letter", "name": "Alpha", "description": "the letter Alpha"},
			{"type": "letter", "name": "Beta", "description": "the letter Beta"},
		},
		relations: []map[string]any{
			{"type": "linked", "source": "Alpha", "target": "Beta", "description": "Alpha is a Beta"},
		},
	},
	{
		phrase: "Beta related to Gamma",
		entities: []map[string]any{
			{"type": "letter", "name": "Beta", "description": "the letter Beta"},
			{"type": "letter", "name": "Gamma", "description": "the letter Gamma"},
		},
		relations: []map[string]any{
			{"type": "linked", "source": "Beta", "target": "Gamma", "description": "Beta related to Gamma"},
		},
	},
	{
		phrase: "Al partners with Gamma",
		entities: []map[string]any{
			{"type": "letter", "name": "Al", "description": "short for Alpha"},
			{"type": "letter", "name": "Gamma", "description": "the letter Gamma"},
		},
		relations: []map[string]any{
			{"type": "partners_with", "source": "Al", "target": "Gamma", "description": "Al partners with Gamma"},
		},
	},
	{
		phrase: "Alpha also links Gamma",
		entities: []map[string]any{
			{"type": "letter", "name": "Alpha", "description": "the letter Alpha"},
			{"type": "letter", "name": "Gamma", "description": "the letter Gamma"},
		},
		relations: []map[string]any{
			{"type": "linked", "source": "Alpha", "target": "Gamma", "description": "Alpha also links Gamma"},
		},
	},
}

func itemsJSON(items []map[string]any, chunkIDs []string) string {
	for _, item := range items {
		item["source_chunk_ids"] = chunkIDs
	}
	b, _ := json.Marshal(map[string]any{"items": items})
	return string(b)
}

func chunkIDsFromPrompt(prompt string) []string {
	var ids []string
	for _, id := range []string{"c1", "c2", "c3"} {
		if strings.Contains(prompt, "[CHUNK_ID: "+id+"]") {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []string{"c1"}
	}
	return ids
}

func (m *graphChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	ids := chunkIDsFromPrompt(req.UserPrompt)
	switch {
	case strings.HasPrefix(req.UserPrompt, "Pair "):
		// Batched merge judge: one call covers every (Item A, Item B) pair.
		m.mergeCalls++
		pairs := parseBatchPairs(req.UserPrompt)
		var results []string
		for _, p := range pairs {
			if sameLogicalItem(p.a, p.b) {
				results = append(results, fmt.Sprintf(`{"index":%d,"duplicated":true,"merged":%s}`, p.index, payloadJSON(p.a)))
			} else {
				results = append(results, fmt.Sprintf(`{"index":%d,"duplicated":false,"merged":null}`, p.index))
			}
		}
		return &common.ChatResponse{Content: fmt.Sprintf(`{"pairs":[%s]}`, strings.Join(results, ","))}, nil
	case strings.HasPrefix(req.UserPrompt, "Item A (existing):"):
		m.mergeCalls++
		a, b := parseMergeItems(req.UserPrompt)
		if sameLogicalItem(a, b) {
			return &common.ChatResponse{Content: fmt.Sprintf(`{"duplicated":true,"merged":%s}`, payloadJSON(a))}, nil
		}
		return &common.ChatResponse{Content: `{"duplicated":false,"merged":null}`}, nil
	case strings.Contains(req.SystemPrompt, "strict-chain constraint"):
		// Chain correction: keep Alpha→Beta, drop the fan-out partner.
		return &common.ChatResponse{Content: `{"keep":[{"from":"Alpha","to":"Beta"}]}`}, nil
	case strings.Contains(req.SystemPrompt, "## Known Entities:"):
		m.edgeCalls++
		var rels []map[string]any
		for _, f := range graphFixtures {
			if strings.Contains(req.UserPrompt, f.phrase) {
				for _, r := range f.relations {
					rels = append(rels, copyPayload(r))
				}
			}
		}
		return &common.ChatResponse{Content: itemsJSON(rels, ids)}, nil
	default:
		m.nodeCalls++
		var ents []map[string]any
		for _, f := range graphFixtures {
			if strings.Contains(req.UserPrompt, f.phrase) {
				for _, e := range f.entities {
					ents = append(ents, copyPayload(e))
				}
			}
		}
		return &common.ChatResponse{Content: itemsJSON(ents, ids)}, nil
	}
}

func copyPayload(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		out[k] = v
	}
	return out
}

// parseMergeItems extracts the Item A / Item B payload JSONs from a merge
// judge user prompt.
func parseMergeItems(prompt string) (map[string]any, map[string]any) {
	body := strings.TrimPrefix(prompt, "Item A (existing):\n")
	parts := strings.SplitN(body, "\n\nItem B (incoming):\n", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	return parsePayload(parts[0]), parsePayload(parts[1])
}

// sameLogicalItem mirrors what a reasonable judge would decide on the
// fixtures: entities are duplicates when their names match (or the known
// Al≡Alpha alias pair); relations when source+target+type all match.
func parseBatchPairs(prompt string) []struct {
	index int
	a, b  map[string]any
} {
	sections := strings.Split(prompt, "\nPair ")
	var pairs []struct {
		index int
		a, b  map[string]any
	}
	for i, s := range sections {
		if i == 0 {
			// First section may start with "Pair 0:" without a leading "\n".
			if !strings.HasPrefix(s, "Pair ") {
				continue
			}
		}
		// Drop the leading "N:\n".
		rest := s
		if idx := strings.Index(rest, ":\n"); idx >= 0 {
			rest = rest[idx+2:]
		}
		aBody, bBody, ok := strings.Cut(rest, "\n\nItem B (incoming):\n")
		if !ok {
			continue
		}
		aBody = strings.TrimPrefix(aBody, "Item A (existing):\n")
		pairs = append(pairs, struct {
			index int
			a, b  map[string]any
		}{index: i, a: parsePayload(aBody), b: parsePayload(bBody)})
	}
	return pairs
}

func sameLogicalItem(a, b map[string]any) bool {
	if a == nil || b == nil {
		return false
	}
	aName, bName := entityName(a), entityName(b)
	if aName != "" && bName != "" {
		if aName == bName {
			return true
		}
		pair := map[string]bool{aName: true, bName: true}
		return pair["Al"] && pair["Alpha"]
	}
	as, bs := relationEndpoint(a, "", "source"), relationEndpoint(b, "", "source")
	at, bt := relationEndpoint(a, "", "target"), relationEndpoint(b, "", "target")
	return as != "" && as == bs && at == bt && stringOf(a["type"]) == stringOf(b["type"])
}

// hashEmbedder is content-deterministic: identical text => identical (unit)
// vector => cosine 1.0. Near-identical texts do NOT collide, so only exact
// duplicate payloads reach the LLM judge.
type hashEmbedder struct{ dim int }

func (m hashEmbedder) Dimensions() int { return m.dim }

func (m hashEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, m.dim)
		for j := 0; j < m.dim; j++ {
			h := xxhash.Sum64String(fmt.Sprintf("%d:%s", j, t))
			v[j] = float32(int64(h%(1<<20))-(1<<19)) / float32(1<<19)
		}
		var s float64
		for _, x := range v {
			s += float64(x) * float64(x)
		}
		if s = math.Sqrt(s); s > 0 {
			for k := range v {
				v[k] = float32(float64(v[k]) / s)
			}
		}
		out[i] = v
	}
	return out, nil
}

// constEmbedder maps every text to the same vector, so every pair in a dedup
// bucket reaches the LLM judge (used to exercise alias merges deterministically).
type constEmbedder struct{ dim int }

func (m constEmbedder) Dimensions() int { return m.dim }
func (m constEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	v := make([]float32, m.dim)
	for i := range v {
		v[i] = 1
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = v
	}
	return out, nil
}

func graphParserConfig() map[string]any {
	return map[string]any{
		"kind": "graph",
		"entity": map[string]any{"fields": []any{
			map[string]any{"type": "letter", "description": "a single letter entity"},
		}},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "linked", "description": "a link between letters"},
			map[string]any{"type": "partners_with", "description": "a partnership"},
		}},
	}
}

func testParam() common.Param {
	p := common.Param{}.Defaults()
	p.Variant = common.VariantStructure
	p.SimilarityThreshold = 0.99
	p.MaxWorkers = 1
	return p
}

// ---- prompt alignment tests ----

func TestHypergraphPromptsTemplateShape(t *testing.T) {
	cfg := map[string]any{
		"kind":         "graph",
		"guideline":    map[string]any{"target": "Extract the letter graph.", "rules_for_entities": "Be precise.", "rules_for_relations": "Only stated links.", "rules_for_time": "As of {observation_time}."},
		"global_rules": "No inventions.",
		"entity": map[string]any{
			"description": "Letter entities.",
			"fields":      []any{map[string]any{"type": "letter", "description": "a letter", "rule": "single glyph"}},
		},
		"relation": map[string]any{
			"description": "Letter links.",
			"fields":      []any{map[string]any{"type": "linked", "description": "a link"}},
		},
	}
	node, edge := HypergraphPrompts(cfg, "en")

	for _, want := range []string{
		"# Role and Task:\nExtract the letter graph.",
		"## Global Rules:\nNo inventions.",
		"## Entity Extraction Rules:\nBe precise.",
		"## Entity Description:\nLetter entities.",
		"## Entity Fields:\n- type: letter\n  description: a letter\n  rule: single glyph",
		`Auto-type: "hypergraph". `,
		"Return JSON only, no commentary.",
		`"name": "<exact extracted item text>"`,
		`"source_chunk_ids": ["<source chunk id>", ...]`,
	} {
		if !strings.Contains(node, want) {
			t.Errorf("node prompt missing %q\n---\n%s", want, node)
		}
	}
	if strings.Contains(node, "Items must be unique.") {
		t.Errorf("hypergraph (not set) must not demand uniqueness")
	}

	for _, want := range []string{
		"## Relation Extraction Rules:\nOnly stated links.",
		"## Time Rules:\nAs of ",
		"## Relation Description:\nLetter links.",
		"## Relation Fields:\n- type: linked\n  description: a link",
		"## Known Entities:\n{known_nodes}",
		"Only create relations between entities listed in 'Known Entities'.",
		`"source": "<known entity name>"`,
	} {
		if !strings.Contains(edge, want) {
			t.Errorf("edge prompt missing %q\n---\n%s", want, edge)
		}
	}
	if strings.Contains(edge, "{observation_time}") {
		t.Errorf("observation_time placeholder was not substituted")
	}
}

func TestHypergraphPromptsSetUniquenessAndNoRelations(t *testing.T) {
	cfg := map[string]any{
		"compile_type": "set",
		"entity":       map[string]any{"fields": []any{map[string]any{"type": "letter"}}},
	}
	node, edge := HypergraphPrompts(cfg, "en")
	if !strings.Contains(node, "Items must be unique. ") {
		t.Errorf("set kind must demand uniqueness:\n%s", node)
	}
	if !strings.Contains(node, `Auto-type: "set". `) {
		t.Errorf("set Auto-type missing:\n%s", node)
	}
	if edge != "" {
		t.Errorf("config without relations must skip the edge stage, got:\n%s", edge)
	}
	if got := InferType(cfg); got != TypeSet {
		t.Errorf("InferType = %q, want set", got)
	}
	// "graph" / "knowledge_graph" aliases resolve to hypergraph.
	for _, alias := range []string{"graph", "knowledge_graph"} {
		if got := InferType(map[string]any{"kind": alias}); got != TypeHypergraph {
			t.Errorf("InferType(kind=%q) = %q, want hypergraph", alias, got)
		}
	}
	// Arbitrary kinds (timeline, page_index) are returned verbatim — Python
	// stamps them as the autotype/compile_kwd.
	if got := InferType(map[string]any{"kind": "timeline"}); got != Type("timeline") {
		t.Errorf("InferType(kind=timeline) = %q, want verbatim timeline", got)
	}
	// An unknown compile_type is NOT a compile kind and falls through to kind.
	if got := InferType(map[string]any{"compile_type": "timeline"}); got != TypeList {
		t.Errorf("InferType(compile_type=timeline) = %q, want list (falls through)", got)
	}
	if got := InferType(map[string]any{}); got != TypeList {
		t.Errorf("InferType(empty) = %q, want list", got)
	}
}

func TestFillKnownNodes(t *testing.T) {
	tmpl := "## Known Entities:\n{known_nodes}"
	if got := fillKnownNodes(tmpl, nil); !strings.Contains(got, "(none)") {
		t.Fatalf("empty known list must render (none): %q", got)
	}
	got := fillKnownNodes(tmpl, []string{"Alpha", "Beta"})
	if !strings.Contains(got, "- Alpha\n- Beta") {
		t.Fatalf("known list not rendered: %q", got)
	}
}

func TestExtractHypergraphSkipsEdgesWithoutEntities(t *testing.T) {
	chat := &graphChat{}
	nodes, edges, err := extractHypergraph(context.Background(), common.Deps{Chat: chat}, CompileConfig{LLMID: "llm", ParserConfig: map[string]any{"entity": map[string]any{"id": "name"}}}, "node", "## Known Entities: {known_nodes}", "unrelated text")
	if err != nil {
		t.Fatalf("extractHypergraph: %v", err)
	}
	if len(nodes) != 0 || len(edges) != 0 {
		t.Fatalf("empty entity batch returned nodes=%v edges=%v", nodes, edges)
	}
	if chat.nodeCalls != 1 || chat.edgeCalls != 0 {
		t.Fatalf("LLM calls = node:%d edge:%d, want node:1 edge:0", chat.nodeCalls, chat.edgeCalls)
	}
}

// ---- payload helpers ----

func TestPayloadChunkIDs(t *testing.T) {
	payload := map[string]any{"source_chunk_ids": []any{"c1", "nope", "c1"}}
	got := payloadChunkIDs(payload, []string{"c1", "c2"})
	if len(got) != 1 || got[0] != "c1" {
		t.Fatalf("filter+dedupe failed: %v", got)
	}
	// Fallback: none of the model's ids belong to the batch -> all batch ids.
	payload = map[string]any{"source_chunk_ids": []any{"nope"}}
	got = payloadChunkIDs(payload, []string{"c1", "c2"})
	if len(got) != 2 {
		t.Fatalf("fallback to batch ids failed: %v", got)
	}
	// A bare string is accepted (mirrors Python).
	payload = map[string]any{"source_chunk_ids": "c2"}
	got = payloadChunkIDs(payload, []string{"c1", "c2"})
	if len(got) != 1 || got[0] != "c2" {
		t.Fatalf("string form failed: %v", got)
	}
}

func TestPayloadDescriptionSortedAndFlattened(t *testing.T) {
	payload := map[string]any{
		"name":             "Beta",
		"type":             "letter",
		"mention_count":    1,
		"source_chunk_ids": []any{"c1", "c2"},
	}
	got := payloadDescription(payload)
	// Keys are sorted for determinism, and the bookkeeping keys are excluded:
	// source_chunk_ids are opaque hex that would tokenize into garbage terms
	// and mention_count is a number every row shares (mirrors Python's
	// _STRUCT_INDEX_EXCLUDED_KEYS).
	if got != "Beta letter" {
		t.Fatalf("payloadDescription = %q, want %q", got, "Beta letter")
	}
}

// ---- Run: extraction + LLM dedup + graph ----

func TestStructureRunGraphKind(t *testing.T) {
	deps := common.Deps{Chat: &graphChat{}, Embed: hashEmbedder{dim: 16}, TenantID: "t1", DatasetID: "d1"}
	param := testParam()
	inputs := common.Inputs{
		DocID: "doc1",
		Chunks: []common.Chunk{
			{ID: "c1", Text: "Alpha is a Beta."},
			{ID: "c2", Text: "Beta related to Gamma."},
		},
		VariantSpecific: map[string]any{"parser_config": graphParserConfig()},
	}

	out, err := Run(context.Background(), deps, param, inputs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var entities, relations int
	var betaProduct *common.Product
	for i, p := range out.Products {
		switch p.Meta["kind"] {
		case "entity":
			entities++
			if p.Meta["name"] == "Beta" {
				betaProduct = &out.Products[i]
			}
			if p.Meta["entity_type"] != "letter" {
				t.Errorf("entity %v missing entity_type stamp", p.Meta["name"])
			}
			if p.Meta["compile_kwd"] != "hypergraph" {
				t.Errorf("entity row compile_kwd = %v, want hypergraph", p.Meta["compile_kwd"])
			}
		case "relation":
			relations++
		case "graph":
			t.Errorf("unexpected graph blob product: no compact graph row is written anymore")
		}
	}
	if entities != 3 || relations != 2 {
		t.Fatalf("products = %d entities + %d relations, want 3+2 (no graph blob)", entities, relations)
	}
	if out.DuplicatesDropped != 1 {
		t.Fatalf("DuplicatesDropped = %d, want 1 (the cross-chunk Beta)", out.DuplicatesDropped)
	}
	if betaProduct == nil {
		t.Fatal("no Beta entity product")
	}
	ids := metaStrings(betaProduct.Meta, "source_chunk_ids")
	if len(ids) != 2 {
		t.Fatalf("Beta provenance = %v, want {c1,c2} unioned through the merge", ids)
	}
	// The merged entity keeps the LLM-merged payload content (parseable JSON).
	if parsePayload(betaProduct.Content) == nil {
		t.Fatalf("entity content is not payload JSON: %q", betaProduct.Content)
	}
}

func TestStructureListKindSkipsRelations(t *testing.T) {
	deps := common.Deps{Chat: &graphChat{}, Embed: hashEmbedder{dim: 16}, TenantID: "t1", DatasetID: "d1"}
	param := testParam()
	inputs := common.Inputs{
		DocID:  "doc1",
		Chunks: []common.Chunk{{ID: "c1", Text: "Alpha is a Beta."}},
		// No parser_config: InferType yields "list", so no edge stage runs.
	}
	out, err := Run(context.Background(), deps, param, inputs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var relations int
	for _, p := range out.Products {
		if p.Meta["kind"] == "relation" {
			relations++
		}
		if p.Meta["compile_kwd"] != "list" {
			t.Errorf("compile_kwd = %v, want list", p.Meta["compile_kwd"])
		}
	}
	if relations != 0 {
		t.Fatalf("list kind must not extract relations, got %d", relations)
	}
}

func TestStructureAliasRewrite(t *testing.T) {
	// constEmbedder makes every pair a judge candidate; the mock judge merges
	// Al into Alpha (the known alias pair), so the relation Al→Gamma must be
	// rewritten to Alpha→Gamma by the alias pass.
	deps := common.Deps{Chat: &graphChat{}, Embed: constEmbedder{dim: 4}, TenantID: "t1", DatasetID: "d1"}
	param := testParam()
	inputs := common.Inputs{
		DocID: "doc1",
		Chunks: []common.Chunk{
			{ID: "c1", Text: "Alpha is a Beta."},
			{ID: "c2", Text: "Al partners with Gamma."},
		},
		VariantSpecific: map[string]any{"parser_config": graphParserConfig()},
	}
	out, err := Run(context.Background(), deps, param, inputs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var sawAlEntity, sawRewritten bool
	for _, p := range out.Products {
		if p.Meta["kind"] == "entity" && p.Meta["name"] == "Al" {
			sawAlEntity = true
		}
		if p.Meta["kind"] == "relation" {
			payload := parsePayload(p.Content)
			if payload == nil {
				t.Fatalf("relation content not payload JSON: %q", p.Content)
			}
			if relationEndpoint(payload, "", "source") == "Alpha" && relationEndpoint(payload, "", "target") == "Gamma" {
				sawRewritten = true
				if p.Meta["from"] != "Alpha" || p.Meta["to"] != "Gamma" {
					t.Errorf("rewritten relation meta from/to = %v/%v", p.Meta["from"], p.Meta["to"])
				}
			}
		}
	}
	if sawAlEntity {
		t.Errorf("Al entity should have been merged into Alpha")
	}
	if !sawRewritten {
		t.Errorf("relation Al→Gamma was not rewritten to Alpha→Gamma")
	}
}

func TestStructureSkipsSelfLoopRelations(t *testing.T) {
	deps := common.Deps{Embed: hashEmbedder{dim: 4}}
	cfg := CompileConfig{
		TenantID:     "t1",
		DocID:        "d1",
		Type:         TypeHypergraph,
		ParserConfig: graphParserConfig(),
	}
	rows, err := buildRows(context.Background(), deps, cfg, nil, []map[string]any{
		{"type": "linked", "source": "Root", "target": "Root"},
		{"type": "linked", "source": "Root", "target": "Child"},
	}, []string{"c1"})
	if err != nil {
		t.Fatalf("buildRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want only the non-self relation", len(rows))
	}

	filtered := filterSelfLoopRelations(append(rows, common.Product{
		Content: payloadJSON(map[string]any{"source": "Child", "target": "Child", "type": "linked"}),
		Meta:    map[string]any{"kind": "relation", "from": "Child", "to": "Child"},
	}))
	if len(filtered) != 1 {
		t.Fatalf("filtered rows = %d, want self-loop removed", len(filtered))
	}
}

// ---- merge unit tests ----

func TestLLMMergeDeciderContracts(t *testing.T) {
	chat := &graphChat{}
	d := NewLLMMergeDecider(chat, "llm1", hashEmbedder{dim: 8}, 0.99)

	// Below threshold: no LLM call, keep both.
	before := chat.mergeCalls
	dec, _, err := d.Decide(context.Background(), common.Product{}, common.Product{}, 0.5)
	if err != nil || dec != DecisionKeepBoth || chat.mergeCalls != before {
		t.Fatalf("below-threshold pair must be kept without an LLM call: dec=%v err=%v", dec, err)
	}

	existing := common.Product{
		ID:      "row-alpha",
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Alpha", "description": "the letter Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Alpha", "source_chunk_ids": []string{"c1"}},
	}
	incoming := common.Product{
		ID:      "row-alpha-2",
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Al", "description": "short for Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Al", "source_chunk_ids": []string{"c2"}},
	}
	dec, merged, err := d.Decide(context.Background(), existing, incoming, 1.0)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec != DecisionMerge {
		t.Fatalf("alias pair must merge: dec=%v", dec)
	}
	if merged.ID != "row-alpha" {
		t.Errorf("merged row must preserve the existing id, got %q", merged.ID)
	}
	if got := entityName(parsePayload(merged.Content)); got != "Alpha" {
		t.Errorf("merged canonical name = %q, want Alpha", got)
	}
	if ids := metaStrings(merged.Meta, "source_chunk_ids"); len(ids) != 2 {
		t.Errorf("merged provenance = %v, want {c1,c2}", ids)
	}
	if aliases := d.Aliases(); aliases["Al"] != "Alpha" {
		t.Errorf("alias map = %v, want Al→Alpha", aliases)
	}
}

// TestLLMMergeDeciderDecideBatch locks the batched judge: every pair is
// judged in a single LLM call (one mergeCalls increment), and the verdict
// array is returned in input order with duplicated/merged fields set.
func TestLLMMergeDeciderDecideBatch(t *testing.T) {
	chat := &graphChat{}
	d := NewLLMMergeDecider(chat, "llm1", hashEmbedder{dim: 8}, 0.99)

	alpha := common.Product{
		ID:      "row-alpha",
		DocID:   "kb1",
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Alpha", "description": "the letter Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Alpha", "source_chunk_ids": []string{"c1"}},
	}
	al := common.Product{
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Al", "description": "short for Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Al", "source_chunk_ids": []string{"c2"}},
	}
	beta := common.Product{
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Beta", "description": "the letter Beta"}),
		Meta:    map[string]any{"kind": "entity", "name": "Beta", "source_chunk_ids": []string{"c3"}},
	}

	pairs := []MergePairInput{
		{Index: 0, Existing: alpha.Content, Incoming: al.Content},   // duplicated (Al≡Alpha)
		{Index: 1, Existing: alpha.Content, Incoming: beta.Content}, // distinct
	}
	before := chat.mergeCalls
	results, err := d.DecideBatch(context.Background(), pairs)
	if err != nil {
		t.Fatalf("DecideBatch: %v", err)
	}
	if chat.mergeCalls != before+1 {
		t.Errorf("batched judge made %d LLM calls, want 1", chat.mergeCalls-before)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].Duplicated || results[0].Merged == nil {
		t.Errorf("pair 0 (Al→Alpha) should be duplicated")
	}
	if results[1].Duplicated || results[1].Merged != nil {
		t.Errorf("pair 1 (Beta) should be distinct")
	}
	if results[0].Index != 0 || results[1].Index != 1 {
		t.Errorf("results must preserve input index order")
	}
}

// TestLLMMergeDeciderDecideBatchEmpty locks that an empty pair slice is a
// no-op and never invokes the LLM.
func TestLLMMergeDeciderDecideBatchEmpty(t *testing.T) {
	chat := &graphChat{}
	d := NewLLMMergeDecider(chat, "llm1", hashEmbedder{dim: 8}, 0.99)
	before := chat.mergeCalls
	out, err := d.DecideBatch(context.Background(), nil)
	if err != nil || out != nil {
		t.Fatalf("empty DecideBatch: err=%v out=%v", err, out)
	}
	if chat.mergeCalls != before {
		t.Errorf("empty DecideBatch must not call the LLM")
	}
}

// TestLLMMergeDeciderDecideBatchSplitsByTokenBudget locks that a tight token
// budget forces DecideBatch to issue several LLM calls (sub-batches) while
// still returning every verdict keyed by its original global index. This is
// the guard against overflowing the model's max_token on large candidate sets.
func TestLLMMergeDeciderDecideBatchSplitsByTokenBudget(t *testing.T) {
	chat := &graphChat{}
	d := NewLLMMergeDecider(chat, "llm1", hashEmbedder{dim: 8}, 0.99)
	submittedBatches := 0
	submittedChunks := 0
	d.SetSubmitter(func(ctx context.Context, jobs []func() error) error {
		submittedBatches++
		submittedChunks = len(jobs)
		for _, job := range jobs {
			if err := job(); err != nil {
				return err
			}
		}
		return nil
	})
	// Tiny model budget → one pair per sub-batch (exercises the split path).
	// Combined budget = 20 * 0.6 = 12 tokens, well under each ~40-token pair's
	// (input+output) estimate. Output budget disabled (0) → combined-budget-only
	// batching.
	d.SetMaxBatchTokens(20, 0)

	alpha := common.Product{
		ID:      "row-alpha",
		DocID:   "kb1",
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Alpha", "description": "the letter Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Alpha", "source_chunk_ids": []string{"c1"}},
	}
	al := common.Product{
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Al", "description": "short for Alpha"}),
		Meta:    map[string]any{"kind": "entity", "name": "Al", "source_chunk_ids": []string{"c2"}},
	}
	beta := common.Product{
		Content: payloadJSON(map[string]any{"type": "letter", "name": "Beta", "description": "the letter Beta"}),
		Meta:    map[string]any{"kind": "entity", "name": "Beta", "source_chunk_ids": []string{"c3"}},
	}

	pairs := []MergePairInput{
		{Index: 0, Existing: alpha.Content, Incoming: al.Content},   // duplicated (Al≡Alpha)
		{Index: 1, Existing: alpha.Content, Incoming: beta.Content}, // distinct
		{Index: 2, Existing: beta.Content, Incoming: alpha.Content}, // distinct
	}
	before := chat.mergeCalls
	results, err := d.DecideBatch(context.Background(), pairs)
	if err != nil {
		t.Fatalf("DecideBatch: %v", err)
	}
	// Budget=1 → one LLM call per pair.
	if chat.mergeCalls != before+3 {
		t.Errorf("token-split DecideBatch made %d LLM calls, want 3", chat.mergeCalls-before)
	}
	if submittedBatches != 1 || submittedChunks != 3 {
		t.Fatalf("token-split DecideBatch submitted %d batches/%d chunks, want one batch containing all chunks", submittedBatches, submittedChunks)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// Verdicts must stay keyed by the original global index, not re-indexed.
	wantDup := map[int]bool{0: true, 1: false, 2: false}
	for _, r := range results {
		if r.Duplicated != wantDup[r.Index] {
			t.Errorf("pair %d: duplicated=%v, want %v", r.Index, r.Duplicated, wantDup[r.Index])
		}
	}
}

func TestApplyMergeInvariants(t *testing.T) {
	existing := common.Product{
		Content: payloadJSON(map[string]any{"type": "linked", "source": "Alpha", "target": "Beta"}),
		Meta:    map[string]any{"kind": "relation"},
	}
	merged := map[string]any{"type": "linked", "source": "WRONG", "target": "ALSO_WRONG", "description": "richer"}
	out := applyMergeInvariants(existing, merged)
	if out["source"] != "Alpha" || out["target"] != "Beta" {
		t.Fatalf("relation endpoints must be pinned to the existing payload: %v", out)
	}
	if out["description"] != "richer" {
		t.Fatalf("non-endpoint fields must survive the merge: %v", out)
	}
}

func TestResolveAliasToleratesCycles(t *testing.T) {
	aliases := map[string]string{"A": "B", "B": "A"}
	if got := resolveAlias("A", aliases); got != "A" && got != "B" {
		t.Fatalf("cycle must terminate: %q", got)
	}
	if got := resolveAlias("x", map[string]string{"x": "y", "y": "z"}); got != "z" {
		t.Fatalf("chain resolution failed: %q", got)
	}
}

func TestCosineDecider(t *testing.T) {
	d := CosineDecider{Threshold: 0.99}
	got, _, err := d.Decide(context.Background(), common.Product{}, common.Product{}, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if got != DecisionDropIncoming {
		t.Fatalf("expected drop at 1.0, got %v", got)
	}
	got, _, _ = d.Decide(context.Background(), common.Product{}, common.Product{}, 0.5)
	if got != DecisionKeepBoth {
		t.Fatalf("expected keep at 0.5, got %v", got)
	}
}

func TestMergeEntitiesByNameIgnoresType(t *testing.T) {
	existing := common.Product{
		Content: `{"name":"Engine","type":"component","description":"mechanical part"}`,
		Meta:    map[string]any{"kind": "entity", "name": "Engine", "entity_type": "component"},
	}
	incoming := common.Product{
		Content: `{"name":"engine","type":"system","description":"controls the vehicle"}`,
		Meta:    map[string]any{"kind": "entity", "name": "engine", "entity_type": "system"},
	}
	if !sameEntityName(existing, incoming) {
		t.Fatal("entities with the same name should share one identity regardless of type")
	}
	merged := mergeEntitiesByName(existing, incoming)
	if merged["name"] != "Engine" || merged["type"] != "component" {
		t.Fatalf("merge should preserve the first entity identity: %v", merged)
	}
	if !strings.Contains(stringOf(merged["description"]), "controls the vehicle") {
		t.Fatalf("merge should retain both descriptions: %v", merged)
	}
}

func TestGroupedDeduperMergesSameNameBeforeVectorSimilarity(t *testing.T) {
	chat := &graphChat{}
	deduper := NewGroupedDeduper(NewLLMMergeDecider(chat, "llm", constEmbedder{dim: 2}, 0.99))
	rows := []common.Product{
		{ID: "first", Content: `{"name":"Engine","type":"other","description":"first"}`, Vector: []float32{1, 0}, Meta: map[string]any{"kind": "entity", "name": "Engine", "entity_type": "other", "source_chunk_ids": []string{"c1"}}},
		{ID: "second", Content: `{"name":" engine ","type":"component","description":"second"}`, Vector: []float32{0, 1}, Meta: map[string]any{"kind": "entity", "name": " engine ", "entity_type": "component", "source_chunk_ids": []string{"c2"}}},
	}
	for _, row := range rows {
		if err := deduper.Add(context.Background(), row); err != nil {
			t.Fatal(err)
		}
	}
	got := deduper.Rows()
	if len(got) != 1 {
		t.Fatalf("same-name entities must merge despite dissimilar vectors: %+v", got)
	}
	payload := parsePayload(got[0].Content)
	if payload["type"] != "component" {
		t.Fatalf("specific type should replace other: %v", payload)
	}
	if ids := metaStrings(got[0].Meta, "source_chunk_ids"); len(ids) != 2 {
		t.Fatalf("source chunks were not merged: %v", ids)
	}
	if chat.mergeCalls != 0 {
		t.Fatalf("exact-name merge should not call the LLM, got %d calls", chat.mergeCalls)
	}
}

func TestMergeGraphEntitiesNormalizesName(t *testing.T) {
	got := mergeGraphEntities([]map[string]any{
		{"name": "Engine", "type": "other", "description": "first", "mention_count": 1},
		{"name": " engine ", "type": "component", "description": "second", "mention_count": 1},
	})
	if len(got) != 1 {
		t.Fatalf("case and surrounding whitespace must not split graph nodes: %+v", got)
	}
	if got[0]["type"] != "component" || mentionOf(got[0]) != 2 {
		t.Fatalf("merged graph entity = %+v", got[0])
	}
}

type similarityMergeDecider struct{}

func (similarityMergeDecider) Decide(_ context.Context, existing, incoming common.Product, score float64) (MergeDecision, common.Product, error) {
	if existing.ID == "" || score < 0.9 {
		return DecisionKeepBoth, common.Product{}, nil
	}
	replacement := existing
	replacement.Meta = make(map[string]any, len(existing.Meta))
	for key, value := range existing.Meta {
		replacement.Meta[key] = value
	}
	replacement.Meta["source_chunk_ids"] = unionOrdered(metaStrings(existing.Meta, "source_chunk_ids"), metaStrings(incoming.Meta, "source_chunk_ids"))
	return DecisionMerge, replacement, nil
}

func TestGroupedDeduperTracksNameAfterSemanticMerge(t *testing.T) {
	deduper := NewGroupedDeduper(similarityMergeDecider{})
	rows := []common.Product{
		{ID: "canonical", Content: `{"name":"Alpha","type":"letter"}`, Vector: []float32{1, 0}, Meta: map[string]any{"kind": "entity", "name": "Alpha", "source_chunk_ids": []string{"c1"}}},
		{ID: "alias-1", Content: `{"name":"Al","type":"letter"}`, Vector: []float32{1, 0}, Meta: map[string]any{"kind": "entity", "name": "Al", "source_chunk_ids": []string{"c2"}}},
		{ID: "alias-2", Content: `{"name":"Al","type":"letter"}`, Vector: []float32{0, 1}, Meta: map[string]any{"kind": "entity", "name": "Al", "source_chunk_ids": []string{"c3"}}},
	}
	for _, row := range rows {
		if err := deduper.Add(context.Background(), row); err != nil {
			t.Fatal(err)
		}
	}
	got := deduper.Rows()
	if len(got) != 1 {
		t.Fatalf("semantic alias must remain addressable by its own name: %+v", got)
	}
	if ids := metaStrings(got[0].Meta, "source_chunk_ids"); len(ids) != 3 {
		t.Fatalf("semantic and exact-name merges lost provenance: %v", ids)
	}
}

// ---- ontology domain / range enforcement ----

func ontologyParserConfig() map[string]any {
	return map[string]any{
		"kind": "ontology",
		"entity": map[string]any{
			"fields": []any{
				map[string]any{"type": "agent", "description": "anything bearing a role"},
				map[string]any{"type": "person", "parent": "agent", "description": "a person"},
				map[string]any{"type": "place", "description": "a place"},
			},
			"output_fields": []any{
				map[string]any{"name": "attributes", "shape": `{"<attribute>": <value>}`},
			},
		},
		"relation": map[string]any{"fields": []any{
			map[string]any{
				"type": "born_in", "kind": "object",
				"domain": "person", "range": "place",
				"description": "a person was born in a place",
			},
			map[string]any{
				"type": "alias", "kind": "datatype",
				"domain": "agent", "datatype": "list",
				"description": "alternative names",
			},
			map[string]any{
				"type": "birth_date", "kind": "datatype",
				"domain": "person", "datatype": "date",
				"description": "the date of birth",
			},
		}},
	}
}

func relationRow(prop, fromType, toType string) common.Product {
	meta := map[string]any{"kind": "relation", "relation_type": prop}
	if fromType != "" {
		meta["from_type"] = fromType
	}
	if toType != "" {
		meta["to_type"] = toType
	}
	return common.Product{Meta: meta}
}

// A contradicting endpoint class is the one case the projection drops; an
// unknown class or an undeclared property must survive, because neither
// contradicts the declaration and both are reported elsewhere (the model graph's
// unattributed count and its "observed, not declared" class).
func TestFilterOutOfOntologyRelations(t *testing.T) {
	cfg := ontologyParserConfig()
	rows := []common.Product{
		relationRow("born_in", "person", "place"),               // matches the declaration
		relationRow("born_in", "place", "person"),               // reversed => drop
		relationRow("born_in", "person", "person"),              // wrong range => drop
		relationRow("born_in", "person", ""),                    // class unknown => keep
		relationRow("invented", "place", "place"),               // undeclared => keep
		{Meta: map[string]any{"kind": "entity", "name": "Ada"}}, // not a relation
	}
	got, _ := filterOutOfOntologyRelations(rows, cfg)
	if len(got) != 4 {
		t.Fatalf("rows = %d, want 4 (only the two contradicting endpoints dropped)", len(got))
	}
	// Order is preserved, so the survivors are rows 0, 3, 4 and 5.
	wantProp := []string{"born_in", "born_in", "invented", ""}
	wantFrom := []string{"person", "person", "place", ""}
	for i := range got {
		prop, _ := got[i].Meta["relation_type"].(string)
		from, _ := got[i].Meta["from_type"].(string)
		if prop != wantProp[i] || from != wantFrom[i] {
			t.Fatalf("survivor %d = (%q, from %q), want (%q, from %q)",
				i, prop, from, wantProp[i], wantFrom[i])
		}
	}
}

// A subclass instance satisfies a property declared on its superclass: person is
// declared as a child of agent, so an assertion typed (person, place) must
// survive a property whose domain is agent. The model graph expands inheritance
// the same way (ancestorChain), and a writer that did not would drop rows the
// view then claims exist — ontology.md §8 (20).
func TestFilterOutOfOntologyRelationsKeepsSubclassEndpoints(t *testing.T) {
	cfg := ontologyParserConfig()
	rel := cfg["relation"].(map[string]any)
	rel["fields"] = append(rel["fields"].([]any), map[string]any{
		"type": "leads", "kind": "object", "domain": "agent", "range": "place",
	})
	rows := []common.Product{
		relationRow("leads", "person", "place"), // person is a declared child of agent => keep
		relationRow("leads", "agent", "place"),  // the declared class itself => keep
		relationRow("leads", "place", "place"),  // unrelated class => the domain rejects it
	}
	kept, rejected := filterOutOfOntologyRelations(rows, cfg)
	if len(kept) != 2 {
		t.Fatalf("kept = %d, want 2 (the subclass and the declared class itself)", len(kept))
	}
	if len(rejected) != 1 {
		t.Fatalf("rejected = %d, want 1", len(rejected))
	}
}

// A rejected assertion is handed back as a row instead of vanishing: it keeps
// the property and both endpoint classes so the quality panel can list it, and
// the reason rides in the content (no column of its own). A rejection nobody
// wrote down cannot drive the feedback loop — ontology.md §2.7 (1).
func TestFilterOutOfOntologyRelationsReportsRejected(t *testing.T) {
	rows := []common.Product{relationRow("born_in", "place", "person")} // reversed => domain rejects
	kept, rejected := filterOutOfOntologyRelations(rows, ontologyParserConfig())
	if len(kept) != 0 || len(rejected) != 1 {
		t.Fatalf("kept = %d rejected = %d, want 0 / 1", len(kept), len(rejected))
	}
	got := rejected[0]
	if kind, _ := got.Meta["kind"].(string); kind != "dropped_relation" {
		t.Fatalf("kind = %q, want dropped_relation", kind)
	}
	if side, _ := got.Meta["drop_side"].(string); side != "domain" {
		t.Fatalf("drop_side = %q, want domain", side)
	}
	if prop, _ := got.Meta["relation_type"].(string); prop != "born_in" {
		t.Fatalf("prop = %q, want born_in", prop)
	}
	if from, _ := got.Meta["from_type"].(string); from != "place" {
		t.Fatalf("from_type = %q, want the observed class kept for the panel", from)
	}
	if !strings.Contains(got.Content, "person") {
		t.Fatalf("content %q must name the declared class", got.Content)
	}
}

// A template that declares no domain / range (every non-ontology template, and
// knowledge_graph.yaml specifically) must see its relations untouched — the
// enforcement cannot silently change existing behaviour.
func TestFilterOutOfOntologyRelationsLeavesPlainGraphAlone(t *testing.T) {
	rows := []common.Product{
		relationRow("linked", "letter", "letter"),
		relationRow("linked", "anything", "whatever"),
	}
	got, _ := filterOutOfOntologyRelations(rows, graphParserConfig())
	if len(got) != len(rows) {
		t.Fatalf("rows = %d, want %d untouched", len(got), len(rows))
	}
}

// A subclass carries its parent's attributes: `alias` is declared on agent but
// belongs to person too. Own attributes come first, then inherited ones, so a
// subclass overrides its parent for the same name (the dedup keeps the first
// occurrence).
func TestDatatypePropertiesByClassInherits(t *testing.T) {
	byClass := datatypePropertiesByClass(ontologyParserConfig())
	want := map[string][]string{
		"agent":  {"alias"},
		"person": {"birth_date", "alias"},
	}
	for class, wantAttrs := range want {
		got := byClass[class]
		if len(got) != len(wantAttrs) {
			t.Fatalf("%s attributes = %v, want %v", class, got, wantAttrs)
		}
		for i := range wantAttrs {
			if got[i] != wantAttrs[i] {
				t.Fatalf("%s attributes = %v, want %v", class, got, wantAttrs)
			}
		}
	}
	if _, present := byClass["place"]; present {
		t.Fatalf("a class with no attributes must not appear: %v", byClass)
	}
}

// A class may name several superclasses (`|`-separated `parent`). Every one of
// them contributes its attributes, nearest first — the same breadth-first order
// ontologyClassAncestors uses — and an ancestor reached through two parents (the
// diamond here: `agent` via both person and organization) is reported once.
func TestDatatypePropertiesByClassInheritsFromEveryParent(t *testing.T) {
	cfg := map[string]any{
		"kind": "ontology",
		"entity": map[string]any{"fields": []any{
			map[string]any{"type": "agent", "description": "anything bearing a role"},
			map[string]any{"type": "person", "parent": "agent", "description": "a person"},
			map[string]any{"type": "organization", "parent": "agent", "description": "a company"},
			map[string]any{"type": "employee", "parent": "person | organization", "description": "an employed person"},
		}},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "alias", "kind": "datatype", "domain": "agent", "datatype": "list"},
			map[string]any{"type": "birth_date", "kind": "datatype", "domain": "person", "datatype": "date"},
			map[string]any{"type": "founded_on", "kind": "datatype", "domain": "organization", "datatype": "date"},
			map[string]any{"type": "employee_id", "kind": "datatype", "domain": "employee", "datatype": "string"},
		}},
	}
	byClass := datatypePropertiesByClass(cfg)
	want := map[string][]string{
		"employee":     {"employee_id", "birth_date", "founded_on", "alias"},
		"person":       {"birth_date", "alias"},
		"organization": {"founded_on", "alias"},
	}
	for class, wantAttrs := range want {
		got := byClass[class]
		if len(got) != len(wantAttrs) {
			t.Fatalf("%s attributes = %v, want %v", class, got, wantAttrs)
		}
		for i := range wantAttrs {
			if got[i] != wantAttrs[i] {
				t.Fatalf("%s attributes = %v, want %v", class, got, wantAttrs)
			}
		}
	}
}

func TestOntologyAttributes(t *testing.T) {
	cfg := ontologyParserConfig()

	// Flat payload keys, plus the inherited `alias`.
	flat, rejected := ontologyAttributes(map[string]any{
		"type": "person", "alias": []any{"Ada Augusta Byron"}, "birth_date": "1815-12-10",
	}, "person", cfg)
	if rejected != 0 || len(flat) != 2 || flat["birth_date"] != "1815-12-10" {
		t.Fatalf("flat attributes = %v (rejected %d), want alias + birth_date", flat, rejected)
	}

	// The nested shape object the entity output_fields ask for. The invented key
	// must not reach the filterable column.
	nested, _ := ontologyAttributes(map[string]any{
		"type":       "person",
		"attributes": map[string]any{"birth_date": "1815-12-10", "invented": "x"},
	}, "person", cfg)
	if len(nested) != 1 || nested["birth_date"] != "1815-12-10" {
		t.Fatalf("nested attributes = %v, want only birth_date", nested)
	}

	// Values that carry nothing must not bloat the column.
	if got, _ := ontologyAttributes(map[string]any{
		"type": "person", "birth_date": "   ", "alias": []any{},
	}, "person", cfg); got != nil {
		t.Fatalf("empty attribute values must yield no attr, got %v", got)
	}

	// A template declaring no datatype properties yields nothing, so every
	// non-ontology template is unaffected.
	if got, _ := ontologyAttributes(map[string]any{"type": "letter", "x": "y"}, "letter", graphParserConfig()); got != nil {
		t.Fatalf("plain graph template must yield no attr, got %v", got)
	}
}

// A declared datatype is a promise about the stored value, and `attr` is the only
// place a value is ever range-filtered: an integer kept as "328" and a date kept
// as "3 May 1937" answer zero rows forever. Every value is therefore coerced, and
// a value that fits nothing is counted instead of stored.
func TestOntologyAttributesNormalizeDatatypes(t *testing.T) {
	cfg := map[string]any{
		"kind": "ontology",
		"entity": map[string]any{
			"fields": []any{map[string]any{"type": "work", "description": "a work"}},
		},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "published_date", "kind": "datatype", "domain": "work", "datatype": "date"},
			map[string]any{"type": "pages", "kind": "datatype", "domain": "work", "datatype": "int"},
			map[string]any{"type": "genre", "kind": "datatype", "domain": "work", "datatype": "list",
				"enum": []any{"novel", "poem"}},
			map[string]any{"type": "is_open", "kind": "datatype", "domain": "work", "datatype": "bool"},
			map[string]any{"type": "note", "kind": "datatype", "domain": "work"},
		}},
	}

	got, rejected := ontologyAttributes(map[string]any{
		"type":           "work",
		"published_date": "3 May 1937",
		"pages":          "328",
		"genre":          []any{"novel", "treatise"},
		"is_open":        "true",
		"note":           " kept verbatim ",
	}, "work", cfg)
	if rejected != 0 {
		t.Fatalf("rejected = %d, want 0 (attrs %v)", rejected, got)
	}
	if got["published_date"] != "1937-05-03" {
		t.Errorf("published_date = %v, want 1937-05-03", got["published_date"])
	}
	if got["pages"] != int64(328) {
		t.Errorf("pages = %v, want int64(328)", got["pages"])
	}
	// The value set keeps what it recognises and drops the rest, so a stray
	// category never reaches a filter.
	if genres, ok := got["genre"].([]string); !ok || len(genres) != 1 || genres[0] != "novel" {
		t.Errorf("genre = %v, want [novel]", got["genre"])
	}
	if got["is_open"] != true {
		t.Errorf("is_open = %v, want true", got["is_open"])
	}
	// No declared datatype means no coercion: the template never said what the
	// value is, so trimming is the only safe change.
	if got["note"] != "kept verbatim" {
		t.Errorf("note = %v, want trimmed string", got["note"])
	}

	// A value that fits no declared type is dropped and counted, not stored as if
	// it were valid.
	bad, rejected := ontologyAttributes(map[string]any{
		"type": "work", "published_date": "sometime in spring", "pages": "many",
	}, "work", cfg)
	if rejected != 2 || len(bad) != 0 {
		t.Fatalf("bad values = %v (rejected %d), want nothing kept and 2 counted", bad, rejected)
	}
}

// The projection stamps a row with the canonical class, so a model that answers
// with a declared alias does not create a second spelling of the same class.
// An undeclared name keeps its own spelling, because drift must stay visible.
func TestOntologyTermIndexResolvesDeclaredAliases(t *testing.T) {
	cfg := map[string]any{
		"kind": "ontology",
		"entity": map[string]any{"fields": []any{
			map[string]any{"type": "organization", "aliases": []any{"company", "firm"}},
			map[string]any{"type": "person"},
		}},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "born_in", "kind": "object", "domain": "person", "range": "organization",
				"aliases": []any{"place_of_birth"}},
			map[string]any{"type": "works_for", "kind": "object", "domain": "person", "range": "organization"},
		}},
	}
	classes, props := ontologyTermIndex(cfg)

	if got := resolveDeclaredType(classes, "Company"); got != "organization" {
		t.Errorf("class alias Company = %q, want organization", got)
	}
	if got := resolveDeclaredType(classes, "PERSON"); got != "person" {
		t.Errorf("declared name must resolve case-insensitively, got %q", got)
	}
	if got := resolveDeclaredType(classes, "startup"); got != "startup" {
		t.Errorf("undeclared class = %q, want it kept as observed", got)
	}
	if got := resolveDeclaredType(props, "place_of_birth"); got != "born_in" {
		t.Errorf("property alias = %q, want born_in", got)
	}
	if got := resolveDeclaredType(props, "works_for"); got != "works_for" {
		t.Errorf("declared property = %q, want works_for", got)
	}
	if got := resolveDeclaredType(props, "invented"); got != "invented" {
		t.Errorf("undeclared property = %q, want it kept as observed", got)
	}
}

// The projection — not just the prompt — honours a declared alias, so choosing the
// alternative name does not create a second spelling of the same class. That
// matters downstream: `entity_type` is what the model graph counts, what
// type-scoped retrieval filters on, and what the quality ledger would otherwise
// report as vocabulary drift.
func TestBuildRowsResolvesDeclaredAliases(t *testing.T) {
	cfg := ontologyParserConfig()
	cfg["entity"].(map[string]any)["fields"].([]any)[1].(map[string]any)["aliases"] =
		[]any{"human", "individual"}
	cfg["relation"].(map[string]any)["fields"].([]any)[0].(map[string]any)["aliases"] =
		[]any{"place_of_birth"}

	rows, err := buildRows(context.Background(), common.Deps{Embed: hashEmbedder{dim: 4}}, CompileConfig{
		TenantID:     "t1",
		DocID:        "d1",
		Type:         TypeHypergraph,
		ParserConfig: cfg,
	},
		[]map[string]any{{"type": "Human", "name": "Ada"}},
		[]map[string]any{{"type": "place_of_birth", "source": "Ada", "target": "London"}},
		[]string{"c1"},
	)
	if err != nil {
		t.Fatalf("buildRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want entity + relation", len(rows))
	}
	for _, row := range rows {
		switch kind, _ := row.Meta["kind"].(string); kind {
		case "entity":
			if got := row.Meta["entity_type"]; got != "person" {
				t.Errorf("entity_type = %v, want the declared class person", got)
			}
		case "relation":
			if got := row.Meta["relation_type"]; got != "born_in" {
				t.Errorf("relation_type = %v, want the declared property born_in", got)
			}
		}
	}
}

// A declared alias and a closed value set are generative facts: they stop the
// model proposing a second name for a class it already has, and stop it inventing
// a category. `required` is an acceptance rule and must not be stated, because a
// model told an attribute is mandatory will supply one rather than leave a gap.
func TestOntologyPromptStatesAliasesAndEnumsButNotRequired(t *testing.T) {
	cfg := map[string]any{
		"kind": "ontology",
		"entity": map[string]any{"fields": []any{
			map[string]any{"type": "organization", "description": "an organisation",
				"aliases": []any{"company", "firm"}},
		}},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "works_for", "kind": "object", "domain": "organization",
				"range": "organization", "aliases": []any{"employed_by"}},
			map[string]any{"type": "genre", "kind": "datatype", "domain": "organization",
				"datatype": "string", "enum": []any{"novel", "poem"}, "required": true},
		}},
	}

	node, edge := HypergraphPrompts(cfg, "en")

	if !strings.Contains(node, "  aliases: company, firm") {
		t.Errorf("entity prompt must state class aliases\n---\n%s", node)
	}
	for _, want := range []string{"  aliases: employed_by", "  enum: novel | poem"} {
		if !strings.Contains(edge, want) {
			t.Errorf("relation prompt missing %q\n---\n%s", want, edge)
		}
	}
	for _, prompt := range []string{node, edge} {
		if strings.Contains(prompt, "  required:") {
			t.Errorf("required must never reach the extraction prompt\n---\n%s", prompt)
		}
	}
}

// The entity stage must state each class's datatype attributes — inheritance
// included — because that list is what makes the model emit them at all, and the
// relation stage must state the standard model's structure (kind / domain /
// range / datatype) so it can honour domain and range.
func TestOntologyPromptsCarryTheStandardModel(t *testing.T) {
	node, edge := HypergraphPrompts(ontologyParserConfig(), "en")

	for _, want := range []string{
		"  parent: agent",
		"  attributes: birth_date (date), alias (list)",
	} {
		if !strings.Contains(node, want) {
			t.Errorf("entity prompt missing %q\n---\n%s", want, node)
		}
	}
	for _, want := range []string{
		"  kind: object",
		"  kind: datatype",
		"  domain: person",
		"  range: place",
		"  datatype: date",
	} {
		if !strings.Contains(edge, want) {
			t.Errorf("relation prompt missing %q\n---\n%s", want, edge)
		}
	}
}

// ---- the hypernode / hyperedge layer ----

func entityRow(name, class string, attrs map[string]any, chunks ...string) common.Product {
	meta := map[string]any{"kind": "entity", "name": name, "entity_type": class}
	if len(attrs) > 0 {
		meta["attr"] = attrs
	}
	if len(chunks) > 0 {
		meta["source_chunk_ids"] = chunks
	} else {
		meta["source_chunk_ids"] = []string{"c1"}
	}
	return common.Product{Meta: meta}
}

func TestNLKeyIsTheMechanicalPathPhrased(t *testing.T) {
	got := nlKey("crop:has_growing_zones.crop_growing_zone:name")
	if want := "crop has growing zones crop growing zone name"; got != want {
		t.Fatalf("nlKey = %q, want %q", got, want)
	}
}

// A list-valued attribute contributes one fact per item: joining them would
// silently drop the extra values from the value side of the index.
func TestFlattenValues(t *testing.T) {
	if got := flattenValues([]any{"A", "B"}); len(got) != 2 {
		t.Fatalf("list = %v, want 2 items", got)
	}
	if got := flattenValues("  x  "); len(got) != 1 || got[0] != "x" {
		t.Fatalf("scalar = %v, want [x]", got)
	}
	if got := flattenValues([]any{}); len(got) != 0 {
		t.Fatalf("empty list = %v, want nothing", got)
	}
	if got := flattenValues("   "); len(got) != 0 {
		t.Fatalf("blank = %v, want nothing", got)
	}
}

func adaAndLondon() []common.Product {
	return []common.Product{
		entityRow("Ada Lovelace", "person", map[string]any{
			"birth_date": "1815-12-10",
			"alias":      []any{"Ada Augusta Byron"},
		}, "c1"),
		entityRow("London", "place", nil, "c2"),
		{Meta: map[string]any{
			"kind": "relation", "from": "Ada Lovelace", "to": "London",
			"relation_type": "born_in",
		}},
	}
}

// The layer is the whole input of OG-RAG's Algorithm 1: one hyperedge per
// flattened block (the dictionary payload RAG_QUERY_PROMPT asks for), and two
// rows per unique (path, value) so the key leg and the value leg each have their
// own vector.
func TestBuildHypergraphMaterializesFacts(t *testing.T) {
	cfg := CompileConfig{DocID: "d1", TenantID: "t1", TemplateID: "tpl", ParserConfig: ontologyParserConfig()}
	rows, err := buildHypergraph(context.Background(), common.Deps{Embed: hashEmbedder{dim: 8}}, cfg, adaAndLondon())
	if err != nil {
		t.Fatal(err)
	}

	var edges, keyRows, valueRows []common.Product
	for _, r := range rows {
		switch kind, _ := r.Meta["kind"].(string); kind {
		case "hyperedge":
			edges = append(edges, r)
		case "hypernode":
			if role, _ := r.Meta["node_role"].(string); role == "key" {
				keyRows = append(keyRows, r)
			} else {
				valueRows = append(valueRows, r)
			}
		default:
			t.Fatalf("unexpected row kind %q", kind)
		}
	}
	// Ada is the only root (London is targeted), so: Ada's own block plus the
	// block extended through born_in.
	if len(edges) != 2 {
		t.Fatalf("hyperedges = %d, want 2", len(edges))
	}
	// 3 facts on Ada (name, birth_date, alias) + 1 on London = 4 unique pairs.
	if len(keyRows) != 4 || len(valueRows) != 4 {
		t.Fatalf("hypernode rows = %d key / %d value, want 4 / 4", len(keyRows), len(valueRows))
	}

	// The deepest block is the dictionary OG-RAG hands to the model.
	deep := parsePayload(edges[1].Content)
	for wantKey, wantVal := range map[string]string{
		"person name":               "Ada Lovelace",
		"person birth date":         "1815-12-10",
		"person alias":              "Ada Augusta Byron",
		"person born in place name": "London",
	} {
		if got, _ := deep[wantKey].(string); got != wantVal {
			t.Errorf("deep block[%q] = %q, want %q", wantKey, got, wantVal)
		}
	}

	// The root's own pair belongs to BOTH blocks; the nested pair to one. This
	// many-to-many membership is why hyperedge_ids is an array column.
	rootEdgeIDs := metaStrings(edges[0].Meta, "id")
	_ = rootEdgeIDs
	for _, r := range keyRows {
		path, _ := r.Meta["path"].(string)
		ids := metaStrings(r.Meta, "hyperedge_ids")
		switch path {
		case "person:name":
			if len(ids) != 2 {
				t.Errorf("person:name belongs to %d hyperedges, want 2 (both blocks)", len(ids))
			}
		case "person:born_in.place:name":
			if len(ids) != 1 {
				t.Errorf("nested fact belongs to %d hyperedges, want 1", len(ids))
			}
		}
		if prop, _ := r.Meta["prop"].(string); prop == "" {
			t.Errorf("hypernode %q carries no prop", path)
		}
	}

	// Each half carries only its own key so PayloadDescription cannot mix them.
	for _, r := range keyRows {
		p := parsePayload(r.Content)
		if _, ok := p["key"]; !ok || len(p) != 1 {
			t.Errorf("key row payload = %v, want exactly {key}", p)
		}
	}
	for _, r := range valueRows {
		p := parsePayload(r.Content)
		if _, ok := p["value"]; !ok || len(p) != 1 {
			t.Errorf("value row payload = %v, want exactly {value}", p)
		}
	}

	// The two halves of one hypernode share the hash and differ by the suffix.
	if !strings.HasSuffix(keyRows[0].ID, ":k") || !strings.HasSuffix(valueRows[0].ID, ":v") {
		t.Fatalf("ids = %q / %q, want :k / :v suffixes", keyRows[0].ID, valueRows[0].ID)
	}
	if strings.TrimSuffix(keyRows[0].ID, ":k") != strings.TrimSuffix(valueRows[0].ID, ":v") {
		t.Fatalf("the two halves must share hypernode_hash: %q vs %q", keyRows[0].ID, valueRows[0].ID)
	}
}

// The same (path, value) reached from two different roots must collapse into ONE
// hypernode whose hyperedge_ids names both blocks. That is what keeps the value
// side of the index small, and why membership is an array column rather than a
// single parent pointer.
func TestBuildHypergraphHypernodeIsSharedAcrossRoots(t *testing.T) {
	cfg := CompileConfig{DocID: "d1", TenantID: "t1", TemplateID: "tpl", ParserConfig: ontologyParserConfig()}
	prods := []common.Product{
		entityRow("Ada Lovelace", "person", nil, "c1"),
		entityRow("Grace Hopper", "person", nil, "c2"),
		entityRow("London", "place", nil, "c3"),
		{Meta: map[string]any{"kind": "relation", "from": "Ada Lovelace", "to": "London", "relation_type": "born_in"}},
		{Meta: map[string]any{"kind": "relation", "from": "Grace Hopper", "to": "London", "relation_type": "born_in"}},
	}
	rows, err := buildHypergraph(context.Background(), common.Deps{Embed: hashEmbedder{dim: 8}}, cfg, prods)
	if err != nil {
		t.Fatal(err)
	}

	// London is reached from both roots, so this pair is produced twice.
	const shared = "person:born_in.place:name"
	found := 0
	for _, r := range rows {
		if kind, _ := r.Meta["kind"].(string); kind != "hypernode" {
			continue
		}
		if role, _ := r.Meta["node_role"].(string); role != "key" {
			continue
		}
		if path, _ := r.Meta["path"].(string); path != shared {
			continue
		}
		found++
		if ids := metaStrings(r.Meta, "hyperedge_ids"); len(ids) != 2 {
			t.Errorf("shared hypernode belongs to %d hyperedges, want 2", len(ids))
		}
	}
	if found != 1 {
		t.Fatalf("the shared (path, value) produced %d hypernode rows, want 1", found)
	}

	// Both roots still produced their own block, so the two memberships are two
	// distinct hyperedges rather than one block counted twice.
	edges := 0
	for _, r := range rows {
		if kind, _ := r.Meta["kind"].(string); kind == "hyperedge" {
			edges++
		}
	}
	if edges != 4 {
		t.Fatalf("hyperedges = %d, want 4 (two roots x two blocks each)", edges)
	}
}

// Membership is what ONE document can compute: the hyperedge ids a row carries
// are the blocks that document produced. Two documents stating the same
// (path, value) must therefore get two rows — with a document-free id they
// share one, and because the writer replaces whole rows the later document's
// membership would be the only one left, silently dropping the earlier
// document's slots from the layer (hypergraph_retrieval.md §3 P0-1).
func TestBuildHypergraphHypernodeIsDocumentScoped(t *testing.T) {
	prods := adaAndLondon()
	ids := map[string]string{}
	for _, docID := range []string{"d1", "d2"} {
		cfg := CompileConfig{DocID: docID, TenantID: "t1", TemplateID: "tpl", ParserConfig: ontologyParserConfig()}
		rows, err := buildHypergraph(context.Background(), common.Deps{Embed: hashEmbedder{dim: 8}}, cfg, prods)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			kind, _ := r.Meta["kind"].(string)
			if kind != "hypernode" {
				continue
			}
			path, _ := r.Meta["path"].(string)
			if path != "person:name" {
				continue
			}
			if role, _ := r.Meta["node_role"].(string); role != "key" {
				continue
			}
			if prev, ok := ids[docID]; ok {
				t.Fatalf("document %s produced two person:name key rows (%s, %s)", docID, prev, r.ID)
			}
			ids[docID] = r.ID
		}
	}
	if ids["d1"] == "" || ids["d2"] == "" {
		t.Fatalf("missing hypernode rows: %v", ids)
	}
	if ids["d1"] == ids["d2"] {
		t.Fatalf("the same (path, value) in two documents shares row id %q; "+
			"the second document would overwrite the first one's membership", ids["d1"])
	}
}

// node_value is the slot's value in exact spelling on BOTH halves: a key row's
// payload holds the attribute path, so without the column a key row cannot be
// joined to its value row and an entity's name node can only be found by full
// text.
func TestBuildHypergraphHypernodeCarriesNodeValue(t *testing.T) {
	cfg := CompileConfig{DocID: "d1", TenantID: "t1", TemplateID: "tpl", ParserConfig: ontologyParserConfig()}
	rows, err := buildHypergraph(context.Background(), common.Deps{Embed: hashEmbedder{dim: 8}}, cfg, adaAndLondon())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, r := range rows {
		if kind, _ := r.Meta["kind"].(string); kind != "hypernode" {
			continue
		}
		path, _ := r.Meta["path"].(string)
		role, _ := r.Meta["node_role"].(string)
		value, _ := r.Meta["node_value"].(string)
		if value == "" {
			t.Fatalf("%s %s row carries no node_value", path, role)
		}
		seen[path+"|"+role] = value
	}
	for _, want := range []string{
		"person:name|key", "person:name|value",
		"person:birth_date|key", "person:birth_date|value",
	} {
		if seen[want] == "" {
			t.Errorf("no node_value for %s", want)
		}
	}
	if seen["person:name|key"] != "Ada Lovelace" || seen["person:name|value"] != "Ada Lovelace" {
		t.Errorf("both halves must state the same value, got key=%q value=%q",
			seen["person:name|key"], seen["person:name|value"])
	}
}

// A template with no declared object properties writes no extra rows, so every
// other variant's row count is untouched.
func TestBuildHypergraphSkippedWithoutOntology(t *testing.T) {
	for name, pc := range map[string]map[string]any{
		"plain graph": graphParserConfig(),
		"ontology without object properties": func() map[string]any {
			c := ontologyParserConfig()
			c["relation"] = map[string]any{"fields": []any{
				map[string]any{"type": "alias", "kind": "datatype", "domain": "person", "datatype": "list", "description": "a"},
			}}
			return c
		}(),
	} {
		cfg := CompileConfig{DocID: "d1", ParserConfig: pc}
		rows, err := buildHypergraph(context.Background(), common.Deps{Embed: hashEmbedder{dim: 8}}, cfg, adaAndLondon())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(rows) != 0 {
			t.Fatalf("%s: wrote %d rows, want 0", name, len(rows))
		}
	}
}
