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

	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// TestVisionEnhancement_SkipsEmptyJSON pins the gate equivalence: a not-produced
// (zero) ParseResult is skipped by the vision enhancement pass and returned
// unchanged after the backend-description marker strip. Previously gated on
// OutputFormat != "json"; now gated on Err != nil || len(JSON) == 0.
func TestVisionEnhancement_SkipsEmptyJSON(t *testing.T) {
	res, handled, err := maybeDispatchVisionEnhancement(
		context.Background(), nil, utility.FileTypePDF,
		parser.ParseResult{}, map[string]any{}, visionSettings{}, progress.Window{},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if handled {
		t.Error("expected not handled for a zero (not-produced) result")
	}
	_ = res
}
