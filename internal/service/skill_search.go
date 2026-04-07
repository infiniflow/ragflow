//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SkillSearchService handles business logic for skill search operations
type SkillSearchService struct {
	configDAO     *dao.SkillSearchConfigDAO
	modelProvider ModelProvider
}

// NewSkillSearchService creates a new SkillSearchService instance
func NewSkillSearchService() *SkillSearchService {
	return &SkillSearchService{
		configDAO:     dao.NewSkillSearchConfigDAO(),
		modelProvider: NewModelProvider(),
	}
}

// SetModelProvider sets the model provider for embedding generation
func (s *SkillSearchService) SetModelProvider(provider ModelProvider) {
	s.modelProvider = provider
}

// GetConfigRequest represents the request to get skill search config
type GetConfigRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	HubID    string `json:"hub_id"`
}

// GetConfig retrieves the search configuration for a tenant
func (s *SkillSearchService) GetConfig(tenantID, hubID, embdID string) (map[string]interface{}, common.ErrorCode, error) {
	hubID = normalizeHubID(hubID)
	var config *entity.SkillSearchConfig
	var err error

	if embdID == "" {
		// If embd_id is not provided, get the latest config for the tenant
		// Prioritize configs with non-empty embd_id (user-saved configs)
		config, err = s.configDAO.GetLatestByTenantID(tenantID, hubID)
		if err != nil {
			// No config found, return default config
			config = &entity.SkillSearchConfig{
				TenantID:               tenantID,
				HubID:                  hubID,
				EmbdID:                 "",
				VectorSimilarityWeight: 0.3,
				SimilarityThreshold:    0.2,
				FieldConfig: map[string]interface{}{
					"name":        map[string]interface{}{"enabled": true, "weight": 3.0},
					"tags":        map[string]interface{}{"enabled": true, "weight": 2.0},
					"description": map[string]interface{}{"enabled": true, "weight": 1.0},
					"content":     map[string]interface{}{"enabled": false, "weight": 0.5},
				},
				TopK: 10,
			}
		}
	} else {
		config, err = s.configDAO.GetByTenantAndEmbdID(tenantID, hubID, embdID)
		if err != nil {
			// Config not found, create default one
			config, err = s.configDAO.GetOrCreate(tenantID, hubID, embdID)
			if err != nil {
				return nil, common.CodeOperatingError, fmt.Errorf("failed to get or create config: %w", err)
			}
		}
	}

	return config.ToMap(), common.CodeSuccess, nil
}

// UpdateConfigRequest represents the request to update skill search config
type UpdateConfigRequest struct {
	TenantID               string             `json:"tenant_id"`
	HubID                  string             `json:"hub_id"`
	EmbdID                 string             `json:"embd_id" binding:"required"`
	VectorSimilarityWeight float64            `json:"vector_similarity_weight"`
	SimilarityThreshold    float64            `json:"similarity_threshold"`
	FieldConfig            entity.FieldConfig `json:"field_config"`
	RerankID               string             `json:"rerank_id"`
	TopK                   int64              `json:"top_k"`
}

// UpdateConfig updates the search configuration for a tenant
func (s *SkillSearchService) UpdateConfig(req *UpdateConfigRequest) (map[string]interface{}, common.ErrorCode, error) {
	req.HubID = normalizeHubID(req.HubID)
	// Validate vector_similarity_weight
	if req.VectorSimilarityWeight < 0 || req.VectorSimilarityWeight > 1 {
		return nil, common.CodeDataError, errors.New("vector_similarity_weight must be between 0 and 1")
	}

	// Validate similarity_threshold
	if req.SimilarityThreshold < 0 || req.SimilarityThreshold > 1 {
		return nil, common.CodeDataError, errors.New("similarity_threshold must be between 0 and 1")
	}

	// Validate top_k
	if req.TopK <= 0 {
		return nil, common.CodeDataError, errors.New("top_k must be positive")
	}

	config, err := s.configDAO.GetOrCreate(req.TenantID, req.HubID, req.EmbdID)
	if err != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("failed to get config: %w", err)
	}

	fieldConfigMap := entity.JSONMap{
		"name": map[string]interface{}{
			"enabled": req.FieldConfig.Name.Enabled,
			"weight":  req.FieldConfig.Name.Weight,
		},
		"tags": map[string]interface{}{
			"enabled": req.FieldConfig.Tags.Enabled,
			"weight":  req.FieldConfig.Tags.Weight,
		},
		"description": map[string]interface{}{
			"enabled": req.FieldConfig.Description.Enabled,
			"weight":  req.FieldConfig.Description.Weight,
		},
		"content": map[string]interface{}{
			"enabled": req.FieldConfig.Content.Enabled,
			"weight":  req.FieldConfig.Content.Weight,
		},
	}

	updates := map[string]interface{}{
		"vector_similarity_weight": req.VectorSimilarityWeight,
		"similarity_threshold":     req.SimilarityThreshold,
		"field_config":             fieldConfigMap,
		"top_k":                    req.TopK,
	}

	if req.RerankID != "" {
		updates["rerank_id"] = req.RerankID
	}

	if err := s.configDAO.UpdateByTenantAndEmbdID(req.TenantID, req.HubID, req.EmbdID, updates); err != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("failed to update config: %w", err)
	}

	// Refresh config
	config, err = s.configDAO.GetByTenantAndEmbdID(req.TenantID, req.HubID, req.EmbdID)
	if err != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("failed to refresh config: %w", err)
	}

	return config.ToMap(), common.CodeSuccess, nil
}

// SearchRequest represents the skill search request
type SearchRequest struct {
	TenantID string `json:"tenant_id"` // Set from user context, not from request body
	HubID    string `json:"hub_id"`
	Query    string `json:"query" binding:"required"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// SearchResponse represents the skill search response
type SearchResponse struct {
	Skills     []entity.SkillSearchResult `json:"skills"` // Changed from "results" to match frontend
	Total      int64                      `json:"total"`
	Query      string                     `json:"query"`
	SearchType string                     `json:"search_type"` // "keyword", "vector", "hybrid"
}

// Search performs skill search with the configured strategy
func (s *SkillSearchService) Search(ctx context.Context, req *SearchRequest, docEngine engine.DocEngine) (*SearchResponse, common.ErrorCode, error) {
	req.HubID = normalizeHubID(req.HubID)
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	// Check if index exists before searching
	indexName := getSkillIndexName(req.TenantID, req.HubID)

	indexExists, err := docEngine.IndexExists(ctx, indexName)
	if err != nil {
		logger.Error("Failed to check index existence", err)
		return nil, common.CodeOperatingError, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !indexExists {
		return nil, common.CodeOperatingError, fmt.Errorf("search index '%s' for hub '%s' does not exist. This usually means:\n\n"+
			"1. No skills have been uploaded to this hub yet\n"+
			"2. The skill was uploaded by a different user (index is per-user)\n"+
			"3. The index was deleted\n\n"+
			"Please upload at least one skill to this hub using the same user account.", indexName, req.HubID)
	}

	// Get config for search strategy
	// Use GetLatestByTenantID to prioritize configs with non-empty embd_id
	config, err := s.configDAO.GetLatestByTenantID(req.TenantID, req.HubID)
	if err != nil {
		// Use default config if not found
		config = &entity.SkillSearchConfig{
			HubID:                  req.HubID,
			VectorSimilarityWeight: 0.3,
			SimilarityThreshold:    0.2,
			FieldConfig: map[string]interface{}{
				"name":        map[string]interface{}{"enabled": true, "weight": 3.0},
				"tags":        map[string]interface{}{"enabled": true, "weight": 2.0},
				"description": map[string]interface{}{"enabled": true, "weight": 1.0},
				"content":     map[string]interface{}{"enabled": false, "weight": 0.5},
			},
			TopK: 10,
		}
	}

	var results []entity.SkillSearchResult
	searchType := "hybrid"

	// Check if embedding model is configured
	hasEmbdConfig := config.EmbdID != ""

	switch {
	case config.VectorSimilarityWeight == 0 || !hasEmbdConfig:
		// Pure keyword search (BM25)
		// Also fallback to keyword search if no embedding model configured
		searchType = "keyword"
		results, err = s.keywordSearch(ctx, docEngine, indexName, req.Query, config)
	case config.VectorSimilarityWeight == 1:
		// Pure vector search
		searchType = "vector"
		results, err = s.vectorSearch(ctx, docEngine, indexName, req.Query, config, req.TenantID)
		if err != nil {
			logger.Warn("Vector search failed, falling back to keyword search", zap.Error(err))
			searchType = "keyword"
			results, err = s.keywordSearch(ctx, docEngine, indexName, req.Query, config)
		}
	default:
		// Hybrid search
		results, err = s.hybridSearch(ctx, docEngine, indexName, req.Query, config, req.TenantID)
	}

	if err != nil {
		logger.Error("Skill search failed", err)
		return nil, common.CodeOperatingError, fmt.Errorf("search failed: %w", err)
	}

	// Apply pagination
	total := int64(len(results))
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}
	paginatedResults := results[start:end]

	return &SearchResponse{
		Skills:     paginatedResults,
		Total:      total,
		Query:      req.Query,
		SearchType: searchType,
	}, common.CodeSuccess, nil
}

// keywordSearch performs pure keyword search using BM25
func (s *SkillSearchService) keywordSearch(ctx context.Context, docEngine engine.DocEngine, indexName, query string, config *entity.SkillSearchConfig) ([]entity.SkillSearchResult, error) {
	// Analyze query: tokenize and extract keywords
	matchText, keywords := s.analyzeQuery(query)

	// Use unified search request with analyzed query
	searchReq := &types.SearchRequest{
		IndexNames:          []string{indexName},
		Question:            query,
		MatchText:           matchText,
		Keywords:            keywords,
		KeywordOnly:         true,
		Page:                1,
		Size:                100,
		TopK:                100,
		SimilarityThreshold: config.SimilarityThreshold,
	}

	resp, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, err
	}

	// Parse response
	searchResp, ok := resp.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type: %T", resp)
	}

	// Convert chunks to SkillSearchResult
	return s.convertChunksToResults(searchResp.Chunks, config.SimilarityThreshold), nil
}

// vectorSearch performs pure vector search
func (s *SkillSearchService) vectorSearch(ctx context.Context, docEngine engine.DocEngine, indexName, query string, config *entity.SkillSearchConfig, tenantID string) ([]entity.SkillSearchResult, error) {
	// Get embedding for query
	vector, err := s.getEmbedding(ctx, query, config.EmbdID, tenantID)
	if err != nil {
		logger.Warn("Vector search: failed to get embedding, will fallback to keyword search",
			zap.String("embdID", config.EmbdID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}
	logger.Debug("Vector search: successfully got embedding",
		zap.String("embdID", config.EmbdID),
		zap.Int("dimension", len(vector)))

	// Analyze query for potential keyword filtering
	matchText, keywords := s.analyzeQuery(query)

	// Use unified search request
	searchReq := &types.SearchRequest{
		IndexNames:             []string{indexName},
		Question:               query,
		MatchText:              matchText,
		Keywords:               keywords,
		Vector:                 vector,
		KeywordOnly:            false,
		Page:                   1,
		Size:                   100,
		TopK:                   int(config.TopK),
		SimilarityThreshold:    config.SimilarityThreshold,
		VectorSimilarityWeight: 1.0, // Pure vector search
	}

	resp, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		logger.Warn("Vector search: search execution failed",
			zap.String("indexName", indexName),
			zap.Error(err))
		return nil, err
	}

	// Parse response
	searchResp, ok := resp.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type: %T", resp)
	}

	results := s.convertChunksToResults(searchResp.Chunks, config.SimilarityThreshold)
	logger.Debug("Vector search: completed",
		zap.Int("totalChunks", len(searchResp.Chunks)),
		zap.Int("filteredResults", len(results)))

	// If no results, return error to trigger fallback
	if len(results) == 0 {
		logger.Info("Vector search: no results found, will fallback to keyword search",
			zap.String("indexName", indexName),
			zap.String("query", query))
		return nil, fmt.Errorf("vector search returned no results")
	}

	return results, nil
}

// hybridSearch performs hybrid search combining BM25 and vector search
func (s *SkillSearchService) hybridSearch(ctx context.Context, docEngine engine.DocEngine, indexName, query string, config *entity.SkillSearchConfig, tenantID string) ([]entity.SkillSearchResult, error) {
	// Analyze query first: tokenize and extract keywords
	matchText, keywords := s.analyzeQuery(query)

	// Get embedding for query
	vector, err := s.getEmbedding(ctx, query, config.EmbdID, tenantID)
	if err != nil {
		logger.Warn("Hybrid search: failed to get embedding, falling back to keyword search",
			zap.String("embdID", config.EmbdID),
			zap.Error(err))
		// Fallback to keyword search with analyzed query
		return s.executeKeywordSearch(ctx, docEngine, indexName, query, matchText, keywords, config)
	}
	logger.Debug("Hybrid search: successfully got embedding",
		zap.String("embdID", config.EmbdID),
		zap.Int("dimension", len(vector)))

	// Use unified search request for hybrid search with analyzed query
	searchReq := &types.SearchRequest{
		IndexNames:             []string{indexName},
		Question:               query,
		MatchText:              matchText,
		Keywords:               keywords,
		Vector:                 vector,
		KeywordOnly:            false,
		Page:                   1,
		Size:                   100,
		TopK:                   int(config.TopK),
		SimilarityThreshold:    config.SimilarityThreshold,
		VectorSimilarityWeight: config.VectorSimilarityWeight,
	}

	resp, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		logger.Warn("Hybrid search: search execution failed, falling back to keyword search",
			zap.String("indexName", indexName),
			zap.Error(err))
		return s.executeKeywordSearch(ctx, docEngine, indexName, query, matchText, keywords, config)
	}

	// Parse response
	searchResp, ok := resp.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type: %T", resp)
	}

	results := s.convertChunksToResults(searchResp.Chunks, config.SimilarityThreshold)
	logger.Debug("Hybrid search: completed",
		zap.Int("totalChunks", len(searchResp.Chunks)),
		zap.Int("filteredResults", len(results)))

	// If no results, fallback to keyword search
	if len(results) == 0 {
		logger.Info("Hybrid search: no results found, falling back to keyword search",
			zap.String("indexName", indexName),
			zap.String("query", query))
		return s.executeKeywordSearch(ctx, docEngine, indexName, query, matchText, keywords, config)
	}

	return results, nil
}

// executeKeywordSearch executes a keyword search (used for fallback)
func (s *SkillSearchService) executeKeywordSearch(ctx context.Context, docEngine engine.DocEngine, indexName, query, matchText string, keywords []string, config *entity.SkillSearchConfig) ([]entity.SkillSearchResult, error) {
	logger.Info("Executing fallback keyword search",
		zap.String("indexName", indexName),
		zap.String("query", query))

	searchReq := &types.SearchRequest{
		IndexNames:          []string{indexName},
		Question:            query,
		MatchText:           matchText,
		Keywords:            keywords,
		KeywordOnly:         true,
		Page:                1,
		Size:                100,
		TopK:                100,
		SimilarityThreshold: config.SimilarityThreshold,
	}

	resp, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		logger.Error("Keyword search fallback failed", err)
		return nil, err
	}

	searchResp, ok := resp.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type: %T", resp)
	}

	results := s.convertChunksToResults(searchResp.Chunks, config.SimilarityThreshold)
	logger.Info("Keyword search fallback completed",
		zap.Int("totalChunks", len(searchResp.Chunks)),
		zap.Int("results", len(results)))

	return results, nil
}

// convertChunksToResults converts search chunks to SkillSearchResult
// Deduplicates by skill name, keeping only the highest scored result for each skill
func (s *SkillSearchService) convertChunksToResults(chunks []map[string]interface{}, threshold float64) []entity.SkillSearchResult {
	// Use a map to deduplicate by skill name, keeping the highest scored version
	skillMap := make(map[string]entity.SkillSearchResult)

	for _, chunk := range chunks {
		// Get score
		score := 0.0
		if scoreVal, ok := chunk["_score"].(float64); ok {
			score = scoreVal
		}

		// Filter by threshold
		if score < threshold {
			continue
		}

		// Extract fields
		skillID := getString(chunk, "skill_id")
		folderID := getString(chunk, "folder_id")
		name := getString(chunk, "name")
		description := getString(chunk, "description")

		// Extract tags
		var tags []string
		if tagsVal, ok := chunk["tags"].([]interface{}); ok {
			for _, tag := range tagsVal {
				if tagStr, ok := tag.(string); ok {
					tags = append(tags, tagStr)
				}
			}
		}

		// Use skill name as the deduplication key (skillID may contain version suffix)
		skillKey := name
		if skillKey == "" {
			skillKey = skillID
		}

		// Extract create_time
		var createTime int64
		if ctVal, ok := chunk["create_time"].(float64); ok {
			createTime = int64(ctVal)
		} else if ctVal, ok := chunk["create_time"].(int64); ok {
			createTime = ctVal
		}

		result := entity.SkillSearchResult{
			SkillID:     skillID,
			FolderID:    folderID,
			Name:        name,
			Description: description,
			Tags:        tags,
			Score:       score,
			CreateTime:  createTime,
		}

		// Keep only the highest scored result for each skill
		if existing, ok := skillMap[skillKey]; !ok || score > existing.Score {
			skillMap[skillKey] = result
		}
	}

	// Convert map to slice
	var results []entity.SkillSearchResult
	for _, result := range skillMap {
		results = append(results, result)
	}

	// Sort by score descending
	sortResults(results)

	return results
}

// getEmbedding generates embedding for text using the specified model
func (s *SkillSearchService) getEmbedding(ctx context.Context, text, embdID, tenantID string) ([]float64, error) {
	if s.modelProvider == nil {
		return nil, fmt.Errorf("model provider not set")
	}

	if embdID == "" {
		return nil, fmt.Errorf("embedding model ID not configured")
	}

	embeddingModel, err := s.modelProvider.GetEmbeddingModel(ctx, tenantID, embdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}

	// Truncate text to prevent exceeding model's max length (consistent with Python implementation)
	maxLength := embeddingModel.MaxLength()
	truncatedText := truncate(text, maxLength-10)

	vector, err := embeddingModel.EncodeQuery(truncatedText)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	return vector, nil
}

// Helper functions
func getSkillIndexName(tenantID, hubID string) string {
	hubID = normalizeHubID(hubID)
	hubID = strings.ToLower(hubID)
	replacer := strings.NewReplacer("-", "_", "/", "_", "\\", "_", " ", "_", ".", "_", ":", "_")
	sanitizedHubID := replacer.Replace(hubID)
	// Use shortened IDs to keep index name readable but unique
	// First 12 chars of UUID provides 48-bit entropy, sufficient for uniqueness
	shortTenant := tenantID
	if len(tenantID) > 12 {
		shortTenant = tenantID[:12]
	}
	shortHub := sanitizedHubID
	if len(sanitizedHubID) > 12 {
		shortHub = sanitizedHubID[:12]
	}
	return fmt.Sprintf("skill_%s_%s", shortTenant, shortHub)
}

func normalizeHubID(hubID string) string {
	hubID = strings.TrimSpace(hubID)
	if hubID == "" {
		return "default"
	}
	return hubID
}



func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func sortResults(results []entity.SkillSearchResult) {
	// Simple bubble sort for now, could use sort.Slice
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// GenerateID generates a unique ID
func generateID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}

// CalculateContentHash calculates SHA256 hash of skill content
func CalculateContentHash(name, description string, tags []string, content string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(description))
	for _, tag := range tags {
		h.Write([]byte(tag))
	}
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// BuildVectorText builds the text for vector generation
func BuildVectorText(name, description string, tags []string, content string, fieldConfig entity.FieldConfig) string {
	var parts []string

	if fieldConfig.Name.Enabled && name != "" {
		parts = append(parts, name)
	}
	if fieldConfig.Tags.Enabled && len(tags) > 0 {
		parts = append(parts, strings.Join(tags, " "))
	}
	if fieldConfig.Description.Enabled && description != "" {
		parts = append(parts, description)
	}
	if fieldConfig.Content.Enabled && content != "" {
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n")
}

// analyzeQuery analyzes the search query and extracts keywords
// Similar to Python's FulltextQueryer.question method
func (s *SkillSearchService) analyzeQuery(query string) (matchText string, keywords []string) {
	if query == "" {
		return "", nil
	}

	// Clean and normalize query
	cleaned := s.cleanQueryText(query)

	// Extract keywords by tokenizing
	keywords = s.tokenize(cleaned)

	// Build match text for ES query_string
	// Similar to Python's query building logic
	matchText = s.buildMatchText(cleaned, keywords)

	return matchText, keywords
}

// cleanQueryText cleans and normalizes query text
func (s *SkillSearchService) cleanQueryText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Replace special characters with spaces
	// Similar to Python: re.sub(r"[ :|\r\n\t,，。？?/`!！&^%%()\[\]{}<>]+", " ", text)
	specialChars := []string{
		":", "|", "\r", "\n", "\t", ",", "，", "。", "？", "?", "/", "`",
		"!", "！", "&", "^", "%", "(", ")", "[", "]", "{", "}", "<", ">",
	}
	for _, char := range specialChars {
		text = strings.ReplaceAll(text, char, " ")
	}

	// Remove extra spaces
	fields := strings.Fields(text)
	return strings.Join(fields, " ")
}

// tokenize splits text into tokens/keywords
func (s *SkillSearchService) tokenize(text string) []string {
	if text == "" {
		return nil
	}

	// Simple tokenization by splitting on whitespace
	// For Chinese text, this keeps characters together
	fields := strings.Fields(text)

	// Remove duplicates and empty strings
	seen := make(map[string]bool)
	var keywords []string
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		keywords = append(keywords, field)

		// For longer tokens, also add sub-tokens (for Chinese fine-grained tokenization)
		if len([]rune(field)) > 2 {
			runes := []rune(field)
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				if !seen[bigram] {
					seen[bigram] = true
					keywords = append(keywords, bigram)
				}
			}
		}
	}

	// Limit keywords to avoid too many
	if len(keywords) > 32 {
		keywords = keywords[:32]
	}

	return keywords
}

// buildMatchText builds the match text for ES query_string
// Similar to Python's FulltextQueryer.question output
func (s *SkillSearchService) buildMatchText(originalText string, keywords []string) string {
	if len(keywords) == 0 {
		return originalText
	}

	// Build boosted query for keywords
	// Similar to Python: "(keyword1^weight1 keyword2^weight2 ...)"
	var parts []string

	// Add the original text with high boost
	if originalText != "" {
		parts = append(parts, fmt.Sprintf("(\"%s\")^2.0", originalText))
	}

	// Add individual keywords with decreasing weights
	for i, keyword := range keywords {
		if keyword == "" {
			continue
		}
		// First few keywords get higher weight
		weight := 1.0
		if i < 3 {
			weight = 1.5
		} else if i < 6 {
			weight = 1.2
		}

		// Escape special characters in keyword
		escaped := s.escapeQueryString(keyword)
		parts = append(parts, fmt.Sprintf("(%s)^%.1f", escaped, weight))
	}

	// Join with OR operator
	return strings.Join(parts, " OR ")
}

// escapeQueryString escapes special characters for ES query_string
func (s *SkillSearchService) escapeQueryString(text string) string {
	specialChars := []string{"\\", "+", "-", "=", "&&", "||", ">", "<", "!", "(", ")", "{", "}", "[", "]", "^", "\"", "~", "*", "?", ":", "/"}
	result := text
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// SkillInfo represents skill information for indexing
type SkillInfo struct {
	ID          string   `json:"id"`
	FolderID    string   `json:"folder_id"` // File system folder ID for retrieving files
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Content     string   `json:"content"`
}

// IndexSkillsRequest represents the request to index skills
type IndexSkillsRequest struct {
	TenantID string      `json:"tenant_id" binding:"required"`
	Skills   []SkillInfo `json:"skills" binding:"required"`
}

// ReindexRequest represents the request to reindex all skills
type ReindexRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	HubID    string `json:"hub_id" binding:"required"`
	EmbdID   string `json:"embd_id"` // Optional, will use config's embd_id if empty
}
