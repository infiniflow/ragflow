package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newReplicateForTest(baseURL string) *ReplicateModel {
	return NewReplicateModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/predictions", Models: "v1/models"},
	)
}

func TestReplicateName(t *testing.T) {
	if got := newReplicateForTest("http://unused").Name(); got != "replicate" {
		t.Errorf("Name()=%q", got)
	}
}

func TestReplicateFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Replicate", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*ReplicateModel); !ok {
		t.Fatalf("driver type=%T, want *ReplicateModel", driver)
	}
}

func TestReplicatePromptFromMessages(t *testing.T) {
	prompt, system := replicatePromptFromMessages([]Message{
		{Role: "system", Content: "be terse"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: map[string]interface{}{"text": "again"}},
	})
	if system != "be terse" {
		t.Errorf("system=%q", system)
	}
	want := "user: hello\nassistant: hi\nuser: {\"text\":\"again\"}"
	if prompt != want {
		t.Errorf("prompt=%q want %q", prompt, want)
	}
}

func TestReplicatePredictionEndpoint(t *testing.T) {
	m := newReplicateForTest("https://api.example.test")

	endpoint, version, err := m.predictionEndpoint(&APIConfig{}, "meta/meta-llama-3-70b-instruct")
	if err != nil {
		t.Fatalf("official endpoint: %v", err)
	}
	if endpoint != "https://api.example.test/v1/models/meta/meta-llama-3-70b-instruct/predictions" {
		t.Errorf("official endpoint=%q", endpoint)
	}
	if version != "" {
		t.Errorf("official version=%q want empty", version)
	}

	endpoint, version, err = m.predictionEndpoint(&APIConfig{}, "replicate/hello-world:5c7d5dc6dd8bf75c1acaa8565735e7986bc5b66206b55cca93cb72c9bf15ccaa")
	if err != nil {
		t.Fatalf("version endpoint: %v", err)
	}
	if endpoint != "https://api.example.test/v1/predictions" {
		t.Errorf("version endpoint=%q", endpoint)
	}
	if version != "replicate/hello-world:5c7d5dc6dd8bf75c1acaa8565735e7986bc5b66206b55cca93cb72c9bf15ccaa" {
		t.Errorf("version=%q", version)
	}
}

func TestReplicateOfficialChatHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models/meta/meta-llama-3-70b-instruct/predictions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		if got := r.Header.Get("Prefer"); got != "wait=60" {
			t.Errorf("Prefer=%q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
			return
		}
		if body["version"] != nil {
			t.Errorf("official model requests must not send version=%v", body["version"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v", body["stream"])
		}
		input := body["input"].(map[string]interface{})
		if input["prompt"] != "hello" {
			t.Errorf("prompt=%v", input["prompt"])
		}
		if input["system_prompt"] != "be helpful" {
			t.Errorf("system_prompt=%v", input["system_prompt"])
		}
		if input["max_new_tokens"] != float64(128) {
			t.Errorf("max_new_tokens=%v", input["max_new_tokens"])
		}
		// Stop is deliberately filtered out because Replicate model
		// inputs are model-specific and upstream support is undefined.
		if input["stop"] != nil {
			t.Errorf("unexpected stop=%v", input["stop"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "successful",
			"output": []string{"hel", "lo"},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	maxTokens := 128
	stop := []string{"END"}
	resp, err := newReplicateForTest(srv.URL).ChatWithMessages(
		"meta/meta-llama-3-70b-instruct",
		[]Message{{Role: "system", Content: "be helpful"}, {Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "hello" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestReplicateCommunityChatUsesVersionEndpoint(t *testing.T) {
	const version = "replicate/hello-world:5c7d5dc6dd8bf75c1acaa8565735e7986bc5b66206b55cca93cb72c9bf15ccaa"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/predictions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
			return
		}
		if body["version"] != version {
			t.Errorf("version=%v", body["version"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "successful",
			"output": "ok",
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newReplicateForTest(srv.URL).ChatWithMessages(
		version,
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "ok" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
}

func TestReplicateChatPollsUntilSucceeded(t *testing.T) {
	var getCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		switch r.URL.Path {
		case "/v1/models/meta/meta-llama-3-70b-instruct/predictions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "processing",
				"urls": map[string]string{
					"get": "http://" + r.Host + "/v1/predictions/p1",
				},
			})
		case "/v1/predictions/p1":
			getCount++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "successful",
				"output": "done",
			})
		default:
			t.Errorf("unexpected path=%s", r.URL.Path)
		}
	}))
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newReplicateForTest(srv.URL).ChatWithMessages(
		"meta/meta-llama-3-70b-instruct",
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if getCount != 1 {
		t.Errorf("getCount=%d", getCount)
	}
	if *resp.Answer != "done" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
}

func TestReplicateStreamHappyPath(t *testing.T) {
	var streamURL string
	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: output\n")
		_, _ = io.WriteString(w, "data: Hello\n\n")
		_, _ = io.WriteString(w, "event: output\n")
		_, _ = io.WriteString(w, "data:  world\n\n")
		_, _ = io.WriteString(w, "event: done\n")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	defer streamSrv.Close()
	streamURL = streamSrv.URL

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models/meta/meta-llama-3-70b-instruct/predictions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
			return
		}
		if body["stream"] != true {
			t.Errorf("stream=%v", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "starting",
			"urls": map[string]string{
				"stream": streamURL,
			},
		})
	}))
	defer apiSrv.Close()

	apiKey := "test-key"
	var chunks []string
	err := newReplicateForTest(apiSrv.URL).ChatStreamlyWithSender(
		"meta/meta-llama-3-70b-instruct",
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, _ *string) error {
			if c != nil {
				chunks = append(chunks, *c)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(chunks, "") != "Hello world[DONE]" {
		t.Errorf("chunks=%q", strings.Join(chunks, ""))
	}
}

func TestReplicateListModelsAndCheckConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]string{
				{"owner": "meta", "name": "meta-llama-3-70b-instruct"},
				{"owner": "replicate", "name": "hello-world"},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := newReplicateForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "meta/meta-llama-3-70b-instruct,replicate/hello-world" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestReplicateUnsupportedMethods(t *testing.T) {
	m := newReplicateForTest("http://unused")
	if _, err := m.Rerank(nil, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank error=%v", err)
	}
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
}
