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

package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/handler"
	"ragflow/internal/openapi"

	"github.com/gin-gonic/gin"
)

func TestRouteGuard_BaselineBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler()}
	r.Setup(engine)

	currentCount := CountUntypedRoutes(engine)
	if currentCount > MaxAllowedUntypedRoutes {
		t.Fatalf("untyped routes %d exceeds fixed ceiling %d; new routes must use RegisterTypedRoute", currentCount, MaxAllowedUntypedRoutes)
	}

	// 1. Verify baseline passes the budget check against the fixed ceiling
	if err := ValidateRouteBudget(engine, MaxAllowedUntypedRoutes); err != nil {
		t.Fatalf("expected baseline to pass budget: %v", err)
	}

	// 2. Verify adding a raw untyped route triggers interceptor and returns guidance,
	// even if a higher budget number is passed.
	engine.POST("/api/v1/unapproved_raw_route", func(c *gin.Context) {
		c.String(http.StatusOK, "unapproved")
	})

	err := ValidateRouteBudget(engine, 9999)
	if err == nil {
		t.Fatal("expected ValidateRouteBudget to intercept and return error when raw route is added outside baseline")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "API Specification Guard: Found 1 unapproved new untyped route(s)") {
		t.Errorf("error missing guard header: %s", errMsg)
	}
	if !strings.Contains(errMsg, "RegisterTypedRoute") {
		t.Errorf("error missing guidance pointing to RegisterTypedRoute: %s", errMsg)
	}
}

type sampleItemRequest struct {
	Name string `json:"name" doc:"Name of the sample item"`
}

func (r *sampleItemRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name cannot be empty")
	}
	return nil
}

type sampleItemResponse struct {
	ID   string `json:"id" doc:"Generated item identifier"`
	Name string `json:"name" doc:"Echoed item name"`
}

func sampleItemHandler(ctx context.Context, req *sampleItemRequest) (*sampleItemResponse, error) {
	return &sampleItemResponse{
		ID:   "item-12345",
		Name: req.Name,
	}, nil
}

func TestRouteGuard_TypedRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler()}
	r.Setup(engine)

	baselineCount := CountUntypedRoutes(engine)

	// Register conforming typed endpoint
	RegisterTypedRoute(r, engine, http.MethodPost, "/api/v1/typed-sample", "Sample typed endpoint", sampleItemHandler)

	// Verify budget is NOT exceeded because typed route was registered conforming to spec
	if err := ValidateRouteBudget(engine, baselineCount); err != nil {
		t.Fatalf("typed route should not count against untyped budget: %v", err)
	}

	// Test 1: Successful invocation with validation passing
	validBody, _ := json.Marshal(sampleItemRequest{Name: "valid_item"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/typed-sample", bytes.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/typed-sample status=%d; want 200, body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":0`) || !strings.Contains(resp.Body.String(), "valid_item") {
		t.Errorf("unexpected response body: %s", resp.Body.String())
	}

	// Test 2: Validation failure intercepted with 400
	invalidBody, _ := json.Marshal(sampleItemRequest{Name: ""}) // empty name fails Validate()
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/typed-sample", bytes.NewReader(invalidBody))
	badReq.Header.Set("Content-Type", "application/json")
	badResp := httptest.NewRecorder()
	engine.ServeHTTP(badResp, badReq)

	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got status=%d", badResp.Code)
	}
	if !strings.Contains(badResp.Body.String(), "cannot be empty") {
		t.Errorf("expected validation error message in body: %s", badResp.Body.String())
	}

	// Test 3: OpenAPI spec includes typed schema with standard envelope
	spec := r.openapiHandler.Registry().GetSpec()
	pathItem := spec.Paths["/api/v1/typed-sample"]
	if pathItem == nil || pathItem.Post == nil {
		t.Fatal("expected /api/v1/typed-sample POST operation in OpenAPI spec")
	}
	if pathItem.Post.RequestBody == nil {
		t.Fatal("expected RequestBody in OpenAPI spec for typed route")
	}
	resp200 := pathItem.Post.Responses["200"]
	if resp200 == nil || resp200.Content["application/json"] == nil {
		t.Fatal("expected 200 JSON response in OpenAPI spec")
	}
	schema200 := resp200.Content["application/json"].Schema
	if schema200.Properties["code"] == nil || schema200.Properties["data"] == nil {
		t.Errorf("expected standard envelope (code, message, data) in OpenAPI 200 response, got %#v", schema200.Properties)
	}
}

type testPathParamReq struct {
	ID    string `uri:"id" json:"id"`
	Limit int    `form:"limit"`
}

type testPathParamResp struct {
	CapturedID    string `json:"captured_id"`
	ContextID     string `json:"context_id"`
	CapturedUser  string `json:"captured_user"`
	CapturedLimit int    `json:"captured_limit"`
}

func TestRouteGuard_GroupAuthAndPathParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	// 1. Create an authenticated router group
	protected := engine.Group("/api/v1/protected")
	protected.Use(func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token != "valid-token" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("user_id", "user-uuid-999")
		c.Next()
	})

	// 2. Register typed route on the router group with path parameter :id and explicit auth requirement
	RegisterTypedRoute(
		r,
		protected,
		http.MethodPost,
		"/items/:id",
		"Get protected item",
		func(ctx context.Context, req *testPathParamReq) (*testPathParamResp, error) {
			return &testPathParamResp{
				CapturedID:    req.ID,
				ContextID:     RouteParam(ctx, "id"),
				CapturedUser:  UserID(ctx),
				CapturedLimit: req.Limit,
			}, nil
		},
		WithAuth(true),
	)

	// Test A: Unauthenticated request must be rejected by group's auth middleware
	reqNoAuth := httptest.NewRequest(http.MethodPost, "/api/v1/protected/items/dataset-42?limit=10", strings.NewReader(`{}`))
	reqNoAuth.Header.Set("Content-Type", "application/json")
	respNoAuth := httptest.NewRecorder()
	engine.ServeHTTP(respNoAuth, reqNoAuth)
	if respNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without token, got status=%d", respNoAuth.Code)
	}

	// Test B: Authenticated request with path param and query param
	reqAuth := httptest.NewRequest(http.MethodPost, "/api/v1/protected/items/dataset-42?limit=10", strings.NewReader(`{}`))
	reqAuth.Header.Set("Authorization", "valid-token")
	reqAuth.Header.Set("Content-Type", "application/json")
	respAuth := httptest.NewRecorder()
	engine.ServeHTTP(respAuth, reqAuth)

	if respAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", respAuth.Code, respAuth.Body.String())
	}
	bodyAuth := respAuth.Body.String()
	if !strings.Contains(bodyAuth, `"captured_id":"dataset-42"`) {
		t.Errorf("path parameter was not bound to struct: %s", bodyAuth)
	}
	if !strings.Contains(bodyAuth, `"context_id":"dataset-42"`) {
		t.Errorf("path parameter was not available in context: %s", bodyAuth)
	}
	if !strings.Contains(bodyAuth, `"captured_user":"user-uuid-999"`) {
		t.Errorf("user ID was not available in context: %s", bodyAuth)
	}
	if !strings.Contains(bodyAuth, `"captured_limit":10`) {
		t.Errorf("query parameter was not bound to struct: %s", bodyAuth)
	}

	// Test C: Malformed query parameter returns 400 Bad Request
	reqBadQuery := httptest.NewRequest(http.MethodPost, "/api/v1/protected/items/dataset-42?limit=not_a_number", strings.NewReader(`{}`))
	reqBadQuery.Header.Set("Authorization", "valid-token")
	reqBadQuery.Header.Set("Content-Type", "application/json")
	respBadQuery := httptest.NewRecorder()
	engine.ServeHTTP(respBadQuery, reqBadQuery)
	if respBadQuery.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for malformed query, got status=%d", respBadQuery.Code)
	}
	if !strings.Contains(respBadQuery.Body.String(), "not_a_number") && !strings.Contains(respBadQuery.Body.String(), "error") {
		t.Errorf("expected malformed query parameter to be rejected, got: %s", respBadQuery.Body.String())
	}

	// Test D: Chunked transfer request (ContentLength == -1) properly binds JSON body
	chunkedReq := httptest.NewRequest(http.MethodPost, "/api/v1/protected/items/dataset-42", strings.NewReader(`{}`))
	chunkedReq.Header.Set("Authorization", "valid-token")
	chunkedReq.Header.Set("Content-Type", "application/json")
	chunkedReq.ContentLength = -1 // simulate Transfer-Encoding: chunked
	respChunked := httptest.NewRecorder()
	engine.ServeHTTP(respChunked, chunkedReq)
	if respChunked.Code != http.StatusOK {
		t.Fatalf("expected chunked request to succeed, got status=%d, body=%s", respChunked.Code, respChunked.Body.String())
	}
}

type testIDMismatchReq struct {
	ID   string `uri:"id" json:"id"`
	Name string `json:"name" binding:"required"`
}

type testIDMismatchResp struct {
	ConfirmedID string `json:"confirmed_id"`
	Name        string `json:"name"`
}

func TestRouteGuard_PathParameterCannotBeOverwrittenByJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/api/v1/datasets/:id",
		"Update dataset by ID",
		func(ctx context.Context, req *testIDMismatchReq) (*testIDMismatchResp, error) {
			return &testIDMismatchResp{ConfirmedID: req.ID, Name: req.Name}, nil
		},
	)

	// Scenario 1: Malicious or buggy JSON body attempts to override path parameter
	tamperedBody := `{"id": "tampered-dataset-999", "name": "my-dataset"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/dataset-original-123", strings.NewReader(tamperedBody))
	req1.Header.Set("Content-Type", "application/json")
	resp1 := httptest.NewRecorder()
	engine.ServeHTTP(resp1, req1)

	if !strings.Contains(resp1.Body.String(), "route parameter mismatch") {
		t.Errorf("expected route parameter mismatch rejection, got response: %s", resp1.Body.String())
	}

	// Scenario 2: Normal body without ID field uses path parameter
	normalBody := `{"name": "my-dataset"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/dataset-original-123", strings.NewReader(normalBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)

	if resp2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", resp2.Code, resp2.Body.String())
	}
	if !strings.Contains(resp2.Body.String(), `"confirmed_id":"dataset-original-123"`) {
		t.Errorf("expected path parameter to be preserved, got: %s", resp2.Body.String())
	}

	// Scenario 3: Body with identical matching ID succeeds
	matchingBody := `{"id": "dataset-original-123", "name": "my-dataset"}`
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/dataset-original-123", strings.NewReader(matchingBody))
	req3.Header.Set("Content-Type", "application/json")
	resp3 := httptest.NewRecorder()
	engine.ServeHTTP(resp3, req3)

	if resp3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", resp3.Code, resp3.Body.String())
	}
	if !strings.Contains(resp3.Body.String(), `"confirmed_id":"dataset-original-123"`) {
		t.Errorf("expected matching ID to succeed, got: %s", resp3.Body.String())
	}
}

type testMixedBindingReq struct {
	Sort  string `form:"sort" binding:"required"`
	Title string `json:"title" binding:"required"`
}

type testMixedBindingResp struct {
	CapturedSort  string `json:"sort"`
	CapturedTitle string `json:"title"`
}

func TestRouteGuard_MixedQueryAndJSONNoPrematureValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/api/v1/mixed-endpoint",
		"Mixed query and json endpoint",
		func(ctx context.Context, req *testMixedBindingReq) (*testMixedBindingResp, error) {
			return &testMixedBindingResp{CapturedSort: req.Sort, CapturedTitle: req.Title}, nil
		},
	)

	// Scenario 1: Both query param and JSON body required fields present -> must succeed!
	// (Previously failed prematurely because ShouldBindQuery validated before JSON body was bound)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/mixed-endpoint?sort=asc", strings.NewReader(`{"title": "Valid Title"}`))
	req1.Header.Set("Content-Type", "application/json")
	resp1 := httptest.NewRecorder()
	engine.ServeHTTP(resp1, req1)

	if resp1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", resp1.Code, resp1.Body.String())
	}
	if !strings.Contains(resp1.Body.String(), `"sort":"asc"`) || !strings.Contains(resp1.Body.String(), `"title":"Valid Title"`) {
		t.Errorf("expected bound query and body values in response: %s", resp1.Body.String())
	}

	// Scenario 2: Missing required query parameter -> must fail validation
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/mixed-endpoint", strings.NewReader(`{"title": "Valid Title"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)

	if !strings.Contains(resp2.Body.String(), "Sort") && !strings.Contains(resp2.Body.String(), "error") {
		t.Errorf("expected validation failure for missing query param, got: %s", resp2.Body.String())
	}

	// Scenario 3: Missing required JSON body field -> must fail validation
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/mixed-endpoint?sort=asc", strings.NewReader(`{}`))
	req3.Header.Set("Content-Type", "application/json")
	resp3 := httptest.NewRecorder()
	engine.ServeHTTP(resp3, req3)

	if !strings.Contains(resp3.Body.String(), "Title") && !strings.Contains(resp3.Body.String(), "error") {
		t.Errorf("expected validation failure for missing json body field, got: %s", resp3.Body.String())
	}
}

type testQueryDocReq struct {
	Page   int    `form:"page" doc:"Page number" binding:"required"`
	Filter string `json:"filter" doc:"Keyword search filter"`
}

type testQueryDocResp struct {
	Items []string `json:"items"`
}

func TestRouteGuard_OpenAPIQueryParametersAndSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler: handler.NewAuthHandler(),
	}
	// Note: openapiHandler is nil initially to test lazy initialization and Setup initialization
	r.Setup(engine)

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/api/v1/query-spec",
		"Query parameter spec test",
		func(ctx context.Context, req *testQueryDocReq) (*testQueryDocResp, error) {
			return &testQueryDocResp{Items: []string{}}, nil
		},
		WithAuth(true),
	)

	spec := r.openapiHandler.Registry().GetSpec()
	pathItem := spec.Paths["/api/v1/query-spec"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected /api/v1/query-spec GET operation in OpenAPI spec")
	}

	op := pathItem.Get
	var hasPage, hasFilter bool
	for _, p := range op.Parameters {
		if p.In == "query" && p.Name == "page" {
			hasPage = true
			if !p.Required {
				t.Errorf("expected page query parameter to be required")
			}
			if p.Description != "Page number" {
				t.Errorf("expected page query parameter description 'Page number', got %q", p.Description)
			}
		}
		if p.In == "query" && p.Name == "filter" {
			hasFilter = true
			if p.Description != "Keyword search filter" {
				t.Errorf("expected filter query parameter description, got %q", p.Description)
			}
		}
	}

	if !hasPage {
		t.Errorf("expected query parameter 'page' in OpenAPI parameters, got: %v", op.Parameters)
	}
	if !hasFilter {
		t.Errorf("expected query parameter 'filter' in OpenAPI parameters, got: %v", op.Parameters)
	}

	// Verify operation requires ApiKeyAuth
	if len(op.Security) == 0 || op.Security[0]["ApiKeyAuth"] == nil {
		t.Errorf("expected ApiKeyAuth security requirement on operation, got: %v", op.Security)
	}
}

type testPointerPathReq struct {
	ID   *string `uri:"id" json:"id"`
	Name string  `json:"name"`
}

type testPointerPathResp struct {
	CapturedID string `json:"captured_id"`
	Name       string `json:"name"`
}

func TestRouteGuard_PointerPathParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/api/v1/pointer-items/:id",
		"Pointer path parameter test",
		func(ctx context.Context, req *testPointerPathReq) (*testPointerPathResp, error) {
			idVal := ""
			if req.ID != nil {
				idVal = *req.ID
			}
			return &testPointerPathResp{CapturedID: idVal, Name: req.Name}, nil
		},
	)

	// Scenario 1: Normal request without ID body field -> sets pointer to path param
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/pointer-items/orig-id-123", strings.NewReader(`{"name": "test"}`))
	req1.Header.Set("Content-Type", "application/json")
	resp1 := httptest.NewRecorder()
	engine.ServeHTTP(resp1, req1)

	if resp1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", resp1.Code, resp1.Body.String())
	}
	if !strings.Contains(resp1.Body.String(), `"captured_id":"orig-id-123"`) {
		t.Errorf("expected path parameter pointer to be populated, got: %s", resp1.Body.String())
	}

	// Scenario 2: Body with matching ID pointer succeeds
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/pointer-items/orig-id-123", strings.NewReader(`{"id": "orig-id-123", "name": "test"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)

	if resp2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got status=%d, body=%s", resp2.Code, resp2.Body.String())
	}
	if !strings.Contains(resp2.Body.String(), `"captured_id":"orig-id-123"`) {
		t.Errorf("expected matching ID to succeed, got: %s", resp2.Body.String())
	}

	// Scenario 3: Body with conflicting ID pointer is rejected with 400 mismatch
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/pointer-items/orig-id-123", strings.NewReader(`{"id": "tampered-id", "name": "test"}`))
	req3.Header.Set("Content-Type", "application/json")
	resp3 := httptest.NewRecorder()
	engine.ServeHTTP(resp3, req3)

	if !strings.Contains(resp3.Body.String(), "route parameter mismatch") {
		t.Errorf("expected route parameter mismatch rejection for pointer field, got: %s", resp3.Body.String())
	}
}

func TestRouteGuard_MigrationStatsAndContractValidation(t *testing.T) {
	// Test 1: ValidateContract rejects empty summary or missing responses
	emptySummaryContract := openapi.Contract{
		Summary:   "",
		Responses: map[string]*openapi.Response{"200": {Description: "OK"}},
	}
	if err := ValidateContract(emptySummaryContract); err == nil {
		t.Errorf("expected ValidateContract to reject empty summary")
	}

	emptyRespContract := openapi.Contract{
		Summary:   "Has summary",
		Responses: map[string]*openapi.Response{},
	}
	if err := ValidateContract(emptyRespContract); err == nil {
		t.Errorf("expected ValidateContract to reject empty responses")
	}

	no2xxContract := openapi.Contract{
		Summary:   "Has summary",
		Responses: map[string]*openapi.Response{"400": {Description: "Bad Request"}},
	}
	if err := ValidateContract(no2xxContract); err == nil {
		t.Errorf("expected ValidateContract to reject responses without 2xx")
	}

	validContract := openapi.Contract{
		Summary:   "Valid",
		Responses: map[string]*openapi.Response{"200": {Description: "Success"}, "default": {Description: "Default"}},
	}
	if err := ValidateContract(validContract); err != nil {
		t.Errorf("expected valid contract to pass validation: %v", err)
	}

	invalidCodeContract := openapi.Contract{
		Summary:   "Invalid code",
		Responses: map[string]*openapi.Response{"200": {Description: "OK"}, "2bad": {Description: "Bad"}},
	}
	if err := ValidateContract(invalidCodeContract); err == nil {
		t.Errorf("expected ValidateContract to reject invalid status code '2bad'")
	}

	non3DigitContract := openapi.Contract{
		Summary:   "Non 3-digit code",
		Responses: map[string]*openapi.Response{"200": {Description: "OK"}, "0200": {Description: "Padded"}},
	}
	if err := ValidateContract(non3DigitContract); err == nil {
		t.Errorf("expected ValidateContract to reject non-3-digit status code '0200'")
	}

	// Test 2: Migration stats tracking
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	engine.GET("/api/v1/raw-untyped-route", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	r.RegisterContract(http.MethodGet, "/api/v1/test-contracted", validContract)
	engine.GET("/api/v1/test-contracted", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	stats := GetRouteMigrationStats(engine)
	if stats.ContractedRoutes < 1 {
		t.Errorf("expected at least 1 contracted route, got %d", stats.ContractedRoutes)
	}
	if stats.UntypedRoutes < 1 {
		t.Errorf("expected at least 1 untyped route, got %d", stats.UntypedRoutes)
	}
	var foundRaw bool
	for _, u := range stats.UntypedList {
		if strings.Contains(u, "/api/v1/raw-untyped-route") {
			foundRaw = true
			break
		}
	}
	if !foundRaw {
		t.Errorf("expected raw untyped route in stats.UntypedList: %v", stats.UntypedList)
	}
}

type testPublicReq struct {
	Query string `json:"query"`
}

type testPublicResp struct {
	Result string `json:"result"`
}

func TestRouteGuard_PublicTypedRouteOutputsEmptySecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}
	r.openapiHandler.RegisterRoutes(engine)

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/api/v1/public-endpoint",
		"Public endpoint with no auth",
		func(ctx context.Context, req *testPublicReq) (*testPublicResp, error) {
			return &testPublicResp{Result: "public"}, nil
		},
		WithAuth(false),
	)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json status=%d", resp.Code)
	}

	rawJSON := resp.Body.String()
	// Public operation must explicitly output "security": [] in OpenAPI JSON
	if !strings.Contains(rawJSON, `"security":[]`) && !strings.Contains(rawJSON, `"security": []`) {
		t.Errorf("expected public typed route to explicitly output 'security: []' in json: %s", rawJSON)
	}
}

type testUnifiedQueryReq struct {
	Page   int    `form:"page" doc:"Page number"`
	Filter string `json:"filter" doc:"Filter string with only json tag"`
	Sort   string `doc:"Sort field without form or json tag"`
}

type testUnifiedQueryResp struct {
	Page   int    `json:"page"`
	Filter string `json:"filter"`
	Sort   string `json:"sort"`
}

func TestRouteGuard_QueryParamNameUnification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/api/v1/unified-query",
		"Unified query parameter test",
		func(ctx context.Context, req *testUnifiedQueryReq) (*testUnifiedQueryResp, error) {
			return &testUnifiedQueryResp{
				Page:   req.Page,
				Filter: req.Filter,
				Sort:   req.Sort,
			}, nil
		},
	)

	// Verify OpenAPI spec parameter names
	spec := r.openapiHandler.Registry().GetSpec()
	pathItem := spec.Paths["/api/v1/unified-query"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatal("missing /api/v1/unified-query GET operation in spec")
	}

	names := make(map[string]bool)
	for _, p := range pathItem.Get.Parameters {
		if p.In == "query" {
			names[p.Name] = true
		}
	}
	for _, expectedName := range []string{"page", "filter", "sort"} {
		if !names[expectedName] {
			t.Errorf("expected query parameter %q in openapi documentation, got %v", expectedName, names)
		}
	}

	// Verify runtime binding with the documented query parameter names
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unified-query?page=5&filter=my_filter&sort=asc", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"page":5`) {
		t.Errorf("expected page:5 in response: %s", body)
	}
	if !strings.Contains(body, `"filter":"my_filter"`) {
		t.Errorf("expected filter:my_filter in response: %s", body)
	}
	if !strings.Contains(body, `"sort":"asc"`) {
		t.Errorf("expected sort:asc in response: %s", body)
	}
}

type testIntPathReq struct {
	ID    int    `uri:"id" json:"id"`
	Count *int   `uri:"count" json:"count"`
	Code  uint   `uri:"code" json:"code"`
	Name  string `json:"name"`
}

type testIntPathResp struct {
	ID    int    `json:"id"`
	Count int    `json:"count"`
	Code  uint   `json:"code"`
	Name  string `json:"name"`
}

func TestRouteGuard_IntegerAndPointerPathParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/api/v1/items/:id/counts/:count/codes/:code",
		"Int and pointer path parameters test",
		func(ctx context.Context, req *testIntPathReq) (*testIntPathResp, error) {
			countVal := 0
			if req.Count != nil {
				countVal = *req.Count
			}
			return &testIntPathResp{
				ID:    req.ID,
				Count: countVal,
				Code:  req.Code,
				Name:  req.Name,
			}, nil
		},
	)

	// Scenario A: Normal request binding int, *int, and uint path parameters
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/items/42/counts/100/codes/99", strings.NewReader(`{"name": "test_item"}`))
	reqA.Header.Set("Content-Type", "application/json")
	respA := httptest.NewRecorder()
	engine.ServeHTTP(respA, reqA)

	if respA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body=%s", respA.Code, respA.Body.String())
	}
	bodyA := respA.Body.String()
	if !strings.Contains(bodyA, `"id":42`) || !strings.Contains(bodyA, `"count":100`) || !strings.Contains(bodyA, `"code":99`) {
		t.Errorf("expected bound int/pointer/uint path parameters: %s", bodyA)
	}

	// Scenario B: Conflicting ID in body rejected with 400 mismatch
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/items/42/counts/100/codes/99", strings.NewReader(`{"id": 999, "name": "tampered"}`))
	reqB.Header.Set("Content-Type", "application/json")
	respB := httptest.NewRecorder()
	engine.ServeHTTP(respB, reqB)

	if respB.Code != http.StatusBadRequest || !strings.Contains(respB.Body.String(), "route parameter mismatch") {
		t.Errorf("expected 400 route parameter mismatch for ID, got %d, body=%s", respB.Code, respB.Body.String())
	}

	// Scenario C: Conflicting Count in body rejected with 400 mismatch
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/items/42/counts/100/codes/99", strings.NewReader(`{"count": 888, "name": "tampered"}`))
	reqC.Header.Set("Content-Type", "application/json")
	respC := httptest.NewRecorder()
	engine.ServeHTTP(respC, reqC)

	if respC.Code != http.StatusBadRequest || !strings.Contains(respC.Body.String(), "route parameter mismatch") {
		t.Errorf("expected 400 route parameter mismatch for Count pointer, got %d, body=%s", respC.Code, respC.Body.String())
	}

	// Scenario D: Conflicting Code in body rejected with 400 mismatch
	reqD := httptest.NewRequest(http.MethodPost, "/api/v1/items/42/counts/100/codes/99", strings.NewReader(`{"code": 777, "name": "tampered"}`))
	reqD.Header.Set("Content-Type", "application/json")
	respD := httptest.NewRecorder()
	engine.ServeHTTP(respD, reqD)

	if respD.Code != http.StatusBadRequest || !strings.Contains(respD.Body.String(), "route parameter mismatch") {
		t.Errorf("expected 400 route parameter mismatch for Code uint, got %d, body=%s", respD.Code, respD.Body.String())
	}

	// Scenario E: Matching values in body succeed
	reqE := httptest.NewRequest(http.MethodPost, "/api/v1/items/42/counts/100/codes/99", strings.NewReader(`{"id": 42, "count": 100, "code": 99, "name": "valid"}`))
	reqE.Header.Set("Content-Type", "application/json")
	respE := httptest.NewRecorder()
	engine.ServeHTTP(respE, reqE)

	if respE.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for matching values, got %d, body=%s", respE.Code, respE.Body.String())
	}

	// Scenario F: Non-numeric path parameter in URL rejected with 400
	reqF := httptest.NewRequest(http.MethodPost, "/api/v1/items/not_a_number/counts/100/codes/99", strings.NewReader(`{}`))
	reqF.Header.Set("Content-Type", "application/json")
	respF := httptest.NewRecorder()
	engine.ServeHTTP(respF, reqF)

	if respF.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for non-integer path parameter, got %d, body=%s", respF.Code, respF.Body.String())
	}
}

func TestRouteGuard_GroupAuthMiddlewareInheritance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}
	r.openapiHandler.RegisterRoutes(engine)

	authGroup := engine.Group("/api/v1")
	authGroup.Use(func(c *gin.Context) { c.Next() })

	type protectedReq struct {
		ID string `uri:"id"`
	}
	type protectedResp struct {
		Status string `json:"status"`
	}

	// Register on protected group without passing WithAuth
	RegisterTypedRoute(
		r,
		authGroup,
		http.MethodGet,
		"/protected/:id",
		"Protected without WithAuth",
		func(ctx context.Context, req *protectedReq) (*protectedResp, error) {
			return &protectedResp{Status: "ok"}, nil
		},
	)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	var doc openapi.Document
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	pathItem := doc.Paths["/api/v1/protected/{id}"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected /api/v1/protected/{id} GET operation in openapi")
	}

	if len(pathItem.Get.Security) != 1 || pathItem.Get.Security[0]["ApiKeyAuth"] == nil {
		t.Errorf("expected ApiKeyAuth security requirement on protected group route, got: %#v", pathItem.Get.Security)
	}

	resp401 := pathItem.Get.Responses["401"]
	if resp401 == nil {
		t.Fatalf("expected 401 response definition")
	}
	schema401 := resp401.Content["application/json"].Schema
	if schema401.Properties["code"] == nil || schema401.Properties["message"] == nil {
		t.Errorf("expected 401 response to have code and message properties, got: %#v", schema401.Properties)
	}
	if schema401.Properties["error"] != nil {
		t.Errorf("401 response should not have error property: %#v", schema401.Properties)
	}
}

type testNarrowIntReq struct {
	U8 uint8 `uri:"u8"`
	I8 int8  `uri:"i8"`
}
type testNarrowIntResp struct {
	U8 uint8 `json:"u8"`
	I8 int8  `json:"i8"`
}

func TestRouteGuard_NarrowIntegerPathParameterOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/narrow/:u8/:i8",
		"Narrow int test",
		func(ctx context.Context, req *testNarrowIntReq) (*testNarrowIntResp, error) {
			return &testNarrowIntResp{U8: req.U8, I8: req.I8}, nil
		},
		WithAuth(false),
	)

	// Case 1: uint8 = 256 (overflows uint8: 0..255)
	req1 := httptest.NewRequest(http.MethodGet, "/narrow/256/10", nil)
	resp1 := httptest.NewRecorder()
	engine.ServeHTTP(resp1, req1)
	if resp1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for uint8=256 overflow, got %d, body=%s", resp1.Code, resp1.Body.String())
	}

	// Case 2: int8 = 128 (overflows int8: -128..127)
	req2 := httptest.NewRequest(http.MethodGet, "/narrow/255/128", nil)
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for int8=128 overflow, got %d, body=%s", resp2.Code, resp2.Body.String())
	}

	// Case 3: valid uint8 = 255, int8 = 127
	req3 := httptest.NewRequest(http.MethodGet, "/narrow/255/127", nil)
	resp3 := httptest.NewRecorder()
	engine.ServeHTTP(resp3, req3)
	if resp3.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid bounds, got %d, body=%s", resp3.Code, resp3.Body.String())
	}
	var res struct {
		Code int                `json:"code"`
		Data *testNarrowIntResp `json:"data"`
	}
	json.Unmarshal(resp3.Body.Bytes(), &res)
	if res.Data == nil || res.Data.U8 != 255 || res.Data.I8 != 127 {
		t.Errorf("expected u8=255, i8=127, got %#v", res.Data)
	}
}

type testFormOptionReq struct {
	Page int `form:"page,default=1"`
}
type testFormOptionResp struct {
	Page int `json:"page"`
}

func TestRouteGuard_FormTagWithOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/form-options",
		"Form option test",
		func(ctx context.Context, req *testFormOptionReq) (*testFormOptionResp, error) {
			return &testFormOptionResp{Page: req.Page}, nil
		},
		WithAuth(false),
	)

	// Call with ?page=2
	req := httptest.NewRequest(http.MethodGet, "/form-options?page=2", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", resp.Code, resp.Body.String())
	}
	var res struct {
		Code int                 `json:"code"`
		Data *testFormOptionResp `json:"data"`
	}
	json.Unmarshal(resp.Body.Bytes(), &res)
	if res.Data == nil || res.Data.Page != 2 {
		t.Errorf("expected page=2 with form tag option, got %#v", res.Data)
	}
}

type testRecursiveTreeNode struct {
	Name     string                   `json:"name"`
	Children []*testRecursiveTreeNode `json:"children"`
}

func TestRouteGuard_RecursiveStructRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}
	r.openapiHandler.RegisterRoutes(engine)

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/tree-nodes",
		"Tree node recursive struct",
		func(ctx context.Context, req *testRecursiveTreeNode) (*testRecursiveTreeNode, error) {
			return req, nil
		},
		WithAuth(false),
	)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var doc openapi.Document
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	pathItem := doc.Paths["/tree-nodes"]
	if pathItem == nil || pathItem.Post == nil {
		t.Fatalf("expected /tree-nodes POST operation in openapi")
	}
}

type testPollutionReq struct {
	Name string `json:"name"`
	URL  string `query:"url"`
}

type testPollutionResp struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func TestRouteGuard_QueryParamPollutionProtection(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/pollute-test",
		"Test query param pollution",
		func(ctx context.Context, req *testPollutionReq) (*testPollutionResp, error) {
			return &testPollutionResp{Name: req.Name, URL: req.URL}, nil
		},
		WithAuth(false),
	)

	// Injected ?name=injected must NOT pollute req.Name, but ?url=val MUST bind to req.URL
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/pollute-test?name=injected&url=https://safe.example.com", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.Code, resp.Body.String())
	}
	var res struct {
		Data testPollutionResp `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res.Data.Name == "injected" {
		t.Fatalf("VULNERABILITY DETECTED: req.Name was polluted by URL query string: %s", res.Data.Name)
	}
	if res.Data.URL != "https://safe.example.com" {
		t.Fatalf("expected URL query parameter to bind: %s", res.Data.URL)
	}
}

type testNonStringQueryReq struct {
	Page   int      `query:"page"`
	Active *bool    `query:"active"`
	Score  float64  `query:"score"`
	Count  uint32   `query:"count"`
}

type testNonStringQueryResp struct {
	Page   int     `json:"page"`
	Active bool    `json:"active"`
	Score  float64 `json:"score"`
	Count  uint32  `json:"count"`
}

func TestRouteGuard_NonStringQueryParameterBinding(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/query-types",
		"Test query types",
		func(ctx context.Context, req *testNonStringQueryReq) (*testNonStringQueryResp, error) {
			act := false
			if req.Active != nil {
				act = *req.Active
			}
			return &testNonStringQueryResp{
				Page:   req.Page,
				Active: act,
				Score:  req.Score,
				Count:  req.Count,
			}, nil
		},
		WithAuth(false),
	)

	req := httptest.NewRequest(http.MethodGet, "/query-types?page=42&active=true&score=98.5&count=100", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.Code, resp.Body.String())
	}
	var res struct {
		Data testNonStringQueryResp `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res.Data.Page != 42 || !res.Data.Active || res.Data.Score != 98.5 || res.Data.Count != 100 {
		t.Fatalf("query types binding failed: got %#v", res.Data)
	}
}

type TestEmbeddedPointerCommon struct {
	ItemID string `uri:"item_id"`
}

type TestEmbeddedPointerReq struct {
	*TestEmbeddedPointerCommon
	Extra string `json:"extra"`
}

func TestRouteGuard_AnonymousPointerStructPathParam(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/items/:item_id/info",
		"Embedded pointer param test",
		func(ctx context.Context, req *TestEmbeddedPointerReq) (map[string]string, error) {
			id := ""
			if req.TestEmbeddedPointerCommon != nil {
				id = req.ItemID
			}
			return map[string]string{"id": id}, nil
		},
		WithAuth(false),
	)

	req := httptest.NewRequest(http.MethodGet, "/items/xyz-789/info", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var res struct {
		Data map[string]string `json:"data"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &res)
	if res.Data["id"] != "xyz-789" {
		t.Fatalf("expected id xyz-789 from embedded pointer struct, got %s", res.Data["id"])
	}
}

type testQueryPointerReq struct {
	Count  int   `query:"count"`
	Active *bool `query:"active"`
}

func TestRouteGuard_QueryParamMalformedErrorAndPointerIntegrity(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/test/pointer-query",
		"Pointer query test",
		func(ctx context.Context, req *testQueryPointerReq) (*testQueryPointerReq, error) {
			return req, nil
		},
		WithAuth(false),
	)

	// 1. Valid request with omitted pointer: Active should be nil
	{
		req := httptest.NewRequest(http.MethodGet, "/test/pointer-query?count=10", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var res struct {
			Data testQueryPointerReq `json:"data"`
		}
		_ = json.Unmarshal(resp.Body.Bytes(), &res)
		if res.Data.Count != 10 || res.Data.Active != nil {
			t.Fatalf("expected count=10 and active=nil, got count=%d active=%v", res.Data.Count, res.Data.Active)
		}
	}

	// 2. Malformed integer: should return 400 Bad Request
	{
		req := httptest.NewRequest(http.MethodGet, "/test/pointer-query?count=notanint", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for malformed int, got %d: %s", resp.Code, resp.Body.String())
		}
	}

	// 3. Malformed bool pointer: should return 400 Bad Request and not corrupt pointer
	{
		req := httptest.NewRequest(http.MethodGet, "/test/pointer-query?active=invalidbool", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for malformed bool, got %d: %s", resp.Code, resp.Body.String())
		}
	}

	// 4. Valid bool pointer
	{
		req := httptest.NewRequest(http.MethodGet, "/test/pointer-query?active=true", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var res struct {
			Data testQueryPointerReq `json:"data"`
		}
		_ = json.Unmarshal(resp.Body.Bytes(), &res)
		if res.Data.Active == nil || !*res.Data.Active {
			t.Fatalf("expected active=true, got %v", res.Data.Active)
		}
	}
}

type TestPaginationQuery struct {
	Page     int `query:"page" doc:"Page index"`
	PageSize int `query:"page_size" doc:"Page size"`
}

type TestEmbeddedQueryReq struct {
	TestPaginationQuery
	Filter string `query:"filter" doc:"Search filter"`
}

func TestRouteGuard_EmbeddedStructQueryParamsAndDoc(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	reg := openapi.NewRegistry()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(reg),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodGet,
		"/test/embedded-query",
		"Embedded query test",
		func(ctx context.Context, req *TestEmbeddedQueryReq) (*TestEmbeddedQueryReq, error) {
			return req, nil
		},
		WithAuth(false),
	)

	// Verify OpenAPI documented parameters includes page, page_size, and filter
	spec := reg.GetSpec()
	pathItem := spec.Paths["/test/embedded-query"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected /test/embedded-query path in OpenAPI spec")
	}
	paramNames := make(map[string]bool)
	for _, p := range pathItem.Get.Parameters {
		paramNames[p.Name] = true
	}
	if !paramNames["page"] || !paramNames["page_size"] || !paramNames["filter"] {
		t.Fatalf("missing parameters in OpenAPI doc: got %v", paramNames)
	}

	// Verify runtime binding
	req := httptest.NewRequest(http.MethodGet, "/test/embedded-query?page=3&page_size=25&filter=active", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var res struct {
		Data TestEmbeddedQueryReq `json:"data"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &res)
	if res.Data.Page != 3 || res.Data.PageSize != 25 || res.Data.Filter != "active" {
		t.Fatalf("expected page=3 page_size=25 filter=active, got %#v", res.Data)
	}
}

func TestRouteGuard_NilBodyWithMaxBytesReader(t *testing.T) {
	t.Cleanup(ClearContractedRoutes)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/test/nil-body",
		"Nil body test",
		func(ctx context.Context, req *struct{}) (map[string]string, error) {
			return map[string]string{"status": "ok"}, nil
		},
		WithAuth(false),
		WithMaxBodyBytes(1024),
	)

	req := httptest.NewRequest(http.MethodPost, "/test/nil-body", nil)
	req.Body = nil // explicitly nil
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
}
