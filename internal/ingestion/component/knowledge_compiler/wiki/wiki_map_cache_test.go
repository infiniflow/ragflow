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
	"encoding/json"
	"fmt"
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

type claimRequestChat struct {
	countingWikiMapChat
	requests []common.ChatRequest
}

type claimGuidanceCacheStore struct {
	memoryWikiMapVersionStore
	activeStates map[string][]byte
	activeReads  []string
}

func (s *claimGuidanceCacheStore) GetWikiMapActiveState(_ context.Context, tenantID, datasetID, key string) ([]byte, error) {
	if tenantID != "tenant-1" || datasetID != "kb-1" {
		return nil, fmt.Errorf("unexpected active-state scope: %s/%s", tenantID, datasetID)
	}
	s.activeReads = append(s.activeReads, key)
	return s.activeStates[key], nil
}

func (s *claimGuidanceCacheStore) PutWikiMapActiveState(_ context.Context, state common.WikiMapActiveState) error {
	if state.TenantID != "tenant-1" || state.DatasetID != "kb-1" || state.DocumentID != "doc-1" {
		return fmt.Errorf("unexpected active-state scope: %s/%s/%s", state.TenantID, state.DatasetID, state.DocumentID)
	}
	s.activeStates[state.Key] = append([]byte(nil), state.Payload...)
	return nil
}

func (c *claimRequestChat) Chat(ctx context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	c.requests = append(c.requests, req)
	return c.countingWikiMapChat.Chat(ctx, req)
}

// Cache fixtures use the old MAP fingerprint before canonical guidance was
// read. The combined fixture includes the prior claim-only rendering version.
// Only changed guidance needs a new request, then the result must be reusable.
func TestVersionedMapRefreshesSavedGuidance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		config    map[string]any
		oldKey    string
		oldActive string
		wantCalls int
		guidance  []string
	}{
		{
			name: "canonical claim guidance", guidance: []string{"Extract audited financial claims.", "Use legal entity names."},
			config: map[string]any{"claim": map[string]any{"fields": []any{map[string]any{
				"statement": "Extract audited financial claims.", "subject": "Use legal entity names.",
			}}}},
			oldKey: "9c642c4dc3c80dca321f0d9e0bf3d73ada735e1d73e3f12a6031182b77e22aa9", wantCalls: 1,
			oldActive: "e483b7cc26f82581f10a6a7d9b789cfd92d0718f236b0edb06a77fd090c2a6cb",
		},
		{
			name: "no claim guidance", config: map[string]any{},
			oldKey:    "e1a30f6aa9da75ae27839f4b561326cc5751c7bcb3e26c5997081ecc45ea673e",
			oldActive: "2452ff89474f54b2b0fc137cba1e958fb1c99fb84f32e2a0032c8c4d8470e933",
		},
		{
			name: "empty claim guidance", config: map[string]any{"claim": map[string]any{"fields": []any{}}},
			oldKey:    "4c3e0f2f1926ad93402083427b3c99cc57a293854f2412445f17f7b2e86ee47f",
			oldActive: "bd296f8dfc82e4ce3987c2dbcb48bdf1418506ef553333975cafdea396ce4b8d",
		},
		{
			name:      "entity type guidance",
			config:    map[string]any{"entity": map[string]any{"fields": []any{map[string]any{"type": "sensor", "description": "An installed physical sensor.", "rule": "Keep its exact commissioning identifier."}}}},
			oldKey:    "2aba3689af18d4f2826737da24e496006ff303813bb13379919c9b9f36bcd85e",
			oldActive: "bb279aa92aa9b60d4bd6e3578b7fc8474232b16c18fd501fafeb0722b014d2e0",
			wantCalls: 1,
			guidance:  []string{"sensor", "An installed physical sensor.", "Keep its exact commissioning identifier."},
		},
		{
			name:      "relation type guidance",
			config:    map[string]any{"relation": map[string]any{"fields": []any{map[string]any{"type": "calibrates", "description": "Calibration performed on equipment.", "rule": "Direction is technician to sensor."}}}},
			oldKey:    "4ebf5eb5310d51227137f1260e5178e03cc16ef12655887b8033f362ceebfe3a",
			oldActive: "98d2ab4f5adf878341cf2f4de8f7767f111bd6524a2f5332aad0120128079c2d",
			wantCalls: 1,
			guidance:  []string{"calibrates", "Calibration performed on equipment.", "Direction is technician to sensor."},
		},
		{
			name:      "type without optional guidance",
			config:    map[string]any{"entity": map[string]any{"fields": []any{map[string]any{"type": "sensor", "description": "", "rule": ""}}}},
			oldKey:    "9ae6c8e69518e1f5b12c817ef506167b9b204616db3a04ec8509fb7640646762",
			oldActive: "62abb1a62b7806e0604bfd28c58529ddb3ef04fce7d52289ab2f70ea78f899c5",
			wantCalls: 1,
			guidance:  []string{"sensor"},
		},
		{
			name:      "empty type guidance",
			config:    map[string]any{"entity": map[string]any{"fields": []any{}}, "relation": map[string]any{"fields": []any{}}},
			oldKey:    "c0caf21eea57d8d8d1a3bd85d666c7d040450487aa440998bda60f84e1c50e2f",
			oldActive: "0ac014733e54da6bf33670be9bcbafd1b0f2898b65266bb6b5cb9db27b07e9ee",
			wantCalls: 0,
			guidance:  []string{},
		},
		{
			name:      "blank type retains defaults",
			config:    map[string]any{"entity": map[string]any{"fields": []any{map[string]any{"type": "  ", "description": "ignored", "rule": "ignored"}}}},
			oldKey:    "7fb96c2e0a8534a5adf4f84647b890f612580d69f8ece3a1314ce370b56f0524",
			oldActive: "91d9b232e7b7f6e185289f7b2bc48d5857b573dd1922524256da58e3f36d7674",
			wantCalls: 0,
			guidance:  []string{},
		},
		{
			name:      "combined claim and type guidance",
			config:    map[string]any{"entity": map[string]any{"fields": []any{map[string]any{"type": "sensor", "description": "Installed sensor", "rule": "Keep ID"}}}, "claim": map[string]any{"fields": []any{map[string]any{"statement": "Extract audited financial claims.", "subject": "Use legal entity names."}}}},
			oldKey:    "ab22dc3331a2febbbccc5a84971159d79e5835049e519cdd739377288724cea1",
			oldActive: "21fa972bf5c9c62a220644758767241c194ca88385a94041f4f5b4f40cc93550",
			wantCalls: 1,
			guidance:  []string{"sensor", "Installed sensor", "Keep ID", "Extract audited financial claims.", "Use legal entity names."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldExtract := wikiExtract{Claims: []wikiClaim{{Statement: "cached without guidance", Subject: "Alpha", SourceChunkIDs: []string{"c1"}}}}
			oldPayload, err := json.Marshal(oldExtract)
			if err != nil {
				t.Fatal(err)
			}
			oldActive, err := json.Marshal(wikiMapActiveSnapshot{Chunks: map[string]wikiMapActiveChunk{"c1": {Key: tc.oldKey, Extract: oldExtract}}})
			if err != nil {
				t.Fatal(err)
			}
			store := &claimGuidanceCacheStore{
				memoryWikiMapVersionStore: memoryWikiMapVersionStore{payloads: map[string][]byte{tc.oldKey: oldPayload}},
				activeStates:              map[string][]byte{tc.oldActive: oldActive},
			}
			chat := &claimRequestChat{}
			newPipeline := func() *wikiPipeline {
				return &wikiPipeline{
					ctx: t.Context(), deps: common.Deps{Chat: chat, WikiMapVersions: store},
					param: common.Param{TemplateID: "tpl-claim", Language: "English", TemplateConfig: tc.config},
					inputs: common.Inputs{
						Chunks:          []common.Chunk{{ID: "c1", Text: "Alpha exists."}},
						VariantSpecific: map[string]any{"parser_config": tc.config},
					},
					docID: "doc-1", tenantID: "tenant-1", datasetID: "kb-1", llmID: "llm-1",
				}
			}
			first := newPipeline()
			if err := first.runMap(); err != nil {
				t.Fatal(err)
			}
			if len(chat.requests) != tc.wantCalls {
				t.Fatalf("MAP requests = %d, want %d; old cache may have bypassed corrected guidance", len(chat.requests), tc.wantCalls)
			}
			if len(store.activeReads) != 1 || store.activeReads[0] != first.activeStateKey || first.mapChanged != (tc.wantCalls > 0) {
				t.Fatalf("unexpected active-state read/change: keys=%v changed=%t", store.activeReads, first.mapChanged)
			}
			if (first.activeStateKey != tc.oldActive) != (tc.wantCalls > 0) {
				t.Fatalf("active-state scope did not follow saved guidance rendering: key=%s old=%s", first.activeStateKey, tc.oldActive)
			}
			if tc.wantCalls > 0 {
				for _, guidance := range tc.guidance {
					if !strings.Contains(chat.requests[0].UserPrompt, guidance) {
						t.Errorf("refresh request missing %q", guidance)
					}
				}
				if len(first.mapExtracts) != 1 || len(first.mapExtracts[0].Claims) != 1 || first.mapExtracts[0].Claims[0].Statement != "Alpha exists" {
					t.Fatalf("MAP result did not replace old cached extraction: %#v", first.mapExtracts)
				}
			}
			activePayload, err := json.Marshal(first.nextActiveState)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PutWikiMapActiveState(t.Context(), common.WikiMapActiveState{
				Key: first.activeStateKey, TenantID: "tenant-1", DatasetID: "kb-1", DocumentID: "doc-1", Payload: activePayload,
			}); err != nil {
				t.Fatal(err)
			}
			second := newPipeline()
			if err := second.runMap(); err != nil {
				t.Fatal(err)
			}
			if len(chat.requests) != tc.wantCalls || len(store.payloads) != 1+tc.wantCalls || second.mapChanged || second.activeStateKey != first.activeStateKey {
				t.Fatalf("repeated run did not reuse cache: requests=%d versions=%d", len(chat.requests), len(store.payloads))
			}
		})
	}
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
