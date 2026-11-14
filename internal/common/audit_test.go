//go:build !ee

package common_test

import (
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

// TestSyncAuditNoPanic verifies the OSS no-op audit flush is callable and
// returns without panicking. The function must remain defined in the OSS stub
// so the server entrypoint (cmd/ragflow_server.go) compiles and runs.
func TestSyncAuditNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SyncAudit panicked: %v", r)
		}
	}()
	common.SyncAudit()
}

// TestSyncAuditMultipleCalls guards against a regression where the no-op flush
// cannot be invoked repeatedly (e.g. an accidental removal that leaves a dangling
// deferred call in the server startup path).
func TestSyncAuditMultipleCalls(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SyncAudit panicked on repeated call: %v", r)
		}
	}()
	common.SyncAudit()
	common.SyncAudit()
	common.SyncAudit()
}

func TestOperationLogLeavesCommunityRequestUnchanged(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest("POST", "/datasets", nil)
	c.Set("user_id", "actor")
	c.Set("existing", "preserved")
	common.StartOperationLog(c, "create_dataset")
	common.SetOperationResource(c, "dataset_id", "dataset-one")
	common.RespondSuccess(c, map[string]any{"id": "dataset-one"}, "")
	common.FinishOperationLog(c)
	if len(c.Keys) != 2 || c.GetString("existing") != "preserved" || c.GetString("user_id") != "actor" {
		t.Fatalf("community context changed: %v", c.Keys)
	}
	if writer.Code != 200 || writer.Body.String() != `{"code":0,"message":"success","data":{"id":"dataset-one"}}` {
		t.Fatalf("community response changed: %s", writer.Body.String())
	}
}
