package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

type fakeChatChannelService struct {
	createFn  func(context.Context, string, string, entity.JSONMap, *string) (*entity.ChatChannel, error)
	listFn    func(context.Context) ([]*entity.ChatChannelListResponse, error)
	getFn     func(context.Context, string) (*entity.ChatChannel, common.ErrorCode, error)
	updateFn  func(context.Context, string, map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error)
	runtimeFn func(context.Context, string) (map[string]any, common.ErrorCode, error)
	deleteFn  func(context.Context, string) (bool, common.ErrorCode, error)
}

func (f fakeChatChannelService) CreateChatChannel(ctx context.Context, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
	if f.createFn == nil {
		return nil, errors.New("unexpected CreateChatChannel call")
	}
	return f.createFn(ctx, name, channelType, config, chatID)
}

func (f fakeChatChannelService) List(ctx context.Context) ([]*entity.ChatChannelListResponse, error) {
	if f.listFn == nil {
		return nil, errors.New("unexpected List call")
	}
	return f.listFn(ctx)
}

func (f fakeChatChannelService) GetChatChannel(ctx context.Context, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	if f.getFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected GetChatChannel call")
	}
	return f.getFn(ctx, channelID)
}

func (f fakeChatChannelService) UpdateChatChannel(ctx context.Context, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	if f.updateFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected UpdateChatChannel call")
	}
	return f.updateFn(ctx, channelID, req)
}

func (f fakeChatChannelService) GetChatChannelRuntime(ctx context.Context, channelID string) (map[string]any, common.ErrorCode, error) {
	if f.runtimeFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected GetChatChannelRuntime call")
	}
	return f.runtimeFn(ctx, channelID)
}

func (f fakeChatChannelService) DeleteChatChannel(ctx context.Context, channelID string) (bool, common.ErrorCode, error) {
	if f.deleteFn == nil {
		return false, common.CodeServerError, errors.New("unexpected DeleteChatChannel call")
	}
	return f.deleteFn(ctx, channelID)
}

func setChatChannelHandlerIdentity(c *gin.Context) {
	c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "tenant-1", "tenant-1"))
}

func TestChatChannelHandlerCreateSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotTenantID, gotName, gotChannel string
	var gotConfig entity.JSONMap
	var gotChatID *string

	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			createFn: func(ctx context.Context, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
				gotTenantID = permission.TenantIDFromContext(ctx)
				gotName = name
				gotChannel = channelType
				gotConfig = config
				gotChatID = chatID
				return &entity.ChatChannel{
					ID:       "cc-1",
					TenantID: permission.TenantIDFromContext(ctx),
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
		setChatChannelHandlerIdentity(c)
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
			createFn: func(ctx context.Context, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
				called = true
				return nil, nil
			},
		},
	}

	router := gin.New()
	router.POST("/api/v1/chat-channels", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
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
			listFn: func(ctx context.Context) ([]*entity.ChatChannelListResponse, error) {
				gotTenantID = permission.TenantIDFromContext(ctx)
				return []*entity.ChatChannelListResponse{
					{ID: "cc-1", Name: "bot-a", Channel: "dingtalk", Status: 1},
				}, nil
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
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
			listFn: func(ctx context.Context) ([]*entity.ChatChannelListResponse, error) {
				return nil, errors.New("db failed")
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
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

func TestChatChannelHandlerGetChatChannelHidesPermissionDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			getFn: func(ctx context.Context, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
				return nil, common.CodeNotFound, errors.New("Resource not found")
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels/:channel_id", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
		h.GetChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat-channels/cc-1", nil)
	router.ServeHTTP(resp, req)

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeNotFound) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["data"] != nil {
		t.Fatalf("payload=%v", payload)
	}
}

func TestChatChannelHandlerUpdateChatChannelUnwrapsData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotReq map[string]interface{}
	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			updateFn: func(ctx context.Context, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
				gotReq = req
				return &entity.ChatChannel{ID: channelID, TenantID: permission.TenantIDFromContext(ctx), Name: "new-name"}, common.CodeSuccess, nil
			},
		},
	}

	router := gin.New()
	router.PATCH("/api/v1/chat-channels/:channel_id", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
		h.UpdateChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat-channels/cc-1", strings.NewReader(`{"data":{"name":"new-name"}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if gotReq["name"] != "new-name" {
		t.Fatalf("req=%v", gotReq)
	}
}

func TestChatChannelHandlerDeleteChatChannelSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotUserID, gotChannelID string
	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			deleteFn: func(ctx context.Context, channelID string) (bool, common.ErrorCode, error) {
				gotUserID = permission.UserIDFromContext(ctx)
				gotChannelID = channelID
				return true, common.CodeSuccess, nil
			},
		},
	}

	router := gin.New()
	router.DELETE("/api/v1/chat-channels/:channel_id", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
		h.DeleteChatChannel(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/chat-channels/cc-1", nil)
	router.ServeHTTP(resp, req)

	if gotUserID != "tenant-1" || gotChannelID != "cc-1" {
		t.Fatalf("userID=%q channelID=%q", gotUserID, gotChannelID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if data, exists := payload["data"]; exists {
		t.Fatalf("data must be omitted, got %v", data)
	}
}

func TestChatChannelHandlerGetRuntimeSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotUserID, gotChannelID string
	h := &ChatChannelHandler{
		chatChannelService: fakeChatChannelService{
			runtimeFn: func(ctx context.Context, channelID string) (map[string]any, common.ErrorCode, error) {
				gotUserID = permission.UserIDFromContext(ctx)
				gotChannelID = channelID
				return map[string]any{"status": "waiting"}, common.CodeSuccess, nil
			},
		},
	}

	router := gin.New()
	router.GET("/api/v1/chat-channels/:channel_id/runtime", func(c *gin.Context) {
		setChatChannelHandlerIdentity(c)
		h.GetChatChannelRuntime(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat-channels/cc-1/runtime", nil)
	router.ServeHTTP(resp, req)

	if gotUserID != "tenant-1" || gotChannelID != "cc-1" {
		t.Fatalf("userID=%q channelID=%q", gotUserID, gotChannelID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	data, _ := payload["data"].(map[string]interface{})
	if data["status"] != "waiting" {
		t.Fatalf("payload=%v", payload)
	}
}
