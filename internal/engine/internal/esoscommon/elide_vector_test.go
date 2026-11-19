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

package esoscommon

import (
	"strings"
	"sync"
	"testing"
)

// elideFieldNames covers both engines' wire-format vector field tokens.
var elideFieldNames = []string{QueryVectorFieldName, VectorFieldName}

// TestElideQueryVectorReplacesArrayWithElidedString checks the happy path for
// each engine: the dense-vector array is replaced by the "<elided>" string and
// the original values disappear.
func TestElideQueryVectorReplacesArrayWithElidedString(t *testing.T) {
	for _, field := range elideFieldNames {
		t.Run(field, func(t *testing.T) {
			body := []byte(`{"knn":{"` + field + `":[0.1,0.2,0.3],"k":3}}`)
			elided := ElideQueryVector(body, field)
			if !strings.Contains(elided, `"`+field+`":"<elided>"`) {
				t.Fatalf("ElideQueryVector = %s, want %q present", elided, field+`":"<elided>"`)
			}
			if strings.Contains(elided, "0.1") {
				t.Fatalf("ElideQueryVector left the embedding in %s", elided)
			}
			if strings.Contains(elided, `"`+field+`":[`) {
				t.Fatalf("ElideQueryVector still contains the array form: %s", elided)
			}
		})
	}
}

// TestElideQueryVectorIsolatesFieldByName is the adversarial distinguishing
// case: a body that carries BOTH engine field names must only elide the one
// requested. This is what proves the field name is a real parameter rather than
// a hardcoded token, and that the two engines cannot cross-elide each other.
func TestElideQueryVectorIsolatesFieldByName(t *testing.T) {
	body := []byte(`{"es":{"query_vector":[1,2,3]},"os":{"vector":[4,5,6]}}`)

	esElided := ElideQueryVector(body, QueryVectorFieldName)
	if strings.Contains(esElided, `"query_vector":[`) {
		t.Fatalf("query_vector array not elided: %s", esElided)
	}
	if !strings.Contains(esElided, `"vector":[4,5,6]`) {
		t.Fatalf("vector array wrongly touched when eliding query_vector: %s", esElided)
	}

	osElided := ElideQueryVector(body, VectorFieldName)
	if strings.Contains(osElided, `"vector":[`) {
		t.Fatalf("vector array not elided: %s", osElided)
	}
	if !strings.Contains(osElided, `"query_vector":[1,2,3]`) {
		t.Fatalf("query_vector array wrongly touched when eliding vector: %s", osElided)
	}
}

// TestElideQueryVectorLeavesBodyWithoutVectorUnchanged guards against the
// regex accidentally matching unrelated fields or injecting a placeholder into
// a vector-free body.
func TestElideQueryVectorLeavesBodyWithoutVectorUnchanged(t *testing.T) {
	body := []byte(`{"query":{"match":{"title":"hello"}}}`)
	elided := ElideQueryVector(body, QueryVectorFieldName)
	if strings.Contains(elided, "<elided>") {
		t.Fatalf("ElideQueryVector injected <elided> into a vector-free body: %s", elided)
	}
	if elided != string(body) {
		t.Fatalf("ElideQueryVector mutated a vector-free body: got %s want %s", elided, body)
	}
}

// TestElideQueryVectorEmptyBody checks the degenerate input cannot panic and
// yields an empty string.
func TestElideQueryVectorEmptyBody(t *testing.T) {
	if got := ElideQueryVector([]byte{}, QueryVectorFieldName); got != "" {
		t.Fatalf("ElideQueryVector(empty) = %q, want empty", got)
	}
}

// TestElideQueryVectorMultipleOccurrences checks every occurrence of the named
// field is elided, not just the first.
func TestElideQueryVectorMultipleOccurrences(t *testing.T) {
	for _, field := range elideFieldNames {
		t.Run(field, func(t *testing.T) {
			body := []byte(`{"a":{"` + field + `":[1,2]},"b":{"` + field + `":[3,4]}}`)
			elided := ElideQueryVector(body, field)
			if n := strings.Count(elided, `"<elided>"`); n != 2 {
				t.Fatalf("ElideQueryVector replaced %d occurrences, want 2: %s", n, elided)
			}
			if strings.Contains(elided, "[1,2]") || strings.Contains(elided, "[3,4]") {
				t.Fatalf("ElideQueryVector left an array: %s", elided)
			}
		})
	}
}

// TestElideQueryVectorConcurrentFieldNames exercises the per-field regex cache
// from multiple goroutines to surface any data race in the cache map/mutex.
func TestElideQueryVectorConcurrentFieldNames(t *testing.T) {
	body := []byte(`{"query_vector":[1,2,3],"vector":[4,5,6]}`)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			field := QueryVectorFieldName
			if i%2 == 0 {
				field = VectorFieldName
			}
			elided := ElideQueryVector(body, field)
			if !strings.Contains(elided, `"<elided>"`) {
				t.Errorf("elided missing placeholder: %s", elided)
			}
		}(i)
	}
	wg.Wait()
}
