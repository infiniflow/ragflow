//
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
//

package connector

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zeebo/xxh3"
)

var syncFilenameUnsafeRE = regexp.MustCompile(`[\\/:*?"<>|]+`)

func stableFingerprint(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(fmt.Sprint(value))
	}
	return contentFingerprint(data)
}

func contentFingerprint(data []byte) string {
	sum := xxh3.Hash128(data).Bytes()
	return hex.EncodeToString(sum[:])
}

func syncSourceDocumentFilenameFromDocument(doc SourceDocument) string {
	name := strings.TrimSpace(syncFilenameUnsafeRE.ReplaceAllString(doc.SemanticIdentifier, "_"))
	if name == "" {
		name = strings.TrimSpace(syncFilenameUnsafeRE.ReplaceAllString(doc.SourceID, "_"))
	}
	if name == "" {
		name = "document"
	}

	extension := strings.TrimSpace(doc.Extension)
	if extension == "" {
		extension = ".txt"
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(extension)) {
		name += extension
	}
	if len(name) <= 255 {
		return name
	}

	ext := filepath.Ext(name)
	baseLimit := 255 - len(ext)
	if baseLimit < 1 {
		return name[:255]
	}
	return name[:baseLimit] + ext
}
