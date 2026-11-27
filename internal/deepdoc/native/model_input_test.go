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

import "testing"

func TestModelInputValidate(t *testing.T) {
	// Happy path: data length matches shape product.
	good := ModelInput{Data: make([]float32, 3*2*2), Shape: []int64{1, 3, 2, 2}}
	if err := good.validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	// Adversarial: shape product differs from data length -> must error.
	bad := ModelInput{Data: make([]float32, 3*2*2), Shape: []int64{1, 3, 2, 3}}
	if err := bad.validate(); err == nil {
		t.Fatal("mismatched data/shape accepted; expected error")
	}

	// Adversarial (falsifies "derive shape from len"): 960x640 and 640x960
	// have identical element counts but are DIFFERENT shapes. validate() only
	// checks the count; it must accept both, proving the shape is carried
	// explicitly in ModelInput and not reconstructed from len(Data).
	a := ModelInput{Data: make([]float32, 3*960*640), Shape: []int64{1, 3, 960, 640}}
	b := ModelInput{Data: make([]float32, 3*640*960), Shape: []int64{1, 3, 640, 960}}
	if len(a.Data) != len(b.Data) {
		t.Fatal("test setup: shapes must have equal element counts")
	}
	if err := a.validate(); err != nil {
		t.Fatalf("shape 960x640 rejected: %v", err)
	}
	if err := b.validate(); err != nil {
		t.Fatalf("shape 640x960 rejected: %v", err)
	}

	// Adversarial: non-positive dimension must be rejected.
	zero := ModelInput{Data: nil, Shape: []int64{1, 3, 0, 4}}
	if err := zero.validate(); err == nil {
		t.Fatal("zero dimension accepted; expected error")
	}

	// Adversarial: empty shape must be rejected.
	empty := ModelInput{Data: make([]float32, 4), Shape: nil}
	if err := empty.validate(); err == nil {
		t.Fatal("empty shape accepted; expected error")
	}
}

// BenchmarkModelInputValidate guards the per-Run validation cost. Run() calls
// validate() on every inference, so this must stay cheap (a few ns).
func BenchmarkModelInputValidate(b *testing.B) {
	in := ModelInput{Data: make([]float32, 3*1024*1024), Shape: []int64{1, 3, 1024, 1024}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := in.validate(); err != nil {
			b.Fatal(err)
		}
	}
}
