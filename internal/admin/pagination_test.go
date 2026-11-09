package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
)

func TestAdminListHandlersRejectOversizedPagination(t *testing.T) {
	for name, handle := range map[string]gin.HandlerFunc{
		"users":  (&Handler{}).ListUsers,
		"models": (&Handler{}).ListAllModels,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest("GET", "/?page_size=101", nil)
			handle(ctx)
			var response struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != common.CodeBadRequest {
				t.Fatalf("unexpected response: %s", rec.Body.String())
			}
		})
	}
}
