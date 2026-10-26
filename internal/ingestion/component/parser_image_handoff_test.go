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

package component

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

// TestImageHandoffAlwaysEmitsJSON pins the #16835 contract: the Parser's image
// branch hands the Tokenizer a JSON payload regardless of what the user's
// setups["image"]["output_format"] says. The image parser has no text payload
// to fall back on, so the emitted format is not a user choice.
//
// The failure mode is silent: a "text" handoff routes the Tokenizer to its
// text branch, which reads the top-level text field the image parser never
// fills, and the document indexes zero chunks while the pipeline reports
// success.
func TestImageHandoffAlwaysEmitsJSON(t *testing.T) {
	origResolver := resolveTenantModelByType
	t.Cleanup(func() { resolveTenantModelByType = origResolver })

	drv := &imagePromptCaptureDriver{}
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return drv, "img-model", &modelModule.APIConfig{}, 0, nil
	}

	setups := defaultSetups()
	setups["image"] = schema.ParserSetup{
		"parse_method":  "ocr",
		"suffix":        []any{"png"},
		"output_format": "text",
	}

	c := &ParserComponent{setups: setups, enableVisionEnhancement: true}
	out, err := c.Invoke(t.Context(), dao.DB, map[string]any{
		"binary":    picturePNG(t),
		"name":      "Screenshot.png",
		"file_type": "png",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["output_format"]; got != "json" {
		t.Fatalf("output_format = %v, want json", got)
	}

	items, ok := out["json"].([]parser.DocItem)
	if !ok || len(items) != 1 {
		t.Fatalf("json = %T/%d, want one image item", out["json"], len(items))
	}
	item := items[0].ToMap()
	if item["doc_type_kwd"] != "image" {
		t.Fatalf("doc_type_kwd = %v, want image", item["doc_type_kwd"])
	}
	text, _ := item["text"].(string)
	if text == "" {
		t.Fatal("image item carries no text; the vision answer was lost")
	}

	// The reported symptom is measured at the Tokenizer boundary, so assert the
	// handoff there rather than only on the Parser's own map.
	upstream, err := decodeTokenizerFromUpstream(out)
	if err != nil {
		t.Fatalf("decodeTokenizerFromUpstream: %v", err)
	}
	if got := upstream.OutputFormat; got != schema.PayloadFormatJSON {
		t.Fatalf("tokenizer output_format = %q, want %q", got, schema.PayloadFormatJSON)
	}
	chunks := chunksFromTokenizerUpstream(upstream)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0].Text != text {
		t.Errorf("chunk text = %q, want %q", chunks[0].Text, text)
	}
}
