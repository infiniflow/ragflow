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

package admin

import (
	"context"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/knowledge_compile"
	servicepkg "ragflow/internal/service"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type userDeletionData struct {
	ownedTenantID string
	datasets      []entity.Knowledgebase
	documents     []entity.Document
	files         []entity.File
	memories      []entity.Memory
	spaces        []entity.SkillSpace
	datasetIDs    []string
	documentIDs   []string
	fileIDs       []string
	docTenants    map[string]string
}

func (s *Service) deleteUserData(ctx context.Context, user *entity.User) (*DeleteUserResult, error) {
	data, err := loadUserDeletionData(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	docEngine := s.deleteEngine
	if docEngine == nil {
		docEngine = engine.Get()
	}
	store := s.deleteStorage
	if store == nil {
		store = storage.GetStorageFactory().GetStorage()
	}
	if err := data.deleteExternalData(ctx, docEngine, store); err != nil {
		return nil, err
	}

	result := &DeleteUserResult{
		Username:       user.Email,
		DeletedDetails: []string{fmt.Sprintf("Drop user: %s", user.Email)},
	}
	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return data.deleteDatabaseRows(ctx, tx, user.ID, result)
	}); err != nil {
		return nil, fmt.Errorf("failed to delete user data: %w", err)
	}
	datasetIDs := make(map[string]struct{}, len(data.datasetIDs))
	for _, id := range data.datasetIDs {
		datasetIDs[id] = struct{}{}
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	publishWarning := false
	for _, document := range data.documents {
		if _, deletingDataset := datasetIDs[document.KbID]; deletingDataset {
			continue
		}
		if err := publishCtx.Err(); err != nil {
			common.Warn("stopped publishing document deletions", zap.Error(err))
			publishWarning = true
			break
		}
		if err := knowledge_compile.PublishDeleted(publishCtx, data.docTenants[document.KbID], document.KbID, document.ID, nil); err != nil {
			common.Warn("failed to publish document deletion", zap.String("document_id", document.ID), zap.Error(err))
			publishWarning = true
		}
	}
	if publishWarning {
		result.DeletedDetails = append(result.DeletedDetails, "- Warning: dataset artifact refresh could not be scheduled for some deleted documents.")
	}
	result.DeletedDetails = append(result.DeletedDetails, "Delete done!")
	common.Info("Delete user success with all related data")
	return result, nil
}

func loadUserDeletionData(ctx context.Context, userID string) (*userDeletionData, error) {
	db := dao.DB.WithContext(ctx)
	var tenants []entity.UserTenant
	if err := db.Where("user_id = ? AND role = ?", userID, "owner").Find(&tenants).Error; err != nil {
		return nil, fmt.Errorf("load owned tenant: %w", err)
	}
	if len(tenants) > 1 {
		return nil, fmt.Errorf("user %s owns more than one tenant", userID)
	}
	data := &userDeletionData{}
	if len(tenants) == 1 {
		data.ownedTenantID = tenants[0].TenantID
	}
	datasetQuery := db.Model(&entity.Knowledgebase{}).Select("id", "tenant_id", "created_by").Where("created_by = ?", userID)
	if data.ownedTenantID != "" {
		datasetQuery = datasetQuery.Or("tenant_id = ?", data.ownedTenantID)
	}
	if err := datasetQuery.Find(&data.datasets).Error; err != nil {
		return nil, fmt.Errorf("load datasets: %w", err)
	}
	for _, dataset := range data.datasets {
		data.datasetIDs = append(data.datasetIDs, dataset.ID)
	}
	if err := db.Model(&entity.Document{}).Select("id", "kb_id", "location", "token_num", "chunk_num").Where("created_by = ?", userID).Find(&data.documents).Error; err != nil {
		return nil, fmt.Errorf("load documents: %w", err)
	}
	seenDocuments := make(map[string]struct{}, len(data.documents))
	for _, document := range data.documents {
		seenDocuments[document.ID] = struct{}{}
	}
	for start := 0; start < len(data.datasetIDs); start += 1000 {
		end := min(start+1000, len(data.datasetIDs))
		var documents []entity.Document
		if err := db.Model(&entity.Document{}).Select("id", "kb_id", "location", "token_num", "chunk_num").Where("kb_id IN ?", data.datasetIDs[start:end]).Find(&documents).Error; err != nil {
			return nil, fmt.Errorf("load dataset documents: %w", err)
		}
		for _, document := range documents {
			if _, seen := seenDocuments[document.ID]; !seen {
				data.documents = append(data.documents, document)
				seenDocuments[document.ID] = struct{}{}
			}
		}
	}
	for _, document := range data.documents {
		data.documentIDs = append(data.documentIDs, document.ID)
	}
	data.docTenants = make(map[string]string)
	if len(data.documents) > 0 {
		deletingDatasets := make(map[string]struct{}, len(data.datasetIDs))
		for _, id := range data.datasetIDs {
			deletingDatasets[id] = struct{}{}
		}
		remainingDatasetIDs := make(map[string]struct{})
		for _, document := range data.documents {
			if _, deleting := deletingDatasets[document.KbID]; !deleting {
				remainingDatasetIDs[document.KbID] = struct{}{}
			}
		}
		if len(remainingDatasetIDs) > 0 {
			ids := make([]string, 0, len(remainingDatasetIDs))
			for id := range remainingDatasetIDs {
				ids = append(ids, id)
			}
			for start := 0; start < len(ids); start += 1000 {
				end := min(start+1000, len(ids))
				var datasets []entity.Knowledgebase
				if err := db.Select("id", "tenant_id").Where("id IN ?", ids[start:end]).Find(&datasets).Error; err != nil {
					return nil, fmt.Errorf("load document tenants: %w", err)
				}
				for _, dataset := range datasets {
					data.docTenants[dataset.ID] = dataset.TenantID
				}
			}
			if len(data.docTenants) != len(ids) {
				return nil, fmt.Errorf("some documents refer to missing datasets")
			}
		}
	}
	fileQuery := db.Model(&entity.File{}).Select("id", "parent_id", "tenant_id", "location", "type", "source_type").Where("created_by = ?", userID)
	if data.ownedTenantID != "" {
		fileQuery = fileQuery.Or("tenant_id = ?", data.ownedTenantID)
	}
	if err := fileQuery.Find(&data.files).Error; err != nil {
		return nil, fmt.Errorf("load files: %w", err)
	}
	for _, file := range data.files {
		data.fileIDs = append(data.fileIDs, file.ID)
	}
	if data.ownedTenantID != "" {
		if err := db.Select("id", "tenant_id").Where("tenant_id = ?", data.ownedTenantID).Find(&data.memories).Error; err != nil {
			return nil, fmt.Errorf("load memories: %w", err)
		}
		if err := db.Select("id", "tenant_id").Where("tenant_id = ?", data.ownedTenantID).Find(&data.spaces).Error; err != nil {
			return nil, fmt.Errorf("load skill spaces: %w", err)
		}
	}
	return data, nil
}

func (data *userDeletionData) deleteExternalData(ctx context.Context, docEngine engine.DocEngine, store storage.Storage) error {
	// External stores cannot join the SQL transaction. Keep the user row until
	// cleanup succeeds so a failed request can be retried.
	if docEngine == nil && (data.ownedTenantID != "" || len(data.datasets) > 0 || len(data.documents) > 0) {
		return fmt.Errorf("document engine is unavailable for user data cleanup")
	}
	if store == nil && (len(data.datasets) > 0 || len(data.documents) > 0 || len(data.files) > 0) {
		return fmt.Errorf("storage is unavailable for user data cleanup")
	}
	datasetIDs := make(map[string]struct{}, len(data.datasetIDs))
	for _, id := range data.datasetIDs {
		datasetIDs[id] = struct{}{}
	}
	for _, document := range data.documents {
		if document.Location == nil || *document.Location == "" {
			continue
		}
		exists, err := store.ObjectExists(ctx, document.KbID, *document.Location)
		if err != nil {
			return fmt.Errorf("check document %s: %w", document.ID, err)
		}
		if !exists {
			common.Warn("Document object already missing", zap.String("document_id", document.ID), zap.String("bucket", document.KbID))
			continue
		}
		if err := store.Remove(ctx, document.KbID, *document.Location); err != nil {
			return fmt.Errorf("remove document %s: %w", document.ID, err)
		}
		common.Info("Removed document object", zap.String("document_id", document.ID), zap.String("bucket", document.KbID))
	}
	for _, dataset := range data.datasets {
		exists, err := store.BucketExistsWithError(ctx, dataset.ID)
		if err != nil {
			return fmt.Errorf("check dataset bucket %s: %w", dataset.ID, err)
		}
		if !exists {
			common.Warn("Dataset bucket already missing", zap.String("bucket", dataset.ID))
			continue
		}
		if err := store.RemoveEmptyBucket(ctx, dataset.ID); err != nil {
			common.Warn("Unable to remove empty dataset bucket", zap.String("bucket", dataset.ID), zap.Error(err))
		} else {
			common.Info("Removed empty dataset bucket", zap.String("bucket", dataset.ID))
		}
	}
	for _, file := range data.files {
		if file.SourceType != string(entity.FileSourceKnowledgebase) && file.Location != nil && *file.Location != "" && file.Type != "folder" {
			exists, err := store.ObjectExists(ctx, file.ParentID, *file.Location)
			if err != nil {
				return fmt.Errorf("check file %s: %w", file.ID, err)
			}
			if !exists {
				common.Warn("File object already missing", zap.String("file_id", file.ID), zap.String("bucket", file.ParentID))
				continue
			}
			if err := store.Remove(ctx, file.ParentID, *file.Location); err != nil {
				return fmt.Errorf("remove file %s: %w", file.ID, err)
			}
			common.Info("Removed file object", zap.String("file_id", file.ID), zap.String("bucket", file.ParentID))
		}
	}
	for _, file := range data.files {
		if file.Type != "folder" || file.SourceType == string(entity.FileSourceKnowledgebase) || file.TenantID != data.ownedTenantID {
			continue
		}
		exists, err := store.BucketExistsWithError(ctx, file.ID)
		if err != nil {
			return fmt.Errorf("check folder bucket %s: %w", file.ID, err)
		}
		if !exists {
			common.Warn("Folder bucket already missing", zap.String("bucket", file.ID))
			continue
		}
		if err := store.RemoveEmptyBucket(ctx, file.ID); err != nil {
			common.Warn("Failed to remove empty folder bucket", zap.String("bucket", file.ID), zap.Error(err))
		} else {
			common.Info("Removed empty folder bucket", zap.String("bucket", file.ID))
		}
	}
	if docEngine == nil {
		return nil
	}
	if data.ownedTenantID != "" {
		ownedDatasetIDs := make([]string, 0)
		for _, dataset := range data.datasets {
			if dataset.TenantID == data.ownedTenantID {
				ownedDatasetIDs = append(ownedDatasetIDs, dataset.ID)
			}
		}
		if err := dropUserChunkStore(ctx, docEngine, servicepkg.IndexName(data.ownedTenantID), ownedDatasetIDs); err != nil {
			return fmt.Errorf("remove tenant chunks: %w", err)
		}
		if err := dropUserChunkStore(ctx, docEngine, servicepkg.MemoryIndexName(data.ownedTenantID), memoryIDs(data.memories)); err != nil {
			return fmt.Errorf("remove memory messages: %w", err)
		}
		if err := docEngine.DropMetadataStore(ctx, data.ownedTenantID); err != nil {
			return fmt.Errorf("remove tenant metadata: %w", err)
		}
	}
	for _, dataset := range data.datasets {
		if dataset.TenantID != data.ownedTenantID {
			indexName := servicepkg.IndexName(dataset.TenantID)
			exists, err := docEngine.ChunkStoreExists(ctx, indexName, dataset.ID)
			if err != nil {
				return fmt.Errorf("check dataset chunks %s: %w", dataset.ID, err)
			}
			if !exists {
				continue
			}
			if _, err := docEngine.DeleteChunks(ctx, map[string]interface{}{"kb_id": dataset.ID}, indexName, dataset.ID); err != nil {
				return fmt.Errorf("remove dataset chunks %s: %w", dataset.ID, err)
			}
		}
	}
	documentsByDataset := make(map[string][]string)
	for _, document := range data.documents {
		if _, deletingDataset := datasetIDs[document.KbID]; !deletingDataset {
			documentsByDataset[document.KbID] = append(documentsByDataset[document.KbID], document.ID)
		}
	}
	for kbID, docIDs := range documentsByDataset {
		tenantID := data.docTenants[kbID]
		if tenantID == "" {
			return fmt.Errorf("dataset %s for documents not found", kbID)
		}
		indexName := servicepkg.IndexName(tenantID)
		exists, err := docEngine.ChunkStoreExists(ctx, indexName, kbID)
		if err != nil {
			return fmt.Errorf("check document chunks for dataset %s: %w", kbID, err)
		}
		if exists {
			for start := 0; start < len(docIDs); start += 1000 {
				end := min(start+1000, len(docIDs))
				if _, err := docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": docIDs[start:end]}, indexName, kbID); err != nil {
					return fmt.Errorf("remove document chunks for dataset %s: %w", kbID, err)
				}
			}
		}
		metadataExists, err := docEngine.MetadataStoreExists(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("check document metadata for dataset %s: %w", kbID, err)
		}
		if metadataExists {
			for start := 0; start < len(docIDs); start += 1000 {
				end := min(start+1000, len(docIDs))
				if _, err := docEngine.DeleteMetadata(ctx, map[string]interface{}{"id": docIDs[start:end]}, tenantID); err != nil {
					return fmt.Errorf("remove document metadata for dataset %s: %w", kbID, err)
				}
			}
		}
	}
	for _, space := range data.spaces {
		if err := docEngine.DropChunkStore(ctx, servicepkg.SkillIndexName(space.TenantID, space.ID), "skill"); err != nil {
			return fmt.Errorf("remove skill index %s: %w", space.ID, err)
		}
	}
	return nil
}

func dropUserChunkStore(ctx context.Context, docEngine engine.DocEngine, indexName string, ids []string) error {
	if docEngine.GetType() == string(engine.EngineInfinity) {
		for _, id := range ids {
			if err := docEngine.DropChunkStore(ctx, indexName, id); err != nil {
				return err
			}
		}
		return nil
	}
	return docEngine.DropChunkStore(ctx, indexName, "")
}

func memoryIDs(memories []entity.Memory) []string {
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		ids = append(ids, memory.ID)
	}
	return ids
}

func (data *userDeletionData) deleteDatabaseRows(ctx context.Context, tx *gorm.DB, userID string, result *DeleteUserResult) error {
	selectIDs := func(model interface{}, condition string, args ...interface{}) ([]string, error) {
		var ids []string
		err := tx.WithContext(ctx).Model(model).Where(condition, args...).Pluck("id", &ids).Error
		return ids, err
	}
	remove := func(label string, model interface{}, condition string, args ...interface{}) (int64, error) {
		deleted := tx.WithContext(ctx).Unscoped().Where(condition, args...).Delete(model)
		if deleted.Error != nil {
			return 0, fmt.Errorf("delete %s: %w", label, deleted.Error)
		}
		if deleted.RowsAffected > 0 {
			result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d %s records.", deleted.RowsAffected, label))
		}
		return deleted.RowsAffected, nil
	}
	removeIDs := func(label string, model interface{}, column string, ids []string) error {
		var total int64
		for start := 0; start < len(ids); start += 1000 {
			end := min(start+1000, len(ids))
			deleted := tx.WithContext(ctx).Unscoped().Where(column+" IN ?", ids[start:end]).Delete(model)
			if deleted.Error != nil {
				return fmt.Errorf("delete %s: %w", label, deleted.Error)
			}
			total += deleted.RowsAffected
		}
		if total > 0 {
			result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d %s records.", total, label))
		}
		return nil
	}
	owner := data.ownedTenantID
	var chatIDs, canvasIDs, conversationIDs, apiConversationIDs []string
	var evaluationDatasetIDs, evaluationRunIDs, connectorIDs, providerIDs, instanceIDs, modelIDs, groupIDs []string
	var ingestionTaskIDs, memoryTaskIDs, commitIDs []string
	var err error
	if owner != "" {
		if chatIDs, err = selectIDs(&entity.Chat{}, "tenant_id = ?", owner); err != nil {
			return err
		}
		if evaluationDatasetIDs, err = selectIDs(&entity.EvaluationDataset{}, "tenant_id = ? OR created_by = ?", owner, userID); err != nil {
			return err
		}
		if connectorIDs, err = selectIDs(&entity.Connector{}, "tenant_id = ?", owner); err != nil {
			return err
		}
		if providerIDs, err = selectIDs(&entity.TenantModelProvider{}, "tenant_id = ?", owner); err != nil {
			return err
		}
	} else if evaluationDatasetIDs, err = selectIDs(&entity.EvaluationDataset{}, "created_by = ?", userID); err != nil {
		return err
	}
	if canvasIDs, err = selectIDs(&entity.UserCanvas{}, "user_id = ?", userID); err != nil {
		return err
	}
	if len(chatIDs) > 0 {
		conversationIDs, err = selectIDs(&entity.ChatSession{}, "dialog_id IN ? OR user_id = ?", chatIDs, userID)
		if err != nil {
			return err
		}
	} else {
		conversationIDs, err = selectIDs(&entity.ChatSession{}, "user_id = ?", userID)
		if err != nil {
			return err
		}
	}
	apiDialogIDs := append(append([]string{}, chatIDs...), canvasIDs...)
	if len(apiDialogIDs) > 0 {
		apiConversationIDs, err = selectIDs(&entity.API4Conversation{}, "dialog_id IN ? OR user_id = ?", apiDialogIDs, userID)
	} else {
		apiConversationIDs, err = selectIDs(&entity.API4Conversation{}, "user_id = ?", userID)
	}
	if err != nil {
		return err
	}
	if len(evaluationDatasetIDs) > 0 {
		evaluationRunIDs, err = selectIDs(&entity.EvaluationRun{}, "dataset_id IN ? OR created_by = ?", evaluationDatasetIDs, userID)
	} else {
		evaluationRunIDs, err = selectIDs(&entity.EvaluationRun{}, "created_by = ?", userID)
	}
	if err != nil {
		return err
	}
	if len(providerIDs) > 0 {
		if instanceIDs, err = selectIDs(&entity.TenantModelInstance{}, "provider_id IN ?", providerIDs); err != nil {
			return err
		}
		if modelIDs, err = selectIDs(&entity.TenantModel{}, "provider_id IN ?", providerIDs); err != nil {
			return err
		}
		if err = tx.Model(&entity.TenantModelGroupMapping{}).Where("provider_id IN ?", providerIDs).Distinct().Pluck("group_id", &groupIDs).Error; err != nil {
			return err
		}
	}
	if len(data.documentIDs) > 0 || len(data.datasetIDs) > 0 {
		query := tx.Model(&entity.IngestionTask{}).Where("user_id = ?", userID)
		if len(data.documentIDs) > 0 {
			query = query.Or("document_id IN ?", data.documentIDs)
		}
		if len(data.datasetIDs) > 0 {
			query = query.Or("dataset_id IN ?", data.datasetIDs)
		}
		err = query.Pluck("id", &ingestionTaskIDs).Error
	} else {
		ingestionTaskIDs, err = selectIDs(&entity.IngestionTask{}, "user_id = ?", userID)
	}
	if err != nil {
		return err
	}
	if len(data.memories) > 0 {
		if err = tx.Model(&entity.MemoryTask{}).Where("memory_id IN ?", memoryIDs(data.memories)).Pluck("task_id", &memoryTaskIDs).Error; err != nil {
			return err
		}
	}
	commitQuery := tx.Model(&entity.FileCommit{}).Where("author_id = ?", userID)
	if len(data.fileIDs) > 0 {
		commitQuery = commitQuery.Or("folder_id IN ?", data.fileIDs)
	}
	if err = commitQuery.Pluck("id", &commitIDs).Error; err != nil {
		return err
	}

	for _, child := range []struct {
		label string
		model interface{}
		ids   []string
	}{
		{"conversation messages", &entity.ConversationMessage{}, conversationIDs},
		{"conversation references", &entity.ConversationReference{}, conversationIDs},
		{"API conversation messages", &entity.API4ConversationMessage{}, apiConversationIDs},
		{"API conversation references", &entity.API4ConversationReference{}, apiConversationIDs},
	} {
		if err := removeIDs(child.label, child.model, "conversation_id", child.ids); err != nil {
			return err
		}
	}
	if err := removeIDs("conversations", &entity.ChatSession{}, "id", conversationIDs); err != nil {
		return err
	}
	if err := removeIDs("API conversations", &entity.API4Conversation{}, "id", apiConversationIDs); err != nil {
		return err
	}
	if err := removeIDs("chat channels", &entity.ChatChannel{}, "chat_id", chatIDs); err != nil {
		return err
	}
	if err := removeIDs("chats", &entity.Chat{}, "id", chatIDs); err != nil {
		return err
	}
	if err := removeIDs("agent versions", &entity.UserCanvasVersion{}, "user_canvas_id", canvasIDs); err != nil {
		return err
	}
	if err := removeIDs("agents", &entity.UserCanvas{}, "id", canvasIDs); err != nil {
		return err
	}
	if err := removeIDs("evaluation results", &entity.EvaluationResult{}, "run_id", evaluationRunIDs); err != nil {
		return err
	}
	if err := removeIDs("evaluation runs", &entity.EvaluationRun{}, "id", evaluationRunIDs); err != nil {
		return err
	}
	if len(evaluationDatasetIDs) > 0 {
		var caseIDs []string
		if caseIDs, err = selectIDs(&entity.EvaluationCase{}, "dataset_id IN ?", evaluationDatasetIDs); err != nil {
			return err
		}
		if err := removeIDs("evaluation results", &entity.EvaluationResult{}, "case_id", caseIDs); err != nil {
			return err
		}
		if err := removeIDs("evaluation cases", &entity.EvaluationCase{}, "id", caseIDs); err != nil {
			return err
		}
		if err := removeIDs("evaluation datasets", &entity.EvaluationDataset{}, "id", evaluationDatasetIDs); err != nil {
			return err
		}
	}
	if err := removeIDs("ingestion task logs", &entity.IngestionTaskLog{}, "task_id", ingestionTaskIDs); err != nil {
		return err
	}
	if err := removeIDs("ingestion tasks", &entity.IngestionTask{}, "id", ingestionTaskIDs); err != nil {
		return err
	}
	if err := removeIDs("memory tasks", &entity.MemoryTask{}, "memory_id", memoryIDs(data.memories)); err != nil {
		return err
	}
	if err := removeIDs("memory task records", &entity.Task{}, "id", memoryTaskIDs); err != nil {
		return err
	}
	if err := removeIDs("document tasks", &entity.Task{}, "doc_id", data.documentIDs); err != nil {
		return err
	}
	if err := removeIDs("document file links", &entity.File2Document{}, "document_id", data.documentIDs); err != nil {
		return err
	}
	if err := removeIDs("file document links", &entity.File2Document{}, "file_id", data.fileIDs); err != nil {
		return err
	}
	if err := removeIDs("documents", &entity.Document{}, "id", data.documentIDs); err != nil {
		return err
	}
	if err := removeIDs("file commit items", &entity.FileCommitItem{}, "commit_id", commitIDs); err != nil {
		return err
	}
	if err := removeIDs("file commit items", &entity.FileCommitItem{}, "file_id", data.fileIDs); err != nil {
		return err
	}
	if err := removeIDs("file commits", &entity.FileCommit{}, "id", commitIDs); err != nil {
		return err
	}
	if err := removeIDs("files", &entity.File{}, "id", data.fileIDs); err != nil {
		return err
	}
	if err := removeIDs("connector dataset links", &entity.Connector2Kb{}, "connector_id", connectorIDs); err != nil {
		return err
	}
	if err := removeIDs("connector dataset links", &entity.Connector2Kb{}, "kb_id", data.datasetIDs); err != nil {
		return err
	}
	if err := removeIDs("sync logs", &entity.SyncLogs{}, "connector_id", connectorIDs); err != nil {
		return err
	}
	if err := removeIDs("sync logs", &entity.SyncLogs{}, "kb_id", data.datasetIDs); err != nil {
		return err
	}
	if err := removeIDs("connectors", &entity.Connector{}, "id", connectorIDs); err != nil {
		return err
	}
	if err := removeIDs("knowledge compile records", &entity.KnowledgeCompileDataset{}, "dataset_id", data.datasetIDs); err != nil {
		return err
	}
	if err := removeIDs("wiki document records", &entity.WikiDocumentDirty{}, "document_id", data.documentIDs); err != nil {
		return err
	}
	if err := removeIDs("wiki document records", &entity.WikiDocumentDirty{}, "dataset_id", data.datasetIDs); err != nil {
		return err
	}
	if err := removeIDs("pipeline logs", &entity.PipelineOperationLog{}, "document_id", data.documentIDs); err != nil {
		return err
	}
	if err := removeIDs("pipeline logs", &entity.PipelineOperationLog{}, "kb_id", data.datasetIDs); err != nil {
		return err
	}
	if err := removeIDs("datasets", &entity.Knowledgebase{}, "id", data.datasetIDs); err != nil {
		return err
	}
	datasetIDs := make(map[string]struct{}, len(data.datasetIDs))
	for _, id := range data.datasetIDs {
		datasetIDs[id] = struct{}{}
	}
	counts := make(map[string]struct{ documents, chunks, tokens int64 })
	for _, document := range data.documents {
		if _, deletingDataset := datasetIDs[document.KbID]; deletingDataset {
			continue
		}
		count := counts[document.KbID]
		count.documents++
		count.chunks += document.ChunkNum
		count.tokens += document.TokenNum
		counts[document.KbID] = count
	}
	for kbID, count := range counts {
		if err := tx.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Updates(map[string]interface{}{
			"doc_num":   gorm.Expr("CASE WHEN doc_num >= ? THEN doc_num - ? ELSE 0 END", count.documents, count.documents),
			"chunk_num": gorm.Expr("CASE WHEN chunk_num >= ? THEN chunk_num - ? ELSE 0 END", count.chunks, count.chunks),
			"token_num": gorm.Expr("CASE WHEN token_num >= ? THEN token_num - ? ELSE 0 END", count.tokens, count.tokens),
		}).Error; err != nil {
			return fmt.Errorf("update dataset counters: %w", err)
		}
	}
	if len(groupIDs) > 0 {
		if err := removeIDs("model group mappings", &entity.TenantModelGroupMapping{}, "provider_id", providerIDs); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			var remaining int64
			if err := tx.Model(&entity.TenantModelGroupMapping{}).Where("group_id = ?", groupID).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				if _, err := remove("model groups", &entity.TenantModelGroup{}, "id = ?", groupID); err != nil {
					return err
				}
			}
		}
	}
	if err := removeIDs("models", &entity.TenantModel{}, "id", modelIDs); err != nil {
		return err
	}
	if err := removeIDs("model instances", &entity.TenantModelInstance{}, "id", instanceIDs); err != nil {
		return err
	}
	if err := removeIDs("model providers", &entity.TenantModelProvider{}, "id", providerIDs); err != nil {
		return err
	}
	if owner != "" {
		for _, item := range []struct {
			label string
			model interface{}
		}{
			{"chat channels", &entity.ChatChannel{}},
			{"searches", &entity.Search{}},
			{"memories", &entity.Memory{}},
			{"MCP servers", &entity.MCPServer{}},
			{"skill search configurations", &entity.SkillSearchConfig{}},
			{"skill spaces", &entity.SkillSpace{}},
			{"compilation templates", &entity.CompilationTemplate{}},
			{"compilation template groups", &entity.CompilationTemplateGroup{}},
			{"pipeline logs", &entity.PipelineOperationLog{}},
			{"knowledge compile records", &entity.KnowledgeCompileDataset{}},
			{"wiki document records", &entity.WikiDocumentDirty{}},
			{"API tokens", &entity.APIToken{}},
		} {
			if _, err := remove(item.label, item.model, "tenant_id = ?", owner); err != nil {
				return err
			}
		}
		tenantLLMCount, err := remove("tenant LLMs", &entity.TenantLLM{}, "tenant_id = ?", owner)
		if err != nil {
			return err
		}
		result.TenantLLMCount = int(tenantLLMCount)
		langfuseCount, err := remove("Langfuse credentials", &entity.TenantLangfuse{}, "tenant_id = ?", owner)
		if err != nil {
			return err
		}
		result.LangfuseCount = int(langfuseCount)
		result.MetadataTable = servicepkg.BuildMetadataIndexName(owner)
		ownerMemberships, err := remove("tenant memberships", &entity.UserTenant{}, "tenant_id = ?", owner)
		if err != nil {
			return err
		}
		result.UserTenantCount += int(ownerMemberships)
		count, err := remove("tenants", &entity.Tenant{}, "id = ?", owner)
		if err != nil {
			return err
		}
		result.TenantCount = int(count)
	}
	if _, err := remove("searches", &entity.Search{}, "created_by = ?", userID); err != nil {
		return err
	}
	invitationCondition := "user_id = ?"
	invitationArgs := []interface{}{userID}
	if owner != "" {
		invitationCondition += " OR tenant_id = ?"
		invitationArgs = append(invitationArgs, owner)
	}
	if _, err := remove("invitations", &entity.InvitationCode{}, invitationCondition, invitationArgs...); err != nil {
		return err
	}
	count, err := remove("user tenant memberships", &entity.UserTenant{}, "user_id = ?", userID)
	if err != nil {
		return err
	}
	result.UserTenantCount += int(count)
	count, err = remove("users", &entity.User{}, "id = ?", userID)
	if err != nil {
		return err
	}
	result.UserCount = int(count)
	return nil
}
