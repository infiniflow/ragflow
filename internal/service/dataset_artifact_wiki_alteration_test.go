package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// fakeWikiSearchEngine is a DocEngine stub that only implements Search; the
// other interface methods are inherited from the embedded nil interface and
// would panic if called, which is fine because GetWikiAlteration only searches.
type fakeWikiSearchEngine struct {
	engine.DocEngine
	search func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
}

func (e fakeWikiSearchEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return e.search(ctx, req)
}

// queryProbe records the SQL of executed queries and counts those touching the
// user_canvas / document tables. It is the guard that proves the over-read and
// N+1 fixes actually changed the generated SQL.
type queryProbe struct {
	mu        sync.Mutex
	canvasSQL []string
	docSQL    []string
}

func (p *queryProbe) addCanvas(sql string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.canvasSQL = append(p.canvasSQL, sql)
}

func (p *queryProbe) addDoc(sql string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.docSQL = append(p.docSQL, sql)
}

func (p *queryProbe) canvasCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.canvasSQL)
}

func (p *queryProbe) docCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.docSQL)
}

func installQueryProbe(t *testing.T, db *gorm.DB) *queryProbe {
	t.Helper()
	p := &queryProbe{}
	name := "wiki_probe_" + t.Name()
	// Registering After("gorm:query") guarantees c.Statement.SQL is populated.
	if err := db.Callback().Query().After("gorm:query").Register(name, func(c *gorm.DB) {
		sql := strings.ToLower(c.Statement.SQL.String())
		if strings.Contains(sql, "user_canvas") {
			p.addCanvas(sql)
		}
		if strings.Contains(sql, "`document`") || strings.Contains(sql, "from document") {
			p.addDoc(sql)
		}
	}); err != nil {
		t.Fatalf("install query probe: %v", err)
	}
	return p
}

// newWikiAlterationTest wires an in-memory DB (with CompilationTemplate
// migrated) and a controllable fake search engine, returning the service and a
// query probe.
func newWikiAlterationTest(t *testing.T) (*DatasetArtifactService, *queryProbe) {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.CompilationTemplate{}); err != nil {
		t.Fatalf("migrate compilation_template: %v", err)
	}
	pushServiceDB(t, db)
	probe := installQueryProbe(t, db)
	svc := NewDatasetArtifactService()
	// By default the search returns no wiki pages, so the "involved" set is
	// empty and removed/newly-uploaded are driven only by eligibility.
	svc.docEngine = &fakeWikiSearchEngine{search: func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
		return &types.SearchResult{Chunks: nil, Total: 0}, nil
	}}
	return svc, probe
}

// setWikiSearchInvolved makes the fake search report the given doc ids as the
// previously compiled pages (the "involved" set).
func setWikiSearchInvolved(svc *DatasetArtifactService, ids ...string) {
	chunks := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		chunks = append(chunks, map[string]interface{}{"source_doc_ids": []interface{}{id}})
	}
	svc.docEngine = &fakeWikiSearchEngine{search: func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
		return &types.SearchResult{Chunks: chunks, Total: int64(len(chunks))}, nil
	}}
}

func insertWikiTemplate(t *testing.T, id, tenantID, kind string) {
	t.Helper()
	tmpl := &entity.CompilationTemplate{
		ID:       id,
		UserID:   "u1",
		TenantID: &tenantID,
		Name:     "tmpl-" + id,
		Kind:     kind,
		Config:   entity.JSONMap{},
		Status:   sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(tmpl).Error; err != nil {
		t.Fatalf("insert wiki template %s: %v", id, err)
	}
}

func insertWikiCanvas(t *testing.T, id, tenantID, userID string, dsl entity.JSONMap) {
	t.Helper()
	c := &entity.UserCanvas{
		ID:             id,
		UserID:         userID,
		TenantID:       &tenantID,
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            dsl,
	}
	if err := dao.DB.Create(c).Error; err != nil {
		t.Fatalf("insert wiki canvas %s: %v", id, err)
	}
}

func insertWikiDoc(t *testing.T, id, kbID, tenantID, status string, pipelineID *string, cfg entity.JSONMap) {
	t.Helper()
	d := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: cfg,
		Status:       sptr(status),
		PipelineID:   pipelineID,
	}
	if err := dao.DB.Create(d).Error; err != nil {
		t.Fatalf("insert wiki doc %s: %v", id, err)
	}
}

// TestGetWikiAlteration_StatusZeroSkipped: invariant (1) — a document whose
// status is "0" must never be eligible, even when its config carries a wiki
// template. This is the distinguishing condition (status "0" vs "1").
func TestGetWikiAlteration_StatusZeroSkipped(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-wiki", tenantID, "wiki")
	insertWikiDoc(t, "d-skipped", kbID, tenantID, "0", nil, entity.JSONMap{"compilation_template_id": "tmpl-wiki"})
	insertWikiDoc(t, "d-kept", kbID, tenantID, "1", nil, entity.JSONMap{"compilation_template_id": "tmpl-wiki"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 1 || got.EligibleDocIDs[0] != "d-kept" {
		t.Fatalf("eligible = %v; want [d-kept]", got.EligibleDocIDs)
	}
}

// TestGetWikiAlteration_TemplateInConfig: invariant (2) — a wiki template in the
// document's own parser config makes it eligible.
func TestGetWikiAlteration_TemplateInConfig(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-wiki", tenantID, "wiki")
	insertWikiDoc(t, "d1", kbID, tenantID, "1", nil, entity.JSONMap{"compilation_template_id": "tmpl-wiki"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 1 || got.EligibleDocIDs[0] != "d1" {
		t.Fatalf("eligible = %v; want [d1]", got.EligibleDocIDs)
	}
}

// TestGetWikiAlteration_WrongKindNotEligible: a template of a non-wiki kind must
// not be treated as a wiki compiler template (adversarial: kind mismatch).
func TestGetWikiAlteration_WrongKindNotEligible(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-skill", tenantID, "skill")
	insertWikiDoc(t, "d1", kbID, tenantID, "1", nil, entity.JSONMap{"compilation_template_id": "tmpl-skill"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 0 {
		t.Fatalf("eligible = %v; want [] (skill template is not wiki)", got.EligibleDocIDs)
	}
}

// TestGetWikiAlteration_NoTemplateNoPipeline: invariant (3) — without a template
// in config and with an empty pipeline id, the document is not eligible. This is
// the single-field (omitted pipeline) case.
func TestGetWikiAlteration_NoTemplateNoPipeline(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiDoc(t, "d1", kbID, tenantID, "1", nil, entity.JSONMap{"chunk_strategy": "naive"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 0 {
		t.Fatalf("eligible = %v; want []", got.EligibleDocIDs)
	}
}

// TestGetWikiAlteration_PipelineCanvasMissing: invariant (4a) — config has no
// template, pipeline id is set, but the canvas row does not exist
// (ErrRecordNotFound). The document must not be eligible and no error returned.
func TestGetWikiAlteration_PipelineCanvasMissing(t *testing.T) {
	svc, probe := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiDoc(t, "d1", kbID, tenantID, "1", sptr("p-missing"), entity.JSONMap{"chunk_strategy": "naive"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 0 {
		t.Fatalf("eligible = %v; want [] (canvas missing)", got.EligibleDocIDs)
	}
	// The missing canvas is still looked up exactly once (batch path).
	if probe.canvasCount() != 1 {
		t.Fatalf("user_canvas queries = %d; want 1", probe.canvasCount())
	}
}

// TestGetWikiAlteration_TemplateInPipelineDSL: invariant (4b) — config has no
// template, but the referenced pipeline's DSL carries a wiki template, so the
// document is eligible. This is the N+1 trigger path.
func TestGetWikiAlteration_TemplateInPipelineDSL(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-wiki", tenantID, "wiki")
	insertWikiCanvas(t, "p1", tenantID, "u1", entity.JSONMap{"compilation_template_id": "tmpl-wiki"})
	insertWikiDoc(t, "d1", kbID, tenantID, "1", sptr("p1"), entity.JSONMap{"chunk_strategy": "naive"})

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != 1 || got.EligibleDocIDs[0] != "d1" {
		t.Fatalf("eligible = %v; want [d1]", got.EligibleDocIDs)
	}
}

// TestGetWikiAlteration_BatchCanvasLoadEliminatesNPlus1: the core performance
// fix. N documents sharing one pipeline id must result in exactly ONE
// user_canvas query, not N. On the pre-fix code each document triggers its own
// point query, so this assertion fails until the batch load lands.
func TestGetWikiAlteration_BatchCanvasLoadEliminatesNPlus1(t *testing.T) {
	svc, probe := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-wiki", tenantID, "wiki")
	insertWikiCanvas(t, "p-shared", tenantID, "u1", entity.JSONMap{"compilation_template_id": "tmpl-wiki"})

	const n = 5
	want := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := "d" + string(rune('a'+i))
		want = append(want, id)
		insertWikiDoc(t, id, kbID, tenantID, "1", sptr("p-shared"), entity.JSONMap{"chunk_strategy": "naive"})
	}

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	if len(got.EligibleDocIDs) != n {
		t.Fatalf("eligible = %v (len %d); want all %d docs", got.EligibleDocIDs, len(got.EligibleDocIDs), n)
	}
	if probe.canvasCount() != 1 {
		t.Fatalf("user_canvas queries = %d; want 1 (N+1 not eliminated)", probe.canvasCount())
	}
}

// TestGetWikiAlteration_PipelineLoadErrorAborts: invariant (5) — when the
// pipeline load fails (here forced on the user_canvas query), GetWikiAlteration
// must surface the error rather than silently returning partial results. This
// guards the new batch-load error path.
func TestGetWikiAlteration_PipelineLoadErrorAborts(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiDoc(t, "d1", kbID, tenantID, "1", sptr("p1"), entity.JSONMap{"chunk_strategy": "naive"})
	db := dao.DB
	if err := db.Callback().Query().Before("gorm:query").Register("wiki_force_err_"+t.Name(), func(c *gorm.DB) {
		if c.Statement != nil && c.Statement.Table == "user_canvas" {
			c.AddError(errors.New("forced canvas failure"))
		}
	}); err != nil {
		t.Fatalf("install force-err callback: %v", err)
	}

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err == nil {
		t.Fatalf("expected error from failed canvas load, got result %v", got)
	}
}

// TestGetWikiAlteration_ProjectionAvoidsOverRead: the document list query must
// project only the columns actually consumed (id, status, pipeline_id,
// parser_config) instead of SELECT *. The projection lists parser_config
// explicitly, whereas SELECT * does not, so this fails on the pre-fix code.
func TestGetWikiAlteration_ProjectionAvoidsOverRead(t *testing.T) {
	svc, probe := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiDoc(t, "d1", kbID, tenantID, "1", nil, entity.JSONMap{"chunk_strategy": "naive"})

	if _, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID); err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	var docSel string
	for _, sql := range probe.docSQL {
		if strings.Contains(sql, "parser_config") {
			docSel = sql
			break
		}
	}
	if docSel == "" {
		t.Fatalf("document list query did not project parser_config; doc queries = %v", probe.docSQL)
	}
	if strings.Contains(docSel, "*") {
		t.Fatalf("document list query is SELECT * (over-read); got %q", docSel)
	}
}

// TestGetWikiAlteration_RemovedAndNewlyUploaded: combined behavior — the
// "involved" set comes from the compiled search; eligibility comes from the
// documents. removed = involved - eligible, newlyUploaded = eligible - involved.
func TestGetWikiAlteration_RemovedAndNewlyUploaded(t *testing.T) {
	svc, _ := newWikiAlterationTest(t)
	const tenantID, kbID = "t1", "kb1"
	insertWikiTemplate(t, "tmpl-wiki", tenantID, "wiki")
	// a: was compiled, still eligible (template in config).
	insertWikiDoc(t, "a", kbID, tenantID, "1", nil, entity.JSONMap{"compilation_template_id": "tmpl-wiki"})
	// b: was compiled, no longer eligible (no template, no pipeline).
	insertWikiDoc(t, "b", kbID, tenantID, "1", nil, entity.JSONMap{"chunk_strategy": "naive"})
	// c: newly uploaded, eligible (template in config), not yet compiled.
	insertWikiDoc(t, "c", kbID, tenantID, "1", nil, entity.JSONMap{"compilation_template_id": "tmpl-wiki"})
	// involved from the compiled index = {a, b}
	setWikiSearchInvolved(svc, "a", "b")

	got, err := svc.GetWikiAlteration(t.Context(), tenantID, kbID)
	if err != nil {
		t.Fatalf("GetWikiAlteration: %v", err)
	}
	assertSameStringSet(t, got.EligibleDocIDs, []string{"a", "c"})
	assertSameStringSet(t, got.InvolvedDocIDs, []string{"a", "b"})
	assertSameStringSet(t, got.RemovedDocIDs, []string{"b"})
	assertSameStringSet(t, got.NewlyUploadedDocIDs, []string{"c"})
}

func assertSameStringSet(t *testing.T, got, want []string) {
	t.Helper()
	gs := sortedSetKeys(toSet(got))
	ws := sortedSetKeys(toSet(want))
	if len(gs) != len(ws) {
		t.Fatalf("set = %v; want %v", gs, ws)
	}
	for i := range gs {
		if gs[i] != ws[i] {
			t.Fatalf("set = %v; want %v", gs, ws)
		}
	}
}

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, it := range items {
		s[it] = struct{}{}
	}
	return s
}
