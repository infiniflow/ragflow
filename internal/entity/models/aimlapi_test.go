package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var aimlapiAttributionHeaderNames = []string{
	"X-AIMLAPI-Source",
	"X-AIMLAPI-Partner-ID",
	"HTTP-Referer",
	"X-Title",
}

func newAimlapiForTest(baseURL string) *AimlapiModel {
	return NewAimlapiModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "v1/chat/completions",
			Embedding: "v1/embeddings",
			Models:    "v1/models",
			Balance:   "v1/key",
		},
	)
}

// newAimlapiServer stands up an httptest server that checks the path and
// auth, records the request headers, and lets the caller answer.
func newAimlapiServer(t *testing.T, expectedPath string, seen *http.Header, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if seen != nil {
			*seen = r.Header.Clone()
		}
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read body: %v", err)
				return
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("invalid JSON body: %v\n%s", err, string(raw))
				return
			}
		}
		handler(t, body, w)
	}))
}

func TestAimlapiName(t *testing.T) {
	if got := newAimlapiForTest("http://unused").Name(); got != "aimlapi.com" {
		t.Errorf("Name()=%q, want %q", got, "aimlapi.com")
	}
}

func TestAimlapiFactoryRoute(t *testing.T) {
	// The provider name is the factory name saved in tenant configs.
	driver, err := NewModelFactory().CreateModelDriver("aimlapi.com", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AimlapiModel); !ok {
		t.Fatalf("driver type=%T, want *AimlapiModel (a miss falls through to DummyModel, which answers \"not implemented\")", driver)
	}
}

func TestAimlapiChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	thinking := true
	srv := newAimlapiServer(t, "/v1/chat/completions", nil, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "anthropic/claude-sonnet-5.5" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		// Reasoning controls differ per model behind the gateway; no
		// vendor-specific thinking field is sent.
		if _, ok := body["thinking"]; ok {
			t.Errorf("unexpected thinking field: %v", body["thinking"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "pong"}}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAimlapiForTest(srv.URL).ChatWithMessages(t.Context(), "anthropic/claude-sonnet-5.5",
		[]Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, &ChatConfig{Thinking: &thinking}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("answer=%v, want pong", resp.Answer)
	}
}

func TestAimlapiChatStream(t *testing.T) {
	withSSRFBypass(t)
	srv := newAimlapiServer(t, "/v1/chat/completions", nil, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		opts, _ := body["stream_options"].(map[string]interface{})
		if opts["include_usage"] != true {
			t.Errorf("stream_options=%v, want include_usage=true", body["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"Hello "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"world"}}]}`+"\n"+
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	var got strings.Builder
	sawDone := false
	err := newAimlapiForTest(srv.URL).ChatStreamlyWithSender(t.Context(), "openai/gpt-6.1-sol",
		[]Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, nil, nil,
		func(content *string, _ *string) error {
			if content == nil {
				return nil
			}
			// the end-of-stream sentinel is a transport signal, not text
			if *content == "[DONE]" {
				sawDone = true
				return nil
			}
			got.WriteString(*content)
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got.String() != "Hello world" {
		t.Errorf("streamed=%q, want %q", got.String(), "Hello world")
	}
	if !sawDone {
		t.Error("expected sender to receive the [DONE] sentinel")
	}
}

func TestAimlapiEmbedOrdersByIndex(t *testing.T) {
	withSSRFBypass(t)
	srv := newAimlapiServer(t, "/v1/embeddings", nil, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["dimensions"] != float64(256) {
			t.Errorf("dimensions=%v, want 256", body["dimensions"])
		}
		// returned out of order on purpose
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{2}, "index": 1},
				{"embedding": []float64{1}, "index": 0},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "openai/text-embedding-3-small"
	out, err := newAimlapiForTest(srv.URL).Embed(t.Context(), &model, EmbedRequest{Texts: []string{"a", "b"}},
		&APIConfig{ApiKey: &apiKey}, &EmbeddingConfig{Dimension: 256}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 2 || out[0].Embedding[0] != 1 || out[1].Embedding[0] != 2 {
		t.Errorf("embeddings not ordered by index: %+v", out)
	}
}

func TestAimlapiListModelsKeepsChatAndEmbeddingOnly(t *testing.T) {
	withSSRFBypass(t)
	ctx := 128000
	srv := newAimlapiServer(t, "/v1/models", nil, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				// one chat model published under BOTH chat types: one entry
				{"id": "anthropic/claude-haiku-5.5", "type": "openai/chat-completions", "info": map[string]interface{}{"contextLength": ctx}},
				{"id": "anthropic/claude-haiku-5.5", "type": "anthropic/messages", "info": map[string]interface{}{"contextLength": ctx}},
				{"id": "openai/text-embedding-3-small", "type": "openai/embeddings", "info": map[string]interface{}{}},
				// families this driver cannot call
				{"id": "flux/dev", "type": "openai/image-generations", "info": map[string]interface{}{}},
				{"id": "openai/gpt-6.1-sol", "type": "openai/responses/submit", "info": map[string]interface{}{}},
				{"id": "x/video", "type": "internal/video-generations/submit", "info": map[string]interface{}{}},
			},
		})
	})
	defer srv.Close()

	models, err := newAimlapiForTest(srv.URL).ListModels(t.Context(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	byName := map[string]ListModelResponse{}
	for _, m := range models {
		if _, dup := byName[m.Name]; dup {
			t.Errorf("model %q listed twice", m.Name)
		}
		byName[m.Name] = m
	}
	if len(byName) != 2 {
		t.Fatalf("got %d models %v, want exactly the chat and the embedding model", len(byName), byName)
	}
	if got := byName["openai/text-embedding-3-small"].ModelTypes; len(got) != 1 || got[0] != "embedding" {
		t.Errorf("embedding model types=%v, want [embedding]", got)
	}
	if c := byName["anthropic/claude-haiku-5.5"].ContextLength; c == nil || *c != ctx {
		t.Errorf("context length=%v, want %d", c, ctx)
	}
}

func TestAimlapiCheckConnectionUsesTheKeyEndpoint(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		status int
		ok     bool
	}{{http.StatusOK, true}, {http.StatusUnauthorized, false}} {
		srv := newAimlapiServer(t, "/v1/key", nil, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
			w.WriteHeader(tc.status)
			if tc.status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"limit": nil, "monthly_usage_usd": 1.5}})
				return
			}
			_, _ = io.WriteString(w, `{"message":"This request requires a valid API key"}`)
		})
		apiKey := "test-key"
		err := newAimlapiForTest(srv.URL).CheckConnection(t.Context(), &APIConfig{ApiKey: &apiKey})
		srv.Close()
		if (err == nil) != tc.ok {
			t.Errorf("status %d: CheckConnection err=%v, want ok=%v", tc.status, err, tc.ok)
		}
	}
}

func TestAimlapiUnsupportedMethods(t *testing.T) {
	m := newAimlapiForTest("http://unused")
	_, err := m.Rerank(t.Context(), nil, RerankRequest{}, nil, nil, nil)
	requireNoSuchMethod(t, "Rerank", err)
	_, err = m.OCRFile(t.Context(), nil, nil, nil, nil, nil, nil)
	requireNoSuchMethod(t, "OCRFile", err)
}

// --- attribution -----------------------------------------------------------

func attributionOn(rawURL string) http.Header {
	req, _ := http.NewRequest(http.MethodPost, rawURL, nil)
	applyAimlapiAttribution(req)
	return req.Header
}

func TestAimlapiAttributionForTheGateway(t *testing.T) {
	h := attributionOn("https://api.aimlapi.com/v1/chat/completions")
	if got := h.Get("X-AIMLAPI-Source"); got != "agent/ragflow" {
		t.Errorf("X-AIMLAPI-Source=%q", got)
	}
	if got := h.Get("X-AIMLAPI-Partner-ID"); got != aimlapiPartnerID {
		t.Errorf("X-AIMLAPI-Partner-ID=%q, want %q", got, aimlapiPartnerID)
	}
	if !regexp.MustCompile(`^part_[A-Za-z0-9]{1,64}$`).MatchString(aimlapiPartnerID) {
		t.Errorf("partner id %q is not a minted id", aimlapiPartnerID)
	}
	if h.Get("HTTP-Referer") == "" || h.Get("X-Title") != "RAGFlow" {
		t.Errorf("HTTP-Referer=%q X-Title=%q", h.Get("HTTP-Referer"), h.Get("X-Title"))
	}
}

func TestAimlapiAttributionNeverLeavesTheGatewayHost(t *testing.T) {
	for _, u := range []string{
		// look-alikes a suffix or substring test would attribute
		"https://api.aimlapi.com.example.net/v1/chat/completions",
		"https://notapi.aimlapi.com/v1/chat/completions",
		// userinfo: the real host is evil.example
		"https://api.aimlapi.com@evil.example/v1/chat/completions",
		// the apex and a sibling are not the inference gateway
		"https://aimlapi.com/v1/chat/completions",
		"https://staging.api.aimlapi.com/v1/chat/completions",
		// a base URL override: proxy, local
		"https://proxy.example.test/v1/chat/completions",
		"http://127.0.0.1:8080/v1/chat/completions",
	} {
		h := attributionOn(u)
		for _, name := range aimlapiAttributionHeaderNames {
			if h.Get(name) != "" {
				t.Errorf("%s leaked to %s", name, u)
			}
		}
	}
}

func TestAimlapiAttributionPartnerOverride(t *testing.T) {
	t.Setenv("AIMLAPI_PARTNER_ID", "part_stagingOverride")
	if got := attributionOn("https://api.aimlapi.com/v1/models").Get("X-AIMLAPI-Partner-ID"); got != "part_stagingOverride" {
		t.Errorf("override not honored: %q", got)
	}
}

type aimlapiRecordingTransport struct {
	headers map[string]http.Header
}

func (rt *aimlapiRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.headers[req.URL.Path] = req.Header.Clone()
	body := `{"choices":[{"message":{"content":"ok"}}]}`
	switch req.URL.Path {
	case "/v1/embeddings":
		body = `{"data":[{"embedding":[1],"index":0}]}`
	case "/v1/models":
		body = `{"data":[{"id":"m","type":"openai/chat-completions","info":{}}]}`
	case "/v1/key":
		body = `{"data":{"limit":null}}`
	}
	contentType := "application/json"
	if strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		contentType = "text/event-stream"
		body = `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n" + "data: [DONE]\n"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// TestAimlapiAttributionOnEveryRequestPath drives every network path of the
// driver at the real gateway URL through a recording transport, proving the
// hook is wired through the shared request helpers rather than only callable.
func TestAimlapiAttributionOnEveryRequestPath(t *testing.T) {
	rt := &aimlapiRecordingTransport{headers: map[string]http.Header{}}
	m := newAimlapiForTest("https://api.aimlapi.com")
	m.baseModel.httpClient = &http.Client{Transport: rt}
	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}
	msgs := []Message{{Role: "user", Content: "hi"}}
	model := "openai/text-embedding-3-small"

	if _, err := m.ChatWithMessages(t.Context(), "m", msgs, cfg, nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}
	chatHeaders := rt.headers["/v1/chat/completions"]
	if err := m.ChatStreamlyWithSender(t.Context(), "m", msgs, cfg, nil, nil, func(*string, *string) error { return nil }); err != nil {
		t.Fatalf("stream: %v", err)
	}
	streamHeaders := rt.headers["/v1/chat/completions"]
	if _, err := m.Embed(t.Context(), &model, EmbedRequest{Texts: []string{"a"}}, cfg, nil, nil); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if _, err := m.ListModels(t.Context(), cfg); err != nil {
		t.Fatalf("list models: %v", err)
	}
	if err := m.CheckConnection(t.Context(), cfg); err != nil {
		t.Fatalf("check connection: %v", err)
	}

	for path, h := range map[string]http.Header{
		"chat":       chatHeaders,
		"stream":     streamHeaders,
		"embeddings": rt.headers["/v1/embeddings"],
		"models":     rt.headers["/v1/models"],
		"key":        rt.headers["/v1/key"],
	} {
		if h == nil {
			t.Errorf("%s: request never reached the transport", path)
			continue
		}
		if got := h.Get("X-AIMLAPI-Partner-ID"); got != aimlapiPartnerID {
			t.Errorf("%s: X-AIMLAPI-Partner-ID=%q, want %q", path, got, aimlapiPartnerID)
		}
		if got := h.Get("X-AIMLAPI-Source"); got != "agent/ragflow" {
			t.Errorf("%s: X-AIMLAPI-Source=%q", path, got)
		}
		if got := h.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("%s: Authorization=%q", path, got)
		}
	}
}

// TestAimlapiAttributionNotSentToAnOverriddenBaseURL is the converse on the
// wire: the same driver pointed at another host (here a local server, as a
// proxy or staging override would be) sends no attribution at all.
func TestAimlapiAttributionNotSentToAnOverriddenBaseURL(t *testing.T) {
	withSSRFBypass(t)
	var seen http.Header
	srv := newAimlapiServer(t, "/v1/chat/completions", &seen, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	if _, err := newAimlapiForTest(srv.URL).ChatWithMessages(t.Context(), "m",
		[]Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}
	for _, name := range aimlapiAttributionHeaderNames {
		if seen.Get(name) != "" {
			t.Errorf("%s leaked to %s", name, srv.URL)
		}
	}
	if seen.Get("Authorization") != "Bearer test-key" {
		t.Errorf("Authorization=%q: the key still goes to the configured host", seen.Get("Authorization"))
	}
}

// TestRequestHeadersRunBeforeAuthentication pins the documented order in
// applyAuth: a driver's requestHeaders hook runs first, so a header it sets
// can never replace the authentication header.
func TestRequestHeadersRunBeforeAuthentication(t *testing.T) {
	b := BaseModel{requestHeaders: func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer from-the-hook")
		req.Header.Set("X-Driver-Header", "kept")
	}}
	req, _ := http.NewRequest(http.MethodPost, "https://api.aimlapi.com/v1/chat/completions", nil)
	apiKey := "test-key"
	b.applyAuth(req, &APIConfig{ApiKey: &apiKey})
	if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization=%q, want the API key to win over the hook", got)
	}
	if got := req.Header.Get("X-Driver-Header"); got != "kept" {
		t.Errorf("X-Driver-Header=%q, want the hook's own header kept", got)
	}
}

// --- provider config -------------------------------------------------------

func TestAimlapiProviderConfig(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "conf", "models", "aimlapi.json"))
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	var conf struct {
		Name      string            `json:"name"`
		Rank      int               `json:"rank"`
		URL       map[string]string `json:"url"`
		URLSuffix URLSuffix         `json:"url_suffix"`
		Models    []struct {
			Name       string   `json:"name"`
			ModelTypes []string `json:"model_types"`
			Alias      []string `json:"alias"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &conf); err != nil {
		t.Fatalf("parse conf: %v", err)
	}
	// the name is what the factory switches on, and what tenants have saved
	driver, err := NewModelFactory().CreateModelDriver(conf.Name, conf.URL, conf.URLSuffix)
	if err != nil {
		t.Fatalf("CreateModelDriver(%q): %v", conf.Name, err)
	}
	if _, ok := driver.(*AimlapiModel); !ok {
		t.Fatalf("conf name %q routes to %T, not *AimlapiModel", conf.Name, driver)
	}
	if conf.URL["default"] != "https://api.aimlapi.com" {
		t.Errorf("default url=%q", conf.URL["default"])
	}
	if conf.Rank != 995 {
		t.Errorf("rank=%d, want 995", conf.Rank)
	}
	seen := map[string]bool{}
	for _, m := range conf.Models {
		if seen[m.Name] {
			t.Errorf("model %q listed twice", m.Name)
		}
		seen[m.Name] = true
		if !strings.Contains(m.Name, "/") {
			t.Errorf("model %q is not vendor-prefixed", m.Name)
		}
		// unprefixed gateway aliases would collide across providers
		if len(m.Alias) != 0 {
			t.Errorf("model %q carries aliases %v", m.Name, m.Alias)
		}
		if len(m.ModelTypes) == 0 {
			t.Errorf("model %q has no model_types", m.Name)
		}
	}
}
