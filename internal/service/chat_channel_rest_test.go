package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestChatChannelRESTExecution(t *testing.T) {
	for _, agent := range []bool{false, true} {
		t.Run(fmt.Sprint(agent), func(t *testing.T) {
			db := setupChatChannelServiceTestDB(t)
			if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
				t.Fatal(err)
			}
			serviceTestContext(t, "owner", "owner", "member")
			for _, row := range []any{
				&entity.User{ID: "owner", Email: "owner@example.com", AccessToken: new("owner-access")},
				&entity.User{ID: "member", Email: "member@example.com", AccessToken: new("member-access")},
				&entity.APIToken{TenantID: "owner", UserID: new("member"), Token: "aaa-denied"},
				&entity.APIToken{TenantID: "owner", UserID: new("owner"), Token: "first-key"},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			channel := &entity.ChatChannel{Config: entity.JSONMap{}, ID: "channel", TenantID: "owner", Name: "Support", Status: 1, ChatID: new("target")}
			if agent {
				channel.ChatID = nil
				channel.AgentID = new("target")
				if err := db.Create(&entity.UserCanvas{ID: "target", UserID: "owner"}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				createServiceTestDialog(t, db, "target", "owner", "Support")
			}
			createServiceTestChannel(t, db, channel)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer first-key" {
					t.Error("missing first API key")
				}
				path := "/api/v1/chat/completions"
				if agent {
					path = "/api/v1/agents/chat/completions"
				}
				if r.URL.Path != path {
					t.Errorf("path=%s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				cc, _ := body["channel_context"].(map[string]any)
				if cc["channel_id"] != "channel" || cc["chat_id"] != "external" || body["stream"] != true {
					t.Errorf("body=%v", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if agent {
					fmt.Fprint(w, "data: {\"event\":\"message\",\"data\":{\"start_to_think\":true}}\n\ndata: {\"event\":\"message\",\"data\":{\"content\":\"private\"}}\n\ndata: {\"event\":\"message\",\"data\":{\"end_to_think\":true}}\n\ndata: {\"event\":\"message\",\"data\":{\"content\":\"answer [ID:1]\"}}\n\ndata:[DONE]\n\n")
				} else {
					fmt.Fprint(w, "data: {\"code\":0,\"data\":{\"answer\":\"answer [ID:1]\"}}\n\ndata: {\"code\":0,\"data\":true}\n\n")
				}
			}))
			defer api.Close()
			client := NewChatChannelRESTClient(api.URL)
			var deltas []string
			answer, err := client.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: "channel", ChatID: "external", Text: "hello"}, func(s string) { deltas = append(deltas, s) })
			if err != nil || strings.TrimSpace(answer) != "answer" {
				t.Fatalf("answer=%q error=%v", answer, err)
			}
			if len(deltas) == 0 || strings.Contains(strings.Join(deltas, ""), "private") || strings.Contains(strings.Join(deltas, ""), "[ID:") {
				t.Fatalf("deltas=%q", deltas)
			}
			if err := db.Where("tenant_id = ? AND token = ?", "owner", "first-key").Delete(&entity.APIToken{}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := client.HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: "channel", ChatID: "external", Text: "hello"}, nil); err == nil || !strings.Contains(err.Error(), "no tenant API key can run") {
				t.Fatalf("expected rejection with only an unauthorized key, got %v", err)
			}
		})
	}
}

func TestChatChannelRESTStreamFailures(t *testing.T) {
	for _, body := range []string{"data: {\"code\":500,\"message\":\"secret-key\",\"data\":false}\n\n", "data: {\"event\":\"message\",\"data\":{\"content\":\"partial\"}}\n\n", "data: broken\n\n"} {
		t.Run(body, func(t *testing.T) {
			_, err := consumeChatChannelRESTStream(strings.NewReader(body), true, nil)
			if err == nil || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestChatChannelRESTPreservesFinalAndUserInput(t *testing.T) {
	tests := []struct {
		name       string
		agent      bool
		body, want string
	}{
		{"rewritten final", false, "data: {\"code\":0,\"data\":{\"answer\":\"draft\"}}\n\ndata: {\"code\":0,\"data\":{\"answer\":\"<think>private</think>final [1]\",\"final\":true}}\n\ndata: {\"data\":true}\n\n", "final "},
		{"user input", true, "data: {\"event\":\"waiting_for_user\",\"data\":{\"tips\":\"Your answer?\",\"inputs\":{\"answer\":{}}}}\n\ndata:[DONE]\n\n", "Your answer?"},
		{"structured input", true, "data: {\"event\":\"waiting_for_user\",\"data\":{\"inputs\":{\"a\":{},\"b\":{}}}}\n\ndata:[DONE]\n\n", "This workflow requires structured input. Please continue in RAGFlow."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := consumeChatChannelRESTStream(strings.NewReader(tt.body), tt.agent, nil)
			if err != nil || answer != tt.want {
				t.Fatalf("answer=%q error=%v", answer, err)
			}
		})
	}
}

func TestChatChannelRESTRejectsMissingKeyAndStopsDisabledChannel(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("1 = 1").Delete(&entity.APIToken{}).Error; err != nil {
		t.Fatal(err)
	}
	createServiceTestChannel(t, db, &entity.ChatChannel{Config: entity.JSONMap{}, ID: "no-key", TenantID: "owner", ChatID: new("chat"), Status: 1})
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("request sent without API key") }))
	defer api.Close()
	client := NewChatChannelRESTClient(api.URL)
	msg := ChatChannelIncomingMessage{AccountID: "no-key", ChatID: "external", Text: "hello"}
	if _, err := client.HandleIncomingMessage(t.Context(), msg, nil); err == nil || !strings.Contains(err.Error(), "Please create API Key first") {
		t.Fatalf("missing key error=%v", err)
	}
	if err := db.Model(&entity.ChatChannel{}).Where("id = ?", "no-key").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if answer, err := client.HandleIncomingMessage(t.Context(), msg, nil); err != nil || answer != "" {
		t.Fatalf("disabled=%q %v", answer, err)
	}
}

func TestChatChannelRESTRejectsBusinessErrorWithoutLeakingMessage(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createChatChannelTestAPIKey(t, db, "owner")
	serviceTestContext(t, "owner", "owner")
	if err := db.Create(&entity.User{ID: "owner", Email: "owner@example.com", AccessToken: new("owner-access")}).Error; err != nil {
		t.Fatal(err)
	}
	createServiceTestDialog(t, db, "chat", "owner", "Support")
	createServiceTestChannel(t, db, &entity.ChatChannel{ID: "channel", TenantID: "owner", Config: entity.JSONMap{}, ChatID: new("chat"), Status: 1})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":403,"message":"sensitive-provider-details"}`)
	}))
	defer api.Close()
	answer, err := NewChatChannelRESTClient(api.URL).HandleIncomingMessage(t.Context(), ChatChannelIncomingMessage{AccountID: "channel", ChatID: "external", Text: "hello"}, nil)
	if err != nil || answer != "Sorry, I couldn't process your message right now." {
		t.Fatalf("answer=%q error=%v", answer, err)
	}
}
