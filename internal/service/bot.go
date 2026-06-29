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

// BotService is the shared service layer for the public
// chatbot/agentbot endpoints (api/v1/chatbots/...,
// api/v1/agentbots/...) plus the agent attachment download. It is
// intentionally a thin aggregator — it sequences DAO lookups, the
// tenant/status authorisation guard, and delegates the heavy work
// (LLM call, canvas run) to the existing services.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/dsl"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// BotService coordinates chatbot + agentbot reads and the matching
// completion paths. Mirrors the Python
// `api/db/services/conversation_service.py::async_iframe_completion`
// + `api/db/services/canvas_service.py::completion` flow but stays
// stateless — it does not own the LLM or canvas runner; it just
// sequences them.
type BotService struct {
	chatDAO             *dao.ChatSessionDAO
	canvasDAO           *dao.UserCanvasDAO
	api4ConversationDAO *dao.API4ConversationDAO
	agentService        *AgentService
	llmService          *LLMService
}

// NewBotService wires a fresh BotService. agentSvc is required for
// AgentbotCompletion; llmSvc is required for ChatbotCompletion (in
// step 6). Both are nullable in unit tests.
func NewBotService(agentSvc *AgentService, llmSvc *LLMService) *BotService {
	return &BotService{
		chatDAO:             dao.NewChatSessionDAO(),
		canvasDAO:           dao.NewUserCanvasDAO(),
		api4ConversationDAO: dao.NewAPI4ConversationDAO(),
		agentService:        agentSvc,
		llmService:          llmSvc,
	}
}

// ChatbotInfo returns the public metadata of a chatbot dialog.
//
// Mirrors the python `bot_api.py::chatbot_info` handler. The
// authorisation check is: dialog must exist, the requester must own
// it (TenantID match), and Status must equal common.StatusDialogValid
// (the python StatusEnum.VALID.value).
func (s *BotService) ChatbotInfo(ctx context.Context, tenantID, dialogID string) (
	title, avatar, prologue, llmID string, hasTavilyKey bool, ec common.ErrorCode, err error,
) {
	dialog, err := s.chatDAO.GetDialogByID(dialogID)
	if err != nil {
		return "", "", "", "", false, common.CodeDataError, err
	}
	if dialog == nil || dialog.TenantID != tenantID ||
		dialog.Status == nil || *dialog.Status != common.StatusDialogValid {
		return "", "", "", "", false, common.CodeDataError,
			errors.New("Authentication error: no access to this chatbot!")
	}
	pc := dialog.PromptConfig
	// Defensive lookups mirroring python's
	// dialog.prompt_config.get("prologue", "") and
	// dialog.prompt_config.get("tavily_api_key", "").strip()
	// semantics. A hard type assertion here would panic on a missing
	// or non-string prologue field — this endpoint is public over
	// persisted JSON config and the schema is not guaranteed.
	prologue = stringFromMap(pc, "prologue")
	tk := stringFromMap(pc, "tavily_api_key")
	return botDerefStr(dialog.Name), botDerefStr(dialog.Icon), prologue,
		dialog.LLMID, strings.TrimSpace(tk) != "", common.CodeSuccess, nil
}

// AgentbotInputs returns the public metadata of an agentbot canvas.
//
// Mirrors the python `bot_api.py::agentbot_inputs` handler. The
// authorisation check is the same IDOR guard the production
// AgentService uses (canvas must be visible to the requesting user).
func (s *BotService) AgentbotInputs(ctx context.Context, tenantID, agentID string) (
	title, avatar, prologue, mode string, inputs map[string]any,
	ec common.ErrorCode, err error,
) {
	cv, err := s.loadCanvas(ctx, tenantID, agentID)
	if err != nil {
		return "", "", "", "", nil, common.CodeDataError, err
	}
	dslMap := canvasDSLMap(cv)
	// Resolve the begin component ID first, then pass that ID to
	// ExtractComponentInputForm. ExtractComponentInputForm is keyed
	// by component ID, NOT component name — passing the literal
	// "begin" would only succeed when the canvas happens to use
	// "begin" as the component ID.
	beginID, idErr := dsl.FindBeginComponentID(dslMap)
	if idErr != nil {
		// No begin component (or malformed DSL). Degrade gracefully —
		// empty prologue / mode / inputs, matching the Python
		// behaviour when Canvas.get_component_input_form returns an
		// empty dict.
		return botDerefStr(cv.Title), botDerefStr(cv.Avatar), "", "", nil, common.CodeSuccess, nil
	}
	inputs, _ = dsl.ExtractComponentInputForm(dslMap, beginID)
	prologue, _ = dsl.ExtractPrologue(dslMap)
	mode, _ = dsl.ExtractMode(dslMap)
	return botDerefStr(cv.Title), botDerefStr(cv.Avatar), prologue, mode, inputs, common.CodeSuccess, nil
}

// AgentbotCompletion is a thin wrapper around AgentService.RunAgent
// for the /api/v1/agentbots/<agent_id>/completions endpoint.
//
// Defence-in-depth (security H2): the IDOR guard runs BEFORE the
// delegate so an unauthorised caller can never trigger canvas
// compile/invoke (which would spend LLM tokens + emit canvas
// telemetry even for "not found" paths). RunAgent re-runs the
// same guard internally — this is intentional; the upstream check
// is the cheap fast-fail that costs a single DAO roundtrip
// instead of a full canvas compile.
func (s *BotService) AgentbotCompletion(
	ctx context.Context, tenantID, agentID string, req AgentbotCompletionRequest,
) (<-chan canvas.RunEvent, common.ErrorCode, error) {
	if s.agentService == nil {
		return nil, common.CodeServerError, fmt.Errorf("bot: agent service not wired")
	}
	if _, err := s.loadCanvas(ctx, tenantID, agentID); err != nil {
		return nil, common.CodeDataError, err
	}
	// Compose the canvas user input from req.UserInput (the
	// `inputs` dict body field) plus the top-level `question` and
	// `files` fields. The python canvas_service.completion at
	// api/db/services/canvas_service.py:313 reads all three; the
	// previous code dropped question/files, so a body like
	// `{"question":"hi"}` reached the canvas with empty inputs.
	userInput := make(map[string]any, len(req.UserInput)+2)
	for k, v := range req.UserInput {
		userInput[k] = v
	}
	if req.Question != "" {
		userInput["question"] = req.Question
	}
	if len(req.Files) > 0 {
		userInput["files"] = req.Files
	}
	ch, err := s.agentService.RunAgent(ctx, tenantID, agentID,
		req.SessionID, "", userInput)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	return ch, common.CodeSuccess, nil
}

// AgentbotCompletionRequest is the request body for
// /api/v1/agentbots/<agent_id>/completions. We intentionally accept
// the same fields the production /agents/chat/completions handler
// accepts; the URL-bound agent_id is the authoritative canvas id
// (matches python bot_api.py:159).
type AgentbotCompletionRequest struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Stream    bool   `json:"stream"`
	// UserInput is the dict-shaped root input the canvas run expects
	// (mirrors the python "question"/"files"/"inputs" trio collapsed
	// into one map).
	UserInput map[string]any `json:"inputs"`
	Question  string         `json:"question"`
	Files     []string       `json:"files"`
}

// ChatbotCompletionRequest is the request body for
// /api/v1/chatbots/<dialog_id>/completions. Mirrors the python
// `async_iframe_completion` body shape (session_id, question,
// tts (unused) and a freeform dict).
type ChatbotCompletionRequest struct {
	SessionID string         `json:"session_id"`
	Question  string         `json:"question"`
	Stream    bool           `json:"stream"`
	Inputs    map[string]any `json:"inputs"`
}

// loadCanvas is the IDOR guard for agentbot reads. It mirrors the
// private loadCanvasForUser helper on AgentService without taking a
// dependency on the agentService pointer (so BotService can be unit-
// tested with a nil agentService).
func (s *BotService) loadCanvas(ctx context.Context, tenantID, agentID string) (*entity.UserCanvas, error) {
	if agentID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	if tenantID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	userTenantDAO := dao.NewUserTenantDAO()
	tenants, err := userTenantDAO.GetTenantIDsByUserID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("bot: tenants for user %s: %w", tenantID, err)
	}
	return s.canvasDAO.GetByIDForUser(agentID, tenantID, tenants)
}

// canvasDSLMap projects a UserCanvas.DSL JSONMap into a
// map[string]any. Returns an empty map (not nil) on miss so
// downstream dsl helpers can still scan it.
func canvasDSLMap(cv *entity.UserCanvas) map[string]any {
	if cv == nil {
		return map[string]any{}
	}
	// cv.DSL is entity.JSONMap (alias for map[string]interface{}).
	// We must return a fresh map[string]any because the dsl
	// helpers expect that concrete type.
	return map[string]any(cv.DSL)
}

// botDerefStr returns *s or "" if nil. Used to read pointer-string
// fields on entities (Name, Icon, Title, Avatar). Prefixed with bot
// to avoid colliding with the test-only botDerefStr in
// openai_chat_test.go.
func botDerefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// stringFromMap returns m[key] as a string. Returns "" if the key is
// absent or the value is not a string. Used for defensive reads
// over JSONMap-shaped fields (dialog.prompt_config) where a hard
// type assertion would panic.
func stringFromMap(m entity.JSONMap, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
