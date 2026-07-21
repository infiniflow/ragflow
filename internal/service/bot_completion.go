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
	"strings"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
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
	// Final marks the last answer frame of a turn. It is
	// rendered as `"final": true` in the data payload so the
	// front-end replaces (instead of appends to) the
	// accumulated text — required because the final pipeline
	// result is decorated (citation markers inserted mid-text)
	// and no longer a strict superset of the streamed deltas.
	Final bool `json:"-"`
	// StartToThink / EndToThink bracket the reasoning segment,
	// rendered as start_to_think / end_to_think so the
	// front-end can wrap it in <think> markers like the python
	// side does.
	StartToThink bool `json:"-"`
	EndToThink   bool `json:"-"`
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
		if f.Final {
			data["final"] = true
		}
		if f.StartToThink {
			data["start_to_think"] = true
		}
		if f.EndToThink {
			data["end_to_think"] = true
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
// The "done" event type emits `data:[DONE]\n\n` (no envelope),
// matching the Python agent API terminator.
//
// Returns the write error so callers can short-circuit; both nil
// and io.ErrClosedPipe are tolerated because the client may have
// disconnected mid-stream.
func WriteChatbotRunEvent(w http.ResponseWriter, ev canvas.RunEvent) error {
	if ev.Type == "done" {
		_, err := w.Write([]byte("data:[DONE]\n\n"))
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
// The completion runs through ChatPipelineService.AsyncChat — the
// same RAG pipeline that serves the regular chat endpoints — so
// knowledge-base retrieval, the dialog's configured empty_response
// fallback, citations and the system prompt all behave identically
// to the in-app chat. Mirrors the python
// api/db/services/conversation_service.py::async_iframe_completion,
// which delegates to the same async_chat used by regular sessions.
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
	if req.SessionID == "" {
		// No session yet: seed one with the prologue and return it
		// immediately WITHOUT running the pipeline. Mirrors python
		// async_iframe_completion (conversation_service.py:324-334):
		// the share page calls this endpoint once with an empty
		// question to obtain a session_id, then sends the real
		// questions with that session_id.
		prologue := stringFromMap(dialog.PromptConfig, "prologue")
		seedMsg, _ := json.Marshal([]map[string]any{
			{
				"role":       "assistant",
				"content":    prologue,
				"created_at": time.Now().Unix(),
			},
		})
		session := &entity.API4Conversation{
			ID:       utility.GenerateUUID(),
			DialogID: dialogID,
			UserID:   tenantID,
			Message:  seedMsg,
		}
		if err = s.api4ConversationDAO.Create(session); err != nil {
			return nil, common.CodeServerError, err
		}

		// Mirror python async_iframe_completion
		// (conversation_service.py:324-334): a request without a
		// session_id is the share page's opening handshake — the
		// front-end sends an empty question only to obtain a session.
		// Persist the prologue-seeded session and stream the prologue
		// back WITHOUT invoking the pipeline; running the model here
		// would fabricate a reply to a message the user never sent.
		out := make(chan ChatbotSSEFrame, 2)
		go func() {
			defer close(out)
			out <- ChatbotSSEFrame{
				Data:      prologue,
				Reference: map[string]any{},
				SessionID: session.ID,
			}
			out <- ChatbotSSEFrame{Done: true}
		}()
		return out, common.CodeSuccess, nil
	}

	session, err := s.api4ConversationDAO.GetBySessionID(req.SessionID, dialogID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if session == nil || session.UserID != tenantID {
		return nil, common.CodeDataError, errors.New("session not found")
	}

	// 3. Guard rails mirroring the previous implementation: surface
	// a sanitised error before any SSE byte is written when the
	// service is unwired (test boot path) or the dialog has no LLM
	// configured — see WriteChatbotFrame's sanitization contract.
	// NewBotService wires both dependencies; the nil checks only
	// guard a hand-rolled zero-value BotService against panicking.
	if s.llmService == nil {
		return nil, common.CodeServerError, errors.New("bot: llm service not wired")
	}
	if s.pipeline == nil {
		return nil, common.CodeServerError, errors.New("bot: chat pipeline not wired")
	}
	if dialog.LLMID == "" {
		return nil, common.CodeDataError, errors.New("no LLM configured for this chatbot")
	}

	// 4. Build the pipeline input. The Message column on
	// api_4_conversation is a json.RawMessage array of
	// {role, content, created_at} dicts; the pipeline expects the
	// same filtered shape python builds in async_iframe_completion
	// (drop system turns, drop the leading assistant prologue,
	// append the new user turn last).
	messageID := utility.GenerateUUID()
	messages := buildChatbotPipelineMessages(session.Message, req.Question, messageID)

	// python bot_api.py:72-73 defaults quote to False for chatbot
	// completions when the caller omits it.
	kwargs := map[string]interface{}{
		"quote": req.Quote != nil && *req.Quote,
	}
	if reasoning, ok := normalizeBotBoolFlag(req.Reasoning); ok {
		kwargs["reasoning"] = reasoning
	}
	if req.Internet != nil {
		kwargs["internet"] = req.Internet
	}
	if req.DocIDs != "" {
		kwargs["doc_ids"] = req.DocIDs
	}

	results, err := s.pipeline.AsyncChat(ctx, tenantID, dialog, messages, true, kwargs)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// 5. Translate pipeline results into chatbot frames. The
	// pipeline streams deltas; the python iframe contract sends the
	// full accumulated answer in every frame, so we accumulate here
	// (the front-end accepts both shapes, but full-text frames are
	// byte-parity with python). The final pipeline result carries
	// the decorated full answer plus the retrieval reference.
	out := make(chan ChatbotSSEFrame, 16)
	go func() {
		defer close(out)
		var (
			fullAnswer string
			finalRef   map[string]any
			errored    bool
		)
		for res := range results {
			if res.Final {
				if res.Answer != "" {
					// Decorated full answer (citations
					// resolved). Replaces the accumulated
					// deltas; the empty_response fallback
					// path yields an empty final answer, in
					// which case the accumulated fallback
					// text stands.
					fullAnswer = res.Answer
				}
				if res.Reference != nil {
					finalRef = res.Reference
				}
				if strings.HasPrefix(fullAnswer, "**ERROR**") {
					errored = true
				}
				out <- ChatbotSSEFrame{
					Data:      fullAnswer,
					Reference: referenceOrEmpty(finalRef),
					SessionID: session.ID,
					Final:     true,
				}
				continue
			}
			if res.StartToThink || res.EndToThink {
				out <- ChatbotSSEFrame{
					Data:         fullAnswer,
					Reference:    map[string]any{},
					SessionID:    session.ID,
					StartToThink: res.StartToThink,
					EndToThink:   res.EndToThink,
				}
				continue
			}
			fullAnswer += res.Answer
			if len(res.Reference) > 0 {
				// The pipeline only populates Reference on the final
				// result today; tracking it here keeps finalRef
				// correct if a mid-stream result ever carries one.
				// Intermediate frames still send an empty reference
				// object for wire parity with python
				// async_iframe_completion — only the final frame
				// carries the retrieval reference.
				finalRef = res.Reference
			}
			out <- ChatbotSSEFrame{
				Data:      fullAnswer,
				Reference: map[string]any{},
				SessionID: session.ID,
			}
		}

		// 6. Persist the completed turn pair (user + assistant)
		// plus the retrieval reference, mirroring python
		// API4ConversationService.append_message after the stream.
		// Persistence errors are logged but do NOT fail the SSE
		// stream — the answer has already been produced. On a
		// pipeline-level error ("**ERROR**" answer) nothing is
		// persisted, matching the python exception path.
		if !errored {
			if pErr := s.persistChatbotTurn(session, req.Question, fullAnswer, messageID, finalRef); pErr != nil {
				common.Error("bot: ChatbotCompletion session update failed",
					pErr,
					zap.String("dialog_id", dialogID),
					zap.String("session_id", session.ID),
				)
			}
		}
		out <- ChatbotSSEFrame{Done: true}
	}()
	return out, common.CodeSuccess, nil
}

// buildChatbotPipelineMessages projects the session.Message JSON
// array plus the new user question onto the message shape
// ChatPipelineService.AsyncChat expects. Mirrors the filtering in
// python async_iframe_completion (conversation_service.py:341-356):
// system turns are dropped and a leading assistant turn (the seeded
// prologue) is dropped so the first message the LLM sees is a user
// turn. Tolerates an empty / malformed Message column by starting
// from just the new question.
func buildChatbotPipelineMessages(raw json.RawMessage, question, messageID string) []map[string]interface{} {
	turns := parseChatbotTurns(raw)
	turns = append(turns, map[string]any{
		"role":    "user",
		"content": question,
		"id":      messageID,
	})
	msg := make([]map[string]interface{}, 0, len(turns))
	for _, m := range turns {
		role, _ := m["role"].(string)
		if role == "system" {
			continue
		}
		if role == "assistant" && len(msg) == 0 {
			continue
		}
		msg = append(msg, m)
	}
	return msg
}

// parseChatbotTurns decodes the session.Message JSON array.
// Returns an empty (non-nil) slice on empty or malformed input so
// callers can always append.
func parseChatbotTurns(raw json.RawMessage) []map[string]any {
	turns := make([]map[string]any, 0)
	if len(raw) == 0 {
		return turns
	}
	if err := json.Unmarshal(raw, &turns); err != nil || turns == nil {
		return make([]map[string]any, 0)
	}
	return turns
}

// persistChatbotTurn appends the finished user/assistant turn pair
// and the retrieval reference to the api_4_conversation row so the
// next ChatbotCompletion call with the same session_id sees this
// turn in its history. Mirrors python
// API4ConversationService.append_message.
func (s *BotService) persistChatbotTurn(
	session *entity.API4Conversation, question, answer, messageID string, reference map[string]any,
) error {
	// Serialise the read-modify-write per session and re-read the row
	// inside the lock: the caller's session was loaded before the
	// stream ran, so a concurrent request on the same session_id may
	// already have appended its own turn. Without the lock + re-read
	// the last Update would silently drop the other exchange.
	lock := s.persistLock(session.ID)
	lock.Lock()
	defer lock.Unlock()
	fresh, err := s.api4ConversationDAO.GetBySessionID(session.ID, session.DialogID)
	if err != nil {
		return err
	}
	if fresh != nil {
		session = fresh
	}

	now := time.Now().Unix()
	turns := parseChatbotTurns(session.Message)
	// Both turns of the pair share messageID by design: a Q&A exchange
	// is addressed as a unit — mirrors the in-app chat convention
	// where the answer id is derived from the question id so the pair
	// is deleted together (web/src/hooks/logic-hooks.ts
	// buildMessageUuid).
	turns = append(turns,
		map[string]any{
			"role":       "user",
			"content":    question,
			"id":         messageID,
			"created_at": now,
		},
		map[string]any{
			"role":       "assistant",
			"content":    answer,
			"id":         messageID,
			"created_at": now,
		},
	)
	rawMsg, err := json.Marshal(turns)
	if err != nil {
		return err
	}
	session.Message = rawMsg

	refs := make([]any, 0)
	if len(session.Reference) > 0 {
		// Tolerate malformed / missing reference history — the
		// message turns above are the authoritative history;
		// a lost reference list degrades citation display only.
		_ = json.Unmarshal(session.Reference, &refs)
	}
	if reference == nil {
		reference = map[string]any{"chunks": []any{}, "doc_aggs": []any{}}
	}
	refs = append(refs, reference)
	rawRef, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	session.Reference = rawRef

	return s.api4ConversationDAO.Update(session)
}

// normalizeBotBoolFlag coerces the JSON-encoded reasoning / internet
// flags to a bool. Widgets send them as true/false or 0/1 numbers;
// ok=false means the value was absent or unrecognised and the caller
// should leave the pipeline default in place.
func normalizeBotBoolFlag(v any) (value, ok bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case float64:
		if x == 0 || x == 1 {
			return x == 1, true
		}
	case int:
		if x == 0 || x == 1 {
			return x == 1, true
		}
	}
	return false, false
}

// referenceOrEmpty returns ref or an empty map so SSE frames always
// carry a JSON object in the reference field, never null.
func referenceOrEmpty(ref map[string]any) map[string]any {
	if ref == nil {
		return map[string]any{}
	}
	return ref
}
