//go:build integration

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
	"context"
	"os"
	"ragflow/internal/agentic_rag"
	"strconv"
	"strings"
	"testing"
)

func TestContextLiveNativeSearchCallback(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("CONTEXT_DEV_TEST_API_KEY"))
	if key == "" {
		t.Skip("CONTEXT_DEV_TEST_API_KEY required; not a live pass")
	}
	callback := (&ChatPipelineService{}).webSearchFunc(map[string]interface{}{"web_search_provider": "context", "context_dev_api_key": key})
	if callback == nil {
		t.Fatal("native callback unavailable")
	}
	var hits []agentic_rag.WebResult
	search := func(ctx context.Context, query string) ([]agentic_rag.WebResult, error) {
		results, err := callback(ctx, query)
		hits = results
		return results, err
	}
	output, err := agentic_rag.NewWebSearchTool(search).InvokableRun(t.Context(), `{"queries":["PostgreSQL official documentation CREATE INDEX"]}`)
	if err != nil {
		t.Fatal("native live Context search failed; inspect sanitized evidence")
	}
	if len(hits) == 0 || len(hits) > webSearchResultCount {
		t.Fatal("unexpected live result count")
	}
	if !strings.Contains(output, `source="web"`) || !strings.Contains(output, `<search_results count="`+strconv.Itoa(len(hits))+`"`) {
		t.Fatal("native model-facing tool did not render live web citations")
	}
	if strings.Contains(output, key) {
		t.Fatal("credential in tool output")
	}
	highlighted := 0
	for _, hit := range hits {
		if strings.TrimSpace(hit.Content) == "" || !strings.HasPrefix(hit.URL, "https://") {
			t.Fatal("missing live passage/citation")
		}
		if !strings.Contains(hit.Content, "[Highlights unavailable;") {
			highlighted++
		}
		if strings.Contains(hit.Content, key) {
			t.Fatal("credential in result")
		}
	}
	if highlighted == 0 {
		t.Fatal("live search returned no successful Highlights")
	}
	t.Logf("real model-facing web_search tool returned %d cited Context results, %d with successful Highlights", len(hits), highlighted)
}
