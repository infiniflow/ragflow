package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type fakeTenantService struct {
	gotTenantID string
	gotUserID   string
	gotEmail    string
	createCalls int
}

func (f *fakeTenantService) ListTenantDefaultModels(userID string) ([]service.ModelItem, error) {
	return nil, nil
}

func (f *fakeTenantService) SetTenantDefaultModels(userID, modelProvider, modelInstance, modelName, modelType string) error {
	return nil
}

func (f *fakeTenantService) GetTenantInfo(userID string) (*service.TenantInfoResponse, error) {
	return nil, nil
}

func (f *fakeTenantService) GetTenantList(userID string) ([]*service.TenantListItem, error) {
	return nil, nil
}

func (f *fakeTenantService) InviteTenantUser(tenantID, currentUserID, email string) (*service.TenantInvitedUserResponse, common.ErrorCode, error) {
	f.gotTenantID = tenantID
	f.gotUserID = currentUserID
	f.gotEmail = email
	if strings.TrimSpace(email) == "" {
		return nil, common.CodeDataError, commonErr("email is required")
	}
	avatar := "https://example.com/avatar.png"
	return &service.TenantInvitedUserResponse{
		ID:       "invitee123",
		Avatar:   &avatar,
		Email:    email,
		Nickname: "Invited User",
	}, common.CodeSuccess, nil
}

func (f *fakeTenantService) CreateTenantUserInvite(tenantID, currentUserID, email string) (common.ErrorCode, error) {
	f.createCalls++
	return common.CodeSuccess, nil
}

func (f *fakeTenantService) CreateMetadataStore(tenantID string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}

func (f *fakeTenantService) DeleteMetadataStore(tenantID string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}

func (f *fakeTenantService) CreateChunkStore(req *service.CreateDatasetTableRequest) (*service.CreateChunkStoreResponse, common.ErrorCode, error) {
	return nil, common.CodeSuccess, nil
}

func (f *fakeTenantService) DeleteChunkStore(kbID string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}

type commonErr string

func (e commonErr) Error() string {
	return string(e)
}

func TestAddTenantUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalSendInvite := sendTenantInviteEmail
	sendTenantInviteEmail = func(toEmail, recipientEmail, tenantID, inviter string) error {
		if toEmail != "invitee@example.com" {
			t.Fatalf("toEmail = %q, want invitee@example.com", toEmail)
		}
		if tenantID != "tenant123" {
			t.Fatalf("tenantID = %q, want tenant123", tenantID)
		}
		if inviter != "Owner User" {
			t.Fatalf("inviter = %q, want Owner User", inviter)
		}
		return nil
	}
	defer func() {
		sendTenantInviteEmail = originalSendInvite
	}()

	tenantService := &fakeTenantService{}
	h := &TenantHandler{tenantService: tenantService}
	r := gin.New()
	r.POST("/api/v1/tenants/:tenant_id/users", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant123", Nickname: "Owner User", Email: "owner@example.com"})
		h.AddTenantUser(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/tenant123/users", strings.NewReader(`{"email":"invitee@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if tenantService.gotTenantID != "tenant123" {
		t.Fatalf("tenantID = %q, want tenant123", tenantService.gotTenantID)
	}
	if tenantService.gotUserID != "tenant123" {
		t.Fatalf("userID = %q, want tenant123", tenantService.gotUserID)
	}
	if tenantService.gotEmail != "invitee@example.com" {
		t.Fatalf("email = %q, want invitee@example.com", tenantService.gotEmail)
	}
	if tenantService.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", tenantService.createCalls)
	}

	var resp struct {
		Code common.ErrorCode                  `json:"code"`
		Data service.TenantInvitedUserResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != common.CodeSuccess {
		t.Fatalf("code = %d, want %d, body: %s", resp.Code, common.CodeSuccess, w.Body.String())
	}
	if resp.Data.ID != "invitee123" || resp.Data.Email != "invitee@example.com" {
		t.Fatalf("response data = %+v", resp.Data)
	}
}

func TestAddTenantUserAllowsServiceValidationForMissingEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tenantService := &fakeTenantService{}
	h := &TenantHandler{tenantService: tenantService}
	r := gin.New()
	r.POST("/api/v1/tenants/:tenant_id/users", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant123", Nickname: "Owner User", Email: "owner@example.com"})
		h.AddTenantUser(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/tenant123/users", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	var resp struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != common.CodeDataError {
		t.Fatalf("code = %d, want %d, body: %s", resp.Code, common.CodeDataError, w.Body.String())
	}
	if resp.Message != "email is required" {
		t.Fatalf("message = %q, want %q", resp.Message, "email is required")
	}
}

func TestAddTenantUserDoesNotPersistInviteWhenEmailSendFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalSendInvite := sendTenantInviteEmail
	sendTenantInviteEmail = func(toEmail, recipientEmail, tenantID, inviter string) error {
		return commonErr("smtp down")
	}
	defer func() {
		sendTenantInviteEmail = originalSendInvite
	}()

	tenantService := &fakeTenantService{}
	h := &TenantHandler{tenantService: tenantService}
	r := gin.New()
	r.POST("/api/v1/tenants/:tenant_id/users", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant123", Nickname: "Owner User", Email: "owner@example.com"})
		h.AddTenantUser(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/tenant123/users", strings.NewReader(`{"email":"invitee@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	var resp struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != common.CodeServerError {
		t.Fatalf("code = %d, want %d, body: %s", resp.Code, common.CodeServerError, w.Body.String())
	}
	if resp.Message != "Failed to send invite email." {
		t.Fatalf("message = %q, want %q", resp.Message, "Failed to send invite email.")
	}
	if tenantService.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", tenantService.createCalls)
	}
}
