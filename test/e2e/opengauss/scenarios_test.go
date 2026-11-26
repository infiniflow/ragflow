//go:build e2e

package opengauss_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type chunkList struct {
	Chunks []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"chunks"`
	Total int `json:"total"`
	Doc   struct {
		Status          string `json:"ingestion_status"`
		ProgressMessage string `json:"progress_msg"`
	} `json:"doc"`
}

func (s suite) addChunk(t *testing.T, path, content string) string {
	t.Helper()
	data := s.call(t, "POST", path, object{"content": content, "important_keywords": []string{"orbital", "轨道"}, "questions": []string{"How is orbital navigation performed?"}})
	chunk := decode[map[string]json.RawMessage](t, data)["chunk"]
	return field(t, chunk, "id")
}

func (s suite) rejected(t *testing.T, method, path, auth string, body any, wantCode int) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	status, r := s.request(t, method, path, auth, "application/json", bytes.NewReader(b))
	if status >= 500 || *r.Code != wantCode {
		t.Fatalf("%s %s: unexpected HTTP %d, code %d (%s)", method, path, status, *r.Code, r.Message)
	}
}

func (s suite) chunks(t *testing.T) {
	a, b := s.dataset(t, "-a"), s.dataset(t, "-b")
	docA, docB := s.upload(t, a, "a.txt", "fixture A"), s.upload(t, b, "b.txt", "fixture B")
	pathA := "/datasets/" + a + "/documents/" + docA + "/chunks"
	pathB := "/datasets/" + b + "/documents/" + docB + "/chunks"
	const content = "Orbital navigation uses a sapphire beacon. 轨道导航。"
	idA, idB := s.addChunk(t, pathA, content), s.addChunk(t, pathB, "Orbital navigation uses an amber beacon.")
	if s.chunkCount(t, a, idA) != 1 || s.chunkCount(t, b, idB) != 1 {
		t.Fatal("missing rows in openGauss")
	}
	if got := field(t, s.call(t, "GET", pathA+"/"+idA, nil), "content_with_weight"); got != content {
		t.Fatalf("content = %q", got)
	}
	t.Run("search_modes_and_scope", func(t *testing.T) {
		for _, w := range []float64{0, 0.5, 1} {
			hits := s.search(t, []string{a, b}, "orbital navigation", w, nil)
			if !hasHit(hits, idA, a) || !hasHit(hits, idB, b) {
				t.Fatalf("weight %g: expected both chunks, got %.650v", w, hits)
			}
			hits = s.search(t, []string{a, b}, "orbital navigation", w, object{"document_ids": []string{docA}})
			if len(hits) != 1 || !hasHit(hits, idA, a) {
				t.Fatalf("weight %g, document filter: %.650v", w, hits)
			}
		}
	})
	t.Run("pagination", func(t *testing.T) {
		got := decode[chunkList](t, s.call(t, "GET", pathA+"?page=2&page_size=1", nil))
		if got.Total != 1 || len(got.Chunks) != 0 {
			t.Fatalf("second page: %+v", got)
		}
	})
	t.Run("private_dataset", func(t *testing.T) {
		s.rejected(t, "GET", pathA+"/"+idA, s.guest, nil, 403)
		s.rejected(t, "POST", "/retrieval", s.guest, object{"dataset_ids": []string{a}, "question": "orbital"}, 403)
	})
	s.call(t, "PATCH", "/datasets/"+a+"/documents/metadatas", object{"selector": object{"document_ids": []string{docA}}, "updates": []object{{"key": "region", "value": "east"}}})
	t.Run("metadata", func(t *testing.T) {
		data := s.call(t, "GET", "/datasets/"+a+"/metadata/summary?doc_ids="+docA, nil)
		if !bytes.Contains(data, []byte(`"region"`)) || !bytes.Contains(data, []byte(`"east"`)) {
			t.Fatalf("metadata summary: %s", data)
		}
	})
	t.Run("metadata_retrieval", func(t *testing.T) {
		for _, region := range []string{"east", "west"} {
			extra := object{"meta_data_filter": object{"method": "manual", "manual": []object{{"key": "region", "op": "=", "value": region}}}}
			hits := s.search(t, []string{a, b}, "orbital navigation", 0.5, extra)
			if region == "east" && (len(hits) != 1 || !hasHit(hits, idA, a)) || region == "west" && len(hits) != 0 {
				t.Fatalf("region %q, hits %.450v", region, hits)
			}
		}
	})
	t.Run("update_and_invalid_input", func(t *testing.T) {
		const revised = "Orbital telemetry now uses a violet prism. 轨道遥测。"
		s.call(t, "PATCH", pathA+"/"+idA, object{"content": revised})
		s.rejected(t, "PATCH", pathA+"/"+idA, s.owner, object{"available": "invalid"}, 101)
		s.rejected(t, "POST", pathA, s.owner, object{"content": ""}, 102)
		if got := field(t, s.call(t, "GET", pathA+"/"+idA, nil), "content_with_weight"); got != revised {
			t.Fatalf("stored content: %q", got)
		}
		if hits := s.search(t, []string{a}, "violet prism", 0, nil); len(hits) != 1 || !hasHit(hits, idA, a) {
			t.Fatalf("updated text not found: %.450v", hits)
		}
	})
	t.Run("availability", func(t *testing.T) {
		for _, available := range []bool{false, true} {
			s.call(t, "PATCH", pathA+"/"+idA, object{"available": available})
			hits := s.search(t, []string{a}, "orbital navigation", 0.5, nil)
			if hasHit(hits, idA, a) != available {
				t.Fatalf("available=%t, hits %.500v", available, hits)
			}
		}
	})
	s.call(t, "DELETE", pathA, object{"chunk_ids": []string{idA}})
	if s.chunkCount(t, a, idA) != 0 || s.chunkCount(t, b, idB) != 1 {
		t.Fatal("chunk deletion crossed dataset boundary or left stale rows")
	}
	hits := s.search(t, []string{a, b}, "orbital navigation", 0.5, nil)
	if len(hits) != 1 || !hasHit(hits, idB, b) {
		t.Fatalf("post-delete search: %.500v", hits)
	}
	t.Run("delete_dataset_preserves_sibling", func(t *testing.T) {
		kb := s.createDataset(t, "-temporary")
		deleted := false
		t.Cleanup(func() {
			if !deleted {
				s.call(t, "DELETE", "/datasets", object{"ids": []string{kb}})
			}
		})
		doc := s.upload(t, kb, "temporary.txt", "Temporary orbital guide")
		chunk := s.addChunk(t, "/datasets/"+kb+"/documents/"+doc+"/chunks", "Orbital navigation uses a quartz lens.")
		if s.chunkCount(t, kb, chunk) != 1 {
			t.Fatal("temporary chunk missing before dataset deletion")
		}
		s.call(t, "DELETE", "/datasets", object{"ids": []string{kb}})
		deleted = true
		waitFor(t, "dataset index deletion", func() (bool, string) {
			n := s.chunkCount(t, kb, chunk)
			return n == 0, fmt.Sprint(n)
		})
		if s.chunkCount(t, b, idB) != 1 || !hasHit(s.search(t, []string{b}, "orbital navigation", 0.5, nil), idB, b) {
			t.Fatal("sibling dataset lost its searchable chunk")
		}
	})
}

func (s suite) ingest(t *testing.T) {
	kb := s.dataset(t, "-parse")
	const sentence = "The telescope calibrates its orbital navigation with a sapphire beacon."
	doc := s.upload(t, kb, "navigation.txt", sentence+"\nObservations are recorded every night.")
	base := "/datasets/" + kb + "/documents"
	path := base + "/" + doc + "/chunks"
	s.call(t, "POST", base+"/parse", object{"document_ids": []string{doc}})
	var id string
	waitFor(t, "TXT ingestion", func() (bool, string) {
		res := decode[chunkList](t, s.call(t, "GET", path, nil))
		if res.Doc.Status == "FAILED" || res.Doc.Status == "CANCELED" {
			t.Fatalf("ingestion failed: %s", res.Doc.ProgressMessage)
		}
		for _, row := range res.Chunks {
			if strings.Contains(row.Content, sentence) {
				id = row.ID
				return id != "" && res.Doc.Status == "COMPLETED", res.Doc.Status
			}
		}
		return false, fmt.Sprintf("run=%s, total=%d, progress=%s", res.Doc.Status, res.Total, res.Doc.ProgressMessage)
	})
	hits := s.search(t, []string{kb}, "telescope sapphire beacon", 0.5, nil)
	if !hasHit(hits, id, kb) || s.chunkCount(t, kb, id) != 1 {
		t.Fatalf("parsed chunk unavailable: %.500v", hits)
	}
	s.call(t, "DELETE", base, object{"ids": []string{doc}})
	waitFor(t, "document index deletion", func() (bool, string) { n := s.chunkCount(t, kb, id); return n == 0, fmt.Sprint(n) })
}

func (s suite) skills(t *testing.T) {
	space := field(t, s.call(t, "POST", "/skills/spaces", object{"name": s.prefix + "-skills", "embd_id": s.model}), "id")
	t.Cleanup(func() { s.call(t, "DELETE", "/skills/spaces/"+space, nil) })
	id := "guide/setup"
	post := object{"space_id": space, "embd_id": s.model, "skills": []object{{"id": id, "name": "Orbital navigation", "description": "Locate the sapphire beacon", "content": "Orbital navigation guide", "tags": []string{"orbital"}, "version": "1.2"}}}
	s.call(t, "POST", "/skills/index", post)
	search := func(t *testing.T) json.RawMessage {
		return s.call(t, "POST", "/skills/search", object{"space_id": space, "query": "orbital navigation"})
	}
	for _, tc := range []struct {
		name   string
		weight float64
	}{{"keyword", 0}, {"vector", 1}, {"hybrid", 0.5}} {
		t.Run(tc.name, func(t *testing.T) {
			s.call(t, "POST", "/skills/config", object{"space_id": space, "embd_id": s.model, "vector_similarity_weight": tc.weight, "similarity_threshold": 0, "top_k": 10, "field_config": object{"name": object{"enabled": true, "weight": 3}, "description": object{"enabled": true, "weight": 1}}})
			r := decode[struct {
				SearchType string `json:"search_type"`
				Skills     []struct {
					ID            string `json:"skill_id"`
					Name, Version string
				}
			}](t, search(t))
			if r.SearchType != tc.name || len(r.Skills) != 1 || r.Skills[0].ID != id || r.Skills[0].Name != "Orbital navigation" || r.Skills[0].Version != "1.2" {
				t.Fatalf("skill search result: %+v", r)
			}
		})
	}
	t.Run("replace_version", func(t *testing.T) {
		post["skills"] = []object{{"id": id, "name": "Orbital navigation", "description": "Revised beacon guide", "version": "2.0"}}
		s.call(t, "POST", "/skills/index", post)
		result := decode[struct {
			Skills []struct {
				ID      string `json:"skill_id"`
				Version string
			}
		}](t, search(t))
		if len(result.Skills) != 1 || result.Skills[0].ID != id || result.Skills[0].Version != "2.0" {
			t.Fatalf("version replacement: %+v", result)
		}
	})
	t.Run("delete_exact_skill_id", func(t *testing.T) {
		const sibling = "guide_setup"
		s.call(t, "POST", "/skills/config", object{"space_id": space, "embd_id": s.model, "vector_similarity_weight": 0, "similarity_threshold": 0, "top_k": 10})
		post["skills"] = []object{
			{"id": id, "name": "Orbital navigation", "version": "2.0"},
			{"id": sibling, "name": "Orbital navigation checklist", "version": "3.0"},
		}
		s.call(t, "POST", "/skills/index", post)
		result := func() map[string]string {
			t.Helper()
			rows := decode[struct {
				Skills []struct {
					ID      string `json:"skill_id"`
					Version string
				}
			}](t, search(t)).Skills
			versions := make(map[string]string, len(rows))
			for _, row := range rows {
				if _, exists := versions[row.ID]; exists {
					t.Fatalf("duplicate search result for skill %q", row.ID)
				}
				versions[row.ID] = row.Version
			}
			return versions
		}
		if got := result(); len(got) != 2 || got[id] != "2.0" || got[sibling] != "3.0" {
			t.Fatalf("skills before deletion: %v", got)
		}
		s.call(t, "DELETE", "/skills/index?space_id="+space+"&skill_id="+url.QueryEscape(id), nil)
		if got := result(); len(got) != 1 || got[sibling] != "3.0" {
			t.Fatalf("skills after deletion: %v", got)
		}
		s.call(t, "DELETE", "/skills/index?space_id="+space+"&skill_id="+url.QueryEscape(sibling), nil)
		if got := result(); len(got) != 0 {
			t.Fatalf("deleted skill still searchable: %v", got)
		}
	})
}

func (s suite) memory(t *testing.T) {
	id := field(t, s.call(t, "POST", "/memories", object{"name": s.prefix + "-memory", "memory_type": []string{"raw"}, "embd_id": s.model, "llm_id": "raw-only"}), "id")
	path := "/memories/" + id
	t.Cleanup(func() { s.call(t, "DELETE", path, nil) })
	const note = "The orbital sapphire beacon is ready."
	s.call(t, "POST", "/messages", object{"memory_id": []string{id}, "agent_id": "navigator", "session_id": "flight", "user_input": note, "agent_response": "Acknowledged."})
	var msgID json.Number
	waitFor(t, "raw memory persistence", func() (bool, string) {
		type item struct {
			ID json.Number `json:"message_id"`
		}
		res := s.call(t, "GET", path, nil)
		rows := decode[struct {
			Messages struct {
				List []item `json:"message_list"`
			}
		}](t, res).Messages.List
		if len(rows) == 1 {
			msgID = rows[0].ID
			return msgID != "", string(res)
		}
		return false, string(res)
	})
	msgPath := "/messages/" + id + ":" + msgID.String()
	if b := s.call(t, "GET", msgPath+"/content", nil); !strings.Contains(string(b), note) {
		t.Fatalf("memory content: %.500s", b)
	}
	search := func(t *testing.T, w, session string) []json.RawMessage {
		return decode[[]json.RawMessage](t, s.call(t, "GET", "/messages/search?memory_id="+id+"&query=orbital+sapphire&similarity_threshold=0&keywords_similarity_weight="+w+"&agent_id=navigator&session_id="+session, nil))
	}
	s.rejected(t, "GET", msgPath+"/content", s.guest, nil, 404)
	for _, w := range []string{"0", "0.5", "1"} {
		rows := search(t, w, "flight")
		if len(rows) != 1 || !strings.Contains(string(rows[0]), note) {
			t.Errorf("weight %s: %.500s", w, rows)
		}
	}
	if len(search(t, "0.5", "unrelated")) != 0 {
		t.Error("unexpected hit from another session")
	}
	for _, enabled := range []bool{false, true} {
		s.call(t, "PUT", msgPath, object{"status": enabled})
		want := 0
		if enabled {
			want = 1
		}
		if got := len(search(t, "0.5", "flight")); got != want {
			t.Errorf("enabled=%t: got %d messages, want %d", enabled, got, want)
		}
	}
	s.call(t, "DELETE", msgPath, nil)
	if len(search(t, "0.5", "flight")) != 0 {
		t.Error("forgotten message still searchable")
	}
}

func (s suite) concurrentWrites(t *testing.T) {
	kb := s.dataset(t, "-concurrent")
	doc := s.upload(t, kb, "inventory.txt", "Observatory components")
	path := "/datasets/" + kb + "/documents/" + doc + "/chunks"
	entries := []string{"Sapphire telescope", "Quartz mirror", "Amber prism", "Copper sensor"}
	ids := make([]string, len(entries))
	t.Run("writers", func(t *testing.T) {
		for i, content := range entries {
			t.Run(content, func(t *testing.T) {
				t.Parallel()
				ids[i] = s.addChunk(t, path, content)
			})
		}
	})
	rows := decode[chunkList](t, s.call(t, "GET", path, nil))
	if rows.Total != len(entries) || len(rows.Chunks) != len(entries) {
		t.Fatalf("concurrent count = %d, listed %d", rows.Total, len(rows.Chunks))
	}
	for i, id := range ids {
		if id == "" || s.chunkCount(t, kb, id) != 1 {
			t.Fatalf("missing row for %s", entries[i])
		}
		if got := field(t, s.call(t, "GET", path+"/"+id, nil), "content_with_weight"); got != entries[i] {
			t.Fatalf("concurrent write %d: %q", i, got)
		}
	}
}
