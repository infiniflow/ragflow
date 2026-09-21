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

package service

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRunProgressZeroTotal(t *testing.T) {
	r := newRunProgress()
	r.MarkDone("File")
	r.SetFrac("Parser:abc", 0.5)
	if got := r.Percent(); got != 0 {
		t.Fatalf("Percent without total = %v, want 0", got)
	}
}

func TestRunProgressDoneAndFrac(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	r.MarkDone("File")
	r.SetFrac("Parser:abc", 0.5)
	if got := r.Percent(); !almostEqual(got, 0.3) {
		t.Fatalf("Percent = %v, want 0.3 ((1 done + 0.5 frac)/5)", got)
	}
}

func TestRunProgressMarkDoneClearsFrac(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	r.SetFrac("Parser:abc", 0.5)
	r.MarkDone("Parser:abc")
	if got := r.Percent(); !almostEqual(got, 0.2) {
		t.Fatalf("Percent = %v, want 0.2 (frac must not double-count after done)", got)
	}
}

func TestRunProgressSetFracAfterDoneIgnored(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	r.MarkDone("Parser:abc")
	r.SetFrac("Parser:abc", 0.5)
	if got := r.Percent(); !almostEqual(got, 0.2) {
		t.Fatalf("Percent = %v, want 0.2 (late frac for done component ignored)", got)
	}
}

func TestRunProgressMonotonicOnFracRegression(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	r.SetFrac("Parser:abc", 0.9)
	if got := r.Percent(); !almostEqual(got, 0.18) {
		t.Fatalf("Percent = %v, want 0.18", got)
	}
	r.SetFrac("Parser:abc", 0.1)
	if got := r.Percent(); !almostEqual(got, 0.18) {
		t.Fatalf("Percent after regression = %v, want high-water 0.18", got)
	}
	r.SetFrac("Parser:abc", 0.95)
	if got := r.Percent(); !almostEqual(got, 0.19) {
		t.Fatalf("Percent after advance = %v, want 0.19", got)
	}
}

func TestRunProgressClampsFracAndPercent(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(2)
	r.SetFrac("a", -0.5)
	r.SetFrac("b", 1.5)
	if got := r.Percent(); !almostEqual(got, 0.5) {
		t.Fatalf("Percent = %v, want 0.5 (frac clamped to [0,1])", got)
	}
	r.MarkDone("a")
	r.MarkDone("b")
	r.MarkDone("c")
	if got := r.Percent(); got != 1 {
		t.Fatalf("Percent = %v, want 1 (clamped when done exceeds total)", got)
	}
}

func TestRunProgressIgnoresNaN(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	r.SetFrac("Parser:abc", 0.4)
	if got := r.Percent(); !almostEqual(got, 0.08) {
		t.Fatalf("Percent = %v, want 0.08", got)
	}

	r.SetFrac("Parser:abc", math.NaN())
	if got := r.Percent(); !almostEqual(got, 0.08) {
		t.Fatalf("Percent after NaN report = %v, want 0.08 (NaN must not poison the high-water mark)", got)
	}
}

func TestRunProgressIdempotentRepeatedReports(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(5)
	for i := 0; i < 3; i++ {
		r.MarkDone("File")
		r.SetFrac("Parser:abc", 0.5)
	}
	if got := r.Percent(); !almostEqual(got, 0.3) {
		t.Fatalf("Percent = %v, want 0.3 (repeated reports must be idempotent)", got)
	}
}

func TestRunProgressConcurrentReports(t *testing.T) {
	r := newRunProgress()
	r.SetTotal(100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("comp-%d", i)
			r.SetFrac(name, 0.5)
			r.MarkDone(name)
			r.Percent()
		}(i)
	}
	wg.Wait()
	if got := r.Percent(); got != 1 {
		t.Fatalf("Percent = %v, want 1 after all components done", got)
	}
}
