package tool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestYouComToolSearchAndReferences(t *testing.T) {
	for _, tc := range []struct {
		name          string
		key           string
		freshness     string
		wantFreshness string
	}{
		{name: "keyless default"},
		{name: "keyless any", freshness: "any"},
		{name: "keyed week", key: "test-key", freshness: "week", wantFreshness: "week"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if got := r.URL.Query().Get("query"); got != "RAGFlow testing" {
					t.Errorf("query = %q", got)
				}
				if got := r.URL.Query().Get("count"); got != "3" {
					t.Errorf("count = %q", got)
				}
				if got := r.URL.Query().Get("freshness"); got != tc.wantFreshness {
					t.Errorf("freshness = %q, want %q", got, tc.wantFreshness)
				}
				if got := r.Header.Get("X-API-Key"); got != tc.key {
					t.Errorf("X-API-Key = %q, want %q", got, tc.key)
				}
				if got := r.Header.Get("User-Agent"); got != youComUserAgent {
					t.Errorf("User-Agent = %q", got)
				}
				if tc.key == "" && r.URL.Path != "/keyless" || tc.key != "" && r.URL.Path != "/keyed" {
					t.Errorf("path = %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":{"web":[{"title":"First","url":"https://example.com/1","snippets":["passage one"," ","passage two"],"description":"ignored"},{"title":"Blank","url":"https://example.com/blank","description":" "}],"news":[{"title":"News","url":"https://example.com/2","description":"news detail"},{"title":"Extra","url":"https://example.com/3","description":"extra"}]}}`))
			}))
			defer server.Close()

			tool := newYouComTool(NewHTTPHelper().WithClient(server.Client()), tc.key, youComParams{TopN: 3, Freshness: tc.freshness})
			tool.keylessEndpoint = server.URL + "/keyless"
			tool.keyedEndpoint = server.URL + "/keyed"
			raw, err := tool.InvokableRun(t.Context(), `{"query":"RAGFlow testing"}`)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want 1", requests)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
				t.Fatal(err)
			}
			results := envelopeSlice(envelope, "results")
			if len(results) != 3 {
				t.Fatalf("results = %#v, want three raw hits", results)
			}
			first := results[0].(map[string]any)
			if first["title"] != "First" || len(first["snippets"].([]any)) != 3 {
				t.Errorf("first = %#v", first)
			}
			third := results[2].(map[string]any)
			if third["description"] != "news detail" || third["title"] != "News" {
				t.Errorf("third = %#v", third)
			}
			chunks, docs := tool.BuildReferences(t.Context(), envelope)
			if len(chunks) != 2 || len(docs) != 2 || chunks[0]["url"] != "https://example.com/1" || chunks[1]["content"] != "news detail" {
				t.Errorf("references = %#v, %#v", chunks, docs)
			}
			outputs := tool.BuildComponentOutputs(envelope)
			if !strings.Contains(outputs["formalized_content"].(string), "passage one passage two") || len(outputs["json"].([]any)) != 3 {
				t.Errorf("outputs = %#v", outputs)
			}
		})
	}
}

func TestYouComToolInvalidInputsAndFailures(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"key":"do-not-expose"}`))
	}))
	defer server.Close()
	tool := newYouComTool(NewHTTPHelper().WithClient(server.Client()), "private-key", youComParams{})
	tool.keyedEndpoint = server.URL
	for _, input := range []string{`{"query":""}`, `{"query":"   "}`} {
		if raw, err := tool.InvokableRun(t.Context(), input); err != nil || raw != `{"results":[]}` {
			t.Errorf("blank query: %q, %v", raw, err)
		}
	}
	if requests != 0 {
		t.Errorf("blank query made %d requests", requests)
	}
	for _, input := range []string{`{`, `{"query":"x","freshness":"never"}`} {
		if _, err := tool.InvokableRun(t.Context(), input); err == nil {
			t.Errorf("input %q accepted", input)
		}
	}
	_, err := tool.InvokableRun(t.Context(), `{"query":"private query"}`)
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "private-key") || strings.Contains(err.Error(), "private query") {
		t.Errorf("failure = %v", err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
}

func TestYouComToolCapsUpstreamCountButKeepsRequestedMergedLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count := r.URL.Query().Get("count"); count != "100" {
			t.Errorf("upstream count = %q, want 100", count)
		}
		web := make([]map[string]any, 100)
		news := make([]map[string]any, 100)
		for i := range web {
			web[i] = map[string]any{"title": "web", "url": "https://example.com", "description": "web"}
			news[i] = map[string]any{"title": "news", "url": "https://example.com", "description": "news"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"web": web, "news": news}})
	}))
	defer server.Close()
	tool := newYouComTool(NewHTTPHelper().WithClient(server.Client()), "", youComParams{TopN: 150})
	tool.keylessEndpoint = server.URL
	raw, err := tool.InvokableRun(t.Context(), `{"query":"bounded"}`)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	results := envelopeSlice(envelope, "results")
	if len(results) != 150 || results[99].(map[string]any)["title"] != "web" || results[100].(map[string]any)["title"] != "news" {
		t.Fatalf("merged results = %#v", results)
	}
}

func TestYouComToolPreservesRawLargeNumbers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"web":[{"title":"large","url":"https://example.com","id":9007199254740993,"score":0.125}],"news":[]}}`))
	}))
	defer server.Close()
	tool := newYouComTool(NewHTTPHelper().WithClient(server.Client()), "", youComParams{})
	tool.keylessEndpoint = server.URL
	raw, err := tool.InvokableRun(t.Context(), `{"query":"precision"}`)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	result := envelopeSlice(envelope, "results")[0].(map[string]any)
	if result["id"] != json.Number("9007199254740993") || result["score"] != json.Number("0.125") {
		t.Errorf("raw numeric values changed: %#v", result)
	}
}

func TestYouComCanvasRegistration(t *testing.T) {
	for _, name := range []string{"YouComSearch", "youcom", "youcom_search"} {
		built, err := BuildByName(name, map[string]any{"api_key": "test-key", "freshness": "day", "top_n": float64(3)})
		if err != nil {
			t.Fatalf("BuildByName(%q): %v", name, err)
		}
		got, ok := built.(*YouComTool)
		if !ok || got.apiKey != "test-key" || !reflect.DeepEqual(got.defaults, youComParams{TopN: 3, Freshness: "day"}) {
			t.Errorf("BuildByName(%q) = %#v", name, built)
		}
	}
	for _, invalid := range []map[string]any{{"top_n": 0}, {"top_n": 1.5}, {"freshness": "never"}, {"api_key": 42}} {
		if _, err := BuildByName("YouComSearch", invalid); err == nil {
			t.Errorf("accepted invalid params %#v", invalid)
		}
	}
}
