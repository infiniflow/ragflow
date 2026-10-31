package document

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

func TestBatchUpdateDocumentMetadatasSelection(t *testing.T) {
	cases := []struct {
		name     string
		selector *MetadataSelector
		wantIDs  []string
	}{
		{name: "nil selector selects all dataset documents", wantIDs: []string{"doc-1", "doc-2", "doc-without-metadata"}},
		{name: "empty selector selects all dataset documents", selector: &MetadataSelector{}, wantIDs: []string{"doc-1", "doc-2", "doc-without-metadata"}},
		{name: "explicit empty IDs select nothing", selector: &MetadataSelector{DocumentIDs: []string{}}},
		{name: "condition selects matching dataset documents", selector: &MetadataSelector{MetadataCondition: makeMetadataSelectionCondition("ready")}, wantIDs: []string{"doc-1"}},
		{name: "condition with no matches selects nothing", selector: &MetadataSelector{MetadataCondition: makeMetadataSelectionCondition("missing")}},
		{name: "empty IDs remain empty with matching condition", selector: &MetadataSelector{DocumentIDs: []string{}, MetadataCondition: makeMetadataSelectionCondition("ready")}},
		{name: "IDs and conditions select their intersection", selector: &MetadataSelector{DocumentIDs: []string{"doc-1", "doc-2"}, MetadataCondition: makeMetadataSelectionCondition("ready")}, wantIDs: []string{"doc-1"}},
		{name: "IDs and conditions can have an empty intersection", selector: &MetadataSelector{DocumentIDs: []string{"doc-2"}, MetadataCondition: makeMetadataSelectionCondition("ready")}},
		{name: "explicit ID selects document without metadata", selector: &MetadataSelector{DocumentIDs: []string{"doc-without-metadata"}}, wantIDs: []string{"doc-without-metadata"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newMetadataSelectionTestService(t)
			wantRecords := cloneMetadataSelectionRecords(store.records)
			for _, docID := range tc.wantIDs {
				if wantRecords[docID] == nil {
					wantRecords[docID] = make(map[string]interface{})
				}
				wantRecords[docID]["selected"] = "yes"
			}
			resp, code, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-1", tc.selector,
				[]MetadataUpdate{{Key: "selected", Value: "yes"}}, nil)
			if err != nil || code != common.CodeSuccess || resp == nil {
				t.Fatalf("BatchUpdateDocumentMetadatas() response=%#v code=%v error=%v", resp, code, err)
			}
			if resp.Updated != len(tc.wantIDs) || resp.MatchedDocs != len(tc.wantIDs) {
				t.Errorf("response = %#v, want updated=%d matched_docs=%d", resp, len(tc.wantIDs), len(tc.wantIDs))
			}
			if !reflect.DeepEqual(store.records, wantRecords) {
				t.Errorf("metadata records = %#v, want %#v", store.records, wantRecords)
			}
			writtenIDs := make([]string, 0, len(store.writeCalls))
			for _, call := range store.writeCalls {
				if call.datasetID != "kb-1" || call.tenantID != "tenant-1" {
					t.Errorf("metadata write crossed dataset/tenant boundary: %#v", call)
				}
				writtenIDs = append(writtenIDs, call.documentID)
			}
			slices.Sort(writtenIDs)
			wantIDs := slices.Clone(tc.wantIDs)
			slices.Sort(wantIDs)
			if !slices.Equal(writtenIDs, wantIDs) {
				t.Errorf("written document IDs = %#v, want %#v", writtenIDs, wantIDs)
			}
			if len(tc.wantIDs) == 0 && store.mutations != 0 {
				t.Errorf("metadata mutations = %d, want 0", store.mutations)
			}
		})
	}
}

func TestBatchUpdateDocumentMetadatasEmptyDatasetSelection(t *testing.T) {
	cases := []struct {
		name     string
		selector *MetadataSelector
	}{
		{name: "nil selector"},
		{name: "empty selector", selector: &MetadataSelector{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newMetadataSelectionTestService(t)
			insertTestKB(t, "kb-empty", "tenant-1", 0, 0, 0)
			before := cloneMetadataSelectionRecords(store.records)
			resp, code, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-empty", tc.selector,
				[]MetadataUpdate{{Key: "selected", Value: "yes"}}, nil)
			if err != nil || code != common.CodeSuccess || resp == nil || resp.Updated != 0 || resp.MatchedDocs != 0 {
				t.Fatalf("empty dataset response=%#v code=%v error=%v, want success with zero matches/updates", resp, code, err)
			}
			assertMetadataSelectionNoWrites(t, store, before)
		})
	}
}

func TestBatchUpdateDocumentMetadatasSelectionListFailure(t *testing.T) {
	cases := []struct {
		name     string
		selector *MetadataSelector
	}{
		{name: "nil selector"},
		{name: "empty selector", selector: &MetadataSelector{}},
		{name: "explicit IDs", selector: &MetadataSelector{DocumentIDs: []string{"doc-1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newMetadataSelectionTestService(t)
			before := cloneMetadataSelectionRecords(store.records)
			listErr := errors.New("dataset document query failed")
			if err := dao.DB.Callback().Query().Before("gorm:query").Register("metadata_selection:document_query_error", func(tx *gorm.DB) {
				if tx.Statement.Table == "document" {
					tx.AddError(listErr)
				}
			}); err != nil {
				t.Fatalf("register document query failure: %v", err)
			}
			resp, code, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-1", tc.selector,
				[]MetadataUpdate{{Key: "selected", Value: "yes"}}, nil)
			if resp != nil || code != common.CodeServerError || !errors.Is(err, listErr) {
				t.Errorf("list failure response=%#v code=%v error=%v, want server error wrapping %v", resp, code, err, listErr)
			}
			assertMetadataSelectionNoWrites(t, store, before)
		})
	}
}

func TestBatchUpdateDocumentMetadatasSelectionRejectsForeignIDs(t *testing.T) {
	cases := []struct {
		name      string
		ids       []string
		foreignID string
	}{
		{name: "same tenant other dataset", ids: []string{"doc-same-tenant"}, foreignID: "doc-same-tenant"},
		{name: "other tenant", ids: []string{"doc-other-tenant"}, foreignID: "doc-other-tenant"},
		{name: "missing document", ids: []string{"doc-missing"}, foreignID: "doc-missing"},
		{name: "valid ID alongside foreign ID", ids: []string{"doc-1", "doc-same-tenant"}, foreignID: "doc-same-tenant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newMetadataSelectionTestService(t)
			before := cloneMetadataSelectionRecords(store.records)
			resp, code, err := svc.BatchUpdateDocumentMetadatas(t.Context(), "kb-1", &MetadataSelector{DocumentIDs: tc.ids},
				[]MetadataUpdate{{Key: "selected", Value: "yes"}}, nil)
			if resp != nil || code != common.CodeDataError || err == nil || !strings.Contains(err.Error(), tc.foreignID) {
				t.Errorf("foreign ID response=%#v code=%v error=%v, want data error naming %q", resp, code, err, tc.foreignID)
			}
			assertMetadataSelectionNoWrites(t, store, before)
		})
	}
}

// NewMetadataSelectionServiceForTest builds the service for HTTP route regression tests.
func NewMetadataSelectionServiceForTest(store engine.DocEngine) *DocumentService {
	return &DocumentService{
		documentDAO: dao.NewDocumentDAO(),
		docEngine:   store,
		metadataSvc: service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), store),
	}
}

func newMetadataSelectionTestService(t *testing.T) (*DocumentService, *metadataSelectionDocEngine) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 3, 0, 0)
	insertTestKB(t, "kb-2", "tenant-1", 1, 0, 0)
	insertTestKB(t, "kb-3", "tenant-2", 1, 0, 0)
	docKBs := map[string]string{
		"doc-1": "kb-1", "doc-2": "kb-1", "doc-without-metadata": "kb-1",
		"doc-same-tenant": "kb-2", "doc-other-tenant": "kb-3",
	}
	for docID, kbID := range docKBs {
		insertNamedTestDoc(t, docID, kbID, docID+".txt", 0, 0)
	}
	store := &metadataSelectionDocEngine{metadataDocEngine: newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1":            {"state": "ready"},
		"doc-2":            {"state": "pending"},
		"doc-same-tenant":  {"state": "ready"},
		"doc-other-tenant": {"state": "ready"},
	}, docKBs)}
	svc := testDocumentService(t)
	svc.docEngine = store
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), store)
	return svc, store
}

func makeMetadataSelectionCondition(value string) map[string]interface{} {
	return map[string]interface{}{"conditions": []interface{}{
		map[string]interface{}{"name": "state", "comparison_operator": "is", "value": value},
	}}
}

func cloneMetadataSelectionRecords(records map[string]map[string]interface{}) map[string]map[string]interface{} {
	cloned := make(map[string]map[string]interface{}, len(records))
	for docID, meta := range records {
		cloned[docID] = cloneDocumentMetadata(meta)
	}
	return cloned
}

func assertMetadataSelectionNoWrites(t *testing.T, store *metadataSelectionDocEngine, before map[string]map[string]interface{}) {
	t.Helper()
	if store.mutations != 0 || len(store.writeCalls) != 0 {
		t.Errorf("metadata mutations=%d update calls=%d, want 0", store.mutations, len(store.writeCalls))
	}
	if !reflect.DeepEqual(store.records, before) {
		t.Errorf("metadata records changed: got %#v, want %#v", store.records, before)
	}
}

type metadataSelectionWrite struct {
	documentID string
	datasetID  string
	tenantID   string
}

type metadataSelectionDocEngine struct {
	*metadataDocEngine
	writeCalls []metadataSelectionWrite
	mutations  int
}

func (e *metadataSelectionDocEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	e.mutations++
	return e.metadataDocEngine.CreateMetadataStore(ctx, tenantID)
}

func (e *metadataSelectionDocEngine) UpdateMetadata(ctx context.Context, docID, datasetID string, fields map[string]interface{}, tenantID string) error {
	e.mutations++
	e.writeCalls = append(e.writeCalls, metadataSelectionWrite{documentID: docID, datasetID: datasetID, tenantID: tenantID})
	return e.metadataDocEngine.UpdateMetadata(ctx, docID, datasetID, fields, tenantID)
}

func (e *metadataSelectionDocEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	e.mutations++
	return e.metadataDocEngine.InsertMetadata(ctx, metadata, tenantID)
}

func (e *metadataSelectionDocEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	e.mutations++
	return e.metadataDocEngine.DeleteMetadata(ctx, condition, tenantID)
}

func (e *metadataSelectionDocEngine) DeleteMetadataKeys(ctx context.Context, docID, datasetID string, keys []string, tenantID string) error {
	e.mutations++
	return e.metadataDocEngine.DeleteMetadataKeys(ctx, docID, datasetID, keys, tenantID)
}
