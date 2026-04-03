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
	"encoding/json"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"ragflow/internal/tokenizer"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SkillVersionInfo represents a skill version in the file system
type SkillVersionInfo struct {
	SkillName   string   `json:"skill_name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Content     string   `json:"content"`
}

// FileSystemClient defines the interface for accessing skill files
type FileSystemClient interface {
	ListSkills(ctx context.Context, tenantID string) ([]SkillVersionInfo, error)
	GetSkillContent(ctx context.Context, tenantID, skillName string) (*SkillVersionInfo, error)
}

// SkillIndexerService handles skill indexing operations
type SkillIndexerService struct {
	configDAO     *dao.SkillSearchConfigDAO
	modelProvider ModelProvider
}

// NewSkillIndexerService creates a new SkillIndexerService instance
func NewSkillIndexerService() *SkillIndexerService {
	return &SkillIndexerService{
		configDAO:     dao.NewSkillSearchConfigDAO(),
		modelProvider: NewModelProvider(),
	}
}

// isElasticsearch checks if the engine is Elasticsearch
func isElasticsearch(docEngine engine.DocEngine) bool {
	return docEngine.GetType() == "elasticsearch"
}

// IndexSkill indexes a single skill
// Uses skill_id as doc_id for direct mapping, with version control for incremental updates
// For ES: xxx fields store original content, xxx_tks fields store RAG-tokenized content (space-separated)
// For Infinity: only xxx fields with built-in rag-analyzer
func (s *SkillIndexerService) IndexSkill(ctx context.Context, tenantID, hubID string, skill SkillInfo, docEngine engine.DocEngine, embdID string) error {
	hubID = normalizeHubID(hubID)

	config, err := s.configDAO.GetOrCreate(tenantID, hubID, embdID)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Get field config
	fieldConfig := entity.DefaultFieldConfig()
	if config.FieldConfig != nil {
		if fcJSON, err := json.Marshal(config.FieldConfig); err == nil {
			json.Unmarshal(fcJSON, &fieldConfig)
		}
	}

	// Build vector text from enabled fields
	vectorText := BuildVectorText(skill.Name, skill.Description, skill.Tags, skill.Content, fieldConfig)

	// Generate embedding (optional - continue on failure)
	vector, err := s.generateEmbedding(ctx, vectorText, embdID, tenantID)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to generate embedding for skill %s: %v. Continuing with text-only index.", skill.ID, err))
	}

	// Build document with RAG tokenization for ES
	now := time.Now()
	timestamp := now.UnixMilli()

	// Use actual vector dimension or default
	dimension := getEmbeddingDimension(embdID)
	if vector != nil {
		dimension = len(vector)
	}
	vectorField := fmt.Sprintf("q_%d_vec", dimension)

	// Determine engine type
	isES := isElasticsearch(docEngine)

	// Build base document
	doc := map[string]interface{}{
		"skill_id":    skill.ID,
		"hub_id":      hubID,
		"folder_id":   skill.FolderID,
		"name":        skill.Name,
		"tags":        strings.Join(skill.Tags, ", "),
		"description": skill.Description,
		"content":     skill.Content,
		"version":     config.IndexVersion,
		"status":      "1",
		"create_time": timestamp,
		"update_time": timestamp,
	}

	// Add vector if available
	if vector != nil {
		doc[vectorField] = vector
	} else if docEngine.GetType() == "infinity" {
		// For Infinity: use zero vector as placeholder
		doc[vectorField] = make([]float64, dimension)
	}

	// For ES: add tokenized fields for BM25 search
	// For Infinity: fields have built-in analyzer, no need for xxx_tks
	if isES {
		nameTokens, _ := tokenizer.Tokenize(skill.Name)
		tagsText := strings.Join(skill.Tags, " ")
		tagsTokens, _ := tokenizer.Tokenize(tagsText)

		doc["name_tks"] = nameTokens
		doc["tags_tks"] = tagsTokens

		if fieldConfig.Description.Enabled {
			descTokens, _ := tokenizer.Tokenize(skill.Description)
			doc["description_tks"] = descTokens
		}
		if fieldConfig.Content.Enabled {
			contentTokens, _ := tokenizer.Tokenize(skill.Content)
			doc["content_tks"] = contentTokens
		}
	}

	indexName := getSkillIndexName(tenantID, hubID)

	// For Infinity: ensure table exists with correct dimension BEFORE inserting
	if docEngine.GetType() == "infinity" {
		exists, _ := docEngine.IndexExists(ctx, indexName)
		if !exists {
			logger.Info(fmt.Sprintf("Creating Infinity table with dimension %d", dimension))
			if err := s.createIndexWithDimension(ctx, tenantID, hubID, docEngine, embdID, dimension); err != nil {
				return fmt.Errorf("failed to create index with dimension %d: %w", dimension, err)
			}
		}
	}

	// ES document ID cannot contain '/' - replace with '_'
	docID := strings.ReplaceAll(skill.ID, "/", "_")
	logger.Info(fmt.Sprintf("Calling IndexDocument: indexName=%s, docID=%s, engineType=%s", indexName, docID, docEngine.GetType()))
	if err := docEngine.IndexDocument(ctx, indexName, docID, doc); err != nil {
		logger.Error(fmt.Sprintf("IndexDocument failed: indexName=%s, docID=%s", indexName, docID), err)
		return fmt.Errorf("failed to index document: %w", err)
	}
	logger.Info(fmt.Sprintf("IndexDocument succeeded: indexName=%s, docID=%s", indexName, docID))

	return nil
}

// BatchIndexSkills indexes multiple skills in batch
// Optimized to use batch embedding API for better performance
func (s *SkillIndexerService) BatchIndexSkills(ctx context.Context, tenantID, hubID string, skills []SkillInfo, docEngine engine.DocEngine, embdID string) error {
	hubID = normalizeHubID(hubID)
	if len(skills) == 0 {
		return nil
	}

	config, err := s.configDAO.GetOrCreate(tenantID, hubID, embdID)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Get field config
	fieldConfig := entity.DefaultFieldConfig()
	if config.FieldConfig != nil {
		if fcJSON, err := json.Marshal(config.FieldConfig); err == nil {
			json.Unmarshal(fcJSON, &fieldConfig)
		}
	}

	// Build vector texts for all skills
	vectorTexts := make([]string, len(skills))
	for i, skill := range skills {
		vectorTexts[i] = BuildVectorText(skill.Name, skill.Description, skill.Tags, skill.Content, fieldConfig)
	}

	// Generate embeddings in batch FIRST to get actual dimension
	logger.Info(fmt.Sprintf("Generating embeddings for %d skills with embdID=%s", len(skills), embdID))
	vectors, err := s.generateEmbeddings(ctx, vectorTexts, embdID, tenantID)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to generate embeddings: %v. Continuing with text-only index.", err))
		vectors = nil // Continue without vectors
	} else {
		logger.Info(fmt.Sprintf("Generated %d vectors", len(vectors)))
	}

	// Get actual vector dimension from generated embeddings
	dimension := getEmbeddingDimension(embdID) // default from config
	if len(vectors) > 0 && len(vectors[0]) > 0 {
		dimension = len(vectors[0])
		logger.Info(fmt.Sprintf("ACTUAL EMBEDDING DIMENSION: %d", dimension))
	} else {
		logger.Info(fmt.Sprintf("Using default dimension: %d (no embeddings generated)", dimension))
	}
	vectorField := fmt.Sprintf("q_%d_vec", dimension)

	// Ensure index exists with correct dimension
	indexName := getSkillIndexName(tenantID, hubID)
	if docEngine.GetType() == "infinity" {
		// For Infinity: must ensure table exists with correct dimension BEFORE inserting
		logger.Info(fmt.Sprintf("Checking if index exists: %s", indexName))
		exists, err := docEngine.IndexExists(ctx, indexName)
		if err != nil {
			logger.Warn(fmt.Sprintf("Error checking index existence: %v", err))
		}
		logger.Info(fmt.Sprintf("Index exists: %v", exists))

		needsCreate := true
		if exists {
			// Check if we can get the table's vector column dimension
			// If dimension mismatch, delete and recreate
			predictedDim := getEmbeddingDimension(embdID)
			if predictedDim != dimension {
				logger.Info(fmt.Sprintf("Dimension mismatch: predicted=%d, actual=%d. Recreating table.", predictedDim, dimension))
				if err := docEngine.DeleteIndex(ctx, indexName); err != nil {
					logger.Warn(fmt.Sprintf("Failed to delete existing index: %v", err))
				}
			} else {
				needsCreate = false
				logger.Info("Table exists with correct dimension")
			}
		}

		if needsCreate {
			logger.Info(fmt.Sprintf("Creating index with actual dimension %d", dimension))
			if err := s.createIndexWithDimension(ctx, tenantID, hubID, docEngine, embdID, dimension); err != nil {
				return fmt.Errorf("failed to create index with dimension %d: %w", dimension, err)
			}
			logger.Info("Index created successfully")
		} else {
			logger.Info("Skipping index creation, already exists with correct dimension")
		}
	} else {
		// For ES: just ensure index exists
		if err := s.EnsureIndex(ctx, tenantID, hubID, docEngine, embdID); err != nil {
			return fmt.Errorf("failed to ensure index exists: %w", err)
		}
	}

	// Index all skills
	now := time.Now()
	timestamp := now.UnixMilli()
	isES := isElasticsearch(docEngine)

	var indexErrors []string
	for i, skill := range skills {
		// ES document ID cannot contain '/' - replace with '_'
		docID := strings.ReplaceAll(skill.ID, "/", "_")

		doc := map[string]interface{}{
			"skill_id":    skill.ID,
			"hub_id":      hubID,
			"folder_id":   skill.FolderID,
			"name":        skill.Name,
			"tags":        strings.Join(skill.Tags, ", "),
			"description": skill.Description,
			"content":     skill.Content,
			"version":     config.IndexVersion,
			"status":      "1",
			"create_time": timestamp,
			"update_time": timestamp,
		}

		// Add vector only if available
		if vectors != nil && i < len(vectors) {
			doc[vectorField] = vectors[i]
		} else {
			logger.Info(fmt.Sprintf("No vector for skill %s, creating text-only index", skill.ID))
			// For Infinity: use zero vector as placeholder (table schema requires vector column)
			if docEngine.GetType() == "infinity" {
				zeroVector := make([]float64, dimension)
				doc[vectorField] = zeroVector
			}
		}

		// For ES: add tokenized fields for BM25 search
		if isES {
			nameTokens, _ := tokenizer.Tokenize(skill.Name)
			tagsText := strings.Join(skill.Tags, " ")
			tagsTokens, _ := tokenizer.Tokenize(tagsText)

			doc["name_tks"] = nameTokens
			doc["tags_tks"] = tagsTokens

			if fieldConfig.Description.Enabled {
				descTokens, _ := tokenizer.Tokenize(skill.Description)
				doc["description_tks"] = descTokens
			}
			if fieldConfig.Content.Enabled {
				contentTokens, _ := tokenizer.Tokenize(skill.Content)
				doc["content_tks"] = contentTokens
			}
		}

		logger.Info("Batch: Calling IndexDocument", zap.String("indexName", indexName), zap.String("docID", docID), zap.Int("index", i))
		if err := docEngine.IndexDocument(ctx, indexName, docID, doc); err != nil {
			logger.Error(fmt.Sprintf("Failed to index skill %s", skill.ID), err)
			indexErrors = append(indexErrors, fmt.Sprintf("%s: %v", skill.ID, err))
			continue
		}
	}

	if len(indexErrors) > 0 {
		return fmt.Errorf("failed to index %d skill(s): %s", len(indexErrors), strings.Join(indexErrors, "; "))
	}

	return nil
}

// DeleteSkillIndex deletes a skill's index by skill ID
// Returns nil if the document doesn't exist (idempotent delete)
func (s *SkillIndexerService) DeleteSkillIndex(ctx context.Context, tenantID, hubID, skillID string, docEngine engine.DocEngine) error {
	hubID = normalizeHubID(hubID) // Normalize hub ID to match indexing
	indexName := getSkillIndexName(tenantID, hubID)
	// ES document ID cannot contain '/' - replace with '_'
	docID := strings.ReplaceAll(skillID, "/", "_")
	if err := docEngine.DeleteDocument(ctx, indexName, docID); err != nil {
		// Check if it's a "not found" error - this is OK, document might not have been indexed
		if strings.Contains(err.Error(), "not found") {
			logger.Info(fmt.Sprintf("Document %s not found in index %s, treating as already deleted", skillID, indexName))
			return nil
		}
		logger.Error(fmt.Sprintf("Failed to delete document %s from index %s", skillID, indexName), err)
		return err
	}
	return nil
}

// DeleteSkillByName deletes a skill's index by skill name (used when deleting a skill)
func (s *SkillIndexerService) DeleteSkillByName(ctx context.Context, tenantID, hubID, skillName string, docEngine engine.DocEngine) error {
	// Use skill name as doc_id
	return s.DeleteSkillIndex(ctx, tenantID, hubID, skillName, docEngine)
}

// UpdateSkillVersion updates a skill's index when version changes
// Deletes old version and indexes new version
func (s *SkillIndexerService) UpdateSkillVersion(ctx context.Context, tenantID, hubID string, skill SkillInfo, docEngine engine.DocEngine, embdID string) error {
	// Delete old version first (upsert behavior)
	if err := s.DeleteSkillByName(ctx, tenantID, hubID, skill.Name, docEngine); err != nil {
		// Log but don't fail - the document might not exist
		logger.Info(fmt.Sprintf("No existing index to delete for skill %s", skill.Name))
	}

	// Index new version
	return s.IndexSkill(ctx, tenantID, hubID, skill, docEngine, embdID)
}

// ReindexAll reindexes all skills for a tenant
// Increments semantic version, deletes each skill's old version document, and indexes new version
func (s *SkillIndexerService) ReindexAll(ctx context.Context, tenantID, hubID string, skills []SkillInfo, docEngine engine.DocEngine, embdID string) (map[string]interface{}, error) {
	hubID = normalizeHubID(hubID)
	// Get current config and increment semantic version
	config, err := s.configDAO.GetOrCreate(tenantID, hubID, embdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// Increment semantic version (e.g., "1.0.0" -> "1.0.1" or "1.0.9" -> "1.1.0")
	newVersion := incrementSemanticVersion(config.IndexVersion)
	if err := s.configDAO.UpdateByTenantID(tenantID, hubID, map[string]interface{}{
		"index_version": newVersion,
	}); err != nil {
		return nil, fmt.Errorf("failed to update version: %w", err)
	}

	// Index all skills with new version (upsert behavior)
	// Each skill_id will be overwritten with new version data
	successCount := 0
	failedSkills := []string{}

	for _, skill := range skills {
		if err := s.IndexSkill(ctx, tenantID, hubID, skill, docEngine, embdID); err != nil {
			logger.Error(fmt.Sprintf("Failed to index skill %s", skill.ID), err)
			failedSkills = append(failedSkills, skill.ID)
			continue
		}
		successCount++
	}

	// Clean up old version documents
	if err := s.cleanupOldVersions(ctx, tenantID, hubID, newVersion, docEngine); err != nil {
		logger.Error("Failed to cleanup old versions", err)
	}

	result := map[string]interface{}{
		"indexed_count": successCount,
		"total_skills":  len(skills),
		"version":       newVersion,
		"failed_count":  len(failedSkills),
	}

	if len(failedSkills) > 0 {
		result["failed_skills"] = failedSkills
	}

	return result, nil
}

// incrementSemanticVersion increments the patch version of a semantic version string
// Supports format: "major.minor.patch" (e.g., "1.0.0" -> "1.0.1")
// If version is empty or invalid, returns "1.0.0"
func incrementSemanticVersion(version string) string {
	if version == "" {
		return "1.0.0"
	}

	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		// Invalid format, reset to 1.0.0
		return "1.0.0"
	}

	// Try to parse patch version
	var major, minor, patch int
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	fmt.Sscanf(parts[2], "%d", &patch)

	// Increment patch version
	patch++
	if patch > 999 {
		patch = 0
		minor++
		if minor > 999 {
			minor = 0
			major++
		}
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// cleanupOldVersions removes documents with version less than current version
func (s *SkillIndexerService) cleanupOldVersions(ctx context.Context, tenantID, hubID string, currentVersion string, docEngine engine.DocEngine) error {
	// This is a placeholder - actual implementation would:
	// 1. Search for documents where version < currentVersion (semantic version comparison)
	// 2. Delete those documents
	// The actual implementation depends on the search engine's query capabilities
	// For now, we rely on the fact that skill_id is used as doc_id,
	// so re-indexing the same skill_id will overwrite the document
	return nil
}

// InitializeIndex initializes the skill search index for a tenant
func (s *SkillIndexerService) InitializeIndex(ctx context.Context, tenantID, hubID string, docEngine engine.DocEngine, embdID string) error {
	// Check if index exists
	indexName := getSkillIndexName(tenantID, hubID)

	logger.Info("Checking skill index existence", zap.String("indexName", indexName), zap.String("tenantID", tenantID), zap.String("hubID", hubID))

	exists, err := docEngine.IndexExists(ctx, indexName)
	if err != nil {
		logger.Error("Failed to check index existence", err)
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if !exists {
		logger.Info("Skill index does not exist, creating...", zap.String("indexName", indexName))
		return s.createIndex(ctx, tenantID, hubID, docEngine, embdID)
	}

	logger.Info("Skill search index already exists", zap.String("indexName", indexName))
	return nil
}

// createIndex creates the skill index using mapping files
func (s *SkillIndexerService) createIndex(ctx context.Context, tenantID, hubID string, docEngine engine.DocEngine, embdID string) error {
	dimension := getEmbeddingDimension(embdID)
	return s.createIndexWithDimension(ctx, tenantID, hubID, docEngine, embdID, dimension)
}

// createIndexWithDimension creates the skill index with a specific vector dimension
func (s *SkillIndexerService) createIndexWithDimension(ctx context.Context, tenantID, hubID string, docEngine engine.DocEngine, embdID string, dimension int) error {
	indexName := getSkillIndexName(tenantID, hubID)

	logger.Info(fmt.Sprintf("Creating skill index with dimension %d", dimension),
		zap.String("indexName", indexName),
		zap.String("hubID", hubID),
		zap.Int("dimension", dimension),
		zap.String("engineType", docEngine.GetType()))

	// For Infinity: check if table exists and needs recreation (dimension mismatch)
	if docEngine.GetType() == "infinity" {
		exists, err := docEngine.IndexExists(ctx, indexName)
		if err != nil {
			logger.Warn(fmt.Sprintf("Error checking if index exists: %v", err))
		}
		if exists {
			logger.Info(fmt.Sprintf("Index exists, deleting for recreation with dimension %d", dimension),
				zap.String("indexName", indexName))
			if err := docEngine.DeleteIndex(ctx, indexName); err != nil {
				logger.Warn(fmt.Sprintf("Failed to delete existing index: %v", err))
			}
		}
	}

	// Use the doc engine's CreateIndex method with skill-specific mapping
	// The mapping file is loaded from conf/skill_es_mapping.json or conf/skill_infinity_mapping.json
	err := docEngine.CreateIndex(ctx, indexName, "skill", dimension, "")
	if err != nil {
		logger.Error("Failed to create skill index", err)
		return err
	}
	logger.Info("Successfully created skill index", zap.String("indexName", indexName))
	return nil
}

// EnsureIndex ensures the skill index exists for a tenant
func (s *SkillIndexerService) EnsureIndex(ctx context.Context, tenantID, hubID string, docEngine engine.DocEngine, embdID string) error {
	return s.InitializeIndex(ctx, tenantID, hubID, docEngine, embdID)
}

// generateEmbedding generates embedding for text using the specified model
func (s *SkillIndexerService) generateEmbedding(ctx context.Context, text, embdID, tenantID string) ([]float64, error) {
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
		return nil, fmt.Errorf("failed to encode text: %w", err)
	}

	return vector, nil
}

// generateEmbeddings generates embeddings for multiple texts in batch
// This is more efficient than calling generateEmbedding individually
func (s *SkillIndexerService) generateEmbeddings(ctx context.Context, texts []string, embdID, tenantID string) ([][]float64, error) {
	logger.Info(fmt.Sprintf("generateEmbeddings called: texts=%d, embdID=%s, tenantID=%s", len(texts), embdID, tenantID))

	if s.modelProvider == nil {
		return nil, fmt.Errorf("model provider not set")
	}

	if embdID == "" {
		return nil, fmt.Errorf("embedding model ID not configured")
	}

	logger.Info(fmt.Sprintf("Getting embedding model for %s", embdID))
	embeddingModel, err := s.modelProvider.GetEmbeddingModel(ctx, tenantID, embdID)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to get embedding model: %v", err), err)
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	logger.Info(fmt.Sprintf("Got embedding model, maxLength=%d", embeddingModel.MaxLength()))

	// Truncate texts to prevent exceeding model's max length
	maxLength := embeddingModel.MaxLength()
	truncatedTexts := make([]string, len(texts))
	for i, text := range texts {
		truncatedTexts[i] = truncate(text, maxLength-10)
	}

	logger.Info(fmt.Sprintf("Encoding %d texts", len(truncatedTexts)))
	// Use batch encode API (consistent with Python's encode(texts: list))
	vectors, err := embeddingModel.Encode(truncatedTexts)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to encode texts: %v", err), err)
		return nil, fmt.Errorf("failed to encode texts: %w", err)
	}

	logger.Info(fmt.Sprintf("Encoded successfully, got %d vectors", len(vectors)))
	if len(vectors) > 0 {
		logger.Info(fmt.Sprintf("Vector dimension: %d", len(vectors[0])))
	}

	return vectors, nil
}

// truncate truncates text to maxLen characters
// Similar to Python's truncate function in rag/llm/embedding_model.py
func truncate(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}
