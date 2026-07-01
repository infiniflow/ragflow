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

package task

import (
	"testing"

	"ragflow/internal/entity"
)

func TestTaskHandler_Dispatch(t *testing.T) {
	tests := []struct {
		name       string
		taskType   string
		wantErr    bool
		wantPanic  bool
	}{
		{"memory", "memory", false, false},
		{"dataflow", "dataflow", false, false},
		{"dataflow with suffix", "dataflow_test", false, false},
		{"raptor", "raptor", false, false},
		{"graphrag", "graphrag", false, false},
		{"mindmap", "mindmap", false, false},
		{"evaluation", "evaluation", false, false},
		{"reembedding", "reembedding", false, false},
		{"clone", "clone", false, false},
		{"standard (empty task_type)", "", false, false},
		{"standard (unknown task_type)", "unknown_type", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &TaskContext{
				Task: entity.Task{
					ID:       "task-1",
					DocID:    "doc-1",
					TaskType: tt.taskType,
				},
				Doc: entity.Document{
					ID:           "doc-1",
					KbID:         "kb-1",
					ParserID:     "naive",
					ParserConfig: entity.JSONMap{},
				},
				KB: entity.Knowledgebase{
					ID:       "kb-1",
					TenantID: "tenant-1",
					EmbdID:   "embd-1",
				},
				Tenant: entity.Tenant{
					ID:    "tenant-1",
					LLMID: "gpt-4",
				},
			}

			handler := NewTaskHandler(ctx)
			err := handler.Handle()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
