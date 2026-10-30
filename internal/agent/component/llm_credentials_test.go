package component

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestCompositeModelPropagatesLookupErrors(t *testing.T) {
	for _, tc := range []struct {
		name, table, instance string
	}{
		{"provider", "tenant_model_provider", "default"},
		{"default instance", "tenant_model_instance", "default"},
		{"named instance", "tenant_model_instance", "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCategorizeDecisionModel(t, "https://example.invalid/v1")
			lookupErr := errors.New("lookup unavailable")
			queries := make(map[string]int)
			if err := db.Callback().Query().Before("gorm:query").Register("test:lookup_error", func(tx *gorm.DB) {
				queries[tx.Statement.Table]++
				if tx.Statement.Table == tc.table && queries[tc.table] == 1 {
					tx.AddError(lookupErr)
				}
			}); err != nil {
				t.Fatal(err)
			}
			ref := fmt.Sprintf("decision-model-preview@%s@SystemOne", tc.instance)
			_, _, _, _, _, err := resolveModelRefWithType(stateWithTenant("tenant-1"), db, ref, "", "", "", entity.ModelTypeDecision)
			if !errors.Is(err, lookupErr) {
				t.Fatalf("resolveModelRefWithType error = %v, want %v", err, lookupErr)
			}
			if queries["tenant_model_provider"] != 1 || queries[tc.table] != 1 || queries["tenant_model"] != 0 {
				t.Fatalf("lookup error triggered further resolution: %v", queries)
			}
		})
	}
}

func TestCompositeModelMissingRecordsRemainUnresolved(t *testing.T) {
	// Two-part model@provider references keep the legacy fallback.
	for _, ref := range []string{"decision-model-preview@missing-provider"} {
		t.Run(ref, func(t *testing.T) {
			db := setupCategorizeDecisionModel(t, "https://example.invalid/v1")
			_, _, key, baseURL, _, ok, err := resolveTenantCompositeModel(stateWithTenant("tenant-1"), db, ref, "override-key", "https://example.invalid/override", entity.ModelTypeDecision)
			if err != nil || ok || key != "override-key" || baseURL != "https://example.invalid/override" {
				t.Fatalf("missing record resolution = %t, %v; overrides preserved = %t", ok, err, key == "override-key" && baseURL == "https://example.invalid/override")
			}
		})
	}
}

func TestCompositeModelRejectsUnresolvedInstanceReference(t *testing.T) {
	for _, ref := range []string{
		"decision-model-preview@default@Nace",
		"decision-model-preview@missing-instance@SystemOne",
	} {
		t.Run(ref, func(t *testing.T) {
			db := setupCategorizeDecisionModel(t, "https://example.invalid/v1")
			_, _, _, _, _, err := resolveModelRefWithType(stateWithTenant("tenant-1"), db, ref, "", "override-key", "https://example.invalid/override", entity.ModelTypeDecision)
			if err == nil {
				t.Fatal("unresolved three-part reference must not fall back to the legacy split")
			}
		})
	}
}

func TestCompositeModelMissingDefaultUsesSoleActiveInstance(t *testing.T) {
	db := setupCategorizeDecisionModel(t, "https://example.invalid/v1")
	if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("instance_name", "custom").Error; err != nil {
		t.Fatal(err)
	}
	model, driver, _, baseURL, modelType, ok, err := resolveTenantCompositeModel(stateWithTenant("tenant-1"), db, "decision-model-preview@default@SystemOne", "", "", entity.ModelTypeDecision)
	if err != nil || !ok || model != "decision-model-preview" || driver != "SystemOne" || baseURL != "https://example.invalid/v1" || modelType != entity.ModelTypeDecision {
		t.Fatalf("default fallback = %q, %q, %q, %v, %t, %v", model, driver, baseURL, modelType, ok, err)
	}
}

func TestCompositeModelMissingModelRowChecksInstanceStatus(t *testing.T) {
	ref := "gone-model@default@SystemOne"
	db := setupCategorizeDecisionModel(t, "https://example.invalid/v1")
	// Active instance: the fallback to the instance credentials is kept.
	if _, _, _, _, _, err := resolveModelRefWithType(stateWithTenant("tenant-1"), db, ref, "", "", "", entity.ModelTypeDecision); err != nil {
		t.Fatalf("active instance, missing model row: %v", err)
	}
	if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "decision-instance").Update("status", "inactive").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, key, _, _, err := resolveModelRefWithType(stateWithTenant("tenant-1"), db, ref, "", "", "", entity.ModelTypeDecision); err == nil {
		t.Fatalf("inactive instance with a missing model row resolved (key %q)", key)
	}
}

func TestNaceSavedKeyIsNotSentToAnotherOrigin(t *testing.T) {
	naceInstance := func(extra string) *entity.TenantModelInstance {
		return &entity.TenantModelInstance{APIKey: "saved-key", Extra: extra}
	}
	for _, tc := range []struct {
		name, provider, extra, override string
		wantKey                         string
	}{
		{"no override", "Nace", "", "", "saved-key"},
		{"default origin", "Nace", "", "https://console.nace.ai/v1", "saved-key"},
		{"default port spelled out", "Nace", "", "https://console.nace.ai:443/v1", "saved-key"},
		{"other host", "Nace", "", "https://attacker.example/v1", ""},
		{"subdomain", "Nace", "", "https://x.console.nace.ai/v1", ""},
		{"other port", "Nace", "", "https://console.nace.ai:8443/v1", ""},
		{"other scheme", "Nace", "", "http://console.nace.ai/v1", ""},
		{"saved origin", "Nace", `{"base_url":"https://nace.internal:9000/v1"}`, "https://nace.internal:9000/other", "saved-key"},
		{"saved origin, other host", "Nace", `{"base_url":"https://nace.internal:9000/v1"}`, "https://console.nace.ai/v1", ""},
		{"other providers keep behaviour", "OpenAI", "", "https://proxy.example/v1", "saved-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, baseURL := fillInstanceCredentials(tc.provider, naceInstance(tc.extra), "", tc.override)
			if key != tc.wantKey || (tc.override != "" && baseURL != tc.override) {
				t.Fatalf("key=%q baseURL=%q, want key %q", key, baseURL, tc.wantKey)
			}
		})
	}
}

func TestNaceLegacyFallbackDoesNotSendSavedKeyToAnotherOrigin(t *testing.T) {
	db := setupCategorizeDecisionModel(t, "https://console.nace.ai/v1")
	if err := db.Model(&entity.TenantModelProvider{}).Where("id = ?", "decision-provider").Update("provider_name", "Nace").Error; err != nil {
		t.Fatal(err)
	}
	if key, _, ok := resolveTenantModelInstanceCredentials(stateWithTenant("tenant-1"), db, "tenant-1", "decision-model-preview@default@Nace", "https://attacker.example/v1"); ok || key != "" {
		t.Fatalf("saved key released to another origin: %q, %t", key, ok)
	}
	if key, _, ok := resolveTenantModelInstanceCredentials(stateWithTenant("tenant-1"), db, "tenant-1", "decision-model-preview@default@Nace", "https://console.nace.ai/v1"); !ok || key != "test-key" {
		t.Fatalf("same-origin override lost the saved key: %q, %t", key, ok)
	}
}

// setupLegacyProviderModels creates a tenant with a non-decision provider and
// one model row, as an LLM or Agent node would reference it.
func setupLegacyProviderModels(t *testing.T, modelType int, status string) *gorm.DB {
	t.Helper()
	db := setupComponentTestDB(t)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "zhipu-provider", TenantID: "tenant-1", ProviderName: "ZHIPU-AI"},
		&entity.TenantModelInstance{ID: "zhipu-instance", ProviderID: "zhipu-provider", InstanceName: "default", APIKey: "saved-key", Status: "active", Extra: `{"base_url":"https://saved.example/v1"}`},
		&entity.TenantModel{ID: "zhipu-model", ProviderID: "zhipu-provider", InstanceID: "zhipu-instance", ModelName: "glm-4v", ModelType: modelType, Status: status},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// Non-decision references must resolve as they did before decision routing:
// vision-only and disabled rows, and providers with no tenant row, are neither
// rejected nor matched, and explicit credentials are left alone.
func TestCompositeModelLeavesNonDecisionReferencesUntouched(t *testing.T) {
	for _, tc := range []struct {
		name      string
		modelType int
		status    string
		ref       string
		apiKey    string
		baseURL   string
		wantKey   string
		wantBase  string
	}{
		{"vision-only three-part (agent)", int(entity.ModelTypeImage2Text), "active", "glm-4v@default@ZHIPU-AI", "", "", "saved-key", "https://saved.example/v1"},
		{"vision-only two-part (agent)", int(entity.ModelTypeImage2Text), "active", "glm-4v@ZHIPU-AI", "", "", "saved-key", "https://saved.example/v1"},
		{"disabled chat row", int(entity.ModelTypeChat), "inactive", "glm-4v@default@ZHIPU-AI", "", "", "saved-key", "https://saved.example/v1"},
		{"explicit key keeps provider default base", int(entity.ModelTypeChat), "active", "glm-4v@default@ZHIPU-AI", "explicit", "", "explicit", ""},
		{"no tenant provider row, explicit credentials", int(entity.ModelTypeChat), "active", "glm-4v@default@OTHER-PROVIDER", "explicit", "https://wf.example/v1", "explicit", "https://wf.example/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupLegacyProviderModels(t, tc.modelType, tc.status)
			model, driver, key, baseURL, err := resolveChatModelRef(stateWithTenant("tenant-1"), db, tc.ref, "", tc.apiKey, tc.baseURL)
			if err != nil {
				t.Fatalf("resolveChatModelRef: %v", err)
			}
			wantDriver := "ZHIPU-AI"
			if strings.Contains(tc.ref, "OTHER") {
				wantDriver = "OTHER-PROVIDER"
			}
			if model != "glm-4v" || driver != wantDriver || key != tc.wantKey || baseURL != tc.wantBase {
				t.Fatalf("got %q %q key=%q base=%q; want glm-4v %s key=%q base=%q", model, driver, key, baseURL, wantDriver, tc.wantKey, tc.wantBase)
			}
		})
	}
}
