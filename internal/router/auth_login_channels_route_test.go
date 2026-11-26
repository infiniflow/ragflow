package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

// TestRouterAnswersLoginChannels pins GET /api/v1/auth/login/channels: the login
// page calls it before any session exists, so it must be registered on the
// unauthenticated group and answer with a list (empty in the community edition),
// never a 404.
func TestRouterAnswersLoginChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler()}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/channels", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.Code, resp.Body.String())
	}

	var body struct {
		Code int               `json:"code"`
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", resp.Body.String(), err)
	}
	if body.Code != 0 {
		t.Fatalf("code=%d body=%s; want 0", body.Code, resp.Body.String())
	}
	if body.Data == nil {
		t.Fatalf("body=%s; want an empty data list rather than a missing key", resp.Body.String())
	}
	if len(body.Data) != 0 {
		t.Fatalf("body=%s; the community edition offers no login channels", resp.Body.String())
	}
}
