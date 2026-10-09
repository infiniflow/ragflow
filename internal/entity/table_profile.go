package entity

import (
	"encoding/json"
	"fmt"
	"sort"

	"ragflow/internal/common"
)

// TableProfileMetadataField is the reserved document-metadata key holding the column
// state a successful table run indexed. It lives in the document's metadata
// record because that record is already per document, already cleaned up when
// the document is deleted, and already readable per knowledge base.
//
// Being a metadata key it is also visible to the metadata panel and to
// meta_data_filter; every user-facing read of document metadata must exclude it
// (WithoutTableProfileField).
const TableProfileMetadataField = "_table_profile"

// TableProfile is what one document actually indexed: the engine the rows were
// written to, the columns those rows carry, and the document-metadata keys the table system
// still owns.
//
// The columns must be recorded because data_key is a hash: nothing about
// "c_9f3a..." recovers the header it came from, so the readable name of an
// indexed column exists only in this record.
type TableProfile struct {
	// Engine is the index engine this record was written under. A record
	// written for another engine does not take part in queries until the
	// document is re-parsed into the current engine.
	Engine string `json:"engine"`
	// Columns is the deduplicated column identity of every indexed row, in
	// key order.
	Columns []common.TableColumn `json:"columns"`
	// OwnedMetadata lists the document-metadata keys the table system wrote
	// and may replace or delete. A key a user or the LLM took over is not in
	// this list.
	OwnedMetadata []string `json:"owned_metadata"`
}

// FieldMap returns the mapping a structured query needs: the JSON key written
// to chunk_data mapped to the name a model and a reader recognise. Two
// documents that indexed the same header agree on both halves, because
// data_key is derived from the key alone.
func (p *TableProfile) FieldMap() map[string]string {
	out := make(map[string]string, len(p.Columns))
	for _, col := range p.Columns {
		out[col.DataKey] = col.DisplayName
	}
	return out
}

// OwnedKeys returns the document-metadata keys this record says the table
// system wrote. It is nil-safe so a caller holding a run's result can compare
// what the previous run owned against what this one produces without first
// checking whether either side exists.
func (p *TableProfile) OwnedKeys() []string {
	if p == nil {
		return nil
	}
	return p.OwnedMetadata
}

// Encode renders the profile as the value stored under ProfileMetadataField.
// It is a JSON string rather than a nested object so that every engine treats
// it as one opaque metadata value: a nested object would be mapped and indexed
// as user fields by the document-metadata stores.
func (p *TableProfile) Encode() (string, error) {
	normalized := TableProfile{
		Engine:        p.Engine,
		Columns:       append([]common.TableColumn(nil), p.Columns...),
		OwnedMetadata: append([]string(nil), p.OwnedMetadata...),
	}
	sort.Slice(normalized.Columns, func(i, j int) bool {
		return normalized.Columns[i].Key < normalized.Columns[j].Key
	})
	sort.Strings(normalized.OwnedMetadata)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("table profile: %w", err)
	}
	return string(raw), nil
}

// DecodeTableProfile reads the value stored under ProfileMetadataField. ok=false
// means the document has no usable record: absent, not a string, or not valid
// profile JSON. A caller treats all three the same way — the document does not
// contribute queryable fields — but only a malformed record is worth logging.
func DecodeTableProfile(value any) (profile *TableProfile, ok bool, err error) {
	raw, isString := value.(string)
	if !isString || raw == "" {
		return nil, false, nil
	}
	var decoded TableProfile
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, false, fmt.Errorf("table profile: %w", err)
	}
	if decoded.Engine == "" || len(decoded.Columns) == 0 {
		return nil, false, nil
	}
	return &decoded, true, nil
}

// WithoutTableProfileField returns the document-metadata fields a reader may see,
// with the reserved key dropped. The map is copied only when the key is
// actually present, so the ordinary document contributes no allocation and a
// caller cannot lose the record it still holds a reference to.
func WithoutTableProfileField(fields map[string]any) map[string]any {
	if _, ok := fields[TableProfileMetadataField]; !ok {
		return fields
	}
	out := make(map[string]any, len(fields)-1)
	for key, value := range fields {
		if key == TableProfileMetadataField {
			continue
		}
		out[key] = value
	}
	return out
}
