package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

func TestSystemHandlerGetVersion(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	NewSystemHandler(service.NewSystemService()).GetVersion(c)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || body.Code != 0 || body.Data.Version != common.GetRAGFlowVersion() {
		t.Fatalf("unexpected version response: HTTP %d, %s", w.Code, w.Body.String())
	}
}
