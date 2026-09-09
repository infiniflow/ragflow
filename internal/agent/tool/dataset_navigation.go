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
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/service/nav"
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
	DocScope   string   `json:"doc_scope,omitempty"`
	MaxDocs    int      `json:"max_docs,omitempty"`
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
	seen := map[string]struct{}{}
	var docs []string
	collect := func(id string) {
		if id == "" {
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

	// Primary: semantic search over each dataset's nav tree.
	for _, datasetID := range datasetIDs {
		hits, err := ns.Search(ctx, tenantID, datasetID, query, nil, maxDocs)
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

	// Fallback: if semantic routing found nothing (e.g. no embedder), walk the
	// root clusters so the tool still returns a useful (if coarse) doc set.
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
	if len(args.DatasetIDs) == 0 && len(d.defaults.DatasetIDs) != 0 {
		args.DatasetIDs = append([]string(nil), d.defaults.DatasetIDs...)
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
