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

//go:build !ee

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminPasswordChangeRouteIsEnterpriseOnly(t *testing.T) {
	engine := gin.New()
	NewRouter(NewHandler(NewService())).Setup(engine)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/api/v1/admin/me/password", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("community password-change endpoint returned HTTP %d, want 404", res.Code)
	}
	if err := checkAdminPasswordChangeTarget("same-user", "same-user"); err != nil {
		t.Fatalf("community password management was restricted: %v", err)
	}
}
