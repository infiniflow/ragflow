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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// docMetadataResolver is a scripted runtime.MetadataResolver: it records the
// documents it was asked for and answers from a fixed doc_id → fields map.
type docMetadataResolver struct {
	meta      map[string]map[string]any
	requested []string
	askedKBs  []string
	calls     int
}

func (r *docMetadataResolver) FilterDocIDsByMetaPushdown(context.Context, []string, []map[string]any, string) ([]string, bool) {
	return nil, false
}

func (r *docMetadataResolver) GetFlattedMetaByKBs(context.Context, []string) (common.MetaData, error) {
	return common.MetaData{}, nil
}

func (r *docMetadataResolver) MetadataForDocIDs(_ context.Context, kbIDs, docIDs []string) (map[string]map[string]any, error) {
	r.calls++
	r.askedKBs = append([]string(nil), kbIDs...)
	r.requested = append([]string(nil), docIDs...)
	return r.meta, nil
}

// declaringKB is a dataset row whose parser_config declares one extracted field
// and two built-in ones — the shape the metadata config API writes.
func declaringKB() []*entity.Knowledgebase {
	return []*entity.Knowledgebase{{
		ID: "kb1",
		ParserConfig: entity.JSONMap{
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "title", "type": "string"},
				},
				"built_in_metadata": []any{
					map[string]any{"key": "update_time", "type": "time"},
					map[string]any{"key": "file_name", "type": "string"},
				},
			},
		},
	}}
}

// composePrompt runs the terminal composition and returns the user prompt the
// model received — the text the evidence blocks are rendered into.
func composePrompt(t *testing.T, deps RAGTools, kb *runtime.Kbinfos) string {
	t.Helper()
	model := &fakeModel{replies: []*runtime.ModelReply{{Content: "answer"}}}
	deps.Model = model
	composeFinalAnswer(context.Background(), deps, runtime.RunRequest{Question: "q"}, kb,
		&RunResponse{}, _LOG, false, false, "q")
	return model.lastUserPrompt()
}

// TestAttachDocMetadataStampsDeclaredFieldsOnly pins both halves of the write:
// the DECLARED fields reach the evidence block, and a field the dataset never
// declared does not. The undeclared half is the observed metadata-extraction
// leak (a judge's rationale landing in a document's metadata) — an unfiltered
// dump would put it in every evidence block.
func TestAttachDocMetadataStampsDeclaredFieldsOnly(t *testing.T) {
	resolver := &docMetadataResolver{meta: map[string]map[string]any{
		"doc1": {
			"update_time": "2026-09-20 13:55:35",
			"file_name":   "VecTree-RAG.pdf",
			"title":       "VecTree-RAG",
			"rationale":   "an undeclared field an extraction run leaked",
		},
	}}
	kb := &runtime.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "doc1", "content": "the passage body", "similarity": 0.9},
	}}
	deps := RAGTools{KBs: declaringKB(), MetadataResolver: resolver}

	prompt := composePrompt(t, deps, kb)

	for _, want := range []string{
		"\u251c\u2500\u2500 update_time: 2026-09-20 13:55:35",
		"\u251c\u2500\u2500 file_name: VecTree-RAG.pdf",
		"\u251c\u2500\u2500 title: VecTree-RAG",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("evidence block missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "rationale") {
		t.Errorf("an undeclared field reached the evidence block:\n%s", prompt)
	}
	if resolver.calls != 1 || len(resolver.requested) != 1 || resolver.requested[0] != "doc1" {
		t.Errorf("metadata read = %d call(s) for %v, want one read for [doc1]", resolver.calls, resolver.requested)
	}
	if len(resolver.askedKBs) != 1 || resolver.askedKBs[0] != "kb1" {
		t.Errorf("metadata read datasets = %v, want the declaring dataset", resolver.askedKBs)
	}
}

// TestAttachDocMetadataLeavesEvidenceUnchangedWithoutDeclarations pins the two
// gates that keep this a no-op where it cannot help: no resolver, and no
// declared fields. The second one must also skip the read — a dataset that
// declares nothing has no field to put on a block, so the round-trip would be
// paid for an empty map.
func TestAttachDocMetadataLeavesEvidenceUnchangedWithoutDeclarations(t *testing.T) {
	newKB := func() *runtime.Kbinfos {
		return &runtime.Kbinfos{Chunks: []map[string]any{
			{"chunk_id": "c1", "doc_id": "doc1", "content": "the passage body", "similarity": 0.9},
		}}
	}
	resolver := &docMetadataResolver{meta: map[string]map[string]any{
		"doc1": {"update_time": "2026-09-20 13:55:35"},
	}}

	noResolver := composePrompt(t, RAGTools{KBs: declaringKB()}, newKB())
	noDeclarations := composePrompt(t, RAGTools{MetadataResolver: resolver}, newKB())

	for name, prompt := range map[string]string{"no resolver": noResolver, "no declarations": noDeclarations} {
		if strings.Contains(prompt, "document_metadata") || strings.Contains(prompt, "update_time") {
			t.Errorf("%s: evidence block grew a metadata line:\n%s", name, prompt)
		}
	}
	if resolver.calls != 0 {
		t.Errorf("metadata read = %d call(s), want none without a declared field", resolver.calls)
	}
}

// TestAttachDocMetadataTruncatesLongValues pins the budget guard: a long value
// is rendered as a pointer to the document, not as reading material, because
// every metadata line spends the evidence block's token budget.
func TestAttachDocMetadataTruncatesLongValues(t *testing.T) {
	long := strings.Repeat("x", docMetaValueRunes+50)
	resolver := &docMetadataResolver{meta: map[string]map[string]any{
		"doc1": {"file_name": long},
	}}
	kb := &runtime.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "doc1", "content": "the passage body", "similarity": 0.9},
	}}

	prompt := composePrompt(t, RAGTools{KBs: declaringKB(), MetadataResolver: resolver}, kb)

	if !strings.Contains(prompt, strings.Repeat("x", docMetaValueRunes)) {
		t.Errorf("evidence block lost the truncated value:\n%s", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("x", docMetaValueRunes+1)) {
		t.Errorf("a value longer than %d runes reached the evidence block", docMetaValueRunes)
	}
}

// TestAttachDocMetadataKeysOnDocID pins the id semantics: a retrieval chunk's
// "id" is its CHUNK id, so keying the metadata lookup on it would ask the index
// for a document that does not exist. The lookup must use doc_id, and a resolver
// that only knows the chunk id must stamp nothing.
func TestAttachDocMetadataKeysOnDocID(t *testing.T) {
	chunk := func() map[string]any {
		// "id" and "doc_id" disagree on purpose: that is the shape a retrieval
		// result has (service.SourcedChunk reads ID from chunk_id/id).
		return map[string]any{
			"id": "chunk-1", "chunk_id": "chunk-1", "doc_id": "doc-1",
			"content": "the passage body", "similarity": 0.9,
		}
	}

	byDocID := &docMetadataResolver{meta: map[string]map[string]any{
		"doc-1": {"update_time": "2026-09-20 13:55:35"},
	}}
	prompt := composePrompt(t, RAGTools{KBs: declaringKB(), MetadataResolver: byDocID},
		&runtime.Kbinfos{Chunks: []map[string]any{chunk()}})

	if len(byDocID.requested) != 1 || byDocID.requested[0] != "doc-1" {
		t.Fatalf("metadata read requested %v, want [doc-1] (the DOCUMENT id)", byDocID.requested)
	}
	if !strings.Contains(prompt, "update_time: 2026-09-20 13:55:35") {
		t.Errorf("evidence block missing the document's metadata:\n%s", prompt)
	}

	byChunkID := &docMetadataResolver{meta: map[string]map[string]any{
		"chunk-1": {"update_time": "2026-09-20 13:55:35"},
	}}
	other := composePrompt(t, RAGTools{KBs: declaringKB(), MetadataResolver: byChunkID},
		&runtime.Kbinfos{Chunks: []map[string]any{chunk()}})

	if strings.Contains(other, "update_time") {
		t.Errorf("a chunk-id-keyed answer stamped metadata onto the block:\n%s", other)
	}
}
