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

package common

import (
	"sort"
	"strings"
)

func Deduplicate(names []string) []string {
	if names == nil {
		return nil
	}

	seen := make(map[string]bool)
	result := make([]string, 0, len(names))

	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return result
}

// DeduplicateTrimmed removes surrounding whitespace, drops empty strings, and
// removes duplicates while preserving order. Returns nil when the result is
// empty.
func DeduplicateTrimmed(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// DeduplicateSorted drops empty strings, removes duplicates, and sorts the
// result. It does not mutate the input slice. Returns nil when empty.
func DeduplicateSorted(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	if len(result) == 0 {
		return nil
	}
	out := append([]string(nil), result...)
	sort.Strings(out)
	return out
}

// AppendUnique appends values not already present in dst (empty strings
// dropped), preserving order, and returns the extended dst.
func AppendUnique(dst, values []string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}
