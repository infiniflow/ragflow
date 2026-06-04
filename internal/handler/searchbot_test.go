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
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// fakeSearchbotLLM implements searchbotLLM for testing.
type fakeSearchbotLLM struct {
	response string
	err      error
}

func (f *fakeSearchbotLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &modelModule.ChatResponse{Answer: &f.response}, nil
}

func setupSearchbotRequest(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/related_questions",
		strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	return c, w
}

// TestSearchbotHandler_Success verifies the happy path.
func TestSearchbotHandler_Success(t *testing.T) {
	llm := &fakeSearchbotLLM{
		response: `Here are some related questions:
1. How do EV impact environment?
2. What are advantages of EV?
3. Cost of EV?`,
	}
	h := NewSearchbotHandler(nil, nil, llm)

	c, w := setupSearchbotRequest(`{"question": "EV benefits"}`)
	h.Handle(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp["message"])
	}

	questions, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(questions) != 3 {
		t.Fatalf("expected 3 questions, got %d", len(questions))
	}
	if questions[0] != "How do EV impact environment?" {
		t.Errorf("unexpected [0]: %v", questions[0])
	}
}

// TestSearchbotHandler_EmptyResponse verifies empty LLM response returns empty list.
func TestSearchbotHandler_EmptyResponse(t *testing.T) {
	llm := &fakeSearchbotLLM{
		response: "No related questions found.",
	}
	h := NewSearchbotHandler(nil, nil, llm)

	c, w := setupSearchbotRequest(`{"question": "EV benefits"}`)
	h.Handle(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp["message"])
	}
	questions, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(questions) != 0 {
		t.Errorf("expected 0 questions, got %d", len(questions))
	}
}

// TestSearchbotHandler_LLMFailure verifies error handling on LLM failure.
func TestSearchbotHandler_LLMFailure(t *testing.T) {
	llm := &fakeSearchbotLLM{
		err: errFake{msg: "LLM unavailable"},
	}
	h := NewSearchbotHandler(nil, nil, llm)

	c, w := setupSearchbotRequest(`{"question": "EV benefits"}`)
	h.Handle(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected error code, got 0")
	}
}

// TestSearchbotHandler_MissingQuestion verifies validation.
func TestSearchbotHandler_MissingQuestion(t *testing.T) {
	llm := &fakeSearchbotLLM{response: "dummy"}
	h := NewSearchbotHandler(nil, nil, llm)

	c, w := setupSearchbotRequest(`{}`)
	h.Handle(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected error code, got 0")
	}
}

// errFake implements error for testing.
type errFake struct{ msg string }

func (e errFake) Error() string { return e.msg }

// Existing parse tests below
func TestParseRelatedQuestions_Standard(t *testing.T) {
	input := `1. How do electric vehicles impact the environment?
2. What are the advantages of owning an electric car?
3. What is the cost-effectiveness?`

	got := parseRelatedQuestions(input)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "How do electric vehicles impact the environment?" {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "What are the advantages of owning an electric car?" {
		t.Errorf("unexpected [1]: %q", got[1])
	}
	if got[2] != "What is the cost-effectiveness?" {
		t.Errorf("unexpected [2]: %q", got[2])
	}
}

func TestParseRelatedQuestions_Empty(t *testing.T) {
	got := parseRelatedQuestions("")
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestParseRelatedQuestions_NoNumberedLines(t *testing.T) {
	input := `Here are some related questions:
- First question
- Second question`

	got := parseRelatedQuestions(input)
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestParseRelatedQuestions_MixedContent(t *testing.T) {
	input := `Here are some related questions:
1. First related question.
Some explanation text.
2. Second related question.
More text.
3. Third related question.`

	got := parseRelatedQuestions(input)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "First related question." {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "Second related question." {
		t.Errorf("unexpected [1]: %q", got[1])
	}
	if got[2] != "Third related question." {
		t.Errorf("unexpected [2]: %q", got[2])
	}
}

func TestParseRelatedQuestions_MultiDigit(t *testing.T) {
	input := `10. Tenth question.
11. Eleventh question.`

	got := parseRelatedQuestions(input)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0] != "Tenth question." {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "Eleventh question." {
		t.Errorf("unexpected [1]: %q", got[1])
	}
}
