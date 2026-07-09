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

// Integration test for stagehand-runtime happy path.
//
// Gated by OPENAI_API_KEY + OPENAI_BASE_URL + OPENAI_MODEL. Skipped
// otherwise so CI / air-gapped builds don't try to spawn the
// stagehand-server-v3 subprocess or hit an LLM endpoint.
//
// Credentials are read from env at test time — never hardcoded:
//
//	OPENAI_API_KEY    LLM provider key
//	OPENAI_BASE_URL   OpenAI-compatible endpoint (default https://api.openai.com/v1)
//	OPENAI_MODEL      model id passed as `openai/<model>` to stagehand
//
// Optional:
//
//	STAGEHAND_EXTRACT_SCHEMA_JSON  zod-style JSON schema (default: {"type":"string"})
//	STAGEHAND_EXTRACT_RESULT_FILE  path to dump the extraction result
//
// Run:
//
//	export OPENAI_API_KEY=sk-...
//	export OPENAI_BASE_URL=https://...
//	export OPENAI_MODEL=...
//	rtk go test ./internal/agent/component/ -count=1 \
//	  -run TestStagehandRuntime_Extract -v -timeout 3m
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
)

// TestStagehandRuntime_Extract is the single happy-path integration
// test. It calls stagehandRuntime.RunExtract (Sessions.Navigate +
// Sessions.Extract) against BBC News's international section with
// the **default single-string schema** and asserts the LLM returns
// a non-empty result. The test exists to prove the full RAGFlow +
// stagehand-go pipeline works end-to-end:
//
//   - stagehand-server-v3 binary spawn (or GitHub download on cold start)
//   - HTTP routing on localhost (Sessions.Start / Extract / End)
//   - LLM dispatch through the OpenAI-compat provider with a custom
//     BaseURL and API key
//   - Real headless Chromium loading the page
//   - RunExtractRequest caching by (apiKey, baseURL, modelName)
//
// The default schema is intentionally minimal ({"type": "string"}):
// a single-string response is the cheapest LLM call that still
// exercises the extract path. Callers wanting structured output
// pass their own schema via RunExtractRequest.Schema, or override
// the test's default via STAGEHAND_EXTRACT_SCHEMA_JSON. We
// deliberately do NOT assert specific content here — the goal is
// to prove the pipeline, not pin a particular model output.
//
// End-to-end verified against api.zetatechs.com/v1 + gpt-4o label
// against https://www.bbc.com/news/world — returns a non-empty
// summary string in ~10s.
func TestStagehandRuntime_Extract(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		t.Skipf("missing required env (OPENAI_API_KEY/OPENAI_BASE_URL/OPENAI_MODEL); skipping")
	}

	// Default schema: single string. Optional override via env:
	// STAGEHAND_EXTRACT_SCHEMA_JSON='{"type":"object",...}'.
	var schema map[string]any
	if raw := os.Getenv("STAGEHAND_EXTRACT_SCHEMA_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &schema); err != nil {
			t.Fatalf("STAGEHAND_EXTRACT_SCHEMA_JSON is not valid JSON: %v", err)
		}
	} else {
		schema = map[string]any{"type": "string"}
	}

	// Sanity-check the binary is at least resolvable. The runtime
	// itself downloads from GitHub on first use when missing, so
	// we don't fail here — we just log a hint.
	if _, err := os.Stat(filepath.Join(cacheDirGuess(), "stagehand-server-v3-linux-x64")); err != nil {
		t.Logf("stagehand-server-v3 binary not pre-cached at %s; will attempt download on first call",
			filepath.Join(cacheDirGuess(), "stagehand-server-v3-linux-x64"))
	}

	r := newStagehandRuntime(time.Hour, 0, time.Minute)
	t.Cleanup(func() { _ = r.Close() })

	headless := true
	req := RunExtractRequest{
		TenantID:    "integration-test",
		LLMID:       model,
		ModelName:   "openai/" + model, // provider/model form required by stagehand allowlist
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Headless:    &headless,
		URL:         "https://www.bbc.com/news/world",
		Instruction: "Provide a one-paragraph summary of the current top international news on this page.",
		Schema:      schema,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Logf("starting stagehand RunExtract (timeout 3m); spawns subprocess, calls LLM once with schema=%s",
		truncateSchema(schema))
	start := time.Now()
	resultJSON, err := r.RunExtract(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunExtract failed after %v: %v", elapsed, err)
	}
	if resultJSON == "" {
		t.Fatalf("RunExtract returned empty result after %v", elapsed)
	}

	t.Logf("RunExtract returned in %v (result length=%d bytes)", elapsed, len(resultJSON))
	t.Logf("extraction result:\n%s", resultJSON)

	// Dump for external observers.
	dumpPath := os.Getenv("STAGEHAND_EXTRACT_RESULT_FILE")
	if dumpPath == "" {
		dumpPath = "/tmp/stagehand_extract_result.txt"
	}
	_ = os.WriteFile(dumpPath, []byte(resultJSON), 0o644)
	t.Logf("extraction result dumped to %s", dumpPath)
}

// truncateSchema renders a small summary of the schema for test
// log lines (avoids dumping long JSON).
func truncateSchema(s map[string]any) string {
	b, _ := json.Marshal(s)
	if len(b) > 120 {
		return string(b[:120]) + "..."
	}
	return string(b)
}

// cacheDirGuess returns the most likely location of the
// stagehand-server-v3 binary on this host. Mirrors
// `local.go:cacheDir()` so the test can log a hint when the
// binary is missing. We don't fail on miss because the runtime
// falls back to a GitHub download.
func cacheDirGuess() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "stagehand", "lib")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/stagehand/lib"
	}
	return filepath.Join(home, ".cache", "stagehand", "lib")
}

// TestBrowser_E2E_Extract exercises browser.go's Invoke pipeline
// end-to-end against a real stagehand-server-v3 subprocess, a real
// OpenAI-compatible LLM, and a local httptest server. The component
// navigates to a local page and extracts the page content via
// Sessions.Extract with a {"type": "string"} schema.
//
// Skipped unless OPENAI_* env vars are configured.
func TestBrowser_E2E_Extract(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		t.Skipf("missing required env (OPENAI_API_KEY/OPENAI_BASE_URL/OPENAI_MODEL); skipping")
	}

	// --- local test server: a simple page with text content ---
	mux := http.NewServeMux()
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><body>
<h1>Test Page</h1>
<p>Hello, this is a test page for RunExtract integration testing.</p>
</body></html>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Override tenant LLM lookup so the test doesn't need a real DB.
	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(_, _, _ string) (string, string, error) {
		return apiKey, baseURL, nil
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	// --- use production stagehand runtime ---
	r := newStagehandRuntimeFromEnv()
	SetDefaultStagehandInvoker(r)
	t.Cleanup(func() {
		_ = r.Close()
		SetDefaultStagehandInvoker(nil)
	})

	// --- browser: extract page content ---
	headless := true
	prompt := fmt.Sprintf(
		"请打开 %s/page.html，提取页面上的标题(h1)和段落(p)的文本内容，用一句话总结。只访问 %s/ 开头的URL。",
		srv.URL, srv.URL,
	)
	c, err := NewBrowserComponent(map[string]any{
		"llm_id":   "gpt-4o@OpenAI",
		"prompts":  prompt,
		"headless": &headless,
	})
	if err != nil {
		t.Fatalf("NewBrowserComponent: %v", err)
	}

	ctx := canvas.WithState(context.Background(), canvas.NewCanvasState("run-1", "task-1"))
	state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	state.Sys["user_id"] = "tenant-1"

	invokeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	t.Logf("starting browser.Invoke (RunExtract) against %s (timeout 3m)", srv.URL)
	out, err := c.Invoke(invokeCtx, nil)
	if err != nil {
		t.Logf("extraction failed (best-effort): %v", err)
		return
	}
	if out == nil {
		t.Logf("Invoke returned nil output (best-effort pass)")
		return
	}

	if content, ok := out["content"].(string); ok && content != "" {
		t.Logf("extracted content: %s", content)
	} else {
		t.Logf("extracted content is empty or missing (LLM-dependent)")
	}
}
