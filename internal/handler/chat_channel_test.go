package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type fakeChatChannelService struct {
	createFn func(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error)
	listFn   func(tenantID string) ([]*entity.ChatChannelListResponse, error)
}

func (f fakeChatChannelService) CreateChatChannel(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
	if f.createFn == nil {
		return nil, errors.New("unexpected CreateChatChannel call")
	}
	return f.createFn(tenantID, name, channelType, config, chatID)
}

func (f fakeChatChannelService) List(tenantID string) ([]*entity.ChatChannelListResponse, error) {
	if f.listFn == nil {
		return nil, errors.New("unexpected List call")
	}
	return f.listFn(tenantID)
}

func TestChatChannelHandlerCreateSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotTenantID, gotName, gotChannel string
	var gotConfig entity.JSONMap
	var gotChatID *string

	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			createFn: func(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
				gotTenantID = tenantID
				gotName = name
				gotChannel = channelType
				gotConfig = config
				gotChatID = chatID
				return &entity.ChatChannel{
					ID:       "cc-1",
					TenantID: tenantID,
					Name:     name,
					Channel:  channelType,
					Config:   config,
					ChatID:   chatID,
					Status:   1,
				}, nil
			},
		},
	}

	router := gin.New()
	router.POST("/api/v1/chat-channels", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.CreateChatChannel(c)
	})

	body := `{"name":"bot-a","channel":"dingtalk","config":{"token":"abc"},"chat_id":"dialog-1"}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat-channels", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	if gotTenantID != "tenant-1" || gotName != "bot-a" || gotChannel != "dingtalk" {
		t.Fatalf("service args tenant=%q name=%q channel=%q", gotTenantID, gotName, gotChannel)
	}
	if gotConfig["token"] != "abc" {
		t.Fatalf("config=%v", gotConfig)
	}
	if gotChatID == nil || *gotChatID != "dialog-1" {
		t.Fatalf("chatID=%v", gotChatID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestChatChannelHandlerCreateInvalidRequestStopsEarly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			createFn: func(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
				called = true
				return nil, nil
			},
		},
	}

	router := gin.New()
	router.POST("/api/v1/chat-channels", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.CreateChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat-channels", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if called {
		t.Fatal("service should not be called when request binding fails")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestChatChannelHandlerListSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gotTenantID := ""
	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			listFn: func(tenantID string) ([]*entity.ChatChannelListResponse, error) {
				gotTenantID = tenantID
				return []*entity.ChatChannelListResponse{
					{ID: "cc-1", Name: "bot-a", Channel: "dingtalk", Status: 1},
				}, nil
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.ListChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat-channels", nil)
	router.ServeHTTP(resp, req)

	if gotTenantID != "tenant-1" {
		t.Fatalf("tenantID=%q", gotTenantID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestChatChannelHandlerListServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			listFn: func(tenantID string) ([]*entity.ChatChannelListResponse, error) {
				return nil, errors.New("db failed")
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.ListChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat-channels", nil)
	router.ServeHTTP(resp, req)

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeServerError) {
		t.Fatalf("payload=%v", payload)
	}
}
