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

package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestConvertGinPathToOpenAPI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "/api/v1/datasets/:dataset_id/documents/:document_id",
			want:  "/api/v1/datasets/{dataset_id}/documents/{document_id}",
		},
		{
			input: "/api/v1/artifacts/:page_type/*slug",
			want:  "/api/v1/artifacts/{page_type}/{slug}",
		},
		{
			input: "/api/v1/users/me",
			want:  "/api/v1/users/me",
		},
	}

	for _, tt := range tests {
		got := ConvertGinPathToOpenAPI(tt.input)
		if got != tt.want {
			t.Errorf("ConvertGinPathToOpenAPI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractPathParameters(t *testing.T) {
	params := ExtractPathParameters("/api/v1/datasets/:dataset_id/documents/:document_id/*file")
	if len(params) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(params))
	}
	if params[0] != "dataset_id" || params[1] != "document_id" || params[2] != "file" {
		t.Errorf("unexpected parameters: %v", params)
	}
}

func TestDeriveTag(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/datasets/create", "Datasets"},
		{"/api/v1/documents/upload", "Documents"},
		{"/api/v1/chats", "Chats"},
		{"/api/v1/agents/run", "Agents"},
		{"/api/v1/retrieval", "Retrieval & Search"},
		{"/api/v1/files/download", "Files & Commits"},
		{"/api/v1/unknown_feature", "Unknown_feature"},
	}
	for _, tt := range tests {
		got := DeriveTag(tt.path)
		if got != tt.want {
			t.Errorf("DeriveTag(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestOpenAPIRegistryAndHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 1. Raw Gin handler that is NOT registered with a contract
	engine.GET("/api/v1/raw-uncontracted", func(c *gin.Context) {
		c.String(http.StatusOK, "raw")
	})

	registry := NewRegistry()

	// 2. Explicitly registered public contract
	registry.RegisterContract(http.MethodGet, "/api/v1/system/ping", Contract{
		Summary:     "Ping system",
		Description: "Check system health",
		Auth:        false,
		Responses: map[string]*Response{
			"200": BuildStandardResponse(map[string]any{"ping": "pong"}, "Ping response"),
		},
	})

	// 3. Explicitly registered protected multipart contract
	registry.RegisterContract(http.MethodPost, "/api/v1/documents/upload", Contract{
		Summary:     "Upload document",
		Description: "Upload a file to dataset",
		Auth:        true,
		RequestBody: BuildMultipartRequestBody(map[string]string{
			"file*":       "Document binary file",
			"dataset_id*": "Target dataset ID",
		}, "Multipart upload form"),
		Responses: map[string]*Response{
			"200": BuildStandardResponse(map[string]any{"id": "doc-1"}, "Upload result"),
			"401": BuildErrorResponse("Unauthorized"),
		},
	})

	// 4. Explicitly registered protected contract with path and query parameters
	registry.RegisterContract(http.MethodGet, "/api/v1/datasets/:dataset_id/documents", Contract{
		Summary:     "List dataset documents",
		Description: "Get paginated documents in dataset",
		Auth:        true,
		Parameters: []*Parameter{
			{
				Name:        "dataset_id",
				In:          "path",
				Required:    true,
				Description: "Dataset ID",
				Schema:      &Schema{Type: "string"},
			},
			{
				Name:        "page",
				In:          "query",
				Required:    false,
				Description: "Page number",
				Schema:      &Schema{Type: "integer"},
			},
		},
		Responses: map[string]*Response{
			"200": BuildStandardResponse(map[string]any{"total": 1}, "Document list"),
			"401": BuildErrorResponse("Unauthorized"),
		},
	})

	handler := NewHandler(registry)
	handler.RegisterRoutes(engine)

	// Verify /openapi.json only contains explicitly registered contracts
	resp := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json status = %d, want 200", resp.Code)
	}
	if !strings.Contains(resp.Header().Get("Content-Type"), "application/json") {
		t.Errorf("expected json content-type, got %s", resp.Header().Get("Content-Type"))
	}

	var doc Document
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to parse openapi json: %v", err)
	}
	if doc.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %s", doc.OpenAPI)
	}

	// Uncontracted route must NOT appear in OpenAPI spec
	if _, exists := doc.Paths["/api/v1/raw-uncontracted"]; exists {
		t.Errorf("uncontracted route /api/v1/raw-uncontracted should NOT appear in openapi spec")
	}

	// Public route /api/v1/system/ping must have no ApiKeyAuth and explicitly serialize security: []
	pingItem := doc.Paths["/api/v1/system/ping"]
	if pingItem == nil || pingItem.Get == nil {
		t.Fatalf("missing /api/v1/system/ping in openapi doc")
	}
	if len(pingItem.Get.Security) != 0 {
		t.Errorf("expected public route /api/v1/system/ping to have no security requirement, got %v", pingItem.Get.Security)
	}
	if !strings.Contains(resp.Body.String(), `"security":[]`) && !strings.Contains(resp.Body.String(), `"security": []`) {
		t.Errorf("expected public route to explicitly output 'security: []' in serialized json: %s", resp.Body.String())
	}

	// Multipart upload route must have ApiKeyAuth and multipart/form-data content
	uploadItem := doc.Paths["/api/v1/documents/upload"]
	if uploadItem == nil || uploadItem.Post == nil {
		t.Fatalf("missing /api/v1/documents/upload POST operation in openapi doc")
	}
	if len(uploadItem.Post.Security) == 0 || uploadItem.Post.Security[0]["ApiKeyAuth"] == nil {
		t.Errorf("expected ApiKeyAuth on upload operation, got %v", uploadItem.Post.Security)
	}
	if uploadItem.Post.RequestBody == nil || uploadItem.Post.RequestBody.Content["multipart/form-data"] == nil {
		t.Fatalf("expected multipart/form-data RequestBody on upload operation")
	}
	mpSchema := uploadItem.Post.RequestBody.Content["multipart/form-data"].Schema
	if mpSchema.Properties["file"] == nil || mpSchema.Properties["file"].Format != "binary" {
		t.Errorf("expected binary file schema in multipart properties, got %#v", mpSchema.Properties["file"])
	}

	// Path & query params route
	docListItem := doc.Paths["/api/v1/datasets/{dataset_id}/documents"]
	if docListItem == nil || docListItem.Get == nil {
		t.Fatalf("missing /api/v1/datasets/{dataset_id}/documents GET operation in openapi doc")
	}
	var hasPath, hasQuery bool
	for _, p := range docListItem.Get.Parameters {
		if p.In == "path" && p.Name == "dataset_id" {
			hasPath = true
		}
		if p.In == "query" && p.Name == "page" {
			hasQuery = true
		}
	}
	if !hasPath || !hasQuery {
		t.Errorf("expected path and query parameters, got: %v", docListItem.Get.Parameters)
	}

	// Verify /docs
	docsResp := httptest.NewRecorder()
	docsReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/docs", nil)
	engine.ServeHTTP(docsResp, docsReq)

	if docsResp.Code != http.StatusOK {
		t.Fatalf("GET /docs status = %d, want 200", docsResp.Code)
	}
	if !strings.Contains(docsResp.Header().Get("Content-Type"), "text/html") {
		t.Errorf("expected html content-type, got %s", docsResp.Header().Get("Content-Type"))
	}
	if !strings.Contains(docsResp.Body.String(), "SwaggerUIBundle") {
		t.Errorf("expected SwaggerUIBundle script in docs html")
	}

	// Verify /api/v1/docs requests /api/v1/openapi.json
	apiDocsResp := httptest.NewRecorder()
	apiDocsReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/docs", nil)
	engine.ServeHTTP(apiDocsResp, apiDocsReq)

	if apiDocsResp.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/docs status = %d, want 200", apiDocsResp.Code)
	}
	if !strings.Contains(apiDocsResp.Body.String(), "/api/v1/openapi.json") {
		t.Errorf("expected /api/v1/openapi.json in /api/v1/docs html, got %s", apiDocsResp.Body.String())
	}
}

func TestOpenAPIRegistry_EmptyPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	registry := NewRegistry()
	handler := NewHandler(registry)
	handler.RegisterRoutes(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json status = %d, want 200", resp.Code)
	}

	rawJSON := resp.Body.String()
	if !strings.Contains(rawJSON, `"paths":{}`) && !strings.Contains(rawJSON, `"paths": {}`) {
		t.Errorf("expected empty paths '{}' in openapi spec, got: %s", rawJSON)
	}

	var doc Document
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to unmarshal empty openapi doc: %v", err)
	}
	if len(doc.Paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(doc.Paths))
	}
}

type samplePointerStruct struct {
	Name    *string `json:"name" doc:"Name pointer"`
	Age     *int    `json:"age" doc:"Age pointer"`
	Enabled *bool   `json:"enabled" doc:"Enabled pointer"`
}

func TestBuildSchemaFromType_Pointers(t *testing.T) {
	typ := reflect.TypeOf(samplePointerStruct{})
	schema := BuildSchemaFromType(typ)

	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %s", schema.Type)
	}
	if schema.Properties["name"] == nil || schema.Properties["name"].Type != "string" {
		t.Errorf("expected string property for *string field, got %#v", schema.Properties["name"])
	}
	if schema.Properties["age"] == nil || schema.Properties["age"].Type != "integer" {
		t.Errorf("expected integer property for *int field, got %#v", schema.Properties["age"])
	}
	if schema.Properties["enabled"] == nil || schema.Properties["enabled"].Type != "boolean" {
		t.Errorf("expected boolean property for *bool field, got %#v", schema.Properties["enabled"])
	}
}

type sampleRecursiveNode struct {
	ID       string                 `json:"id" doc:"Node ID"`
	Children []*sampleRecursiveNode `json:"children" doc:"Child nodes"`
}

func TestBuildSchemaFromType_RecursiveStruct(t *testing.T) {
	typ := reflect.TypeOf(sampleRecursiveNode{})
	components := make(map[string]*Schema)
	schema := BuildSchemaWithComponents(typ, components)

	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %s", schema.Type)
	}
	if schema.Properties["id"] == nil || schema.Properties["id"].Type != "string" {
		t.Fatalf("expected string id property")
	}
	children := schema.Properties["children"]
	if children == nil || children.Type != "array" {
		t.Fatalf("expected array children property, got %#v", children)
	}
	if children.Items == nil {
		t.Fatalf("expected items in children array")
	}
	if children.Items.Ref != "#/components/schemas/sampleRecursiveNode" {
		t.Errorf("expected circular reference $ref to sampleRecursiveNode, got %s", children.Items.Ref)
	}

	// Verify recursive struct schema is registered in components
	compSchema := components["sampleRecursiveNode"]
	if compSchema == nil {
		t.Fatalf("expected sampleRecursiveNode in components map, got nil")
	}
	if compSchema.Properties["id"] == nil || compSchema.Properties["id"].Type != "string" {
		t.Errorf("expected sampleRecursiveNode in components to have string id property")
	}

	// Verify registry registers component schema into document
	reg := NewRegistry()
	reg.RegisterComponentSchema("sampleRecursiveNode", compSchema)
	doc := reg.GetSpec()
	if doc.Components.Schemas["sampleRecursiveNode"] == nil {
		t.Errorf("expected sampleRecursiveNode in doc.Components.Schemas")
	}
}

type samplePagination struct {
	Page     int `json:"page" doc:"Page number"`
	PageSize int `json:"page_size" doc:"Page size"`
}

type sampleListReq struct {
	samplePagination
	Filter string `json:"filter" doc:"Filter query" binding:"required"`
	Cond   string `json:"cond" doc:"Conditional field" binding:"required_if=Filter all"`
}

func TestBuildSchemaFromType_EmbeddedStructAndBinding(t *testing.T) {
	typ := reflect.TypeOf(sampleListReq{})
	schema := BuildSchemaFromType(typ)

	if schema.Type != "object" {
		t.Fatalf("expected object, got %s", schema.Type)
	}

	// Verify embedded fields are flattened into parent properties
	if schema.Properties["page"] == nil || schema.Properties["page"].Type != "integer" {
		t.Errorf("expected flattened page property from embedded struct, got %#v", schema.Properties["page"])
	}
	if schema.Properties["page_size"] == nil || schema.Properties["page_size"].Type != "integer" {
		t.Errorf("expected flattened page_size property from embedded struct, got %#v", schema.Properties["page_size"])
	}
	if schema.Properties["filter"] == nil || schema.Properties["filter"].Type != "string" {
		t.Errorf("expected filter property, got %#v", schema.Properties["filter"])
	}

	// Verify required field includes "filter" but NOT "cond" (due to required_if)
	hasFilter := false
	hasCond := false
	for _, req := range schema.Required {
		if req == "filter" {
			hasFilter = true
		}
		if req == "cond" {
			hasCond = true
		}
	}
	if !hasFilter {
		t.Errorf("expected filter to be required")
	}
	if hasCond {
		t.Errorf("expected cond not to be required since it is required_if")
	}
}

func TestRegisterContract_EmptyResponsesDefault(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterContract(http.MethodGet, "/api/v1/test", Contract{
		Summary: "Test operation",
	})
	doc := reg.GetSpec()
	pathItem := doc.Paths["/api/v1/test"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected registered GET operation")
	}
	if len(pathItem.Get.Responses) == 0 {
		t.Fatalf("expected non-empty default responses for OAS 3.0.3 compliance")
	}
	if pathItem.Get.Responses["200"] == nil {
		t.Errorf("expected default 200 response")
	}
}

func TestGenerateOperationID(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{"GET", "/api/v1/documents/:id", "get_api_v1_documents_id"},
		{"POST", "/api/v1/agents/:canvas_id/upload", "post_api_v1_agents_canvas_id_upload"},
		{"GET", "/api/v1/users", "get_api_v1_users"},
	}

	for _, tt := range tests {
		got := GenerateOperationID(tt.method, tt.path)
		if got != tt.want {
			t.Errorf("GenerateOperationID(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
		}
	}
}

type customTextStruct struct {
	Text string
}

func (c customTextStruct) MarshalText() ([]byte, error) {
	return []byte(c.Text), nil
}

type sampleTimeAndFallbackStruct struct {
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  *time.Time       `json:"updated_at,omitempty"`
	TextVal    customTextStruct `json:"text_val"`
	ByteSlice  []byte           `json:"byte_slice"`
	ByteArray  [16]byte         `json:"byte_array"`
	DynamicAny any              `json:"dynamic_any"`
	NoTag      string
	OmitEmpty  string `json:",omitempty"`
}

func TestBuildSchemaFromType_TimeAndFieldNameFallback(t *testing.T) {
	typ := reflect.TypeOf(sampleTimeAndFallbackStruct{})
	schema := BuildSchemaFromType(typ)

	if schema.Type != "object" {
		t.Fatalf("expected object, got %s", schema.Type)
	}

	// 1. time.Time mapped to string with date-time format
	createdAt := schema.Properties["created_at"]
	if createdAt == nil || createdAt.Type != "string" || createdAt.Format != "date-time" {
		t.Errorf("expected created_at to be string with format date-time, got %#v", createdAt)
	}
	updatedAt := schema.Properties["updated_at"]
	if updatedAt == nil || updatedAt.Type != "string" || updatedAt.Format != "date-time" {
		t.Errorf("expected updated_at to be string with format date-time, got %#v", updatedAt)
	}

	// 2. TextMarshaler (struct-backed) mapped to string
	textVal := schema.Properties["text_val"]
	if textVal == nil || textVal.Type != "string" {
		t.Errorf("expected text_val to be string, got %#v", textVal)
	}

	// 3. Byte slice mapped to base64 string with byte format; byte array stays array of integers per encoding/json
	byteSlice := schema.Properties["byte_slice"]
	if byteSlice == nil || byteSlice.Type != "string" || byteSlice.Format != "byte" {
		t.Errorf("expected byte_slice to be string with format byte, got %#v", byteSlice)
	}
	byteArray := schema.Properties["byte_array"]
	if byteArray == nil || byteArray.Type != "array" || byteArray.Items == nil || byteArray.Items.Type != "integer" {
		t.Errorf("expected byte_array to be array of integers, got %#v", byteArray)
	}

	// Verify byte array serialization matches OpenAPI schema (array of integers, NOT base64 string)
	var sampleArr [16]byte
	arrJSON, err := json.Marshal(sampleArr)
	if err != nil {
		t.Fatalf("json.Marshal([16]byte) failed: %v", err)
	}
	var arrElements []int
	if err := json.Unmarshal(arrJSON, &arrElements); err != nil {
		t.Fatalf("json.Unmarshal byte array output as []int failed: %v", err)
	}
	if len(arrElements) != 16 {
		t.Errorf("expected 16 integer elements from json.Marshal([16]byte), got %d", len(arrElements))
	}

	// 4. Interface/any mapped to unconstrained empty schema
	dynamicAny := schema.Properties["dynamic_any"]
	if dynamicAny == nil || dynamicAny.Type != "" {
		t.Errorf("expected dynamic_any to be empty schema without type constraint, got %#v", dynamicAny)
	}

	// 5. Fallback to Go field name matching encoding/json
	if schema.Properties["NoTag"] == nil || schema.Properties["NoTag"].Type != "string" {
		t.Errorf("expected property 'NoTag' matching Go field name, got %#v", schema.Properties["NoTag"])
	}
	if schema.Properties["OmitEmpty"] == nil || schema.Properties["OmitEmpty"].Type != "string" {
		t.Errorf("expected property 'OmitEmpty' matching Go field name, got %#v", schema.Properties["OmitEmpty"])
	}
}
