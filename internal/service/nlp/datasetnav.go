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

package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service/nav"
)

// Dataset-nav constants. These mirror Python's dataset_nav.py: nav rows live in
// the document index as compile_kwd="dataset_nav" rows with available_int=0
// (invisible to the default retriever, which filters available_int=1), and the
// tree is threaded through parent_kwd ("root" for depth-0 clusters).
const (
	navCompileKwd = "dataset_nav"
	navRootParent = "root"

	navMergeThreshold = 0.80 // sim >= this -> merge doc into cluster
	navRecurse        = 0.65 // descend while sim >= this
	navMinSim         = 0.50 // sim >= this -> new sibling cluster
	navMaxDepth       = 6    // max descent levels during best-cluster search

	// Split thresholds (mirror Python dataset_nav._MAX_FANOUT / _MAX_DOCS_PER_CLUSTER).
	// Shared by maybeSplitCluster and the append-path guard so an append only pays
	// the children-search cost when the cluster is plausibly overfull.
	navMaxFanout         = 64 // max direct children before rebalance
	navMaxDocsPerCluster = 50 // max docs per leaf cluster before split
)

// NavEmbedder embeds text. tenantID lets a production implementation resolve
// the tenant's embedding model. Kept as an interface so tests inject a stub.
type NavEmbedder interface {
	Encode(ctx context.Context, tenantID string, texts []string) ([][]float32, error)
}

// NavService is the concrete, ES-backed implementation of nav.NavService.
type NavService struct {
	embed  NavEmbedder
	engine engine.DocEngine // optional; falls back to engine.Get() when nil
	llm    nav.NavMergeLLM  // optional; nil disables LLM merge/summary
}

// NewNavService builds the ES-backed NavService. embed may be nil when the
// caller guarantees every UpsertDocInput carries a precomputed Embedd.
func NewNavService(embed NavEmbedder) *NavService {
	return &NavService{embed: embed}
}

// SetNavMergeLLM installs the optional LLM used for cluster-description merging
// and summary naming (mirroring Python dataset_nav._llm_merge/_llm_create_summary).
// When nil (the default), NavService falls back to deterministic naming and
// concatenated descriptions.
func (s *NavService) SetNavMergeLLM(llm nav.NavMergeLLM) {
	s.llm = llm
}

func (s *NavService) docEngine() (engine.DocEngine, error) {
	if s.engine != nil {
		return s.engine, nil
	}
	de := engine.Get()
	if de == nil {
		return nil, fmt.Errorf("document engine is not initialized")
	}
	return de, nil
}

// navIndexName returns the tenant document index name (ragflow_<tenantID>).
func (s *NavService) navIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_%s", tenantID)
}

// navFilter builds the common nav filter that pins compile_kwd.
func navFilter(extra map[string]interface{}) map[string]interface{} {
	f := map[string]interface{}{"compile_kwd": []string{navCompileKwd}}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

// navSearch runs a filtered read over the tenant index for the dataset.
func (s *NavService) navSearch(ctx context.Context, tenantID, kbID string, filter map[string]interface{}, selectFields []string, offset, limit int, matchExprs []interface{}) ([]map[string]interface{}, int64, error) {
	de, err := s.docEngine()
	if err != nil {
		return nil, 0, err
	}
	merged := make(map[string]interface{}, len(filter)+1)
	for k, v := range filter {
		merged[k] = v
	}
	merged["kb_id"] = []string{kbID}
	req := &types.SearchRequest{
		IndexNames:   []string{s.navIndexName(tenantID)},
		KbIDs:        []string{kbID},
		Offset:       offset,
		Limit:        limit,
		SelectFields: selectFields,
		Filter:       merged,
		MatchExprs:   matchExprs,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	if res == nil {
		return nil, 0, nil
	}
	return res.Chunks, res.Total, nil
}

// ListClusters returns the depth-0 clusters (parent_kwd=root).
func (s *NavService) ListClusters(ctx context.Context, tenantID, kbID string, page, pageSize int) ([]nav.NavNode, int64, error) {
	if pageSize <= 0 {
		pageSize = 100
	}
	offset := page * pageSize
	chunks, total, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{
			"type_kwd":   []string{"nav_cluster"},
			"parent_kwd": []string{navRootParent},
		}),
		[]string{"title_kwd", "content_with_weight", "doc_count_int", "type_kwd"}, offset, pageSize, nil)
	if err != nil {
		return nil, 0, err
	}
	nodes := make([]nav.NavNode, 0, len(chunks))
	for _, c := range chunks {
		nodes = append(nodes, s.nodeFromRow(c, "cluster"))
	}
	return nodes, total, nil
}

// ListChildren returns the direct children of a cluster (parent_kwd=name).
func (s *NavService) ListChildren(ctx context.Context, tenantID, kbID, name string, page, pageSize int) ([]nav.NavNode, int64, error) {
	if pageSize <= 0 {
		pageSize = 100
	}
	offset := page * pageSize
	chunks, total, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"parent_kwd": []string{name}}),
		[]string{"title_kwd", "content_with_weight", "doc_count_int", "type_kwd", "doc_id"}, offset, pageSize, nil)
	if err != nil {
		return nil, 0, err
	}
	nodes := make([]nav.NavNode, 0, len(chunks))
	for _, c := range chunks {
		typ := firstStringValue(c["type_kwd"])
		nodeType := "doc"
		if typ == "nav_cluster" {
			nodeType = "cluster"
		}
		nodes = append(nodes, s.nodeFromRow(c, nodeType))
	}
	return nodes, total, nil
}

// nodeFromRow converts an engine row into a NavNode.
func (s *NavService) nodeFromRow(row map[string]interface{}, fallbackType string) nav.NavNode {
	name := firstStringValue(row["title_kwd"])
	// Prefer an explicit readable "name" column (Python rows carry one); fall
	// back to title_kwd.
	if n := firstStringValue(row["name"]); n != "" {
		name = n
	}
	// A raw id (doc_id or "cluster_<hash>") is not a human-readable name; fall
	// back to a title derived from the payload description. Cluster names are the
	// child-lookup key (parent_kwd references them verbatim), so they must stay
	// intact; only non-cluster (leaf) rows get the readable fallback, otherwise
	// GET /navigation/{cluster}/children would no longer match (review Major).
	isCluster := firstStringValue(row["type_kwd"]) == "nav_cluster"
	if !isCluster && graphIsRawID(name) {
		name = ""
	}
	node := nav.NavNode{
		Name:     name,
		DocCount: intValue(row["doc_count_int"]),
		Type:     fallbackType,
		DocID:    firstStringValue(row["doc_id"]),
	}
	if t := firstStringValue(row["type_kwd"]); t != "" {
		if t == "nav_cluster" {
			node.Type = "cluster"
		} else {
			node.Type = "doc"
		}
	}
	if payload, ok := row["content_with_weight"].(string); ok {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &m); err == nil {
			if d, ok := m["description"].(string); ok {
				node.Description = d
			}
			if node.Name == "" {
				if t, ok := m["title"].(string); ok && strings.TrimSpace(t) != "" {
					node.Name = cleanTitle(t)
				} else if d, ok := m["description"].(string); ok && strings.TrimSpace(d) != "" {
					node.Name = cleanTitle(fallbackTitle(d))
				}
			}
		}
	}
	node.HasChildren = node.Type == "cluster"
	if node.DocCount <= 0 {
		node.DocCount = 1
	}
	return node
}

// graphIsRawID reports whether a name is a meaningless internal id (a doc id or
// a "cluster_<hex>" key) rather than a human-readable title.
func graphIsRawID(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, "cluster_") && len(name) == len("cluster_")+8 {
		return true
	}
	if len(name) == 32 && isHexString(name) {
		return true
	}
	return false
}

func isHexString(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// Search runs query KNN over nav rows and returns routed doc ids.
func (s *NavService) Search(ctx context.Context, tenantID, kbID, query string, embd []float32, topK int) ([]nav.NavHit, error) {
	if topK <= 0 {
		topK = 8
	}
	vec := embd
	if len(vec) == 0 {
		if s.embed == nil {
			return nil, fmt.Errorf("datasetnav: no embedding available for Search")
		}
		embeddings, err := s.embed.Encode(ctx, tenantID, []string{query})
		if err != nil {
			return nil, err
		}
		if len(embeddings) == 0 {
			return nil, fmt.Errorf("datasetnav: embedding produced no vector")
		}
		vec = embeddings[0]
	}
	f64 := f32ToF64Slice(vec)
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(nil),
		[]string{"type_kwd", "title_kwd", "doc_id", "doc_ids_kwd", "_score"}, 0, topK,
		[]interface{}{&types.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(f64)),
			EmbeddingData:     f64,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              topK,
			ExtraOptions:      map[string]interface{}{"similarity": 0.0},
		}})
	if err != nil {
		return nil, err
	}
	hits := make([]nav.NavHit, 0, len(chunks))
	for _, c := range chunks {
		h := nav.NavHit{
			Type:  firstStringValue(c["type_kwd"]),
			Name:  firstStringValue(c["title_kwd"]),
			DocID: firstStringValue(c["doc_id"]),
		}
		if sc, ok := c["_score"].(float64); ok {
			h.Score = sc
		} else if sc, ok := c["_score"].(float32); ok {
			h.Score = float64(sc)
		}
		if ds, ok := c["doc_ids_kwd"].([]interface{}); ok {
			for _, d := range ds {
				if dd, ok := d.(string); ok {
					h.DocIDs = append(h.DocIDs, dd)
				}
			}
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// UpsertDoc places one document summary into the nav tree. Minimal closed loop:
// deterministic placement (KNN find best cluster -> merge if sim>=0.80, else a
// new root-level cluster). No LLM, no split/rebalance, no cascade cleanup.
func (s *NavService) UpsertDoc(ctx context.Context, in nav.UpsertDocInput) error {
	if strings.TrimSpace(in.Summary) == "" {
		return nil
	}
	de, err := s.docEngine()
	if err != nil {
		return err
	}
	if s.embed == nil && len(in.Embedd) == 0 {
		return fmt.Errorf("datasetnav: embedder required for UpsertDoc")
	}
	vec := in.Embedd
	if len(vec) == 0 {
		embeddings, err := s.embed.Encode(ctx, in.TenantID, []string{in.Summary})
		if err != nil {
			return err
		}
		if len(embeddings) == 0 {
			return nil
		}
		vec = embeddings[0]
	}

	// storeGet: skip if a nav_doc for this doc already exists with same summary.
	existing, _, err := s.navSearch(ctx, in.TenantID, in.KbID,
		navFilter(map[string]interface{}{"doc_id": []string{in.DocID}}),
		[]string{"content_with_weight"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		if payload, ok := existing[0]["content_with_weight"].(string); ok {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &m); err == nil {
				if d, _ := m["description"].(string); d == in.Summary {
					return nil // unchanged
				}
			}
		}
		// Changed summary: remove the old nav_doc first (and prune any emptied
		// cluster). The doc may have lived in a cluster; the cluster is then
		// decremented/cascaded by RemoveDoc's cleanup logic.
		parent, err := s.deleteNavDoc(ctx, in.TenantID, in.KbID, in.DocID)
		if err != nil {
			return err
		}
		if parent != "" && parent != navRootParent {
			if err := s.removeDocFromCluster(ctx, in.TenantID, in.KbID, parent, in.DocID); err != nil {
				return err
			}
		}
		if err := s.cleanupEmptyCluster(ctx, in.TenantID, in.KbID, parent); err != nil {
			return err
		}
	}

	bestName, sim, bestDepth, err := s.findBestCluster(ctx, in.TenantID, in.KbID, vec)
	if err != nil {
		return err
	}
	idx := s.navIndexName(in.TenantID)

	if bestName != "" && sim >= navMergeThreshold {
		parent := bestName
		if err := s.appendDocToCluster(ctx, de, in.TenantID, in.KbID, bestName, in.DocID,
			fmt.Sprintf("q_%d_vec", len(vec))); err != nil {
			return err
		}
		// Explicit stable id (A5): the nav_doc row is addressable by a
		// deterministic id (hash of the "dataset_nav:doc:{doc_id}" key) so
		// RemoveDoc / cascade cleanup can locate and delete it precisely,
		// instead of relying on a doc_id + type filter alone.
		_, err = de.InsertChunks(ctx, []map[string]interface{}{{
			"id":            navDocID(in.TenantID, in.KbID, in.DocID),
			"compile_kwd":   navCompileKwd,
			"available_int": 0,
			"type_kwd":      "nav_doc",
			// The nav_doc's display name is a readable title derived from the summary
			// (Python _clean_title/_fallback_title) — NOT the raw doc id, which is
			// meaningless in the UI. The doc_id is still stored for lookups.
			"title_kwd":  cleanTitle(fallbackTitle(in.Summary)),
			"parent_kwd": parent,
			// The nav_doc sits one level below its (possibly nested) parent
			// cluster, so its depth is parentDepth+1 — not a hard-coded 1.
			"depth_int":           bestDepth + 1,
			"doc_id":              in.DocID,
			"doc_count_int":       1,
			"content_with_weight": payloadJSONNav(map[string]interface{}{"type": "nav_doc", "description": in.Summary}),
			"q_" + fmt.Sprintf("%d", len(vec)) + "_vec": f32ToF64Slice(vec),
		}}, idx, in.KbID)
		return err
	}

	// A similar-but-not-mergeable cluster creates a sibling sub-cluster (Python
	// _MIN_SIM=0.50); otherwise a fresh root cluster. This keeps the nav tree
	// from degrading into one root per document.
	parent := navRootParent
	depth := 0
	if bestName != "" && sim >= navMinSim {
		parent = bestName
		// A sibling of the (possibly nested) best cluster is one level deeper
		// than it, so depth = parentDepth+1 — not a hard-coded 1.
		depth = bestDepth + 1
	}
	// New cluster: when an LLM is configured, generate a short name + summary
	// (Python _llm_create_summary); otherwise fall back to a deterministic name
	// (A1, graceful without LLM).
	name, summary := s.llmCreateSummary(ctx, in.TenantID, in.Summary)
	if summary == "" {
		summary = in.Summary
	}
	_, err = de.InsertChunks(ctx, []map[string]interface{}{{
		"id":                  navClusterID(in.TenantID, in.KbID, name),
		"doc_id":              in.KbID, // cluster rows carry the kb as doc_id (Python _build_nav_cluster_row), so ES InsertChunks does not skip them
		"compile_kwd":         navCompileKwd,
		"available_int":       0,
		"type_kwd":            "nav_cluster",
		"title_kwd":           name,
		"parent_kwd":          parent,
		"depth_int":           depth,
		"doc_count_int":       1,
		"doc_ids_kwd":         []string{in.DocID},
		"content_with_weight": payloadJSONNav(map[string]interface{}{"type": "nav_cluster", "description": summary}),
		"q_" + fmt.Sprintf("%d", len(vec)) + "_vec": f32ToF64Slice(vec),
	}}, idx, in.KbID)
	if err != nil {
		return err
	}
	// Always emit a nav_doc leaf under the (new) cluster, mirroring Python
	// upsert_dataset_nav_doc (the merge branch does the same at its parent). A
	// new root cluster that only folds the doc into doc_ids_kwd would leave the
	// nav tree with a single cluster and no nav_doc child, so /children returns
	// empty. The nav_doc carries a readable title + parent_kwd = the cluster name.
	_, err = de.InsertChunks(ctx, []map[string]interface{}{{
		"id":                  navDocID(in.TenantID, in.KbID, in.DocID),
		"compile_kwd":         navCompileKwd,
		"available_int":       0,
		"type_kwd":            "nav_doc",
		"title_kwd":           cleanTitle(fallbackTitle(in.Summary)),
		"parent_kwd":          name,
		"depth_int":           depth + 1,
		"doc_id":              in.DocID,
		"doc_count_int":       1,
		"content_with_weight": payloadJSONNav(map[string]interface{}{"type": "nav_doc", "description": in.Summary}),
		"q_" + fmt.Sprintf("%d", len(vec)) + "_vec": f32ToF64Slice(vec),
	}}, idx, in.KbID)
	return err
}

// navDocID returns the explicit stable id for a nav_doc row (A5), a hash of the
// canonical "dataset_nav:doc:{doc_id}" key. It is deterministic so deletes and
// cascade cleanup can address the row without a doc_id+type filter scan.
func navDocID(tenantID, kbID, docID string) string {
	return "dataset_nav_doc_" + contentHash8(tenantID+"\x00"+kbID+"\x00"+docID)
}

// navClusterID returns the explicit stable id for a nav_cluster row (A5), a hash
// of the canonical "dataset_nav:cluster:{kb}:{name}" key (mirroring Python
// _nav_cluster_id).
func navClusterID(tenantID, kbID, name string) string {
	return "dataset_nav_cluster_" + contentHash8(tenantID+"\x00"+kbID+"\x00"+name)
}

// cleanupEmptyCluster deletes a nav_cluster that holds no docs and no child
// clusters, then recurses to its parent (cascade, mirroring Python
// _cleanup_empty_cluster). It is called by RemoveDoc after deleting a nav_doc so
// an emptied cluster does not linger.
func (s *NavService) cleanupEmptyCluster(ctx context.Context, tenantID, kbID, clusterName string) error {
	if clusterName == "" || clusterName == navRootParent {
		return nil
	}
	de, err := s.docEngine()
	if err != nil {
		return err
	}
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_cluster"}, "title_kwd": []string{clusterName}}),
		[]string{"id", "parent_kwd", "doc_count_int", "doc_ids_kwd"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	// Children: count any nav_cluster/nav_doc whose parent is this cluster.
	children, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"parent_kwd": []string{clusterName}}),
		[]string{"id"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return nil // not empty: has children
	}
	parent := firstStringValue(chunks[0]["parent_kwd"])
	if intValue(chunks[0]["doc_count_int"]) > 0 {
		return nil // still has docs
	}
	// Delete this cluster by the row id returned by the search (robust whether
	// the engine preserves our explicit stable id or assigns its own).
	rowID := firstStringValue(chunks[0]["id"])
	if rowID == "" {
		rowID = navClusterID(tenantID, kbID, clusterName)
	}
	_, err = de.DeleteChunks(ctx,
		map[string]interface{}{"id": []string{rowID}, "kb_id": kbID},
		s.navIndexName(tenantID), kbID)
	if err != nil {
		return err
	}
	// Recurse upward.
	return s.cleanupEmptyCluster(ctx, tenantID, kbID, parent)
}

// findBestCluster finds the best-matching cluster via level-by-level descent
// (mirroring Python _find_best_cluster). It KNNs the current level's clusters
// and, when the best match is >= recurse threshold, descends into that cluster's
// children. Returns the best cluster name, similarity, and its depth (0 = root)
// so callers can assign consistent child depth_int values.
func (s *NavService) findBestCluster(ctx context.Context, tenantID, kbID string, vec []float32) (string, float64, int, error) {
	f64 := f32ToF64Slice(vec)
	parent := navRootParent
	bestName := ""
	bestSim := 0.0
	bestDepth := 0
	for level := 0; level < navMaxDepth; level++ {
		// KNN among clusters whose parent is the current level.
		chunks, _, err := s.navSearch(ctx, tenantID, kbID,
			navFilter(map[string]interface{}{
				"type_kwd":   []string{"nav_cluster"},
				"parent_kwd": []string{parent},
			}),
			[]string{"title_kwd", "_score"}, 0, 1,
			[]interface{}{&types.MatchDenseExpr{
				VectorColumnName:  fmt.Sprintf("q_%d_vec", len(f64)),
				EmbeddingData:     f64,
				EmbeddingDataType: "float",
				DistanceType:      "cosine",
				TopN:              1,
				ExtraOptions:      map[string]interface{}{"similarity": 0.0},
			}})
		if err != nil {
			return "", 0, 0, err
		}
		if len(chunks) == 0 {
			break
		}
		name := firstStringValue(chunks[0]["title_kwd"])
		sim := rowScore(chunks[0])
		// Keep the STRONGEST match seen so far across all levels, so a strong
		// ancestor is never displaced by a weaker descendant. Record its depth
		// so the caller can set consistent child depth_int values.
		if sim > bestSim {
			bestName, bestSim, bestDepth = name, sim, level
		}
		// Descend only while the current match is strong enough that a deeper
		// child could be a better target.
		if sim < navRecurse {
			break
		}
		parent = name
	}
	return bestName, bestSim, bestDepth, nil
}

// rowScore extracts the engine's _score field as float64.
func rowScore(row map[string]interface{}) float64 {
	if sc, ok := row["_score"].(float64); ok {
		return sc
	}
	if sc, ok := row["_score"].(float32); ok {
		return float64(sc)
	}
	return 0
}

// maybeSplitCluster rebalances an overfull cluster into two sibling clusters,
// mirroring Python dataset_nav._maybe_split_cluster. A cluster is split when it
// has too many direct children (> _MAX_FANOUT=64) or too many nav_doc children
// (> _MAX_DOCS_PER_CLUSTER=50). The minimal loop uses a deterministic 2-way
// partition by child index parity (a stand-in for k-means) to keep behavior
// reproducible without an embedder; the LLM-enhanced implementation would pick
// the two best seed vectors instead.
func (s *NavService) maybeSplitCluster(ctx context.Context, tenantID, kbID, clusterName, vecCol string) error {
	de, err := s.docEngine()
	if err != nil {
		return err
	}
	// Count direct children (nav_cluster + nav_doc) of this cluster. vecCol (the
	// engine column q_<dim>_vec, e.g. from the doc being appended) is selected so
	// the split siblings can inherit a representative vector for KNN routing; an
	// empty vecCol skips it (callers without a known dim).
	selectFields := []string{"id", "doc_id", "doc_count_int", "doc_ids_kwd", "title_kwd", "type_kwd"}
	if vecCol != "" {
		selectFields = append(selectFields, vecCol)
	}
	children, total, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"parent_kwd": []string{clusterName}}),
		selectFields, 0, 200, nil)
	if err != nil {
		return err
	}
	if len(children) < 2 {
		return nil
	}
	// Split signal comes purely from the direct children (mirroring Python
	// dataset_nav._maybe_split_cluster): too many total children (fanout) or too
	// many nav_doc children (a leaf that accumulated more docs than the cap). The
	// cluster's own row is only re-read below when a split is actually happening,
	// so the common (under-threshold) append path performs a single children
	// search instead of two (review Major).
	fanout := 0
	navDocKids := 0
	for _, c := range children {
		if firstStringValue(c["type_kwd"]) == "nav_doc" {
			navDocKids++
		}
		fanout++
	}
	if fanout <= navMaxFanout && navDocKids <= navMaxDocsPerCluster {
		return nil
	}
	// The split needs the original cluster's row id, parent, depth and directly
	// held docs; select id + doc_ids_kwd so the split can delete by the
	// engine-assigned row id and inherit the cluster's directly-held documents.
	clusterChunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_cluster"}, "title_kwd": []string{clusterName}}),
		[]string{"id", "doc_count_int", "doc_ids_kwd", "parent_kwd", "depth_int"}, 0, 1, nil)
	if err != nil {
		return err
	}

	// Split into two sibling clusters "A"/"B", reparenting children by parity of
	// their doc count so each half gets a comparable load.
	parent := navRootParent
	if len(clusterChunks) > 0 {
		parent = firstStringValue(clusterChunks[0]["parent_kwd"])
	}
	splitA := clusterName + ":A"
	splitB := clusterName + ":B"
	// Rehome each child under splitA/splitB by parity of child index, and
	// accumulate the aggregate (doc_count_int, doc_ids_kwd, vector) that each
	// split must inherit so the replacement clusters stay searchable by KNN and
	// can support deletion bookkeeping (review issue 5). nav_doc children carry
	// doc_ids_kwd of their own doc; nav_cluster children carry their subtree's.
	accA := struct {
		count int
		ids   []string
		vec   []float32
	}{}
	accB := struct {
		count int
		ids   []string
		vec   []float32
	}{}
	for i, child := range children {
		acc := &accA
		if i%2 == 1 {
			acc = &accB
		}
		title := firstStringValue(child["title_kwd"])
		typ := firstStringValue(child["type_kwd"])
		childID := firstStringValue(child["id"])
		if title == "" && childID == "" {
			continue
		}
		if typ == "" {
			// The engine returned a child without a type discriminator; skip it
			// so we never rehome with an empty-type filter that matches nothing
			// (leaving the child orphaned under a deleted cluster name).
			continue
		}
		// Aggregate this child's doc count + doc ids into the target split.
		if c := intValue(child["doc_count_int"]); c > 0 {
			acc.count += c
		}
		childDocIDs := firstStringSlice(child["doc_ids_kwd"])
		// nav_doc rows keep their sole member in doc_id; doc_ids_kwd belongs to
		// nav_cluster rows only. Preserve both shapes when rebuilding the split
		// clusters' membership indexes for RemoveDoc.
		if typ == "nav_doc" {
			childDocIDs = append(childDocIDs, firstStringValue(child["doc_id"]))
		}
		acc.ids = appendUnique(acc.ids, childDocIDs)
		// Rehome the child by its row id (precise) rather than an empty-safe
		// type+title filter, so the UpdateChunks filter always matches.
		upd := map[string]interface{}{"parent_kwd": accTarget(i, splitA, splitB)}
		if err := de.UpdateChunks(ctx,
			map[string]interface{}{
				"compile_kwd": []string{navCompileKwd},
				"type_kwd":    []string{typ},
				"title_kwd":   []string{title},
				"kb_id":       kbID,
			}, upd, s.navIndexName(tenantID), kbID); err != nil {
			return err
		}
	}
	// The original cluster may be tracked by a row id the engine assigned (not
	// our deterministic navClusterID); delete by the search-returned id, falling
	// back to the deterministic id, so the delete always matches (review Major).
	origRowID := ""
	if len(clusterChunks) > 0 {
		origRowID = firstStringValue(clusterChunks[0]["id"])
	}
	if origRowID == "" {
		origRowID = navClusterID(tenantID, kbID, clusterName)
	}
	// The original cluster may itself directly hold documents (the standalone
	// cluster case), which must be inherited by the split siblings.
	origIDs := []string{}
	if len(clusterChunks) > 0 {
		origIDs = firstStringSlice(clusterChunks[0]["doc_ids_kwd"])
	}
	// Delete the original cluster and insert the two split clusters carrying the
	// aggregated doc count, doc ids, and a representative vector so the split
	// clusters remain KNN-searchable.
	_, err = de.DeleteChunks(ctx,
		map[string]interface{}{"id": []string{origRowID}, "kb_id": kbID},
		s.navIndexName(tenantID), kbID)
	if err != nil {
		return err
	}
	for _, spl := range []struct {
		name  string
		count int
		ids   []string
	}{{splitA, accA.count, accA.ids}, {splitB, accB.count, accB.ids}} {
		// Distribute the original cluster's directly-held docs into each split by
		// parity so no tracked document is lost (review Major).
		for i, d := range origIDs {
			if i%2 == 0 && spl.name == splitA {
				spl.ids = appendUnique(spl.ids, []string{d})
				spl.count++
			} else if i%2 == 1 && spl.name == splitB {
				spl.ids = appendUnique(spl.ids, []string{d})
				spl.count++
			}
		}
		row := map[string]interface{}{
			"id":                  navClusterID(tenantID, kbID, spl.name),
			"compile_kwd":         navCompileKwd,
			"available_int":       0,
			"type_kwd":            "nav_cluster",
			"title_kwd":           spl.name,
			"parent_kwd":          parent,
			"depth_int":           clusterDepth(clusterChunks),
			"doc_count_int":       spl.count,
			"doc_ids_kwd":         spl.ids,
			"content_with_weight": payloadJSONNav(map[string]interface{}{"type": "nav_cluster", "description": "split of " + clusterName}),
		}
		// Representative vector: if any reparented child carried a vector, use it
		// so the split cluster participates in KNN routing. This is a heuristic
		// stand-in for Python's k-means centroid.
		if vec := pickAnyVector(children, i2boolForTarget(spl.name, splitA)); len(vec) > 0 {
			row["q_"+fmt.Sprintf("%d", len(vec))+"_vec"] = f32ToF64Slice(vec)
		}
		if _, err := de.InsertChunks(ctx, []map[string]interface{}{row}, s.navIndexName(tenantID), kbID); err != nil {
			return err
		}
	}
	_ = total
	return nil
}

// accTarget returns the split target name for a child index by parity.
func accTarget(i int, splitA, splitB string) string {
	if i%2 == 1 {
		return splitB
	}
	return splitA
}

// i2boolForTarget reports whether name equals splitA (used to pick which split's
// children to source a representative vector from).
func i2boolForTarget(name, splitA string) bool { return name == splitA }

// pickAnyVector returns the first non-empty vector among the children that map
// to a given parity bucket (splitA→even, splitB→odd).
func pickAnyVector(children []map[string]interface{}, wantA bool) []float32 {
	for i, c := range children {
		if (i%2 == 1) == wantA {
			continue
		}
		// The engine returns dense vectors as []interface{} of float64 (q_<n>_vec
		// is unboxed); walk any key that looks like a q_*_vec.
		for k, v := range c {
			if !strings.HasPrefix(k, "q_") || !strings.HasSuffix(k, "_vec") {
				continue
			}
			return toF32Vector(v)
		}
	}
	return nil
}

// toF32Vector converts an engine vector value ([]interface{} of float64) into
// []float32, returning nil on mismatch.
func toF32Vector(v interface{}) []float32 {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]float32, 0, len(arr))
	for _, x := range arr {
		f, ok := x.(float64)
		if !ok {
			return nil
		}
		out = append(out, float32(f))
	}
	return out
}

// clusterDepth returns the depth_int of the original cluster (for the split
// siblings), defaulting to 0.
func clusterDepth(clusterChunks []map[string]interface{}) int {
	if len(clusterChunks) == 0 {
		return 0
	}
	return intValue(clusterChunks[0]["depth_int"])
}

// llmMergeDescription merges a set of source descriptions into one, using the
// optional LLM when available (temperature 0.1, mirroring Python _llm_merge);
// without an LLM it concatenates them deterministically.
func (s *NavService) llmMergeDescription(ctx context.Context, tenantID string, texts []string) string {
	if s.llm != nil && len(texts) > 0 {
		if merged, err := s.llm.Merge(ctx, tenantID, texts); err == nil && strings.TrimSpace(merged) != "" {
			return merged
		}
	}
	return strings.Join(texts, "\n")
}

// cleanTitle normalizes a raw title/summary into a one-line, length-capped
// display name (mirroring Python dataset_nav._clean_title).
func cleanTitle(title string) string {
	return truncateString(strings.Join(strings.Fields(title), " "), 48)
}

// fallbackTitle derives a short readable title from a summary by taking the
// first non-empty line and stripping Markdown emphasis/heading markers. The
// tree-root summaries carry a one-line Markdown title ("**Title**" or
// "## Title") on the first line, so this yields a clean, URL-friendly label —
// far more readable than taking the first whitespace words of the body.
func fallbackTitle(summary string) string {
	line := summary
	if idx := strings.IndexAny(line, "\n\r"); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	// Strip Markdown emphasis and heading markers.
	line = strings.Trim(line, "*# \t")
	if line == "" {
		return "Cluster"
	}
	return line
}

// readableClusterName returns a readable yet unique nav-cluster key of the form
// "<title> <8-hex>" (mirroring Python dataset_nav._readable_cluster_name): the
// title keeps the node name human-readable, the short hash of the seed keeps the
// per-KB uniqueness that the tree keying relies on.
func readableClusterName(title, seed string) string {
	t := cleanTitle(title)
	if t == "" {
		t = "Cluster"
	}
	return t + " " + shortHash8(seed)
}

// llmCreateSummary builds a short cluster name + summary for a source text via
// the optional LLM (mirroring Python _llm_create_summary); without an LLM it
// falls back to a readable name derived from the summary's first words plus a
// short hash, so nav clusters are human-readable instead of "cluster_<hash>".
func (s *NavService) llmCreateSummary(ctx context.Context, tenantID, text string) (name, summary string) {
	if s.llm != nil {
		if n, sm, err := s.llm.CreateSummary(ctx, tenantID, text); err == nil && n != "" {
			return n, sm
		}
	}
	return readableClusterName(fallbackTitle(text), text), text
}

// appendDocToCluster appends a doc id to a cluster's doc_ids_kwd and bumps its
// doc_count_int. Implemented as a read-modify-write. vecCol names the engine
// vector column (q_<dim>_vec) so an overfull-cluster split can inherit a
// representative vector; pass "" when the caller has no known dimension.
func (s *NavService) appendDocToCluster(ctx context.Context, de engine.DocEngine, tenantID, kbID, clusterName, docID, vecCol string) error {
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_cluster"}, "title_kwd": []string{clusterName}}),
		[]string{"doc_ids_kwd", "doc_count_int"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	// Use firstStringSlice (not a []interface{} assertion): the engine may return
	// doc_ids_kwd as either []string or []interface{}, and a bare type assertion
	// would silently clear the field on a []string (review Critical).
	ids := make([]string, 0, 8)
	for _, d := range firstStringSlice(chunks[0]["doc_ids_kwd"]) {
		if d != docID {
			ids = append(ids, d)
		}
	}
	found := false
	for _, id := range ids {
		if id == docID {
			found = true
			break
		}
	}
	if !found {
		ids = append(ids, docID)
	}
	count := intValue(chunks[0]["doc_count_int"])
	if !found {
		count++
	}
	// Pin the update to the nav_cluster row only: a regular chunk sharing the
	// same title_kwd must never be clobbered. The read-modify-write here is
	// expected to run under a per-dataset lock held by the UpsertDoc caller;
	// without it, concurrent appends to the same cluster can lose updates.
	if err := de.UpdateChunks(ctx,
		map[string]interface{}{
			"compile_kwd": []string{navCompileKwd},
			"type_kwd":    []string{"nav_cluster"},
			"title_kwd":   []string{clusterName},
			"kb_id":       kbID,
		},
		map[string]interface{}{"doc_ids_kwd": ids, "doc_count_int": count},
		s.navIndexName(tenantID), kbID); err != nil {
		return err
	}
	// Rebalance only when the cluster is plausibly overfull: the cluster's own
	// doc count already exceeds the per-cluster cap, so a split is possible.
	// Skipping the children-search on every append avoids 2N extra engine
	// round-trips for a dataset compile (review Major). maybeSplitCluster still
	// re-checks both thresholds, so fanout-only splits via the direct call path
	// (tests, rebuild) are unaffected. A split failure aborts the batch.
	if count <= navMaxDocsPerCluster {
		return nil
	}
	return s.maybeSplitCluster(ctx, tenantID, kbID, clusterName, vecCol)
}

// deleteNavDoc deletes a nav_doc row by doc_id, returning the doc's parent
// cluster name (if any) so RemoveDoc can run cascade cleanup. It reads the
// parent_kwd of the nav_doc before deleting so the emptied cluster can be pruned.
func (s *NavService) deleteNavDoc(ctx context.Context, tenantID, kbID, docID string) (string, error) {
	de, err := s.docEngine()
	if err != nil {
		return "", err
	}
	// Restrict the delete to nav_doc rows only (review issue 8): a nav_cluster
	// row must never be matched by a doc_id filter.
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_doc"}, "doc_id": []string{docID}}),
		[]string{"id", "parent_kwd"}, 0, 100, nil)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(chunks))
	var parent string
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
		if parent == "" {
			parent = firstStringValue(c["parent_kwd"])
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	if _, err := de.DeleteChunks(ctx, map[string]interface{}{"id": ids, "kb_id": kbID}, s.navIndexName(tenantID), kbID); err != nil {
		return "", err
	}
	return parent, nil
}

// RemoveDoc removes a document's nav_doc (if any) and then cascades: the doc is
// removed from the cluster(s) that list it in doc_ids_kwd (decrementing
// doc_count_int), and any cluster left empty (no docs, no children) is pruned
// upward (A3, mirroring Python _cleanup_empty_cluster). A doc that created a
// standalone cluster (no separate nav_doc row) is handled by the
// doc-in-cluster lookup below.
func (s *NavService) RemoveDoc(ctx context.Context, tenantID, kbID, docID string) error {
	parent, err := s.deleteNavDoc(ctx, tenantID, kbID, docID)
	if err != nil {
		return err
	}
	// Remove the doc from its parent cluster's doc_ids_kwd and bump the count.
	if parent != "" && parent != navRootParent {
		if err := s.removeDocFromCluster(ctx, tenantID, kbID, parent, docID); err != nil {
			return err
		}
	} else {
		// No separate nav_doc: the doc is a member of a cluster's doc_ids_kwd
		// (it created a standalone cluster, or lives under a cluster but the
		// nav_doc row is absent). Locate and decrement that cluster.
		cluster, err := s.findClusterContainingDoc(ctx, tenantID, kbID, docID)
		if err != nil {
			return err
		}
		if cluster != "" {
			if err := s.removeDocFromCluster(ctx, tenantID, kbID, cluster, docID); err != nil {
				return err
			}
			parent = cluster
		}
	}
	// Cascade cleanup of the emptied cluster (and its ancestors).
	return s.cleanupEmptyCluster(ctx, tenantID, kbID, parent)
}

// findClusterContainingDoc returns the title_kwd of the first nav_cluster whose
// doc_ids_kwd contains docID, or "" when none does.
func (s *NavService) findClusterContainingDoc(ctx context.Context, tenantID, kbID, docID string) (string, error) {
	de, err := s.docEngine()
	if err != nil {
		return "", err
	}
	// Page through clusters (review issue 19) so a KB with more than one page of
	// clusters does not hide the doc's cluster past the first page.
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		res, err := de.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{s.navIndexName(tenantID)},
			KbIDs:        []string{kbID},
			SelectFields: []string{"title_kwd", "doc_ids_kwd"},
			Filter:       map[string]interface{}{"type_kwd": "nav_cluster", "compile_kwd": []string{navCompileKwd}},
			Offset:       offset,
			Limit:        pageSize,
		})
		if err != nil {
			return "", err
		}
		for _, row := range res.Chunks {
			for _, d := range firstStringSlice(row["doc_ids_kwd"]) {
				if d == docID {
					return firstStringValue(row["title_kwd"]), nil
				}
			}
		}
		if len(res.Chunks) < pageSize {
			return "", nil
		}
	}
}

// removeDocFromCluster removes a doc id from a cluster's doc_ids_kwd and
// decrements its doc_count_int (read-modify-write). It is the inverse of
// appendDocToCluster.
func (s *NavService) removeDocFromCluster(ctx context.Context, tenantID, kbID, clusterName, docID string) error {
	de, err := s.docEngine()
	if err != nil {
		return err
	}
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_cluster"}, "title_kwd": []string{clusterName}}),
		[]string{"doc_ids_kwd", "doc_count_int"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	// Use firstStringSlice (not a []interface{} assertion): the engine may return
	// doc_ids_kwd as either []string or []interface{}, and a bare type assertion
	// would silently clear the field on a []string (review Critical).
	ids := make([]string, 0, 8)
	for _, d := range firstStringSlice(chunks[0]["doc_ids_kwd"]) {
		if d != docID {
			ids = append(ids, d)
		}
	}
	count := intValue(chunks[0]["doc_count_int"])
	if count > 0 {
		count--
	}
	return de.UpdateChunks(ctx,
		map[string]interface{}{
			"compile_kwd": []string{navCompileKwd},
			"type_kwd":    []string{"nav_cluster"},
			"title_kwd":   []string{clusterName},
			"kb_id":       kbID,
		},
		map[string]interface{}{"doc_ids_kwd": ids, "doc_count_int": count},
		s.navIndexName(tenantID), kbID)
}

// f32ToF64Slice converts a float32 vector to float64.
func f32ToF64Slice(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

// payloadJSONNav marshals a nav payload map into the content_with_weight JSON.
func payloadJSONNav(v map[string]interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// truncateString caps s to n runes (not bytes).
func truncateString(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// shortHash8 is a stable 8-char hash suffix for readable nav names.
func shortHash8(s string) string {
	return contentHash8(s)
}

// contentHash8 is a stable 8-char hash.
func contentHash8(s string) string {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// firstStringValue returns the first string value of a (possibly list-wrapped)
// engine field.
func firstStringValue(v interface{}) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []string:
		if len(tv) > 0 {
			return tv[0]
		}
	case []interface{}:
		if len(tv) > 0 {
			if s, ok := tv[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// intValue returns the integer value of an engine field.
func firstStringSlice(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// appendUnique appends only values not already present, preserving order.
func appendUnique(dst, values []string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}

func intValue(v interface{}) int {
	switch tv := v.(type) {
	case float64:
		return int(tv)
	case float32:
		return int(tv)
	case int:
		return tv
	case int64:
		return int(tv)
	case []float64:
		if len(tv) > 0 {
			return int(tv[0])
		}
	case []interface{}:
		if len(tv) > 0 {
			switch n := tv[0].(type) {
			case float64:
				return int(n)
			case int:
				return n
			}
		}
	}
	return 0
}
