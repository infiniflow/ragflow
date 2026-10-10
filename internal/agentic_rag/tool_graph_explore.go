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

	"ragflow/internal/common"
)

// graphExploreToolName is the tool the model calls to walk the compiled
// knowledge graph.
const graphExploreToolName = "graph_explore"

const graphExploreToolDescription = `Explore the COMPILED KNOWLEDGE GRAPH of the bound datasets: it seeds the entities most relevant to your query, hops along their RELATIONS, and returns the subgraph (entities + relations) plus the source passages behind the relevant nodes.
USE IT for RELATIONAL and MULTI-HOP questions - "who is connected to X", "which person/place/thing links A to B", "what does A have to do with B" - where the answer depends on an EDGE between entities rather than on one passage. It complements the text tools: the text tools prove what a document SAYS, the graph walk proves what the corpus CONNECTS.
Returns an XML <kg_explore> document: a <subgraph> element whose <entity name type/> and <relation from to type/> children are the walk result, then one <passage chunk_id doc_id doc_name ref> element per source chunk, each with a <content> body. The ref attribute is the [ID:n] handle to cite; cite only passages you actually read here or with list_chunks.
An empty result (<note> with no subgraph) means this dataset has NO compiled knowledge graph in scope - do not retry it, go back to grep_chunks / search_bm25_chunks / search_semantic_chunks.`

// graphExploreArgs is the JSON the model sends into InvokableRun.
type graphExploreArgs struct {
	Query      string   `json:"query"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	DocScope   []string `json:"doc_scope,omitempty"`
}

// Rendering caps: a walk can surface hundreds of nodes and the model reads a
// bounded payload, not the whole graph.
const (
	graphExploreMaxEntities  = 40
	graphExploreMaxRelations = 60
	graphExploreMaxPassages  = 6
)

// GraphExploreTool walks the compiled knowledge graph via KgService. It is
// stateless: the tenant and dataset scope are injected at construction from the
// session.
type GraphExploreTool struct {
	tenantID   string
	datasetIDs []string
}

// NewGraphExploreTool returns a GraphExploreTool scoped to the given tenant and
// datasets, implementing eino's runtime.InvokableTool.
func NewGraphExploreTool(tenantID string, datasetIDs []string) *GraphExploreTool {
	return &GraphExploreTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (g *GraphExploreTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "REQUIRED: the relational question to walk the graph for, phrased as entities plus the relation between them (e.g. \\\"which organization connects Marie Curie to the Radium Institute\\\"). Use the question's own proper nouns."
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict the walk to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "doc_scope": {
      "type": "array",
      "description": "Optional document ids to restrict the walk to (at most 10).",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["query"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("graph_explore: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        graphExploreToolName,
		Desc:        graphExploreToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun walks the graph and returns the XML <kg_explore> document.
func (g *GraphExploreTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, graphExploreToolName, g.invokableRun, argumentsInJSON)
}

func (g *GraphExploreTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args graphExploreArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("graph_explore: parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("graph_explore: query is required and must be a non-empty string")
	}
	if !navToolEnabled(ctx, graphExploreToolName) {
		// Marked unavailable for this conversation (bound datasets have no
		// compiled knowledge graph): skip the backend and return the same
		// "no graph" note the caller falls back from.
		common.DebugCtx(ctx, "graph_explore: marked unavailable; skipping backend (no compiled knowledge graph)")
		return renderKgExplore(ctx, query, KgExploreResult{}), nil
	}
	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("graph_explore: %w", err)
	}
	if len(datasetIDs) == 0 {
		// Bound scope is empty: short-circuit before touching the backend so the
		// tool never reads outside the conversation's allowed datasets.
		return renderKgExplore(ctx, query, KgExploreResult{}), nil
	}

	result, err := GetKgService().Explore(ctx, KgExploreRequest{
		TenantID:   g.tenantID,
		DatasetIDs: datasetIDs,
		DocScope:   args.DocScope,
		Query:      query,
	})
	if err != nil {
		return "", fmt.Errorf("graph_explore: %w", err)
	}
	if len(result.Entities) == 0 && len(result.Relations) == 0 {
		// A DATASET-level absence (no compiled knowledge graph in scope): disable
		// the tool for the rest of the conversation so the agent stops spending
		// calls on it and falls back to the text tools.
		markNavToolUnavailable(ctx, graphExploreToolName)
		common.DebugCtx(ctx, "graph_explore: marking unavailable for conversation (no compiled knowledge graph)")
	}
	return renderKgExplore(ctx, query, result), nil
}

// renderKgExplore serialises one walk result. Passages carry the same `ref`
// handles the locate/deep-read tools stamp, so a fact read from the graph is
// citable exactly like one read from a chunk.
func renderKgExplore(ctx context.Context, query string, result KgExploreResult) string {
	entities := result.Entities
	if len(entities) > graphExploreMaxEntities {
		entities = entities[:graphExploreMaxEntities]
	}
	relations := result.Relations
	if len(relations) > graphExploreMaxRelations {
		relations = relations[:graphExploreMaxRelations]
	}
	passages := result.Passages
	if len(passages) > graphExploreMaxPassages {
		passages = passages[:graphExploreMaxPassages]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"<kg_explore query=\"%s\" entities=\"%d\" relations=\"%d\" passages=\"%d\">\n",
		xmlEscape(query), len(entities), len(relations), len(passages)))

	if len(entities) == 0 && len(relations) == 0 {
		b.WriteString("<note>This dataset has NO compiled knowledge graph (or none in the given scope). graph_explore is unavailable here - use grep_chunks / search_bm25_chunks / search_semantic_chunks / list_chunks instead.</note>\n")
		b.WriteString("</kg_explore>")
		return b.String()
	}

	b.WriteString("<subgraph>\n")
	for _, e := range entities {
		b.WriteString(fmt.Sprintf("<entity name=\"%s\" type=\"%s\"", xmlEscape(e.Name), xmlEscape(e.Type)))
		if e.Description != "" {
			b.WriteString(fmt.Sprintf(" description=\"%s\"", xmlEscape(truncateRunes(e.Description, 300))))
		}
		b.WriteString("/>\n")
	}
	for _, r := range relations {
		b.WriteString(fmt.Sprintf("<relation from=\"%s\" to=\"%s\" type=\"%s\"/>\n",
			xmlEscape(r.From), xmlEscape(r.To), xmlEscape(r.Type)))
	}
	b.WriteString("</subgraph>\n")

	// Passages are stamped on serve so a citation names a handle the model saw.
	reg := evidenceRegistryFrom(ctx)
	for _, p := range passages {
		b.WriteString(fmt.Sprintf(
			"<passage chunk_id=\"%s\" doc_id=\"%s\" doc_name=\"%s\" dataset_id=\"%s\"%s>\n",
			xmlEscape(p.ChunkID), xmlEscape(p.DocID), xmlEscape(p.DocName), xmlEscape(p.DatasetID),
			refAttrOf(reg, p.ChunkID)))
		b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(p.Content)))
		b.WriteString("</passage>\n")
	}
	if len(passages) == 0 {
		b.WriteString("<note>The subgraph has no source passages in scope. Treat the nodes and edges above as a map of what the corpus connects, and read the documents the nodes name with grep_chunks / list_chunks before answering.</note>\n")
	}
	b.WriteString("</kg_explore>")
	return b.String()
}
