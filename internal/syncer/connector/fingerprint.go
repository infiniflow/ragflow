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

	"github.com/zeebo/xxh3"
)

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

func syncDocumentID(value string) string {
	sum := xxh3.Hash128([]byte(value)).Bytes()
	return hex.EncodeToString(sum[:])
}

func syncDocumentStoredFingerprint(fingerprints map[string]string, kbID, connectorID, sourceID string) string {
	if len(fingerprints) == 0 || connectorID == "" || sourceID == "" {
		return ""
	}
	legacyID := syncDocumentID(connectorID + ":" + sourceID)
	newID := syncDocumentID(kbID + ":" + connectorID + ":" + sourceID)
	if stored := fingerprints[legacyID]; stored != "" {
		return stored
	}
	return fingerprints[newID]
}

func isSubmittedUnchanged(fingerprints map[string]string, kbID, connectorID string, doc SourceDocument) bool {
	stored := syncDocumentStoredFingerprint(fingerprints, kbID, connectorID, doc.SourceID)
	return stored != "" && stored == doc.Fingerprint
}
