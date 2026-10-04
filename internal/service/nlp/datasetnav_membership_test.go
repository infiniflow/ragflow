package nlp

import (
	"context"
	"fmt"
	"ragflow/internal/engine/types"
	"testing"
)

// wireVectorNavEngine preserves the production search response's JSON vector shape.
type wireVectorNavEngine struct{ *memNavEngine }

func (e wireVectorNavEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	result, err := e.memNavEngine.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]interface{}, len(result.Chunks))
	for i, row := range result.Chunks {
		clone := make(map[string]interface{}, len(row))
		for key, value := range row {
			if vector, ok := value.([]float64); ok {
				array := make([]interface{}, len(vector))
				for j, v := range vector {
					array[j] = v
				}
				clone[key] = array
			} else {
				clone[key] = value
			}
		}
		rows[i] = clone
	}
	return &types.SearchResult{Chunks: rows, Total: result.Total}, nil
}

func assertNavMembership(t *testing.T, engine *memNavEngine, want int) {
	t.Helper()
	clusters := map[string]map[string]interface{}{}
	members := map[string]string{}
	for _, row := range engine.rows {
		if row["type_kwd"] != "nav_cluster" {
			continue
		}
		name := firstStringValue(row["title_kwd"])
		if docID := firstStringValue(row["doc_id"]); docID == "" || docID != firstStringValue(row["kb_id"]) {
			t.Fatalf("cluster %s lacks the dataset doc_id required by the bulk writer", name)
		}
		clusters[name] = row
		ids := firstStringSlice(row["doc_ids_kwd"])
		if count := intValAny(row["doc_count_int"]); count != len(ids) {
			t.Errorf("%s count=%d ids=%d", name, count, len(ids))
		}
		for _, id := range ids {
			if old, found := members[id]; found {
				t.Errorf("member %s belongs to %s and %s", id, old, name)
			}
			members[id] = name
		}
	}
	if len(members) != want {
		t.Errorf("members=%d, want %d", len(members), want)
	}
	leaves := 0
	for _, row := range engine.rows {
		if row["type_kwd"] != "nav_doc" {
			continue
		}
		leaves++
		id := firstStringValue(row["doc_id"])
		parent := firstStringValue(row["parent_kwd"])
		if _, found := clusters[parent]; !found {
			t.Errorf("leaf %s has missing parent %s", id, parent)
		}
		if members[id] != parent {
			t.Errorf("leaf %s parent=%s member owner=%s", id, parent, members[id])
		}
	}
	if leaves != want {
		t.Errorf("leaves=%d, want %d", leaves, want)
	}
}

// TestNavMembershipSurvivesSplitsAndRemoval checks document ownership instead of partition implementation details.
func TestNavMembershipSurvivesSplitsAndRemoval(t *testing.T) {
	engine := newMemNavEngine()
	service := newTestNav(engine)
	service.engine = wireVectorNavEngine{engine}
	for i := 0; i < 110; i++ {
		id := fmt.Sprintf("member_%03d", i)
		input := navUpsertInput("t1", "kb1", id, "Alpha Topic")
		if err := service.UpsertDoc(t.Context(), input); err != nil {
			t.Fatal(err)
		}
		assertNavMembership(t, engine, i+1)
		if i%17 == 0 {
			if err := service.UpsertDoc(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			assertNavMembership(t, engine, i+1)
		}
	}
	// Remove the document that first triggered a split and retry the removal.
	if err := service.RemoveDoc(t.Context(), "t1", "kb1", "member_051"); err != nil {
		t.Fatal(err)
	}
	assertNavMembership(t, engine, 109)
	if err := service.RemoveDoc(t.Context(), "t1", "kb1", "member_051"); err != nil {
		t.Fatal(err)
	}
	assertNavMembership(t, engine, 109)
	if err := service.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "member_051", "Alpha Topic")); err != nil {
		t.Fatal(err)
	}
	assertNavMembership(t, engine, 110)
}

// TestNavAppendMemberRetryDoesNotIncrement checks both supported keyword-array response shapes.
func TestNavAppendMemberRetryDoesNotIncrement(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprint(wrapped), func(t *testing.T) {
			engine := newMemNavEngine()
			service := newTestNav(engine)
			if err := service.UpsertDoc(t.Context(), navUpsertInput("t1", "kb1", "member", "Alpha")); err != nil {
				t.Fatal(err)
			}
			var cluster string
			for _, row := range engine.rows {
				if row["type_kwd"] == "nav_cluster" {
					cluster = firstStringValue(row["title_kwd"])
					if wrapped {
						row["doc_ids_kwd"] = []interface{}{"member"}
					}
				}
			}
			parent, err := service.appendDocToCluster(t.Context(), engine, "t1", "kb1", cluster, "member", "")
			if err != nil || parent != cluster {
				t.Fatalf("parent=%q err=%v", parent, err)
			}
			assertNavMembership(t, engine, 1)
		})
	}
}
