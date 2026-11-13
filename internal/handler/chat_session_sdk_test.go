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
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestListChatSessionsPublicSchemaAndUserFilter(t *testing.T) {
	for _, tc := range []struct {
		query        string
		count, total int
	}{
		{"", 3, 3}, {"user_id=user-1&page_size=1", 1, 2}, {"user_id=absent", 0, 0}, {"user_id=user-1&id=session-a", 1, 1}, {"user_id=absent&id=session-a", 0, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/?"+tc.query)
			if err := db.AutoMigrate(&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&entity.Chat{ID: "chat", TenantID: "user-1", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, Status: sptr("1")}).Error; err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"session-a", "session-b"} {
				if err := dao.NewChatSessionDAO().Create(t.Context(), db, &entity.ChatSession{ID: id, DialogID: "chat", UserID: sptr("user-1"), Message: json.RawMessage(`[{"role":"assistant","content":"hello"}]`)}); err != nil {
					t.Fatal(err)
				}
			}

			if err := dao.NewChatSessionDAO().Create(t.Context(), db, &entity.ChatSession{ID: "session-other-user", DialogID: "chat", UserID: sptr("external-user"), Message: json.RawMessage(`[{"role":"assistant","content":"hello"}]`)}); err != nil {
				t.Fatal(err)
			}
			c.Params = gin.Params{{Key: "chat_id", Value: "chat"}}
			NewChatSessionHandler(service.NewChatSessionService(), nil).ListChatSessions(c)
			var response struct {
				Code  int                      `json:"code"`
				Data  []map[string]interface{} `json:"data"`
				Total int                      `json:"total"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != 0 || len(response.Data) != tc.count || response.Total != tc.total {
				t.Fatalf("response=%s", w.Body.String())
			}
			for _, row := range response.Data {
				if row["chat_id"] != "chat" {
					t.Fatal(row)
				}
				messages, ok := row["messages"].([]interface{})
				if !ok || len(messages) != 1 {
					t.Fatal(row)
				}
				if _, ok := row["dialog_id"]; ok {
					t.Fatal(row)
				}
				if _, ok := row["message"]; ok {
					t.Fatal(row)
				}
			}
		})
	}
}
