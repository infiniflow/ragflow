package handler

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
)

func setupChannelCompletionAPI(t *testing.T) (*gin.Engine, *AgentHandler) {
	t.Helper()
	t.Setenv(common.EnvRAGFlowSecretKey, "channel-completion-test-secret")
	if err := server.Init(""); err != nil {
		t.Fatal(err)
	}
	_, _, db := setupGinContextWithUserAndDB(t, http.MethodPost, "/")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Tenant{}, &entity.APIToken{},
		&entity.API4ConversationReference{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{},
		&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&entity.User{ID: "user-1", AccessToken: new("access-token"), Nickname: "Owner", Email: "owner@example.com"},
		&entity.Tenant{ID: "user-1", Name: new("Owner")},
		&entity.UserTenant{ID: "membership", UserID: "user-1", TenantID: "user-1", Role: "owner", Status: new("1")},
		&entity.APIToken{TenantID: "user-1", UserID: new("user-1"), Token: "first-key"},
		&entity.UserCanvas{ID: "workflow", UserID: "user-1", TenantID: new("user-1"), Permission: "me",
			CanvasCategory: "agent_canvas", DSL: entity.JSONMap(channelCompletionDSL("first"))},
		&entity.ChatChannel{ID: "channel", TenantID: "user-1", Name: "Telegram", Channel: "telegram", Config: entity.JSONMap{}, Status: 1, AgentID: new("workflow")},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "user-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runner := service.NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, canvas.NewRunTrackerWithClient(client, time.Hour))
	handler := &AgentHandler{chatRunner: runner}
	local.InitAdminStatus()
	local.SetAdminStatus(local.Available, "unit test")
	engine := gin.New()
	engine.Use(NewAuthHandler().AuthMiddleware())
	engine.POST("/api/v1/agents/chat/completions", handler.AgentChatCompletions)
	engine.POST("/api/v1/chat/completions", NewChatSessionHandler(service.NewChatSessionService(), nil).ChatCompletions)
	return engine, handler
}

func channelCompletionDSL(text string) map[string]any {
	return map[string]any{"components": map[string]any{
		"begin":   map[string]any{"obj": map[string]any{"component_name": "Begin", "params": map[string]any{}}, "downstream": []any{"message"}},
		"message": map[string]any{"obj": map[string]any{"component_name": "Message", "params": map[string]any{"text": text}}, "upstream": []any{"begin"}},
	}}
}

func TestChatChannelCompletionRESTAgentRefreshAndResume(t *testing.T) {
	engine, _ := setupChannelCompletionAPI(t)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	client := service.NewChatChannelRESTClient(server.URL)
	msg := service.ChatChannelIncomingMessage{AccountID: "channel", ChatID: "external", Text: "hello"}
	run := func(want string) {
		t.Helper()
		answer, err := client.HandleIncomingMessage(t.Context(), msg, nil)
		if err != nil || answer != want {
			t.Fatalf("answer=%q error=%v; want %q", answer, err, want)
		}
	}
	run("first")
	var first entity.API4Conversation
	if err := dao.DB.Take(&first, "dialog_id = ?", "workflow").Error; err != nil {
		t.Fatal(err)
	}
	if first.Source == nil || *first.Source != string(entity.AgentSessionSourceChatChannel) || first.Name == nil || *first.Name != "(channel: Telegram) hello" {
		t.Fatalf("channel metadata lost: %+v", first)
	}
	for _, row := range []any{
		&entity.User{ID: "member", AccessToken: new("member-access"), Nickname: "Member", Email: "member@example.com"},
		&entity.UserTenant{ID: "member-membership", UserID: "member", TenantID: "user-1", Role: "normal", Status: new("1")},
	} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(dao.DB, "user-1", "member"); err != nil {
		t.Fatal(err)
	}
	if err := testsupport.GrantTestResourcePermission(dao.DB, "user-1", "canvas", "workflow", "member", "read"); err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.APIToken{}).Where("token = ?", "first-key").Update("user_id", "member").Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(channelCompletionDSL("updated"))).Error; err != nil {
		t.Fatal(err)
	}
	msg.Text = "second"
	run("updated")
	var resumed entity.API4Conversation
	if err := dao.DB.Take(&resumed, "id = ?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(resumed.DSL["history"].([]any)) != 4 || resumed.UserID != first.UserID {
		t.Fatalf("history not retained: %+v", resumed.DSL["history"])
	}
	paused := channelCompletionDSL("got: {{fill@user_input}}")
	components := paused["components"].(map[string]any)
	components["begin"].(map[string]any)["downstream"] = []any{"fill"}
	components["fill"] = map[string]any{
		"obj":      map[string]any{"component_name": "UserFillUp", "params": map[string]any{"enable_tips": true, "tips": "Your answer?"}},
		"upstream": []any{"begin"}, "downstream": []any{"message"},
	}
	components["message"].(map[string]any)["upstream"] = []any{"fill"}
	paused["path"] = []any{"begin", "fill", "message"}
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(paused)).Error; err != nil {
		t.Fatal(err)
	}
	msg.Text = "start paused run"
	run("Your answer?")
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(channelCompletionDSL("after pause"))).Error; err != nil {
		t.Fatal(err)
	}
	msg.Text = "answer"
	run("got: answer")
	msg.Text = "fresh"
	run("after pause")
	var count int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("conversation count=%d err=%v", count, err)
	}
}

func TestChatChannelCompletionRESTChatMemberKey(t *testing.T) {
	t.Setenv("ALLOW_ANY_HOST", "true")
	engine, _ := setupChannelCompletionAPI(t)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"answer\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello back\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"answer\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(llm.Close)
	extra, err := json.Marshal(map[string]string{"base_url": llm.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&entity.User{ID: "member", AccessToken: new("member-access"), Nickname: "Member", Email: "member@example.com"},
		&entity.UserTenant{ID: "member-membership", UserID: "member", TenantID: "user-1", Role: "normal", Status: new("1")},
		&entity.TenantModelProvider{ID: "provider", UserID: "user-1", TenantID: "user-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "provider", InstanceName: "default", APIKey: "local-key", Status: "active", Extra: string(extra)},
		&entity.TenantModel{ID: "model", ProviderID: "provider", InstanceID: "instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active"},
		&entity.Chat{ID: "chat", UserID: "member", TenantID: "user-1", Name: new("Chat"), Status: new("1"), LLMID: "model",
			LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}},
	} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(dao.DB, "user-1", "member"); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []struct{ kind, id string }{{"chat", "chat"}, {"model_instance", "instance"}} {
		if err := testsupport.GrantTestResourcePermission(dao.DB, "user-1", resource.kind, resource.id, "member", "owner"); err != nil {
			t.Fatal(err)
		}
	}
	if err := dao.DB.Model(&entity.APIToken{}).Where("token = ?", "first-key").Update("user_id", "member").Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.ChatChannel{}).Where("id = ?", "channel").Updates(map[string]any{"agent_id": nil, "chat_id": "chat"}).Error; err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(engine)
	t.Cleanup(api.Close)
	client := service.NewChatChannelRESTClient(api.URL)
	msg := service.ChatChannelIncomingMessage{AccountID: "channel", ChatID: "external", Text: "hello"}
	answer, err := client.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || answer != "hello back" {
		t.Fatalf("member chat answer=%q err=%v", answer, err)
	}
	var session entity.ChatSession
	if err := dao.DB.Take(&session, "dialog_id = ?", "chat").Error; err != nil {
		t.Fatal(err)
	}
	if session.UserID == nil || *session.UserID != "member" {
		t.Fatalf("new session owner=%v", session.UserID)
	}
	if err := dao.DB.Model(&entity.ChatSession{}).Where("id = ?", session.ID).Update("user_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	msg.Text = "second"
	answer, err = client.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || answer != "hello back" {
		t.Fatalf("historical chat answer=%q err=%v", answer, err)
	}
	loaded, err := dao.NewChatSessionDAO().GetByID(t.Context(), dao.DB, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var history []map[string]any
	if err := json.Unmarshal(loaded.Message, &history); err != nil || len(history) != 4 || loaded.UserID != nil {
		t.Fatalf("legacy history/ownership changed: %+v err=%v", loaded, err)
	}
}

func TestChatChannelCompletionRejectsSessionOverride(t *testing.T) {
	for _, agent := range []bool{true, false} {
		t.Run(fmt.Sprint(agent), func(t *testing.T) {
			engine, _ := setupChannelCompletionAPI(t)
			path, target := "/api/v1/agents/chat/completions", `"agent_id":"workflow"`
			if !agent {
				if err := dao.DB.Create(&entity.Chat{ID: "chat", TenantID: "user-1", Name: new("Chat"), Status: new("1"),
					LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}}).Error; err != nil {
					t.Fatal(err)
				}
				if err := dao.DB.Model(&entity.ChatChannel{}).Where("id = ?", "channel").Updates(map[string]any{"agent_id": nil, "chat_id": "chat"}).Error; err != nil {
					t.Fatal(err)
				}
				path, target = "/api/v1/chat/completions", `"chat_id":"chat"`
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{`+target+`,"question":"hello","session_id":"somebody-elses-session","channel_context":{"channel_id":"channel","chat_id":"external"}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer first-key")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			var response struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Code != int(common.CodeArgumentError) {
				t.Fatalf("session override accepted: %s err=%v", recorder.Body.String(), err)
			}
		})
	}
}

func TestChatChannelCompletionPreservesErrorEnvelope(t *testing.T) {
	engine, handler := setupChannelCompletionAPI(t)
	handler.chatRunner = &stubChatRunner{events: []canvas.RunEvent{
		{Type: "error", Data: `{"message":"private failure","kind":"user"}`},
		{Type: "done"},
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/chat/completions",
		strings.NewReader(`{"agent_id":"workflow","question":"hello","stream":true,"channel_context":{"channel_id":"channel","chat_id":"external"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer first-key")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if !strings.Contains(recorder.Body.String(), `"code":500`) || strings.Contains(recorder.Body.String(), `"event":"message"`) {
		t.Fatalf("error became conversation content: %s", recorder.Body.String())
	}
}
