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
	"regexp"
	"sync"
)

// QueryVectorFieldName is the dense-vector query field used by the
// Elasticsearch wire format.
const QueryVectorFieldName = "query_vector"

// VectorFieldName is the dense-vector query field used by the OpenSearch wire
// format (Elasticsearch uses QueryVectorFieldName).
const VectorFieldName = "vector"

var (
	elideVectorMu    sync.Mutex
	elideVectorCache = map[string]*regexp.Regexp{}
)

// elideVectorRegex returns a cached, compiled regex that matches the serialized
// dense-vector query under the given field name: "field":[...]. The regex is
// cached per field name because the only two callers (the ES and OS engines)
// invoke this on the hot path of every vector search whose request body is
// written to the log, so recompiling on each call would be wasteful.
func elideVectorRegex(fieldName string) *regexp.Regexp {
	elideVectorMu.Lock()
	defer elideVectorMu.Unlock()
	if re, ok := elideVectorCache[fieldName]; ok {
		return re
	}
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(fieldName) + `":\s*\[[^\]]*\]`)
	elideVectorCache[fieldName] = re
	return re
}

// ElideQueryVector removes the dense-vector array from a search request body
// before the body is logged, replacing it with a short "<elided>" placeholder
// so that 1024-dimension vectors never bloat the logs. fieldName is the
// engine's vector field token (QueryVectorFieldName for Elasticsearch,
// VectorFieldName for OpenSearch). Only the named field is elided; other
// fields are left intact, which keeps the two engines from cross-eliding each
// other's payloads. The replacement intentionally turns "field":[...] into
// "field":"<elided>" (an array becomes a string); that shape change is an
// expected side effect of log redaction, not a lossless transformation.
func ElideQueryVector(body []byte, fieldName string) string {
	re := elideVectorRegex(fieldName)
	replacement := `"` + fieldName + `":"<elided>"`
	return re.ReplaceAllString(string(body), replacement)
}
