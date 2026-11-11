//go:build manual

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

package service

// Live round trip for the Future AGI tenant export, run against a real
// collector and a real OpenAI-compatible model:
//
//	FUTUREAGI_LIVE=1 FI_BASE_URL=http://localhost:4318 FI_API_KEY=... FI_SECRET_KEY=... \
//	FUTUREAGI_LIVE_PROJECT=ragflow-live LIVE_LLM_BASE_URL=https://api.openai.com/v1 \
//	LIVE_LLM_API_KEY=... LIVE_LLM_MODEL=gpt-4o-mini [TAVILY_API_KEY=...] \
//	bash build.sh --test-manual -run TestFutureAGILive -v ./internal/service
//
// It drives RAGFlow's own services in process (model provider, chat
// assistant, chat session completion, the OpenAI-compatible endpoint) on an
// in-memory SQLite database instead of MySQL, with no document engine. The
// chat retrieves through the dialog's web search (Tavily) when
// TAVILY_API_KEY is set, so knowledge-base retrieval is not exercised here.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

func liveEnv(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Fatalf("%s is required for the live test", key)
	}
	return v
}

func setupLiveRAGFlow(t *testing.T) {
	t.Helper()
	if common.Logger == nil {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
		if l, err := cfg.Build(); err == nil {
			common.Logger = l
		}
	}
	db, err := gorm.Open(sqlite.Open("file:futureagi_live?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.TenantLLM{},
		&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{},
		&entity.APIToken{}, &entity.Knowledgebase{}, &entity.Document{}, &entity.LLMFactories{}, &entity.LLM{},
		&entity.TenantLangfuse{}, &entity.TenantFutureAGI{}, &entity.SystemSettings{},
		&entity.TenantModelInstance{}, &entity.TenantModel{}, &entity.TenantModelProvider{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	_, file, _, _ := runtime.Caller(0)
	if modelModule.GetProviderManager() == nil {
		if err := modelModule.InitProviderManager(filepath.Join(filepath.Dir(file), "..", "..", "conf", "models")); err != nil {
			t.Fatalf("model providers: %v", err)
		}
	}
}

func TestFutureAGILive(t *testing.T) {
	if os.Getenv("FUTUREAGI_LIVE") != "1" {
		t.Skip("set FUTUREAGI_LIVE=1 to run against a real collector and model")
	}
	setupLiveRAGFlow(t)
	ctx := permission.WithIdentity(context.Background(), "f0a6c0de0000000000000000000000aa", "f0a6c0de0000000000000000000000aa")
	marker := os.Getenv("FUTUREAGI_LIVE_MARKER")
	tenantID := "f0a6c0de0000000000000000000000aa"
	db := dao.DB
	db.Create(&entity.User{ID: tenantID, Nickname: "live", Email: "live@ragflow.invalid"})
	db.Create(&entity.Tenant{ID: tenantID, Name: sptr("live")})
	db.Create(&entity.UserTenant{ID: tenantID + "-own", UserID: tenantID, TenantID: tenantID, Role: "owner", Status: sptr("1")})

	// 1. Model provider + instances, the way the model settings page adds them.
	mps := NewModelProviderService()
	if _, err := mps.AddModelProvider(ctx, "OpenAI-API-Compatible"); err != nil {
		t.Fatalf("add provider: %v", err)
	}
	model := liveEnv(t, "LIVE_LLM_MODEL")
	info := []CreateInstanceModelInfo{{ModelName: model, ModelTypes: []string{"chat"}, MaxTokens: 8192}}
	if _, err := mps.CreateProviderInstance(ctx, "OpenAI-API-Compatible", "live", liveEnv(t, "LIVE_LLM_API_KEY"), liveEnv(t, "LIVE_LLM_BASE_URL"), "", info); err != nil {
		t.Fatalf("instance: %v", err)
	}
	if _, err := mps.CreateProviderInstance(ctx, "OpenAI-API-Compatible", "broken", "not-a-valid-key", liveEnv(t, "LIVE_LLM_BASE_URL"), "", info); err != nil {
		t.Fatalf("broken instance: %v", err)
	}

	// 2. Future AGI settings through the settings service (verifies the keys).
	fi := NewFutureAGIService()
	if _, _, err := fi.SetAPIKey(ctx, FutureAGISettings{APIKey: "bad", SecretKey: "bad", ProjectName: "x", Endpoint: liveEnv(t, "FI_BASE_URL")}); err == nil {
		t.Fatal("bad Future AGI keys must fail the save")
	} else {
		t.Logf("bad keys rejected: %v", err)
	}
	saved, _, err := fi.SetAPIKey(ctx, FutureAGISettings{
		APIKey:      liveEnv(t, "FI_API_KEY"),
		SecretKey:   liveEnv(t, "FI_SECRET_KEY"),
		ProjectName: liveEnv(t, "FUTUREAGI_LIVE_PROJECT"),
		Endpoint:    liveEnv(t, "FI_BASE_URL"),
		Redact:      os.Getenv("FUTUREAGI_LIVE_REDACT") == "true",
	})
	if err != nil {
		t.Fatalf("save Future AGI keys: %v", err)
	}
	t.Logf("saved settings: project=%s endpoint=%s redact=%v keys=%s/%s", saved.ProjectName, saved.Endpoint, saved.Redact, saved.APIKey, saved.SecretKey)
	if os.Getenv("FUTUREAGI_LIVE_OUTAGE") == "1" {
		// Simulate a collector outage after a good save: point the stored
		// row at a closed port. Chats must answer as usual.
		dao.DB.Model(&entity.TenantFutureAGI{}).Where("tenant_id = ?", tenantID).Update("endpoint", "http://127.0.0.1:9")
		t.Log("collector outage simulated: export endpoint now refuses connections")
	}

	// 3. Chat assistants: one that searches the web, one with a broken model.
	promptConfig := map[string]interface{}{
		"system":     "You are a concise assistant. Answer in two sentences. Use the knowledge below when it helps.\n{knowledge}",
		"parameters": []interface{}{map[string]interface{}{"key": "knowledge", "optional": true}},
		"quote":      false,
	}
	if key := os.Getenv("TAVILY_API_KEY"); key != "" {
		promptConfig["tavily_api_key"] = key
	}
	chats := NewChatService()
	mk := func(name, instance string) string {
		res, _, err := chats.Create(ctx, map[string]interface{}{
			"name":          name,
			"llm_id":        model + "@" + instance + "@OpenAI-API-Compatible",
			"prompt_config": promptConfig,
		})
		if err != nil {
			t.Fatalf("create chat %s: %v", name, err)
		}
		return res["id"].(string)
	}
	webChat := mk("live web chat", "live")
	brokenChat := mk("live broken chat", "broken")

	sessions := NewChatSessionService()
	runSession := func(chatID, question string, stream bool, kwargs map[string]interface{}) string {
		var streamChan chan string
		done := make(chan struct{})
		var lines []string
		if stream {
			streamChan = make(chan string, 64)
			go func() {
				defer close(done)
				for line := range streamChan {
					lines = append(lines, line)
				}
			}()
		}
		res, err := sessions.ChatCompletions(ctx, chatID, "", nil, question, nil, "", nil, kwargs, false, stream, streamChan)
		if stream {
			close(streamChan)
			<-done
		}
		if err != nil {
			return "error: " + err.Error()
		}
		if stream {
			return strings.Join(lines, "")
		}
		b, _ := json.Marshal(res)
		return string(b)
	}

	q := "What is RAGFlow, the open-source RAG engine? " + marker
	began := time.Now()
	out := runSession(webChat, q, true, map[string]interface{}{"internet": true})
	t.Logf("session (stream, web search) answer bytes=%d contains_error=%v took=%s", len(out), strings.Contains(out, "**ERROR**"), time.Since(began).Round(time.Millisecond))
	if strings.Contains(out, "**ERROR**") {
		t.Errorf("web chat failed: %.300s", out)
	}

	out = runSession(brokenChat, "This call should fail. "+marker, false, nil)
	t.Logf("session (broken model) contains_error=%v", strings.Contains(out, "**ERROR**"))

	// 4. The OpenAI-compatible endpoint (no web flag there: LLM-only path).
	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(map[string]interface{}{
		"model":    "model",
		"stream":   false,
		"messages": []map[string]interface{}{{"role": "user", "content": "Name one benefit of retrieval-augmented generation. " + marker}},
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/openai/"+webChat+"/chat/completions", strings.NewReader(string(body)))
	c.Request = c.Request.WithContext(ctx)
	NewOpenAIChatService().OpenAIChatCompletions(c, webChat, body)
	t.Logf("openai-compatible status=%d bytes=%d", rec.Code, rec.Body.Len())
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "**ERROR**") {
		t.Errorf("openai-compatible call failed: %.300s", rec.Body.String())
	}

	flushCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ShutdownFutureAGIClients(flushCtx)
}
