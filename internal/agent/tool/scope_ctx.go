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

import "context"

// scopeCtxKey carries a conversation-level scope (tenant + dataset ids) so
// tools can resolve their search scope without a canvas state. This lets the
// same tools serve both the canvas agent runtime and the conversation-level
// smart-reasoning entrypoint.
type scopeCtxKey struct{}

type scopeValue struct {
	tenantID   string
	datasetIDs []string
}

// WithScope attaches an explicit tenant + dataset scope to ctx for tool calls.
// It takes precedence over canvas state in canvasTenantID / canvasDatasetIDs.
func WithScope(ctx context.Context, tenantID string, datasetIDs []string) context.Context {
	return context.WithValue(ctx, scopeCtxKey{}, scopeValue{
		tenantID:   tenantID,
		datasetIDs: datasetIDs,
	})
}

// scopeFromContext returns the injected scope, or nil.
func scopeFromContext(ctx context.Context) *scopeValue {
	if v, ok := ctx.Value(scopeCtxKey{}).(scopeValue); ok {
		return &v
	}
	return nil
}
