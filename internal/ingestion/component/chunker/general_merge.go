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

package chunker

import (
	"encoding/json"
	"regexp"
	"strings"

	"ragflow/internal/ingestion/component/schema"
)

// mergeGeneralUnits implements the general OVER_CAP policy. A text unit is
// appended while the current chunk is at or below the overlap-scaled target;
// the append itself may cross the target. Oversized units remain indivisible.
func mergeGeneralUnits(units []schema.ChunkDoc, target int, overlapPct float64, joinSep string) []schema.ChunkDoc {
	overlapPct = max(0, min(100, overlapPct))
	threshold := float64(target) * (100 - overlapPct) / 100
	merged := make([]schema.ChunkDoc, 0, len(units))
	current := -1

	for _, unit := range units {
		if itemDocType(unit) != "text" {
			merged = append(merged, cloneChunkDoc(unit))
			current = -1
			continue
		}
		if strings.TrimSpace(unit.Text) == "" {
			continue
		}

		count := generalUnitTokens(unit)
		unit.DocType = "text"
		unit.CKType = "text"
		unit.TKNums = intPtr(count)
		if count > target {
			merged = append(merged, cloneChunkDoc(unit))
			current = -1
			continue
		}
		if current < 0 || float64(intValue(merged[current].TKNums)) > threshold {
			merged = append(merged, cloneChunkDoc(unit))
			current = len(merged) - 1
			continue
		}

		mergeGeneralChunk(&merged[current], unit, joinSep)
	}

	return applyGeneralOverlap(merged, overlapPct)
}

func generalUnitTokens(unit schema.ChunkDoc) int {
	if unit.TKNums != nil && *unit.TKNums > 0 {
		return *unit.TKNums
	}
	return tokenizeStr(unit.Text)
}

func mergeGeneralChunk(dst *schema.ChunkDoc, src schema.ChunkDoc, joinSep string) {
	if dst.Text != "" && src.Text != "" {
		dst.Text += joinSep
	}
	dst.Text += src.Text
	dst.TKNums = intPtr(intValue(dst.TKNums) + generalUnitTokens(src))
	dst.PDFPositions = mergeGeneralPositions(dst.PDFPositions, src.PDFPositions)
	dst.Positions = mergeGeneralPositions(dst.Positions, src.Positions)
	mergeGeneralMetadata(dst, src)
}

func mergeGeneralMetadata(dst *schema.ChunkDoc, src schema.ChunkDoc) {
	if dst.Image == "" {
		dst.Image = src.Image
	}
	if dst.ImgID == "" {
		dst.ImgID = src.ImgID
	}
	if dst.Layout == "" {
		dst.Layout = src.Layout
	}
	if dst.LayoutType == "" {
		dst.LayoutType = src.LayoutType
	}
	if dst.LayoutNo == "" {
		dst.LayoutNo = src.LayoutNo
	}
	if dst.PageNumber == nil {
		dst.PageNumber = src.PageNumber
	}
	if dst.Extra == nil && len(src.Extra) > 0 {
		dst.Extra = make(map[string]json.RawMessage, len(src.Extra))
	}
	for key, value := range src.Extra {
		if _, exists := dst.Extra[key]; !exists {
			dst.Extra[key] = append(json.RawMessage(nil), value...)
		}
	}
}

func mergeGeneralPositions(first, second json.RawMessage) json.RawMessage {
	if positions := mergePositionMatrix(first, second); len(positions) > 0 {
		encoded, err := json.Marshal(positions)
		if err == nil {
			return encoded
		}
	}
	return extendRawJSONArray(first, second)
}

func applyGeneralOverlap(chunks []schema.ChunkDoc, overlapPct float64) []schema.ChunkDoc {
	if overlapPct <= 0 {
		return chunks
	}
	previousText := -1
	for i := range chunks {
		if itemDocType(chunks[i]) != "text" {
			previousText = -1
			continue
		}
		if previousText >= 0 {
			prefix, _ := computeOverlapPrefix(chunks[previousText].Text, overlapPct)
			if prefix != "" {
				current := cloneChunkDoc(chunks[i])
				current.Text = prefix + current.Text
				current.TKNums = intPtr(tokenizeStr(current.Text))
				current.PDFPositions = mergeGeneralPositions(chunks[previousText].PDFPositions, current.PDFPositions)
				current.Positions = mergeGeneralPositions(chunks[previousText].Positions, current.Positions)
				mergeGeneralMetadata(&current, chunks[previousText])
				chunks[i] = current
			}
		}
		previousText = i
	}
	return chunks
}

func splitGeneralUnits(units []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	result := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		unit = cloneChunkDoc(unit)
		unit.Text = normalizeGeneralNewlines(itemTextOrFallback(unit))
		unit.DocType = itemDocType(unit)
		unit.CKType = unit.DocType
		if unit.DocType != "text" || pattern == nil || !pattern.MatchString(unit.Text) {
			unit.TKNums = intPtr(tokenizeStr(unit.Text))
			result = append(result, unit)
			continue
		}
		for _, part := range splitDroppingDelim(unit.Text, pattern) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			piece := cloneChunkDoc(unit)
			piece.Text = part
			piece.TKNums = intPtr(tokenizeStr(part))
			result = append(result, piece)
		}
	}
	return result
}

func normalizeGeneralNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func finalizeGeneralChunks(chunks []schema.ChunkDoc, childrenPattern *regexp.Regexp) []schema.ChunkDoc {
	visible := make([]schema.ChunkDoc, 0, len(chunks))
	for _, chunk := range chunks {
		chunk.Text = removeTag(chunk.Text)
		if strings.TrimSpace(chunk.Text) == "" && chunk.Image == "" {
			continue
		}
		visible = append(visible, chunk)
	}
	visible = splitByChildren(visible, childrenPattern)
	for i := range visible {
		visible[i].TKNums = intPtr(tokenizeStr(visible[i].Text))
	}
	return visible
}
