package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/permission"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

func TestCreateKeyQuotaReturnsBadRequest(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.Tenant{}, &entity.UserTenant{}, &entity.APIToken{}, &entity.SystemSettings{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	if err := db.Create(&entity.Tenant{ID: "tenant-1", ParserIDs: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserTenant{ID: "membership", UserID: "user-1", TenantID: "tenant-1", Role: "owner"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := range dao.DefaultAPITokenLimit {
		if err := db.Create(&entity.APIToken{TenantID: "tenant-1", Token: fmt.Sprint(i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("tenant_id", "tenant-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/system/keys", nil).
		WithContext(permission.WithIdentity(t.Context(), "user-1", "tenant-1"))
	NewSystemHandler(&service.SystemService{}).CreateKey(c)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "API key limit reached") {
		t.Fatalf("unexpected quota response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
