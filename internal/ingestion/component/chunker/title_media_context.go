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

package chunker

import (
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// titleMediaContextIDKey is transient provenance added by the title crop bridge
// and removed before returning chunks. It never enters saved pipeline output.
const titleMediaContextIDKey = "_title_media_context_id"

// attachTitleMediaContext adapts this invocation's source records to the shared
// General collector. Call after Manual's physical sort and before heading
// grouping: context must not include duplicated ancestor/root headings or
// cap-split copies. The collector skips other media, respects sheet boundaries
// and selects complete sentences independently on either side of each media.
func attachTitleMediaContext(records []lineRecord, tableTokens, imageTokens int) {
	attached, withoutContext := 0, 0
	defer func() {
		common.Debug("chunker stage",
			zap.String("variant", "title-media-context"),
			zap.Int("records", len(records)),
			zap.Int("table_context_size", tableTokens),
			zap.Int("image_context_size", imageTokens),
			zap.Int("media_with_context", attached),
			zap.Int("enabled_media_without_context", withoutContext),
		)
	}()
	if tableTokens <= 0 && imageTokens <= 0 {
		return
	}
	units := make([]schema.ChunkDoc, len(records))
	for i, rec := range records {
		units[i] = schema.ChunkDoc{
			Text: removeTag(rec.text), DocType: rec.docType,
			Sheet: rec.sheet, SheetIndex: rec.sheetIndex,
		}
	}
	attachGeneralMediaContext(units, tableTokens, imageTokens, nil)
	for i, unit := range units {
		if (unit.DocType == "table" && tableTokens > 0) || (unit.DocType == "image" && imageTokens > 0) {
			if unit.ContextAbove != "" || unit.ContextBelow != "" {
				attached++
			} else {
				// The shared collector found no eligible neighboring text
				// within this media record's sheet boundary.
				withoutContext++
			}
		}
		// Keep an explicit boundary between prose and the atomic media body,
		// including captionless images; do not glue words or HTML to prose.
		if unit.ContextAbove != "" {
			records[i].contextAbove = unit.ContextAbove + "\n"
		}
		if unit.ContextBelow != "" {
			records[i].contextBelow = "\n" + unit.ContextBelow
		}
	}
}
