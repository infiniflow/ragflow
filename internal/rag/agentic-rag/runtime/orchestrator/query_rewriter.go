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

package orchestrator

// MissingPiece is one gap: what is missing, and a hint for searching for it.
//
// It used to be defined beside the sufficiency review, which produced gaps of its own, and then
// consumed by the gap→query rewriter, which turned them into the next round's queries. Both are
// gone: the gaps still come from the round's own record (the plan slots it could not fill; see
// unresolvedClueGaps), and the only thing that reads them now is the DIRECTION that the next
// session is handed — the session writes its own queries (see the note where queryRewriteNode
// lived). So the type stays, with the code that renders it.
type MissingPiece struct {
	What       string
	SearchHint string
}
