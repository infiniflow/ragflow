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
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/openapi"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	contractedRoutesMu sync.RWMutex
	contractedRoutes   = make(map[string]bool)
)

// MarkContractedRoute marks a route as having a verified contract or typed specification.
func MarkContractedRoute(method, path string) {
	contractedRoutesMu.Lock()
	defer contractedRoutesMu.Unlock()
	contractedRoutes[method+" "+path] = true
}

// MarkTypedRoute marks a route as registered conforming to the typed specification.
func MarkTypedRoute(method, path string) {
	MarkContractedRoute(method, path)
}

// IsContractedRoute returns whether a route has a verified contract or typed specification.
func IsContractedRoute(method, path string) bool {
	contractedRoutesMu.RLock()
	defer contractedRoutesMu.RUnlock()
	return contractedRoutes[method+" "+path]
}

// IsTypedRoute returns whether a route was registered via the typed specification.
func IsTypedRoute(method, path string) bool {
	return IsContractedRoute(method, path)
}

// ClearContractedRoutes resets contracted routes (primarily used in tests).
func ClearContractedRoutes() {
	contractedRoutesMu.Lock()
	defer contractedRoutesMu.Unlock()
	contractedRoutes = make(map[string]bool)
}

// ClearTypedRoutes resets typed routes (primarily used in tests).
func ClearTypedRoutes() {
	ClearContractedRoutes()
}

// ValidateContract verifies that an OpenAPI contract is accurate and not a placeholder.
func ValidateContract(c openapi.Contract) error {
	if strings.TrimSpace(c.Summary) == "" {
		return errors.New("contract summary must not be empty")
	}
	if len(c.Responses) == 0 {
		return errors.New("contract responses must not be empty")
	}
	hasSuccess := false
	for code, resp := range c.Responses {
		if resp == nil {
			return fmt.Errorf("contract response for %q must not be nil", code)
		}
		if strings.TrimSpace(resp.Description) == "" {
			return fmt.Errorf("contract response for %q must define a description", code)
		}
		if code != "default" {
			if len(code) != 3 {
				return fmt.Errorf("invalid OpenAPI response status code %q", code)
			}
			status, err := strconv.Atoi(code)
			if err != nil || status < 100 || status > 599 {
				return fmt.Errorf("invalid OpenAPI response status code %q", code)
			}
			if status >= 200 && status < 300 {
				hasSuccess = true
			}
		}
	}
	if !hasSuccess {
		return errors.New("contract must define at least one successful (2xx) response")
	}
	return nil
}

// MaxAllowedUntypedRoutes is the fixed ceiling for uncontracted legacy routes.
// Any newly added route must be registered via RegisterTypedRoute or
// RegisterContract with a verified contract, or this ratchet will fail CI.
const MaxAllowedUntypedRoutes = 299

// RouteMigrationStats records route migration progress towards typed/contracted APIs.
type RouteMigrationStats struct {
	TotalRoutes      int
	ContractedRoutes int
	UntypedRoutes    int
	UntypedList      []string
}

// GetRouteMigrationStats inspects all registered engine routes and computes migration progress.
func GetRouteMigrationStats(engine *gin.Engine) RouteMigrationStats {
	var stats RouteMigrationStats
	for _, route := range engine.Routes() {
		if isExemptRoute(route.Method, route.Path) {
			continue
		}
		stats.TotalRoutes++
		if IsContractedRoute(route.Method, route.Path) {
			stats.ContractedRoutes++
		} else {
			stats.UntypedRoutes++
			stats.UntypedList = append(stats.UntypedList, route.Method+" "+route.Path)
		}
	}
	sort.Strings(stats.UntypedList)
	return stats
}

// CountUntypedRoutes counts routes on the engine that lack a verified contract.
func CountUntypedRoutes(engine *gin.Engine) int {
	return GetRouteMigrationStats(engine).UntypedRoutes
}

// ValidateRouteBudget checks that untyped routes conform to the approved legacy baseline allowlist
// and do not exceed the allowed legacy budget.
// If allowedLegacyBudget <= 0, MaxAllowedUntypedRoutes is used as the default ceiling.
// When an unapproved raw Gin route is added (even if an existing legacy route was deleted),
// it returns an educational error explaining how to conform to the new API specification.
func ValidateRouteBudget(engine *gin.Engine, allowedLegacyBudget int) error {
	if allowedLegacyBudget <= 0 {
		allowedLegacyBudget = MaxAllowedUntypedRoutes
	}
	stats := GetRouteMigrationStats(engine)

	// 1. Strict allowlist check: no new untyped routes are permitted outside the legacy baseline.
	var unapprovedNewRoutes []string
	for _, route := range stats.UntypedList {
		if !legacyUntypedRoutesBaseline[route] {
			unapprovedNewRoutes = append(unapprovedNewRoutes, route)
		}
	}
	if len(unapprovedNewRoutes) > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf(
			"❌ API Specification Guard: Found %d unapproved new untyped route(s) outside legacy baseline!\n"+
				"Unapproved routes:\n",
			len(unapprovedNewRoutes),
		))
		for _, r := range unapprovedNewRoutes {
			b.WriteString(fmt.Sprintf("  - %s\n", r))
		}
		b.WriteString("\nGuidance for new API development:\n" +
			"  1. Do NOT register raw Gin handlers without a contract.\n" +
			"  2. Use router.RegisterTypedRoute[Req, Resp] with strongly typed structs,\n" +
			"     OR register an explicit contract via r.RegisterContract(method, path, contract).\n" +
			"  3. Struct fields must have tags (`uri`, `form`, `query`, `json`, `doc`) to guarantee\n" +
			"     automatic OpenAPI/Swagger documentation and parameter validation.\n")
		return fmt.Errorf("%s", b.String())
	}

	// 2. Count budget check: total untyped routes cannot exceed the ceiling.
	if stats.UntypedRoutes > allowedLegacyBudget {
		var b strings.Builder
		b.WriteString(fmt.Sprintf(
			"❌ API Specification Guard: Untyped route budget exceeded!\n"+
				"Allowed legacy routes: %d, current untyped routes: %d (excess: %d).\n\n"+
				"Guidance for new API development:\n"+
				"  1. Do NOT register raw Gin handlers without a contract.\n"+
				"  2. Use router.RegisterTypedRoute[Req, Resp] with strongly typed structs,\n"+
				"     OR register an explicit contract via r.RegisterContract(method, path, contract).\n"+
				"  3. Struct fields must have tags (`uri`, `form`, `query`, `json`, `doc`) to guarantee\n"+
				"     automatic OpenAPI/Swagger documentation and parameter validation.\n",
			allowedLegacyBudget, stats.UntypedRoutes, stats.UntypedRoutes-allowedLegacyBudget,
		))
		return fmt.Errorf("%s", b.String())
	}
	return nil
}

func isExemptRoute(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/health",
		"/openapi.json",
		"/api/v1/openapi.json",
		"/docs",
		"/docs/",
		"/api/v1/docs",
		"/api/v1/docs/":
		return true
	default:
		return false
	}
}
