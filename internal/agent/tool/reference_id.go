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

package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// referenceIDs derives the citation identifiers for one search result.
// documentID keys the chunk and its docAgg; displayID is the citation marker
// rendered into the prompt. The URL participates in the seed so identical
// content served from different URLs stays distinguishable, and the 64-bit
// halves make collisions negligible at search-result volumes.
func referenceIDs(resultURL, content string) (documentID, displayID string) {
	seed := strings.TrimSpace(resultURL) + "\n" + content
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8]), hex.EncodeToString(sum[8:16])
}
