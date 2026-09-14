package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// rewriteDuplicateTreeNames rewrites duplicate summary titles whose
// descriptions differ, mirroring Python
// chunk_post_processor.rewrite_duplicate_tree_names (called by
// Compiler._compile_tree_templates before raptor_tree_to_graph): the graph
// projection uses titles as entity names and relation endpoints, so two nodes
// sharing a title render as one ambiguous node.
//
// Two passes, matching Python:
//  1. LLM pass — group summary/root products by title; for a group with ≥2
//     members and ≥2 distinct descriptions, ask the LLM for a concise distinct
//     title per node (best-effort: a failed group keeps its titles).
//  2. Deterministic pass — walk every product in slice order and suffix any
//     repeated title with " (n)", enforcing the distinctness contract even
//     when the LLM output collides or the call failed.
//
// A rewrite updates both Meta["title"] and the first line of Content (the
// title IS the first line by construction, see titleOf), keeping the two in
// sync for downstream indexing.
func rewriteDuplicateTreeNames(ctx context.Context, deps common.Deps, llmID string, products []common.Product) {
	if deps.Chat == nil || len(products) == 0 {
		return
	}

	// ---- LLM pass ----
	groups := map[string][]int{}
	for i := range products {
		if !isTreeSummaryProduct(products[i]) {
			continue
		}
		title := productTitle(products[i])
		if title == "" {
			continue
		}
		groups[title] = append(groups[title], i)
	}
	titles := make([]string, 0, len(groups))
	for t := range groups {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	for _, title := range titles {
		idxs := groups[title]
		if len(idxs) < 2 {
			continue
		}
		descSet := map[string]struct{}{}
		for _, i := range idxs {
			descSet[products[i].Content] = struct{}{}
		}
		if len(descSet) < 2 {
			continue
		}
		if err := rewriteTitleGroup(ctx, deps, llmID, title, products, idxs); err != nil {
			log.Printf("tree: duplicate title rewrite failed for title=%q (keeping titles): %v", title, err)
		}
	}

	// ---- Deterministic uniqueness pass (always, matching Python) ----
	used := map[string]int{}
	for i := range products {
		if !isTreeSummaryProduct(products[i]) {
			continue
		}
		title := productTitle(products[i])
		if title == "" {
			continue
		}
		used[title]++
		if occurrence := used[title]; occurrence > 1 {
			setProductTitle(&products[i], fmt.Sprintf("%s (%d)", title, occurrence))
		}
	}
}

// isTreeSummaryProduct reports whether p is a tree node whose title matters
// (cluster summaries and the synthetic root).
func isTreeSummaryProduct(p common.Product) bool {
	switch p.Meta["kind"] {
	case "summary", "root":
		return true
	}
	return false
}

func productTitle(p common.Product) string {
	t, _ := p.Meta["title"].(string)
	return strings.TrimSpace(t)
}

// setProductTitle rewrites the title in Meta and replaces the first line of
// Content so titleOf(content) stays consistent with the meta title.
func setProductTitle(p *common.Product, title string) {
	p.Meta["title"] = title
	if idx := strings.IndexByte(p.Content, '\n'); idx >= 0 {
		p.Content = title + p.Content[idx:]
	} else {
		p.Content = title
	}
}

// renameOut is one LLM rename instruction.
type renameOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// rewriteTitleGroup asks the LLM for one distinct title per member of a
// duplicate-title group and applies the results.
func rewriteTitleGroup(ctx context.Context, deps common.Deps, llmID, title string, products []common.Product, idxs []int) error {
	type renameItem struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}

	items := make([]renameItem, 0, len(idxs))
	for pos, i := range idxs {
		items = append(items, renameItem{ID: strconv.Itoa(pos), Description: products[i].Content})
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return err
	}
	prompt := "The following tree nodes currently have the same title but describe different content. " +
		"Give each node a concise, distinct human-readable title. Preserve the original language, " +
		"do not add numbering unless necessary, and return only a JSON array of objects with the " +
		"same ids and a name field.\n\n" +
		"Current title: " + title + "\n" +
		"Nodes: " + string(itemsJSON)

	temp := 0.0
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: "You rename duplicate tree node titles for display.",
		UserPrompt:   prompt,
		JSONMode:     true,
		Temperature:  &temp,
	})
	if err != nil {
		return err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return fmt.Errorf("empty rename response")
	}

	var rewrites []renameOut
	if err := unmarshalRenames(resp.Content, &rewrites); err != nil {
		return err
	}
	for _, r := range rewrites {
		newTitle := strings.TrimSpace(r.Name)
		if newTitle == "" {
			continue
		}
		pos, convErr := strconv.Atoi(strings.TrimSpace(r.ID))
		if convErr != nil || pos < 0 || pos >= len(idxs) {
			// A hallucinated id must not touch an unrelated product.
			continue
		}
		setProductTitle(&products[idxs[pos]], newTitle)
	}
	return nil
}

// unmarshalRenames parses the LLM rename array, tolerating code fences.
func unmarshalRenames(raw string, out *[]renameOut) error {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if err := json.Unmarshal([]byte(s), out); err == nil {
		return nil
	}
	// Fallback: the model may have wrapped the array in extra prose — grab the
	// first '[' … last ']' span.
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return fmt.Errorf("no JSON array in rename response")
	}
	return json.Unmarshal([]byte(s[start:end+1]), out)
}
