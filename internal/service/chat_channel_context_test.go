package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestPrepareChatChannelCompletionAuthorization(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	if err := db.AutoMigrate(&entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{}); err != nil {
		t.Fatal(err)
	}
	owner := serviceTestContext(t, "owner", "owner")
	outsider := serviceTestContext(t, "outsider", "outsider")
	createServiceTestDialog(t, db, "chat", "owner", "Chat")
	createServiceTestDialog(t, db, "other", "outsider", "Private")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "channel", TenantID: "owner", Config: entity.JSONMap{}, Status: 1, ChatID: new("chat"),
	})
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "revoked", TenantID: "owner", Config: entity.JSONMap{}, Status: 1, ChatID: new("other"),
	})
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "disabled", TenantID: "owner", Config: entity.JSONMap{}, Status: 0, ChatID: new("chat"),
	})
	if err := db.Model(&entity.ChatChannel{}).Where("id = ?", "disabled").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, channel, external, target string
		ctx                             context.Context
		agent                           bool
	}{
		{"unauthenticated", "channel", "external", "chat", t.Context(), false},
		{"cross tenant", "channel", "external", "chat", outsider, false},
		{"missing channel", "missing", "external", "chat", owner, false},
		{"disabled", "disabled", "external", "chat", owner, false},
		{"target mismatch", "channel", "external", "other", owner, false},
		{"wrong target kind", "channel", "external", "chat", owner, true},
		{"revoked target", "revoked", "external", "other", owner, false},
		{"missing external chat", "channel", "", "chat", owner, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := PrepareChatChannelCompletion(tc.ctx, ChatChannelExecutionContext{ChannelID: tc.channel, ChatID: tc.external}, tc.target, "hello", tc.agent)
			if err == nil {
				t.Fatal("accepted unauthorized or invalid execution context")
			}
		})
	}
}

func TestPrepareChatChannelCompletionKeepsChatHistory(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	if err := db.AutoMigrate(&entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{}); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "owner", "owner")
	createServiceTestDialog(t, db, "chat", "owner", "Chat")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "channel", TenantID: "owner", Name: "Telegram", Config: entity.JSONMap{}, Status: 1, ChatID: new("chat"),
	})
	execution := ChatChannelExecutionContext{ChannelID: "channel", ChatID: "external"}
	prepared, sessionID, err := PrepareChatChannelCompletion(ctx, execution, "chat", "first", false)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != channelSessionID("chat", "channel", "external") || permission.UserIDFromContext(prepared) != "owner" {
		t.Fatalf("session=%s identity=%s", sessionID, permission.UserIDFromContext(prepared))
	}
	if err := dao.NewChatSessionDAO().UpdateByID(ctx, db, sessionID, map[string]any{
		"history_update": dao.ConversationHistoryUpdate{Message: map[string]any{"id": "first", "role": "user", "content": "first"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, sameID, err := PrepareChatChannelCompletion(ctx, execution, "chat", "second", false)
	if err != nil || sameID != sessionID {
		t.Fatalf("second turn: %q %v", sameID, err)
	}
	var session entity.ChatSession
	loaded, err := dao.NewChatSessionDAO().GetByID(ctx, db, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	session = *loaded
	if !strings.Contains(string(session.Message), `"content":"first"`) || session.Name == nil || *session.Name != "(channel: Telegram) first" || session.Source != entity.ChatSessionSourceChatChannel {
		t.Fatalf("session metadata/history changed: %+v", session)
	}
	execution.ChatID = "other-external"
	_, otherID, err := PrepareChatChannelCompletion(ctx, execution, "chat", "new", false)
	if err != nil || otherID == sessionID {
		t.Fatalf("external conversation was not isolated: %q %v", otherID, err)
	}
}

func TestPrepareChatChannelCompletionRejectsOrdinarySession(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "owner", "owner")
	createServiceTestDialog(t, db, "chat", "owner", "Chat")
	if err := db.Create(&entity.UserCanvas{ID: "workflow", UserID: "owner"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, agent := range []bool{false, true} {
		t.Run(fmt.Sprint(agent), func(t *testing.T) {
			channel := &entity.ChatChannel{ID: "chat-channel", TenantID: "owner", Status: 1, Config: entity.JSONMap{}, ChatID: new("chat")}
			targetID := "chat"
			if agent {
				channel.ID, channel.ChatID, channel.AgentID = "agent-channel", nil, new("workflow")
				targetID = "workflow"
			}
			createServiceTestChannel(t, db, channel)
			id := channelSessionID(targetID, channel.ID, "external")
			var row any = &entity.ChatSession{ID: id, DialogID: targetID, UserID: new("owner"), Source: entity.ChatSessionSourceNormal}
			if agent {
				row = &entity.API4Conversation{ID: id, DialogID: targetID, UserID: "owner", Source: new(string(entity.AgentSessionSourceAgent))}
			}
			if err := db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
			if _, _, err := PrepareChatChannelCompletion(ctx, ChatChannelExecutionContext{ChannelID: channel.ID, ChatID: "external"}, targetID, "hello", agent); !errors.Is(err, permission.ErrPermissionDenied) {
				t.Fatalf("ordinary session became a channel session: %v", err)
			}
		})
	}
}

func TestPrepareChatChannelCompletionAgentRefreshAndResume(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.ChatChannel{}); err != nil {
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
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "channel", TenantID: "owner", Name: "Telegram", Config: entity.JSONMap{}, Status: 1, AgentID: new("workflow"),
	})
	tracker, mr := newRunTrackerForTest(t, time.Hour)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	newRunner := func() *AgentService {
		return NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
	}
	runner := newRunner()
	run := func(question string) ([]canvas.MessageEvent, []canvas.WaitingForUserEvent) {
		t.Helper()
		prepared, id, err := PrepareChatChannelCompletion(ctx, ChatChannelExecutionContext{ChannelID: "channel", ChatID: "external"}, "workflow", question, true)
		if err != nil {
			t.Fatal(err)
		}
		if id != channelSessionID("workflow", "channel", "external") || AgentSessionIDFromContext(prepared) != id {
			t.Fatalf("untrusted or unstable session: %q", id)
		}
		events, err := runner.RunAgent(prepared, "workflow", id, "", question, nil)
		if err != nil {
			t.Fatal(err)
		}
		messages, waiting, errs, _, _ := drainAgentEvents(t, events)
		if len(errs) != 0 {
			t.Fatalf("run errors: %+v", errs)
		}
		return messages, waiting
	}
	_, waiting := run("first")
	if len(waiting) != 1 {
		t.Fatalf("expected paused run: %+v", waiting)
	}
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", "workflow").Update("dsl", entity.JSONMap(channelMessageDSL("updated"))).Error; err != nil {
		t.Fatal(err)
	}
	runner = newRunner()
	messages, _ := run("answer")
	if len(messages) != 1 || messages[0].Content != "got: answer" {
		t.Fatalf("paused graph was replaced: %+v", messages)
	}
	messages, _ = run("next")
	if len(messages) != 1 || messages[0].Content != "updated" {
		t.Fatalf("fresh turn did not refresh graph: %+v", messages)
	}
	session := getAPIConversationForTest(t, db, channelSessionID("workflow", "channel", "external"))
	if session.Source == nil || *session.Source != string(entity.AgentSessionSourceChatChannel) ||
		session.Name == nil || *session.Name != "(channel: Telegram) first" || len(session.DSL["history"].([]any)) < 4 {
		t.Fatalf("channel metadata/history lost: %+v", session)
	}
}
