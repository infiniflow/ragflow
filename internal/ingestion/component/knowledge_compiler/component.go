// Package knowledge_compiler implements the KnowledgeCompiler ingestion
// component: a single runtime.Component that dispatches to one of the
// knowledge-compile variants (structure / wiki / tree / mindmap) based on the
// `variant` param. See PORT_PLAN.md for the full design.
package knowledge_compiler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"ragflow/internal/agent/runtime"
	clog "ragflow/internal/common"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/mindmap"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/ingestion/component/knowledge_compiler/tree"
	"ragflow/internal/ingestion/component/knowledge_compiler/wiki"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
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

// componentNameCompiler is the canonical, unified component name for the
// knowledge-compilation flow. It matches the Python side
// (rag/flow/compiler/compiler.py registers component_name = "Compiler"), so a
// canvas saved by the Python frontend and Go's built-in ingestion templates
// both reference the node as "Compiler" and resolve to the same component.
const componentNameCompiler = "Compiler"

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
	// Resolve the run-level tenant scope from the shared CanvasState.Globals
	// bag first (seeded by the pipeline at run start), falling back to the
	// component's own input map. Mirrors parser.go: it keeps the tenant id from
	// being lost when the upstream output map narrows it, which would otherwise
	// leave the template-group lookup with an empty tenant and fail loudly.
	tenantID := globals.GlobalOrInput(ctx, inputs, "tenant_id", "")
	// The ingestion pipeline seeds kb_id (not dataset_id) into the canvas
	// globals/inputs. In RAGFlow the knowledge base id IS the dataset id, so
	// fall back to kb_id when dataset_id is absent. This keeps the compiler's
	// dataset-scoped writes (merged wiki_page rows, wiki graph) keyed to the
	// correct dataset instead of an empty string.
	datasetID := globals.GlobalOrInput(ctx, inputs, "dataset_id", "")
	if datasetID == "" {
		datasetID = globals.GlobalOrInput(ctx, inputs, "kb_id", "")
	}

	// Resolve the compilation template spec(s). Priority:
	// compilation_template_id > compilation_template_group_id. The variant is
	// derived from each template's kind (see common.KindToVariant), not from
	// the DSL.
	specs, err := resolveTemplateSpecs(ctx, db, tenantID, param)
	if err != nil {
		return nil, err
	}

	// The eino wiring does not auto-merge run-level metadata into every
	// component's input map, so doc_id may arrive only via CanvasState.Globals
	// (seeded by the pipeline at run start) rather than inputs["doc_id"].
	// Mirror the dataset_id/kb_id resolution above: fall back to globals so the
	// compiler (and its wiki sub-logs) see the real doc_id instead of "unknown".
	if d, ok := inputs["doc_id"].(string); !ok || d == "" {
		if g := globals.GlobalOrInput(ctx, inputs, "doc_id", ""); g != "" {
			inputs["doc_id"] = g
		}
	}

	in, err := buildInputs(inputs, param)
	if err != nil {
		return nil, err
	}

	// Each spec compiles independently; products are stamped with the producing
	// template's id and kind. All products are buffered (no streaming sink), so
	// the post-run loop below covers every row (M1).
	var out common.Outputs
	for _, spec := range specs {
		variant, err := common.KindToVariant(spec.Kind)
		if err != nil {
			return nil, err
		}

		// Per-spec Param: variant + resolved template config, with scalar config
		// overrides (language, similarity, workers, dedup, llm/embedding) layered
		// on top of the DSL params and Extra. This is how the template's stored
		// "content" drives the compile without the caller passing variant-specific
		// params inline.
		specParam := param
		specParam.Variant = variant
		specParam.TemplateID = spec.ID
		specParam.TemplateConfig = spec.Config
		overlayTemplateConfig(&specParam, spec.Config)

		deps, err := common.ResolveDeps(tenantID, specParam.LLMID, specParam.EmbeddingModel)
		if err != nil {
			return nil, err
		}
		deps.TenantID = tenantID
		deps.DatasetID = datasetID

		// Per-spec Inputs copy: each spec must get its own VariantSpecific map,
		// otherwise specIn.VariantSpecific below would mutate the shared map and
		// let a later template inherit the previous template's parser_config
		// (a template with an empty config would then run with the wrong parser
		// behavior). Copy the map (and preserve an empty map when nil).
		specIn := in
		specIn.VariantSpecific = make(map[string]any, len(in.VariantSpecific)+1)
		for k, v := range in.VariantSpecific {
			specIn.VariantSpecific[k] = v
		}
		// The template config (flat: kind/entity/relation/plan/…) is delivered to
		// the structure and wiki variants under the "parser_config" key — the SAME
		// key those variants read (structure.Run / wikiPipeline.mapBatch do
		// VariantSpecific["parser_config"]). Storing it as "config" left the
		// variants with a nil config, so InferType saw no "kind" and fell back to
		// "list" (breaking timeline: its compile_kwd became "list" instead of
		// "timeline", so dropIsolatedTimelineEntities never ran and the timeline
		// rendered every entity isolated).
		//
		// Only overwrite when the template actually carries a config: an empty
		// template config (e.g. a resolver stub) must not clobber a parser_config
		// the caller already supplied on the inputs.
		if len(spec.Config) > 0 {
			specIn.VariantSpecific["parser_config"] = spec.Config
		}

		var o common.Outputs
		switch variant {
		case common.VariantStructure:
			o, err = structure.Run(ctx, deps, specParam, specIn)
		case common.VariantWiki:
			o, err = wiki.Run(ctx, deps, specParam, specIn)
		case common.VariantTree:
			o, err = tree.Run(ctx, deps, specParam, specIn)
		case common.VariantMindmap:
			o, err = mindmap.Run(ctx, deps, specParam, specIn)
		default:
			return nil, fmt.Errorf("%w: %q", common.ErrUnknownVariant, variant)
		}
		if err != nil {
			return nil, err
		}

		for i := range o.Products {
			if o.Products[i].Meta == nil {
				o.Products[i].Meta = map[string]any{}
			}
			o.Products[i].Meta["compilation_template_ids"] = []string{spec.ID}
			o.Products[i].Kind = spec.Kind
			o.Products[i].TemplateID = spec.ID
			o.Products[i].Variant = variant
		}
		out.Products = append(out.Products, o.Products...)
		out.AffectedPageSlugs = append(out.AffectedPageSlugs, o.AffectedPageSlugs...)
		out.RemovedPageSlugs = append(out.RemovedPageSlugs, o.RemovedPageSlugs...)
		out.WikiActiveStates = append(out.WikiActiveStates, o.WikiActiveStates...)
	}

	// Convert the compiled products into chunk-aligned docs (matching
	// conf/infinity_mapping.json) and merge them into the upstream input
	// chunks. The component stays DB-independent and no longer routes through a
	// separate writer seam: its output is plain chunks.
	compiled, err := productsToChunkDocs(out.Products)
	if err != nil {
		return nil, err
	}
	// Per-doc telemetry: confirm how many compiled rows (and specifically
	// wiki_page wiki sections vs pages) this single document produced, so a
	// missing dataset-level wiki_page can be traced to "never generated" vs
	// "generated then dropped".
	var pageCount, sectionCount, entityCount, relationCount int
	for _, p := range out.Products {
		switch {
		case p.Variant == common.VariantWiki && metaString(p.Meta, "kind") == "page":
			pageCount++
		case p.Variant == common.VariantWiki && metaString(p.Meta, "kind") == "section":
			sectionCount++
		case p.Variant == common.VariantWiki && metaString(p.Meta, "kind") == "entity":
			entityCount++
		case p.Variant == common.VariantWiki && metaString(p.Meta, "kind") == "relation":
			relationCount++
		}
	}
	clog.Info("knowledge_compiler: per-doc products generated",
		zap.String("doc_id", in.DocID),
		zap.Int("total_products", len(out.Products)),
		zap.Int("wiki_page", pageCount),
		zap.Int("wiki_section", sectionCount),
		zap.Int("wiki_entity", entityCount),
		zap.Int("wiki_relation", relationCount),
		zap.Int("chunk_docs", len(compiled)),
	)
	result := mergeChunks(inputs, compiled)
	if len(out.AffectedPageSlugs) > 0 {
		result["wiki_affected_slugs"] = uniqueSorted(out.AffectedPageSlugs)
	}
	if len(out.RemovedPageSlugs) > 0 {
		result["wiki_removed_slugs"] = uniqueSorted(out.RemovedPageSlugs)
	}
	if len(out.WikiActiveStates) > 0 {
		result["wiki_active_map_states"] = wikiActiveStateValues(out.WikiActiveStates)
	}
	return result, nil
}

// wikiActiveStateValues keeps the component output checkpoint-safe. Pipeline
// node outputs cross an eino serialization boundary, so package-specific Go
// structs must not escape in map[string]any values.
func wikiActiveStateValues(states []common.WikiMapActiveState) []map[string]any {
	values := make([]map[string]any, 0, len(states))
	for _, state := range states {
		values = append(values, map[string]any{
			"key":         state.Key,
			"tenant_id":   state.TenantID,
			"dataset_id":  state.DatasetID,
			"document_id": state.DocumentID,
			"payload":     string(state.Payload),
		})
	}
	return values
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// resolveTemplateSpecs resolves the configured compilation template spec(s) to
// their TemplateInfo rows. Priority: compilation_template_id >
// compilation_template_group_id. The group path resolves the group to its
// child template ids and then loads each template.
func resolveTemplateSpecs(ctx context.Context, db *gorm.DB, tenantID string, param common.Param) ([]common.TemplateInfo, error) {
	if param.CompilationTemplateID != "" {
		info, err := common.ResolveTemplate(ctx, db, tenantID, param.CompilationTemplateID)
		if err != nil {
			return nil, err
		}
		return []common.TemplateInfo{info}, nil
	}
	if param.CompilationTemplateGroupID != "" {
		ids, err := common.ResolveGroupTemplateIDs(ctx, db, tenantID, []string{param.CompilationTemplateGroupID})
		if err != nil {
			return nil, err
		}
		specs := make([]common.TemplateInfo, 0, len(ids))
		for _, id := range ids {
			info, err := common.ResolveTemplate(ctx, db, tenantID, id)
			if err != nil {
				return nil, err
			}
			specs = append(specs, info)
		}
		return specs, nil
	}
	return nil, fmt.Errorf("knowledge_compiler: one of compilation_template_id or compilation_template_group_id is required")
}

// overlayTemplateConfig layers scalar fields from the resolved template config
// (the template "content") onto the param. For self-documenting defaults
// (language, similarity_threshold, max_workers, enable_historical_dedup) the
// config value wins when the param is unset. For llm_id / embedding_model, the
// caller's per-call override always wins; the config only fills them in when
// the caller left them empty.
func overlayTemplateConfig(param *common.Param, cfg map[string]any) {
	if cfg == nil {
		return
	}
	if v, ok := cfg["language"].(string); ok && v != "" {
		param.Language = v
	}
	if v, ok := cfg["similarity_threshold"].(float64); ok && v > 0 {
		param.SimilarityThreshold = v
	}
	if v, ok := cfg["max_workers"].(float64); ok && int(v) > 0 {
		param.MaxWorkers = int(v)
	}
	if v, ok := cfg["enable_historical_dedup"].(bool); ok {
		param.EnableHistoricalDedup = v
	}
	if param.Plan == nil {
		if v, ok := cfg["no_plan"].(bool); ok && v {
			disabled := false
			param.Plan = &disabled
		} else if v, ok := cfg["plan"].(bool); ok {
			param.Plan = &v
		}
	}
	// llm_id / embedding_model are optional per-call overrides documented on
	// Invoke. The template config supplies defaults, so only apply them when
	// the caller has not already provided an explicit value (the caller wins).
	if v, ok := cfg["llm_id"].(string); ok && v != "" && param.LLMID == "" {
		param.LLMID = v
	}
	if v, ok := cfg["embedding_model"].(string); ok && v != "" && param.EmbeddingModel == "" {
		param.EmbeddingModel = v
	}
}

// kindOrVariant returns the original template kind when present (the true
// compilation_template.kind, e.g. "page_index"), otherwise the collapsed Go
// variant. It drives the compilation_template_kind_kwd stamp.
func kindOrVariant(p common.Product) string {
	if p.Kind != "" {
		return p.Kind
	}
	return string(p.Variant)
}

// variantCompileKWD maps each Go variant to the compile_kwd discriminator value
// Python writes into ES (rag/advanced_rag/knowlege_compile). It is the primary
// key that distinguishes compiled knowledge units from ordinary chunks and
// routes retrieval-side filters. The wiki value MUST be "wiki_page" (Python's
// canonical WIKI_PAGE_COMPILE_KWD in wiki.py:1661 / wiki_incremental.py:44 /
// dataset_wiki_generator.py:108) so Go-produced wiki pages are visible to the
// artifact API (dataset_artifact_service.go reads compile_kwd="wiki_page").
var variantCompileKWD = map[common.Variant]string{
	common.VariantStructure: "structure",
	common.VariantWiki:      "wiki_page",
	common.VariantTree:      "tree",
	common.VariantMindmap:   "mindmap",
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
		// Wiki sub-parts: sections get their own compile_kwd so that a page
		// search on compile_kwd="wiki_page" returns pages only (page.go emits
		// both kind:"page" and kind:"section" rows under VariantWiki). This is
		// the schema-backed page/section discriminator: "wiki_page" == page,
		// "wiki_section" == a page sub-section.
		if p.Variant == common.VariantWiki && metaString(p.Meta, "kind") == "section" && compileKWD == "wiki_page" {
			compileKWD = "wiki_section"
		}
		if compileKWD == "" {
			compileKWD = string(p.Variant)
		}
		if err := doc.SetExtraValue("compile_kwd", compileKWD); err != nil {
			return nil, err
		}
		if err := doc.SetExtraValue("compilation_template_kind_kwd", kindOrVariant(p)); err != nil {
			return nil, err
		}
		// scope_kwd marks doc/dataset-level rows (B8/O1=B). A doc-level compiled
		// product is always a per-document (scope="doc") input to the dataset-level
		// merge; the consumer rewrites dataset-level rows with scope_kwd="dataset".
		// This is the dataset-level writer-layer discriminator, not a doc-level
		// compile-internal concern.
		if err := doc.SetExtraValue("scope_kwd", "doc"); err != nil {
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
		return applyStructureGraphColumns(doc, p, kind)

	case common.VariantWiki:
		// One artifact_page row per wiki page; section rows reuse the same
		// page-level columns so retrieval-side filters work uniformly.
		// Match the Python writer contract (api/db/db_models.py slug_kwd):
		// slug_kwd stores the full "<page_type>/<slug>" form, so retrieval
		// filters (GetWikiPage) can reconstruct it directly. page_type is also
		// stored separately for topic grouping.
		if pageType := metaString(p.Meta, "page_type"); pageType != "" {
			if slug := metaString(p.Meta, "slug"); slug != "" {
				// Normalize to the full "<page_type>/<slug>" form (Python writer
				// contract). Idempotent: a slug that already carries the prefix
				// (some producers emit pageType/slug directly) is left as-is.
				fullSlug := slug
				if !strings.Contains(slug, "/") {
					fullSlug = pageType + "/" + slug
				}
				if err := doc.SetExtraValue("slug_kwd", fullSlug); err != nil {
					return err
				}
				if err := doc.SetExtraValue("artifact_slug_kwd", fullSlug); err != nil {
					return err
				}
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

	case common.VariantTree:
		switch kind {
		case "entity", "relation", "graph":
			// The tree is also projected onto the structure-graph shape (Python
			// raptor_tree_to_graph + _struct_upsert_tree_graph_rows): entity /
			// relation rows carry knowledge_graph_kwd and the compact graph blob
			// (kind "graph") is the /structure/graph discovery row. This is the
			// same storage contract as the structure variant, so both share
			// applyStructureGraphColumns.
			return applyStructureGraphColumns(doc, p, kind)
		default:
			// RAPTOR summary/root rows: raptor_kwd tags the node kind;
			// raptor_layer_int records tree depth.
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
		}

	case common.VariantMindmap:
		// Mindmap now emits entity/relation rows (plan §1.2) so it participates in
		// dataset-level merge exactly like graph/timeline: each node is an entity,
		// each parent→child edge is a relation. Reuse the shared structure-graph
		// column contract (knowledge_graph_kwd + from/to_entity_kwd + name_kwd +
		// entity_type_kwd + mention_count_int). The relation type lives in the
		// content_with_weight payload ({"from","to","type"}), matching Python —
		// NOT a dedicated relation_type_kwd column.
		return applyStructureGraphColumns(doc, p, kind)
	}

	return nil
}

// applyStructureGraphColumns emits the structure-graph row columns shared by the
// structure and tree variants (Python _struct_to_doc_storage_doc contract):
//   - knowledge_graph_kwd: "entity" | "relation" | "graph"
//   - relations: from_entity_kwd / to_entity_kwd
//   - entities: name_kwd (lowercased) / entity_type_kwd
//   - mention_count_int
//
// Keeping this in one helper prevents the two variants' storage contracts from
// diverging (review Major).
func applyStructureGraphColumns(doc *schema.ChunkDoc, p common.Product, kind string) error {
	if kind != "" {
		if err := doc.SetExtraValue("knowledge_graph_kwd", kind); err != nil {
			return err
		}
	}
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
	// Accept both the []any and []map[string]any chunk carriers (the chunker
	// emits the latter; buildInputs already handles both). Without this, the
	// original source chunks would be dropped when the carrier is []map[string]any.
	var raw []any
	switch v := inputs["chunks"].(type) {
	case []any:
		raw = v
	case []map[string]any:
		raw = make([]any, 0, len(v))
		for _, m := range v {
			raw = append(raw, m)
		}
	default:
		log.Printf("knowledge_compiler: mergeChunks: unexpected chunks type %T", inputs["chunks"])
	}
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
// mapKeys returns the sorted keys of m, for diagnostics logging.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func buildInputs(inputs map[string]any, param common.Param) (common.Inputs, error) {
	in := common.Inputs{
		LLMID:           param.LLMID,
		EmbeddingModel:  param.EmbeddingModel,
		VariantSpecific: map[string]any{},
	}
	if d, ok := inputs["doc_id"].(string); ok && d != "" {
		in.DocID = d
	}
	// The upstream pipeline hands chunks over as a []any of map[string]any in
	// some paths and as a []map[string]any in others (the chunker emits the
	// latter). Accept both so the knowledge compiler never silently drops the
	// whole upstream output on a type mismatch.
	var raw []map[string]any
	switch v := inputs["chunks"].(type) {
	case []any:
		raw = make([]map[string]any, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			raw = append(raw, m)
		}
	case []map[string]any:
		raw = v
	default:
		log.Printf("knowledge_compiler: buildInputs: unexpected chunks type %T", inputs["chunks"])
	}
	log.Printf("knowledge_compiler: buildInputs: accepted %d chunk(s) from inputs[chunks]", len(raw))
	for _, m := range raw {
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
		if vec, err := common.VectorFromChunkMap(m, 0); err == nil {
			ch.Vector = vec
		}
		in.Chunks = append(in.Chunks, ch)
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
	// Register under the single unified name "Compiler" (matching the Python
	// side) so both Python-saved canvases and Go's built-in ingestion templates
	// resolve to the same component without name translation.
	meta := runtime.Metadata{
		Version: "0.1.0",
		Inputs: map[string]string{
			"chunks":                "Upstream chunker/parser output chunks (id + text/content_with_weight).",
			"historical_candidates": "Optional historical dedup candidates for offline/test runs.",
		},
		Outputs: chunkerOutputs,
	}
	runtime.MustRegister(componentNameCompiler, runtime.CategoryIngestion,
		NewKnowledgeCompilerComponent, meta)
}
