package dataset

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

func TestSearchDatasetsResolvesSavedRerankerInSearchTenant(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatal(err)
	}
	insertCreateDatasetTenant(t, "team")
	insertCreateDatasetTenant(t, "member")
	ensureDatasetTestMembership(t, "member", "team", "normal")
	ensureDatasetTestMembership(t, "other-member", "team", "normal")

	valid := string(entity.StatusValid)
	app := &entity.Search{ID: "app", TenantID: "team", CreatedBy: "member", Status: &valid,
		SearchConfig: entity.JSONMap{"rerank_id": "team-model"}}
	for _, row := range []any{
		&entity.Knowledgebase{ID: "kb", TenantID: "team", CreatedBy: "team", Permission: "team", Status: &valid},
		app,
		&entity.TenantModelProvider{ID: "team-provider", UserID: "team", TenantID: "team", ProviderName: "Xinference"},
		&entity.TenantModelInstance{ID: "team-instance", ProviderID: "team-provider", InstanceName: "local", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "team-model", ProviderID: "team-provider", InstanceID: "team-instance", ModelName: "test-reranker", ModelType: int(entity.ModelTypeRerank), Status: "active"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	svc := &DatasetService{kbDAO: dao.NewKnowledgebaseDAO(), searchService: service.NewSearchService()}
	// Reranking rejects page 2 after resolving the model and before querying the index.
	req := &service.SearchDatasetsRequest{
		Question: "example", DatasetIDs: []string{"kb"}, SearchID: &app.ID, Page: iptr(2),
	}
	ctx := withDatasetTestContext(t.Context(), "member", "member")
	if _, err := svc.SearchDatasets(ctx, req); err == nil || !strings.Contains(err.Error(), "Pagination is not supported when rerank_mdl is specified") {
		t.Errorf("search with saved team model returned %v, want rerank pagination error", err)
	}

	otherCtx := withDatasetTestContext(t.Context(), "other-member", "team")
	if _, err := svc.SearchDatasets(otherCtx, req); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Errorf("search without app access returned %v, want permission denied", err)
	}

	if err := db.Model(app).Update("search_config", entity.JSONMap{}).Error; err != nil {
		t.Fatal(err)
	}
	req.RerankID = sptr("team-model")
	if _, err := svc.SearchDatasets(ctx, req); err == nil || !strings.Contains(err.Error(), "can use only its owner's model configuration") {
		t.Errorf("search with request-supplied team model returned %v, want active tenant ownership error", err)
	}
}

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

		KnowledgeCompilationScope: "all",
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
	if converted.KnowledgeCompilationScope != req.KnowledgeCompilationScope {
		t.Fatalf("knowledge_compilation_scope=%v want %v", converted.KnowledgeCompilationScope, req.KnowledgeCompilationScope)
	}
}

func TestKnowledgeCompilationFilter(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": "source-1", "doc_id": "doc-1"},
		{"id": "source-2", "doc_id": "doc-2"},
		{"id": "document-1", "compile_kwd": "graph", "scope_kwd": "doc", "doc_id": "doc-1"},
		{"id": "document-2", "compile_kwd": "graph", "scope_kwd": "doc", "doc_id": "doc-2"},
		{"id": "dataset-1", "compile_kwd": "graph", "scope_kwd": "dataset", "doc_id": "kb-1", "source_doc_ids": []string{"doc-1", "doc-2"}},
		{"id": "dataset-2", "compile_kwd": "graph", "scope_kwd": "dataset", "doc_id": "kb-1", "source_doc_ids": []string{"doc-2"}},
		{"id": "unscoped", "compile_kwd": "graph", "doc_id": "doc-2"},
	}
	for _, tt := range []struct {
		name         string
		include      *bool
		scope        string
		docIDs, want []string
	}{
		{name: "omitted", want: []string{"source-1", "source-2", "dataset-1", "dataset-2"}},
		{name: "omitted with compilation enabled", include: new(true), want: []string{"source-1", "source-2", "dataset-1", "dataset-2"}},
		{name: "omitted with compilation disabled", include: new(false), want: []string{"source-1", "source-2"}},
		{name: "omitted with selected document", docIDs: []string{"doc-1"}, want: []string{"source-1", "dataset-1", "dataset-2"}},
		{name: "dataset", include: new(true), scope: "dataset", want: []string{"source-1", "source-2", "dataset-1", "dataset-2"}},
		{name: "document", include: new(true), scope: "document", want: []string{"source-1", "source-2", "document-1", "document-2"}},
		{name: "all", include: new(true), scope: "all", want: []string{"source-1", "source-2", "document-1", "document-2", "dataset-1", "dataset-2", "unscoped"}},
		{name: "default include", scope: "document", want: []string{"source-1", "source-2", "document-1", "document-2"}},
		{name: "disabled overrides scope", include: new(false), scope: "all", want: []string{"source-1", "source-2"}},
		{name: "all with selected document", scope: "all", docIDs: []string{"doc-1"}, want: []string{"source-1", "document-1", "dataset-1", "dataset-2"}},
		{name: "dataset with selected document", scope: "dataset", docIDs: []string{"doc-1"}, want: []string{"source-1", "dataset-1", "dataset-2"}},
		{name: "document with selected document", scope: "document", docIDs: []string{"doc-1"}, want: []string{"source-1", "document-1"}},
		{name: "all with unscoped compilation", scope: "all", docIDs: []string{"doc-2"}, want: []string{"source-2", "document-2", "dataset-1", "dataset-2", "unscoped"}},
		{name: "dataset with unrelated document", scope: "dataset", docIDs: []string{"doc-3"}, want: []string{"dataset-1", "dataset-2"}},
		{name: "document with unrelated document", scope: "document", docIDs: []string{"doc-3"}},
		{name: "all with unrelated document", scope: "all", docIDs: []string{"doc-3"}, want: []string{"dataset-1", "dataset-2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := nlp.RetrievalSearchRequest{DocIDs: tt.docIDs, Filter: knowledgeCompilationFilter(tt.include, tt.scope)}
			if len(tt.docIDs) > 0 && (tt.include == nil || *tt.include) && tt.scope != "document" {
				req.DocIDs = append(slices.Clone(tt.docIDs), "kb-1")
			}
			filter := req.GetFilters()
			var got []string
			for _, row := range rows {
				if matchesKnowledgeCompilationFilter(filter, row) {
					got = append(got, row["id"].(string))
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("matched chunks=%v want %v", got, tt.want)
			}
		})
	}
}

func matchesKnowledgeCompilationFilter(filter, row map[string]interface{}) bool {
	for field, value := range filter {
		switch field {
		case "or":
			matched := false
			for _, branch := range types.FilterClauses(value) {
				matched = matched || matchesKnowledgeCompilationFilter(branch, row)
			}
			if !matched {
				return false
			}
		case "must_not":
			if matchesKnowledgeCompilationFilter(value.(map[string]interface{}), row) {
				return false
			}
		case "exists":
			if row[value.(string)] == nil {
				return false
			}
		default:
			if values, ok := value.([]string); ok {
				matched := false
				for _, expected := range values {
					if actual, ok := row[field].([]string); ok {
						for _, item := range actual {
							matched = matched || item == expected
						}
					} else {
						matched = matched || row[field] == expected
					}
				}
				if !matched {
					return false
				}
			} else if row[field] != value {
				return false
			}
		}
	}
	return true
}

func TestFormatDatasetSearchChunksCompilationVisibility(t *testing.T) {
	enabled, disabled := true, false
	for _, tt := range []struct {
		name    string
		include *bool
		present bool
	}{
		{name: "disabled", include: &disabled},
		{name: "enabled", include: &enabled, present: true},
		{name: "unspecified", present: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chunks := []map[string]interface{}{
				{"chunk_id": "compiled-1", "content_with_weight": "compiled content", "compile_kwd": "page_index", "vector": []float64{1}},
				{"chunk_id": "ordinary-1", "content_with_weight": "ordinary content"},
			}
			formatDatasetSearchChunks(chunks, tt.include)
			if len(chunks) != 2 || chunks[0]["id"] != "compiled-1" || chunks[0]["content"] != "compiled content" {
				t.Fatalf("API fields changed: %#v", chunks)
			}
			kind, present := chunks[0]["compile_kwd"]
			if present != tt.present || (present && kind != "page_index") {
				t.Fatalf("compile_kwd = %v, present = %v; want present = %v", kind, present, tt.present)
			}
			if _, present := chunks[1]["compile_kwd"]; present {
				t.Fatalf("ordinary chunk has compilation marker: %#v", chunks[1])
			}
			if _, present := chunks[0]["vector"]; present {
				t.Fatalf("API chunk still has vector: %#v", chunks[0])
			}
		})
	}
}

func TestDisabledCompilationStillPromotesOrdinaryParent(t *testing.T) {
	disabled := false
	eng := &compilationSearchEngine{rows: []map[string]interface{}{
		{"id": "compiled", "compile_kwd": "page_index", "available_int": 1},
		{"id": "child-1", "mom_id": "parent-1", "doc_id": "doc-1", "kb_id": "kb-1", "available_int": 1},
		{"id": "parent-1", "doc_id": "doc-1", "kb_id": "kb-1", "content_with_weight": "parent content", "available_int": 0},
	}}
	filter := knowledgeCompilationFilter(&disabled, "all")
	filter["available_int"] = 1
	hits, err := eng.Search(t.Context(), &types.SearchRequest{Filter: filter})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Chunks) != 1 || hits.Chunks[0]["id"] != "child-1" {
		t.Fatalf("initial hits = %#v, want only the ordinary child", hits.Chunks)
	}
	hits.Chunks[0]["chunk_id"] = hits.Chunks[0]["id"]
	chunks := nlp.RetrievalByChildren(hits.Chunks, []string{"tenant-1"}, eng, t.Context())
	formatDatasetSearchChunks(chunks, &disabled)
	if len(chunks) != 1 || chunks[0]["id"] != "parent-1" || chunks[0]["content"] != "parent content" {
		t.Fatalf("parent promotion changed: %#v", chunks)
	}
	if _, present := chunks[0]["compile_kwd"]; present {
		t.Fatalf("disabled response has compilation marker: %#v", chunks[0])
	}
}

type compilationSearchEngine struct {
	fakeChatDocEngine
	rows []map[string]interface{}
}

func (e *compilationSearchEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	var chunks []map[string]interface{}
	for _, row := range e.rows {
		if !matchesKnowledgeCompilationFilter(req.Filter, row) {
			continue
		}
		chunk := make(map[string]interface{}, len(row))
		for field, value := range row {
			if len(req.SelectFields) == 0 || field == "id" || slices.Contains(req.SelectFields, field) {
				chunk[field] = value
			}
		}
		chunks = append(chunks, chunk)
	}
	return &types.SearchResult{Chunks: chunks, Total: int64(len(chunks))}, nil
}
