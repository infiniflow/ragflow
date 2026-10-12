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

package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"ragflow/internal/common"
)

// metadataResolver is the agentic_rag package's view of the document-metadata
// service. It is declared here — not imported from internal/service — because
// service imports agentic_rag, so importing it back would form an import cycle.
// The service layer wires the concrete *service.MetadataService through
// SetMetadataService before an agent turn; the tool reads it via
// getMetadataService.
//
// The method set mirrors internal/service.MetadataService exactly so the concrete
// type satisfies this interface structurally (no adapter needed).
type metadataResolver interface {
	FilterDocIDsByMetaPushdown(ctx context.Context, kbIDs []string, filters []map[string]any, logic string) ([]string, bool)
	GetFlattedMetaByKBs(ctx context.Context, kbIDs []string) (common.MetaData, error)
	MetadataForDocIDs(ctx context.Context, kbIDs, docIDs []string) (map[string]map[string]any, error)
	DeclaredMetadataFields(ctx context.Context, kbIDs []string) ([]common.MetadataFieldDef, error)
}

// metadataResolverSvc is the wired metadata resolver, or nil when the deployment
// has not wired one (the tool then reports "unavailable").
var metadataResolverSvc metadataResolver

// SetMetadataService wires the document-metadata resolver used by the
// search_metadata tool. Called by the service layer (which owns the concrete
// *service.MetadataService) before an agent run; pass nil to clear it.
func SetMetadataService(s metadataResolver) { metadataResolverSvc = s }

func getMetadataService() metadataResolver { return metadataResolverSvc }

const metadataSearchToolName = "search_metadata"

const metadataSearchToolDescription = `Document-metadata SELECTOR: given metadata conditions, return the document ids (doc_ids) whose metadata matches — it runs NO content retrieval of its own.

Use this to narrow the corpus to the documents that satisfy a metadata predicate (e.g. author = "X", year contains "2024", doc_type in ["report","memo"]) before reading them. The returned doc_ids are a handle the model spends on other tools: pass them as search_chunks' "doc_scope" to search ONLY inside those documents, or as list_chunks' "doc_id" for a deep read.

## Arguments
- "filters" (required, 1-10): each is {key, op, value}. key is a metadata field name (see the dataset's available fields); op is one of: contains | = | start with | end with | not contains | in | not in | empty | not empty; value is the comparison value — a single value for scalar ops, or a list for "in"/"not in".
- "logic" (optional): "and" (default) or "or" — how multiple filters combine.
- "dataset_ids" (optional): restrict to these dataset ids; omit to use the conversation's bound datasets.

## Output (JSON)
{"kind":"search_metadata","doc_ids":[...],"documents":[{"doc_id":...,"metadata":{...}}],"filters":[...]}. doc_ids is the result to act on; documents carries each matched doc's own metadata values so you can answer from the selection without another call. An empty doc_ids with a "note" means no document matched (loosen the filters) or the dataset has no metadata (use search_chunks / grep_chunks instead).`

type metadataFilterArg struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type metadataSearchArgs struct {
	Filters    []metadataFilterArg `json:"filters"`
	Logic      string              `json:"logic,omitempty"`
	DatasetIDs []string            `json:"dataset_ids,omitempty"`
}

// MetadataSearchTool resolves documents by metadata and returns their doc_ids.
// The tenant and dataset scope are injected at construction from the session.
type MetadataSearchTool struct {
	tenantID   string
	datasetIDs []string
}

// NewMetadataSearchTool returns a MetadataSearchTool scoped to the given tenant
// and datasets, implementing eino's runtime.InvokableTool.
func NewMetadataSearchTool(tenantID string, datasetIDs []string) *MetadataSearchTool {
	return &MetadataSearchTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (m *MetadataSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "filters": {
      "type": "array",
      "description": "REQUIRED: 1-10 metadata conditions. Each: {key, op, value}. key is a metadata field name; op is one of contains | = | start with | end with | not contains | in | not in | empty | not empty; value is a single value for scalar ops or a list for in/not in.",
      "items": {
        "type": "object",
        "properties": {
          "key": { "type": "string" },
          "op": { "type": "string" },
          "value": { }
        },
        "required": ["key", "op"]
      },
      "minItems": 1,
      "maxItems": 10
    },
    "logic": {
      "type": "string",
      "description": "How multiple filters combine: and (default) or or.",
      "enum": ["and", "or"]
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict to (at most 10). When omitted, the conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["filters"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("search_metadata: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        metadataSearchToolName,
		Desc:        metadataSearchToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun resolves documents by metadata and returns their doc_ids.
func (m *MetadataSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, metadataSearchToolName, m.invokableRun, argumentsInJSON)
}

func (m *MetadataSearchTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args metadataSearchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_metadata: parse arguments: %w", err)
	}
	filters := normalizeMetadataFilters(args.Filters)
	if len(filters) == 0 {
		return metadataSearchResult([]string{}, nil,
			"No metadata conditions given. search_metadata needs at least one {key, op, value} filter; otherwise use search_chunks / grep_chunks to locate by content.")
	}
	svc := getMetadataService()
	if svc == nil {
		return metadataSearchResult([]string{}, nil,
			"search_metadata is unavailable in this deployment (no metadata resolver is wired). Use search_chunks / grep_chunks / list_chunks.")
	}
	kbIDs, err := resolveDatasetScope(m.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("search_metadata: %w", err)
	}
	if len(kbIDs) == 0 {
		return metadataSearchResult([]string{}, nil,
			"search_metadata needs a dataset scope; none is bound to this conversation.")
	}
	known := metadataKnownFields(ctx, svc, kbIDs)
	var bad []string
	for _, f := range filters {
		if key, _ := f["key"].(string); !known[key] {
			bad = append(bad, key)
		}
	}
	if len(bad) > 0 {
		return metadataSearchResult([]string{}, nil,
			fmt.Sprintf("Metadata key(s) %v do not exist in this dataset. Available: %s.", bad, metadataAvailableKeys(known)))
	}
	logic := args.Logic
	if logic != "or" {
		logic = "and"
	}
	docIDs, ok := svc.FilterDocIDsByMetaPushdown(ctx, kbIDs, filters, logic)
	if !ok {
		metas, ferr := svc.GetFlattedMetaByKBs(ctx, kbIDs)
		if ferr != nil || metas == nil {
			return metadataSearchResult([]string{}, nil,
				"The document-metadata index could not be read (infrastructure failure — NOT a statement about the dataset). Fall back to search_chunks / grep_chunks / list_chunks.")
		}
		docIDs = common.MetaFilter(metas, &common.MetaFilterInput{
			Conditions: metadataConditions(filters),
			Logic:      logic,
		})
	}
	docIDs = intersectDocumentScope(ctx, docIDs)
	if len(docIDs) == 0 {
		return metadataSearchResult([]string{}, nil,
			"No documents match the given metadata conditions. Loosen or change the filters, or use search_chunks / grep_chunks for unfiltered search.")
	}
	docs := metadataContextDocs(ctx, svc, kbIDs, docIDs, known)
	return metadataSearchResult(docIDs, docs, "")
}

// normalizeMetadataFilters turns the model-supplied filters into the {key, op,
// value} form the resolver expects, coercing a value that arrives as a list into
// a single keyword for scalar ops (in / not in keep their list). An entry without
// a key or op is dropped so a malformed entry cannot ride along as a match-all.
func normalizeMetadataFilters(in []metadataFilterArg) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, f := range in {
		key := strings.TrimSpace(f.Key)
		op := strings.TrimSpace(f.Op)
		if key == "" || op == "" {
			continue
		}
		out = append(out, map[string]any{
			"key":   key,
			"op":    op,
			"value": normalizeMetadataValue(f.Value, op),
		})
	}
	return out
}

// normalizeMetadataValue collapses a list value to its first non-empty element
// for scalar ops (one condition = one keyword); list ops keep the list. Mirrors
// the runtime package's NormalizeMetadataValue.
func normalizeMetadataValue(value any, op string) any {
	if op == "in" || op == "not in" {
		return value
	}
	items, ok := value.([]any)
	if !ok {
		return value
	}
	flat := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(it)); s != "" {
			flat = append(flat, s)
		}
	}
	if len(flat) == 0 {
		return nil
	}
	return flat[0]
}

// metadataConditions adapts the resolver filters to common.MetaFilter's
// condition shape, normalising operator aliases.
func metadataConditions(filters []map[string]any) []common.MetaCondition {
	out := make([]common.MetaCondition, 0, len(filters))
	for _, f := range filters {
		key, _ := f["key"].(string)
		op, _ := f["op"].(string)
		out = append(out, common.MetaCondition{
			Key:      key,
			Operator: common.NormalizeOperator(op),
			Value:    f["value"],
		})
	}
	return out
}

// metadataKnownFields merges the dataset's declared and observed metadata fields
// into the set the tool will accept as a valid filter key.
func metadataKnownFields(ctx context.Context, svc metadataResolver, kbIDs []string) map[string]bool {
	known := map[string]bool{}
	if declared, err := svc.DeclaredMetadataFields(ctx, kbIDs); err == nil {
		for _, d := range declared {
			if d.Key != "" {
				known[d.Key] = true
			}
		}
	}
	if metas, err := svc.GetFlattedMetaByKBs(ctx, kbIDs); err == nil {
		for k := range metas {
			known[k] = true
		}
	}
	return known
}

func metadataAvailableKeys(known map[string]bool) string {
	if len(known) == 0 {
		return "NONE — this dataset has no metadata; use search_chunks / grep_chunks"
	}
	keys := make([]string, 0, len(known))
	for k := range known {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	const maxHint = 30
	if len(keys) > maxHint {
		keys = keys[:maxHint]
	}
	return strings.Join(keys, ", ")
}

// metadataContextDocs renders the per-document context block: each matched
// document's own metadata values, restricted to the known fields, in doc_ids
// order, capped at metadataContextDocsMax. A nil perDoc (the read failed or the
// resolver answered nothing) yields nil — the ids alone are still a complete
// result.
func metadataContextDocs(ctx context.Context, svc metadataResolver, kbIDs, docIDs []string, known map[string]bool) []map[string]any {
	perDoc, err := svc.MetadataForDocIDs(ctx, kbIDs, docIDs)
	if err != nil || len(perDoc) == 0 {
		return nil
	}
	const maxDocs = 20
	out := make([]map[string]any, 0, len(docIDs))
	for _, docID := range docIDs {
		if len(out) >= maxDocs {
			break
		}
		fields, ok := perDoc[docID]
		if !ok {
			continue
		}
		values := make(map[string]any, len(fields))
		for k, v := range fields {
			if !known[k] {
				continue
			}
			values[k] = v
		}
		if len(values) == 0 {
			continue
		}
		out = append(out, map[string]any{"doc_id": docID, "metadata": values})
	}
	return out
}

// metadataSearchResult serialises the tool's JSON payload. docIDs is always a
// (possibly empty) slice; note carries a model-readable explanation on a miss or
// infrastructure failure.
func metadataSearchResult(docIDs []string, docs []map[string]any, note string) (string, error) {
	if docIDs == nil {
		docIDs = []string{}
	}
	payload := map[string]any{
		"kind":    "search_metadata",
		"doc_ids": docIDs,
	}
	if docs != nil {
		payload["documents"] = docs
	}
	if note != "" {
		payload["note"] = note
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("search_metadata: marshal result: %w", err)
	}
	return string(b), nil
}
