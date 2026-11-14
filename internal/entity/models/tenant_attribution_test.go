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

package models

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

type attributionCapturingDriver struct {
	ModelDriver
	capturedUsages []*common.ModelUsage
	tokensPerText  int
	failChat       bool
	failStream     bool
}

func (d *attributionCapturingDriver) ChatWithMessages(
	ctx context.Context,
	_ string,
	_ []Message,
	_ *APIConfig,
	_ *ChatConfig,
	usage *common.ModelUsage,
) (*ChatResponse, error) {
	d.capturedUsages = append(d.capturedUsages, usage)
	if d.failChat {
		return nil, errors.New("simulated chat error")
	}
	ans := "ok"
	respUsage := &TokenUsage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15}
	recordResponseUsageWithContext(ctx, usage, "req-stub", respUsage, "chat")
	return &ChatResponse{
		Answer: &ans,
		Usage:  respUsage,
	}, nil
}

func (d *attributionCapturingDriver) ChatStreamlyWithSender(
	_ context.Context,
	_ string,
	_ []Message,
	_ *APIConfig,
	_ *ChatConfig,
	usage *common.ModelUsage,
	sender func(*string, *string) error,
) error {
	d.capturedUsages = append(d.capturedUsages, usage)
	if d.failStream {
		return errors.New("simulated stream error")
	}
	if sender != nil {
		ans := "ok"
		_ = sender(&ans, nil)
	}
	return nil
}

func (d *attributionCapturingDriver) Embed(
	_ context.Context,
	_ *string,
	request EmbedRequest,
	_ *APIConfig,
	_ *EmbeddingConfig,
	usage *common.ModelUsage,
) ([]EmbeddingData, error) {
	d.capturedUsages = append(d.capturedUsages, usage)
	tCount := 10
	if d.tokensPerText > 0 {
		tCount = d.tokensPerText
	}
	if usage != nil {
		usage.InputTokens += len(request.Texts) * tCount
		usage.TotalTokens += len(request.Texts) * tCount
	}
	result := make([]EmbeddingData, len(request.Texts))
	for i := range result {
		result[i] = EmbeddingData{
			Embedding:  []float64{0.1, 0.2},
			Index:      i,
			TokenCount: tCount,
		}
	}
	return result, nil
}

func (d *attributionCapturingDriver) Rerank(
	_ context.Context,
	_ *string,
	_ RerankRequest,
	_ *APIConfig,
	_ *RerankConfig,
	usage *common.ModelUsage,
) (*RerankResponse, error) {
	d.capturedUsages = append(d.capturedUsages, usage)
	return &RerankResponse{
		Data: []RerankResult{{Index: 0, RelevanceScore: 0.9}},
	}, nil
}

func TestChatModelAttributionFromContext(t *testing.T) {
	driver := &attributionCapturingDriver{}
	apiKey := "test-key"
	apiCfg := &APIConfig{ApiKey: &apiKey}
	modelName := "gpt-4o"
	info := &ModelInfo{
		Name:          modelName,
		ProviderName:  "openai",
		InstanceID:    "inst-1",
		TenantID:      "tenant-456",
		IsSavedConfig: true,
	}
	cm := NewChatModelWithInfo(driver, &modelName, apiCfg, info)

	ctx := permission.WithIdentity(context.Background(), "user-123", "tenant-456")

	// Call with nil modelUsage: wrapper must resolve identity from ctx and info
	_, err := cm.ChatWithMessages(ctx, []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages error: %v", err)
	}

	if len(driver.capturedUsages) != 1 {
		t.Fatalf("captured %d usages, want 1", len(driver.capturedUsages))
	}
	u := driver.capturedUsages[0]
	if u == nil {
		t.Fatal("driver received nil ModelUsage")
	}
	if u.TenantID != "tenant-456" {
		t.Errorf("got TenantID %q, want %q", u.TenantID, "tenant-456")
	}
	if u.UserID != "user-123" {
		t.Errorf("got UserID %q, want %q", u.UserID, "user-123")
	}
	if u.ModelName != "gpt-4o" {
		t.Errorf("got ModelName %q, want %q", u.ModelName, "gpt-4o")
	}
	if u.ProviderName != "openai" {
		t.Errorf("got ProviderName %q, want %q", u.ProviderName, "openai")
	}
	if u.InstanceID != "inst-1" {
		t.Errorf("got InstanceID %q, want %q", u.InstanceID, "inst-1")
	}
	if u.APIKey != "test-key" {
		t.Errorf("got APIKey %q, want %q", u.APIKey, "test-key")
	}
	if u.Type != "chat" {
		t.Errorf("got Type %q, want %q", u.Type, "chat")
	}
	if u.StartAt.IsZero() {
		t.Error("expected non-zero StartAt")
	}
}

func TestChatModelAttributionNoCallerLeavesUserIDEmpty(t *testing.T) {
	driver := &attributionCapturingDriver{}
	modelName := "claude-3-5-sonnet"
	info := &ModelInfo{
		Name:          modelName,
		ProviderName:  "anthropic",
		InstanceID:    "inst-saved-1",
		TenantID:      "saved-tenant",
		UserID:        "provider-owner-not-caller",
		IsSavedConfig: true,
	}
	cm := NewChatModelWithInfo(driver, &modelName, nil, info)

	// Context without caller identity: wrapper must NOT fall back to provider/owner UserID
	_, err := cm.ChatWithMessages(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages error: %v", err)
	}

	if len(driver.capturedUsages) != 1 || driver.capturedUsages[0] == nil {
		t.Fatalf("invalid captured usages: %v", driver.capturedUsages)
	}
	u := driver.capturedUsages[0]
	if u.TenantID != "saved-tenant" {
		t.Errorf("got TenantID %q, want %q", u.TenantID, "saved-tenant")
	}
	if u.UserID != "" {
		t.Errorf("got UserID %q, want empty string when no clear caller exists (must not use provider user_id)", u.UserID)
	}
	if u.ProviderName != "anthropic" {
		t.Errorf("got ProviderName %q, want %q", u.ProviderName, "anthropic")
	}
	if u.InstanceID != "inst-saved-1" {
		t.Errorf("got InstanceID %q, want %q", u.InstanceID, "inst-saved-1")
	}
}

func TestChatStreamlyWithSenderAttribution(t *testing.T) {
	driver := &attributionCapturingDriver{}
	modelName := "deepseek-chat"
	info := &ModelInfo{
		Name:         modelName,
		ProviderName: "deepseek",
	}
	cm := NewChatModelWithInfo(driver, &modelName, nil, info)

	ctx := permission.WithIdentity(context.Background(), "u-stream", "t-stream")
	err := cm.ChatStreamlyWithSender(ctx, []Message{{Role: "user", Content: "stream"}}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender error: %v", err)
	}

	if len(driver.capturedUsages) != 1 || driver.capturedUsages[0] == nil {
		t.Fatalf("invalid captured usages: %v", driver.capturedUsages)
	}
	u := driver.capturedUsages[0]
	if u.TenantID != "t-stream" {
		t.Errorf("got TenantID %q, want %q", u.TenantID, "t-stream")
	}
	if u.UserID != "u-stream" {
		t.Errorf("got UserID %q, want %q", u.UserID, "u-stream")
	}
	if u.Type != "chat" {
		t.Errorf("got Type %q, want chat", u.Type)
	}
}

func TestEmbeddingModelAttributionAndAggregation(t *testing.T) {
	driver := &attributionCapturingDriver{tokensPerText: 7}
	modelName := "text-embedding-3-small"
	batchSize := 2
	info := &ModelInfo{
		Name:         modelName,
		ProviderName: "openai",
		Catalog:      &Model{MaxBatchSize: &batchSize},
	}
	em := NewEmbeddingModelWithInfo(driver, &modelName, nil, 8192, info)

	ctx := permission.WithTenantID(context.Background(), "t-embed")

	// Passing 5 texts with batch size 2 -> 3 batches (2, 2, 1)
	texts := []string{"t1", "t2", "t3", "t4", "t5"}
	usage := &common.ModelUsage{}

	_, err := em.Embed(ctx, EmbedRequest{Texts: texts}, nil, usage)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}

	if len(driver.capturedUsages) != 3 {
		t.Fatalf("got %d driver batch calls, want 3", len(driver.capturedUsages))
	}
	for i, batchU := range driver.capturedUsages {
		if batchU.TenantID != "t-embed" {
			t.Errorf("batch %d: got TenantID %q, want %q", i, batchU.TenantID, "t-embed")
		}
		if batchU.Type != "embedding" {
			t.Errorf("batch %d: got Type %q, want embedding", i, batchU.Type)
		}
	}

	// Verify aggregated usage on caller's pointer
	if usage.TenantID != "t-embed" {
		t.Errorf("caller usage TenantID=%q, want %q", usage.TenantID, "t-embed")
	}
	wantTotal := 5 * 7
	if usage.TotalTokens != wantTotal {
		t.Errorf("caller usage TotalTokens=%d, want %d", usage.TotalTokens, wantTotal)
	}
	if usage.InputTokens != wantTotal {
		t.Errorf("caller usage InputTokens=%d, want %d", usage.InputTokens, wantTotal)
	}
}

func TestRerankModelAttribution(t *testing.T) {
	driver := &attributionCapturingDriver{}
	modelName := "bge-reranker-large"
	info := &ModelInfo{
		Name:         modelName,
		ProviderName: "baai",
	}
	rm := NewRerankModelWithInfo(driver, &modelName, nil, 1024, info)

	ctx := permission.WithTenantID(context.Background(), "t-rerank")
	_, err := rm.Rerank(ctx, RerankRequest{Query: "q", Documents: []string{"d1", "d2"}}, nil, nil)
	if err != nil {
		t.Fatalf("Rerank error: %v", err)
	}

	if len(driver.capturedUsages) != 1 || driver.capturedUsages[0] == nil {
		t.Fatalf("invalid captured usages: %v", driver.capturedUsages)
	}
	u := driver.capturedUsages[0]
	if u.TenantID != "t-rerank" {
		t.Errorf("got TenantID %q, want %q", u.TenantID, "t-rerank")
	}
	if u.Type != "rerank" {
		t.Errorf("got Type %q, want rerank", u.Type)
	}
}

func TestRecordResponseUsageWithContextIdentity(t *testing.T) {
	ctx := permission.WithIdentity(context.Background(), "user-ctx", "tenant-ctx")
	usage := &common.ModelUsage{StartAt: time.Now().Add(-50 * time.Millisecond)}

	tokenUsage := &TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	recordResponseUsageWithContext(ctx, usage, "req-123", tokenUsage, "chat")

	if usage.TenantID != "tenant-ctx" {
		t.Errorf("TenantID=%q, want tenant-ctx", usage.TenantID)
	}
	if usage.UserID != "user-ctx" {
		t.Errorf("UserID=%q, want user-ctx", usage.UserID)
	}
	if usage.RequestID != "req-123" {
		t.Errorf("RequestID=%q, want req-123", usage.RequestID)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 50 || usage.TotalTokens != 150 {
		t.Errorf("tokens mismatch: %#v", usage)
	}
	if usage.ResponseTimeMS <= 0 {
		t.Errorf("ResponseTimeMS=%d, want > 0", usage.ResponseTimeMS)
	}
}

func TestApplyStreamUsageWithContextIdentity(t *testing.T) {
	ctx := permission.WithIdentity(context.Background(), "user-stream", "tenant-stream")
	cfg := &ChatConfig{}
	usage := &common.ModelUsage{StartAt: time.Now().Add(-20 * time.Millisecond)}

	tokenUsage := &TokenUsage{
		PromptTokens:     40,
		CompletionTokens: 60,
		TotalTokens:      100,
	}

	applyStreamUsageWithContext(ctx, cfg, usage, tokenUsage)

	if usage.TenantID != "tenant-stream" {
		t.Errorf("TenantID=%q, want tenant-stream", usage.TenantID)
	}
	if usage.UserID != "user-stream" {
		t.Errorf("UserID=%q, want user-stream", usage.UserID)
	}
	if cfg.UsageResult == nil || cfg.UsageResult.TotalTokens != 100 {
		t.Errorf("cfg.UsageResult=%#v, want total 100", cfg.UsageResult)
	}
	if usage.InputTokens != 40 || usage.OutputTokens != 60 || usage.TotalTokens != 100 {
		t.Errorf("tokens mismatch: %#v", usage)
	}
}

func TestCrossTenantAttribution(t *testing.T) {
	t.Run("embedding", func(t *testing.T) {
		driver := &attributionCapturingDriver{}
		modelName := "text-embedding-3-small"
		info := &ModelInfo{
			Name:          modelName,
			ProviderName:  "openai",
			InstanceID:    "inst-owner-b",
			TenantID:      "tenant-owner-B",
			UserID:        "user-owner-B",
			IsSavedConfig: true,
		}
		em := NewEmbeddingModelWithInfo(driver, &modelName, nil, 8192, info)

		// Caller A calls model owned by tenant B
		ctx := permission.WithIdentity(context.Background(), "user-caller-A", "tenant-caller-A")
		usage := &common.ModelUsage{}
		_, err := em.Embed(ctx, EmbedRequest{Texts: []string{"hello"}}, nil, usage)
		if err != nil {
			t.Fatalf("Embed error: %v", err)
		}

		if usage.TenantID != "tenant-owner-B" {
			t.Errorf("got TenantID %q, want tenant-owner-B (credential owner)", usage.TenantID)
		}
		if usage.UserID != "user-caller-A" {
			t.Errorf("got UserID %q, want user-caller-A (request caller)", usage.UserID)
		}
		if usage.InstanceID != "inst-owner-b" {
			t.Errorf("got InstanceID %q, want inst-owner-b", usage.InstanceID)
		}
		if usage.ProviderName != "openai" {
			t.Errorf("got ProviderName %q, want openai", usage.ProviderName)
		}
	})

	t.Run("chat", func(t *testing.T) {
		driver := &attributionCapturingDriver{}
		modelName := "claude-3-5-sonnet"
		info := &ModelInfo{
			Name:          modelName,
			ProviderName:  "anthropic",
			InstanceID:    "inst-b",
			TenantID:      "tenant-B",
			UserID:        "provider-owner-B",
			IsSavedConfig: true,
		}
		cm := NewChatModelWithInfo(driver, &modelName, nil, info)
		ctx := permission.WithIdentity(context.Background(), "user-caller-A", "tenant-caller-A")

		_, err := cm.ChatWithMessages(ctx, []Message{{Role: "user", Content: "hello"}}, nil, nil)
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if len(driver.capturedUsages) == 0 {
			t.Fatal("no usage captured")
		}
		u := driver.capturedUsages[len(driver.capturedUsages)-1]
		if u.TenantID != "tenant-B" {
			t.Errorf("TenantID=%q, want tenant-B (credential owner)", u.TenantID)
		}
		if u.UserID != "user-caller-A" {
			t.Errorf("UserID=%q, want user-caller-A (actual initiator)", u.UserID)
		}
		if u.InstanceID != "inst-b" {
			t.Errorf("InstanceID=%q, want inst-b", u.InstanceID)
		}
	})
}

func TestVLMProductTypeAttribution(t *testing.T) {
	driver := &attributionCapturingDriver{}
	modelName := "qwen-vl-max"

	// Case 1: Model configured with ProductType "image2text"
	infoVLM := &ModelInfo{
		Name:         modelName,
		ProviderName: "aliyun",
		ProductType:  "image2text",
		ModelTypes:   []string{"chat", "image2text"},
	}
	cmVLM := NewChatModelWithInfo(driver, &modelName, nil, infoVLM)
	ctx := permission.WithIdentity(context.Background(), "user-1", "tenant-1")

	_, err := cmVLM.ChatWithMessages(ctx, []Message{{Role: "user", Content: "describe this image"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages error: %v", err)
	}
	if len(driver.capturedUsages) < 1 {
		t.Fatal("no usage captured")
	}
	if got := driver.capturedUsages[len(driver.capturedUsages)-1].Type; got != "image2text" {
		t.Errorf("got usage Type %q, want image2text", got)
	}

	// Case 2: Multimodal model configured as chat preserves "chat"
	infoChat := &ModelInfo{
		Name:         "gpt-4o",
		ProviderName: "openai",
		ProductType:  "chat",
		ModelTypes:   []string{"chat", "image2text"},
	}
	cmChat := NewChatModelWithInfo(driver, &infoChat.Name, nil, infoChat)
	_, err = cmChat.ChatWithMessages(ctx, []Message{{Role: "user", Content: "hello"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages error: %v", err)
	}
	if got := driver.capturedUsages[len(driver.capturedUsages)-1].Type; got != "chat" {
		t.Errorf("got usage Type %q, want chat", got)
	}

	// Case 3: SetProductType dynamically updates classification on VLM fallback to LLM
	cmDynamic := NewChatModelWithInfo(driver, &modelName, nil, &ModelInfo{Name: modelName})
	cmDynamic.SetProductType("chat")
	_, err = cmDynamic.ChatWithMessages(ctx, []Message{{Role: "user", Content: "look"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages error: %v", err)
	}
	if got := driver.capturedUsages[len(driver.capturedUsages)-1].Type; got != "chat" {
		t.Errorf("got usage Type %q, want chat after fallback", got)
	}
}

// TestSavedConfigMissingAttributionDropped verifies that when required attribution
// (tenant_id, provider_name, instance_id) is missing for saved configs, the recording point
// drops the record, logs a server error, increments the dropped counter, and leaves the model call uninterrupted.
func TestSavedConfigMissingAttributionDropped(t *testing.T) {
	tests := []struct {
		name        string
		usage       *common.ModelUsage
		info        *ModelInfo
		checkCall   bool
		wantTenant  string
		checkTenant bool
	}{
		{
			name: "saved config missing tenant_id drops and succeeds call",
			info: &ModelInfo{
				Name:          "deepseek-chat",
				ProviderName:  "deepseek",
				InstanceID:    "inst-missing-tenant",
				TenantID:      "",
				IsSavedConfig: true,
			},
			checkCall: true,
		},
		{
			name: "pre-filled caller tenant cleared when config tenant missing",
			usage: &common.ModelUsage{
				TenantID: "tenant-caller-A",
			},
			info: &ModelInfo{
				Name:          "deepseek-chat",
				ProviderName:  "deepseek",
				InstanceID:    "inst-missing-tenant",
				TenantID:      "",
				IsSavedConfig: true,
			},
			checkCall:  true,
			wantTenant: "",
		},
		{
			name: "missing instance_id and tenant_id drops",
			info: &ModelInfo{
				Name:          "deepseek-chat",
				ProviderName:  "deepseek",
				InstanceID:    "",
				TenantID:      "",
				IsSavedConfig: true,
			},
			checkCall: true,
		},
		{
			name: "direct record missing provider_name drops",
			usage: &common.ModelUsage{
				IsSavedConfig: true,
				InstanceID:    "inst-1",
				TenantID:      "tenant-1",
				ModelName:     "gpt-4o",
				ProviderName:  "",
			},
		},
		{
			name: "direct record missing instance_id drops",
			usage: &common.ModelUsage{
				IsSavedConfig: true,
				InstanceID:    "",
				TenantID:      "tenant-1",
				ModelName:     "gpt-4o",
				ProviderName:  "openai",
			},
		},
		{
			name: "saved config with info nil clears unverified caller tenant",
			usage: &common.ModelUsage{
				TenantID:      "tenant-caller-A",
				ProviderName:  "openai",
				InstanceID:    "inst-1",
				IsSavedConfig: true,
			},
			wantTenant:  "",
			checkTenant: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetDroppedMissingAttributionCount()
			initialDropped := DroppedMissingAttributionCount()

			ctx := permission.WithIdentity(context.Background(), "user-caller-A", "tenant-caller-A")

			if tc.checkCall {
				driver := &attributionCapturingDriver{}
				cm := NewChatModelWithInfo(driver, &tc.info.Name, nil, tc.info)
				resp, err := cm.ChatWithMessages(ctx, []Message{{Role: "user", Content: "hello"}}, nil, tc.usage)
				if err != nil {
					t.Fatalf("call failed: %v", err)
				}
				if resp == nil || resp.Answer == nil || *resp.Answer != "ok" {
					t.Fatalf("unexpected response: %v", resp)
				}
				if tc.usage != nil && tc.usage.TenantID != tc.wantTenant {
					t.Errorf("got usage TenantID %q, want %q", tc.usage.TenantID, tc.wantTenant)
				}
			} else {
				resolved := resolveModelUsage(ctx, tc.usage, nil, nil, tc.info, "chat")
				if tc.checkTenant && resolved.TenantID != tc.wantTenant {
					t.Errorf("got TenantID=%q, want %q", resolved.TenantID, tc.wantTenant)
				}
				recordResponseUsageWithContext(ctx, resolved, "req-direct-drop", &TokenUsage{TotalTokens: 10}, "chat")
			}

			if got := DroppedMissingAttributionCount(); got <= initialDropped {
				t.Errorf("DroppedMissingAttributionCount=%d, want > %d", got, initialDropped)
			}
		})
	}
}

func TestBackgroundTasksAttribution(t *testing.T) {
	driver := &attributionCapturingDriver{}
	modelName := "gpt-4o"
	info := &ModelInfo{
		Name:          modelName,
		ProviderName:  "openai",
		InstanceID:    "inst-bg",
		TenantID:      "tenant-bg",
		UserID:        "provider-owner",
		IsSavedConfig: true,
	}
	cm := NewChatModelWithInfo(driver, &modelName, nil, info)

	// Initiated by user -> UserID passed
	ctxWithUser := permission.WithIdentity(context.Background(), "initiating-user-42", "tenant-bg")
	_, err := cm.ChatWithMessages(ctxWithUser, []Message{{Role: "user", Content: "hello"}}, nil, nil)
	if err != nil {
		t.Fatalf("task call failed: %v", err)
	}
	u := driver.capturedUsages[len(driver.capturedUsages)-1]
	if u.UserID != "initiating-user-42" {
		t.Errorf("UserID=%q, want initiating-user-42", u.UserID)
	}

	// System background task without user initiator -> UserID left empty
	ctxNoUser := context.Background()
	_, err = cm.ChatWithMessages(ctxNoUser, []Message{{Role: "user", Content: "hello"}}, nil, nil)
	if err != nil {
		t.Fatalf("system task call failed: %v", err)
	}
	uNoUser := driver.capturedUsages[len(driver.capturedUsages)-1]
	if uNoUser.UserID != "" {
		t.Errorf("UserID=%q, want empty string (must NOT fall back to provider user_id %q)", uNoUser.UserID, info.UserID)
	}
}

func TestDirectCredentialsNotTreatedAsSavedConfig(t *testing.T) {
	resetDroppedMissingAttributionCount()
	initialDropped := DroppedMissingAttributionCount()

	driver := &attributionCapturingDriver{}
	modelName := "custom-model"
	key := "direct-api-key"
	info := &ModelInfo{
		Name:          modelName,
		ProviderName:  "openai",
		IsSavedConfig: false,
		InstanceID:    "",
		TenantID:      "",
	}
	cm := NewChatModelWithInfo(driver, &modelName, &APIConfig{ApiKey: &key}, info)
	ctx := permission.WithIdentity(context.Background(), "user-direct", "tenant-direct")

	_, err := cm.ChatWithMessages(ctx, []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	u := driver.capturedUsages[len(driver.capturedUsages)-1]
	if u.TenantID != "tenant-direct" {
		t.Errorf("TenantID=%q, want tenant-direct from context for unsaved/direct call", u.TenantID)
	}
	if u.UserID != "user-direct" {
		t.Errorf("UserID=%q, want user-direct", u.UserID)
	}
	if got := DroppedMissingAttributionCount(); got != initialDropped {
		t.Errorf("DroppedMissingAttributionCount=%d, want unchanged %d", got, initialDropped)
	}
}

func TestFailoverDoesNotLeakModelInfo(t *testing.T) {
	nameA, nameB := "model-a", "model-b"
	infoA := &ModelInfo{
		Name:          nameA,
		ProviderName:  "provider-A",
		InstanceID:    "inst-A",
		TenantID:      "tenant-A",
		IsSavedConfig: true,
	}

	t.Run("stream", func(t *testing.T) {
		driverA := &attributionCapturingDriver{failStream: true}
		driverB := &attributionCapturingDriver{failStream: false}
		cmA := NewChatModelWithInfo(driverA, &nameA, nil, infoA)
		cmB := NewChatModelWithInfo(driverB, &nameB, nil, nil)

		failover, err := NewFailoverEinoChatModel([]*ChatModel{cmA, cmB}, nil)
		if err != nil {
			t.Fatalf("NewFailoverEinoChatModel: %v", err)
		}

		ctx := permission.WithIdentity(context.Background(), "user-caller", "tenant-caller")
		sr, err := failover.Stream(ctx, []*schema.Message{schema.UserMessage("hi")})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		for {
			_, recvErr := sr.Recv()
			if recvErr != nil {
				if errors.Is(recvErr, io.EOF) {
					break
				}
				t.Fatalf("Recv: %v", recvErr)
			}
		}

		if len(driverB.capturedUsages) == 0 {
			t.Fatal("driverB did not receive stream call")
		}
		uB := driverB.capturedUsages[len(driverB.capturedUsages)-1]
		if uB.TenantID == "tenant-A" || uB.InstanceID == "inst-A" || uB.ProviderName == "provider-A" {
			t.Errorf("driverB incorrectly inherited attribution from failed attempt: %+v", uB)
		}
		if uB.TenantID != "tenant-caller" {
			t.Errorf("driverB got TenantID=%q, want tenant-caller", uB.TenantID)
		}
	})

	t.Run("generate", func(t *testing.T) {
		driverA := &attributionCapturingDriver{failChat: true}
		driverB := &attributionCapturingDriver{failChat: false}
		cmA := NewChatModelWithInfo(driverA, &nameA, nil, infoA)
		cmB := NewChatModelWithInfo(driverB, &nameB, nil, nil)

		failover, err := NewFailoverEinoChatModel([]*ChatModel{cmA, cmB}, nil)
		if err != nil {
			t.Fatalf("NewFailoverEinoChatModel: %v", err)
		}

		ctx := permission.WithIdentity(context.Background(), "user-caller", "tenant-caller")
		_, err = failover.Generate(ctx, []*schema.Message{schema.UserMessage("hi")})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}

		if len(driverB.capturedUsages) == 0 {
			t.Fatal("driverB did not receive chat call")
		}
		uB := driverB.capturedUsages[len(driverB.capturedUsages)-1]
		if uB.TenantID == "tenant-A" || uB.InstanceID == "inst-A" || uB.ProviderName == "provider-A" {
			t.Errorf("driverB incorrectly inherited attribution from failed attempt: %+v", uB)
		}
		if uB.TenantID != "tenant-caller" {
			t.Errorf("driverB got TenantID=%q, want tenant-caller", uB.TenantID)
		}
	})
}

func TestResolveModelUsage_ContextIsolation(t *testing.T) {
	parentInfo := &ModelInfo{
		TenantID:      "parent-tenant",
		ProviderName:  "parent-provider",
		InstanceID:    "parent-instance",
		Name:          "parent-model",
		IsSavedConfig: true,
	}
	ctx := WithModelInfo(context.Background(), parentInfo)
	ctx = permission.WithIdentity(ctx, "caller-user", "caller-tenant")

	t.Run("explicit credentials do not adopt saved context info", func(t *testing.T) {
		apiKey := "explicit-key"
		cfg := &APIConfig{ApiKey: &apiKey}
		modelName := "parent-model"
		usage := resolveModelUsage(ctx, nil, &modelName, cfg, nil, "chat")
		if usage.TenantID == parentInfo.TenantID || usage.InstanceID == parentInfo.InstanceID {
			t.Errorf("expected explicit key to avoid parent attribution, got tenant=%q, inst=%q", usage.TenantID, usage.InstanceID)
		}
		if usage.TenantID != "caller-tenant" {
			t.Errorf("expected caller tenant for explicit key, got %q", usage.TenantID)
		}
	})

	t.Run("model name mismatch does not adopt context info", func(t *testing.T) {
		diffModel := "child-model"
		usage := resolveModelUsage(ctx, nil, &diffModel, nil, nil, "chat")
		if usage.TenantID == parentInfo.TenantID || usage.InstanceID == parentInfo.InstanceID {
			t.Errorf("expected different model name to avoid parent attribution, got tenant=%q, inst=%q", usage.TenantID, usage.InstanceID)
		}
	})

	t.Run("clearing context info with nil", func(t *testing.T) {
		clearedCtx := WithModelInfo(ctx, nil)
		if got := ModelInfoFromContext(clearedCtx); got != nil {
			t.Errorf("expected nil ModelInfoFromContext after WithModelInfo(nil), got %+v", got)
		}
	})
}
