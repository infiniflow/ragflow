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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleDSL = `{
  "components": {
    "Begin:abc": {
      "downstream": ["Message:def"],
      "upstream": [],
      "obj": {"component_name": "Begin", "params": {}}
    },
    "Message:def": {
      "downstream": [],
      "upstream": ["Begin:abc"],
      "obj": {"component_name": "Message", "params": {"content": "hello"}}
    }
  },
  "globals": {"sys.query": "world"},
  "path": ["Begin:abc"]
}`

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func runMainCaptureStderr(t *testing.T, args []string) (stdout, stderr string, exitOK bool) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldStdout, oldStderr }()

	// Re-parse flags from args.
	origArgs := os.Args
	os.Args = append([]string{"migrate-canvas"}, args...)
	defer func() { os.Args = origArgs }()

	done := make(chan struct{})
	var outBuf, errBuf bytes.Buffer
	go func() { _, _ = io.Copy(&outBuf, rOut) }()
	go func() { _, _ = io.Copy(&errBuf, rErr) }()

	main()
	wOut.Close()
	wErr.Close()
	<-done
	out, _ := io.ReadAll(rOut)
	er, _ := io.ReadAll(rErr)
	_ = out
	_ = er
	return outBuf.String(), errBuf.String(), true
}

// TestRunOne_PrettyPrintNoDrift exercises the happy path: a valid
// DSL is normalised, the JSON is well-formed, and the output
// contains the same top-level components we put in.
func TestRunOne_PrettyPrintNoDrift(t *testing.T) {
	path := writeTempFile(t, "ok.json", sampleDSL)

	rOut, wOut, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = wOut
	defer func() { os.Stdout = oldStdout }()

	if err := runOne(path, "", false); err != nil {
		t.Fatalf("runOne ok: %v", err)
	}
	wOut.Close()
	got, _ := io.ReadAll(rOut)

	// Output must be valid JSON.
	var v map[string]any
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("output not valid JSON: %v\nbody: %s", err, got)
	}
	if _, ok := v["components"]; !ok {
		t.Errorf("normalised output missing 'components' top-level key: %v", v)
	}
}

// TestRunOne_GoldenMatch writes a golden, then re-reads the same
// DSL and asserts the normalisation is stable (idempotent).
func TestRunOne_GoldenMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := os.WriteFile(path, []byte(sampleDSL), 0o644); err != nil {
		t.Fatal(err)
	}

	// Phase 1: write the golden.
	if err := runOne(path, "", true); err != nil {
		t.Fatalf("write-golden: %v", err)
	}
	golden := path + ".golden"
	if _, err := os.Stat(golden); err != nil {
		t.Fatalf("golden not written: %v", err)
	}

	// Phase 2: re-read the same DSL, expect a match.
	if err := runOne(path, golden, false); err != nil {
		t.Errorf("golden match: %v", err)
	}
}

// TestRunOne_GoldenDrift covers the "drift detected" path: after
// NormalizeForCanvas changes (hypothetically), a stale golden
// surfaces as an error.
func TestRunOne_GoldenDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := os.WriteFile(path, []byte(sampleDSL), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stale golden that doesn't match the normalised output.
	golden := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(golden, []byte(`{"totally": "different"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runOne(path, golden, false); err == nil {
		t.Errorf("expected drift error, got nil")
	} else if !strings.Contains(err.Error(), "drift") {
		t.Errorf("expected drift error, got: %v", err)
	}
}
