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

package chunk

import (
	"ragflow/internal/entity"
	"ragflow/internal/service/chunkversion"
)

// chunkSnapshotMeta describes the producer of the derived fields a version
// stores: the dataset's embedding model and language. Both are read off the
// dataset the request resolved, so they name the model this write actually used
// rather than a process-wide default.
//
// It lives outside the edtion-tagged files because both editions name it at the
// edit call sites; only the enterprise edition stores the result.
func chunkSnapshotMeta(kb *entity.Knowledgebase, language string) chunkversion.SnapshotMeta {
	if kb == nil {
		return chunkversion.SnapshotMeta{Language: language}
	}
	return chunkversion.SnapshotMeta{EmbeddingID: kb.EmbdID, Language: language}
}
