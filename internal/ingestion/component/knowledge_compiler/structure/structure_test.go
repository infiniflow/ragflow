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
		"source_chunk_ids": []any{"c1", "c2"},
	}
	got := payloadDescription(payload)
	// Keys are sorted for determinism: name < source_chunk_ids < type.
	if got != "Beta c1 c2 letter" {
		t.Fatalf("payloadDescription = %q, want %q", got, "Beta c1 c2 letter")
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

	var entities, relations, graphs int
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
			graphs++
		}
	}
	if entities != 3 || relations != 2 || graphs != 1 {
		t.Fatalf("products = %d entities + %d relations + %d graph, want 3+2+1", entities, relations, graphs)
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

	// Graph summary mirrors Python's {entities, relations} shape.
	g := parsePayload(graphContentOf(out))
	if g == nil {
		t.Fatal("graph product missing/unparseable")
	}
	gEnts, _ := g["entities"].([]any)
	gRels, _ := g["relations"].([]any)
	if len(gEnts) != 3 || len(gRels) != 2 {
		t.Fatalf("graph = %d entities + %d relations, want 3+2", len(gEnts), len(gRels))
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

// ---- helpers ----

func graphContentOf(out common.Outputs) string {
	for _, p := range out.Products {
		if p.Meta["kind"] == "graph" {
			return p.Content
		}
	}
	return ""
}
