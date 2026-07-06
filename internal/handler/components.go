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

// Phase 4 of plan port-rag-flow-pipeline-to-go.md.
//
// Exposes GET /api/v1/components?category=ingestion,agent,shared
// (case-insensitive, comma-separated). The data source is
// runtime.DefaultRegistry — there is no separate catalog map (per
// plan §4 task 2). Category values are validated against the three
// known runtime.Category constants; an unknown value yields a
// 400-style error envelope so the frontend can surface a useful
// message instead of silently dropping the request.
package handler

import (
	"net/http"
	"ragflow/internal/common"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/service"
)

// ComponentsHandler serves the component-catalog endpoint.
type ComponentsHandler struct {
	svc *service.ComponentsService
}

// NewComponentsHandler wires the handler to a ComponentsService
// instance. The service is stateless so the same pointer can be
// shared across handlers; construction happens once at server
// startup (cmd/server_main.go).
func NewComponentsHandler(svc *service.ComponentsService) *ComponentsHandler {
	return &ComponentsHandler{svc: svc}
}

// Get handles GET /api/v1/components.
//
// Query parameters:
//   - category (optional, repeatable as comma-separated values).
//     Case-insensitive. Allowed values: "agent", "ingestion",
//     "shared". An empty / missing filter means "all categories".
//
// Response shape (success):
//
//	{ "data": [ { "name": "...", "category": "...",
//	              "inputs": {...}, "outputs": {...} } ] }
//
// On an unknown category, the response uses the standard error
// envelope (gin.H{code, message, data}) with HTTP 400. Service
// failures bubble up as HTTP 500 with the same envelope shape.
func (h *ComponentsHandler) Get(c *gin.Context) {
	raw := c.Query("category")
	cats, err := parseCategories(raw)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	out, err := h.svc.List(cats...)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, out, "success")
}

// parseCategories splits a comma-separated category query string into
// a slice of runtime.Category values. The slice is empty when the
// raw string is empty (meaning "all categories"). An unknown token
// yields an error; the first invalid token wins so the message
// identifies the offender.
func parseCategories(raw string) ([]runtime.Category, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []runtime.Category
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		switch p {
		case "agent":
			out = append(out, runtime.CategoryAgent)
		case "ingestion":
			out = append(out, runtime.CategoryIngestion)
		case "shared":
			out = append(out, runtime.CategoryShared)
		default:
			return nil, &categoryError{value: p}
		}
	}
	return out, nil
}

// categoryError is the error type returned by parseCategories for an
// unrecognized category token. Its Error() message matches the
// expected plan test contract ("unknown category: <value>").
type categoryError struct {
	value string
}

func (e *categoryError) Error() string {
	return "unknown category: " + e.value
}
