package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2/server"
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
	if _, err := svc.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: "historical", ChatID: "external", Text: "hello"}, nil); !errors.Is(err, permission.ErrPermissionDenied) {
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
	if _, err := svc.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: channel.ID, ChatID: "external", Text: "hello"}, nil); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("revoked workflow binding: %v", err)
	}
}

func TestChatChannelAgentRealEngineContinuationAndResume(t *testing.T) {
	for _, mode := range []string{"local", "different-process", "without-tracker"} {
		t.Run(mode, func(t *testing.T) { testChatChannelAgentUpdateDuringPause(t, mode) })
	}
}

func testChatChannelAgentUpdateDuringPause(t *testing.T, mode string) {
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
	if mode == "without-tracker" {
		tracker = nil
	}
	runner := NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
	svc := NewChatChannelServiceWithAgentService(runner)
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, new("workflow"))
	if err != nil {
		t.Fatal(err)
	}
	msg := ChatChannelIncomingMessage{AccountID: channel.ID, ChatID: "external-one", Text: "start"}
	answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || !strings.Contains(answer, "Your answer?") {
		t.Fatalf("waiting: %q %v", answer, err)
	}
	updated := channelMessageDSL("updated")
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(updated)).Error; err != nil {
		t.Fatal(err)
	}
	if mode == "different-process" {
		// Recover the persisted interrupt without the first process's Runner.
		svc.agentRunner = NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
	}
	var session entity.API4Conversation
	if err := db.First(&session, "id = ?", channelSessionID("workflow", channel.ID, "external-one")).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.Name == nil || *session.Name != "(channel: channel) start" {
		t.Fatalf("session name = %v", session.Name)
	}
	msg.Text = "hello"
	answer, err = svc.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || answer != "got: hello" {
		t.Fatalf("resume: %q %v", answer, err)
	}
	answer, err = svc.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || answer != "updated" {
		t.Fatalf("fresh turn after resume: %q %v", answer, err)
	}
	msg.ChatID = "external-two"
	answer, err = svc.HandleIncomingMessage(t.Context(), msg, nil)
	if err != nil || answer != "updated" {
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
	if answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil); answer != "" || err != nil {
		t.Fatalf("disabled channel: %q %v", answer, err)
	}
}

func channelMessageDSL(text string) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"begin": map[string]any{"obj": map[string]any{"component_name": "Begin", "params": map[string]any{}}, "downstream": []any{"message"}},
			"message": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": text}},
				"upstream": []any{"begin"},
			},
		},
	}
}

func TestRefreshChannelAgentDSLRetainsOnlyCompatibleConversationState(t *testing.T) {
	draft := channelMessageDSL("updated")
	draft["globals"] = map[string]any{"env.keep": "default", "env.changed": "new", "env.retyped": 1.0, "env.added": true}
	draft["variables"] = map[string]any{"changed": map[string]any{"type": "string", "value": "new"}}
	old := map[string]any{
		"history": []any{[]any{"user", "previous"}},
		"memory":  []any{[]any{"remembered"}},
		"path":    []any{"removed"},
		"globals": map[string]any{
			"env.keep": "remembered", "env.changed": "old", "env.retyped": "string", "env.removed": "old",
			"sys.history": []any{"user: previous"}, "sys.conversation_turns": 3.0, "removed@output": "stale",
		},
		"variables": map[string]any{"changed": map[string]any{"type": "string", "value": "old"}},
	}
	got := refreshChannelAgentDSL(draft, old)
	for _, key := range []string{"history", "memory"} {
		if !reflect.DeepEqual(got[key], old[key]) {
			t.Fatalf("%s = %#v, want %#v", key, got[key], old[key])
		}
	}
	globals := got["globals"].(map[string]any)
	for key, want := range map[string]any{
		"env.keep": "remembered", "env.changed": "new", "env.retyped": 1.0, "env.added": true,
		"sys.history": []any{"user: previous"}, "sys.conversation_turns": 3.0,
	} {
		if !reflect.DeepEqual(globals[key], want) {
			t.Fatalf("%s = %#v, want %#v", key, globals[key], want)
		}
	}
	for _, key := range []string{"env.removed", "removed@output"} {
		if _, ok := globals[key]; ok {
			t.Fatalf("retained obsolete state %s", key)
		}
	}
	if len(got["path"].([]any)) != 0 || draft["globals"].(map[string]any)["env.keep"] != "default" {
		t.Fatal("retained old execution path or mutated draft")
	}
}

func TestChatChannelAgentUpdatesPreserveConversation(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}, &entity.APIToken{},
		&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "owner", "owner")
	makeCanvasWithDSL(t, "workflow", "owner", "owner", "released", channelMessageDSL("v1"))
	makeCanvasWithDSL(t, "other", "owner", "owner", "other-version", channelMessageDSL("other"))
	tracker, mr := newRunTrackerForTest(t, time.Hour)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runner := NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
	svc := NewChatChannelServiceWithAgentService(runner)
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, new("workflow"))
	if err != nil {
		t.Fatal(err)
	}
	msg := ChatChannelIncomingMessage{AccountID: channel.ID, ChatID: "external", Text: "first"}
	if answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil); err != nil || answer != "v1" {
		t.Fatalf("first turn: %q %v", answer, err)
	}
	sessionID := channelSessionID("workflow", channel.ID, msg.ChatID)
	session := getAPIConversationForTest(t, db, sessionID)
	beforeHistory := session.DSL["history"].([]any)
	if len(beforeHistory) != 2 {
		t.Fatalf("first turn history = %#v", beforeHistory)
	}
	// Ordinary and explicit-version sessions retain their established snapshots.
	run := func(id, version, want string) {
		t.Helper()
		events, err := runner.RunAgent(WithAgentSessionID(ctx, id), "workflow", id, version, "probe", nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, _, errs, _, _ := drainAgentEvents(t, events)
		if len(errs) != 0 || len(messages) != 1 || messages[0].Content != want {
			t.Fatalf("ordinary session %s: messages=%+v errors=%+v", id, messages, errs)
		}
	}
	run("ordinary", "", "v1")
	updated := channelMessageDSL("v2")
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(updated)).Error; err != nil {
		t.Fatal(err)
	}
	msg.Text = "second"
	if answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil); err != nil || answer != "v2" {
		t.Fatalf("updated same conversation: %q %v", answer, err)
	}
	session = getAPIConversationForTest(t, db, sessionID)
	history := session.DSL["history"].([]any)
	if len(history) != 4 || !reflect.DeepEqual(history[0], beforeHistory[0]) {
		t.Fatalf("conversation history = %#v", history)
	}
	message := session.DSL["components"].(map[string]any)["message"].(map[string]any)
	if text := message["params"].(map[string]any)["text"]; text != "v2" {
		t.Fatalf("persisted graph text = %v", text)
	}
	run("ordinary", "", "v1")
	run("explicit-version", "released", "v1")
	run("new-ordinary", "", "v2")
	for _, chatID := range []string{"new-external", "external"} {
		msg.ChatID = chatID
		if answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil); err != nil || answer != "v2" {
			t.Fatalf("conversation %s: %q %v", chatID, answer, err)
		}
	}
	mr.Server().SetPreHook(func(peer *server.Peer, command string, args ...string) bool {
		if strings.EqualFold(command, "HGET") && len(args) == 2 && args[1] == "interrupt_id" {
			peer.WriteError("ERR unavailable interrupt state")
			return true
		}
		return false
	})
	if _, err := svc.HandleIncomingMessage(t.Context(), msg, nil); !errors.Is(err, ErrAgentStorageError) {
		t.Fatalf("unavailable interrupt state must prevent execution: %v", err)
	}
	mr.Server().SetPreHook(nil)
	if _, _, err := svc.UpdateChatChannel(ctx, channel.ID, map[string]any{"agent_id": "other"}); err != nil {
		t.Fatal(err)
	}
	if answer, err := svc.HandleIncomingMessage(t.Context(), msg, nil); err != nil || answer != "other" {
		t.Fatalf("switched target: %q %v", answer, err)
	}
	otherSession := getAPIConversationForTest(t, db, channelSessionID("other", channel.ID, msg.ChatID))
	if otherSession.DialogID != "other" || len(otherSession.DSL["history"].([]any)) != 2 {
		t.Fatalf("switched target session = %+v", otherSession)
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
			answer, err := svc.handleChannelAgentMessage(t.Context(), &entity.ChatChannel{ID: "channel", AgentID: new("agent")}, ChatChannelIncomingMessage{ChatID: "chat", Text: "hello"}, nil)
			if answer != tc.want || (err != nil) != tc.failed {
				t.Fatalf("answer=%q err=%v", answer, err)
			}
		})
	}
}
