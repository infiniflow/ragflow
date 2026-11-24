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

package tool

import (
	"fmt"
	"strings"
)

// ValidateSearchFilters checks filter shapes while allowing unfinished draft rows.
func ValidateSearchFilters(params map[string]any, draft bool) error {
	for _, field := range []string{"include_domains", "exclude_domains", "site_include", "site_exclude", "country_include", "language_include"} {
		raw, present := params[field]
		if !present || raw == nil {
			continue
		}
		var values []string
		switch entries := raw.(type) {
		case []string:
			values = entries
		case []any:
			for _, entry := range entries {
				value, ok := entry.(string)
				if !ok {
					return fmt.Errorf("%s must contain strings", field)
				}
				values = append(values, value)
			}
		default:
			return fmt.Errorf("%s must be an array", field)
		}
		if !draft {
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("%s does not support empty entries", field)
				}
			}
		}
	}
	return nil
}
