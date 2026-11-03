package entity

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestPipelineOperationLogProcessEndAtNotPersisted verifies ProcessEndAt is an
// in-memory-only field and is never mapped to a database column. The begin/duration
// columns it would otherwise duplicate must remain present.
func TestPipelineOperationLogProcessEndAtNotPersisted(t *testing.T) {
	sc, err := schema.Parse(&PipelineOperationLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	for _, f := range sc.Fields {
		if f.DBName == "process_end_at" {
			t.Fatalf("ProcessEndAt must not map to a DB column, but schema field %q has DBName %q", f.Name, f.DBName)
		}
	}
	haveBegin, haveDur := false, false
	for _, f := range sc.Fields {
		if f.DBName == "process_begin_at" {
			haveBegin = true
		}
		if f.DBName == "process_duration" {
			haveDur = true
		}
	}
	if !haveBegin {
		t.Error("process_begin_at column missing from schema (unexpected)")
	}
	if !haveDur {
		t.Error("process_duration column missing from schema (unexpected)")
	}
}
