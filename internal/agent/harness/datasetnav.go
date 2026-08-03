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

package harness

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
	"ragflow/internal/service/nav"
)

// Dataset-nav router tunables mirror Python navigation.py.
const (
	navMaxDocs          = 8
	navMaxClusters      = 500
	navChildrenPageSize = 1000
	navTreeMaxDepth     = 6
	navTreeMaxLeaves    = 300
)

// navSelectSystem mirrors Python _NAV_SELECT_SYSTEM.
const navSelectSystem = `You are routing a question through a dataset's navigation tree.

You are given a QUESTION and a numbered list of {noun}, each with a name and a short description.
Choose the {noun} most likely to contain information relevant to answering the question.

Rules:
1. Judge only from the names and descriptions shown.
2. Be selective — include an item only if it is plausibly relevant. Include several when several are equally plausible.
3. If none are clearly relevant, return an empty list.
4. Return the bracketed index numbers of the chosen {noun}.

Output ONLY JSON, no prose, no code fences:
{"relevant": [<index>, ...]}`

type navSelectVerdict struct {
	Relevant []int `json:"relevant"`
}

// NavigateDatasetByTree walks the dataset nav tree with two LLM passes
// (cluster-select → document-select) and returns the routed doc_ids (capped at
// navMaxDocs). This is the LLM two-round selection (P1b) implemented in the
// harness package, which can import the chat invoker (agent/tool cannot without
// an import cycle). It routes only — it does not retrieve.
func NavigateDatasetByTree(ctx context.Context, db *gorm.DB, ns nav.NavService, tenantID, kbID, query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	// 1. List top-level clusters.
	clusters, _, err := ns.ListClusters(ctx, tenantID, kbID, 0, navMaxClusters)
	if err != nil {
		log.Printf("datasetnav: list clusters failed: %v", err)
		return nil
	}
	if len(clusters) == 0 {
		return nil
	}
	clusterItems := make([]navSelectItem, len(clusters))
	for i, c := range clusters {
		clusterItems[i] = navSelectItem{Name: c.Name, Description: c.Description, DocCount: c.DocCount}
	}

	// 2. LLM selects relevant clusters.
	selected := askNavSelect(ctx, db, query, "clusters", clusterItems, navMaxClusters)
	if len(selected) == 0 {
		return nil
	}

	// 3. BFS-descend selected clusters to document leaves.
	leaves := collectNavLeaves(ctx, ns, tenantID, kbID, selected)

	// 4. LLM selects relevant documents.
	docs := askNavSelect(ctx, db, query, "documents", leaves, navTreeMaxLeaves)

	// Dedup + cap.
	seen := map[string]bool{}
	var routed []string
	for _, d := range docs {
		if d.DocID == "" || seen[d.DocID] {
			continue
		}
		seen[d.DocID] = true
		routed = append(routed, d.DocID)
		if len(routed) >= navMaxDocs {
			break
		}
	}
	return routed
}

// navSelectItem is a renderable cluster/document with a name + description.
type navSelectItem struct {
	Name        string
	Description string
	DocCount    int
	DocID       string
}

// askNavSelect renders items as a numbered list and asks the model which indices
// are relevant. Returns the selected items (a subset). Mirrors _ask_nav_select.
func askNavSelect(ctx context.Context, db *gorm.DB, query, noun string, items []navSelectItem, maxItems int) []navSelectItem {
	if len(items) == 0 {
		return nil
	}
	capped := items
	if len(capped) > maxItems {
		capped = capped[:maxItems]
	}
	var b strings.Builder
	for i, it := range capped {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = fmt.Sprintf("item-%d", i)
		}
		desc := strings.Join(strings.Fields(it.Description), " ")
		if len(desc) > 300 {
			desc = desc[:300]
		}
		b.WriteString(fmt.Sprintf("[%d] %s", i, name))
		if it.DocCount > 0 {
			b.WriteString(fmt.Sprintf(" [%d docs]", it.DocCount))
		}
		b.WriteString(": " + desc + "\n")
	}

	system := strings.ReplaceAll(navSelectSystem, "{noun}", noun)
	user := fmt.Sprintf("Question:\n%s\n\n%s (numbered):\n%s\n\nOutput JSON:", query, strings.Title(noun), b.String())

	inv := chat.GetDefaultInvoker()
	if inv == nil {
		log.Printf("datasetnav: LLM %s selection skipped (chat invoker not configured)", noun)
		return nil
	}
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: system},
			{Role: schema.User, Content: user},
		},
	})
	if err != nil {
		log.Printf("datasetnav: LLM %s selection failed: %v", noun, err)
		return nil
	}
	var v navSelectVerdict
	if err := unmarshalModelJSON(resp.Content, &v); err != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []navSelectItem
	for _, idx := range v.Relevant {
		if idx < 0 || idx >= len(capped) || seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, capped[idx])
	}
	return out
}

// collectNavLeaves BFS-descents selected clusters to document leaves. Mirrors
// Python _collect_nav_leaves.
func collectNavLeaves(ctx context.Context, ns nav.NavService, tenantID, kbID string, selected []navSelectItem) []navSelectItem {
	type node struct {
		name  string
		depth int
	}
	frontier := make([]node, 0, len(selected))
	for _, c := range selected {
		if c.Name != "" {
			frontier = append(frontier, node{c.Name, 0})
		}
	}
	var leaves []navSelectItem
	seenDocs := map[string]bool{}
	seenNodes := map[string]bool{}
	for len(frontier) > 0 && len(leaves) < navTreeMaxLeaves {
		cur := frontier[0]
		frontier = frontier[1:]
		if seenNodes[cur.name] {
			continue
		}
		seenNodes[cur.name] = true
		children, _, err := ns.ListChildren(ctx, tenantID, kbID, cur.name, 0, navChildrenPageSize)
		if err != nil {
			continue
		}
		for _, ch := range children {
			if ch.Type == "doc" {
				did := strings.TrimSpace(ch.DocID)
				if did != "" && !seenDocs[did] {
					seenDocs[did] = true
					leaves = append(leaves, navSelectItem{Name: ch.Name, Description: ch.Description, DocID: did})
					if len(leaves) >= navTreeMaxLeaves {
						break
					}
				}
			} else if ch.Type == "cluster" && ch.Name != "" && cur.depth+1 < navTreeMaxDepth {
				frontier = append(frontier, node{ch.Name, cur.depth + 1})
			}
		}
	}
	return leaves
}
