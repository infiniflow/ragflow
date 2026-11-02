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

	"ragflow/internal/entity"

	"gorm.io/gorm/clause"
)

type skillSpaceAuthorization struct {
	ID       string
	TenantID string
	FolderID string
}

type folderAuthorization struct {
	TenantID string
	Type     string
}

func (s *databaseSource) loadMCPServerTenants(ctx context.Context, ids []string) (map[string]string, error) {
	tenantIDsByServer := make(map[string]string)
	ids = uniqueStringValues(ids)
	if len(ids) == 0 {
		return tenantIDsByServer, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.MCPServer{}).
		Select("id", "tenant_id").
		Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var servers []struct {
		ID       string
		TenantID string
	}
	if err := query.Scan(&servers).Error; err != nil {
		return nil, err
	}
	for _, server := range servers {
		tenantIDsByServer[server.ID] = server.TenantID
	}
	return tenantIDsByServer, nil
}

func (s *databaseSource) loadChatChannels(ctx context.Context, ids []string) (map[string]*entity.ChatChannel, error) {
	channelsByID := make(map[string]*entity.ChatChannel)
	ids = uniqueStringValues(ids)
	if len(ids) == 0 {
		return channelsByID, nil
	}
	query := s.db.WithContext(ctx).Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var channels []*entity.ChatChannel
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		channelsByID[channel.ID] = channel
	}
	return channelsByID, nil
}

func (s *databaseSource) loadSkillSpaces(ctx context.Context, ids []string) (map[string]skillSpaceAuthorization, error) {
	spacesByID := make(map[string]skillSpaceAuthorization)
	ids = uniqueStringValues(ids)
	if len(ids) == 0 {
		return spacesByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.SkillSpace{}).
		Select("id", "tenant_id", "folder_id").
		Where("id IN ? AND status IN ?", ids, []string{entity.SpaceStatusActive, entity.SpaceStatusDeleting})
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var spaces []skillSpaceAuthorization
	if err := query.Scan(&spaces).Error; err != nil {
		return nil, err
	}
	for _, space := range spaces {
		spacesByID[space.ID] = space
	}
	return spacesByID, nil
}

func (s *databaseSource) loadSkillSpaceFolders(ctx context.Context, ids []string) (map[string]folderAuthorization, error) {
	foldersByID := make(map[string]folderAuthorization)
	ids = uniqueStringValues(ids)
	if len(ids) == 0 {
		return foldersByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.File{}).
		Select("id", "tenant_id", "type").
		Where("id IN ?", ids)
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var folders []struct {
		ID       string
		TenantID string
		Type     string
	}
	if err := query.Scan(&folders).Error; err != nil {
		return nil, err
	}
	for _, folder := range folders {
		foldersByID[folder.ID] = folderAuthorization{TenantID: folder.TenantID, Type: folder.Type}
	}
	return foldersByID, nil
}
