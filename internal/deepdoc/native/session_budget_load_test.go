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

//go:build cgo

package native

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSessionBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_budget.ee.gpu.json")
	json := `{
  "baseline_vram_mb": 2800,
  "models": {
    "dla": {"peak_rps": 15.5, "saturation": 8, "session_mem_mb": 675},
    "tsr": {"peak_rps": 24.35, "saturation": 4, "session_mem_mb": 823},
    "ocr_det": {"peak_rps": 71.0, "saturation": 8, "session_mem_mb": 320},
    "ocr_rec": {"peak_rps": 80.0, "saturation": 4, "session_mem_mb": 268}
  }
}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadSessionBudget(path)
	if err != nil {
		t.Fatalf("LoadSessionBudget: %v", err)
	}
	if b.DLA != 8 || b.TSR != 4 || b.DET != 8 || b.REC != 4 {
		t.Fatalf("parsed budget = %+v", b)
	}
}

func TestLoadSessionBudgetMissingFile(t *testing.T) {
	_, err := LoadSessionBudget("/nonexistent/session_budget.ee.gpu.json")
	if err == nil {
		t.Fatal("missing file must return error")
	}
}

func TestLoadSessionBudgetMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSessionBudget(path)
	if err == nil {
		t.Fatal("malformed JSON must return error")
	}
}

func TestSessionBudgetTotalK(t *testing.T) {
	b := SessionBudget{DLA: 4, TSR: 2, DET: 8, REC: 4}
	if got := b.TotalSessions(); got != 18 {
		t.Fatalf("TotalSessions = %d, want 18", got)
	}
}
