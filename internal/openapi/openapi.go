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
	"encoding"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// Document represents an OpenAPI 3.0.3 document.
type Document struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Servers    []Server              `json:"servers,omitempty"`
	Paths      map[string]*PathItem  `json:"paths"`
	Components *Components           `json:"components,omitempty"`
	Security   []map[string][]string `json:"security,omitempty"`
	Tags       []Tag                 `json:"tags,omitempty"`
}

// Info provides metadata about the API.
type Info struct {
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty"`
	Contact        *Contact `json:"contact,omitempty"`
	License        *License `json:"license,omitempty"`
	Version        string   `json:"version"`
}

// Contact information for the exposed API.
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License information for the exposed API.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Server represents a Server.
type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Tag provides additional metadata for tags used by operations.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Get         *Operation   `json:"get,omitempty"`
	Put         *Operation   `json:"put,omitempty"`
	Post        *Operation   `json:"post,omitempty"`
	Delete      *Operation   `json:"delete,omitempty"`
	Patch       *Operation   `json:"patch,omitempty"`
	Head        *Operation   `json:"head,omitempty"`
	Parameters  []*Parameter `json:"parameters,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Parameters  []*Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
}

// MarshalJSON customizes serialization to guarantee public operations output "security": []
// instead of omitting the field, thereby explicitly disabling root security in OpenAPI 3.
func (o *Operation) MarshalJSON() ([]byte, error) {
	type Alias Operation
	if o.Security != nil && len(o.Security) == 0 {
		return json.Marshal(&struct {
			*Alias
			Security []map[string][]string `json:"security"`
		}{
			Alias:    (*Alias)(o),
			Security: []map[string][]string{},
		})
	}
	return json.Marshal((*Alias)(o))
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // "path", "query", "header", "cookie"
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

// RequestBody describes a single request body.
type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Content     map[string]*MediaType `json:"content"`
	Required    bool                  `json:"required,omitempty"`
}

// MediaType provides schema and examples for a media type.
type MediaType struct {
	Schema  *Schema `json:"schema,omitempty"`
	Example any     `json:"example,omitempty"`
}

// Response describes a single response from an API Operation.
type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Schema represents a JSON Schema object for OpenAPI 3.0.
type Schema struct {
	Ref         string             `json:"$ref,omitempty"`
	Type        string             `json:"type,omitempty"`
	Format      string             `json:"format,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Default     any                `json:"default,omitempty"`
	Enum        []any              `json:"enum,omitempty"`
	Example     any                `json:"example,omitempty"`
	OneOf       []*Schema          `json:"oneOf,omitempty"`
	AnyOf       []*Schema          `json:"anyOf,omitempty"`
}

// Components holds reusable objects.
type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme defines a security scheme.
type SecurityScheme struct {
	Type         string `json:"type"`
	Description  string `json:"description,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Scheme       string `json:"scheme,omitempty"`
}

var (
	ginParamRegex = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)
	ginWildRegex  = regexp.MustCompile(`\*([a-zA-Z0-9_]+)`)
)

// ConvertGinPathToOpenAPI converts Gin route patterns to OpenAPI path format.
// e.g., /api/v1/datasets/:dataset_id/documents/*filename -> /api/v1/datasets/{dataset_id}/documents/{filename}
func ConvertGinPathToOpenAPI(ginPath string) string {
	res := ginParamRegex.ReplaceAllString(ginPath, "{$1}")
	res = ginWildRegex.ReplaceAllString(res, "{$1}")
	return res
}

// ExtractPathParameters extracts parameter names from a Gin path.
func ExtractPathParameters(ginPath string) []string {
	var params []string
	matches := ginParamRegex.FindAllStringSubmatch(ginPath, -1)
	for _, m := range matches {
		if len(m) > 1 {
			params = append(params, m[1])
		}
	}
	wildMatches := ginWildRegex.FindAllStringSubmatch(ginPath, -1)
	for _, m := range wildMatches {
		if len(m) > 1 {
			params = append(params, m[1])
		}
	}
	return params
}

// DeriveTag derives an intuitive tag name from route path.
func DeriveTag(path string) string {
	clean := strings.TrimPrefix(path, "/api/v1/")
	clean = strings.TrimPrefix(clean, "api/v1/")
	clean = strings.TrimPrefix(clean, "/")
	parts := strings.Split(clean, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "Default"
	}
	switch parts[0] {
	case "datasets":
		return "Datasets"
	case "documents":
		return "Documents"
	case "chats", "chat":
		return "Chats"
	case "agents", "agentbots", "tasks":
		return "Agents"
	case "searches", "searchbots", "retrieval":
		return "Retrieval & Search"
	case "files", "folders", "workspace-files", "workspaces":
		return "Files & Commits"
	case "auth", "users":
		return "Auth & Users"
	case "tenants", "tenant":
		return "Tenants"
	case "providers", "models", "all-models":
		return "Models & Providers"
	case "memories", "messages":
		return "Memory"
	case "skills":
		return "Skills"
	case "connectors":
		return "Connectors"
	case "mcp":
		return "MCP"
	case "system", "settings":
		return "System"
	case "compilation-templates", "compilation-template-groups":
		return "Compilation"
	case "chat-channels":
		return "Chat Channels"
	case "langfuse":
		return "Observability"
	case "dify":
		return "Dify Integration"
	default:
		return upperFirst(parts[0])
	}
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// Registry manages the OpenAPI document.
type Registry struct {
	mu         sync.RWMutex
	doc        *Document
	cachedJSON []byte
}

// NewRegistry creates a new OpenAPI registry with default metadata.
func NewRegistry() *Registry {
	return &Registry{
		doc: &Document{
			OpenAPI: "3.0.3",
			Info: Info{
				Title:       "RAGFlow API",
				Version:     "1.0.0",
				Description: "OpenAPI Specification and interactive documentation for RAGFlow RESTful endpoints.",
				Contact: &Contact{
					Name: "InfiniFlow",
					URL:  "https://ragflow.io",
				},
				License: &License{
					Name: "Apache 2.0",
					URL:  "https://www.apache.org/licenses/LICENSE-2.0",
				},
			},
			Servers: []Server{
				{URL: "/", Description: "Current host"},
			},
			Paths: make(map[string]*PathItem),
			Security: []map[string][]string{
				{"ApiKeyAuth": {}},
			},
			Components: &Components{
				Schemas: make(map[string]*Schema),
				SecuritySchemes: map[string]*SecurityScheme{
					"ApiKeyAuth": {
						Type:        "apiKey",
						Name:        "Authorization",
						In:          "header",
						Description: "Bearer API token: 'Bearer <api_key>'",
					},
				},
			},
		},
	}
}

// Contract defines an accurate OpenAPI contract for an endpoint.
type Contract struct {
	Summary          string
	Description      string
	Tags             []string
	Auth             bool // whether ApiKeyAuth is required
	Deprecated       bool
	Parameters       []*Parameter
	RequestBody      *RequestBody
	Responses        map[string]*Response
	ComponentSchemas map[string]*Schema
}

// RegisterOperation explicitly registers or overrides an operation.
func (r *Registry) RegisterOperation(method, path string, op *Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	openAPIPath := ConvertGinPathToOpenAPI(path)
	pathItem := r.doc.Paths[openAPIPath]
	if pathItem == nil {
		pathItem = &PathItem{}
		r.doc.Paths[openAPIPath] = pathItem
	}

	if op != nil && len(op.Responses) == 0 {
		op.Responses = map[string]*Response{
			"200": {Description: "Successful operation"},
		}
	}
	setMethodOperation(pathItem, method, op)
	r.syncTagsLocked()
	r.cachedJSON = nil
}

// RegisterComponentSchema registers a reusable schema under components.schemas.
func (r *Registry) RegisterComponentSchema(name string, s *Schema) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.doc.Components == nil {
		r.doc.Components = &Components{
			Schemas: make(map[string]*Schema),
		}
	}
	if r.doc.Components.Schemas == nil {
		r.doc.Components.Schemas = make(map[string]*Schema)
	}
	r.doc.Components.Schemas[name] = s
	r.cachedJSON = nil
}

// GenerateOperationID generates a deterministic operation ID from HTTP method and route path.
// It converts Gin path parameters (:id -> {id}) and strips braces to produce a clean identifier.
func GenerateOperationID(method, path string) string {
	openAPIPath := ConvertGinPathToOpenAPI(path)
	opID := strings.ToLower(method) + "_" + strings.ReplaceAll(strings.Trim(openAPIPath, "/"), "/", "_")
	opID = strings.ReplaceAll(opID, "{", "")
	opID = strings.ReplaceAll(opID, "}", "")
	return opID
}

// RegisterContract registers an explicit, verified API contract for an endpoint.
func (r *Registry) RegisterContract(method, path string, c Contract) {
	var sec []map[string][]string
	if c.Auth {
		sec = []map[string][]string{{"ApiKeyAuth": {}}}
	} else {
		sec = []map[string][]string{}
	}

	tags := c.Tags
	if len(tags) == 0 {
		tags = []string{DeriveTag(path)}
	}

	responses := c.Responses
	if len(responses) == 0 {
		responses = map[string]*Response{
			"200": {Description: "Successful operation"},
		}
	}

	op := &Operation{
		Tags:        tags,
		Summary:     c.Summary,
		Description: c.Description,
		OperationID: GenerateOperationID(method, path),
		Parameters:  c.Parameters,
		RequestBody: c.RequestBody,
		Responses:   responses,
		Security:    sec,
		Deprecated:  c.Deprecated,
	}

	if c.ComponentSchemas != nil {
		for name, s := range c.ComponentSchemas {
			r.RegisterComponentSchema(name, s)
		}
	}

	r.RegisterOperation(method, path, op)
}

// BuildJSONRequestBody creates a RequestBody with an application/json schema derived from sample.
func BuildJSONRequestBody(sample any, required bool, description ...string) *RequestBody {
	desc := "JSON request payload"
	if len(description) > 0 && description[0] != "" {
		desc = description[0]
	}
	schema := BuildSchemaFromSample(sample)
	return &RequestBody{
		Description: desc,
		Required:    required,
		Content: map[string]*MediaType{
			"application/json": {Schema: schema},
		},
	}
}

// BuildMultipartRequestBody creates a RequestBody for multipart/form-data.
// Field names ending in '*' are treated as binary files and marked required.
func BuildMultipartRequestBody(fields map[string]string, description ...string) *RequestBody {
	desc := "Multipart form data"
	if len(description) > 0 && description[0] != "" {
		desc = description[0]
	}
	props := make(map[string]*Schema)
	var required []string
	for name, fieldDesc := range fields {
		if strings.HasSuffix(name, "*") {
			cleanName := strings.TrimSuffix(name, "*")
			props[cleanName] = &Schema{
				Type:        "string",
				Format:      "binary",
				Description: fieldDesc,
			}
			required = append(required, cleanName)
		} else {
			props[name] = &Schema{
				Type:        "string",
				Description: fieldDesc,
			}
		}
	}
	sort.Strings(required)
	return &RequestBody{
		Description: desc,
		Required:    true,
		Content: map[string]*MediaType{
			"multipart/form-data": {
				Schema: &Schema{
					Type:       "object",
					Properties: props,
					Required:   required,
				},
			},
		},
	}
}

// BuildStandardResponse creates a 200 Response wrapping sample in RAGFlow's standard {code, message, data} envelope.
func BuildStandardResponse(sample any, description ...string) *Response {
	desc := "Successful operation"
	if len(description) > 0 && description[0] != "" {
		desc = description[0]
	}
	dataSchema := BuildSchemaFromSample(sample)
	envelope := &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"code": {
				Type:        "integer",
				Description: "Application error code (0 = success)",
			},
			"message": {
				Type:        "string",
				Description: "Response status message",
			},
			"data": dataSchema,
		},
		Required: []string{"code", "message"},
	}
	return &Response{
		Description: desc,
		Content: map[string]*MediaType{
			"application/json": {Schema: envelope},
		},
	}
}

// BuildErrorResponse creates an error response envelope.
func BuildErrorResponse(description string) *Response {
	return &Response{
		Description: description,
		Content: map[string]*MediaType{
			"application/json": {
				Schema: &Schema{
					Type: "object",
					Properties: map[string]*Schema{
						"code":    {Type: "integer", Description: "Application error code"},
						"message": {Type: "string", Description: "Error message"},
					},
					Required: []string{"code", "message"},
				},
			},
		},
	}
}

// BuildSchemaFromSample converts a sample value, pointer, or reflect.Type to a Schema.
func BuildSchemaFromSample(sample any) *Schema {
	if sample == nil {
		return &Schema{Type: "object"}
	}
	if t, ok := sample.(reflect.Type); ok {
		return BuildSchemaFromType(t)
	}
	return BuildSchemaFromType(reflect.TypeOf(sample))
}

// BuildSchemaWithComponents converts a reflect.Type to an OpenAPI Schema and registers
// any referenced recursive struct schemas into the components map.
func BuildSchemaWithComponents(t reflect.Type, components map[string]*Schema) *Schema {
	recursiveTypes := make(map[reflect.Type]string)
	ancestors := make(map[reflect.Type]bool)
	return buildSchemaFromTypeInternal(t, ancestors, recursiveTypes, components)
}

var (
	timeType          = reflect.TypeOf(time.Time{})
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// BuildSchemaFromType converts a reflect.Type to an OpenAPI Schema.
// It detects recursive/cyclic type hierarchies and emits $ref pointers instead of recursing infinitely.
func BuildSchemaFromType(t reflect.Type) *Schema {
	return BuildSchemaWithComponents(t, nil)
}

func buildSchemaFromTypeInternal(
	t reflect.Type,
	ancestors map[reflect.Type]bool,
	recursiveTypes map[reflect.Type]string,
	components map[string]*Schema,
) *Schema {
	if t == nil {
		return &Schema{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == timeType {
		return &Schema{Type: "string", Format: "date-time"}
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return &Schema{Type: "string"}
	}

	if ancestors[t] {
		typeName := t.Name()
		if typeName != "" && components != nil {
			recursiveTypes[t] = typeName
			return &Schema{Ref: "#/components/schemas/" + typeName}
		}
		return &Schema{Type: "object", Description: "recursive reference"}
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string", Format: "byte"}
		}
		return &Schema{
			Type:  "array",
			Items: buildSchemaFromTypeInternal(t.Elem(), ancestors, recursiveTypes, components),
		}
	case reflect.Map:
		return &Schema{
			Type: "object",
		}
	case reflect.Interface:
		return &Schema{}
	case reflect.Struct:
		ancestors[t] = true
		defer delete(ancestors, t)

		props := make(map[string]*Schema)
		var required []string

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() && !field.Anonymous {
				continue
			}

			jsonTag := field.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}

			// Flatten anonymous embedded structs without explicit json tag into parent object,
			// matching encoding/json semantics.
			if field.Anonymous && (jsonTag == "" || jsonTag == ",inline") {
				fieldType := field.Type
				if fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				if fieldType.Kind() == reflect.Struct {
					embeddedSchema := buildSchemaFromTypeInternal(fieldType, ancestors, recursiveTypes, components)
					if embeddedSchema != nil {
						for k, v := range embeddedSchema.Properties {
							props[k] = v
						}
						required = append(required, embeddedSchema.Required...)
					}
					continue
				}
			}

			uriTag := field.Tag.Get("uri")
			formTag := field.Tag.Get("form")
			queryTag := field.Tag.Get("query")
			if jsonTag == "" && (uriTag != "" || formTag != "" || queryTag != "") {
				continue
			}

			name := strings.Split(jsonTag, ",")[0]
			if name == "" {
				name = field.Name
			}

			fieldSchema := buildSchemaFromTypeInternal(field.Type, ancestors, recursiveTypes, components)
			doc := field.Tag.Get("doc")
			if doc == "" {
				doc = field.Tag.Get("desc")
			}
			if doc != "" && fieldSchema.Ref == "" {
				fieldSchema.Description = doc
			}

			bindingTag := field.Tag.Get("binding")
			isRequired := field.Tag.Get("required") == "true"
			if !isRequired && bindingTag != "" {
				for _, part := range strings.Split(bindingTag, ",") {
					if strings.TrimSpace(part) == "required" {
						isRequired = true
						break
					}
				}
			}
			if isRequired {
				required = append(required, name)
			}

			props[name] = fieldSchema
		}

		sort.Strings(required)
		schema := &Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}

		if typeName, isRec := recursiveTypes[t]; isRec && typeName != "" && components != nil {
			components[typeName] = schema
		}

		return schema
	default:
		return &Schema{Type: "object"}
	}
}

func (r *Registry) syncTagsLocked() {
	tagSet := make(map[string]bool)
	for _, item := range r.doc.Paths {
		for _, op := range []*Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch, item.Head} {
			if op != nil {
				for _, t := range op.Tags {
					tagSet[t] = true
				}
			}
		}
	}
	var tags []Tag
	for t := range tagSet {
		tags = append(tags, Tag{Name: t})
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	r.doc.Tags = tags
}

func setMethodOperation(item *PathItem, method string, op *Operation) {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		item.Get = op
	case http.MethodPost:
		item.Post = op
	case http.MethodPut:
		item.Put = op
	case http.MethodDelete:
		item.Delete = op
	case http.MethodPatch:
		item.Patch = op
	case http.MethodHead:
		item.Head = op
	}
}

// GetSpec returns the OpenAPI document.
func (r *Registry) GetSpec() *Document {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.doc
}

// GetSpecJSON returns the marshaled JSON bytes of the OpenAPI document.
func (r *Registry) GetSpecJSON() ([]byte, error) {
	r.mu.RLock()
	if r.cachedJSON != nil {
		data := r.cachedJSON
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cachedJSON != nil {
		return r.cachedJSON, nil
	}
	data, err := json.MarshalIndent(r.doc, "", "  ")
	if err != nil {
		return nil, err
	}
	r.cachedJSON = data
	return data, nil
}

// SwaggerUIHTML returns the standalone Swagger UI HTML template.
const SwaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>RAGFlow API Documentation</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
    .topbar { display: none !important; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      var pathname = window.location.pathname;
      var specURL = pathname.indexOf("/api/v1") !== -1 ? "/api/v1/openapi.json" : "/openapi.json";
      window.ui = SwaggerUIBundle({
        url: specURL,
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
    };
  </script>
</body>
</html>`

// Handler provides Gin HTTP handlers for OpenAPI and Swagger UI.
type Handler struct {
	registry *Registry
}

// Registry returns the underlying OpenAPI registry.
func (h *Handler) Registry() *Registry {
	return h.registry
}

// NewHandler creates a new OpenAPI Handler.
func NewHandler(r *Registry) *Handler {
	return &Handler{registry: r}
}

// GetOpenAPISpec handles GET /openapi.json.
func (h *Handler) GetOpenAPISpec(c *gin.Context) {
	data, err := h.registry.GetSpecJSON()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OpenAPI spec"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

// GetDocsUI handles GET /docs.
func (h *Handler) GetDocsUI(c *gin.Context) {
	specURL := "/openapi.json"
	if strings.Contains(c.Request.URL.Path, "/api/v1") {
		specURL = "/api/v1/openapi.json"
	}
	content := strings.Replace(SwaggerUIHTML, `var specURL = pathname.indexOf("/api/v1") !== -1 ? "/api/v1/openapi.json" : "/openapi.json";`, fmt.Sprintf(`var specURL = %q;`, specURL), 1)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// RegisterRoutes registers /openapi.json and /docs routes on the Gin engine.
func (h *Handler) RegisterRoutes(engine *gin.Engine) {
	engine.GET("/openapi.json", h.GetOpenAPISpec)
	engine.GET("/api/v1/openapi.json", h.GetOpenAPISpec)
	engine.GET("/docs", h.GetDocsUI)
	engine.GET("/docs/", h.GetDocsUI)
	engine.GET("/api/v1/docs", h.GetDocsUI)
	engine.GET("/api/v1/docs/", h.GetDocsUI)
}
