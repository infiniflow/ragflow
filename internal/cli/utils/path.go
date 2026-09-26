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
//  See the specific language governing permissions and limitations
//  under the License.
//

package utils

import (
	"net/url"
	"strings"
)

// APIPath joins a prefix and path segments into an API path, escaping every
// segment so a value containing "/" or "?" cannot break out of its own
// segment. CLI paths are built from user-supplied names and IDs, and
// fmt.Sprintf leaves those values unescaped.
//
// Pass RAW values only. A value that is already percent-encoded (such callers
// in the CLI name it with an "Encoded"/"encoded" prefix) must NOT go through
// here again — PathEscape would rewrite "%2F" as "%252F".
func APIPath(prefix string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, prefix)
	for _, s := range segments {
		parts = append(parts, url.PathEscape(s))
	}
	return strings.Join(parts, "/")
}

// APIQuery builds an encoded query string from key/value pairs, so a value
// containing "&" or "=" cannot inject extra parameters.
func APIQuery(pairs ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		q.Set(pairs[i], pairs[i+1])
	}
	return q.Encode()
}
