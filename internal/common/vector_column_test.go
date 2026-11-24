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

package common

import "testing"

func TestVectorColumnName(t *testing.T) {
	cases := []struct {
		dim  int
		want string
	}{
		{768, "q_768_vec"},
		{1, "q_1_vec"},
		{0, "q_0_vec"},
		{1536, "q_1536_vec"},
	}
	for _, c := range cases {
		if got := VectorColumnName(c.dim); got != c.want {
			t.Errorf("VectorColumnName(%d) = %q, want %q", c.dim, got, c.want)
		}
	}
	// Adversarial: the format must be exactly "q_<dim>_vec" with no extra
	// characters (e.g. not "q_768vector" or "q_768_vec_").
	if got := VectorColumnName(768); got != "q_768_vec" {
		t.Errorf("VectorColumnName(768) = %q, want exact %q", got, "q_768_vec")
	}
	// Negative dimension is nonsensical but must not panic; document the
	// formatting behavior so a future change cannot silently alter it.
	if got := VectorColumnName(-1); got != "q_-1_vec" {
		t.Errorf("VectorColumnName(-1) = %q, want %q", got, "q_-1_vec")
	}
}

func TestIsVectorColumn(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"q_768_vec", true},
		{"doc_id", false},
		{"q_768_vec_n", false}, // SereneDB normalization shadow column must NOT match
		{"x_768_vec", false},   // wrong prefix
		{"q_768_vex", false},   // wrong suffix
		{"q_vec", true},        // mirrors the historical HasPrefix/HasSuffix check
	}
	for _, c := range cases {
		if got := IsVectorColumn(c.key); got != c.want {
			t.Errorf("IsVectorColumn(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestVectorColumnDim(t *testing.T) {
	cases := []struct {
		key string
		dim int
		ok  bool
	}{
		{"q_768_vec", 768, true},
		{"q_1_vec", 1, true},
		{"doc_id", 0, false},
		{"q_768_vec_n", 0, false}, // shadow column: must NOT parse to 768
		{"q_abc_vec", 0, false},   // non-numeric dimension
		{"q_vec", 0, false},       // matches IsVectorColumn but has no dimension
	}
	for _, c := range cases {
		dim, ok := VectorColumnDim(c.key)
		if ok != c.ok || dim != c.dim {
			t.Errorf("VectorColumnDim(%q) = (%d, %v), want (%d, %v)", c.key, dim, ok, c.dim, c.ok)
		}
	}
}
