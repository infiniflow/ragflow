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
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	ort "github.com/infiniflow/onnxruntime_go"
)

// TestDynamicShapeSameSession verifies that a single DynamicAdvancedSession
// can run two different input shapes and that its outputs match sessions
// created for each shape independently. Proves inShape can move to Run time.
func TestDynamicShapeSameSession(t *testing.T) {
	if err := InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}
	modelDir := os.Getenv("EE_MODEL_DIR")
	if modelDir == "" {
		modelDir = os.Getenv("MODEL_DIR")
	}
	if modelDir == "" {
		t.Skip("no model dir")
	}
	modelFile := os.Getenv("DET_MODEL_FILE")
	if modelFile == "" {
		modelFile = "det.ort"
	}
	modelPath := filepath.Join(modelDir, modelFile)
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("det model not found at %s: %v", modelPath, err)
	}

	const inName = "x"
	const outName = "sigmoid_0.tmp_0"

	run := func(sess *ort.DynamicAdvancedSession, h, w int64) ([]float32, error) {
		in := make([]float32, 3*h*w)
		for i := range in {
			in[i] = float32((i % 251) / 251.0)
		}
		inT, err := ort.NewTensor(ort.NewShape(1, 3, h, w), in)
		if err != nil {
			return nil, fmt.Errorf("tensor %dx%d: %w", h, w, err)
		}
		defer inT.Destroy()
		outs := []ort.Value{nil}
		if err := sess.RunWithOptions([]ort.Value{inT}, outs, nil); err != nil {
			return nil, fmt.Errorf("run %dx%d: %w", h, w, err)
		}
		outT, ok := outs[0].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected output type %T", outs[0])
		}
		defer outT.Destroy()
		return append([]float32(nil), outT.GetData()...), nil
	}

	shapes := [][2]int64{{320, 320}, {640, 640}}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		t.Fatalf("NewSessionOptions: %v", err)
	}
	defer opts.Destroy()

	shared, err := ort.NewDynamicAdvancedSession(modelPath, []string{inName}, []string{outName}, opts)
	if err != nil {
		t.Fatalf("create shared session: %v", err)
	}
	defer shared.Destroy()

	sharedOut := map[[2]int64][]float32{}
	for _, sh := range shapes {
		out, err := run(shared, sh[0], sh[1])
		if err != nil {
			t.Fatalf("shared session run %v: %v", sh, err)
		}
		sharedOut[sh] = out
		t.Logf("shared session ran shape %v OK, output len=%d", sh, len(out))
	}

	for _, sh := range shapes {
		ref, err := ort.NewDynamicAdvancedSession(modelPath, []string{inName}, []string{outName}, opts)
		if err != nil {
			t.Fatalf("create ref session %v: %v", sh, err)
		}
		out, err := run(ref, sh[0], sh[1])
		ref.Destroy()
		if err != nil {
			t.Fatalf("ref session run %v: %v", sh, err)
		}
		got := sharedOut[sh]
		if len(got) != len(out) {
			t.Fatalf("shape %v: len mismatch shared=%d ref=%d", sh, len(got), len(out))
		}
		maxDiff := 0.0
		for i := range got {
			d := math.Abs(float64(got[i] - out[i]))
			if d > maxDiff {
				maxDiff = d
			}
		}
		t.Logf("shape %v: shared vs ref maxDiff=%.6g", sh, maxDiff)
		if maxDiff > 1e-6 {
			t.Fatalf("shape %v: output differs, maxDiff=%g", sh, maxDiff)
		}
	}
	t.Log("PASS: one dynamic session produces identical outputs across shapes")
}
