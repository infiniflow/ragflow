// Copyright 2025 Infiniflow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

// Canonical envelope keys. The canvas Component.Invoke contract keeps the
// envelope as map[string]any, but each well-known key carries a single typed
// payload. The "chunks" key (chunker/tokenizer output) has a typed accessor,
// GetChunks, so downstream components read []ChunkDoc instead of bare type
// assertions, eliminating the silent type-mismatch skips that the untyped map
// form allowed. The "json" key (parser output) remains a plain wire key.
//
// These live in the schema package because it owns ChunkDoc and is imported by
// every component subpackage (component, chunker, knowledge_compiler) without
// introducing an import cycle.
const (
	KeyParseItems = "json"
	KeyChunks     = "chunks"
)

// GetChunks reads the canonical "chunks" envelope key. It accepts either a
// typed []ChunkDoc or the legacy []map[string]any / []any form via
// ChunkDocsFromAny, so migration can proceed producer-by-producer without a
// flag day. A missing key, nil input, or a non-chunk carrier returns
// (nil, false). For the []map[string]any form a malformed entry rejects the
// whole carrier (nil, false) rather than being skipped; []any non-map entries
// are skipped. It never panics.
func GetChunks(in map[string]any) ([]ChunkDoc, bool) {
	if in == nil {
		return nil, false
	}
	v, ok := in[KeyChunks]
	if !ok {
		return nil, false
	}
	docs, ok, err := ChunkDocsFromAny(v)
	if err != nil || !ok {
		return nil, false
	}
	return docs, true
}

// ChunkCarrierToMaps returns a "chunks" carrier value as []map[string]any,
// bridging every form losslessly: []map[string]any is returned verbatim (no
// ChunkDoc round-trip, so non-ChunkDoc fields like content_with_weight
// survive), []ChunkDoc is converted via ChunkDocsToMaps, and []any is
// unwrapped to its map elements. Any other value, or a []any carrying a
// non-map element, returns (nil, false).
func ChunkCarrierToMaps(v any) ([]map[string]any, bool) {
	switch t := v.(type) {
	case []map[string]any:
		return t, true
	case []ChunkDoc:
		return ChunkDocsToMaps(t), true
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, it := range t {
			m, ok := it.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, m)
		}
		return out, true
	default:
		return nil, false
	}
}
