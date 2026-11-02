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

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// graphDocOutlineToolName returns a whole document's compiled skeleton in one
// call: entities plus relations, hubs first.
const graphDocOutlineToolName = "graph_doc_outline"

const graphDocOutlineToolDescription = `Get the COMPILED GRAPH SKELETON of whole documents in one call: the entities and relations the compiler extracted from each, TOGETHER, every entity carrying its mention count, type and description, hubs first.

## Two ways in
- **query** (the way in when you do NOT yet hold a doc_id): a short keyword phrase naming what the document is about. The tool locates the candidate documents for that phrase ITSELF and returns their skeletons, so you do not have to search first.
- **doc_ids** (1-3): documents you already identified in earlier tool output.

Pass either doc_ids or query; doc_ids wins when both are given.

Use it BEFORE you deep-read: one call gives you a document's key entities and the edges between them, which is far cheaper than scanning it chunk by chunk, and it tells you which entities are worth following up with graph_path or a keyword search. It also returns the source passages behind the entities so the skeleton comes with citable text.

Returns an XML <kg_doc_outline> document: <entity name type mentions description> children in descending mention order, <relation from to type> children, then one <passage chunk_id doc_id doc_name ref> element per source chunk with a <content> body. The ref attribute is the [ID:n] handle to cite.

An empty result with error="no compiled knowledge graph" is a DATASET-level absence - do not retry this tool. An empty result carrying located_nothing="true" is a QUERY-level miss: the query matched no document - rephrase it with the corpus's own wording, or locate documents with grep_chunks / search_bm25_chunks and call again with their doc_ids. Otherwise an empty result means these documents carry no compiled graph row: that is a finding about the document, not a failure - read it with list_chunks instead.`

// graphDocOutlineArgs is the JSON the model sends into InvokableRun.
type graphDocOutlineArgs struct {
	DocIDs     []string `json:"doc_ids"`
	Query      string   `json:"query,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
}

// Rendering caps: the model reads a bounded payload, not the whole graph.
const (
	graphDocOutlineMaxPassages = 3
	graphDocOutlineMaxDatasets = 10
	// graphDocOutlineLocateTopN is the chunk window one BM25 pass reads to turn
	// a query into candidate documents. Only the documents matter, and the
	// first few hits are the ones whose skeleton is worth the graph read.
	graphDocOutlineLocateTopN = 12
)

// GraphDocOutlineTool reads per-document graph panoramas via KgService. It is
// stateless: the tenant and dataset scope are injected at construction.
type GraphDocOutlineTool struct {
	tenantID   string
	datasetIDs []string
}

// NewGraphDocOutlineTool returns a GraphDocOutlineTool scoped to the given
// tenant and datasets, implementing eino's runtime.InvokableTool.
func NewGraphDocOutlineTool(tenantID string, datasetIDs []string) *GraphDocOutlineTool {
	return &GraphDocOutlineTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (g *GraphDocOutlineTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "doc_ids": {
      "type": "array",
      "description": "1-3 document ids whose compiled skeleton you want, taken from earlier tool output. Omit this and pass query instead when you do not have a doc_id yet.",
      "items": { "type": "string" },
      "minItems": 1,
      "maxItems": 3
    },
    "query": {
      "type": "string",
      "description": "A short keyword phrase naming what the document is about. Use this when you do NOT yet hold a doc_id: the tool locates the candidate documents for the phrase itself and returns their compiled skeletons. Pass either doc_ids or query.",
      "minLength": 1
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict the lookup to (at most 10). When omitted, the conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    }
  }
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("graph_doc_outline: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        graphDocOutlineToolName,
		Desc:        graphDocOutlineToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun returns the XML <kg_doc_outline> document.
func (g *GraphDocOutlineTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, graphDocOutlineToolName, g.invokableRun, argumentsInJSON)
}

func (g *GraphDocOutlineTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args graphDocOutlineArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("graph_doc_outline: parse arguments: %w", err)
	}
	docIDs := nonEmptyStrings(args.DocIDs)
	query := strings.TrimSpace(args.Query)
	// Entry trace: logged BEFORE validation and before the service call, so the
	// A/B gate can tell "never called" from "called and failed".
	common.InfoCtx(ctx, "agentic_rag: graph_doc_outline called",
		zap.Int("docs", len(docIDs)),
		zap.Bool("via_query", len(docIDs) == 0 && query != ""))
	if len(docIDs) == 0 && query == "" {
		return "", fmt.Errorf("graph_doc_outline: pass either 1-%d doc_ids or a query to locate them", kgOutlineMaxDocs)
	}
	if len(docIDs) > kgOutlineMaxDocs {
		return "", fmt.Errorf("graph_doc_outline: at most %d doc_ids allowed, got %d", kgOutlineMaxDocs, len(docIDs))
	}
	if !navToolEnabled(ctx, graphDocOutlineToolName) {
		return renderKgDocOutline(ctx, docIDs, KgExploreResult{GraphAbsent: true}), nil
	}

	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("graph_doc_outline: %w", err)
	}
	if len(datasetIDs) == 0 {
		// Bound scope is empty: short-circuit before touching the backend so the
		// tool never reads outside the conversation's allowed datasets.
		return renderKgDocOutline(ctx, docIDs, KgExploreResult{GraphAbsent: true}), nil
	}

	// Query entry: the model knows the question but not which document holds
	// it. Locating here costs one BM25 pass and saves the model a whole search
	// call plus a second outline call carrying the doc_ids that search returned.
	if len(docIDs) == 0 {
		docIDs = locateDocIDsForOutline(ctx, g.tenantID, datasetIDs, query)
		if len(docIDs) == 0 {
			return renderKgDocOutlineLocateMiss(ctx, query), nil
		}
	}

	result, err := GetKgService().DocOutline(ctx, KgDocOutlineRequest{
		TenantID:   g.tenantID,
		DatasetIDs: datasetIDs,
		DocIDs:     docIDs,
	})
	if err != nil {
		return "", fmt.Errorf("graph_doc_outline: %w", err)
	}
	if result.GraphAbsent {
		markNavToolUnavailable(ctx, graphDocOutlineToolName)
		common.DebugCtx(ctx, "graph_doc_outline: marking unavailable for conversation (no compiled knowledge graph)")
	}
	common.InfoCtx(ctx, "agentic_rag: graph_doc_outline",
		zap.Int("docs", len(docIDs)),
		zap.Int("datasets", len(datasetIDs)),
		zap.Int("entities", len(result.Entities)),
		zap.Int("relations", len(result.Relations)),
		zap.Int("passages", len(result.Passages)))
	return renderKgDocOutline(ctx, docIDs, result), nil
}

// renderKgDocOutline serialises one skeleton read. Passages carry the same
// `ref` handles the locate/deep-read tools stamp, so a fact read from the graph
// is citable exactly like one read from a chunk.
func renderKgDocOutline(ctx context.Context, docIDs []string, result KgExploreResult) string {
	passages := result.Passages
	if len(passages) > graphDocOutlineMaxPassages {
		passages = passages[:graphDocOutlineMaxPassages]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<kg_doc_outline docs=\"%d\" entities=\"%d\" relations=\"%d\" passages=\"%d\">\n",
		len(docIDs), len(result.Entities), len(result.Relations), len(passages)))
	if len(result.Entities) == 0 && len(result.Relations) == 0 {
		if result.GraphAbsent {
			b.WriteString("<note error=\"no compiled knowledge graph\">This dataset has NO compiled knowledge graph (or none in the given scope). graph_doc_outline is unavailable here - do not retry it; read the documents with list_chunks instead.</note>\n")
		} else {
			b.WriteString("<note>These documents carry no compiled graph row. That is a finding about the document, not a failure - read it with list_chunks instead.</note>\n")
		}
		b.WriteString("</kg_doc_outline>")
		return b.String()
	}
	for _, e := range result.Entities {
		b.WriteString(fmt.Sprintf("<entity name=\"%s\" type=\"%s\" mentions=\"%d\"",
			xmlEscape(e.Name), xmlEscape(e.Type), e.MentionCount))
		if e.Description != "" {
			b.WriteString(fmt.Sprintf(" description=\"%s\"", xmlEscape(truncateRunes(e.Description, 150))))
		}
		b.WriteString("/>\n")
	}
	for _, r := range result.Relations {
		b.WriteString(fmt.Sprintf("<relation from=\"%s\" to=\"%s\" type=\"%s\"/>\n",
			xmlEscape(r.From), xmlEscape(r.To), xmlEscape(r.Type)))
	}
	reg := evidenceRegistryFrom(ctx)
	if len(passages) > 0 {
		// These passages are the full chunk text with the same ref handles the
		// deep-read tools stamp. Saying so in the payload is what stops the
		// agent from paying for the same evidence twice: an outline followed by
		// list_chunks on the chunks it already carried is pure duplicated cost.
		b.WriteString("<note>The passages below are FULL chunk text with ref handles - cite them directly. Do NOT re-read them with list_chunks; that would duplicate text you already hold.</note>\n")
	}
	for _, p := range passages {
		b.WriteString(fmt.Sprintf(
			"<passage chunk_id=\"%s\" doc_id=\"%s\" doc_name=\"%s\" dataset_id=\"%s\"%s>\n",
			xmlEscape(p.ChunkID), xmlEscape(p.DocID), xmlEscape(p.DocName), xmlEscape(p.DatasetID),
			refAttrOf(reg, p.ChunkID)))
		b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(p.Content)))
		b.WriteString("</passage>\n")
	}
	b.WriteString("</kg_doc_outline>")
	return b.String()
}

// locateDocIDsForOutline turns a query into the documents whose compiled
// skeletons should be read: one lexical BM25 pass, documents taken in
// retrieval order, capped at kgOutlineMaxDocs. It is the entry point that lets
// the model reach the compiled graph before it holds any doc_id. The pass is
// lexical on purpose - that keeps the entry working on a corpus whose
// embedding backend is unavailable.
func locateDocIDsForOutline(ctx context.Context, tenantID string, datasetIDs []string, query string) []string {
	chunks, err := runtime.GetBm25Service().SearchBm25(ctx, runtime.Bm25Request{
		Queries:    []string{query},
		DatasetIDs: datasetIDs,
		TopN:       graphDocOutlineLocateTopN,
		TenantID:   tenantID,
	})
	if err != nil {
		common.WarnCtx(ctx, "graph_doc_outline: query entry locate failed",
			zap.String("query", query), zap.Error(err))
		return nil
	}
	ids := make([]string, 0, kgOutlineMaxDocs)
	seen := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		if c.DocumentID == "" || seen[c.DocumentID] {
			continue
		}
		seen[c.DocumentID] = true
		ids = append(ids, c.DocumentID)
		if len(ids) == kgOutlineMaxDocs {
			break
		}
	}
	return ids
}

// renderKgDocOutlineLocateMiss reports a query-entry miss. It is a QUERY-level
// miss - the graph itself is present - so it must neither disable the tool nor
// read as a dataset-level absence.
func renderKgDocOutlineLocateMiss(ctx context.Context, query string) string {
	var b strings.Builder
	b.WriteString("<kg_doc_outline docs=\"0\" entities=\"0\" relations=\"0\" passages=\"0\" located_nothing=\"true\">\n")
	b.WriteString(fmt.Sprintf("<note>The query %q located no document, so there is no skeleton to read. This is a QUERY-level miss, not a missing graph - the tool still works here. Rephrase it with the corpus's own wording, or locate documents with grep_chunks / search_bm25_chunks and call graph_doc_outline again with their doc_ids.</note>\n", xmlEscape(query)))
	b.WriteString("</kg_doc_outline>")
	return b.String()
}
