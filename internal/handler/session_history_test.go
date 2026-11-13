package handler

import (
	"encoding/json"
	"io"
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
	for _, endpoint := range []string{"chat", "chat_list", "agent", "agent_list", "create", "update", "delete_message", "feedback"} {
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
				mutation := endpoint == "create" || endpoint == "update" || endpoint == "delete_message" || endpoint == "feedback"
				c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/?"+flags.query)
				if mutation {
					body := `{"name":"session"}`
					if endpoint == "feedback" {
						body = `{"thumbup":true}`
					}
					c.Request.Body = io.NopCloser(strings.NewReader(body))
				}
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
				if mutation {
					for _, update := range []dao.ConversationHistoryUpdate{
						{Message: map[string]interface{}{"id": "turn", "role": "user", "content": "question"}},
						{QuestionID: "turn", Message: map[string]interface{}{"id": "turn", "role": "assistant", "content": "answer", "prompt": "large prompt", "reference": map[string]interface{}{"chunks": []interface{}{}}}},
					} {
						if err := dao.NewChatSessionDAO().UpdateByID(c.Request.Context(), db, "session", map[string]interface{}{"history_update": update}); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := dao.NewAPI4ConversationDAO().Create(c.Request.Context(), db, &entity.API4Conversation{ID: "session", DialogID: "agent", UserID: "user-1", Message: message}); err != nil {
					t.Fatal(err)
				}
				var historySQL string
				if err := db.Callback().Query().After("gorm:query").Register("capture_session_history", func(tx *gorm.DB) {
					sql := tx.Statement.SQL.String()
					if strings.Contains(sql, "`content`") && (strings.Contains(sql, "FROM `conversation_message`") || strings.Contains(sql, "FROM `api_4_conversation_message`")) {
						historySQL = sql
					}
				}); err != nil {
					t.Fatal(err)
				}
				chat := NewChatSessionHandler(service.NewChatSessionService(), nil)
				agent := NewAgentHandler(c.Request.Context(), service.NewAgentService(), nil)
				c.Params = gin.Params{{Key: "chat_id", Value: "chat"}, {Key: "canvas_id", Value: "agent"}, {Key: "session_id", Value: "session"}, {Key: "msg_id", Value: "turn"}}
				switch endpoint {
				case "chat":
					chat.GetSession(c)
				case "chat_list":
					chat.ListChatSessions(c)
				case "agent":
					agent.GetAgentSession(c)
				case "agent_list":
					agent.ListAgentSessions(c)
				case "create":
					chat.CreateSession(c)
				case "update":
					chat.UpdateSession(c)
				case "delete_message":
					chat.DeleteSessionMessage(c)
				case "feedback":
					chat.UpdateMessageFeedback(c)
				}
				var response struct {
					Code common.ErrorCode `json:"code"`
					Data json.RawMessage  `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if flags.invalid && !mutation {
					if response.Code != common.CodeArgumentError || historySQL != "" {
						t.Fatalf("invalid flags should fail before history SQL: %s", w.Body.String())
					}
					return
				}
				if response.Code != common.CodeSuccess {
					t.Fatalf("session request failed: %s", w.Body.String())
				}
				if mutation {
					var session struct {
						Messages []map[string]interface{} `json:"messages"`
					}
					if err := json.Unmarshal(response.Data, &session); err != nil || len(session.Messages) == 0 {
						t.Fatalf("invalid mutation response: %s, %v", response.Data, err)
					}
					for _, message := range session.Messages {
						for _, field := range []string{"reference", "prompt"} {
							if _, exists := message[field]; exists {
								t.Fatalf("mutation returned %s: %s", field, response.Data)
							}
						}
					}
					if strings.Contains(historySQL, "`prompt`") || strings.Contains(historySQL, "`reference`") != (endpoint == "feedback") {
						t.Fatalf("mutation loaded unneeded large fields: %s", historySQL)
					}
					return
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
				if endpoint != "chat" && endpoint != "chat_list" {
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

func TestGetAgentSessionIncludeDSL(t *testing.T) {
	for _, tt := range []struct {
		name       string
		query      string
		dsl        string
		code       common.ErrorCode
		includeDSL bool
	}{
		{name: "default", dsl: `{"components":{}}`, code: common.CodeSuccess},
		{name: "false", query: "include_dsl=false", dsl: `{"components":{}}`, code: common.CodeSuccess},
		{name: "true", query: "include_dsl=true", dsl: `{"components":{}}`, code: common.CodeSuccess, includeDSL: true},
		{name: "default skips invalid DSL", dsl: `"invalid"`, code: common.CodeSuccess},
		{name: "false skips invalid DSL", query: "include_dsl=false", dsl: `"invalid"`, code: common.CodeSuccess},
		{name: "true reports invalid DSL", query: "include_dsl=true", dsl: `"invalid"`, code: common.CodeServerError, includeDSL: true},
		{name: "invalid boolean", query: "include_dsl=invalid", dsl: `{}`, code: common.CodeArgumentError},
		{name: "empty boolean", query: "include_dsl=", dsl: `{}`, code: common.CodeArgumentError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/agent/sessions/session?"+tt.query)
			c.Params = gin.Params{{Key: "canvas_id", Value: "agent"}, {Key: "session_id", Value: "session"}}
			if err := db.Create(&entity.UserCanvas{ID: "agent", UserID: "user-1"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.NewAPI4ConversationDAO().Create(c.Request.Context(), db, &entity.API4Conversation{
				ID: "session", DialogID: "agent", UserID: "user-1",
				Message: json.RawMessage(`[{"id":"answer","role":"assistant","content":"answer"}]`),
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session").Update("dsl", tt.dsl).Error; err != nil {
				t.Fatal(err)
			}
			var sessionSQL string
			if err := db.Callback().Query().After("gorm:query").Register("capture_session_dsl", func(tx *gorm.DB) {
				if tx.Statement.Table == "api_4_conversation" {
					sessionSQL = tx.Statement.SQL.String()
				}
			}); err != nil {
				t.Fatal(err)
			}
			NewAgentHandler(c.Request.Context(), service.NewAgentService(), nil).GetAgentSession(c)
			var response struct {
				Code common.ErrorCode           `json:"code"`
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tt.code {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
			if tt.code == common.CodeArgumentError {
				if sessionSQL != "" {
					t.Fatalf("invalid query reached the database: %s", sessionSQL)
				}
				return
			}
			if sessionSQL == "" || strings.Contains(sessionSQL, "`dsl`") != tt.includeDSL {
				t.Fatalf("unexpected DSL selection: %s", sessionSQL)
			}
			if tt.code != common.CodeSuccess {
				return
			}
			if _, exists := response.Data["dsl"]; exists != tt.includeDSL {
				t.Fatalf("unexpected DSL in response: %s", w.Body.String())
			}
			if tt.includeDSL && string(response.Data["dsl"]) != tt.dsl {
				t.Fatalf("unexpected DSL value: %s", response.Data["dsl"])
			}
			var messages []map[string]interface{}
			if err := json.Unmarshal(response.Data["message"], &messages); err != nil || len(messages) != 1 || messages[0]["content"] != "answer" {
				t.Fatalf("unexpected session history: %s, %v", w.Body.String(), err)
			}
		})
	}
}
