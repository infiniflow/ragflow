// Package common holds the shared, dependency-light foundation for the
// KnowledgeCompiler component: parameter/IO types, the in-memory product
// store, tokenization/batching helpers, the LLM chat seam, and the
// concurrency pool.
//
// It deliberately imports only the standard library plus a stable-hash
// dependency, so it (and the M1 unit tests) build without the CGO native
// libraries that other ingestion paths require.
package common

import (
	"errors"
	"fmt"
	"strings"
)

// Variant identifies which knowledge-compile strategy to run.
type Variant string

const (
	VariantStructure Variant = "structure"
	VariantWiki      Variant = "wiki"
	VariantTree      Variant = "tree"
	VariantMindmap   Variant = "mindmap"
)

// Sentinel errors.
var (
	ErrUnknownVariant = errors.New("knowledge_compiler: unknown variant")
	ErrNotImplemented = errors.New("knowledge_compiler: variant not yet implemented")
)

// Param is built from the DSL params map. The component is driven by a
// compilation template (or template group); the variant is NOT taken from the
// DSL — it is derived from the resolved template's kind at Invoke time (see
// KindToVariant).
type Param struct {
	// CompilationTemplateID selects a single compilation template directly.
	// CompilationTemplateGroupID selects a compilation-template group, which
	// resolves to one or more templates. When both are non-empty,
	// CompilationTemplateID wins (priority: id > group_id).
	CompilationTemplateID      string
	CompilationTemplateGroupID string

	LLMID          string
	EmbeddingModel string
	Language       string
	// SimilarityThreshold defaults to 0.99 when not provided (see Defaults).
	SimilarityThreshold   float64
	MaxWorkers            int
	EnableHistoricalDedup bool
	// Extra carries arbitrary caller-provided overrides merged into the
	// resolved template config.
	Extra map[string]any

	// The following fields are resolved at runtime (not part of the DSL
	// surface) and are set by the component before dispatching to a variant:
	//
	// Variant is derived from the resolved template's kind (see KindToVariant),
	// not from the DSL.
	Variant Variant
	// TemplateID is the resolved compilation template id for the current spec
	// (the id path uses it directly; the group path resolves each child). It is
	// plumbed to the variant for stamping compilation_template_ids.
	TemplateID string
	// TemplateConfig is the resolved template's config blob (the template
	// "content"), plumbed to the variant for prompt/structure extraction.
	TemplateConfig map[string]any
}

// Defaults returns a Param populated with safe production fallbacks.
func (p Param) Defaults() Param {
	p.SimilarityThreshold = 0.99
	p.MaxWorkers = 4
	p.Extra = map[string]any{}
	return p
}

// Candidate is a historical product vector used for cross-run dedup.
type Candidate struct {
	ID     string
	Vector []float32
}

// Chunk is one upstream chunk fed to the component.
type Chunk struct {
	ID      string
	Text    string // "text" field from upstream
	Content string // "content_with_weight" field from upstream
	// Vector is the pre-computed embedding of the chunk, when the caller has
	// already embedded it. Variants reuse it instead of re-embedding; a nil
	// Vector means "not embedded yet" and the variant computes one on demand.
	Vector []float32
	Meta   map[string]any
}

// Inputs is the resolved input set passed to a variant Run.
type Inputs struct {
	Chunks               []Chunk
	DocID                string // owning document id (defaults to DatasetID when empty)
	LLMID                string
	EmbeddingModel       string
	VariantSpecific      map[string]any
	HistoricalCandidates []Candidate // optional override path (test/offline)
}

// Product is one compiled output row (schema_version=1).
type Product struct {
	ID       string
	DocID    string
	TenantID string
	Variant  Variant
	// Kind is the original compilation_template.kind (e.g. "page_index"),
	// distinct from Variant (the collapsed Go strategy, e.g. "structure"). It
	// is stamped onto compilation_template_kind_kwd so the document-structure
	// endpoint can group rows by the true template kind.
	Kind string
	// TemplateID is the compilation template that produced this row.
	TemplateID string
	Content    string
	Vector     []float32
	ParentID   string
	Meta       map[string]any
	// Merged marks rows that already went through dataset-level dedup
	// (kc_merged=1, doc_id=kb). The consumer distinguishes these from the
	// per-document compiled rows (doc_id=<source doc>, no kc_merged) so it can
	// KNN against only the merged set instead of re-deduping the whole KB.
	Merged bool
}

// Outputs is the result of a variant Run. All compiled products are buffered
// here; the component merges them into the upstream chunk stream.
type Outputs struct {
	Products          []Product
	DuplicatesDropped int
}

// ParseParam builds a Param from the DSL params map. The variant is NOT taken
// from the DSL — it is derived from the resolved template's kind at Invoke time
// via KindToVariant. At least one of compilation_template_id /
// compilation_template_group_id is required.
func ParseParam(m map[string]any) (Param, error) {
	p := Param{}.Defaults()
	if m == nil {
		return p, fmt.Errorf("knowledge_compiler: params required (compilation_template_id missing)")
	}
	if v, ok := m["compilation_template_id"].(string); ok && strings.TrimSpace(v) != "" {
		p.CompilationTemplateID = strings.TrimSpace(v)
	}
	if v, ok := m["compilation_template_group_id"].(string); ok && strings.TrimSpace(v) != "" {
		p.CompilationTemplateGroupID = strings.TrimSpace(v)
	}
	if p.CompilationTemplateID == "" && p.CompilationTemplateGroupID == "" {
		return p, fmt.Errorf("knowledge_compiler: one of 'compilation_template_id' or 'compilation_template_group_id' is required")
	}
	if v, ok := m["llm_id"].(string); ok {
		p.LLMID = v
	}
	if v, ok := m["embedding_model"].(string); ok {
		p.EmbeddingModel = v
	}
	if v, ok := m["language"].(string); ok {
		p.Language = v
	}
	if v, ok := m["similarity_threshold"].(float64); ok {
		p.SimilarityThreshold = v
	}
	if v, ok := m["enable_historical_dedup"].(bool); ok {
		p.EnableHistoricalDedup = v
	}
	if raw, ok := m["extra"].(map[string]any); ok {
		p.Extra = raw
	}
	return p, nil
}

// KindToVariant maps a compilation_template.kind to the Go compiler Variant.
//
// The Python model uses richer kind values (mind_map, page_index,
// session_essence, session_graph, timeline, knowledge_graph, tree, wiki); the
// Go port collapses them onto its variants. Per the agreed mapping:
//
//	tree                            -> tree
//	mind_map                        -> mindmap
//	wiki                            -> wiki
//	page_index / session_essence /  -> structure (the graph/knowledge-graph path)
//	session_graph / timeline /      ->
//	knowledge_graph                 ->
//
// The canonical variant names are also accepted as identity (so a template kind
// may equal the variant). Unknown kinds return ErrUnknownVariant; the caller
// turns that into a hard failure rather than silently emitting uncompiled rows.
//
// Note: "datasetnav"/"dataset_nav" intentionally has NO mapping here — dataset
// navigation is not an independent compile kind in Python; it is a by-product
// written after tree/page_index compilation via internal/service datasetnav
// (see tasks/agentic_search_port_plan.md).
func KindToVariant(kind string) (Variant, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "tree":
		return VariantTree, nil
	case "mind_map", "mindmap":
		return VariantMindmap, nil
	case "wiki":
		return VariantWiki, nil
	case "page_index", "session_essence", "session_graph", "timeline",
		"knowledge_graph", "structure", "knowledgegraph", "graph":
		return VariantStructure, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownVariant, kind)
	}
}

// FirstNonEmpty returns the first string that is non-empty after trimming.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// VectorFromChunkMap extracts a pre-computed embedding from a chunk map. The
// upstream pipeline (and the knowledge-compile store) carry the vector under a
// dimension-specific column key "q_<dim>_vec" — set by the tokenizer via
// SetExtraValue and flattened into the map by ChunkDoc.ToMap. The value may
// arrive as []float32 (store read) or []float64 (pipeline map); both are
// normalised to []float32. A nil result means the chunk has no embedding yet.
//
// dim selects the exact "q_<dim>_vec" key when the embedding dimension is known
// (dim > 0); otherwise every "q_*_vec" key is considered. A chunk must carry at
// most one embedding vector: multiple q_*_vec fields signal mixed embedding
// models, and because Go map iteration order is nondeterministic, picking among
// them would be unstable across runs — such chunks are rejected with an error.
func VectorFromChunkMap(m map[string]any, dim int) ([]float32, error) {
	if dim > 0 {
		if v, ok := m[fmt.Sprintf("q_%d_vec", dim)]; ok {
			return toFloat32Slice(v), nil
		}
	}
	var keys []string
	for k := range m {
		if strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") {
			keys = append(keys, k)
		}
	}
	switch len(keys) {
	case 0:
		return nil, nil
	case 1:
		return toFloat32Slice(m[keys[0]]), nil
	default:
		return nil, fmt.Errorf("knowledge_compiler: chunk carries %d embedding vectors %v; expected exactly one", len(keys), keys)
	}
}

// toFloat32Slice normalises a numeric slice (any of []float32, []float64, or
// []any of float64) to []float32, returning nil when the value is not a usable
// vector.
func toFloat32Slice(v any) []float32 {
	switch arr := v.(type) {
	case []float32:
		return arr
	case []float64:
		out := make([]float32, len(arr))
		for i, x := range arr {
			out[i] = float32(x)
		}
		return out
	case []any:
		out := make([]float32, 0, len(arr))
		for _, e := range arr {
			if f, ok := e.(float64); ok {
				out = append(out, float32(f))
			}
		}
		return out
	}
	return nil
}
