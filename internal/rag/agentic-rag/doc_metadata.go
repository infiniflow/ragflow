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

package agentic_rag

import (
	"context"
	"log"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// docMetaValueRunes caps one rendered metadata value. The evidence block is a
// per-passage token budget (evidenceBudgetTokens, graph_compose.go), so a long
// title or a stitched value must not spend it: the block carries the value as a
// pointer, not as reading material.
const docMetaValueRunes = 200

// attachDocMetadata stamps the DECLARED document-level metadata onto the pool's
// chunks, so that the final answer's evidence block carries each passage's
// document attributes (file name, update time, title, ...) beside its text.
//
// WHY the evidence block is the only place this can work. The answer prompt is
// built from two carriers (see answerPromptWithEvidence): the slot record, which
// is declared INTERNAL and explicitly NOT evidence, and the numbered evidence
// blocks, which are the only text the citation contract lets the answer rest on.
// Document attributes previously reached the run only through the slot table
// (metadata_search's result block is context for the DIRECTION sessions and
// produces no passages: runtime/tool_executor.go, EvidenceIDs nil). That left a
// two-hop question — "which documents, and what is in them" — with the
// membership in the record and the content in the evidence, and the record's own
// contract ("never quote these lines") weighing against the answer using it.
// Stamping the attributes onto the passages puts both halves in the same block.
//
// WHY the KB row and not the metadata resolver's declared-field read. The
// declared fields live in parser_config, and the run already holds the KB rows
// (RAGTools.KBs, loaded by the caller so this package stays DB-free). Reading
// them here is the same source the metadata_search catalog uses
// (common.DeclaredMetadataFieldsFromParserConfig covers parser_config.metadata
// AND built_in_metadata) with no extra index scan.
//
// WHY only declared fields. The doc-metadata index also carries extraction and
// annotation noise the dataset never declared — a metadata-extraction run that
// echoed a judge's rationale and verdict into the document's metadata is a real
// observed case — and an unfiltered dump would put it in every evidence block.
// A dataset that declares nothing therefore gets no block-level metadata at all,
// which is exactly the behaviour before this function existed.
//
// Best effort by contract: a missing resolver, unresolvable KB rows or a failed
// metadata read degrades to "no document metadata in the evidence" rather than
// failing the answer. The compose call must not depend on it.
func attachDocMetadata(ctx context.Context, deps RAGTools, kb *runtime.Kbinfos, logger *log.Logger) {
	if kb == nil || deps.MetadataResolver == nil || len(kb.Chunks) == 0 {
		return
	}
	keys := declaredDocMetadataKeys(deps.KBs)
	if len(keys) == 0 {
		return
	}
	datasetIDs := kbIDsOf(deps.KBs)
	if len(datasetIDs) == 0 {
		return
	}
	docIDs := pooledDocIDs(kb.Chunks)
	if len(docIDs) == 0 {
		return
	}
	metaByDoc, err := deps.MetadataResolver.MetadataForDocIDs(ctx, datasetIDs, docIDs)
	if err != nil {
		logger.Printf("[Formalize][docmeta] metadata read degraded: %v", err)
	}
	if len(metaByDoc) == 0 {
		return
	}
	stamped := 0
	for _, chunk := range kb.Chunks {
		meta := metaByDoc[chunkDocID(chunk)]
		if len(meta) == 0 {
			continue
		}
		picked := make(map[string]any, len(keys))
		for _, key := range keys {
			if text, ok := docMetaText(meta[key]); ok {
				picked[key] = text
			}
		}
		if len(picked) == 0 {
			continue
		}
		chunk["document_metadata"] = picked
		stamped++
	}
	logger.Printf("[Formalize][docmeta] declared=%v docs=%d chunks=%d/%d",
		keys, len(metaByDoc), stamped, len(kb.Chunks))
}

// declaredDocMetadataKeys returns the union of the datasets' declared metadata
// fields, deduped and sorted so the request and the rendered block are stable.
// Declaration order is per dataset and a run may span several, so the union is
// ordered here rather than left to map iteration.
func declaredDocMetadataKeys(kbs []*entity.Knowledgebase) []string {
	seen := make(map[string]bool, 8)
	keys := make([]string, 0, 8)
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		for _, def := range common.DeclaredMetadataFieldsFromParserConfig(kb.ParserConfig) {
			key := strings.TrimSpace(def.Key)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// kbIDsOf returns the datasets of the loaded KB rows, in the caller's order.
func kbIDsOf(kbs []*entity.Knowledgebase) []string {
	ids := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || strings.TrimSpace(kb.ID) == "" {
			continue
		}
		ids = append(ids, kb.ID)
	}
	return ids
}

// pooledDocIDs returns the distinct document ids the pool holds, sorted so the
// metadata read is deterministic (and so a whole-corpus lookup is one request).
func pooledDocIDs(chunks []map[string]any) []string {
	seen := make(map[string]bool, len(chunks))
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		id := chunkDocID(chunk)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// chunkDocID reads a chunk's DOCUMENT id.
//
// It deliberately does not fall back to "id": in a retrieval chunk that key is
// the CHUNK id (see service.SourcedChunk, which reads ID from chunk_id/id and
// DocID from doc_id/document_id), so keying the metadata lookup on it would ask
// the metadata index for a document that does not exist and stamp nothing.
func chunkDocID(chunk map[string]any) string {
	for _, key := range []string{"doc_id", "document_id"} {
		if id, ok := chunk[key].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// docMetaText renders one metadata value for an evidence block, or ok=false when
// the value has no flat reading there.
//
// Text (and a list of text, which the index writes for multi-valued fields) is
// what a "key: value" line can carry. A structured value — a nested outline, a
// list of objects — is dropped rather than stringified into noise: the block is
// a pointer to the document, not a rendering of its metadata.
func docMetaText(value any) (string, bool) {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []string:
		text = strings.Join(typed, ", ")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		text = strings.Join(parts, ", ")
	default:
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return runtime.TruncateRunes(text, docMetaValueRunes), true
}
