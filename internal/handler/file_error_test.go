package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ragflow/internal/service/file"

	"github.com/gin-gonic/gin"
)

func TestRespondFileServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		err  error
		code int
		msg  string
	}{{"authorization", file.ErrNoAuthorization, 102, "no authorization"}} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondFileServiceError(c, tc.err)
			if w.Code != 200 {
				t.Fatalf("HTTP status = %d, want 200", w.Code)
			}
			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != tc.code || body.Message != tc.msg {
				t.Fatalf("body = %s, want code=%d message=%q", w.Body, tc.code, tc.msg)
			}
		})
	}
}
