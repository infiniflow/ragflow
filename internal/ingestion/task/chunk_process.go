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
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"

	"github.com/pkoukk/tiktoken-go"
)

var keywordsSplitRE = regexp.MustCompile(`[,，;；、\r\n]+`)

// TruncateTexts truncates each text by token count using cl100k_base encoding.
// maxLength is reduced by 10 as a safety margin, matching Python.
// Mirrors Python: EmbeddingUtils.truncate_texts()
func TruncateTexts(texts []string, maxLength int) []string {
	if texts == nil {
		return nil
	}
	safeMax := maxLength - 10
	if safeMax < 0 {
		safeMax = 0
	}
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fallback: if tiktoken fails, return as-is
		result := make([]string, len(texts))
		copy(result, texts)
		return result
	}
	result := make([]string, len(texts))
	for i, t := range texts {
		tokens := enc.Encode(t, nil, nil)
		if len(tokens) > safeMax {
			result[i] = enc.Decode(tokens[:safeMax])
		} else {
			result[i] = t
		}
	}
	return result
}

// SplitQuestions splits a questions string by newline, keeping all elements.
// Mirrors Python: ck["questions"].split("\n") — keeps empty strings
func SplitQuestions(questions string) []string {
	return strings.Split(questions, "\n")
}

// SplitKeywords splits a keywords string by common delimiters.
// Mirrors Python: re.split(r"[,，;；、\r\n]+", keywords)
func SplitKeywords(keywords string) []string {
	if keywords == "" {
		return nil
	}
	parts := keywordsSplitRE.Split(keywords, -1)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// CreateChunkTime returns the current timestamp as a formatted string and float Unix timestamp.
// The float has sub-second precision, matching Python: datetime.now().timestamp()
func CreateChunkTime() (string, float64) {
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:04:05")
	return timeStr, float64(now.UnixMicro()) / 1e6
}

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
// the pipeline and returns merged metadata.
func ProcessChunksForPipeline(
	chunks []map[string]any,
	docID string,
	kbID string,
	docName string,
	now time.Time,
) map[string]any {
	if chunks == nil {
		return nil
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
			text, err := MustGetChunkTextString(ck)
			if err != nil {
				common.Error("unexpected error", err)
			}
			ck["id"] = ChunkID(text, docID)
		}

		processChunkQuestions(ck)
		processChunkKeywords(ck)
		processChunkSummary(ck)
		metadata = mergeChunkMetadata(metadata, ck)
		RenameTextToContentWithWeight(ck)
		processChunkPositions(ck)
		removeInternalChunkFields(ck)
	}
	return metadata
}

func removeInternalChunkFields(ck map[string]any) {
	delete(ck, "_pdf_positions")
	delete(ck, "image")
}

func processChunkQuestions(ck map[string]any) {
	if _, exists := ck["questions"]; !exists {
		return
	}
	if _, hasTks := ck["question_tks"]; !hasTks {
		q, _ := ck["questions"].(string)
		ck["question_kwd"] = strings.Split(q, "\n")
		tks, err := tokenizer.Tokenize(q)
		if err == nil {
			ck["question_tks"] = tks
		} else {
			ck["question_tks"] = q
		}
	}
	delete(ck, "questions")
}

func processChunkKeywords(ck map[string]any) {
	if _, exists := ck["keywords"]; !exists {
		return
	}
	if _, hasTks := ck["important_tks"]; !hasTks {
		kws, _ := ck["keywords"].(string)
		ck["important_kwd"] = SplitKeywords(kws)
		tks, err := tokenizer.Tokenize(kws)
		if err == nil {
			ck["important_tks"] = tks
		} else {
			ck["important_tks"] = kws
		}
	}
	delete(ck, "keywords")
}

func processChunkSummary(ck map[string]any) {
	if _, exists := ck["summary"]; !exists {
		return
	}
	if _, hasLtks := ck["content_ltks"]; !hasLtks {
		smmry, _ := ck["summary"].(string)
		ltks, err := tokenizer.Tokenize(smmry)
		if err == nil {
			ck["content_ltks"] = ltks
		} else {
			ck["content_ltks"] = smmry
		}
		smLtks, err := tokenizer.FineGrainedTokenize(ck["content_ltks"].(string))
		if err == nil {
			ck["content_sm_ltks"] = smLtks
		} else {
			ck["content_sm_ltks"] = ck["content_ltks"].(string)
		}
	}
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

func processChunkPositions(ck map[string]any) {
	poss, exists := ck["positions"]
	if !exists {
		return
	}
	if positions, ok := poss.([]float64); ok {
		AddPositions(ck, positions)
	}
	delete(ck, "positions")
}
