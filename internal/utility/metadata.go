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

package utility

import (
	"encoding/json"
	"reflect"
)

type metadataValueKind uint8

const (
	metadataValueUnsupported metadataValueKind = iota
	metadataValueString
	metadataValueStringList
	metadataValueScalar
	metadataValueStructured
)

var jsonNumberType = reflect.TypeOf(json.Number(""))

// UpdateMetadataTo merges metadata into an existing metadata map. Strings and
// all-string lists retain Python's ordered merge and de-duplication behavior.
// JSON-compatible scalar and structured values are preserved so callers that
// completely replace a metadata map do not lose existing fields.
// Mirrors Python: common.metadata_utils.update_metadata_to().
func UpdateMetadataTo(target map[string]any, meta any) map[string]any {
	if target == nil || meta == nil {
		return target
	}

	entries, ok := metadataEntries(meta)
	if !ok || len(entries) == 0 {
		return target
	}

	// Validate every incoming value before changing target. The persisted
	// document metadata is JSON, so encoding/json is the authoritative
	// boundary; values are retained in their original Go representation.
	normalizedEntries := make([]metadataEntry, 0, len(entries))
	for _, entry := range entries {
		normalized, kind := normalizeMetaValue(entry.value)
		if kind != metadataValueUnsupported {
			normalizedEntries = append(normalizedEntries, metadataEntry{key: entry.key, value: normalized, kind: kind})
		}
	}

	for _, entry := range normalizedEntries {
		key, normalized, kind := entry.key, entry.value, entry.kind
		existing, exists := target[key]
		if !exists {
			target[key] = normalized
			continue
		}

		switch kind {
		case metadataValueStructured:
			// Structured values are retained only when absent.
			continue
		case metadataValueScalar:
			// Scalar values always replace the target value.
			target[key] = normalized
		case metadataValueString, metadataValueStringList:
			existingStrings, existingIsStringList := metadataListStrings(existing)
			if existingIsStringList {
				incomingStrings, _ := metadataStringList(normalized)
				target[key] = dedupeStrings(append(existingStrings, incomingStrings...))
				continue
			}
			if isStructuredMetadataList(existing) {
				// Do not replace or extend a structured list with extracted strings.
				continue
			}
			// For all other collisions, the incoming string/list wins.
			target[key] = normalized
		}
	}

	return target
}

// normalizeMetaValue classifies values that encoding/json can serialize. This
// intentionally accepts marshalable structs, pointers, maps, slices, and
// arrays consistently, including nested forms; it does not parse, transform,
// recursively merge, or deep-copy metadata.
func normalizeMetaValue(value any) (any, metadataValueKind) {
	if value == nil {
		return nil, metadataValueScalar
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, metadataValueUnsupported
	}
	if _, ok := value.(json.Number); ok {
		return value, metadataValueScalar
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), metadataValueString
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return value, metadataValueScalar
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			return nil, metadataValueUnsupported
		}
		if strings, ok := metadataStringList(value); ok {
			return dedupeStrings(strings), metadataValueStringList
		}
		return value, metadataValueStructured
	case reflect.Map:
		return value, metadataValueStructured
	default:
		return value, metadataValueStructured
	}
}

type metadataEntry struct {
	key   string
	value any
	kind  metadataValueKind
}

// metadataEntries accepts maps whose keys have string kind, including named
// map types. This matches encoding/json's object-key contract without changing
// the caller's value representation.
func metadataEntries(meta any) ([]metadataEntry, bool) {
	rv := reflect.ValueOf(meta)
	if !rv.IsValid() || rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}

	entries := make([]metadataEntry, 0, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		entries = append(entries, metadataEntry{key: iter.Key().String(), value: iter.Value().Interface()})
	}
	return entries, true
}

// metadataStringList reports whether value is a string or a list whose every
// element is a string. It accepts typed string slices and arrays as well.
func metadataStringList(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.String {
		return []string{rv.String()}, true
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	if rv.Len() == 0 {
		return nil, rv.Type().Elem().Kind() == reflect.String || rv.Type().Elem().Kind() == reflect.Interface
	}
	strings := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		element := rv.Index(i)
		for element.Kind() == reflect.Interface {
			if element.IsNil() {
				return nil, false
			}
			element = element.Elem()
		}
		if element.Kind() != reflect.String {
			return nil, false
		}
		if element.Type() == jsonNumberType {
			return nil, false
		}
		strings = append(strings, element.String())
	}
	return strings, true
}

func metadataListStrings(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	return metadataStringList(value)
}

func isStructuredMetadataList(value any) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	_, isStringList := metadataStringList(value)
	return !isStringList
}

// dedupeStrings removes duplicates while preserving order.
func dedupeStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
