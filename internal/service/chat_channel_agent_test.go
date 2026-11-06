package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestChatChannelAgentBindingRoundTrip(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	ctx := serviceTestContext(t, "owner", "owner")
	if err := db.Create(&entity.UserCanvas{ID: "agent", UserID: "owner", Title: new("Workflow")}).Error; err != nil {
		t.Fatal(err)
	}
	createServiceTestDialog(t, db, "chat", "owner", "Chat")
	svc := NewChatChannelService()
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, new("agent"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].AgentID == nil || *rows[0].AgentID != "agent" || rows[0].AgentName == nil || *rows[0].AgentName != "Workflow" {
		t.Fatalf("list = %+v, %v", rows, err)
	}
	for _, binding := range []map[string]any{
		{"chat_id": "chat", "agent_id": nil},
		{"chat_id": nil, "agent_id": "agent"},
		{"chat_id": nil, "agent_id": nil},
	} {
		row, code, err := svc.UpdateChatChannel(ctx, channel.ID, binding)
		if err != nil || code != common.CodeSuccess {
			t.Fatalf("switch %+v: %+v %v", binding, row, err)
		}
		if (row.ChatID != nil) != (binding["chat_id"] != nil) || (row.AgentID != nil) != (binding["agent_id"] != nil) {
			t.Fatalf("binding did not persist: %+v", row)
		}
	}
	for _, binding := range []map[string]any{
		{"chat_id": "chat", "agent_id": "agent"},
		{"agent_id": 42},
		{"agent_id": "missing"},
	} {
		if _, code, err := svc.UpdateChatChannel(ctx, channel.ID, binding); err == nil || code == common.CodeSuccess {
			t.Fatalf("accepted invalid binding %+v", binding)
		}
	}
	if _, err := svc.CreateChatChannel(ctx, "invalid", "telegram", entity.JSONMap{}, new("chat"), new("agent")); err == nil {
		t.Fatal("create accepted two targets")
	}
}

func TestChatChannelRejectsCrossTenantBindingAndExecution(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "private", "actor", "Private")
	ctx := serviceTestContext(t, "actor", "other", "other")
	svc := NewChatChannelService()
	if _, err := svc.CreateChatChannel(ctx, "invalid", "telegram", entity.JSONMap{}, new("private"), nil); err == nil {
		t.Fatal("accepted a target unavailable to the channel tenant")
	}
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "historical", TenantID: "other", Channel: "telegram", Config: entity.JSONMap{}, ChatID: new("private"), Status: 1,
	})
	if _, err := svc.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: "historical", ChatID: "external", Text: "hello"}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("historical unauthorized binding: %v", err)
	}
}

func TestChatChannelAgentRevokedAccess(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	ctx := serviceTestContext(t, "owner", "owner")
	agent := &entity.UserCanvas{ID: "shared", UserID: "creator", TenantID: new("owner"), Permission: string(entity.TenantPermissionTeam)}
	if err := db.Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewChatChannelService()
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, new("shared"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(agent).Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatal(err)
	}
	svc.agentRunner = channelAgentRunnerFunc(func(context.Context, string, string, string, any, []map[string]any) (<-chan canvas.RunEvent, error) {
		t.Fatal("revoked workflow must not execute")
		return nil, nil
	})
	if _, err := svc.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: channel.ID, ChatID: "external", Text: "hello"}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("revoked workflow binding: %v", err)
	}
}

func TestChatChannelAgentRealEngineContinuationAndResume(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}, &entity.APIToken{},
		&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "owner", "owner")
	dsl := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{"obj": map[string]any{"component_name": "Begin", "params": map[string]any{}}, "downstream": []any{"fill"}},
			"fill": map[string]any{
				"obj":      map[string]any{"component_name": "UserFillUp", "params": map[string]any{"enable_tips": true, "tips": "Your answer?"}},
				"upstream": []any{"begin"}, "downstream": []any{"message"},
			},
			"message": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "got: {{fill@user_input}}"}},
				"upstream": []any{"fill"},
			},
		},
		"path": []any{"begin", "fill", "message"},
	}
	makeCanvasWithDSL(t, "workflow", "owner", "owner", "version", dsl)
	tracker, mr := newRunTrackerForTest(t, time.Hour)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runner := NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
	svc := NewChatChannelServiceWithAgentService(runner)
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, new("workflow"))
	if err != nil {
		t.Fatal(err)
	}
	msg := ChatChannelIncomingMessage{AccountID: channel.ID, ChatID: "external-one", Text: "start"}
	answer, err := svc.HandleIncomingMessage(t.Context(), msg)
	if err != nil || !strings.Contains(answer, "Your answer?") {
		t.Fatalf("waiting: %q %v", answer, err)
	}
	var session entity.API4Conversation
	if err := db.First(&session, "id = ?", channelSessionID("workflow", channel.ID, "external-one")).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.Name == nil || *session.Name != "(channel: channel) start" {
		t.Fatalf("session name = %v", session.Name)
	}
	msg.Text = "hello"
	answer, err = svc.HandleIncomingMessage(t.Context(), msg)
	if err != nil || answer != "got: hello" {
		t.Fatalf("resume: %q %v", answer, err)
	}
	msg.ChatID = "external-two"
	answer, err = svc.HandleIncomingMessage(t.Context(), msg)
	if err != nil || !strings.Contains(answer, "Your answer?") {
		t.Fatalf("isolated conversation: %q %v", answer, err)
	}
	var count int64
	db.Model(&entity.API4Conversation{}).Count(&count)
	if count != 2 {
		t.Fatalf("sessions = %d, want 2", count)
	}
	if err := db.Model(&entity.ChatChannel{}).Where("id = ?", channel.ID).Update("status", 0).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}
	if answer, err := svc.HandleIncomingMessage(t.Context(), msg); answer != "" || err != nil {
		t.Fatalf("disabled channel: %q %v", answer, err)
	}
}

type channelAgentRunnerFunc func(context.Context, string, string, string, any, []map[string]any) (<-chan canvas.RunEvent, error)

func (f channelAgentRunnerFunc) RunAgent(ctx context.Context, agent, session, version string, input any, files []map[string]any) (<-chan canvas.RunEvent, error) {
	return f(ctx, agent, session, version, input, files)
}

func TestChatChannelAgentErrorAndThinking(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []canvas.RunEvent
		want   string
		failed bool
	}{
		{"text", []canvas.RunEvent{{Type: "message", Data: `{"start_to_think":true}`}, {Type: "message", Data: `{"content":"private reasoning"}`}, {Type: "message", Data: `{"end_to_think":true}`}, {Type: "message", Data: `{"content":"answer"}`}}, "answer", false},
		{"error", []canvas.RunEvent{{Type: "error", Data: `{"message":"sensitive provider failure"}`}}, "", true},
		{"cancelled", []canvas.RunEvent{{Type: "message", Data: `{"content":"partial"}`}, {Type: "cancelled"}}, "", true},
		{"form", []canvas.RunEvent{{Type: "waiting_for_user", Data: `{"inputs":{"a":{},"b":{}}}`}}, "This workflow requires structured input. Please continue in RAGFlow.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &ChatChannelService{agentRunner: channelAgentRunnerFunc(func(ctx context.Context, agent, session, version string, input any, files []map[string]any) (<-chan canvas.RunEvent, error) {
				events := make(chan canvas.RunEvent, len(tc.events))
				for _, ev := range tc.events {
					events <- ev
				}
				close(events)
				return events, nil
			})}
			answer, err := svc.handleChannelAgentMessage(t.Context(), &entity.ChatChannel{ID: "channel", AgentID: new("agent")}, ChatChannelIncomingMessage{ChatID: "chat", Text: "hello"})
			if answer != tc.want || (err != nil) != tc.failed {
				t.Fatalf("answer=%q err=%v", answer, err)
			}
		})
	}
}
