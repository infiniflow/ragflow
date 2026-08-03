package nlp

import (
	"context"
	"math"
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
		dim := 8
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
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d1", "alpha")); err != nil {
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
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d1", "aaa")); err != nil {
		t.Fatal(err)
	}
	clusters, total, err := ns.ListClusters(context.Background(), "t1", "kb1", 0, 10)
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

// TestNavService_Search_ReturnsHit asserts acceptance #5.
func TestNavService_Search_ReturnsHit(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d1", "aaa")); err != nil {
		t.Fatal(err)
	}
	hits, err := ns.Search(context.Background(), "t1", "kb1", "aaa", nil, 5)
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
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d1", "aaa one")); err != nil {
		t.Fatal(err)
	}
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d2", "aaa two")); err != nil {
		t.Fatal(err)
	}
	clusters, _, err := ns.ListClusters(context.Background(), "t1", "kb1", 0, 10)
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
	children, total, err := ns.ListChildren(context.Background(), "t1", "kb1", name, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one nav_doc (for d2) sits under the cluster; d1 is the cluster.
	if total != 1 || len(children) != 1 {
		t.Fatalf("expected 1 child under cluster, got total=%d len=%d", total, len(children))
	}
	if children[0].DocID != "d2" {
		t.Errorf("child doc_id = %q, want d2", children[0].DocID)
	}
	if children[0].Type != "doc" {
		t.Errorf("child type = %q, want doc", children[0].Type)
	}
}

// TestNavService_NavDocDepth asserts a nav_doc merged under a root cluster
// (depth 0) gets depth_int = parentDepth+1 = 1, not a hard-coded value.
func TestNavService_NavDocDepth(t *testing.T) {
	eng := newMemNavEngine()
	ns := newTestNav(eng)
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d1", "aaa one")); err != nil {
		t.Fatal(err)
	}
	if err := ns.UpsertDoc(context.Background(), navUpsertInput("t1", "kb1", "d2", "aaa two")); err != nil {
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

func navUpsertInput(tenant, kb, doc, summary string) nav.UpsertDocInput {
	return nav.UpsertDocInput{TenantID: tenant, KbID: kb, DocID: doc, Summary: summary}
}
