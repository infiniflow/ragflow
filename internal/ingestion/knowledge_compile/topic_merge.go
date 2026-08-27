package knowledge_compile

import (
	"context"
	"fmt"
	"strings"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const topicRouteSystemPrompt = "You route incoming Wiki pages to an existing knowledge-base topic path. A topic uses '/' as a hierarchy separator. Decide by semantic topic relevance, not title similarity alone. Return only JSON: {\"merge\":true|false,\"topic\":\"canonical/materialized/topic/path\"}. Set merge=true only when the incoming page belongs to the candidate topic path. When merge=true, copy the candidate topic path exactly. Set merge=false when it should remain a separate topic."

type topicRouter interface {
	RouteTopic(ctx context.Context, incoming, existing kccommon.Product) (bool, string, error)
}

func (x *llmDeduper) RouteTopic(ctx context.Context, incoming, existing kccommon.Product) (bool, string, error) {
	if x == nil || x.decider == nil || x.decider.Chat == nil {
		return false, "", nil
	}
	prompt := fmt.Sprintf("INCOMING PAGE\ntopic: %s\ntitle: %s\nentities: %s\nsummary: %s\n\nCANDIDATE TOPIC PAGE\ntopic: %s\ntitle: %s\nentities: %s\nsummary: %s",
		productTopic(incoming), productTitle(incoming), strings.Join(productEntities(incoming), ", "), productSummary(incoming),
		productTopic(existing), productTitle(existing), strings.Join(productEntities(existing), ", "), productSummary(existing))
	result, err := kccommon.GenJSON(ctx, x.decider.Chat, kccommon.ChatRequest{
		LLMID: x.decider.LLMID, SystemPrompt: topicRouteSystemPrompt, UserPrompt: prompt,
	})
	if err != nil {
		return false, "", err
	}
	merge, _ := result["merge"].(bool)
	topic, _ := result["topic"].(string)
	if merge {
		if candidateTopic := productTopic(existing); candidateTopic != "" {
			return true, candidateTopic, nil
		}
	}
	return merge, kccommon.NormalizeWikiTopicPath(topic), nil
}

func isTopicPage(product kccommon.Product) bool {
	return strings.EqualFold(strings.TrimSpace(metaString(product.Meta, "page_type")), "topic")
}

func productTopic(product kccommon.Product) string {
	return kccommon.NormalizeWikiTopicPath(metaString(product.Meta, "topic"))
}

func productTitle(product kccommon.Product) string {
	return strings.TrimSpace(metaString(product.Meta, "title"))
}

func productSummary(product kccommon.Product) string {
	if summary := strings.TrimSpace(metaString(product.Meta, "summary")); summary != "" {
		return summary
	}
	return strings.TrimSpace(product.Content)
}

func productEntities(product kccommon.Product) []string {
	return metaStringSlice(product.Meta, "entity_names")
}

func firstTopicString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func topicKey(topic string) string {
	return strings.ToLower(kccommon.NormalizeWikiTopicPath(topic))
}

// prepareTopicProduct normalizes topic metadata for legacy routing code. The
// active merge path does not call it to decide identity; slug remains the
// caller-provided stable page identity and is never replaced by a hash.
func prepareTopicProduct(product kccommon.Product, topic string) kccommon.Product {
	product.Meta = copyMeta(product.Meta)
	topic = kccommon.NormalizeWikiTopicPath(topic)
	if topic == "" {
		topic = productTopic(product)
	}
	product.Meta["page_type"] = "topic"
	product.Meta["topic"] = topic
	return product
}

// mergeTopicProducts folds only pages with the same canonical slug and
// preserves the first dataset-level page identity. Similar topics with
// different slugs remain separate pages.
func mergeTopicProducts(tenant, kb string, products []kccommon.Product) ([]kccommon.Product, []string) {
	groups := make(map[string][]kccommon.Product)
	order := make([]string, 0, len(products))
	for _, product := range products {
		key := wikiPageMergeKey(product)
		if key == "" {
			key = candidateIdentity(product)
		}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], product)
	}
	merged := make([]kccommon.Product, 0, len(groups))
	stale := make([]string, 0)
	for _, key := range order {
		items := groups[key]
		if len(items) == 0 {
			continue
		}
		current := items[0]
		for _, item := range items[1:] {
			if item.ID != "" && item.ID != current.ID && item.Merged && item.DocID == kb {
				stale = append(stale, item.ID)
			}
			if isTopicPage(current) {
				current = mergeTopicPage(current, item)
			} else {
				current = wikiEntityMerge(current, item)
			}
		}
		current.Merged = true
		currentID := datasetLevelID(tenant, kb, current)
		filteredStale := stale[:0]
		for _, id := range stale {
			if id != currentID {
				filteredStale = append(filteredStale, id)
			}
		}
		stale = filteredStale
		merged = append(merged, current)
	}
	// The writer's dataset-level id is derived from the surviving topic slug.
	// Remove superseded dataset rows after the replacement is written.
	seen := make(map[string]struct{}, len(stale))
	uniqueStale := make([]string, 0, len(stale))
	for _, id := range stale {
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueStale = append(uniqueStale, id)
	}
	return merged, uniqueStale
}

func mergeTopicPage(existing, incoming kccommon.Product) kccommon.Product {
	merged := wikiEntityMerge(existing, incoming)
	merged.Meta = copyMeta(merged.Meta)
	merged.Meta["slug"] = metaString(existing.Meta, "slug")
	merged.Meta["page_type"] = "topic"
	topic := kccommon.NormalizeWikiTopicPath(firstTopicString(metaString(incoming.Meta, "topic"), metaString(existing.Meta, "topic")))
	merged.Meta["topic"] = topic
	merged.Meta["title"] = firstTopicString(metaString(existing.Meta, "title"), metaString(incoming.Meta, "title"), topic)
	merged.Meta["entity_names"] = unionStrs(metaStringSlice(existing.Meta, "entity_names"), metaStringSlice(incoming.Meta, "entity_names"))
	return merged
}
