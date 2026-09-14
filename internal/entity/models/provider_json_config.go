//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ProviderJSONConfigValue reads string fields from a tenant provider api_key JSON blob.
// keys are tried in order; env-style key names in the JSON are supported as fallbacks.
func ProviderJSONConfigValue(apiKey string, keys ...string) string {
	if strings.TrimSpace(apiKey) == "" {
		return ""
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(apiKey), &config); err != nil {
		return ""
	}
	if nested, ok := config["api_key"].(map[string]any); ok {
		config = nested
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
