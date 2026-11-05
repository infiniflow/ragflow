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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/openapi"
	"ragflow/internal/permission"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var (
	fileHeaderType      = reflect.TypeOf((*multipart.FileHeader)(nil))
	fileHeaderSliceType = reflect.TypeOf(([]*multipart.FileHeader)(nil))
)

func isMultipartType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i).Type
		if ft == fileHeaderType || ft == fileHeaderSliceType {
			return true
		}
	}
	return false
}

type ginContextKey struct{}
type routeParamsKey struct{}

// WithGinContext stores the gin.Context into ctx.
func WithGinContext(ctx context.Context, c *gin.Context) context.Context {
	return context.WithValue(ctx, ginContextKey{}, c)
}

// GinContext extracts *gin.Context from ctx if present.
func GinContext(ctx context.Context) *gin.Context {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(ginContextKey{}).(*gin.Context)
	return c
}

// WithRouteParams stores route path parameters into ctx.
func WithRouteParams(ctx context.Context, params gin.Params) context.Context {
	return context.WithValue(ctx, routeParamsKey{}, params)
}

// RouteParam retrieves a named route path parameter (e.g. "id") from ctx.
func RouteParam(ctx context.Context, name string) string {
	if ctx == nil {
		return ""
	}
	if params, ok := ctx.Value(routeParamsKey{}).(gin.Params); ok {
		return params.ByName(name)
	}
	if c := GinContext(ctx); c != nil {
		return c.Param(name)
	}
	return ""
}

// RouteParams returns all route path parameters from ctx.
func RouteParams(ctx context.Context) gin.Params {
	if ctx == nil {
		return nil
	}
	if params, ok := ctx.Value(routeParamsKey{}).(gin.Params); ok {
		return params
	}
	if c := GinContext(ctx); c != nil {
		return c.Params
	}
	return nil
}

// UserID returns the authenticated user ID from context or Gin.
func UserID(ctx context.Context) string {
	if uid := permission.UserIDFromContext(ctx); uid != "" {
		return uid
	}
	if c := GinContext(ctx); c != nil {
		return c.GetString("user_id")
	}
	return ""
}

// TenantID returns the active tenant ID from context or Gin.
func TenantID(ctx context.Context) string {
	if tid := permission.TenantIDFromContext(ctx); tid != "" {
		return tid
	}
	if c := GinContext(ctx); c != nil {
		return c.GetString("tenant_id")
	}
	return ""
}

func joinRoutePaths(base, relative string) string {
	if base == "" {
		if !strings.HasPrefix(relative, "/") {
			return "/" + relative
		}
		return relative
	}
	if relative == "" || relative == "/" {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(relative, "/")
}

func bindAndEnforcePathParamsValue(val reflect.Value, p gin.Param) (bool, error) {
	if val.Kind() != reflect.Struct {
		return false, nil
	}
	typ := val.Type()
	paramName := strings.ToLower(p.Key)
	cleanParam := strings.ReplaceAll(paramName, "_", "")

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		if field.Anonymous {
			target := fieldVal
			if field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct {
				if target.IsNil() {
					target.Set(reflect.New(field.Type.Elem()))
				}
				target = target.Elem()
			}
			if target.Kind() == reflect.Struct {
				matched, err := bindAndEnforcePathParamsValue(target, p)
				if err != nil {
					return false, err
				}
				if matched {
					return true, nil
				}
			}
		}

		uriTag := strings.ToLower(field.Tag.Get("uri"))
		jsonTag := strings.ToLower(strings.Split(field.Tag.Get("json"), ",")[0])
		formTag := strings.ToLower(strings.Split(field.Tag.Get("form"), ",")[0])
		fieldName := strings.ToLower(field.Name)
		cleanField := strings.ReplaceAll(fieldName, "_", "")

		if uriTag == paramName || jsonTag == paramName || formTag == paramName ||
			fieldName == paramName || cleanField == cleanParam {
			targetVal := fieldVal
			isPtr := targetVal.Kind() == reflect.Pointer
			if isPtr {
				if targetVal.IsNil() {
					targetVal.Set(reflect.New(targetVal.Type().Elem()))
				}
				targetVal = targetVal.Elem()
			}

			switch targetVal.Kind() {
			case reflect.String:
				existing := targetVal.String()
				if existing != "" && existing != p.Value {
					return false, fmt.Errorf("route parameter mismatch for '%s': URL has '%s' but request body has '%s'", p.Key, p.Value, existing)
				}
				targetVal.SetString(p.Value)
				return true, nil
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				bitSize := 64
				switch targetVal.Kind() {
				case reflect.Int8:
					bitSize = 8
				case reflect.Int16:
					bitSize = 16
				case reflect.Int32:
					bitSize = 32
				case reflect.Int:
					bitSize = strconv.IntSize
				}
				intVal, err := strconv.ParseInt(p.Value, 10, bitSize)
				if err != nil {
					return false, fmt.Errorf("invalid path parameter '%s': integer out of range or invalid format", p.Key)
				}
				existing := targetVal.Int()
				if existing != 0 && existing != intVal {
					return false, fmt.Errorf("route parameter mismatch for '%s': URL has '%d' but request body has '%d'", p.Key, intVal, existing)
				}
				targetVal.SetInt(intVal)
				return true, nil
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				bitSize := 64
				switch targetVal.Kind() {
				case reflect.Uint8:
					bitSize = 8
				case reflect.Uint16:
					bitSize = 16
				case reflect.Uint32:
					bitSize = 32
				case reflect.Uint:
					bitSize = strconv.IntSize
				}
				uintVal, err := strconv.ParseUint(p.Value, 10, bitSize)
				if err != nil {
					return false, fmt.Errorf("invalid path parameter '%s': unsigned integer out of range or invalid format", p.Key)
				}
				existing := targetVal.Uint()
				if existing != 0 && existing != uintVal {
					return false, fmt.Errorf("route parameter mismatch for '%s': URL has '%d' but request body has '%d'", p.Key, uintVal, existing)
				}
				targetVal.SetUint(uintVal)
				return true, nil
			}
		}
	}
	return false, nil
}

func bindAndEnforcePathParams(target any, params gin.Params) error {
	if len(params) == 0 || target == nil {
		return nil
	}
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return nil
	}
	val = val.Elem()
	for _, p := range params {
		if _, err := bindAndEnforcePathParamsValue(val, p); err != nil {
			return err
		}
	}
	return nil
}

// TypedHandler receives a strongly-typed request struct and returns a response struct or error.
type TypedHandler[Req any, Resp any] func(ctx context.Context, req *Req) (Resp, error)

// Validator is an optional interface that request structs can implement for validation.
type Validator interface {
	Validate() error
}

// TypedRouteOption configures a typed route.
type TypedRouteOption func(*TypedRouteConfig)

// TypedRouteConfig holds metadata options for a typed route.
type TypedRouteConfig struct {
	AuthRequired       *bool
	Tags               []string
	Description        string
	Deprecated         bool
	Middlewares        []gin.HandlerFunc
	MaxBodyBytes       int64
	ResponseDataSchema *openapi.Schema
}

// WithAuth explicitly specifies whether authentication is required for this route.
func WithAuth(required bool) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.AuthRequired = &required
	}
}

// WithTags sets custom OpenAPI tags for the route.
func WithTags(tags ...string) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.Tags = tags
	}
}

// WithDescription sets the description for the route.
func WithDescription(desc string) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.Description = desc
	}
}

// WithMiddleware attaches route-specific Gin middleware to run before binding and handler execution.
func WithMiddleware(handlers ...gin.HandlerFunc) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.Middlewares = append(c.Middlewares, handlers...)
	}
}

// WithMaxBodyBytes sets the maximum allowed request body size in bytes.
// Enforced via http.MaxBytesReader and ContentLength validation.
func WithMaxBodyBytes(limit int64) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.MaxBodyBytes = limit
	}
}

// WithResponseDataSchema overrides the default OpenAPI schema for the 200 response envelope's data property.
func WithResponseDataSchema(schema *openapi.Schema) TypedRouteOption {
	return func(c *TypedRouteConfig) {
		c.ResponseDataSchema = schema
	}
}

func hasAuthMiddleware(target gin.IRoutes) bool {
	if rg, ok := target.(*gin.RouterGroup); ok {
		val := reflect.ValueOf(rg)
		if val.Kind() == reflect.Pointer && !val.IsNil() {
			val = val.Elem()
			engineField := val.FieldByName("engine")
			if engineField.IsValid() && !engineField.IsNil() {
				engineVal := engineField.Elem()
				engineHandlers := engineVal.FieldByName("Handlers")
				if engineHandlers.IsValid() {
					engineHandlerCount := engineHandlers.Len()
					if len(rg.Handlers) > engineHandlerCount {
						return true
					}
				}
			} else {
				if len(rg.Handlers) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func hasBindingRequired(bindingTag string) bool {
	if bindingTag == "" {
		return false
	}
	for _, part := range strings.Split(bindingTag, ",") {
		if strings.TrimSpace(part) == "required" {
			return true
		}
	}
	return false
}

func bindQueryPrimitive(targetVal reflect.Value, raw string) error {
	isPtr := targetVal.Kind() == reflect.Pointer
	destVal := targetVal
	if isPtr {
		destVal = reflect.New(targetVal.Type().Elem()).Elem()
	}
	switch destVal.Kind() {
	case reflect.String:
		destVal.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bitSize := 64
		switch destVal.Kind() {
		case reflect.Int8:
			bitSize = 8
		case reflect.Int16:
			bitSize = 16
		case reflect.Int32:
			bitSize = 32
		case reflect.Int:
			bitSize = strconv.IntSize
		}
		intVal, err := strconv.ParseInt(raw, 10, bitSize)
		if err != nil {
			return err
		}
		destVal.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bitSize := 64
		switch destVal.Kind() {
		case reflect.Uint8:
			bitSize = 8
		case reflect.Uint16:
			bitSize = 16
		case reflect.Uint32:
			bitSize = 32
		case reflect.Uint:
			bitSize = strconv.IntSize
		}
		uintVal, err := strconv.ParseUint(raw, 10, bitSize)
		if err != nil {
			return err
		}
		destVal.SetUint(uintVal)
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		destVal.SetBool(boolVal)
	case reflect.Float32, reflect.Float64:
		bitSize := 64
		if destVal.Kind() == reflect.Float32 {
			bitSize = 32
		}
		floatVal, err := strconv.ParseFloat(raw, bitSize)
		if err != nil {
			return err
		}
		destVal.SetFloat(floatVal)
	default:
		return nil
	}

	if isPtr {
		if targetVal.IsNil() {
			ptrVal := reflect.New(targetVal.Type().Elem())
			ptrVal.Elem().Set(destVal)
			targetVal.Set(ptrVal)
		} else {
			targetVal.Elem().Set(destVal)
		}
	}
	return nil
}

// RegisterTypedRoute registers a type-safe API endpoint onto a Gin engine or RouterGroup
// and automatically documents its request/response schemas in the OpenAPI registry.
func RegisterTypedRoute[Req any, Resp any](
	r *Router,
	target gin.IRoutes,
	method string,
	path string,
	summary string,
	handler TypedHandler[Req, Resp],
	opts ...TypedRouteOption,
) {
	var cfg TypedRouteConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	fullPath := path
	if rg, ok := target.(*gin.RouterGroup); ok {
		fullPath = joinRoutePaths(rg.BasePath(), path)
	}

	// Register operation and schemas with OpenAPI registry
	if r != nil {
		if r.openapiHandler == nil {
			r.openapiHandler = openapi.NewHandler(openapi.NewRegistry())
		}
		reqType := reflect.TypeOf((*Req)(nil)).Elem()
		respType := reflect.TypeOf((*Resp)(nil)).Elem()

		components := make(map[string]*openapi.Schema)
		respDataSchema := cfg.ResponseDataSchema
		if respDataSchema == nil {
			respDataSchema = openapi.BuildSchemaWithComponents(respType, components)
		}

		envelopeSchema := &openapi.Schema{
			Type: "object",
			Properties: map[string]*openapi.Schema{
				"code": {
					Type:        "integer",
					Description: "Application error code (0 = success)",
				},
				"message": {
					Type:        "string",
					Description: "Response status message",
				},
				"data": respDataSchema,
			},
			Required: []string{"code", "message"},
		}

		var authRequired bool
		if cfg.AuthRequired != nil {
			authRequired = *cfg.AuthRequired
		} else {
			authRequired = hasAuthMiddleware(target)
		}

		var sec []map[string][]string
		if authRequired {
			sec = []map[string][]string{{"ApiKeyAuth": {}}}
		} else if cfg.AuthRequired != nil && !*cfg.AuthRequired {
			// Explicitly public via WithAuth(false)
			sec = []map[string][]string{}
		} else {
			// Undeclared with no auth middleware: do not mark explicitly as public
			sec = nil
		}

		tags := cfg.Tags
		if len(tags) == 0 {
			tags = []string{openapi.DeriveTag(fullPath)}
		}

		responses := map[string]*openapi.Response{
			"200": {
				Description: "Standard RAGFlow response envelope (code=0 on success)",
				Content: map[string]*openapi.MediaType{
					"application/json": {Schema: envelopeSchema},
				},
			},
			"400": {
				Description: "Bad Request (parameter binding or validation failure)",
				Content: map[string]*openapi.MediaType{
					"application/json": {
						Schema: &openapi.Schema{
							Type: "object",
							Properties: map[string]*openapi.Schema{
								"code":    {Type: "integer", Description: "Error code (100 = Bad Request)"},
								"message": {Type: "string", Description: "Validation or binding error message"},
							},
							Required: []string{"code", "message"},
						},
					},
				},
			},
			"500": {
				Description: "Internal Server Error",
				Content: map[string]*openapi.MediaType{
					"application/json": {
						Schema: &openapi.Schema{
							Type: "object",
							Properties: map[string]*openapi.Schema{
								"code":    {Type: "integer", Description: "Error code (500 = Server Error)"},
								"message": {Type: "string", Description: "Internal server error message"},
							},
							Required: []string{"code", "message"},
						},
					},
				},
			},
		}
		if authRequired {
			responses["401"] = &openapi.Response{
				Description: "Unauthorized (missing or invalid API token)",
				Content: map[string]*openapi.MediaType{
					"application/json": {
						Schema: &openapi.Schema{
							Type: "object",
							Properties: map[string]*openapi.Schema{
								"code":    {Type: "integer", Description: "Error code (401 = Unauthorized)"},
								"message": {Type: "string", Description: "Unauthorized error message"},
							},
							Required: []string{"code", "message"},
						},
					},
				},
			}
		}

		op := &openapi.Operation{
			Tags:        tags,
			Summary:     summary,
			Description: cfg.Description,
			OperationID: openapi.GenerateOperationID(method, fullPath),
			Security:    sec,
			Deprecated:  cfg.Deprecated,
			Responses:   responses,
		}

		pathParams := openapi.ExtractPathParameters(fullPath)
		pathParamSet := make(map[string]bool)
		for _, p := range pathParams {
			pathParamSet[strings.ToLower(p)] = true
		}

		actualReqType := reqType
		if actualReqType.Kind() == reflect.Pointer {
			actualReqType = actualReqType.Elem()
		}

		documentedPathParams := make(map[string]bool)

		var collectDocParams func(t reflect.Type)
		collectDocParams = func(t reflect.Type) {
			if t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			if t.Kind() != reflect.Struct {
				return
			}
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				if !field.IsExported() {
					continue
				}
				if field.Anonymous {
					collectDocParams(field.Type)
					continue
				}

				uriTag := strings.ToLower(field.Tag.Get("uri"))
				formTag := strings.Split(field.Tag.Get("form"), ",")[0]
				jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
				doc := field.Tag.Get("doc")
				if doc == "" {
					doc = field.Tag.Get("desc")
				}
				bindingTag := field.Tag.Get("binding")
				isRequired := field.Tag.Get("required") == "true" || hasBindingRequired(bindingTag)

				fieldNameLower := strings.ToLower(field.Name)
				cleanField := strings.ReplaceAll(fieldNameLower, "_", "")

				// Check if this field corresponds to a path parameter
				isPathParam := false
				matchedPathParam := ""
				for p := range pathParamSet {
					cleanP := strings.ReplaceAll(p, "_", "")
					if uriTag == p || fieldNameLower == p || strings.ToLower(jsonTag) == p ||
						strings.ToLower(formTag) == p || cleanField == cleanP {
						isPathParam = true
						matchedPathParam = p
						break
					}
				}

				if isPathParam {
					op.Parameters = append(op.Parameters, &openapi.Parameter{
						Name:        matchedPathParam,
						In:          "path",
						Required:    true,
						Description: doc,
						Schema:      openapi.BuildSchemaWithComponents(field.Type, components),
					})
					documentedPathParams[matchedPathParam] = true
					continue
				}

				// Check if this field is a query parameter
				isQueryParam := false
				if field.Type != fileHeaderType && field.Type != fileHeaderSliceType {
					if method == http.MethodGet || method == http.MethodDelete {
						if formTag != "-" && jsonTag != "-" {
							isQueryParam = true
						}
					} else if field.Tag.Get("query") != "" {
						isQueryParam = true
					}
				}

				if isQueryParam {
					queryParamName := resolveQueryParamName(field)
					op.Parameters = append(op.Parameters, &openapi.Parameter{
						Name:        queryParamName,
						In:          "query",
						Required:    isRequired,
						Description: doc,
						Schema:      openapi.BuildSchemaWithComponents(field.Type, components),
					})
				}
			}
		}

		if actualReqType.Kind() == reflect.Struct {
			collectDocParams(actualReqType)
		}

		// Ensure all path parameters from the URL are documented even if not in struct
		for _, param := range pathParams {
			if !documentedPathParams[strings.ToLower(param)] {
				op.Parameters = append(op.Parameters, &openapi.Parameter{
					Name:        param,
					In:          "path",
					Required:    true,
					Description: param + " parameter",
					Schema:      &openapi.Schema{Type: "string"},
				})
			}
		}

		// Request body for POST / PUT / PATCH
		if method != http.MethodGet && method != http.MethodDelete && actualReqType.Kind() == reflect.Struct && actualReqType.NumField() > 0 {
			if isMultipartType(actualReqType) {
				props := make(map[string]*openapi.Schema)
				var required []string
				for i := 0; i < actualReqType.NumField(); i++ {
					field := actualReqType.Field(i)
					if !field.IsExported() {
						continue
					}
					fieldNameLower := strings.ToLower(field.Name)
					uriTag := strings.ToLower(field.Tag.Get("uri"))
					cleanField := strings.ReplaceAll(fieldNameLower, "_", "")
					isPath := false
					for p := range pathParamSet {
						cleanP := strings.ReplaceAll(p, "_", "")
						if uriTag == p || fieldNameLower == p || cleanField == cleanP {
							isPath = true
							break
						}
					}
					if isPath {
						continue
					}

					formTag := strings.Split(field.Tag.Get("form"), ",")[0]
					queryTag := strings.Split(field.Tag.Get("query"), ",")[0]
					if queryTag != "" || formTag == "-" {
						continue
					}
					targetKey := formTag
					if targetKey == "" || targetKey == "-" {
						targetKey = strings.Split(field.Tag.Get("json"), ",")[0]
					}
					if targetKey == "" || targetKey == "-" {
						targetKey = strings.ToLower(field.Name)
					}

					doc := field.Tag.Get("doc")
					if doc == "" {
						doc = field.Tag.Get("desc")
					}

					isRequired := field.Tag.Get("required") == "true" || hasBindingRequired(field.Tag.Get("binding"))
					if isRequired {
						required = append(required, targetKey)
					}

					if field.Type == fileHeaderType {
						props[targetKey] = &openapi.Schema{
							Type:        "string",
							Format:      "binary",
							Description: doc,
						}
					} else if field.Type == fileHeaderSliceType {
						props[targetKey] = &openapi.Schema{
							Type:  "array",
							Items: &openapi.Schema{Type: "string", Format: "binary"},
							Description: doc,
						}
					} else {
						s := openapi.BuildSchemaWithComponents(field.Type, components)
						if doc != "" && s.Ref == "" {
							s.Description = doc
						}
						props[targetKey] = s
					}
				}

				op.RequestBody = &openapi.RequestBody{
					Required: true,
					Content: map[string]*openapi.MediaType{
						"multipart/form-data": {
							Schema: &openapi.Schema{
								Type:       "object",
								Properties: props,
								Required:   required,
							},
						},
					},
				}
			} else {
				reqSchema := openapi.BuildSchemaWithComponents(actualReqType, components)
				if len(reqSchema.Properties) > 0 {
					op.RequestBody = &openapi.RequestBody{
						Required: true,
						Content: map[string]*openapi.MediaType{
							"application/json": {Schema: reqSchema},
						},
					}
				}
			}
		}

		r.openapiHandler.Registry().RegisterOperation(method, fullPath, op)
		for name, compSchema := range components {
			r.openapiHandler.Registry().RegisterComponentSchema(name, compSchema)
		}
	}

	// Register Gin HTTP handler with automatic binding and validation
	ginHandler := func(c *gin.Context) {
		var req Req

		// 1. Enforce body size limit if configured (default 64 MiB for multipart to avoid unbounded disk/memory exhaustion)
		limit := cfg.MaxBodyBytes
		contentType := c.GetHeader("Content-Type")
		isMultipart := strings.Contains(contentType, "multipart/form-data") || isMultipartType(reflect.TypeOf((*Req)(nil)).Elem())
		if limit <= 0 && isMultipart {
			limit = 64 << 20
		}
		if limit > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
			if cl := c.Request.ContentLength; cl > limit {
				common.RespondError(c, common.CodeArgumentError, "request body too large.")
				return
			}
		}

		// 2. Check if Content-Type is multipart/form-data
		if isMultipart {
			const multipartMaxMemory = 8 << 20 // 8 MiB in-memory buffer; larger parts spill to temp files
			maxMem := int64(multipartMaxMemory)
			if limit > 0 && limit < maxMem {
				maxMem = limit
			}
			if err := c.Request.ParseMultipartForm(maxMem); err != nil {
				common.RespondError(c, common.CodeArgumentError, "invalid multipart form: "+err.Error())
				return
			}
			defer func() {
				if c.Request.MultipartForm != nil {
					_ = c.Request.MultipartForm.RemoveAll()
				}
			}()
			form := c.Request.MultipartForm
			if form != nil {
				reqVal := reflect.ValueOf(&req).Elem()
				if reqVal.Kind() == reflect.Struct {
					reqType := reqVal.Type()
					for i := 0; i < reqType.NumField(); i++ {
						f := reqType.Field(i)
						fieldVal := reqVal.Field(i)
						if !fieldVal.CanSet() {
							continue
						}
						rawFormTag := f.Tag.Get("form")
						if rawFormTag == "-" {
							continue
						}
						targetKey := strings.Split(rawFormTag, ",")[0]
						if targetKey == "" {
							targetKey = f.Name
						}
						if f.Type == fileHeaderType {
							if files := form.File[targetKey]; len(files) > 0 {
								fieldVal.Set(reflect.ValueOf(files[0]))
							}
						} else if f.Type == fileHeaderSliceType {
							if files := form.File[targetKey]; len(files) > 0 {
								fieldVal.Set(reflect.ValueOf(files))
							}
						}
					}
				}
				if err := binding.MapFormWithTag(&req, form.Value, "form"); err != nil {
					common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
					return
				}
			}
		}

		// 3. Bind query parameters with unified naming resolution
		if len(c.Request.URL.RawQuery) > 0 {
			queryVals := c.Request.URL.Query()
			formVals := make(url.Values)
			reqVal := reflect.ValueOf(&req).Elem()
			isMutating := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodDelete
			var collectFormVals func(t reflect.Type)
			collectFormVals = func(t reflect.Type) {
				if t.Kind() == reflect.Pointer {
					t = t.Elem()
				}
				if t.Kind() != reflect.Struct {
					return
				}
				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)
					if !f.IsExported() {
						continue
					}
					if f.Anonymous {
						collectFormVals(f.Type)
						continue
					}
					qTag := f.Tag.Get("query")
					// Skip fields explicitly excluded from query binding
					if qTag == "-" {
						continue
					}
					// On mutating endpoints (POST, PUT, PATCH), ONLY bind fields that explicitly declare query or form tag.
					// On GET/DELETE endpoints, also skip fields tagged json:"-" when they lack explicit query or form tag.
					hasQueryOrForm := (qTag != "" && qTag != "-") || (f.Tag.Get("form") != "" && f.Tag.Get("form") != "-")
					if (isMutating || strings.Split(f.Tag.Get("json"), ",")[0] == "-") && !hasQueryOrForm {
						continue
					}
					qName := resolveQueryParamName(f)
					targetKey := qTag
					if targetKey == "" || targetKey == "-" {
						targetKey = strings.Split(f.Tag.Get("form"), ",")[0]
					}
					if targetKey == "" || targetKey == "-" {
						targetKey = f.Name
					}
					var foundVals []string
					if vals, ok := queryVals[qName]; ok && len(vals) > 0 {
						foundVals = vals
					} else if vals, ok := queryVals[f.Name]; ok && len(vals) > 0 {
						foundVals = vals
					} else if vals, ok := queryVals[targetKey]; ok && len(vals) > 0 {
						foundVals = vals
					}
					if len(foundVals) > 0 {
						formVals[targetKey] = foundVals
						if f.Name != targetKey {
							formVals[f.Name] = foundVals
						}
					}
				}
			}

			if reqVal.Kind() == reflect.Struct {
				collectFormVals(reqVal.Type())
			}
			if len(formVals) > 0 {
				if err := binding.MapFormWithTag(&req, formVals, "form"); err != nil {
					common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
					return
				}
			}

			var bindPrimitives func(v reflect.Value) error
			bindPrimitives = func(v reflect.Value) error {
				if v.Kind() == reflect.Pointer {
					if v.IsNil() && v.CanSet() {
						v.Set(reflect.New(v.Type().Elem()))
					}
					v = v.Elem()
				}
				if v.Kind() != reflect.Struct {
					return nil
				}
				t := v.Type()
				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)
					if !f.IsExported() {
						continue
					}
					fieldVal := v.Field(i)
					if f.Anonymous {
						if fieldVal.Kind() == reflect.Pointer {
							if fieldVal.IsNil() && fieldVal.CanSet() {
								fieldVal.Set(reflect.New(fieldVal.Type().Elem()))
							}
						}
						if err := bindPrimitives(fieldVal); err != nil {
							return err
						}
						continue
					}
					qTag := strings.Split(f.Tag.Get("query"), ",")[0]
					if qTag != "" && qTag != "-" {
						if vals := queryVals[qTag]; len(vals) > 0 {
							if fieldVal.CanSet() {
								if err := bindQueryPrimitive(fieldVal, vals[0]); err != nil {
									return fmt.Errorf("invalid query parameter %q: %w", qTag, err)
								}
							}
						}
					}
				}
				return nil
			}

			if reqVal.Kind() == reflect.Struct {
				if err := bindPrimitives(reqVal); err != nil {
					common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
					return
				}
			}
		}

		// 4. Bind JSON body (if not multipart; supports ContentLength > 0 and chunked transfer ContentLength == -1)
		if !isMultipart {
			hasBody := c.Request.Body != nil && (c.Request.ContentLength > 0 || c.Request.ContentLength == -1)
			if hasBody {
				isJSON := strings.Contains(contentType, "application/json") ||
					(contentType == "" && (c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch))
				if isJSON {
					bodyBytes, err := io.ReadAll(c.Request.Body)
					if err != nil {
						common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
						return
					}
					c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					trimmed := bytes.TrimSpace(bodyBytes)
					if len(trimmed) > 0 {
						if err := json.Unmarshal(trimmed, &req); err != nil {
							common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
							return
						}
					}
				}
			}
		}

		// 5. Bind route/URI path parameters and strictly enforce path parameter precedence
		if len(c.Params) > 0 {
			if err := bindAndEnforcePathParams(&req, c.Params); err != nil {
				common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
				return
			}
		}

		// 6. Validate complete request struct using Gin validator
		if binding.Validator != nil {
			if err := binding.Validator.ValidateStruct(&req); err != nil {
				common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
				return
			}
		}

		// 7. Custom Validator interface if implemented
		if v, ok := any(&req).(Validator); ok {
			if err := v.Validate(); err != nil {
				common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
				return
			}
		}

		// 8. Enrich request context with Gin context and route parameters
		ctx := c.Request.Context()
		ctx = WithGinContext(ctx, c)
		if len(c.Params) > 0 {
			ctx = WithRouteParams(ctx, c.Params)
		}

		resp, err := handler(ctx, &req)
		if err != nil {
			var codedErr *common.CodedError
			if errors.As(err, &codedErr) {
				common.RespondError(c, codedErr.Code, codedErr.Message)
				return
			}
			common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
			return
		}
		common.RespondSuccess(c, resp, "")
	}

	var handlers []gin.HandlerFunc
	if len(cfg.Middlewares) > 0 {
		handlers = append(handlers, cfg.Middlewares...)
	}
	handlers = append(handlers, ginHandler)
	target.Handle(method, path, handlers...)
	MarkTypedRoute(method, fullPath)
}

func resolveQueryParamName(field reflect.StructField) string {
	queryTag := strings.Split(field.Tag.Get("query"), ",")[0]
	if queryTag != "" && queryTag != "-" {
		return queryTag
	}
	formTag := strings.Split(field.Tag.Get("form"), ",")[0]
	if formTag != "" && formTag != "-" {
		return formTag
	}
	jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
	if jsonTag != "" && jsonTag != "-" {
		return jsonTag
	}
	return strings.ToLower(field.Name)
}

