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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

// TestBetaAuthMiddleware_MissingHeader pins the no-header branch —
// the middleware must short-circuit with 401/CodeUnauthorized and
// must not call into UserService. The other branches (regular JWT
// and beta token) require a live DB to resolve, so they are covered
// by the cross-cutting TestBotRoutes_RequireAuth criterion in
// bot_test.go.
func TestBetaAuthMiddleware_MissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ah := &AuthHandler{userService: nil}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	mw := ah.BetaAuthMiddleware()
	mw(c)

	if !c.IsAborted() {
		t.Fatalf("context not aborted, want aborted (no Authorization header)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Beta endpoints follow Python's login_required(auth_types=AUTH_BETA):
	// auth failures are business errors (code 102), not HTTP 401.
	var resp struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v; body = %s", err, rec.Body.String())
	}
	if resp.Code != common.CodeDataError {
		t.Errorf("code = %d, want %d; body = %s",
			resp.Code, common.CodeDataError, rec.Body.String())
	}
	if resp.Message != "Authorization is not valid!" {
		t.Errorf("message = %q; body = %s", resp.Message, rec.Body.String())
	}
}
