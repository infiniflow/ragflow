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
	"encoding/json"
	"fmt"
	"os"
)

// SessionBudget holds calibrated per-model session counts.
type SessionBudget struct {
	DLA, TSR, DET, REC int
}

// sessionBudgetFile is the JSON written by tools/scripts/calibrate_session_budget.py:
// {"models": {"dla": {"saturation": N}, "tsr": {...},
//
//	"ocr_det": {...}, "ocr_rec": {...}}}
type sessionBudgetFile struct {
	Models map[string]struct {
		Saturation int `json:"saturation"`
	} `json:"models"`
}

// LoadSessionBudget reads a calibration result. Missing or malformed files are
// errors so the caller can distinguish "no calibration" from "bad calibration".
func LoadSessionBudget(path string) (SessionBudget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionBudget{}, fmt.Errorf("read session budget %s: %w", path, err)
	}
	var f sessionBudgetFile
	if err := json.Unmarshal(data, &f); err != nil {
		return SessionBudget{}, fmt.Errorf("parse session budget %s: %w", path, err)
	}
	b := SessionBudget{
		DLA: f.Models["dla"].Saturation,
		TSR: f.Models["tsr"].Saturation,
		DET: f.Models["ocr_det"].Saturation,
		REC: f.Models["ocr_rec"].Saturation,
	}
	return b, nil
}

// TotalSessions is the sum of per-model session quotas; used as the inference
// gate K so the gate admits at most as many concurrent Runs as there are
// supplyable sessions.
func (b SessionBudget) TotalSessions() int {
	return b.DLA + b.TSR + b.DET + b.REC
}

// ApplySessionBudget installs calibrated idle-session caps on the model pools.
// A zero entry means "not calibrated" and is skipped. DLA and TSR share the
// fixed-shape pool (modelSessions), so its cap is the larger of the two.
func ApplySessionBudget(b SessionBudget) {
	if b.DLA > 0 || b.TSR > 0 {
		cap := b.DLA
		if b.TSR > cap {
			cap = b.TSR
		}
		modelSessions.SetMaxFree(cap)
	}
	if b.DET > 0 {
		detSessions.SetMaxFree(b.DET)
	}
	if b.REC > 0 {
		recSessions.SetMaxFree(b.REC)
	}
}

// snapshotPoolMaxFree captures current caps (test helper for restore).
func snapshotPoolMaxFree() map[string]int {
	return map[string]int{
		"dla": modelSessions.maxFree,
		"tsr": modelSessions.maxFree,
		"det": detSessions.maxFree,
		"rec": recSessions.maxFree,
	}
}

// restorePoolMaxFree returns a cleanup func restoring the captured caps.
func restorePoolMaxFree(saved map[string]int) func() {
	return func() {
		if v, ok := saved["dla"]; ok {
			modelSessions.SetMaxFree(v)
		}
		if v, ok := saved["det"]; ok {
			detSessions.SetMaxFree(v)
		}
		if v, ok := saved["rec"]; ok {
			recSessions.SetMaxFree(v)
		}
	}
}
