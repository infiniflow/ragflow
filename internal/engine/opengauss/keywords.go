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

package opengauss

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	pq "gitcode.com/opengauss/openGauss-connector-go-pq"
)

// A nonempty delimiter-joined list cannot begin with three hashes followed by
// a brace: a leading hash element followed by a separator has at least four hashes.
const keywordArrayPrefix = "###"

func chunkKeywordField(field string) bool {
	return fieldKeyword(field) || field == "source_doc_ids" || field == "source_chunk_ids"
}

func encodeKeywords(value any) (any, error) {
	items, list := interfaceSlice(value)
	if !list {
		if value == "" {
			return "", nil
		}
		items = []any{value}
	}
	words := make(pq.StringArray, 0, len(items))
	needsArray := false
	for _, item := range items {
		word := stringValue(item)
		if word == "" {
			return nil, fmt.Errorf("opengauss: empty keyword element")
		}
		words = append(words, word)
		needsArray = needsArray || strings.Contains(word, "#")
	}
	if needsArray {
		literal, err := words.Value()
		if err != nil {
			return nil, err
		}
		return keywordArrayPrefix + literal.(string), nil
	}
	return strings.Join(words, keywordSeparator), nil
}

// Recover element boundaries for both array literals and ordinary joined lists.
func keywordArraySQL(field string) string {
	col := quoteIdent(field)
	return fmt.Sprintf("CASE WHEN left(%[1]s,4) = '###{' THEN substr(%[1]s,4)::text[] ELSE COALESCE(string_to_array(%[1]s,'###'),ARRAY[]::text[]) END", col)
}

// keywordUpdates applies list edits inside the scoped UPDATE so concurrent
// edits cannot overwrite one another between a separate read and write.
func keywordUpdates(values map[string]interface{}, columns map[string]columnMeta) ([]string, []interface{}, error) {
	expressions := map[string]string{}
	args := &sqlArgs{}
	for _, operation := range []string{"remove", "add"} {
		raw, present := values[operation]
		if !present {
			continue
		}
		changes, ok := raw.(map[string]interface{})
		if !ok {
			if _, reset := raw.(string); reset && operation == "remove" {
				continue
			}
			return nil, nil, fmt.Errorf("opengauss: %s requires a field map", operation)
		}
		for _, field := range slices.Sorted(maps.Keys(changes)) {
			switch field {
			case "source_id", "important_kwd", "question_kwd", "tag_kwd", "entities_kwd":
			default:
				return nil, nil, fmt.Errorf("opengauss: %s is not a list field", field)
			}
			if _, ok := columns[field]; !ok {
				return nil, nil, fmt.Errorf("opengauss: unknown list field %q", field)
			}
			if _, ok := values[field]; ok || values["remove"] == field {
				return nil, nil, fmt.Errorf("opengauss: conflicting updates to %s", field)
			}
			item, ok := changes[field].(string)
			if !ok || item == "" {
				return nil, nil, fmt.Errorf("opengauss: invalid list element for %s", field)
			}
			array := expressions[field]
			if array == "" {
				array = keywordArraySQL(field)
			}
			param := fmt.Sprintf("$%d::text", args.add(item))
			if operation == "remove" {
				array = "array_remove(" + array + "," + param + ")"
			} else {
				array = "CASE WHEN " + param + " = ANY(" + array + ") THEN " + array + " ELSE array_append(" + array + "," + param + ") END"
			}
			expressions[field] = array
		}
	}
	var assignments []string
	for _, field := range slices.Sorted(maps.Keys(expressions)) {
		assignments = append(assignments, fmt.Sprintf("%s = CASE WHEN array_length(%s,1) > 0 THEN '###' || (%s)::text ELSE '' END", quoteIdent(field), expressions[field], expressions[field]))
	}
	return assignments, args.values, nil
}
