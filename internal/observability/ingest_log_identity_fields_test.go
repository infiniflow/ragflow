package observability

import (
	"reflect"
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
