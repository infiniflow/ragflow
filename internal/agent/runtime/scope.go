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

package runtime

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
// It takes precedence over canvas state in TenantID / DatasetIDs resolution.
// The RAGFlow session (created in the UI) decides tenantID and datasetIDs, which
// are injected here and read back by the retrieval tools.
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

// TenantID resolves the tenant id for a tool call. Precedence: an explicit
// WithScope injection, else canvas state tenant_id, falling back to user_id.
func TenantID(ctx context.Context) string {
	if sc := scopeFromContext(ctx); sc != nil && sc.tenantID != "" {
		return sc.tenantID
	}
	state, _, err := GetStateFromContext[*CanvasState](ctx)
	if err != nil || state == nil {
		return ""
	}
	if tenantID, _ := state.Sys["tenant_id"].(string); tenantID != "" {
		return tenantID
	}
	userID, _ := state.Sys["user_id"].(string)
	return userID
}

// DatasetIDs resolves the dataset scope for a tool call. Precedence:
//
//  1. A trusted scope injected via WithScope (conversation-level, e.g. the
//     chat's bound KBs) is authoritative: any tool-supplied `explicit` ids are
//     INTERSECTED with it so a prompt-injected model cannot escalate to KBs
//     outside the current conversation. If explicit is empty, the injected
//     scope is used verbatim.
//  2. No injection (canvas runtime): explicit ids are authoritative (all of
//     them, preserving multi-KB sessions); else the canvas sys dataset_id.
func DatasetIDs(ctx context.Context, explicit []string) []string {
	explicit = nonEmptyStringSlice(explicit)
	if sc := scopeFromContext(ctx); sc != nil {
		trusted := nonEmptyStringSlice(sc.datasetIDs)
		if len(explicit) == 0 {
			return trusted
		}
		return intersectStringSlices(explicit, trusted)
	}
	if len(explicit) > 0 {
		return explicit
	}
	state, _, err := GetStateFromContext[*CanvasState](ctx)
	if err != nil || state == nil {
		return nil
	}
	if id, _ := state.Sys["dataset_id"].(string); id != "" {
		return []string{id}
	}
	return nil
}

// nonEmptyStringSlice drops empty strings while preserving order and duplicates.
func nonEmptyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, id := range in {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// intersectStringSlices returns the elements of a that also appear in b,
// preserving a's order and duplicates.
func intersectStringSlices(a, b []string) []string {
	allowed := make(map[string]struct{}, len(b))
	for _, id := range b {
		allowed[id] = struct{}{}
	}
	var out []string
	for _, id := range a {
		if _, ok := allowed[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
