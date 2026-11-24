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

package service

import (
	"context"
	"testing"

	servicepkg "ragflow/internal/service"
)

type detailRecord struct {
	component string
	unit      string
	current   uint32
	total     uint32
}

// TestProgressSinkOnProgressDetailForwards verifies the sink forwards every
// detail report to the installed emitter, including boundary values.
func TestProgressSinkOnProgressDetailForwards(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	var got []detailRecord
	sink.detailEmitter = func(component, unit string, current, total uint32) {
		got = append(got, detailRecord{component, unit, current, total})
	}

	sink.OnProgressDetail(context.Background(), "Tokenizer", "chunk", 257, 3000)
	sink.OnProgressDetail(context.Background(), "Parser", "page", 0, 0)

	if len(got) != 2 {
		t.Fatalf("expected 2 emitted details, got %d", len(got))
	}
	if got[0] != (detailRecord{"Tokenizer", "chunk", 257, 3000}) {
		t.Errorf("detail 0 = %+v, want Tokenizer/chunk/257/3000", got[0])
	}
	if got[1] != (detailRecord{"Parser", "page", 0, 0}) {
		t.Errorf("detail 1 = %+v, want Parser/page/0/0", got[1])
	}
}

// TestProgressSinkOnProgressDetailNoEmitter verifies no-op safety when no
// emitter is installed (headless / OSS without a sink).
func TestProgressSinkOnProgressDetailNoEmitter(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
	// no emitter set — must not panic
	sink.OnProgressDetail(context.Background(), "Tokenizer", "chunk", 1, 1)
}
