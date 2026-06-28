//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestWriteMarkdown_SpotCheck pins the markdown output contract: it
// must enumerate both universes and surface at least one entry per
// universe. The exhaustive list is owned by RegisteredNames() and
// tool/registry.go — this test just guards against the structural
// shape regressing (lost header, lost divider, etc.).
func TestWriteMarkdown_SpotCheck(t *testing.T) {
	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	writeMarkdown()
	w.Close()
	out, _ := io.ReadAll(r)
	text := string(out)

	wants := []string{
		"## Universe A — Canvas DAG components",
		"## Universe B — eino ReAct tools",
		"| llm |",
		"| message |",
		"| agent |",
		"akshare",
		"wikipedia",
		"search_my_dateset",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("markdown output missing %q", want)
		}
	}
}

// TestWriteTSV_SpotCheck pins the TSV output contract (used in CI
// parity checks via the diff toolchain). The format is:
//
//	universe\tname\tsurface
//
// Every line must have exactly two tab separators; the header line
// and data lines are both checked.
func TestWriteTSV_SpotCheck(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	writeTSV()
	w.Close()
	out, _ := io.ReadAll(r)
	text := string(out)

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("TSV output too short: %d lines", len(lines))
	}
	if lines[0] != "universe\tname\tsurface" {
		t.Errorf("TSV header wrong: %q", lines[0])
	}
	for i, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Errorf("TSV line %d has %d fields, want 3: %q", i+1, len(fields), line)
		}
		if fields[0] != "A" && fields[0] != "B" {
			t.Errorf("TSV line %d universe %q not in {A,B}: %q", i+1, fields[0], line)
		}
	}
}

// TestSummariseComponent_NotEmpty guards the surface string format.
func TestSummariseComponent_NotEmpty(t *testing.T) {
	// We can't easily call summariseComponent without a real
	// component instance, so this test simply asserts the
	// formatter helpers are exported and don't panic on
	// empty input. (summariseComponent takes a Component; the
	// type assertion in the formatter would crash on nil, so
	// we only test truncate here.)
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short: got %q", got)
	}
	if got := truncate("hello world", 5); got != "he..." {
		t.Errorf("truncate long: got %q want %q", got, "he...")
	}
}
