//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

// migrate-canvas applies Go's dsl.NormalizeForCanvas to one or more
// JSON files and emits the normalized form. It is the "Go-side" of
// the parity corpus described in the agent-go-port-design doc §7.
//
// Usage:
//
//	# Pretty-print the normalized form of a single DSL file:
//	go run ./tools/migrate-canvas docs/develop/sample.json
//
//	# Diff against a "golden" normalized file (CI mode):
//	go run ./tools/migrate-canvas -golden=expected.json docs/develop/sample.json
//
//	# Walk every testdata fixture and emit the normalized form:
//	go run ./tools/migrate-canvas -walk internal/agent/dsl/testdata
//
// Behaviour:
//   - In non-CI mode, the tool writes the normalized JSON to stdout
//     (pretty-printed) and prints a one-line summary to stderr.
//   - In CI mode (-golden=<path>), the tool compares the actual
//     normalized form to the golden file and exits non-zero on drift.
//   - The tool never panics on malformed input: NormalizeForCanvas
//     is best-effort and the tool reports the result as a warning
//     when the input is empty / unparseable.
//
// Limitations vs the original plan §7 migrate-canvas spec:
//   - This tool does NOT shell out to Python. The Python side's
//     normalize_chunker_dsl is for the chunker DSL, which is a
//     different domain (chunking pipeline, not agent canvas). The
//     Go-side NormalizeForCanvas covers the agent canvas normalize
//     path; the chunker DSL still goes through the Python path
//     (deferred per plan v3.3.1 user decision).
//   - The "fixture corpus" used in CI is the existing 7 files in
//     internal/agent/dsl/testdata/; the tool can be extended to
//     add a -generate-golden flag that writes the current output
//     as the new golden (the standard update-on-intent pattern).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dslpkg "ragflow/internal/agent/dsl"
)

func main() {
	goldenFlag := flag.String("golden", "", "path to golden file for CI drift check (mutually exclusive with -write-golden)")
	writeGolden := flag.Bool("write-golden", false, "write the normalized output to <input>.golden (update pattern)")
	walkDir := flag.String("walk", "", "if set, treat positional args as a directory; every *.json file is normalised")
	flag.Parse()

	if *goldenFlag != "" && *writeGolden {
		fmt.Fprintln(os.Stderr, "migrate-canvas: -golden and -write-golden are mutually exclusive")
		os.Exit(2)
	}

	if *walkDir != "" {
		// Walk directory mode: positional args ignored; walk <walkDir>/**/*.json.
		runWalk(*walkDir, *goldenFlag, *writeGolden)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "migrate-canvas: expected <file.json> (or -walk <dir>)")
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	exit := 0
	for _, path := range args {
		if err := runOne(path, *goldenFlag, *writeGolden); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", path, err)
			exit = 1
		}
	}
	os.Exit(exit)
}

func runOne(path, goldenPath string, writeGolden bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	normalised := dslpkg.NormalizeForCanvas(dsl)

	if writeGolden {
		goldenPath = path + ".golden"
		out, mErr := marshalPretty(normalised)
		if mErr != nil {
			return fmt.Errorf("marshal: %w", mErr)
		}
		if err := os.WriteFile(goldenPath, out, 0o644); err != nil {
			return fmt.Errorf("write golden: %w", err)
		}
		fmt.Fprintf(os.Stderr, "OK %s -> %s\n", path, goldenPath)
		return nil
	}

	if goldenPath != "" {
		golden, gErr := os.ReadFile(goldenPath)
		if gErr != nil {
			return fmt.Errorf("read golden: %w", gErr)
		}
		actual, mErr := marshalPretty(normalised)
		if mErr != nil {
			return fmt.Errorf("marshal: %w", mErr)
		}
		if !bytes.Equal(bytes.TrimSpace(golden), bytes.TrimSpace(actual)) {
			return fmt.Errorf("drift: golden != actual (run with -write-golden to update)")
		}
		fmt.Fprintf(os.Stderr, "OK %s (matches %s)\n", path, goldenPath)
		return nil
	}

	out, mErr := marshalPretty(normalised)
	if mErr != nil {
		return fmt.Errorf("marshal: %w", mErr)
	}
	fmt.Print(string(out))
	fmt.Fprintf(os.Stderr, "OK %s (%d bytes)\n", path, len(out))
	return nil
}

func runWalk(dir, goldenDir string, writeGolden bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-canvas: walk: %v\n", err)
		os.Exit(1)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	exit := 0
	for _, p := range files {
		var golden string
		if goldenDir != "" {
			golden = filepath.Join(goldenDir, filepath.Base(p)+".golden")
		}
		if err := runOne(p, golden, writeGolden); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", p, err)
			exit = 1
		}
	}
	os.Exit(exit)
}

func marshalPretty(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
