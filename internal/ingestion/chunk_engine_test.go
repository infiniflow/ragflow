//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ingestion

import (
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/chunk"
)

// The full DSL example from chunk_engine.go L22-L95.
var mediaAwareDSL = `{
  "version": "1.0",
  "name": "media_aware_chunking",
  "description": "Disable overlap when encountering image/video URLs",
  
  "pipeline": [
    {
      "stage": "preprocess",
      "normalize_newlines": true,
      "strip_whitespace": true,
      "remove_empty_lines": true
    },
    
    {
      "stage": "split",
      "strategy": "sentence",
      "params": {
        "boundaries": [". ", "! ", "? ", "\n"],
        "keep_separators": true
      }
    },
    
    {
      "stage": "postprocess",
      "merge": {
        "target_size": 500,
        "strategy": "greedy"
      },
      "overlap": {
        "enabled": true,
        "unit": "char",
        "mode": "if_only",
        "conditions": [
          {
            "name": "Contains media URL",
            "if": "has_media_url = true",
            "then": {"size": 0}
          },
          {
            "name": "Contains image URL",
            "if": "has_image_url = true",
            "then": {"size": 0}
          },
          {
            "name": "Contains video URL", 
            "if": "has_video_url = true",
            "then": {"size": 0}
          },
          {
            "name": "Normal English long sentence",
            "if": "language = 'en' AND length > 50 AND has_media_url = false",
            "then": {"size": 1, "unit": "sentence"}
          },
          {
            "name": "Normal English short sentence",
            "if": "language = 'en' AND length <= 50 AND has_media_url = false",
            "then": {"size": 30}
          }
        ],
        "default": {"size": 50}
      },
      "filter": {
        "min_length": 10,
        "max_length": 1200
      },
      "add_metadata": {
        "include_index": true,
        "custom_fields": {
          "has_media_url": "auto_detect"
        }
      }
    }
  ]
}`

var minimalDSL = `{
  "pipeline": [
    {"stage": "preprocess", "normalize_newlines": true},
    {"stage": "split", "strategy": "sentence", "params": {"boundaries": ["\n"], "keep_separators": false}},
    {"stage": "postprocess", "filter": {"min_length": 1}}
  ]
}`

// ---------------------------------------------------------------------------
// Plan success tests
// ---------------------------------------------------------------------------

func TestPlan_FullDSL(t *testing.T) {
	engine := NewChunkEngine()
	plan, err := engine.Plan(&mediaAwareDSL)
	if err != nil {
		t.Fatalf("Plan(mediaAwareDSL) unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("Plan returned nil")
	}

	// Must have exactly 3 operators
	if len(plan.Operators) != 3 {
		t.Fatalf("expected 3 operators, got %d", len(plan.Operators))
	}

	// Verify operator types in order
	expectedTypes := []string{
		"*chunk.PreprocessOperator",
		"*chunk.SplitOperator",
		"*chunk.PostprocessOperator",
	}
	for i, op := range plan.Operators {
		typ := fmt.Sprintf("%T", op)
		if typ != expectedTypes[i] {
			t.Errorf("operator[%d]: expected %s, got %s", i, expectedTypes[i], typ)
		}
	}

	// Verify operators implement Operator interface
	for i, op := range plan.Operators {
		var iface chunk.Operator = op
		_ = iface // compile-time check
		if op == nil {
			t.Errorf("operator[%d] is nil", i)
		}
	}
}

func TestPlan_MinimalDSL(t *testing.T) {
	engine := NewChunkEngine()
	plan, err := engine.Plan(&minimalDSL)
	if err != nil {
		t.Fatalf("Plan(minimalDSL) unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("Plan returned nil")
	}
	if len(plan.Operators) != 3 {
		t.Fatalf("expected 3 operators, got %d", len(plan.Operators))
	}
}

// ---------------------------------------------------------------------------
// Plan error tests
// ---------------------------------------------------------------------------

func TestPlan_InvalidJSON(t *testing.T) {
	engine := NewChunkEngine()
	invalid := `{bad json}`
	plan, err := engine.Plan(&invalid)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if plan != nil {
		t.Fatal("expected nil plan on error")
	}
}

func TestPlan_UnknownStage(t *testing.T) {
	engine := NewChunkEngine()
	dsl := `{"pipeline": [{"stage": "unknown_stage"}]}`
	plan, err := engine.Plan(&dsl)
	if err == nil {
		t.Fatal("expected error for unknown stage, got nil")
	}
	if !strings.Contains(err.Error(), "unknown_stage") {
		t.Errorf("error should mention unknown stage, got: %v", err)
	}
	if plan != nil {
		t.Fatal("expected nil plan on error")
	}
}

func TestPlan_EmptyPipeline(t *testing.T) {
	engine := NewChunkEngine()
	dsl := `{"pipeline": []}`
	plan, err := engine.Plan(&dsl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("plan should not be nil")
	}
	if len(plan.Operators) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(plan.Operators))
	}
}

func TestPlan_MissingPipeline(t *testing.T) {
	engine := NewChunkEngine()
	dsl := `{"version": "1.0"}`
	plan, err := engine.Plan(&dsl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("plan should not be nil")
	}
	if len(plan.Operators) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(plan.Operators))
	}
}

// ---------------------------------------------------------------------------
// Plan + Execute integration test
// ---------------------------------------------------------------------------

func TestPlan_Execute_FullPipeline(t *testing.T) {
	engine := NewChunkEngine()
	plan, err := engine.Plan(&mediaAwareDSL)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	inputText := `这是第一句话。这是第二句话！这是第三句话？\n这是第四句话。`
	ctx, err := engine.Execute(plan, inputText)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if ctx == nil {
		t.Fatal("Execute returned nil context")
	}
	if len(ctx.Chunks) == 0 {
		t.Fatal("expected at least one chunk after execution")
	}
	for i, c := range ctx.Chunks {
		if c.Content == "" {
			t.Errorf("chunk[%d] has empty content", i)
		}
		if c.Metadata == nil {
			t.Errorf("chunk[%d] has nil metadata", i)
		}
	}
}

func TestPlan_Execute_MinimalPipeline(t *testing.T) {
	engine := NewChunkEngine()
	plan, err := engine.Plan(&minimalDSL)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	inputText := "Hello world.\nGoodbye world."
	ctx, err := engine.Execute(plan, inputText)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if ctx == nil {
		t.Fatal("Execute returned nil context")
	}
	if len(ctx.Chunks) == 0 {
		t.Fatal("expected at least one chunk after execution")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestPlan_Explain(t *testing.T) {
	engine := NewChunkEngine()
	plan, err := engine.Plan(&mediaAwareDSL)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	explanation, err := engine.Explain(plan)
	if err != nil {
		t.Fatalf("Explain error: %v", err)
	}
	if !strings.Contains(explanation, "Chunk Pipeline Plan") {
		t.Errorf("explanation should contain header, got: %s", explanation)
	}
	if !strings.Contains(explanation, "PreprocessOperator") {
		t.Errorf("explanation should mention PreprocessOperator, got: %s", explanation)
	}
	if !strings.Contains(explanation, "SplitOperator") {
		t.Errorf("explanation should mention SplitOperator, got: %s", explanation)
	}
	if !strings.Contains(explanation, "PostprocessOperator") {
		t.Errorf("explanation should mention PostprocessOperator, got: %s", explanation)
	}
}

func TestPlan_ReuseEngine(t *testing.T) {
	engine := NewChunkEngine()

	// First plan
	plan1, err := engine.Plan(&mediaAwareDSL)
	if err != nil {
		t.Fatalf("first Plan error: %v", err)
	}

	// Second plan from the same engine
	plan2, err := engine.Plan(&minimalDSL)
	if err != nil {
		t.Fatalf("second Plan error: %v", err)
	}

	if len(plan1.Operators) != len(plan2.Operators) {
		t.Errorf("plan1 has %d operators, plan2 has %d", len(plan1.Operators), len(plan2.Operators))
	}
}

// Benchmark
func BenchmarkPlan_FullDSL(b *testing.B) {
	engine := NewChunkEngine()
	dsl := mediaAwareDSL
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Plan(&dsl)
		if err != nil {
			b.Fatalf("Plan error: %v", err)
		}
	}
}

func BenchmarkPlan_Execute_FullDSL(b *testing.B) {
	engine := NewChunkEngine()
	dsl := mediaAwareDSL
	plan, err := engine.Plan(&dsl)
	if err != nil {
		b.Fatalf("Plan error: %v", err)
	}
	inputText := strings.Repeat("这是第一句话。这是第二句话！这是第三句话？\n", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Execute(plan, inputText)
		if err != nil {
			b.Fatalf("Execute error: %v", err)
		}
	}
}
