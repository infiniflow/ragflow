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

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

var platformDefaultTypes = []string{"chat", "embedding", "asr", "vision", "rerank", "tts", "ocr", "decision"}
var errInvalidPlatformDefault = errors.New("invalid platform default model")

type platformDefaultItem struct {
	ModelType string `json:"model_type"`
	ModelID   string `json:"model_id"`
}

func listPlatformDefaults(ctx context.Context, db *gorm.DB) ([]platformDefaultItem, error) {
	items := make([]platformDefaultItem, 0)
	for _, modelType := range platformDefaultTypes {
		id, err := (dao.PlatformModelSettings{}).Default(ctx, db, modelType)
		if err != nil {
			return nil, err
		}
		if id != "" {
			items = append(items, platformDefaultItem{ModelType: modelType, ModelID: id})
		}
	}
	return items, nil
}

func setPlatformDefault(ctx context.Context, db *gorm.DB, modelType, modelID string) error {
	modelType = strings.ToLower(strings.TrimSpace(modelType))
	switch modelType {
	case "speech2text":
		modelType = "asr"
	case "image2text":
		modelType = "vision"
	}
	valid := false
	for _, candidate := range platformDefaultTypes {
		if candidate == modelType {
			valid = true
			break
		}
	}
	if !valid {
		return errInvalidPlatformDefault
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return (dao.PlatformModelSettings{}).SetDefault(ctx, db, modelType, "")
	}
	var model entity.TenantModel
	err := db.WithContext(ctx).Where("id = ?", modelID).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errInvalidPlatformDefault
	}
	if err != nil {
		return err
	}
	var provider entity.TenantModelProvider
	err = db.WithContext(ctx).Where("id = ? AND tenant_id = ?", model.ProviderID, entity.PlatformTenantID).Take(&provider).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errInvalidPlatformDefault
	}
	if err != nil {
		return err
	}
	var instance entity.TenantModelInstance
	err = db.WithContext(ctx).Where("id = ? AND provider_id = ?", model.InstanceID, provider.ID).Take(&instance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errInvalidPlatformDefault
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(model.Status, "active") || !strings.EqualFold(instance.Status, "active") || !entity.ModelType(model.ModelType).Has(entity.ModelTypeFromString(modelType)) {
		return errInvalidPlatformDefault
	}
	return (dao.PlatformModelSettings{}).SetDefault(ctx, db, modelType, modelID)
}

func (h *Handler) ListPlatformDefaults(c *gin.Context) {
	items, err := listPlatformDefaults(c.Request.Context(), dao.DB)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to load platform defaults")
		return
	}
	common.RespondSuccess(c, gin.H{"models": items}, "")
}

func (h *Handler) SetPlatformDefault(c *gin.Context) {
	var req struct {
		ModelType string          `json:"model_type"`
		ModelID   json.RawMessage `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "Invalid request")
		return
	}
	if len(req.ModelID) == 0 {
		common.RespondError(c, common.CodeArgumentError, "model_id is required")
		return
	}
	var id string
	if string(req.ModelID) != "null" {
		if err := json.Unmarshal(req.ModelID, &id); err != nil {
			common.RespondError(c, common.CodeArgumentError, "Invalid model_id")
			return
		}
	}
	err := setPlatformDefault(c.Request.Context(), dao.DB, req.ModelType, id)
	if errors.Is(err, errInvalidPlatformDefault) {
		common.RespondError(c, common.CodeDataError, errInvalidPlatformDefault.Error())
		return
	}
	if err != nil {
		common.RespondError(c, common.CodeServerError, "Failed to update platform default")
		return
	}
	common.RespondSuccess(c, nil, "")
}
