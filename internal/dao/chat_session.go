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

package dao

import (
	"ragflow/internal/model"
)

// ChatSessionDAO chat session data access object
type ChatSessionDAO struct{}

// NewChatSessionDAO create chat session DAO
func NewChatSessionDAO() *ChatSessionDAO {
	return &ChatSessionDAO{}
}

// GetByID gets chat session by ID
func (dao *ChatSessionDAO) GetByID(id string) (*model.ChatSession, error) {
	var conv model.ChatSession
	err := DB.Where("id = ?", id).First(&conv).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// Create creates a new chat session
func (dao *ChatSessionDAO) Create(conv *model.ChatSession) error {
	return DB.Create(conv).Error
}

// UpdateByID updates a chat session by ID
func (dao *ChatSessionDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&model.ChatSession{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID deletes a chat session by ID (hard delete)
func (dao *ChatSessionDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&model.ChatSession{}).Error
}

// ListByDialogID lists chat sessions by dialog ID
func (dao *ChatSessionDAO) ListByDialogID(dialogID string) ([]*model.ChatSession, error) {
	var convs []*model.ChatSession
	err := DB.Where("dialog_id = ?", dialogID).
		Order("create_time DESC").
		Find(&convs).Error
	return convs, err
}

// CheckDialogExists checks if a dialog exists with given tenant_id and dialog_id
func (dao *ChatSessionDAO) CheckDialogExists(tenantID, dialogID string) (bool, error) {
	var count int64
	err := DB.Model(&model.Chat{}).
		Where("tenant_id = ? AND id = ? AND status = ?", tenantID, dialogID, "1").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetDialogByID gets dialog by ID
func (dao *ChatSessionDAO) GetDialogByID(dialogID string) (*model.Chat, error) {
	var dialog model.Chat
	err := DB.Where("id = ? AND status = ?", dialogID, "1").First(&dialog).Error
	if err != nil {
		return nil, err
	}
	return &dialog, nil
}
