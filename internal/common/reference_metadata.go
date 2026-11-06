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
// optional config. Request values take precedence over config values, and the
// top-level request keys include_metadata / metadata_fields take precedence over
// the nested reference_metadata {include, fields}.
//
// The returned fields are nil, meaning every field, when the selection is
// absent, empty or not a list of strings.
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
		// Top-level request keys (override reference_metadata)
		if v, ok := requestPayload["include_metadata"]; ok {
			resolved["include"] = v
		}
		if v, ok := requestPayload["metadata_fields"]; ok {
			resolved["fields"] = v
		}
	}

	include, _ := resolved["include"].(bool)

	var fields []string
	switch raw := resolved["fields"].(type) {
	case []string:
		fields = append(fields, raw...)
	case []interface{}:
		for _, f := range raw {
			if s, ok := f.(string); ok {
				fields = append(fields, s)
			}
		}
	}
	return include, fields
}
