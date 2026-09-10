package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/service/nav"

	"gorm.io/gorm"
)

// memNavEngine is an in-memory DocEngine double sufficient for the datasetnav
// minimal closed loop. It stores rows in a slice and supports the filtered
// search / insert / update / delete operations NavService uses. Dense-vector
// KNN is approximated by a deterministic cosine over a synthetic q_<dim>_vec.
type memNavEngine struct {
	rows   []map[string]interface{}
	nextID int
}

func newMemNavEngine() *memNavEngine { return &memNavEngine{} }

func (m *memNavEngine) InsertChunks(_ context.Context, chunks []map[string]interface{}, _ string, datasetID string) ([]string, error) {
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		m.nextID++
		id := "nav" + strconvItoa(m.nextID)
		cp := make(map[string]interface{}, len(c)+2)
		for k, v := range c {
			cp[k] = v
		}
		cp["id"] = id
		cp["kb_id"] = datasetID
		m.rows = append(m.rows, cp)
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *memNavEngine) UpdateChunks(_ context.Context, cond map[string]interface{}, newValue map[string]interface{}, _ string, _ string) error {
	for _, r := range m.rows {
		if matchNavRow(r, cond) {
			for k, v := range newValue {
				r[k] = v
			}
		}
	}
	return nil
}

func (m *memNavEngine) DeleteChunks(_ context.Context, cond map[string]interface{}, _ string, _ string) (int64, error) {
	out := m.rows[:0]
	var deleted int64
	for _, r := range m.rows {
		if matchNavRow(r, cond) {
			deleted++
			continue
		}
		out = append(out, r)
	}
	m.rows = out
	return deleted, nil
}

func (m *memNavEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	var matched []map[string]interface{}
	hasDense := len(req.MatchExprs) > 0
	var queryVec []float64
	if hasDense {
		if de, ok := req.MatchExprs[0].(*types.MatchDenseExpr); ok {
			queryVec = de.EmbeddingData
		}
	}
	for _, r := range m.rows {
		if !matchNavRow(r, req.Filter) {
			continue
		}
		if hasDense {
			col := "q_" + strconvItoa(len(queryVec)) + "_vec"
			rv, ok := r[col].([]float64)
			if !ok {
				continue
			}
			r["_score"] = cosineNav(rv, queryVec)
		}
		matched = append(matched, r)
	}
	if hasDense {
		for i := 1; i < len(matched); i++ {
			for j := i; j > 0 && scoreNavOf(matched[j]) > scoreNavOf(matched[j-1]); j-- {
				matched[j], matched[j-1] = matched[j-1], matched[j]
			}
		}
	}
	offset, limit := req.Offset, req.Limit
	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if limit <= 0 || end > len(matched) {
		end = len(matched)
	}
	return &types.SearchResult{Chunks: matched[offset:end], Total: int64(len(matched))}, nil
}

func (m *memNavEngine) DropChunkStore(context.Context, string, string) error { return nil }
func (m *memNavEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (m *memNavEngine) Close() error               { return nil }
func (m *memNavEngine) Ping(context.Context) error { return nil }
func (m *memNavEngine) GetType() string            { return "mem" }
func (m *memNavEngine) SupportsPageRank() bool     { return false }
func (m *memNavEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (m *memNavEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (m *memNavEngine) CreateMetadataStore(context.Context, string) error { return nil }
func (m *memNavEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (m *memNavEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (m *memNavEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (m *memNavEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (m *memNavEngine) DropMetadataStore(context.Context, string) error { return nil }
func (m *memNavEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return true, nil
}
func (m *memNavEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (m *memNavEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (m *memNavEngine) DeleteDocument(context.Context, string, string) error { return nil }
func (m *memNavEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (m *memNavEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (m *memNavEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (m *memNavEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (m *memNavEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *memNavEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (m *memNavEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (m *memNavEngine) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (m *memNavEngine) FilterDocIdsByMetaPushdown(context.Context, *gorm.DB, []string, []map[string]interface{}, string) []string {
	return nil
}

func matchNavRow(row map[string]interface{}, cond map[string]interface{}) bool {
	for k, v := range cond {
		rv, ok := row[k]
		if !ok {
			return false
		}
		switch want := v.(type) {
		case []string:
			ok = false
			for _, w := range want {
				if rv == w {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		default:
			if rv != v {
				return false
			}
		}
	}
	return true
}

func scoreNavOf(r map[string]interface{}) float64 {
	switch s := r["_score"].(type) {
	case float64:
		return s
	case float32:
		return float64(s)
	}
	return 0
}

func cosineNav(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// stubNavEmbedder returns a fixed deterministic vector per distinct text.
type stubNavEmbedder struct{}

func (stubNavEmbedder) Encode(_ context.Context, _ string, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		// The ES document index template (mapping.json, the ragflow_*
		// dynamic_templates) maps q_<dim>_vec to a dense_vector field only for
		// the standard embedding dimensions 512/768/1024/1536. The integration
		// test runs NavService.Search as a real knn query against that index,
		// so the synthetic vector must use one of those dimensions — otherwise
		// ES dynamically maps q_<dim>_vec as a plain float array and the knn
		// query fails with "[knn] queries are only supported on [dense_vector]
		// fields". 1024 is the canonical RAGFlow embedding size.
		dim := 1024
		v := make([]float32, dim)
		for d := 0; d < dim; d++ {
			v[d] = float32(int(t[0]) + d)
		}
		out[i] = v
	}
	return out, nil
}

func newTestNav(eng *memNavEngine) *NavService {
	ns := NewNavService(stubNavEmbedder{})
	ns.engine = eng
	return ns
}

// TestNavService_UpsertDoc_WritesNavRow asserts acceptance #1: after UpsertDoc
// the row carries both compile_kwd=dataset_nav and available_int=0.
func TestNavService_UpsertDoc_WritesNavRow(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "alpha")); err != nil {
		t.Fatalf("UpsertDoc: %v", err)
	}
	if len(eng.rows) == 0 {
		t.Fatal("expected at least one nav row")
	}
	row := eng.rows[0]
	if row["compile_kwd"] != "dataset_nav" {
		t.Errorf("compile_kwd = %v, want dataset_nav", row["compile_kwd"])
	}
	if row["available_int"] != 0 {
		t.Errorf("available_int = %v, want 0", row["available_int"])
	}
	if row["type_kwd"] != "nav_cluster" {
		t.Errorf("type_kwd = %v, want nav_cluster", row["type_kwd"])
	}
	if row["parent_kwd"] != "root" {
		t.Errorf("parent_kwd = %v, want root", row["parent_kwd"])
	}
}

// TestNavService_ListClusters_FiltersRoot asserts acceptance #3.
func TestNavService_ListClusters_FiltersRoot(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "aaa")); err != nil {
		t.Fatal(err)
	}
	clusters, total, err := ns.ListClusters(t.Context(), "t1", "kb1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(clusters) != 1 {
		t.Fatalf("expected 1 root cluster, got total=%d len=%d", total, len(clusters))
	}
	if clusters[0].Type != "cluster" {
		t.Errorf("cluster type = %s, want cluster", clusters[0].Type)
	}
	if clusters[0].DocCount < 1 {
		t.Errorf("cluster doc_count = %d, want >=1", clusters[0].DocCount)
	}
}

// TestNavNode_JSONShape_SnakeCase locks the REST contract: the frontend
// DatasetNavNode and Python GET /navigation both use snake_case keys. If the
// Go field names leaked (Name/Description/DocCount...) the frontend tree would
// read undefined fields and render empty.
func TestNavNode_JSONShape_SnakeCase(t *testing.T) {
	n := nav.NavNode{Name: "cluster_x", Description: "d", DocCount: 3, Type: "cluster", HasChildren: true}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name", "description", "doc_count", "type", "has_children"} {
		if _, ok := m[want]; !ok {
			t.Errorf("NavNode JSON missing snake_case key %q; got %s", want, b)
		}
	}
	for _, bad := range []string{"Name", "Description", "DocCount", "HasChildren"} {
		if _, ok := m[bad]; ok {
			t.Errorf("NavNode JSON leaked PascalCase key %q; got %s", bad, b)
		}
	}
}

// TestNavNamingHelpers_Readable verifies the nav display-name helpers produce
// human-readable names (mirroring Python _clean_title/_fallback_title/
// _readable_cluster_name) instead of raw ids.
func TestNavNamingHelpers_Readable(t *testing.T) {
	if cleanTitle("  hello   world  ") != "hello world" {
		t.Errorf("cleanTitle = %q, want %q", cleanTitle("  hello   world  "), "hello world")
	}
	// fallbackTitle takes the first non-empty line and strips Markdown markers.
	if got := fallbackTitle("a b c d e f g h"); got != "a b c d e f g h" {
		t.Errorf("fallbackTitle = %q, want the first line", got)
	}
	if got := fallbackTitle("**何进诛阉与董后之废**\n\nbody text"); got != "何进诛阉与董后之废" {
		t.Errorf("fallbackTitle = %q, want the stripped markdown title", got)
	}
	if got := fallbackTitle(""); got != "Cluster" {
		t.Errorf("fallbackTitle('') = %q, want Cluster", got)
	}
	name := readableClusterName("何进诛阉", "seed-text")
	if !strings.HasPrefix(name, "何进诛阉 ") {
		t.Errorf("readableClusterName = %q, want prefix %q", name, "何进诛阉 ")
	}
}

// TestNavNamingHelpers_RawIDDetection verifies meaningless ids are detected so
// nodeFromRow can fall back to a readable title.
func TestNavNamingHelpers_RawIDDetection(t *testing.T) {
	for _, raw := range []string{"d3778ef9c0f5495fa4bdadc00a5bf15c", "cluster_abc12345", ""} {
		if !graphIsRawID(raw) {
			t.Errorf("graphIsRawID(%q) = false, want true", raw)
		}
	}
	for _, ok := range []string{"何进诛阉", "NVIDIA financial performance 8738e200"} {
		if graphIsRawID(ok) {
			t.Errorf("graphIsRawID(%q) = true, want false", ok)
		}
	}
}

// TestNodeFromRow_ReadableName verifies nodeFromRow falls back to a readable
// title derived from the payload description when title_kwd is a raw id.
func TestNodeFromRow_ReadableName(t *testing.T) {
	ns := &NavService{}
	row := map[string]interface{}{
		"title_kwd":           "d3778ef9c0f5495fa4bdadc00a5bf15c",
		"doc_id":              "d3778ef9c0f5495fa4bdadc00a5bf15c",
		"type_kwd":            "nav_doc",
		"doc_count_int":       1,
		"content_with_weight": `{"type":"nav_doc","description":"刘备三战黄巾军与何进诛阉之议\nsecond line"}`,
	}
	node := ns.nodeFromRow(row, "doc")
	if node.Name == "" || node.Name == "d3778ef9c0f5495fa4bdadc00a5bf15c" {
		t.Fatalf("nodeFromRow name = %q, want readable fallback", node.Name)
	}
	if node.DocID != "d3778ef9c0f5495fa4bdadc00a5bf15c" {
		t.Errorf("nodeFromRow DocID = %q, want the raw doc id preserved", node.DocID)
	}
	// A cluster row keeps its stored key verbatim (it is the child-lookup
	// parent_kwd), even when it looks like a raw id (review Major).
	clusterRow := map[string]interface{}{
		"title_kwd":     "cluster_abc12345",
		"type_kwd":      "nav_cluster",
		"doc_count_int": 2,
	}
	cluster := ns.nodeFromRow(clusterRow, "cluster")
	if cluster.Name != "cluster_abc12345" {
		t.Errorf("cluster key = %q, want it kept verbatim as the parent_kwd lookup key", cluster.Name)
	}
}

// TestNavService_Search_ReturnsHit asserts acceptance #5.
func TestNavService_Search_ReturnsHit(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "aaa")); err != nil {
		t.Fatal(err)
	}
	hits, err := ns.Search(t.Context(), "t1", "kb1", "aaa", nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].Name == "" {
		t.Error("hit name empty")
	}
}

// TestNavService_Acceptance4_ListChildren asserts acceptance #4: ListChildren
// returns only the rows whose parent_kwd=name. Two docs with identical stub
// vectors merge into one root cluster: the first becomes the cluster itself,
// the second merges in as a nav_doc (parent_kwd=clusterName). So the cluster
// doc_count reflects both docs, and exactly one nav_doc sits under it.
func TestNavService_Acceptance4_ListChildren(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	// Two docs that merge into one root cluster (same first char -> identical
	// stub vectors -> sim=1.0 >= merge threshold).
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "aaa one")); err != nil {
		t.Fatal(err)
	}
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d2", "aaa two")); err != nil {
		t.Fatal(err)
	}
	clusters, _, err := ns.ListClusters(t.Context(), "t1", "kb1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].DocCount != 2 {
		t.Fatalf("cluster doc_count = %d, want 2 (both docs merged into the cluster)", clusters[0].DocCount)
	}
	name := clusters[0].Name
	children, total, err := ns.ListChildren(t.Context(), "t1", "kb1", name, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Every doc upserted under the cluster emits a nav_doc leaf (Python
	// upsert_dataset_nav_doc), so d1 (which created the cluster) and d2 (which
	// merged in) both sit under it: two nav_docs total.
	if total != 2 || len(children) != 2 {
		t.Fatalf("expected 2 children under cluster, got total=%d len=%d", total, len(children))
	}
	gotIDs := map[string]bool{}
	for _, c := range children {
		gotIDs[c.DocID] = true
		if c.Type != "doc" {
			t.Errorf("child type = %q, want doc", c.Type)
		}
	}
	if !gotIDs["d1"] || !gotIDs["d2"] {
		t.Errorf("expected nav_docs for both d1 and d2, got %v", gotIDs)
	}
}

// TestNavService_NavDocDepth asserts a nav_doc merged under a root cluster
// (depth 0) gets depth_int = parentDepth+1 = 1, not a hard-coded value.
func TestNavService_NavDocDepth(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "aaa one")); err != nil {
		t.Fatal(err)
	}
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d2", "aaa two")); err != nil {
		t.Fatal(err)
	}
	// The nav_doc for d2 sits under the root cluster; its depth_int must be 1.
	for _, row := range eng.rows {
		if row["doc_id"] == "d2" {
			if d, ok := row["depth_int"].(int); !ok || d != 1 {
				t.Errorf("nav_doc d2 depth_int = %v, want 1 (parentDepth 0 + 1)", row["depth_int"])
			}
		}
	}
}

// TestNavService_RemoveDoc_CascadesToEmptyCluster covers A3: after removing a
// doc, a cluster left with no docs and no children is pruned (cleanupEmptyCluster),
// and the doc is dropped from its parent cluster's doc_ids_kwd.
func TestNavService_RemoveDoc_CascadesToEmptyCluster(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	// d1 creates a root cluster (name derived from its summary hash).
	if err := ns.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "d1", "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := ns.RemoveDoc(t.Context(), "t1", "kb1", "d1"); err != nil {
		t.Fatal(err)
	}
	// The nav_doc is gone, and the root cluster (now empty) is pruned.
	for _, row := range eng.rows {
		if row["type_kwd"] == "nav_doc" && row["doc_id"] == "d1" {
			t.Error("nav_doc for d1 should have been removed")
		}
		if row["type_kwd"] == "nav_cluster" && row["title_kwd"] != "" {
			t.Error("root cluster should have been pruned when empty")
		}
	}
}

// TestNavService_MaybeSplitCluster_SplitsOverfull covers A2: a cluster with more
// than maxDocsPerCluster direct docs is split into two siblings. The overfull
// state is seeded directly (InsertChunks) rather than via UpsertDoc, because
// UpsertDoc triggers the split during seeding once the count crosses the
// threshold — the manual split trigger here must act on an already-overfull,
// not-yet-split cluster.
func TestNavService_MaybeSplitCluster_SplitsOverfull(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	clusterName := "root_cluster"
	// Seed the overfull cluster directly: one nav_cluster (parent=root sentinel,
	// depth 1) carrying 55 nav_doc children.
	idx := sTestNavIndex(ns)
	rows := []map[string]interface{}{
		{
			"compile_kwd":   navCompileKwd,
			"available_int": 0,
			"type_kwd":      "nav_cluster",
			"title_kwd":     clusterName,
			"parent_kwd":    navRootParent,
			"depth_int":     1,
			"doc_count_int": 55, // over the maxDocsPerCluster=50 threshold
			"doc_ids_kwd":   []string{},
		},
	}
	for i := 0; i < 55; i++ {
		rows = append(rows, map[string]interface{}{
			"compile_kwd":   navCompileKwd,
			"available_int": 0,
			"type_kwd":      "nav_doc",
			"title_kwd":     fmt.Sprintf("d%02d", i),
			"parent_kwd":    clusterName,
			"depth_int":     2,
			"doc_id":        fmt.Sprintf("d%02d", i),
			"doc_count_int": 1,
		})
	}
	if _, err := eng.InsertChunks(t.Context(), rows, idx, "kb1"); err != nil {
		t.Fatal(err)
	}
	if err := ns.maybeSplitCluster(t.Context(), "t1", "kb1", clusterName, ""); err != nil {
		t.Fatal(err)
	}
	splitA := clusterName + ":A"
	splitB := clusterName + ":B"
	foundA, foundB := false, false
	var countA, countB int
	var idsA, idsB []string
	for _, row := range eng.rows {
		if row["type_kwd"] == "nav_cluster" && row["title_kwd"] == splitA {
			foundA = true
			countA = intValAny(row["doc_count_int"])
			idsA = firstStringSlice(row["doc_ids_kwd"])
		}
		if row["type_kwd"] == "nav_cluster" && row["title_kwd"] == splitB {
			foundB = true
			countB = intValAny(row["doc_count_int"])
			idsB = firstStringSlice(row["doc_ids_kwd"])
		}
	}
	if !foundA || !foundB {
		t.Errorf("split clusters :A / :B missing (A=%v B=%v)", foundA, foundB)
	}
	// Review issue 5: the split clusters must inherit the aggregate doc count so
	// they stay searchable and support deletion bookkeeping — they must not be
	// empty shells.
	if countA == 0 && countB == 0 {
		t.Errorf("split clusters must carry aggregate doc_count_int (A=%d B=%d)", countA, countB)
	}
	if countA+countB != 55 {
		t.Errorf("split clusters must preserve all 55 docs (A=%d B=%d)", countA, countB)
	}
	if len(idsA)+len(idsB) != 55 {
		t.Errorf("split clusters must preserve all 55 doc ids (A=%v B=%v)", idsA, idsB)
	}
	// Review issue 5/6: every child must be reparented under :A or :B, and the
	// original cluster must be gone (not left orphaned).
	for _, row := range eng.rows {
		if row["type_kwd"] == "nav_doc" {
			p, _ := row["parent_kwd"].(string)
			if p != splitA && p != splitB {
				t.Errorf("nav_doc %v left orphaned under parent %q", row["title_kwd"], p)
			}
		}
	}
	origGone := true
	for _, row := range eng.rows {
		if row["type_kwd"] == "nav_cluster" && row["title_kwd"] == clusterName {
			origGone = false
		}
	}
	if !origGone {
		t.Error("original cluster should have been deleted after split")
	}
	// A production nav_doc has doc_id but no doc_ids_kwd. Removing one after a
	// split must update the replacement cluster that inherited its membership.
	if err := ns.RemoveDoc(t.Context(), "t1", "kb1", "d00"); err != nil {
		t.Fatalf("RemoveDoc after split: %v", err)
	}
	var remainingCount, remainingIDs int
	for _, row := range eng.rows {
		if row["type_kwd"] != "nav_cluster" {
			continue
		}
		remainingCount += intValAny(row["doc_count_int"])
		for _, id := range firstStringSlice(row["doc_ids_kwd"]) {
			if id == "d00" {
				t.Errorf("removed doc d00 still present in split cluster membership")
			}
			remainingIDs++
		}
	}
	if remainingCount != 54 || remainingIDs != 54 {
		t.Errorf("RemoveDoc after split must update count and membership: count=%d ids=%d", remainingCount, remainingIDs)
	}
}

// sTestNavIndex returns the nav index name for a test NavService (tenant "t1").
func sTestNavIndex(ns *NavService) string {
	return "ragflow_t1"
}

func navUpsertInput(tenant, kb, doc, summary string) nav.UpsertDocInput {
	return nav.UpsertDocInput{TenantID: tenant, KbID: kb, DocID: doc, Summary: summary}
}

// intValAny coerces a stored integer value (int/int64/float64) from the mem
// engine row into an int for assertions.
func intValAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	}
	return 0
}
