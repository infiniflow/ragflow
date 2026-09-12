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

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/dao"
	"ragflow/internal/service/nav"
)

// Content-recall fallback tunables, mirroring Python _content_recall_docs
// (navigation.py:387-394, :539, :547-549).
const (
	// datasetNavRecallTopN is _NAV_RECALL_TOP_N: chunk candidates fetched
	// before doc aggregation.
	datasetNavRecallTopN = 40
	// datasetNavRecallMinScore is the literal similarity_threshold Python
	// passes (:548).
	datasetNavRecallMinScore = 0.2
	// datasetNavRecallVectorWeight is the hybrid vector blend Python hardcodes
	// when an embedder exists (:539): `vector_weight = 0.3 if embd_mdl else 0`.
	datasetNavRecallVectorWeight = 0.3
)

// datasetNavigationToolName mirrors Python's dataset_navigation_by_tree router
// tool. It navigates the dataset nav tree and returns the doc_ids to read.
const datasetNavigationToolName = "dataset_navigation_by_tree"

const datasetNavigationToolDescription = "Navigate a dataset's navigation tree by topic and return the document ids that are likely relevant."

// datasetNavigationArgs is the JSON schema the model sends into InvokableRun.
type datasetNavigationArgs struct {
	Topic      string   `json:"topic"`
	Keywords   string   `json:"keywords,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	// DocScope restricts the routed documents, mirroring Python's
	// dataset_navigation_by_tree(doc_scope: list[str] | None = None).
	DocScope []string `json:"doc_scope,omitempty"`
	MaxDocs  int      `json:"max_docs,omitempty"`
}

// datasetNavigationResult is the JSON shape returned to the model.
type datasetNavigationResult struct {
	Docs     []string `json:"docs,omitempty"`
	Error    string   `json:"_ERROR,omitempty"`
	NotFound bool     `json:"not_found,omitempty"`
}

// datasetNavigationDefaultMaxDocs caps the number of doc_ids returned.
const datasetNavigationDefaultMaxDocs = 8

// DatasetNavigationByTree is the dataset-navigation router tool. Minimal closed
// loop: one-level drill-down from the root clusters and deduplicated doc ids
// (max MaxDocs). LLM-guided multi-level selection is deferred.
type DatasetNavigationByTree struct {
	defaults datasetNavigationArgs
}

// NewDatasetNavigationByTree returns a DatasetNavigationByTree implementing
// eino's tool.InvokableTool interface.
func NewDatasetNavigationByTree() *DatasetNavigationByTree {
	return NewDatasetNavigationByTreeWithDefaults(datasetNavigationArgs{})
}

// NewDatasetNavigationByTreeWithDefaults returns a DatasetNavigationByTree with
// node-level defaults.
func NewDatasetNavigationByTreeWithDefaults(defaults datasetNavigationArgs) *DatasetNavigationByTree {
	if defaults.MaxDocs <= 0 {
		defaults.MaxDocs = datasetNavigationDefaultMaxDocs
	}
	return &DatasetNavigationByTree{defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (d *DatasetNavigationByTree) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: datasetNavigationToolName,
		Desc: datasetNavigationToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"topic": {
				Type:     schema.String,
				Desc:     "The topic to navigate to. Use the core subject from the original request.",
				Required: true,
			},
			"keywords": {
				Type: schema.String,
				Desc: "Optional additional keywords to disambiguate the topic.",
			},
			"doc_scope": {
				Type: schema.Array,
				Desc: "Optional doc ids to restrict the navigation to.",
			},
		}),
	}, nil
}

// InvokableRun executes the tool. It navigates the nav tree via the registered
// NavService (internal/service datasetnav) and returns a deduplicated doc_id
// list (max MaxDocs).
func (d *DatasetNavigationByTree) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args datasetNavigationArgs
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("dataset_navigation: parse arguments: %w", err)
		}
	}
	args = d.mergeDefaults(args)
	if args.Topic == "" {
		return "", fmt.Errorf("dataset_navigation: topic is required")
	}
	// Per-request max_docs overrides the node default; default to a sane cap.
	maxDocs := args.MaxDocs
	if maxDocs <= 0 {
		maxDocs = datasetNavigationDefaultMaxDocs
	}

	ns := nav.GetNavService()
	if ns == nil {
		return datasetNavigationJSON(datasetNavigationResult{
			Error: "dataset navigation service not initialized (SetNavService must be called at bootstrap)",
		}), nil
	}

	tenantID := canvasTenantID(ctx)
	datasetIDs := canvasDatasetIDs(ctx, args.DatasetIDs)
	if tenantID == "" || len(datasetIDs) == 0 {
		return datasetNavigationJSON(datasetNavigationResult{
			NotFound: true,
			Error:    "dataset navigation requires a tenant and dataset context",
		}), nil
	}

	// Route RELEVANT docs by querying the nav tree with the topic (semantic KNN).
	// The topic is the routing signal — we must not return arbitrary doc ids.
	query := strings.TrimSpace(args.Topic + " " + args.Keywords)

	// Honor the supplied doc scope. Python's dataset_navigation_by_tree threads
	// tools.scoped_doc_ids(doc_scope) into BOTH its tree walk and its
	// content-recall fallback; the canvas context carries no session scope, so
	// only the caller-supplied scope applies here. It is enforced in collect()
	// rather than only at the Search call, because the cluster-walk fallback's
	// ListClusters/ListChildren take no scope argument.
	docScope := compactStrings(args.DocScope)
	scopeSet := make(map[string]struct{}, len(docScope))
	for _, id := range docScope {
		scopeSet[id] = struct{}{}
	}
	inScope := func(id string) bool {
		if len(scopeSet) == 0 {
			return true
		}
		_, ok := scopeSet[id]
		return ok
	}

	seen := map[string]struct{}{}
	var docs []string
	collect := func(id string) {
		if id == "" || !inScope(id) {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		if len(docs) >= maxDocs {
			return
		}
		seen[id] = struct{}{}
		docs = append(docs, id)
	}

	// Primary: semantic search over each dataset's nav tree. The scope is
	// forwarded so the service also trims each cluster's returned coverage to it
	// (a cluster that merely OVERLAPS the scope must not surface extra docs).
	for _, datasetID := range datasetIDs {
		hits, err := ns.Search(ctx, tenantID, datasetID, query, nil, docScope, maxDocs)
		if err != nil {
			continue
		}
		for _, h := range hits {
			collect(h.DocID)
			for _, id := range h.DocIDs {
				collect(id)
			}
		}
		if len(docs) >= maxDocs {
			break
		}
	}

	// Fallback 1 — content recall (Python _content_recall_docs,
	// navigation.py:514-565, the miss-tier of dataset_navigation_by_tree): when
	// no compiled tree routed — the nav rows do not exist or matched nothing —
	// recall documents by chunk CONTENT. A plain hybrid retrieval runs over the
	// datasets' chunk index and the hits aggregate to docs most-hit-first: a
	// question matching detail that only lives in a document BODY never appears
	// in the tree, so the retrieval that reads real chunk text is what catches
	// it. The caller's doc scope is forwarded as DocScope (Python forwards it
	// as doc_ids to the retrieval), and collect()'s inScope still applies.
	if len(docs) == 0 {
		threshold := datasetNavRecallMinScore
		w := datasetNavRecallVectorWeight
		chunks, err := GetRetrievalService().Search(ctx, dao.DB, RetrievalRequest{
			Query:                  query,
			DatasetIDs:             datasetIDs,
			TopN:                   datasetNavRecallTopN,
			SimilarityThreshold:    &threshold,
			VectorSimilarityWeight: &w,
			TenantID:               tenantID,
			DocScope:               docScope,
			RetrievalFrom:          "dataset",
		})
		if err != nil {
			log.Printf("[Dataset navigation] content-recall retrieval failed: %v", err)
		} else {
			// Python :557-563 — the retrieval's doc_aggs read in order; the ES
			// aggregation orders by hit count descending, so the same order is
			// derived from the flat chunk hits here.
			order := []string{}
			counts := map[string]int{}
			for _, c := range chunks {
				did := strings.TrimSpace(c.DocumentID)
				if did == "" {
					continue
				}
				if _, ok := counts[did]; !ok {
					order = append(order, did)
				}
				counts[did]++
			}
			sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
			log.Printf("[Dataset navigation] Content recall found %d candidate doc(s).", len(order))
			for _, did := range order {
				collect(did)
			}
		}
	}

	// Fallback 2 — Go-only last resort: if even content recall found nothing
	// (e.g. no retrieval service wired), walk the root clusters so the tool
	// still returns a useful (if coarse) doc set. Python has no such tier — its
	// walk ends at content recall — and this one applies no relevance signal
	// beyond cluster order, so it must stay BEHIND the recall tier. The scope
	// still applies — collect() filters these leaves.
	if len(docs) == 0 {
		for _, datasetID := range datasetIDs {
			clusters, _, err := ns.ListClusters(ctx, tenantID, datasetID, 0, 100)
			if err != nil {
				continue
			}
			for _, c := range clusters {
				children, _, err := ns.ListChildren(ctx, tenantID, datasetID, c.Name, 0, 100)
				if err != nil {
					continue
				}
				for _, ch := range children {
					collect(ch.DocID)
					if len(docs) >= maxDocs {
						break
					}
				}
				if len(docs) >= maxDocs {
					break
				}
			}
			if len(docs) >= maxDocs {
				break
			}
		}
	}

	if len(docs) == 0 {
		return datasetNavigationJSON(datasetNavigationResult{NotFound: true}), nil
	}
	return datasetNavigationJSON(datasetNavigationResult{Docs: docs}), nil
}

func (d *DatasetNavigationByTree) mergeDefaults(args datasetNavigationArgs) datasetNavigationArgs {
	// Blank request values count as NOT SUPPLIED, and they must be compacted
	// before the default is considered: a request like {"doc_scope":[" "]} has a
	// non-zero length, so it used to suppress d.defaults.DocScope and then
	// compact away at the use site — leaving an empty scope, which inScope() and
	// ns.Search read as "unscoped". The configured restriction was therefore
	// silently disabled by a value that carries no document id, letting the tool
	// return documents the default scope excludes. Python's
	// RAGTools.scoped_doc_ids treats a falsy request scope the same way this now
	// does: fall back to the configured scope (agentic_rag.py:352-358).
	args.DocScope = compactStrings(args.DocScope)
	if len(args.DatasetIDs) == 0 && len(d.defaults.DatasetIDs) != 0 {
		args.DatasetIDs = append([]string(nil), d.defaults.DatasetIDs...)
	}
	if len(args.DocScope) == 0 && len(d.defaults.DocScope) != 0 {
		args.DocScope = compactStrings(d.defaults.DocScope)
	}
	if args.MaxDocs <= 0 {
		args.MaxDocs = d.defaults.MaxDocs
	}
	return args
}

func datasetNavigationJSON(r datasetNavigationResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"dataset_navigation: marshal result: %s"}`, err)
	}
	return string(b)
}
