package esoscommon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/engine/internal/esoscommon"
)

// The OpenSearch and Elasticsearch engines render vector mappings differently
// (knn_vector vs dense_vector, dimension vs dims, space_type vs similarity, and
// the index.knn setting). These tests lock the per-engine VectorFieldSpec and
// the parameterized mapping builders so a future change cannot silently collapse
// the two engines onto one flavour again.

func TestESVectorFieldSpec(t *testing.T) {
	vf := esoscommon.ESVectorField
	if vf.Type != "dense_vector" || vf.DimsKey != "dims" ||
		vf.SimKey != "similarity" || vf.SimValue != "cosine" || vf.IndexKnnbool {
		t.Fatalf("ESVectorField wrong: %+v", vf)
	}
}

func TestOSVectorFieldSpec(t *testing.T) {
	vf := esoscommon.OSVectorField
	if vf.Type != "knn_vector" || vf.DimsKey != "dimension" ||
		vf.SimKey != "space_type" || vf.SimValue != "cosinesimil" || !vf.IndexKnnbool {
		t.Fatalf("OSVectorField wrong: %+v", vf)
	}
}

func TestMemoryMessageVectorProperty_ES(t *testing.T) {
	p := esoscommon.MemoryMessageVectorProperty(128, esoscommon.ESVectorField)
	if p["type"] != "dense_vector" {
		t.Errorf("type=%v", p["type"])
	}
	if p["dims"] != 128 {
		t.Errorf("dims=%v", p["dims"])
	}
	// Adversarial: the ES flavour must not emit the OpenSearch key.
	if _, ok := p["dimension"]; ok {
		t.Error("ES must not emit 'dimension' key")
	}
}

func TestMemoryMessageVectorProperty_OS(t *testing.T) {
	p := esoscommon.MemoryMessageVectorProperty(128, esoscommon.OSVectorField)
	if p["type"] != "knn_vector" {
		t.Errorf("type=%v", p["type"])
	}
	if p["dimension"] != 128 {
		t.Errorf("dimension=%v", p["dimension"])
	}
	if p["space_type"] != "cosinesimil" {
		t.Errorf("space_type=%v", p["space_type"])
	}
	// Adversarial: the OS flavour must not emit the Elasticsearch key.
	if _, ok := p["dims"]; ok {
		t.Error("OS must not emit 'dims' key")
	}
}

func TestGetMemoryMessageMapping_OSKnn(t *testing.T) {
	m := esoscommon.GetMemoryMessageMapping(128, esoscommon.OSVectorField)
	settings, ok := m["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("settings missing")
	}
	index, ok := settings["index"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.index missing for OpenSearch")
	}
	if index["knn"] != true {
		t.Errorf("OpenSearch must set index.knn=true, got %v", index["knn"])
	}
}

func TestGetMemoryMessageMapping_ESNoKnn(t *testing.T) {
	m := esoscommon.GetMemoryMessageMapping(128, esoscommon.ESVectorField)
	settings, ok := m["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("settings missing")
	}
	// Adversarial: Elasticsearch must not set settings.index.knn.
	if _, ok := settings["index"]; ok {
		t.Error("Elasticsearch must not set settings.index.knn")
	}
}

func TestGetDefaultSkillMapping_OSVectorType(t *testing.T) {
	m := esoscommon.GetDefaultSkillMapping(esoscommon.OSVectorField)
	// OpenSearch must set settings.index.knn=true for knn_vector fields.
	settings, ok := m["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("settings missing")
	}
	index, ok := settings["index"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.index missing for OpenSearch")
	}
	if index["knn"] != true {
		t.Errorf("OpenSearch must set settings.index.knn=true, got %v", index["knn"])
	}
	props := m["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	q := props["q_3072_vec"].(map[string]interface{})
	if q["type"] != "knn_vector" {
		t.Errorf("type=%v", q["type"])
	}
	if q["dimension"] != 3072 {
		t.Errorf("dimension=%v", q["dimension"])
	}
	if q["space_type"] != "cosinesimil" {
		t.Errorf("space_type=%v", q["space_type"])
	}
	// Adversarial: OpenSearch must not emit the Elasticsearch keys.
	if _, ok := q["dims"]; ok {
		t.Error("OpenSearch must not emit 'dims'")
	}
	if _, ok := q["similarity"]; ok {
		t.Error("OpenSearch must not emit 'similarity'")
	}
}

func TestGetDefaultSkillMapping_ESVectorType(t *testing.T) {
	m := esoscommon.GetDefaultSkillMapping(esoscommon.ESVectorField)
	// Elasticsearch must not set settings.index.knn (dense_vector needs no knn).
	settings, ok := m["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("settings missing")
	}
	if _, ok := settings["index"]; ok {
		t.Error("Elasticsearch must not set settings.index.knn")
	}
	props := m["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	q := props["q_3072_vec"].(map[string]interface{})
	if q["type"] != "dense_vector" {
		t.Errorf("type=%v", q["type"])
	}
	if q["dims"] != 3072 {
		t.Errorf("dims=%v", q["dims"])
	}
	if q["similarity"] != "cosine" {
		t.Errorf("similarity=%v", q["similarity"])
	}
}

func TestBuildRankFeatureQuery_PagerankToggle(t *testing.T) {
	rf := map[string]float64{"tag_a": 1.0, "pagerank_fea": 2.0}
	// Disabled: must NOT contain a rank_feature clause for pagerank_fea.
	off := esoscommon.BuildRankFeatureQuery(rf, false)
	for _, clause := range off {
		if f, ok := clause["rank_feature"].(map[string]interface{}); ok {
			if f["field"] == "pagerank_fea" {
				t.Error("pagerank clause must be skipped when disabled")
			}
		}
	}
	// Enabled: MUST contain a rank_feature clause for pagerank_fea.
	on := esoscommon.BuildRankFeatureQuery(rf, true)
	found := false
	for _, clause := range on {
		if f, ok := clause["rank_feature"].(map[string]interface{}); ok {
			if f["field"] == "pagerank_fea" {
				found = true
			}
		}
	}
	if !found {
		t.Error("pagerank clause must be present when enabled")
	}
}

func TestLoadSkillMapping_ReadsGivenFile(t *testing.T) {
	// Adversarial: the loader must read the file NAME it is given, not a hardcoded one.
	dir := t.TempDir()
	f := filepath.Join(dir, "skill_os_mapping.json")
	data := `{"mappings":{"properties":{"q_256_vec":{"type":"knn_vector","dimension":256,"space_type":"cosinesimil"}}}}`
	if err := os.WriteFile(f, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := esoscommon.LoadSkillMapping(f, esoscommon.OSVectorField)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, _ := json.Marshal(m)
	if !strings.Contains(string(b), "knn_vector") {
		t.Errorf("loader must read the given file, got %s", b)
	}
}

func TestLoadSkillMapping_MissingFileFallsBack(t *testing.T) {
	// When the file is missing the loader must not error; it must return the
	// engine-correct default mapping (OpenSearch -> knn_vector) rather than a
	// hardcoded ES flavour.
	m, err := esoscommon.LoadSkillMapping(filepath.Join(t.TempDir(), "nope.json"), esoscommon.OSVectorField)
	if err != nil {
		t.Fatalf("missing file should fall back, not error: %v", err)
	}
	props := m["mappings"].(map[string]interface{})["properties"].(map[string]interface{})
	q := props["q_3072_vec"].(map[string]interface{})
	if q["type"] != "knn_vector" {
		t.Errorf("fallback must be engine-correct (knn_vector), got %v", q["type"])
	}
}
