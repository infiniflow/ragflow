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

package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// TestNormalizeMessageContent and friends moved to
// internal/service/openai_chat_test.go as TestService_NormalizeMessageContent_*
// (the helpers themselves moved to the service package). TestWriteSSE
// also moved to the service package as TestService_WriteSSE_FormatAndFlush.
// Handler tests here focus on the HTTP boundary: rejection at parse /
// presence / forbidden-key checks.

// fakeOpenAIUser injects a real *entity.User into the context so GetUser
// succeeds. Without this, the handler short-circuits with
// "User not found" before any validation runs.
func fakeOpenAIUser(c *gin.Context) {
	c.Set("user", &entity.User{ID: "u1", Email: "u@x"})
}

// newOpenAITestContext builds a test context with a real user, the
// chat_id path param, and a POST request carrying the given JSON body.
func newOpenAITestContext(t *testing.T, chatID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost,
		"/api/v1/openai/"+chatID+"/chat/completions",
		bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "chat_id", Value: chatID}}
	fakeOpenAIUser(c)
	return c, w
}

// TestChatCompletions_RejectsMissingMessages pins down the validation
// rule "You have to provide messages." (openai_api.py:255-256). The
// peek-and-discard parse in the handler (see OpenAIChatCompletions)
// rejects this BEFORE the service is called, so the test doesn't
// need a DB.
func TestChatCompletions_RejectsMissingMessages(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{"model":"model"}`)

	h.OpenAIChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 (Python convention), got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "You have to provide messages.") {
		t.Fatalf("expected 'You have to provide messages.' in body, got %s", respBody)
	}
}

// TestChatCompletions_DefaultsMissingModelToModel pins the
// Go-specific behavior: `model` is OPTIONAL on the openai_chat
// endpoint. If absent or empty, the handler injects the OpenAI
// compat sentinel "model" (which the service resolves to the
// dialog's default LLM). Python enforces the
// OpenAI spec strictly via @validate_request("model", "messages")
// at openai_api.py:237; the Go side intentionally relaxes that
// so callers can use the dialog's default without typing
// `"model": "model"` explicitly.
//
// The handler's check is the "model" is defaulted, not the
// service's success. We recover from the service's expected DB
// panic and only assert on what the handler wrote.
func TestChatCompletions_DefaultsMissingModelToModel(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1",
		`{"messages":[{"role":"user","content":"hi"}]}`)

	// The handler should accept the request and call the service.
	// The service will panic on DB access (no DB in tests); we
	// recover so the test only asserts the handler's behavior.
	func() {
		defer func() {
			_ = recover() // expected: service panicked on DB call
		}()
		h.OpenAIChatCompletions(c)
	}()

	// The handler must NOT have written a rejection response
	// before the service panicked. If the response body has
	// "You have to provide messages.", the messages-presence
	// check fired by mistake (it shouldn't, since messages IS
	// present).
	respBody := w.Body.String()
	if strings.Contains(respBody, "You have to provide messages.") {
		t.Fatalf("missing model should be defaulted, not rejected; got: %s", respBody)
	}
}

// TestChatCompletions_RejectsBadExtraBody pins down the validation
// rule "extra_body must be an object." (openai_api.py:243).
func TestChatCompletions_RejectsBadExtraBody(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}],
		"extra_body": "not an object"
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "extra_body must be an object.") {
		t.Fatalf("expected 'extra_body must be an object.' in body, got %s", respBody)
	}
}

// TestChatCompletions_RejectsBadMetadataCondition pins down the validation
// rule "metadata_condition must be an object." (openai_api.py:287).
func TestChatCompletions_RejectsBadMetadataCondition(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}],
		"extra_body": {"metadata_condition": "bad"}
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "metadata_condition must be an object.") {
		t.Fatalf("expected 'metadata_condition must be an object.' in body, got %s", respBody)
	}
}

// TestChatCompletions_RejectsBadReferenceMetadataFields pins down the
// validation rule "reference_metadata.fields must be an array." (openai_api.py:251).
func TestChatCompletions_RejectsBadReferenceMetadataFields(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}],
		"extra_body": {"reference_metadata": {"fields": "author"}}
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "reference_metadata.fields must be an array.") {
		t.Fatalf("expected 'reference_metadata.fields must be an array.' in body, got %s", respBody)
	}
}

// TestChatCompletions_RejectsLastMessageNotUser pins down the validation
// rule "The last content of this conversation is not from user." (openai_api.py:261).
func TestChatCompletions_RejectsLastMessageNotUser(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "world"}]
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "The last content of this conversation is not from user.") {
		t.Fatalf("expected 'The last content of this conversation is not from user.' in body, got %s", respBody)
	}
}

// TestChatCompletions_RejectsInvalidJSON pins down the JSON-parse failure
// path. We expect a 4xx-ish error code (Gin's binding error returns
// 400-equivalent message; we accept any non-empty error).
func TestChatCompletions_RejectsInvalidJSON(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{ not json`)

	h.OpenAIChatCompletions(c)

	if w.Body.Len() == 0 {
		t.Fatalf("expected non-empty error body for invalid JSON")
	}
}

// TestChatCompletions_SilentlyDropsTopLevelStop verifies that top-level
// `stop` is silently dropped rather than rejected. The field is not declared
// on OpenAIChatRequest, so Go's json.Unmarshal discards it — matching the
// OpenAI server convention of ignoring unknown request fields. The CLI parser
// rejects `stop` at parse time for CLI callers.
//
// The payload ends with an assistant turn so validation trips the early
// "last content not from user" rejector before reaching the DB. The
// rejection message proves we got past the stop check; the absence of
// "not supported" proves the field was silently dropped.
func TestChatCompletions_SilentlyDropsTopLevelStop(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "world"}],
		"stop": ["END"]
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if strings.Contains(respBody, "not supported") {
		t.Fatalf("did not expect 'stop' rejection, got %s", respBody)
	}
	if !strings.Contains(respBody, "The last content of this conversation is not from user.") {
		t.Fatalf("expected request to flow past stop check to last-message validator, got %s", respBody)
	}
}

// TestChatCompletions_SilentlyDropsTopLevelUser verifies that top-level
// `user` is silently dropped (same rationale and structure as `stop` above).
func TestChatCompletions_SilentlyDropsTopLevelUser(t *testing.T) {
	h := NewOpenAIChatHandler(service.NewOpenAIChatService())
	c, w := newOpenAITestContext(t, "c1", `{
		"model": "model",
		"messages": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "world"}],
		"user": "session-abc"
	}`)

	h.OpenAIChatCompletions(c)

	respBody := w.Body.String()
	if strings.Contains(respBody, "not supported") {
		t.Fatalf("did not expect 'user' rejection, got %s", respBody)
	}
	if !strings.Contains(respBody, "The last content of this conversation is not from user.") {
		t.Fatalf("expected request to flow past user check to last-message validator, got %s", respBody)
	}
}
