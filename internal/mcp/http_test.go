package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testConnector struct{}

func (testConnector) ListDatasets(context.Context, int, int, string, bool) (string, error) {
	return `{"id":"dataset-1","name":"Test","description":""}`, nil
}
func (testConnector) ListChats(context.Context, int, int, string, bool) (string, error) {
	return `{"id":"chat-1","name":"Test","description":""}`, nil
}
func (testConnector) Retrieval(context.Context, RetrievalRequest) (string, error) {
	return `{"ok":true}`, nil
}

func TestHTTPHandlerProtocolAndRequestScopedFactory(t *testing.T) {
	seen := make([]string, 0, 2)
	h := NewHTTPHandler(func(r *http.Request) (*Server, error) {
		seen = append(seen, r.Header.Get("Authorization"))
		return NewServer(testConnector{}), nil
	})
	server := httptest.NewServer(h)
	defer server.Close()

	for _, tc := range []struct {
		name string
		body string
		auth string
		want string
	}{
		{"initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "Bearer one", "protocolVersion"},
		{"tools", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "Bearer two", "ragflow_retrieval"},
		{"call", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ragflow_list_datasets"}}`, "Bearer three", "dataset-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(tc.body))
			req.Header.Set("Authorization", tc.auth)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(payload)
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("response %s does not contain %q", data, tc.want)
			}
		})
	}
	if len(seen) != 3 || seen[0] != "Bearer one" || seen[1] != "Bearer two" || seen[2] != "Bearer three" {
		t.Fatalf("authorization leaked or was not request-scoped: %#v", seen)
	}
}
