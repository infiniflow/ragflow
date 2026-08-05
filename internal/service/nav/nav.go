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

// Package nav defines the dataset-navigation service interface. It is kept as a
// dependency-light leaf package so that both the agent tool layer
// (internal/agent/tool) and the concrete implementation
// (internal/service/nlp) can depend on it without creating an import cycle —
// mirroring how RetrievalService lives in the agent tool package. See
// tasks/agentic_search_port_plan.md.
package nav

import (
	"context"
	"sync"
)

// NavNode mirrors Python's _nav_item (dataset_api_service.py).
type NavNode struct {
	Name        string // name
	Description string // content_with_weight payload description
	DocCount    int    // doc_count_int (cluster) or 1 (leaf)
	Type        string // "cluster" | "doc"
	DocID       string // leaf doc_id; empty for cluster
	HasChildren bool   // is_cluster
}

// NavHit is one KNN hit on a nav row.
type NavHit struct {
	Type   string // "nav_doc" | "nav_cluster"
	DocID  string
	DocIDs []string
	Name   string
	Score  float64
}

// UpsertDocInput carries the per-document summary to place into the nav tree.
type UpsertDocInput struct {
	TenantID string
	KbID     string
	DocID    string
	Summary  string    // document summary text (tree product or page_index summary)
	Embedd   []float32 // optional precomputed embedding
}

// NavService is the single read/write entrypoint for a dataset's navigation
// tree. It is the only nav consumer used by agent tools, REST handlers and the
// tree/structure compile-complete hooks.
type NavService interface {
	// UpsertDoc places one document summary into the nav tree (incremental,
	// ES-backed read-modify-write; deterministic placement in the minimal loop).
	UpsertDoc(ctx context.Context, in UpsertDocInput) error
	// RemoveDoc removes a document's nav rows for the given doc. The minimal-loop
	// implementation deletes the nav_doc row(s) for the doc; empty-cluster
	// cascade cleanup is NOT yet implemented.
	RemoveDoc(ctx context.Context, tenantID, kbID, docID string) error

	// Search runs query KNN over nav rows and returns the routed doc ids.
	Search(ctx context.Context, tenantID, kbID, query string, embd []float32, topK int) ([]NavHit, error)
	// ListClusters returns the depth-0 clusters (parent_kwd=root).
	ListClusters(ctx context.Context, tenantID, kbID string, page, pageSize int) ([]NavNode, int64, error)
	// ListChildren returns the direct children of a cluster (parent_kwd=name).
	ListChildren(ctx context.Context, tenantID, kbID, name string, page, pageSize int) ([]NavNode, int64, error)
}

var (
	svcMu   sync.RWMutex
	svcInst NavService
)

// SetNavService installs the (production or test) NavService singleton.
func SetNavService(s NavService) {
	svcMu.Lock()
	defer svcMu.Unlock()
	svcInst = s
}

// GetNavService returns the installed NavService. It may return nil until
// SetNavService is called during server bootstrap.
func GetNavService() NavService {
	svcMu.RLock()
	defer svcMu.RUnlock()
	return svcInst
}
