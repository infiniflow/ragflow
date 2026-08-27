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

package knowledge_compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const wikiContributionCompileKWD = "wiki_contribution_state"

type wikiContributionPage struct {
	Key       string `json:"key"`
	Slug      string `json:"slug"`
	PageType  string `json:"page_type"`
	Signature string `json:"signature"`
}

type wikiDocumentContribution struct {
	DocumentID string                 `json:"document_id"`
	Pages      []wikiContributionPage `json:"pages"`
}

type wikiContributionStore interface {
	Get(ctx context.Context, tenantID, datasetID, documentID string) (wikiDocumentContribution, bool, error)
	Put(ctx context.Context, tenantID, datasetID string, contribution wikiDocumentContribution) error
	Delete(ctx context.Context, tenantID, datasetID, documentID string) error
	ClearDataset(ctx context.Context, tenantID, datasetID string) error
}

type engineWikiContributionStore struct {
	engine engine.DocEngine
}

func newWikiContributionStore(docEngine engine.DocEngine) wikiContributionStore {
	if docEngine == nil {
		return &memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}
	}
	return &engineWikiContributionStore{engine: docEngine}
}

func (s *engineWikiContributionStore) Get(ctx context.Context, tenantID, datasetID, documentID string) (wikiDocumentContribution, bool, error) {
	key := wikiContributionStateID(datasetID, documentID)
	result, err := s.engine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Limit:        1,
		SelectFields: []string{"id", "compile_kwd", "content_with_weight"},
		Filter: map[string]any{
			"id":            []string{key},
			"compile_kwd":   wikiContributionCompileKWD,
			"available_int": 0,
		},
	})
	if err != nil {
		return wikiDocumentContribution{}, false, err
	}
	if result == nil || len(result.Chunks) == 0 {
		return wikiDocumentContribution{}, false, nil
	}
	var contribution wikiDocumentContribution
	if err := json.Unmarshal([]byte(mapStoreString(result.Chunks[0]["content_with_weight"])), &contribution); err != nil {
		return wikiDocumentContribution{}, false, fmt.Errorf("decode Wiki contribution for document %s: %w", documentID, err)
	}
	return contribution, true, nil
}

func (s *engineWikiContributionStore) Put(ctx context.Context, tenantID, datasetID string, contribution wikiDocumentContribution) error {
	if contribution.DocumentID == "" {
		return fmt.Errorf("save Wiki contribution: document_id is required")
	}
	payload, err := json.Marshal(contribution)
	if err != nil {
		return fmt.Errorf("encode Wiki contribution: %w", err)
	}
	row := map[string]any{
		"id":                  wikiContributionStateID(datasetID, contribution.DocumentID),
		"doc_id":              "wiki_contribution:" + contribution.DocumentID,
		"tenant_id":           tenantID,
		"kb_id":               datasetID,
		"compile_kwd":         wikiContributionCompileKWD,
		"scope_kwd":           "doc",
		"source_doc_ids":      []string{contribution.DocumentID},
		"content_with_weight": string(payload),
		"available_int":       0,
	}
	_, err = s.engine.InsertChunks(ctx, []map[string]any{row}, fmt.Sprintf("ragflow_%s", tenantID), datasetID)
	return err
}

func (s *engineWikiContributionStore) Delete(ctx context.Context, tenantID, datasetID, documentID string) error {
	_, err := s.engine.DeleteChunks(ctx, map[string]any{
		"id":    []string{wikiContributionStateID(datasetID, documentID)},
		"kb_id": datasetID,
	}, fmt.Sprintf("ragflow_%s", tenantID), datasetID)
	return err
}

func (s *engineWikiContributionStore) ClearDataset(ctx context.Context, tenantID, datasetID string) error {
	_, err := s.engine.DeleteChunks(ctx, map[string]any{
		"kb_id":         datasetID,
		"compile_kwd":   wikiContributionCompileKWD,
		"available_int": 0,
	}, fmt.Sprintf("ragflow_%s", tenantID), datasetID)
	return err
}

func wikiContributionStateID(datasetID, documentID string) string {
	sum := sha256.Sum256([]byte(datasetID + "\x00" + documentID + "\x00wiki-contribution"))
	return hex.EncodeToString(sum[:])
}

type memoryWikiContributionStore struct {
	mu    sync.Mutex
	items map[string]wikiDocumentContribution
}

func (s *memoryWikiContributionStore) Get(_ context.Context, _, datasetID, documentID string) (wikiDocumentContribution, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[datasetID+"\x00"+documentID]
	return item, ok, nil
}

func (s *memoryWikiContributionStore) Put(_ context.Context, _, datasetID string, contribution wikiDocumentContribution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[datasetID+"\x00"+contribution.DocumentID] = contribution
	return nil
}

func (s *memoryWikiContributionStore) Delete(_ context.Context, _, datasetID, documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, datasetID+"\x00"+documentID)
	return nil
}

func (s *memoryWikiContributionStore) ClearDataset(_ context.Context, _, datasetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := datasetID + "\x00"
	for key := range s.items {
		if strings.HasPrefix(key, prefix) {
			delete(s.items, key)
		}
	}
	return nil
}

func buildWikiDocumentContribution(documentID string, products []kccommon.Product) wikiDocumentContribution {
	pagesByKey := make(map[string]wikiContributionPage)
	for _, product := range products {
		if product.Variant != kccommon.VariantWiki || metaString(product.Meta, "kind") != "page" {
			continue
		}
		key := wikiPageMergeKey(product)
		if key == "" {
			continue
		}
		pagesByKey[key] = wikiContributionPage{
			Key:       key,
			Slug:      strings.TrimSpace(metaString(product.Meta, "slug")),
			PageType:  strings.ToLower(strings.TrimSpace(metaString(product.Meta, "page_type"))),
			Signature: wikiContributionSignature(product),
		}
	}
	keys := make([]string, 0, len(pagesByKey))
	for key := range pagesByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pages := make([]wikiContributionPage, 0, len(keys))
	for _, key := range keys {
		pages = append(pages, pagesByKey[key])
	}
	return wikiDocumentContribution{DocumentID: documentID, Pages: pages}
}

func diffWikiDocumentContribution(previous, current wikiDocumentContribution) (map[string]struct{}, map[string]struct{}) {
	previousByKey := make(map[string]wikiContributionPage, len(previous.Pages))
	for _, page := range previous.Pages {
		previousByKey[page.Key] = page
	}
	currentByKey := make(map[string]wikiContributionPage, len(current.Pages))
	for _, page := range current.Pages {
		currentByKey[page.Key] = page
	}
	affectedKeys := make(map[string]struct{})
	affectedSlugs := make(map[string]struct{})
	for key, page := range currentByKey {
		old, exists := previousByKey[key]
		if exists && old.Signature == page.Signature {
			continue
		}
		affectedKeys[key] = struct{}{}
		if page.Slug != "" {
			affectedSlugs[page.Slug] = struct{}{}
		}
	}
	for key, page := range previousByKey {
		if _, exists := currentByKey[key]; exists {
			continue
		}
		affectedKeys[key] = struct{}{}
		if page.Slug != "" {
			affectedSlugs[page.Slug] = struct{}{}
		}
	}
	return affectedKeys, affectedSlugs
}

func wikiContributionSignature(product kccommon.Product) string {
	normalized := struct {
		Content        string
		Title          string
		Topic          string
		Summary        string
		EntityNames    []string
		RelatedKBPages []string
		Outlinks       []string
		SourceChunkIDs []string
	}{
		Content:        strings.TrimSpace(product.Content),
		Title:          strings.TrimSpace(metaString(product.Meta, "title")),
		Topic:          kccommon.NormalizeWikiTopicPath(metaString(product.Meta, "topic")),
		Summary:        strings.TrimSpace(metaString(product.Meta, "summary")),
		EntityNames:    sortedUniqueCopy(metaStringSlice(product.Meta, "entity_names")),
		RelatedKBPages: sortedUniqueCopy(metaStringSlice(product.Meta, "related_kb_pages")),
		Outlinks:       sortedUniqueCopy(metaStringSlice(product.Meta, "outlinks")),
		SourceChunkIDs: sortedUniqueCopy(metaStringSlice(product.Meta, "source_chunk_ids")),
	}
	payload, _ := json.Marshal(normalized)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func sortedUniqueCopy(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	out := copyValues[:0]
	for _, value := range copyValues {
		if value == "" || len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}

type wikiContributionDiffResult struct {
	productsByDoc map[string][]kccommon.Product
	currentByDoc  map[string]wikiDocumentContribution
	affectedKeys  map[string]struct{}
	affectedSlugs map[string]struct{}
}

func (c *Consumer) prepareWikiContributionDiff(ctx context.Context, tenantID, datasetID string, completed []BacklogEntry, deleted []string) (wikiContributionDiffResult, error) {
	if c.contributions == nil {
		c.contributions = newWikiContributionStore(nil)
	}
	result := wikiContributionDiffResult{
		productsByDoc: make(map[string][]kccommon.Product, len(completed)),
		currentByDoc:  make(map[string]wikiDocumentContribution, len(completed)),
		affectedKeys:  map[string]struct{}{},
		affectedSlugs: map[string]struct{}{},
	}
	completedByDoc := make(map[string]BacklogEntry, len(completed))
	for _, entry := range completed {
		completedByDoc[entry.DocID] = entry
		products, err := c.reader.LoadDocProducts(ctx, tenantID, datasetID, entry.DocID)
		if err != nil {
			return wikiContributionDiffResult{}, err
		}
		result.productsByDoc[entry.DocID] = products
		wikiProducts := products
		if len(entry.Variants) > 0 && !variantsContain(entry.Variants, "wiki") {
			wikiProducts = nil
		}
		result.currentByDoc[entry.DocID] = buildWikiDocumentContribution(entry.DocID, wikiProducts)
	}

	allDocIDs := make([]string, 0, len(completed)+len(deleted))
	for docID := range completedByDoc {
		allDocIDs = append(allDocIDs, docID)
	}
	allDocIDs = append(allDocIDs, deleted...)
	sort.Strings(allDocIDs)

	previousByDoc := make(map[string]wikiDocumentContribution, len(allDocIDs))
	missingPrevious := make(map[string]struct{})
	for _, docID := range allDocIDs {
		previous, exists, err := c.contributions.Get(ctx, tenantID, datasetID, docID)
		if err != nil {
			return wikiContributionDiffResult{}, err
		}
		if exists {
			previousByDoc[docID] = previous
		} else {
			missingPrevious[docID] = struct{}{}
		}
	}

	// Upgrade legacy datasets lazily. Before contribution snapshots existed, the
	// only durable ownership record was source_doc_ids on merged pages. Recover
	// enough page identity from those rows to retract a deleted/disabled document
	// correctly on its first post-upgrade event.
	if len(missingPrevious) > 0 {
		if reader, ok := c.reader.(mergedWikiPageReader); ok {
			mergedPages, err := reader.LoadMergedWikiPages(ctx, tenantID, datasetID)
			if err != nil {
				return wikiContributionDiffResult{}, err
			}
			legacyProducts := make(map[string][]kccommon.Product, len(missingPrevious))
			for _, page := range mergedPages {
				for _, docID := range metaStringSlice(page.Meta, "source_doc_ids") {
					if _, wanted := missingPrevious[docID]; wanted {
						legacyProducts[docID] = append(legacyProducts[docID], page)
					}
				}
			}
			for docID, products := range legacyProducts {
				previousByDoc[docID] = buildWikiDocumentContribution(docID, products)
			}
		}
	}

	for _, docID := range allDocIDs {
		previous := previousByDoc[docID]
		current, completedNow := result.currentByDoc[docID]
		if !completedNow {
			current = wikiDocumentContribution{DocumentID: docID}
		}
		keys, slugs := diffWikiDocumentContribution(previous, current)
		for key := range keys {
			result.affectedKeys[key] = struct{}{}
		}
		for slug := range slugs {
			result.affectedSlugs[slug] = struct{}{}
		}
	}
	return result, nil
}

func (c *Consumer) commitWikiContributions(ctx context.Context, tenantID, datasetID string, current map[string]wikiDocumentContribution, deleted []string) error {
	for documentID, contribution := range current {
		if len(contribution.Pages) == 0 {
			if err := c.contributions.Delete(ctx, tenantID, datasetID, documentID); err != nil {
				return fmt.Errorf("delete Wiki contribution for document %s: %w", documentID, err)
			}
			continue
		}
		if err := c.contributions.Put(ctx, tenantID, datasetID, contribution); err != nil {
			return fmt.Errorf("save Wiki contribution for document %s: %w", documentID, err)
		}
	}
	for _, documentID := range deleted {
		if err := c.contributions.Delete(ctx, tenantID, datasetID, documentID); err != nil {
			return fmt.Errorf("delete Wiki contribution for document %s: %w", documentID, err)
		}
	}
	return nil
}
