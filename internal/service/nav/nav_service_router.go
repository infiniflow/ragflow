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

// The production nav-tree router, the Go mirror of Python
// search_dataset_layers(kb.id, tenant_id, query, "navigation_tree", top_k,
// doc_scope) (navigation.py:705). The capability is nav.NavService.Search — KNN
// over the dataset's compiled nav rows, the same thing the "navigation_tree"
// mode exposes.
//
// It lives in the nav service package (not the agentic harness) because the
// Python source of this logic is dataset_api_service.py, not harness/. The
// harness tool layer only consumes it through the agentic NavTreeRouter
// interface.
package nav

import (
	"context"
	"log"
	"strings"
)

// navServiceMaxDocs is the router-local fallback doc count when the caller
// passes topK <= 0. The harness NavigateTree passes its own navSearchMaxDocs,
// so this only triggers if a router is invoked without an explicit budget.
const navServiceMaxDocs = 12

// NavServiceRouter routes through the production navigation-tree service.
type NavServiceRouter struct{}

// Route implements the agentic NavTreeRouter contract (defined in the harness
// package, structural).
//
// Returns (nil, nil) when no NavService is installed — treated as "no compiled
// tree" (dataset-level), which is what an unconfigured deployment is.
//
// Two contract notes that drive the error handling below:
//
//  1. "no compiled tree" and "routed to nothing" are DIFFERENT signals. The
//     former is dataset-level (the caller may disable the tool for the session);
//     the latter is query-level (structure exists, this query just missed). The
//     Python side distinguishes them via empty_reason, and conflating them is
//     how a single unrouted query used to disable the tool for a whole session.
//  2. NavHit carries Name but not the leaf's Description, so the routed summary
//     is Name when a richer field is unavailable. Routing still labels the
//     route for free — no document load.
func (NavServiceRouter) Route(ctx context.Context, tenantID, kbID, query string, docScope []string, topK int) ([][2]string, error) {
	svc := GetNavService()
	if svc == nil {
		return nil, nil // no compiled tree available
	}
	if strings.TrimSpace(query) == "" || kbID == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = navServiceMaxDocs
	}

	hits, err := svc.Search(ctx, tenantID, kbID, query, nil, docScope, topK)
	if err != nil {
		// A backend failure is NOT "no structure": log and let the caller fall
		// back rather than disabling the tool for the session.
		log.Printf("[Dataset navigation search] NavService.Search failed for kb=%s: %v", kbID, err)
		return nil, err
	}
	// Empty non-nil slice = structure exists, this query routed to nothing.
	// (nil would mean "no tree", see above.)
	routed := make([][2]string, 0, len(hits))
	for _, h := range hits {
		// Only document leaves route: the strategy mirrored here pins
		// type_kwd="nav_doc" (dataset_api_service.py:4107-4119), so a nav_cluster
		// row must not become a routed "document" — a cluster's doc_id is the
		// kb_id, not a document.
		if h.Type == TypeNavCluster {
			continue
		}
		did := strings.TrimSpace(h.DocID)
		if did == "" {
			continue
		}
		routed = append(routed, [2]string{did, strings.TrimSpace(h.Name)})
	}
	return routed, nil
}

// NewNavServiceRouter returns the production router.
func NewNavServiceRouter() *NavServiceRouter { return &NavServiceRouter{} }
