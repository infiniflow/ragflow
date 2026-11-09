//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

func TestReportsPreservesHeartbeatResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousStore := GlobalServerStore
	GlobalServerStore = &ServerStore{servers: make(map[string]*common.BaseMessage)}
	t.Cleanup(func() { GlobalServerStore = previousStore })

	for _, tt := range []struct {
		name    string
		body    string
		status  int
		code    common.ErrorCode
		message string
		data    map[string]any
	}{
		{
			name:    "valid heartbeat retains nonzero code and data",
			body:    `{"report_type":"heartbeat","server_id":"reports-test","server_type":"api"}`,
			status:  http.StatusOK,
			code:    common.CodeLicenseValid,
			message: "License valid",
			data:    map[string]any{},
		},
		{
			name:    "service error retains code and message",
			body:    `{"report_type":"heartbeat","server_id":"reports-test","server_type":"invalid"}`,
			status:  http.StatusOK,
			code:    common.CodeErrorServerStatus,
			message: "unknown server type: invalid",
		},
		{
			name:    "unsupported report remains an HTTP error",
			body:    `{"report_type":"metric"}`,
			status:  http.StatusBadRequest,
			code:    common.CodeBadRequest,
			message: "Unsupported report type: metric",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/reports", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			(&Handler{service: &Service{}}).Reports(ctx)

			var response struct {
				Code    common.ErrorCode `json:"code"`
				Message string           `json:"message"`
				Data    map[string]any   `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if recorder.Code != tt.status || response.Code != tt.code || response.Message != tt.message || !reflect.DeepEqual(response.Data, tt.data) {
				t.Fatalf("status = %d, response = %s; want status %d, code %d, message %q, data %#v", recorder.Code, recorder.Body.String(), tt.status, tt.code, tt.message, tt.data)
			}
		})
	}
}
