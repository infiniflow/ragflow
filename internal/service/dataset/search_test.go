package dataset

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestSearchDatasetRequestToSearchDatasetsRequest(t *testing.T) {
	page := 2
	pageSize := 15
	knnTopK := 128
	knnNumCandidates := 256
	useKG := true
	keyword := true
	similarityThreshold := 0.42
	vectorSimilarityWeight := 0.8
	searchID := "search-1"
	rerankID := "rerank-1"
	includeKnowledgeCompilation := false
	includeMetadata := true
	req := &service.SearchDatasetRequest{
		Question:               "hello world",
		Page:                   &page,
		PageSize:               &pageSize,
		DocumentIDs:            []string{"doc-1", "doc-2"},
		UseKG:                  &useKG,
		KNNTopK:                &knnTopK,
		KNNNumCandidates:       &knnNumCandidates,
		CrossLanguages:         []string{"en", "zh"},
		SearchID:               &searchID,
		MetadataCondition:      map[string]interface{}{"logic": "and"},
		RerankID:               &rerankID,
		Keyword:                &keyword,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		IncludeCompiledChunks:  &includeKnowledgeCompilation,
		IncludeMetadata:        &includeMetadata,
		MetadataFields:         []string{"author"},
		ReferenceMetadata:      &service.ReferenceMetadata{Fields: []string{"year"}},
	}

	converted := req.ToSearchDatasetsRequest("dataset-1")
	if len(converted.DatasetIDs) != 1 || converted.DatasetIDs[0] != "dataset-1" {
		t.Fatalf("dataset_ids=%v want [dataset-1]", converted.DatasetIDs)
	}
	if converted.Question != req.Question || converted.Page != req.Page || converted.PageSize != req.PageSize {
		t.Fatalf("converted request did not preserve pagination/question fields: %#v", converted)
	}
	if len(converted.DocumentIDs) != 2 || converted.DocumentIDs[0] != "doc-1" || converted.DocumentIDs[1] != "doc-2" {
		t.Fatalf("document_ids=%v want [doc-1 doc-2]", converted.DocumentIDs)
	}
	if converted.UseKG != req.UseKG || converted.KNNTopK != req.KNNTopK || converted.KNNNumCandidates != req.KNNNumCandidates || converted.SearchID != req.SearchID {
		t.Fatalf("converted request did not preserve optional fields: %#v", converted)
	}
	if converted.MetadataCondition["logic"] != "and" || converted.RerankID != req.RerankID || converted.Keyword != req.Keyword {
		t.Fatalf("converted request did not preserve search config fields: %#v", converted)
	}
	if converted.SimilarityThreshold != req.SimilarityThreshold || converted.VectorSimilarityWeight != req.VectorSimilarityWeight {
		t.Fatalf("converted request did not preserve thresholds: %#v", converted)
	}
	if converted.IncludeCompiledChunks != req.IncludeCompiledChunks {
		t.Fatalf("converted request did not preserve include_knowledge_compilation: %#v", converted)
	}
	if converted.IncludeMetadata != req.IncludeMetadata || !reflect.DeepEqual(converted.MetadataFields, req.MetadataFields) || converted.ReferenceMetadata != req.ReferenceMetadata {
		t.Fatalf("converted request did not preserve the document metadata selection: %#v", converted)
	}
}

func TestSelectMetadataFilteredDocIDs(t *testing.T) {
	tests := []struct {
		name                 string
		current              []string
		filtered             []string
		hasMetadataCondition bool
		filterReturnedEmpty  bool
		want                 []string
	}{
		{name: "metadata filter narrows explicit document ids", current: []string{"doc-1"}, filtered: []string{"doc-1"}, want: []string{"doc-1"}},
		{name: "metadata filter cannot widen explicit document ids", current: []string{"doc-1"}, filtered: []string{}, want: []string{}},
		{name: "empty generated filter keeps explicit document ids", current: []string{"doc-1"}, filtered: nil, filterReturnedEmpty: true, want: []string{"doc-1"}},
		{name: "metadata condition uses definitive empty result", current: []string{"doc-1"}, filtered: nil, hasMetadataCondition: true, filterReturnedEmpty: true, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectMetadataFilteredDocIDs(tt.current, tt.filtered, tt.hasMetadataCondition, tt.filterReturnedEmpty)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSearchDatasetsRequestDocumentMetadataSelection(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantInclude bool
		wantFields  []string
	}{
		{name: "off by default", body: `{}`},
		{name: "fields without include stay off", body: `{"metadata_fields":["author"],"reference_metadata":{"fields":["year"]}}`, wantFields: []string{"author"}},
		{name: "include_metadata", body: `{"include_metadata":true}`, wantInclude: true},
		{name: "metadata_fields filter", body: `{"include_metadata":true,"metadata_fields":["author"]}`, wantInclude: true, wantFields: []string{"author"}},
		{name: "nested reference_metadata", body: `{"reference_metadata":{"include":true,"fields":["year"]}}`, wantInclude: true, wantFields: []string{"year"}},
		{name: "top-level keys override nested", body: `{"include_metadata":false,"metadata_fields":["author"],"reference_metadata":{"include":true,"fields":["year"]}}`, wantFields: []string{"author"}},
		{name: "empty field list selects every field", body: `{"include_metadata":true,"metadata_fields":[]}`, wantInclude: true},
		{name: "empty nested field list selects every field", body: `{"reference_metadata":{"include":true,"fields":[]}}`, wantInclude: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req service.SearchDatasetsRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			include, fields := req.DocumentMetadataSelection(nil)
			if include != tt.wantInclude {
				t.Fatalf("include = %v, want %v", include, tt.wantInclude)
			}
			if !reflect.DeepEqual(fields, tt.wantFields) {
				t.Fatalf("fields = %#v, want %#v", fields, tt.wantFields)
			}
		})
	}
}

func TestSearchDatasetsRequestDocumentMetadataSelectionFallsBackToSearchConfig(t *testing.T) {
	saved := func(referenceMetadata map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"reference_metadata": referenceMetadata}
	}
	tests := []struct {
		name         string
		body         string
		searchConfig map[string]interface{}
		wantInclude  bool
		wantFields   []string
	}{
		{name: "saved setting applies", body: `{}`, searchConfig: saved(map[string]interface{}{"include": true, "fields": []interface{}{"year"}}), wantInclude: true, wantFields: []string{"year"}},
		{name: "saved empty field list selects every field", body: `{}`, searchConfig: saved(map[string]interface{}{"include": true, "fields": []interface{}{}}), wantInclude: true},
		{name: "saved fields without include stay off", body: `{}`, searchConfig: saved(map[string]interface{}{"fields": []interface{}{"year"}}), wantFields: []string{"year"}},
		{name: "search config without reference_metadata stays off", body: `{}`, searchConfig: map[string]interface{}{"top_k": float64(1024)}},
		{name: "request include wins", body: `{"include_metadata":false}`, searchConfig: saved(map[string]interface{}{"include": true, "fields": []interface{}{"year"}}), wantFields: []string{"year"}},
		{name: "request fields win", body: `{"reference_metadata":{"fields":["author"]}}`, searchConfig: saved(map[string]interface{}{"include": true, "fields": []interface{}{"year"}}), wantInclude: true, wantFields: []string{"author"}},
		{name: "request empty field list wins", body: `{"metadata_fields":[]}`, searchConfig: saved(map[string]interface{}{"include": true, "fields": []interface{}{"year"}}), wantInclude: true},
		{name: "request turns it on over a saved off", body: `{"reference_metadata":{"include":true}}`, searchConfig: saved(map[string]interface{}{"include": false, "fields": []interface{}{"year"}}), wantInclude: true, wantFields: []string{"year"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req service.SearchDatasetsRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			include, fields := req.DocumentMetadataSelection(tt.searchConfig)
			if include != tt.wantInclude {
				t.Fatalf("include = %v, want %v", include, tt.wantInclude)
			}
			if !reflect.DeepEqual(fields, tt.wantFields) {
				t.Fatalf("fields = %#v, want %#v", fields, tt.wantFields)
			}
		})
	}
}

type enrichCall struct {
	tenantID string
	chunkIDs []string
	fields   []string
}

type fakeDocMetadataEnricher struct {
	calls []enrichCall
}

func (f *fakeDocMetadataEnricher) EnrichChunksWithDocMetadata(_ context.Context, chunks []map[string]interface{}, tenantID string, fields []string) {
	call := enrichCall{tenantID: tenantID, fields: fields}
	for _, chunk := range chunks {
		call.chunkIDs = append(call.chunkIDs, chunk["chunk_id"].(string))
	}
	f.calls = append(f.calls, call)
}

func TestAttachDocumentMetadataGroupsChunksByTenant(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{ID: "kb-a", TenantID: "tenant-1"},
		{ID: "kb-b", TenantID: "tenant-2"},
		{ID: "kb-c", TenantID: "tenant-1"},
	}
	chunks := []map[string]interface{}{
		{"chunk_id": "c1", "doc_id": "d1", "kb_id": "kb-a"},
		{"chunk_id": "c2", "doc_id": "d2", "kb_id": "kb-b"},
		{"chunk_id": "c3", "doc_id": "d3", "kb_id": "kb-c"},
		{"chunk_id": "c4", "doc_id": "d4", "kb_id": "kb-unknown"},
	}
	enricher := &fakeDocMetadataEnricher{}
	attachDocumentMetadata(context.Background(), enricher, chunks, kbs, []string{"author"})

	sort.Slice(enricher.calls, func(i, j int) bool { return enricher.calls[i].tenantID < enricher.calls[j].tenantID })
	want := []enrichCall{
		{tenantID: "tenant-1", chunkIDs: []string{"c1", "c3"}, fields: []string{"author"}},
		{tenantID: "tenant-2", chunkIDs: []string{"c2"}, fields: []string{"author"}},
	}
	if !reflect.DeepEqual(enricher.calls, want) {
		t.Fatalf("calls = %#v, want %#v", enricher.calls, want)
	}
}

// docMetadataEngine serves fixed document metadata records.
type docMetadataEngine struct {
	fakeChatDocEngine
	records []map[string]interface{}
}

func (e docMetadataEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return &types.SearchMetadataResult{MetadataRecords: e.records}, nil
}

func TestAttachDocumentMetadataFieldSelection(t *testing.T) {
	eng := docMetadataEngine{records: []map[string]interface{}{
		{"id": "doc-1", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"author": "Jane Doe", "year": float64(2024)}},
	}}
	kbs := []*entity.Knowledgebase{{ID: "kb-1", TenantID: "tenant-1"}}
	tests := []struct {
		name   string
		body   string
		want   map[string]interface{}
		wantOK bool
	}{
		{name: "fields omitted attaches every field", body: `{"include_metadata":true}`, want: map[string]interface{}{"author": "Jane Doe", "year": float64(2024)}, wantOK: true},
		{name: "empty field list attaches every field", body: `{"include_metadata":true,"metadata_fields":[]}`, want: map[string]interface{}{"author": "Jane Doe", "year": float64(2024)}, wantOK: true},
		{name: "field list restricts the keys", body: `{"reference_metadata":{"include":true,"fields":["year"]}}`, want: map[string]interface{}{"year": float64(2024)}, wantOK: true},
		{name: "no requested field present attaches nothing", body: `{"include_metadata":true,"metadata_fields":["department"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req service.SearchDatasetsRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			include, fields := req.DocumentMetadataSelection(nil)
			if !include {
				t.Fatal("expected document metadata to be requested")
			}
			chunks := []map[string]interface{}{
				{"chunk_id": "chunk-1", "doc_id": "doc-1", "kb_id": "kb-1"},
				{"chunk_id": "chunk-2", "doc_id": "doc-2", "kb_id": "kb-1"},
			}
			attachDocumentMetadata(context.Background(), service.NewMetadataServiceForTest(nil, eng), chunks, kbs, fields)

			got, ok := chunks[0]["document_metadata"]
			if ok != tt.wantOK {
				t.Fatalf("document_metadata present = %v, want %v (%#v)", ok, tt.wantOK, got)
			}
			if tt.wantOK && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("document_metadata = %#v, want %#v", got, tt.want)
			}
			if _, ok := chunks[1]["document_metadata"]; ok {
				t.Fatalf("a chunk whose document has no metadata must not get document_metadata, got %#v", chunks[1]["document_metadata"])
			}
		})
	}
}
