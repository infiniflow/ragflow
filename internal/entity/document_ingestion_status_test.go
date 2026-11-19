package entity

import (
	"testing"

	"ragflow/internal/common"
)

// A restored document's status comes from what the restore put back; the parse
// marker distinguishes an empty parse from never parsed.
func TestIngestionStatusWithRestoredEmptyParse(t *testing.T) {
	restored := int64(3)
	doc := &Document{RestoredVersion: &restored, ChunkNum: 0}

	if got := doc.IngestionStatusWith(""); got != common.UNSTART {
		t.Fatalf("restored empty document without a parse marker = %q, want %q", got, common.UNSTART)
	}

	doc.RestoredFromParse = true
	if got := doc.IngestionStatusWith(""); got != common.COMPLETED {
		t.Fatalf("restored empty document with a parse marker = %q, want %q", got, common.COMPLETED)
	}

	// A live run still wins over the content marker.
	if got := doc.IngestionStatusWith(common.RUNNING); got != common.RUNNING {
		t.Fatalf("restored empty document with a live run = %q, want %q", got, common.RUNNING)
	}
}
