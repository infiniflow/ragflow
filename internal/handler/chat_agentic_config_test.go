package handler

import (
	"encoding/json/v2"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

func TestGetAgenticRAGConfigHandler(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tenantID string
		wantCode common.ErrorCode
	}{
		{name: "owner", tenantID: "user-1", wantCode: common.CodeSuccess},
		{name: "other tenant", tenantID: "other-tenant", wantCode: common.CodeNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupChatHandlerTestDB(t)
			createChatHandlerTestChat(t, db, "chat-1", tc.tenantID)
			t.Setenv("AGENTIC_RAG_CONFIG", filepath.Join("..", "..", "conf", "agentic_rag.yaml"))
			h := NewChatHandler(service.NewChatService(), service.NewUserService())
			c, w := setupGinContextWithUser("GET", "/api/v1/chats/chat-1/agentic-rag", "")
			c.Params = []gin.Param{{Key: "chat_id", Value: "chat-1"}}

			h.GetAgenticRAGConfig(c)

			if w.Code != http.StatusOK {
				t.Fatalf("HTTP status = %d: %s", w.Code, w.Body.String())
			}
			var response struct {
				Code common.ErrorCode `json:"code"`
				Data map[string]any   `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.wantCode {
				t.Fatalf("response code = %d, want %d: %s", response.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantCode == common.CodeSuccess && len(response.Data) == 0 {
				t.Fatal("missing settings response")
			}
			if tc.wantCode != common.CodeSuccess && response.Data != nil {
				t.Fatalf("unauthorized response exposes settings: %#v", response.Data)
			}
		})
	}
}
