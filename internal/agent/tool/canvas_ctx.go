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

// CanvasTenantID is the historical name for the tenant scope resolver.
// Re-exported from runtime (single owner).
func CanvasTenantID(ctx context.Context) string { return runtime.TenantID(ctx) }

// CanvasDatasetIDs is the historical name for the dataset scope resolver.
// Re-exported from runtime (single owner).
func CanvasDatasetIDs(ctx context.Context, explicit []string) []string {
	return runtime.DatasetIDs(ctx, explicit)
}

// canvasTenantID is the package-internal form used by the canvas tools.
func canvasTenantID(ctx context.Context) string { return runtime.TenantID(ctx) }

// canvasDatasetIDs is the package-internal form used by the canvas tools.
func canvasDatasetIDs(ctx context.Context, explicit []string) []string {
	return runtime.DatasetIDs(ctx, explicit)
}
