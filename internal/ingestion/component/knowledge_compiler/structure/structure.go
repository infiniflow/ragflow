// Package structure implements the "structure" variant of KnowledgeCompiler:
// document-level structure compilation (list / set / hypergraph — the graph
// kind) as a two-stage entity → relation LLM extraction with template-driven
// prompts, followed by LLM-judged in-run merge dedup. Stage semantics and
// prompts mirror Python's rag/advanced_rag/knowlege_compile/structure.py; the
// Go port keeps all intermediate state in memory (no ES reads/writes).
package structure

import (
	"context"
	"fmt"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// batchSubmitter fans out the MAP-stage extraction jobs on the process-wide
// knowledge-compilation pool. It is injected by the knowledge_compiler wiring
// (component.go) so every stage shares one vCPU-sized concurrency bound; when
// nil the batches run sequentially (the historic default).
var batchSubmitter func(ctx context.Context, jobs []func() error) error

// SetBatchSubmitter installs the shared-pool fan-out used by Run's MAP stage.
// Pass nil to revert to serial execution.
func SetBatchSubmitter(submit func(ctx context.Context, jobs []func() error) error) {
	batchSubmitter = submit
}

// runBatches executes the MAP-stage jobs. When a shared-pool submitter is
// wired in, the jobs run concurrently under the single process-wide, vCPU-sized
// compiler-pool concurrency bound; otherwise they run sequentially. On any
// error the first non-nil error is returned after all jobs settle — the global
// pool is never StopWait'd, so an error here does not disrupt other stages.
func runBatches(ctx context.Context, jobs []func() error) error {
	if len(jobs) == 0 {
		return nil
	}
	if batchSubmitter != nil {
		return batchSubmitter(ctx, jobs)
	}
	for _, j := range jobs {
		if err := j(); err != nil {
			return err
		}
	}
	return nil
}

// structureBatchTokenBudget caps one extraction batch's packed chunk tokens.
// Python derives the budget from chat_mdl.max_length minus the prompt
// overhead; the Go ChatInvoker seam does not expose the model window, so we
// use the same conservative constant the wiki variant uses.
const structureBatchTokenBudget = 4096

// Run executes the structure variant:
//  1. MAP — per-batch two-stage (node → edge) extraction, parallel across
//     batches, results kept in batch order (mirrors _run_chunked_pipeline).
//  2. DEDUP — sequential LLM-judged merge in batch order, grouped by
//     relation endpoints, then a relation-rewrite pass for entity aliases
//     (mirrors _struct_local_dedup).
//  3. KIND POST-PROCESSING — chain validation for list/timeline (LLM
//     correction, fail-open) and the timeline orphan-entity filter (mirrors
//     validate_and_correct_chain + cleanup_timeline_isolated_entities).
//  4. GRAPH — one compact {"entities","relations"} summary row (mirrors
//     _struct_rebuild_graph_json).
//
// It never writes ES; the downstream writer persists the returned products.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	parserConfig, _ := inputs.VariantSpecific["parser_config"].(map[string]any)
	compileType := InferType(parserConfig)
	docID := common.FirstNonEmpty(inputs.DocID, deps.DatasetID, "unknown")
	llmID := common.FirstNonEmpty(param.LLMID, inputs.LLMID)
	cfg := CompileConfig{
		LLMID:        llmID,
		Type:         compileType,
		TenantID:     deps.TenantID,
		DocID:        docID,
		Variant:      common.VariantStructure,
		Lang:         param.Language,
		ParserConfig: parserConfig,
		TemplateID:   param.TemplateID,
	}

	nodePrompt, edgePromptTmpl := HypergraphPrompts(parserConfig, param.Language)

	// ---- MAP ----
	batches := common.PackBatches(inputs.Chunks, structureBatchTokenBudget, deps.Tokenizer)
	perBatch := make([][]common.Product, len(batches))
	jobs := make([]func() error, 0, len(batches))
	for i, batch := range batches {
		jobs = append(jobs, func() error {
			packed, batchIDs := PackBatch(batch)
			if len(batchIDs) == 0 {
				return nil
			}
			nodes, edges, err := extractHypergraph(ctx, deps, cfg, nodePrompt, edgePromptTmpl, packed)
			if err != nil {
				return err
			}
			rows, err := buildRows(ctx, deps, cfg, nodes, edges, batchIDs)
			if err != nil {
				return err
			}
			// Distinct slice index per batch → no cross-goroutine contention.
			perBatch[i] = rows
			return nil
		})
	}
	// The extraction batches are LLM-bounded, not CPU-bounded: run them on the
	// shared global compiler pool (vCPU-sized) when a submitter is wired in,
	// otherwise fall back to serial execution (historic default).
	if err := runBatches(ctx, jobs); err != nil {
		return common.Outputs{}, err
	}

	// ---- DEDUP ----
	// Sequential in batch order so merge outcomes are deterministic and match
	// Python's _struct_local_dedup (which folds docs in list order).
	decider := NewLLMMergeDecider(deps.Chat, llmID, deps.Embed, param.SimilarityThreshold)
	deduper := NewGroupedDeduper(decider)
	for _, rows := range perBatch {
		for _, row := range rows {
			if err := deduper.Add(ctx, row); err != nil {
				return common.Outputs{}, err
			}
		}
	}
	if err := deduper.RewriteRelations(ctx, decider.Aliases(), deps.Embed); err != nil {
		return common.Outputs{}, err
	}
	stats := deduper.Stats()
	prods := deduper.Rows()

	// ---- KIND POST-PROCESSING ----
	// Chain kinds (list/timeline): relations must form a strict linear chain;
	// offending relations the LLM does not keep are dropped (fail-open).
	// Timeline additionally drops entity rows no surviving relation references.
	// (Mirrors Python's validate_and_correct_chain — which runs right after
	// local dedup — and cleanup_timeline_isolated_entities.)
	if ChainKinds[compileType] {
		chunksByID := make(map[string]string, len(inputs.Chunks))
		for _, ch := range inputs.Chunks {
			if id := ch.ID; id != "" {
				chunksByID[id] = common.FirstNonEmpty(ch.Text, ch.Content)
			}
		}
		prods = validateAndCorrectChain(ctx, deps, llmID, prods, chunksByID, compileType)
	}
	if compileType == Type("timeline") {
		prods = dropIsolatedTimelineEntities(prods)
	}

	// Python stamps the inferred compile kind (list/set/hypergraph) as each
	// row's compile_kwd; the chunk converter picks it up from Meta.
	for i := range prods {
		prods[i].Meta["compile_kwd"] = string(compileType)
	}

	// ---- GRAPH ----
	graphProduct, err := buildGraphProduct(ctx, deps, cfg, prods)
	if err != nil {
		return common.Outputs{}, err
	}

	// Buffer every product (plus the graph) in one slice; the component merges
	// them into the upstream chunk stream (matching Python, which appends
	// compiled units onto the chunk list).
	products := append([]common.Product{}, prods...)
	products = append(products, graphProduct)

	out := common.Outputs{
		Products:          products,
		DuplicatesDropped: stats.DuplicatesDropped,
	}
	return out, nil
}

// buildGraphProduct rebuilds the compact graph JSON from the surviving
// entity/relation rows and wraps it as a single "graph" product so the
// downstream writer has a ready structure to persist. The row id mirrors
// Python's _struct_graph_row_id (doc : structure_graph : compile : template).
func buildGraphProduct(ctx context.Context, deps common.Deps, cfg CompileConfig, prods []common.Product) (common.Product, error) {
	if deps.Embed == nil {
		return common.Product{}, fmt.Errorf("knowledge_compiler: embedding model is required to build the graph product")
	}
	graph := RebuildStructureGraph(prods)
	graphContent := payloadJSON(graph)
	vecs, err := deps.Embed.Encode(ctx, []string{graphContent})
	if err != nil {
		return common.Product{}, err
	}
	if len(vecs) == 0 {
		return common.Product{}, fmt.Errorf("knowledge_compiler: embedding the graph summary returned no vector")
	}
	idParts := []string{cfg.DocID, "structure_graph", string(cfg.Type)}
	if cfg.TemplateID != "" {
		idParts = append(idParts, cfg.TemplateID)
	}
	return common.Product{
		ID:       common.StableRowID(idParts...),
		DocID:    cfg.DocID,
		TenantID: cfg.TenantID,
		Variant:  cfg.Variant,
		Content:  graphContent,
		Vector:   vecs[0],
		Meta: map[string]any{
			"kind":           "graph",
			"compile_kwd":    string(cfg.Type),
			"source_doc_ids": []string{cfg.DocID},
		},
	}, nil
}
