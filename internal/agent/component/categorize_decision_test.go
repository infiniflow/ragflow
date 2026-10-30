package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const categorizeDecisionModelID = "3d2d824e7e5d11f1a845455b140cef91"

func setupCategorizeDecisionModel(t *testing.T, baseURL string) *gorm.DB {
	t.Helper()
	dao.GetModelProviderManager()
	db := setupComponentTestDB(t)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "decision-provider", TenantID: "tenant-1", ProviderName: "SystemOne"},
		&entity.TenantModelInstance{ID: "decision-instance", ProviderID: "decision-provider", InstanceName: "default", APIKey: "test-key", Status: "active", Extra: fmt.Sprintf(`{"base_url":%q}`, baseURL)},
		&entity.TenantModel{ID: categorizeDecisionModelID, ProviderID: "decision-provider", InstanceID: "decision-instance", ModelName: "decision-model-preview", ModelType: int(entity.ModelTypeDecision), Status: "active"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestCategorizeDecisionRegisteredRouting(t *testing.T) {
	var captured struct {
		Model     string            `json:"model"`
		State     map[string]string `json:"state"`
		Questions map[string]struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
		} `json:"questions"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected decision request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"answers":{"category_0":{"type":"noul","noul":0.1},"category_1":{"type":"noul","noul":0.9}}}`))
	}))
	defer srv.Close()
	db := setupCategorizeDecisionModel(t, srv.URL+"/v1")
	c, err := New("Categorize", map[string]any{
		"llm_id": categorizeDecisionModelID,
		"query":  "sys.query", "message_history_window_size": 2,
		"sys_prompt": "Prefer the customer's current request.",
		"items": []any{
			map[string]any{"name": "sales", "description": "New purchases", "uuid": "sales-route"},
			map[string]any{"name": "support", "description": "Fix product problems", "uuid": "support-route", "examples": []any{map[string]any{"value": "Login failed"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := canvas.NewCanvasState("run", "task")
	state.Sys["tenant_id"] = "tenant-1"
	state.Sys["query"] = "My login is broken"
	state.History = []map[string]any{{"role": "assistant", "content": "How can I help?"}, {"role": "user", "content": "stale query"}}
	out, err := c.Invoke(canvas.WithState(t.Context(), state), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["category"] != "support" || out["category_name"] != "support" || !reflect.DeepEqual(out["_next"], []string{"support-route"}) {
		t.Fatalf("unexpected category routing: %v", out)
	}
	if !reflect.DeepEqual(out["scores"], map[string]float64{"sales": 0, "support": 1}) {
		t.Fatalf("unexpected scores: %v", out["scores"])
	}
	if captured.Model != "decision-model-preview" || captured.State["query"] != "My login is broken" || !strings.Contains(captured.State["conversation"], "How can I help?") || strings.Contains(captured.State["conversation"], "stale query") {
		t.Fatalf("incorrect model or classification state: %+v", captured)
	}
	for _, question := range captured.Questions {
		if question.Type != "noul" {
			t.Errorf("question type: %s", question.Type)
		}
		for _, want := range []string{"New purchases", "Fix product problems", "Login failed", "Prefer the customer's current request."} {
			if !strings.Contains(question.Instructions, want) {
				t.Errorf("instructions missing %q", want)
			}
		}
	}
}

func TestCategorizeNaceDecisionRouting(t *testing.T) {
	for _, ref := range []string{categorizeDecisionModelID, "4d2d824e7e5d11f1a845455b140cef91", "decision-model-preview@default@Nace", "decision-model-preview@Nace"} {
		t.Run(ref, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected decision request: %s %s", r.Method, r.URL.Path)
				}
				var request struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "decision-model-preview" {
					t.Errorf("unexpected model request: %+v, %v", request, err)
				}
				_, _ = w.Write([]byte(`{"answers":{"category_0":{"type":"noul","noul":0.1},"category_1":{"type":"noul","noul":0.9}}}`))
			}))
			defer srv.Close()
			db := setupCategorizeDecisionModel(t, srv.URL+"/v1")
			if err := db.Model(&entity.TenantModelProvider{}).Where("id = ?", "decision-provider").Update("provider_name", "Nace").Error; err != nil {
				t.Fatal(err)
			}
			const instanceID = "4d2d824e7e5d11f1a845455b140cef91"
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("id", instanceID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.TenantModel{}).Where("id = ?", categorizeDecisionModelID).Updates(map[string]any{"instance_id": instanceID, "model_name": "decision-model-preview"}).Error; err != nil {
				t.Fatal(err)
			}
			c := NewCategorizeComponent(CategorizeParam{ModelID: ref, APIKey: "test-key", Categories: []string{"sales", "support"}, CategoryRoutes: map[string]string{"support": "support-route"}})
			out, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"query": "Login is broken"})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || out["category"] != "support" || !reflect.DeepEqual(out["_next"], []string{"support-route"}) {
				t.Fatalf("Invoke = %v; decision calls = %d", out, calls)
			}
		})
	}
}

func TestCategorizeNaceCompositeRejectsUnavailableModels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		update func(*gorm.DB) error
	}{
		{"disabled model", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModel{}).Where("id = ?", categorizeDecisionModelID).Update("status", "inactive").Error
		}},
		{"disabled instance", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("status", "inactive").Error
		}},
		{"mismatched instance", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("provider_id", "other-provider").Error
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("unavailable model reached upstream") }))
			defer srv.Close()
			db := setupCategorizeDecisionModel(t, srv.URL)
			if err := db.Model(&entity.TenantModelProvider{}).Where("id = ?", "decision-provider").Update("provider_name", "Nace").Error; err != nil {
				t.Fatal(err)
			}
			if err := tc.update(db); err != nil {
				t.Fatal(err)
			}
			c := NewCategorizeComponent(CategorizeParam{ModelID: "decision-model-preview@default@Nace", Categories: []string{"sales", "support"}})
			if _, err := c.Invoke(stateWithTenant("tenant-1"), db, nil); err == nil || calls != 0 {
				t.Fatalf("Invoke error=%v; HTTP calls=%d", err, calls)
			}
		})
	}
}

func TestCategorizeDecisionSelection(t *testing.T) {
	for _, tc := range []struct {
		name            string
		probabilities   []float64
		defaultCategory string
		want            string
	}{
		{"highest", []float64{0.2, 0.8, 0.1}, "a", "b"},
		{"tie", []float64{0.8, 0.8, 0.1}, "c", "a"},
		{"zero", []float64{0, 0, 0}, "c", "c"},
		{"zero without default", []float64{0, 0, 0}, "", "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				answers := map[string]any{}
				for i, probability := range tc.probabilities {
					answers[fmt.Sprintf("category_%d", i)] = map[string]any{"type": "noul", "noul": probability}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
			}))
			defer srv.Close()
			db := setupCategorizeDecisionModel(t, srv.URL)
			c := NewCategorizeComponent(CategorizeParam{ModelID: categorizeDecisionModelID, Categories: []string{"a", "b", "c"}, DefaultCategory: tc.defaultCategory})
			out, err := c.Invoke(stateWithTenant("tenant-1"), db, map[string]any{"query": "query"})
			if err != nil || out["category"] != tc.want {
				t.Fatalf("Invoke = %v, %v; want %s", out, err, tc.want)
			}
		})
	}
}

func TestCategorizeDecisionBatchesAllCategories(t *testing.T) {
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		batchSizes = append(batchSizes, len(request.Questions))
		answers := map[string]any{}
		for key := range request.Questions {
			probability := 0.1
			if key == "category_32" {
				probability = 0.9
			}
			answers[key] = map[string]any{"type": "noul", "noul": probability}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer srv.Close()
	db := setupCategorizeDecisionModel(t, srv.URL)
	categories := make([]string, 33)
	for i := range categories {
		categories[i] = fmt.Sprintf("category %d", i)
	}
	c := NewCategorizeComponent(CategorizeParam{ModelID: categorizeDecisionModelID, Categories: categories})
	out, err := c.Invoke(stateWithTenant("tenant-1"), db, nil)
	if err != nil || out["category"] != "category 32" || !reflect.DeepEqual(batchSizes, []int{16, 16, 1}) {
		t.Fatalf("Invoke = %v, %v; batches=%v", out, err, batchSizes)
	}
}

func TestCategorizeDecisionPreservesCategoryNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"category_0":{"type":"noul","noul":0.1},"category_1":{"type":"noul","noul":0.9}}}`))
	}))
	defer srv.Close()
	db := setupCategorizeDecisionModel(t, srv.URL)
	for _, name := range []string{"Category:support", `"support"`, " support "} {
		c := NewCategorizeComponent(CategorizeParam{
			ModelID: categorizeDecisionModelID, Categories: []string{"sales", name}, CategoryRoutes: map[string]string{name: "support-route"},
		})
		out, err := c.Invoke(stateWithTenant("tenant-1"), db, nil)
		if err != nil || out["category"] != name || !reflect.DeepEqual(out["_next"], []string{"support-route"}) {
			t.Fatalf("category %q: Invoke = %v, %v", name, out, err)
		}
	}
}

func TestCategorizeDecisionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing answer", `{"answers":{"category_0":{"type":"noul","noul":0.9}}}`, 200},
		{"missing probability", `{"answers":{"category_0":{"type":"noul"},"category_1":{"type":"noul","noul":0.1}}}`, 200},
		{"wrong type", `{"answers":{"category_0":{"type":"choice","noul":0.9},"category_1":{"type":"noul","noul":0.1}}}`, 200},
		{"invalid probability", `{"answers":{"category_0":{"type":"noul","noul":1.5}}}`, 200},
		{"invalid json", `invalid`, 200},
		{"unauthorized", `{}`, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			db := setupCategorizeDecisionModel(t, srv.URL)
			c := NewCategorizeComponent(CategorizeParam{ModelID: categorizeDecisionModelID, Categories: []string{"a", "b"}})
			if _, err := c.Invoke(stateWithTenant("tenant-1"), db, nil); err == nil {
				t.Fatal("invalid model response did not fail")
			}
		})
	}
}

func TestCategorizeDecisionRejectsUnavailableModels(t *testing.T) {
	for _, tc := range []struct {
		name, tenant string
		update       func(*gorm.DB) error
	}{
		{"other tenant", "tenant-2", nil},
		{"disabled model", "tenant-1", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModel{}).Where("id = ?", categorizeDecisionModelID).Update("status", "inactive").Error
		}},
		{"disabled instance", "tenant-1", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("status", "inactive").Error
		}},
		{"wrong type", "tenant-1", func(db *gorm.DB) error {
			return db.Model(&entity.TenantModel{}).Where("id = ?", categorizeDecisionModelID).Update("model_type", int(entity.ModelTypeEmbedding)).Error
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCategorizeDecisionModel(t, "http://unused")
			if tc.update != nil {
				if err := tc.update(db); err != nil {
					t.Fatal(err)
				}
			}
			c := NewCategorizeComponent(CategorizeParam{ModelID: categorizeDecisionModelID, Categories: []string{"a", "b"}})
			if _, err := c.Invoke(stateWithTenant(tc.tenant), db, nil); err == nil {
				t.Fatal("unavailable model was accepted")
			}
		})
	}
}

func TestCategorizeDecisionCanceled(t *testing.T) {
	db := setupCategorizeDecisionModel(t, "http://unused")
	ctx, cancel := context.WithCancel(stateWithTenant("tenant-1"))
	cancel()
	c := NewCategorizeComponent(CategorizeParam{ModelID: categorizeDecisionModelID, Categories: []string{"a", "b"}})
	if _, err := c.Invoke(ctx, db, nil); err == nil {
		t.Fatal("canceled classification succeeded")
	}
}

// An explicit decision-only driver with a model name the catalog does not know
// has no resolvable type; it must still take the decision path.
func TestCategorizeExplicitDecisionDriverUnknownModel(t *testing.T) {
	for _, driver := range []string{"SystemOne", "Nace"} {
		t.Run(driver, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/systemone" {
					t.Errorf("unexpected request path %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"answers":{"category_0":{"type":"noul","noul":0.1},"category_1":{"type":"noul","noul":0.9}}}`))
			}))
			defer srv.Close()
			c := NewCategorizeComponent(CategorizeParam{
				Driver: driver, ModelID: "not-in-any-catalog", APIKey: "k", BaseURL: srv.URL + "/v1",
				Categories: []string{"sales", "support"},
			})
			out, err := c.Invoke(stateWithTenant("tenant-1"), setupComponentTestDB(t), map[string]any{"query": "Login is broken"})
			if err != nil || calls != 1 || out["category"] != "support" {
				t.Fatalf("Invoke = %v, %v; decision calls = %d", out, err, calls)
			}
		})
	}
}

func TestCategorizeDecisionTimeout(t *testing.T) {
	if got := categorizeDecisionTimeout("SystemOne"); got != 30*time.Second {
		t.Errorf("SystemOne timeout = %v, want 30s", got)
	}
	if got := categorizeDecisionTimeout("Nace"); got < 60*time.Second {
		t.Errorf("Nace timeout = %v, want at least 60s", got)
	}
}
