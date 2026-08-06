// Package wiki implements the "wiki" variant of KnowledgeCompiler: a
// document artifact pipeline MAP -> REDUCE -> PLAN -> REFINE that produces a
// wiki-style page (and supporting section products) with in-memory state only.
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

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
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
}

type wikiExtract struct {
	Entities  []wikiEntity   `json:"entities"`
	Concepts  []wikiConcept  `json:"concepts"`
	Claims    []wikiClaim    `json:"claims"`
	Relations []wikiRelation `json:"relations"`
	Topics    []string       `json:"topics"`
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
	MentionCount int `json:"-"`
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
	p := &wikiPipeline{
		ctx:       ctx,
		deps:      deps,
		param:     param,
		inputs:    inputs,
		docID:     docID,
		tenantID:  deps.TenantID,
		datasetID: deps.DatasetID,
		llmID:     llmID,
	}
	if err := p.run(); err != nil {
		return common.Outputs{}, err
	}

	products := buildWikiPageProducts(p.tenantID, p.docID, p.pages)
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
	}
	return out, nil
}

func (p *wikiPipeline) run() error {
	if len(p.inputs.Chunks) == 0 {
		return nil
	}
	if err := p.runMap(); err != nil {
		return err
	}
	p.reduced = reduceExtracts(p.mapExtracts)
	// Layer embedding + LLM disambiguation onto the exact-merged entities
	// (REDUCE enhancement; concepts keep exact dedup). Degrades to a no-op when
	// the embedder/chat seams are unavailable.
	p.reduced.Entities = p.dedupeEntities(p.reduced.Entities)
	plan, err := p.runPlan()
	if err != nil {
		return err
	}
	p.plan = plan
	pages, err := p.runRefine()
	if err != nil {
		return err
	}
	p.pages = pages
	return nil
}

func (p *wikiPipeline) runMap() error {
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
	return nil
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
	return parseWikiExtract(raw), nil
}

func (p *wikiPipeline) runPlan() (wikiPlan, error) {
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
			plan, err := p.runPlanBatch(batch, i+1, len(batches), quota)
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
	pages := normalizeWikiPlanPages(p.plan.Pages, p.reduced)
	if len(pages) == 0 {
		return nil, nil
	}
	pageTitles := map[string]string{}
	slugToPageType := map[string]string{}
	allPlanSlugs := make([]string, 0, len(pages))
	for _, page := range pages {
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
			return nil
		})
	}
	if err := runBatches(p.ctx, jobs); err != nil {
		return nil, err
	}
	return results, nil
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
		"existing_section": existingSection,
		"source_context":   sourceContext,
		"evidence_count":   fmt.Sprintf("%d", len(evidence)),
		"evidence_blocks":  formatWikiEvidenceBlocks(evidence),
	})
	resp, err := p.deps.Chat.Chat(p.ctx, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: buildWikiRefineWriterSystem(""),
		UserPrompt:   user,
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

func (p *wikiPipeline) runPlanBatch(batch wikiExtract, batchIndex, batchTotal, quota int) (wikiPlan, error) {
	user := renderWikiTemplate(wikiPlanBatchUserTemplate, map[string]string{
		"doc_id":      p.docID,
		"batch_index": fmt.Sprintf("%d", batchIndex),
		"batch_total": fmt.Sprintf("%d", batchTotal),
		"max_pages":   fmt.Sprintf("%d", maxPagesForBatch(quota)),
		"entities":    mustJSON(batch.Entities),
		"concepts":    mustJSON(batch.Concepts),
		"claims":      mustJSON(batch.Claims),
		"relations":   mustJSON(batch.Relations),
		"topics":      mustJSON(batch.Topics),
	})
	raw, err := common.GenJSON(p.ctx, p.deps.Chat, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiPlanSystem,
		UserPrompt:   user,
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
	merged.Pages = normalizeWikiPlanPageLinks(merged.Pages)
	return merged
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
		page.Topic = firstNonEmpty(existing.Topic, page.Topic)
		page.RelatedKB = mergeStrings(page.RelatedKB, existing.RelatedKBPages)
		page.EntityNames = mergeStrings(page.EntityNames, existing.EntityNames)
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
	cands = rerankWikiPlanCandidates(page, cands)
	targetNames := normalizedStringSet(page.EntityNames)
	pageTitle := normKey(page.Title)
	pageTopic := normKey(page.Topic)
	maybe := make([]common.WikiPageCandidate, 0, len(cands))
	for i := range cands {
		cand := cands[i]
		if cand.Score >= wikiPlanUpdateThreshold {
			return &cand, nil
		}
		if cand.Score < wikiPlanMaybeThreshold {
			continue
		}
		if normKey(cand.Title) == pageTitle || normKey(cand.Topic) == pageTopic {
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
	pageTopic := normKey(page.Topic)
	scoredCands := make([]scored, 0, len(candidates))
	for _, cand := range candidates {
		boost := cand.Score
		if pageType != "" && normKey(cand.PageType) == pageType {
			boost += 0.08
		}
		if pageTitle != "" && normKey(cand.Title) == pageTitle {
			boost += 0.12
		}
		if pageTopic != "" && normKey(cand.Topic) == pageTopic {
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
	for _, line := range strings.Split(md, "\n") {
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
	seenRelations := map[string]bool{}
	seenTopics := map[string]bool{}

	for _, ex := range extracts {
		for _, e := range ex.Entities {
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
			item.Name = strings.TrimSpace(item.Name)
			item.Type = strings.TrimSpace(item.Type)
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
			if seenRelations[key] {
				continue
			}
			r.SourceChunkIDs = uniqueStrings(r.SourceChunkIDs)
			seenRelations[key] = true
			out.Relations = append(out.Relations, r)
		}
		for _, t := range ex.Topics {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			key := normKey(t)
			if seenTopics[key] {
				continue
			}
			seenTopics[key] = true
			out.Topics = append(out.Topics, t)
		}
	}

	out.Entities = make([]wikiEntity, 0, len(entities))
	keys := make([]entityKey, 0, len(entities))
	for k := range entities {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name == keys[j].Name {
			return keys[i].Type < keys[j].Type
		}
		return keys[i].Name < keys[j].Name
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
	return out
}

func parseWikiExtract(raw map[string]any) wikiExtract {
	out := wikiExtract{}
	out.Entities = parseWikiEntities(raw["entities"])
	out.Concepts = parseWikiConcepts(raw["concepts"])
	out.Claims = parseWikiClaims(raw["claims"])
	out.Relations = parseWikiRelations(raw["relations"])
	out.Topics = parseWikiStrings(raw["topics"])
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
			plan.Title = reduced.Topics[0]
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
	existing := map[string]wikiPlanPage{}
	out := make([]wikiPlanPage, 0, len(pages))
	for _, page := range pages {
		page = normalizeWikiPlanPage(page)
		if page.Slug == "" {
			continue
		}
		if _, ok := existing[page.Slug]; ok {
			continue
		}
		existing[page.Slug] = page
		out = append(out, page)
	}
	fallback := buildWikiFallbackPages(reduced)
	if len(out) == 0 {
		for _, page := range fallback {
			if page.Slug == "" {
				continue
			}
			existing[page.Slug] = page
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

func normalizeWikiPlanPage(page wikiPlanPage) wikiPlanPage {
	page.Action = strings.ToUpper(strings.TrimSpace(page.Action))
	if page.Action == "" {
		page.Action = "CREATE"
	}
	page.Slug = strings.TrimSpace(page.Slug)
	if page.Slug == "" {
		page.Slug = slugify(page.Title)
	}
	page.Title = strings.TrimSpace(page.Title)
	if page.Title == "" {
		page.Title = page.Slug
	}
	page.PageType = strings.TrimSpace(page.PageType)
	if page.PageType == "" {
		page.PageType = "concept"
	}
	page.Topic = strings.TrimSpace(page.Topic)
	if page.Topic == "" {
		page.Topic = page.Title
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
			Slug:        "entity/" + slugify(e.Name),
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
			appendPage(wikiPlanPage{
				Action:   "CREATE",
				Slug:     "topic/" + slugify(t),
				Title:    t,
				PageType: "topic",
				Topic:    t,
				Priority: len(out) + 1,
				Lead:     t,
				Sections: []wikiPlanSection{{Heading: "Overview", Points: []string{t}}},
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
		return reduced.Topics[0]
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
		lines = append(lines, fmt.Sprintf("- topic: %s", t))
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

func transformWikiLinks(content, kbID string, pageTitles, slugToPageType map[string]string) (string, []string) {
	kbID = strings.TrimSpace(kbID)
	seen := map[string]bool{}
	var outlinks []string
	track := func(slug string) {
		slug = strings.TrimSpace(slug)
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
		bareSlug := slug
		pageType := pageTypeOf(slug, slugToPageType)
		if idx := strings.Index(slug, "/"); idx >= 0 && slug[:idx] == pageType {
			bareSlug = slug[idx+1:]
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
	return out, outlinks
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
