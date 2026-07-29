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
	// jsonError writes 200 with a CodeUnauthorized body. Confirm the
	// body shape matches the wire contract used by the rest of the
	// bot handlers by decoding the JSON envelope and asserting the
	// code field rather than just checking for a non-empty body.
	var resp struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v; body = %s", err, rec.Body.String())
	}
	if resp.Code != common.CodeUnauthorized {
		t.Errorf("code = %d, want %d; body = %s",
			resp.Code, common.CodeUnauthorized, rec.Body.String())
	}
}
