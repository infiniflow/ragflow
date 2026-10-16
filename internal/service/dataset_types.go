package service

import (
	"fmt"

	"ragflow/internal/common"
)

// CheckEmbeddingRequest is the request structure for checking embedding compatibility.
type CheckEmbeddingRequest struct {
	EmbeddingID string `json:"embd_id" binding:"required"`
	CheckNum    *int   `json:"check_num,omitempty"`
}

// EmbeddingCheckSummary is the summary of an embedding model compatibility check.
type EmbeddingCheckSummary struct {
	KbID      string  `json:"kb_id"`
	Model     string  `json:"model"`
	Sampled   int     `json:"sampled"`
	Valid     int     `json:"valid"`
	AvgCosSim float64 `json:"avg_cos_sim"`
	MinCosSim float64 `json:"min_cos_sim"`
	MaxCosSim float64 `json:"max_cos_sim"`
	MatchMode string  `json:"match_mode"`
}

// EmbeddingCheckResult is one chunk result in an embedding compatibility check.
type EmbeddingCheckResult struct {
	ChunkID     string  `json:"chunk_id"`
	DocID       string  `json:"doc_id,omitempty"`
	DocName     string  `json:"doc_name,omitempty"`
	VectorField string  `json:"vector_field,omitempty"`
	VectorDim   int     `json:"vector_dim,omitempty"`
	CosSim      float64 `json:"cos_sim,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

// EmbeddingCheckResponse is the response wrapper for embedding checks.
type EmbeddingCheckResponse struct {
	Summary EmbeddingCheckSummary  `json:"summary"`
	Results []EmbeddingCheckResult `json:"results"`
}

// SearchDatasetsRequest is the request structure for searching chunks across datasets.
type SearchDatasetsRequest struct {
	DatasetIDs               []string               `json:"dataset_ids" binding:"required"`
	Question                 string                 `json:"question" binding:"required"`
	Page                     *int                   `json:"page,omitempty"`
	PageSize                 *int                   `json:"page_size,omitempty"`
	Size                     *int                   `json:"size,omitempty"`
	RerankCandidatesCount    *int                   `json:"rerank_candidates_count,omitempty"`
	DocumentIDs              []string               `json:"document_ids,omitempty"`
	DocIDs                   []string               `json:"doc_ids,omitempty"`
	UseKG                    *bool                  `json:"use_kg,omitempty"`
	KNNTopK                  *int                   `json:"knn_top_k,omitempty"`
	TopK                     *int                   `json:"top_k,omitempty"` // Legacy alias for knn_top_k.
	KNNNumCandidates         *int                   `json:"knn_num_candidates,omitempty"`
	CrossLanguages           []string               `json:"cross_languages,omitempty"`
	SearchID                 *string                `json:"search_id,omitempty"`
	MetadataCondition        map[string]interface{} `json:"metadata_condition,omitempty"`
	MetadataFilter           map[string]interface{} `json:"meta_data_filter,omitempty"`
	RerankID                 *string                `json:"rerank_id,omitempty"`
	Keyword                  *bool                  `json:"keyword,omitempty"`
	Highlight                *bool                  `json:"highlight,omitempty"`
	SimilarityThreshold      *float64               `json:"similarity_threshold,omitempty"`
	KeywordsSimilarityWeight *float64               `json:"keywords_similarity_weight,omitempty"`
	VectorSimilarityWeight   *float64               `json:"vector_similarity_weight,omitempty"`
	IncludeCompiledChunks    *bool                  `json:"include_knowledge_compilation,omitempty"`
	IncludeMetadata          *bool                  `json:"include_metadata,omitempty"`
	MetadataFields           []string               `json:"metadata_fields,omitempty"`
	ReferenceMetadata        *ReferenceMetadata     `json:"reference_metadata,omitempty"`
	ForceRefresh             bool                   `json:"force_refresh"`

	KnowledgeCompilationScope string `json:"knowledge_compilation_scope,omitempty"`
}

// ReferenceMetadata selects the document metadata attached to retrieved chunks.
type ReferenceMetadata struct {
	Include *bool    `json:"include,omitempty"`
	Fields  []string `json:"fields,omitempty"`
}

// DocumentMetadataSelection reports whether retrieved chunks get a
// document_metadata object and which metadata keys it keeps (nil keeps every
// key). include_metadata and metadata_fields take precedence over
// reference_metadata, and the request takes precedence key by key over the
// reference_metadata saved in searchConfig, the config of the request's
// search_id.
func (req *SearchDatasetsRequest) DocumentMetadataSelection(searchConfig map[string]interface{}) (bool, []string) {
	payload := map[string]interface{}{}
	if ref := req.ReferenceMetadata; ref != nil {
		nested := map[string]interface{}{}
		if ref.Include != nil {
			nested["include"] = *ref.Include
		}
		if ref.Fields != nil {
			nested["fields"] = ref.Fields
		}
		payload["reference_metadata"] = nested
	}
	if req.IncludeMetadata != nil {
		payload["include_metadata"] = *req.IncludeMetadata
	}
	if req.MetadataFields != nil {
		payload["metadata_fields"] = req.MetadataFields
	}
	return common.ResolveReferenceMetadata(payload, searchConfig)
}

// SearchDatasetsResponse is the response structure for dataset search results.
type SearchDatasetsResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *map[string]float64      `json:"labels"`
	Total   int64                    `json:"total"`
}

// SearchDatasetRequest is the request structure for searching chunks within one dataset.
type SearchDatasetRequest struct {
	Question                 string                 `json:"question"`
	Page                     *int                   `json:"page,omitempty"`
	PageSize                 *int                   `json:"page_size,omitempty"`
	Size                     *int                   `json:"size,omitempty"`
	RerankCandidatesCount    *int                   `json:"rerank_candidates_count,omitempty"`
	DocumentIDs              []string               `json:"document_ids,omitempty"`
	DocIDs                   []string               `json:"doc_ids,omitempty"`
	UseKG                    *bool                  `json:"use_kg,omitempty"`
	KNNTopK                  *int                   `json:"knn_top_k,omitempty"`
	TopK                     *int                   `json:"top_k,omitempty"` // Legacy alias for knn_top_k.
	KNNNumCandidates         *int                   `json:"knn_num_candidates,omitempty"`
	CrossLanguages           []string               `json:"cross_languages,omitempty"`
	SearchID                 *string                `json:"search_id,omitempty"`
	MetadataCondition        map[string]interface{} `json:"metadata_condition,omitempty"`
	MetadataFilter           map[string]interface{} `json:"meta_data_filter,omitempty"`
	RerankID                 *string                `json:"rerank_id,omitempty"`
	Keyword                  *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold      *float64               `json:"similarity_threshold,omitempty"`
	KeywordsSimilarityWeight *float64               `json:"keywords_similarity_weight,omitempty"`
	VectorSimilarityWeight   *float64               `json:"vector_similarity_weight,omitempty"`
	IncludeCompiledChunks    *bool                  `json:"include_knowledge_compilation,omitempty"`
	IncludeMetadata          *bool                  `json:"include_metadata,omitempty"`
	MetadataFields           []string               `json:"metadata_fields,omitempty"`
	ReferenceMetadata        *ReferenceMetadata     `json:"reference_metadata,omitempty"`

	KnowledgeCompilationScope string `json:"knowledge_compilation_scope,omitempty"`
}

// ToSearchDatasetsRequest converts a single-dataset search request into the multi-dataset form.
func (req *SearchDatasetRequest) ToSearchDatasetsRequest(datasetID string) *SearchDatasetsRequest {
	if req == nil {
		return &SearchDatasetsRequest{DatasetIDs: []string{datasetID}}
	}
	return &SearchDatasetsRequest{
		DatasetIDs:               []string{datasetID},
		Question:                 req.Question,
		Page:                     req.Page,
		PageSize:                 req.PageSize,
		Size:                     req.Size,
		RerankCandidatesCount:    req.RerankCandidatesCount,
		DocumentIDs:              req.DocumentIDs,
		DocIDs:                   req.DocIDs,
		UseKG:                    req.UseKG,
		KNNTopK:                  req.KNNTopK,
		TopK:                     req.TopK,
		KNNNumCandidates:         req.KNNNumCandidates,
		CrossLanguages:           req.CrossLanguages,
		SearchID:                 req.SearchID,
		MetadataCondition:        req.MetadataCondition,
		MetadataFilter:           req.MetadataFilter,
		RerankID:                 req.RerankID,
		Keyword:                  req.Keyword,
		SimilarityThreshold:      req.SimilarityThreshold,
		KeywordsSimilarityWeight: req.KeywordsSimilarityWeight,
		VectorSimilarityWeight:   req.VectorSimilarityWeight,
		IncludeCompiledChunks:    req.IncludeCompiledChunks,
		IncludeMetadata:          req.IncludeMetadata,
		MetadataFields:           req.MetadataFields,
		ReferenceMetadata:        req.ReferenceMetadata,

		KnowledgeCompilationScope: req.KnowledgeCompilationScope,
	}
}

// ValidateKnowledgeCompilationScope checks the supported search scopes.
func ValidateKnowledgeCompilationScope(scope string) error {
	if scope != "" && scope != "dataset" && scope != "document" && scope != "all" {
		return fmt.Errorf("knowledge_compilation_scope must be dataset, document, or all")
	}
	return nil
}

// MetadataConfigField mirrors one field in the dataset metadata config API.
type MetadataConfigField struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Description *string  `json:"description"`
	Enum        []string `json:"enum"`
}

// MetadataConfigRequest mirrors PUT /datasets/:dataset_id/metadata/config.
type MetadataConfigRequest struct {
	Enabled         *bool                 `json:"enabled,omitempty"`
	Metadata        []MetadataConfigField `json:"metadata"`
	BuiltInMetadata []MetadataConfigField `json:"built_in_metadata"`
}

// CreateDatasetRequest represents the request for creating a dataset.
type CreateDatasetRequest struct {
	Name           string                 `json:"name" binding:"required"`
	EmbeddingModel *string                `json:"embedding_model,omitempty"`
	ParserConfig   map[string]interface{} `json:"parser_config,omitempty"`
	Language       *string                `json:"language,omitempty"`
	Permission     *string                `json:"permission,omitempty"`
	ParserID       *string                `json:"parser_id,omitempty"`
	PipelineID     *string                `json:"pipeline_id,omitempty"`
	// ParseType indicates pipeline selection mode: 1 = BuiltIn (parser_id),
	// 2 = Pipeline (pipeline_id). nil means unspecified.
	ParseType *int `json:"parse_type,omitempty"`
}

// DatasetConnectorRequest represents a connector link request.
type DatasetConnectorRequest struct {
	ID        string `json:"id"`
	AutoParse string `json:"auto_parse,omitempty"`
}

// UpdateDatasetRequest represents the request for updating a dataset.
type UpdateDatasetRequest struct {
	Name           *string                    `json:"name,omitempty"`
	Avatar         *string                    `json:"avatar,omitempty"`
	Description    *string                    `json:"description,omitempty"`
	Language       *string                    `json:"language,omitempty"`
	Connectors     *[]DatasetConnectorRequest `json:"connectors,omitempty"`
	EmbdID         *string                    `json:"embd_id,omitempty"`
	EmbeddingModel *string                    `json:"embedding_model,omitempty"`
	Permission     *string                    `json:"permission,omitempty"`
	ParserID       *string                    `json:"parser_id,omitempty"`
	Pagerank       *int64                     `json:"pagerank,omitempty"`
	ParserConfig   map[string]interface{}     `json:"parser_config,omitempty"`
	PipelineID     *string                    `json:"pipeline_id,omitempty"`
	// ParseType indicates pipeline selection mode: 1 = BuiltIn (parser_id),
	// 2 = Pipeline (pipeline_id). nil means unspecified.
	ParseType *int `json:"parse_type,omitempty"`

	// ParserConfigProvided reports whether the raw request body contained a
	// "parser_config" key. An explicitly provided parser_config ({} or null) is a
	// valid no-op that must succeed, unlike a truly empty request body.
	ParserConfigProvided bool `json:"-"`
}
