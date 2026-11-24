// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package tool

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// The fixture records parameter names from the 2026-11-16 AnySearch catalog.
// Synthetic values prove transport fidelity, not vendor-side validity.
func TestAnySearchCatalogParameterForwarding(t *testing.T) {
	raw, err := os.ReadFile("testdata/anysearch-catalog-2026-11-16.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []struct {
		Tag        string   `json:"tag"`
		Parameters []string `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 39 {
		t.Fatalf("catalog has %d tags, want 39", len(catalog))
	}
	parameterCount := 0
	for _, entry := range catalog {
		parameterCount += len(entry.Parameters)
		t.Run(entry.Tag, func(t *testing.T) {
			params := make(map[string]any, len(entry.Parameters))
			for _, name := range entry.Parameters {
				params[name] = "value:" + name
			}
			helper, recorder := anySearchTestServer(t, anySearchOK(`[]`))
			args := map[string]any{"query": "catalog mapping", "tag": entry.Tag, "params": params, "zone": "intl", "language": "en", "max_results": 3}
			out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), mustJSON(t, args))
			if err != nil || decodeAnySearchEnvelope(t, out).Error != "" {
				t.Fatalf("InvokableRun: result=%s error=%v", out, err)
			}
			requests := recorder.all()
			if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/v1/search" {
				t.Fatalf("requests=%+v", requests)
			}
			body := decodeAnySearchBody(t, requests[0].Body)
			if body["tag"] != entry.Tag || body["zone"] != "intl" || body["language"] != "en" || body["max_results"] != json.Number("3") {
				t.Fatalf("incorrect routing or options: %#v", body)
			}
			if len(params) == 0 {
				if _, exists := body["params"]; exists {
					t.Fatalf("empty parameters should be omitted: %#v", body)
				}
			} else if mustJSON(t, body["params"]) != mustJSON(t, params) {
				t.Fatalf("parameters changed: got=%s want=%s", mustJSON(t, body["params"]), mustJSON(t, params))
			}
		})
	}
	if parameterCount != 190 {
		t.Fatalf("catalog has %d parameters, want 190", parameterCount)
	}
}

func TestAnySearchVerticalSourceAndConditionalFields(t *testing.T) {
	cases := []struct {
		name, tag string
		params    map[string]any
	}{
		{"US fundamentals keeps unused China code", "finance.fundamental", map[string]any{"type": "overview", "symbol": "AAPL", "cn_code": ""}},
		{"China fundamentals keeps unused symbol", "finance.fundamental", map[string]any{"type": "income", "symbol": "", "cn_code": "600519.SH"}},
		{"CVE lookup", "security.vuln", map[string]any{"type": "cve", "value": "CVE-2021-44228"}},
		{"package lookup", "security.vuln", map[string]any{"type": "package", "value": "PyPI:jinja2@2.4.1"}},
		{"academic source filter", "academic.paper", map[string]any{"source": "MED", "data_source": "arxiv,openalex"}},
	}
	for _, source := range []string{"sina", "wallstreetcn", "10jqka", "eastmoney", "cls", "yicai", "yuncaijing", "fenghuang", "jinrongjie"} {
		cases = append(cases, struct {
			name, tag string
			params    map[string]any
		}{"finance source " + source, "finance.news", map[string]any{"type": "flash", "news_src": source}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			helper, recorder := anySearchTestServer(t, anySearchOK(`[]`))
			args := map[string]any{"query": "example", "tag": tc.tag, "params": tc.params}
			out, err := NewAnySearchToolWith(helper, "").InvokableRun(t.Context(), mustJSON(t, args))
			if err != nil || decodeAnySearchEnvelope(t, out).Error != "" {
				t.Fatalf("InvokableRun: result=%s error=%v", out, err)
			}
			requests := recorder.all()
			if len(requests) != 1 {
				t.Fatalf("requests=%d, want 1", len(requests))
			}
			body := decodeAnySearchBody(t, requests[0].Body)
			if body["tag"] != tc.tag || mustJSON(t, body["params"]) != mustJSON(t, tc.params) {
				t.Fatalf("source or conditional parameters changed: %#v", body)
			}
		})
	}
}
