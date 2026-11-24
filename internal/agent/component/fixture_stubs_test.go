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

package component_test

import (
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
)

func TestCompileRejectsAnswerComponent(t *testing.T) {
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"begin": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"answer"},
			},
			"answer": {
				Obj:      canvas.CanvasComponentObj{ComponentName: "Answer"},
				Upstream: []string{"begin"},
			},
		},
	}

	_, err := canvas.Compile(t.Context(), c)
	if err == nil {
		t.Fatal("Compile accepted unsupported Answer component")
	}
	if !strings.Contains(err.Error(), `unknown component "Answer"`) {
		t.Fatalf("Compile error = %q, want unknown Answer component", err)
	}
}
