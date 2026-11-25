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

package pipeline

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

type detailRecordingSink struct {
	recordingSink
	got []struct {
		component string
		unit      string
		current   uint32
		total     uint32
	}
}

func (r *detailRecordingSink) OnProgressDetail(_ context.Context, component, unit string, current, total uint32) {
	r.got = append(r.got, struct {
		component string
		unit      string
		current   uint32
		total     uint32
	}{component, unit, current, total})
}

// TestComponentDetailCallbackForwards verifies the pipeline builds a callback
// that routes a component's structured detail to a sink implementing the
// optional detailProgressSink interface.
func TestComponentDetailCallbackForwards(t *testing.T) {
	sink := &detailRecordingSink{}
	p := &Pipeline{sink: sink}
	cb := p.componentDetailCallback(context.Background())
	if cb == nil {
		t.Fatal("expected a detail callback for a sink implementing OnProgressDetail")
	}
	cb("Tokenizer", runtime.ProgressDetail{Unit: "chunk", Current: 257, Total: 3000})
	if len(sink.got) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(sink.got))
	}
	if g := sink.got[0]; g.component != "Tokenizer" || g.unit != "chunk" || g.current != 257 || g.total != 3000 {
		t.Errorf("forwarded detail = %+v, want Tokenizer/chunk/257/3000", g)
	}
}

// TestComponentDetailCallbackNilWhenUnsupported is the adversarial fixture: a
// sink without OnProgressDetail must yield a nil callback (no false channel).
func TestComponentDetailCallbackNilWhenUnsupported(t *testing.T) {
	p := &Pipeline{sink: &recordingSink{}}
	if cb := p.componentDetailCallback(context.Background()); cb != nil {
		t.Error("expected nil detail callback for a sink that does not implement OnProgressDetail")
	}
}
