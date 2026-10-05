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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type fakeLangfuseService struct {
	setFn    func(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error)
	getFn    func(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error)
	deleteFn func(ctx context.Context, tenantID string) (bool, common.ErrorCode, string, error)
}

func (f fakeLangfuseService) SetAPIKey(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
	if f.setFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected SetAPIKey call")
	}
	return f.setFn(ctx, tenantID, secretKey, publicKey, host)
}

func (f fakeLangfuseService) GetAPIKey(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
	if f.getFn == nil {
		return nil, common.CodeServerError, "", errors.New("unexpected GetAPIKey call")
	}
	return f.getFn(ctx, tenantID)
}

func (f fakeLangfuseService) DeleteAPIKey(ctx context.Context, tenantID string) (bool, common.ErrorCode, string, error) {
	if f.deleteFn == nil {
		return false, common.CodeServerError, "", errors.New("unexpected DeleteAPIKey call")
	}
	return f.deleteFn(ctx, tenantID)
}

// fakeUserTenantFinder satisfies handler.userTenantFinder without a real DB.
// The default constructor (called with no fields set) returns a single row
// mapping the test user "tenant-1" to tenant id "tenant-1" so the existing
// happy-path tests pass without modification. Tests for the resolution
// failure mode override the findFn field.
type fakeUserTenantFinder struct {
	findFn func(ctx context.Context, userID, role string) ([]entity.UserTenant, error)
}

func (f fakeUserTenantFinder) GetByUserIDAndRole(_ context.Context, _ *gorm.DB, userID, role string) ([]entity.UserTenant, error) {
	if f.findFn != nil {
		return f.findFn(context.Background(), userID, role)
	}
	return []entity.UserTenant{{UserID: userID, TenantID: "tenant-1", Role: role}}, nil
}

// stubTenantFinder returns a finder whose resolution matches the supplied
// tenants (or an empty slice if tenants is nil). Convenience for the
// failure-path tests below.
func stubTenantFinder(tenants []entity.UserTenant, err error) fakeUserTenantFinder {
	return fakeUserTenantFinder{
		findFn: func(_ context.Context, userID, role string) ([]entity.UserTenant, error) {
			if err != nil {
				return nil, err
			}
			if tenants == nil {
				return nil, nil
			}
			return tenants, nil
		},
	}
}

func serveLangfuse(method, target, body string, h func(c *gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, target, func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h(c)
	})

	resp := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(resp, req)
	return resp
}

// serveLangfuseAs serves the request as the given user. The user id is what
// gets passed to userTenantFinder.GetByUserIDAndRole.
func serveLangfuseAs(method, target, body, userID string, h func(c *gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, target, func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
		h(c)
	})

	resp := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(resp, req)
	return resp
}

func decode(t *testing.T, resp *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, resp.Body.String())
	}
	return payload
}

func TestLangfuseHandler_SetAPIKey_Success(t *testing.T) {
	var gotTenant, gotSecret, gotPublic, gotHost string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			gotTenant, gotSecret, gotPublic, gotHost = tenantID, secretKey, publicKey, host
			return &entity.TenantLangfuse{TenantID: tenantID, SecretKey: secretKey, PublicKey: publicKey, Host: host}, common.CodeSuccess, nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	body := `{"secret_key":"sk","public_key":"pk","host":"https://a.langfuse.com"}`
	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", body, h.SetAPIKey)

	if gotTenant != "tenant-1" || gotSecret != "sk" || gotPublic != "pk" || gotHost != "https://a.langfuse.com" {
		t.Fatalf("service args: tenant=%q secret=%q public=%q host=%q", gotTenant, gotSecret, gotPublic, gotHost)
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %v", payload["data"])
	}
	if data["secret_key"] != "sk" || data["public_key"] != "pk" || data["host"] != "https://a.langfuse.com" || data["tenant_id"] != "tenant-1" {
		t.Fatalf("unexpected data: %v", data)
	}
}

func TestLangfuseHandler_SetAPIKey_ServiceError(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("Invalid Langfuse keys")
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	body := `{"secret_key":"sk","public_key":"pk","host":"host"}`
	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", body, h.SetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Invalid Langfuse keys" {
		t.Fatalf("message=%v", payload["message"])
	}
}

func TestLangfuseHandler_SetAPIKey_BindFailureStopsEarly(t *testing.T) {
	called := false
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			called = true
			return nil, common.CodeSuccess, nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", `{not-json`, h.SetAPIKey)

	if called {
		t.Fatal("service should not be called when binding fails")
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestLangfuseHandler_GetAPIKey_Success(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return &entity.LangfuseInfoResponse{
				TenantID: tenantID, Host: "host", SecretKey: "sk", PublicKey: "pk",
				ProjectID: "proj-1", ProjectName: "My Project",
			}, common.CodeSuccess, "success", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) || payload["message"] != "success" {
		t.Fatalf("payload=%v", payload)
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %v", payload["data"])
	}
	if data["project_id"] != "proj-1" || data["project_name"] != "My Project" {
		t.Fatalf("unexpected data: %v", data)
	}
}

func TestLangfuseHandler_GetAPIKey_NoRecord(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return nil, common.CodeSuccess, "Have not record any Langfuse keys.", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Have not record any Langfuse keys." {
		t.Fatalf("message=%v", payload["message"])
	}
	if payload["data"] != nil {
		t.Fatalf("expected nil data, got %v", payload["data"])
	}
}

func TestLangfuseHandler_GetAPIKey_Unauthorized(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return nil, common.CodeDataError, "Invalid Langfuse keys loaded", errors.New("unauthorized")
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Invalid Langfuse keys loaded" {
		t.Fatalf("message=%v", payload["message"])
	}
}

func TestLangfuseHandler_DeleteAPIKey_Success(t *testing.T) {
	var gotTenant string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		deleteFn: func(ctx context.Context, tenantID string) (bool, common.ErrorCode, string, error) {
			gotTenant = tenantID
			return true, common.CodeSuccess, "", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodDelete, "/api/v1/langfuse/api-key", "", h.DeleteAPIKey)

	if gotTenant != "tenant-1" {
		t.Fatalf("tenant=%q", gotTenant)
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["data"] != true {
		t.Fatalf("expected data true, got %v", payload["data"])
	}
}

func TestLangfuseHandler_DeleteAPIKey_NoRecord(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		deleteFn: func(ctx context.Context, tenantID string) (bool, common.ErrorCode, string, error) {
			return false, common.CodeSuccess, "Have not record any Langfuse keys.", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{}}

	resp := serveLangfuse(http.MethodDelete, "/api/v1/langfuse/api-key", "", h.DeleteAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Have not record any Langfuse keys." {
		t.Fatalf("message=%v", payload["message"])
	}
	if payload["data"] != nil {
		t.Fatalf("expected nil data, got %v", payload["data"])
	}
}

// TestLangfuseHandler_TenantResolution_ResolvesUserToTenant pins the
// bug-fix contract for #20553: the handler must translate user.ID into
// tenants[0].TenantID before calling the service, and the service must
// receive that tenant id rather than the user id. The bug prior to this
// commit scoped every Langfuse row to the configuring user, breaking
// tenant-level sharing even though entity/DAO column-name said otherwise.
func TestLangfuseHandler_TenantResolution_ResolvesUserToTenant(t *testing.T) {
	const userID = "user-A-uuid"
	const tenantID = "tenant-real"

	var gotTenant string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(ctx context.Context, got string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			gotTenant = got
			return &entity.LangfuseInfoResponse{
				TenantID: got, Host: "host", SecretKey: "sk", PublicKey: "pk",
				ProjectID: "proj-1", ProjectName: "My Project",
			}, common.CodeSuccess, "success", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{
		findFn: func(_ context.Context, u, role string) ([]entity.UserTenant, error) {
			if u != userID || role != "owner" {
				t.Fatalf("finder lookup u=%q role=%q, want %q / %q", u, role, userID, "owner")
			}
			return []entity.UserTenant{{UserID: u, TenantID: tenantID, Role: role}}, nil
		},
	}}

	resp := serveLangfuseAs(http.MethodGet, "/api/v1/langfuse/api-key", "", userID, h.GetAPIKey)

	if gotTenant != tenantID {
		t.Fatalf("service received tenant=%q, want %q (user.ID was %q)", gotTenant, tenantID, userID)
	}
	if payload := decode(t, resp); payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
}

// TestLangfuseHandler_TenantResolution_NoOwnerReturns400 locks the
// "user has no owner tenant" branch: the handler must short-circuit with
// a 400 "Tenant not found" rather than calling the service with a
// bogus tenant id. The bug prior to this commit would have called the
// service with the user id and silently scoped the row to the user.
func TestLangfuseHandler_TenantResolution_NoOwnerReturns400(t *testing.T) {
	var serviceCalled bool
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(ctx context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			serviceCalled = true
			return nil, common.CodeServerError, "", nil
		},
	}, userTenantDAO: stubTenantFinder(nil, nil)}

	resp := serveLangfuseAs(http.MethodGet, "/api/v1/langfuse/api-key", "", "user-without-tenant", h.GetAPIKey)

	if serviceCalled {
		t.Fatal("service should not be called when user has no owner tenant")
	}
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.Code)
	}
	payload := decode(t, resp)
	if payload["message"] != "Tenant not found" {
		t.Fatalf("message=%v, want %q", payload["message"], "Tenant not found")
	}
}

// TestLangfuseHandler_TenantResolution_DAOErrorReturns400 locks the
// DB-error branch from the resolver. The handler must short-circuit with
// a 400 rather than calling the service. The bug prior to this commit
// would have silently masked the DAO error and called the service.
func TestLangfuseHandler_TenantResolution_DAOErrorReturns400(t *testing.T) {
	var serviceCalled bool
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(ctx context.Context, tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			serviceCalled = true
			return nil, common.CodeServerError, errors.New("unreachable")
		},
	}, userTenantDAO: stubTenantFinder(nil, errors.New("user_tenant read failed"))}

	body := `{"secret_key":"sk","public_key":"pk","host":"https://a.langfuse.com"}`
	resp := serveLangfuseAs(http.MethodPost, "/api/v1/langfuse/api-key", body, "user-1", h.SetAPIKey)

	if serviceCalled {
		t.Fatal("service should not be called when the tenant resolver errors")
	}
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.Code)
	}
}

// TestLangfuseHandler_TenantResolution_MultiUserSameTenant pins the
// real-world scenario from #20553: user A and user B both belong to
// tenant T. A configures Langfuse keys, B fetches them. With the fix,
// both see the same row. With the bug prior to the fix, each user would
// have stored a row under their own user id and the GET would have
// returned "Have not record any Langfuse keys" for the second user.
func TestLangfuseHandler_TenantResolution_MultiUserSameTenant(t *testing.T) {
	const tenantID = "tenant-shared"

	lookupFn := func(_ context.Context, u, role string) ([]entity.UserTenant, error) {
		return []entity.UserTenant{{UserID: u, TenantID: tenantID, Role: role}}, nil
	}

	// User A configures the keys.
	var seenTenantByA string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(_ context.Context, tenantID, _, _, _ string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			seenTenantByA = tenantID
			return &entity.TenantLangfuse{TenantID: tenantID}, common.CodeSuccess, nil
		},
	}, userTenantDAO: fakeUserTenantFinder{findFn: lookupFn}}

	resp := serveLangfuseAs(http.MethodPost, "/api/v1/langfuse/api-key",
		`{"secret_key":"sk","public_key":"pk","host":"https://a.langfuse.com"}`,
		"user-A", h.SetAPIKey)
	if resp.Code != http.StatusOK {
		t.Fatalf("A: status=%d, want 200", resp.Code)
	}
	if seenTenantByA != tenantID {
		t.Fatalf("A: service received tenant=%q, want %q", seenTenantByA, tenantID)
	}

	// User B fetches the keys.
	var seenTenantByB string
	h2 := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(_ context.Context, tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			seenTenantByB = tenantID
			return &entity.LangfuseInfoResponse{
				TenantID: tenantID, Host: "host", SecretKey: "sk", PublicKey: "pk",
				ProjectID: "proj-1", ProjectName: "Shared",
			}, common.CodeSuccess, "success", nil
		},
	}, userTenantDAO: fakeUserTenantFinder{findFn: lookupFn}}

	resp = serveLangfuseAs(http.MethodGet, "/api/v1/langfuse/api-key", "", "user-B", h2.GetAPIKey)
	if resp.Code != http.StatusOK {
		t.Fatalf("B: status=%d, want 200", resp.Code)
	}
	if seenTenantByB != tenantID {
		t.Fatalf("B: service received tenant=%q, want %q", seenTenantByB, tenantID)
	}
	if seenTenantByA != seenTenantByB {
		t.Fatalf("A and B should resolve to the same tenant, got %q vs %q", seenTenantByA, seenTenantByB)
	}
}
