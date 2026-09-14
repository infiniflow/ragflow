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

package common

import (
	"encoding/json"
	"fmt"
	"ragflow/internal/entity"
	"strconv"
	"strings"
)

func FormatSystemSetting(setting entity.SystemSettings) map[string]interface{} {
	return map[string]interface{}{
		"data_type":    setting.DataType,
		"name":         setting.Name,
		"setting_type": "config",
		"value":        setting.Value,
	}
}

func FormatSystemSettings(settings []entity.SystemSettings) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(settings))
	for _, setting := range settings {
		result = append(result, FormatSystemSetting(setting))
	}
	return result
}

func ValidateSystemSettingValue(setting entity.SystemSettings, value string) error {
	dataType := strings.ToLower(setting.DataType)
	switch dataType {
	case "string":
		return nil
	case "integer", "int":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid integer value for %s: %s", setting.Name, value)
		}
	case "bool", "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("invalid bool value for %s: expected true or false", setting.Name)
		}
	case "json":
		if !json.Valid([]byte(value)) {
			return fmt.Errorf("invalid JSON value for %s", setting.Name)
		}
	default:
		return fmt.Errorf("unsupported data type for %s: %s", setting.Name, setting.DataType)
	}
	return nil
}

func InferSystemSettingDataType(name string) string {
	if strings.HasPrefix(name, "sandbox.") {
		return "json"
	}
	if strings.HasSuffix(name, ".enabled") {
		return "bool"
	}
	return "string"
}
