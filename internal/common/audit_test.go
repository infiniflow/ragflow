//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

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
