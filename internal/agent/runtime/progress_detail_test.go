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

import (
	"context"
	"testing"
)

// TestReportProgressDetailBindsComponent verifies the framework binding: a
// component reports only unit/current/total and the bound component id is
// attached. Adversarial: two independently-bound contexts must not cross
// attribute their reports.
func TestReportProgressDetailBindsComponent(t *testing.T) {
	type got struct {
		component string
		detail    ProgressDetail
	}
	var events []got
	cb := func(component string, d ProgressDetail) {
		events = append(events, got{component, d})
	}
	ctxA := BindComponentDetail(WithProgressDetailCallback(context.Background(), cb), "Tokenizer")
	ctxB := BindComponentDetail(WithProgressDetailCallback(context.Background(), cb), "Parser")

	ReportProgressDetail(ctxA, "chunk", 257, 3000)
	ReportProgressDetail(ctxB, "page", 12, 100)

	if len(events) != 2 {
		t.Fatalf("expected 2 detail events, got %d", len(events))
	}
	if events[0].component != "Tokenizer" || events[0].detail.Unit != "chunk" ||
		events[0].detail.Current != 257 || events[0].detail.Total != 3000 {
		t.Errorf("event 0 misattributed/mismapped: %+v", events[0])
	}
	if events[1].component != "Parser" || events[1].detail.Unit != "page" ||
		events[1].detail.Current != 12 || events[1].detail.Total != 100 {
		t.Errorf("event 1 misattributed/mismapped: %+v", events[1])
	}
}

// TestReportProgressDetailNoSink verifies the no-op safety: without a bound
// callback (headless run), reporting must not panic or allocate events.
func TestReportProgressDetailNoSink(t *testing.T) {
	ctx := context.Background() // no callback, no binding
	ReportProgressDetail(ctx, "chunk", 1, 1)
	ReportProgressDetail(nil, "chunk", 1, 1) // nil ctx must not panic
}

// TestReportProgressDetailUnboundContext verifies that binding without a
// run-level callback is a no-op (framework pays nothing when no sink attached).
func TestReportProgressDetailUnboundContext(t *testing.T) {
	ctx := BindComponentDetail(context.Background(), "Tokenizer")
	// no callback attached — must not panic
	ReportProgressDetail(ctx, "chunk", 1, 1)
}
