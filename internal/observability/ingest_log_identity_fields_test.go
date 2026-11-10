package observability

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// TestIngestionLogHasIdentityFields guards the shared schema extension the EE
// ClickHouse sink needs: IngestionLog must carry IngestorName and NatsQueue so
// the EE build can populate them at emit time. Reflection (not a compile-time
// field access) keeps this RED on a real assertion instead of a compile error
// while the struct still lacks the fields.
func TestIngestionLogHasIdentityFields(t *testing.T) {
	typ := reflect.TypeOf(common.IngestionLog{})
	for _, name := range []string{"IngestorName", "NatsQueue"} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("common.IngestionLog must declare field %q for EE sink population", name)
		}
	}
}

// TestBuildIngestLogAcceptsIdentityParams guards that BuildIngestLog exposes
// ingestorName and natsQueue parameters so callers (the EE emit path) can fill
// them. A source scan (not a compile-time call) keeps this RED on an assertion.
func TestBuildIngestLogAcceptsIdentityParams(t *testing.T) {
	src, err := os.ReadFile("ingestion_log.go")
	if err != nil {
		t.Fatalf("read ingestion_log.go: %v", err)
	}
	s := string(src)
	for _, need := range []string{"ingestorName", "natsQueue"} {
		if !strings.Contains(s, need) {
			t.Fatalf("BuildIngestLog must accept param %q", need)
		}
	}
}
