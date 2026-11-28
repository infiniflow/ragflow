package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupChatPipelineVisionTestDB seeds an in-memory model-provider scope with
// a ZHIPU-AI provider, a default instance, and enrolled models:
//
//	glm-4-flash     — chat only (text-only: rejects image content with error 1210)
//	glm-4v          — chat + image2text in one tenant_model row
//	glm-4.6v-Flash  — combined chat|image2text bitmask in a single row
//
// Enrollment rows are what getLLMModelConfig probes to decide whether image
// attachments may be sent.
func setupChatPipelineVisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate model tables: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-zhipu", UserID: "user-1", TenantID: "tenant-1", ProviderName: "ZHIPU-AI"},
		&entity.TenantModelInstance{ID: "instance-zhipu", ProviderID: "provider-zhipu", InstanceName: "default", APIKey: "sk-test", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-flash-chat", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4-flash", ModelType: int(entity.ModelTypeChat), Status: "active"},
		&entity.TenantModel{ID: "model-4v", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4v", ModelType: int(entity.ModelTypeChat | entity.ModelTypeImage2Text), Status: "active"},
		// Combined chat+image2text enrollment (bitmask), as produced for
		// catalog entries with model_types ["chat", "vision"] (e.g. glm-4.6v-Flash).
		&entity.TenantModel{ID: "model-46v-combined", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4.6v-Flash", ModelType: int(entity.ModelTypeChat | entity.ModelTypeImage2Text), Status: "active"},
		&entity.TenantModel{ID: "model-vision-only", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-vision-only", ModelType: int(entity.ModelTypeImage2Text), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
	return db
}

func seedChatPipelineVisionTenant(t *testing.T, db *gorm.DB, defaultLLMID string) {
	t.Helper()
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		Status: &status,
	}).Error; err != nil {
		t.Fatalf("set tenant owner default chat model: %v", err)
	}
	if err := db.Model(&entity.UserTenant{}).Where("id = ?", "user-tenant-1").Update("llm_id", defaultLLMID).Error; err != nil {
		t.Fatalf("failed to configure owner default model: %v", err)
	}
}

// A text-only chat model (glm-4-flash, enrolled as chat only) must resolve
// to model_type "chat" so the pipeline drops image attachments instead of
// sending image content blocks the provider rejects (Zhipu error 1210).
func TestGetLLMModelConfig_TextOnlyModelResolvesChat(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4-flash@ZHIPU-AI")

	svc := NewChatPipelineService()
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4-flash@ZHIPU-AI",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "chat" {
		t.Fatalf("model_type = %v, want chat for text-only model", got)
	}
}

// A vision-capable model (glm-4v, enrolled as chat + image2text) must
// resolve to model_type "image2text" so image attachments still reach it.
func TestGetLLMModelConfig_VisionModelResolvesImage2Text(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4v@ZHIPU-AI")

	svc := NewChatPipelineService()
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4v@ZHIPU-AI",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "image2text" {
		t.Fatalf("model_type = %v, want image2text for vision-capable model", got)
	}
}

// A model enrolled as combined chat+image2text (bitmask 1|8, e.g.
// glm-4.6v-Flash from a ["chat", "vision"] catalog entry) must still resolve
// to "image2text" so its image inputs are kept, not silently dropped.
func TestGetLLMModelConfig_CombinedChatAndImage2TextResolvesImage2Text(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4.6v-Flash@ZHIPU-AI")

	svc := NewChatPipelineService()
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4.6v-Flash@ZHIPU-AI",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "image2text" {
		t.Fatalf("model_type = %v, want image2text for combined chat+image2text model", got)
	}
}

// Branch 3 (no explicit dialog LLM): a text-only tenant default resolves to
// "chat" so image attachments are dropped rather than rejected upstream.
func TestGetLLMModelConfig_TenantDefaultTextOnlyResolvesChat(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4-flash@ZHIPU-AI")

	svc := NewChatPipelineService()
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "chat" {
		t.Fatalf("model_type = %v, want chat for text-only tenant default", got)
	}
}

// Branch 3 (no explicit dialog LLM): a vision-capable tenant default must
// still dispatch as image2text, otherwise its image inputs would be lost.
func TestGetLLMModelConfig_TenantDefaultVisionResolvesImage2Text(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4v@ZHIPU-AI")

	svc := NewChatPipelineService()
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "image2text" {
		t.Fatalf("model_type = %v, want image2text for vision-capable tenant default", got)
	}
}

// A vision-capable model chosen as a standard chat dialog keeps
// product_model_type "chat" for billing/ledger accounting, while model_type
// remains "image2text" so image attachments are not dropped.
func TestGetLLMModelConfig_ProductTypeRetainsChatForVisionModel(t *testing.T) {
	db := setupChatPipelineVisionTestDB(t)
	seedChatPipelineVisionTenant(t, db, "glm-4v@ZHIPU-AI")

	svc := NewChatPipelineService()
	// Case 1: Standard chat without explicit VLM product type setting
	cfg, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4v@ZHIPU-AI",
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfg["model_type"]; got != "image2text" {
		t.Fatalf("model_type = %v, want image2text for vision capability", got)
	}
	if got := cfg["product_model_type"]; got != "chat" {
		t.Fatalf("product_model_type = %v, want chat for product accounting", got)
	}

	// Case 2: Multimodal model with real frontend shape []interface{}{"chat", "vision"}
	cfgWebUI, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID:   "tenant-1",
		LLMID:      "glm-4v@ZHIPU-AI",
		LLMSetting: map[string]interface{}{"model_type": []interface{}{"chat", "vision"}},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgWebUI["model_type"]; got != "image2text" {
		t.Fatalf("model_type = %v, want image2text for vision capability", got)
	}
	if got := cfgWebUI["product_model_type"]; got != "chat" {
		t.Fatalf("product_model_type = %v, want chat for multimodal chat selection", got)
	}

	// Case 3: Multimodal model with []string{"chat", "vision"}
	cfgStringSlice, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID:   "tenant-1",
		LLMID:      "glm-4v@ZHIPU-AI",
		LLMSetting: map[string]interface{}{"model_type": []string{"chat", "vision"}},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgStringSlice["product_model_type"]; got != "chat" {
		t.Fatalf("product_model_type = %v, want chat", got)
	}

	// Case 4: Vision-only model with real frontend shape []interface{}{"vision"}
	cfgVisionOnly, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID:   "tenant-1",
		LLMID:      "glm-vision-only@ZHIPU-AI",
		LLMSetting: map[string]interface{}{"model_type": []interface{}{"vision"}},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgVisionOnly["product_model_type"]; got != "image2text" {
		t.Fatalf("product_model_type = %v, want image2text for vision-only model", got)
	}

	// Case 5: Vision-only model with string "vision"
	cfgVisionStr, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID:   "tenant-1",
		LLMID:      "glm-vision-only@ZHIPU-AI",
		LLMSetting: map[string]interface{}{"model_type": "vision"},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgVisionStr["product_model_type"]; got != "image2text" {
		t.Fatalf("product_model_type = %v, want image2text for vision string", got)
	}

	// Case 6: Chat explicitly configured as image2text product
	cfgExplicit, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID:   "tenant-1",
		LLMID:      "glm-4v@ZHIPU-AI",
		LLMSetting: map[string]interface{}{"model_type": "image2text"},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgExplicit["product_model_type"]; got != "image2text" {
		t.Fatalf("product_model_type = %v, want image2text when explicitly set", got)
	}

	// Case 7: Explicit product_model_type: "image2text" overrides chat capability
	cfgExplicitVLM, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4v@ZHIPU-AI",
		LLMSetting: map[string]interface{}{
			"product_model_type": "image2text",
			"model_type":         []interface{}{"chat", "vision"},
		},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgExplicitVLM["product_model_type"]; got != "image2text" {
		t.Fatalf("product_model_type = %v, want image2text when product_model_type explicitly set", got)
	}

	// Case 8: Explicit product_model_type: "chat" overrides vision-only model_type
	cfgExplicitChat, _, _, _, err := svc.getLLMModelConfig(t.Context(), &entity.Chat{
		TenantID: "tenant-1",
		LLMID:    "glm-4v@ZHIPU-AI",
		LLMSetting: map[string]interface{}{
			"product_model_type": "chat",
			"model_type":         []interface{}{"vision"},
		},
	})
	if err != nil {
		t.Fatalf("getLLMModelConfig() error = %v", err)
	}
	if got := cfgExplicitChat["product_model_type"]; got != "chat" {
		t.Fatalf("product_model_type = %v, want chat when product_model_type explicitly set", got)
	}
}
