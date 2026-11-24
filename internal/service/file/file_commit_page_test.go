package file

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPageCommitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Keep a single connection so the :memory: database is shared across all
	// goroutines (sqlite :memory: is otherwise per-connection); this also
	// serializes the concurrent page-commit test through one connection.
	if sqlDB, serr := db.DB(); serr == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	oldStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() {
		dao.DB = old
		storage.GetStorageFactory().SetStorage(oldStorage)
	})
	return db
}

func TestRecordPageEdit_CreatesCommitAndItem(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	in := PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "First edit",
		AuthorID:   "u1",
		OldContent: "hello world",
		NewContent: "hello world, edited",
	}
	commit, err := svc.RecordPageEdit(ctx, in)
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}
	if commit.ID == "" {
		t.Fatal("expected a generated commit id")
	}
	if commit.AuthorID != "u1" || commit.Message != "First edit" || commit.FileCount != 1 {
		t.Fatalf("unexpected commit: %+v", commit)
	}
	if commit.FolderID != "kb1" {
		t.Fatalf("expected folder_id kb1 for dataset scope, got %s", commit.FolderID)
	}
	if commit.ParentID != nil {
		t.Fatalf("first edit should have no parent, got %v", *commit.ParentID)
	}

	var items []entity.FileCommitItem
	if err := dao.DB.Where("commit_id = ?", commit.ID).Find(&items).Error; err != nil {
		t.Fatalf("load items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Operation != "modify" {
		t.Fatalf("expected operation modify, got %s", it.Operation)
	}
	if it.FileID != "kb1/wiki/page-a" {
		t.Fatalf("expected file_id kb1/wiki/page-a (dataset-scoped), got %s", it.FileID)
	}
	if it.SlugKwd == nil || *it.SlugKwd != "page-a" {
		t.Fatalf("expected slug_kwd page-a, got %v", it.SlugKwd)
	}
	if it.PageTypeKwd == nil || *it.PageTypeKwd != "wiki" {
		t.Fatalf("expected page_type_kwd wiki, got %v", it.PageTypeKwd)
	}
	if it.Diff == nil || !strings.Contains(*it.Diff, "hello world, edited") {
		t.Fatalf("expected diff containing new content, got %v", it.Diff)
	}
	if it.ContentAfterStorage == nil || *it.ContentAfterStorage != "minio" {
		t.Fatalf("expected content_after_storage minio, got %v", it.ContentAfterStorage)
	}
	if it.ContentAfterLocation == nil || !strings.HasPrefix(*it.ContentAfterLocation, ".wiki_commits/") {
		t.Fatalf("expected content snapshot location, got %v", it.ContentAfterLocation)
	}
}

func TestGetPageCommitDetailReadsContentSnapshot(t *testing.T) {
	newPageCommitTestDB(t)
	if err := dao.DB.AutoMigrate(&entity.Knowledgebase{}, &entity.User{}); err != nil {
		t.Fatalf("migrate page detail models: %v", err)
	}
	status := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.Knowledgebase{
		ID:         "kb1",
		TenantID:   "tenant1",
		Name:       "Knowledge Base",
		EmbdID:     "embedding",
		Permission: string(entity.TenantPermissionMe),
		CreatedBy:  "u1",
		ParserID:   "general",
		Status:     &status,
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := dao.DB.Create(&entity.User{
		ID:              "u1",
		Nickname:        "Tester",
		Email:           "tester@example.com",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	svc := NewFileCommitService()
	commit, err := svc.RecordPageEdit(t.Context(), PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "Page A",
		Comments:   "published",
		AuthorID:   "u1",
		OldContent: "old body",
		NewContent: "new body",
	})
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}

	detail, err := svc.GetPageCommitDetail(t.Context(), "kb1", commit.ID)
	if err != nil {
		t.Fatalf("GetPageCommitDetail: %v", err)
	}
	if detail.KBID != "kb1" || detail.TenantID != "tenant1" {
		t.Fatalf("unexpected scope: %+v", detail)
	}
	if detail.Title != "Page A" || detail.Comments != "published" || detail.UserNickname != "Tester" {
		t.Fatalf("unexpected metadata: %+v", detail)
	}
	if detail.ContentAfter != "new body" || !strings.Contains(detail.Diff, "new body") {
		t.Fatalf("unexpected content: %+v", detail)
	}
}

func TestRecordPageEdit_SecondEditLinksParent(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	base := PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "first",
		AuthorID:   "u1",
		NewContent: "initial content",
	}
	if _, err := svc.RecordPageEdit(ctx, base); err != nil {
		t.Fatalf("first RecordPageEdit: %v", err)
	}

	second := base
	second.Title = "second"
	second.OldContent = "initial content"
	second.NewContent = "hello world"
	commit2, err := svc.RecordPageEdit(ctx, second)
	if err != nil {
		t.Fatalf("second RecordPageEdit: %v", err)
	}
	if commit2.ParentID == nil {
		t.Fatal("second edit should link to the first commit as parent")
	}
	var first entity.FileCommit
	if err := dao.DB.Where("title = ?", "first").First(&first).Error; err != nil {
		t.Fatalf("load first commit: %v", err)
	}
	if *commit2.ParentID != first.ID {
		t.Fatalf("parent id mismatch: got %s want %s", *commit2.ParentID, first.ID)
	}
}

func TestRecordPageEdit_FirstGeneratedVersionUsesAdd(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()

	commit, err := svc.RecordPageEdit(t.Context(), PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		OldContent: "",
		NewContent: "initial content",
	})
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}
	var item entity.FileCommitItem
	if err := dao.DB.Where("commit_id = ?", commit.ID).First(&item).Error; err != nil {
		t.Fatalf("load commit item: %v", err)
	}
	if item.Operation != "add" {
		t.Fatalf("first generated version operation = %q, want add", item.Operation)
	}
}

func TestRecordPageEdit_SkipsNoop(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()

	commit, err := svc.RecordPageEdit(t.Context(), PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		OldContent: "same",
		NewContent: "same",
	})
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}
	if commit != nil {
		t.Fatalf("no-op edit created commit %s", commit.ID)
	}
}

func TestRecordPageEdit_IsolatesDatasets(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	mk := func(datasetID string) PageEditCommitInput {
		return PageEditCommitInput{
			DatasetID:  datasetID,
			Slug:       "page-a",
			PageType:   "wiki",
			Title:      datasetID + "-edit",
			AuthorID:   "u1",
			OldContent: "old",
			NewContent: "new",
		}
	}
	if _, err := svc.RecordPageEdit(ctx, mk("kb1")); err != nil {
		t.Fatalf("kb1 RecordPageEdit: %v", err)
	}
	if _, err := svc.RecordPageEdit(ctx, mk("kb2")); err != nil {
		t.Fatalf("kb2 RecordPageEdit: %v", err)
	}

	// A second edit in kb1 must parent to kb1's own first commit, not kb2's.
	kb1Again := mk("kb1")
	kb1Again.Title = "kb1-edit-2"
	kb1Again.OldContent = "old"
	kb1Again.NewContent = "newer"
	kb1Commit2, err := svc.RecordPageEdit(ctx, kb1Again)
	if err != nil {
		t.Fatalf("kb1 second RecordPageEdit: %v", err)
	}
	if kb1Commit2.ParentID == nil {
		t.Fatal("kb1 second edit should have a parent")
	}

	var kb1First, kb2First entity.FileCommit
	if err := dao.DB.Where("title = ?", "kb1-edit").First(&kb1First).Error; err != nil {
		t.Fatalf("load kb1 first commit: %v", err)
	}
	if err := dao.DB.Where("title = ?", "kb2-edit").First(&kb2First).Error; err != nil {
		t.Fatalf("load kb2 first commit: %v", err)
	}
	if *kb1Commit2.ParentID != kb1First.ID {
		t.Fatalf("kb1 parent mismatch: got %s want %s", *kb1Commit2.ParentID, kb1First.ID)
	}
	if *kb1Commit2.ParentID == kb2First.ID {
		t.Fatal("kb1 parent must not cross into kb2 history")
	}
}

func TestRecordPageEdit_ConcurrentEditsFormLinearChain(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	const edits = 8
	var wg sync.WaitGroup
	errs := make([]error, edits)
	for i := 0; i < edits; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			in := PageEditCommitInput{
				DatasetID:  "kb1",
				Slug:       "page-a",
				PageType:   "wiki",
				Title:      "edit-" + strconv.Itoa(idx),
				AuthorID:   "u1",
				OldContent: "old",
				NewContent: "new-" + strconv.Itoa(idx),
			}
			_, errs[idx] = svc.RecordPageEdit(ctx, in)
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("edit %d failed: %v", i, e)
		}
	}

	var commits []entity.FileCommit
	if err := dao.DB.Where("folder_id = ?", "kb1").Order("create_time ASC").Find(&commits).Error; err != nil {
		t.Fatalf("load commits: %v", err)
	}
	if len(commits) != edits {
		t.Fatalf("expected %d commits, got %d", edits, len(commits))
	}
	// Every commit except the first must have a parent, and the parents must
	// form a linear chain (no two commits share the same parent).
	seenParents := map[string]bool{}
	for i, c := range commits {
		if i == 0 {
			if c.ParentID != nil {
				t.Fatalf("first commit should have no parent")
			}
			continue
		}
		if c.ParentID == nil {
			t.Fatalf("commit %s should have a parent", c.ID)
		}
		if seenParents[*c.ParentID] {
			t.Fatalf("two commits share parent %s -> forked chain", *c.ParentID)
		}
		seenParents[*c.ParentID] = true
	}
}

// seedPageCommit writes one page commit with a stored body, bypassing
// RecordPageEdit so the timestamps (the anchor this is about) are exact.
func seedPageCommit(t *testing.T, id string, at int64, fileID, pageType, slug, location, body string) {
	t.Helper()
	seedPageCommitOp(t, id, at, fileID, pageType, slug, location, body, "modify")
}

// seedPageCommitOp is seedPageCommit with the operation spelled out, for the
// commits whose operation is what the test is about.
func seedPageCommitOp(t *testing.T, id string, at int64, fileID, pageType, slug, location, body, operation string) {
	t.Helper()
	if err := storage.GetStorageFactory().GetStorage().Put(t.Context(), "kb1", location, []byte(body)); err != nil {
		t.Fatalf("store body for %s: %v", id, err)
	}
	title := "edit " + id
	commit := &entity.FileCommit{
		ID:       id,
		FolderID: "kb1",
		Message:  title,
		AuthorID: "u1",
		Title:    &title,
	}
	commit.CreateTime = &at
	if err := dao.DB.Create(commit).Error; err != nil {
		t.Fatalf("create commit %s: %v", id, err)
	}
	item := &entity.FileCommitItem{
		ID:                   "item-" + id,
		CommitID:             id,
		FileID:               fileID,
		Operation:            operation,
		PageTypeKwd:          &pageType,
		SlugKwd:              &slug,
		ContentAfterLocation: &location,
	}
	item.CreateTime = &at
	if err := dao.DB.Create(item).Error; err != nil {
		t.Fatalf("create commit item %s: %v", id, err)
	}
}

// TestPageContentsAtReadsEachPageAtTheAnchor is what a restore asks: every page
// as it stood at the version, one body per page, taken from that page's last
// edit at or before the anchor.
func TestPageContentsAtReadsEachPageAtTheAnchor(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	seedPageCommit(t, "c1", 100, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h1", "page-a v1")
	seedPageCommit(t, "c2", 200, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h2", "page-a v2")
	seedPageCommit(t, "c3", 150, "kb1/wiki/page-b", "wiki", "page-b", ".wiki_commits/h3", "page-b v1")

	bodies := func(pages []PageContentAt) map[string]string {
		out := make(map[string]string, len(pages))
		for _, page := range pages {
			out[page.PageType+"/"+page.Slug] = page.Content
		}
		return out
	}

	pages, err := svc.PageContentsAt(ctx, "kb1", 200)
	if err != nil {
		t.Fatalf("PageContentsAt: %v", err)
	}
	got := bodies(pages)
	if len(got) != 2 || got["wiki/page-a"] != "page-a v2" || got["wiki/page-b"] != "page-b v1" {
		t.Fatalf("at anchor 200 got %+v, want page-a v2 and page-b v1", got)
	}

	// Before page-a's second edit the page still says what it said first; the
	// older commit is not simply skipped, it is what the page held then.
	pages, err = svc.PageContentsAt(ctx, "kb1", 199)
	if err != nil {
		t.Fatalf("PageContentsAt at 199: %v", err)
	}
	got = bodies(pages)
	if got["wiki/page-a"] != "page-a v1" || got["wiki/page-b"] != "page-b v1" {
		t.Fatalf("at anchor 199 got %+v, want page-a v1 and page-b v1", got)
	}

	// An anchor before every commit describes no pages at all.
	pages, err = svc.PageContentsAt(ctx, "kb1", 99)
	if err != nil {
		t.Fatalf("PageContentsAt at 99: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("at anchor 99 got %+v, want no pages", pages)
	}
}

// TestPageContentsAtSkipsPagesWithNoStoredBody keeps one bad commit from making
// a restore impossible: the writer tolerates a failed object write, so a commit
// can exist without bytes, and the rest of the pages still come back.
func TestPageContentsAtSkipsPagesWithNoStoredBody(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	seedPageCommit(t, "c1", 100, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h1", "page-a v1")

	title := "no body"
	commit := &entity.FileCommit{ID: "c2", FolderID: "kb1", Message: title, AuthorID: "u1", Title: &title}
	at := int64(200)
	commit.CreateTime = &at
	if err := dao.DB.Create(commit).Error; err != nil {
		t.Fatalf("create commit: %v", err)
	}
	pageType, slug := "wiki", "page-c"
	item := &entity.FileCommitItem{
		ID:          "item-c2",
		CommitID:    "c2",
		FileID:      "kb1/wiki/page-c",
		Operation:   "modify",
		PageTypeKwd: &pageType,
		SlugKwd:     &slug,
	}
	item.CreateTime = &at
	if err := dao.DB.Create(item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	pages, err := svc.PageContentsAt(ctx, "kb1", 300)
	if err != nil {
		t.Fatalf("PageContentsAt: %v", err)
	}
	if len(pages) != 1 || pages[0].Slug != "page-a" {
		t.Fatalf("got %+v, want only the page that has a body", pages)
	}
}

// TestPageContentsAtKeepsNestedSlugsAndTitles pins the identity a restore writes
// back: the slug is the tail the page API addresses (nested segments kept), and
// the title travels with it so the page comes back with the name it had.
func TestPageContentsAtKeepsNestedSlugsAndTitles(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()

	// slug_kwd is stored with its page-type prefix; the page API addresses the
	// bare tail.
	seedPageCommit(t, "c1", 100, "kb1/topic/People/Writers", "topic", "topic/People/Writers", ".wiki_commits/h1", "nested body")

	pages, err := svc.PageContentsAt(t.Context(), "kb1", 100)
	if err != nil {
		t.Fatalf("PageContentsAt: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %+v, want one page", pages)
	}
	if pages[0].PageType != "topic" || pages[0].Slug != "People/Writers" {
		t.Fatalf("got %+v, want topic/People/Writers", pages[0])
	}
	if pages[0].Title != "edit c1" || pages[0].Content != "nested body" {
		t.Fatalf("got %+v, want the commit title and body", pages[0])
	}
}

// TestPageSnapshotKeepsTheWholePage is what a restore replays: the body, and the
// summary, links and entities that went with it. A body-only snapshot cannot put
// the page back, only its text.
func TestPageSnapshotKeepsTheWholePage(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	if _, err := svc.RecordPageEdit(ctx, PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "Page A",
		AuthorID:   "u1",
		OldContent: "",
		NewContent: "# Page A",
		Page: map[string]any{
			"content_with_weight":  "# Page A",
			"title_kwd":            "Page A",
			"summary_with_weight":  "a summary",
			"topic_kwd":            "topics/x",
			"entity_names_kwd":     []string{"A"},
			"outlinks_kwd":         []string{"wiki/page-b"},
			"related_kb_pages_kwd": []string{"wiki/page-c"},
			"source_chunk_ids":     []string{"chunk-1"},
			"source_doc_ids":       []string{"doc-1"},
		},
	}); err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}

	pages, err := svc.PageContentsAt(ctx, "kb1", time.Now().UnixMilli()+1000)
	if err != nil {
		t.Fatalf("PageContentsAt: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %+v, want one page", pages)
	}
	page := pages[0]
	if page.Content != "# Page A" || page.Title != "Page A" {
		t.Fatalf("got content/title %q/%q, want the snapshot's", page.Content, page.Title)
	}
	if got := metadataStringOf(page.Page, "summary_with_weight"); got != "a summary" {
		t.Fatalf("summary = %q, want it kept", got)
	}
	if got := metadataStringOf(page.Page, "topic_kwd"); got != "topics/x" {
		t.Fatalf("topic = %q, want it kept", got)
	}
	outlinks, _ := page.Page["outlinks_kwd"].([]any)
	if len(outlinks) != 1 || outlinks[0] != "wiki/page-b" {
		t.Fatalf("outlinks = %+v, want the stored link", page.Page["outlinks_kwd"])
	}
	if got := metadataStringOf(page.Page, "source_doc_ids"); got == "" {
		t.Fatal("source doc ids were dropped from the snapshot")
	}
}

// TestPageContentsAtReadsLegacyBodyObject keeps the older half of the history
// readable: those objects are the body itself, with no snapshot envelope.
func TestPageContentsAtReadsLegacyBodyObject(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	// seedPageCommit writes the body raw, exactly as the history did before
	// snapshots existed.
	seedPageCommit(t, "c1", 100, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h1", "legacy body")

	pages, err := svc.PageContentsAt(ctx, "kb1", 100)
	if err != nil {
		t.Fatalf("PageContentsAt: %v", err)
	}
	if len(pages) != 1 || pages[0].Content != "legacy body" {
		t.Fatalf("got %+v, want the legacy object read as its body", pages)
	}
	if got := metadataStringOf(pages[0].Page, "content_with_weight"); got != "legacy body" {
		t.Fatalf("decoded content = %q, want the body", got)
	}
}

// seedPageDelete records a deletion: the item names the page but carries no
// body, because a delete describes the page's absence rather than its content.
func seedPageDelete(t *testing.T, id string, at int64, fileID, pageType, slug string) {
	t.Helper()
	commit := &entity.FileCommit{ID: id, FolderID: "kb1", Message: "delete " + id, AuthorID: "u1"}
	commit.CreateTime = &at
	if err := dao.DB.Create(commit).Error; err != nil {
		t.Fatalf("create delete commit %s: %v", id, err)
	}
	item := &entity.FileCommitItem{
		ID:          "item-" + id,
		CommitID:    id,
		FileID:      fileID,
		Operation:   "delete",
		PageTypeKwd: &pageType,
		SlugKwd:     &slug,
	}
	item.CreateTime = &at
	if err := dao.DB.Create(item).Error; err != nil {
		t.Fatalf("create delete item %s: %v", id, err)
	}
}

// TestPageContentsAtSkipsPagesDeletedAtTheAnchor pins the difference between "no
// edit yet" and "gone": a page whose latest commit at the anchor is a deletion
// did not exist then, so replaying its older body would resurrect it.
func TestPageContentsAtSkipsPagesDeletedAtTheAnchor(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	seedPageCommit(t, "c1", 100, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h1", "page-a v1")
	seedPageDelete(t, "c2", 200, "kb1/wiki/page-a", "wiki", "page-a")

	// Before the deletion the page still held its body.
	pages, err := svc.PageContentsAt(ctx, "kb1", 199)
	if err != nil {
		t.Fatalf("PageContentsAt at 199: %v", err)
	}
	if len(pages) != 1 || pages[0].Slug != "page-a" || pages[0].Content != "page-a v1" {
		t.Fatalf("at anchor 199 got %+v, want page-a v1", pages)
	}

	// At the anchor after it the version describes a dataset without the page.
	pages, err = svc.PageContentsAt(ctx, "kb1", 250)
	if err != nil {
		t.Fatalf("PageContentsAt at 250: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("at anchor 250 got %+v, want no page", pages)
	}

	// The deletion is still part of the page's history: the dataset knew about
	// the page, which is what a strict restore keys its guard on.
	keys, err := svc.PageKeysWithHistory(ctx, "kb1")
	if err != nil {
		t.Fatalf("PageKeysWithHistory: %v", err)
	}
	if _, ok := keys[PageKey("wiki", "page-a")]; !ok {
		t.Fatalf("history keys %+v do not mention the deleted page", keys)
	}
}

// TestPageContentsAtRecoversTheBodyBeforeTheFirstRecordedEdit is the case a page
// generated before page history existed falls into: the only commit is the edit
// being undone, so the body the version held lives in that edit's before-side.
func TestPageContentsAtRecoversTheBodyBeforeTheFirstRecordedEdit(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	if _, err := svc.RecordPageEdit(ctx, PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "Page A",
		AuthorID:   "u1",
		OldContent: "page-a v1\n",
		NewContent: "page-a v2\n",
	}); err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}

	// The commit exists, but only after the anchor: no commit at or before it.
	before, err := svc.PageContentsAt(ctx, "kb1", 1)
	if err != nil {
		t.Fatalf("PageContentsAt before the edit: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("got %+v, want the page the version held", before)
	}
	if before[0].PageType != "wiki" || before[0].Slug != "page-a" {
		t.Fatalf("got %q/%q, want wiki/page-a", before[0].PageType, before[0].Slug)
	}
	if before[0].Content != "page-a v1\n" {
		t.Fatalf("content = %q, want the body the edit started from", before[0].Content)
	}
	if got := metadataStringOf(before[0].Page, "content_with_weight"); got != "page-a v1\n" {
		t.Fatalf("snapshot content = %q, want the before-side body", got)
	}

	// The same anchor after the edit still resolves the edit's own body: the
	// fallback must not override a commit that is at or before the anchor.
	after, err := svc.PageContentsAt(ctx, "kb1", time.Now().UnixMilli()+1000)
	if err != nil {
		t.Fatalf("PageContentsAt after the edit: %v", err)
	}
	if len(after) != 1 || after[0].Content != "page-a v2\n" {
		t.Fatalf("got %+v, want page-a v2", after)
	}
}

// TestPageContentsUntilIncludesAPageAddedInsideTheWindow is the shape a parse
// run leaves: the version row is written when the pipeline reports the parse, and
// the pages that parse compiled are committed a moment later. Bounded by the next
// version, that creation belongs to the version and comes back with it.
func TestPageContentsUntilIncludesAPageAddedInsideTheWindow(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	seedPageCommitOp(t, "c1", 200, "kb1/concept/关圣帝君", "concept", "关圣帝君", ".wiki_commits/h1", "compiled body", "add")

	// The next version is at 300, so the addition is inside the window.
	pages, err := svc.PageContentsUntil(ctx, "kb1", 100, 300)
	if err != nil {
		t.Fatalf("PageContentsUntil: %v", err)
	}
	if len(pages) != 1 || pages[0].Content != "compiled body" {
		t.Fatalf("got %+v, want the compiled page", pages)
	}
	if pages[0].PageType != "concept" || pages[0].Slug != "关圣帝君" {
		t.Fatalf("got %q/%q, want concept/关圣帝君", pages[0].PageType, pages[0].Slug)
	}

	// A window that closes before the addition leaves the page to a later version:
	// the version at 100 did not hold it.
	if pages, err := svc.PageContentsUntil(ctx, "kb1", 100, 150); err != nil || len(pages) != 0 {
		t.Fatalf("narrow window got %+v, err %v, want no page", pages, err)
	}

	// Without a bound the caller asked for the strict reading - a page whose
	// history begins after the anchor did not exist then.
	if pages, err := svc.PageContentsAt(ctx, "kb1", 100); err != nil || len(pages) != 0 {
		t.Fatalf("unbounded got %+v, err %v, want no page", pages, err)
	}
}

// TestReverseApplyDiffUndoesAnEdit pins the reconstruction a page's first edit
// relies on: the before-side body comes back exactly, and a diff that does not
// belong to the body is refused instead of producing a mixture of both.
func TestReverseApplyDiffUndoesAnEdit(t *testing.T) {
	oldText := "one\ntwo\nthree\n"
	newText := "one\nTWO\nthree\n"
	diff := unifiedDiff(oldText, newText)
	if diff == "" {
		t.Fatal("expected a diff for a changed body")
	}
	if got, ok := reverseApplyDiff(newText, diff); !ok || got != oldText {
		t.Fatalf("reverse = %q ok=%v, want %q", got, ok, oldText)
	}

	// A deletion stores no body after the edit, and the whole removed text is
	// what the version held.
	removed := unifiedDiff("x\ny\n", "")
	got, ok := reverseApplyDiff("", removed)
	if !ok || got != "x\ny\n" {
		t.Fatalf("reverse of a delete = %q ok=%v, want the removed body", got, ok)
	}

	// A body the diff was not taken from is refused: applying it would write a
	// page nobody recorded.
	if _, ok := reverseApplyDiff("something else\n", diff); ok {
		t.Fatal("a diff was applied to a body it does not describe")
	}
	if _, ok := reverseApplyDiff(newText, "not a diff"); ok {
		t.Fatal("a diff with no hunk was applied")
	}
}

// TestPageDiffOfCommitAndAtAnchor keeps the diff a version entry replays
// reachable two ways: by the commit the entry recorded, and - for an entry that
// recorded none - by the page's newest commit at the version's anchor.
func TestPageDiffOfCommitAndAtAnchor(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	commit, err := svc.RecordPageEdit(ctx, PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "Page A",
		AuthorID:   "u1",
		OldContent: "a\nb\n",
		NewContent: "a\nc\n",
	})
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}
	if commit == nil {
		t.Fatal("RecordPageEdit recorded no commit for a changed page")
	}

	diff, err := svc.PageDiffOfCommit(ctx, commit.ID)
	if err != nil {
		t.Fatalf("PageDiffOfCommit: %v", err)
	}
	if !strings.Contains(diff, "-b") || !strings.Contains(diff, "+c") {
		t.Fatalf("diff %q does not describe the edit", diff)
	}

	// The anchor lookup answers with the same commit's diff: the page's newest
	// commit at or before the version's moment is the edit the version names.
	atAnchor, err := svc.PageDiffAt(ctx, "kb1", time.Now().UnixMilli()+1000, "wiki", "page-a")
	if err != nil {
		t.Fatalf("PageDiffAt: %v", err)
	}
	if atAnchor != diff {
		t.Fatalf("diff at anchor %q, want the commit's %q", atAnchor, diff)
	}

	// A page with no history, and a commit id that does not exist, answer with
	// nothing rather than an error: the entry keeps its sentence.
	if none, err := svc.PageDiffAt(ctx, "kb1", time.Now().UnixMilli()+1000, "wiki", "page-z"); err != nil || none != "" {
		t.Fatalf("unknown page diff = %q, err = %v, want nothing", none, err)
	}
	if none, err := svc.PageDiffOfCommit(ctx, "missing"); err != nil || none != "" {
		t.Fatalf("unknown commit diff = %q, err = %v, want nothing", none, err)
	}
}

// TestPageKeysWithHistoryListsEveryPageEverCommitted is the guard a strict page
// set restore needs: pages the history has recorded, deletions included, and
// nothing from another dataset.
func TestPageKeysWithHistoryListsEveryPageEverCommitted(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	seedPageCommit(t, "c1", 100, "kb1/wiki/page-a", "wiki", "page-a", ".wiki_commits/h1", "a")
	seedPageCommit(t, "c2", 200, "kb1/topic/People/Writers", "topic", "topic/People/Writers", ".wiki_commits/h2", "w")
	seedPageDelete(t, "c3", 300, "kb1/wiki/page-b", "wiki", "page-b")

	keys, err := svc.PageKeysWithHistory(ctx, "kb1")
	if err != nil {
		t.Fatalf("PageKeysWithHistory: %v", err)
	}
	for _, want := range []string{"wiki/page-a", "topic/People/Writers", "wiki/page-b"} {
		if _, ok := keys[want]; !ok {
			t.Fatalf("history keys %+v are missing %q", keys, want)
		}
	}
	if len(keys) != 3 {
		t.Fatalf("got %d keys, want 3: %+v", len(keys), keys)
	}

	// Another dataset's commit must not lend this one any history.
	if other, err := svc.PageKeysWithHistory(ctx, "kb2"); err != nil {
		t.Fatalf("PageKeysWithHistory kb2: %v", err)
	} else if len(other) != 0 {
		t.Fatalf("kb2 got %+v, want no history", other)
	}

	// An unnamed dataset asks about nothing and must not query.
	if none, err := svc.PageKeysWithHistory(ctx, ""); err != nil {
		t.Fatalf("PageKeysWithHistory empty: %v", err)
	} else if none == nil || len(none) != 0 {
		t.Fatalf("empty dataset got %+v, want an empty set", none)
	}
}

func TestSplitWikiPageFileID(t *testing.T) {
	pageType, slug := "wiki", "wiki/page-a"
	if gotType, gotSlug, ok := splitWikiPageFileID("kb1", wikiPageCommitRow{
		FileID:      "kb1/wiki/page-a",
		PageTypeKwd: sql.NullString{String: pageType, Valid: true},
		SlugKwd:     sql.NullString{String: slug, Valid: true},
	}); !ok || gotType != "wiki" || gotSlug != "page-a" {
		t.Fatalf("columns: got %q/%q ok=%v, want wiki/page-a", gotType, gotSlug, ok)
	}

	// Rows written before the page columns existed still name their page in the
	// file id.
	if gotType, gotSlug, ok := splitWikiPageFileID("kb1", wikiPageCommitRow{FileID: "kb1/topic/People/Writers"}); !ok ||
		gotType != "topic" || gotSlug != "People/Writers" {
		t.Fatalf("file id: got %q/%q ok=%v, want topic/People/Writers", gotType, gotSlug, ok)
	}

	// A row that names no page is reported unreadable rather than guessed at.
	if _, _, ok := splitWikiPageFileID("kb1", wikiPageCommitRow{FileID: "kb1"}); ok {
		t.Fatal("a bare dataset id should not read as a page")
	}
	if _, _, ok := splitWikiPageFileID("kb1", wikiPageCommitRow{FileID: "other/wiki/page"}); ok {
		t.Fatal("a file id outside the dataset should not read as a page")
	}
}

func TestUnifiedDiff_EmptyWhenNoChange(t *testing.T) {
	if d := unifiedDiff("same", "same"); d != "" {
		t.Fatalf("expected empty diff for identical text, got %q", d)
	}
}

func TestUnifiedDiff_DetectsAddition(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nb\nc\nd\n")
	if !strings.Contains(d, "@@") || !strings.Contains(d, "+d") {
		t.Fatalf("expected diff to contain added line, got %q", d)
	}
}

func TestUnifiedDiff_DetectsRemoval(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nc\n")
	if !strings.Contains(d, "@@") || !strings.Contains(d, "-b") {
		t.Fatalf("expected diff to contain removed line, got %q", d)
	}
}

func TestUnifiedDiff_UsesStandardHunkForLongDiff(t *testing.T) {
	oldLines := make([]string, 50)
	newLines := make([]string, 50)
	for i := range oldLines {
		oldLines[i] = "old-line"
		newLines[i] = "new-line"
	}
	d := unifiedDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if !strings.Contains(d, "--- a\n+++ b\n@@") {
		t.Fatalf("expected standard unified diff headers, got %q", d)
	}
	if !strings.Contains(d, "-old-line") || !strings.Contains(d, "+new-line") {
		t.Fatalf("expected removed and added lines, got %q", d)
	}
}

func TestListPageCommits_ReturnsRecordedEdits(t *testing.T) {
	db := newPageCommitTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	if err := db.Create(&entity.User{
		ID:              "u1",
		Nickname:        "Tester",
		Email:           "u1@example.com",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := NewFileCommitService()
	ctx := context.Background()

	base := PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "fireworks display",
		PageType:   "topic",
		AuthorID:   "u1",
		OldContent: "one",
		NewContent: "two",
	}
	first := base
	first.Title = "first edit"
	first.Comments = "first note"
	firstCommit, err := svc.RecordPageEdit(ctx, first)
	if err != nil {
		t.Fatalf("first RecordPageEdit: %v", err)
	}
	second := base
	second.Title = "second edit"
	second.Comments = "second note"
	second.OldContent = "two"
	second.NewContent = "three"
	secondCommit, err := svc.RecordPageEdit(ctx, second)
	if err != nil {
		t.Fatalf("second RecordPageEdit: %v", err)
	}

	// Pin both commit items to one timestamp so the newest-first assertions
	// below can only hold through the seq DESC tie-break, mirroring
	// same-millisecond edits in production.
	var firstItem entity.FileCommitItem
	if err := db.Where("commit_id = ?", firstCommit.ID).First(&firstItem).Error; err != nil {
		t.Fatalf("load first commit item: %v", err)
	}
	if firstItem.CreateTime == nil {
		t.Fatal("expected commit item create_time to be set")
	}
	if err := db.Model(&entity.FileCommitItem{}).
		Where("commit_id IN ?", []string{firstCommit.ID, secondCommit.ID}).
		UpdateColumn("create_time", *firstItem.CreateTime).Error; err != nil {
		t.Fatalf("pin equal create_time: %v", err)
	}

	rows, total, err := svc.ListPageCommits(ctx, "kb1", "topic", "fireworks display", 1, 15)
	if err != nil {
		t.Fatalf("ListPageCommits: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("expected 2 commits, got total=%d len=%d", total, len(rows))
	}
	if rows[0].ID != secondCommit.ID {
		t.Errorf("expected newest commit %s first, got %s", secondCommit.ID, rows[0].ID)
	}
	if rows[1].ID != firstCommit.ID {
		t.Errorf("expected oldest commit %s second, got %s", firstCommit.ID, rows[1].ID)
	}
	if rows[0].Title != "second edit" || rows[1].Title != "first edit" {
		t.Errorf("unexpected titles: %q / %q", rows[0].Title, rows[1].Title)
	}
	if rows[0].Comments != "second note" {
		t.Errorf("expected commit comments \"second note\", got %q", rows[0].Comments)
	}
	if rows[0].UserID != "u1" || rows[0].UserNickname != "Tester" {
		t.Errorf("expected user u1/Tester, got %q/%q", rows[0].UserID, rows[0].UserNickname)
	}
	if rows[0].CreateTime == nil {
		t.Error("expected create_time to be set")
	}
}

func TestListPageCommits_NestedSlugMatchesRecordPageEditKey(t *testing.T) {
	db := newPageCommitTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	if err := db.Create(&entity.User{
		ID:              "u1",
		Nickname:        "Tester",
		Email:           "u1@example.com",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := NewFileCommitService()
	ctx := context.Background()

	// UpdateArtifact routes PUT /artifacts/topic/People/Writers with
	// pageType="topic" and the full nested tail as the slug.
	nested := PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "People/Writers",
		PageType:   "topic",
		AuthorID:   "u1",
		OldContent: "one",
		NewContent: "two",
	}
	first := nested
	first.Title = "nested first"
	firstCommit, err := svc.RecordPageEdit(ctx, first)
	if err != nil {
		t.Fatalf("first RecordPageEdit: %v", err)
	}
	second := nested
	second.Title = "nested second"
	second.OldContent = "two"
	second.NewContent = "three"
	secondCommit, err := svc.RecordPageEdit(ctx, second)
	if err != nil {
		t.Fatalf("second RecordPageEdit: %v", err)
	}
	if secondCommit.ParentID == nil || *secondCommit.ParentID != firstCommit.ID {
		t.Fatalf("expected nested page edits to chain via the same file key, got parent %v", secondCommit.ParentID)
	}

	// The write side must scope the page file key exactly as the read side
	// recomputes it after the handler's first-segment split.
	var items []entity.FileCommitItem
	if err := dao.DB.Where("commit_id = ?", secondCommit.ID).Find(&items).Error; err != nil {
		t.Fatalf("load nested items: %v", err)
	}
	if len(items) != 1 || items[0].FileID != "kb1/topic/People/Writers" {
		t.Fatalf("expected file_id kb1/topic/People/Writers, got %+v", items)
	}

	// A page that only shares the first nested segment has its own history.
	sibling := nested
	sibling.Slug = "People/Editors"
	sibling.Title = "sibling page"
	siblingCommit, err := svc.RecordPageEdit(ctx, sibling)
	if err != nil {
		t.Fatalf("sibling RecordPageEdit: %v", err)
	}

	rows, total, err := svc.ListPageCommits(ctx, "kb1", "topic", "People/Writers", 1, 15)
	if err != nil {
		t.Fatalf("ListPageCommits: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("expected 2 commits, got total=%d len=%d", total, len(rows))
	}
	if rows[0].ID != secondCommit.ID || rows[1].ID != firstCommit.ID {
		t.Errorf("expected newest-first %s, %s; got %s, %s", secondCommit.ID, firstCommit.ID, rows[0].ID, rows[1].ID)
	}
	for _, row := range rows {
		if row.ID == siblingCommit.ID {
			t.Errorf("sibling page history leaked into nested slug listing: %s", row.ID)
		}
	}
}

func TestListPageCommits_ResolvesNicknamesInSingleBatchedLookup(t *testing.T) {
	db := newPageCommitTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	for _, u := range []entity.User{
		{ID: "u1", Nickname: "Tester", Email: "u1@example.com", IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0"},
		{ID: "u2", Nickname: "Reviewer", Email: "u2@example.com", IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0"},
	} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed user %s: %v", u.ID, err)
		}
	}

	var queries []string
	if err := db.Callback().Query().After("gorm:query").Register("count_user_lookups", func(tx *gorm.DB) {
		queries = append(queries, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	svc := NewFileCommitService()
	ctx := context.Background()
	base := PageEditCommitInput{
		DatasetID:  "kb1",
		Slug:       "fireworks display",
		PageType:   "topic",
		OldContent: "one",
	}
	for i, author := range []string{"u1", "u2", "u3"} {
		in := base
		in.AuthorID = author
		in.NewContent = strconv.Itoa(i + 2)
		if _, err := svc.RecordPageEdit(ctx, in); err != nil {
			t.Fatalf("RecordPageEdit by %s: %v", author, err)
		}
	}

	queries = nil
	rows, total, err := svc.ListPageCommits(ctx, "kb1", "topic", "fireworks display", 1, 15)
	if err != nil {
		t.Fatalf("ListPageCommits: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("expected 3 commits, got total=%d len=%d", total, len(rows))
	}
	// Three distinct authors must be resolved by exactly one query against
	// the user table (u3 has no user row and degrades to an empty nickname).
	userQueries := 0
	for _, q := range queries {
		if strings.Contains(q, "FROM `user`") {
			userQueries++
		}
	}
	if userQueries != 1 {
		t.Errorf("expected exactly 1 batched user lookup for 3 distinct authors, got %d (queries: %v)", userQueries, queries)
	}
	wantNicknames := map[string]string{"u1": "Tester", "u2": "Reviewer", "u3": ""}
	for _, row := range rows {
		if row.UserNickname != wantNicknames[row.UserID] {
			t.Errorf("author %s: expected nickname %q, got %q", row.UserID, wantNicknames[row.UserID], row.UserNickname)
		}
	}
}
