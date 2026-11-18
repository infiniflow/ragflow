package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"github.com/gin-gonic/gin"
)

func TestGenerateUserAPITokenQuotaReturnsBadRequest(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.APIToken{}, &entity.SystemSettings{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	if err := db.Create(&entity.User{ID: "user-1", Email: "user@example.com", Nickname: "user"}).Error; err != nil {
		t.Fatal(err)
	}
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
	c.Params = gin.Params{{Key: "username", Value: "user@example.com"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/users/user@example.com/tokens", nil)
	NewHandler(NewService()).GenerateUserAPIToken(c)
	var response setVariableResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != int(common.CodeBadRequest) {
		t.Fatalf("unexpected quota response: %s", recorder.Body.String())
	}
}
