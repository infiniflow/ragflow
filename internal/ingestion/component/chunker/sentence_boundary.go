//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except under the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package chunker

import "regexp"

// sentenceBoundaryRe is the single sentence-boundary definition shared by the
// title chunker family and the token chunker's JSON path. It mirrors Python's
// rag/flow/chunker/_sentence_boundary.py:SENTENCE_BOUNDARY_RE exactly: the
// Chinese/English period, exclamation mark, question mark, the Chinese
// variants, the newline, and the English ". " (period + space) boundary. The
// capturing group keeps the delimiter attached to the preceding sentence so
// re-merged text preserves original boundaries.
//
// The token chunker's TEXT path (naive_merge port) uses a different delimiter
// — sentenceDelimiter in token.go, mirroring Python naive_merge's production
// default DEFAULT_DELIMITER ("\n!?;。；！？", no ". "). The two constants mirror
// two distinct Python constants and differ only in that English boundary, so
// they are intentionally NOT merged into one.
var sentenceBoundaryRe = regexp.MustCompile(`([。!?？；！\n]|\. )`)
