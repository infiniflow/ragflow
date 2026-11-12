package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

func TestAdminAuditRecordsOutcomesAndRejectsUnauditedMutations(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.AutoMigrate(&entity.AdminOperationLog{}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(NewService())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "admin", Email: "admin@example.test"}) })
	executed := 0
	r.PUT("/api/v1/admin/users/:username/password", h.auditMiddleware(), func(c *gin.Context) {
		executed++
		if c.Param("username") == "failure@example.test" {
			common.RespondError(c, common.CodeBadRequest, "fixture failure")
			return
		}
		common.RespondSuccess(c, nil, "updated")
	})
	for _, name := range []string{"target@example.test", "failure@example.test"} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+name+"/password",
			bytes.NewBufferString(`{"new_password":"fixture-credential"}`))
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
	var records []entity.AdminOperationLog
	if err := db.Order("target_email DESC").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ActorID != "admin" ||
		records[0].TargetEmail != "target@example.test" || records[0].Result != "success" ||
		records[1].Result != "failure" || records[0].Action != "user.password.reset" {
		t.Fatalf("incorrect audit outcomes: %+v", records)
	}
	if err := db.Migrator().DropTable(&entity.AdminOperationLog{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/target@example.test/password", nil))
	if recorder.Code != http.StatusServiceUnavailable || executed != 2 {
		t.Fatalf("unaudited mutation was not blocked: status=%d handler_calls=%d", recorder.Code, executed)
	}
}

func TestAdminAuditFiltersAndTotal(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.AutoMigrate(&entity.AdminOperationLog{}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(NewService())
	for i := range 7 {
		entry := entity.AdminOperationLog{
			ID: fmt.Sprintf("event-%d", i), ActorID: "admin", ActorEmail: "admin@example.test",
			TargetID: "target", TargetEmail: "member@example.test",
			Action: "user.status.update", Result: "success",
		}
		if err := db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/operation-logs?keyword=MEMBER&action=user.status.update&result=success&page=2&page_size=3", nil)
	h.ListOperationLogs(c)
	var body struct {
		Code  int                        `json:"code"`
		Total int64                      `json:"total"`
		Data  []entity.AdminOperationLog `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Total != 7 || len(body.Data) != 3 || body.Data[0].ID != "event-3" {
		t.Fatalf("incorrect audit page: %s", response.Body.String())
	}
}
