package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

type modelInputDebugEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// setupModelInputDebug creates an owned Agent canvas backed by an isolated test
// database and configures its provider to use the supplied HTTP endpoint.
func setupModelInputDebug(t *testing.T, endpoint string) (*AgentHandler, *entity.UserCanvas) {
	t.Helper()
	db := setupHandlerAgentsTestDB(t)
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
	previousHostSetting := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = previousHostSetting })
	if err := models.InitProviderManager("../../conf/models"); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID: "c1", UserID: "u-1", Permission: "me",
		DSL: map[string]any{"components": map[string]any{
			"agent": map[string]any{"obj": map[string]any{
				"component_name": "Agent",
				"params": map[string]any{
					"llm_id": "qwen-flash", "driver": "Tongyi-Qianwen",
					"api_key": "private-test-key", "base_url": endpoint,
					"sys_prompt": "Follow the instruction", "user_prompt": "Hello {{source@content}}",
					"max_rounds": 1,
				},
			}},
		}},
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatal(err)
	}
	return &AgentHandler{loader: &fakeCanvasLoader{canvas: canvas}}, canvas
}

// runModelInputDebug invokes Agent debug with the supplied raw JSON flag and
// decodes the response envelope; an empty flag omits the option.
func runModelInputDebug(t *testing.T, handler *AgentHandler, flag string) modelInputDebugEnvelope {
	t.Helper()
	extra := ""
	if flag != "" {
		extra = `,"include_model_inputs":` + flag
	}
	body := `{"params":{"source@content":{"value":"resolved"}}` + extra + `}`
	c, w := componentCtx(t, http.MethodPost, "/api/v1/agents/c1/components/agent/debug", body)
	c.Params = gin.Params{{Key: "canvas_id", Value: "c1"}, {Key: "component_id", Value: "agent"}}
	handler.DebugComponent(c)
	var envelope modelInputDebugEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response: %s (%v)", w.Body.String(), err)
	}
	return envelope
}

// TestDebugAgentModelInputsMatchProviderMessages checks the production Agent
// path's captured system and resolved user messages against the HTTP request.
func TestDebugAgentModelInputsMatchProviderMessages(t *testing.T) {
	var mu sync.Mutex
	var providerMessages json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		providerMessages = body.Messages
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	handler, _ := setupModelInputDebug(t, server.URL)
	response := runModelInputDebug(t, handler, "true")
	if response.Code != 0 {
		t.Fatalf("debug failed: %+v", response)
	}
	var data struct {
		Outputs     map[string]any     `json:"outputs"`
		ModelInputs models.ModelInputs `json:"model_inputs"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Outputs["content"] != "ok" || len(data.ModelInputs.Calls) != 1 || data.ModelInputs.OmittedCount != 0 {
		t.Fatalf("unexpected debug data: %s", response.Data)
	}
	call := data.ModelInputs.Calls[0]
	if call.Model != "qwen-flash" || call.Status != "captured" || call.Sequence != 1 {
		t.Fatalf("unexpected call: %+v", call)
	}
	var actual, expected any
	if err := json.Unmarshal(call.Messages, &actual); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if err := json.Unmarshal(providerMessages, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) || !strings.Contains(string(call.Messages), "Follow the instruction") || !strings.Contains(string(call.Messages), "Hello resolved") {
		t.Fatalf("capture=%s provider=%s", call.Messages, providerMessages)
	}
	if strings.Contains(string(response.Data), "private-test-key") || strings.Contains(string(response.Data), server.URL) {
		t.Fatalf("provider configuration leaked: %s", response.Data)
	}
}

// TestDebugAgentModelInputsDefaultAndErrorContracts checks unchanged default
// outputs and error codes, with opt-in diagnostics retained on provider failure.
func TestDebugAgentModelInputsDefaultAndErrorContracts(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"provider rejected request"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	handler, _ := setupModelInputDebug(t, server.URL)
	for _, flag := range []string{"", "false"} {
		response := runModelInputDebug(t, handler, flag)
		var outputs map[string]any
		if err := json.Unmarshal(response.Data, &outputs); err != nil {
			t.Fatal(err)
		}
		if response.Code != 0 || outputs["content"] != "ok" || outputs["model_inputs"] != nil || outputs["outputs"] != nil {
			t.Fatalf("default contract changed: %+v %s", response, response.Data)
		}
	}
	fail.Store(true)
	without := runModelInputDebug(t, handler, "false")
	with := runModelInputDebug(t, handler, "true")
	if without.Code == 0 || without.Code != with.Code || without.Message != with.Message || string(without.Data) != "null" {
		t.Fatalf("error contract changed: without=%+v with=%+v", without, with)
	}
	var data struct {
		Outputs     map[string]any     `json:"outputs"`
		ModelInputs models.ModelInputs `json:"model_inputs"`
	}
	if err := json.Unmarshal(with.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Outputs != nil || len(data.ModelInputs.Calls) != 1 || data.ModelInputs.Calls[0].Status != "captured" {
		t.Fatalf("failed provider input was lost: %s", with.Data)
	}
}

// TestDebugAgentModelInputsRejectsInvalidFlagAndUnauthorizedExecution checks
// strict boolean validation and update authorization before provider execution.
func TestDebugAgentModelInputsRejectsInvalidFlagAndUnauthorizedExecution(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	handler, _ := setupModelInputDebug(t, server.URL)
	for _, flag := range []string{`null`, `"true"`, `1`, `{}`, `[]`} {
		if response := runModelInputDebug(t, handler, flag); response.Code != int(common.CodeArgumentError) || string(response.Data) != "null" {
			t.Fatalf("flag %s accepted: %+v", flag, response)
		}
	}
	// The read-side loader deliberately succeeds. The debug endpoint must still
	// check Update against the stored canvas before any Agent/model execution.
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "c1").Update("user_id", "another-owner").Error; err != nil {
		t.Fatal(err)
	}
	response := runModelInputDebug(t, handler, "true")
	if response.Code != int(common.CodeOperatingError) || response.Message != canvasNoAccessMessage || string(response.Data) != "null" {
		t.Fatalf("unauthorized result: %+v", response)
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected requests executed %d provider calls", calls.Load())
	}
}

// TestDebugModelInputsRejectsNonAgent checks that other component types cannot
// opt into model-input capture.
func TestDebugModelInputsRejectsNonAgent(t *testing.T) {
	handler := &AgentHandler{loader: &fakeCanvasLoader{canvas: beginCanvas()}}
	c, w := componentCtx(t, http.MethodPost, "/api/v1/agents/c1/components/begin/debug", `{"params":{"query":{"value":"hello"}},"include_model_inputs":true}`)
	c.Params = gin.Params{{Key: "canvas_id", Value: "c1"}, {Key: "component_id", Value: "begin"}}
	handler.DebugComponent(c)
	var response modelInputDebugEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != int(common.CodeArgumentError) || string(response.Data) != "null" {
		t.Fatalf("non-Agent capture accepted: %+v", response)
	}
}
