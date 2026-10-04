//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

func providerJSONConfigMap(apiKey string) map[string]any {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(trimmed), &config); err != nil {
		return nil
	}
	if nested, ok := config["api_key"].(map[string]any); ok {
		config = nested
	}
	return config
}

// ProviderJSONConfigValue reads string fields from a tenant provider api_key JSON blob.
// keys are tried in order.
func ProviderJSONConfigValue(apiKey string, keys ...string) string {
	config := providerJSONConfigMap(apiKey)
	if config == nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := config[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func ProviderJSONConfigValueFromAPIConfig(apiConfig *APIConfig, keys ...string) string {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return ""
	}
	return ProviderJSONConfigValue(*apiConfig.ApiKey, keys...)
}
