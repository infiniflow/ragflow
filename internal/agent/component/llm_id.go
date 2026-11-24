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

package component

import "strings"

// splitCompositeLLMID extracts the provider driver and bare model id from the
// canvas llm_id convention:
//   - "model@provider"          -> ("model", "provider", true)
//   - "model@instance@provider" -> ("model", "provider", true)
//   - bare "model"              -> ("model", "", false)
//
// The split is right-anchored: the provider is the last segment, the instance
// (dropped here) the second-to-last, and everything further left is the model
// name — which may itself contain '@' (e.g. LM Studio quant-suffixed ids such
// as "text-embedding-nomic-embed-text-v1.5@q8_0"). Mirrors Python's
// split_model_name (rsplit("@", 2)) and parseModelName in
// internal/service/model_service.go.
func splitCompositeLLMID(s string) (modelName, driver string, hasDriver bool) {
	parts := strings.Split(strings.TrimSpace(s), "@")
	switch len(parts) {
	case 2:
		return parts[0], parts[1], true
	default:
		if len(parts) >= 3 {
			return strings.Join(parts[:len(parts)-2], "@"), parts[len(parts)-1], true
		}
		return s, "", false
	}
}
