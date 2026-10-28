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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// singleJSONResponse decodes the recorder body as one JSON object. A handler
// that writes an error response and then falls through to write a success
// response produces TWO concatenated JSON objects, which json.Unmarshal
// rejects (after the first object) as "invalid character '{' after top-level
// value". So a clean decode proves the handler returned after the error.
func singleJSONResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var first map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("response is not valid JSON: %v; body=%q", err, body)
	}
	// A second dec.Decode returning io.EOF means exactly one JSON object was
	// written. Any other result means the body has a second concatenated object
	// (the double-response bug).
	var second map[string]any
	if err := dec.Decode(&second); err == nil {
		t.Fatalf("response has a second concatenated JSON object (double response): %q", body)
	}
	return first
}

// TestSetRoleDefaultModelReturnsAfterBindError pins the fix for the missing
// `return` after ShouldBindJSON fails: a malformed body must yield exactly one
// 400 response, not a 400 followed by a 200 success (the SetRoleDefaultModel
// service is a stub returning nil, so the old fall-through wrote a second
// success object on top of the 400).
func TestSetRoleDefaultModelReturnsAfterBindError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{not valid json"))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = append(ctx.Params, gin.Param{Key: "role_name", Value: "r1"})

	(&Handler{service: NewService()}).SetRoleDefaultModel(ctx)

	resp := singleJSONResponse(t, recorder.Body.Bytes())
	if code, ok := resp["code"].(float64); !ok || int(code) == 0 {
		t.Fatalf("response code = %v, want a non-zero error code; body=%q", resp["code"], recorder.Body.String())
	}
}

// TestListUsersStorageReturnsAfterTopParseError pins the fix for the missing
// `return` after the `top` path param fails to parse as an integer. The
// handler must write one 400 and stop, not fall through to the service with
// top=0 and write a second response. (pageIndex/pageSize already returned
// correctly; only top was missing the return.)
func TestListUsersStorageReturnsAfterTopParseError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// page/page_size valid; top is non-integer → triggers the top parse error.
	ctx.Params = append(ctx.Params,
		gin.Param{Key: "page", Value: "1"},
		gin.Param{Key: "page_size", Value: "10"},
		gin.Param{Key: "top", Value: "abc"},
	)

	(&Handler{service: NewService()}).ListUsersStorage(ctx)

	resp := singleJSONResponse(t, recorder.Body.Bytes())
	if msg, _ := resp["message"].(string); msg != "Top must be an integer" {
		t.Fatalf("response message = %q, want 'Top must be an integer'; body=%q", msg, recorder.Body.String())
	}
}
