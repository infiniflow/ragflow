// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

package entity

import "encoding/json"

// MediaKind names how a MediaRef locates its image in the source document.
type MediaKind string

const (
	// MediaKindDataURI locates an inline data URI inside the source document.
	// The reference is content-addressed: SHA256 identifies the image, so a
	// re-extraction that walks the source in a different order still finds it.
	MediaKindDataURI MediaKind = "data-uri"
)

// MediaRef is a parser-emitted reference to an image in the source document.
// In ref mode the parse result carries only the reference; in both mode it
// accompanies the inline bytes. It lets the ingestion chunker (or any later
// consumer) re-materialize exactly one image from the source instead of
// carrying every image in memory for the whole parse.
//
// The reference is intentionally source-relative: it carries no DocID. The
// parser does not know the document ID, and every consumer already resolves
// the source document from the ingestion payload.
type MediaRef struct {
	// Format is the parser family that produced the reference ("html", ...).
	Format string `json:"format"`
	// Kind is the locating scheme; see the MediaKind constants.
	Kind MediaKind `json:"kind"`
	// SHA256 is the lowercase hex digest of the decoded image bytes. It is
	// both the lookup key and the integrity check after re-materialization.
	SHA256 string `json:"sha256"`
	// MIME is the image media type from the source ("image/png", ...).
	MIME string `json:"mime,omitempty"`
	// Size is the decoded image size in bytes.
	Size int64 `json:"size,omitempty"`
}

// Valid reports whether the reference carries the fields a resolver needs.
func (r *MediaRef) Valid() bool {
	return r != nil && r.Format != "" && r.Kind != "" && r.SHA256 != ""
}

// MediaRefFromAny decodes a media_ref value from either its typed form or a
// JSON-round-tripped map. It returns nil for absent or unusable values so
// callers can treat "no reference" and "bad reference" the same way.
func MediaRefFromAny(v any) *MediaRef {
	switch ref := v.(type) {
	case nil:
		return nil
	case *MediaRef:
		if ref.Valid() {
			return ref
		}
		return nil
	case MediaRef:
		if ref.Valid() {
			return &ref
		}
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var ref MediaRef
	if err := json.Unmarshal(b, &ref); err != nil || !ref.Valid() {
		return nil
	}
	return &ref
}
