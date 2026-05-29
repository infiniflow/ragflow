package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

type fakeBotService struct {
	tenantID string
	authErr  error
	info     map[string]interface{}
	infoErr  error
	detail   map[string]interface{}
	detErr   error
}

func (s fakeBotService) AuthByBetaToken(string) (string, error) {
	return s.tenantID, s.authErr
}

func (s fakeBotService) GetChatbotInfo(string, string) (map[string]interface{}, error) {
	return s.info, s.infoErr
}

func (s fakeBotService) GetSearchbotDetail(string, string) (map[string]interface{}, error) {
	return s.detail, s.detErr
}

func TestBotHandlerChatbotInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		auth     string
		service  fakeBotService
		wantCode common.ErrorCode
	}{
		{
			name:     "success",
			auth:     "Bearer beta-token",
			service:  fakeBotService{tenantID: "t1", info: map[string]interface{}{"title": "Bot"}},
			wantCode: common.CodeSuccess,
		},
		{
			name:     "missing authorization",
			auth:     "",
			service:  fakeBotService{},
			wantCode: common.CodeDataError,
		},
		{
			name:     "invalid api key",
			auth:     "Bearer bad",
			service:  fakeBotService{authErr: service.ErrBotInvalidAPIKey},
			wantCode: common.CodeDataError,
		},
		{
			name:     "no access",
			auth:     "Bearer beta-token",
			service:  fakeBotService{tenantID: "t1", infoErr: service.ErrBotNoChatbotAccess},
			wantCode: common.CodeDataError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &BotHandler{botService: tt.service}
			router := gin.New()
			router.GET("/api/v1/chatbots/:dialog_id/info", h.ChatbotInfo)

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/chatbots/dialog-1/info", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v want=%v body=%v", body["code"], tt.wantCode, body)
			}
		})
	}
}

func TestBotHandlerSearchbotDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		query    string
		service  fakeBotService
		wantCode common.ErrorCode
		wantData interface{}
	}{
		{
			name:     "success",
			query:    "?search_id=s1",
			service:  fakeBotService{tenantID: "t1", detail: map[string]interface{}{"id": "s1"}},
			wantCode: common.CodeSuccess,
		},
		{
			name:     "missing search_id",
			query:    "",
			service:  fakeBotService{tenantID: "t1"},
			wantCode: common.CodeArgumentError,
		},
		{
			name:     "no permission",
			query:    "?search_id=s1",
			service:  fakeBotService{tenantID: "t1", detErr: service.ErrBotNoSearchPermission},
			wantCode: common.CodeOperatingError,
			wantData: false,
		},
		{
			name:     "not found",
			query:    "?search_id=s1",
			service:  fakeBotService{tenantID: "t1", detErr: service.ErrBotSearchNotFound},
			wantCode: common.CodeDataError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &BotHandler{botService: tt.service}
			router := gin.New()
			router.GET("/api/v1/searchbots/detail", h.SearchbotDetail)

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/searchbots/detail"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer beta-token")
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v want=%v body=%v", body["code"], tt.wantCode, body)
			}
			if tt.wantData != nil && body["data"] != tt.wantData {
				t.Fatalf("data=%v want=%v body=%v", body["data"], tt.wantData, body)
			}
		})
	}
}
