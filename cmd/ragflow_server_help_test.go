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

package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"ragflow/internal/server/bootstrap"
)

// captureHelpStderr runs fn with os.Stderr redirected to a pipe and returns
// everything it wrote. printHelp writes to os.Stderr directly, so this is the
// only way to assert on its output without changing production code.
func captureHelpStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close help pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read help output: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close help read pipe: %v", err)
	}
	return string(out)
}

// TestPrintHelpIngestorDocumentsIngestorTier verifies that the ingestor help
// block exposes every ingestor-specific startup knob, including the tier.
// Without this an operator cannot discover the flag from --help even though
// it is accepted by the parser.
func TestPrintHelpIngestorDocumentsIngestorTier(t *testing.T) {
	mode := "ingestor"
	args := &bootstrap.ServerArgs{Mode: &mode}
	help := captureHelpStderr(t, func() { printHelp(args) })
	if !strings.Contains(help, "--ingestor-tier") {
		t.Fatalf("ingestor help does not document --ingestor-tier:\n%s", help)
	}
	// Adversarial: the flag is ingestor-specific; it must not leak into an
	// unrelated mode's help block. A wrong implementation that simply printed
	// the flag unconditionally would be caught here.
	adminMode := "admin"
	adminArgs := &bootstrap.ServerArgs{Mode: &adminMode}
	adminHelp := captureHelpStderr(t, func() { printHelp(adminArgs) })
	if strings.Contains(adminHelp, "--ingestor-tier") {
		t.Fatalf("admin help must not document --ingestor-tier:\n%s", adminHelp)
	}
}

func TestPrintHelpChatter(t *testing.T) {
	for _, mode := range []string{"api", "chatter"} {
		help := captureHelpStderr(t, func() { printHelp(&bootstrap.ServerArgs{Mode: new(mode)}) })
		want := "--disable-chat-channels"
		if mode == "chatter" {
			want = "--chatter"
		}
		if !strings.Contains(help, want) {
			t.Fatalf("%s help missing %s: %s", mode, want, help)
		}
	}
}
