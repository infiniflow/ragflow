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

package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReasoningSimple represents simple reasoning capability
type ReasoningSimple struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Default bool   `json:"default"`
}

// ReasoningBudget represents budget-based reasoning capability
type ReasoningBudget struct {
	Type          string `json:"type"`
	Enabled       bool   `json:"enabled"`
	DefaultTokens int    `json:"default_tokens"`
	TokenRange    struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"token_range"`
}

// ReasoningEffort represents effort-based reasoning capability
type ReasoningEffort struct {
	Type    string   `json:"type"`
	Enabled bool     `json:"enabled"`
	Default string   `json:"default"`
	Options []string `json:"options"`
}

// Reasoning represents the reasoning capability (can be one of three types)
type Reasoning struct {
	Simple  *ReasoningSimple `json:"-"`
	Budget  *ReasoningBudget `json:"-"`
	Effort  *ReasoningEffort `json:"-"`
	RawType string           `json:"type"`
}

// Reasoning represents the reasoning capability (can be one of three types)
type ClearReasoningContent struct {
	DefaultValue    bool     `json:"default_value"`
	SupportedModels []string `json:"supported_models"`
}

// Reasoning represents the reasoning capability (can be one of three types)
type Thinking struct {
	DefaultValue    bool     `json:"default_value"`
	SupportedModels []string `json:"supported_models"`
}

// UnmarshalJSON custom unmarshal for Reasoning
func (r *Reasoning) UnmarshalJSON(data []byte) error {
	var temp map[string]interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	typeVal, ok := temp["type"].(string)
	if !ok {
		return fmt.Errorf("reasoning type is required")
	}

	r.RawType = typeVal

	switch typeVal {
	case "simple":
		var simple ReasoningSimple
		dataBytes, _ := json.Marshal(temp)
		if err := json.Unmarshal(dataBytes, &simple); err != nil {
			return err
		}
		r.Simple = &simple
	case "budget":
		var budget ReasoningBudget
		dataBytes, _ := json.Marshal(temp)
		if err := json.Unmarshal(dataBytes, &budget); err != nil {
			return err
		}
		r.Budget = &budget
	case "effort":
		var effort ReasoningEffort
		dataBytes, _ := json.Marshal(temp)
		if err := json.Unmarshal(dataBytes, &effort); err != nil {
			return err
		}
		r.Effort = &effort
	default:
		return fmt.Errorf("unknown reasoning type: %s", typeVal)
	}

	return nil
}

// MarshalJSON custom marshal for Reasoning
func (r *Reasoning) MarshalJSON() ([]byte, error) {
	switch r.RawType {
	case "simple":
		if r.Simple != nil {
			return json.Marshal(r.Simple)
		}
	case "budget":
		if r.Budget != nil {
			return json.Marshal(r.Budget)
		}
	case "effort":
		if r.Effort != nil {
			return json.Marshal(r.Effort)
		}
	}
	return nil, fmt.Errorf("invalid reasoning state")
}

// Multimodal represents multimodal capability
type Multimodal struct {
	Enabled          bool     `json:"enabled"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
}

// Features represents all features of a model
type Features struct {
	Multimodal    *Multimodal            `json:"multimodal,omitempty"`
	Reasoning     *Reasoning             `json:"reasoning,omitempty"`
	Thinking      *Thinking              `json:"thinking,omitempty"`
	ClearThinking *ClearReasoningContent `json:"clear_thinking,omitempty"`
}

type ModelThinking struct {
	DefaultValue  bool `json:"default_value"`
	ClearThinking bool `json:"clear_thinking"`
}

// Model represents a single LLM model
type Model struct {
	Name         string         `json:"name"`
	MaxTokens    *int           `json:"max_tokens"`
	ModelTypes   []string       `json:"model_types"`
	Thinking     *ModelThinking `json:"thinking"`
	Class        *string        `json:"class"`
	MaxDimension *int           `json:"max_dimension"` // used by embedding models
	Dimensions   []int          `json:"dimensions"`
	Alias        []string       `json:"alias"`
	ModelTypeMap map[string]bool
}

// Provider represents an LLM provider
type Provider struct {
	Name        string            `json:"name"`
	URL         map[string]string `json:"url"`
	URLSuffix   URLSuffix         `json:"url_suffix"`
	Models      []*Model          `json:"models"`
	Features    Features          `json:"features"`
	Class       string            `json:"class"`
	ModelDriver ModelDriver
}

// ProviderManager manages provider and model operations
type ProviderManager struct {
	Providers        []Provider     `json:"model_providers"`
	AllModels        []Model        `json:"all_models"`
	Alias2ModelIndex map[string]int `json:"alias2_model_index_map"`
}

// ModelResponse represents the standard response structure
type ModelResponse struct {
	Code    int                      `json:"code"`
	Data    []map[string]interface{} `json:"data"`
	Message string                   `json:"message"`
}

func decodeProviderConfig(data []byte) (Provider, error) {
	var provider Provider
	if err := json.Unmarshal(data, &provider); err != nil {
		return Provider{}, err
	}

	var rawProvider struct {
		URLSuffix json.RawMessage `json:"url_suffix"`
	}
	if err := json.Unmarshal(data, &rawProvider); err != nil {
		return Provider{}, err
	}
	if len(rawProvider.URLSuffix) == 0 {
		return provider, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(rawProvider.URLSuffix))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provider.URLSuffix); err != nil {
		return Provider{}, err
	}

	return provider, nil
}

var providerManager *ProviderManager

func GetProviderManager() *ProviderManager {
	return providerManager
}

// InitProviderManager creates a new ProviderManager by reading all JSON files from a directory
func InitProviderManager(dirPath string) error {
	providers := []Provider{}

	// Read all files in the directory
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("error reading directory %s: %w", dirPath, err)
	}

	modelFactory := NewModelFactory()

	// Iterate through all files
	for _, file := range files {
		// Skip directories
		if file.IsDir() {
			continue
		}

		// Only process JSON files
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// Build full file path
		filePath := filepath.Join(dirPath, file.Name())

		// Read the file
		var data []byte
		data, err = os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", filePath, err)
		}

		// Parse JSON
		var provider Provider
		if provider, err = decodeProviderConfig(data); err != nil {
			return fmt.Errorf("error parsing JSON from file %s: %w", filePath, err)
		}

		for _, model := range provider.Models {
			// if the prefix of mode.Name is matched with keys of modelSupportThinking
			if provider.Class == "" {
				pos := strings.Index(model.Name, "-")
				if pos >= 0 {
					modelClass := model.Name[0:pos]
					model.Class = &modelClass
				}
			} else {
				model.Class = &provider.Name
			}

			model.ModelTypeMap = make(map[string]bool)
			for _, modelType := range model.ModelTypes {
				model.ModelTypeMap[modelType] = true
			}
		}

		provider.ModelDriver, err = modelFactory.CreateModelDriver(provider.Name, provider.URL, provider.URLSuffix)
		if err != nil {
			return fmt.Errorf("error creating model driver for provider %s: %w", provider.Name, err)
		}

		// Add to providers list
		providers = append(providers, provider)
	}

	if len(providers) == 0 {
		return fmt.Errorf("no JSON files found in directory %s", dirPath)
	}

	// Read the file.  Use a repo-root-relative path so that go test
	// (which sets CWD to the package directory) can still find it.
	var data []byte
	data, err = os.ReadFile(filepath.Join(findRepoRoot(), "conf", "all_models.json"))
	if err != nil {
		return fmt.Errorf("error reading file 'conf/all_models.json': %w", err)
	}

	// Parse JSON
	type AllModels struct {
		Models []Model `json:"models"`
	}
	var allModels AllModels
	if err = json.Unmarshal(data, &allModels); err != nil {
		return fmt.Errorf("error parsing JSON from file 'conf/all_models.json': %w", err)
	}

	alias2ModelIndex := make(map[string]int)
	for idx, model := range allModels.Models {
		if model.Alias == nil {
			alias2ModelIndex[strings.ToLower(model.Name)] = idx
		} else {
			for _, alias := range model.Alias {
				lowerAlias := strings.ToLower(alias)
				if existingIdx, ok := alias2ModelIndex[lowerAlias]; ok && existingIdx != idx {
					return fmt.Errorf("duplicate alias %q for models %q and %q", alias, allModels.Models[existingIdx].Name, model.Name)
				}
				alias2ModelIndex[lowerAlias] = idx
			}
		}
	}

	providerManager = &ProviderManager{
		Providers:        providers,
		AllModels:        allModels.Models,
		Alias2ModelIndex: alias2ModelIndex,
	}
	return nil
}

// 1. List all providers
func (pm *ProviderManager) ListProviders() ([]map[string]interface{}, error) {

	var providers []map[string]interface{}

	for _, provider := range pm.Providers {

		modelTypeSet := make(map[string]struct{})
		for _, model := range provider.Models {
			for _, modelType := range model.ModelTypes {
				modelTypeSet[modelType] = struct{}{}
			}
		}

		var modelTypes []string
		for modelType := range modelTypeSet {
			modelTypes = append(modelTypes, modelType)
		}

		providerData := map[string]interface{}{
			"name":        provider.Name,
			"url":         provider.URL,
			"model_types": modelTypes,
			"url_suffix":  provider.URLSuffix,
		}
		providers = append(providers, providerData)
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers found")
	}

	return providers, nil
}

func (pm *ProviderManager) ListAllModels() ([]map[string]interface{}, error) {

	var modelList []map[string]interface{}

	for _, model := range pm.AllModels {

		modelData := map[string]interface{}{
			"name":        model.Name,
			"model_types": model.ModelTypes,
		}
		if model.Alias != nil {
			modelData["alias"] = model.Alias
		}
		if model.Thinking != nil {
			modelData["thinking"] = model.Thinking
		}
		if model.MaxTokens != nil {
			modelData["max_tokens"] = *model.MaxTokens
		}
		if model.MaxDimension != nil {
			modelData["max_dimension"] = *model.MaxDimension
		}
		if len(model.Dimensions) > 0 {
			modelData["dimensions"] = model.Dimensions
		}
		modelList = append(modelList, modelData)
	}

	if len(modelList) == 0 {
		return nil, fmt.Errorf("no models found")
	}

	return modelList, nil
}

func (pm *ProviderManager) GetModelByNameOrAlias(modelName string) *Model {
	lowerModelName := strings.ToLower(modelName)
	// Check if it is alias
	modelIndex, ok := pm.Alias2ModelIndex[lowerModelName]
	if ok {
		return &pm.AllModels[modelIndex]
	}
	return nil
}

// 2. Show specific provider information (including base_url)
func (pm *ProviderManager) GetProviderByName(providerName string) (map[string]interface{}, error) {

	provider := pm.FindProvider(providerName)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' not found", providerName)
	}

	providerInfo := map[string]interface{}{
		"name":         provider.Name,
		"base_url":     provider.URL,
		"total_models": len(provider.Models),
	}

	return providerInfo, nil
}

// 3. List models under a specific provider
func (pm *ProviderManager) ListModels(providerName string) ([]map[string]interface{}, error) {
	provider := pm.FindProvider(providerName)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' not found", providerName)
	}

	modelList := []map[string]interface{}{}
	for _, model := range provider.Models {
		modelData := map[string]interface{}{
			"name":        model.Name,
			"max_tokens":  model.MaxTokens,
			"model_types": model.ModelTypes,
		}
		if model.MaxDimension != nil {
			modelData["max_dimension"] = *model.MaxDimension
		}
		if len(model.Dimensions) > 0 {
			modelData["dimensions"] = model.Dimensions
		}
		modelList = append(modelList, modelData)
	}

	if len(modelList) == 0 {
		return nil, fmt.Errorf("no models found")
	}

	return modelList, nil
}

func (pm *ProviderManager) GetModelByName(providerName, modelName string) (*Model, error) {
	provider := pm.FindProvider(providerName)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' not found", providerName)
	}
	model := pm.findModel(provider, modelName)
	if model == nil {
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}
	return model, nil
}

func (pm *ProviderManager) GetModelUrl(providerName, modelName, modelType string) (*string, *string, error) {
	provider := pm.FindProvider(providerName)
	if provider == nil {
		return nil, nil, fmt.Errorf("provider '%s' not found", providerName)
	}
	model := pm.findModel(provider, modelName)
	if model == nil {
		return nil, nil, fmt.Errorf("model '%s' not found", modelName)
	}

	if !model.ModelTypeMap[modelType] {
		return nil, nil, fmt.Errorf("model '%s' does not support model type '%s'", modelName, modelType)
	}

	switch modelType {
	case "chat":
		url := fmt.Sprintf("%s%s", provider.URL, provider.URLSuffix.Chat)
		return &url, nil, nil
	case "async_chat":
		chatUrl := fmt.Sprintf("%s%s", provider.URL, provider.URLSuffix.AsyncChat)
		resultUrl := fmt.Sprintf("%s%s", provider.URL, provider.URLSuffix.AsyncResult)
		return &chatUrl, &resultUrl, nil
	case "embedding":
		url := fmt.Sprintf("%s%s", provider.URL, provider.URLSuffix.Embedding)
		return &url, nil, nil
	case "rerank":
		url := fmt.Sprintf("%s%s", provider.URL, provider.URLSuffix.Rerank)
		return &url, nil, nil
	default:
		return nil, nil, fmt.Errorf("model '%s' does not support model type '%s'", modelName, modelType)
	}
}

// 4. Search specific model information with filtering by max_tokens or type
func (pm *ProviderManager) SearchModelInfo(providerName, modelName string, filterBy string, filterValue interface{}) ModelResponse {
	resp := ModelResponse{
		Code:    0,
		Data:    []map[string]interface{}{},
		Message: "success",
	}

	provider := pm.FindProvider(providerName)
	if provider == nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("Provider '%s' not found", providerName)
		return resp
	}

	model := pm.findModel(provider, modelName)
	if model == nil {
		resp.Code = 404
		resp.Message = fmt.Sprintf("Model '%s' not found in provider '%s'", modelName, providerName)
		return resp
	}

	// Apply filters
	matchFilter := true
	if filterBy != "" && filterValue != nil {
		switch filterBy {
		case "max_tokens":
			if maxVal, ok := filterValue.(int); ok {
				if *model.MaxTokens < maxVal {
					matchFilter = false
					resp.Code = 400
					resp.Message = fmt.Sprintf("Model does not meet filter criteria: max_tokens (%d) < %d",
						model.MaxTokens, maxVal)
				}
			}
		case "type":
			if typeVal, ok := filterValue.(string); ok {
				if !containsModelType(model.ModelTypes, typeVal) {
					matchFilter = false
					resp.Code = 400
					resp.Message = fmt.Sprintf("Model does not meet filter criteria: type '%s' not found", typeVal)
				}
			}
		}
	}

	if matchFilter {
		modelData := map[string]interface{}{
			"name":        model.Name,
			"max_tokens":  model.MaxTokens,
			"model_types": model.ModelTypes,
			//"features":    getFeaturesMap(model.Features),
		}

		if filterBy != "" && filterValue != nil {
			modelData["filter_applied"] = map[string]interface{}{
				"field": filterBy,
				"value": filterValue,
			}
		}

		resp.Data = append(resp.Data, modelData)
	}

	return resp
}

// 5. Display models with specific features
func (pm *ProviderManager) SearchByFeature(featureType string) ModelResponse {
	resp := ModelResponse{
		Code:    0,
		Data:    []map[string]interface{}{},
		Message: "success",
	}

	//for _, provider := range pm.Providers {
	//	for _, model := range provider.Models {
	//		if modelHasFeature(model.Features, featureType) {
	//			modelData := map[string]interface{}{
	//				"provider":    provider.Name,
	//				"name":        model.Name,
	//				"max_tokens":  model.MaxTokens,
	//				"model_types": model.ModelTypes,
	//				"features":    getFeaturesMap(model.Features),
	//			}
	//			resp.Data = append(resp.Data, modelData)
	//		}
	//	}
	//}

	if len(resp.Data) == 0 {
		resp.Code = 404
		resp.Message = fmt.Sprintf("No models found with feature '%s'", featureType)
	}

	return resp
}

// 6. Display models with specific type
func (pm *ProviderManager) SearchByType(modelType string) ModelResponse {
	resp := ModelResponse{
		Code:    0,
		Data:    []map[string]interface{}{},
		Message: "success",
	}

	for _, provider := range pm.Providers {
		for _, model := range provider.Models {
			if containsModelType(model.ModelTypes, modelType) {
				modelData := map[string]interface{}{
					"provider":    provider.Name,
					"name":        model.Name,
					"max_tokens":  model.MaxTokens,
					"model_types": model.ModelTypes,
					//"features":    getFeaturesMap(model.Features),
				}
				resp.Data = append(resp.Data, modelData)
			}
		}
	}

	if len(resp.Data) == 0 {
		resp.Code = 404
		resp.Message = fmt.Sprintf("No models found with type '%s'", modelType)
	}

	return resp
}

func GetFeatures(model *Model) []string {
	var features []string
	if model.Thinking != nil {
		features = append(features, "thinking")
	}
	return features
}

func ConvertToFeaturesMap(model *Model) map[string]interface{} {
	featuresMap := make(map[string]interface{})
	if model.Thinking != nil {
		thinkingMap := map[string]interface{}{
			"default_value":   model.Thinking.DefaultValue,
			"clear_reasoning": model.Thinking.ClearThinking,
		}
		featuresMap["thinking"] = thinkingMap
	}
	return featuresMap
}

// Helper: Get features map for response
func getFeaturesMap(features Features) map[string]interface{} {
	featuresMap := make(map[string]interface{})

	if features.Multimodal != nil && features.Multimodal.Enabled {
		multimodalMap := map[string]interface{}{
			"enabled":           features.Multimodal.Enabled,
			"input_modalities":  features.Multimodal.InputModalities,
			"output_modalities": features.Multimodal.OutputModalities,
		}
		featuresMap["multimodal"] = multimodalMap
	}

	if features.Reasoning != nil {
		reasoningMap := make(map[string]interface{})
		switch features.Reasoning.RawType {
		case "simple":
			if features.Reasoning.Simple != nil {
				reasoningMap["type"] = "simple"
				reasoningMap["enabled"] = features.Reasoning.Simple.Enabled
				reasoningMap["default"] = features.Reasoning.Simple.Default
			}
		case "budget":
			if features.Reasoning.Budget != nil {
				reasoningMap["type"] = "budget"
				reasoningMap["enabled"] = features.Reasoning.Budget.Enabled
				reasoningMap["default_tokens"] = features.Reasoning.Budget.DefaultTokens
				reasoningMap["token_range"] = map[string]int{
					"min": features.Reasoning.Budget.TokenRange.Min,
					"max": features.Reasoning.Budget.TokenRange.Max,
				}
			}
		case "effort":
			if features.Reasoning.Effort != nil {
				reasoningMap["type"] = "effort"
				reasoningMap["enabled"] = features.Reasoning.Effort.Enabled
				reasoningMap["default"] = features.Reasoning.Effort.Default
				reasoningMap["options"] = features.Reasoning.Effort.Options
			}
		}
		featuresMap["reasoning"] = reasoningMap
	}

	return featuresMap
}

// Helper: Check if model has a specific feature
func modelHasFeature(features Features, featureType string) bool {
	switch strings.ToLower(featureType) {
	case "multimodal":
		return features.Multimodal != nil && features.Multimodal.Enabled
	case "reasoning":
		return features.Reasoning != nil
	case "reasoning_simple":
		return features.Reasoning != nil && features.Reasoning.RawType == "simple"
	case "reasoning_budget":
		return features.Reasoning != nil && features.Reasoning.RawType == "budget"
	case "reasoning_effort":
		return features.Reasoning != nil && features.Reasoning.RawType == "effort"
	default:
		return false
	}
}

// findRepoRoot walks up from CWD until it finds the repo root (marked by
// conf/all_models.json).  This makes tests work regardless of the Go test
// binary's CWD (which is set to the package directory by go test).
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir != "/" && dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "conf", "all_models.json")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return "."
}

// Helper: Find provider by name
func (pm *ProviderManager) FindProvider(name string) *Provider {
	for i := range pm.Providers {
		if strings.EqualFold(pm.Providers[i].Name, name) {
			return &pm.Providers[i]
		}
	}
	return nil
}

// Helper: Find model by name
func (pm *ProviderManager) findModel(provider *Provider, modelName string) *Model {
	for i := range provider.Models {
		if strings.EqualFold(provider.Models[i].Name, modelName) {
			return provider.Models[i]
		}
	}
	return nil
}

// Helper: Check if model types contains target
func containsModelType(types []string, target string) bool {
	for _, t := range types {
		if strings.EqualFold(t, target) {
			return true
		}
	}
	return false
}
