package wiki

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestWikiTargetPageCount_Clamp(t *testing.T) {
	cases := []struct {
		total int
		want  int
	}{
		{0, 8},    // default floor
		{1, 8},    // below floor
		{24, 8},   // 24//3 = 8
		{60, 20},  // 60//3 = 20
		{180, 60}, // 180//3 = 60 (cap)
		{500, 60}, // above cap
	}
	for _, c := range cases {
		if got := wikiTargetPageCount(c.total); got != c.want {
			t.Fatalf("wikiTargetPageCount(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

// TestDeriveWikiPlanBudget_MaxReflectsOutputCapacity locks the corrected P0
// contract: Max is the unbreakable output-capacity bound and is NOT raised back
// up to Target. A small-window model must never be asked for more pages than its
// output capacity permits.
func TestDeriveWikiPlanBudget_MaxReflectsOutputCapacity(t *testing.T) {
	// Tiny window (modelLen=1024): output_tokens = max(1024, 1024*0.4=409) =
	// 1024; capacity = (1024-256)//48 = 16. For a large item count
	// (target=60), Max must stay at 16 (capacity-bound), NOT be raised to 60.
	b := deriveWikiPlanBudget(1024, 1000)
	if b.Target != 60 {
		t.Fatalf("Target = %d, want 60", b.Target)
	}
	if b.Max != 16 {
		t.Fatalf("Max = %d, want 16 (capacity-bound, must not re-raise to Target 60)", b.Max)
	}
	// A tiny item count with the same window: target = 8, max = min(16, 16, 16)
	// = 16.
	b = deriveWikiPlanBudget(1024, 1)
	if b.Max != 16 {
		t.Fatalf("Max = %d, want 16", b.Max)
	}
	// A roomy window: Max = min(capacity, target+8, target*2). For total=1000
	// (target 60) and window 8192: output=3276, capacity=62 -> max=min(62,68,120)=62.
	b = deriveWikiPlanBudget(8192, 1000)
	if b.Max != 62 {
		t.Fatalf("Max = %d, want 62", b.Max)
	}
}

func TestDeriveWikiPlanBudget_OutputCapacityBounds(t *testing.T) {
	// With a 8192 model: output_tokens = min(4096, max(1024, 8192*0.4=3276))
	// = 3276; capacity = (3276-256)//48 = 62. For total=1000 target=60,
	// max = min(62, max(68, 120)) = 62. Max must equal 62 and be >= target 60.
	b := deriveWikiPlanBudget(8192, 1000)
	if b.Target != 60 {
		t.Fatalf("Target = %d, want 60", b.Target)
	}
	want := 62
	if b.Max != want {
		t.Fatalf("Max = %d, want %d (output-token capacity)", b.Max, want)
	}
}

func TestAllocatePlanQuotas_SumsToTarget(t *testing.T) {
	batches := []wikiExtract{
		{Entities: make([]wikiEntity, 5)},
		{Concepts: make([]wikiConcept, 5)},
		{Claims: make([]wikiClaim, 5)},
	}
	quotas := allocatePlanQuotas(batches, 10)
	sum := 0
	for _, q := range quotas {
		sum += q
	}
	if sum != 10 {
		t.Fatalf("quota sum = %d, want 10 (got %v)", sum, quotas)
	}
	if len(quotas) != 3 {
		t.Fatalf("len(quotas) = %d, want 3", len(quotas))
	}
}

func TestAllocatePlanQuotas_LargestRemainderOrdered(t *testing.T) {
	// 7 items in batch0, 3 in batch1, target=10:
	// floors: 7 and 3; remainders 0 and 0 -> [7,3].
	batches := []wikiExtract{
		{Entities: make([]wikiEntity, 7)},
		{Concepts: make([]wikiConcept, 3)},
	}
	quotas := allocatePlanQuotas(batches, 10)
	if quotas[0] != 7 || quotas[1] != 3 {
		t.Fatalf("quotas = %v, want [7 3]", quotas)
	}

	// 7,2,1 target=10: floors 7,2,1 rem=0 -> [7,2,1].
	batches = []wikiExtract{
		{Entities: make([]wikiEntity, 7)},
		{Concepts: make([]wikiConcept, 2)},
		{Claims: make([]wikiClaim, 1)},
	}
	quotas = allocatePlanQuotas(batches, 10)
	if quotas[0] != 7 || quotas[1] != 2 || quotas[2] != 1 {
		t.Fatalf("quotas = %v, want [7 2 1]", quotas)
	}
}

func TestAllocatePlanQuotas_ZeroForOverflowingBatches(t *testing.T) {
	// More batches than target: some batches must get a zero quota and none may
	// exceed the target.
	target := 4
	batches := make([]wikiExtract, 8)
	for i := range batches {
		batches[i] = wikiExtract{Entities: []wikiEntity{{Name: "e"}}}
	}
	quotas := allocatePlanQuotas(batches, target)
	sum := 0
	zero := 0
	for _, q := range quotas {
		sum += q
		if q == 0 {
			zero++
		}
	}
	if sum != target {
		t.Fatalf("quota sum = %d, want %d", sum, target)
	}
	if zero == 0 {
		t.Fatalf("expected at least one zero quota with %d batches > target %d", len(batches), target)
	}
	for _, q := range quotas {
		if q > target {
			t.Fatalf("quota %d exceeds target %d", q, target)
		}
	}
}

func TestTruncatePlanPagesByCap_SelectsByMentionCount(t *testing.T) {
	reduced := wikiExtract{
		Entities: []wikiEntity{
			{Name: "High", SourceChunkIDs: []string{"a", "b", "c", "d"}},
			{Name: "Low", SourceChunkIDs: []string{"a"}},
		},
	}
	pages := []wikiPlanPage{
		{Slug: "entity/low", Title: "Low", EntityNames: []string{"Low"}, Priority: 1},
		{Slug: "entity/high", Title: "High", EntityNames: []string{"High"}, Priority: 2},
	}
	kept, excluded := truncatePlanPagesByCap(pages, 1, reduced)
	if excluded != 1 {
		t.Fatalf("excluded = %d, want 1", excluded)
	}
	if len(kept) != 1 || kept[0].Slug != "entity/high" {
		t.Fatalf("kept = %#v, want entity/high", kept)
	}
}

func TestTruncatePlanPagesByCap_NoCapNoDrop(t *testing.T) {
	pages := []wikiPlanPage{
		{Slug: "a", Priority: 1},
		{Slug: "b", Priority: 2},
	}
	kept, excluded := truncatePlanPagesByCap(pages, 5, wikiExtract{})
	if excluded != 0 || len(kept) != 2 {
		t.Fatalf("got kept=%d excluded=%d, want 2/0", len(kept), excluded)
	}
}

func TestTruncatePlanPagesByCap_PreservesInputOrder(t *testing.T) {
	reduced := wikiExtract{
		Entities: []wikiEntity{
			{Name: "X", SourceChunkIDs: []string{"a"}},
			{Name: "Y", SourceChunkIDs: []string{"a", "b"}},
		},
	}
	// Cap is large enough to keep everything; input order must be preserved.
	pages := []wikiPlanPage{
		{Slug: "z", Title: "Z", EntityNames: []string{"X"}, Priority: 2},
		{Slug: "a", Title: "A", EntityNames: []string{"Y"}, Priority: 1},
	}
	kept, _ := truncatePlanPagesByCap(pages, 5, reduced)
	if len(kept) != 2 || kept[0].Slug != "z" || kept[1].Slug != "a" {
		t.Fatalf("kept = %#v, want input order [z a]", kept)
	}
}

// TestRunPlan_PromptMaxPagesNeverExceedsCap locks the capacity-limited quota
// fix: when the model's output capacity is smaller than the item-derived target
// (e.g. ModelContextLen=1024, target 60, Max 16), the sum of the per-batch
// "at most N page entries" values placed in the planner prompts must never
// exceed Max. This prevents the truncated-JSON risk from re-appearing.
func TestRunPlan_PromptMaxPagesNeverExceedsCap(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		for _, j := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := j(); err != nil {
				return err
			}
		}
		return ctx.Err()
	})

	var mu sync.Mutex
	var maxPagesSeen []int
	big := strings.Repeat("x", 5000)
	// 12 large entities each pack as their own (or small) batch, giving multiple
	// batches. total items >= 36 => target clamps to 60; ModelContextLen=1024 =>
	// output capacity 16 => Max = min(16, 68, 120) = 16 => Cap = 16.
	entities := make([]wikiEntity, 0, 12)
	for i := 0; i < 12; i++ {
		entities = append(entities, wikiEntity{Name: "Ent " + itoa(i) + big})
	}
	p := &wikiPipeline{
		ctx: context.Background(),
		deps: common.Deps{
			ModelContextLen: 1024,
			Chat: chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
				if n := extractMaxPages(req.UserPrompt); n >= 0 {
					mu.Lock()
					maxPagesSeen = append(maxPagesSeen, n)
					mu.Unlock()
				}
				return &common.ChatResponse{Content: `{"pages":[]}`}, nil
			}),
		},
		reduced: wikiExtract{Entities: entities},
		docID:   "doc-1",
	}
	if _, err := p.runPlan(); err != nil {
		t.Fatalf("runPlan err = %v", err)
	}
	if len(maxPagesSeen) == 0 {
		t.Fatalf("no planning prompt captured max_pages")
	}
	sum := 0
	for _, n := range maxPagesSeen {
		sum += n
	}
	if sum > p.planBudget.Max {
		t.Fatalf("sum of per-batch max_pages = %d, want <= Max %d (target 60)", sum, p.planBudget.Max)
	}
}

// extractMaxPages parses the "at most N page entries" instruction from a plan
// prompt, returning -1 when absent.
func extractMaxPages(prompt string) int {
	const marker = "at most "
	idx := strings.Index(prompt, marker)
	if idx < 0 {
		return -1
	}
	rest := prompt[idx+len(marker):]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 {
		return -1
	}
	n := 0
	for _, c := range rest[:j] {
		n = n*10 + int(c-'0')
	}
	return n
}

// TestMergePlanCandidates_FallbackOnlyUsesApprovedItems locks F3: the fallback
// page set is built from the approved (non-zero-quota) item set only, so items
// from skipped zero-quota batches can never leak back into the plan.
func TestMergePlanCandidates_FallbackOnlyUsesApprovedItems(t *testing.T) {
	p := &wikiPipeline{docID: "doc-1"}
	approved := wikiExtract{
		Entities: []wikiEntity{{Name: "Approved", SourceChunkIDs: []string{"c1"}}},
	}
	// All approved batches returned no pages; the merged plan must fall back to
	// approved items only.
	merged := p.mergePlanCandidates(nil, approved)
	if len(merged.Pages) == 0 {
		t.Fatalf("expected at least one fallback page")
	}
	hasApproved := false
	for _, pg := range merged.Pages {
		for _, n := range pg.EntityNames {
			if strings.Contains(n, "Skipped") {
				t.Fatalf("fallback leaked zero-quota item %q", n)
			}
			if strings.Contains(n, "Approved") {
				hasApproved = true
			}
		}
	}
	if !hasApproved {
		t.Fatalf("fallback missing approved item")
	}
}

// TestRunPlan_TruncatesToGlobalHardCap drives runPlan through a planner that
// returns more pages than the derived global max_page_count, and asserts the
// merged page list is truncated to the hard cap with the excluded count
// recorded. This is the P0 acceptance criterion that the final page count never
// exceeds max_page_count after slug dedup + global cap.
func TestRunPlan_TruncatesToGlobalHardCap(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		for _, j := range jobs {
			if err := j(); err != nil {
				return err
			}
		}
		return nil
	})

	// Planner returns 30 pages. With one entity and ModelContextLen unset,
	// target = clamp(8, 1//3, 60) = 8, and max = min(capacity=62, 16, 16) = 16.
	pages := make([]map[string]any, 0, 30)
	for i := 0; i < 30; i++ {
		pages = append(pages, map[string]any{
			"action":       "CREATE",
			"slug":         "entity/item-" + itoa(i),
			"title":        "Item " + itoa(i),
			"page_type":    "entity",
			"topic":        "Item",
			"entity_names": []any{"Entity"},
			"priority":     i + 1,
		})
	}
	payload := map[string]any{"pages": pages}
	p := &wikiPipeline{
		ctx: context.Background(),
		deps: common.Deps{
			Chat: reconcileChatStub{resp: mustJSON(payload)},
		},
		reduced: wikiExtract{
			Entities: []wikiEntity{{Name: "Entity", SourceChunkIDs: []string{"c1"}}},
		},
		docID: "doc-1",
	}
	plan, err := p.runPlan()
	if err != nil {
		t.Fatalf("runPlan err = %v", err)
	}
	if got := len(plan.Pages); got != 16 {
		t.Fatalf("plan pages = %d, want 16 (global hard cap)", got)
	}
	if got := p.planCapacityExcluded; got != 14 {
		t.Fatalf("planCapacityExcluded = %d, want 14", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// batchPlanChatStub returns one page per planning batch based on which entity
// name is present in the batch prompt. It lets a fake submitter drive each
// batch's planner call with a distinct, deterministic result.
type batchPlanChatStub struct{}

func (batchPlanChatStub) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	var title string
	switch {
	case strings.Contains(req.UserPrompt, "Alpha"):
		title = "Alpha"
	case strings.Contains(req.UserPrompt, "Beta"):
		title = "Beta"
	default:
		title = "Gamma"
	}
	return &common.ChatResponse{Content: `{"pages":[{"action":"CREATE","slug":"entity/` + slugify(title) + `","title":"` + title + `","page_type":"entity","topic":"` + title + `","entity_names":["` + title + `"],"priority":1}]}`}, nil
}

// TestRunPlan_ParallelBatchesMergeInOrder drives runPlan through a submitter
// that completes batches out of order (batch1 finishes before batch0) and
// asserts the merged plan preserves the original batch order deterministically.
// This exercises the P1 invariant that jobs write only their own index and the
// merge reads slots in order.
func TestRunPlan_ParallelBatchesMergeInOrder(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		var wg sync.WaitGroup
		for i, j := range jobs {
			i, j := i, j
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i == 0 {
					time.Sleep(30 * time.Millisecond) // batch0 completes last
				}
				j()
			}()
		}
		wg.Wait()
		return ctx.Err()
	})

	// Three entities sized so Alpha+Beta pack into batch1 and Gamma falls into
	// batch2 (token budget 3500).
	big := strings.Repeat("x", 7000)
	p := &wikiPipeline{
		ctx: context.Background(),
		deps: common.Deps{
			Chat: batchPlanChatStub{},
		},
		reduced: wikiExtract{
			Entities: []wikiEntity{
				{Name: "Alpha" + big},
				{Name: "Beta"},
				{Name: "Gamma" + big},
			},
		},
		docID: "doc-1",
	}
	plan, err := p.runPlan()
	if err != nil {
		t.Fatalf("runPlan err = %v", err)
	}
	// Batch1 (Alpha) must appear before batch2 (Gamma) in the merged plan.
	if len(plan.Pages) < 2 {
		t.Fatalf("plan pages = %d, want >= 2", len(plan.Pages))
	}
	if plan.Pages[0].Title != "Alpha" {
		t.Fatalf("merged pages[0].Title = %q, want Alpha (batch order preserved)", plan.Pages[0].Title)
	}
	if plan.Pages[1].Title != "Gamma" {
		t.Fatalf("merged pages[1].Title = %q, want Gamma", plan.Pages[1].Title)
	}
}

// TestRunPlan_ParallelBatchesFirstError verifies the P1 error model: the first
// batch error is returned after all submitted jobs settle.
func TestRunPlan_ParallelBatchesFirstError(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		var wg sync.WaitGroup
		errs := make(chan error, len(jobs))
		for _, j := range jobs {
			j := j
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- j()
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return err
			}
		}
		return ctx.Err()
	})

	big := strings.Repeat("x", 7000)
	boom := errors.New("planning failed")
	p := &wikiPipeline{
		ctx: context.Background(),
		deps: common.Deps{
			Chat: failPlanChatStub{err: boom},
		},
		reduced: wikiExtract{
			Entities: []wikiEntity{
				{Name: "Alpha" + big},
				{Name: "Beta"},
				{Name: "Gamma" + big},
			},
		},
		docID: "doc-1",
	}
	if _, err := p.runPlan(); err != boom {
		t.Fatalf("runPlan err = %v, want boom", err)
	}
}

// failPlanChatStub fails every planning call with a fixed error.
type failPlanChatStub struct {
	err error
}

func (f failPlanChatStub) Chat(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
	return nil, f.err
}

// TestRunPlan_CancelledCtxAborts verifies that a cancelled context aborts the
// planning fan-out and surfaces the context error.
func TestRunPlan_CancelledCtxAborts(t *testing.T) {
	previous := batchSubmitter
	defer SetBatchSubmitter(previous)

	SetBatchSubmitter(func(ctx context.Context, jobs []func() error) error {
		for _, j := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := j(); err != nil {
				return err
			}
		}
		return ctx.Err()
	})

	big := strings.Repeat("x", 7000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &wikiPipeline{
		ctx: ctx,
		deps: common.Deps{
			Chat: batchPlanChatStub{},
		},
		reduced: wikiExtract{
			Entities: []wikiEntity{
				{Name: "Alpha" + big},
				{Name: "Beta"},
				{Name: "Gamma" + big},
			},
		},
		docID: "doc-1",
	}
	if _, err := p.runPlan(); err == nil {
		t.Fatalf("runPlan err = nil, want context cancelled")
	}
}
