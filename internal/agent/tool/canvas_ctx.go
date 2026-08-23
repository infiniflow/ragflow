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
	"context"

	"ragflow/internal/agent/runtime"
)

// canvasTenantID derives the tenant id from canvas state, falling back to
// user_id. Shared by agentic search and dataset-navigation tools.
func canvasTenantID(ctx context.Context) string {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return ""
	}
	if tenantID, _ := state.Sys["tenant_id"].(string); tenantID != "" {
		return tenantID
	}
	userID, _ := state.Sys["user_id"].(string)
	return userID
}

// canvasDatasetIDs returns the explicit dataset ids (all of them, preserving
// multi-KB sessions), else the canvas sys dataset_id as a single-element list.
func canvasDatasetIDs(ctx context.Context, explicit []string) []string {
	if len(explicit) > 0 {
		out := make([]string, 0, len(explicit))
		for _, id := range explicit {
			if id != "" {
				out = append(out, id)
			}
		}
		return out
	}
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return nil
	}
	if id, _ := state.Sys["dataset_id"].(string); id != "" {
		return []string{id}
	}
	return nil
}
