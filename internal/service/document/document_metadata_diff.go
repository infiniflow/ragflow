package document

import (
	"encoding/json"
	"sort"

	"ragflow/internal/entity"
)

// metadataFieldChanges is what an edit of a document's metadata values moved,
// key by key: the value the key held, the value it holds now, and nothing for a
// key the edit left alone.
//
// The diff is taken on the values rather than on the store's state, because the
// version's change set is read as "what this edit did". A side that holds no
// value is the absence of that side: the store keeps no nulls, and the metadata
// history drops them too - so a key the edit introduced has no before, a key it
// removed has no after, and both are the shape the reader renders as "(none)".
//
// Values compare through their JSON form for the same reason the snapshots do:
// a number read back from the store as float64 equals the int the request
// carried, so it does not read as an edit that never happened.
func metadataFieldChanges(before, after map[string]interface{}) []entity.MetadataFieldChange {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for _, values := range []map[string]interface{}{before, after} {
		for key, value := range values {
			if value == nil {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	// Sorted so one edit reads the same way twice, and so a bounded set keeps a
	// predictable prefix of the keys rather than whichever three a map offered.
	sort.Strings(keys)

	changes := make([]entity.MetadataFieldChange, 0, len(keys))
	for _, key := range keys {
		beforeValue := metadataValueOf(before, key)
		afterValue := metadataValueOf(after, key)
		if beforeValue.set && afterValue.set && metadataValuesEqual(beforeValue.value, afterValue.value) {
			continue
		}
		change := entity.MetadataFieldChange{Key: key}
		if beforeValue.set {
			change.Before = metadataValueJSON(beforeValue.value)
		}
		if afterValue.set {
			change.After = metadataValueJSON(afterValue.value)
		}
		if change.Before == nil && change.After == nil {
			continue
		}
		changes = append(changes, change)
	}
	return changes
}

// metadataValue is one side of a comparison: the value and whether it is there
// at all. The two cannot be one field, because no value is a state a key can be
// in and it has to read differently from the key not existing.
type metadataValue struct {
	value interface{}
	set   bool
}

// metadataValueOf reads one key, treating a null the same way the history does:
// as the key not being held.
func metadataValueOf(values map[string]interface{}, key string) metadataValue {
	if values == nil {
		return metadataValue{}
	}
	value, ok := values[key]
	if !ok || value == nil {
		return metadataValue{}
	}
	return metadataValue{value: value, set: true}
}

// metadataValuesEqual compares two metadata values by their JSON form.
func metadataValuesEqual(left, right interface{}) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

// metadataValueJSON is one side of a change as the reader receives it: the value
// in whatever shape it has, so a list stays a list. A value that cannot be
// encoded is reported as absent rather than as a broken document.
func metadataValueJSON(value interface{}) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}
