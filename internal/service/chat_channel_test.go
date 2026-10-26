package service

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func setupChatChannelServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err = db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}, &entity.User{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = origDB
	})

	return db
}

func createServiceTestDialog(t *testing.T, db *gorm.DB, id, tenantID, name string) *entity.Chat {
	t.Helper()

	dialogName := name
	status := string(entity.StatusValid)
	dialog := &entity.Chat{
		ID:           id,
		TenantID:     tenantID,
		Name:         &dialogName,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{"temperature": 0.1},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{"system": "sys"},
		KBIDs:        entity.JSONSlice{"kb-1"},
		Status:       &status,
	}
	if err := db.Create(dialog).Error; err != nil {
		t.Fatalf("failed to create dialog: %v", err)
	}
	return dialog
}

func createServiceTestChannel(t *testing.T, db *gorm.DB, channel *entity.ChatChannel) *entity.ChatChannel {
	t.Helper()

	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("failed to create chat channel: %v", err)
	}
	return channel
}

func createServiceTestMembership(t *testing.T, db *gorm.DB, userID, tenantID string) {
	t.Helper()

	status := "1"
	member := &entity.UserTenant{
		ID:       userID + "_" + tenantID,
		UserID:   userID,
		TenantID: tenantID,
		Role:     "normal",
		Status:   &status,
	}
	if err := db.Create(member).Error; err != nil {
		t.Fatalf("failed to create tenant membership: %v", err)
	}
}

func TestChatChannelServiceCreateAndList(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-1", "tenant-1", "Assistant A")

	svc := NewChatChannelService()
	chatID := "dialog-1"

	ctx := serviceTestContext(t, "tenant-1", "tenant-1")
	channel, err := svc.CreateChatChannel(
		ctx,
		"tenant-1",
		"bot-a",
		"dingtalk",
		entity.JSONMap{"token": "abc"},
		&chatID,
	)
	if err != nil {
		t.Fatalf("CreateChatChannel failed: %v", err)
	}
	if channel.ID == "" {
		t.Fatal("expected generated id")
	}
	if channel.TenantID != "tenant-1" || channel.Name != "bot-a" || channel.Channel != "dingtalk" {
		t.Fatalf("unexpected created channel: %+v", channel)
	}

	list, err := svc.List(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(list))
	}
	if list[0].DialogName == nil || *list[0].DialogName != "Assistant A" {
		t.Fatalf("expected joined dialog name, got %+v", list[0])
	}
}

func TestChatChannelServiceGetChatChannelAllowsJoinedTenant(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "abc"},
		Status:   1,
	})
	createServiceTestMembership(t, db, "user-2", "tenant-1")

	ctx := serviceTestContext(t, "user-2", "tenant-1")
	grantServiceTestResource(t, "tenant-1", "dialog", "cc-1", "user-2", testResourcePermissionRead)
	channel, code, err := NewChatChannelService().GetChatChannel(ctx, "user-2", "cc-1")
	if err != nil {
		t.Fatalf("GetChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %v", code)
	}
	if channel == nil || channel.ID != "cc-1" {
		t.Fatalf("unexpected channel: %+v", channel)
	}
}

func TestChatChannelServiceListUsesPermissionScope(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "channel-team", TenantID: "tenant-1", Name: "team", Channel: "wecom", Config: entity.JSONMap{}, Status: 1,
	})
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID: "channel-other", TenantID: "tenant-2", Name: "other", Channel: "wecom", Config: entity.JSONMap{}, Status: 1,
	})
	createServiceTestMembership(t, db, "user-2", "tenant-1")

	ctx := serviceTestContext(t, "user-2", "tenant-1")
	grantServiceTestResource(t, "tenant-1", "dialog", "channel-team", "user-2", testResourcePermissionRead)
	channels, err := NewChatChannelService().List(ctx, "user-2")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "channel-team" {
		t.Fatalf("List() = %+v, want only member-visible channel-team", channels)
	}
}

func TestChatChannelServiceUpdateChatChannelSuccess(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-1", "tenant-1", "Assistant A")
	createServiceTestDialog(t, db, "dialog-2", "tenant-1", "Assistant B")
	chatID := "dialog-1"
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		ChatID:   &chatID,
		Status:   1,
	})

	ctx := serviceTestContext(t, "tenant-1", "tenant-1")
	updated, code, err := NewChatChannelService().UpdateChatChannel(ctx, "tenant-1", "cc-1", map[string]interface{}{
		"name":    "bot-b",
		"config":  map[string]interface{}{"token": "new"},
		"chat_id": "dialog-2",
	})
	if err != nil {
		t.Fatalf("UpdateChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %v", code)
	}
	if updated.Name != "bot-b" {
		t.Fatalf("expected updated name, got %+v", updated)
	}
	if updated.ChatID == nil || *updated.ChatID != "dialog-2" {
		t.Fatalf("expected updated chat_id, got %+v", updated.ChatID)
	}
	if updated.Config["token"] != "new" {
		t.Fatalf("expected updated config, got %+v", updated.Config)
	}
}

func TestChatChannelServiceUpdateChatChannelRejectsCrossTenantDialog(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-2", "tenant-2", "Assistant B")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		Status:   1,
	})

	ctx := serviceTestContext(t, "tenant-1", "tenant-1")
	_, code, err := NewChatChannelService().UpdateChatChannel(ctx, "tenant-1", "cc-1", map[string]interface{}{
		"chat_id": "dialog-2",
	})
	if code != common.CodeNotFound {
		t.Fatalf("expected hidden not-found error, got %v", code)
	}
	if err == nil {
		t.Fatalf("expected cross-tenant dialog to be hidden, got %v", err)
	}
}

func TestChatChannelServiceUpdateChatChannelRequiresChannelOwnerAccess(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-2", "tenant-2", "Assistant B")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		Status:   1,
	})
	createServiceTestMembership(t, db, "user-1", "tenant-1")
	createServiceTestMembership(t, db, "user-1", "tenant-2")

	_, code, err := NewChatChannelService().UpdateChatChannel(serviceTestContext(t, "user-1", "tenant-1", "tenant-1"), "user-1", "cc-1", map[string]interface{}{
		"chat_id": "dialog-2",
	})
	if code != common.CodeNotFound || !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("UpdateChatChannel() = (%d, %v), want hidden denial because channel owner cannot use Chat", code, err)
	}
}

func TestChatChannelServiceDeleteChatChannel(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	if err := db.Create(&entity.User{ID: "user-2", Nickname: "member", Email: "user-2@test.com"}).Error; err != nil {
		t.Fatalf("create non-owner user: %v", err)
	}
	createServiceTestMembership(t, db, "user-2", "tenant-2")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		Status:   1,
	})

	svc := NewChatChannelService()

	ctx := permission.WithSubject(t.Context(), permission.Subject{UserID: "user-2", TenantID: "tenant-2"})
	deleted, code, err := svc.DeleteChatChannel(ctx, "user-2", "cc-1")
	if code != common.CodeNotFound || !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("expected hidden permission denial, got code=%v err=%v", code, err)
	}
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if deleted {
		t.Fatal("expected delete to be rejected")
	}

	deleted, code, err = svc.DeleteChatChannel(serviceTestContext(t, "tenant-1", "tenant-1"), "tenant-1", "cc-1")
	if err != nil {
		t.Fatalf("DeleteChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess || !deleted {
		t.Fatalf("unexpected delete result: deleted=%v code=%v err=%v", deleted, code, err)
	}

	if _, err = dao.NewChatChannel().GetByID(ctx, dao.DB, "cc-1"); err == nil {
		t.Fatal("expected deleted record to be gone")
	}
}

func countChannelSessions(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&entity.ChatSession{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count sessions: %v", err)
	}
	return count
}

func TestGetOrCreateForChannelNamesFromFirstQueryAndChannelName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	session, err := NewChatSessionService().GetOrCreateForChannel(
		t.Context(), "dialog-1", "cc-1", "discord-channel-1", "你好，帮我查一下退款政策", "Discord 客服",
	)
	if err != nil {
		t.Fatalf("GetOrCreateForChannel failed: %v", err)
	}
	if session.ID != channelSessionID("dialog-1", "cc-1", "discord-channel-1") {
		t.Fatalf("unexpected session id: %s", session.ID)
	}
	if session.DialogID != "dialog-1" {
		t.Fatalf("unexpected dialog id: %s", session.DialogID)
	}
	if session.Name == nil || *session.Name != "(channel: Discord 客服) 你好，帮我查一下退款政策" {
		t.Fatalf("unexpected session name: %v", session.Name)
	}
}

func TestGetOrCreateForChannelKeepsNameAfterFirstMessage(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	svc := NewChatSessionService()
	ctx := t.Context()

	first, err := svc.GetOrCreateForChannel(ctx, "dialog-1", "cc-1", "discord-channel-1", "第一句", "客服")
	if err != nil {
		t.Fatalf("first GetOrCreateForChannel failed: %v", err)
	}

	second, err := svc.GetOrCreateForChannel(ctx, "dialog-1", "cc-1", "discord-channel-1", "第二句", "改过的名字")
	if err != nil {
		t.Fatalf("second GetOrCreateForChannel failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same session, got %s and %s", first.ID, second.ID)
	}
	if second.Name == nil || *second.Name != "(channel: 客服) 第一句" {
		t.Fatalf("expected the first name to stick, got %v", second.Name)
	}
	if count := countChannelSessions(t, db); count != 1 {
		t.Fatalf("expected 1 session, got %d", count)
	}
}

func TestGetOrCreateForChannelTruncatesLongFirstQuery(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	query := strings.Repeat("长", 300)
	session, err := NewChatSessionService().GetOrCreateForChannel(
		t.Context(), "dialog-1", "cc-1", "discord-channel-1", query, "客服",
	)
	if err != nil {
		t.Fatalf("GetOrCreateForChannel failed: %v", err)
	}
	if session.Name == nil {
		t.Fatal("expected a session name")
	}
	name := *session.Name
	if utf8.RuneCountInString(name) != channelSessionNameMaxRunes {
		t.Fatalf("expected %d runes, got %d", channelSessionNameMaxRunes, utf8.RuneCountInString(name))
	}
	if !strings.HasPrefix(name, "(channel: 客服) ") {
		t.Fatalf("expected the channel prefix to survive, got %q", name)
	}
	wantPrefix := "(channel: 客服) " + string([]rune(query)[:channelSessionNameMaxRunes-utf8.RuneCountInString("(channel: 客服) ")])
	if name != wantPrefix {
		t.Fatalf("unexpected truncated name: %q", name)
	}
}

func TestGetOrCreateForChannelNormalizesWhitespace(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	session, err := NewChatSessionService().GetOrCreateForChannel(
		t.Context(), "dialog-1", "cc-1", "discord-channel-1", "你好\n\n帮我\t查一下", "客服",
	)
	if err != nil {
		t.Fatalf("GetOrCreateForChannel failed: %v", err)
	}
	if session.Name == nil || *session.Name != "(channel: 客服) 你好 帮我 查一下" {
		t.Fatalf("unexpected session name: %v", session.Name)
	}
}

func TestGetOrCreateForChannelStripsMentionMarkupFromName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	session, err := NewChatSessionService().GetOrCreateForChannel(
		t.Context(), "dialog-1", "cc-1", "discord-channel-1", "<@1562356664286445598> 林黛玉是谁", "cc",
	)
	if err != nil {
		t.Fatalf("GetOrCreateForChannel failed: %v", err)
	}
	if session.Name == nil || *session.Name != "(channel: cc) 林黛玉是谁" {
		t.Fatalf("unexpected session name: %v", session.Name)
	}
}

func TestChannelSessionNameStripsMentionMarkup(t *testing.T) {
	tests := []struct {
		name        string
		firstQuery  string
		channelName string
		want        string
	}{
		{
			name:        "user mention prefix",
			firstQuery:  "<@1562356664286445598> 林黛玉是谁",
			channelName: "cc",
			want:        "(channel: cc) 林黛玉是谁",
		},
		{
			name:        "nickname mention and role mention",
			firstQuery:  "<@!42> <@&7> 查一下退款政策",
			channelName: "客服",
			want:        "(channel: 客服) 查一下退款政策",
		},
		{
			name:        "channel mention",
			firstQuery:  "<#99> 历史消息",
			channelName: "客服",
			want:        "(channel: 客服) 历史消息",
		},
		{
			name:        "mention only falls back to the channel label",
			firstQuery:  "<@1562356664286445598>",
			channelName: "cc",
			want:        "(channel: cc)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelSessionName(tt.firstQuery, tt.channelName); got != tt.want {
				t.Fatalf("channelSessionName(%q, %q) = %q, want %q", tt.firstQuery, tt.channelName, got, tt.want)
			}
		})
	}
}

func TestGetOrCreateForChannelFallbackNames(t *testing.T) {
	t.Run("blank query uses the channel label", func(t *testing.T) {
		db := setupServiceTestDB(t)
		pushServiceDB(t, db)

		session, err := NewChatSessionService().GetOrCreateForChannel(
			t.Context(), "dialog-1", "cc-1", "discord-channel-1", "  \n ", "客服",
		)
		if err != nil {
			t.Fatalf("GetOrCreateForChannel failed: %v", err)
		}
		if session.Name == nil || *session.Name != "(channel: 客服)" {
			t.Fatalf("unexpected session name: %v", session.Name)
		}
	})

	t.Run("blank query and channel name use the legacy name", func(t *testing.T) {
		db := setupServiceTestDB(t)
		pushServiceDB(t, db)

		session, err := NewChatSessionService().GetOrCreateForChannel(
			t.Context(), "dialog-1", "cc-1", "discord-channel-1", "", "",
		)
		if err != nil {
			t.Fatalf("GetOrCreateForChannel failed: %v", err)
		}
		if session.Name == nil || *session.Name != "channel:cc-1:discord-channel-1" {
			t.Fatalf("unexpected session name: %v", session.Name)
		}
	})
}
