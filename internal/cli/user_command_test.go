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

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDevUpdateChunkCommandUsesChunkResource(t *testing.T) {
	updated := make(chan struct{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/datasets":
			if r.Method != http.MethodGet {
				t.Errorf("dataset lookup method = %s, want GET", r.Method)
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":[{"id":"dataset-1","name":"test"}]}`)
		case "/api/v1/datasets/dataset-1/documents/doc-1/chunks/chunk-1":
			if r.Method != http.MethodPatch {
				t.Errorf("update method = %s, want PATCH", r.Method)
			}
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode update: %v", err)
			}
			if payload["content"] != "updated" || payload["available"] != false {
				t.Errorf("unexpected update payload: %#v", payload)
			}
			updated <- struct{}{}
			_, _ = fmt.Fprint(w, `{"code":0,"message":"success"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	result, err := c.DevUpdateChunkCommand(1, &Command{Params: map[string]interface{}{
		"dataset_name": "test",
		"doc_id":       "doc-1",
		"chunk_id":     "chunk-1",
		"json_body":    `{"content":"updated","available":false}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-updated:
	default:
		t.Fatal("chunk update was not sent")
	}
	if response, ok := result.(*SimpleResponse); !ok || response.Code != 0 {
		t.Fatalf("unexpected response: %#v", result)
	}
}

// TestAPIListIngestionTasksSendsDatasetIDFilter pins the fix for the wrong
// type assertion: the parser stores dataset_id as a plain string, but the old
// code asserted .(*string), which always failed and sent dataset_id: null —
// so the server returned tasks across ALL datasets instead of the requested
// one. The id must now reach the request body.
func TestAPIListIngestionTasksSendsDatasetIDFilter(t *testing.T) {
	got := make(chan map[string]interface{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/ingestion/tasks" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
			http.NotFound(w, r)
			return
		}
		got <- payload
		_, _ = fmt.Fprint(w, `{"code":0,"message":"success","data":[]}`)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	if _, err := c.APIListIngestionTasks(1, &Command{Params: map[string]interface{}{"dataset_id": "ds-1"}}); err != nil {
		t.Fatalf("APIListIngestionTasks: %v", err)
	}
	select {
	case payload := <-got:
		if payload["dataset_id"] != "ds-1" {
			t.Fatalf("dataset_id = %#v, want \"ds-1\" (filter must reach the body)", payload["dataset_id"])
		}
	default:
		t.Fatal("no request was sent")
	}
}

// TestAPIListIngestionTasksOmitsDatasetIDWhenAbsent confirms the fix keeps
// the "no filter" semantics when the caller does not supply a dataset_id:
// the field must be omitted/null (the server filters only when it is
// non-nil), not an empty string that would trigger a permission check.
func TestAPIListIngestionTasksOmitsDatasetIDWhenAbsent(t *testing.T) {
	got := make(chan map[string]interface{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		got <- payload
		_, _ = fmt.Fprint(w, `{"code":0,"message":"success","data":[]}`)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	if _, err := c.APIListIngestionTasks(1, &Command{Params: map[string]interface{}{}}); err != nil {
		t.Fatalf("APIListIngestionTasks: %v", err)
	}
	select {
	case payload := <-got:
		if v, present := payload["dataset_id"]; present && v != nil {
			t.Fatalf("dataset_id = %#v, want omitted or null when no filter given", v)
		}
	default:
		t.Fatal("no request was sent")
	}
}

const tokenStatsFixture = `{"code":0,"message":"success","data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"Asia/Shanghai"},"filters":{"provider_name":"openai","instance_name":"production","model_name":null},"group_by":["day","model"],"summary":{"input_tokens":120000,"output_tokens":30000,"total_tokens":150000},"rows":[{"date":"2026-10-23","provider_name":"openai","instance_name":"production","model_name":"gpt-4o","input_tokens":120000,"output_tokens":30000,"total_tokens":150000}]}}`

func TestTokenStatsRequestAndJSON(t *testing.T) {
	for _, auth := range []string{"login", "api"} {
		t.Run(auth, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/stats/token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				wantAuth := "session-token"
				if auth == "api" {
					wantAuth = "Bearer api-key"
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Errorf("authorization = %q", r.Header.Get("Authorization"))
				}
				wantQuery := url.Values{"days": {"7"}, "provider_name": {"openai"}, "instance_name": {"production"}, "group_by": {"day,model"}}
				if !reflect.DeepEqual(r.URL.Query(), wantQuery) {
					t.Errorf("query = %v, want %v", r.URL.Query(), wantQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tokenStatsFixture)
			}))
			token := "session-token"
			client.LoginToken = &token
			if auth == "api" {
				key := "api-key"
				client.LoginToken = nil
				client.APIKey = &key
				client.useAPIKey = true
			}
			c := tokenStatsCLI(client, OutputFormatJSON)
			output := captureTokenStatsOutput(t, func() {
				if err := c.execute("TOKEN STATS DAYS 7 PROVIDER 'openai' INSTANCE 'production' BY DAY, MODEL;"); err != nil {
					t.Fatal(err)
				}
			})
			var data struct {
				Summary struct {
					Total int64 `json:"total_tokens"`
				} `json:"summary"`
				Rows    []map[string]interface{} `json:"rows"`
				GroupBy []string                 `json:"group_by"`
			}
			if err := json.Unmarshal([]byte(output), &data); err != nil {
				t.Fatalf("JSON output invalid: %v: %s", err, output)
			}
			if data.Summary.Total != 150000 || len(data.Rows) != 1 || !reflect.DeepEqual(data.GroupBy, []string{"day", "model"}) {
				t.Fatalf("incomplete JSON output: %s", output)
			}
		})
	}
}

func TestTokenStatsDefaultAndDateRequests(t *testing.T) {
	for _, tt := range []struct{ input, query string }{
		{"TOKEN STATS", ""},
		{"TOKEN STATS TODAY", "days=1"},
		{"TOKEN STATS FROM '2026-10-01' TO '2026-10-07' PROVIDER 'a & b' INSTANCE 'prod/eu' MODEL 'x+y' BY MODEL", "from=2026-10-01&group_by=model&instance_name=prod%2Feu&model_name=x%2By&provider_name=a+%26+b&to=2026-10-07"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != tt.query {
					t.Errorf("query=%q, want %q", r.URL.RawQuery, tt.query)
				}
				fmt.Fprint(w, tokenStatsFixture)
			}))
			token := "session"
			client.LoginToken = &token
			c := tokenStatsCLI(client, OutputFormatJSON)
			captureTokenStatsOutput(t, func() {
				if err := c.execute(tt.input); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestTokenStatsRequiresAuthorization(t *testing.T) {
	c := tokenStatsCLI(NewHTTPClient(), OutputFormatTable)
	if err := c.execute("TOKEN STATS"); err == nil || !strings.Contains(err.Error(), "no authorization") {
		t.Fatalf("expected authorization error, got %v", err)
	}
}

func TestTokenStatsOutput(t *testing.T) {
	for _, format := range []OutputFormat{OutputFormatTable, OutputFormatPlain} {
		t.Run(string(format), func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tokenStatsFixture) }))
			token := "session"
			client.LoginToken = &token
			output := captureTokenStatsOutput(t, func() {
				if err := tokenStatsCLI(client, format).execute("TOKEN STATS"); err != nil {
					t.Fatal(err)
				}
			})
			for _, want := range []string{"Period:", "2026-10-17", "Timezone:", "Asia/Shanghai", "Provider:", "openai", "Instance:", "production", "Group by:", "Total tokens:", "150000", "INPUT TOKENS", "OUTPUT TOKENS", "TOTAL TOKENS", "gpt-4o"} {
				if !strings.Contains(strings.ToLower(output), strings.ToLower(want)) {
					t.Errorf("missing %q in output: %s", want, output)
				}
			}
			if strings.Contains(output, "|") != (format == OutputFormatTable) {
				t.Errorf("incorrect table borders for %s: %s", format, output)
			}
		})
	}
}

func TestTokenStatsResponseErrorsAndEmpty(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body, want string
		wantError  bool
	}{
		{"http", 503, `{"message":"unavailable"}`, "HTTP 503", true},
		{"business", 200, `{"code":100,"message":"Invalid date range"}`, "Invalid date range", true},
		{"json", 200, `broken`, "invalid JSON", true},
		{"missing data", 200, `{"code":0}`, "missing", true},
		{"null data", 200, `{"code":0,"data":null}`, "missing", true},
		{"missing summary", 200, `{"code":0,"data":{}}`, "missing", true},
		{"missing grouping", 200, `{"code":0,"data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"UTC"},"summary":{"total_tokens":50},"rows":[]}}`, "group_by", true},
		{"unknown grouping", 200, `{"code":0,"data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"UTC"},"summary":{"total_tokens":50},"group_by":["unknown"],"rows":[]}}`, "group_by", true},
		{"multiple resource groups", 200, `{"code":0,"data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"UTC"},"summary":{"total_tokens":50},"group_by":["model","instance"],"rows":[]}}`, "group_by", true},
		{"empty", 200, `{"code":0,"data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"UTC"},"filters":{},"group_by":["provider"],"summary":{"input_tokens":0,"output_tokens":0,"total_tokens":0},"rows":[]}}`, "No usage data for this period.", false},
		{"total only", 200, `{"code":0,"data":{"period":{"from":"2026-10-17","to":"2026-10-23","timezone":"UTC"},"filters":{"provider_name":"openai","instance_name":"prod","model_name":"gpt-4o"},"group_by":[],"summary":{"input_tokens":null,"output_tokens":null,"total_tokens":50},"rows":[]}}`, "N/A", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.status); fmt.Fprint(w, tt.body) }))
			token := "session"
			client.LoginToken = &token
			var err error
			output := captureTokenStatsOutput(t, func() { err = tokenStatsCLI(client, OutputFormatTable).execute("TOKEN STATS") })
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error=%v, want %s", err, tt.want)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output, tt.want) {
					t.Fatalf("missing %s: %s", tt.want, output)
				}
				if tt.name == "total only" && strings.Contains(output, "No usage data") {
					t.Fatalf("valid total presented as empty: %s", output)
				}
			}
		})
	}
}

func tokenStatsCLI(client *HTTPClient, format OutputFormat) *CLI {
	return &CLI{Config: &CommandLineConfig{CLIMode: APIMode, OutputFormat: format, APIClientConfig: APIModeConfig{CurrentAPIServer: "test"}}, APIServerClientMap: map[string]*HTTPClient{"test": client}}
}

func captureTokenStatsOutput(t *testing.T, run func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = original; reader.Close(); writer.Close() }()
	output := make(chan string, 1)
	go func() { data, _ := io.ReadAll(reader); output <- string(data) }()
	run()
	writer.Close()
	return <-output
}

func TestTokenStatsGroupingColumns(t *testing.T) {
	for _, tt := range []struct{ group, header string }{
		{`["provider"]`, "provider input tokens output tokens total tokens"},
		{`["instance"]`, "provider instance input tokens output tokens total tokens"},
		{`["model"]`, "provider instance model input tokens output tokens total tokens"},
		{`["day"]`, "date input tokens output tokens total tokens"},
		{`["day","provider"]`, "date provider input tokens output tokens total tokens"},
		{`["day","instance"]`, "date provider instance input tokens output tokens total tokens"},
		{`["day","model"]`, "date provider instance model input tokens output tokens total tokens"},
	} {
		t.Run(tt.group, func(t *testing.T) {
			body := strings.Replace(tokenStatsFixture, `["day","model"]`, tt.group, 1)
			result, err := handleTokenStatsResponse(&Response{StatusCode: 200, Body: []byte(body)})
			if err != nil {
				t.Fatal(err)
			}
			result.SetOutputFormat(OutputFormatPlain)
			output := captureTokenStatsOutput(t, result.PrintOut)
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(line, "input tokens") && strings.Contains(line, "output tokens") {
					if header := strings.Join(strings.Fields(line), " "); header != tt.header {
						t.Fatalf("header=%q, want %q", header, tt.header)
					}
					return
				}
			}
			t.Fatalf("missing detail header: %s", output)
		})
	}
}
