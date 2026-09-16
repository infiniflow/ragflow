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

// ResolveReferenceMetadata resolves metadata include/fields from request and
// optional config. Request values take precedence over config values.
// Supports legacy request keys: include_metadata / metadata_fields.
// Python equivalent: api/utils/reference_metadata_utils.py::resolve_reference_metadata_preferences()
func ResolveReferenceMetadata(requestPayload, configPayload map[string]interface{}) (bool, []string) {
	resolved := make(map[string]interface{})

	// Config reference_metadata
	if configPayload != nil {
		if cfg, ok := configPayload["reference_metadata"].(map[string]interface{}); ok {
			for k, v := range cfg {
				resolved[k] = v
			}
		}
	}

	// Request reference_metadata (overrides config)
	if requestPayload != nil {
		if req, ok := requestPayload["reference_metadata"].(map[string]interface{}); ok {
			for k, v := range req {
				resolved[k] = v
			}
		}
		// Legacy keys
		if v, ok := requestPayload["include_metadata"]; ok {
			resolved["include"] = v
		}
		if v, ok := requestPayload["metadata_fields"]; ok {
			resolved["fields"] = v
		}
	}

	include, _ := resolved["include"].(bool)

	rawFields, ok := resolved["fields"].([]interface{})
	if !ok {
		return include, nil
	}
	var fields []string
	for _, f := range rawFields {
		if s, ok := f.(string); ok {
			fields = append(fields, s)
		}
	}
	return include, fields
}
