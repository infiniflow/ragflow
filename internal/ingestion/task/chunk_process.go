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

package task

import (
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

// RenameTextToContentWithWeight renames the "text" key to "content_with_weight".
// If "content_with_weight" already exists, the "text" key is simply removed.
// Mirrors Python: ck["content_with_weight"] = ck["text"]; del ck["text"]
func RenameTextToContentWithWeight(chunk map[string]any) {
	if _, exists := chunk["content_with_weight"]; !exists {
		if text, ok := chunk["text"]; ok {
			chunk["content_with_weight"] = text
		}
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
func ProcessChunksForPipeline(
	chunks []map[string]any,
	docID string,
	kbID string,
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
		ck["kb_id"] = []string{kbID}
		ck["docnm_kwd"] = docName
		ck["create_time"] = timeStr
		ck["create_timestamp_flt"] = timestamp

		if _, exists := ck["id"]; !exists {
			text, _ := ck["text"].(string)
			ck["id"] = common.ChunkID(docID, text)
		}

		cleanupConsumedChunkFields(ck)
		metadata = mergeChunkMetadata(metadata, ck)
		RenameTextToContentWithWeight(ck)
		processChunkPositions(ck)
		removeInternalChunkFields(ck)
	}
	return metadata, nil
}

func removeInternalChunkFields(ck map[string]any) {
	delete(ck, "_pdf_positions")
	delete(ck, "image")
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

func mergeChunkMetadata(metadata map[string]any, ck map[string]any) map[string]any {
	metaVal, exists := ck["metadata"]
	if !exists {
		return metadata
	}
	if metaMap, ok := metaVal.(map[string]any); ok {
		metadata = utility.UpdateMetadataTo(metadata, metaMap)
	}
	delete(ck, "metadata")
	return metadata
}

// processChunkPositions converts the raw "positions" field into indexable
// position fields (page_num_int, top_int, position_int) via AddPositions,
// then removes the raw field.
//
// Two source types reach this point:
//   - []float64 — flat array of 5-tuples [page,left,right,top,bottom,…] from
//     parsers that emit positions directly as a flat float64 slice.
//   - [][]float64 — the production path: positions flow through ChunkDoc
//     (json.RawMessage → decodeStructuredValue) which produces a slice of
//     5-element groups.
//
// Both are flattened into a single []float64 for AddPositions, which groups
// by 5 internally. Unexpected types are logged and discarded.
func processChunkPositions(ck map[string]any) {
	poss, exists := ck["positions"]
	if !exists {
		return
	}
	switch v := poss.(type) {
	case []float64:
		AddPositions(ck, v)
	case [][]float64:
		flat := make([]float64, 0, len(v)*5)
		for _, group := range v {
			flat = append(flat, group...)
		}
		AddPositions(ck, flat)
	default:
		common.Warn(fmt.Sprintf("chunk positions unexpected type %T; discarding", poss))
	}
	delete(ck, "positions")
}

// AggregateTableDocMetadata collects unique per-column values across all chunks
// for columns with role "metadata" or "both", merges them into document metadata.
// Mirrors Python: rag/utils/table_es_metadata.py:aggregate_table_doc_metadata
func AggregateTableDocMetadata(chunks []map[string]any, parserConfig map[string]interface{}) map[string]any {
	mode, roles, tableColumnNames := resolveTableColumnConfig(parserConfig)
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "manual" {
		return nil
	}
	if roles == nil {
		roles = map[string]interface{}{}
	}
	var metaCols []string
	if len(tableColumnNames) > 0 {
		for _, n := range tableColumnNames {
			col, _ := n.(string)
			if col == "" {
				continue
			}
			role, _ := roles[col].(string)
			if role == "" {
				role = "both"
			}
			if role == "metadata" || role == "both" {
				metaCols = append(metaCols, col)
			}
		}
	} else {
		for col, v := range roles {
			role, _ := v.(string)
			if role == "metadata" || role == "both" {
				metaCols = append(metaCols, col)
			}
		}
	}
	if len(metaCols) == 0 {
		return nil
	}

	acc := make(map[string]map[string]struct{}, len(metaCols))
	for _, col := range metaCols {
		acc[col] = make(map[string]struct{})
	}
	for _, ck := range chunks {
		cd, _ := ck["chunk_data"].(map[string]interface{})
		if cd == nil {
			continue
		}
		for _, col := range metaCols {
			val, ok := cd[col]
			if !ok {
				continue
			}
			s, _ := val.(string)
			if s == "" {
				continue
			}
			acc[col][s] = struct{}{}
		}
	}

	out := make(map[string]any, len(acc))
	for col, vals := range acc {
		if len(vals) == 0 {
			continue
		}
		deduped := make([]string, 0, len(vals))
		for v := range vals {
			deduped = append(deduped, v)
		}
		out[col] = deduped
	}
	return out
}

// resolveTableColumnConfig reads table column settings from parser_config.
// Tries root-level flat keys first; falls back to resolving from a Parser
// component entry's spreadsheet config in a component-ID-keyed parser_config.
func resolveTableColumnConfig(parserConfig map[string]interface{}) (mode string, roles map[string]interface{}, names []interface{}) {
	mode, _ = parserConfig["table_column_mode"].(string)
	roles, _ = parserConfig["table_column_roles"].(map[string]interface{})
	rawNames, _ := parserConfig["table_column_names"].([]interface{})
	if mode != "" || roles != nil || len(rawNames) > 0 {
		return mode, roles, rawNames
	}
	for cid, raw := range parserConfig {
		if !strings.HasPrefix(cid, "Parser:") {
			continue
		}
		comp, _ := raw.(map[string]interface{})
		if comp == nil {
			continue
		}
		ss, _ := comp["spreadsheet"].(map[string]interface{})
		if ss == nil {
			continue
		}
		if v, ok := ss["column_mode"].(string); ok {
			mode = v
		}
		if v, ok := ss["column_roles"].(map[string]interface{}); ok {
			roles = v
		}
		if v, ok := ss["column_names"].([]interface{}); ok {
			rawNames = v
		}
		return mode, roles, rawNames
	}
	return "", nil, nil
}
