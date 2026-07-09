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

// bot_completion.go is the SSE envelope writer + ChatbotCompletion
// service path for /api/v1/chatbots/<dialog_id>/completions. The wire
// shape is dictated by the existing python
// `api/db/services/conversation_service.py::async_iframe_completion`
// — JS widgets reading the iframe SDK expect this exact envelope, so
// any change to the frame keys is a wire-contract change.
//
// Frame shape (one JSON object per `data:` line):
//
//	{"code":0,"message":"","data":{"answer":"...","reference":{...},
//	 "audio_binary":null,"id":"...","session_id":"..."}, ...}
//
// The final completion marker is `data: {"code":0,"message":"",
// "data":true}` followed by the OpenAI-style `data: [DONE]` line
// that the existing Go SSE writers emit on the production
// /agents/chat/completions path.

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"ragflow/internal/utility"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// ChatbotSSEFrame is one envelope pushed to the SSE writer by the
// chatbot completion path. Err takes precedence over Data and is
// rendered as a python-style {code:500, message:str(e),
// data:{answer:"**ERROR**..."}} frame.
type ChatbotSSEFrame struct {
	// Event is the canvas.RunEvent type ("message",
	// "user_inputs", "workflow_finished", etc.). It is
	// forwarded in the SSE envelope as the `event` field so the
	// front-end can distinguish interactive form pauses from
	// plain assistant text (PR #14589). The field is omitted
	// from the JSON when empty to preserve the original wire
	// shape for callers that do not set it.
	Event     string         `json:"event,omitempty"`
	Data      string         `json:"-"`
	Reference map[string]any `json:"-"`
	SessionID string         `json:"-"`
	Done      bool           `json:"-"`
	Err       error          `json:"-"`
}

// WriteChatbotFrame emits one python-style SSE frame and flushes the
// underlying http.ResponseWriter. The frame is `data: <json>\n\n`
// and is byte-equivalent to the python side so the iframe SDK and
// existing JS widgets keep working.
//
// Error frames sanitize the message — internal errors (gorm stack
// frames, SQL details, storage paths) MUST NOT be echoed to the
// client. The caller is expected to log the real error via
// common.Error / zap before publishing the frame; only a generic
// placeholder is rendered here. Mirrors the python
// `api/db/services/conversation_service.py` error frame shape.
func WriteChatbotFrame(w http.ResponseWriter, f ChatbotSSEFrame) error {
	var payload map[string]any
	if f.Err != nil {
		const clientErrMsg = "an internal error occurred"
		payload = map[string]any{
			"code":    500,
			"message": clientErrMsg,
			"data": map[string]any{
				"answer":    clientErrMsg,
				"reference": map[string]any{},
			},
		}
	} else {
		data := map[string]any{
			"answer":       f.Data,
			"reference":    f.Reference,
			"audio_binary": nil,
			"id":           nil,
			"session_id":   f.SessionID,
		}
		// Forward the canvas event type so the front-end can
		// distinguish interactive form pauses ("user_inputs",
		// "workflow_finished") from plain assistant messages
		// (PR #14589). When Event is empty the field is omitted
		// from the JSON so existing message frames stay
		// byte-compatible.
		if f.Event != "" {
			data["event"] = f.Event
		}
		payload = map[string]any{
			"code":    0,
			"message": "",
			"data":    data,
		}
	}
	// Use SafeJSONMarshal to handle non-serializable values (funcs,
	// channels) that may have leaked into SSE payload maps. Mirrors
	// the Python PR #14210 _canvas_json_default fallback in agent_api.py.
	b, err := runtime.SafeJSONMarshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// WriteDoneFrame emits the python completion marker
// `data: {"code":0,"message":"","data":true}\n\n` followed by the
// OpenAI-style `data: [DONE]\n\n` terminator. Used by both bot
// completion paths.
func WriteDoneFrame(w http.ResponseWriter) error {
	if _, err := w.Write([]byte(`data: {"code":0,"message":"","data":true}` + "\n\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// WriteChatbotRunEvent translates one canvas.RunEvent into the flat
// Python agent-canvas SSE envelope:
//
//	data: {"event":"message","message_id":"...","task_id":"...",
//	  "session_id":"...","created_at":123,"data":{"content":"..."}}\n\n
//
// This is intentionally different from WriteChatbotFrame's legacy
// chatbot `{code,data:{answer:"..."}}` shape. The agent React page's
// use-send-message.ts parser appends each parsed object directly to
// answerList and expects top-level `event` / `message_id`, plus a
// typed `data` payload. If RunEvent frames are double-wrapped in
// data.answer, the browser receives bytes but cannot render the
// assistant message or correlate the current Log panel.
//
// The "done" event type emits `data: [DONE]\n\n` (no envelope),
// matching the Python agent API terminator.
//
// Returns the write error so callers can short-circuit; both nil
// and io.ErrClosedPipe are tolerated because the client may have
// disconnected mid-stream.
func WriteChatbotRunEvent(w http.ResponseWriter, ev canvas.RunEvent) error {
	if ev.Type == "done" {
		_, err := w.Write([]byte("data: [DONE]\n\n"))
		if err != nil {
			return err
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return nil
	}

	var data any = map[string]any{}
	if ev.Data != "" {
		if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
			data = ev.Data
		}
	}
	if ev.Type == "error" {
		msg := "an internal error occurred"
		if m, ok := data.(map[string]any); ok {
			if s, _ := m["message"].(string); s != "" {
				msg = s
			}
		}
		payload := map[string]any{
			"code":    500,
			"message": msg,
			"data":    false,
		}
		return writeSSEJSON(w, payload)
	}

	payload := map[string]any{
		"data":       data,
		"created_at": ev.CreatedAt,
	}
	if ev.Type != "" {
		payload["event"] = ev.Type
	}
	if ev.MessageID != "" {
		payload["message_id"] = ev.MessageID
	}
	if ev.TaskID != "" {
		payload["task_id"] = ev.TaskID
	}
	if ev.SessionID != "" {
		payload["session_id"] = ev.SessionID
	}
	return writeSSEJSON(w, payload)
}

func writeSSEJSON(w http.ResponseWriter, payload map[string]any) error {
	b, err := runtime.SafeJSONMarshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("data:")); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// AgentbotSSEFrame mirrors ChatbotSSEFrame for the agentbot
// completion path. The envelope shape is the same; the only
// difference is that the LLM call goes through the canvas runner
// (AgentService.RunAgent) instead of the legacy dialog async_chat.
type AgentbotSSEFrame = ChatbotSSEFrame

// WriteAgentbotFrame is an alias for WriteChatbotFrame — both bot
// completion paths emit the same python wire shape.
func WriteAgentbotFrame(w http.ResponseWriter, f ChatbotSSEFrame) error {
	return WriteChatbotFrame(w, f)
}

// ChatbotCompletion streams an SSE response for
// /api/v1/chatbots/<dialog_id>/completions.
//
// The full LLM session-lifecycle implementation is added below. It
// is a v1 port: it yields a single frame per turn (the Go LLMBundle
// chat call is non-streaming), seeded with the dialog's prologue
// when the request creates a new session.
//
// Authorisation: dialog must exist, belong to the requester's tenant,
// and have status == common.StatusDialogValid.
func (s *BotService) ChatbotCompletion(
	ctx context.Context, tenantID, dialogID string, req ChatbotCompletionRequest,
) (<-chan ChatbotSSEFrame, common.ErrorCode, error) {
	// 1. Load and authorise the dialog.
	//
	// ChatSessionDAO.GetDialogByID already filters by status = "1"
	// so a returned row is valid; we still nil-check defensively
	// before dereferencing for symmetry with the session path.
	dialog, err := s.chatDAO.GetDialogByID(dialogID)
	if err != nil || dialog == nil ||
		dialog.TenantID != tenantID ||
		dialog.Status == nil || *dialog.Status != common.StatusDialogValid {
		return nil, common.CodeDataError, errors.New("no access to this chatbot")
	}

	// 2. Resolve or create the session row.
	//
	// API4ConversationDAO.GetBySessionID returns (nil, nil) on miss
	// (not an error) — see internal/dao/api_token.go:146. We MUST
	// check the pointer before dereferencing, otherwise the
	// session-tenant check below nil-derefs. Plan Risk R7.
	//
	// UserID vs tenantID (security H3 follow-up):
	// `entity.API4Conversation.UserID` is a generic user-id slot
	// in the production Python flow
	// (api/db/services/conversation_service.py:258 — the python
	// async_iframe_completion saves `user_id=kwargs.get("user_id", "")`).
	// The Go BotHandler routes pass `user.ID` through the
	// "tenantID" parameter (the Go User struct collapses user and
	// tenant into one identifier — see project CLAUDE.md), so
	// writing `tenantID` here actually stores the requester's
	// user-id (== tenant-id) in the python user-id slot. The
	// session-tenant check on the read path compares against the
	// same value, so write/read stay symmetric. We keep this
	// behaviour and add the comment so a future reader doesn't
	// "fix" it to a tenant-id lookup and break the symmetry.
	var session *entity.API4Conversation
	if req.SessionID != "" {
		session, err = s.api4ConversationDAO.GetBySessionID(req.SessionID, dialogID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if session == nil || session.UserID != tenantID {
			return nil, common.CodeDataError, errors.New("session not found")
		}
	} else {
		// Seed a new session. The Message column is json.RawMessage;
		// pre-serialise the prologue turn as a JSON array of
		// {role,content,created_at} dicts — same shape the python
		// conversation_service.py:253-272 writes. Plan Risk R4.
		prologue := stringFromMap(dialog.PromptConfig, "prologue")
		seedMsg, _ := json.Marshal([]map[string]any{
			{
				"role":       "assistant",
				"content":    prologue,
				"created_at": time.Now().Unix(),
			},
		})
		session = &entity.API4Conversation{
			ID:       utility.GenerateUUID(),
			DialogID: dialogID,
			UserID:   tenantID,
			Message:  seedMsg,
		}
		if err = s.api4ConversationDAO.Create(session); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	// 3. Resolve the chat LLM via ModelProviderService. The python
	// async_iframe_completion resolves the same way through
	// LLMBundle(tenant_id, dialog.llm_id); the Go equivalent is
	// GetChatModelConfig → NewChatModel → driver.ChatWithMessages.
	//
	// If llmService is unwired (test boot path) or the dialog has
	// no LLM configured, we surface a sanitized CodeDataError
	// rather than echoing the bare error string into the SSE
	// envelope — see WriteChatbotFrame's sanitization contract.
	if s.llmService == nil {
		return nil, common.CodeServerError, errors.New("bot: llm service not wired")
	}
	if dialog.LLMID == "" {
		return nil, common.CodeDataError, errors.New("no LLM configured for this chatbot")
	}
	modelProvider := NewModelProviderService()
	driver, modelName, apiConfig, _, err := modelProvider.GetChatModelConfig(tenantID, dialog.LLMID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("no LLM configured for this chatbot")
	}
	chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)

	// 4. Build the prompt from prior conversation history plus the
	// new user turn. Without this, a resumed session_id would
	// authorise reuse but the LLM call would still be stateless
	// turn-to-turn — a Python parity regression for any multi-turn
	// chatbot client. The Message column on api_4_conversation is a
	// json.RawMessage array of {role, content, created_at} dicts,
	// matching the python conversation_service.py:253-272 shape.
	messages := historyToMessages(session.Message)
	messages = append(messages, modelModule.Message{Role: "user", Content: req.Question})

	// 5. Yield frames on a channel.
	out := make(chan ChatbotSSEFrame, 4)
	go func() {
		defer close(out)
		resp, callErr := chatModel.ModelDriver.ChatWithMessages(
			modelName, messages, chatModel.APIConfig, &modelModule.ChatConfig{},
		)
		if callErr != nil {
			// Log the real error with structured context so
			// ops can debug, but do NOT echo the raw
			// err.Error() to the client (security M2:
			// internal gorm/SQL/file-path leaks).
			common.Error("bot: ChatbotCompletion LLM call failed",
				callErr,
				zap.String("dialog_id", dialogID),
				zap.String("session_id", session.ID),
				zap.String("llm_id", dialog.LLMID),
			)
			out <- ChatbotSSEFrame{
				Err:       errors.New("an internal error occurred"),
				SessionID: session.ID,
			}
			out <- ChatbotSSEFrame{Done: true}
			return
		}
		answer := ""
		if resp != nil && resp.Answer != nil {
			answer = *resp.Answer
		}

		// Persist the new turn pair (user + assistant) back to
		// api_4_conversation so the NEXT call to ChatbotCompletion
		// with the same session_id sees this turn in messages.
		// Update errors are logged but do NOT fail the SSE stream
		// — the answer has already been produced. The next call
		// will rebuild from the prior (pre-this-turn) snapshot,
		// losing at most the latest exchange; acceptable for v1.
		newTurns := append(historyFromMessages(messages),
			map[string]any{"role": "assistant", "content": answer, "created_at": time.Now().Unix()},
		)
		if updated, mErr := json.Marshal(newTurns); mErr == nil {
			session.Message = updated
			if uErr := s.api4ConversationDAO.Update(session); uErr != nil {
				common.Error("bot: ChatbotCompletion session update failed",
					uErr,
					zap.String("dialog_id", dialogID),
					zap.String("session_id", session.ID),
				)
			}
		}

		out <- ChatbotSSEFrame{
			Data:      answer,
			Reference: map[string]any{},
			SessionID: session.ID,
		}
		out <- ChatbotSSEFrame{Done: true}
	}()
	return out, common.CodeSuccess, nil
}

// historyToMessages reads the session.Message JSON array of
// {role, content, ...} dicts and projects it onto modelModule.Message
// for the LLM driver. Tolerates an empty / malformed Message column
// by returning an empty slice — the caller appends the new user turn
// so the LLM still receives the current prompt.
func historyToMessages(raw json.RawMessage) []modelModule.Message {
	if len(raw) == 0 {
		return nil
	}
	var turns []map[string]any
	if err := json.Unmarshal(raw, &turns); err != nil {
		return nil
	}
	out := make([]modelModule.Message, 0, len(turns))
	for _, t := range turns {
		role, _ := t["role"].(string)
		content, _ := t["content"].(string)
		if role == "" || content == "" {
			continue
		}
		out = append(out, modelModule.Message{Role: role, Content: content})
	}
	return out
}

// historyFromMessages is the inverse projection — used to write the
// updated turn list back to the api_4_conversation.Message column.
func historyFromMessages(msgs []modelModule.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	now := time.Now().Unix()
	for i, m := range msgs {
		out = append(out, map[string]any{
			"role":       m.Role,
			"content":    m.Content,
			"created_at": now + int64(i), // preserve order, monotonic
		})
	}
	return out
}
