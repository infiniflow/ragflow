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
}

// NewNavService builds the ES-backed NavService. embed may be nil when the
// caller guarantees every UpsertDocInput carries a precomputed Embedd.
func NewNavService(embed NavEmbedder) *NavService {
	return &NavService{embed: embed}
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
	node := nav.NavNode{
		Name:     firstStringValue(row["title_kwd"]),
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
		}
	}
	node.HasChildren = node.Type == "cluster"
	if node.DocCount <= 0 {
		node.DocCount = 1
	}
	return node
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
		// Changed summary: remove the old nav_doc first (no cascade in minimal loop).
		if _, err := s.deleteNavDoc(ctx, in.TenantID, in.KbID, in.DocID); err != nil {
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
		if err := s.appendDocToCluster(ctx, de, in.TenantID, in.KbID, bestName, in.DocID); err != nil {
			return err
		}
		_, err = de.InsertChunks(ctx, []map[string]interface{}{{
			"compile_kwd":   navCompileKwd,
			"available_int": 0,
			"type_kwd":      "nav_doc",
			"title_kwd":     in.DocID,
			"parent_kwd":    parent,
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
	name := navDocName(in.DocID, in.Summary)
	_, err = de.InsertChunks(ctx, []map[string]interface{}{{
		"compile_kwd":         navCompileKwd,
		"available_int":       0,
		"type_kwd":            "nav_cluster",
		"title_kwd":           name,
		"parent_kwd":          parent,
		"depth_int":           depth,
		"doc_count_int":       1,
		"doc_ids_kwd":         []string{in.DocID},
		"content_with_weight": payloadJSONNav(map[string]interface{}{"type": "nav_cluster", "description": in.Summary}),
		"q_" + fmt.Sprintf("%d", len(vec)) + "_vec": f32ToF64Slice(vec),
	}}, idx, in.KbID)
	return err
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

// appendDocToCluster appends a doc id to a cluster's doc_ids_kwd and bumps its
// doc_count_int. Implemented as a read-modify-write.
func (s *NavService) appendDocToCluster(ctx context.Context, de engine.DocEngine, tenantID, kbID, clusterName, docID string) error {
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"type_kwd": []string{"nav_cluster"}, "title_kwd": []string{clusterName}}),
		[]string{"doc_ids_kwd", "doc_count_int"}, 0, 1, nil)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	ids := []string{}
	if raw, ok := chunks[0]["doc_ids_kwd"].([]interface{}); ok {
		for _, d := range raw {
			if dd, ok := d.(string); ok {
				ids = append(ids, dd)
			}
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

// deleteNavDoc deletes a nav_doc row by doc_id.
func (s *NavService) deleteNavDoc(ctx context.Context, tenantID, kbID, docID string) (int64, error) {
	de, err := s.docEngine()
	if err != nil {
		return 0, err
	}
	chunks, _, err := s.navSearch(ctx, tenantID, kbID,
		navFilter(map[string]interface{}{"doc_id": []string{docID}}),
		[]string{"id"}, 0, 100, nil)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return de.DeleteChunks(ctx, map[string]interface{}{"id": ids, "kb_id": kbID}, s.navIndexName(tenantID), kbID)
}

// RemoveDoc removes a document's nav_doc (no cascade cleanup in minimal loop).
func (s *NavService) RemoveDoc(ctx context.Context, tenantID, kbID, docID string) error {
	_, err := s.deleteNavDoc(ctx, tenantID, kbID, docID)
	return err
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

// navDocName builds a deterministic cluster name for the minimal loop.
func navDocName(docID, summary string) string {
	return docID + "_" + contentHash8(summary)
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
