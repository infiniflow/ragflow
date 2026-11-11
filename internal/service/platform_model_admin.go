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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

// PlatformProvider resolves a provider strictly inside the reserved scope.
func PlatformProvider(ctx context.Context, db *gorm.DB, providerID string) (*entity.TenantModelProvider, error) {
	var provider entity.TenantModelProvider
	if err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", providerID, entity.PlatformTenantID).Take(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

var ErrInvalidPlatformModelType = errors.New("invalid platform model type")

// ErrPlatformModelNameRequired reports a platform model without a name.
var ErrPlatformModelNameRequired = errors.New("platform model name is required")

func normalizePlatformModelTypes(types []string) error {
	for i, value := range types {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "doc_parse" && entity.ModelTypeFromString(value) == 0 {
			return ErrInvalidPlatformModelType
		}
		types[i] = value
	}
	return nil
}

func validatePlatformParserTypes(ctx context.Context, db *gorm.DB, providerID string, types []string) error {
	provider, err := PlatformProvider(ctx, db, providerID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(provider.ProviderName, "somark") && !strings.EqualFold(provider.ProviderName, "opendataloader") {
		return nil
	}
	hasParser, hasOCR := false, false
	for _, kind := range types {
		hasParser = hasParser || kind == "doc_parse"
		hasOCR = hasOCR || kind == "ocr"
	}
	if hasParser && !hasOCR {
		return ErrInvalidPlatformModelType
	}
	return nil
}

// stripPlatformModelTypes removes model type overrides from a model extra.
// JSON decoding matches keys case-insensitively, so every spelling is removed;
// types are set only through the validated model_type field.
func stripPlatformModelTypes(extra map[string]interface{}) {
	for key := range extra {
		if strings.EqualFold(key, "model_types") {
			delete(extra, key)
		}
	}
}

var ErrPlatformInstanceConflict = errors.New("platform instance already exists")

// CreatePlatformInstance is called by the authenticated platform admin API.
func (m *ModelProviderService) CreatePlatformInstance(ctx context.Context, db *gorm.DB, providerID string, req CreateProviderInstanceRequest) (*entity.TenantModelInstance, error) {
	if err := ValidateProviderInstanceName(req.InstanceName); err != nil {
		return nil, err
	}
	for i := range req.ModelInfo {
		req.ModelInfo[i].ModelName = strings.TrimSpace(req.ModelInfo[i].ModelName)
		if req.ModelInfo[i].ModelName == "" {
			return nil, ErrPlatformModelNameRequired
		}
		if len(req.ModelInfo[i].ModelTypes) == 0 {
			return nil, ErrInvalidPlatformModelType
		}
		if err := normalizePlatformModelTypes(req.ModelInfo[i].ModelTypes); err != nil {
			return nil, err
		}
		if err := validatePlatformParserTypes(ctx, db, providerID, req.ModelInfo[i].ModelTypes); err != nil {
			return nil, err
		}
		stripPlatformModelTypes(req.ModelInfo[i].Extra)
		if len(req.ModelInfo[i].ModelTypes) > 0 {
			if req.ModelInfo[i].Extra == nil {
				req.ModelInfo[i].Extra = map[string]interface{}{}
			}
			req.ModelInfo[i].Extra["model_types"] = req.ModelInfo[i].ModelTypes
		}
	}
	apiKey := NormalizeAPIKey(req.APIKey)
	data, err := json.Marshal(map[string]string{"base_url": strings.TrimSpace(req.BaseURL), "region": strings.TrimSpace(req.Region)})
	if err != nil {
		return nil, err
	}
	instance := &entity.TenantModelInstance{ID: common.GenerateToken(), ProviderID: providerID, InstanceName: req.InstanceName, APIKey: apiKey, Status: "active", Extra: string(data)}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var provider entity.TenantModelProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", providerID, entity.PlatformTenantID).Take(&provider).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&entity.TenantModelInstance{}).Where("provider_id = ? AND LOWER(instance_name) = LOWER(?)", providerID, req.InstanceName).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrPlatformInstanceConflict
		}
		if err := m.modelInstanceDAO.Create(ctx, tx, instance); err != nil {
			return err
		}
		// Platform models reach every tenant, so only the models the admin
		// lists are added; the catalog is not copied in implicitly.
		for _, model := range req.ModelInfo {
			if err := m.addModelToInstance(ctx, tx, &provider, instance, model); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

var ErrPlatformInstanceNotFound = errors.New("platform provider instance not found")

func PlatformInstance(ctx context.Context, db *gorm.DB, providerID, instanceID string) (*entity.TenantModelInstance, error) {
	if _, err := PlatformProvider(ctx, db, providerID); err != nil {
		return nil, err
	}
	var instance entity.TenantModelInstance
	if err := db.WithContext(ctx).Where("id = ? AND provider_id = ?", instanceID, providerID).Take(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformInstanceNotFound
		}
		return nil, err
	}
	return &instance, nil
}

// PlatformInstanceDetails returns form metadata without returning credentials.
func PlatformInstanceDetails(ctx context.Context, db *gorm.DB, providerID, instanceID string) (map[string]interface{}, error) {
	instance, err := PlatformInstance(ctx, db, providerID, instanceID)
	if err != nil {
		return nil, err
	}
	extra := map[string]string{}
	if instance.Extra != "" {
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
			return nil, err
		}
	}
	baseURL := extra["base_url"]
	safeBaseURL := baseURL
	if parsed, err := url.Parse(baseURL); err != nil || (parsed != nil && (parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "")) {
		safeBaseURL = ""
	}
	configured, config, status, _ := providerInstanceCredentialMetadata(instance.APIKey)
	safeConfig := platformSafeExtra(config)
	// Name the addresses withheld from safeConfig, so a form can tell them
	// apart from values that were saved empty.
	hidden := []string{}
	for key, value := range config {
		if text, ok := value.(string); ok && text != "" && safeConfig[key] == "" {
			hidden = append(hidden, key)
		}
	}
	sort.Strings(hidden)
	return map[string]interface{}{
		"id": instance.ID, "provider_id": instance.ProviderID, "instance_name": instance.InstanceName,
		"status": instance.Status, "base_url": safeBaseURL, "base_url_configured": baseURL != "", "region": extra["region"],
		"api_key_configured": configured, "api_key_config": safeConfig, "api_key_config_hidden": hidden,
		"credential_status": status,
	}, nil
}

// UpdatePlatformInstanceRequest is the platform instance edit form.
type UpdatePlatformInstanceRequest struct {
	InstanceName string                     `json:"instance_name"`
	APIKey       json.RawMessage            `json:"api_key"`
	BaseURL      string                     `json:"base_url"`
	Region       string                     `json:"region"`
	Status       *string                    `json:"status"`
	ModelInfo    *[]CreateInstanceModelInfo `json:"model_info"`
	ClearAPIKey  bool                       `json:"clear_api_key"`
}

// UpdatePlatformInstance preserves omitted credentials and existing models.
func (m *ModelProviderService) UpdatePlatformInstance(ctx context.Context, db *gorm.DB, providerID, instanceID string, req UpdatePlatformInstanceRequest) error {
	if req.ClearAPIKey {
		return errors.New("clear_api_key is not supported")
	}
	if req.ModelInfo == nil {
		return errors.New("model_info is required")
	}
	for i := range *req.ModelInfo {
		model := &(*req.ModelInfo)[i]
		if err := normalizePlatformModelTypes(model.ModelTypes); err != nil {
			return err
		}
		if err := validatePlatformParserTypes(ctx, db, providerID, model.ModelTypes); err != nil {
			return err
		}
		stripPlatformModelTypes(model.Extra)
		if len(model.ModelTypes) > 0 {
			if model.Extra == nil {
				model.Extra = map[string]interface{}{}
			}
			model.Extra["model_types"] = model.ModelTypes
		}
	}
	if req.InstanceName != "" {
		if err := ValidateProviderInstanceName(req.InstanceName); err != nil {
			return err
		}
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
		return errors.New("invalid instance status")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var provider entity.TenantModelProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", providerID, entity.PlatformTenantID).Take(&provider).Error; err != nil {
			return err
		}
		var instance entity.TenantModelInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND provider_id = ?", instanceID, providerID).Take(&instance).Error; err != nil {
			return err
		}
		if req.InstanceName != "" && !strings.EqualFold(req.InstanceName, instance.InstanceName) {
			var count int64
			if err := tx.Model(&entity.TenantModelInstance{}).Where("provider_id = ? AND LOWER(instance_name) = LOWER(?) AND id <> ?", providerID, req.InstanceName, instance.ID).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return ErrPlatformInstanceConflict
			}
		}
		apiKey, err := mergeProviderAPIKey(instance.APIKey, req.APIKey, false)
		if err != nil {
			return err
		}
		extra := map[string]interface{}{}
		if instance.Extra != "" {
			if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
				return err
			}
		}
		if extra == nil {
			extra = map[string]interface{}{}
		}
		// A blank address or region keeps the saved one.
		if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
			extra["base_url"] = baseURL
		}
		if region := strings.TrimSpace(req.Region); region != "" {
			extra["region"] = region
		}
		encoded, err := json.Marshal(extra)
		if err != nil {
			return err
		}
		updates := map[string]interface{}{"api_key": apiKey, "extra": string(encoded)}
		if req.InstanceName != "" {
			updates["instance_name"] = req.InstanceName
		}
		if req.Status != nil {
			updates["status"] = *req.Status
		}
		if err := tx.Model(&entity.TenantModelInstance{}).Where("id = ?", instance.ID).Updates(updates).Error; err != nil {
			return err
		}
		for _, model := range *req.ModelInfo {
			model.ModelName = strings.TrimSpace(model.ModelName)
			if model.ModelName == "" {
				return ErrPlatformModelNameRequired
			}
			var existing entity.TenantModel
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id = ? AND instance_id = ? AND model_name = ?", provider.ID, instance.ID, model.ModelName).Take(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if len(model.ModelTypes) == 0 {
					return ErrInvalidPlatformModelType
				}
				if err := m.addModelToInstance(ctx, tx, &provider, &instance, model); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			modelExtra := map[string]interface{}{}
			if existing.Extra != "" {
				if err := json.Unmarshal([]byte(existing.Extra), &modelExtra); err != nil {
					return err
				}
			}
			if modelExtra == nil {
				modelExtra = map[string]interface{}{}
			}
			for k, v := range model.Extra {
				modelExtra[k] = v
			}
			if model.MaxTokens > 0 {
				modelExtra["max_tokens"] = model.MaxTokens
			}
			data, err := json.Marshal(modelExtra)
			if err != nil {
				return err
			}
			modelUpdates := map[string]interface{}{"extra": string(data)}
			if len(model.ModelTypes) > 0 {
				modelUpdates["model_type"] = int(entity.ModelTypeFromStrings(model.ModelTypes))
			}
			if err := tx.Model(&entity.TenantModel{}).Where("id = ?", existing.ID).Updates(modelUpdates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PlatformModels lists models under one platform instance without credentials.
func PlatformModels(ctx context.Context, db *gorm.DB, providerID, instanceID string) ([]map[string]interface{}, error) {
	if _, err := PlatformInstance(ctx, db, providerID, instanceID); err != nil {
		return nil, err
	}
	var models []entity.TenantModel
	if err := db.WithContext(ctx).Where("provider_id = ? AND instance_id = ?", providerID, instanceID).Order("model_name").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(models))
	for _, model := range models {
		extra := map[string]interface{}{}
		if model.Extra != "" {
			if err := json.Unmarshal([]byte(model.Extra), &extra); err != nil {
				return nil, err
			}
		}
		extra = platformSafeExtra(extra)
		maxTokens := 8192
		if n, ok := extra["max_tokens"].(float64); ok {
			maxTokens = int(n)
		}
		modelTypes := entity.ModelType(model.ModelType).HumanReadable()
		if saved, ok := extra["model_types"].([]interface{}); ok {
			for _, item := range saved {
				if value, ok := item.(string); ok && value == "doc_parse" {
					modelTypes = append(modelTypes, value)
				}
			}
		}
		items = append(items, map[string]interface{}{"model_id": model.ID, "model_name": model.ModelName, "model_type": modelTypes, "status": model.Status, "max_tokens": maxTokens, "extra": extra})
	}
	return items, nil
}

func platformSafeExtra(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		if key != "max_tokens" && (isProviderCredentialField(key) || strings.Contains(strings.ToLower(key), "authorization")) {
			continue
		}
		if text, ok := value.(string); ok && isPlatformAddressField(key) {
			if _, err := url.Parse(text); err != nil {
				out[key] = ""
				continue
			}
		}
		out[key] = platformSafeExtraValue(value)
	}
	return out
}

func isPlatformAddressField(key string) bool {
	switch strings.ToLower(key) {
	case "base_url", "mineru_apiserver", "mineru_server_url", "paddleocr_api_url", "opendataloader_apiserver":
		return true
	default:
		return false
	}
}

func platformSafeExtraValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		parsed, err := url.Parse(typed)
		if err == nil && (parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "") {
			return ""
		}
		return typed
	case map[string]interface{}:
		return platformSafeExtra(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = platformSafeExtraValue(item)
		}
		return out
	default:
		return value
	}
}

var ErrPlatformModelConflict = errors.New("platform model already exists")

// AddPlatformModel adds a model without impersonating a tenant owner.
func (m *ModelProviderService) AddPlatformModel(ctx context.Context, db *gorm.DB, providerID, instanceID string, req AddModelRequest) (string, error) {
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ModelName == "" || len(req.ModelTypes) == 0 {
		return "", errors.New("model name and type are required")
	}
	if err := normalizePlatformModelTypes(req.ModelTypes); err != nil {
		return "", err
	}
	if err := validatePlatformParserTypes(ctx, db, providerID, req.ModelTypes); err != nil {
		return "", err
	}
	if req.Extra == nil {
		req.Extra = map[string]interface{}{}
	}
	// Model types are set only through model_type, which is validated.
	stripPlatformModelTypes(req.Extra)
	req.Extra["model_types"] = req.ModelTypes
	var id string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock before any plain read: under REPEATABLE READ the first read
		// fixes the snapshot, and the duplicate check below must see models
		// committed by the transaction that held these locks.
		var provider entity.TenantModelProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", providerID, entity.PlatformTenantID).Take(&provider).Error; err != nil {
			return err
		}
		var instance entity.TenantModelInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND provider_id = ?", instanceID, providerID).Take(&instance).Error; err != nil {
			return err
		}
		_, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(ctx, tx, provider.ID, instance.ID, req.ModelName)
		if err == nil {
			return ErrPlatformModelConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		maxTokens := req.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		if err := m.addModelToInstance(ctx, tx, &provider, &instance, CreateInstanceModelInfo{ModelName: req.ModelName, ModelTypes: req.ModelTypes, MaxTokens: maxTokens, Extra: req.Extra}); err != nil {
			return err
		}
		model, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(ctx, tx, providerID, instanceID, req.ModelName)
		if err != nil {
			return err
		}
		id = model.ID
		return nil
	})
	return id, err
}

type UpdatePlatformModelRequest struct {
	Status     *string                `json:"status"`
	ModelTypes *[]string              `json:"model_type"`
	MaxTokens  *int                   `json:"max_tokens"`
	Extra      map[string]interface{} `json:"extra"`
}

func UpdatePlatformModel(ctx context.Context, db *gorm.DB, providerID, instanceID, modelID string, req UpdatePlatformModelRequest) error {
	if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
		return errors.New("invalid model status")
	}
	if req.ModelTypes != nil && len(*req.ModelTypes) == 0 {
		return errors.New("model_type cannot be empty")
	}
	if req.ModelTypes != nil {
		if err := normalizePlatformModelTypes(*req.ModelTypes); err != nil {
			return err
		}
		if err := validatePlatformParserTypes(ctx, db, providerID, *req.ModelTypes); err != nil {
			return err
		}
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := PlatformInstance(ctx, tx, providerID, instanceID); err != nil {
			return err
		}
		var model entity.TenantModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND provider_id = ? AND instance_id = ?", modelID, providerID, instanceID).Take(&model).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{}
		if req.Status != nil {
			updates["status"] = *req.Status
		}
		if req.ModelTypes != nil {
			updates["model_type"] = int(entity.ModelTypeFromStrings(*req.ModelTypes))
		}
		if req.Extra != nil || req.MaxTokens != nil || req.ModelTypes != nil {
			extra := map[string]interface{}{}
			if model.Extra != "" {
				if err := json.Unmarshal([]byte(model.Extra), &extra); err != nil {
					return err
				}
			}
			if extra == nil {
				extra = map[string]interface{}{}
			}
			for key, value := range req.Extra {
				// Model types are set only through model_type, which is validated.
				if !strings.EqualFold(key, "model_types") {
					extra[key] = value
				}
			}
			if req.ModelTypes != nil {
				extra["model_types"] = *req.ModelTypes
			}
			if req.MaxTokens != nil && *req.MaxTokens > 0 {
				extra["max_tokens"] = *req.MaxTokens
			}
			encoded, err := json.Marshal(extra)
			if err != nil {
				return err
			}
			updates["extra"] = string(encoded)
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&entity.TenantModel{}).Where("id = ?", model.ID).Updates(updates).Error
	})
}

// ResolvePlatformProviderConfig checks the complete platform path before
// retrieving saved credentials for admin-only connection operations.
func ResolvePlatformProviderConfig(ctx context.Context, db *gorm.DB, providerID string, req CheckConnectionRequest) (*entity.TenantModelProvider, SavedProviderModelConfig, error) {
	provider, err := PlatformProvider(ctx, db, providerID)
	if err != nil {
		return nil, SavedProviderModelConfig{}, err
	}
	if req.ClearAPIKey {
		return nil, SavedProviderModelConfig{}, errors.New("clear_api_key is not supported")
	}
	config := SavedProviderModelConfig{APIKey: strings.TrimSpace(NormalizeAPIKey(req.APIKey)), BaseURL: strings.TrimSpace(req.BaseURL), Region: strings.TrimSpace(req.Region)}
	var instance *entity.TenantModelInstance
	if req.InstanceID != "" {
		instance, err = PlatformInstance(ctx, db, providerID, req.InstanceID)
		if err != nil {
			return nil, SavedProviderModelConfig{}, err
		}
	}
	if req.UseSavedCredentials {
		if instance == nil || config.APIKey != "" {
			return nil, SavedProviderModelConfig{}, errors.New("saved credentials require an instance and no submitted key")
		}
		config.APIKey = instance.APIKey
		if hasSavedProviderAPIKeyConfig(req.APIKeyConfig) {
			config.APIKey, err = mergeSavedProviderAPIKeyConfig(config.APIKey, req.APIKeyConfig)
			if err != nil {
				return nil, SavedProviderModelConfig{}, err
			}
		}
	} else if hasSavedProviderAPIKeyConfig(req.APIKeyConfig) {
		return nil, SavedProviderModelConfig{}, errors.New("api_key_config requires saved credentials")
	}
	if instance != nil {
		extra := map[string]string{}
		if instance.Extra != "" {
			if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
				return nil, SavedProviderModelConfig{}, err
			}
		}
		if config.BaseURL == "" {
			config.BaseURL = extra["base_url"]
		}
		if config.Region == "" {
			config.Region = extra["region"]
		}
	}
	if config.Region == "" {
		config.Region = "default"
	}
	return provider, config, nil
}

// CheckPlatformConnection shares the existing provider verification machinery.
func CheckPlatformConnection(ctx context.Context, db *gorm.DB, providerID string, req CheckConnectionRequest) error {
	provider, config, err := ResolvePlatformProviderConfig(ctx, db, providerID, req)
	if err != nil {
		return err
	}
	catalog := dao.GetModelProviderManager().FindProvider(provider.ProviderName)
	if catalog == nil || catalog.ModelDriver == nil {
		return errors.New("provider is unavailable")
	}
	driver := catalog.ModelDriver
	if strings.EqualFold(catalog.Class, "local") {
		if config.BaseURL == "" {
			return errors.New("base_url is required")
		}
		driver, err = newModelDriverForBaseURL(driver, provider.ProviderName, config.Region, config.BaseURL)
		if err != nil {
			return err
		}
	}
	apiConfig := &models.APIConfig{ApiKey: &config.APIKey, BaseURL: &config.BaseURL, Region: &config.Region}
	var results map[string]string
	var verifyErr error
	if len(req.ModelInfo) == 0 {
		results, verifyErr = verifyProviderModel(ctx, driver, catalog.Models, apiConfig, nil)
	} else {
		results = make(map[string]string)
		var failures []error
		for _, model := range req.ModelInfo {
			itemResults, itemErr := verifyProviderModel(ctx, driver, catalog.Models, apiConfig, []CheckConnectionModelInfo{model})
			for name, status := range itemResults {
				results[name] = status
			}
			if itemErr != nil {
				failures = append(failures, itemErr)
			}
		}
		if len(failures) > 0 {
			verifyErr = errors.Join(failures...)
		}
	}
	specified := make(map[string]struct{}, len(req.ModelInfo))
	for _, model := range req.ModelInfo {
		if name := strings.TrimSpace(model.ModelName); name != "" {
			specified[name] = struct{}{}
		}
	}
	if req.InstanceID != "" {
		for name, status := range results {
			if _, ok := specified[name]; !ok {
				continue
			}
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var model entity.TenantModel
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id = ? AND instance_id = ? AND model_name = ?", provider.ID, req.InstanceID, name).Take(&model).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				if err != nil {
					return err
				}
				extra := map[string]interface{}{}
				if model.Extra != "" {
					if err := json.Unmarshal([]byte(model.Extra), &extra); err != nil {
						return err
					}
				}
				if extra == nil {
					extra = map[string]interface{}{}
				}
				extra["verify"] = status
				data, err := json.Marshal(extra)
				if err != nil {
					return err
				}
				return tx.Model(&entity.TenantModel{}).Where("id = ? AND provider_id = ? AND instance_id = ?", model.ID, provider.ID, req.InstanceID).Update("extra", string(data)).Error
			})
			if err != nil {
				return err
			}
		}
	}
	return verifyErr
}
