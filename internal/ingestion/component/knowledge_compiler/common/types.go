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
	"log"
	"strings"
)

// Variant identifies which knowledge-compile strategy to run.
type Variant string

const (
	VariantStructure  Variant = "structure"
	VariantWiki       Variant = "wiki"
	VariantRaptor     Variant = "raptor"
	VariantMindmap    Variant = "mindmap"
	VariantDatasetnav Variant = "datasetnav"
)

// Sentinel errors.
var (
	ErrUnknownVariant = errors.New("knowledge_compiler: unknown variant")
	ErrNotImplemented = errors.New("knowledge_compiler: variant not yet implemented")
)

// Param is built from the DSL params map. Missing keys fall back to
// Defaults(); variant-name aliases are normalised (see ParseParam).
type Param struct {
	Variant               Variant
	LLMID                 string
	EmbeddingModel        string
	Language              string
	TemplateIDs           []string
	GroupIDs              []string
	SimilarityThreshold   float64
	MaxWorkers            int
	EnableHistoricalDedup bool
	Extra                 map[string]any
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

// LogDeprecated emits a one-line deprecation notice for legacy param names.
func LogDeprecated(old, replacement string) {
	log.Printf("[knowledge_compiler] deprecated variant name %q; use %q", old, replacement)
}

// Product is one compiled output row (schema_version=1).
type Product struct {
	ID       string
	DocID    string
	TenantID string
	Variant  Variant
	Content  string
	Vector   []float32
	ParentID string
	Meta     map[string]any
}

// Outputs is the result of a variant Run. All compiled products are buffered
// here; the component merges them into the upstream chunk stream.
type Outputs struct {
	Products          []Product
	DuplicatesDropped int
}

// ParseParam builds a Param from the DSL params map, applying variant-name
// aliases (mind_map=>mindmap, dataset_nav=>datasetnav) and deprecation logs.
func ParseParam(m map[string]any) (Param, error) {
	p := Param{}.Defaults()
	if m == nil {
		return p, fmt.Errorf("knowledge_compiler: params required (variant missing)")
	}
	if v, ok := m["variant"].(string); ok && v != "" {
		p.Variant = normalizeVariant(v)
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
	// compilation_template_ids / compilation_template_group_ids mirror the
	// Python parser_config keys (see api/utils/validation_utils.py). Template
	// ids are stamped onto every compiled row so the document-structure
	// endpoint can group rows by template and the UI renders one tab per
	// template. Group ids are resolved to concrete template ids by the caller
	// (production wiring resolves them via the compilation-template-group
	// service); when passed through raw they are carried as-is on GroupIDs.
	p.TemplateIDs = parseStringList(m, "compilation_template_ids", "template_ids")
	p.GroupIDs = parseStringList(m, "compilation_template_group_ids", "template_group_ids", "group_ids")
	if raw, ok := m["extra"].(map[string]any); ok {
		p.Extra = raw
	}
	if p.Variant == "" {
		return p, fmt.Errorf("knowledge_compiler: 'variant' is required")
	}
	return p, nil
}

// normalizeVariant maps a DSL variant name to the canonical Variant,
// logging a deprecation note for the legacy underscore forms.
func normalizeVariant(s string) Variant {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "structure":
		return VariantStructure
	case "wiki":
		return VariantWiki
	case "raptor":
		return VariantRaptor
	case "mindmap", "mind_map":
		if strings.Contains(s, "_") {
			LogDeprecated("mind_map", "mindmap")
		}
		return VariantMindmap
	case "datasetnav", "dataset_nav":
		if strings.Contains(s, "_") {
			LogDeprecated("dataset_nav", "datasetnav")
		}
		return VariantDatasetnav
	default:
		return Variant(s)
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

// parseStringList reads a []string from the params map under any of the given
// keys (first hit wins). Accepts []string, []any (of strings), or a single
// whitespace/comma-separated string. Returns nil when no key is present.
func parseStringList(m map[string]any, keys ...string) []string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case []string:
			if len(v) > 0 {
				return v
			}
		case []any:
			out := make([]string, 0, len(v))
			for _, e := range v {
				if s, ok := e.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						out = append(out, s)
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				continue
			}
			// Tolerate comma- or whitespace-separated lists.
			parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
			var out []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
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
