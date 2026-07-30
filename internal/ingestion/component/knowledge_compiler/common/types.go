// Package common holds the shared, dependency-light foundation for the
// KnowledgeCompiler component: parameter/IO types, capacity guardrails,
// the in-memory product store, tokenization/batching helpers, the LLM
// chat seam, and the concurrency pool.
//
// It deliberately imports only the standard library plus a stable-hash
// dependency, so it (and the M1 unit tests) build without the CGO native
// libraries that other ingestion paths require.
package common

import (
	"context"
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
	ErrCapacityExceeded = errors.New("knowledge_compiler: capacity exceeded")
	ErrUnknownVariant   = errors.New("knowledge_compiler: unknown variant")
	ErrNotImplemented   = errors.New("knowledge_compiler: variant not yet implemented")
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
	Guardrails            CapacityGuardrails
}

// Defaults returns a Param populated with safe production fallbacks.
func (p Param) Defaults() Param {
	p.SimilarityThreshold = 0.99
	p.MaxWorkers = 4
	p.Extra = map[string]any{}
	p.Guardrails = CapacityGuardrails{
		MaxItems:       50000,
		MaxVectorBytes: 200 << 20, // 200 MiB
		MaxOutputBytes: 400 << 20, // 400 MiB
		OnExceed:       ExceedError,
	}
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
	Meta    map[string]any
}

// Inputs is the resolved input set passed to a variant Run.
type Inputs struct {
	Chunks               []Chunk
	DocID                string // owning document id (defaults to DatasetID when empty)
	LLMID                string
	EmbeddingModel       string
	VariantSpecific      map[string]any
	HistoricalCandidates []Candidate // optional override path (test/offline)
	Sink                 ChunkedSink // optional; used when guardrails flush
}

// LogDeprecated emits a one-line deprecation notice for legacy param names.
func LogDeprecated(old, replacement string) {
	log.Printf("[knowledge_compiler] deprecated variant name %q; use %q", old, replacement)
}

// ChunkedSink receives products incrementally when capacity guardrails
// demand flushing instead of a single-shot Outputs().
type ChunkedSink interface {
	Emit(ctx context.Context, items []Product) error
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

// Outputs is the result of a variant Run.
type Outputs struct {
	Products          []Product
	DuplicatesDropped int
	Items             int
	VectorBytes       int64
	Flushed           bool
}

// SerializedBytes estimates the serialized footprint of the products.
func (o Outputs) SerializedBytes() int64 {
	var n int64
	for _, p := range o.Products {
		n += int64(len(p.Content)) + int64(len(p.Vector)*4) + 64
	}
	return n
}

// CapacityGuardrails bounds a single Invoke's product volume.
type CapacityGuardrails struct {
	MaxItems       int
	MaxVectorBytes int64
	MaxOutputBytes int64
	OnExceed       ExceedAction
}

// ExceedAction controls behaviour when a guardrail is breached.
type ExceedAction string

const (
	ExceedError ExceedAction = "error"
	ExceedFlush ExceedAction = "flush"
)

// Exceeded reports whether any guardrail is breached by o.
func (g CapacityGuardrails) Exceeded(o Outputs) bool {
	if g.MaxItems > 0 && len(o.Products) > g.MaxItems {
		return true
	}
	if g.MaxVectorBytes > 0 && o.VectorBytes > g.MaxVectorBytes {
		return true
	}
	if g.MaxOutputBytes > 0 && o.SerializedBytes() > g.MaxOutputBytes {
		return true
	}
	return false
}

// EnforceGuardrails applies the variant's capacity policy uniformly:
//   - OnExceed == ExceedError: returns ErrCapacityExceeded when any guardrail
//     is breached (caller must propagate the error and drop the outputs).
//   - OnExceed == ExceedFlush: emits the current products via sink and resets
//     the buffer; subsequent variant Run iterations can keep producing
//     without holding the full result set in memory. When sink is nil the
//     policy degrades to "no-op" (the buffer is preserved; downstream
//     callers may still see all products in the final merged chunks).
//
// Centralising the policy here keeps the five variants (structure/wiki/
// raptor/mindmap/datasetnav) consistent and prevents the gap where only
// structure used to honour the flush path.
func (o *Outputs) EnforceGuardrails(g CapacityGuardrails, sink ChunkedSink, ctx context.Context) error {
	if !g.Exceeded(*o) {
		return nil
	}
	switch g.OnExceed {
	case ExceedError:
		return ErrCapacityExceeded
	case ExceedFlush:
		if sink == nil {
			return nil
		}
		if err := sink.Emit(ctx, o.Products); err != nil {
			return err
		}
		o.Flushed = true
		// Hand off ownership to the sink (see FlushIfNeeded, M9) and reset all
		// per-buffer accounting so a reused Outputs starts empty and does not
		// re-trigger flushes on stale thresholds (M9 follow-up).
		o.Products = nil
		o.Items = 0
		o.VectorBytes = 0
		return nil
	default:
		return nil
	}
}

// ProductSink accumulates compiled products and flushes them to a ChunkedSink
// as the capacity guardrails are exceeded, bounding peak memory. A variant that
// may produce a large result set should accumulate through a ProductSink
// instead of appending to a plain slice and calling EnforceGuardrails only at
// the end: the end-of-run flush in EnforceGuardrails can only release memory
// AFTER the full result set has already been materialized, which defeats the
// anti-OOM goal (缺口 A).
//
// When OnExceed != ExceedFlush, or no sink is supplied, the sink behaves like a
// plain slice — every product stays in Products() until the caller decides what
// to do with it. This keeps the default (no-sink) pipeline behaviour unchanged:
// the full result set is returned in Outputs.Products for the component to merge
// into the upstream chunk stream.
type ProductSink struct {
	products []Product
	bytes    int64
	total    int
	flushed  bool
	g        CapacityGuardrails
	sink     ChunkedSink
	ctx      context.Context
}

// NewProductSink constructs a sink bound to the given guardrails and (optional)
// flush target.
func NewProductSink(ctx context.Context, g CapacityGuardrails, sink ChunkedSink) *ProductSink {
	return &ProductSink{g: g, sink: sink, ctx: ctx}
}

// Add appends p to the buffer and, when the flush policy is active and the
// guardrails are breached, emits the current buffer to the sink and resets it.
// This is what keeps peak memory near the guardrail threshold rather than the
// full result-set size.
func (s *ProductSink) Add(p Product) error {
	s.products = append(s.products, p)
	s.bytes += int64(len(p.Vector) * 4)
	s.total++
	if s.g.OnExceed == ExceedFlush && s.sink != nil {
		o := Outputs{Products: s.products, VectorBytes: s.bytes, Items: len(s.products)}
		if s.g.Exceeded(o) {
			if err := s.sink.Emit(s.ctx, s.products); err != nil {
				return err
			}
			s.flushed = true
			// Hand off ownership of the backing array to the sink (M9).
			s.products = nil
			s.bytes = 0
		}
	}
	return nil
}

// Products returns the not-yet-flushed buffer.
func (s *ProductSink) Products() []Product { return s.products }

// Bytes returns the serialized-vector footprint of the not-yet-flushed buffer.
func (s *ProductSink) Bytes() int64 { return s.bytes }

// TotalItems returns the cumulative number of products added (flushed + buffered),
// suitable for Outputs.Items.
func (s *ProductSink) TotalItems() int { return s.total }

// Flushed reports whether at least one incremental flush has happened.
func (s *ProductSink) Flushed() bool { return s.flushed }

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
