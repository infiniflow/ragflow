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

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"

	"gorm.io/gorm"
)

// tenantPDFParserEngine identifies parser backends, not generic ModelDrivers.
// Their tenant records are resolved directly through the existing DAOs because
// SoMark and OpenDataLoader do not have a chat/OCR driver in conf/models.
func tenantPDFParserEngine(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "somark":
		return "somark"
	case "opendataloader":
		return "opendataloader"
	}
	return ""
}

// maybeDispatchTenantPDFParser resolves an explicit model at execution time.
// Bare engine names retain setup/environment-based parsing. A selected tenant
// reference never falls back to another instance, environment settings or VLM.
func maybeDispatchTenantPDFParser(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup) (parser.ParseResult, bool, error) {
	selector, selected := resolvePDFVisionModelID(setup)
	if !selected {
		return parser.ParseResult{}, false, nil
	}
	composite := strings.Contains(selector, "@")
	engine := ""
	modelName, instanceName, providerName := "", "", ""
	if composite {
		var err error
		modelName, instanceName, providerName, err = parseCompositeModelName(selector)
		engine = tenantPDFParserEngine(providerName)
		if engine == "" {
			return parser.ParseResult{}, false, nil
		}
		if err != nil || strings.TrimSpace(modelName) == "" || strings.TrimSpace(instanceName) == "" {
			return parser.ParseResult{}, true, fmt.Errorf("parser: invalid %s model reference", engine)
		}
		if engine == "somark" {
			providerName = "SoMark"
		} else {
			providerName = "OpenDataLoader"
		}
	}
	if db == nil {
		if engine == "" {
			return parser.ParseResult{}, false, nil
		}
		return parser.ParseResult{}, true, fmt.Errorf("parser: %s model resolution requires a database", engine)
	}

	modelDAO := dao.NewTenantModelDAO()
	providerDAO := dao.NewTenantModelProviderDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	var model *entity.TenantModel
	var provider *entity.TenantModelProvider
	var instance *entity.TenantModelInstance
	var err error
	if composite {
		provider, err = providerDAO.GetByTenantIDAndProviderName(ctx, db, tenantID, providerName)
		if err == nil {
			instance, err = instanceDAO.GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
		}
		if err == nil {
			model, err = modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
				ctx, db, provider.ID, instance.ID, int(entity.ModelTypeOCR), modelName)
		}
	} else {
		model, err = modelDAO.GetByID(ctx, db, selector)
		if err == nil {
			provider, err = providerDAO.GetByID(ctx, db, model.ProviderID)
		}
		if err == nil {
			engine = tenantPDFParserEngine(provider.ProviderName)
			if engine == "" {
				return parser.ParseResult{}, false, nil
			}
			instance, err = instanceDAO.GetByID(ctx, db, model.InstanceID)
		}
	}
	if err != nil {
		return parser.ParseResult{}, true, fmt.Errorf("parser: tenant PDF model lookup: %w", err)
	}
	if tenantID == "" || provider.TenantID != tenantID || instance.ProviderID != provider.ID {
		return parser.ParseResult{}, true, fmt.Errorf("parser: selected PDF model does not belong to this tenant/provider")
	}
	if model.Status != "active" || instance.Status != "active" || !entity.ModelType(model.ModelType).Has(entity.ModelTypeOCR) {
		return parser.ParseResult{}, true, fmt.Errorf("parser: selected PDF model and instance must be active OCR configuration")
	}

	baseURL, apiKey, err := tenantPDFParserCredentials(engine, instance)
	if err != nil {
		return parser.ParseResult{}, true, err
	}
	options := maps.Clone(setup)
	if engine == "somark" && strings.TrimSpace(model.Extra) != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(model.Extra), &extra); err != nil {
			return parser.ParseResult{}, true, fmt.Errorf("parser: invalid selected PDF model options")
		}
		// Only non-secret parser options belong here. Explicit pipeline options
		// win; credentials and endpoint always come from the selected instance.
		for _, key := range []string{
			"somark_image_format", "somark_formula_format", "somark_table_format", "somark_cs_format",
			"somark_enable_text_cross_page", "somark_enable_table_cross_page",
			"somark_enable_title_level_recognition", "somark_enable_inline_image",
			"somark_enable_table_image", "somark_enable_image_understanding", "somark_keep_header_footer",
		} {
			if value, ok := extra[key]; ok {
				if _, explicit := options[key]; !explicit {
					_, valid := value.(bool)
					if strings.HasSuffix(key, "_format") {
						_, valid = value.(string)
					}
					if !valid {
						return parser.ParseResult{}, true, fmt.Errorf("parser: invalid selected PDF model option %s", key)
					}
					options[key] = value
				}
			}
		}
	}
	pdf := parser.NewPDFParser()
	pdf.ConfigureFromSetup(options)
	pdf.ParseMethod = engine
	pdf.UseTenantModelConfig = true
	if engine == "somark" {
		pdf.SoMarkBaseURL, pdf.SoMarkAPIKey = baseURL, apiKey
	} else {
		pdf.OpenDataLoaderAPIServer, pdf.OpenDataLoaderAPIKey = baseURL, apiKey
	}
	result := pdf.ParseWithResult(ctx, filename, binary)
	if result.Err != nil {
		return result, true, result.Err
	}
	if result.File == nil {
		result.File = make(map[string]any)
	}
	result.File["parse_method"] = selector
	return result, true, nil
}

// tenantPDFParserCredentials accepts instance-card JSON and ordinary key/base
// URL storage. Missing optional keys stay empty, never borrowing a global key.
func tenantPDFParserCredentials(engine string, instance *entity.TenantModelInstance) (string, string, error) {
	var extra struct {
		BaseURL string `json:"base_url"`
	}
	if strings.TrimSpace(instance.Extra) != "" {
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
			return "", "", fmt.Errorf("parser: invalid selected PDF instance settings")
		}
	}
	baseURL := strings.TrimSpace(extra.BaseURL)
	key := instance.APIKey
	if strings.HasPrefix(strings.TrimSpace(key), "{") {
		var config map[string]any
		if err := json.Unmarshal([]byte(key), &config); err != nil {
			return "", "", fmt.Errorf("parser: invalid selected PDF instance credentials")
		}
		if nested, ok := config["api_key"].(map[string]any); ok {
			config = nested
		}
		for _, field := range []string{
			engine + "_apiserver", engine + "_base_url", strings.ToUpper(engine) + "_APISERVER",
			strings.ToUpper(engine) + "_BASE_URL", "base_url", engine + "_api_key",
			strings.ToUpper(engine) + "_API_KEY", "api_key",
		} {
			if value, ok := config[field]; ok && value != nil {
				if _, ok := value.(string); !ok {
					return "", "", fmt.Errorf("parser: invalid selected PDF instance credential field %s", field)
				}
			}
		}
		if baseURL == "" {
			baseURL = modelModule.ProviderJSONConfigValue(key,
				engine+"_apiserver", engine+"_base_url", strings.ToUpper(engine)+"_APISERVER",
				strings.ToUpper(engine)+"_BASE_URL", "base_url")
		}
		key = modelModule.ProviderJSONConfigValue(key, engine+"_api_key", strings.ToUpper(engine)+"_API_KEY", "api_key")
	}
	if baseURL == "" {
		return "", "", fmt.Errorf("parser: selected %s instance requires an endpoint", engine)
	}
	return baseURL, strings.TrimSpace(key), nil
}
