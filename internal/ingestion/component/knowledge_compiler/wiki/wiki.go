// Package wiki implements the "wiki" variant of KnowledgeCompiler: a
// document artifact pipeline MAP -> REDUCE -> PLAN -> REFINE that produces a
// wiki-style page (and supporting section products). MAP results may be reused
// from the immutable per-chunk version store; later stages remain in memory.
// The stage semantics are aligned with the Python wiki.py design, but the Go
// port keeps all intermediate artifacts in memory instead of persisting them to
// ES between stages.
package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/agent/runtime"
	appcommon "ragflow/internal/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"

	"go.uber.org/zap"
)

// batchSubmitter fans out the MAP-stage extraction jobs on the process-wide
// knowledge-compilation pool. It is injected by the knowledge_compiler wiring
// so every variant shares one vCPU-sized concurrency bound; when nil the
// batches run sequentially (the historic default).
var batchSubmitter func(ctx context.Context, jobs []func() error) error

// SetBatchSubmitter installs the shared-pool fan-out used by Run's MAP stage.
// Pass nil to revert to serial execution.
func SetBatchSubmitter(submit func(ctx context.Context, jobs []func() error) error) {
	batchSubmitter = submit
}

// runBatches mirrors the other compiler variants: concurrent under the wired
// global compiler pool, or serial when no submitter is set. The first error is
// returned after all jobs settle; the global pool is never StopWait'd.
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

// wikiMapTokenBudget is the input-token budget per MAP extraction batch. It is
// intentionally well below the chat model's context window so the LLM has
// generous room to emit the entity/concept/claim/relation/topic JSON without
// hitting the output-token limit and truncating the payload.
const wikiMapTokenBudget = 2048

const wikiRefineProgressStep = 5

// wikiMapMaxTokens derives the extraction output budget from the model's
// context length and the per-batch input budget: once the batch has consumed
// wikiMapTokenBudget input tokens, the rest of the window is handed to the
// output — but never below the input budget itself, so a small-input batch can
// still get a proportionally large extraction payload. modelContextLen is the
// model's total context window in tokens (0 means unknown).
func wikiMapMaxTokens(modelContextLen int) int {
	if modelContextLen <= 0 {
		modelContextLen = common.DefaultLLMContextLength
	}
	return max(modelContextLen-wikiMapTokenBudget, wikiMapTokenBudget)
}

// wikiRefineMaxTokens gives the page writer (REFINE step) a generous but
// bounded output cap so a long page body is not cut off mid-stream by a small
// default completion limit. It reuses the map/extraction budget derivation:
// once the input budget is consumed, the rest of the model window is handed to
// the output. modelContextLen is the model's total context window in tokens (0
// means unknown).
func wikiRefineMaxTokens(modelContextLen int) int {
	return wikiMapMaxTokens(modelContextLen)
}

type wikiPipeline struct {
	ctx       context.Context
	deps      common.Deps
	param     common.Param
	inputs    common.Inputs
	docID     string
	tenantID  string
	datasetID string
	llmID     string

	mapExtracts []wikiExtract
	reduced     wikiExtract
	plan        wikiPlan
	pages       []wikiPageResult
	// planBudget is the resolved global page budget for the current planning
	// run (target approx + hard cap).
	planBudget wikiPlanBudget
	// planCapacityExcluded counts the planned pages dropped to fit the global
	// hard cap; testable and reported for observability.
	planCapacityExcluded int
	neighborCache        map[string]*common.WikiPageCandidate
	incremental          bool
	activeStateKey       string
	previousActiveState  wikiMapActiveSnapshot
	nextActiveState      wikiMapActiveSnapshot
	affectedPageSlugs    map[string]struct{}
	removedPageSlugs     []string
	mapChanged           bool
	affectedTerms        map[string]struct{}
	pendingActiveState   *common.WikiMapActiveState
}

type wikiExtract struct {
	Entities  []wikiEntity   `json:"entities"`
	Concepts  []wikiConcept  `json:"concepts"`
	Claims    []wikiClaim    `json:"claims"`
	Relations []wikiRelation `json:"relations"`
	Topics    []wikiTopic    `json:"topics"`
	Mode      string         `json:"mode,omitempty"`
}

type wikiMapActiveSnapshot struct {
	Chunks map[string]wikiMapActiveChunk `json:"chunks"`
	Plan   []wikiPlanPage                `json:"plan"`
}

type wikiMapActiveChunk struct {
	Key     string      `json:"key"`
	Extract wikiExtract `json:"extract"`
}

type wikiEntity struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Aliases        []string `json:"aliases,omitempty"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

type wikiConcept struct {
	Term           string   `json:"term"`
	Definition     string   `json:"definition_excerpt"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

type wikiClaim struct {
	Statement      string   `json:"statement"`
	Subject        string   `json:"subject"`
	Confidence     string   `json:"confidence"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

type wikiRelation struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Type           string   `json:"type"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

type wikiTopic struct {
	Path           string   `json:"path"`
	Description    string   `json:"description,omitempty"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

type wikiPlan struct {
	Title    string            `json:"title"`
	Slug     string            `json:"slug"`
	Lead     string            `json:"lead"`
	Sections []wikiPlanSection `json:"sections"`
	PageType string            `json:"page_type,omitempty"`
	Topic    string            `json:"topic,omitempty"`
	Entities []string          `json:"entity_names,omitempty"`
	Related  []string          `json:"related_kb_pages,omitempty"`
	Pages    []wikiPlanPage    `json:"pages,omitempty"`
}

type wikiPlanSection struct {
	Heading string   `json:"heading"`
	Points  []string `json:"points"`
}

type wikiPlanPage struct {
	Action      string            `json:"action"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	PageType    string            `json:"page_type"`
	Topic       string            `json:"topic"`
	EntityNames []string          `json:"entity_names"`
	RelatedKB   []string          `json:"related_kb_pages"`
	Priority    int               `json:"priority"`
	Lead        string            `json:"lead"`
	Sections    []wikiPlanSection `json:"sections"`
	// MentionCount is an internal (non-serialized) signal used for the
	// deterministic page selection when the merged plan exceeds the global hard
	// cap. It is computed from the reduced extract, not read from JSON.
	MentionCount int    `json:"-"`
	PlanGroup    string `json:"plan_group,omitempty"`
}

// Reconciliation thresholds. These are a deliberate Go-specific refinement of
// the Python wiki.py contract, NOT a byte-for-byte alignment:
//
// Python `_wiki_reconcile_with_kb` (wiki.py:1900-1957) queries KNN with
// `extra_options={"similarity": update_threshold}` (update_threshold=0.95), so
// candidates below that threshold are normally filtered out at retrieval and
// the item becomes CREATE directly. Its MAYBE band ([maybe=0.60, update=0.95))
// is only reachable when a backend still returns low-score candidates despite
// the similarity filter.
//
// Go's `FindSimilarPages` returns top-K candidates without a similarity floor,
// so it genuinely sees low-score candidates Python never does. To exploit that
// richer signal we keep a real two-band decision:
//
//   - Score >= update_threshold (0.92)           -> direct UPDATE
//   - Score <  maybe_threshold (0.78)            -> CREATE (no match)
//   - Score in [maybe, update)                   -> title/topic/entity overlap
//     heuristic first (direct UPDATE), else MAYBE resolved by the LLM.
//
// The title/topic/entity overlap straight-to-UPDATE shortcut is a Go-only
// enhancement; it is NOT a Python-aligned behavior. Keep it documented as such.
const (
	wikiPlanUpdateThreshold = 0.92
	wikiPlanMaybeThreshold  = 0.78
	wikiPlanSearchTopK      = 3
	wikiMergeShrinkRatio    = 0.7
)

// Run executes the wiki variant.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	docID := inputs.DocID
	llmID := firstNonEmpty(inputs.LLMID, param.LLMID)
	if docID == "" {
		docID = "unknown"
	}
	appcommon.Info("wiki: Run start",
		zap.String("dataset_id", deps.DatasetID),
		zap.String("doc_id", docID),
		zap.Int("chunks", len(inputs.Chunks)),
		zap.Bool("chat_ready", deps.Chat != nil),
		zap.Bool("embed_ready", deps.Embed != nil))
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf("Wiki Started: input_chunks=%d", len(inputs.Chunks)))
	p := &wikiPipeline{
		ctx:               ctx,
		deps:              deps,
		param:             param,
		inputs:            inputs,
		docID:             docID,
		tenantID:          deps.TenantID,
		datasetID:         deps.DatasetID,
		llmID:             llmID,
		neighborCache:     make(map[string]*common.WikiPageCandidate),
		affectedPageSlugs: make(map[string]struct{}),
	}
	if incremental, ok := inputs.VariantSpecific["wiki_incremental"].(bool); ok {
		p.incremental = incremental
	}
	if err := p.run(); err != nil {
		appcommon.Error("wiki: Run pipeline failed", err,
			zap.String("dataset_id", deps.DatasetID),
			zap.String("doc_id", docID))
		return common.Outputs{}, err
	}
	appcommon.Info("wiki: pipeline done",
		zap.String("dataset_id", deps.DatasetID),
		zap.String("doc_id", docID),
		zap.Int("extracted_entities", len(p.reduced.Entities)),
		zap.Int("plan_pages", len(p.plan.Pages)),
		zap.Int("refined_pages", len(p.pages)))

	products := buildWikiPageProducts(p.tenantID, p.docID, p.pages)
	pageProducts, sectionProducts := countWikiProducts(products)
	runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf("Wiki Done: input_chunks=%d output_products=%d wiki_pages=%d wiki_sections=%d", len(inputs.Chunks), len(products), pageProducts, sectionProducts))
	appcommon.Info("wiki: built page products",
		zap.String("dataset_id", deps.DatasetID),
		zap.String("doc_id", docID),
		zap.Int("products", len(products)))
	for i := range products {
		if products[i].Meta == nil {
			products[i].Meta = map[string]any{}
		}
	}
	if len(products) > 0 {
		if deps.Chat == nil {
			return common.Outputs{}, fmt.Errorf("wiki: chat model required for page generation")
		}
		if deps.Embed == nil {
			return common.Outputs{}, fmt.Errorf("wiki: embedder required (page + sections must carry vectors)")
		}
		texts := make([]string, len(products))
		for i, p := range products {
			texts[i] = p.Content
		}
		vectors, err := deps.Embed.Encode(ctx, texts)
		if err != nil {
			return common.Outputs{}, err
		}
		for i := range products {
			if i < len(vectors) {
				products[i].Vector = vectors[i]
			}
		}
	}

	store := common.NewMemStore()
	decider := structure.CosineDecider{Threshold: param.SimilarityThreshold}
	stats := structure.MergeStats{}
	for _, p := range products {
		s, err := structure.MergeIntoStore(ctx, store, decider, []common.Product{p})
		if err != nil {
			return common.Outputs{}, err
		}
		stats.Inserted += s.Inserted
		stats.Updated += s.Updated
		stats.DuplicatesDropped += s.DuplicatesDropped
	}

	prod := store.Snapshot()
	if param.EnableHistoricalDedup || len(inputs.HistoricalCandidates) > 0 {
		survivors, dropped, err := dedupHistorical(ctx, deps, param, p.tenantID, p.datasetID, inputs.HistoricalCandidates, prod)
		if err != nil {
			return common.Outputs{}, err
		}
		prod = survivors
		stats.DuplicatesDropped += dropped
	}

	// Buffer every product in one slice; the component merges them into the
	// upstream chunk stream (matching Python, which appends compiled units onto
	// the chunk list).
	out := common.Outputs{
		Products:          prod,
		DuplicatesDropped: stats.DuplicatesDropped,
		AffectedPageSlugs: sortedStringSet(p.affectedPageSlugs),
		RemovedPageSlugs:  append([]string(nil), p.removedPageSlugs...),
	}
	if p.pendingActiveState != nil {
		out.WikiActiveStates = append(out.WikiActiveStates, *p.pendingActiveState)
	}
	return out, nil
}

// runKey returns a stable identity for this wiki run's log lines. In a
// per-document run docID is populated; in a dataset-level run (the
// knowledge_compile aggregator) docID is empty, so fall back to a dataset
// scope prefix. This avoids masking which dataset a MAP/REDUCE pass ran for
// behind an uninformative "unknown".
func (p *wikiPipeline) runKey() string {
	if p.docID != "" {
		return p.docID
	}
	if p.datasetID != "" {
		return "dataset:" + p.datasetID
	}
	return "unknown"
}

func (p *wikiPipeline) run() error {
	if len(p.inputs.Chunks) == 0 {
		runtime.ReportProgressMessage(p.ctx, "Compiler", "Wiki Empty: input_chunks=0 output_products=0")
		return nil
	}
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki MAP Started: input_chunks=%d", len(p.inputs.Chunks)))
	if err := p.runMap(); err != nil {
		return err
	}
	appcommon.Info("wiki: MAP done",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("batches", len(p.mapExtracts)),
		zap.Int("raw_entities", countExtractEntities(p.mapExtracts)))
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki MAP Done: input_chunks=%d output_batches=%d entities=%d concepts=%d claims=%d relations=%d topics=%d", len(p.inputs.Chunks), len(p.mapExtracts), countExtractEntities(p.mapExtracts), countExtractConcepts(p.mapExtracts), countExtractClaims(p.mapExtracts), countExtractRelations(p.mapExtracts), countExtractTopics(p.mapExtracts)))
	appcommon.Debug("wiki: MAP intermediate",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("batches", len(p.mapExtracts)),
		zap.Strings("entities", extractEntitiesDebug(p.mapExtracts)),
		zap.Strings("concepts", extractConceptDebug(p.mapExtracts)),
		zap.Strings("claims", extractClaimDebug(p.mapExtracts)),
		zap.Strings("relations", extractRelationDebug(p.mapExtracts)))
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki REDUCE Started: input_entities=%d input_concepts=%d input_claims=%d input_relations=%d", countExtractEntities(p.mapExtracts), countExtractConcepts(p.mapExtracts), countExtractClaims(p.mapExtracts), countExtractRelations(p.mapExtracts)))
	reduced := reduceExtracts(p.mapExtracts)
	p.reduced = reduced
	appcommon.Debug("wiki: REDUCE exact done",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Strings("entities", entitiesDebug(reduced.Entities)),
		zap.Strings("concepts", conceptsDebug(reduced.Concepts)),
		zap.Int("claims", len(reduced.Claims)),
		zap.Strings("relations", relationsDebug(reduced.Relations)))
	appcommon.Info("wiki: REDUCE done",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("entities", len(p.reduced.Entities)),
		zap.Int("claims", len(p.reduced.Claims)))
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki REDUCE Done: output_entities=%d output_concepts=%d output_claims=%d output_relations=%d", len(p.reduced.Entities), len(p.reduced.Concepts), len(p.reduced.Claims), len(p.reduced.Relations)))
	// PLAN uses the same topic-assignment path in both modes. Entity mode keeps
	// one identity per page, while topic mode may merge related identities in
	// the planner before assigning the resulting pages to MAP topics.
	var plan wikiPlan
	var err error
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki PLAN Started: input_entities=%d input_concepts=%d input_relations=%d", len(p.reduced.Entities), len(p.reduced.Concepts), len(p.reduced.Relations)))
	plan, err = p.runPlan()
	if err != nil {
		return err
	}
	appcommon.Info("wiki: PLAN done", zap.String("dataset_id", p.datasetID), zap.String("doc_id", p.runKey()), zap.String("mode", p.wikiMode()), zap.Int("plan_pages", len(plan.Pages)))
	p.plan = plan
	p.selectAffectedPages(plan.Pages)
	appcommon.Info("wiki: PLAN done",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("plan_pages", len(plan.Pages)),
		zap.Int("capacity_excluded", p.planCapacityExcluded))
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki PLAN Done: output_pages=%d topics=%d", len(plan.Pages), countPlanTopics(plan.Pages)))
	appcommon.Debug("wiki: PLAN intermediate",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.String("plan_title", plan.Title),
		zap.String("plan_slug", plan.Slug),
		zap.Int("plan_pages", len(plan.Pages)),
		zap.Int("capacity_excluded", p.planCapacityExcluded),
		zap.Strings("pages", planPagesDebug(plan.Pages)))
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki REFINE Started: input_pages=%d", len(p.plan.Pages)))
	pages, err := p.runRefine()
	if err != nil {
		return err
	}
	p.pages = pages
	activeState, err := p.buildActiveMapState(plan.Pages)
	if err != nil {
		return err
	}
	if activeState != nil {
		p.pendingActiveState = activeState
	}
	runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki REFINE Done: input_pages=%d output_pages=%d", len(p.plan.Pages), len(pages)))
	appcommon.Info("wiki: REFINE done",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("refined_pages", len(pages)))
	appcommon.Debug("wiki: REFINE intermediate",
		zap.String("dataset_id", p.datasetID),
		zap.String("doc_id", p.runKey()),
		zap.Int("refined_pages", len(pages)),
		zap.Strings("pages", pageResultsDebug(pages)))
	return nil
}

// countExtractEntities sums the entity count across a list of map extracts.
func countExtractEntities(extracts []wikiExtract) int {
	n := 0
	for _, e := range extracts {
		n += len(e.Entities)
	}
	return n
}

func countExtractConcepts(extracts []wikiExtract) int {
	n := 0
	for _, e := range extracts {
		n += len(e.Concepts)
	}
	return n
}

func countExtractClaims(extracts []wikiExtract) int {
	n := 0
	for _, e := range extracts {
		n += len(e.Claims)
	}
	return n
}

func countExtractRelations(extracts []wikiExtract) int {
	n := 0
	for _, e := range extracts {
		n += len(e.Relations)
	}
	return n
}

func countExtractTopics(extracts []wikiExtract) int {
	n := 0
	for _, e := range extracts {
		n += len(e.Topics)
	}
	return n
}

func countPlanTopics(pages []wikiPlanPage) int {
	topics := make(map[string]struct{})
	for _, page := range pages {
		topic := common.NormalizeWikiTopicPath(page.Topic)
		if topic != "" {
			topics[wikiTopicKey(topic)] = struct{}{}
		}
	}
	return len(topics)
}

func countWikiProducts(products []common.Product) (pages, sections int) {
	for _, product := range products {
		kind, _ := product.Meta["kind"].(string)
		switch kind {
		case "page":
			pages++
		case "section":
			sections++
		}
	}
	return pages, sections
}

// --- Debug helpers: compact per-item descriptors for the stage-intermediate
// logs in run(). They deliberately record only identity-level fields (name /
// slug / type / aliases / endpoints), never full content or embedding vectors,
// so a batch with many entities stays readable and does not blow the log line.

func entityDebug(e wikiEntity) string {
	if len(e.Aliases) == 0 {
		return fmt.Sprintf("%s(%s)", e.Name, e.Type)
	}
	return fmt.Sprintf("%s(%s){aliases:%s}", e.Name, e.Type, strings.Join(e.Aliases, ","))
}

func conceptDebug(c wikiConcept) string {
	return c.Term
}

func claimDebug(c wikiClaim) string {
	return c.Subject + ": " + c.Statement
}

func relationDebug(r wikiRelation) string {
	return fmt.Sprintf("%s-[%s]->%s", r.From, r.Type, r.To)
}

func extractEntitiesDebug(extracts []wikiExtract) []string {
	var out []string
	for bi, ex := range extracts {
		for _, e := range ex.Entities {
			out = append(out, fmt.Sprintf("batch%d:%s", bi, entityDebug(e)))
		}
	}
	return out
}

func extractConceptDebug(extracts []wikiExtract) []string {
	var out []string
	for bi, ex := range extracts {
		for _, c := range ex.Concepts {
			out = append(out, fmt.Sprintf("batch%d:%s", bi, conceptDebug(c)))
		}
	}
	return out
}

func extractClaimDebug(extracts []wikiExtract) []string {
	var out []string
	for bi, ex := range extracts {
		for _, c := range ex.Claims {
			out = append(out, fmt.Sprintf("batch%d:%s", bi, claimDebug(c)))
		}
	}
	return out
}

func extractRelationDebug(extracts []wikiExtract) []string {
	var out []string
	for bi, ex := range extracts {
		for _, r := range ex.Relations {
			out = append(out, fmt.Sprintf("batch%d:%s", bi, relationDebug(r)))
		}
	}
	return out
}

func entitiesDebug(es []wikiEntity) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, entityDebug(e))
	}
	return out
}

func entityNamesDebug(es []wikiEntity) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func conceptsDebug(cs []wikiConcept) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, conceptDebug(c))
	}
	return out
}

func relationsDebug(rs []wikiRelation) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, relationDebug(r))
	}
	return out
}

func planPagesDebug(pages []wikiPlanPage) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, fmt.Sprintf("%s:%s(%s) prio=%d related=%s",
			p.Slug, p.Title, p.PageType, p.Priority, strings.Join(p.RelatedKB, ",")))
	}
	return out
}

func pageResultsDebug(pages []wikiPageResult) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, fmt.Sprintf("%s:%s(%s) outlinks=%s",
			p.Slug, p.Title, p.PageType, strings.Join(p.Outlinks, ",")))
	}
	return out
}

func (p *wikiPipeline) runMap() error {
	// The versioned MAP cache is dataset-scoped: its DocStore rows live under
	// ragflow_<tenant_id>/<kb_id>. A canvas debug (dataflow dry-run) run has no
	// knowledgebase (NewDebugTaskContext forces kb_id == ""), so there is no
	// scope to key cache rows on — the strict-scope store would fail the whole
	// run with "tenant_id and dataset_id are required". Run the cache-less MAP
	// there instead, mirroring how the debug tokenizer skips embedding and the
	// debug executor skips persistence: a dry-run must stay side-effect free.
	if p.deps.WikiMapVersions != nil && strings.TrimSpace(p.tenantID) != "" && strings.TrimSpace(p.datasetID) != "" {
		err := p.runVersionedMap()
		for i := range p.mapExtracts {
			p.mapExtracts[i].Mode = p.wikiMode()
		}
		return err
	}
	// Keep each batch small enough that the LLM's entity/relation JSON output
	// for the batch stays well under the model's output-token limit. 2048 input
	// tokens per batch (was a hard-coded 4096) leaves generous headroom for the
	// extraction payload; oversize batches caused truncated, unparseable JSON
	// (unexpected end of JSON input) on the real pipeline.
	batches := common.PackBatches(p.inputs.Chunks, wikiMapTokenBudget, p.deps.Tokenizer)
	extracts, err := runMapBatches(p.ctx, batches, p.mapBatch)
	if err != nil {
		return err
	}
	p.mapExtracts = append(p.mapExtracts, extracts...)
	for i := range p.mapExtracts {
		p.mapExtracts[i].Mode = p.wikiMode()
	}
	return nil
}

func (p *wikiPipeline) wikiMode() string {
	if p.param.PlanEnabled() {
		return "topic"
	}
	return "entity"
}

func runMapBatches(
	ctx context.Context,
	batches [][]common.Chunk,
	mapBatch func([]common.Chunk) (wikiExtract, error),
) ([]wikiExtract, error) {
	if len(batches) == 0 {
		return nil, nil
	}
	extracts := make([]wikiExtract, len(batches))
	jobs := make([]func() error, 0, len(batches))
	for i, batch := range batches {
		i, batch := i, batch
		jobs = append(jobs, func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			extract, err := mapBatch(batch)
			if err != nil {
				return err
			}
			// Distinct slice index per batch keeps results stable without locks.
			extracts[i] = extract
			return nil
		})
	}
	if err := runBatches(ctx, jobs); err != nil {
		return nil, err
	}
	return extracts, nil
}

func (p *wikiPipeline) mapBatch(batch []common.Chunk) (wikiExtract, error) {
	parserConfig, _ := p.inputs.VariantSpecific["parser_config"].(map[string]any)
	user, _ := buildWikiMapPrompt(p.docID, batch, parserConfig, p.param.Language)
	// Give the extraction step a generous output budget so the entity/relation
	// JSON is not silently truncated by the model's default output cap (that
	// produced "unexpected end of JSON input" from GenJSON). The output budget is
	// tied to the per-batch input budget: once the batch consumes
	// wikiMapTokenBudget tokens of the model's context, the remainder is left
	// for the extraction payload (and never less than the input budget itself).
	mt := wikiMapMaxTokens(p.deps.ModelContextLen)
	raw, err := common.GenJSON(p.ctx, p.deps.Chat, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiMapSystem,
		UserPrompt:   user,
		MaxTokens:    &mt,
	})
	if err != nil {
		return wikiExtract{}, err
	}
	extract := parseWikiExtract(raw)
	extract.Mode = p.wikiMode()
	return extract, nil
}

func (p *wikiPipeline) runPlan() (wikiPlan, error) {
	if p.wikiMode() == "topic" && p.deps.Embed != nil {
		return p.runTopicPlan()
	}
	return p.runLegacyPlan()
}

func (p *wikiPipeline) runLegacyPlan() (wikiPlan, error) {
	batches := packWikiPlanBatches(p.reduced, wikiPlanTokenBudget)
	if len(batches) == 0 {
		batches = []wikiExtract{p.reduced}
	}
	totalItems := 0
	for _, b := range batches {
		totalItems += wikiExtractItemCount(b)
	}
	p.planBudget = deriveWikiPlanBudget(p.deps.ModelContextLen, totalItems)
	// Quota allocation must use the achievable cap (min(Target, Max)): when the
	// model's output capacity is smaller than the item-count-derived target, the
	// planner must be asked for at most Max pages so the sum of per-batch
	// max_pages never exceeds the capacity that can actually be emitted. Using
	// Target here would re-introduce the truncated-JSON risk the budget exists
	// to eliminate.
	quotas := allocatePlanQuotas(batches, p.planBudget.Cap())

	// approvedReduced is the set of items that actually got a non-zero quota.
	// It is what the fallback/normalization may reference so zero-quota items
	// can never leak back into the plan via buildWikiFallbackPages.
	approved := wikiExtract{}
	plans := make([]wikiPlan, len(batches))
	jobs := make([]func() error, 0, len(batches))
	for i, batch := range batches {
		i, batch := i, batch
		quota := quotas[i]
		if quota <= 0 {
			// Zero-quota batch: no planner call and no fallback page. It is
			// intentionally left as the zero wikiPlan{} so the merge sees no
			// pages from it.
			continue
		}
		approved.Entities = append(approved.Entities, batch.Entities...)
		approved.Concepts = append(approved.Concepts, batch.Concepts...)
		approved.Claims = append(approved.Claims, batch.Claims...)
		approved.Relations = append(approved.Relations, batch.Relations...)
		approved.Topics = append(approved.Topics, batch.Topics...)
		jobs = append(jobs, func() error {
			if err := p.ctx.Err(); err != nil {
				return err
			}
			plan, err := p.runPlanBatch(batch, i+1, len(batches), quota, p.planBudget.MaxTokens)
			if err != nil {
				return err
			}
			// Distinct slice index keeps results stable without locks.
			plans[i] = plan
			return nil
		})
	}
	if err := runBatches(p.ctx, jobs); err != nil {
		return wikiPlan{}, err
	}

	merged := p.mergePlanCandidates(plans, approved)
	var excluded int
	merged.Pages, excluded = truncatePlanPagesByCap(merged.Pages, p.planBudget.Max, approved)
	p.planCapacityExcluded += excluded
	merged.Pages = normalizeWikiPlanPageLinks(merged.Pages)
	reconciled, err := p.reconcilePlan(merged)
	if err != nil {
		return wikiPlan{}, err
	}
	// reconcilePlan rewrites page.Slug to the matched existing page's slug, which
	// can re-introduce self-referential or plan-absent related links; normalize
	// once more so stale links don't reach the stored pages.
	reconciled.Pages = normalizeWikiPlanPageLinks(reconciled.Pages)
	return reconciled, nil
}

// runRefine fans the REFINE stage out to page-level jobs on the shared compiler
// pool, mirroring the P1 error model: all jobs are submitted, awaited, and the
// first error is returned; each job checks ctx before starting. Results are
// written to pre-allocated per-index slots so the output is in normalized-plan
// order regardless of completion order (no concurrent append to a shared slice).
// When no submitter is wired, jobs run serially (historic default).
func (p *wikiPipeline) runRefine() ([]wikiPageResult, error) {
	allPages := normalizeWikiPlanPages(p.plan.Pages, p.reduced)
	if len(allPages) == 0 {
		return nil, nil
	}
	pages := allPages
	if p.incremental {
		pages = pages[:0:0]
		for _, page := range allPages {
			if _, affected := p.affectedPageSlugs[page.Slug]; affected {
				pages = append(pages, page)
			}
		}
	}
	pageTitles := map[string]string{}
	slugToPageType := map[string]string{}
	allPlanSlugs := make([]string, 0, len(allPages))
	for _, page := range allPages {
		if page.Slug == "" {
			continue
		}
		allPlanSlugs = append(allPlanSlugs, page.Slug)
		pageTitles[page.Slug] = page.Title
		// Every planned page carries its type (entity/concept/...); the link
		// renderer needs it so artifact/<kb>/<page_type>/<slug> links are
		// clickable (frontend parseWikiLinkHref only matches entity|concept).
		if pt := strings.TrimSpace(page.PageType); pt != "" {
			slugToPageType[page.Slug] = pt
		}
	}
	entityLookup := buildWikiEntityLookup(p.reduced.Entities)
	conceptLookup := buildWikiConceptLookup(p.reduced.Concepts)

	results := make([]wikiPageResult, len(pages))
	jobs := make([]func() error, 0, len(pages))
	var progressMu sync.Mutex
	completed := 0
	for i, planItem := range pages {
		i, planItem := i, planItem
		jobs = append(jobs, func() error {
			if err := p.ctx.Err(); err != nil {
				return err
			}
			res, err := p.runRefinePage(planItem, allPlanSlugs, pageTitles, slugToPageType, entityLookup, conceptLookup)
			if err != nil {
				return err
			}
			results[i] = res
			progressMu.Lock()
			completed++
			if shouldReportRefineProgress(completed, len(pages)) {
				runtime.ReportProgressMessage(p.ctx, "Compiler", fmt.Sprintf("Wiki REFINE Progress: completed_pages=%d total_pages=%d", completed, len(pages)))
			}
			progressMu.Unlock()
			return nil
		})
	}
	if err := runBatches(p.ctx, jobs); err != nil {
		return nil, err
	}
	return results, nil
}

func shouldReportRefineProgress(completed, total int) bool {
	return completed > 0 && total > 0 && (completed%wikiRefineProgressStep == 0 || completed == total)
}

// runRefinePage generates one page result from a normalized plan page. UPDATE
// merge, evidence assembly, and source-context building are unchanged; this is
// the per-page unit that runRefine fans out.
func (p *wikiPipeline) runRefinePage(
	planItem wikiPlanPage,
	allPlanSlugs []string,
	pageTitles map[string]string,
	slugToPageType map[string]string,
	entityLookup map[string]wikiExtractItem,
	conceptLookup map[string]wikiExtractItem,
) (wikiPageResult, error) {
	evidence := assembleWikiPageEvidence(planItem, p.reduced.Claims, entityLookup, conceptLookup)
	sourceChunkIDs := collectWikiEvidenceChunkIDs(evidence)
	sourceContext := buildSourceContext(p.inputs.Chunks, sourceChunkIDs)
	if strings.TrimSpace(sourceContext) == "" {
		sourceContext = buildSourceContext(p.inputs.Chunks, p.reduced.sourceChunkIDs())
	}
	available := make([]string, 0, len(allPlanSlugs))
	for _, slug := range allPlanSlugs {
		if slug != planItem.Slug {
			available = append(available, "- [["+slug+"]]")
		}
	}
	if len(available) == 0 {
		available = []string{"(none — this is the only page)"}
	}
	var existing *common.WikiPageCandidate
	var err error
	if strings.EqualFold(planItem.Action, "UPDATE") && p.deps.WikiPages != nil {
		existing, err = p.deps.WikiPages.GetPageBySlug(p.ctx, p.tenantID, p.datasetID, planItem.Slug)
		if err != nil {
			return wikiPageResult{}, err
		}
	}
	existingSection := ""
	existingRaw := ""
	if existing != nil {
		existingRaw = firstNonEmpty(existing.ContentMDRaw, existing.ContentMD)
		if strings.TrimSpace(existingRaw) != "" {
			existingSection = "## Existing page content (UPDATE — integrate new evidence into this)\n\n" + existingRaw + "\n"
		}
	}
	user := renderWikiTemplate(wikiRefineWriterUserTemplate, map[string]string{
		"action":           firstNonEmpty(planItem.Action, "CREATE"),
		"slug":             planItem.Slug,
		"title":            firstNonEmpty(planItem.Title, planItem.Slug),
		"page_type":        firstNonEmpty(planItem.PageType, "concept"),
		"all_plan_slugs":   strings.Join(available, "\n"),
		"related_kb_pages": formatWikiRelatedKB(planItem.RelatedKB, pageTitles),
		"existing_section": existingSection,
		"source_context":   sourceContext,
		"evidence_count":   fmt.Sprintf("%d", len(evidence)),
		"evidence_blocks":  formatWikiEvidenceBlocks(evidence),
	})
	// wikiRefineMaxTokens gives the page writer a generous but bounded output
	// cap so a long page is not cut off mid-stream by a small default completion
	// limit (which would yield an unusable/cut page body).
	rmt := wikiRefineMaxTokens(p.deps.ModelContextLen)
	resp, err := p.deps.Chat.Chat(p.ctx, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: buildWikiRefineWriterSystem(""),
		UserPrompt:   user,
		MaxTokens:    &rmt,
	})
	if err != nil {
		return wikiPageResult{}, err
	}
	if resp == nil {
		return wikiPageResult{}, fmt.Errorf("knowledge_compiler: wiki refine returned no response")
	}
	contentRaw := strings.TrimSpace(firstNonEmpty(resp.Content))
	if contentRaw == "" {
		contentRaw = "# " + firstNonEmpty(planItem.Title, planItem.Slug) + "\n\n(Page generation produced no content.)"
	}
	if strings.TrimSpace(existingRaw) != "" {
		contentRaw, err = p.mergeWikiPageContent(existingRaw, contentRaw, planItem.Slug)
		if err != nil {
			return wikiPageResult{}, err
		}
	}
	contentRendered, outlinks := transformWikiLinks(contentRaw, firstNonEmpty(p.datasetID, p.docID), pageTitles, slugToPageType)
	// wiki_incremental port (O2): deterministically guarantee a "See also"
	// cross-link section from RelatedKB. transformWikiLinks/resolveSlug drop
	// links whose target cannot be resolved to a known slug, so an LLM-authored
	// page could silently lose every RelatedKB edge. We append the related pages
	// AFTER transformWikiLinks and do NOT re-run the resolver, so dead links are
	// bypassed and the missing target does not drop the edge — each RelatedKB
	// full-slug is linked once, only if not already present in Outlinks.
	contentRendered, outlinks = appendWikiSeeAlso(contentRendered, outlinks, planItem.RelatedKB, p.datasetID, pageTitles, slugToPageType)
	sourceDocIDs := collectWikiSourceDocIDs(p.inputs.Chunks, sourceChunkIDs, p.docID)
	summary := firstParagraph(contentRendered)
	if summary == "" {
		summary = firstNonEmpty(planItem.Title, planItem.Slug)
	}
	topic := firstNonEmpty(planItem.Topic, planItem.Title, planItem.Slug)
	return wikiPageResult{
		Slug:           planItem.Slug,
		Title:          firstNonEmpty(planItem.Title, planItem.Slug),
		PageType:       firstNonEmpty(planItem.PageType, "concept"),
		Topic:          topic,
		Action:         firstNonEmpty(planItem.Action, "CREATE"),
		EntityNames:    uniqueStrings(planItem.EntityNames),
		RelatedKBPages: uniqueStrings(planItem.RelatedKB),
		ContentRaw:     contentRaw,
		Content:        contentRendered,
		Summary:        summary,
		Outlinks:       outlinks,
		SourceChunkIDs: sourceChunkIDs,
		SourceDocIDs:   sourceDocIDs,
		PlanGroup:      planItem.PlanGroup,
	}, nil
}

// maxPagesForBatch is the per-batch planner ceiling. It is the allocated quota
// (a fraction of the global target) directly: the quota is already bounded by
// target <= max <= output-token capacity, so no separate static cap is needed.
// A zero/negative quota yields 1 so runPlanBatch is never told "0 pages". The
// old static wikiPlanMaxPagesPerBatch cap is gone: capping a large quota at 8
// would make the P0 target unreachable for a single high-quota batch.
func maxPagesForBatch(quota int) int {
	if quota < 1 {
		return 1
	}
	return quota
}

func (p *wikiPipeline) runPlanBatch(batch wikiExtract, batchIndex, batchTotal, quota, maxTokens int) (wikiPlan, error) {
	planningModeRules := "Entity mode: create exactly one page for each extracted entity or concept. Do not merge multiple identities into one page, and each page's entity_names must contain only its own identity."
	if p.wikiMode() == "topic" {
		planningModeRules = "Topic mode: closely related extracted entities or concepts may be combined into one page when the source evidence supports it. Include every combined identity in entity_names."
	}
	user := renderWikiTemplate(wikiPlanBatchUserTemplate, map[string]string{
		"doc_id":              p.docID,
		"batch_index":         fmt.Sprintf("%d", batchIndex),
		"batch_total":         fmt.Sprintf("%d", batchTotal),
		"max_pages":           fmt.Sprintf("%d", maxPagesForBatch(quota)),
		"entities":            mustJSON(batch.Entities),
		"concepts":            mustJSON(batch.Concepts),
		"claims":              mustJSON(batch.Claims),
		"relations":           mustJSON(batch.Relations),
		"topics":              mustJSON(batch.Topics),
		"planning_mode_rules": planningModeRules,
	})
	raw, err := common.GenJSON(p.ctx, p.deps.Chat, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiPlanSystem,
		UserPrompt:   user,
		MaxTokens:    &maxTokens,
	})
	if err != nil {
		return wikiPlan{}, err
	}
	return parseWikiPlan(raw, p.docID, batch), nil
}

// mergePlanCandidates merges per-batch plans into one plan. reduced is the item
// set the merged plan is allowed to reference (fallback/normalization); in the
// quota-filtered PLAN path this is the approved (non-zero-quota) set so items
// from skipped batches can never leak back in via fallback pages.
func (p *wikiPipeline) mergePlanCandidates(plans []wikiPlan, reduced wikiExtract) wikiPlan {
	merged := wikiPlan{}
	mergedEntities := map[string]bool{}
	mergedRelated := map[string]bool{}
	for _, plan := range plans {
		if merged.Title == "" {
			merged.Title = strings.TrimSpace(plan.Title)
		}
		if merged.Slug == "" {
			merged.Slug = strings.TrimSpace(plan.Slug)
		}
		if merged.Lead == "" {
			merged.Lead = strings.TrimSpace(plan.Lead)
		}
		if merged.PageType == "" {
			merged.PageType = strings.TrimSpace(plan.PageType)
		}
		if merged.Topic == "" {
			merged.Topic = strings.TrimSpace(plan.Topic)
		}
		if len(merged.Sections) == 0 && len(plan.Sections) > 0 {
			merged.Sections = append([]wikiPlanSection(nil), plan.Sections...)
		}
		merged.Pages = append(merged.Pages, plan.Pages...)
		for _, name := range plan.Entities {
			name = strings.TrimSpace(name)
			if name != "" && !mergedEntities[name] {
				mergedEntities[name] = true
				merged.Entities = append(merged.Entities, name)
			}
		}
		for _, slug := range plan.Related {
			slug = strings.TrimSpace(slug)
			if slug != "" && !mergedRelated[slug] {
				mergedRelated[slug] = true
				merged.Related = append(merged.Related, slug)
			}
		}
	}
	merged = normalizeWikiPlan(merged, p.docID, reduced)
	merged.Pages = assembleWikiPlanRelatedPages(merged.Pages, reduced.Relations)
	merged.Pages = normalizeWikiPlanPageLinks(merged.Pages)
	return merged
}

// assembleWikiPlanRelatedPages derives page links from the reduced relation
// graph. The planner only decides page identity and topic; generated slugs,
// priorities, actions, and cross-page links stay deterministic and do not
// consume LLM output tokens.
func assembleWikiPlanRelatedPages(pages []wikiPlanPage, relations []wikiRelation) []wikiPlanPage {
	if len(pages) == 0 || len(relations) == 0 {
		return pages
	}

	pageByName := make(map[string][]int)
	for i, page := range pages {
		names := uniqueStrings(append([]string{}, page.EntityNames...))
		if len(names) == 0 && strings.TrimSpace(page.Title) != "" {
			names = []string{page.Title}
		}
		for _, name := range names {
			if key := normKey(name); key != "" {
				pageByName[key] = append(pageByName[key], i)
			}
		}
	}

	related := make([]map[string]struct{}, len(pages))
	for i := range related {
		related[i] = make(map[string]struct{})
		for _, slug := range pages[i].RelatedKB {
			if slug = strings.TrimSpace(slug); slug != "" {
				related[i][slug] = struct{}{}
			}
		}
	}
	for _, relation := range relations {
		fromPages := pageByName[normKey(relation.From)]
		toPages := pageByName[normKey(relation.To)]
		for _, from := range fromPages {
			for _, to := range toPages {
				if from == to || strings.TrimSpace(pages[to].Slug) == "" {
					continue
				}
				related[from][pages[to].Slug] = struct{}{}
				if strings.TrimSpace(pages[from].Slug) != "" {
					related[to][pages[from].Slug] = struct{}{}
				}
			}
		}
	}

	for i := range pages {
		pages[i].RelatedKB = make([]string, 0, len(related[i]))
		for slug := range related[i] {
			pages[i].RelatedKB = append(pages[i].RelatedKB, slug)
		}
		sort.Strings(pages[i].RelatedKB)
	}
	return pages
}

func (p *wikiPipeline) reconcilePlan(plan wikiPlan) (wikiPlan, error) {
	if p.deps.WikiPages == nil || p.deps.Embed == nil || strings.TrimSpace(p.datasetID) == "" || len(plan.Pages) == 0 {
		return plan, nil
	}
	queryTexts := make([]string, 0, len(plan.Pages))
	pageIdx := make([]int, 0, len(plan.Pages))
	for i, page := range plan.Pages {
		page = normalizeWikiPlanPage(page)
		plan.Pages[i] = page
		query := buildWikiPageQueryText(page)
		if strings.TrimSpace(query) == "" {
			continue
		}
		queryTexts = append(queryTexts, query)
		pageIdx = append(pageIdx, i)
	}
	if len(queryTexts) == 0 {
		return plan, nil
	}
	vectors, err := p.deps.Embed.Encode(p.ctx, queryTexts)
	if err != nil {
		return wikiPlan{}, err
	}
	for i, idx := range pageIdx {
		if idx >= len(plan.Pages) || i >= len(vectors) {
			continue
		}
		page := plan.Pages[idx]
		existing, err := p.reconcilePlanPage(page, vectors[i])
		if err != nil {
			return wikiPlan{}, err
		}
		if existing == nil {
			continue
		}
		page.Action = "UPDATE"
		page.Slug = firstNonEmpty(existing.Slug, page.Slug)
		page.Title = firstNonEmpty(existing.Title, page.Title)
		page.PageType = firstNonEmpty(existing.PageType, page.PageType)
		if existing.RoutedTopic != "" {
			page.Topic = common.NormalizeWikiTopicPath(existing.RoutedTopic)
		} else {
			page.Topic = common.NormalizeWikiTopicPath(firstNonEmpty(existing.Topic, page.Topic))
		}
		page.RelatedKB = mergeStrings(page.RelatedKB, existing.RelatedKBPages)
		if len(existing.RoutedEntityNames) > 0 {
			// An explicit route membership set is authoritative: this is a
			// migration, so do not union members that the LLM removed.
			page.EntityNames = uniqueStrings(existing.RoutedEntityNames)
		} else {
			page.EntityNames = mergeStrings(page.EntityNames, existing.EntityNames)
		}
		plan.Pages[idx] = page
	}
	plan.Pages = normalizeWikiPlanPages(plan.Pages, p.reduced)
	return plan, nil
}

// reconcilePlanPage decides UPDATE / CREATE for one planned page against the
// existing wiki-page store. See the threshold block above for how the band and
// the overlap heuristic relate to Python's wiki.py contract.
func (p *wikiPipeline) reconcilePlanPage(page wikiPlanPage, queryVec []float32) (*common.WikiPageCandidate, error) {
	if p.deps.WikiPages == nil {
		return nil, nil
	}
	if hit, err := p.deps.WikiPages.GetPageBySlug(p.ctx, p.tenantID, p.datasetID, page.Slug); err != nil {
		return nil, err
	} else if hit != nil {
		return hit, nil
	}
	cands, err := p.deps.WikiPages.FindSimilarPages(p.ctx, p.tenantID, p.datasetID, queryVec, wikiPlanSearchTopK)
	if err != nil || len(cands) == 0 {
		return nil, err
	}
	// Topic incremental routing uses more than page-vector similarity: pages
	// linked from the KNN candidates are added as semantic neighbors, so a
	// related page whose title is not embedding-similar can still be considered
	// by the LLM route decision.
	cands = p.expandTopicNeighborCandidates(page, cands)
	cands = rerankWikiPlanCandidates(page, cands)
	targetNames := normalizedStringSet(page.EntityNames)
	pageTitle := normKey(page.Title)
	pageTopic := wikiTopicKey(page.Topic)
	maybe := make([]common.WikiPageCandidate, 0, len(cands))
	for i := range cands {
		cand := cands[i]
		if cand.Score >= wikiPlanUpdateThreshold {
			return &cand, nil
		}
		if cand.Score < wikiPlanMaybeThreshold {
			continue
		}
		if normKey(cand.Title) == pageTitle || wikiTopicKey(cand.Topic) == pageTopic {
			return &cand, nil
		}
		if intersectsNormalized(targetNames, cand.EntityNames) {
			return &cand, nil
		}
		maybe = append(maybe, cand)
	}
	if len(maybe) > 0 {
		return p.resolveMaybePlanPage(page, maybe)
	}
	return nil, nil
}

func (p *wikiPipeline) expandTopicNeighborCandidates(page wikiPlanPage, candidates []common.WikiPageCandidate) []common.WikiPageCandidate {
	if p.deps.WikiPages == nil {
		return candidates
	}
	if p.neighborCache == nil {
		p.neighborCache = make(map[string]*common.WikiPageCandidate)
	}
	out := append([]common.WikiPageCandidate(nil), candidates...)
	seen := make(map[string]struct{}, len(out))
	for _, candidate := range out {
		seen[candidate.Slug] = struct{}{}
	}
	for _, candidate := range candidates {
		for _, slug := range candidate.RelatedKBPages {
			if len(out) >= len(candidates)+wikiTopicNeighborMax {
				break
			}
			slug = strings.TrimSpace(slug)
			if slug == "" {
				continue
			}
			if _, exists := seen[slug]; exists {
				continue
			}
			var neighbor *common.WikiPageCandidate
			if cached, ok := p.neighborCache[slug]; ok {
				neighbor = cached
			} else {
				loaded, err := p.deps.WikiPages.GetPageBySlug(p.ctx, p.tenantID, p.datasetID, slug)
				if err != nil {
					p.neighborCache[slug] = nil
					continue
				}
				neighbor = loaded
				p.neighborCache[slug] = loaded
			}
			if neighbor == nil {
				continue
			}
			seen[neighbor.Slug] = struct{}{}
			neighbor.Score = candidate.Score * 0.95
			out = append(out, *neighbor)
		}
	}
	if store, ok := p.deps.WikiPages.(common.WikiPageCooccurrenceStore); ok {
		if chunks := p.sourceChunksForPlanPage(page); len(chunks) > 0 {
			cooccurring, err := store.FindPagesBySourceChunks(p.ctx, p.tenantID, p.datasetID, chunks, wikiPlanSearchTopK)
			if err == nil {
				for _, candidate := range cooccurring {
					if candidate.Slug == "" {
						continue
					}
					if _, exists := seen[candidate.Slug]; exists {
						continue
					}
					candidate.Score = 0.68
					seen[candidate.Slug] = struct{}{}
					out = append(out, candidate)
				}
			}
		}
	}
	return out
}

func (p *wikiPipeline) sourceChunksForPlanPage(page wikiPlanPage) []string {
	names := normalizedStringSet(page.EntityNames)
	for _, name := range []string{page.Title, page.Topic, common.WikiTopicLeaf(page.Topic)} {
		if key := normKey(name); key != "" {
			names[key] = struct{}{}
		}
	}
	chunks := make([]string, 0)
	for _, entity := range p.reduced.Entities {
		if _, ok := names[normKey(entity.Name)]; ok {
			chunks = append(chunks, entity.SourceChunkIDs...)
		}
	}
	for _, concept := range p.reduced.Concepts {
		if _, ok := names[normKey(concept.Term)]; ok {
			chunks = append(chunks, concept.SourceChunkIDs...)
		}
	}
	for _, claim := range p.reduced.Claims {
		if _, ok := names[normKey(claim.Subject)]; ok {
			chunks = append(chunks, claim.SourceChunkIDs...)
		}
	}
	for _, relation := range p.reduced.Relations {
		if _, fromOK := names[normKey(relation.From)]; fromOK {
			chunks = append(chunks, relation.SourceChunkIDs...)
			continue
		}
		if _, toOK := names[normKey(relation.To)]; toOK {
			chunks = append(chunks, relation.SourceChunkIDs...)
		}
	}
	chunks = uniqueStrings(chunks)
	if len(chunks) > wikiTopicSourceChunkMax {
		chunks = chunks[:wikiTopicSourceChunkMax]
	}
	return chunks
}

func rerankWikiPlanCandidates(page wikiPlanPage, candidates []common.WikiPageCandidate) []common.WikiPageCandidate {
	if len(candidates) <= 1 {
		return candidates
	}
	type scored struct {
		candidate common.WikiPageCandidate
		boost     float64
	}
	targetNames := normalizedStringSet(page.EntityNames)
	pageType := normKey(page.PageType)
	pageTitle := normKey(page.Title)
	pageTopic := wikiTopicKey(page.Topic)
	scoredCands := make([]scored, 0, len(candidates))
	for _, cand := range candidates {
		boost := cand.Score
		if pageType != "" && normKey(cand.PageType) == pageType {
			boost += 0.08
		}
		if pageTitle != "" && normKey(cand.Title) == pageTitle {
			boost += 0.12
		}
		if pageTopic != "" && wikiTopicKey(cand.Topic) == pageTopic {
			boost += 0.08
		}
		if overlap := normalizedOverlapCount(targetNames, cand.EntityNames); overlap > 0 {
			boost += 0.05 * float64(overlap)
		}
		scoredCands = append(scoredCands, scored{candidate: cand, boost: boost})
	}
	sort.SliceStable(scoredCands, func(i, j int) bool {
		if scoredCands[i].boost == scoredCands[j].boost {
			return scoredCands[i].candidate.Slug < scoredCands[j].candidate.Slug
		}
		return scoredCands[i].boost > scoredCands[j].boost
	})
	out := make([]common.WikiPageCandidate, 0, len(scoredCands))
	for _, item := range scoredCands {
		out = append(out, item.candidate)
	}
	return out
}

func (p *wikiPipeline) resolveMaybePlanPage(page wikiPlanPage, candidates []common.WikiPageCandidate) (*common.WikiPageCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	raw, err := common.GenJSON(p.ctx, p.deps.Chat, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiPlanReconcileSystem,
		UserPrompt: renderWikiTemplate(wikiPlanReconcileUserTemplate, map[string]string{
			"planned_page": mustPrettyJSON(page),
			"candidates":   mustPrettyJSON(candidates),
		}),
	})
	if err != nil {
		return nil, err
	}
	action := strings.ToUpper(strings.TrimSpace(firstString(raw["action"])))
	if action != "UPDATE" {
		return nil, nil
	}
	slug := strings.TrimSpace(firstString(raw["slug"]))
	if slug == "" {
		return nil, nil
	}
	for i := range candidates {
		if candidates[i].Slug == slug {
			if topic := common.NormalizeWikiTopicPath(candidates[i].Topic); topic != "" {
				// UPDATE preserves the existing page's materialized topic path.
				candidates[i].RoutedTopic = topic
			} else if topic := common.NormalizeWikiTopicPath(firstString(raw["topic"])); topic != "" {
				candidates[i].RoutedTopic = topic
			}
			if names := parseWikiStrings(raw["entity_names"]); len(names) > 0 {
				candidates[i].RoutedEntityNames = names
			}
			return &candidates[i], nil
		}
	}
	return nil, nil
}

func buildWikiPageQueryText(page wikiPlanPage) string {
	parts := []string{page.Title, page.Topic, strings.Join(page.EntityNames, " ")}
	var out []string
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

func (p *wikiPipeline) mergeWikiPageContent(existingMD, newMD, slug string) (string, error) {
	if strings.TrimSpace(existingMD) == "" {
		return newMD, nil
	}
	if strings.TrimSpace(newMD) == "" {
		return existingMD, nil
	}
	if strings.TrimSpace(existingMD) == strings.TrimSpace(newMD) {
		return newMD, nil
	}
	fallback := conservativeMergeWikiMarkdown(existingMD, newMD)
	user := fmt.Sprintf("Merge these two versions of wiki page `%s`:\n\n## EXISTING VERSION\n\n%s\n\n---\n\n## INCOMING VERSION\n\n%s\n\n---\n\nProduce the merged page now. Return only the markdown content.", slug, existingMD, newMD)
	resp, err := p.deps.Chat.Chat(p.ctx, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiRefineMergeSystem,
		UserPrompt:   user,
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return fallback, nil
	}
	merged := strings.TrimSpace(firstNonEmpty(resp.Content))
	if merged == "" {
		return fallback, nil
	}
	minAcceptable := int(float64(maxInt(len(existingMD), len(newMD))) * wikiMergeShrinkRatio)
	if len(merged) < minAcceptable {
		return fallback, nil
	}
	if wikiMarkdownDropsContent(merged, existingMD) || wikiMarkdownDropsContent(merged, newMD) {
		return fallback, nil
	}
	return merged, nil
}

func conservativeMergeWikiMarkdown(existingMD, newMD string) string {
	if strings.Contains(existingMD, newMD) {
		return strings.TrimSpace(existingMD)
	}
	if strings.Contains(newMD, existingMD) {
		return strings.TrimSpace(newMD)
	}
	title := firstNonEmpty(wikiMarkdownTitle(existingMD), wikiMarkdownTitle(newMD))
	blocks := make([]string, 0, 8)
	seen := map[string]bool{}
	appendBlocks := func(body string) {
		for _, block := range splitWikiMarkdownBlocks(body) {
			key := normKey(block)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			blocks = append(blocks, block)
		}
	}
	appendBlocks(wikiMarkdownBody(existingMD))
	appendBlocks(wikiMarkdownBody(newMD))
	if title != "" {
		if len(blocks) == 0 {
			return "# " + title
		}
		return "# " + title + "\n\n" + strings.Join(blocks, "\n\n")
	}
	return strings.Join(blocks, "\n\n")
}

func wikiMarkdownDropsContent(mergedMD, sourceMD string) bool {
	mergedNorm := normKey(mergedMD)
	for _, block := range splitWikiMarkdownBlocks(wikiMarkdownBody(sourceMD)) {
		if !wikiMarkdownContentBlock(block) {
			continue
		}
		if !strings.Contains(mergedNorm, normKey(block)) {
			return true
		}
	}
	return false
}

func wikiMarkdownTitle(md string) string {
	for line := range strings.SplitSeq(md, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if line != "" {
			break
		}
	}
	return ""
}

func wikiMarkdownBody(md string) string {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
		if line != "" {
			break
		}
	}
	return strings.TrimSpace(md)
}

func splitWikiMarkdownBlocks(md string) []string {
	parts := strings.Split(strings.TrimSpace(md), "\n\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func wikiMarkdownContentBlock(block string) bool {
	block = strings.TrimSpace(block)
	if block == "" {
		return false
	}
	if strings.HasPrefix(block, "#") {
		return false
	}
	return true
}

func reduceExtracts(extracts []wikiExtract) wikiExtract {
	out := wikiExtract{}
	type entityKey struct {
		Name string
		Type string
	}
	type conceptKey struct {
		Term string
	}
	entities := map[entityKey]*wikiEntity{}
	concepts := map[conceptKey]*wikiConcept{}
	relationIndexes := map[string]int{}
	topics := map[string]*wikiTopic{}

	for _, ex := range extracts {
		for _, e := range ex.Entities {
			e.Name = strings.Join(strings.Fields(e.Name), " ")
			e.Type = strings.Join(strings.Fields(e.Type), " ")
			key := entityKey{Name: normKey(e.Name), Type: normKey(e.Type)}
			if cur, ok := entities[key]; ok {
				cur.Aliases = mergeStrings(cur.Aliases, e.Aliases)
				cur.SourceChunkIDs = mergeStrings(cur.SourceChunkIDs, e.SourceChunkIDs)
				if cur.Type == "" {
					cur.Type = e.Type
				}
				continue
			}
			item := e
			item.Aliases = uniqueStrings(item.Aliases)
			item.SourceChunkIDs = uniqueStrings(item.SourceChunkIDs)
			entities[key] = &item
		}
		for _, c := range ex.Concepts {
			key := conceptKey{Term: normKey(c.Term)}
			if cur, ok := concepts[key]; ok {
				if cur.Definition == "" {
					cur.Definition = c.Definition
				}
				cur.SourceChunkIDs = mergeStrings(cur.SourceChunkIDs, c.SourceChunkIDs)
				continue
			}
			item := c
			item.Term = strings.TrimSpace(item.Term)
			item.Definition = strings.TrimSpace(item.Definition)
			item.SourceChunkIDs = uniqueStrings(item.SourceChunkIDs)
			concepts[key] = &item
		}
		for _, c := range ex.Claims {
			c.SourceChunkIDs = uniqueStrings(c.SourceChunkIDs)
			out.Claims = append(out.Claims, c)
		}
		for _, r := range ex.Relations {
			key := normKey(r.From) + "\x00" + normKey(r.Type) + "\x00" + normKey(r.To)
			if cur, ok := relationIndexes[key]; ok {
				out.Relations[cur].SourceChunkIDs = mergeStrings(out.Relations[cur].SourceChunkIDs, r.SourceChunkIDs)
				continue
			}
			r.SourceChunkIDs = uniqueStrings(r.SourceChunkIDs)
			relationIndexes[key] = len(out.Relations)
			out.Relations = append(out.Relations, r)
		}
		for _, t := range ex.Topics {
			t.Path = common.NormalizeWikiTopicPath(t.Path)
			if t.Path == "" {
				continue
			}
			key := normKey(t.Path)
			if current := topics[key]; current != nil {
				if current.Description == "" {
					current.Description = strings.TrimSpace(t.Description)
				}
				current.SourceChunkIDs = mergeStrings(current.SourceChunkIDs, t.SourceChunkIDs)
				continue
			}
			t.Description = strings.TrimSpace(t.Description)
			t.SourceChunkIDs = uniqueStrings(t.SourceChunkIDs)
			topic := t
			topics[key] = &topic
		}
	}

	out.Entities = make([]wikiEntity, 0, len(entities))
	keys := make([]entityKey, 0, len(entities))
	for k := range entities {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Type == keys[j].Type {
			return keys[i].Name < keys[j].Name
		}
		return keys[i].Type < keys[j].Type
	})
	for _, k := range keys {
		out.Entities = append(out.Entities, *entities[k])
	}
	out.Concepts = make([]wikiConcept, 0, len(concepts))
	ckeys := make([]conceptKey, 0, len(concepts))
	for k := range concepts {
		ckeys = append(ckeys, k)
	}
	sort.Slice(ckeys, func(i, j int) bool { return ckeys[i].Term < ckeys[j].Term })
	for _, k := range ckeys {
		out.Concepts = append(out.Concepts, *concepts[k])
	}
	topicKeys := make([]string, 0, len(topics))
	for key := range topics {
		topicKeys = append(topicKeys, key)
	}
	sort.Strings(topicKeys)
	for _, key := range topicKeys {
		out.Topics = append(out.Topics, *topics[key])
	}
	return out
}

func parseWikiExtract(raw map[string]any) wikiExtract {
	out := wikiExtract{}
	out.Entities = parseWikiEntities(raw["entities"])
	out.Concepts = parseWikiConcepts(raw["concepts"])
	out.Claims = parseWikiClaims(raw["claims"])
	out.Relations = parseWikiRelations(raw["relations"])
	out.Topics = parseWikiTopics(raw["topics"])
	return out
}

func parseWikiPlan(raw map[string]any, docID string, reduced wikiExtract) wikiPlan {
	plan := wikiPlan{
		Title:    firstString(raw["title"]),
		Slug:     firstString(raw["slug"]),
		Lead:     firstString(raw["lead"]),
		Sections: parseWikiPlanSections(raw["sections"]),
		PageType: firstString(raw["page_type"]),
		Topic:    firstString(raw["topic"]),
		Entities: parseWikiStrings(raw["entity_names"]),
		Related:  parseWikiStrings(raw["related_kb_pages"]),
		Pages:    parseWikiPlanPages(raw["pages"]),
	}
	if len(plan.Pages) > 0 {
		first := plan.Pages[0]
		if plan.Title == "" {
			plan.Title = first.Title
		}
		if plan.Slug == "" {
			plan.Slug = first.Slug
		}
		if plan.Lead == "" {
			plan.Lead = first.Lead
		}
		if plan.PageType == "" {
			plan.PageType = first.PageType
		}
		if plan.Topic == "" {
			plan.Topic = first.Topic
		}
		if len(plan.Entities) == 0 {
			plan.Entities = first.EntityNames
		}
		if len(plan.Related) == 0 {
			plan.Related = first.RelatedKB
		}
		if len(plan.Sections) == 0 {
			plan.Sections = first.Sections
		}
	}
	if plan.Title == "" {
		if len(reduced.Topics) > 0 {
			plan.Title = common.WikiTopicLeaf(reduced.Topics[0].Path)
		} else if len(reduced.Entities) > 0 {
			plan.Title = reduced.Entities[0].Name
		} else if len(reduced.Concepts) > 0 {
			plan.Title = reduced.Concepts[0].Term
		} else {
			plan.Title = docID
		}
	}
	if plan.Slug == "" {
		plan.Slug = slugify(plan.Title)
	}
	if plan.Lead == "" {
		plan.Lead = plan.Title
	}
	if len(plan.Sections) == 0 {
		plan.Sections = []wikiPlanSection{{Heading: "Overview", Points: []string{plan.Lead}}}
	}
	return plan
}

func normalizeWikiPlan(plan wikiPlan, docID string, reduced wikiExtract) wikiPlan {
	plan.Pages = normalizeWikiPlanPages(plan.Pages, reduced)
	if plan.Title == "" {
		plan.Title = firstPlanTitle(plan.Pages, reduced, docID)
	}
	if plan.Slug == "" {
		plan.Slug = slugify(plan.Title)
	}
	if plan.Lead == "" {
		plan.Lead = plan.Title
	}
	if len(plan.Sections) == 0 {
		plan.Sections = []wikiPlanSection{{Heading: "Overview", Points: []string{plan.Lead}}}
	}
	return plan
}

func normalizeWikiPlanPages(pages []wikiPlanPage, reduced wikiExtract) []wikiPlanPage {
	// existing maps a plan slug to its index in out (exact-slug dedup); byTitle
	// additionally collapses non-canonical pages that share the same page_type +
	// normalized title. Single-entity pages use their canonical slug instead so
	// same-named entities of different types remain distinct.
	existing := map[string]int{}
	byTitle := map[string]int{}
	out := make([]wikiPlanPage, 0, len(pages))
	for _, page := range pages {
		page = normalizeWikiPlanPage(page)
		page.Topic = selectWikiTopicPath(page.Topic, page.Title, page.PageType, wikiTopicPaths(reduced.Topics))
		if page.Slug == "" {
			continue
		}
		if idx, ok := existing[page.Slug]; ok {
			out[idx].RelatedKB = uniqueStrings(append(out[idx].RelatedKB, page.RelatedKB...))
			continue
		}
		key := wikiTitleKey(page.PageType, page.Title)
		if page.PageType == "entity" && len(page.EntityNames) == 1 {
			key = page.PageType + "\x00" + page.Slug
		}
		if idx, ok := byTitle[key]; ok {
			out[idx].RelatedKB = uniqueStrings(append(out[idx].RelatedKB, page.RelatedKB...))
			continue
		}
		idx := len(out)
		byTitle[key] = idx
		existing[page.Slug] = idx
		out = append(out, page)
	}
	fallback := buildWikiFallbackPages(reduced)
	if len(out) == 0 {
		for _, page := range fallback {
			if page.Slug == "" {
				continue
			}
			page = normalizeWikiPlanPage(page)
			page.Topic = selectWikiTopicPath(page.Topic, page.Title, page.PageType, wikiTopicPaths(reduced.Topics))
			idx := len(out)
			key := wikiTitleKey(page.PageType, page.Title)
			if page.PageType == "entity" && len(page.EntityNames) == 1 {
				key = page.PageType + "\x00" + page.Slug
			}
			byTitle[key] = idx
			existing[page.Slug] = idx
			out = append(out, page)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].Slug < out[j].Slug
		}
		return out[i].Priority < out[j].Priority
	})
	for i := range out {
		if out[i].Priority <= 0 {
			out[i].Priority = i + 1
		}
	}
	return out
}

func normalizeWikiPlanPageLinks(pages []wikiPlanPage) []wikiPlanPage {
	if len(pages) == 0 {
		return nil
	}
	valid := make(map[string]bool, len(pages))
	for _, page := range pages {
		if slug := strings.TrimSpace(page.Slug); slug != "" {
			valid[slug] = true
		}
	}
	out := make([]wikiPlanPage, 0, len(pages))
	for _, page := range pages {
		related := make([]string, 0, len(page.RelatedKB))
		seen := map[string]bool{}
		for _, slug := range page.RelatedKB {
			slug = strings.TrimSpace(slug)
			if slug == "" || slug == page.Slug || !valid[slug] || seen[slug] {
				continue
			}
			seen[slug] = true
			related = append(related, slug)
		}
		page.RelatedKB = related
		out = append(out, page)
	}
	return out
}

func normalizeWikiTopicPaths(topics []string) []string {
	out := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		topic = common.NormalizeWikiTopicPath(topic)
		key := normKey(topic)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, topic)
	}
	return out
}

func wikiTopicPaths(topics []wikiTopic) []string {
	paths := make([]string, 0, len(topics))
	for _, topic := range topics {
		paths = append(paths, topic.Path)
	}
	return normalizeWikiTopicPaths(paths)
}

func wikiTopicKey(topic string) string {
	return normKey(common.NormalizeWikiTopicPath(topic))
}

// selectWikiTopicPath resolves a planner topic to a MAP candidate. PLAN is not
// allowed to create topics: unmatched pages are placed in General. A unique
// leaf match still expands to its complete MAP path, which preserves
// multi-level topics while accepting concise LLM references.
func selectWikiTopicPath(topic, title, pageType string, candidates []string) string {
	topic = common.NormalizeWikiTopicPath(topic)
	candidates = normalizeWikiTopicPaths(candidates)
	pageType = normKey(pageType)
	title = strings.TrimSpace(title)
	selfTopic := func(candidate string) bool {
		return (pageType == "entity" || pageType == "concept") &&
			normKey(common.WikiTopicLeaf(candidate)) == normKey(title)
	}
	if topic != "" {
		for _, candidate := range candidates {
			if normKey(candidate) == normKey(topic) && !selfTopic(candidate) {
				return candidate
			}
		}
		leafMatches := make([]string, 0, 1)
		for _, candidate := range candidates {
			if normKey(common.WikiTopicLeaf(candidate)) == normKey(topic) && !selfTopic(candidate) {
				leafMatches = append(leafMatches, candidate)
			}
		}
		if len(leafMatches) == 1 {
			return leafMatches[0]
		}
		return common.GeneralWikiTopic
	}
	return common.GeneralWikiTopic
}

func normalizeWikiPlanPage(page wikiPlanPage) wikiPlanPage {
	page.Action = strings.ToUpper(strings.TrimSpace(page.Action))
	if page.Action == "" {
		page.Action = "CREATE"
	}
	page.Slug = strings.TrimSpace(page.Slug)
	if page.Slug == "" {
		page.Slug = slugify(page.Title)
	}
	// Canonicalize the slug to the hyphen style used by slug_kwd / outlinks /
	// artifact links. The LLM freely mixes "dong_zhuo" and "dong-zhuo", so this
	// single choke point makes plan slugs, slugToPageType/pageTitles keys and
	// the writer's bare/full reverse index all agree.
	page.Slug = normalizeWikiSlugHyphens(page.Slug)
	page.Title = strings.TrimSpace(page.Title)
	if page.Title == "" {
		page.Title = page.Slug
	}
	page.PageType = strings.TrimSpace(page.PageType)
	if page.PageType == "" {
		page.PageType = "concept"
	}
	page.Topic = common.NormalizeWikiTopicPath(page.Topic)
	if page.Topic == "" {
		page.Topic = common.GeneralWikiTopic
	}
	page.EntityNames = uniqueStrings(page.EntityNames)
	page.RelatedKB = uniqueStrings(page.RelatedKB)
	page.Sections = normalizeWikiPlanSections(page.Sections)
	if page.Priority <= 0 {
		page.Priority = 99
	}
	return page
}

func normalizeWikiPlanSections(sections []wikiPlanSection) []wikiPlanSection {
	if len(sections) == 0 {
		return []wikiPlanSection{{Heading: "Overview", Points: nil}}
	}
	out := make([]wikiPlanSection, 0, len(sections))
	for _, sec := range sections {
		sec.Heading = strings.TrimSpace(sec.Heading)
		sec.Points = uniqueStrings(sec.Points)
		if sec.Heading == "" {
			continue
		}
		out = append(out, sec)
	}
	if len(out) == 0 {
		return []wikiPlanSection{{Heading: "Overview", Points: nil}}
	}
	return out
}

// wikiTitleKey returns the Scheme-A dedup key for a planned page: page_type
// scoped to the normalized title. Two pages collapse only when both their type
// and their (case/whitespace-normalized) title match — a same-title concept and
// topic stay distinct because page identity is page_type/slug.
func wikiTitleKey(pageType, title string) string {
	return strings.TrimSpace(pageType) + "\x00" + normKey(title)
}

func buildWikiFallbackPages(reduced wikiExtract) []wikiPlanPage {
	var out []wikiPlanPage
	seen := map[string]bool{}
	appendPage := func(page wikiPlanPage) {
		page = normalizeWikiPlanPage(page)
		if page.Slug == "" || seen[page.Slug] {
			return
		}
		seen[page.Slug] = true
		out = append(out, page)
	}
	for _, e := range reduced.Entities {
		appendPage(wikiPlanPage{
			Action:      "CREATE",
			Slug:        entityPageSlug(e.Name, e.Type),
			Title:       e.Name,
			PageType:    "entity",
			Topic:       e.Name,
			EntityNames: append([]string{e.Name}, e.Aliases...),
			Priority:    len(out) + 1,
			Lead:        e.Name,
			Sections:    []wikiPlanSection{{Heading: "Overview", Points: []string{e.Name}}},
		})
	}
	for _, c := range reduced.Concepts {
		appendPage(wikiPlanPage{
			Action:      "CREATE",
			Slug:        "concept/" + slugify(c.Term),
			Title:       c.Term,
			PageType:    "concept",
			Topic:       c.Term,
			EntityNames: []string{c.Term},
			Priority:    len(out) + 1,
			Lead:        c.Definition,
			Sections:    []wikiPlanSection{{Heading: "Overview", Points: []string{c.Term}}},
		})
	}
	if len(out) == 0 {
		for _, t := range reduced.Topics {
			title := common.WikiTopicLeaf(t.Path)
			lead := firstNonEmpty(t.Description, title)
			appendPage(wikiPlanPage{
				Action:   "CREATE",
				Slug:     "topic/" + slugify(title),
				Title:    title,
				PageType: "topic",
				Topic:    t.Path,
				Priority: len(out) + 1,
				Lead:     lead,
				Sections: []wikiPlanSection{{Heading: "Overview", Points: []string{lead}}},
			})
		}
	}
	if len(out) == 0 {
		appendPage(wikiPlanPage{
			Action:   "CREATE",
			Slug:     slugify(firstPlanTitle(nil, reduced, "")),
			Title:    firstPlanTitle(nil, reduced, ""),
			PageType: "concept",
			Topic:    firstPlanTitle(nil, reduced, ""),
			Priority: 1,
			Lead:     firstPlanTitle(nil, reduced, ""),
			Sections: []wikiPlanSection{{Heading: "Overview", Points: []string{firstPlanTitle(nil, reduced, "")}}},
		})
	}
	return out
}

func firstPlanTitle(pages []wikiPlanPage, reduced wikiExtract, docID string) string {
	for _, page := range pages {
		if s := strings.TrimSpace(page.Title); s != "" {
			return s
		}
	}
	if len(reduced.Topics) > 0 {
		return common.WikiTopicLeaf(reduced.Topics[0].Path)
	}
	if len(reduced.Entities) > 0 {
		return reduced.Entities[0].Name
	}
	if len(reduced.Concepts) > 0 {
		return reduced.Concepts[0].Term
	}
	if docID != "" {
		return docID
	}
	return "wiki"
}

func (p wikiPlan) firstPageCandidate() wikiPlanPage {
	if len(p.Pages) > 0 {
		return p.Pages[0]
	}
	return wikiPlanPage{
		Action:      "CREATE",
		Slug:        p.Slug,
		Title:       p.Title,
		PageType:    p.PageType,
		Topic:       p.Topic,
		EntityNames: uniqueStrings(p.Entities),
		RelatedKB:   uniqueStrings(p.Related),
		Priority:    1,
		Lead:        p.Lead,
		Sections:    p.Sections,
	}
}

func packWikiPlanBatches(reduced wikiExtract, budget int) []wikiExtract {
	if budget <= 0 {
		budget = wikiPlanTokenBudget
	}
	var batches []wikiExtract
	cur := wikiExtract{}
	curTokens := 0
	flush := func() {
		if len(cur.Entities)+len(cur.Concepts)+len(cur.Claims)+len(cur.Relations)+len(cur.Topics) > 0 {
			batches = append(batches, cur)
			cur = wikiExtract{}
			curTokens = 0
		}
	}
	add := func(itemTokens int, addFn func()) {
		if curTokens > 0 && curTokens+itemTokens > budget {
			flush()
		}
		addFn()
		curTokens += itemTokens
	}
	for _, e := range reduced.Entities {
		add(common.EstimateTokens(mustJSON(e)), func() { cur.Entities = append(cur.Entities, e) })
	}
	for _, c := range reduced.Concepts {
		add(common.EstimateTokens(mustJSON(c)), func() { cur.Concepts = append(cur.Concepts, c) })
	}
	for _, c := range reduced.Claims {
		add(common.EstimateTokens(mustJSON(c)), func() { cur.Claims = append(cur.Claims, c) })
	}
	for _, r := range reduced.Relations {
		add(common.EstimateTokens(mustJSON(r)), func() { cur.Relations = append(cur.Relations, r) })
	}
	for _, t := range reduced.Topics {
		add(common.EstimateTokens(mustJSON(t)), func() { cur.Topics = append(cur.Topics, t) })
	}
	flush()
	return batches
}

func parseWikiPlanPages(raw any) []wikiPlanPage {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiPlanPage, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		page := wikiPlanPage{
			Action:      strings.TrimSpace(firstString(m["action"])),
			Slug:        strings.TrimSpace(firstString(m["slug"])),
			Title:       strings.TrimSpace(firstString(m["title"])),
			PageType:    strings.TrimSpace(firstString(m["page_type"])),
			Topic:       strings.TrimSpace(firstString(m["topic"])),
			EntityNames: parseWikiStrings(m["entity_names"]),
			RelatedKB:   parseWikiStrings(m["related_kb_pages"]),
			Priority:    int(firstNumber(m["priority"])),
			Lead:        strings.TrimSpace(firstString(m["lead"])),
			Sections:    parseWikiPlanSections(m["sections"]),
		}
		if page.Slug != "" || page.Title != "" {
			out = append(out, page)
		}
	}
	return out
}

func parseWikiEntities(raw any) []wikiEntity {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiEntity, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		e := wikiEntity{
			Name:           strings.TrimSpace(firstString(m["name"])),
			Type:           strings.TrimSpace(firstString(m["type"])),
			Aliases:        parseWikiStrings(m["aliases"]),
			SourceChunkIDs: parseWikiStrings(m["source_chunk_ids"]),
		}
		if len(e.SourceChunkIDs) == 0 {
			if s := strings.TrimSpace(firstString(m["source_chunk_id"])); s != "" {
				e.SourceChunkIDs = []string{s}
			}
		}
		if e.Name != "" {
			out = append(out, e)
		}
	}
	return out
}

func parseWikiConcepts(raw any) []wikiConcept {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiConcept, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := wikiConcept{
			Term:           strings.TrimSpace(firstString(m["term"])),
			Definition:     strings.TrimSpace(firstString(m["definition_excerpt"])),
			SourceChunkIDs: parseWikiStrings(m["source_chunk_ids"]),
		}
		if len(c.SourceChunkIDs) == 0 {
			if s := strings.TrimSpace(firstString(m["source_chunk_id"])); s != "" {
				c.SourceChunkIDs = []string{s}
			}
		}
		if c.Term != "" {
			out = append(out, c)
		}
	}
	return out
}

func parseWikiClaims(raw any) []wikiClaim {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiClaim, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := wikiClaim{
			Statement:      strings.TrimSpace(firstString(m["statement"])),
			Subject:        strings.TrimSpace(firstString(m["subject"])),
			Confidence:     strings.TrimSpace(firstString(m["confidence"])),
			SourceChunkIDs: parseWikiStrings(m["source_chunk_ids"]),
		}
		if len(c.SourceChunkIDs) == 0 {
			if s := strings.TrimSpace(firstString(m["source_chunk_id"])); s != "" {
				c.SourceChunkIDs = []string{s}
			}
		}
		if c.Statement != "" {
			out = append(out, c)
		}
	}
	return out
}

func parseWikiRelations(raw any) []wikiRelation {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiRelation, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		r := wikiRelation{
			From:           strings.TrimSpace(firstString(m["from"])),
			To:             strings.TrimSpace(firstString(m["to"])),
			Type:           strings.TrimSpace(firstString(m["type"])),
			SourceChunkIDs: parseWikiStrings(m["source_chunk_ids"]),
		}
		if len(r.SourceChunkIDs) == 0 {
			if s := strings.TrimSpace(firstString(m["source_chunk_id"])); s != "" {
				r.SourceChunkIDs = []string{s}
			}
		}
		if r.From != "" && r.To != "" {
			out = append(out, r)
		}
	}
	return out
}

func parseWikiTopics(raw any) []wikiTopic {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiTopic, 0, len(arr))
	for _, item := range arr {
		// Accept the old string form as a read-only input compatibility path.
		// Newly generated MAP payloads still use the structured object form so
		// source-chunk provenance is retained.
		if path := common.NormalizeWikiTopicPath(firstString(item)); path != "" {
			out = append(out, wikiTopic{Path: path})
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		topic := wikiTopic{
			Path:           common.NormalizeWikiTopicPath(firstString(m["path"])),
			Description:    strings.TrimSpace(firstString(m["description"])),
			SourceChunkIDs: parseWikiStrings(m["source_chunk_ids"]),
		}
		if len(topic.SourceChunkIDs) == 0 {
			if chunkID := strings.TrimSpace(firstString(m["source_chunk_id"])); chunkID != "" {
				topic.SourceChunkIDs = []string{chunkID}
			}
		}
		if topic.Path != "" {
			out = append(out, topic)
		}
	}
	return normalizeWikiTopics(out)
}

func normalizeWikiTopics(topics []wikiTopic) []wikiTopic {
	out := make([]wikiTopic, 0, len(topics))
	indexes := make(map[string]int, len(topics))
	for _, topic := range topics {
		topic.Path = common.NormalizeWikiTopicPath(topic.Path)
		key := normKey(topic.Path)
		if key == "" {
			continue
		}
		topic.Description = strings.TrimSpace(topic.Description)
		topic.SourceChunkIDs = uniqueStrings(topic.SourceChunkIDs)
		if index, exists := indexes[key]; exists {
			if out[index].Description == "" {
				out[index].Description = topic.Description
			}
			out[index].SourceChunkIDs = mergeStrings(out[index].SourceChunkIDs, topic.SourceChunkIDs)
			continue
		}
		indexes[key] = len(out)
		out = append(out, topic)
	}
	return out
}

func parseWikiPlanSections(raw any) []wikiPlanSection {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]wikiPlanSection, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sec := wikiPlanSection{
			Heading: strings.TrimSpace(firstString(m["heading"])),
			Points:  parseWikiStrings(m["points"]),
		}
		if sec.Heading != "" {
			out = append(out, sec)
		}
	}
	return out
}

func firstNumber(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	default:
		return 0
	}
}

func parseWikiStrings(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return uniqueStrings(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(firstString(item)); s != "" {
				out = append(out, s)
			}
		}
		return uniqueStrings(out)
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return []string{s}
		}
	}
	return nil
}

func buildSourceContext(chunks []common.Chunk, sourceChunkIDs []string) string {
	if len(sourceChunkIDs) == 0 {
		var b strings.Builder
		for _, ch := range chunks {
			text := firstNonEmpty(ch.Text, ch.Content)
			if strings.TrimSpace(text) == "" {
				continue
			}
			b.WriteString(text)
			b.WriteString("\n\n")
		}
		return b.String()
	}
	lookup := map[string]string{}
	for i, ch := range chunks {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i+1)
		}
		lookup[id] = firstNonEmpty(ch.Text, ch.Content)
	}
	var b strings.Builder
	for _, id := range sourceChunkIDs {
		if text := strings.TrimSpace(lookup[id]); text != "" {
			b.WriteString("[CHUNK_ID ")
			b.WriteString(id)
			b.WriteString("]\n")
			b.WriteString(text)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func buildEvidenceChecklist(reduced wikiExtract) string {
	var lines []string
	for _, e := range reduced.Entities {
		lines = append(lines, fmt.Sprintf("- entity: %s (%s)", e.Name, e.Type))
	}
	for _, c := range reduced.Concepts {
		lines = append(lines, fmt.Sprintf("- concept: %s", c.Term))
	}
	for _, c := range reduced.Claims {
		lines = append(lines, fmt.Sprintf("- claim: %s", c.Statement))
	}
	for _, r := range reduced.Relations {
		lines = append(lines, fmt.Sprintf("- relation: %s -> %s (%s)", r.From, r.To, r.Type))
	}
	for _, t := range reduced.Topics {
		lines = append(lines, fmt.Sprintf("- topic: %s", t.Path))
	}
	if len(lines) == 0 {
		return "- (none)"
	}
	return strings.Join(lines, "\n")
}

type wikiEvidenceItem struct {
	Statement  string
	Subject    string
	Confidence string
	ChunkIDs   []string
	Synthetic  bool
}

func assembleWikiPageEvidence(planItem wikiPlanPage, claims []wikiClaim, entityByName, conceptByTerm map[string]wikiExtractItem) []wikiEvidenceItem {
	rawNames := uniqueStrings(append([]string{}, planItem.EntityNames...))
	if len(rawNames) == 0 {
		if t := strings.TrimSpace(planItem.Title); t != "" {
			rawNames = []string{t}
		} else if s := strings.TrimSpace(planItem.Slug); s != "" {
			rawNames = []string{s}
		}
	}
	if len(rawNames) == 0 {
		return nil
	}

	namesLower := make([]string, 0, len(rawNames))
	patterns := make([]*regexp.Regexp, 0, len(rawNames))
	for _, n := range rawNames {
		namesLower = append(namesLower, strings.ToLower(n))
		patterns = append(patterns, regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(n)+`\b`))
	}

	var evidence []wikiEvidenceItem
	for _, claim := range claims {
		subjRaw := strings.TrimSpace(claim.Subject)
		if subjRaw == "" {
			continue
		}
		subjLower := strings.ToLower(subjRaw)
		matched := false
		for _, n := range namesLower {
			if subjLower == n {
				matched = true
				break
			}
		}
		if !matched {
			for _, re := range patterns {
				if re.MatchString(subjRaw) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		evidence = append(evidence, wikiEvidenceItem{
			Statement:  claim.Statement,
			Subject:    claim.Subject,
			Confidence: firstNonEmpty(claim.Confidence, "explicit"),
			ChunkIDs:   uniqueStrings(claim.SourceChunkIDs),
		})
	}
	if len(evidence) > 0 {
		return evidence
	}

	var fallbackChunkIDs []string
	matchedNames := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		key := strings.ToLower(strings.TrimSpace(name))
		var hit wikiExtractItem
		var ok bool
		if entityByName != nil {
			hit, ok = entityByName[key]
		}
		if !ok && conceptByTerm != nil {
			hit, ok = conceptByTerm[key]
		}
		if !ok {
			continue
		}
		fallbackChunkIDs = mergeStrings(fallbackChunkIDs, hit.SourceChunkIDs)
		matchedNames = append(matchedNames, name)
	}
	if len(fallbackChunkIDs) == 0 {
		return nil
	}
	subject := rawNames[0]
	if len(matchedNames) > 0 {
		subject = matchedNames[0]
	}
	return []wikiEvidenceItem{{
		Statement:  "",
		Subject:    subject,
		Confidence: "inferred",
		ChunkIDs:   fallbackChunkIDs,
		Synthetic:  true,
	}}
}

type wikiExtractItem struct {
	SourceChunkIDs []string
}

func buildWikiEntityLookup(items []wikiEntity) map[string]wikiExtractItem {
	out := map[string]wikiExtractItem{}
	for _, item := range items {
		hit := wikiExtractItem{SourceChunkIDs: uniqueStrings(item.SourceChunkIDs)}
		name := strings.TrimSpace(item.Name)
		if name != "" {
			out[strings.ToLower(name)] = hit
		}
		for _, alias := range item.Aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				out[strings.ToLower(alias)] = hit
			}
		}
	}
	return out
}

func buildWikiConceptLookup(items []wikiConcept) map[string]wikiExtractItem {
	out := map[string]wikiExtractItem{}
	for _, item := range items {
		hit := wikiExtractItem{SourceChunkIDs: uniqueStrings(item.SourceChunkIDs)}
		term := strings.TrimSpace(item.Term)
		if term != "" {
			out[strings.ToLower(term)] = hit
		}
	}
	return out
}

func formatWikiEvidenceBlocks(evidence []wikiEvidenceItem) string {
	var lines []string
	for _, ev := range evidence {
		if ev.Synthetic {
			continue
		}
		confidence := strings.ToUpper(firstNonEmpty(ev.Confidence, "explicit"))
		lines = append(lines, fmt.Sprintf("%d. [%s] %s\n   %s", len(lines)+1, confidence, ev.Subject, ev.Statement))
	}
	if len(lines) == 0 {
		return "(no pre-extracted evidence — extract facts directly from the source document text above)"
	}
	return strings.Join(lines, "\n\n")
}

func collectWikiEvidenceChunkIDs(evidence []wikiEvidenceItem) []string {
	var out []string
	for _, ev := range evidence {
		out = mergeStrings(out, ev.ChunkIDs)
	}
	return out
}

func collectWikiSourceDocIDs(chunks []common.Chunk, chunkIDs []string, fallback string) []string {
	if len(chunkIDs) == 0 {
		if fallback != "" {
			return []string{fallback}
		}
		return nil
	}
	lookup := map[string][]string{}
	for _, ch := range chunks {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			continue
		}
		docID := strings.TrimSpace(firstString(ch.Meta["doc_id"]))
		if docID == "" {
			docID = strings.TrimSpace(firstString(ch.Meta["doc_id_kwd"]))
		}
		if docID != "" {
			lookup[id] = []string{docID}
		}
	}
	out := make([]string, 0, len(chunkIDs))
	for _, cid := range chunkIDs {
		if ids := lookup[strings.TrimSpace(cid)]; len(ids) > 0 {
			out = mergeStrings(out, ids)
		}
	}
	if len(out) == 0 && fallback != "" {
		return []string{fallback}
	}
	return out
}

var (
	wikiWikilinkPipeRe       = regexp.MustCompile(`\[\[([^\[\]\|]+?)\|([^\[\]]+?)\]\]`)
	wikiWikilinkSimpleRe     = regexp.MustCompile(`\[\[([^\[\]\|]+?)\]\]`)
	wikiArtifactMarkdownLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// pageTypeOf resolves the page_type for a slug so rendered internal links use
// the artifact/<kb>/<page_type>/<slug> form the frontend wiki-link parser
// requires (it only matches entity|concept). Unknown links fall back to "page".
func pageTypeOf(slug string, slugToPageType map[string]string) string {
	if pt := strings.TrimSpace(slugToPageType[slug]); pt != "" {
		return pt
	}
	if strings.Contains(slug, "/") {
		// Slugs may already carry a <page_type>/<name> prefix; reuse it.
		first := strings.SplitN(slug, "/", 2)[0]
		if first == "entity" || first == "concept" || first == "topic" {
			return first
		}
	}
	return "page"
}

// wikiSlugResolver resolves a wikitext slug (a bare "name" or a full
// "<page_type>/<slug>") to the canonical hyphen full-slug used as the page
// identifier (slug_kwd), mirroring Python's _wiki_resolve_dead_slug: plain
// names / titles are reverse-mapped to the full pid. An empty result means the
// target is not a known page and must not be emitted as a link or graph edge.
type wikiSlugResolver struct {
	slugToPageType map[string]string
	bareToSlug     map[string]string
	titleToSlug    map[string]string
}

// newWikiSlugResolver builds the reverse lookup maps from the available-page
// index. Plan slugs are canonicalized to the hyphen style upstream
// (normalizeWikiPlanPage -> normalizeWikiSlugHyphens), so slugToPageType /
// pageTitles keys are already hyphen full-slugs. LLM-authored wikitext links may
// still carry underscores (e.g. "dong_zhuo"); normalize both sides to hyphens so
// the reverse lookup matches.
func newWikiSlugResolver(pageTitles, slugToPageType map[string]string) *wikiSlugResolver {
	r := &wikiSlugResolver{
		slugToPageType: slugToPageType,
		bareToSlug:     map[string]string{},
		titleToSlug:    map[string]string{},
	}
	for fullSlug := range slugToPageType {
		r.bareToSlug[normalizeWikiSlugHyphens(lastPathSlug(fullSlug))] = fullSlug
	}
	for fullSlug, title := range pageTitles {
		if t := strings.TrimSpace(title); t != "" {
			r.titleToSlug[t] = fullSlug
		}
	}
	return r
}

// resolve returns the canonical hyphen full-slug for target, or "" when the
// target is not a known page (dead link). The result is always emitted in the
// canonical hyphen form so outlinks_kwd and slug_kwd agree in format.
func (r *wikiSlugResolver) resolve(target string) string {
	slug := strings.TrimSpace(target)
	if slug == "" {
		return ""
	}
	if _, ok := r.slugToPageType[slug]; ok {
		return normalizeWikiSlugHyphens(slug)
	}
	if full, ok := r.bareToSlug[normalizeWikiSlugHyphens(lastPathSlug(slug))]; ok {
		return normalizeWikiSlugHyphens(full)
	}
	if full, ok := r.titleToSlug[slug]; ok {
		return normalizeWikiSlugHyphens(full)
	}
	return ""
}

func transformWikiLinks(content, kbID string, pageTitles, slugToPageType map[string]string) (string, []string) {
	kbID = strings.TrimSpace(kbID)
	resolver := newWikiSlugResolver(pageTitles, slugToPageType)
	resolveSlug := resolver.resolve
	seen := map[string]bool{}
	var outlinks []string
	track := func(slug string) {
		slug = resolveSlug(slug)
		// Emit the canonical hyphen full-slug so outlinks_kwd and slug_kwd
		// always agree in format (guards against a resolved slug that still
		// carries underscores, e.g. from an old existing page).
		if slug != "" {
			slug = normalizeWikiSlugHyphens(slug)
		}
		if slug == "" || seen[slug] {
			return
		}
		seen[slug] = true
		outlinks = append(outlinks, slug)
	}
	displayText := func(label, slug string) string {
		label = strings.TrimSpace(label)
		if label != slug && label != slugify(lastPathSlug(slug)) {
			return label
		}
		if title := strings.TrimSpace(pageTitles[slug]); title != "" {
			return title
		}
		readable := strings.ReplaceAll(lastPathSlug(slug), "-", " ")
		readable = strings.ReplaceAll(readable, "_", " ")
		readable = strings.TrimSpace(readable)
		if readable == "" {
			return label
		}
		return strings.Title(readable)
	}
	artifactSlug := func(href string) string {
		parsed, err := url.Parse(href)
		if err != nil {
			return ""
		}
		path := parsed.Path
		if path == "" && parsed.Opaque != "" {
			path = parsed.Opaque
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 2 && parts[0] == "artifact" && parts[1] == kbID {
			return strings.Join(parts[2:], "/")
		}
		if len(parts) >= 2 && parts[0] == kbID {
			return strings.Join(parts[1:], "/")
		}
		return ""
	}
	// link renders artifact/<kb>/<page_type>/<slug> so the frontend wiki-link
	// parser (artifact/<kb>/<page_type>/<slug>, page_type in {entity,concept})
	// can resolve and navigate to the target page. Slugs may already carry a
	// <page_type>/<name> prefix, in which case that prefix is reused as the
	// page_type and the name is emitted as the bare slug.
	link := func(label, slug string) string {
		fullSlug := resolveSlug(slug)
		if fullSlug == "" {
			// Unresolved (dead) slug: keep the label as plain text so the
			// frontend does not render a broken artifact link.
			return strings.TrimSpace(label)
		}
		bareSlug := fullSlug
		pageType := pageTypeOf(fullSlug, slugToPageType)
		if idx := strings.Index(fullSlug, "/"); idx >= 0 && fullSlug[:idx] == pageType {
			bareSlug = fullSlug[idx+1:]
		}
		if pageType == "" {
			pageType = "page"
		}
		return "[" + label + "](artifact/" + kbID + "/" + pageType + "/" + bareSlug + ")"
	}
	rewriteMD := func(label, href string) string {
		slug := artifactSlug(href)
		if slug == "" {
			return "[" + label + "](" + href + ")"
		}
		track(slug)
		return link(displayText(label, slug), slug)
	}
	out := wikiArtifactMarkdownLink.ReplaceAllStringFunc(content, func(match string) string {
		sub := wikiArtifactMarkdownLink.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		return rewriteMD(sub[1], sub[2])
	})
	out = wikiWikilinkPipeRe.ReplaceAllStringFunc(out, func(match string) string {
		sub := wikiWikilinkPipeRe.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		slug := strings.TrimSpace(sub[1])
		track(slug)
		return link(sub[2], slug)
	})
	out = wikiWikilinkSimpleRe.ReplaceAllStringFunc(out, func(match string) string {
		sub := wikiWikilinkSimpleRe.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		slug := strings.TrimSpace(sub[1])
		track(slug)
		return link(displayText(slug, slug), slug)
	})
	// rawLinks counts the model-authored wikilinks in the source content. It
	// must be counted against content, not out: by this point every [[...]] has
	// been rewritten to Markdown, so counting on out would always report zero.
	rawLinks := wikiWikilinkSimpleRe.FindAllString(content, -1)
	appcommon.Info("knowledge_compiler: transformWikiLinks resolved outlinks",
		zap.Int("raw_wikilinks", len(rawLinks)),
		zap.Int("resolved_outlinks", len(outlinks)),
		zap.Strings("sample_resolved", firstN(outlinks, 5)),
		zap.Strings("sample_raw", firstN(rawLinks, 5)))
	return out, outlinks
}

// formatWikiRelatedKB renders the RelatedKB page list for the refine-writer
// prompt (O1): one "- [[<slug>]]" bullet per related full-slug, with the page
// title as display text when known. It is a prompt hint only — the authoritative
// See-also section is appended deterministically by appendWikiSeeAlso after the
// LLM output is transformed.
func formatWikiRelatedKB(related []string, pageTitles map[string]string) string {
	if len(related) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, slug := range related {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		canon := normalizeWikiSlugHyphens(slug)
		if title := strings.TrimSpace(pageTitles[canon]); title != "" {
			b.WriteString("- [[" + canon + "|" + title + "]]\n")
		} else {
			b.WriteString("- [[" + canon + "]]\n")
		}
	}
	if b.Len() == 0 {
		return "(none)"
	}
	return strings.TrimRight(b.String(), "\n")
}

// appendWikiSeeAlso guarantees a cross-link section from RelatedKB (O2). It runs
// AFTER transformWikiLinks, which drops dead links (targets that cannot be
// resolved). To avoid losing every RelatedKB edge when a target is missing, we do
// NOT re-run the resolver here: each RelatedKB full-slug is linked once as
// artifact/<kb>/<page_type>/<slug>, only if it is not already present in
// outlinks. This bypasses the dead-link drop and keeps the graph edge.
func appendWikiSeeAlso(content string, outlinks, related []string, kbID string, pageTitles, slugToPageType map[string]string) (string, []string) {
	if len(related) == 0 {
		return content, outlinks
	}
	resolver := newWikiSlugResolver(pageTitles, slugToPageType)
	have := make(map[string]bool, len(outlinks))
	for _, o := range outlinks {
		have[normalizeWikiSlugHyphens(o)] = true
	}
	var bullets []string
	for _, slug := range related {
		// Resolve the RelatedKB target against the current page index. An
		// unresolved target is not a known page: omit it entirely (no broken
		// artifact link) and do NOT append it to outlinks, so no phantom graph
		// edge is persisted for a page that was never compiled.
		canon := resolver.resolve(slug)
		if canon == "" || have[canon] {
			continue
		}
		have[canon] = true
		bareSlug := canon
		pageType := pageTypeOf(canon, slugToPageType)
		if idx := strings.Index(canon, "/"); idx >= 0 && canon[:idx] == pageType {
			bareSlug = canon[idx+1:]
		}
		if pageType == "" {
			pageType = "page"
		}
		label := pageTitles[canon]
		if label == "" {
			label = strings.Title(strings.ReplaceAll(lastPathSlug(canon), "-", " "))
		}
		bullets = append(bullets, "- ["+label+"](artifact/"+kbID+"/"+pageType+"/"+bareSlug+")")
		outlinks = append(outlinks, canon)
	}
	if len(bullets) == 0 {
		return content, outlinks
	}
	// Append a "See also" section if one is not already present; otherwise add
	// the bullets under the existing "See also" heading.
	body := strings.TrimRight(content, "\n")
	if seeAlsoRe.MatchString(body) {
		return body + "\n" + strings.Join(bullets, "\n") + "\n", outlinks
	}
	return body + "\n\n## See also\n\n" + strings.Join(bullets, "\n") + "\n", outlinks
}

// seeAlsoRe matches a Markdown "See also" heading (case-insensitive) so we can
// append related links under an existing section instead of duplicating it.
var seeAlsoRe = regexp.MustCompile(`(?m)^#{1,6}\s*see\s*also\s*$`)

func firstN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func lastPathSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	if idx := strings.LastIndex(slug, "/"); idx >= 0 && idx < len(slug)-1 {
		return slug[idx+1:]
	}
	return slug
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func mustPrettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func renderWikiTemplate(tmpl string, values map[string]string) string {
	out := tmpl
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = strings.ReplaceAll(out, "{"+k+"}", values[k])
	}
	return out
}

func prefixWithDash(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "- "+item)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func mergeStrings(a, b []string) []string {
	return uniqueStrings(append(append([]string{}, a...), b...))
}

func normalizedStringSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if key := normKey(item); key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func intersectsNormalized(needle map[string]struct{}, haystack []string) bool {
	if len(needle) == 0 {
		return false
	}
	for _, item := range haystack {
		if _, ok := needle[normKey(item)]; ok {
			return true
		}
	}
	return false
}

func normalizedOverlapCount(needle map[string]struct{}, haystack []string) int {
	if len(needle) == 0 {
		return 0
	}
	count := 0
	seen := map[string]struct{}{}
	for _, item := range haystack {
		key := normKey(item)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := needle[key]; ok {
			count++
		}
	}
	return count
}

func normKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func firstString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func (e wikiExtract) sourceChunkIDs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, item := range e.Entities {
		for _, id := range item.SourceChunkIDs {
			add(id)
		}
	}
	for _, item := range e.Concepts {
		for _, id := range item.SourceChunkIDs {
			add(id)
		}
	}
	for _, item := range e.Claims {
		for _, id := range item.SourceChunkIDs {
			add(id)
		}
	}
	for _, item := range e.Relations {
		for _, id := range item.SourceChunkIDs {
			add(id)
		}
	}
	for _, item := range e.Topics {
		for _, id := range item.SourceChunkIDs {
			add(id)
		}
	}
	return out
}

func (e wikiExtract) asPlanPayload() map[string]any {
	return map[string]any{
		"entities":  e.Entities,
		"concepts":  e.Concepts,
		"claims":    e.Claims,
		"relations": e.Relations,
		"topics":    e.Topics,
	}
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(bytes.TrimSpace(b))
}

func (p *wikiPipeline) maybeSourceIDs() []string {
	if ids := p.reduced.sourceChunkIDs(); len(ids) > 0 {
		return ids
	}
	seen := map[string]bool{}
	var out []string
	for _, ch := range p.inputs.Chunks {
		id := strings.TrimSpace(ch.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// dedupHistorical drops products that are near-duplicates of existing historical
// artifacts, implementing cross-run dedup for the wiki variant. This remains a
// read-only historical lookup and does not store any wiki intermediate state.
func dedupHistorical(ctx context.Context, deps common.Deps, param common.Param, tenantID, datasetID string, override []common.Candidate, products []common.Product) (survivors []common.Product, dropped int, err error) {
	if len(products) == 0 {
		return products, 0, nil
	}
	threshold := param.SimilarityThreshold

	if len(override) > 0 {
		for _, p := range products {
			if nearDuplicateOf(p.Vector, override, threshold) {
				dropped++
				continue
			}
			survivors = append(survivors, p)
		}
		return survivors, dropped, nil
	}

	if deps.HistoricalKNN == nil {
		return products, 0, nil
	}
	for _, p := range products {
		hits, e := deps.HistoricalKNN.TopKHistory(ctx, tenantID, datasetID, string(common.VariantWiki), p.Vector, wikiHistoricalK, threshold)
		if e != nil {
			return survivors, dropped, e
		}
		if len(hits) > 0 {
			dropped++
			continue
		}
		survivors = append(survivors, p)
	}
	return survivors, dropped, nil
}

func nearDuplicateOf(vec []float32, cands []common.Candidate, threshold float64) bool {
	qn := l2Norm32(vec)
	if qn == 0 {
		qn = 1
	}
	for _, c := range cands {
		cn := l2Norm32(c.Vector)
		if cn == 0 {
			cn = 1
		}
		var dot float64
		for i := 0; i < len(vec) && i < len(c.Vector); i++ {
			dot += float64(vec[i]) * float64(c.Vector[i])
		}
		if dot/(qn*cn) >= threshold {
			return true
		}
	}
	return false
}

func l2Norm32(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func cosine32(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	na := l2Norm32(a)
	nb := l2Norm32(b)
	if na == 0 || nb == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot / (na * nb)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// wikiHistoricalK is the K used for the historical-dedup KNN lookup.
const wikiHistoricalK = 5

// wikiPlanTokenBudget caps one planning round's approximate token load.
const wikiPlanTokenBudget = 3500
