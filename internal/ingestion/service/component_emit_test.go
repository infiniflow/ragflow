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

	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
)

// TestProgressSinkOnComponentProgressEmitsLifecycle verifies that a component
// lifecycle event is forwarded to the installed emitter after being recorded.
// Adversarial: phase and message must pass through verbatim; a sink emitting
// nothing (or wrong phase) fails here.
func TestProgressSinkOnComponentProgressEmitsLifecycle(t *testing.T) {
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	type lifecycleRec struct {
		component string
		phase     uint8
		message   string
	}
	var got []lifecycleRec
	sink.lifecycleEmitter = func(component string, phase uint8, message string) {
		got = append(got, lifecycleRec{component, phase, message})
	}

	sink.OnComponentProgress(context.Background(), pipeline.ProgressEvent{
		TaskID: "task-1", Component: "Parser", Phase: 2, Message: "boom",
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 lifecycle emit, got %d", len(got))
	}
	if g := got[0]; g.component != "Parser" || g.phase != 2 || g.message != "boom" {
		t.Errorf("lifecycle emit = %+v, want Parser/2/boom", g)
	}
}
