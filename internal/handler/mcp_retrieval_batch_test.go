package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
	"ragflow/internal/permission"
	"ragflow/internal/service"
)

// countingMCPDocuments records every document lookup mcpRetrieval performs so
// the N+1 regression is falsifiable: a wrong implementation that keeps issuing
// one query per document fails the call-count assertions.
type countingMCPDocuments struct {
	listCalls       int
	listKbIDs       []string
	listDocIDs      [][]string
	metaCalls       int
	metaSingleCalls int
	metaKbIDs       []string
	metaDocIDs      [][]string

	docs     map[string]*entity.DocumentListItem
	metadata map[string]map[string]interface{}
	listErr  error
	metaErr  error
}

func (f *countingMCPDocuments) ListDocumentsByDatasetIDWithOptions(_ context.Context, opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	f.listCalls++
	f.listKbIDs = append(f.listKbIDs, opts.KbID)
	f.listDocIDs = append(f.listDocIDs, append([]string(nil), opts.DocIDs...))
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	out := make([]*entity.DocumentListItem, 0, len(opts.DocIDs))
	for _, id := range opts.DocIDs {
		if doc := f.docs[id]; doc != nil {
			out = append(out, doc)
		}
	}
	total := int64(len(out))
	if page > 1 && pageSize > 0 {
		start := (page - 1) * pageSize
		if start >= len(out) {
			return []*entity.DocumentListItem{}, total, nil
		}
		out = out[start:]
	}
	if pageSize > 0 && len(out) > pageSize {
		out = out[:pageSize]
	}
	return out, total, nil
}

func (f *countingMCPDocuments) GetDocumentMetadataByID(_ context.Context, docID string) (map[string]interface{}, error) {
	f.metaSingleCalls++
	if meta := f.metadata[docID]; meta != nil {
		return meta, nil
	}
	return map[string]interface{}{}, nil
}

func (f *countingMCPDocuments) GetDocumentMetadataByIDs(_ context.Context, kbID string, docIDs []string) (map[string]map[string]interface{}, error) {
	f.metaCalls++
	f.metaKbIDs = append(f.metaKbIDs, kbID)
	f.metaDocIDs = append(f.metaDocIDs, append([]string(nil), docIDs...))
	if f.metaErr != nil {
		return nil, f.metaErr
	}
	out := make(map[string]map[string]interface{}, len(docIDs))
	for _, id := range docIDs {
		if meta := f.metadata[id]; meta != nil {
			out[id] = meta
		}
	}
	return out, nil
}

func batchTestDoc(kbID, id, name string) *entity.DocumentListItem {
	n := name
	return &entity.DocumentListItem{
		ID: id, KbID: kbID, Name: &n, Type: "pdf", Size: 100,
		ChunkNum: 3, TokenNum: 30,
	}
}

func batchTestChunks() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "c1", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "Doc A", "content_with_weight": "a"},
		{"id": "c2", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "Doc A", "content_with_weight": "a2"},
		{"id": "c3", "dataset_id": "ds-1", "document_id": "doc-b", "document_keyword": "Doc B", "content_with_weight": "b"},
		{"id": "c4", "dataset_id": "ds-2", "document_id": "doc-c", "document_keyword": "Doc C", "content_with_weight": "c"},
		{"id": "c5", "dataset_id": "ds-unknown", "document_id": "doc-x", "document_keyword": "Doc X", "content_with_weight": "x"},
	}
}

func batchTestDocuments() *countingMCPDocuments {
	return &countingMCPDocuments{
		docs: map[string]*entity.DocumentListItem{
			"doc-a": batchTestDoc("ds-1", "doc-a", "A.pdf"),
			"doc-b": batchTestDoc("ds-1", "doc-b", "B.pdf"),
			"doc-c": batchTestDoc("ds-2", "doc-c", "C.pdf"),
			"doc-x": batchTestDoc("ds-unknown", "doc-x", "X.pdf"),
		},
		metadata: map[string]map[string]interface{}{
			"doc-a": {"author": "Alice"},
			"doc-b": {"author": "Bob"},
			"doc-c": {"author": "Carol"},
			"doc-x": {"author": "X"},
		},
	}
}

func decodeBatchResult(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("result is not valid JSON: %v (%q)", err, raw)
	}
	return payload
}

func batchResultChunks(t *testing.T, payload map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := payload["chunks"].([]interface{})
	if !ok {
		t.Fatalf("chunks missing from result: %#v", payload)
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		chunk, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("chunk is not an object: %#v", item)
		}
		out = append(out, chunk)
	}
	return out
}

// TestMCPRetrievalBatchesDocumentLookups is the core RED test: the old code
// issued one document-list AND one metadata query per unique document (3 of
// each here); the batched code must issue exactly one of each per dataset (2).
func TestMCPRetrievalBatchesDocumentLookups(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{
		Chunks: batchTestChunks(),
		Total:  5,
	}}
	docs := batchTestDocuments()

	result, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1", "ds-2"},
		Question:   "alpha",
		Page:       1,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	if probe.getCalls != 2 {
		t.Fatalf("GetDataset calls = %d, want 2", probe.getCalls)
	}
	if docs.listCalls != 2 {
		t.Fatalf("ListDocumentsByDatasetIDWithOptions calls = %d, want 2 (one per dataset), got kbIDs=%v docIDs=%v", docs.listCalls, docs.listKbIDs, docs.listDocIDs)
	}
	if docs.metaCalls != 2 {
		t.Fatalf("GetDocumentMetadataByIDs calls = %d, want 2 (one per dataset), got kbIDs=%v docIDs=%v", docs.metaCalls, docs.metaKbIDs, docs.metaDocIDs)
	}
	if got := docs.listDocIDs[0]; !reflect.DeepEqual(got, []string{"doc-a", "doc-b"}) {
		t.Fatalf("ds-1 list docIDs = %v, want [doc-a doc-b]", got)
	}
	if got := docs.metaDocIDs[0]; !reflect.DeepEqual(got, []string{"doc-a", "doc-b"}) {
		t.Fatalf("ds-1 metadata docIDs = %v, want [doc-a doc-b]", got)
	}
	if got := docs.listKbIDs; !reflect.DeepEqual(got, []string{"ds-1", "ds-2"}) {
		t.Fatalf("list kbIDs = %v, want [ds-1 ds-2]", got)
	}
	if got := docs.metaKbIDs; !reflect.DeepEqual(got, []string{"ds-1", "ds-2"}) {
		t.Fatalf("metadata kbIDs = %v, want [ds-1 ds-2]", got)
	}

	chunks := batchResultChunks(t, decodeBatchResult(t, result))
	if len(chunks) != 5 {
		t.Fatalf("chunks = %d, want 5", len(chunks))
	}
	wantKeys := []string{"document_id", "name", "location", "type", "size", "chunk_count", "create_date", "update_date", "token_count", "thumbnail", "dataset_id", "meta_fields"}
	for _, idx := range []int{0, 1} {
		meta, ok := chunks[idx]["document_metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("chunk %d missing document_metadata: %#v", idx, chunks[idx])
		}
		if len(meta) != len(wantKeys) {
			t.Fatalf("chunk %d metadata keys = %d (%v), want %d", idx, len(meta), meta, len(wantKeys))
		}
		for _, key := range wantKeys {
			if _, ok := meta[key]; !ok {
				t.Fatalf("chunk %d metadata missing key %q: %v", idx, key, meta)
			}
		}
		if meta["name"] != "A.pdf" {
			t.Fatalf("chunk %d metadata name = %v, want A.pdf", idx, meta["name"])
		}
		fields, ok := meta["meta_fields"].(map[string]interface{})
		if !ok || fields["author"] != "Alice" {
			t.Fatalf("chunk %d meta_fields = %#v, want author=Alice", idx, meta["meta_fields"])
		}
	}
	if chunks[4]["dataset_name"] != "Unknown" {
		t.Fatalf("unknown dataset name = %v, want Unknown", chunks[4]["dataset_name"])
	}
	if _, ok := chunks[4]["document_metadata"]; ok {
		t.Fatalf("unknown dataset chunk must not carry document_metadata: %#v", chunks[4])
	}
}

// TestMCPRetrievalBatchDeduplicatesAcrossDatasets exercises the adversarial
// case where the same document appears under two datasets: the first dataset
// wins and only one metadata lookup happens.
func TestMCPRetrievalBatchDeduplicatesAcrossDatasets(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "c1", "dataset_id": "ds-1", "document_id": "shared", "document_keyword": "Shared"},
		{"id": "c2", "dataset_id": "ds-2", "document_id": "shared", "document_keyword": "Shared"},
	}
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{Chunks: chunks, Total: 2}}
	docs := &countingMCPDocuments{
		docs: map[string]*entity.DocumentListItem{
			"shared": batchTestDoc("ds-1", "shared", "shared.pdf"),
		},
		metadata: map[string]map[string]interface{}{"shared": {"author": "Shared"}},
	}

	result, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1", "ds-2"}, Question: "q", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	if docs.metaCalls != 1 || docs.listCalls != 1 {
		t.Fatalf("calls: list=%d metadata=%d, want 1/1 after cross-dataset dedup", docs.listCalls, docs.metaCalls)
	}
	seen := map[string]bool{}
	for _, ids := range docs.metaDocIDs {
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("document %q was looked up more than once: %v", id, docs.metaDocIDs)
			}
			seen[id] = true
		}
	}
	if !seen["shared"] {
		t.Fatalf("shared document was never looked up: %v", docs.metaDocIDs)
	}
	chunksOut := batchResultChunks(t, decodeBatchResult(t, result))
	if _, ok := chunksOut[0]["document_metadata"]; !ok {
		t.Fatalf("first chunk missing document_metadata: %#v", chunksOut[0])
	}
	if _, ok := chunksOut[1]["document_metadata"]; !ok {
		t.Fatalf("second chunk missing document_metadata: %#v", chunksOut[1])
	}
}

func TestMCPRetrievalBatchEmptyChunks(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{}}
	docs := batchTestDocuments()
	if _, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1", "ds-2"}, Question: "q", Page: 1, PageSize: 10,
	}); err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	if docs.listCalls != 0 || docs.metaCalls != 0 {
		t.Fatalf("no chunks: list=%d metadata=%d, want 0/0", docs.listCalls, docs.metaCalls)
	}
}

// TestMCPRetrievalBatchMetadataErrorStillEmitsEntries locks the fallback: when
// a dataset's metadata batch fails, each document still gets a metadata object
// with empty meta_fields, exactly like the old per-document error fallback.
func TestMCPRetrievalBatchMetadataErrorStillEmitsEntries(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "A"},
		},
		Total: 1,
	}}
	docs := batchTestDocuments()
	docs.metaErr = fmt.Errorf("metadata backend down")

	result, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1"}, Question: "q", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	chunks := batchResultChunks(t, decodeBatchResult(t, result))
	meta, ok := chunks[0]["document_metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("document_metadata missing after metadata error: %#v", chunks[0])
	}
	fields, ok := meta["meta_fields"].(map[string]interface{})
	if !ok || len(fields) != 0 {
		t.Fatalf("meta_fields = %#v, want empty map after metadata error", meta["meta_fields"])
	}
	if meta["name"] != "A.pdf" {
		t.Fatalf("name = %v, want A.pdf after metadata error", meta["name"])
	}
}

func BenchmarkMCPRetrievalBatchLookups(b *testing.B) {
	chunks := make([]map[string]interface{}, 0, 50)
	docs := &countingMCPDocuments{
		docs:     make(map[string]*entity.DocumentListItem),
		metadata: make(map[string]map[string]interface{}),
	}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("doc-%d", i)
		chunks = append(chunks, map[string]interface{}{"id": id, "dataset_id": "ds-1", "document_id": id, "document_keyword": id})
		docs.docs[id] = batchTestDoc("ds-1", id, id+".pdf")
		docs.metadata[id] = map[string]interface{}{"author": id}
	}
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{Chunks: chunks, Total: int64(len(chunks))}}
	req := mcp.RetrievalRequest{DatasetIDs: []string{"ds-1"}, Question: "q", Page: 1, PageSize: 10}
	ctx := permission.WithIdentity(context.Background(), "user-1", "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mcpRetrievalWithDocs(ctx, probe, docs, req); err != nil {
			b.Fatalf("mcpRetrievalWithDocs failed: %v", err)
		}
		if docs.listCalls != i+1 || docs.metaCalls != i+1 {
			b.Fatalf("query explosion: list=%d metadata=%d after %d iterations", docs.listCalls, docs.metaCalls, i+1)
		}
		// Keep the recorded argument history bounded: the benchmark only
		// reads the call counters, so retain the cumulative counts and drop
		// the per-iteration ID slices.
		docs.listDocIDs = docs.listDocIDs[:0]
		docs.metaDocIDs = docs.metaDocIDs[:0]
		docs.listKbIDs = docs.listKbIDs[:0]
		docs.metaKbIDs = docs.metaKbIDs[:0]
		probe.getIDs = probe.getIDs[:0]
	}
}

// TestMCPRetrievalBatchMetadataOnlyForListedDocs is adversarial: a document the
// listing did not return (e.g. no read access) must not be included in the
// metadata batch, even though its ID appeared in the retrieval chunks.
func TestMCPRetrievalBatchMetadataOnlyForListedDocs(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "A"},
			{"id": "c2", "dataset_id": "ds-1", "document_id": "doc-b", "document_keyword": "B"},
		},
		Total: 2,
	}}
	docs := batchTestDocuments()
	delete(docs.docs, "doc-b") // listing does not return doc-b

	if _, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1"}, Question: "q", Page: 1, PageSize: 10,
	}); err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	if !reflect.DeepEqual(docs.metaDocIDs, [][]string{{"doc-a"}}) {
		t.Fatalf("metadata docIDs = %v, want [[doc-a]] (unlisted doc must be excluded)", docs.metaDocIDs)
	}
}
