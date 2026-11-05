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
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/handler"
	"ragflow/internal/openapi"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/service/document"
)

// fakeDocumentServiceForMigration satisfies handler.DocumentServiceIface via embedding.
type fakeDocumentServiceForMigration struct {
	handler.DocumentServiceIface
	doc *document.DocumentResponse
	err error
}

func (f *fakeDocumentServiceForMigration) GetDocumentByID(ctx context.Context, id string) (*document.DocumentResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.doc != nil && f.doc.ID == id {
		return f.doc, nil
	}
	return nil, errors.New("record not found")
}

// fakeCanvasLoaderForMigration satisfies handler.CanvasLoader via embedding.
type fakeCanvasLoaderForMigration struct {
	handler.CanvasLoader
	canvas *entity.UserCanvas
	err    error
}

func (f *fakeCanvasLoaderForMigration) LoadCanvasByID(ctx context.Context, id string) (*entity.UserCanvas, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.canvas != nil && f.canvas.ID == id {
		return f.canvas, nil
	}
	return nil, dao.ErrUserCanvasNotFound
}

// fakeAgentFileServiceForMigration satisfies handler.AgentFileService via embedding.
type fakeAgentFileServiceForMigration struct {
	handler.AgentFileService
	uploadResult []map[string]interface{}
	urlUpload    map[string]interface{}
	err          error
}

func (f *fakeAgentFileServiceForMigration) UploadInfos(ctx context.Context, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.uploadResult) > 0 {
		return f.uploadResult, nil
	}
	var res []map[string]interface{}
	for _, file := range files {
		res = append(res, map[string]interface{}{
			"id":   "file-" + file.Filename,
			"name": file.Filename,
			"size": file.Size,
		})
	}
	return res, nil
}

func (f *fakeAgentFileServiceForMigration) UploadFromURL(ctx context.Context, rawURL string) (map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.urlUpload != nil {
		return f.urlUpload, nil
	}
	return map[string]interface{}{"mode": "url", "url": rawURL}, nil
}

// TestMigratedRoute_GetDocumentByID verifies the real DocumentHandler.GetDocumentByID implementation
// mounted as a typed route, including auth middleware inheritance, path param extraction, and OpenAPI schema.
func TestMigratedRoute_GetDocumentByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.User{},
		&entity.Knowledgebase{},
		&entity.UserTenant{},
		&entity.Document{},
	); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}
	validStatus := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID:         "kb-001",
		TenantID:   "tenant-1",
		Name:       "test-kb",
		CreatedBy:  "user-1",
		Permission: string(entity.TenantPermissionTeam),
		Status:     &validStatus,
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID:       "ut-1",
		UserID:   "user-1",
		TenantID: "tenant-1",
		Role:     "owner",
		Status:   &validStatus,
	}).Error; err != nil {
		t.Fatalf("insert user_tenant: %v", err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "tenant-1", "user-1"); err != nil {
		t.Fatalf("prepare fixture: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	docName := "sample.pdf"
	fakeDocSvc := &fakeDocumentServiceForMigration{
		doc: &document.DocumentResponse{
			ID:   "doc-123",
			Name: &docName,
			KbID: "kb-001",
		},
	}
	realDocHandler := handler.NewDocumentHandlerForTest(fakeDocSvc)

	r := &Router{
		authHandler:     handler.NewAuthHandler(),
		documentHandler: realDocHandler,
		openapiHandler:  openapi.NewHandler(openapi.NewRegistry()),
	}

	authGroup := engine.Group("/api/v1")
	authGroup.Use(func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" || !strings.HasPrefix(token, "Bearer ") {
			common.RespondError(c, common.CodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "user-1", "tenant-1"))
		c.Next()
	})
	docsGroup := authGroup.Group("/documents")

	RegisterTypedRoute(
		r,
		docsGroup,
		http.MethodGet,
		"/:id",
		"Get document by ID",
		realDocHandler.GetDocumentByID,
	)

	// 1. Unauthenticated request must be rejected by auth middleware with 109 / 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/documents/doc-123", nil)
	unauthResp := httptest.NewRecorder()
	engine.ServeHTTP(unauthResp, unauthReq)

	var unauthBody struct {
		Code common.ErrorCode `json:"code"`
	}
	_ = json.Unmarshal(unauthResp.Body.Bytes(), &unauthBody)
	if unauthBody.Code != common.CodeUnauthorized {
		t.Fatalf("expected CodeUnauthorized (109), got code=%d body=%s", unauthBody.Code, unauthResp.Body.String())
	}

	// 2. Authenticated request with valid document ID retrieves document
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/doc-123", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.Code, resp.Body.String())
	}
	var res struct {
		Code common.ErrorCode           `json:"code"`
		Data *document.DocumentResponse `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res.Code != common.CodeSuccess || res.Data == nil || res.Data.ID != "doc-123" {
		t.Errorf("unexpected response content: %#v", res)
	}

	// 3. Document not found returns CodeNotFound
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/documents/unknown-doc", nil)
	reqNotFound.Header.Set("Authorization", "Bearer valid-token")
	respNotFound := httptest.NewRecorder()
	engine.ServeHTTP(respNotFound, reqNotFound)

	var resNotFound struct {
		Code common.ErrorCode `json:"code"`
	}
	_ = json.Unmarshal(respNotFound.Body.Bytes(), &resNotFound)
	if resNotFound.Code != common.CodeNotFound {
		t.Errorf("expected CodeNotFound (102), got %d", resNotFound.Code)
	}

	// 4. OpenAPI spec verification
	spec := r.openapiHandler.Registry().GetSpec()
	opItem := spec.Paths["/api/v1/documents/{id}"]
	if opItem == nil || opItem.Get == nil {
		t.Fatalf("expected /api/v1/documents/{id} in openapi paths")
	}
	if len(opItem.Get.Security) != 1 || opItem.Get.Security[0]["ApiKeyAuth"] == nil {
		t.Errorf("expected ApiKeyAuth security requirement on /api/v1/documents/{id}")
	}
	if len(opItem.Get.Parameters) == 0 || opItem.Get.Parameters[0].Name != "id" || opItem.Get.Parameters[0].In != "path" {
		t.Errorf("expected path parameter 'id' in openapi documentation")
	}
}

// TestMigratedRoute_UploadAgentFile verifies the real AgentHandler.UploadAgentFile implementation
// mounted with RequireCanvasAccess, 64 MiB limit, URL query parameter isolation, and OpenAPI schema.
func TestMigratedRoute_UploadAgentFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	canvasLoader := &fakeCanvasLoaderForMigration{canvas: &entity.UserCanvas{ID: "c1"}}
	fileSvc := &fakeAgentFileServiceForMigration{}
	realAgentHandler := handler.NewAgentHandlerForTest(canvasLoader, fileSvc)

	r := &Router{
		authHandler:    handler.NewAuthHandler(),
		agentHandler:   realAgentHandler,
		openapiHandler: openapi.NewHandler(openapi.NewRegistry()),
	}

	betaGroup := engine.Group("/api/v1")
	betaGroup.Use(func(c *gin.Context) { c.Next() })

	RegisterTypedRoute(
		r,
		betaGroup,
		http.MethodPost,
		"/agents/:canvas_id/upload",
		"Upload agent file",
		realAgentHandler.UploadAgentFile,
		WithMiddleware(realAgentHandler.RequireCanvasAccess()),
		WithMaxBodyBytes(64<<20),
		WithResponseDataSchema(&openapi.Schema{
			OneOf: []*openapi.Schema{
				{Type: "object", Description: "Single uploaded file metadata"},
				{Type: "array", Items: &openapi.Schema{Type: "object"}, Description: "List of uploaded file metadata"},
			},
		}),
	)

	// 1. Single file upload returns single dict
	body1 := &bytes.Buffer{}
	mw1 := multipart.NewWriter(body1)
	fw1, _ := mw1.CreateFormFile("file", "f1.txt")
	fw1.Write([]byte("content 1"))
	mw1.Close()

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/c1/upload", body1)
	req1.Header.Set("Content-Type", mw1.FormDataContentType())
	resp1 := httptest.NewRecorder()
	engine.ServeHTTP(resp1, req1)

	if resp1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp1.Code, resp1.Body.String())
	}
	var res1 struct {
		Code common.ErrorCode       `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(resp1.Body.Bytes(), &res1); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res1.Data["id"] != "file-f1.txt" {
		t.Errorf("expected single file data with id file-f1.txt, got %#v", res1.Data)
	}

	// 2. Multiple files upload returns array of dicts
	body2 := &bytes.Buffer{}
	mw2 := multipart.NewWriter(body2)
	fw2a, _ := mw2.CreateFormFile("file", "f1.txt")
	fw2a.Write([]byte("content 1"))
	fw2b, _ := mw2.CreateFormFile("file", "f2.txt")
	fw2b.Write([]byte("content 2"))
	mw2.Close()

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/c1/upload", body2)
	req2.Header.Set("Content-Type", mw2.FormDataContentType())
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)

	var res2 struct {
		Code common.ErrorCode         `json:"code"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &res2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(res2.Data) != 2 {
		t.Errorf("expected 2 file records in data array, got %d", len(res2.Data))
	}

	// 3. URL import via query parameter ?url= with single file
	body3 := &bytes.Buffer{}
	mw3 := multipart.NewWriter(body3)
	fw3, _ := mw3.CreateFormFile("file", "dummy.txt")
	fw3.Write([]byte("dummy"))
	mw3.Close()

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/c1/upload?url=https%3A%2F%2Fexample.com%2Fremote.pdf", body3)
	req3.Header.Set("Content-Type", mw3.FormDataContentType())
	resp3 := httptest.NewRecorder()
	engine.ServeHTTP(resp3, req3)

	var res3 struct {
		Code common.ErrorCode       `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(resp3.Body.Bytes(), &res3)
	if res3.Data["mode"] != "url" || res3.Data["url"] != "https://example.com/remote.pdf" {
		t.Errorf("expected URL import mode, got %#v", res3.Data)
	}

	// 4. Form-data "url" field is ignored; URL import is NOT triggered
	body4 := &bytes.Buffer{}
	mw4 := multipart.NewWriter(body4)
	fw4, _ := mw4.CreateFormFile("file", "f1.txt")
	fw4.Write([]byte("content"))
	_ = mw4.WriteField("url", "https://example.com/should_be_ignored")
	mw4.Close()

	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/c1/upload", body4)
	req4.Header.Set("Content-Type", mw4.FormDataContentType())
	resp4 := httptest.NewRecorder()
	engine.ServeHTTP(resp4, req4)

	var res4 struct {
		Code common.ErrorCode       `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(resp4.Body.Bytes(), &res4)
	if res4.Data["mode"] == "url" {
		t.Errorf("form-data url field should NOT trigger URL import!")
	}
	if res4.Data["id"] != "file-f1.txt" {
		t.Errorf("expected regular file upload, got %#v", res4.Data)
	}

	// 5. Exceeding 64 MiB limit is rejected with CodeArgumentError
	body5 := &bytes.Buffer{}
	req5 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/c1/upload", body5)
	req5.ContentLength = 65 << 20 // 65 MiB > 64 MiB
	resp5 := httptest.NewRecorder()
	engine.ServeHTTP(resp5, req5)

	var res5 struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	_ = json.Unmarshal(resp5.Body.Bytes(), &res5)
	if res5.Code != common.CodeArgumentError || !strings.Contains(res5.Message, "request body too large.") {
		t.Errorf("expected request body too large error, got code=%d msg=%s", res5.Code, res5.Message)
	}

	// 6. Canvas access failure short-circuits BEFORE body parsing with CodeOperatingError
	req6 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/unauthorized-canvas/upload", nil)
	resp6 := httptest.NewRecorder()
	engine.ServeHTTP(resp6, req6)

	var res6 struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	_ = json.Unmarshal(resp6.Body.Bytes(), &res6)
	if res6.Code != common.CodeOperatingError || !strings.Contains(res6.Message, "Make sure you have permission to access the agent.") {
		t.Errorf("expected CodeOperatingError canvas access error, got code=%d msg=%s", res6.Code, res6.Message)
	}

	// 7. OpenAPI spec verification
	spec := r.openapiHandler.Registry().GetSpec()
	opItem := spec.Paths["/api/v1/agents/{canvas_id}/upload"]
	if opItem == nil || opItem.Post == nil {
		t.Fatalf("expected /api/v1/agents/{canvas_id}/upload in openapi spec")
	}

	// Path param canvas_id must be in parameters
	foundCanvasID := false
	foundURLQuery := false
	for _, p := range opItem.Post.Parameters {
		if p.Name == "canvas_id" && p.In == "path" {
			foundCanvasID = true
		}
		if p.Name == "url" && p.In == "query" {
			foundURLQuery = true
		}
	}
	if !foundCanvasID {
		t.Errorf("expected canvas_id path parameter in openapi parameters")
	}
	if !foundURLQuery {
		t.Errorf("expected url query parameter in openapi parameters")
	}

	// RequestBody must be multipart/form-data
	if opItem.Post.RequestBody == nil || opItem.Post.RequestBody.Content["multipart/form-data"] == nil {
		t.Fatalf("expected multipart/form-data request body on agent upload endpoint")
	}
	mpSchema := opItem.Post.RequestBody.Content["multipart/form-data"].Schema
	if mpSchema == nil || mpSchema.Properties["file"] == nil {
		t.Fatalf("expected file property in multipart schema")
	}
	if mpSchema.Properties["url"] != nil {
		t.Errorf("url is a query parameter and should NOT be in multipart form properties!")
	}
	fileProp := mpSchema.Properties["file"]
	if fileProp.Type != "array" || fileProp.Items == nil || fileProp.Items.Format != "binary" {
		t.Errorf("expected binary array schema for file upload, got %#v", fileProp)
	}

	// Response schema data must be oneOf
	resp200 := opItem.Post.Responses["200"]
	if resp200 == nil || resp200.Content["application/json"] == nil {
		t.Fatalf("expected 200 application/json response")
	}
	dataProp := resp200.Content["application/json"].Schema.Properties["data"]
	if dataProp == nil || len(dataProp.OneOf) != 2 {
		t.Fatalf("expected data property to have oneOf with 2 schemas (object and array), got %#v", dataProp)
	}
}
