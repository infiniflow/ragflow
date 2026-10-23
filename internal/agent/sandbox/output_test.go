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

package sandbox

import (
	"strings"
	"sync"
	"testing"
)

// TestOutputCapture_DrainsBeyondLimit checks that a stream keeps accepting
// every write — so a chaty child never blocks on a short write — while
// retaining no more than the shared budget allows.
func TestOutputCapture_DrainsBeyondLimit(t *testing.T) {
	budget := newOutputBudget(32)
	var output outputCapture
	output.budget = budget
	for range 100 {
		if n, err := output.Write([]byte(strings.Repeat("x", 1024))); err != nil || n != 1024 {
			t.Fatalf("Write = (%d, %v), want (1024, nil)", n, err)
		}
	}
	if output.buffer.Len() > 32 {
		t.Errorf("captured %d bytes, want at most 32", output.buffer.Len())
	}
	total, exceeded := budget.usage()
	if total != 102400 {
		t.Errorf("total = %d, want 102400", total)
	}
	if !exceeded {
		t.Error("usage() reported no overflow for 102400 bytes against a 32-byte budget")
	}
}

// TestOutputBudget_SharedAcrossStreams is the regression test for the
// budget being shared: stdout and stderr must draw from one allowance,
// so the process cannot retain twice the configured max_output_bytes by
// filling both streams.
func TestOutputBudget_SharedAcrossStreams(t *testing.T) {
	const budgetBytes = 100
	budget := newOutputBudget(budgetBytes)
	stdout := outputCapture{budget: budget}
	stderr := outputCapture{budget: budget}

	chunk := []byte(strings.Repeat("x", 80))
	for range 10 {
		if _, err := stdout.Write(chunk); err != nil {
			t.Fatal(err)
		}
		if _, err := stderr.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}

	if retained := stdout.buffer.Len() + stderr.buffer.Len(); retained > budgetBytes {
		t.Errorf("retained %d bytes across both streams, want at most %d: the budget is not shared",
			retained, budgetBytes)
	}
	total, exceeded := budget.usage()
	if total != 1600 {
		t.Errorf("total = %d, want 1600", total)
	}
	if !exceeded {
		t.Error("usage() reported no overflow for 1600 bytes against a 100-byte budget")
	}
}

// TestOutputBudget_WithinLimitRetainsEverything checks the accepted path:
// while the combined total stays at or under the budget every write is
// retained in full, so a successful result is never silently truncated.
func TestOutputBudget_WithinLimitRetainsEverything(t *testing.T) {
	const budgetBytes = 4096
	budget := newOutputBudget(budgetBytes)
	stdout := outputCapture{budget: budget}
	stderr := outputCapture{budget: budget}

	if _, err := stdout.Write([]byte(strings.Repeat("a", 2048))); err != nil {
		t.Fatal(err)
	}
	if _, err := stderr.Write([]byte(strings.Repeat("b", 2048))); err != nil {
		t.Fatal(err)
	}

	if stdout.buffer.Len() != 2048 || stderr.buffer.Len() != 2048 {
		t.Errorf("retained stdout=%d stderr=%d, want 2048 each",
			stdout.buffer.Len(), stderr.buffer.Len())
	}
	if _, exceeded := budget.usage(); exceeded {
		t.Error("usage() reported overflow for output exactly at the budget")
	}
}

// TestOutputBudget_Unlimited retains everything when no positive limit is
// configured, so deployments without max_output_bytes are unaffected.
func TestOutputBudget_Unlimited(t *testing.T) {
	for _, limit := range []int{0, -1} {
		budget := newOutputBudget(limit)
		var output outputCapture
		output.budget = budget
		for range 8 {
			if _, err := output.Write([]byte(strings.Repeat("y", 4096))); err != nil {
				t.Fatal(err)
			}
		}
		if got := output.buffer.Len(); got != 8*4096 {
			t.Errorf("limit %d: retained %d bytes, want %d", limit, got, 8*4096)
		}
		if _, exceeded := budget.usage(); exceeded {
			t.Errorf("limit %d: usage() reported overflow, want none", limit)
		}
	}
}

// TestOutputBudget_ConcurrentStreams exercises the same access pattern as
// os/exec and x/crypto/ssh: one writer goroutine per stream drawing on
// the shared budget, read only after both are joined. Intended to be run
// under -race.
func TestOutputBudget_ConcurrentStreams(t *testing.T) {
	const perStream = 1 << 20
	budget := newOutputBudget(64 << 10)
	stdout := outputCapture{budget: budget}
	stderr := outputCapture{budget: budget}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); drain(&stdout, perStream) }()
	go func() { defer wg.Done(); drain(&stderr, perStream) }()
	wg.Wait()

	if retained := stdout.buffer.Len() + stderr.buffer.Len(); retained > 64<<10 {
		t.Errorf("retained %d bytes, want at most %d", retained, 64<<10)
	}
	total, exceeded := budget.usage()
	if total != 2*perStream {
		t.Errorf("total = %d, want %d", total, 2*perStream)
	}
	if !exceeded {
		t.Error("usage() reported no overflow for 2 MiB against a 64 KiB budget")
	}
}

func drain(c *outputCapture, total int) {
	chunk := []byte(strings.Repeat("z", 4096))
	for written := 0; written < total; written += len(chunk) {
		if _, err := c.Write(chunk); err != nil {
			panic(err)
		}
	}
}
