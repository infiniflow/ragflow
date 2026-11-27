//go:build e2e

package service

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Requires a running Go API and actual OpenAI-compatible inference servers.
// The supplied token must belong to a dedicated local validation account.
// Fixtures are manually indexed through the public chunk API; ingestion is not
// exercised. The forwarding observer never supplies a model response.
func TestAnySearchRealModelE2E(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH_E2E") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH_E2E=1 for actual inference and AnySearch")
	}
	required := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}
	apiURL := required("RAGFLOW_TEST_API_URL")
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme != "http" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("RAGFLOW_TEST_API_URL must be a local HTTP validation server")
	}
	h := &anySearchE2EHTTP{t: t, origin: strings.TrimRight(apiURL, "/"), token: required("RAGFLOW_TEST_API_TOKEN"), client: &http.Client{Timeout: 10 * time.Minute}}
	maxTokens := 1024
	if value := strings.TrimSpace(os.Getenv("RAGFLOW_TEST_LLM_MAX_TOKENS")); value != "" {
		maxTokens, err = strconv.Atoi(value)
		if err != nil || maxTokens <= 0 {
			t.Fatal("RAGFLOW_TEST_LLM_MAX_TOKENS must be a positive integer")
		}
	}
	chatModel, embedModel := required("RAGFLOW_TEST_LLM_MODEL"), required("RAGFLOW_TEST_EMBEDDING_MODEL")
	observer, modelURL := newAnySearchModelObserver(t, required("RAGFLOW_TEST_LLM_BASE_URL"))
	embedURL := required("RAGFLOW_TEST_EMBEDDING_BASE_URL")
	nonce := fmt.Sprintf("%x", time.Now().UnixNano())
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	provider := "/api/v1/providers/OpenAI-API-Compatible"
	// This is idempotent for an already added provider. Never remove the provider
	// itself: it can own instances outside this run.
	h.call(http.MethodPut, "/api/v1/providers", map[string]any{"provider_name": "OpenAI-API-Compatible"})
	addModel := func(kind, name, base string, window int) string {
		instance := "anysearch_e2e_" + kind + nonce
		h.call(http.MethodPost, provider+"/instances", map[string]any{"instance_name": instance, "api_key": "", "base_url": base,
			"model_info": []any{map[string]any{"model_name": name, "model_type": []string{kind}, "max_tokens": window, "extra": map[string]any{"is_tools": true}}}})
		t.Cleanup(func() {
			h.cleanup(http.MethodDelete, provider+"/instances", map[string]any{"instances": []string{instance}})
		})
		var rows []struct {
			ID       string `json:"model_id"`
			Instance string `json:"instance_name"`
		}
		h.decode(h.call(http.MethodGet, "/api/v1/models?type="+kind, nil), &rows)
		for _, row := range rows {
			if row.Instance == instance && row.ID != "" {
				return row.ID
			}
		}
		t.Fatalf("created %s model not listed", kind)
		return ""
	}
	modelID := addModel("chat", chatModel, modelURL, 4096)
	embedID := addModel("embedding", embedModel, embedURL, 512)
	var kb struct {
		ID string `json:"id"`
	}
	h.decode(h.call(http.MethodPost, "/api/v1/datasets", map[string]any{"name": "AnySearch E2E " + nonce, "embedding_model": embedID, "parse_type": 1, "parser_id": "naive", "language": "English"}), &kb)
	if kb.ID == "" {
		t.Fatal("dataset ID missing")
	}
	t.Cleanup(func() { h.cleanup(http.MethodDelete, "/api/v1/datasets", map[string]any{"ids": []string{kb.ID}}) })
	h.call(http.MethodPost, "/api/v1/tenant/chunk-store", map[string]any{"kb_id": kb.ID, "vector_size": 384})
	var doc struct {
		ID string `json:"id"`
	}
	h.decode(h.call(http.MethodPost, "/api/v1/datasets/"+kb.ID+"/documents?type=empty", map[string]any{"name": "ragflow-validation.txt"}), &doc)
	if doc.ID == "" {
		t.Fatal("document ID missing")
	}
	marker := "RAGFlow validation code " + nonce
	chunkPath := "/api/v1/datasets/" + kb.ID + "/documents/" + doc.ID + "/chunks"
	h.call(http.MethodPost, chunkPath, map[string]any{"content": marker + ". RAGFlow is an open source retrieval augmented generation system."})
	// Read the stored chunk, rather than treating a successful create as indexing.
	readback := h.call(http.MethodGet, chunkPath, nil)
	if !bytes.Contains(readback, []byte(marker)) {
		t.Fatal("indexed chunk readback lost the unique corpus marker")
	}

	for _, mode := range []struct {
		name      string
		reasoning int
	}{{"normal_chat", 0}, {"agentic_kb_chat", 5}} {
		t.Run(mode.name, func(t *testing.T) {
			local := *h
			local.t = t
			observer.reset()
			var chat struct {
				ID string `json:"id"`
			}
			promptConfig := map[string]any{"system": "Use the supplied knowledge. {knowledge}", "parameters": []any{map[string]any{"key": "knowledge", "optional": false}}, "quote": true, "empty_response": "", "reasoning": mode.reasoning, "web_search_provider": "anysearch", "anysearch_api_key": key}
			if mode.reasoning == 5 {
				promptConfig["agentic_rag"] = map[string]any{
					"tools":   []string{"search_semantic_chunks", "list_chunks"},
					"content": "Answer only from retrieved corpus and web evidence. Find the corpus validation code and RAGFlow official documentation using the available search tools. Return exactly two short factual sentences: quote the corpus validation code verbatim in one and a returned web documentation URL verbatim in the other. End each sentence with its matching [ID:n] citation, using n from that chunk's ref attribute. chunk_id and doc_id are not citation handles. Do not repeat completed searches.",
				}
			}
			local.decode(local.call(http.MethodPost, "/api/v1/chats", map[string]any{"name": mode.name + nonce, "dataset_ids": []string{kb.ID}, "llm_id": modelID,
				"similarity_threshold": 0, "top_n": 1, "llm_setting": map[string]any{"temperature": 0.2, "max_tokens": maxTokens},
				"prompt_config": promptConfig}), &chat)
			if chat.ID == "" {
				t.Fatal("chat ID missing")
			}
			t.Cleanup(func() { local.cleanup(http.MethodDelete, "/api/v1/chats/"+chat.ID, nil) })
			var session struct {
				ID string `json:"id"`
			}
			local.decode(local.call(http.MethodPost, "/api/v1/chats/"+chat.ID+"/sessions", map[string]any{"name": mode.name}), &session)
			if session.ID == "" {
				t.Fatal("session ID missing")
			}
			t.Cleanup(func() {
				local.cleanup(http.MethodDelete, "/api/v1/chats/"+chat.ID+"/sessions", map[string]any{"ids": []string{session.ID}})
			})
			var result struct {
				Answer    string          `json:"answer"`
				Reference json.RawMessage `json:"reference"`
			}
			local.decode(local.call(http.MethodPost, "/api/v1/chat/completions", map[string]any{"chat_id": chat.ID, "session_id": session.ID,
				"question": "Find the RAGFlow validation code in the corpus and search the web for RAGFlow official documentation. Report both sources.", "internet": true, "stream": false, "max_tokens": maxTokens}), &result)
			local.receipt(mode.name+"-response", result)
			local.receipt(mode.name+"-model", observer.snapshot())
			if strings.TrimSpace(result.Answer) == "" || strings.Contains(result.Answer, "**ERROR**") {
				t.Fatal("actual model returned an empty or error answer")
			}
			requests := observer.assertInference(t)
			if !strings.Contains(requests, marker) {
				t.Fatal("actual inference requests never received the indexed corpus marker")
			}
			if mode.reasoning == 5 && !strings.Contains(requests, `"role":"tool"`) {
				t.Fatal("agentic model never received a tool result")
			}
			sourceURLs := observer.chatWebSourceURLs(t, mode.reasoning == 5, result.Reference)
			assertAnySearchChatReferences(t, result.Reference, marker, sourceURLs)
			saved := local.call(http.MethodGet, "/api/v1/chats/"+chat.ID+"/sessions/"+session.ID+"?include_reference=true", nil)
			local.receipt(mode.name+"-saved", json.RawMessage(saved))
			var persisted struct {
				Messages []struct {
					Role      string          `json:"role"`
					Reference json.RawMessage `json:"reference"`
				} `json:"messages"`
			}
			local.decode(saved, &persisted)
			var savedReferences json.RawMessage
			for i := len(persisted.Messages) - 1; i >= 0; i-- {
				if persisted.Messages[i].Role == "assistant" {
					savedReferences = persisted.Messages[i].Reference
					break
				}
			}
			assertAnySearchChatReferences(t, savedReferences, marker, sourceURLs)
			t.Log("actual model inference, indexed corpus and live AnySearch reached saved Chat")
		})
	}

	for _, name := range []string{"standalone_canvas", "standalone_extract", "workflow_agent"} {
		agent := name == "workflow_agent"
		t.Run(name, func(t *testing.T) {
			local := *h
			local.t = t
			observer.reset()
			dsl, nodeID := anySearchRealModelDSL(name, modelID, key, maxTokens)
			var c struct {
				ID string `json:"id"`
			}
			local.decode(local.call(http.MethodPost, "/api/v1/agents", map[string]any{"title": name + nonce, "canvas_category": "agent_canvas", "dsl": dsl}), &c)
			if c.ID == "" {
				t.Fatal("canvas ID missing")
			}
			t.Cleanup(func() { local.cleanup(http.MethodDelete, "/api/v1/agents/"+c.ID, nil) })
			t.Cleanup(func() {
				local.cleanup(http.MethodDelete, "/api/v1/agents/"+c.ID+"/sessions", map[string]any{"delete_all": true})
			})
			events := local.stream("/api/v1/agents/chat/completions", map[string]any{"agent_id": c.ID, "query": "Search the web for RAGFlow official documentation using anysearch_search and report the source URL.", "stream": true})
			var finished map[string]any
			var answer, session string
			for _, event := range events {
				if id, ok := event["session_id"].(string); ok {
					session = id
				}
				data, _ := event["data"].(map[string]any)
				if event["event"] == "error" {
					t.Fatal("canvas emitted an error event")
				}
				if event["event"] == "message" {
					text, _ := data["content"].(string)
					answer += text
				}
				if event["event"] == "node_finished" && data["component_id"] == nodeID {
					finished = data
				}
			}
			if finished == nil || strings.TrimSpace(answer) == "" || session == "" {
				t.Fatal("canvas did not finish its node and produce a saved nonempty answer")
			}
			out, _ := json.Marshal(finished["outputs"])
			local.receipt(name+"-node", finished)
			local.receipt(name+"-model", observer.snapshot())
			if !agent && (!bytes.Contains(out, []byte("request_id")) || !bytes.Contains(out, []byte("https://"))) {
				t.Fatal("node output has no actual AnySearch request receipt/source")
			}
			if name == "standalone_extract" && !bytes.Contains(out, []byte("Example Domain")) {
				t.Fatal("extraction did not deliver the real page content")
			}
			if agent {
				observer.assertInference(t)
				observer.assertAnySearchToolResult(t, finished["outputs"], answer)
			}
			local.call(http.MethodGet, "/api/v1/agents/"+c.ID+"/sessions/"+session, nil)
			t.Logf("saved %s executed actual AnySearch; model inference required=%v", name, agent)
		})
	}
}

type anySearchChatReference struct {
	Chunks []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		URL     string `json:"url"`
	} `json:"chunks"`
}

func assertAnySearchChatReferences(t *testing.T, raw json.RawMessage, marker string, sourceURLs map[string]bool) {
	t.Helper()
	var reference anySearchChatReference
	if json.Unmarshal(raw, &reference) != nil {
		t.Fatal("Chat reference payload is missing or malformed")
	}
	corpus, web := false, false
	for _, chunk := range reference.Chunks {
		corpus = corpus || strings.Contains(chunk.Content, marker)
		web = web || sourceURLs[chunk.URL]
	}
	if !corpus || !web {
		t.Fatal("Chat references must preserve both the indexed corpus marker and a web source actually delivered to inference")
	}
}

func (o *anySearchModelObserver) chatWebSourceURLs(t *testing.T, agentic bool, rawReference json.RawMessage) map[string]bool {
	t.Helper()
	sources := map[string]bool{}
	var reference anySearchChatReference
	if !agentic && json.Unmarshal(rawReference, &reference) != nil {
		t.Fatal("invalid standard Chat reference payload")
	}
	for _, raw := range o.snapshot()["requests"] {
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(raw), &request) != nil {
			t.Fatal("invalid observed model request")
		}
		for _, message := range request.Messages {
			var content string
			if json.Unmarshal(message.Content, &content) != nil {
				continue
			}
			if !agentic {
				for _, chunk := range reference.Chunks {
					if strings.HasPrefix(chunk.ID, "anysearch-") && chunk.URL != "" && strings.Contains(content, chunk.URL) {
						sources[chunk.URL] = true
					}
				}
				continue
			}
			if message.Role != "tool" {
				continue
			}
			var results struct {
				XMLName xml.Name `xml:"search_results"`
				Chunks  []struct {
					Source string `xml:"source,attr"`
					URL    string `xml:"url,attr"`
					Ref    string `xml:"ref,attr"`
				} `xml:"chunk"`
			}
			if xml.Unmarshal([]byte(content), &results) != nil {
				continue
			}
			for _, chunk := range results.Chunks {
				if chunk.Source == "web" && chunk.Ref != "" {
					sources[chunk.URL] = true
				}
			}
		}
	}
	for source := range sources {
		u, err := url.Parse(source)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			delete(sources, source)
		}
	}
	if len(sources) == 0 {
		t.Fatal("no actual web source URL reached model inference")
	}
	return sources
}

type anySearchE2EHTTP struct {
	t             *testing.T
	origin, token string
	client        *http.Client
}

func (h *anySearchE2EHTTP) receipt(name string, value any) {
	h.t.Helper()
	dir := os.Getenv("RAGFLOW_ANYSEARCH_E2E_RECEIPTS")
	if dir == "" {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		h.t.Fatal(err)
	}
	for _, secret := range []string{h.token, strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY")), strings.TrimSpace(os.Getenv("RAGFLOW_TEST_LLM_API_KEY"))} {
		if secret != "" && bytes.Contains(data, []byte(secret)) {
			h.t.Fatal("receipt would expose a credential")
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *anySearchE2EHTTP) request(method, path string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, h.origin+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", h.token)
	res, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s transport failed", method, path)
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%s %s read failed", method, path)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s HTTP %d", method, path, res.StatusCode)
	}
	return data, nil
}
func (h *anySearchE2EHTTP) call(method, path string, body any) json.RawMessage {
	h.t.Helper()
	data, err := h.request(method, path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code != 0 {
		h.t.Fatalf("%s %s invalid response or business failure (body withheld)", method, path)
	}
	return envelope.Data
}
func (h *anySearchE2EHTTP) decode(data []byte, out any) {
	h.t.Helper()
	if json.Unmarshal(data, out) != nil {
		h.t.Fatal("unexpected API response shape")
	}
}
func (h *anySearchE2EHTTP) cleanup(method, path string, body any) {
	data, err := h.request(method, path, body)
	var envelope struct {
		Code int `json:"code"`
	}
	if err != nil || json.Unmarshal(data, &envelope) != nil || envelope.Code != 0 {
		h.t.Errorf("cleanup failed: %s %s (body withheld)", method, path)
	}
}
func (h *anySearchE2EHTTP) stream(path string, body any) []map[string]any {
	h.t.Helper()
	raw, err := h.request(http.MethodPost, path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	var events []map[string]any
	done := false
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if value == "[DONE]" {
			done = true
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(value), &event) != nil {
			h.t.Fatal("invalid canvas SSE data")
		}
		if code, ok := event["code"].(float64); ok && code != 0 {
			h.t.Fatal("canvas stream business failure")
		}
		if event["event"] == "done" || event["data"] == true {
			done = true
		}
		events = append(events, event)
	}
	if !done {
		h.t.Fatal("canvas stream has no terminal event")
	}
	return events
}

// The observer forwards bytes unchanged to the configured inference server and
// retains only bounded, in-memory requests/responses for assertions.
type anySearchModelObserver struct {
	mu                  sync.Mutex
	requests, responses []string
	captureError        string
}
type anySearchObservedBody struct {
	io.ReadCloser
	observer  *anySearchModelObserver
	body      bytes.Buffer
	truncated bool
}

func (b *anySearchObservedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	remaining := (4 << 20) - b.body.Len()
	captured := n
	if captured > remaining {
		captured = remaining
		b.truncated = true
	}
	b.body.Write(p[:captured])
	return n, err
}
func (b *anySearchObservedBody) Close() error {
	b.observer.mu.Lock()
	b.observer.responses = append(b.observer.responses, b.body.String())
	if b.truncated {
		b.observer.captureError = "model response exceeded the observation limit"
	}
	b.observer.mu.Unlock()
	return b.ReadCloser.Close()
}
func newAnySearchModelObserver(t *testing.T, base string) (*anySearchModelObserver, string) {
	t.Helper()
	target, err := url.Parse(base)
	if err != nil || target.Host == "" || target.RawQuery != "" || target.User != nil {
		t.Fatal("invalid model base URL")
	}
	basePath := strings.TrimRight(target.Path, "/")
	target.Path = ""
	o := &anySearchModelObserver{}
	proxy := httputil.NewSingleHostReverseProxy(target)
	apiKey := strings.TrimSpace(os.Getenv("RAGFLOW_TEST_LLM_API_KEY"))
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
		if apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.StatusCode == http.StatusOK {
			r.Body = &anySearchObservedBody{ReadCloser: r.Body, observer: o}
		}
		return nil
	}
	proxy.FlushInterval = -1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
		r.Body.Close()
		if readErr != nil || len(raw) > 4<<20 {
			o.mu.Lock()
			o.captureError = "model request could not be captured completely"
			o.mu.Unlock()
			http.Error(w, "model observation limit or read failure", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		o.mu.Lock()
		o.requests = append(o.requests, string(raw))
		o.mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return o, server.URL + basePath
}
func TestAnySearchModelObserverForwardsHost(t *testing.T) {
	const body = `{"model":"test-model","messages":[]}`
	var expectedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != expectedHost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("forwarded authority/path = %q %q", r.Host, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		got, err := io.ReadAll(r.Body)
		if err != nil || string(got) != body {
			t.Error("forwarding changed the model request body")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	expectedHost = parsed.Host
	_, base := newAnySearchModelObserver(t, upstream.URL+"/v1")
	response, err := http.Post(base+"/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("forwarded model status = %d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
}

func (o *anySearchModelObserver) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests = nil
	o.responses = nil
	o.captureError = ""
}
func (o *anySearchModelObserver) snapshot() map[string][]string {
	o.mu.Lock()
	defer o.mu.Unlock()
	snapshot := map[string][]string{"requests": append([]string(nil), o.requests...), "responses": append([]string(nil), o.responses...)}
	if o.captureError != "" {
		snapshot["capture_errors"] = []string{o.captureError}
	}
	return snapshot
}
func (o *anySearchModelObserver) assertInference(t *testing.T) string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.captureError != "" {
		t.Fatal(o.captureError)
	}
	if len(o.requests) == 0 || len(o.responses) == 0 {
		t.Fatal("actual inference server was not called successfully")
	}
	generated := false
	for _, response := range o.responses {
		frames := []string{response}
		if strings.HasPrefix(strings.TrimSpace(response), "data:") {
			frames = strings.Split(response, "\n")
		}
		for _, frame := range frames {
			var completion struct {
				Choices []struct {
					Message struct {
						Content   string            `json:"content"`
						ToolCalls []json.RawMessage `json:"tool_calls"`
					} `json:"message"`
					Delta struct {
						Content   string            `json:"content"`
						ToolCalls []json.RawMessage `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(frame, "data:"))), &completion) != nil {
				continue
			}
			for _, choice := range completion.Choices {
				generated = generated || strings.TrimSpace(choice.Message.Content) != "" || strings.TrimSpace(choice.Delta.Content) != "" || len(choice.Message.ToolCalls) != 0 || len(choice.Delta.ToolCalls) != 0
			}
		}
	}
	if !generated {
		t.Fatal("inference server returned no model content or tool calls")
	}
	// Normalize JSON whitespace before inspecting the real model request.
	var compact bytes.Buffer
	for _, request := range o.requests {
		var sent struct {
			Model string `json:"model"`
		}
		if json.Unmarshal([]byte(request), &sent) != nil || sent.Model != strings.TrimSpace(os.Getenv("RAGFLOW_TEST_LLM_MODEL")) {
			t.Fatal("inference request did not use the configured model name")
		}
		if json.Compact(&compact, []byte(request)) != nil {
			t.Fatal("invalid model request JSON")
		}
	}
	return compact.String()
}

func (o *anySearchModelObserver) assertAnySearchToolResult(t *testing.T, outputs any, answer string) {
	t.Helper()
	output, ok := outputs.(map[string]any)
	if !ok {
		t.Fatal("workflow node returned no structured outputs")
	}
	invocations, _ := output["tool_invocations"].([]any)
	successfulQueries := map[string]bool{}
	for _, raw := range invocations {
		invocation, ok := raw.(map[string]any)
		if !ok || invocation["tool_name"] != "anysearch_search" || invocation["status"] != "success" {
			continue
		}
		arguments, _ := invocation["arguments"].(map[string]any)
		query, _ := arguments["query"].(string)
		if strings.TrimSpace(query) != "" {
			successfulQueries[query] = true
		}
	}
	if len(successfulQueries) == 0 {
		t.Fatal("workflow has no successful AnySearch invocation with a query")
	}
	for _, raw := range o.snapshot()["requests"] {
		var request struct {
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
				ToolCalls  []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(raw), &request) != nil {
			t.Fatal("invalid observed model request")
		}
		calls := map[string]string{}
		for _, message := range request.Messages {
			if message.Role == "assistant" {
				for _, call := range message.ToolCalls {
					var args struct {
						Query string `json:"query"`
					}
					if call.ID != "" && call.Function.Name == "anysearch_search" && json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && successfulQueries[args.Query] {
						calls[call.ID] = args.Query
					}
				}
			}
			query, matched := calls[message.ToolCallID]
			if message.Role != "tool" || !matched {
				continue
			}
			var content string
			var reply struct {
				Error   string `json:"_ERROR"`
				Queries []struct {
					Index     int    `json:"index"`
					Query     string `json:"query"`
					RequestID string `json:"request_id"`
					Error     string `json:"_ERROR"`
					Results   []struct {
						URL string `json:"url"`
					} `json:"results"`
				} `json:"queries"`
			}
			// The invocation log is only a bounded preview. Inspect the complete
			// tool message actually delivered to inference instead.
			if json.Unmarshal(message.Content, &content) != nil || json.Unmarshal([]byte(content), &reply) != nil || reply.Error != "" || len(reply.Queries) != 1 {
				continue
			}
			result := reply.Queries[0]
			if result.Index != 0 || result.Query != query || result.Error != "" || strings.TrimSpace(result.RequestID) == "" || len(result.Results) == 0 {
				continue
			}
			for _, source := range result.Results {
				u, err := url.Parse(source.URL)
				if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && strings.Contains(answer, source.URL) {
					t.Logf("actual inference consumed successful AnySearch result request_id=%s and cited its URL", result.RequestID)
					return
				}
			}
		}
	}
	t.Fatal("workflow answer has no source URL correlated with a successful AnySearch result received by inference")
}

func anySearchRealModelDSL(mode, modelID, key string, maxTokens int) (map[string]any, string) {
	id, component, nodeType := "AnySearchSearch:live", "AnySearchSearch", "ragNode"
	params := map[string]any{"api_key": key, "query": "sys.query", "top_n": 1}
	output := "formalized_content"
	if mode == "standalone_extract" {
		id, component = "AnySearchExtract:live", "AnySearchExtract"
		params = map[string]any{"api_key": key, "url": "https://example.com/"}
	}
	if mode == "workflow_agent" {
		id, component, nodeType = "Agent:live", "Agent", "agentNode"
		output = "content"
		params = map[string]any{"llm_id": modelID, "max_rounds": 2, "max_tokens": maxTokens, "temperature": 0.2, "sys_prompt": "Use anysearch_search to answer the user. For this general search, supply only query; omit queries, tag and params. After reading the result, answer with its source URL.", "prompts": []any{map[string]any{"role": "user", "content": "{sys.query}"}}, "tools": []any{map[string]any{"component_name": "AnySearchSearch", "params": map[string]any{"api_key": key, "top_n": 1}}}}
	}
	components := map[string]any{}
	nodes := []any{}
	edges := []any{}
	for i, spec := range []struct {
		id, name, kind string
		params         map[string]any
		up, down       []string
	}{{"begin", "Begin", "beginNode", map[string]any{"mode": "conversational", "prologue": ""}, []string{}, []string{id}}, {id, component, nodeType, params, []string{"begin"}, []string{"Message:live"}}, {"Message:live", "Message", "messageNode", map[string]any{"content": []string{"{" + id + "@" + output + "}"}}, []string{id}, []string{}}} {
		components[spec.id] = map[string]any{"obj": map[string]any{"component_name": spec.name, "params": spec.params}, "upstream": spec.up, "downstream": spec.down}
		nodes = append(nodes, map[string]any{"id": spec.id, "type": spec.kind, "position": map[string]int{"x": i * 350, "y": 200}, "data": map[string]any{"label": spec.name, "name": spec.id, "form": spec.params}})
		for _, down := range spec.down {
			edges = append(edges, map[string]any{"id": spec.id + down, "source": spec.id, "sourceHandle": "start", "target": down, "targetHandle": "end"})
		}
	}
	return map[string]any{"components": components, "graph": map[string]any{"nodes": nodes, "edges": edges}, "globals": map[string]any{"sys.query": "", "sys.user_id": "", "sys.conversation_turns": 0, "sys.history": []any{}, "sys.files": []any{}}, "history": []any{}, "memory": []any{}, "messages": []any{}, "path": []any{}, "retrieval": []any{}, "variables": []any{}}, id
}
