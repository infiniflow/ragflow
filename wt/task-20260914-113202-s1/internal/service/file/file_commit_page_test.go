package file

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

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
	t.Cleanup(func() { dao.DB = old })
	return db
}

func TestRecordPageEdit_CreatesCommitAndItem(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	in := PageEditCommitInput{
		DatasetID:  "kb1",
		DocID:      "wiki/page-a",
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
	if it.ContentAfterStorage == nil || *it.ContentAfterStorage != "es" {
		t.Fatalf("expected content_after_storage es, got %v", it.ContentAfterStorage)
	}
	if it.ContentAfterLocation == nil || *it.ContentAfterLocation != "wiki/page-a" {
		t.Fatalf("expected content_after_location wiki/page-a, got %v", it.ContentAfterLocation)
	}
}

func TestRecordPageEdit_SecondEditLinksParent(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	base := PageEditCommitInput{
		DatasetID: "kb1",
		DocID:     "wiki/page-a",
		Slug:      "page-a",
		PageType:  "wiki",
		Title:     "first",
		AuthorID:  "u1",
	}
	if _, err := svc.RecordPageEdit(ctx, base); err != nil {
		t.Fatalf("first RecordPageEdit: %v", err)
	}

	second := base
	second.Title = "second"
	second.OldContent = "hello"
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

func TestRecordPageEdit_IsolatesDatasets(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := t.Context()

	mk := func(datasetID string) PageEditCommitInput {
		return PageEditCommitInput{
			DatasetID:  datasetID,
			DocID:      datasetID + "/wiki/page-a",
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
				DocID:      "wiki/page-a",
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

func TestUnifiedDiff_EmptyWhenNoChange(t *testing.T) {
	if d := unifiedDiff("same", "same"); d != "" {
		t.Fatalf("expected empty diff for identical text, got %q", d)
	}
}

func TestUnifiedDiff_DetectsAddition(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nb\nc\nd\n")
	if !strings.Contains(d, "+d") {
		t.Fatalf("expected diff to contain added line, got %q", d)
	}
}

func TestUnifiedDiff_DetectsRemoval(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nc\n")
	if !strings.Contains(d, "-b") {
		t.Fatalf("expected diff to contain removed line, got %q", d)
	}
}

func TestUnifiedDiff_TruncatesLongDiff(t *testing.T) {
	oldLines := make([]string, 50)
	newLines := make([]string, 50)
	for i := range oldLines {
		oldLines[i] = "old-line"
		newLines[i] = "new-line"
	}
	d := unifiedDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if !strings.Contains(d, "... ") || !strings.Contains(d, "lines omitted") {
		t.Fatalf("expected long diff to be truncated, got %q", d)
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
		DocID:      "topic/fireworks display",
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
		DocID:      "topic/People/Writers",
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
	sibling.DocID = "topic/People/Editors"
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
		DocID:      "topic/fireworks display",
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
