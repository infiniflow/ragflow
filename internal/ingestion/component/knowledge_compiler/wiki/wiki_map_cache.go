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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const wikiMapActiveStateSchemaVersion = "topic-path-v2"

type wikiMapChunkVersion struct {
	index       int
	chunk       common.Chunk
	key         string
	contentHash string
}

// runVersionedMap reuses immutable per-chunk MAP results and only submits cache
// misses to the LLM. Results are split back to one payload per source chunk so
// a later active snapshot can add or retract evidence at chunk granularity.
func (p *wikiPipeline) runVersionedMap() error {
	templateFingerprint, err := p.wikiMapTemplateFingerprint()
	if err != nil {
		return err
	}
	llmFingerprint := wikiMapHash(p.llmID)
	p.activeStateKey = wikiMapHash(strings.Join([]string{
		p.tenantID, p.datasetID, p.docID, templateFingerprint, llmFingerprint,
		wikiMapActiveStateSchemaVersion, "active",
	}, "\x00"))
	p.previousActiveState = wikiMapActiveSnapshot{Chunks: map[string]wikiMapActiveChunk{}}
	if activeStore, ok := p.deps.WikiMapVersions.(common.WikiMapActiveStateStore); ok {
		payload, err := activeStore.GetWikiMapActiveState(p.ctx, p.tenantID, p.datasetID, p.activeStateKey)
		if err != nil {
			return fmt.Errorf("wiki: load active MAP state: %w", err)
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p.previousActiveState); err != nil {
				return fmt.Errorf("wiki: decode active MAP state: %w", err)
			}
		}
	}
	if p.previousActiveState.Chunks == nil {
		p.previousActiveState.Chunks = map[string]wikiMapActiveChunk{}
	}

	versions := make([]wikiMapChunkVersion, 0, len(p.inputs.Chunks))
	keys := make([]string, 0, len(p.inputs.Chunks))
	for i, chunk := range p.inputs.Chunks {
		if strings.TrimSpace(chunk.ID) == "" {
			continue
		}
		contentHash := wikiMapHash(firstNonEmpty(chunk.Text, chunk.Content))
		key := wikiMapHash(strings.Join([]string{
			p.tenantID,
			p.datasetID,
			p.docID,
			chunk.ID,
			contentHash,
			templateFingerprint,
			llmFingerprint,
		}, "\x00"))
		versions = append(versions, wikiMapChunkVersion{index: i, chunk: chunk, key: key, contentHash: contentHash})
		keys = append(keys, key)
	}

	cached, err := p.deps.WikiMapVersions.GetWikiMapVersions(p.ctx, p.tenantID, p.datasetID, keys)
	if err != nil {
		return fmt.Errorf("wiki: load MAP versions: %w", err)
	}

	extracts := make([]wikiExtract, len(p.inputs.Chunks))
	resolved := make([]bool, len(p.inputs.Chunks))
	misses := make([]common.Chunk, 0, len(versions))
	toSave := make([]common.WikiMapVersion, 0, len(versions))
	versionByChunkID := make(map[string]wikiMapChunkVersion, len(versions))
	for _, version := range versions {
		versionByChunkID[version.chunk.ID] = version
		payload, ok := cached[version.key]
		if !ok {
			misses = append(misses, version.chunk)
			continue
		}
		if err := json.Unmarshal(payload, &extracts[version.index]); err != nil {
			return fmt.Errorf("wiki: decode MAP version %s for chunk %s: %w", version.key, version.chunk.ID, err)
		}
		// Stamp the effective compiler mode on the in-memory extract. The mode is
		// part of the dataset FINALIZE contract and cache rows remain immutable.
		extracts[version.index].Mode = p.wikiMode()
		resolved[version.index] = true
	}

	batches := common.PackBatches(misses, wikiMapTokenBudget, p.deps.Tokenizer)
	batchExtracts, err := runMapBatches(p.ctx, batches, p.mapBatch)
	if err != nil {
		return err
	}
	for i, batch := range batches {
		if i >= len(batchExtracts) {
			return fmt.Errorf("wiki: MAP batch result missing at index %d", i)
		}
		perChunk := splitWikiExtractByChunk(batchExtracts[i], batch)
		for _, chunk := range batch {
			version, ok := versionByChunkID[chunk.ID]
			if !ok {
				continue
			}
			extract := perChunk[chunk.ID]
			payload, err := json.Marshal(extract)
			if err != nil {
				return fmt.Errorf("wiki: encode MAP version for chunk %s: %w", chunk.ID, err)
			}
			extracts[version.index] = extract
			resolved[version.index] = true
			toSave = append(toSave, common.WikiMapVersion{
				Key:                 version.key,
				TenantID:            p.tenantID,
				DatasetID:           p.datasetID,
				DocumentID:          p.docID,
				ChunkID:             chunk.ID,
				ContentHash:         version.contentHash,
				TemplateFingerprint: templateFingerprint,
				LLMFingerprint:      llmFingerprint,
				Payload:             payload,
			})
		}
	}
	if err := p.deps.WikiMapVersions.PutWikiMapVersions(p.ctx, toSave); err != nil {
		return fmt.Errorf("wiki: save MAP versions: %w", err)
	}

	// Chunks without a stable id cannot form a reusable cache identity. Keep the
	// old batched behavior for them instead of inventing positional ids.
	var uncacheable []common.Chunk
	for i, chunk := range p.inputs.Chunks {
		if resolved[i] {
			continue
		}
		uncacheable = append(uncacheable, chunk)
	}
	uncacheableBatches := common.PackBatches(uncacheable, wikiMapTokenBudget, p.deps.Tokenizer)
	uncachedExtracts, err := runMapBatches(p.ctx, uncacheableBatches, p.mapBatch)
	if err != nil {
		return err
	}

	for _, extract := range extracts {
		if wikiExtractHasContent(extract) {
			p.mapExtracts = append(p.mapExtracts, extract)
		}
	}
	p.mapExtracts = append(p.mapExtracts, uncachedExtracts...)
	p.nextActiveState = wikiMapActiveSnapshot{Chunks: make(map[string]wikiMapActiveChunk, len(versions))}
	currentChunkIDs := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		currentChunkIDs[version.chunk.ID] = struct{}{}
		extract := extracts[version.index]
		p.nextActiveState.Chunks[version.chunk.ID] = wikiMapActiveChunk{Key: version.key, Extract: extract}
		previous, existed := p.previousActiveState.Chunks[version.chunk.ID]
		if !existed || previous.Key != version.key {
			p.mapChanged = true
			p.addAffectedExtractTerms(previous.Extract)
			p.addAffectedExtractTerms(extract)
		}
	}
	for chunkID, previous := range p.previousActiveState.Chunks {
		if _, active := currentChunkIDs[chunkID]; active {
			continue
		}
		p.mapChanged = true
		p.addAffectedExtractTerms(previous.Extract)
	}
	if len(uncacheable) > 0 {
		p.mapChanged = true
		for _, extract := range uncachedExtracts {
			p.addAffectedExtractTerms(extract)
		}
	}
	return nil
}

func wikiExtractHasContent(extract wikiExtract) bool {
	return len(extract.Entities) > 0 || len(extract.Concepts) > 0 || len(extract.Claims) > 0 || len(extract.Relations) > 0 || len(extract.Topics) > 0
}

func (p *wikiPipeline) wikiMapTemplateFingerprint() (string, error) {
	mapConfig := p.param.TemplateConfig
	if configured, ok := p.inputs.VariantSpecific["parser_config"].(map[string]any); ok {
		mapConfig = configured
	}
	payload, err := json.Marshal(struct {
		TemplateID     string         `json:"template_id"`
		TemplateConfig map[string]any `json:"template_config"`
		Language       string         `json:"language"`
		SystemPrompt   string         `json:"system_prompt"`
		UserTemplate   string         `json:"user_template"`
	}{
		TemplateID:     p.param.TemplateID,
		TemplateConfig: mapConfig,
		Language:       p.param.Language,
		SystemPrompt:   wikiMapSystem,
		UserTemplate:   wikiMapUserTemplate,
	})
	if err != nil {
		return "", fmt.Errorf("wiki: fingerprint MAP template: %w", err)
	}
	return wikiMapHash(string(payload)), nil
}

func wikiMapHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func splitWikiExtractByChunk(extract wikiExtract, batch []common.Chunk) map[string]wikiExtract {
	out := make(map[string]wikiExtract, len(batch))
	known := make(map[string]struct{}, len(batch))
	for _, chunk := range batch {
		if chunk.ID != "" {
			known[chunk.ID] = struct{}{}
			out[chunk.ID] = wikiExtract{}
		}
	}

	for _, entity := range extract.Entities {
		for _, chunkID := range wikiMapItemChunkIDs(entity.SourceChunkIDs, known) {
			part := out[chunkID]
			entityCopy := entity
			entityCopy.SourceChunkIDs = []string{chunkID}
			part.Entities = append(part.Entities, entityCopy)
			out[chunkID] = part
		}
	}
	for _, concept := range extract.Concepts {
		for _, chunkID := range wikiMapItemChunkIDs(concept.SourceChunkIDs, known) {
			part := out[chunkID]
			conceptCopy := concept
			conceptCopy.SourceChunkIDs = []string{chunkID}
			part.Concepts = append(part.Concepts, conceptCopy)
			out[chunkID] = part
		}
	}
	for _, claim := range extract.Claims {
		for _, chunkID := range wikiMapItemChunkIDs(claim.SourceChunkIDs, known) {
			part := out[chunkID]
			claimCopy := claim
			claimCopy.SourceChunkIDs = []string{chunkID}
			part.Claims = append(part.Claims, claimCopy)
			out[chunkID] = part
		}
	}
	for _, relation := range extract.Relations {
		for _, chunkID := range wikiMapItemChunkIDs(relation.SourceChunkIDs, known) {
			part := out[chunkID]
			relationCopy := relation
			relationCopy.SourceChunkIDs = []string{chunkID}
			part.Relations = append(part.Relations, relationCopy)
			out[chunkID] = part
		}
	}
	for _, topic := range extract.Topics {
		for _, chunkID := range wikiMapItemChunkIDs(topic.SourceChunkIDs, known) {
			part := out[chunkID]
			topicCopy := topic
			topicCopy.SourceChunkIDs = []string{chunkID}
			part.Topics = append(part.Topics, topicCopy)
			out[chunkID] = part
		}
	}
	return out
}

func wikiMapItemChunkIDs(sourceChunkIDs []string, known map[string]struct{}) []string {
	if len(sourceChunkIDs) == 0 {
		out := make([]string, 0, len(known))
		for chunkID := range known {
			out = append(out, chunkID)
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(sourceChunkIDs))
	seen := make(map[string]struct{}, len(sourceChunkIDs))
	for _, chunkID := range sourceChunkIDs {
		if _, ok := known[chunkID]; !ok {
			continue
		}
		if _, duplicate := seen[chunkID]; duplicate {
			continue
		}
		seen[chunkID] = struct{}{}
		out = append(out, chunkID)
	}
	return out
}
