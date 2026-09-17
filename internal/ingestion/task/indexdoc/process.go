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
	"sort"
	"strings"
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
		stripPipelineOnlyFields(ck)
		metadata = mergeChunkMetadata(metadata, ck)
		RenameTextToContentWithWeight(ck)
		processChunkPositions(ck)
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
	"table_id", "sheet", "sheet_index", "headers", "cells",
	"row_start", "row_end", "col_start", "col_end",
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
// position fields (page_num_int, top_int, position_int) via AddPositions,
// then removes the raw "positions" and the sibling "_pdf_positions" internal
// field. _pdf_positions is the parser-emitted position matrix consumed by
// the chunker's image-crop pass (pdfcrop_cgo.go); after the chunker it is
// dead weight with no index column, so it is pruned unconditionally —
// independent of "positions" and not gated on the early-return below.
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
	delete(ck, "_pdf_positions")
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
	profile := ResolveTableProfile(parserConfig)
	if profile == nil {
		profile = NewTableProfile(common.TableColumnModeAuto)
	}
	cols := profile.Columns
	if len(cols) == 0 {
		for _, ck := range chunks {
			if names, ok := ck["table_column_names"].([]string); ok && len(names) > 0 {
				cols = names
				break
			}
			if names, ok := ck["table_column_names"].([]interface{}); ok && len(names) > 0 {
				for _, n := range names {
					if s, ok := n.(string); ok {
						cols = append(cols, s)
					}
				}
				break
			}
		}
	}
	if len(cols) == 0 && !isManualProfile(profile) {
		seen := make(map[string]struct{})
		for _, ck := range chunks {
			if cd, ok := ck["chunk_data"].(map[string]interface{}); ok {
				for k := range cd {
					if _, ok := seen[k]; !ok {
						seen[k] = struct{}{}
						cols = append(cols, k)
					}
				}
			}
		}
	}

	var metaCols []string
	if len(cols) > 0 {
		// The column name is the chunk_data key, so it is matched verbatim;
		// trimming or dropping a blank one here would silently exclude a column
		// that exists (rag/utils/table_es_metadata.py:187).
		for _, col := range cols {
			role := roleFor(profile, col)
			if role == common.ColumnRoleMetadata || role == common.ColumnRoleBoth {
				metaCols = append(metaCols, col)
			}
		}
	} else if len(profile.Roles) > 0 {
		for col, role := range profile.Roles {
			if role == common.ColumnRoleMetadata || role == common.ColumnRoleBoth {
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

// ResolveTableProfile extracts the strongly-typed TableProfile from parser_config.
// Tries root-level flat keys first; falls back to the first Parser component, in
// ascending component-id order, whose spreadsheet config actually states a mode,
// a role or a column.
func ResolveTableProfile(parserConfig map[string]interface{}) *TableProfile {
	if parserConfig == nil {
		return nil
	}
	modeStr, _ := parserConfig["table_column_mode"].(string)
	roles, rawRoles := parseTableColumnRoles(parserConfig["table_column_roles"])
	names := parseTableColumnNames(parserConfig["table_column_names"])
	if modeStr != "" || len(roles) > 0 || len(names) > 0 {
		return &TableProfile{
			Mode:     common.NormalizeTableColumnMode(modeStr),
			Roles:    roles,
			RawRoles: rawRoles,
			Columns:  names,
		}
	}
	for _, cid := range sortedParserComponentIDs(parserConfig) {
		comp, _ := parserConfig[cid].(map[string]interface{})
		if comp == nil {
			continue
		}
		ss, _ := comp["spreadsheet"].(map[string]interface{})
		if ss == nil {
			continue
		}
		mode := modeStr
		if v, ok := ss["column_mode"].(string); ok {
			mode = v
		}
		ssRoles, ssRawRoles := parseTableColumnRoles(ss["column_roles"])
		ssNames := parseTableColumnNames(ss["column_names"])
		// An entry that states nothing is not a profile: a canvas can carry a
		// spreadsheet block for a component the user never configured, and
		// stopping there would hide the profile a later component does declare.
		// Same gate as the root level above.
		if mode == "" && len(ssRoles) == 0 && len(ssNames) == 0 {
			continue
		}
		return &TableProfile{
			Mode:     common.NormalizeTableColumnMode(mode),
			Roles:    ssRoles,
			RawRoles: ssRawRoles,
			Columns:  ssNames,
		}
	}
	return nil
}

// ResolveTableColumnConfig reads table column settings from parser_config.
// Tries root-level flat keys first; falls back to resolving from a Parser
// component entry's spreadsheet config in a component-ID-keyed parser_config.
func ResolveTableColumnConfig(parserConfig map[string]interface{}) (mode string, roles map[string]interface{}, names []interface{}) {
	profile := ResolveTableProfile(parserConfig)
	if profile == nil {
		return "", nil, nil
	}
	mode = string(profile.Mode)
	roles = profile.ToRolesInterfaceMap()
	if len(profile.Columns) > 0 {
		names = make([]interface{}, len(profile.Columns))
		for i, c := range profile.Columns {
			names[i] = c
		}
	}
	return mode, roles, names
}

func parseTableColumnRoles(raw any) (map[string]common.ColumnRole, map[string]any) {
	if raw == nil {
		return nil, nil
	}
	switch m := raw.(type) {
	case map[string]common.ColumnRole:
		if len(m) == 0 {
			return nil, nil
		}
		rawMap := make(map[string]any, len(m))
		for k, v := range m {
			rawMap[k] = string(v)
		}
		return m, rawMap
	case map[string]string:
		if len(m) == 0 {
			return nil, nil
		}
		out := make(map[string]common.ColumnRole, len(m))
		rawMap := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = common.NormalizeColumnRole(v)
			rawMap[k] = v
		}
		return out, rawMap
	case map[string]interface{}:
		if len(m) == 0 {
			return nil, nil
		}
		out := make(map[string]common.ColumnRole, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = common.NormalizeColumnRole(s)
			}
		}
		return out, m
	default:
		return nil, nil
	}
}

func parseTableColumnNames(raw any) []string {
	if raw == nil {
		return nil
	}
	switch list := raw.(type) {
	case []string:
		return list
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			// Verbatim, including a blank name: table_column_names carries the
			// columns as the parser named them, and that exact string is both the
			// key a role is looked up under and the key chunk_data is stored with
			// (rag/app/table.py:675-678 writing what :594 built).
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// sortedParserComponentIDs returns the Parser:-prefixed component ids in
// ascending order so nested-config resolution is deterministic.
func sortedParserComponentIDs(parserConfig map[string]interface{}) []string {
	var ids []string
	for cid := range parserConfig {
		if strings.HasPrefix(cid, "Parser:") {
			ids = append(ids, cid)
		}
	}
	sort.Strings(ids)
	return ids
}

// TableParserStripDocMetadataKeys returns the keys to strip from existing document metadata
// on reparse.
func TableParserStripDocMetadataKeys(parserConfig map[string]interface{}) []string {
	profile := ResolveTableProfile(parserConfig)
	if profile == nil {
		return nil
	}
	if len(profile.Columns) > 0 {
		seen := make(map[string]struct{}, len(profile.Columns))
		keys := make([]string, 0, len(profile.Columns))
		for _, s := range profile.Columns {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				keys = append(keys, s)
			}
		}
		return keys
	}
	if len(profile.Roles) > 0 {
		keys := make([]string, 0, len(profile.Roles))
		for k := range profile.Roles {
			s := strings.TrimSpace(k)
			if s != "" {
				keys = append(keys, s)
			}
		}
		return keys
	}
	return nil
}
