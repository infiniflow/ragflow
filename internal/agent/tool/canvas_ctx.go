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

// canvasDatasetID returns the first explicit dataset id, else the canvas sys
// dataset_id.
func canvasDatasetID(ctx context.Context, explicit []string) string {
	if len(explicit) > 0 {
		return explicit[0]
	}
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return ""
	}
	if id, _ := state.Sys["dataset_id"].(string); id != "" {
		return id
	}
	return ""
}
