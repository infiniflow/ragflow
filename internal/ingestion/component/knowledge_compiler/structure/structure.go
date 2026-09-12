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
	"os"
	"strconv"
	"sync"

	"ragflow/internal/agent/runtime"
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

// structureInputBudget mirrors _build_chunk_batches' default mode:
// input_budget = max(int(max_length * INPUT_UTILIZATION) - prompt_overhead, 1024)
// with INPUT_UTILIZATION = 0.5 (rag/prompts/generator.py) and prompt_overhead
// the larger of the two stage prompts. A batch is one LLM call's whole input,
// so a budget that ignores the model window changes how many calls a document
// takes — and with it which entities land in which batch.
func structureInputBudget(modelContextLen, promptOverhead int) int {
	const (
		utilization = 0.5
		floor       = 1024
	)
	if modelContextLen <= 0 {
		return 0
	}
	budget := int(float64(modelContextLen)*utilization) - promptOverhead
	if budget < floor {
		budget = floor
	}
	return budget
}

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
	gateMode := EvidenceGateMode(parserConfig)

	// ---- MAP ----
	// Prompt overhead is counted the same way Python does: the larger of the
	// two stage prompts, subtracted from the window-derived input budget. The
	// tokenizer is optional (offline tests wire none) — without it the
	// overhead is 0 and PackBatches degrades to per-chunk counting.
	promptOverhead := 0
	if deps.Tokenizer != nil {
		promptOverhead = deps.Tokenizer.NumTokens(nodePrompt)
		if t := deps.Tokenizer.NumTokens(edgePromptTmpl); t > promptOverhead {
			promptOverhead = t
		}
	}
	budget := structureInputBudget(deps.ModelContextLen, promptOverhead)
	if budget <= 0 {
		// Model window unknown (the wiring did not set it): keep the historic
		// conservative constant rather than guessing a large window.
		budget = 4096
	}
	batches := common.PackBatches(inputs.Chunks, budget, deps.Tokenizer)
	// Python structure.py _STRUCT_MAX_CHUNKS_PER_BATCH: optional chunk-count cap
	// per extraction batch (0 = window-packed only — a heading has to see its
	// whole section to own it; the 4-per-batch rule belongs to tree's claim
	// harvesting). Overridable for benchmarking, mirrored verbatim.
	if v, err := strconv.Atoi(os.Getenv("STRUCT_MAX_CHUNKS_PER_BATCH")); err == nil && v > 0 {
		batches = capBatchChunkCount(batches, v)
	}
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
		"%s-template: %d chunk(s) -> %d batch(es)", compileType, len(inputs.Chunks), len(batches)))
	// Extraction and embedding are two phases (upstream): the pool workers only
	// extract; buildRows (which calls Embed.Encode) runs serially afterwards so
	// embedding batch jobs are never nested inside a compiler-pool worker.
	type extractedBatch struct {
		nodes, edges []map[string]any
		batchIDs     []string
	}
	extracted := make([]extractedBatch, len(batches))
	perBatch := make([][]common.Product, len(batches))
	jobs := make([]func() error, 0, len(batches))
	// The progress callback is supplied by the caller and is not required to be
	// goroutine-safe; pool workers report out of order, so serialise it.
	var progressMu sync.Mutex
	for i, batch := range batches {
		i, batch := i, batch
		jobs = append(jobs, func() error {
			runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
				"%s-template: extracting batch %d/%d", compileType, i+1, len(batches)))
			packed, batchIDs := PackBatch(batch)
			if len(batchIDs) == 0 {
				return nil
			}
			nodes, edges, err := extractHypergraph(ctx, deps, cfg, nodePrompt, edgePromptTmpl, packed)
			if err != nil {
				return err
			}
			// Evidence gate (mirrors Python _struct_process_batch): validate
			// quotes while the batch's source text is still in hand. It is
			// pure validation — no embedding — so it stays inside the worker,
			// and the vectors buildRows builds later are computed from the
			// surviving payload. Relations are gated only when the template
			// asked them to carry evidence.
			textByID := batchTextByID(batch)
			if len(textByID) > 0 {
				nodes, _, _ = ValidatePayloadEvidence(nodes, textByID, gateMode)
				if len(edges) > 0 && RelationExpectsEvidence(parserConfig) {
					edges, _, _ = ValidatePayloadEvidence(edges, textByID, gateMode)
				}
			}
			// Keep embedding out of the compiler-pool worker. buildRows calls
			// Embed.Encode, which may submit its own batch jobs to that pool;
			// the serial loop after runBatches owns it.
			extracted[i] = extractedBatch{nodes: nodes, edges: edges, batchIDs: batchIDs}
			progressMu.Lock()
			runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
				"%s-template: batch %d/%d done: %d entities, %d relations",
				compileType, i+1, len(batches), len(nodes), len(edges)))
			progressMu.Unlock()
			return nil
		})
	}
	// The extraction batches are LLM-bounded, not CPU-bounded: run them on the
	// shared global compiler pool (vCPU-sized) when a submitter is wired in,
	// otherwise fall back to serial execution (historic default).
	if err := runBatches(ctx, jobs); err != nil {
		return common.Outputs{}, err
	}
	// Embed each extracted batch serially after all MAP jobs have returned.
	// This avoids nesting Embed.Encode (and its batch jobs) inside a worker
	// already occupied by the shared compiler pool.
	rowCount := 0
	for i, result := range extracted {
		if len(result.batchIDs) == 0 {
			continue
		}
		rows, err := buildRows(ctx, deps, cfg, result.nodes, result.edges, result.batchIDs)
		if err != nil {
			return common.Outputs{}, err
		}
		perBatch[i] = rows
		rowCount += len(rows)
		runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
			"%s-template: embedded batch %d/%d (%d rows so far)", compileType, i+1, len(batches), rowCount))
	}
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
		"%s-template: deduplicating %d row(s)", compileType, rowCount))

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
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
		"%s-template: dedup done: %d row(s), %d duplicate(s) dropped",
		compileType, len(prods), stats.DuplicatesDropped))

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

	// The deduplicated entity/relation products are the whole output; the
	// component merges them into the upstream chunk stream. (The compact graph
	// blob was removed: knowledge_graph_kwd="graph" is no longer a storage row,
	// which also saves one embedding call per compile.)
	products := append([]common.Product{}, prods...)
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
		"%s-template: produced %d row(s)", compileType, len(products)))

	out := common.Outputs{
		Products:          products,
		DuplicatesDropped: stats.DuplicatesDropped,
	}
	return out, nil
}

// capBatchChunkCount splits window-packed batches into sub-batches of at most
// cap chunks (Python batch_size_cap greedy mode, chunk-count cutoff). Order is
// preserved; PackBatch labels are per-batch positional so sub-batches renumber
// from C1 exactly like freshly packed batches.
func capBatchChunkCount(batches [][]common.Chunk, cap int) [][]common.Chunk {
	if cap < 1 {
		return batches
	}
	var out [][]common.Chunk
	for _, b := range batches {
		for start := 0; start < len(b); start += cap {
			end := start + cap
			if end > len(b) {
				end = len(b)
			}
			out = append(out, b[start:end])
		}
	}
	return out
}
