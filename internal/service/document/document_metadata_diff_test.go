package document

import (
	"testing"
)

// TestMetadataFieldChangesNamesEachEdit pins what a metadata version records: a
// key the edit introduced, one it changed and one it removed, and nothing for a
// key the edit left alone.
func TestMetadataFieldChangesNamesEachEdit(t *testing.T) {
	changes := metadataFieldChanges(
		map[string]interface{}{"author": "alice", "note": "old"},
		map[string]interface{}{"author": "bob", "tags": []interface{}{"a", "b"}},
	)

	if len(changes) != 3 {
		t.Fatalf("changes = %+v, want the three keys that moved", changes)
	}
	// Sorted by key, so one edit reads the same way twice.
	if changes[0].Key != "author" || string(changes[0].Before) != `"alice"` ||
		string(changes[0].After) != `"bob"` {
		t.Fatalf("first change = %+v, want author alice -> bob", changes[0])
	}
	if changes[1].Key != "note" || changes[1].Before == nil || changes[1].After != nil {
		t.Fatalf("second change = %+v, want the removed key", changes[1])
	}
	if changes[2].Key != "tags" || changes[2].Before != nil ||
		string(changes[2].After) != `["a","b"]` {
		t.Fatalf("third change = %+v, want the introduced list kept a list", changes[2])
	}
}

// TestMetadataFieldChangesIgnoresNoOps keeps a save that changed nothing out of
// the timeline: the values compare by their JSON form, so a number the store
// read back as float64 is not an edit the request made.
func TestMetadataFieldChangesIgnoresNoOps(t *testing.T) {
	if changes := metadataFieldChanges(
		map[string]interface{}{"count": 1, "author": "alice"},
		map[string]interface{}{"count": float64(1), "author": "alice"},
	); len(changes) != 0 {
		t.Fatalf("changes = %+v, want nothing for an unchanged save", changes)
	}

	if changes := metadataFieldChanges(nil, nil); changes != nil {
		t.Fatalf("changes = %+v, want nothing for an empty save", changes)
	}
}

// TestMetadataFieldChangesTreatsNullAsAbsent matches the store and the metadata
// history, which both drop nulls: a key saved as null is the key not being held,
// so it reads as a removal rather than as a value that moved to nothing.
func TestMetadataFieldChangesTreatsNullAsAbsent(t *testing.T) {
	changes := metadataFieldChanges(
		map[string]interface{}{"author": "alice"},
		map[string]interface{}{"author": nil},
	)
	if len(changes) != 1 {
		t.Fatalf("changes = %+v, want the null read as a removal", changes)
	}
	if changes[0].Key != "author" || changes[0].After != nil || string(changes[0].Before) != `"alice"` {
		t.Fatalf("change = %+v, want author removed", changes[0])
	}

	// A null on both sides is no key at all: no entry, and no nil-valued one.
	if changes := metadataFieldChanges(
		map[string]interface{}{"author": nil},
		map[string]interface{}{"author": nil},
	); len(changes) != 0 {
		t.Fatalf("changes = %+v, want nothing for null on both sides", changes)
	}
}
