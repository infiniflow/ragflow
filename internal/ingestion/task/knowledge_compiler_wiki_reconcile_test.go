package task

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

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
}

func (e *dirtyWikiReconcileEngine) GetType() string { return "elasticsearch" }
func (e *dirtyWikiReconcileEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searches++
	if !reflect.DeepEqual(req.IndexNames, []string{"ragflow_tenant"}) || !reflect.DeepEqual(req.KbIDs, []string{"dataset"}) || !reflect.DeepEqual(req.Filter["doc_id"], []string{"document"}) || req.Filter["available_int"] != 0 {
		e.t.Fatalf("unscoped Wiki reconciliation search: %+v", req)
	}
	var rows []map[string]any
	for _, row := range e.rows {
		if row["doc_id"] == "document" && row["kb_id"] == "dataset" && row["available_int"] == 0 && (row["type_kwd"] == "wiki_page" || row["type_kwd"] == "wiki_section") {
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
	ids := condition["id"].([]string)
	for _, id := range ids {
		delete(e.rows, id)
	}
	return int64(len(ids)), nil
}

type dirtyWikiReconcilePublisher struct{ calls int }

func (*dirtyWikiReconcilePublisher) Provision(context.Context) error { return nil }
func (p *dirtyWikiReconcilePublisher) Publish(context.Context, string, string, string, string, []string, []string) error {
	p.calls++
	return nil
}

func dirtyWikiReconcileSetup(t *testing.T) (*dirtyWikiReconcileEngine, *dirtyWikiReconcilePublisher, knowledge_compile.WikiDirtyRequest) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
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
	return map[string]any{"id": id, "parent_id": parent, "type_kwd": "wiki_section", "compile_kwd": "wiki", "doc_id": "document", "kb_id": "dataset", "available_int": 0}
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
	if publisher.calls != 0 {
		t.Fatalf("unchanged products published %d completions", publisher.calls)
	}
}

func TestDirtyWikiNoChangeDoesNotPublish(t *testing.T) {
	eng, publisher, request := dirtyWikiReconcileSetup(t)
	eng.rows["unchanged"] = dirtyWikiReconcilePage("unchanged", "entity/unchanged")
	if err := replaceDirtyWikiProducts(t.Context(), request, nil, nil, nil, []kc.WikiMapActiveState{dirtyWikiReconcileState(t, "unchanged-state", "entity/unchanged")}, false); err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 0 {
		t.Fatalf("unchanged products published %d completions", publisher.calls)
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
			if publisher.calls != 0 {
				t.Fatalf("unchanged products published %d completions", publisher.calls)
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
