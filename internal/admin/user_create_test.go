package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/utility"

	"github.com/gin-gonic/gin"
)

var adminUserCreateTestModels = []any{&entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.File{}}

func TestCreateUserNickname(t *testing.T) {
	for _, tt := range []struct {
		name     string
		nickname *string
		want     string
		code     common.ErrorCode
	}{
		{name: "omitted", code: common.CodeSuccess},
		{name: "empty", nickname: new(""), code: common.CodeSuccess},
		{name: "whitespace", nickname: new(" \t "), code: common.CodeSuccess},
		{name: "display name", nickname: new("  Alice 张 😀  "), want: "Alice 张 😀", code: common.CodeSuccess},
		{name: "maximum unicode length", nickname: new(strings.Repeat("😀", common.NicknameLimit)), want: strings.Repeat("😀", common.NicknameLimit), code: common.CodeSuccess},
		{name: "too long", nickname: new(strings.Repeat("名", common.NicknameLimit+1)), code: common.CodeBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t, adminUserCreateTestModels...)
			t.Cleanup(testutil.ReplaceDBForTest(t, db))
			handler := NewHandler(NewService())
			body := map[string]string{"username": "new@example.com", "password": "test-password!"}
			if tt.nickname != nil {
				body["nickname"] = *tt.nickname
			}
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(payload))
			ctx.Request.Header.Set("Content-Type", "application/json")
			handler.CreateUser(ctx)
			var response adminTestResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != int(tt.code) {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
			if tt.code != common.CodeSuccess {
				var count int64
				if err := db.Model(&entity.User{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("invalid nickname created %d users", count)
				}
				return
			}
			var created struct {
				ID       string `json:"id"`
				Nickname string `json:"nickname"`
			}
			if err := json.Unmarshal(response.Data, &created); err != nil {
				t.Fatal(err)
			}
			var user entity.User
			if err := db.Where("id = ?", created.ID).Take(&user).Error; err != nil {
				t.Fatal(err)
			}
			if user.Nickname != tt.want || created.Nickname != tt.want {
				t.Fatalf("nickname: stored %q, returned %q; want %q", user.Nickname, created.Nickname, tt.want)
			}
			var tenant entity.Tenant
			if err := db.Where("id = ?", created.ID).Take(&tenant).Error; err != nil {
				t.Fatal(err)
			}
			wantTenantName := utility.TruncateRunes(tt.want+"'s Kingdom", common.TenantNameLimit)
			if tenant.Name == nil || *tenant.Name != wantTenantName {
				t.Fatalf("tenant name = %v, want %q", tenant.Name, wantTenantName)
			}
		})
	}
}
