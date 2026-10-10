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
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/service/nav"
)

// navigateTreeToolName is the tool the model calls to route a question to the
// documents most likely to hold the answer by descending the dataset's compiled
// navigation tree.
const navigateTreeToolName = "navigate_tree"

const navigateTreeToolDescription = `Route a question to the documents most likely to hold the answer by descending the bound datasets' COMPILED NAVIGATION TREE (a topic->document outline built at compile time), then return the routed documents with their overall summaries.
USE IT EARLY, before grep_chunks / search_*_chunks, when the corpus is large or you only have a TOPIC (not a distinctive verbatim phrase) — it points you at the right document(s) in ONE cheap call instead of scanning the whole corpus. It ROUTES; it does not read document content.
Returns an XML <tree_navigation> document: a count attribute plus one <doc rank doc_id> element per routed document, each with a <summary> of that document's overall content. Feed those doc_ids to list_chunks (anchor_chunk_ids) to deep-read the routed documents.
An empty result (<tree_navigation count="0" error="...">) means this dataset has NO compiled navigation tree (or this query routed to nothing) - the error attribute says which: "no compiled navigation tree" (structure absent, fall back to grep_chunks / search_bm25_chunks / search_semantic_chunks) or no attribute (structure exists, this query just missed - rephrase and retry, or fall back to the text tools).`

// navigateTreeArgs is the JSON the model sends into InvokableRun.
type navigateTreeArgs struct {
	Query      string   `json:"query"`
	Keywords   string   `json:"keywords,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	DocScope   []string `json:"doc_scope,omitempty"`
}

// Routing/drill caps. Mirrors the harness NavigateTree bounds.
const (
	navTreeMaxDocs   = 8  // navTreeMaxDocs: documents surfaced by navigate_tree
	navSearchMaxDocs = 12 // navSearchMaxDocs: how many documents the router is asked for per dataset
)

// Empty-reason constants, mirroring the harness navigation contract so the model
// sees the same signal shape it does there.
const (
	navTreeReasonInfra       = "infra"
	navTreeReasonBadArgs     = "bad_args"
	navTreeReasonNoStructure = "no_structure"
	navTreeReasonNoDoc       = "no_doc"
)

// NavigateTreeTool routes a question to documents via the compiled navigation
// tree. It is stateless: the tenant and dataset scope are injected at
// construction from the session, and the routing backend is the production
// nav-tree service (nav.NavServiceRouter).
type NavigateTreeTool struct {
	tenantID   string
	datasetIDs []string
}

// NewNavigateTreeTool returns a NavigateTreeTool scoped to the given tenant and
// datasets, implementing eino's runtime.InvokableTool.
func NewNavigateTreeTool(tenantID string, datasetIDs []string) *NavigateTreeTool {
	return &NavigateTreeTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (g *NavigateTreeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "REQUIRED: the question (or topic) to route. Phrase it as the subject you are investigating (e.g. \"the company's 2023 revenue by segment\")."
    },
    "keywords": {
      "type": "string",
      "description": "Optional extra keywords that enrich the routing query without being required to match (free-form, comma-or-space separated)."
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict routing to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "doc_scope": {
      "type": "array",
      "description": "Optional document ids to restrict routing to (at most 10).",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["query"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("navigate_tree: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        navigateTreeToolName,
		Desc:        navigateTreeToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun routes the query over the navigation tree and returns the XML
// <tree_navigation> document.
func (g *NavigateTreeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, navigateTreeToolName, g.invokableRun, argumentsInJSON)
}

func (g *NavigateTreeTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args navigateTreeArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("navigate_tree: parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("navigate_tree: query is required and must be a non-empty string")
	}
	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("navigate_tree: %w", err)
	}
	if navToolDisabled(ctx, navigateTreeToolName) {
		// Session-level disable: this conversation's bound datasets were proven
		// to have no compiled navigation tree, so skip the backend and return the
		// same verdict the caller falls back from.
		common.DebugCtx(ctx, "navigate_tree: session-disabled; skipping backend (no compiled navigation tree)")
		return navTreeEmpty(navTreeReasonNoStructure, "no compiled navigation tree"), nil
	}
	if len(datasetIDs) == 0 {
		// Bound scope is empty: report no structure rather than reading outside
		// the conversation's allowed datasets.
		return navTreeEmpty(navTreeReasonNoStructure, "no bound datasets"), nil
	}

	// The router IS the retrieval backend for this tool; it is always non-nil,
	// and reports "no compiled tree" by returning a nil slice when no NavService
	// is installed (treated exactly like a dataset without a tree).
	router := nav.NewNavServiceRouter()
	full := strings.TrimSpace(query + " " + strings.TrimSpace(args.Keywords))

	var (
		ordered  [][2]string
		seen     = map[string]bool{}
		anyTree  bool
		anyError error
	)
	for _, kbID := range datasetIDs {
		routed, rerr := router.Route(ctx, g.tenantID, kbID, full, args.DocScope, navSearchMaxDocs)
		if rerr != nil {
			anyError = rerr
			common.DebugCtx(ctx, "navigate_tree: nav-tree descent failed",
				zap.String("kb_id", kbID), zap.Error(rerr))
			continue
		}
		if routed == nil {
			// No compiled tree for this dataset (distinct from "routed to
			// nothing", which is a non-nil empty slice).
			continue
		}
		anyTree = true
		for _, pair := range routed {
			did := strings.TrimSpace(pair[0])
			if did == "" || seen[did] {
				continue
			}
			seen[did] = true
			ordered = append(ordered, [2]string{did, strings.TrimSpace(pair[1])})
		}
	}
	if !anyTree {
		if anyError != nil {
			// A backend failure is an INFRASTRUCTURE error, not a dataset
			// verdict: no read succeeded, so "no structure" would be drawn from
			// zero evidence.
			return navTreeEmpty(navTreeReasonInfra, "nav tree descent failed"), nil
		}
		// No compiled tree in any bound dataset: a DATASET-level fact, so the
		// caller falls back to the text tools. No retrieval fallback here. Record
		// it so this conversation stops spending calls on the tool.
		disableNavTool(ctx, navigateTreeToolName)
		common.DebugCtx(ctx, "navigate_tree: disabling for conversation (no compiled navigation tree)")
		return navTreeEmpty(navTreeReasonNoStructure, "no compiled navigation tree"), nil
	}
	if len(ordered) == 0 {
		// Structure exists but THIS query reached nothing: a query-level miss,
		// carrying no error attribute (unlike infra/bad_args).
		return navTreeEmpty(navTreeReasonNoDoc, ""), nil
	}

	if len(ordered) > navTreeMaxDocs {
		ordered = ordered[:navTreeMaxDocs]
	}
	parts := []string{fmt.Sprintf(`<tree_navigation count="%d" query="%s">`, len(ordered), xmlEscape(query))}
	for i, pair := range ordered {
		if pair[1] != "" {
			parts = append(parts,
				fmt.Sprintf(`  <doc rank="%d" doc_id="%s">`, i+1, xmlEscape(pair[0])),
				fmt.Sprintf("    <summary>%s</summary>", xmlEscape(pair[1])),
				"  </doc>")
		} else {
			parts = append(parts, fmt.Sprintf(`  <doc rank="%d" doc_id="%s"/>`, i+1, xmlEscape(pair[0])))
		}
	}
	parts = append(parts, "</tree_navigation>")
	return strings.Join(parts, "\n"), nil
}

// navTreeEmpty returns the empty <tree_navigation count="0"> XML carrying an
// optional error="..." attribute, mirroring the harness navigation contract.
func navTreeEmpty(reason, label string) string {
	_ = reason // reason is carried semantically by the label; kept for parity with the harness constants
	attr := ""
	if label != "" {
		attr = ` error="` + xmlEscape(label) + `"`
	}
	return "<tree_navigation count=\"0\"" + attr + ">\n</tree_navigation>"
}
