package entity

import (
	"reflect"
	"testing"
)

// TestPipelineOperationLogHasNoProcessEndAtField guards that the process_end_at
// column/field was fully removed. The authoritative end time is recovered from
// ProcessBeginAt + ProcessDuration, so the struct must not carry a ProcessEndAt
// field at all (neither persisted nor in-memory). This prevents the field from
// being silently re-introduced.
func TestPipelineOperationLogHasNoProcessEndAtField(t *testing.T) {
	typ := reflect.TypeOf(PipelineOperationLog{})
	if _, ok := typ.FieldByName("ProcessEndAt"); ok {
		t.Fatalf("PipelineOperationLog must not have a ProcessEndAt field; remove it entirely")
	}
}
