package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type headerTransport struct {
	base http.RoundTripper
	key  string
}

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.key)
	return t.base.RoundTrip(r)
}
func clientSession(t *testing.T, url, transport, key string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "parity-test", Version: "1"}, nil)
	hc := &http.Client{Transport: headerTransport{http.DefaultTransport, key}}
	var tr sdk.Transport
	if transport == "sse" {
		tr = &sdk.SSEClientTransport{Endpoint: url + "/sse", HTTPClient: hc}
	} else {
		tr = &sdk.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: hc}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	ss, err := client.Connect(ctx, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	return ss
}
func TestSDKTransportsAndTools(t *testing.T) {
	for _, mode := range []string{"json", "stream", "sse"} {
		t.Run(mode, func(t *testing.T) {
			var retrieval RetrievalRequest
			factory := func(user string) Connector {
				return NewServiceConnector(user,
					func(ctx context.Context, user string, page, size int, _ string, _ bool) ([]map[string]any, int64, error) {
						if user == "empty" {
							return nil, 0, nil
						}
						return []map[string]any{{"id": user, "name": "Dataset", "description": nil}}, 1, nil
					}, func(context.Context, string, int, int, string, bool) ([]map[string]any, int64, error) {
						return nil, 0, nil
					},
					func(ctx context.Context, user string, req RetrievalRequest) (string, error) {
						retrieval = req
						return `{"chunks":[],"query_info":{"question":"hello"}}`, nil
					})
			}
			h := NewHandler(func(_ context.Context, key string) (string, error) {
				if key == "" || key == "invalid" {
					return "", fmt.Errorf("denied")
				}
				return key, nil
			}, factory, Options{true, true, mode == "json"})
			hs := httptest.NewServer(h)
			defer hs.Close()
			defer h.Close()
			ss := clientSession(t, hs.URL, mode, "alice")
			tools, err := ss.ListTools(t.Context(), nil)
			if err != nil || len(tools.Tools) != 3 {
				t.Fatalf("tools=%v err=%v", tools, err)
			}
			var expected []*sdk.Tool
			if err := json.Unmarshal(toolsJSON, &expected); err != nil {
				t.Fatal(err)
			}
			for i, tool := range tools.Tools {
				got, _ := json.Marshal(tool.InputSchema)
				want, _ := json.Marshal(expected[i].InputSchema)
				if string(got) != string(want) {
					t.Fatalf("schema mismatch: %s vs %s", got, want)
				}
			}
			result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: "ragflow_retrieval", Arguments: map[string]any{"question": "hello"}})
			if err != nil || result.IsError {
				t.Fatalf("retrieval: %v %v", result, err)
			}
			if retrieval.Page != 1 || retrieval.PageSize != 10 || retrieval.TopK != 1024 || retrieval.SimilarityThreshold != .2 || retrieval.VectorSimilarityWeight != .3 {
				t.Fatalf("defaults: %+v", retrieval)
			}
			for _, name := range []string{"ragflow_list_datasets", "ragflow_list_chats"} {
				result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: name})
				if err != nil || result.IsError {
					t.Fatalf("%s: %v %v", name, result, err)
				}
				if name == "ragflow_list_chats" {
					b, _ := json.Marshal(result)
					if !strings.Contains(string(b), `"text":""`) {
						t.Fatalf("empty text missing: %s", b)
					}
				}
			}
			for _, args := range []map[string]any{{}, {"question": 3}, {"question": "x", "page": 0}, {"question": "x", "page": 1.5}, {"question": "x", "page_size": 101}, {"question": "x", "top_k": 1025}, {"question": "x", "keyword": "true"}, {"question": "x", "dataset_ids": []any{3}}, {"question": "x", "similarity_threshold": -1}} {
				result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: "ragflow_retrieval", Arguments: args})
				if err != nil || !result.IsError {
					t.Fatalf("invalid input accepted: %v result=%v err=%v", args, result, err)
				}
			}
			empty := clientSession(t, hs.URL, mode, "empty")
			result, err = empty.CallTool(t.Context(), &sdk.CallToolParams{Name: "ragflow_list_datasets"})
			if err != nil || result.IsError || result.Content[0].(*sdk.TextContent).Text != "" {
				t.Fatalf("empty: %v %v", result, err)
			}
		})
	}
}
func TestCredential(t *testing.T) {
	for _, header := range []string{"Authorization", "api_key", "X-API-Key", "Api-Key"} {
		h := http.Header{}
		value := " secret "
		if header == "Authorization" {
			value = " bEaReR secret "
		}
		h.Set(header, value)
		if got := Credential(h); got != "secret" {
			t.Fatalf("%s=%q", header, got)
		}
	}
	if Credential(http.Header{"Authorization": []string{"Basic secret"}}) != "" {
		t.Fatal("accepted Basic")
	}
}
func TestConnectorPaginationAndNulls(t *testing.T) {
	var pages, sizes []int
	c := NewServiceConnector("tenant", func(_ context.Context, user string, page, size int, _ string, _ bool) ([]map[string]any, int64, error) {
		pages = append(pages, page)
		sizes = append(sizes, size)
		return []map[string]any{{"id": fmt.Sprint(page)}}, 2, nil
	}, nil, nil)
	text, err := c.ListDatasets(t.Context(), 1, -1, "create_time", true)
	if err != nil || !reflect.DeepEqual(pages, []int{1, 2}) || !strings.Contains(text, `"description":null`) {
		t.Fatalf("pages=%v text=%s err=%v", pages, text, err)
	}
	_, err = c.ListDatasets(t.Context(), 3, 1000, "create_time", true)
	if err != nil || sizes[len(sizes)-1] != 100 {
		t.Fatalf("cap: %v %v", sizes, err)
	}
}

func TestConcurrentTenantsAndSSEOwnership(t *testing.T) {
	h := NewHandler(func(_ context.Context, key string) (string, error) {
		if key != "alice" && key != "bob" {
			return "", fmt.Errorf("denied")
		}
		return key, nil
	}, func(user string) Connector {
		return NewServiceConnector(user, func(_ context.Context, user string, _, _ int, _ string, _ bool) ([]map[string]any, int64, error) {
			return []map[string]any{{"id": user}}, 1, nil
		}, func(context.Context, string, int, int, string, bool) ([]map[string]any, int64, error) {
			return nil, 0, nil
		}, nil)
	}, Options{true, true, true})
	hs := httptest.NewServer(h)
	defer hs.Close()
	defer h.Close()
	for _, transport := range []string{"sse", "json"} {
		a := clientSession(t, hs.URL, transport, "alice")
		b := clientSession(t, hs.URL, transport, "bob")
		errs := make(chan error, 2)
		for i, ss := range []*sdk.ClientSession{a, b} {
			go func() {
				expected := []string{"alice", "bob"}[i]
				for range 5 {
					res, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: "ragflow_list_datasets"})
					if err != nil {
						errs <- err
						return
					}
					if res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, expected) {
						errs <- fmt.Errorf("identity leak: %v", res)
						return
					}
				}
				errs <- nil
			}()
		}
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		a.Close()
		b.Close()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", hs.URL+"/sse", nil)
	req.Header.Set("X-API-Key", "alice")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	var endpoint string
	for endpoint == "" {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") {
			endpoint = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
	for _, key := range []string{"bob", "invalid", "alice"} {
		request, _ := http.NewRequestWithContext(t.Context(), "POST", hs.URL+endpoint, strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		want := map[string]int{"bob": 404, "invalid": 401, "alice": 202}[key]
		if r.StatusCode != want {
			t.Fatalf("%s got %d want %d", key, r.StatusCode, want)
		}
	}
	h.Close()
	done := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, reader); done <- err }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not close SSE")
	}
}

func TestProtocolErrorsAndCancellation(t *testing.T) {
	started := make(chan context.Context, 1)
	h := NewHandler(func(_ context.Context, key string) (string, error) { return key, nil }, func(user string) Connector {
		return NewServiceConnector(user, func(context.Context, string, int, int, string, bool) ([]map[string]any, int64, error) {
			return nil, 0, nil
		}, func(context.Context, string, int, int, string, bool) ([]map[string]any, int64, error) {
			return nil, 0, nil
		}, func(ctx context.Context, _ string, _ RetrievalRequest) (string, error) {
			started <- ctx
			<-ctx.Done()
			return "", ctx.Err()
		})
	}, Options{true, true, true})
	hs := httptest.NewServer(h)
	defer hs.Close()
	defer h.Close()
	for _, tc := range []struct {
		method, path, body, accept, content string
		status                              int
	}{
		{"GET", "/mcp", "", "application/json", "application/json", 405},
		{"DELETE", "/mcp", "", "application/json", "application/json", 405},
		{"POST", "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, "application/json, text/event-stream", "application/json", 202},
		{"POST", "/mcp", `{`, "application/json, text/event-stream", "application/json", 400},
		{"POST", "/mcp", `{}`, "application/json, text/event-stream", "text/plain", 415},
	} {
		req, _ := http.NewRequest(tc.method, hs.URL+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer alice")
		req.Header.Set("Accept", tc.accept)
		req.Header.Set("Content-Type", tc.content)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, res.StatusCode, body)
		}
	}
	ss := clientSession(t, hs.URL, "json", "alice")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ss.CallTool(ctx, &sdk.CallToolParams{Name: "ragflow_retrieval", Arguments: map[string]any{"question": "x"}})
	}()
	var callctx context.Context
	select {
	case callctx = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("call did not start")
	}
	if deadline, ok := callctx.Deadline(); !ok || time.Until(deadline) > 60*time.Second {
		t.Fatal("missing tool deadline")
	}
	cancel()
	select {
	case <-callctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not reach service")
	}
	<-done
}

func TestTransportsRejectLoopbackDNSRebinding(t *testing.T) {
	for _, path := range []string{"/sse", "/messages/", "/mcp"} {
		for _, tc := range []struct {
			local, host string
			blocked     bool
		}{
			{"127.0.0.1", "attacker.example", true}, {"::1", "localhost.attacker.example", true},
			{"127.0.0.1", "localhost:9382", false}, {"::1", "[::1]:9382", false},
			{"127.0.0.1", "127.0.0.2:9382", false}, {"192.0.2.1", "mcp.example", false},
		} {
			t.Run(path+tc.local+tc.host, func(t *testing.T) {
				called := false
				h := NewHandler(func(context.Context, string) (string, error) { called = true; return "", fmt.Errorf("no credentials") }, nil, Options{true, true, true})
				defer h.Close()
				r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+path, nil)
				r.Header.Set("Origin", "http://"+tc.host)
				r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(tc.local), Port: 9382}))
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				want := http.StatusUnauthorized
				if tc.blocked {
					want = http.StatusForbidden
				}
				if w.Code != want || called == tc.blocked {
					t.Fatalf("status=%d authorization called=%v; want status=%d", w.Code, called, want)
				}
			})
		}
	}
}

func BenchmarkMCPLoopbackRequest(b *testing.B) {
	h := NewHandler(func(context.Context, string) (string, error) { return "user", nil }, nil, Options{true, true, true})
	defer h.Close()
	r := httptest.NewRequest(http.MethodGet, "http://localhost/unknown", nil)
	r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9382}))
	b.ReportAllocs()
	for b.Loop() {
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
}
