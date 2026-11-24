package service

import (
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func createChatChannelTestAPIKey(t *testing.T, db *gorm.DB, tenantID string) {
	t.Helper()
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.APIToken{TenantID: tenantID, UserID: new(tenantID), Token: tenantID + "-key"}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestFirstChatChannelAPIKeyMatchesEmbedSelection(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createChatChannelTestAPIKey(t, db, "other")
	if _, err := FirstChatChannelAPIKey(t.Context(), "owner"); err == nil || err.Error() != "Please create API Key first." {
		t.Fatalf("missing tenant key: %v", err)
	}
	for _, token := range []entity.APIToken{
		{TenantID: "owner", UserID: new("member"), Token: "member-key"},
		{TenantID: "owner", UserID: new("owner"), Token: "owner-key"},
	} {
		if err := db.Create(&token).Error; err != nil {
			t.Fatal(err)
		}
	}
	keys, err := dao.NewAPITokenDAO().GetByTenantID(t.Context(), db, "owner")
	if err != nil {
		t.Fatal(err)
	}
	got, err := FirstChatChannelAPIKey(t.Context(), "owner")
	if err != nil || got != keys[0].Token {
		t.Fatalf("key = %q, %v; want first listed key %q", got, err, keys[0].Token)
	}
	if err := db.Delete(keys[0]).Error; err != nil {
		t.Fatal(err)
	}
	got, err = FirstChatChannelAPIKey(t.Context(), "owner")
	if err != nil || got != keys[1].Token {
		t.Fatalf("key after deletion = %q, %v; want %q", got, err, keys[1].Token)
	}
}

func TestChatChannelRequiresAPIKeyForActivation(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "owner", "owner")
	svc := NewChatChannelService()
	if _, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, nil); err == nil || err.Error() != "Please create API Key first." {
		t.Fatalf("create without key: %v", err)
	}
	var count int64
	if err := db.Model(&entity.ChatChannel{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected create persisted %d channels: %v", count, err)
	}
	createChatChannelTestAPIKey(t, db, "owner")
	channel, err := svc.CreateChatChannel(ctx, "channel", "telegram", entity.JSONMap{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Where("tenant_id = ?", "owner").Delete(&entity.APIToken{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, code, err := svc.UpdateChatChannel(ctx, channel.ID, map[string]any{"name": "new"}); code != common.CodeDataError || err == nil || err.Error() != "Please create API Key first." {
		t.Fatalf("active update without key: %v %v", code, err)
	}
	if err := db.Model(channel).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, code, err := svc.UpdateChatChannel(ctx, channel.ID, map[string]any{"name": "disabled"}); code != common.CodeSuccess || err != nil {
		t.Fatalf("disabled update without key: %v %v", code, err)
	}
	if _, code, err := svc.DeleteChatChannel(ctx, channel.ID); code != common.CodeSuccess || err != nil {
		t.Fatalf("delete without key: %v %v", code, err)
	}
}
