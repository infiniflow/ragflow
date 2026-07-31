// Package knowledge_compiler implements the KnowledgeCompiler ingestion
// component: a single runtime.Component that dispatches to one of the
// knowledge-compile variants (structure / wiki / raptor / mindmap / datasetnav)
// based on the `variant` param. See PORT_PLAN.md for the full design.
package knowledge_compiler

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/datasetnav"
	"ragflow/internal/ingestion/component/knowledge_compiler/mindmap"
	"ragflow/internal/ingestion/component/knowledge_compiler/raptor"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/ingestion/component/knowledge_compiler/wiki"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"

	"gorm.io/gorm"
)

// chunkerOutputs mirrors chunker.ChunkerOutputs so this component's registered
// output schema is byte-for-byte identical to the upstream TokenChunker's. It is
// declared locally (rather than importing the chunker package) to keep the
// knowledge_compiler package free of the chunker's CGO-native dependencies.
var chunkerOutputs = map[string]string{
	"output_format": "Always \"chunks\" on success.",
	"chunks":        "list[object]: per-chunk map (text + optional meta keys).",
	"name":          "Source document name, carried forward from upstream (pass-through) when present — Tokenizer consumes it for title embedding.",
	"tenant_id":     "Carried forward from upstream (pass-through) when present — Tokenizer consumes it to resolve the embedding model.",
	"kb_id":         "Carried forward from upstream (pass-through) when present — Tokenizer consumes it to resolve the embedding model.",
	"_ERROR":        "Set only on validation failure.",
}

const componentNameKnowledgeCompiler = "KnowledgeCompiler"

// KnowledgeCompilerComponent is the runtime.Component surface. Param is set at
// construction from the DSL; per-call overrides flow through the inputs map.
type KnowledgeCompilerComponent struct {
	Param common.Param
}

// NewKnowledgeCompilerComponent builds the component from a DSL params map.
// The name argument matches the runtime.ComponentFactory signature (ignored
// here; the component is registered under a single fixed name).
func NewKnowledgeCompilerComponent(name string, params map[string]any) (runtime.Component, error) {
	p, err := common.ParseParam(params)
	if err != nil {
		return nil, err
	}
	return &KnowledgeCompilerComponent{Param: p}, nil
}

// Inputs documents the component's input surface for the catalog.
func (c *KnowledgeCompilerComponent) Inputs() map[string]string {
	return map[string]string{
		"chunks":                "List of map[string]any from upstream chunker/parser; each must carry id + text/content_with_weight.",
		"llm_id":                "Optional per-call LLM id override.",
		"embedding_model":       "Optional per-call embedding model override.",
		"tenant_id":             "Optional tenant scope (defaults to resolver context).",
		"dataset_id":            "Optional dataset scope (wiki historical dedup).",
		"historical_candidates": "Optional []common.Candidate override for historical dedup (test/offline).",
	}
}

// Outputs documents the component's output surface. It is intentionally
// identical to the upstream chunker's output schema: the compiled knowledge
// units are expressed as chunks (schema-aligned to conf/infinity_mapping.json)
// and merged into the upstream input chunks, so downstream components (e.g. the
// Tokenizer) consume them exactly as they would normal chunks.
func (c *KnowledgeCompilerComponent) Outputs() map[string]string {
	return chunkerOutputs
}

// Invoke resolves deps, builds Inputs, and dispatches to the variant Run. The
// variant returns its compiled knowledge units as internal Product rows; those
// are converted to chunk-aligned docs (conf/infinity_mapping.json schema) and
// merged with the upstream input chunks. The result is the chunker-shaped map
// {output_format:"chunks", chunks:[...]}.
func (c *KnowledgeCompilerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	_ = db
	param := c.Param
	if v, ok := inputs["llm_id"].(string); ok && v != "" {
		param.LLMID = v
	}
	if v, ok := inputs["embedding_model"].(string); ok && v != "" {
		param.EmbeddingModel = v
	}
	tenantID, _ := inputs["tenant_id"].(string)
	datasetID, _ := inputs["dataset_id"].(string)

	// Validate the variant before resolving deps so a bad variant fails fast
	// with ErrUnknownVariant rather than a deps-resolution error.
	switch param.Variant {
	case common.VariantStructure, common.VariantWiki, common.VariantRaptor,
		common.VariantMindmap, common.VariantDatasetnav:
		// recognised; dispatch below
	default:
		return nil, fmt.Errorf("%w: %q", common.ErrUnknownVariant, param.Variant)
	}

	deps, err := common.ResolveDeps(tenantID, param.LLMID, param.EmbeddingModel)
	if err != nil {
		return nil, err
	}
	deps.TenantID = tenantID
	deps.DatasetID = datasetID

	// Resolve compilation-template-group ids to concrete template ids and merge
	// them with any explicitly-passed template ids. Group-only configs that
	// cannot be resolved fail loudly rather than silently emitting rows missing
	// compilation_template_ids (a data-loss path).
	resolvedGroups, err := common.ResolveGroupTemplateIDs(ctx, tenantID, param.GroupIDs)
	if err != nil {
		return nil, err
	}
	templateIDs := append(append([]string{}, param.TemplateIDs...), resolvedGroups...)

	in, err := buildInputs(inputs, param)
	if err != nil {
		return nil, err
	}

	// Stamp the resolved template ids onto every compiled product after the
	// variant returns. All products are buffered in out.Products (there is no
	// streaming sink path), so the post-Run loop below covers every row (M1).
	var out common.Outputs
	switch param.Variant {
	case common.VariantStructure:
		out, err = structure.Run(ctx, deps, param, in)
	case common.VariantWiki:
		out, err = wiki.Run(ctx, deps, param, in)
	case common.VariantRaptor:
		out, err = raptor.Run(ctx, deps, param, in)
	case common.VariantMindmap:
		out, err = mindmap.Run(ctx, deps, param, in)
	case common.VariantDatasetnav:
		out, err = datasetnav.Run(ctx, deps, param, in)
	default:
		return nil, fmt.Errorf("%w: %q", common.ErrUnknownVariant, param.Variant)
	}
	if err != nil {
		return nil, err
	}

	// Stamp the compilation template ids onto every product so the chunk
	// converter can emit compilation_template_ids (Python stamps one
	// template_id per row; here we carry the full resolved list since the Go
	// component runs one variant per Invoke and the caller may pass multiple
	// template ids from a compilation-template-group). The list is the union of
	// explicitly-passed template ids and any resolved from group ids.
	if len(templateIDs) > 0 {
		for i := range out.Products {
			if out.Products[i].Meta == nil {
				out.Products[i].Meta = map[string]any{}
			}
			out.Products[i].Meta["compilation_template_ids"] = templateIDs
		}
	}

	// Convert the compiled products into chunk-aligned docs (matching
	// conf/infinity_mapping.json) and merge them into the upstream input
	// chunks. The component stays DB-independent and no longer routes through a
	// separate writer seam: its output is plain chunks.
	compiled, err := productsToChunkDocs(out.Products)
	if err != nil {
		return nil, err
	}
	return mergeChunks(inputs, compiled), nil
}

// variantCompileKWD maps each Go variant to the compile_kwd discriminator value
// Python writes into ES (rag/advanced_rag/knowlege_compile). It is the primary
// key that distinguishes compiled knowledge units from ordinary chunks and
// routes retrieval-side filters (e.g. "compile_kwd": ["artifact_page"]).
var variantCompileKWD = map[common.Variant]string{
	common.VariantStructure:  "structure",
	common.VariantWiki:       "artifact_page",
	common.VariantRaptor:     "raptor",
	common.VariantMindmap:    "mindmap",
	common.VariantDatasetnav: "dataset_nav",
}

// productsToChunkDocs converts the internal compiled Product rows into
// schema.ChunkDoc values aligned to conf/infinity_mapping.json (lines 1–77).
// The compile_kwd discriminator marks them as compiled knowledge units
// (distinct from plain chunks); variant-specific columns are populated from
// Product.Meta using stable keys (see each variant's build site for the
// contract). The original kind/level/name/size meta is also carried under
// "kc_"-prefixed Extra keys so no information is lost.
func productsToChunkDocs(products []common.Product) ([]schema.ChunkDoc, error) {
	docs := make([]schema.ChunkDoc, 0, len(products))
	for _, p := range products {
		doc := schema.ChunkDoc{
			Text:              p.Content,
			ContentWithWeight: p.Content,
		}
		// Populate content_ltks / content_sm_ltks the same way the chunker
		// components do (see chunker/tag.go, chunker/qa.go): coarse tokenize
		// the content, then fine-grained tokenize the coarse tokens. Errors
		// are ignored (tokenizer pool may be uninitialised in no-CGo tests),
		// leaving the fields empty — matching the chunker's graceful-degrade
		// behaviour.
		if ltks, err := tokenizer.Tokenize(p.Content); err == nil && ltks != "" {
			doc.ContentLtks = ltks
			if sm, err := tokenizer.FineGrainedTokenize(ltks); err == nil && sm != "" {
				doc.ContentSmLtks = sm
			}
		}
		// Common identity columns.
		if err := doc.SetExtraValue("id", p.ID); err != nil {
			return nil, err
		}
		if p.DocID != "" {
			if err := doc.SetExtraValue("doc_id", p.DocID); err != nil {
				return nil, err
			}
		}
		if p.TenantID != "" {
			if err := doc.SetExtraValue("tenant_id", p.TenantID); err != nil {
				return nil, err
			}
		}
		compileKWD := variantCompileKWD[p.Variant]
		// A variant may pin a finer-grained compile_kwd per row via Meta
		// (structure stamps the inferred compile kind — list/set/hypergraph —
		// mirroring Python's per-row autotype stamp).
		if v := metaString(p.Meta, "compile_kwd"); v != "" {
			compileKWD = v
		}
		if compileKWD == "" {
			compileKWD = string(p.Variant)
		}
		if err := doc.SetExtraValue("compile_kwd", compileKWD); err != nil {
			return nil, err
		}
		if err := doc.SetExtraValue("compilation_template_kind_kwd", string(p.Variant)); err != nil {
			return nil, err
		}
		if p.ParentID != "" {
			if err := doc.SetExtraValue("parent_kwd", p.ParentID); err != nil {
				return nil, err
			}
		}
		if len(p.Vector) > 0 {
			if err := doc.SetExtraValue(fmt.Sprintf("q_%d_vec", len(p.Vector)), p.Vector); err != nil {
				return nil, err
			}
		}
		// Provenance columns shared by all compiled rows.
		if ids := metaStringSlice(p.Meta, "source_chunk_ids"); len(ids) > 0 {
			if err := doc.SetExtraValue("source_chunk_ids", ids); err != nil {
				return nil, err
			}
		}
		if ids := metaStringSlice(p.Meta, "source_doc_ids"); len(ids) > 0 {
			if err := doc.SetExtraValue("source_doc_ids", ids); err != nil {
				return nil, err
			}
		}
		// compilation_template_ids: the resolved template ids that produced
		// this row (Python stamps one per row from the active template; the
		// document-structure endpoint groups rows by this column).
		if ids := metaStringSlice(p.Meta, "compilation_template_ids"); len(ids) > 0 {
			if err := doc.SetExtraValue("compilation_template_ids", ids); err != nil {
				return nil, err
			}
		}
		// Per-variant fine-grained columns (conf/infinity_mapping.json §45–77).
		if err := applyVariantColumns(&doc, p); err != nil {
			return nil, err
		}
		// Preserve the raw Product.Meta under kc_* for round-trip fidelity.
		for k, v := range p.Meta {
			if err := doc.SetExtraValue("kc_"+k, v); err != nil {
				return nil, err
			}
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// applyVariantColumns emits the compile-specific columns defined in
// conf/infinity_mapping.json lines 45–77, driven by Product.Meta keys that
// each variant's build site populates. Unknown/absent keys are skipped.
func applyVariantColumns(doc *schema.ChunkDoc, p common.Product) error {
	kind := metaString(p.Meta, "kind")

	switch p.Variant {
	case common.VariantStructure:
		// knowledge_graph_kwd: "entity" | "relation" | "graph".
		if kind != "" {
			if err := doc.SetExtraValue("knowledge_graph_kwd", kind); err != nil {
				return err
			}
		}
		// Relations carry from/to entity endpoints (from_entity_kwd / to_entity_kwd).
		if kind == "relation" {
			if v := metaString(p.Meta, "from"); v != "" {
				if err := doc.SetExtraValue("from_entity_kwd", v); err != nil {
					return err
				}
			}
			if v := metaString(p.Meta, "to"); v != "" {
				if err := doc.SetExtraValue("to_entity_kwd", v); err != nil {
					return err
				}
			}
		}
		// Entities carry their canonical name on name_kwd (lowercased, mirroring
		// Python's _struct_to_doc_storage_doc; the structure-graph endpoints
		// filter/sort on it) plus entity_type_kwd and mention_count_int.
		if kind == "entity" {
			if v := metaString(p.Meta, "name"); v != "" {
				if err := doc.SetExtraValue("name_kwd", strings.ToLower(v)); err != nil {
					return err
				}
			}
			if v := metaString(p.Meta, "entity_type"); v != "" {
				if err := doc.SetExtraValue("entity_type_kwd", v); err != nil {
					return err
				}
			}
		}
		if v, ok := metaInt(p.Meta, "mention_count"); ok {
			if err := doc.SetExtraValue("mention_count_int", v); err != nil {
				return err
			}
		}

	case common.VariantWiki:
		// One artifact_page row per wiki page; section rows reuse the same
		// page-level columns so retrieval-side filters work uniformly.
		if v := metaString(p.Meta, "slug"); v != "" {
			if err := doc.SetExtraValue("slug_kwd", v); err != nil {
				return err
			}
			if err := doc.SetExtraValue("artifact_slug_kwd", v); err != nil {
				return err
			}
		}
		if v := metaString(p.Meta, "title"); v != "" {
			if err := doc.SetExtraValue("title_kwd", v); err != nil {
				return err
			}
			setTitleTokens(doc, v)
		}
		if v := metaString(p.Meta, "page_type"); v != "" {
			if err := doc.SetExtraValue("page_type_kwd", v); err != nil {
				return err
			}
		}
		if v := metaString(p.Meta, "topic"); v != "" {
			if err := doc.SetExtraValue("topic_kwd", v); err != nil {
				return err
			}
		}
		if v := metaString(p.Meta, "summary"); v != "" {
			if err := doc.SetExtraValue("summary_with_weight", v); err != nil {
				return err
			}
		}
		// Section rows also carry level/index so a retriever can scope to a
		// sub-section of a wiki page.
		if v, ok := metaInt(p.Meta, "section_level"); ok {
			if err := doc.SetExtraValue("section_level_int", v); err != nil {
				return err
			}
			if err := doc.SetExtraValue("depth_int", v); err != nil {
				return err
			}
		}
		if v, ok := metaInt(p.Meta, "section_index"); ok {
			if err := doc.SetExtraValue("section_index_int", v); err != nil {
				return err
			}
		}
		if v := metaStringSlice(p.Meta, "entity_names"); len(v) > 0 {
			if err := doc.SetExtraValue("entity_names_kwd", v); err != nil {
				return err
			}
		}
		if v := metaStringSlice(p.Meta, "outlinks"); len(v) > 0 {
			if err := doc.SetExtraValue("outlinks_kwd", v); err != nil {
				return err
			}
			if err := doc.SetExtraValue("outlinks_int", len(v)); err != nil {
				return err
			}
		}
		if v := metaStringSlice(p.Meta, "related_kb_pages"); len(v) > 0 {
			if err := doc.SetExtraValue("related_kb_pages_kwd", v); err != nil {
				return err
			}
		}

	case common.VariantRaptor:
		// raptor_kwd tags summary/root nodes; raptor_layer_int records tree depth.
		if kind != "" {
			if err := doc.SetExtraValue("raptor_kwd", kind); err != nil {
				return err
			}
		}
		if v, ok := metaInt(p.Meta, "level"); ok {
			if err := doc.SetExtraValue("raptor_layer_int", v); err != nil {
				return err
			}
			if err := doc.SetExtraValue("depth_int", v); err != nil {
				return err
			}
		}
		if v := metaStringSlice(p.Meta, "children"); len(v) > 0 {
			if err := doc.SetExtraValue("children_kwd", v); err != nil {
				return err
			}
		}

	case common.VariantMindmap:
		// Tree nodes: depth_int records the outline level.
		if v, ok := metaInt(p.Meta, "level"); ok {
			if err := doc.SetExtraValue("depth_int", v); err != nil {
				return err
			}
		}
		if v := metaString(p.Meta, "name"); v != "" {
			if err := doc.SetExtraValue("title_kwd", v); err != nil {
				return err
			}
			setTitleTokens(doc, v)
		}
		if v := metaStringSlice(p.Meta, "children"); len(v) > 0 {
			if err := doc.SetExtraValue("children_kwd", v); err != nil {
				return err
			}
		}

	case common.VariantDatasetnav:
		// nav_cluster / nav_doc rows: type_kwd discriminates the row kind.
		if v := metaString(p.Meta, "type"); v != "" {
			if err := doc.SetExtraValue("type_kwd", v); err != nil {
				return err
			}
		}
		if v := metaString(p.Meta, "name"); v != "" {
			if err := doc.SetExtraValue("title_kwd", v); err != nil {
				return err
			}
			setTitleTokens(doc, v)
		}
		if v, ok := metaInt(p.Meta, "depth"); ok {
			if err := doc.SetExtraValue("depth_int", v); err != nil {
				return err
			}
		}
		if v, ok := metaInt(p.Meta, "size"); ok {
			if err := doc.SetExtraValue("doc_count_int", v); err != nil {
				return err
			}
		}
		if v := metaStringSlice(p.Meta, "doc_ids"); len(v) > 0 {
			if err := doc.SetExtraValue("doc_ids_kwd", v); err != nil {
				return err
			}
		}
	}

	return nil
}

// metaString reads a string-valued Product.Meta key.
func metaString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// setTitleTokens populates the ChunkDoc title_tks / title_sm_tks fields from a
// title string, mirroring how the chunker/tokenizer components tokenize titles
// (coarse → TitleTks, fine-grained → TitleSmTks). Errors are ignored: when the
// tokenizer pool is uninitialised (no-CGo test path) the fields stay empty,
// matching the chunker's graceful-degrade behaviour.
func setTitleTokens(doc *schema.ChunkDoc, title string) {
	if title == "" {
		return
	}
	if tks, err := tokenizer.Tokenize(title); err == nil && tks != "" {
		doc.TitleTks = tks
		if sm, err := tokenizer.FineGrainedTokenize(tks); err == nil && sm != "" {
			doc.TitleSmTks = sm
		}
	}
}

// metaInt reads an int-valued Product.Meta key (tolerant of float64 from JSON).
func metaInt(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}

// metaStringSlice reads a []string Product.Meta key (tolerant of []any).
func metaStringSlice(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// mergeChunks returns the canonical chunker-shaped output: the upstream input
// chunks followed by the freshly compiled chunk docs, all under the "chunks"
// key with output_format "chunks". This makes KnowledgeCompiler's output schema
// byte-for-byte identical to the upstream chunker's, including the pass-through
// envelope (name / tenant_id / kb_id) the advertised contract promises. In a
// full pipeline those identity keys also live in CanvasState.Globals, but
// headless / manual chaining reads them from the component output map, so they
// must be forwarded when present.
func mergeChunks(inputs map[string]any, compiled []schema.ChunkDoc) map[string]any {
	raw, _ := inputs["chunks"].([]any)
	merged := make([]any, 0, len(raw)+len(compiled))
	for _, r := range raw {
		merged = append(merged, r)
	}
	for _, c := range compiled {
		merged = append(merged, c.ToMap())
	}
	out := map[string]any{
		"output_format": "chunks",
		"chunks":        merged,
	}
	// Forward the chunker-shaped pass-through envelope so downstream
	// components (e.g. Tokenizer) see the same top-level identity keys the
	// upstream chunker would carry. Only present keys are forwarded.
	for _, k := range []string{"name", "tenant_id", "kb_id"} {
		if v, ok := inputs[k]; ok {
			out[k] = v
		}
	}
	return out
}

// buildInputs converts the runtime inputs map into a typed common.Inputs.
// It is necessary because the pipeline passes components a generic
// map[string]any contract while every variant Run consumes a strongly-typed
// common.Inputs: this function is the single translation seam that decouples
// the dependency-light common package (and thus the variants) from the raw
// serialization shape, and the one place where inputs are validated, defaulted,
// and enriched (e.g. extracting each chunk's pre-computed embedding) before any
// LLM/embedding work begins.
func buildInputs(inputs map[string]any, param common.Param) (common.Inputs, error) {
	in := common.Inputs{
		LLMID:           param.LLMID,
		EmbeddingModel:  param.EmbeddingModel,
		VariantSpecific: map[string]any{},
	}
	if d, ok := inputs["doc_id"].(string); ok && d != "" {
		in.DocID = d
	}
	if raw, ok := inputs["chunks"].([]any); ok {
		for _, r := range raw {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			ch := common.Chunk{Meta: m}
			if id, ok := m["id"].(string); ok {
				ch.ID = id
			}
			if t, ok := m["text"].(string); ok {
				ch.Text = t
			}
			if cw, ok := m["content_with_weight"].(string); ok {
				ch.Content = cw
			}
			// Reuse the embedding the upstream pipeline already computed on the
			// chunk (stored under q_<dim>_vec); variants fall back to embedding
			// on demand when it is absent. A chunk must carry exactly one vector.
			vec, err := common.VectorFromChunkMap(m, 0)
			if err != nil {
				return in, err
			}
			ch.Vector = vec
			in.Chunks = append(in.Chunks, ch)
		}
	}
	if hc, ok := inputs["historical_candidates"].([]common.Candidate); ok {
		in.HistoricalCandidates = hc
	}
	known := map[string]bool{
		"doc_id": true, "chunks": true, "historical_candidates": true,
		"llm_id": true, "embedding_model": true, "tenant_id": true, "dataset_id": true,
	}
	for k, v := range inputs {
		if !known[k] {
			in.VariantSpecific[k] = v
		}
	}
	return in, nil
}

func init() {
	runtime.MustRegister(componentNameKnowledgeCompiler, runtime.CategoryIngestion,
		NewKnowledgeCompilerComponent, runtime.Metadata{
			Version: "0.1.0",
			Inputs: map[string]string{
				"chunks":          "Upstream chunker/parser output chunks (id + text/content_with_weight).",
				"llm_id":          "Optional LLM id override.",
				"embedding_model": "Optional embedding model override.",
				"tenant_id":       "Optional tenant scope.",
				"dataset_id":      "Optional dataset scope (wiki historical dedup).",
			},
			Outputs: chunkerOutputs,
		})
}
