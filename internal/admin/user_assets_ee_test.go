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

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserAssetRoutesAreUnavailable(t *testing.T) {
	engine := gin.New()
	NewRouter(&Handler{}).Setup(engine)

	for _, resource := range []string{"agents", "datasets"} {
		t.Run(resource, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/user%40example.com/"+resource, nil)
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatalf("HTTP status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
			}
		})
	}
}
