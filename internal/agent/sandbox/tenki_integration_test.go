//go:build integration

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0.

package sandbox

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	tenkisdk "github.com/LuxorLabs/tenki-sdk-go/sandbox"
)

func TestTenkiProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("TENKI_API_KEY")
	if apiKey == "" {
		t.Skip("TENKI_API_KEY is required")
	}
	p := newTenkiProviderFromConfig(map[string]any{
		"api_key": apiKey, "base_url": os.Getenv("TENKI_API_URL"),
		"timeout": 120, "max_lifetime": 300, "cpu_cores": 2, "memory_mb": 2048,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	if err := p.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.client.Close() })
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created session %s", inst.InstanceID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := p.DestroyInstance(cleanupCtx, inst); err != nil {
			t.Errorf("destroy %s: %v", inst.InstanceID, err)
		}
	})

	result, err := p.ExecuteCode(ctx, inst, `def main(value):
    from pathlib import Path
    print("python output")
    Path("artifacts/result.json").write_text('{"artifact": true}')
    return {"value": value}
`, "python", 10, map[string]any{"value": "sdk14"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "python output") {
		t.Fatalf("unexpected Python result: %+v", result)
	}
	structured, ok := result.Metadata["structured_result"].(map[string]any)
	if !ok || structured["present"] != true {
		t.Fatalf("missing structured result: %v", result.Metadata["structured_result"])
	}
	value, ok := structured["value"].(map[string]any)
	if !ok || value["value"] != "sdk14" {
		t.Fatalf("unexpected structured value: %v", structured["value"])
	}
	artifacts, ok := result.Metadata["artifacts"].([]map[string]any)
	if !ok || len(artifacts) != 1 || artifacts[0]["name"] != "result.json" {
		t.Fatalf("unexpected artifacts: %v", result.Metadata["artifacts"])
	}
	if artifacts[0]["content_b64"] != base64.StdEncoding.EncodeToString([]byte(`{"artifact": true}`)) {
		t.Fatalf("unexpected artifact content: %v", artifacts[0])
	}

	result, err = p.ExecuteCode(ctx, inst, `function main() { console.log("javascript output"); return 42; }`, "javascript", 10, nil)
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, "javascript output") {
		t.Fatalf("JavaScript result=%+v error=%v", result, err)
	}
	result, err = p.ExecuteCode(ctx, inst, `def main():
    raise RuntimeError("expected failure")
`, "python", 10, nil)
	if err != nil || result.ExitCode == 0 || !strings.Contains(result.Stderr, "expected failure") {
		t.Fatalf("failure result=%+v error=%v", result, err)
	}

	result, err = p.ExecuteCode(ctx, inst, `def main():
    import sys, time
    from pathlib import Path
    print("partial stdout", flush=True)
    print("partial stderr", file=sys.stderr, flush=True)
    time.sleep(3)
    Path("artifacts/late.txt").write_text("should not happen")
`, "python", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 124 || result.Metadata["tenki_status"] != string(tenkisdk.CommandStatusTimedOut) ||
		!strings.Contains(result.Stdout, "partial stdout") || !strings.Contains(result.Stderr, "partial stderr") {
		t.Fatalf("unexpected timeout result: %+v", result)
	}
	result, err = p.ExecuteCode(ctx, inst, `def main():
    import time
    from pathlib import Path
    time.sleep(3)
    return Path("artifacts/late.txt").exists()
`, "python", 10, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("post-timeout result=%+v error=%v", result, err)
	}
	structured, ok = result.Metadata["structured_result"].(map[string]any)
	if !ok || structured["value"] != false {
		t.Fatalf("timed-out command continued running: %v", structured)
	}
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatal(err)
	}
	t.Logf("destroyed session %s", inst.InstanceID)
}
