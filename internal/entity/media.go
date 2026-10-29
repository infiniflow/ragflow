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

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// MediaKind names how a MediaRef locates its image bytes.
type MediaKind string

const (
	// MediaKindKV locates an image in the run-scoped Kvrocks media cache.
	MediaKindKV MediaKind = "kvrocks"
)

// MediaRef is a parser-emitted reference to deferred image bytes. A parser
// that emits one keeps image payloads out of its structured result. The
// ingestion run materializes the bytes through the reference's media cache.
type MediaRef struct {
	// Format is the parser family that produced the reference ("html", ...).
	Format string `json:"format"`
	// Kind is the locating scheme; see the MediaKind constants.
	Kind MediaKind `json:"kind"`
	// Scope binds the reference to one ingestion run. A cache lookup from a
	// different run is rejected instead of resolving a same-content image from
	// another task.
	Scope string `json:"scope,omitempty"`
	// SHA256 is the lowercase hex digest of the decoded image bytes. It is
	// both the lookup key and the integrity check after materialization.
	SHA256 string `json:"sha256"`
	// MIME is the image media type from the source ("image/png", ...).
	MIME string `json:"mime,omitempty"`
	// Size is the decoded image size in bytes.
	Size int64 `json:"size,omitempty"`
}

// Valid reports whether the reference carries the fields a resolver needs.
// The digest must be the lowercase hex form the parser emits: the resolver
// compares it as a string, so another spelling would turn a corrupt reference
// into a silent lookup miss.
func (r *MediaRef) Valid() bool {
	if r == nil || r.Format == "" || r.Kind == "" || len(r.SHA256) != sha256.Size*2 {
		return false
	}
	if strings.ToLower(r.SHA256) != r.SHA256 {
		return false
	}
	_, err := hex.DecodeString(r.SHA256)
	return err == nil
}

// IsZero reports whether the reference carries no field at all. JSON `{}`
// decodes into a non-nil zero value; consumers normalize it back to nil so an
// empty object keeps meaning "no reference".
func (r *MediaRef) IsZero() bool {
	return r == nil || (r.Format == "" && r.Kind == "" && r.Scope == "" && r.SHA256 == "" && r.MIME == "" && r.Size == 0)
}

// MediaRefFromAny decodes a media_ref value from either its typed form or a
// JSON-round-tripped map. It returns nil for absent or unusable values;
// callers that must reject a malformed reference check whether a value is
// present before calling it.
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
