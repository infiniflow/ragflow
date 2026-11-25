package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	kc "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/knowledge_compile"
)

// The engine double replaces persistence only; replacement, scope checks,
// parent/section reconciliation and active-state writes use production code.
type dirtyWikiReconcileEngine struct {
	engine.DocEngine
	t                *testing.T
	rows             map[string]map[string]any
	writes, searches int
	total            int64
	beforeSearch     func()
	partialDeleteID  string
}

func (e *dirtyWikiReconcileEngine) GetType() string { return "elasticsearch" }
func (e *dirtyWikiReconcileEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searches++
	if e.beforeSearch != nil {
		e.beforeSearch()
	}
	if !reflect.DeepEqual(req.IndexNames, []string{"ragflow_tenant"}) || !reflect.DeepEqual(req.KbIDs, []string{"dataset"}) || !reflect.DeepEqual(req.Filter["doc_id"], []string{"document"}) || (req.Filter["available_int"] != 0 && req.Filter["available_int"] != 1) {
		e.t.Fatalf("unscoped Wiki reconciliation search: %+v", req)
	}
	var rows []map[string]any
	for _, row := range e.rows {
		if row["doc_id"] == "document" && row["kb_id"] == "dataset" && row["available_int"] == req.Filter["available_int"] && (req.Filter["available_int"] == 1 || row["type_kwd"] == "wiki_page" || row["type_kwd"] == "wiki_section") {
			selected := make(map[string]any, len(req.SelectFields))
			for _, field := range req.SelectFields {
				if value, ok := row[field]; ok {
					selected[field] = value
				}
			}
			rows = append(rows, selected)
		}
	}
	total := int64(len(rows))
	if e.total > total {
		total = e.total
	}
	return &types.SearchResult{Chunks: rows, Total: total}, nil
}
func (e *dirtyWikiReconcileEngine) InsertChunks(_ context.Context, rows []map[string]any, index, dataset, _ string) ([]string, error) {
	if index != "ragflow_tenant" || dataset != "dataset" {
		e.t.Fatalf("unscoped insert: %s %s", index, dataset)
	}
	e.writes++
	for _, row := range rows {
		e.rows[anyString(row["id"])] = row
	}
	return nil, nil
}
func (e *dirtyWikiReconcileEngine) DeleteChunks(_ context.Context, condition map[string]any, index, dataset string) (int64, error) {
	if index != "ragflow_tenant" || dataset != "dataset" || condition["kb_id"] != "dataset" {
		e.t.Fatalf("unscoped delete: %s %s %v", index, dataset, condition)
	}
	e.writes++
	ids, byID := condition["id"].([]string)
	if !byID {
		if condition["type_kwd"] != "wiki_map_active" || condition["available_int"] != 0 || !reflect.DeepEqual(condition["source_doc_ids"], []string{"document"}) {
			e.t.Fatalf("unexpected active-state delete: %v", condition)
		}
		for id, row := range e.rows {
			if row["kb_id"] == "dataset" && row["type_kwd"] == "wiki_map_active" && row["available_int"] == 0 && reflect.DeepEqual(row["source_doc_ids"], []string{"document"}) {
				ids = append(ids, id)
			}
		}
	}
	if e.partialDeleteID != "" {
		for _, id := range ids {
			if id == e.partialDeleteID {
				delete(e.rows, id)
				e.partialDeleteID = ""
				return 1, errors.New("partial delete after deleting page")
			}
		}
		e.t.Fatal("partial-delete fixture did not select its obsolete page")
	}
	for _, id := range ids {
		delete(e.rows, id)
	}
	return int64(len(ids)), nil
}

type dirtyWikiReconcilePublisher struct {
	calls   int
	publish func() error
}

func (*dirtyWikiReconcilePublisher) Provision(context.Context) error { return nil }
func (p *dirtyWikiReconcilePublisher) Publish(context.Context, string, string, string, string, []string, []string) error {
	p.calls++
	if p.publish != nil {
		return p.publish()
	}
	return nil
}

func dirtyWikiReconcileSetup(t *testing.T, fileDB ...bool) (*dirtyWikiReconcileEngine, *dirtyWikiReconcilePublisher, knowledge_compile.WikiDirtyRequest) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	if len(fileDB) > 0 && fileDB[0] {
		dsn = filepath.Join(t.TempDir(), "wiki.db") + "?_pragma=busy_timeout(0)"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(&entity.WikiDocumentDirty{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.WikiDocumentDirty{DocumentID: "document", DatasetID: "dataset", TenantID: "tenant", Revision: 2}).Error; err != nil {
		t.Fatal(err)
	}
	previous := kcWiringDeps
	previousPublisher := knowledge_compile.DefaultPublisher()
	eng := &dirtyWikiReconcileEngine{t: t, rows: map[string]map[string]any{}}
	publisher := &dirtyWikiReconcilePublisher{}
	InitKnowledgeCompilerDeps(db, eng)
	knowledge_compile.SetScheduler(publisher)
	t.Cleanup(func() { kcWiringDeps = previous; knowledge_compile.SetScheduler(previousPublisher) })
	return eng, publisher, knowledge_compile.WikiDirtyRequest{TenantID: "tenant", DatasetID: "dataset", DocumentID: "document", Revision: 2}
}

func dirtyWikiReconcilePage(id, slug string) map[string]any {
	return map[string]any{"id": id, "slug_kwd": slug, "type_kwd": "wiki_page", "compile_kwd": "wiki", "doc_id": "document", "kb_id": "dataset", "available_int": 0}
}
func dirtyWikiReconcileSection(id, parent string) map[string]any {
	return map[string]any{"id": id, "parent_kwd": parent, "type_kwd": "wiki_section", "compile_kwd": "wiki", "doc_id": "document", "kb_id": "dataset", "available_int": 0}
}
func dirtyWikiReconcileState(t *testing.T, key string, slugs ...string) kc.WikiMapActiveState {
	t.Helper()
	plan := make([]map[string]string, 0, len(slugs))
	for _, slug := range slugs {
		plan = append(plan, map[string]string{"slug": slug})
	}
	payload, err := json.Marshal(map[string]any{"plan": plan, "chunks": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	return kc.WikiMapActiveState{Key: key, TenantID: "tenant", DatasetID: "dataset", DocumentID: "document", Payload: payload}
}

type dirtyWikiMAPCapture struct {
	requests []kc.ChatRequest
}

func (c *dirtyWikiMAPCapture) Chat(_ context.Context, request kc.ChatRequest) (*kc.ChatResponse, error) {
	c.requests = append(c.requests, request)
	return nil, context.Canceled
}

func TestDirtyWikiStoredSourceTextReachesMAPRequest(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	if err := kcWiringDeps.DB.AutoMigrate(&entity.Document{}, &entity.CompilationTemplate{}); err != nil {
		t.Fatal(err)
	}
	doc := entity.Document{ID: "document", KbID: "dataset", ParserConfig: entity.JSONMap{"compilation_template_id": "template"}}
	if err := kcWiringDeps.DB.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}
	template := entity.CompilationTemplate{ID: "template", TenantID: &request.TenantID, Name: "Wiki", Kind: "wiki", Config: entity.JSONMap{}}
	if err := kcWiringDeps.DB.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	body := "温度传感器 T-17 由 Chen 校准，阈值为 26°C。"
	eng.rows["source"] = map[string]any{"id": "source", "doc_id": "document", "kb_id": "dataset", "available_int": 1, "docnm_kwd": "manual", "content_with_weight": body}
	eng.rows["compiled"] = map[string]any{"id": "compiled", "doc_id": "document", "kb_id": "dataset", "available_int": 1, "compile_kwd": "wiki", "content_with_weight": "exclude compiled rows"}
	foreign := copyStringAnyMap(eng.rows["source"])
	foreign["id"], foreign["doc_id"], foreign["content_with_weight"] = "foreign-source", "another-document", "exclude foreign rows"
	eng.rows["foreign-source"] = foreign
	chat := &dirtyWikiMAPCapture{}
	kc.SetDepsResolver(func(context.Context, string, string, string) (kc.Deps, error) {
		return kc.Deps{Chat: chat}, nil
	})
	t.Cleanup(func() { kc.SetDepsResolver(newKnowledgeCompilerDepsResolver()) })
	// This follows the dirty-document loader, production component conversion,
	// batching and prompt rendering, stopping only at the model transport seam.
	if err := compileDirtyWikiDocument(t.Context(), request); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected to stop after capturing MAP request, got %v", err)
	}
	if len(chat.requests) != 1 {
		t.Fatalf("MAP request count = %d, want 1", len(chat.requests))
	}
	prompt := chat.requests[0].UserPrompt
	if !strings.Contains(prompt, body) || !strings.Contains(prompt, "source") {
		t.Errorf("stored source body or id missing from MAP request: %q", prompt)
	}
	for _, excluded := range []string{"exclude compiled rows", "exclude foreign rows"} {
		if strings.Contains(prompt, excluded) {
			t.Errorf("MAP request included %q", excluded)
		}
	}
	if eng.writes != 0 || publisher.calls != 0 {
		t.Fatalf("captured MAP failure wrote=%d published=%d", eng.writes, publisher.calls)
	}
}

func TestDirtyWikiRetryRemovesOrphanSectionsAfterPartialDelete(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["old"] = dirtyWikiReconcilePage("old", "entity/old")
	eng.rows["old-section"] = dirtyWikiReconcileSection("old-section", "old")
	eng.rows["other"] = dirtyWikiReconcilePage("other", "entity/other-template")
	eng.rows["other-section"] = dirtyWikiReconcileSection("other-section", "other")
	foreign := dirtyWikiReconcileSection("foreign-section", "foreign-page")
	foreign["doc_id"] = "another-document"
	eng.rows["foreign-section"] = foreign
	states := []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "changed-template"), dirtyWikiReconcileState(t, "other-template", "entity/other-template")}
	eng.partialDeleteID = "old"
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, states, false); err == nil || !strings.Contains(err.Error(), "partial delete") {
		t.Fatalf("first attempt error = %v, want partial deletion", err)
	}
	if eng.rows["old"] != nil || eng.rows["old-section"] == nil || publisher.calls != 0 {
		t.Fatal("partial delete did not persist page-only deletion without publication")
	}
	for retry := 0; retry < 2; retry++ {
		if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, states, false); err != nil {
			t.Fatal(err)
		}
		if eng.rows["old-section"] != nil {
			t.Error("retry left the deleted page's orphan section")
		}
		for _, id := range []string{"other", "other-section", "foreign-section"} {
			if eng.rows[id] == nil {
				t.Errorf("retry removed retained or foreign product %s", id)
			}
		}
	}
	if publisher.calls != 2 {
		t.Fatalf("completion calls = %d, want two successful retry completions", publisher.calls)
	}
}

func TestDirtyWikiRecreatedParentReplacesOnlyOldSections(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["old-section"] = dirtyWikiReconcileSection("old-section", "recreated")
	eng.rows["other"] = dirtyWikiReconcilePage("other", "entity/other-template")
	eng.rows["other-section"] = dirtyWikiReconcileSection("other-section", "other")
	rows := []map[string]any{dirtyWikiReconcilePage("recreated", "entity/recreated"), dirtyWikiReconcileSection("new-section", "recreated")}
	states := []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "new-state", "entity/recreated", "entity/other-template")}
	if err := replaceDirtyWikiProducts(t.Context(), request, rows, nil, nil, states, false); err != nil {
		t.Fatal(err)
	}
	if eng.rows["old-section"] != nil {
		t.Error("recreated parent retained its obsolete section")
	}
	for _, id := range []string{"recreated", "new-section", "other", "other-section"} {
		if eng.rows[id] == nil {
			t.Errorf("replacement removed current product %s", id)
		}
	}
	if publisher.calls != 1 {
		t.Fatalf("completion calls = %d, want 1", publisher.calls)
	}
}

func TestDirtyWikiIncompleteProductSearchFailsClosed(t *testing.T) {
	for _, fullReplace := range []bool{false, true} {
		for _, limitHit := range []bool{false, true} {
			t.Run(fmt.Sprintf("full-replace=%t/limit-hit=%t", fullReplace, limitHit), func(t *testing.T) {
				eng, publisher, request := dirtyWikiReconcileSetup(t)
				count := 1
				if limitHit {
					count = 10000
				} else {
					eng.total = 2
				}
				for i := 0; i < count; i++ {
					id := fmt.Sprintf("old-%d", i)
					eng.rows[id] = dirtyWikiReconcilePage(id, "entity/old")
				}
				eng.rows["active"] = map[string]any{"id": "active", "kb_id": "dataset", "type_kwd": "wiki_map_active", "available_int": 0, "source_doc_ids": []string{"document"}}
				states := []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "new-state", "entity/new")}
				rows := []map[string]any{dirtyWikiReconcilePage("new", "entity/new")}
				if fullReplace {
					rows, states = nil, nil
				}
				err := replaceDirtyWikiProducts(t.Context(), request, rows, nil, nil, states, fullReplace)
				if err == nil || !strings.Contains(err.Error(), "persisted product search is incomplete") {
					t.Errorf("incomplete product search error = %v", err)
				}
				if eng.writes != 0 || publisher.calls != 0 {
					t.Errorf("incomplete product search wrote=%d published=%d", eng.writes, publisher.calls)
				}
				if len(eng.rows) != count+1 || eng.rows["active"] == nil || eng.rows["old-0"] == nil {
					t.Error("incomplete product search changed persisted products or active state")
				}
			})
		}
	}
}

func TestDirtyWikiFullReplaceClearsCompleteProductsAndActiveState(t *testing.T) {
	for _, withProducts := range []bool{false, true} {
		t.Run(fmt.Sprintf("with-products=%t", withProducts), func(t *testing.T) {
			eng, publisher, request := dirtyWikiReconcileSetup(t)
			if withProducts {
				eng.rows["old"] = dirtyWikiReconcilePage("old", "entity/old")
				eng.rows["section"] = dirtyWikiReconcileSection("section", "old")
			}
			eng.rows["active"] = map[string]any{"id": "active", "kb_id": "dataset", "type_kwd": "wiki_map_active", "available_int": 0, "source_doc_ids": []string{"document"}}
			foreign := dirtyWikiReconcilePage("foreign", "entity/foreign")
			foreign["doc_id"] = "another-document"
			eng.rows["foreign"] = foreign
			if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, nil, true); err != nil {
				t.Fatal(err)
			}
			if len(eng.rows) != 1 || eng.rows["foreign"] == nil {
				t.Fatalf("full replacement left products or active state, or removed foreign row: %v", eng.rows)
			}
			if publisher.calls != 1 {
				t.Fatalf("completion calls = %d, want 1", publisher.calls)
			}
		})
	}
}

func TestDirtyWikiReconcilesCompleteTemplatePlans(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["old"] = dirtyWikiReconcilePage("old", "entity/old-format")
	eng.rows["old-section"] = dirtyWikiReconcileSection("old-section", "old")
	eng.rows["other"] = dirtyWikiReconcilePage("other", "entity/other-template")
	eng.rows["other-section"] = dirtyWikiReconcileSection("other-section", "other")
	foreign := dirtyWikiReconcilePage("foreign", "entity/foreign")
	foreign["doc_id"] = "another-document"
	eng.rows["foreign"] = foreign
	states := []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "new-format", "entity/new-format"), dirtyWikiReconcileState(t, "unchanged-template", "entity/other-template")}
	newPage := dirtyWikiReconcilePage("new", "entity/new-format")
	if err := replaceDirtyWikiProducts(t.Context(), request, []map[string]any{newPage}, []string{"entity/new-format"}, nil, states, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "old-section"} {
		if eng.rows[id] != nil {
			t.Errorf("obsolete product %s remains", id)
		}
	}
	for _, id := range []string{"new", "other", "other-section", "foreign"} {
		if eng.rows[id] == nil {
			t.Errorf("current or foreign product %s was deleted", id)
		}
	}
	if publisher.calls != 1 {
		t.Fatalf("completion calls = %d, want 1", publisher.calls)
	}
}

func TestDirtyWikiEmptyPlanRetractsProducts(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["old"] = dirtyWikiReconcilePage("old", "entity/old")
	eng.rows["section"] = dirtyWikiReconcileSection("section", "old")
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "empty")}, false); err != nil {
		t.Fatal(err)
	}
	if eng.rows["old"] != nil || eng.rows["section"] != nil {
		t.Fatal("empty complete plan left old page or section")
	}
	if publisher.calls != 1 {
		t.Fatalf("completion calls = %d, want 1", publisher.calls)
	}
}

func TestDirtyWikiSharedSlugRemainsInOtherTemplate(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["shared"] = dirtyWikiReconcilePage("shared", "entity/shared")
	eng.rows["section"] = dirtyWikiReconcileSection("section", "shared")
	states := []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "removed-from-first"), dirtyWikiReconcileState(t, "retained-in-second", "entity/shared")}
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, []string{"entity/shared"}, []string{"entity/shared"}, states, false); err != nil {
		t.Fatal(err)
	}
	if eng.rows["shared"] == nil || eng.rows["section"] == nil {
		t.Fatal("another template's unchanged shared slug was deleted")
	}
	if publisher.calls != 1 {
		t.Fatalf("pending dirty completion calls = %d, want 1", publisher.calls)
	}
}

func TestDirtyWikiNoChangePublishesPendingCompletion(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["unchanged"] = dirtyWikiReconcilePage("unchanged", "entity/unchanged")
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "unchanged-state", "entity/unchanged")}, false); err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 1 {
		t.Fatalf("pending dirty completion calls = %d, want 1", publisher.calls)
	}
	if eng.rows["unchanged-state"] == nil {
		t.Fatal("no-change run did not persist its active state")
	}
}

func TestDirtyWikiReconciliationFailsClosedBeforeWrites(t *testing.T) {
	for _, name := range []string{"malformed", "missing-plan", "wrong-scope", "missing-state", "invalid-plan", "blank-slug", "wrong-tenant-request", "wrong-dataset-request", "truncated-search", "stale-revision"} {
		t.Run(name, func(t *testing.T) {
			eng, publisher, request := dirtyWikiReconcileSetup(t)
			eng.rows["old"] = dirtyWikiReconcilePage("old", "entity/old")
			state := dirtyWikiReconcileState(t, "state", "entity/new")
			states := []kc.WikiMapActiveState{state}
			switch name {
			case "malformed":
				states[0].Payload = []byte("{broken")
			case "missing-plan":
				states[0].Payload = []byte(`{"chunks":{}}`)
			case "wrong-scope":
				states[0].DatasetID = "another-dataset"
			case "missing-state":
				states = nil
			case "invalid-plan":
				states[0].Payload = []byte(`{"plan":{}}`)
			case "blank-slug":
				states[0].Payload = []byte(`{"plan":[{"slug":" "}]}`)
			case "wrong-tenant-request":
				request.TenantID = "another-tenant"
				states[0].TenantID = request.TenantID
			case "wrong-dataset-request":
				request.DatasetID = "another-dataset"
				states[0].DatasetID = request.DatasetID
			case "truncated-search":
				eng.total = 10001
			case "stale-revision":
				request.Revision--
			}
			err := replaceDirtyWikiProducts(t.Context(), request, []map[string]any{dirtyWikiReconcilePage("new", "entity/new")}, []string{"entity/new"}, nil, states, false)
			if name == "stale-revision" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "Wiki") {
				t.Fatalf("unsafe replacement error = %v", err)
			}
			if eng.writes != 0 || publisher.calls != 0 {
				t.Fatalf("unsafe replacement wrote=%d published=%d", eng.writes, publisher.calls)
			}
			if eng.rows["old"] == nil {
				t.Fatal("unsafe replacement deleted original")
			}
		})
	}
}

func TestDirtyWikiBarePlanSlugUsesStoredPageIdentity(t *testing.T) {
	for _, pageType := range []string{"", "concept", "topic"} {
		t.Run("page-type-"+pageType, func(t *testing.T) {
			eng, publisher, request := dirtyWikiReconcileSetup(t)
			storedType := pageType
			if storedType == "" {
				storedType = "concept"
			}
			eng.rows["unchanged"] = dirtyWikiReconcilePage("unchanged", storedType+"/alpha")
			eng.rows["section"] = dirtyWikiReconcileSection("section", "unchanged")
			state := dirtyWikiReconcileState(t, "state")
			payload, err := json.Marshal(map[string]any{"plan": []map[string]string{{"slug": "alpha", "page_type": pageType}}})
			if err != nil {
				t.Fatal(err)
			}
			state.Payload = payload
			if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{state}, false); err != nil {
				t.Fatal(err)
			}
			if eng.rows["unchanged"] == nil || eng.rows["section"] == nil {
				t.Fatal("bare plan slug retracted its canonical stored page or section")
			}
			if publisher.calls != 1 {
				t.Fatalf("pending dirty completion calls = %d, want 1", publisher.calls)
			}
		})
	}
}

func TestDirtyWikiBareRegeneratedPageRetractsOldSections(t *testing.T) {
	eng, _, request := dirtyWikiReconcileSetup(t)
	eng.rows["page"] = dirtyWikiReconcilePage("page", "concept/alpha")
	eng.rows["old-section"] = dirtyWikiReconcileSection("old-section", "page")
	state := dirtyWikiReconcileState(t, "state")
	state.Payload = []byte(`{"plan":[{"slug":"alpha","page_type":"concept"}]}`)
	rows := []map[string]any{dirtyWikiReconcilePage("page", "concept/alpha"), dirtyWikiReconcileSection("new-section", "page")}
	if err := replaceDirtyWikiProducts(t.Context(), request, rows, []string{"alpha"}, nil, []kc.WikiMapActiveState{state}, false); err != nil {
		t.Fatal(err)
	}
	if eng.rows["old-section"] != nil {
		t.Fatal("regenerated bare-slug page retained its obsolete section")
	}
	if eng.rows["page"] == nil || eng.rows["new-section"] == nil {
		t.Fatal("regenerated page or section was deleted")
	}
}

func TestDirtyWikiRetriesCompletionAfterProductDeletion(t *testing.T) {
	for _, retainPage := range []bool{false, true} {
		t.Run(fmt.Sprint("retain-", retainPage), func(t *testing.T) {
			eng, publisher, request := dirtyWikiReconcileSetup(t)
			eng.rows["old"] = dirtyWikiReconcilePage("old", "concept/old")
			eng.rows["old-section"] = dirtyWikiReconcileSection("old-section", "old")
			state := dirtyWikiReconcileState(t, "state")
			if retainPage {
				eng.rows["retained"] = dirtyWikiReconcilePage("retained", "concept/retained")
				state = dirtyWikiReconcileState(t, "state", "concept/retained")
			}
			failure := errors.New("completion transport unavailable")
			publisher.publish = func() error {
				if publisher.calls == 1 {
					return failure
				}
				return nil
			}
			if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{state}, false); !errors.Is(err, failure) {
				t.Fatalf("first completion error = %v", err)
			}
			if eng.rows["old"] != nil || eng.rows["old-section"] != nil {
				t.Fatal("first replacement did not delete old products")
			}
			// The same durable dirty row survives the error; retry in a fresh
			// invocation after the deletion and active snapshot have persisted.
			if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{state}, false); err != nil {
				t.Fatal(err)
			}
			if publisher.calls != 2 {
				t.Fatalf("unchanged retry lost pending completion: calls=%d", publisher.calls)
			}
			if retainPage && eng.rows["retained"] == nil {
				t.Fatal("retry deleted retained page")
			}
		})
	}
}

func TestDirtyWikiSerializesRevisionDuringProductReconciliation(t *testing.T) {
	eng, _, request := dirtyWikiReconcileSetup(t, true)
	db := kcWiringDeps.DB
	newRequest := request
	newRequest.Revision++
	newState := dirtyWikiReconcileState(t, "new-state", "concept/new")
	blocked := false
	eng.beforeSearch = func() {
		eng.beforeSearch = nil
		attempt := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			attempt <- db.WithContext(ctx).Model(&entity.WikiDocumentDirty{}).Where("document_id = ?", request.DocumentID).UpdateColumn("revision", newRequest.Revision).Error
		}()
		err := <-attempt
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "locked") && !strings.Contains(strings.ToLower(err.Error()), "busy") && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("concurrent revision failed unexpectedly: %v", err)
			}
			blocked = true
			return
		}
		// Without serialization a newer worker can complete before the older
		// worker's search returns, exposing its products to the older plan.
		if err := replaceDirtyWikiProducts(t.Context(), newRequest, []map[string]any{dirtyWikiReconcilePage("new", "concept/new"), dirtyWikiReconcileSection("new-section", "new")}, []string{"concept/new"}, nil, []kc.WikiMapActiveState{newState}, false); err != nil {
			t.Error(err)
		}
	}
	oldState := dirtyWikiReconcileState(t, "old-state", "concept/old")
	if err := replaceDirtyWikiProducts(t.Context(), request, []map[string]any{dirtyWikiReconcilePage("old", "concept/old")}, []string{"concept/old"}, nil, []kc.WikiMapActiveState{oldState}, false); err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Error("new revision entered while older reconciliation held its write window")
		if eng.rows["new"] == nil || eng.rows["new-section"] == nil {
			t.Error("old revision deleted newer page or section")
		}
	}
	// Once the old window has ended, the newer revision can reconcile normally.
	if err := db.Model(&entity.WikiDocumentDirty{}).Where("document_id = ?", request.DocumentID).UpdateColumn("revision", newRequest.Revision).Error; err != nil {
		t.Fatal(err)
	}
	if err := replaceDirtyWikiProducts(t.Context(), newRequest, []map[string]any{dirtyWikiReconcilePage("new", "concept/new"), dirtyWikiReconcileSection("new-section", "new")}, []string{"concept/new"}, nil, []kc.WikiMapActiveState{newState}, false); err != nil {
		t.Fatal(err)
	}
	if eng.rows["new"] == nil || eng.rows["new-section"] == nil || eng.rows["old"] != nil {
		t.Fatal("serialized newer revision did not retain its final products")
	}
}

func TestDirtyWikiPublishesAfterReleasingReconciliationLock(t *testing.T) {
	_, publisher, request := dirtyWikiReconcileSetup(t, true)
	db := kcWiringDeps.DB
	publisher.publish = func() error {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		// The real scheduler starts another DB transaction for its durable
		// backlog. Updating the same row proves reconciliation has unlocked.
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Model(&entity.WikiDocumentDirty{}).Where("document_id = ?", request.DocumentID).UpdateColumn("revision", gorm.Expr("revision")).Error
		})
	}
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "empty")}, false); err != nil {
		t.Fatalf("publisher could not acquire separate transaction: %v", err)
	}
	if publisher.calls != 1 {
		t.Fatalf("completion calls=%d,want 1", publisher.calls)
	}
}
