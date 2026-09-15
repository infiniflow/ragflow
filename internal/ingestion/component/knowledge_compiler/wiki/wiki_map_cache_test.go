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

package wiki

import (
	"context"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

type memoryWikiMapVersionStore struct {
	mu       sync.Mutex
	payloads map[string][]byte
}

func (s *memoryWikiMapVersionStore) GetWikiMapVersions(_ context.Context, _, _ string, keys []string) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string][]byte, len(keys))
	for _, key := range keys {
		if payload, ok := s.payloads[key]; ok {
			out[key] = append([]byte(nil), payload...)
		}
	}
	return out, nil
}

func (s *memoryWikiMapVersionStore) PutWikiMapVersions(_ context.Context, versions []common.WikiMapVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.payloads == nil {
		s.payloads = map[string][]byte{}
	}
	for _, version := range versions {
		if _, exists := s.payloads[version.Key]; !exists {
			s.payloads[version.Key] = append([]byte(nil), version.Payload...)
		}
	}
	return nil
}

type countingWikiMapChat struct {
	mu    sync.Mutex
	calls int
}

type staticWikiEmbedder struct{}

func (staticWikiEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{1, 0}
	}
	return vectors, nil
}

func (staticWikiEmbedder) Dimensions() int { return 2 }

func (c *countingWikiMapChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if strings.Contains(req.SystemPrompt, "knowledge compilation writer") {
		return &common.ChatResponse{Content: "# Alpha\n\nAlpha exists.\n"}, nil
	}
	return &common.ChatResponse{Content: `{
  "entities": [{"name":"Alpha","type":"thing","source_chunk_ids":["c1"]}],
  "concepts": [],
  "claims": [{"statement":"Alpha exists","subject":"Alpha","confidence":"explicit","source_chunk_ids":["c1"]}],
  "relations": [],
  "topics": ["Alpha"]
}`}, nil
}

func (c *countingWikiMapChat) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestRunVersionedMapReusesHistoricalChunkVersion(t *testing.T) {
	store := &memoryWikiMapVersionStore{payloads: map[string][]byte{}}
	chat := &countingWikiMapChat{}
	newPipeline := func(text string) *wikiPipeline {
		return &wikiPipeline{
			ctx:       t.Context(),
			deps:      common.Deps{Chat: chat, WikiMapVersions: store},
			param:     common.Param{TemplateID: "tpl-1", Language: "English", TemplateConfig: map[string]any{"mode": "entity"}},
			inputs:    common.Inputs{Chunks: []common.Chunk{{ID: "c1", Text: text}}},
			docID:     "doc-1",
			tenantID:  "tenant-1",
			datasetID: "kb-1",
			llmID:     "llm-1",
		}
	}

	first := newPipeline("version A")
	if err := first.runMap(); err != nil {
		t.Fatalf("first runMap() error = %v", err)
	}
	if got := chat.callCount(); got != 1 {
		t.Fatalf("first MAP calls = %d, want 1", got)
	}
	if len(first.mapExtracts) != 1 || len(first.mapExtracts[0].Entities) != 1 {
		t.Fatalf("first MAP extracts = %#v", first.mapExtracts)
	}

	second := newPipeline("version B")
	if err := second.runMap(); err != nil {
		t.Fatalf("second runMap() error = %v", err)
	}
	if got := chat.callCount(); got != 2 {
		t.Fatalf("changed-content MAP calls = %d, want 2", got)
	}

	third := newPipeline("version A")
	if err := third.runMap(); err != nil {
		t.Fatalf("third runMap() error = %v", err)
	}
	if got := chat.callCount(); got != 2 {
		t.Fatalf("historical-version MAP calls = %d, want 2", got)
	}
	if len(store.payloads) != 2 {
		t.Fatalf("stored MAP versions = %d, want 2", len(store.payloads))
	}
}

func TestRunWithVersionedMapStoreStillBuildsDocumentPages(t *testing.T) {
	store := &memoryWikiMapVersionStore{payloads: map[string][]byte{}}
	chat := &countingWikiMapChat{}
	deps := common.Deps{
		TenantID: "tenant-1", DatasetID: "kb-1",
		Chat: chat, Embed: staticWikiEmbedder{}, WikiMapVersions: store,
	}
	param := common.Param{}.Defaults()
	param.TemplateID = "tpl-1"
	param.LLMID = "llm-1"
	param.Language = "English"
	param.TemplateConfig = map[string]any{"mode": "entity"}
	inputs := common.Inputs{
		DocID: "doc-1", Chunks: []common.Chunk{{ID: "c1", Text: "Alpha exists."}},
	}
	pipeline := &wikiPipeline{
		ctx: t.Context(), deps: deps, param: param, inputs: inputs,
		docID: "doc-1", tenantID: "tenant-1", datasetID: "kb-1", llmID: "llm-1",
	}
	if err := pipeline.run(); err != nil {
		t.Fatalf("pipeline.run() error = %v", err)
	}
	if len(pipeline.pages) == 0 {
		t.Fatalf("pipeline produced no pages: map=%d entities=%d plan=%d", len(pipeline.mapExtracts), len(pipeline.reduced.Entities), len(pipeline.plan.Pages))
	}
	out, err := Run(t.Context(), deps, param, inputs)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(store.payloads) != 1 {
		t.Fatalf("stored MAP versions = %d, want 1", len(store.payloads))
	}
	if len(out.Products) == 0 {
		t.Fatal("Run() returned no document Wiki pages")
	}
	for _, product := range out.Products {
		if kind, _ := product.Meta["kind"].(string); kind != "page" && kind != "section" {
			t.Fatalf("Run() returned intermediate product kind %q", kind)
		}
		if len(product.Vector) == 0 {
			t.Fatalf("Wiki product %s has no embedding", product.ID)
		}
	}
}

func TestSplitWikiExtractByChunkDropsUnknownProvenance(t *testing.T) {
	parts := splitWikiExtractByChunk(wikiExtract{
		Entities: []wikiEntity{
			{Name: "Alpha", SourceChunkIDs: []string{"c1", "unknown"}},
			{Name: "Ghost", SourceChunkIDs: []string{"unknown"}},
		},
		Claims: []wikiClaim{{Statement: "shared", Subject: "Alpha"}},
		Topics: []wikiTopic{
			{Path: "Knowledge/Alpha", SourceChunkIDs: []string{"c1"}},
			{Path: "Knowledge/Shared"},
		},
	}, []common.Chunk{{ID: "c1"}, {ID: "c2"}})

	if got := len(parts["c1"].Entities); got != 1 {
		t.Fatalf("c1 entities = %d, want 1", got)
	}
	if got := len(parts["c2"].Entities); got != 0 {
		t.Fatalf("c2 entities = %d, want 0", got)
	}
	if got := len(parts["c1"].Claims); got != 1 {
		t.Fatalf("c1 claims = %d, want source-less claim fallback", got)
	}
	if got := len(parts["c2"].Claims); got != 1 {
		t.Fatalf("c2 claims = %d, want source-less claim fallback", got)
	}
	if got := len(parts["c1"].Topics); got != 2 {
		t.Fatalf("c1 topics = %d, want cited and source-less topics", got)
	}
	if got := len(parts["c2"].Topics); got != 1 || parts["c2"].Topics[0].Path != "Knowledge/Shared" {
		t.Fatalf("c2 topics = %#v, want only source-less fallback", parts["c2"].Topics)
	}
}
