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

// Package chunkversion holds the chunk history helpers shared by the editor and
// the ingestion writer. Both write the same table, so the snapshot shape lives
// here instead of in either caller: FullSnapshot (see full_snapshot.go) is the
// whole engine row a parse, an edit or a restore stores, so rewinding to it is a
// copy.
package chunkversion

import "ragflow/internal/entity"

// ChunkState is one chunk's state at a moment: its snapshot, or the fact that it
// did not exist then. It lives here rather than with StatesAt so the community
// stub can return the same type and both builds keep one signature.
type ChunkState struct {
	ChunkID string
	// Version is the number of the row the snapshot was read from, so a rebuild
	// can record what it restored from and notice that the chunk's history has
	// moved past the anchor.
	Version  int
	Snapshot entity.JSONMap
	Exists   bool
}
