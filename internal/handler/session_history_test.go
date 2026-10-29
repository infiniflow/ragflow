package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestSessionHistoryQueryFlags(t *testing.T) {
	for _, endpoint := range []string{"chat", "chat_list", "agent", "agent_list"} {
		for _, flags := range []struct {
			query     string
			reference bool
			prompt    bool
			invalid   bool
		}{
			{},
			{query: "include_reference=false&include_prompt=false"},
			{query: "include_reference=true", reference: true},
			{query: "include_prompt=true", prompt: true},
			{query: "include_reference=true&include_prompt=true", reference: true, prompt: true},
			{query: "include_reference=invalid", invalid: true},
			{query: "include_prompt=invalid", invalid: true},
		} {
			t.Run(endpoint+"/"+flags.query, func(t *testing.T) {
				c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/?"+flags.query)
				if err := db.AutoMigrate(&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}, &entity.API4ConversationMessage{}); err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&entity.Chat{ID: "chat", TenantID: "user-1", LLMID: "model", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DoRefer: "1", Status: sptr("1")}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&entity.UserCanvas{ID: "agent", UserID: "user-1"}).Error; err != nil {
					t.Fatal(err)
				}
				message := json.RawMessage(`[{"id":"answer","role":"assistant","content":"answer","reference":{"chunks":[],"doc_aggs":[]},"prompt":"large prompt","usage":{"total_tokens":1},"elapsed_ms":50}]`)
				if err := dao.NewChatSessionDAO().Create(c.Request.Context(), db, &entity.ChatSession{ID: "session", DialogID: "chat", UserID: sptr("user-1"), Message: message}); err != nil {
					t.Fatal(err)
				}
				if err := dao.NewAPI4ConversationDAO().Create(c.Request.Context(), db, &entity.API4Conversation{ID: "session", DialogID: "agent", UserID: "user-1", Message: message}); err != nil {
					t.Fatal(err)
				}
				var historySQL string
				if err := db.Callback().Query().After("gorm:query").Register("capture_session_history", func(tx *gorm.DB) {
					sql := tx.Statement.SQL.String()
					if strings.Contains(sql, "FROM `conversation_message`") || strings.Contains(sql, "FROM `api_4_conversation_message`") {
						historySQL = sql
					}
				}); err != nil {
					t.Fatal(err)
				}
				chat := NewChatSessionHandler(service.NewChatSessionService(), nil)
				agent := NewAgentHandler(c.Request.Context(), service.NewAgentService(), nil)
				c.Params = gin.Params{{Key: "chat_id", Value: "chat"}, {Key: "canvas_id", Value: "agent"}, {Key: "session_id", Value: "session"}}
				switch endpoint {
				case "chat":
					chat.GetSession(c)
				case "chat_list":
					chat.ListChatSessions(c)
				case "agent":
					agent.GetAgentSession(c)
				case "agent_list":
					agent.ListAgentSessions(c)
				}
				var response struct {
					Code common.ErrorCode `json:"code"`
					Data json.RawMessage  `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if flags.invalid {
					if response.Code != common.CodeArgumentError || historySQL != "" {
						t.Fatalf("invalid flags should fail before history SQL: %s", w.Body.String())
					}
					return
				}
				if response.Code != common.CodeSuccess {
					t.Fatalf("session request failed: %s", w.Body.String())
				}
				data := response.Data
				if strings.HasSuffix(endpoint, "_list") {
					var rows []json.RawMessage
					if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 1 {
						t.Fatalf("invalid session list: %s, %v", data, err)
					}
					data = rows[0]
				}
				var session map[string]json.RawMessage
				if err := json.Unmarshal(data, &session); err != nil {
					t.Fatal(err)
				}
				key := "messages"
				if endpoint != "chat" {
					key = "message"
				}
				var messages []map[string]interface{}
				if err := json.Unmarshal(session[key], &messages); err != nil || len(messages) != 1 {
					t.Fatalf("invalid messages: %s, %v", session[key], err)
				}
				for field, include := range map[string]bool{"reference": flags.reference, "prompt": flags.prompt, "usage": true, "elapsed_ms": true} {
					if _, exists := messages[0][field]; exists != include {
						t.Fatalf("unexpected %s in response: %s", field, w.Body.String())
					}
					if strings.Contains(historySQL, "`"+field+"`") != include {
						t.Fatalf("unexpected %s in SQL: %s", field, historySQL)
					}
				}
			})
		}
	}
}
