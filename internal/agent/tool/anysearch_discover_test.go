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

package tool

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const anySearchCatalogBody = `{"code":0,"message":"success","request_id":"rid-cat","data":{"topology_version":9,"domains":[{"domain":"code","sub_domains":[{"sub_domain":"code.doc","params":{"library":{"required":true}}}]}]}}`

func decodeAnySearchDiscoverEnvelope(t *testing.T, out string) anySearchDiscoverEnvelope {
	t.Helper()
	var env anySearchDiscoverEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

func TestAnySearchDiscover_ListsDomainsAnonymously(t *testing.T) {
	t.Parallel()
	for _, args := range []string{`{}`, `{"domains":[]}`} {
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 200, anySearchCatalogBody)
		})
		out, err := NewAnySearchDiscoverToolWith(helper, "").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%s: %v", args, err)
		}
		env := decodeAnySearchDiscoverEnvelope(t, out)
		if env.Error != "" || len(env.Domains) != 1 || env.RequestID != "rid-cat" {
			t.Errorf("%s: envelope = %+v", args, env)
		}
		seen := rec.all()
		if len(seen) != 1 || seen[0].Method != http.MethodGet || seen[0].Path != "/v1/domains" || seen[0].RawQuery != "" || seen[0].HasAuthorization {
			t.Errorf("%s: requests = %+v, want one anonymous GET /v1/domains", args, seen)
		}
	}
}

func TestAnySearchDiscover_NamedDomainsUseRepeatedParam(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, 200, anySearchCatalogBody)
	})
	out, err := NewAnySearchDiscoverToolWith(helper, " key-1 ").InvokableRun(t.Context(), `{"domains":[" Code ","finance"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if env := decodeAnySearchDiscoverEnvelope(t, out); env.Error != "" {
		t.Fatalf("unexpected _ERROR %q", env.Error)
	}
	seen := rec.all()
	if len(seen) != 1 || seen[0].Path != "/v1/sub-domains" || seen[0].RawQuery != "domain=code&domain=finance" {
		t.Fatalf("requests = %+v, want GET /v1/sub-domains?domain=code&domain=finance", seen)
	}
	if seen[0].Authorization != "Bearer key-1" {
		t.Errorf("Authorization = %q, want the trimmed bearer key", seen[0].Authorization)
	}
}

func TestAnySearchDiscover_RejectsBadArgumentsBeforeTheRequest(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, args, wantErr string }{
		{"query injection", `{"domains":["code&x=1"]}`, "is not a domain id"},
		{"path traversal", `{"domains":["../x"]}`, "is not a domain id"},
		{"blank", `{"domains":["  "]}`, "is not a domain id"},
		{"digit first", `{"domains":["9x"]}`, "is not a domain id"},
		{"too many", `{"domains":["a","b","c","d","e","f"]}`, "at most 5"},
		{"not an array", `{"domains":"code"}`, "parse arguments"},
		{"unparseable", `{"domains":`, "parse arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
				writeAnySearch(w, 200, anySearchCatalogBody)
			})
			out, err := NewAnySearchDiscoverToolWith(helper, "key").InvokableRun(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
			}
			env := decodeAnySearchDiscoverEnvelope(t, out)
			if !strings.Contains(env.Error, tc.wantErr) || env.Domains == nil {
				t.Errorf("envelope = %+v, want _ERROR containing %q and a non-null domain list", env, tc.wantErr)
			}
			if n := len(rec.all()); n != 0 {
				t.Errorf("%d upstream requests, want 0", n)
			}
		})
	}
}

func TestAnySearchDiscover_ResponseStrictnessAndVendorErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"vendor 400", 400, `{"code":-1,"message":"Unknown domain: nope.","request_id":"rid-d"}`, "HTTP 400: Unknown domain: nope. (request_id rid-d)"},
		{"business failure on 200", 200, `{"code":7,"message":"denied","request_id":"rid-7","data":{}}`, "business error 7: denied (request_id rid-7)"},
		{"missing code", 200, `{"data":{"domains":[]}}`, "missing business status"},
		{"missing data", 200, `{"code":0}`, "missing data"},
		{"domains not an array", 200, `{"code":0,"request_id":"rid-x","data":{"domains":{}}}`, "invalid domain catalog"},
		{"invalid JSON", 200, `nope`, "invalid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
				writeAnySearch(w, tc.status, tc.body)
			})
			out, err := NewAnySearchDiscoverToolWith(helper, "key").InvokableRun(t.Context(), `{}`)
			if err != nil {
				t.Fatalf("Go error %v", err)
			}
			if env := decodeAnySearchDiscoverEnvelope(t, out); !strings.Contains(env.Error, tc.wantErr) || env.Domains == nil {
				t.Errorf("envelope = %+v, want _ERROR containing %q", env, tc.wantErr)
			}
		})
	}
}

func TestAnySearchDiscover_ServerErrorHidesTransportDetails(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, 503, `{}`)
	})
	out, _ := NewAnySearchDiscoverToolWith(helper, "secret-key").InvokableRun(t.Context(), `{"domains":["code"]}`)
	for _, leak := range []string{"secret-key", "127.0.0.1", "http://"} {
		if strings.Contains(out, leak) {
			t.Errorf("result leaks %q: %s", leak, out)
		}
	}
	if env := decodeAnySearchDiscoverEnvelope(t, out); !strings.Contains(env.Error, "request failed") {
		t.Errorf("envelope = %s, want a generic request failure after the helper's GET retries", out)
	}
}

func TestAnySearchDiscover_InfoAndRegistration(t *testing.T) {
	t.Parallel()
	info, err := NewAnySearchDiscoverTool().Info(t.Context())
	if err != nil || info.Name != anySearchDiscoverToolName {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, _ := json.Marshal(schema)
	if !strings.Contains(string(raw), `"domains"`) || !strings.Contains(string(raw), `"maxItems":5`) {
		t.Errorf("schema = %s, want domains with maxItems 5", raw)
	}
	if strings.Contains(string(raw), "api_key") {
		t.Errorf("api_key must not be model-visible: %s", raw)
	}
	if _, ok := any(NewAnySearchDiscoverTool()).(ToolComponent); ok {
		t.Error("the catalog tool is Agent-only and must not implement ToolComponent")
	}
	for _, name := range []string{"AnySearchDiscover", "anysearch_discover", "anysearchdiscover"} {
		built, err := BuildByName(name, map[string]any{"api_key": " stored ", "query": "{sys.query}", "outputs": map[string]any{}})
		if err != nil {
			t.Fatalf("BuildByName(%q): %v", name, err)
		}
		tool, ok := built.(*AnySearchDiscoverTool)
		if !ok || tool.client.apiKey != "stored" {
			t.Errorf("BuildByName(%q) = %T %+v, want *AnySearchDiscoverTool with the trimmed key", name, built, built)
		}
	}
}

func TestAnySearchDiscover_BlankArgumentsListDomains(t *testing.T) {
	t.Parallel()
	for _, args := range []string{``, `  `} {
		helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 200, anySearchCatalogBody)
		})
		out, err := NewAnySearchDiscoverToolWith(helper, "").InvokableRun(t.Context(), args)
		if err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if env := decodeAnySearchDiscoverEnvelope(t, out); env.Error != "" || len(env.Domains) != 1 {
			t.Errorf("%q: envelope = %+v, want the catalog", args, env)
		}
		if seen := rec.all(); len(seen) != 1 || seen[0].Path != "/v1/domains" {
			t.Errorf("%q: requests = %+v", args, seen)
		}
	}
}

func TestAnySearchDiscover_OversizedCatalogIsRejectedNotTruncated(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, 200, `{"code":0,"request_id":"rid-big","data":{"domains":[{"domain":"x","pad":"`+strings.Repeat("a", anySearchDiscoverMaxBytes+1)+`"}]}}`)
	})
	out, err := NewAnySearchDiscoverToolWith(helper, "").InvokableRun(t.Context(), `{"domains":["x"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchDiscoverEnvelope(t, out)
	if !strings.Contains(env.Error, "ask for fewer domains") || len(env.Domains) != 0 {
		t.Errorf("envelope = %s, want an error and no partial catalog", out[:min(len(out), 200)])
	}
	if len(out) > 1024 {
		t.Errorf("error result is %d bytes, want it small", len(out))
	}
}
