package dataset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
)

func testDatasetServiceForAggregateTags(t *testing.T, loader func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error)) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:               dao.NewKnowledgebaseDAO(),
		documentDAO:         dao.NewDocumentDAO(),
		tagVocabularyLoader: loader,
	}
}

// insertAggregateTagsDoc seeds a document whose parser_config declares a tag
// source file at the document level, mirroring what the document parser dialog
// writes. An empty tagFileID seeds the inherited dataset-level shape instead.
func insertAggregateTagsDoc(t *testing.T, docID, datasetID, tagFileID string) {
	t.Helper()
	parserConfig := entity.JSONMap{}
	if tagFileID != "" {
		parserConfig["Extractor:AutoExtractDefault"] = map[string]any{
			"tags": map[string]any{"tag_file_id": tagFileID, "top_n": 3},
		}
	}
	doc := &entity.Document{
		ID:           docID,
		KbID:         datasetID,
		Name:         sptr("doc-" + docID[:6]),
		CreatedBy:    datasetID,
		ParserConfig: parserConfig,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test document: %v", err)
	}
}

func insertAggregateTagsKB(t *testing.T, datasetID, tenantID, permission, tagFileID string, docNum int64) {
	t.Helper()
	parserConfig := entity.JSONMap{}
	if tagFileID != "" {
		parserConfig["tags"] = map[string]any{"tag_file_id": tagFileID}
	}
	kb := &entity.Knowledgebase{
		ID:           datasetID,
		TenantID:     tenantID,
		Name:         "kb-" + datasetID[:6],
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    tenantID,
		Permission:   permission,
		ParserID:     "naive",
		ParserConfig: parserConfig,
		DocNum:       docNum,
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func insertAggregateTagsMembership(t *testing.T, tenantID, userID string) {
	t.Helper()
	row := &entity.UserTenant{
		ID:        tenantID + "-" + userID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "member",
		InvitedBy: tenantID,
		Status:    sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(row).Error; err != nil {
		t.Fatalf("insert user_tenant: %v", err)
	}
}

func aggregateTagsResultMap(rows []map[string]interface{}) map[string]int {
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		tag, _ := row["value"].(string)
		count, _ := row["count"].(int)
		result[tag] = count
	}
	return result
}

// TestDatasetServiceAggregateTagsSeedsFromTagFile verifies that the tag
// vocabulary (and its counts) come solely from the configured tag source file,
// with no chunk-level aggregation. The file is configured on the dataset and
// reaches the endpoint through the copy upload puts on its document — the
// endpoint reads documents, not the dataset row.
func TestDatasetServiceAggregateTagsSeedsFromTagFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsDoc(t, "doc-seed", kbID, "file-1")

	loader := func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if tagFileID == "file-1" {
			return map[string]int{"finance": 2, "urgent": 1}, nil
		}
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	want := map[string]int{"finance": 2, "urgent": 1}
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}
}

// TestDatasetServiceAggregateTagsMergesAcrossTagFiles verifies that counts from
// multiple datasets' tag source files are merged by tag name.
func TestDatasetServiceAggregateTagsMergesAcrossTagFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kb1Input := "123e4567-e89b-12d3-a456-426614174000"
	kb2Input := "223e4567-e89b-12d3-a456-426614174001"
	kb1ID := strings.ReplaceAll(kb1Input, "-", "")
	kb2ID := strings.ReplaceAll(kb2Input, "-", "")
	insertAggregateTagsKB(t, kb1ID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsKB(t, kb2ID, "user-1", string(entity.TenantPermissionMe), "file-2", 0)
	insertAggregateTagsDoc(t, "doc-merge-1", kb1ID, "file-1")
	insertAggregateTagsDoc(t, "doc-merge-2", kb2ID, "file-2")

	loader := func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		switch tagFileID {
		case "file-1":
			return map[string]int{"finance": 2, "urgent": 1}, nil
		case "file-2":
			return map[string]int{"finance": 1, "urgent": 3, "internal": 1}, nil
		}
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kb1Input, kb2Input}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	want := map[string]int{"finance": 3, "urgent": 4, "internal": 1}
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}
}

// TestDatasetServiceAggregateTagsEmptyWhenNoTagFile verifies that a dataset
// without a configured tag source file yields an empty vocabulary and does not
// invoke the loader.
func TestDatasetServiceAggregateTagsEmptyWhenNoTagFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "323e4567-e89b-12d3-a456-426614174002"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "", 0)

	loader := func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		t.Fatalf("loader should not be called when no tag_file_id is set")
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(result) != 0 {
		t.Fatalf("result=%v want empty", result)
	}
}

func TestDatasetServiceAggregateTagsRejectsUnauthorizedDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "423e4567-e89b-12d3-a456-426614174003"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "tenant-9", string(entity.TenantPermissionMe), "", 0)
	ctx := t.Context()

	_, code, err := testDatasetServiceForAggregateTags(t, nil).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want=%d", code, common.CodeDataError)
	}
	if err.Error() != "No authorization for dataset '"+kbID+"'" {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDatasetServiceAggregateTagsScopesLoaderToDatasetTenant verifies the loader
// receives the dataset's tenant — the authorization scope used to resolve the
// user-writable tag_file_id — rather than the caller's user ID.
func TestDatasetServiceAggregateTagsScopesLoaderToDatasetTenant(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "523e4567-e89b-12d3-a456-426614174004"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	const kbTenant = "tenant-owner"
	insertAggregateTagsKB(t, kbID, kbTenant, string(entity.TenantPermissionTeam), "file-foreign", 0)
	insertAggregateTagsDoc(t, "doc-scope", kbID, "file-foreign")
	insertAggregateTagsMembership(t, kbTenant, "user-1")
	ctx := t.Context()

	var gotOwner string
	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if tagFileID != "file-foreign" {
			t.Fatalf("tagFileID = %q, want file-foreign", tagFileID)
		}
		gotOwner = ownerTenantID
		return map[string]int{"finance": 1}, nil
	}

	if _, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kbInput}, "user-1"); err != nil {
		t.Fatalf("AggregateTags failed: code=%d err=%v", code, err)
	}
	if gotOwner != kbTenant {
		t.Fatalf("loader ownerTenantID = %q, want %q (must be the dataset tenant, not the user)", gotOwner, kbTenant)
	}
}

// assertTagCounts fails unless the aggregated result holds exactly want.
func assertTagCounts(t *testing.T, result []map[string]interface{}, want map[string]int) {
	t.Helper()
	got := aggregateTagsResultMap(result)
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}
}

// TestDatasetServiceAggregateTagsReadsDocumentTagFile is the reported
// scenario: the dataset keeps the template default (empty tag_file_id) while
// the tag source is declared on a document by the document parser dialog. The
// extractor reads that document-level copy at parse time, so aggregation must
// return its vocabulary as well.
func TestDatasetServiceAggregateTagsReadsDocumentTagFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "623e4567-e89b-12d3-a456-426614174005"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "", 0)
	insertAggregateTagsDoc(t, "doc-tag-a", kbID, "file-1")

	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if ownerTenantID != "user-1" {
			t.Fatalf("ownerTenantID = %q, want user-1", ownerTenantID)
		}
		if tagFileID == "file-1" {
			return map[string]int{"finance": 2, "urgent": 1}, nil
		}
		return nil, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	assertTagCounts(t, result, map[string]int{"finance": 2, "urgent": 1})
}

// TestDatasetServiceAggregateTagsCountsSnapshotOnce: every document inherits
// the dataset's tag source at upload, so the same file is reached both from
// the dataset and from each document — it must be loaded and counted once.
func TestDatasetServiceAggregateTagsCountsSnapshotOnce(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "723e4567-e89b-12d3-a456-426614174006"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsDoc(t, "doc-tag-b", kbID, "file-1")
	insertAggregateTagsDoc(t, "doc-tag-c", kbID, "file-1")

	loads := 0
	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if tagFileID == "file-1" {
			loads++
			return map[string]int{"finance": 2}, nil
		}
		return nil, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if loads != 1 {
		t.Fatalf("loader called %d times, want 1", loads)
	}
	assertTagCounts(t, result, map[string]int{"finance": 2})
}

// TestDatasetServiceAggregateTagsUnionsDocumentTagFiles: documents may
// override the dataset's tag source individually, so a dataset can be served
// by more than one file and all of their vocabularies must be merged.
func TestDatasetServiceAggregateTagsUnionsDocumentTagFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "823e4567-e89b-12d3-a456-426614174007"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsDoc(t, "doc-tag-d", kbID, "file-1")
	insertAggregateTagsDoc(t, "doc-tag-e", kbID, "file-2")

	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		switch tagFileID {
		case "file-1":
			return map[string]int{"finance": 2}, nil
		case "file-2":
			return map[string]int{"urgent": 3, "internal": 1}, nil
		}
		return nil, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	assertTagCounts(t, result, map[string]int{"finance": 2, "urgent": 3, "internal": 1})
}

// TestDatasetServiceAggregateTagsCountsSharedFileOnce: two datasets of the
// same tenant can declare the same source file. The count for a tag is the
// number of times it occurs in that file, so the file is loaded once for the
// whole request rather than once per dataset.
func TestDatasetServiceAggregateTagsCountsSharedFileOnce(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kb1Input := "923e4567-e89b-12d3-a456-426614174008"
	kb2Input := "a23e4567-e89b-12d3-a456-426614174009"
	kb1ID := strings.ReplaceAll(kb1Input, "-", "")
	kb2ID := strings.ReplaceAll(kb2Input, "-", "")
	insertAggregateTagsKB(t, kb1ID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsKB(t, kb2ID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsDoc(t, "doc-shared-1", kb1ID, "file-1")
	insertAggregateTagsDoc(t, "doc-shared-2", kb2ID, "file-1")

	loads := 0
	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if tagFileID == "file-1" {
			loads++
			return map[string]int{"finance": 2}, nil
		}
		return nil, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kb1Input, kb2Input}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if loads != 1 {
		t.Fatalf("loader called %d times, want 1", loads)
	}
	assertTagCounts(t, result, map[string]int{"finance": 2})
}

// missingTagSourceLoader resolves ids that no longer exist through the real
// loader, so the error carries the production sentinel rather than a stub's
// approximation of it.
func missingTagSourceLoader(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
	return component.TagVocabularyFromTagFileID(ctx, tagFileID, ownerTenantID)
}

// TestDatasetServiceAggregateTagsSkipsUnresolvableDocumentTagSource is the
// availability regression the review flagged: a document keeps referencing a
// tag file that has been deleted, and nothing clears that reference. One dead
// document-level source must not discard the vocabulary of every other dataset
// in the same request.
func TestDatasetServiceAggregateTagsSkipsUnresolvableDocumentTagSource(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	deadKB := "b23e4567-e89b-12d3-a456-426614174010"
	liveKB := "c23e4567-e89b-12d3-a456-426614174011"
	insertAggregateTagsKB(t, strings.ReplaceAll(deadKB, "-", ""), "user-1", string(entity.TenantPermissionMe), "", 0)
	insertAggregateTagsDoc(t, "doc-dead", strings.ReplaceAll(deadKB, "-", ""), "file-dead")
	insertAggregateTagsKB(t, strings.ReplaceAll(liveKB, "-", ""), "user-1", string(entity.TenantPermissionMe), "", 0)
	insertAggregateTagsDoc(t, "doc-live", strings.ReplaceAll(liveKB, "-", ""), "file-live")

	loader := func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		if tagFileID == "file-dead" {
			return missingTagSourceLoader(ctx, tagFileID, ownerTenantID)
		}
		return map[string]int{"finance": 2}, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{deadKB, liveKB}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	assertTagCounts(t, result, map[string]int{"finance": 2})
}

// TestDatasetServiceAggregateTagsIgnoresUnappliedDatasetTagSource pins the
// chosen semantics: this endpoint reports what is in effect, not what is
// configured. A source only the dataset row carries has not been applied to
// any chunk, so it is not reported. That is the deliberate cost of answering
// at document level — a dataset configured after its documents were uploaded
// reads empty until those documents are re-uploaded or configured individually.
func TestDatasetServiceAggregateTagsIgnoresUnappliedDatasetTagSource(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "d23e4567-e89b-12d3-a456-426614174012"
	insertAggregateTagsKB(t, strings.ReplaceAll(kbInput, "-", ""), "user-1", string(entity.TenantPermissionMe), "file-1", 0)

	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		t.Fatalf("loader must not be called for a source no document carries, got %q", tagFileID)
		return nil, nil
	}

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(result) != 0 {
		t.Fatalf("result=%v want empty: a dataset-level source with no document carrying it is not in effect", result)
	}
}

// TestDatasetServiceAggregateTagsFailsOnStorageErrorForDocumentTagSource pins
// the boundary of the skip: only a source that cannot be resolved by id may be
// dropped. A storage-layer failure says nothing about that id and must stay a
// hard failure even for a document-level source.
func TestDatasetServiceAggregateTagsFailsOnStorageErrorForDocumentTagSource(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "e23e4567-e89b-12d3-a456-426614174013"
	insertAggregateTagsKB(t, strings.ReplaceAll(kbInput, "-", ""), "user-1", string(entity.TenantPermissionMe), "", 0)
	insertAggregateTagsDoc(t, "doc-storage", strings.ReplaceAll(kbInput, "-", ""), "file-1")

	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		return nil, errors.New("tag source file: no storage backend registered")
	}

	_, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1")
	if err == nil {
		t.Fatal("expected an error for a storage failure on a document-level tag source")
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d want=%d", code, common.CodeServerError)
	}
}

// TestDatasetServiceAggregateTagsScopesDocumentTagSourceToDatasetTenant: the
// document-level id is reached through the same union, so it must be resolved
// against the dataset's tenant rather than the caller's (IDOR, CWE-639).
func TestDatasetServiceAggregateTagsScopesDocumentTagSourceToDatasetTenant(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "f23e4567-e89b-12d3-a456-426614174014"
	const kbTenant = "tenant-owner"
	insertAggregateTagsKB(t, strings.ReplaceAll(kbInput, "-", ""), kbTenant, string(entity.TenantPermissionTeam), "", 0)
	insertAggregateTagsMembership(t, kbTenant, "user-1")
	insertAggregateTagsDoc(t, "doc-owned", strings.ReplaceAll(kbInput, "-", ""), "file-foreign")

	var gotOwner string
	loader := func(_ context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
		gotOwner = ownerTenantID
		return map[string]int{"finance": 1}, nil
	}

	if _, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(t.Context(), []string{kbInput}, "user-1"); err != nil {
		t.Fatalf("AggregateTags failed: code=%d err=%v", code, err)
	}
	if gotOwner != kbTenant {
		t.Fatalf("loader ownerTenantID = %q, want %q (document sources must resolve against the dataset tenant)", gotOwner, kbTenant)
	}
}
