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

package service

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
)

// Knowledgebase type alias for entity.Knowledgebase
type Knowledgebase = entity.Knowledgebase

// tagSourceRefs returns the tag source files configured on the queried KBs,
// deduplicated by (tenant, file). It mirrors Python's
// rag/app/tag.py::label_question collecting tag_kb_ids from the queried KBs:
// Python points at tag datasets, Go points at a tag source file (parser_config
// tags.tag_file_id) that is indexed in memory instead of a dataset.
//
// The last non-nil KB is returned so the caller can read topn_tags from it, as
// Python does with the loop's final kb.
func tagSourceRefs(kbs []*Knowledgebase) ([]component.TagSourceRef, *Knowledgebase) {
	seen := make(map[string]struct{}, len(kbs))
	refs := make([]component.TagSourceRef, 0, len(kbs))
	var last *Knowledgebase
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		last = kb
		fileID := component.TagFileIDFromParserConfig(map[string]any(kb.ParserConfig))
		if fileID == "" {
			continue
		}
		key := kb.TenantID + "\x00" + fileID
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, component.TagSourceRef{FileID: fileID, TenantID: kb.TenantID})
	}
	return refs, last
}

// topnTagsFromKB mirrors Python's parser_config.get("topn_tags", 3).
// JSON-decoded numbers arrive as float64; also tolerate int/int64/json.Number.
func topnTagsFromKB(kb *Knowledgebase) int {
	topnTags := 3
	if kb == nil || kb.ParserConfig == nil {
		return topnTags
	}
	switch v := kb.ParserConfig["topn_tags"].(type) {
	case float64:
		topnTags = int(v)
	case int:
		topnTags = v
	case int64:
		topnTags = int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			topnTags = int(n)
		}
	}
	return topnTags
}

// LabelQuestion returns weighted tag rank features for a question, the Go
// equivalent of Python's rag/app/tag.py::label_question. Go has no tag
// dataset, so the vocabulary comes from the KBs' tag source files
// (parser_config tags.tag_file_id) indexed in memory: the question is matched
// against the file's records and the background proportions (Python's
// all_tags_in_portion) come from TagProportions. There is no tag_kwd column to
// aggregate - auto-tagging writes tag_feas.
//
// Logging: this runs once per retrieval, so the common no-tag-source case stays
// at debug to avoid flooding the chat path; only a successful match is logged at
// info, because that is the signal needed to confirm tagging took effect. The
// question itself is not logged here - callers already log it and it is user
// data; use common.IsDebugEnabled() below if it is ever needed.
func (s *MetadataService) LabelQuestion(ctx context.Context, question string, kbs []*Knowledgebase) map[string]float64 {
	if len(kbs) == 0 || question == "" {
		common.DebugCtx(ctx, "LabelQuestion skipped",
			zap.Int("kbCount", len(kbs)), zap.Bool("emptyQuestion", question == ""))
		return nil
	}
	refs, lastKB := tagSourceRefs(kbs)
	if len(refs) == 0 {
		// No tag source configured on any KB: the normal case, keep it quiet.
		common.DebugCtx(ctx, "LabelQuestion skipped: no tag source configured",
			zap.Int("kbCount", len(kbs)))
		return nil
	}
	idx, err := component.LoadTagQueryIndex(ctx, refs)
	if err != nil {
		common.WarnCtx(ctx, "LabelQuestion: failed to load tag source index",
			zap.Int("tagSources", len(refs)), zap.Error(err))
		return nil
	}
	if idx == nil {
		common.DebugCtx(ctx, "LabelQuestion: tag source index unavailable",
			zap.Int("tagSources", len(refs)))
		return nil
	}
	topN := topnTagsFromKB(lastKB)
	// The index tokenizes the question with the analyzer it was built with, so
	// retrieval cannot drift onto a different one.
	tagFeatures := idx.MatchTagQuery(question, topN)
	if len(tagFeatures) == 0 {
		// Tags configured but the question matched none - return an empty map
		// (not nil) so the caller knows tagging was active for this dataset.
		common.DebugCtx(ctx, "LabelQuestion: no tag matched",
			zap.Int("tagSources", len(refs)), zap.Int("topnTags", topN))
		return make(map[string]float64)
	}
	common.InfoCtx(ctx, "LabelQuestion result",
		zap.Int("tagSources", len(refs)), zap.Int("topnTags", topN), zap.Any("labels", tagFeatures))
	return tagFeatures
}
