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

package indexdoc

import (
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

// RenameTextToContentWithWeight maps the canonical pre-index "text" field to the
// storage field "content_with_weight" and removes "text". The text value is
// always authoritative at this boundary — any pre-existing content_with_weight
// is overwritten so identity/embedding and persisted content cannot diverge.
func RenameTextToContentWithWeight(chunk map[string]any) {
	if text, ok := chunk["text"].(string); ok {
		chunk["content_with_weight"] = text
	}
	delete(chunk, "text")
}

// GetEmbeddingTokenConsumption extracts the embedding token consumption from pipeline output.
// Handles both int (Go native) and float64 (after JSON round-trip).
func GetEmbeddingTokenConsumption(output map[string]any) int {
	if output == nil {
		return 0
	}
	switch v := output[EmbeddingTokenConsumptionKey].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		common.Warn(fmt.Sprintf("unexpected type %T for embedding token consumption, key=%q", v, EmbeddingTokenConsumptionKey))
		return 0
	}
}

// ProcessChunksForPipeline mutates chunks into the pre-index structure used by
// the pipeline and returns merged metadata. It returns an error if a chunk's
// "text" field is present but not a string: that is an upstream contract
// violation (the chunker/parser must emit string text), and continuing would
// collapse every such chunk onto the same empty-text ChunkID, silently
// overwriting each other in the index. The caller fails the task so the
// violation surfaces instead of corrupting the index.
//
// Note: kb_id is intentionally NOT stamped here. It is an index-physical
// concern owned by the search engine at the write boundary (ES chunk.go
// InsertChunks and Infinity chunk.go InsertChunks both set kb_id = datasetID),
// so ingestion never carries the index schema for it. See issue #17371.
func ProcessChunksForPipeline(
	chunks []map[string]any,
	docID string,
	docName string,
	now time.Time,
) (map[string]any, error) {
	if chunks == nil {
		return nil, nil
	}
	metadata := make(map[string]any)
	timeStr := now.Format("2006-01-02 15:04:05")
	timestamp := float64(now.UnixMicro()) / 1e6

	for _, ck := range chunks {
		ck["doc_id"] = docID
		ck["docnm_kwd"] = docName
		ck["create_time"] = timeStr
		ck["create_timestamp_flt"] = timestamp

		text, err := requireStringText(ck)
		if err != nil {
			return nil, err
		}

		if _, exists := ck["id"]; !exists {
			ck["id"] = common.ChunkID(docID, text)
		}

		cleanupConsumedChunkFields(ck)
		// The spreadsheet identity is keyed off fields the strip is about to
		// remove, so it must be read before it.
		spreadsheet := isSpreadsheetChunk(ck)
		stripPipelineOnlyFields(ck)
		metadata = mergeChunkMetadata(metadata, ck)
		RenameTextToContentWithWeight(ck)
		processChunkPositions(ck, spreadsheet)
	}
	return metadata, nil
}

// requireStringText enforces the pre-index wire contract: every chunk must
// carry a string "text" field before chunk-id generation or persistence mapping.
func requireStringText(ck map[string]any) (string, error) {
	textRaw, exists := ck["text"]
	if !exists {
		return "", fmt.Errorf("chunk missing required string text field")
	}
	text, ok := textRaw.(string)
	if !ok {
		return "", fmt.Errorf("chunk text must be string, got %T", textRaw)
	}
	return text, nil
}

// cleanupConsumedChunkFields materializes the stored array form of the
// questions/keywords fields (when the upstream Tokenizer did not already set
// them) and strips the consumed source fields (questions/keywords/summary)
// before persist. This is persist-schema mapping, NOT linguistic tokenization:
// question_tks / important_tks / content_ltks are owned by the Tokenizer
// component; the executor no longer falls back to producing them.
func cleanupConsumedChunkFields(ck map[string]any) {
	if q, ok := ck["questions"].(string); ok {
		if _, has := ck["question_kwd"]; !has {
			ck["question_kwd"] = utility.SplitQuestions(q)
		}
	}
	delete(ck, "questions")

	if kws, ok := ck["keywords"].(string); ok {
		if _, has := ck["important_kwd"]; !has {
			ck["important_kwd"] = utility.SplitKeywords(kws)
		}
	}
	delete(ck, "keywords")

	delete(ck, "summary")
}

// pipelineOnlyFields are the parser/chunker BOOKKEEPING keys: each one is
// consumed inside the pipeline (ck_type drives chunk merging and the image-crop
// decision, tk_nums carries the chunker's token count into the Tokenizer,
// layout*/image/context_* describe the media block, and page_number/table_id/
// sheet/headers/cells describe the table or spreadsheet block) and NONE of them
// is a chunk-store column.
//
// The Python index doc carries none of them — its chunk builder emits only the
// persist fields — but Go's chunker hands them on, and the write boundary is
// strict about unknown columns: Infinity rejects the whole insert with
// "Column ck_type not found in table" (InfinityException 3013). Elasticsearch
// merely swallowed them, because a dynamic mapping accepts any field.
var pipelineOnlyFields = []string{
	"ck_type", "tk_nums", "layout", "layout_type", "layoutno", "image",
	"context_above", "context_below", "page_number",
	"sheet", "sheet_index",
	// The row markers become index columns together with the structured SQL
	// path; until a backend has them, they are bookkeeping only, and Infinity
	// rejects an insert that names a column its table does not have.
	"table_row_source", "table_row_int", "table_profile_key",
}

// stripPipelineOnlyFields drops those bookkeeping keys at the index boundary,
// leaving the chunk with the persist schema only.
func stripPipelineOnlyFields(ck map[string]any) {
	for _, key := range pipelineOnlyFields {
		delete(ck, key)
	}
}

func mergeChunkMetadata(metadata map[string]any, ck map[string]any) map[string]any {
	metaVal, exists := ck["metadata"]
	if !exists {
		return metadata
	}
	metaMap, ok := metaVal.(map[string]any)
	if !ok {
		// Contract: ck["metadata"] is produced by the Extractor component and
		// is always a map[string]any (the merge of enable_metadata + the
		// field_name="metadata" extraction). A non-map value signals an
		// upstream bug, not a value to guess-parse — record and drop it.
		common.Warn(fmt.Sprintf("mergeChunkMetadata: chunk metadata is %T, want map[string]any; dropping", metaVal))
		delete(ck, "metadata")
		return metadata
	}
	metadata = utility.UpdateMetadataTo(metadata, metaMap)
	delete(ck, "metadata")
	return metadata
}

// processChunkPositions converts the raw "positions" field into indexable
// position fields, then removes the raw "positions" and the sibling
// "_pdf_positions" internal field. _pdf_positions is the parser-emitted
// position matrix consumed by the chunker's image-crop pass (pdfcrop_cgo.go);
// after the chunker it is dead weight with no index column, so it is pruned
// unconditionally — independent of "positions" and not gated on the
// early-return below.
//
// The field carries two vocabularies, and the branch on them is explicit
// rather than inferred from the shape:
//   - PDF items: [page, left, right, top, bottom] → page_num_int, top_int and
//     position_int (addPDFPositions);
//   - spreadsheet items (identity present): [sheet, rowStart, rowEnd,
//     colStart, colEnd] → position_int only (addSpreadsheetPositions), which
//     leaves the chunk's own top_int — the QA chunker's row index — intact.
//
// Two source types reach this point:
//   - []float64 — flat array of 5-tuples from parsers that emit positions
//     directly as a flat float64 slice.
//   - [][]float64 — the production path: positions flow through ChunkDoc
//     (json.RawMessage → decodeStructuredValue) which produces a slice of
//     5-element groups.
//
// Both are flattened into a single []float64, which the store helpers group by
// five. Unexpected types are logged and discarded.
func processChunkPositions(ck map[string]any, spreadsheet bool) {
	delete(ck, "_pdf_positions")
	poss, exists := ck["positions"]
	if !exists {
		return
	}
	switch v := poss.(type) {
	case []float64:
		storePositions(ck, v, spreadsheet)
	case [][]float64:
		flat := make([]float64, 0, len(v)*5)
		for _, group := range v {
			flat = append(flat, group...)
		}
		storePositions(ck, flat, spreadsheet)
	default:
		common.Warn(fmt.Sprintf("chunk positions unexpected type %T; discarding", poss))
	}
	delete(ck, "positions")
}

func storePositions(ck map[string]any, flat []float64, spreadsheet bool) {
	if spreadsheet {
		addSpreadsheetPositions(ck, flat)
		return
	}
	addPDFPositions(ck, flat)
}

// isSpreadsheetChunk reports whether a chunk's "positions" carry the
// spreadsheet vocabulary, judged by the identity fields the strip removes
// later in the chunk loop.
func isSpreadsheetChunk(ck map[string]any) bool {
	if v, ok := ck["sheet_index"]; ok && v != nil {
		return true
	}
	sheet, ok := ck["sheet"].(string)
	return ok && sheet != ""
}
