package wiki

import (
	"fmt"
	"sort"
	"strings"

	appcommon "ragflow/internal/common"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"

	"go.uber.org/zap"
)

const (
	wikiTopicCandidateThreshold = 0.72
	wikiTopicCommunityMaxItems  = 12
	wikiTopicNeighborMax        = 32
	wikiTopicSourceChunkMax     = 128
)

type topicCommunityItem struct {
	name    string
	entity  *wikiEntity
	concept *wikiConcept
	vector  []float32
}

// buildTopicCandidateCommunities uses embedding as a candidate-windowing
// signal only. The final page membership is still decided by the LLM planner.
func (p *wikiPipeline) buildTopicCandidateCommunities() []wikiExtract {
	items := make([]topicCommunityItem, 0, len(p.reduced.Entities)+len(p.reduced.Concepts))
	for i := range p.reduced.Entities {
		entity := p.reduced.Entities[i]
		if name := strings.TrimSpace(entity.Name); name != "" {
			items = append(items, topicCommunityItem{name: name, entity: &entity})
		}
	}
	for i := range p.reduced.Concepts {
		concept := p.reduced.Concepts[i]
		if name := strings.TrimSpace(concept.Term); name != "" {
			items = append(items, topicCommunityItem{name: name, concept: &concept})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return normKey(items[i].name) < normKey(items[j].name) })
	if len(items) < 2 || p.deps.Embed == nil {
		return []wikiExtract{p.reduced}
	}
	topics := wikiTopicPaths(p.reduced.Topics)
	texts := make([]string, 0, len(items)+len(topics))
	for i := range items {
		texts = append(texts, items[i].name)
	}
	for i, topic := range topics {
		text := topic
		if i < len(p.reduced.Topics) {
			if description := strings.TrimSpace(p.reduced.Topics[i].Description); description != "" {
				text += "\n" + description
			}
		}
		texts = append(texts, text)
	}
	vectors, err := p.deps.Embed.Encode(p.ctx, texts)
	if err != nil || len(vectors) != len(texts) {
		return []wikiExtract{p.reduced}
	}
	for i := range items {
		items[i].vector = vectors[i]
	}
	communities := make([][]topicCommunityItem, 0)
	for _, item := range items {
		placed := -1
		best := -1.0
		for i, community := range communities {
			if len(community) >= wikiTopicCommunityMaxItems {
				continue
			}
			for _, member := range community {
				score := cosine32(item.vector, member.vector)
				if score >= wikiTopicCandidateThreshold && score > best {
					best = score
					placed = i
				}
			}
		}
		if placed < 0 {
			communities = append(communities, []topicCommunityItem{item})
		} else {
			communities[placed] = append(communities[placed], item)
		}
	}
	topicCommunityIndexes := p.assignTopicCommunities(communities, topics, vectors[len(items):])
	residual := wikiExtract{}
	assignedClaims := make([]bool, len(p.reduced.Claims))
	assignedRelations := make([]bool, len(p.reduced.Relations))
	out := make([]wikiExtract, 0, len(communities))
	for i, community := range communities {
		nameSet := make(map[string]struct{}, len(community))
		extract := wikiExtract{}
		for _, item := range community {
			nameSet[normKey(item.name)] = struct{}{}
			if item.entity != nil {
				extract.Entities = append(extract.Entities, *item.entity)
			}
			if item.concept != nil {
				extract.Concepts = append(extract.Concepts, *item.concept)
			}
		}
		for claimIndex, claim := range p.reduced.Claims {
			if _, ok := nameSet[normKey(claim.Subject)]; ok {
				extract.Claims = append(extract.Claims, claim)
				assignedClaims[claimIndex] = true
			} else if i == len(communities)-1 && !assignedClaims[claimIndex] {
				residual.Claims = append(residual.Claims, claim)
			}
		}
		for relationIndex, relation := range p.reduced.Relations {
			_, fromOK := nameSet[normKey(relation.From)]
			_, toOK := nameSet[normKey(relation.To)]
			if fromOK || toOK {
				extract.Relations = append(extract.Relations, relation)
				assignedRelations[relationIndex] = true
			} else if i == len(communities)-1 && !assignedRelations[relationIndex] {
				residual.Relations = append(residual.Relations, relation)
			}
		}
		for topicIndex, topic := range p.reduced.Topics {
			if topicIndex < len(topicCommunityIndexes) && topicCommunityIndexes[topicIndex] == i {
				extract.Topics = append(extract.Topics, topic)
			} else if i == len(communities)-1 && (topicIndex >= len(topicCommunityIndexes) || topicCommunityIndexes[topicIndex] < 0) {
				residual.Topics = append(residual.Topics, topic)
			}
		}
		extract.Topics = normalizeWikiTopics(extract.Topics)
		out = append(out, extract)
	}
	if len(residual.Claims) > 0 || len(residual.Relations) > 0 || len(residual.Topics) > 0 {
		residual.Topics = normalizeWikiTopics(residual.Topics)
		out = append(out, residual)
	}
	return out
}

// assignTopicCommunities attaches every MAP topic path to the most relevant
// entity/concept community. Embedding is used only for candidate placement;
// the planner still decides the final page-to-topic assignment.
func (p *wikiPipeline) assignTopicCommunities(communities [][]topicCommunityItem, topics []string, vectors [][]float32) []int {
	assignments := make([]int, len(p.reduced.Topics))
	for i := range assignments {
		assignments[i] = -1
	}
	if len(assignments) == 0 || len(communities) == 0 {
		return assignments
	}

	if len(topics) != len(p.reduced.Topics) {
		return assignments
	}
	if len(vectors) == len(topics) {
		for topicIndex, vector := range vectors {
			bestCommunity := -1
			bestScore := -1.0
			for communityIndex, community := range communities {
				for _, member := range community {
					score := cosine32(vector, member.vector)
					if score > bestScore {
						bestScore = score
						bestCommunity = communityIndex
					}
				}
			}
			assignments[topicIndex] = bestCommunity
		}
		return assignments
	}

	// Missing topic vectors must not block compilation. Retain an exact
	// leaf/name fallback and leave unmatched paths for the residual planner batch.
	for topicIndex, topic := range topics {
		leaf := normKey(kccommon.WikiTopicLeaf(topic))
		for communityIndex, community := range communities {
			for _, member := range community {
				if leaf != "" && leaf == normKey(member.name) {
					assignments[topicIndex] = communityIndex
					break
				}
			}
			if assignments[topicIndex] >= 0 {
				break
			}
		}
	}
	return assignments
}

func (p *wikiPipeline) runTopicPlan() (wikiPlan, error) {
	communities := p.buildTopicCandidateCommunities()
	if len(communities) == 0 {
		return wikiPlan{}, nil
	}
	totalItems := 0
	for _, community := range communities {
		totalItems += wikiExtractItemCount(community)
	}
	p.planBudget = deriveWikiPlanBudget(p.deps.ModelContextLen, totalItems)
	quotas := allocatePlanQuotas(communities, p.planBudget.Cap())
	approved := wikiExtract{}
	plans := make([]wikiPlan, len(communities))
	jobs := make([]func() error, 0, len(communities))
	for i, community := range communities {
		i, community := i, community
		quota := quotas[i]
		if quota <= 0 {
			continue
		}
		approved.Entities = append(approved.Entities, community.Entities...)
		approved.Concepts = append(approved.Concepts, community.Concepts...)
		approved.Claims = append(approved.Claims, community.Claims...)
		approved.Relations = append(approved.Relations, community.Relations...)
		approved.Topics = append(approved.Topics, community.Topics...)
		jobs = append(jobs, func() error {
			if err := p.ctx.Err(); err != nil {
				return err
			}
			plan, err := p.runPlanBatch(community, i+1, len(communities), quota, p.planBudget.MaxTokens)
			if err != nil {
				return err
			}
			group := fmt.Sprintf("community-%d", i+1)
			for pageIndex := range plan.Pages {
				plan.Pages[pageIndex].PlanGroup = group
			}
			plans[i] = plan
			return nil
		})
	}
	if err := runBatches(p.ctx, jobs); err != nil {
		return wikiPlan{}, err
	}
	merged := p.mergePlanCandidates(plans, approved)
	var excluded int
	merged.Pages, excluded = truncatePlanPagesByCap(merged.Pages, p.planBudget.Max, approved)
	p.planCapacityExcluded += excluded
	merged.Pages = normalizeWikiPlanPageLinks(merged.Pages)
	reconciled, err := p.reconcilePlan(merged)
	if err != nil {
		return wikiPlan{}, err
	}
	reconciled.Pages = normalizeWikiPlanPageLinks(reconciled.Pages)
	appcommon.Info("wiki: topic candidate communities planned",
		zap.String("dataset_id", p.datasetID), zap.String("doc_id", p.runKey()),
		zap.Int("communities", len(communities)), zap.Int("plan_pages", len(reconciled.Pages)))
	return reconciled, nil
}
