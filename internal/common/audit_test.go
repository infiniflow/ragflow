//go:build !ee

package common_test

import (
	"testing"

	"ragflow/internal/common"
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
