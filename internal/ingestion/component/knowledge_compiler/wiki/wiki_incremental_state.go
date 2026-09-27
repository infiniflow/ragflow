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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func (p *wikiPipeline) addAffectedExtractTerms(extract wikiExtract) {
	if p.affectedTerms == nil {
		p.affectedTerms = make(map[string]struct{})
	}
	add := func(value string) {
		value = normalizedWikiTerm(value)
		if value != "" {
			p.affectedTerms[value] = struct{}{}
		}
	}
	for _, entity := range extract.Entities {
		add(entity.Name)
		for _, alias := range entity.Aliases {
			add(alias)
		}
	}
	for _, concept := range extract.Concepts {
		add(concept.Term)
	}
	for _, claim := range extract.Claims {
		add(claim.Subject)
	}
	for _, relation := range extract.Relations {
		add(relation.From)
		add(relation.To)
	}
	for _, topic := range extract.Topics {
		add(topic.Path)
		add(common.WikiTopicLeaf(topic.Path))
	}
}

func (p *wikiPipeline) selectAffectedPages(current []wikiPlanPage) {
	if p.affectedPageSlugs == nil {
		p.affectedPageSlugs = make(map[string]struct{})
	}
	if !p.incremental || len(p.previousActiveState.Chunks) == 0 {
		for _, page := range current {
			if page.Slug != "" {
				p.affectedPageSlugs[page.Slug] = struct{}{}
			}
		}
		return
	}
	if !p.mapChanged {
		return
	}
	previousBySlug := planPagesBySlug(p.previousActiveState.Plan)
	currentBySlug := planPagesBySlug(current)
	for slug, page := range currentBySlug {
		previous, existed := previousBySlug[slug]
		if !existed || planPageSignature(previous) != planPageSignature(page) || p.pageTouchesAffectedTerms(page) {
			p.affectedPageSlugs[slug] = struct{}{}
		}
	}
	for slug, page := range previousBySlug {
		if _, remains := currentBySlug[slug]; !remains {
			p.affectedPageSlugs[slug] = struct{}{}
			p.removedPageSlugs = append(p.removedPageSlugs, slug)
			continue
		}
		if p.pageTouchesAffectedTerms(page) {
			p.affectedPageSlugs[slug] = struct{}{}
		}
	}
	sort.Strings(p.removedPageSlugs)
}

func (p *wikiPipeline) pageTouchesAffectedTerms(page wikiPlanPage) bool {
	if topic := normalizedWikiTerm(page.Topic); topic != "" {
		if _, ok := p.affectedTerms[topic]; ok {
			return true
		}
	}
	if leaf := normalizedWikiTerm(common.WikiTopicLeaf(page.Topic)); leaf != "" {
		if _, ok := p.affectedTerms[leaf]; ok {
			return true
		}
	}
	for _, name := range page.EntityNames {
		if _, ok := p.affectedTerms[normalizedWikiTerm(name)]; ok {
			return true
		}
	}
	return false
}

func (p *wikiPipeline) buildActiveMapState(plan []wikiPlanPage) (*common.WikiMapActiveState, error) {
	if p.activeStateKey == "" || p.deps.WikiMapVersions == nil {
		return nil, nil
	}
	p.nextActiveState.Plan = append([]wikiPlanPage(nil), plan...)
	payload, err := json.Marshal(p.nextActiveState)
	if err != nil {
		return nil, fmt.Errorf("wiki: encode active MAP state: %w", err)
	}
	return &common.WikiMapActiveState{
		Key:        p.activeStateKey,
		TenantID:   p.tenantID,
		DatasetID:  p.datasetID,
		DocumentID: p.docID,
		Payload:    payload,
	}, nil
}

func planPagesBySlug(pages []wikiPlanPage) map[string]wikiPlanPage {
	result := make(map[string]wikiPlanPage, len(pages))
	for _, page := range pages {
		if page.Slug != "" {
			result[page.Slug] = page
		}
	}
	return result
}

func planPageSignature(page wikiPlanPage) string {
	payload, _ := json.Marshal(struct {
		Title       string
		PageType    string
		Topic       string
		EntityNames []string
		RelatedKB   []string
	}{page.Title, page.PageType, page.Topic, page.EntityNames, page.RelatedKB})
	return string(payload)
}

func normalizedWikiTerm(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
