package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func batchMetaDocs(n int, createTime int64) []*entity.DocumentListItem {
	docs := make([]*entity.DocumentListItem, 0, n)
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("doc-%d", i)
		name := id + ".pdf"
		ct := createTime
		docs = append(docs, &entity.DocumentListItem{
			ID: id, KbID: "ds-1", ParserID: "naive", ParserConfig: "{}",
			SourceType: "local", Type: "document", CreatedBy: "user-1",
			Name: &name, Suffix: "pdf", CreateTime: &ct,
		})
	}
	return docs
}

func listDocsResponse(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var response struct {
		Data struct {
			Docs []map[string]interface{} `json:"docs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return response.Data.Docs
}

// TestListDocumentsHandlerBatchesMetadataLookups is the RED test for §2.2:
// with five documents on one page the old code issued five metadata queries;
// the batched path must issue exactly one.
func TestListDocumentsHandlerBatchesMetadataLookups(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{
		documentList:      batchMetaDocs(5, 100),
		documentListTotal: 5,
		metadataByIDsResult: map[string]map[string]interface{}{
			"doc-1": {"author": "A"},
			"doc-2": {"author": "B"},
			"doc-3": {"author": "C"},
			"doc-4": {"author": "D"},
			"doc-5": {"author": "E"},
		},
	}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}
	c, w := setupHandlerAccessContext("GET", "/api/v1/datasets/ds-1/documents", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.ListDocuments(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.metadataByIDsCalls != 1 {
		t.Fatalf("GetDocumentMetadataByIDs calls = %d, want 1 (single query for the whole page)", fake.metadataByIDsCalls)
	}
	if fake.metadataSingleCalls != 0 {
		t.Fatalf("GetDocumentMetadataByID calls = %d, want 0 (per-document N+1 must be gone)", fake.metadataSingleCalls)
	}
	if fake.metadataByIDsKBID != "ds-1" {
		t.Fatalf("metadata kbID = %q, want ds-1", fake.metadataByIDsKBID)
	}
	wantIDs := []string{"doc-1", "doc-2", "doc-3", "doc-4", "doc-5"}
	if !reflect.DeepEqual(fake.metadataByIDsDocIDs, wantIDs) {
		t.Fatalf("metadata docIDs = %v, want %v", fake.metadataByIDsDocIDs, wantIDs)
	}
	docs := listDocsResponse(t, w)
	if len(docs) != 5 {
		t.Fatalf("docs = %d, want 5", len(docs))
	}
	for i, doc := range docs {
		fields, ok := doc["meta_fields"].(map[string]interface{})
		if !ok {
			t.Fatalf("doc %d meta_fields missing: %#v", i, doc)
		}
		want := string(rune('A' + i))
		if fields["author"] != want {
			t.Fatalf("doc %d meta_fields = %#v, want author=%s", i, fields, want)
		}
	}
}

func TestListDocumentsHandlerBatchEmptyList(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{documentList: nil, documentListTotal: 0}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}
	c, w := setupHandlerAccessContext("GET", "/api/v1/datasets/ds-1/documents", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.ListDocuments(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.metadataByIDsCalls != 0 || fake.metadataSingleCalls != 0 {
		t.Fatalf("empty list: batch=%d single=%d, want 0/0", fake.metadataByIDsCalls, fake.metadataSingleCalls)
	}
}

// TestListDocumentsHandlerBatchMetadataErrorStillEmitsEmptyFields locks the
// fallback: a failed batch must still yield empty meta_fields per document,
// never nil (which would change the JSON shape).
func TestListDocumentsHandlerBatchMetadataErrorStillEmitsEmptyFields(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{
		documentList:      batchMetaDocs(2, 100),
		documentListTotal: 2,
		metadataByIDsErr:  fmt.Errorf("metadata index unavailable"),
	}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}
	c, w := setupHandlerAccessContext("GET", "/api/v1/datasets/ds-1/documents", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.ListDocuments(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	docs := listDocsResponse(t, w)
	if len(docs) != 2 {
		t.Fatalf("docs = %d, want 2 (otherwise the metadata fallback is not exercised)", len(docs))
	}
	for i, doc := range docs {
		fields, ok := doc["meta_fields"].(map[string]interface{})
		if !ok {
			t.Fatalf("doc %d meta_fields must be an object after metadata error: %#v", i, doc["meta_fields"])
		}
		if len(fields) != 0 {
			t.Fatalf("doc %d meta_fields = %#v, want empty after metadata error", i, fields)
		}
	}
}

// TestListDocumentsHandlerBatchExcludesFilteredDocs is adversarial: the
// create_time filter runs before metadata reads, so a filtered-out document
// must not be included in the batch query.
func TestListDocumentsHandlerBatchExcludesFilteredDocs(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	oldDoc := batchMetaDocs(1, 50)[0]
	newDoc := batchMetaDocs(1, 300)[0]
	newDoc.ID = "doc-kept"
	fake := &fakeDocumentService{
		documentList:      []*entity.DocumentListItem{oldDoc, newDoc},
		documentListTotal: 2,
		metadataByIDsResult: map[string]map[string]interface{}{
			"doc-1":    {"author": "old"},
			"doc-kept": {"author": "kept"},
		},
	}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}
	c, w := setupHandlerAccessContext("GET", "/api/v1/datasets/ds-1/documents?create_time_from=200", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.ListDocuments(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(fake.metadataByIDsDocIDs, []string{"doc-kept"}) {
		t.Fatalf("metadata docIDs = %v, want [doc-kept] (filtered doc must be excluded)", fake.metadataByIDsDocIDs)
	}
}
