//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package permission

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type databaseSource struct {
	db       *gorm.DB
	lockRows bool
}

// NewDatabaseChecker creates a Checker backed by the application's SQL database.
func NewDatabaseChecker(db *gorm.DB) *Checker {
	return NewChecker(&databaseSource{db: db})
}

// NewLockingDatabaseChecker creates a database-backed Checker that locks loaded
// resource and membership rows. Use it only with a short-lived transaction.
func NewLockingDatabaseChecker(tx *gorm.DB) *Checker {
	return NewChecker(&databaseSource{db: tx, lockRows: true})
}

func (s *databaseSource) GetMembership(ctx context.Context, userID, tenantID string) (*Membership, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	query := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var relation entity.UserTenant
	if err := query.First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	role, valid, err := s.resolveMembershipRole(ctx, relation.TenantID, relation.Role)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, nil
	}
	return &Membership{
		ID:       relation.ID,
		UserID:   relation.UserID,
		TenantID: relation.TenantID,
		Role:     role,
		Active:   relation.Status != nil && *relation.Status == string(entity.StatusValid),
	}, nil
}

func (s *databaseSource) resolveMembershipRole(ctx context.Context, tenantID, roleValue string) (TenantRole, bool, error) {
	role := TenantRole(strings.TrimSpace(roleValue))
	switch role {
	case RoleOwner, RoleAdmin, RoleNormal, RoleInvite:
		return role, true, nil
	}
	valid, err := isTenantRoleID(ctx, s.db, tenantID, string(role))
	if err != nil {
		return "", false, err
	}
	if !valid {
		return "", false, nil
	}
	// Feature roles grant capabilities, while tenant membership still denotes
	// ordinary membership to the shared resource policy.
	return RoleNormal, true, nil
}

func (s *databaseSource) activeTenantIDsForUser(ctx context.Context, userID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	var memberships []entity.UserTenant
	if err := s.db.WithContext(ctx).
		Select("tenant_id", "role").
		Where("user_id = ? AND status = ?", userID, string(entity.StatusValid)).
		Find(&memberships).Error; err != nil {
		return nil, err
	}

	type roleResult struct {
		role  TenantRole
		valid bool
		err   error
	}
	resolved := make(map[string]roleResult)
	tenantIDs := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		key := membership.TenantID + "\x00" + membership.Role
		result, ok := resolved[key]
		if !ok {
			result.role, result.valid, result.err = s.resolveMembershipRole(ctx, membership.TenantID, membership.Role)
			resolved[key] = result
		}
		if result.err != nil {
			return nil, result.err
		}
		if result.valid && result.role != RoleInvite {
			tenantIDs = append(tenantIDs, membership.TenantID)
		}
	}
	return uniqueStringValues(tenantIDs), nil
}

func (s *databaseSource) GetResources(ctx context.Context, refs []ResourceRef) ([]Resource, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	refs = uniqueRefs(refs)
	datasetIDs := make([]string, 0, len(refs))
	canvasIDs := make([]string, 0, len(refs))
	agentSessionIDs := make([]string, 0, len(refs))
	memoryIDs := make([]string, 0, len(refs))
	fileIDs := make([]string, 0, len(refs))
	searchAppIDs := make([]string, 0, len(refs))
	chatIDs := make([]string, 0, len(refs))
	chatSessionIDs := make([]string, 0, len(refs))
	chatChannelIDs := make([]string, 0, len(refs))
	connectorIDs := make([]string, 0, len(refs))
	syncTaskIDs := make([]string, 0, len(refs))
	modelProviderIDs := make([]string, 0, len(refs))
	modelInstanceIDs := make([]string, 0, len(refs))
	tenantModelIDs := make([]string, 0, len(refs))
	mcpServerIDs := make([]string, 0, len(refs))
	skillSpaceIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		switch ref.Kind {
		case ResourceKindDataset:
			datasetIDs = append(datasetIDs, ref.ID)
		case ResourceKindCanvas:
			canvasIDs = append(canvasIDs, ref.ID)
		case ResourceKindAgentSession:
			agentSessionIDs = append(agentSessionIDs, ref.ID)
		case ResourceKindMemory:
			memoryIDs = append(memoryIDs, ref.ID)
		case ResourceKindFile, ResourceKindFolder:
			fileIDs = append(fileIDs, ref.ID)
		case ResourceKindSearchApp:
			searchAppIDs = append(searchAppIDs, ref.ID)
		case ResourceKindChat:
			chatIDs = append(chatIDs, ref.ID)
		case ResourceKindChatSession:
			chatSessionIDs = append(chatSessionIDs, ref.ID)
		case ResourceKindChatChannel:
			chatChannelIDs = append(chatChannelIDs, ref.ID)
		case ResourceKindConnector:
			connectorIDs = append(connectorIDs, ref.ID)
		case ResourceKindSyncTask:
			syncTaskIDs = append(syncTaskIDs, ref.ID)
		case ResourceKindModelProvider:
			modelProviderIDs = append(modelProviderIDs, ref.ID)
		case ResourceKindModelInstance:
			modelInstanceIDs = append(modelInstanceIDs, ref.ID)
		case ResourceKindModel:
			tenantModelIDs = append(tenantModelIDs, ref.ID)
		case ResourceKindMCPServer:
			mcpServerIDs = append(mcpServerIDs, ref.ID)
		case ResourceKindSkillSpace:
			skillSpaceIDs = append(skillSpaceIDs, ref.ID)
		default:
			return nil, fmt.Errorf("%w: unsupported resource kind %q", ErrInvalidPermission, ref.Kind)
		}
	}
	agentSessionsByID, err := s.loadAgentSessions(ctx, agentSessionIDs)
	if err != nil {
		return nil, err
	}
	for _, session := range agentSessionsByID {
		canvasIDs = append(canvasIDs, session.DialogID)
	}
	chatSessionsByID, err := s.loadChatSessions(ctx, chatSessionIDs)
	if err != nil {
		return nil, err
	}
	for _, session := range chatSessionsByID {
		chatIDs = append(chatIDs, session.DialogID)
	}
	var datasetsByID map[string]*entity.Knowledgebase
	var ownerIDsByTenant map[string]string
	if len(datasetIDs) > 0 {
		var err error
		datasetsByID, ownerIDsByTenant, err = s.loadDatasets(ctx, datasetIDs)
		if err != nil {
			return nil, err
		}
	}
	canvasesByID, err := s.loadCanvases(ctx, canvasIDs)
	if err != nil {
		return nil, err
	}
	memoriesByID, err := s.loadMemories(ctx, memoryIDs)
	if err != nil {
		return nil, err
	}
	filesByID, err := s.loadFiles(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	fileDatasetIDs, err := s.loadFileDatasetIDs(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	linkedDatasetIDs := make([]string, 0)
	for _, ids := range fileDatasetIDs {
		linkedDatasetIDs = append(linkedDatasetIDs, ids...)
	}
	linkedDatasetIDs = uniqueStringValues(linkedDatasetIDs)
	linkedDatasetsByID, linkedOwnerIDsByTenant, err := s.loadDatasets(ctx, linkedDatasetIDs)
	if err != nil {
		return nil, err
	}
	searchAppsByID, err := s.loadSearchApps(ctx, searchAppIDs)
	if err != nil {
		return nil, err
	}
	chatsByID, err := s.loadChats(ctx, chatIDs)
	if err != nil {
		return nil, err
	}
	chatChannelsByID, err := s.loadChatChannels(ctx, chatChannelIDs)
	if err != nil {
		return nil, err
	}
	syncTasksByID, err := s.loadSyncTasks(ctx, syncTaskIDs)
	if err != nil {
		return nil, err
	}
	for _, task := range syncTasksByID {
		connectorIDs = append(connectorIDs, task.ConnectorID)
	}
	connectorIDs = uniqueStringValues(connectorIDs)
	connectorsByID, err := s.loadConnectors(ctx, connectorIDs)
	if err != nil {
		return nil, err
	}
	syncTaskMappings, err := s.loadSyncTaskMappings(ctx, syncTasksByID)
	if err != nil {
		return nil, err
	}
	tenantModelsByID, err := s.loadTenantModels(ctx, tenantModelIDs)
	if err != nil {
		return nil, err
	}
	for _, model := range tenantModelsByID {
		modelProviderIDs = append(modelProviderIDs, model.ProviderID)
		modelInstanceIDs = append(modelInstanceIDs, model.InstanceID)
	}
	modelInstancesByID, err := s.loadModelInstances(ctx, modelInstanceIDs)
	if err != nil {
		return nil, err
	}
	for _, instance := range modelInstancesByID {
		modelProviderIDs = append(modelProviderIDs, instance.ProviderID)
	}
	modelProviderIDs = uniqueStringValues(modelProviderIDs)
	modelProvidersByID, err := s.loadModelProviders(ctx, modelProviderIDs)
	if err != nil {
		return nil, err
	}
	mcpServerTenantsByID, err := s.loadMCPServerTenants(ctx, mcpServerIDs)
	if err != nil {
		return nil, err
	}
	skillSpacesByID, err := s.loadSkillSpaces(ctx, skillSpaceIDs)
	if err != nil {
		return nil, err
	}
	skillSpaceFolderIDs := make([]string, 0, len(skillSpacesByID))
	integrationTenantIDs := make([]string, 0, len(mcpServerTenantsByID)+len(skillSpacesByID))
	for _, tenantID := range mcpServerTenantsByID {
		integrationTenantIDs = append(integrationTenantIDs, tenantID)
	}
	for _, space := range skillSpacesByID {
		skillSpaceFolderIDs = append(skillSpaceFolderIDs, space.FolderID)
		integrationTenantIDs = append(integrationTenantIDs, space.TenantID)
	}
	skillSpaceFoldersByID, err := s.loadSkillSpaceFolders(ctx, skillSpaceFolderIDs)
	if err != nil {
		return nil, err
	}
	integrationOwnerIDsByTenant, err := s.loadTenantOwnerIDs(ctx, integrationTenantIDs)
	if err != nil {
		return nil, err
	}
	resources := make([]Resource, 0, len(refs))
	for _, ref := range refs {
		switch ref.Kind {
		case ResourceKindDataset:
			if dataset, ok := datasetsByID[ref.ID]; ok {
				resources = append(resources, datasetResource(dataset, ownerIDsByTenant))
			}
		case ResourceKindCanvas:
			if canvas, ok := canvasesByID[ref.ID]; ok {
				resources = append(resources, canvasResource(canvas))
			}
		case ResourceKindAgentSession:
			session, ok := agentSessionsByID[ref.ID]
			if !ok {
				continue
			}
			canvas, ok := canvasesByID[session.DialogID]
			if !ok {
				continue
			}
			resources = append(resources, agentSessionResource(session, canvas))
		case ResourceKindMemory:
			if memory, ok := memoriesByID[ref.ID]; ok {
				resources = append(resources, memoryResource(memory))
			}
		case ResourceKindFile, ResourceKindFolder:
			file, ok := filesByID[ref.ID]
			if !ok || (ref.Kind == ResourceKindFolder) != (file.Type == "folder") {
				continue
			}
			resources = append(resources, fileResource(ref.Kind, file, fileDatasetIDs[ref.ID], linkedDatasetsByID, linkedOwnerIDsByTenant))
		case ResourceKindSearchApp:
			if searchApp, ok := searchAppsByID[ref.ID]; ok {
				resources = append(resources, searchAppResource(searchApp))
			}
		case ResourceKindChat:
			if chat, ok := chatsByID[ref.ID]; ok {
				resources = append(resources, chatResource(chat))
			}
		case ResourceKindChatChannel:
			if channel, ok := chatChannelsByID[ref.ID]; ok {
				resources = append(resources, chatChannelResource(channel))
			}
		case ResourceKindChatSession:
			session, ok := chatSessionsByID[ref.ID]
			if !ok {
				continue
			}
			chat, ok := chatsByID[session.DialogID]
			if !ok {
				continue
			}
			resources = append(resources, chatSessionResource(session, chat))
		case ResourceKindConnector:
			if connector, ok := connectorsByID[ref.ID]; ok {
				resources = append(resources, connectorResource(connector))
			}
		case ResourceKindSyncTask:
			task, ok := syncTasksByID[ref.ID]
			if !ok {
				continue
			}
			connector, ok := connectorsByID[task.ConnectorID]
			if !ok {
				continue
			}
			_, linked := syncTaskMappings[connectorDatasetRef{connectorID: task.ConnectorID, datasetID: task.KbID}]
			resources = append(resources, syncTaskResource(task, connector, linked))
		case ResourceKindModelProvider:
			if provider, ok := modelProvidersByID[ref.ID]; ok {
				if strings.TrimSpace(provider.UserID) == "" {
					return nil, fmt.Errorf("%w: model provider %q has no owner user_id", ErrInvalidPermission, provider.ID)
				}
				resources = append(resources, modelProviderResource(provider))
			}
		case ResourceKindModelInstance:
			instance, ok := modelInstancesByID[ref.ID]
			if !ok {
				continue
			}
			provider, ok := modelProvidersByID[instance.ProviderID]
			if !ok {
				continue
			}
			if strings.TrimSpace(provider.UserID) == "" {
				return nil, fmt.Errorf("%w: model provider %q has no owner user_id", ErrInvalidPermission, provider.ID)
			}
			resources = append(resources, modelInstanceResource(instance, provider))
		case ResourceKindModel:
			model, ok := tenantModelsByID[ref.ID]
			if !ok {
				continue
			}
			instance, ok := modelInstancesByID[model.InstanceID]
			if !ok || instance.ProviderID != model.ProviderID {
				continue
			}
			provider, ok := modelProvidersByID[model.ProviderID]
			if !ok {
				continue
			}
			if strings.TrimSpace(provider.UserID) == "" {
				return nil, fmt.Errorf("%w: model provider %q has no owner user_id", ErrInvalidPermission, provider.ID)
			}
			resources = append(resources, tenantModelResource(model, provider))
		case ResourceKindMCPServer:
			tenantID, ok := mcpServerTenantsByID[ref.ID]
			if !ok {
				continue
			}
			ownerUserID := integrationOwnerIDsByTenant[tenantID]
			if ownerUserID == "" {
				return nil, fmt.Errorf("%w: tenant %q has no active owner", ErrInvalidPermission, tenantID)
			}
			resources = append(resources, mcpServerResource(ref.ID, tenantID, ownerUserID))
		case ResourceKindSkillSpace:
			space, ok := skillSpacesByID[ref.ID]
			if !ok {
				continue
			}
			folder, ok := skillSpaceFoldersByID[space.FolderID]
			if !ok || folder.TenantID != space.TenantID || folder.Type != "folder" {
				continue
			}
			ownerUserID := integrationOwnerIDsByTenant[space.TenantID]
			if ownerUserID == "" {
				return nil, fmt.Errorf("%w: tenant %q has no active owner", ErrInvalidPermission, space.TenantID)
			}
			resources = append(resources, skillSpaceResource(space.ID, space.TenantID, ownerUserID))
		}
	}
	return resources, nil
}

func (s *databaseSource) ListResourceRefs(ctx context.Context, subject Subject, kind ResourceKind, parent ResourceRef) ([]ResourceRef, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	var ids []string
	switch kind {
	case ResourceKindDataset:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: datasets cannot have a parent scope", ErrInvalidPermission)
		}
		var tenantIDs []string
		if err := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Distinct("tenant_id").
			Where("user_id = ? AND status = ?", subject.UserID, string(entity.StatusValid)).
			Pluck("tenant_id", &tenantIDs).Error; err != nil {
			return nil, err
		}
		if len(tenantIDs) == 0 {
			return []ResourceRef{}, nil
		}
		err := s.db.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("tenant_id IN ? AND status = ?", tenantIDs, string(entity.StatusValid)).
			Pluck("id", &ids).Error
		if err != nil {
			return nil, err
		}
	case ResourceKindCanvas:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: canvases cannot have a parent scope", ErrInvalidPermission)
		}
		tenantIDs, err := s.activeTenantIDsForUser(ctx, subject.UserID)
		if err != nil {
			return nil, err
		}
		ownerIDs := append(append(make([]string, 0, len(tenantIDs)+1), tenantIDs...), subject.UserID)
		if err := s.db.WithContext(ctx).Model(&entity.UserCanvas{}).
			Where("user_id IN ?", ownerIDs).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindAgentSession:
		if parent.Kind != ResourceKindCanvas || strings.TrimSpace(parent.ID) == "" {
			return nil, fmt.Errorf("%w: agent sessions require a canvas parent scope", ErrInvalidPermission)
		}
		if err := s.db.WithContext(ctx).Model(&entity.API4Conversation{}).
			Where("dialog_id = ?", parent.ID).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindMemory:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: memories cannot have a parent scope", ErrInvalidPermission)
		}
		var tenantIDs []string
		if err := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Distinct("tenant_id").
			Where("user_id = ? AND status = ?", subject.UserID, string(entity.StatusValid)).
			Pluck("tenant_id", &tenantIDs).Error; err != nil {
			return nil, err
		}
		ownerIDs := append(tenantIDs, subject.UserID)
		if err := s.db.WithContext(ctx).Model(&entity.Memory{}).
			Where("tenant_id IN ?", ownerIDs).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindFile, ResourceKindFolder:
		if parent.Kind != ResourceKindFolder || strings.TrimSpace(parent.ID) == "" {
			return nil, fmt.Errorf("%w: files and folders require a folder parent scope", ErrInvalidPermission)
		}
		parentQuery := s.db.WithContext(ctx).Model(&entity.File{}).
			Where("id = ? AND tenant_id = ? AND type = ?", parent.ID, subject.UserID, "folder")
		var parentFile entity.File
		if err := parentQuery.First(&parentFile).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return []ResourceRef{}, nil
			}
			return nil, err
		}
		query := s.db.WithContext(ctx).Model(&entity.File{}).
			Select("id").
			Where("tenant_id = ? AND parent_id = ? AND id <> ?", parentFile.TenantID, parentFile.ID, parentFile.ID)
		if kind == ResourceKindFolder {
			query = query.Where("type = ?", "folder")
		} else {
			query = query.Where("type <> ?", "folder")
		}
		if err := query.Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindSearchApp:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: search apps cannot have a parent scope", ErrInvalidPermission)
		}
		if err := s.db.WithContext(ctx).Model(&entity.Search{}).
			Where("status = ? AND created_by = ?", string(entity.StatusValid), subject.UserID).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindChat:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: chats cannot have a parent scope", ErrInvalidPermission)
		}
		if err := s.db.WithContext(ctx).Model(&entity.Chat{}).
			Where("tenant_id = ? AND status = ?", subject.UserID, string(entity.StatusValid)).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindChatSession:
		if parent.Kind != ResourceKindChat || strings.TrimSpace(parent.ID) == "" {
			return nil, fmt.Errorf("%w: chat sessions require a chat parent scope", ErrInvalidPermission)
		}
		var chatCount int64
		if err := s.db.WithContext(ctx).Model(&entity.Chat{}).
			Where("id = ? AND status = ?", parent.ID, string(entity.StatusValid)).
			Count(&chatCount).Error; err != nil {
			return nil, err
		}
		if chatCount == 0 {
			return []ResourceRef{}, nil
		}
		if err := s.db.WithContext(ctx).Model(&entity.ChatSession{}).
			Where("dialog_id = ?", parent.ID).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindChatChannel:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: chat channels cannot have a parent scope", ErrInvalidPermission)
		}
		tenantIDs, err := s.tenantIDsForSubject(ctx, subject)
		if err != nil {
			return nil, err
		}
		if len(tenantIDs) == 0 {
			return []ResourceRef{}, nil
		}
		if err := s.db.WithContext(ctx).Model(&entity.ChatChannel{}).
			Where("tenant_id IN ?", tenantIDs).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindConnector:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: connectors cannot have a parent scope", ErrInvalidPermission)
		}
		var tenantIDs []string
		if err := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Distinct("tenant_id").
			Where("user_id = ? AND status = ?", subject.UserID, string(entity.StatusValid)).
			Pluck("tenant_id", &tenantIDs).Error; err != nil {
			return nil, err
		}
		tenantIDs = uniqueStringValues(append(tenantIDs, subject.UserID))
		if err := s.db.WithContext(ctx).Model(&entity.Connector{}).
			Where("tenant_id IN ?", tenantIDs).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindSyncTask:
		if parent.Kind != ResourceKindConnector || strings.TrimSpace(parent.ID) == "" {
			return nil, fmt.Errorf("%w: sync tasks require a connector parent scope", ErrInvalidPermission)
		}
		if err := s.db.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("sync_logs.connector_id = ? AND EXISTS (SELECT 1 FROM connector2kb JOIN knowledgebase ON knowledgebase.id = connector2kb.kb_id WHERE connector2kb.connector_id = sync_logs.connector_id AND connector2kb.kb_id = sync_logs.kb_id)", parent.ID).
			Pluck("sync_logs.id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindModelProvider:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: model providers cannot have a parent scope", ErrInvalidPermission)
		}
		tenantIDs, err := s.tenantIDsForSubject(ctx, subject)
		if err != nil {
			return nil, err
		}
		if len(tenantIDs) == 0 {
			return []ResourceRef{}, nil
		}
		if err := s.db.WithContext(ctx).Model(&entity.TenantModelProvider{}).
			Where("tenant_id IN ?", tenantIDs).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindModelInstance:
		if (parent.ID != "" && parent.Kind != ResourceKindModelProvider) || (parent.ID == "" && parent.Kind != "") {
			return nil, fmt.Errorf("%w: model instances require a model provider parent", ErrInvalidPermission)
		}
		query := s.db.WithContext(ctx).Model(&entity.TenantModelInstance{}).Select("tenant_model_instance.id")
		if parent.ID != "" {
			query = query.Where("tenant_model_instance.provider_id = ?", parent.ID)
		} else {
			tenantIDs, err := s.tenantIDsForSubject(ctx, subject)
			if err != nil {
				return nil, err
			}
			if len(tenantIDs) == 0 {
				return []ResourceRef{}, nil
			}
			query = query.Joins("JOIN tenant_model_provider ON tenant_model_provider.id = tenant_model_instance.provider_id").
				Where("tenant_model_provider.tenant_id IN ?", tenantIDs)
		}
		if err := query.Pluck("tenant_model_instance.id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindModel:
		switch parent.Kind {
		case "":
			if parent.ID != "" {
				return nil, fmt.Errorf("%w: model parent kind is required", ErrInvalidPermission)
			}
		case ResourceKindModelProvider, ResourceKindModelInstance:
			if strings.TrimSpace(parent.ID) == "" {
				return nil, fmt.Errorf("%w: model parent ID is required", ErrInvalidPermission)
			}
		default:
			return nil, fmt.Errorf("%w: models require a model provider or model instance parent", ErrInvalidPermission)
		}
		query := s.db.WithContext(ctx).Model(&entity.TenantModel{}).Select("tenant_model.id")
		switch parent.Kind {
		case ResourceKindModelProvider:
			query = query.Where("tenant_model.provider_id = ?", parent.ID)
		case ResourceKindModelInstance:
			query = query.Where("tenant_model.instance_id = ?", parent.ID)
		default:
			tenantIDs, err := s.tenantIDsForSubject(ctx, subject)
			if err != nil {
				return nil, err
			}
			if len(tenantIDs) == 0 {
				return []ResourceRef{}, nil
			}
			query = query.Joins("JOIN tenant_model_provider ON tenant_model_provider.id = tenant_model.provider_id").
				Where("tenant_model_provider.tenant_id IN ?", tenantIDs)
		}
		if err := query.Pluck("tenant_model.id", &ids).Error; err != nil {
			return nil, err
		}
	case ResourceKindMCPServer, ResourceKindSkillSpace:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: %s resources cannot have a parent scope", ErrInvalidPermission, kind)
		}
		tenantIDs, err := s.tenantIDsForSubject(ctx, subject)
		if err != nil {
			return nil, err
		}
		if len(tenantIDs) == 0 {
			return []ResourceRef{}, nil
		}
		var query *gorm.DB
		if kind == ResourceKindMCPServer {
			query = s.db.WithContext(ctx).Model(&entity.MCPServer{}).
				Where("tenant_id IN ?", tenantIDs)
		} else {
			query = s.db.WithContext(ctx).Model(&entity.SkillSpace{}).
				Where("tenant_id IN ? AND status = ?", tenantIDs, entity.SpaceStatusActive)
		}
		if err := query.Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unsupported scope resource kind %q", ErrInvalidPermission, kind)
	}

	refs := make([]ResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, ResourceRef{Kind: kind, ID: id})
	}
	return refs, nil
}

func (s *databaseSource) loadConnectors(ctx context.Context, ids []string) (map[string]*entity.Connector, error) {
	connectorsByID := make(map[string]*entity.Connector)
	if len(ids) == 0 {
		return connectorsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Connector{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var connectors []*entity.Connector
	if err := query.Find(&connectors).Error; err != nil {
		return nil, err
	}
	for _, connector := range connectors {
		connectorsByID[connector.ID] = connector
	}
	return connectorsByID, nil
}

func (s *databaseSource) loadSyncTasks(ctx context.Context, ids []string) (map[string]*entity.SyncLogs, error) {
	tasksByID := make(map[string]*entity.SyncLogs)
	if len(ids) == 0 {
		return tasksByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.SyncLogs{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var tasks []*entity.SyncLogs
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	return tasksByID, nil
}

type connectorDatasetRef struct {
	connectorID string
	datasetID   string
}

func (s *databaseSource) loadSyncTaskMappings(ctx context.Context, tasks map[string]*entity.SyncLogs) (map[connectorDatasetRef]struct{}, error) {
	mappings := make(map[connectorDatasetRef]struct{})
	connectorIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		connectorIDs = append(connectorIDs, task.ConnectorID)
	}
	connectorIDs = uniqueStringValues(connectorIDs)
	if len(connectorIDs) == 0 {
		return mappings, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Connector2Kb{}).
		Joins("JOIN knowledgebase ON knowledgebase.id = connector2kb.kb_id").
		Where("connector2kb.connector_id IN ?", connectorIDs)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var links []entity.Connector2Kb
	if err := query.Find(&links).Error; err != nil {
		return nil, err
	}
	for _, link := range links {
		mappings[connectorDatasetRef{connectorID: link.ConnectorID, datasetID: link.KbID}] = struct{}{}
	}
	return mappings, nil
}

func (s *databaseSource) loadFiles(ctx context.Context, ids []string) (map[string]*entity.File, error) {
	filesByID := make(map[string]*entity.File)
	if len(ids) == 0 {
		return filesByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.File{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var files []*entity.File
	if err := query.Find(&files).Error; err != nil {
		return nil, err
	}
	for _, file := range files {
		filesByID[file.ID] = file
	}
	return filesByID, nil
}

func (s *databaseSource) loadChats(ctx context.Context, ids []string) (map[string]*entity.Chat, error) {
	chatsByID := make(map[string]*entity.Chat)
	if len(ids) == 0 {
		return chatsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Chat{}).
		Where("id IN ? AND status = ?", ids, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var chats []*entity.Chat
	if err := query.Find(&chats).Error; err != nil {
		return nil, err
	}
	for _, chat := range chats {
		chatsByID[chat.ID] = chat
	}
	return chatsByID, nil
}

func (s *databaseSource) loadSearchApps(ctx context.Context, ids []string) (map[string]*entity.Search, error) {
	searchAppsByID := make(map[string]*entity.Search)
	if len(ids) == 0 {
		return searchAppsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Search{}).
		Where("id IN ? AND status = ?", ids, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var searchApps []*entity.Search
	if err := query.Find(&searchApps).Error; err != nil {
		return nil, err
	}
	for _, searchApp := range searchApps {
		searchAppsByID[searchApp.ID] = searchApp
	}
	return searchAppsByID, nil
}

func (s *databaseSource) loadChatSessions(ctx context.Context, ids []string) (map[string]*entity.ChatSession, error) {
	sessionsByID := make(map[string]*entity.ChatSession)
	if len(ids) == 0 {
		return sessionsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.ChatSession{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sessions []*entity.ChatSession
	if err := query.Find(&sessions).Error; err != nil {
		return nil, err
	}
	for _, session := range sessions {
		sessionsByID[session.ID] = session
	}
	return sessionsByID, nil
}

func (s *databaseSource) loadFileDatasetIDs(ctx context.Context, fileIDs []string) (map[string][]string, error) {
	datasetIDsByFile := make(map[string][]string)
	if len(fileIDs) == 0 {
		return datasetIDsByFile, nil
	}
	var links []struct {
		FileID    string `gorm:"column:file_id"`
		DatasetID string `gorm:"column:dataset_id"`
	}
	query := s.db.WithContext(ctx).Table("file2document AS f2d").
		Select("f2d.file_id AS file_id, document.kb_id AS dataset_id").
		Joins("JOIN document ON document.id = f2d.document_id").
		Where("f2d.file_id IN ?", fileIDs)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&links).Error; err != nil {
		return nil, err
	}
	datasetIDs := make(map[string]struct{})
	for _, link := range links {
		if link.FileID == "" || link.DatasetID == "" {
			continue
		}
		datasetIDsByFile[link.FileID] = append(datasetIDsByFile[link.FileID], link.DatasetID)
		datasetIDs[link.DatasetID] = struct{}{}
	}
	for fileID, ids := range datasetIDsByFile {
		datasetIDsByFile[fileID] = uniqueStringValues(ids)
	}
	return datasetIDsByFile, nil
}

func (s *databaseSource) loadCanvases(ctx context.Context, ids []string) (map[string]*entity.UserCanvas, error) {
	canvasesByID := make(map[string]*entity.UserCanvas)
	if len(ids) == 0 {
		return canvasesByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.UserCanvas{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var canvases []*entity.UserCanvas
	if err := query.Find(&canvases).Error; err != nil {
		return nil, err
	}
	for _, canvas := range canvases {
		canvasesByID[canvas.ID] = canvas
	}
	return canvasesByID, nil
}

func (s *databaseSource) loadAgentSessions(ctx context.Context, ids []string) (map[string]*entity.API4Conversation, error) {
	sessionsByID := make(map[string]*entity.API4Conversation)
	if len(ids) == 0 {
		return sessionsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.API4Conversation{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sessions []*entity.API4Conversation
	if err := query.Find(&sessions).Error; err != nil {
		return nil, err
	}
	for _, session := range sessions {
		sessionsByID[session.ID] = session
	}
	return sessionsByID, nil
}

func (s *databaseSource) loadMemories(ctx context.Context, ids []string) (map[string]*entity.Memory, error) {
	memoriesByID := make(map[string]*entity.Memory)
	if len(ids) == 0 {
		return memoriesByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Memory{}).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var memories []*entity.Memory
	if err := query.Find(&memories).Error; err != nil {
		return nil, err
	}
	for _, memory := range memories {
		memoriesByID[memory.ID] = memory
	}
	return memoriesByID, nil
}

func (s *databaseSource) loadDatasets(ctx context.Context, ids []string) (map[string]*entity.Knowledgebase, map[string]string, error) {
	datasetsByID := make(map[string]*entity.Knowledgebase)
	if len(ids) == 0 {
		return datasetsByID, nil, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("id IN ? AND status = ?", ids, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var datasets []*entity.Knowledgebase
	if err := query.Find(&datasets).Error; err != nil {
		return nil, nil, err
	}
	tenantIDs := make([]string, 0, len(datasets))
	seenTenantIDs := make(map[string]struct{}, len(datasets))
	for _, dataset := range datasets {
		if _, seen := seenTenantIDs[dataset.TenantID]; seen {
			continue
		}
		seenTenantIDs[dataset.TenantID] = struct{}{}
		tenantIDs = append(tenantIDs, dataset.TenantID)
	}

	ownerIDsByTenant, err := s.loadTenantOwnerIDs(ctx, tenantIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, dataset := range datasets {
		ownerUserID := ownerIDsByTenant[dataset.TenantID]
		if ownerUserID == "" {
			return nil, nil, fmt.Errorf("%w: tenant %q has no active owner", ErrInvalidPermission, dataset.TenantID)
		}
		datasetsByID[dataset.ID] = dataset
	}
	return datasetsByID, ownerIDsByTenant, nil
}

func datasetResource(dataset *entity.Knowledgebase, ownerIDsByTenant map[string]string) Resource {
	ownerUserID := ownerIDsByTenant[dataset.TenantID]
	visibility := VisibilityPrivate
	switch dataset.Permission {
	case string(entity.TenantPermissionMe):
		visibility = VisibilityPrivate
	case string(entity.TenantPermissionTeam):
		visibility = VisibilityTenant
	}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindDataset, ID: dataset.ID},
		TenantID:          dataset.TenantID,
		CreatedBy:         dataset.CreatedBy,
		OwnerUserID:       ownerUserID,
		Visibility:        visibility,
		TenantRequirement: TenantNormalMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationDeleteContent, OperationShare, OperationRun, OperationUse},
		TenantOperations:  []Operation{OperationRead, OperationUpdate, OperationDeleteContent, OperationRun, OperationUse},
		Active:            dataset.Status != nil && *dataset.Status == string(entity.StatusValid),
	}
}

func canvasResource(canvas *entity.UserCanvas) Resource {
	visibility := VisibilityPrivate
	if canvas.Permission == string(entity.TenantPermissionTeam) {
		visibility = VisibilityTenant
	}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindCanvas, ID: canvas.ID},
		TenantID:          canvas.UserID,
		CreatedBy:         canvas.UserID,
		OwnerUserID:       canvas.UserID,
		Visibility:        visibility,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationUpdate, OperationDelete, OperationShare, OperationRun, OperationUse},
		TenantOperations:  []Operation{OperationRead, OperationUpdate, OperationRun, OperationUse},
		Active:            true,
	}
}

func agentSessionResource(session *entity.API4Conversation, canvas *entity.UserCanvas) Resource {
	return Resource{
		Ref:             ResourceRef{Kind: ResourceKindAgentSession, ID: session.ID},
		TenantID:        canvas.UserID,
		CreatedBy:       session.UserID,
		OwnerUserID:     session.UserID,
		Visibility:      VisibilityPrivate,
		OwnerOperations: []Operation{OperationRead, OperationRun, OperationUpdate, OperationDelete},
		Active:          true,
	}
}

func memoryResource(memory *entity.Memory) Resource {
	visibility := VisibilityPrivate
	if memory.Permissions == string(entity.TenantPermissionTeam) {
		visibility = VisibilityTenant
	}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindMemory, ID: memory.ID},
		TenantID:          memory.TenantID,
		OwnerUserID:       memory.TenantID,
		Visibility:        visibility,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationShare, OperationRun, OperationUse},
		TenantOperations:  []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse},
		Active:            true,
	}
}

func chatResource(chat *entity.Chat) Resource {
	return Resource{
		Ref:             ResourceRef{Kind: ResourceKindChat, ID: chat.ID},
		TenantID:        chat.TenantID,
		OwnerUserID:     chat.TenantID,
		Visibility:      VisibilityPrivate,
		OwnerOperations: []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationRun, OperationUse},
		Active:          chat.Status != nil && *chat.Status == string(entity.StatusValid),
	}
}

func searchAppResource(searchApp *entity.Search) Resource {
	return Resource{
		Ref:             ResourceRef{Kind: ResourceKindSearchApp, ID: searchApp.ID},
		TenantID:        searchApp.TenantID,
		CreatedBy:       searchApp.CreatedBy,
		OwnerUserID:     searchApp.CreatedBy,
		Visibility:      VisibilityPrivate,
		OwnerOperations: []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse},
		Active:          searchApp.Status != nil && *searchApp.Status == string(entity.StatusValid),
	}
}

func chatSessionResource(session *entity.ChatSession, chat *entity.Chat) Resource {
	createdBy := ""
	if session.UserID != nil {
		createdBy = *session.UserID
	}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindChatSession, ID: session.ID},
		TenantID:          chat.TenantID,
		CreatedBy:         createdBy,
		OwnerUserID:       chat.TenantID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse},
		TenantOperations:  []Operation{OperationRead},
		CreatorOperations: []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse},
		Active:            true,
	}
}

func chatChannelResource(channel *entity.ChatChannel) Resource {
	operations := []Operation{OperationRead, OperationUpdate, OperationDelete, OperationUse}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindChatChannel, ID: channel.ID},
		TenantID:          channel.TenantID,
		OwnerUserID:       channel.TenantID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   operations,
		TenantOperations:  operations,
		Active:            true,
	}
}

func connectorResource(connector *entity.Connector) Resource {
	ownerOperations := []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindConnector, ID: connector.ID},
		TenantID:          connector.TenantID,
		OwnerUserID:       connector.TenantID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   ownerOperations,
		TenantOperations:  []Operation{OperationRun, OperationUse},
		Active:            true,
	}
}

func syncTaskResource(task *entity.SyncLogs, connector *entity.Connector, linked bool) Resource {
	operations := []Operation{OperationRead}
	if strings.EqualFold(task.TaskType, "sync") && task.Status == string(entity.TaskStatusFail) {
		operations = append(operations, OperationRun)
	}
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindSyncTask, ID: task.ID},
		TenantID:          connector.TenantID,
		OwnerUserID:       connector.TenantID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   operations,
		TenantOperations:  operations,
		Active:            linked,
	}
}

func fileResource(kind ResourceKind, file *entity.File, datasetIDs []string, datasetsByID map[string]*entity.Knowledgebase, ownerIDsByTenant map[string]string) Resource {
	sharedWithUserIDs := make([]string, 0, len(datasetIDs))
	sharedWithTenantIDs := make([]string, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		dataset, ok := datasetsByID[datasetID]
		if !ok {
			continue
		}
		datasetAccess := datasetResource(dataset, ownerIDsByTenant)
		sharedWithUserIDs = append(sharedWithUserIDs, datasetAccess.OwnerUserID)
		if datasetAccess.Visibility == VisibilityTenant {
			sharedWithTenantIDs = append(sharedWithTenantIDs, datasetAccess.TenantID)
		}
	}
	sharedWithUserIDs = uniqueStringValues(sharedWithUserIDs)
	sharedWithTenantIDs = uniqueStringValues(sharedWithTenantIDs)
	visibility := VisibilityPrivate
	if len(sharedWithUserIDs) > 0 || len(sharedWithTenantIDs) > 0 {
		visibility = VisibilityShared
	}
	return Resource{
		Ref:                 ResourceRef{Kind: kind, ID: file.ID},
		TenantID:            file.TenantID,
		CreatedBy:           file.CreatedBy,
		OwnerUserID:         file.TenantID,
		Visibility:          visibility,
		OwnerOperations:     []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationShare, OperationRun, OperationUse},
		SharedWithUserIDs:   sharedWithUserIDs,
		SharedWithTenantIDs: sharedWithTenantIDs,
		Active:              true,
	}
}

func mcpServerResource(id, tenantID, ownerUserID string) Resource {
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindMCPServer, ID: id},
		TenantID:          tenantID,
		OwnerUserID:       ownerUserID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationUse},
		TenantOperations:  []Operation{OperationUse},
		Active:            true,
	}
}

func skillSpaceResource(id, tenantID, ownerUserID string) Resource {
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindSkillSpace, ID: id},
		TenantID:          tenantID,
		OwnerUserID:       ownerUserID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationUse},
		TenantOperations:  []Operation{OperationRead, OperationUse},
		Active:            true,
	}
}

func uniqueStringValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
