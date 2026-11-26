package knowledge_compile

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/file"
)

type recordingPageCommitter struct {
	inputs []file.PageEditCommitInput
}

func (c *recordingPageCommitter) RecordPageEdit(_ context.Context, input file.PageEditCommitInput) (*entity.FileCommit, error) {
	c.inputs = append(c.inputs, input)
	return &entity.FileCommit{ID: "commit-1"}, nil
}

func TestWriteMergedRecordsGeneratedPageCommitOnly(t *testing.T) {
	committer := &recordingPageCommitter{}
	w := engineWriter{eng: &fakeEngine{}, commitService: committer}
	products := []kccommon.Product{
		{
			Variant: kccommon.VariantWiki,
			Content: "# Alpha\n\nBody",
			Meta: map[string]any{
				"kind":      "page",
				"slug":      "entity/alpha",
				"page_type": "entity",
			},
		},
		{
			Variant: kccommon.VariantWiki,
			Content: "Body",
			Meta: map[string]any{
				"kind":      "section",
				"slug":      "body",
				"page_type": "entity",
			},
		},
	}

	if err := w.WriteMerged(t.Context(), "tenant-1", "kb-1", products); err != nil {
		t.Fatalf("WriteMerged: %v", err)
	}
	if len(committer.inputs) != 1 {
		t.Fatalf("generated page commits = %d, want 1", len(committer.inputs))
	}
	input := committer.inputs[0]
	if input.DatasetID != "kb-1" || input.PageType != "entity" || input.Slug != "alpha" {
		t.Fatalf("unexpected generated page identity: %+v", input)
	}
	if input.OldContent != "" || input.NewContent != "# Alpha\n\nBody" {
		t.Fatalf("unexpected generated page content: %+v", input)
	}
}

// TestWriteMergedAttributesGeneratedPageToItsUploader pins who a generated page
// revision belongs to. The compile worker has no request identity, so it reads
// the uploader of the page's source documents; a page merged from documents
// several users uploaded, and a page whose documents are unknown, fall back to
// the dataset's owner rather than recording nobody.
func TestWriteMergedAttributesGeneratedPageToItsUploader(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "tenant-1", Name: "dataset", EmbdID: "embd@Builtin",
		Permission: "me", CreatedBy: "owner-1",
	}).Error; err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	for _, doc := range []struct{ id, createdBy string }{
		{"doc-1", "user-1"},
		{"doc-2", "user-1"},
		{"doc-3", "user-2"},
	} {
		name := doc.id
		if err := db.Create(&entity.Document{
			ID: doc.id, KbID: "kb-1", Name: &name, CreatedBy: doc.createdBy,
			UserID: doc.createdBy, ParserID: "naive", ParserConfig: entity.JSONMap{},
			Type: "text", Suffix: "txt",
		}).Error; err != nil {
			t.Fatalf("seed document %s: %v", doc.id, err)
		}
	}

	committer := &recordingPageCommitter{}
	w := engineWriter{eng: &fakeEngine{}, db: db, commitService: committer}
	page := func(slug string, sourceDocIDs ...string) kccommon.Product {
		meta := map[string]any{"kind": "page", "slug": slug, "page_type": "entity"}
		if len(sourceDocIDs) > 0 {
			meta["source_doc_ids"] = sourceDocIDs
		}
		return kccommon.Product{Variant: kccommon.VariantWiki, Content: "# " + slug, Meta: meta}
	}
	products := []kccommon.Product{
		page("entity/one-uploader", "doc-1", "doc-2"),
		page("entity/two-uploaders", "doc-1", "doc-3"),
		page("entity/no-source"),
	}

	if err := w.WriteMerged(t.Context(), "tenant-1", "kb-1", products); err != nil {
		t.Fatalf("WriteMerged: %v", err)
	}
	if len(committer.inputs) != 3 {
		t.Fatalf("generated page commits = %d, want 3", len(committer.inputs))
	}
	want := map[string]string{
		"one-uploader":  "user-1",
		"two-uploaders": "owner-1",
		"no-source":     "owner-1",
	}
	for _, input := range committer.inputs {
		if input.AuthorID != want[input.Slug] {
			t.Errorf("page %s: author = %q, want %q", input.Slug, input.AuthorID, want[input.Slug])
		}
	}
}

// TestDeleteMergedWikiPagesAttributesTheRemovalToItsUploader covers the other
// half of the history: a page leaving the dataset is a commit too, and it names
// the uploader of the page's source documents. A document-delete removal
// usually has no document row left to read, so the dataset's owner names it.
func TestDeleteMergedWikiPagesAttributesTheRemovalToItsUploader(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "tenant-1", Name: "dataset", EmbdID: "embd@Builtin",
		Permission: "me", CreatedBy: "owner-1",
	}).Error; err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	name := "kept.txt"
	if err := db.Create(&entity.Document{
		ID: "doc-1", KbID: "kb-1", Name: &name, CreatedBy: "user-1", UserID: "user-1",
		ParserID: "naive", ParserConfig: entity.JSONMap{}, Type: "text", Suffix: "txt",
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	committer := &recordingPageCommitter{}
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{
			"id": "page-1", "compile_kwd": "wiki_page", "available_int": 1,
			"slug_kwd": "entity/alpha", "page_type_kwd": "entity", "title_kwd": "Alpha",
			"content_with_weight": "# Alpha", "source_doc_ids": []string{"doc-1"},
		},
		{
			// Its document is already gone: nothing left to name the uploader.
			"id": "page-2", "compile_kwd": "wiki_page", "available_int": 1,
			"slug_kwd": "entity/beta", "page_type_kwd": "entity", "title_kwd": "Beta",
			"content_with_weight": "# Beta", "source_doc_ids": []string{"doc-gone"},
		},
	}}
	w := engineWriter{eng: eng, db: db, commitService: committer}

	if err := w.DeleteMergedWikiPages(t.Context(), "tenant-1", "kb-1", []string{"page-1", "page-2"}); err != nil {
		t.Fatalf("DeleteMergedWikiPages: %v", err)
	}
	if len(committer.inputs) != 2 {
		t.Fatalf("removal commits = %d, want 2", len(committer.inputs))
	}
	want := map[string]string{"alpha": "user-1", "beta": "owner-1"}
	for _, input := range committer.inputs {
		if input.AuthorID != want[input.Slug] {
			t.Errorf("page %s: author = %q, want %q", input.Slug, input.AuthorID, want[input.Slug])
		}
		if input.NewContent != "" {
			t.Errorf("page %s: removal kept content %q", input.Slug, input.NewContent)
		}
	}
}

// TestPagedRemovalSweepsOrderTheirRows covers the pagination contract of the two
// sweeps that walk a dataset a batch at a time. An unordered offset scan can hand
// the same row back twice and skip another between requests, and a page skipped by
// the clear sweep never gets the removal commit that keeps it out of a later
// restore - the version being restored would then rebuild a page it never held.
func TestPagedRemovalSweepsOrderTheirRows(t *testing.T) {
	page := map[string]interface{}{
		"id": "page-1", "compile_kwd": "wiki", "type_kwd": "wiki_page", "available_int": 1,
		"slug_kwd": "entity/alpha", "entity_type_kwd": "entity", "title_kwd": "Alpha",
		"content_with_weight": "# Alpha",
	}

	clear := &fakeEngine{searchChunks: []map[string]interface{}{page}}
	clearWriter := engineWriter{eng: clear, commitService: &recordingPageCommitter{}}
	clearWriter.recordAllWikiPageRemovals(t.Context(), "tenant-1", "kb-1", "clear", false)
	if clear.lastSearchReq == nil || clear.lastSearchReq.OrderBy == nil {
		t.Fatal("the clear sweep paginates without an order, so a page can be skipped between batches")
	}

	strip := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "page-2", "source_doc_ids": []string{"doc-gone", "doc-kept"}},
	}}
	stripWriter := engineWriter{eng: strip}
	if err := stripWriter.StripMergedSources(t.Context(), "tenant-1", "kb-1", []string{"doc-gone"}); err != nil {
		t.Fatalf("StripMergedSources: %v", err)
	}
	if len(strip.searchReqs) == 0 || strip.searchReqs[0].OrderBy == nil {
		t.Fatal("the source strip paginates without an order, so a row can be skipped between batches")
	}
}

// TestDeleteMergedWikiPagesReadsTheNewPageShape covers the rows the compiler
// writes now: the category sits in entity_type_kwd beside compile_kwd="wiki", and
// page_type_kwd is not set. Reading the old column skipped every one of them, so a
// page leaving the dataset left no commit behind - and a restore had nothing to
// bring it back from.
func TestDeleteMergedWikiPagesReadsTheNewPageShape(t *testing.T) {
	committer := &recordingPageCommitter{}
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{
			"id": "page-1", "compile_kwd": "wiki", "type_kwd": "wiki_page", "available_int": 1,
			"slug_kwd": "topic/People/Writers", "entity_type_kwd": "topic", "title_kwd": "Writers",
			"content_with_weight": "# Writers",
		},
	}}
	w := engineWriter{eng: eng, commitService: committer}

	if err := w.DeleteMergedWikiPages(t.Context(), "tenant-1", "kb-1", []string{"page-1"}); err != nil {
		t.Fatalf("DeleteMergedWikiPages: %v", err)
	}
	if len(committer.inputs) != 1 {
		t.Fatalf("removal commits = %d, want 1", len(committer.inputs))
	}
	input := committer.inputs[0]
	if input.PageType != "topic" || input.Slug != "People/Writers" {
		t.Errorf("removal names page %q of type %q, want topic / People/Writers", input.Slug, input.PageType)
	}
	// The row is named by columns the scan has to ask for: a projection that leaves
	// compile_kwd/type_kwd out makes a compiler-written row look like a non-page.
	if eng.lastSearchReq == nil {
		t.Fatal("the removal scan issued no search request")
	}
	projected := make(map[string]bool, len(eng.lastSearchReq.SelectFields))
	for _, field := range eng.lastSearchReq.SelectFields {
		projected[field] = true
	}
	for _, field := range []string{"compile_kwd", "type_kwd", "entity_type_kwd", "page_type_kwd"} {
		if !projected[field] {
			t.Errorf("removal scan does not select %s, so its rows cannot be recognised as pages", field)
		}
	}
	if input.NewContent != "" {
		t.Errorf("removal kept content %q", input.NewContent)
	}
}

// TestMergedChunkMapKeepsWikiFields locks the fix for the merged-row metadata
// gap: the dataset-level merged row written by mergedChunkMap must carry the
// wiki page fields (entity_type_kwd/topic_kwd/title_kwd/slug_kwd/...) that the
// artifact API (ListArtifacts/ListWikiTopics) and page renderers read. Without
// them the compilation page surfaces no wiki pages from the merged products.
func TestMergedChunkMapKeepsWikiFields(t *testing.T) {
	p := kccommon.Product{
		ID: "merged-1", DocID: "kb1", TenantID: "t1", Variant: kccommon.VariantWiki,
		Content: "# Alpha\n\n**Body** links to [Beta](entity/beta).",
		Vector:  []float32{0.1, 0.2, 0.3},
		Meta: map[string]any{
			"kind":             "page",
			"slug":             "entity/alpha",
			"title":            "Alpha",
			"page_type":        "entity",
			"topic":            " Knowledge / Core / Alpha ",
			"summary":          "A page about Alpha",
			"entity_names":     []string{"Alpha"},
			"related_kb_pages": []string{"entity/beta"},
			"source_doc_ids":   []string{"d1"},
			"source_chunk_ids": []string{"c1"},
		},
	}
	now := time.Now()
	m := mergedChunkMap("t1", "kb1", "run-abc", "hash-123", now, p)

	cases := map[string]string{
		"slug_kwd":            "entity/alpha",
		"title_kwd":           "Alpha",
		"entity_type_kwd":     "entity",
		"topic_kwd":           "Knowledge/Core/Alpha",
		"summary_with_weight": "A page about Alpha",
		"content_with_weight": p.Content,
		"plan_kwd":            "run-abc",
		"input_hash_kwd":      "hash-123",
	}
	// The wall-clock audit fields must be stamped from `now`, not omitted.
	if m["create_time"] != now.Format("2006-01-02 15:04:05") {
		t.Errorf("create_time = %v, want %v", m["create_time"], now.Format("2006-01-02 15:04:05"))
	}
	if m["create_timestamp_flt"] != float64(now.Unix()) {
		t.Errorf("create_timestamp_flt = %v, want %v", m["create_timestamp_flt"], float64(now.Unix()))
	}
	for k, want := range cases {
		if got, _ := m[k].(string); got != want {
			t.Errorf("merged row[%q] = %q, want %q", k, got, want)
		}
	}
	if _, ok := m["md_with_weight"]; ok {
		t.Fatal("merged Wiki rows must not duplicate the body in md_with_weight")
	}
	if v, _ := m["entity_names_kwd"].([]string); len(v) != 1 || v[0] != "Alpha" {
		t.Errorf("entity_names_kwd = %#v, want [Alpha]", m["entity_names_kwd"])
	}
	if m["doc_id"] != "kb1" || m["available_int"] != 1 {
		t.Errorf("merged flags wrong: doc_id=%v available_int=%v", m["doc_id"], m["available_int"])
	}
	// No kc_* key may be persisted: they exist in no engine mapping, and
	// Infinity rejects an insert that carries an unknown column.
	for k := range m {
		if strings.HasPrefix(k, "kc_") {
			t.Errorf("merged row must not persist the non-schema field %q", k)
		}
	}
	if m["q_3_vec"] == nil {
		t.Errorf("vector column missing")
	}
}

// TestProductFromChunkMapRestoresWikiFields locks the reader side: the wiki page
// columns must be reconstructed into the product Meta so the merge step can carry
// them onto the merged row.
func TestProductFromChunkMapRestoresWikiFields(t *testing.T) {
	c := map[string]interface{}{
		"id":                   "wiki/1",
		"doc_id":               "d1",
		"compile_kwd":          "wiki_page",
		"content_with_weight":  "# Alpha\n\nBody",
		"slug_kwd":             "entity/alpha",
		"page_type_kwd":        "entity",
		"topic_kwd":            "Alpha",
		"title_kwd":            "Alpha",
		"summary_with_weight":  "A page about Alpha",
		"entity_names_kwd":     []interface{}{"Alpha"},
		"related_kb_pages_kwd": []interface{}{"entity/beta"},
		"depth_int":            float64(2),
	}
	p, ok := productFromChunkMap(c, "t1", kccommon.VariantWiki)
	if !ok {
		t.Fatalf("productFromChunkMap returned not-ok")
	}
	want := map[string]string{
		"slug": "entity/alpha", "page_type": "entity", "topic": "Alpha",
		"title": "Alpha", "summary": "A page about Alpha",
	}
	for k, v := range want {
		if got, _ := p.Meta[k].(string); got != v {
			t.Errorf("meta[%q] = %q, want %q", k, got, v)
		}
	}
	if v, _ := p.Meta["section_level"].(int64); v != 2 {
		t.Errorf("section_level = %v, want 2", p.Meta["section_level"])
	}
	if v, _ := p.Meta["entity_names"].([]string); len(v) != 1 || v[0] != "Alpha" {
		t.Errorf("entity_names = %#v, want [Alpha]", p.Meta["entity_names"])
	}
	if p.Merged {
		t.Errorf("per-doc row must not be marked merged")
	}
}

func TestWriteMergedStructureMigratesTypeScopedEntityID(t *testing.T) {
	oldID := "dataset_structure_old_type_identity"
	eng := &fakeEngine{engineType: "infinity", searchChunks: []map[string]interface{}{
		{
			"id":                            oldID,
			"compile_kwd":                   "hypergraph",
			"compilation_template_ids":      []string{"tpl1"},
			"compilation_template_kind_kwd": "knowledge_graph",
			"knowledge_graph_kwd":           "entity",
			"name_kwd":                      "Engine",
			"entity_type_kwd":               "other",
			"content_with_weight":           `{"name":"Engine","type":"other","description":"existing"}`,
			"source_doc_ids":                []string{"d1"},
			"source_chunk_ids":              []string{"c1"},
		},
	}}
	w := engineWriter{eng: eng}
	err := w.WriteMergedStructure(context.Background(), "t1", "kb1", []StructureBucket{{
		Name: "engine", Type: "component", Description: "incoming", CompileKwd: "hypergraph",
		TemplateID: "tpl1", TemplateKind: "knowledge_graph", SourceDocIDs: []string{"d2"}, SourceChunkIDs: []string{"c2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(eng.insertedChunks) == 0 {
		t.Fatal("new structure row was not inserted")
	}
	var entity map[string]interface{}
	for _, row := range eng.insertedChunks {
		if row["type_kwd"] == "entity" {
			entity = row
			break
		}
	}
	if entity == nil {
		t.Fatalf("inserted rows contain no entity: %+v", eng.insertedChunks)
	}
	if _, ok := entity["knowledge_graph_kwd"]; ok {
		t.Fatalf("new entity row still writes knowledge_graph_kwd: %+v", entity)
	}
	var foundBuildMeta bool
	for _, row := range eng.insertedChunks {
		if row["type_kwd"] == "kg_build_meta" {
			foundBuildMeta = true
			if _, ok := row["knowledge_graph_kwd"]; ok {
				t.Fatalf("new build marker still writes knowledge_graph_kwd: %+v", row)
			}
		}
	}
	if !foundBuildMeta {
		t.Fatalf("inserted rows contain no type_kwd=kg_build_meta marker: %+v", eng.insertedChunks)
	}
	if entity["id"] == oldID {
		t.Fatalf("old type-scoped id was reused: %v", entity["id"])
	}
	if entity["entity_type_kwd"] != "component" {
		t.Fatalf("specific incoming type should replace existing other: %v", entity["entity_type_kwd"])
	}
	if got := firstStringSlice(entity["source_doc_ids"]); len(got) != 2 {
		t.Fatalf("source documents were not migrated: %v", got)
	}
	ids, ok := eng.lastDeleteCond["id"].([]string)
	if !ok || len(ids) != 1 || ids[0] != oldID {
		t.Fatalf("superseded id was not deleted: %v", eng.lastDeleteCond)
	}
}

func TestWriteMergedStructureKeepsEntityKeywordLowercase(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.WriteMergedStructure(context.Background(), "t1", "kb1", []StructureBucket{{
		Name: "Acme Corp", Type: "organization", Description: "company", CompileKwd: "knowledge_graph", TemplateKind: "knowledge_graph",
	}}); err != nil {
		t.Fatalf("WriteMergedStructure: %v", err)
	}
	for _, row := range eng.insertedChunks {
		if row["type_kwd"] != "entity" {
			continue
		}
		if row["name_kwd"] != "acme corp" {
			t.Errorf("name_kwd = %v, want normalized keyword acme corp", row["name_kwd"])
		}
		payload, _ := row["content_with_weight"].(string)
		if !strings.Contains(payload, `"name":"Acme Corp"`) {
			t.Errorf("entity payload = %s, want extracted spelling Acme Corp", payload)
		}
		for _, field := range []string{"content_ltks", "content_sm_ltks"} {
			text, _ := row[field].(string)
			if !strings.Contains(text, "company") || !strings.Contains(strings.ToLower(text), "acme corp") || strings.Contains(text, `"name":`) {
				t.Errorf("%s = %q, want searchable entity payload text", field, text)
			}
		}
		if _, exists := row["extra"]; exists {
			t.Fatalf("compiler must not write extra: %#v", row["extra"])
		}
		return
	}
	t.Fatal("inserted rows contain no entity")
}

func TestWriteMergedStructurePreservesDistinctRelationTypes(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	buckets := []StructureBucket{
		{Name: "Acme Corp -> Beta Ltd", Type: "relation", FromEntity: "Acme Corp", ToEntity: "Beta Ltd", RelationType: "supplies", Description: "supply", CompileKwd: "knowledge_graph", TemplateKind: "knowledge_graph"},
		{Name: "Acme Corp -> Beta Ltd", Type: "relation", FromEntity: "Acme Corp", ToEntity: "Beta Ltd", RelationType: "owns", Description: "ownership", CompileKwd: "knowledge_graph", TemplateKind: "knowledge_graph"},
	}
	if err := w.WriteMergedStructure(context.Background(), "t1", "kb1", buckets); err != nil {
		t.Fatalf("WriteMergedStructure: %v", err)
	}
	relations := map[string]bool{}
	for _, row := range eng.insertedChunks {
		if row["type_kwd"] != "relation" {
			continue
		}
		for _, field := range []string{"content_ltks", "content_sm_ltks"} {
			text, _ := row[field].(string)
			if !strings.Contains(strings.ToLower(text), "acme corp") || !strings.Contains(strings.ToLower(text), "beta ltd") || strings.Contains(text, `"from":`) {
				t.Errorf("%s = %q, want searchable relation payload text", field, text)
			}
		}
		payload, _ := row["content_with_weight"].(string)
		for _, relationType := range []string{"supplies", "owns"} {
			if strings.Contains(payload, `"type":"`+relationType+`"`) {
				relations[relationType] = true
			}
		}
	}
	if len(relations) != 2 || !relations["supplies"] || !relations["owns"] {
		t.Errorf("stored relation types = %v, want separate supplies and owns rows", relations)
	}
}

func TestWriteMergedStructureDoesNotDeleteForInvalidBucket(t *testing.T) {
	oldID := "dataset_structure_old"
	eng := &fakeEngine{engineType: "infinity", searchChunks: []map[string]interface{}{
		{
			"id":                            oldID,
			"compile_kwd":                   "hypergraph",
			"compilation_template_ids":      []string{"tpl1"},
			"compilation_template_kind_kwd": "knowledge_graph",
			"knowledge_graph_kwd":           "entity",
			"name_kwd":                      "Engine",
			"entity_type_kwd":               "component",
			"content_with_weight":           `{"name":"Engine","type":"component","description":"existing"}`,
		},
	}}
	w := engineWriter{eng: eng}
	err := w.WriteMergedStructure(context.Background(), "t1", "kb1", []StructureBucket{{
		Name: "Engine", Type: "component", CompileKwd: "hypergraph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if eng.lastDeleteCond != nil {
		t.Fatalf("invalid bucket must not delete the existing row: %v", eng.lastDeleteCond)
	}
}

// TestDeleteMergedScopesToMergedWikiRows locks the W1 contract: DeleteMerged
// must only target dataset-level (available_int=1) wiki merged rows. The
// structural filter (kb_id + available_int + wiki page/section compile_kwd) is
// the source of truth, so a wrong tenant/kb can never cascade to per-document
// rows or to rows of other variants.
func TestDeleteMergedScopesToMergedWikiRows(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.DeleteMerged(context.Background(), "t1", "kb1"); err != nil {
		t.Fatalf("DeleteMerged: %v", err)
	}
	cond := eng.lastDeleteCond
	if cond == nil {
		t.Fatal("engine.DeleteChunks was not called")
	}
	if cond["kb_id"] != "kb1" {
		t.Errorf("DeleteMerged kb_id = %v, want kb1", cond["kb_id"])
	}
	if cond["available_int"] != 1 {
		t.Errorf("DeleteMerged must scope to available_int=1, got %v", cond["available_int"])
	}
	variants, ok := cond["type_kwd"].([]string)
	if !ok {
		t.Fatalf("DeleteMerged must pass a compile_kwd string slice, got %T", cond["type_kwd"])
	}
	if len(variants) != 2 || variants[0] != compileKwdWikiPage || variants[1] != compileKwdWikiSection {
		t.Errorf("DeleteMerged compile_kwd = %v, want [%q %q]", variants, compileKwdWikiPage, compileKwdWikiSection)
	}
}

// TestDeleteMergedForVariant_EmptySetClearsAll covers B4/B1b: an empty variants
// slice (full rebuild) clears the full managed set — nav + wiki + structure
// compile_kwds — plus the structure scope_kwd="dataset" sweep.
func TestDeleteMergedForVariant_EmptySetClearsAll(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", nil); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 2 {
		t.Fatalf("full clean issued %d deletes, want 2", len(eng.deleteConds))
	}
	for _, test := range []struct {
		row  map[string]interface{}
		want bool
	}{
		{map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_page", "available_int": 1}, true},
		{map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki_page", "available_int": 1}, true},
		{map[string]interface{}{"kb_id": "kb1", "compile_kwd": "page_index", "type_kwd": "nav_doc", "available_int": 0}, true},
		{map[string]interface{}{"kb_id": "kb1", "compile_kwd": "graph", "type_kwd": "entity", "scope_kwd": "doc"}, false},
		{map[string]interface{}{"kb_id": "kb1", "compile_kwd": "wiki", "type_kwd": "wiki_page", "available_int": 0}, false},
		{map[string]interface{}{"kb_id": "kb2", "compile_kwd": "tree", "type_kwd": "nav_doc"}, false},
	} {
		if got := rowMatchesFilter(test.row, eng.deleteConds[0]); got != test.want {
			t.Errorf("full clean matched %v = %v, want %v", test.row, got, test.want)
		}
	}
	// The second delete sweeps structure scope_kwd="dataset".
	foundScope := false
	for _, cond := range eng.deleteConds {
		if cond["scope_kwd"] == "dataset" {
			foundScope = true
		}
	}
	if !foundScope {
		t.Error("full clean must also sweep scope_kwd=dataset")
	}
}

// TestDeleteMergedForVariant_WikiScopesToAvailableOne covers the per-variant
// wiki clean: it must issue a delete scoped to available_int=1 and the wiki
// page/section kwds (wiki merged rows carry available_int=1).
func TestDeleteMergedForVariant_WikiScopesToAvailableOne(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", []kccommon.Variant{kccommon.VariantWiki}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 1 {
		t.Fatalf("wiki clean should issue exactly one delete, got %d", len(eng.deleteConds))
	}
	cond := eng.deleteConds[0]
	if cond["available_int"] != 1 {
		t.Errorf("wiki clean must scope to available_int=1, got %v", cond["available_int"])
	}
	kwds, ok := cond["type_kwd"].([]string)
	if !ok || len(kwds) != 2 {
		t.Fatalf("wiki clean compile_kwd = %v, want [wiki_page wiki_section]", cond["type_kwd"])
	}
}

// TestDeleteMergedForVariant_StructureScopesToDataset covers the per-variant
// structure clean: dataset rows are tagged scope_kwd="dataset" (compile_kwd may
// be the inferred sub-kind), so the delete is scoped to scope_kwd="dataset".
func TestDeleteMergedForVariant_StructureScopesToDataset(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", []kccommon.Variant{kccommon.VariantStructure}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 1 {
		t.Fatalf("structure clean should issue exactly one delete, got %d", len(eng.deleteConds))
	}
	if eng.deleteConds[0]["scope_kwd"] != "dataset" {
		t.Errorf("structure clean must scope to scope_kwd=dataset, got %v", eng.deleteConds[0]["scope_kwd"])
	}
}

// TestDeleteMergedForVariant_WikiPlusNavCoversNavRows covers the combined-clean
// bug: each variant issues its OWN delete (one AND-ed filter would exclude the
// other's rows — nav rows carry available_int=0, structure rows scope_kwd=
// "dataset"). A wiki+tree clean must produce one wiki delete (available_int=1)
// and one nav delete (compile_kwd=dataset_nav, no available_int).
func TestDeleteMergedForVariant_WikiPlusNavCoversNavRows(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity"}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1",
		[]kccommon.Variant{kccommon.VariantWiki, kccommon.VariantTree}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 2 {
		t.Fatalf("wiki+tree clean should issue 2 deletes (wiki + nav), got %d", len(eng.deleteConds))
	}
	var wikiCond, navCond map[string]interface{}
	for _, cond := range eng.deleteConds {
		if roles, ok := cond["type_kwd"].([]string); ok && len(roles) == 2 && roles[0] == "nav_doc" && roles[1] == "nav_cluster" {
			navCond = cond
		}
		if cond["available_int"] == 1 {
			wikiCond = cond
		}
	}
	if wikiCond == nil {
		t.Error("expected a wiki delete scoped to available_int=1")
	}
	if navCond == nil {
		t.Error("expected a nav delete scoped to nav_doc/nav_cluster")
	} else if _, hasAvail := navCond["available_int"]; hasAvail {
		t.Errorf("nav delete must not carry available_int (nav rows are 0), got %v", navCond["available_int"])
	}
}

// TestDeleteStructureForDocs_RemovesGhostOnly covers G3: structure dataset rows
// whose source docs are ALL deleted are ghosts and removed; rows that still have
// a surviving source doc are kept.
func TestDeleteStructureForDocs_RemovesGhostOnly(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity", searchChunks: []map[string]interface{}{
		{"id": "row_ghost", "source_doc_ids": []any{"d1", "d2"}}, // all deleted -> ghost
		{"id": "row_keep", "source_doc_ids": []any{"d1", "d3"}},  // d3 survives -> keep
	}}
	w := engineWriter{eng: eng}
	if err := w.DeleteStructureForDocs(context.Background(), "t1", "kb1", []string{"d1", "d2"}); err != nil {
		t.Fatalf("DeleteStructureForDocs: %v", err)
	}
	if eng.lastDeleteCond == nil {
		t.Fatal("DeleteChunks not called")
	}
	ids, ok := eng.lastDeleteCond["id"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "row_ghost" {
		t.Fatalf("ghost ids = %v, want [row_ghost]", eng.lastDeleteCond["id"])
	}
}

// TestDeleteStructureForDocs_NoGhostsNoDelete covers G3: when every structure row
// still has a surviving source doc, no delete is issued.
func TestDeleteStructureForDocs_NoGhostsNoDelete(t *testing.T) {
	eng := &fakeEngine{engineType: "infinity", searchChunks: []map[string]interface{}{
		{"id": "row_keep", "source_doc_ids": []any{"d1", "d3"}},
	}}
	before := len(eng.deleteConds)
	w := engineWriter{eng: eng}
	if err := w.DeleteStructureForDocs(context.Background(), "t1", "kb1", []string{"d1"}); err != nil {
		t.Fatalf("DeleteStructureForDocs: %v", err)
	}
	if len(eng.deleteConds) != before {
		t.Fatalf("no ghost should mean no delete, got %d deletes", len(eng.deleteConds)-before)
	}
}

// TestProductFromChunkMapRejectsDirtyKwd locks the dirty-row contract from the
// wiki_incremental plan (Claim 4): productFromChunkMap must reverse-map the raw
// compile_kwd via KwdToVariant and reject any row whose kwd does not map to the
// expected variant, including unknown / malformed kinds (e.g. "artifact_page",
// "garbage", empty). It must not silently fall back to a raw-string comparison.
func TestProductFromChunkMapRejectsDirtyKwd(t *testing.T) {
	base := map[string]interface{}{
		"id":                  "wiki/1",
		"doc_id":              "d1",
		"content_with_weight": "# Alpha",
	}
	dirtyKwds := []string{"artifact_page", "garbage", ""}
	for _, kwd := range dirtyKwds {
		c := map[string]interface{}{}
		for k, v := range base {
			c[k] = v
		}
		c["compile_kwd"] = kwd
		if p, ok := productFromChunkMap(c, "t1", kccommon.VariantWiki); ok {
			t.Errorf("dirty compile_kwd %q should be rejected, got product %+v", kwd, p)
		}
	}

	// Note: wiki_section maps to VariantWiki (page and section share the wiki
	// variant); the page/section distinction lives in Meta.kind and is enforced
	// by filterWikiPageCandidates, NOT by the variant dirty-row check. So a
	// wiki_section row legitimately satisfies a VariantWiki query here — the
	// section is dropped later by the kind filter.

	// A clean wiki_page row must pass for VariantWiki.
	good := map[string]interface{}{}
	for k, v := range base {
		good[k] = v
	}
	good["compile_kwd"] = compileKwdWikiPage
	if _, ok := productFromChunkMap(good, "t1", kccommon.VariantWiki); !ok {
		t.Errorf("clean wiki_page row should satisfy VariantWiki query")
	}
}
