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

package service

import (
	"sort"
	"strings"
)

// The text helpers both graph readers share. They live on their own so that
// neither reader owns a helper the other calls: the ontology graph and the
// structure graph are read by different pages and are compiled together, and a
// symbol defined in one of them reads like a dependency between the two.

// endpointTerms is the set of spellings an endpoint name is looked up by.
func endpointTerms(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return sortedUnique([]string{value, strings.ToLower(value)})
}

// sortedUnique drops the empty entries, de-duplicates, and sorts.
func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
