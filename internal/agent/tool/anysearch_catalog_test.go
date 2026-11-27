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
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
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
		{"academic source filter", "academic.paper", map[string]any{"source": "MED"}},
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

// These cases exercise the public tool boundary with controlled responses.
// They do not establish vendor acceptance, filtering, or source provenance.
func TestAnySearchCatalogContractCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/anysearch-contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Tags []struct {
			Tag    string         `json:"tag"`
			Query  string         `json:"query"`
			Params map[string]any `json:"params"`
			Cases  []struct {
				ID            string         `json:"id"`
				Set           map[string]any `json:"set"`
				Omit          []string       `json:"omit"`
				Query         string         `json:"query"`
				LocalError    string         `json:"local_error"`
				UpstreamError bool           `json:"upstream_error"`
				PackageCount  int            `json:"package_count"`
			} `json:"cases"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tag := range fixture.Tags {
		for _, tc := range tag.Cases {
			t.Run(tc.ID, func(t *testing.T) {
				params := maps.Clone(tag.Params)
				for key, value := range tc.Set {
					params[key] = value
				}
				for _, key := range tc.Omit {
					delete(params, key)
				}
				query := tag.Query
				if tc.Query != "" {
					query = tc.Query
				}
				if tc.PackageCount > 0 {
					packages := make([]string, tc.PackageCount)
					for i := range packages {
						packages[i] = fmt.Sprintf("npm:contract-fixture-%d@1.0.0", i)
					}
					query = strings.Join(packages, ",")
					params["value"] = query
				}
				args := map[string]any{"query": query, "tag": tag.Tag, "params": params, "max_results": 3}
				// This response checks preservation of structured vertical data and references.
				// It makes no assertion about what a particular vendor source returns.
				const results = `[{"title":"Controlled vertical result","url":"https://example.com/contract","content":"Contract result text","source":{"name":"fixture","count":2}}]`
				handler := anySearchOK(results)
				if tc.UpstreamError {
					handler = func(w http.ResponseWriter, _ *http.Request, _ []byte) {
						writeAnySearch(w, http.StatusBadRequest, `{"code":400,"message":"contract probe rejected","request_id":"contract-case-id"}`)
					}
				}
				helper, recorder := anySearchTestServer(t, handler)
				search := NewAnySearchToolWith(helper, "")
				out, err := search.InvokableRun(t.Context(), mustJSON(t, args))
				if err != nil {
					t.Fatal(err)
				}
				env := decodeAnySearchEnvelope(t, out)
				seen := recorder.all()
				if tc.LocalError != "" {
					if len(seen) != 0 || !strings.Contains(env.Error, tc.LocalError) || len(env.Queries) != 0 {
						t.Fatalf("local validation: requests=%d result=%s", len(seen), out)
					}
					return
				}
				if len(seen) != 1 || seen[0].Method != http.MethodPost || seen[0].Path != "/v1/search" {
					t.Fatalf("requests=%+v", seen)
				}
				expected := map[string]any{"query": query, "tag": tag.Tag, "max_results": 3}
				expectedParams := maps.Clone(params)
				if tag.Tag == "travel.flight_status" {
					if _, present := expectedParams["departure"]; !present {
						expectedParams["departure"] = ""
					}
				}
				if len(expectedParams) > 0 {
					expected["params"] = expectedParams
				}
				if got := decodeAnySearchBody(t, seen[0].Body); mustJSON(t, got) != mustJSON(t, expected) {
					t.Fatalf("wire body: got=%s want=%s", mustJSON(t, got), mustJSON(t, expected))
				}
				if len(env.Queries) != 1 {
					t.Fatalf("query association: %s", out)
				}
				record := env.Queries[0]
				if record.Index != 0 || record.Query != query || record.Tag != tag.Tag {
					t.Fatalf("query association: %s", out)
				}
				decoded := decodeAnySearchBody(t, []byte(out))
				chunks, docs := search.BuildReferences(t.Context(), decoded)
				if tc.UpstreamError {
					if !strings.Contains(env.Error, "contract probe rejected") || record.Error == "" || record.RequestID != "contract-case-id" || len(record.Results) != 0 || len(chunks) != 0 || len(docs) != 0 {
						t.Fatalf("upstream error became success or lost diagnostics: %s", out)
					}
					return
				}
				var want []map[string]any
				if err := json.Unmarshal([]byte(results), &want); err != nil {
					t.Fatal(err)
				}
				if env.Error != "" || record.Error != "" || record.RequestID != "rid-ok" || mustJSON(t, record.Results) != mustJSON(t, want) || len(chunks) != 1 || len(docs) != 1 {
					t.Fatalf("delivered result/reference: %s chunks=%v docs=%v", out, chunks, docs)
				}
				if chunks[0]["content"] != "Contract result text" || chunks[0]["url"] != "https://example.com/contract" {
					t.Fatalf("reference=%v", chunks[0])
				}
			})
		}
	}
}
