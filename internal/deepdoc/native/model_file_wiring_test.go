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

//go:build cgo && integration

package native

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModelFileDetWiredToPipeline guards that the modelFileDet override seam is
// actually consumed by RunDet: overriding it to a bogus filename makes RunDet
// fail, proving the seam is wired into the detection pipeline rather than being
// dead. No new model is required — we reuse the existing MODEL_DIR (which holds
// the real det.ort) but point at a name that does not exist there.
//
// The wiring proof is the error itself: the real det.ort loads fine in every
// other integration test, so if modelFileDet were not wired (RunDet hardcoding
// "det.ort") this call would succeed (err == nil) and the test would fail. We
// assert failure rather than matching the filename in the error, because ORT's
// load error ("Error creating C session from file: Invalid fd was supplied: -1")
// does not echo the path.
func TestModelFileDetWiredToPipeline(t *testing.T) {
	skipIfNoModels(t)

	imgPath := filepath.Join("testdata", "page0.png")
	img, err := Decode(imgPath)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	const bogus = "does-not-exist-det.ort"
	saved := modelFileDet
	defer func() { modelFileDet = saved }()
	modelFileDet = bogus

	_, err = RunDet(t.Context(), os.Getenv("MODEL_DIR"), img)
	if err == nil {
		t.Fatalf("expected RunDet to fail loading the overridden (bogus) model file %q, got nil error — the modelFileDet seam may not be wired into RunDet", bogus)
	}
	t.Logf("RunDet failed as expected for bogus modelFileDet %q: %v", bogus, err)
}
