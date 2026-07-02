package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

func TestChatSessionHandlerUpdateSession_RejectsEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/chats/chat-1/sessions/session-1", nil)
	ctx.Params = gin.Params{
		{Key: "chat_id", Value: "chat-1"},
		{Key: "session_id", Value: "session-1"},
	}
	ctx.Set("user", &entity.User{ID: "user-1"})

	handler := NewChatSessionHandler(service.NewChatSessionService(), nil)
	handler.UpdateSession(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got := body["code"]; got != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v", got)
	}
	if got := body["message"]; got != "Request body cannot be empty" {
		t.Fatalf("message=%v", got)
	}
}

func TestChatSessionHandlerUpdateSession_RejectsEmptyJSONObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/chats/chat-1/sessions/session-1", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{
		{Key: "chat_id", Value: "chat-1"},
		{Key: "session_id", Value: "session-1"},
	}
	ctx.Set("user", &entity.User{ID: "user-1"})

	handler := NewChatSessionHandler(service.NewChatSessionService(), nil)
	handler.UpdateSession(ctx)

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got := body["code"]; got != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v", got)
	}
	if got := body["message"]; got != "Request body cannot be empty" {
		t.Fatalf("message=%v", got)
	}
}

func TestChatSessionHandlerUpdateMessageFeedback_RejectsEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/chats/chat-1/sessions/session-1/messages/msg-1/feedback", nil)
	ctx.Params = gin.Params{
		{Key: "chat_id", Value: "chat-1"},
		{Key: "session_id", Value: "session-1"},
		{Key: "msg_id", Value: "msg-1"},
	}
	ctx.Set("user", &entity.User{ID: "user-1"})

	handler := NewChatSessionHandler(service.NewChatSessionService(), nil)
	handler.UpdateMessageFeedback(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got := body["code"]; got != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v", got)
	}
	if got := body["message"]; got != "Request body cannot be empty" {
		t.Fatalf("message=%v", got)
	}
}

func TestChatSessionHandlerUpdateMessageFeedback_RejectsEmptyJSONObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/chats/chat-1/sessions/session-1/messages/msg-1/feedback", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{
		{Key: "chat_id", Value: "chat-1"},
		{Key: "session_id", Value: "session-1"},
		{Key: "msg_id", Value: "msg-1"},
	}
	ctx.Set("user", &entity.User{ID: "user-1"})

	handler := NewChatSessionHandler(service.NewChatSessionService(), nil)
	handler.UpdateMessageFeedback(ctx)

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got := body["code"]; got != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v", got)
	}
	if got := body["message"]; got != "Request body cannot be empty" {
		t.Fatalf("message=%v", got)
	}
}
