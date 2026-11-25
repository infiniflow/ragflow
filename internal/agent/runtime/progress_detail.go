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

// ProgressDetail is a structured progress position reported by a component:
// unit (page/chunk/batch/entity), current and total. It refines the coarse
// 0..1 fraction with discrete, queryable positions for the ingestion log.
type ProgressDetail struct {
	Unit    string
	Current uint32
	Total   uint32
}

// ProgressDetailCallback receives a component's progress detail. component is
// the node id (cpnID), bound by the canvas framework via BindComponentDetail —
// the same attribution model as ProgressFractionCallback.
type ProgressDetailCallback func(component string, detail ProgressDetail)

type progressDetailCBKey struct{}
type componentDetailReporterKey struct{}

// WithProgressDetailCallback attaches a run-level detail sink to ctx. A nil
// callback keeps components observer-independent (headless runs pay nothing).
func WithProgressDetailCallback(ctx context.Context, cb ProgressDetailCallback) context.Context {
	return context.WithValue(ctx, progressDetailCBKey{}, cb)
}

// BindComponentDetail derives a context whose ReportProgressDetail calls carry
// the given component id. When no run-level callback is attached it returns ctx
// unchanged so unbound runs pay nothing.
func BindComponentDetail(ctx context.Context, component string) context.Context {
	cb, _ := ctx.Value(progressDetailCBKey{}).(ProgressDetailCallback)
	if cb == nil {
		return ctx
	}
	return context.WithValue(ctx, componentDetailReporterKey{}, func(d ProgressDetail) {
		cb(component, d)
	})
}

// ReportProgressDetail forwards a component's progress detail to the run-level
// sink. Components call it from their loops without knowing their node id; the
// framework-bound closure supplies attribution. No-op when no sink is attached.
func ReportProgressDetail(ctx context.Context, unit string, current, total uint32) {
	if ctx == nil {
		return
	}
	report, ok := ctx.Value(componentDetailReporterKey{}).(func(ProgressDetail))
	if !ok || report == nil {
		return
	}
	report(ProgressDetail{Unit: unit, Current: current, Total: total})
}
