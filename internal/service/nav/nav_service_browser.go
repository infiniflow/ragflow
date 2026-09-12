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

// The production NavTreeBrowser, the Go mirror of the two dataset_api_service
// browse calls the Python LLM tree walk makes (navigation.py:490, :601):
//
//	list_nav_clusters(kb.id, kb.tenant_id, page=1, page_size=…)
//	list_nav_children(kb.id, kb.tenant_id, name, page=1, page_size=…)
//
// The capability is nav.NavService's ListClusters / ListChildren — the same
// ES-backed nav-row reads, one level at a time. It lives in the nav service
// package (not the agentic harness) because the Python source of this logic is
// dataset_api_service.py, not harness/. The harness consumes it through the
// agentic NavTreeBrowser interface (structural — no import).
package nav

import "context"

// browserPageSize is Python _NAV_CHILDREN_PAGE_SIZE (navigation.py:383): the
// per-node children page the walk fetches. The harness passes its own value;
// this only covers a zero-argument call.
const browserPageSize = 1000

// NavServiceBrowser adapts NavService to the agentic NavTreeBrowser contract.
type NavServiceBrowser struct{}

// navNodeItem shapes one NavNode into Python's _nav_item dict
// (dataset_api_service.py:3416-3437): name / description / keywords / entities
// / doc_count / type ("cluster" | "doc") / doc_id / has_children.
//
// NavNode does not carry the row payload's keywords / entities (the Go nav
// model never reads them), so those render as empty lists — the LLM selector
// simply omits the [tags: …] / [entities: …] heads.
func navNodeItem(n NavNode) map[string]any {
	isCluster := n.Type == TypeNavCluster || n.Type == "cluster"
	docID := ""
	if !isCluster {
		docID = n.DocID
	}
	docCount := 1
	if isCluster {
		docCount = n.DocCount
	}
	t := "doc"
	if isCluster {
		t = "cluster"
	}
	return map[string]any{
		"name":         n.Name,
		"description":  n.Description,
		"keywords":     []any{},
		"entities":     []any{},
		"doc_count":    docCount,
		"type":         t,
		"doc_id":       docID,
		"has_children": isCluster,
	}
}

// ListNavClusters implements the agentic NavTreeBrowser contract (structural).
// A nil NavService yields (nil, nil) — "no tree rows", which the walk treats as
// its no-cluster content-recall fallback, mirroring the unconfigured case of
// NavServiceRouter.Route.
func (NavServiceBrowser) ListNavClusters(ctx context.Context, tenantID, kbID string, pageSize int) ([]map[string]any, error) {
	ns := GetNavService()
	if ns == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = browserPageSize
	}
	nodes, _, err := ns.ListClusters(ctx, tenantID, kbID, 1, pageSize)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, navNodeItem(n))
	}
	return out, nil
}

// ListNavChildren implements the agentic NavTreeBrowser contract (structural).
func (NavServiceBrowser) ListNavChildren(ctx context.Context, tenantID, kbID, name string, pageSize int) ([]map[string]any, error) {
	ns := GetNavService()
	if ns == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = browserPageSize
	}
	nodes, _, err := ns.ListChildren(ctx, tenantID, kbID, name, 1, pageSize)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, navNodeItem(n))
	}
	return out, nil
}

// NewNavServiceBrowser returns the production browser.
func NewNavServiceBrowser() NavServiceBrowser { return NavServiceBrowser{} }
