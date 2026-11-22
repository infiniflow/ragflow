// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestSharedChatChannelRuntimeMatchesConfigurationAndExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewChatChannelRuntimeCache(client)
	row := &entity.ChatChannel{ID: "wa", Channel: "whatsapp", Status: 1, Config: entity.JSONMap{"credential": map[string]any{"gateway_base_url": "https://gateway.test"}}}
	// This is the runtime's existing canonical configuration payload.
	sum := sha256.Sum256([]byte(`{"channel":"whatsapp","credential":{"gateway_base_url":"https://gateway.test"}}`))
	fingerprint := hex.EncodeToString(sum[:])
	snapshot := map[string]any{"account_id": "wa", "status": "qr", "qr_data_url": "data:image/png;base64,cXI="}
	if err := cache.Publish(t.Context(), row.ID, fingerprint, snapshot); err != nil {
		t.Fatal(err)
	}
	if got, ok := cache.Read(t.Context(), row); !ok || got["qr_data_url"] != "data:image/png;base64,cXI=" {
		t.Fatalf("snapshot = %v, %v", got, ok)
	}
	row.Config = entity.JSONMap{"credential": map[string]any{"gateway_base_url": "https://other.test"}}
	if got, ok := cache.Read(t.Context(), row); ok {
		t.Fatalf("stale configuration returned %v", got)
	}
	row.Config = entity.JSONMap{"credential": map[string]any{"gateway_base_url": "https://gateway.test"}}
	row.Status = 0
	if got, ok := cache.Read(t.Context(), row); ok {
		t.Fatalf("disabled channel returned %v", got)
	}
	row.Status = 1
	mr.FastForward(time.Minute)
	if got, ok := cache.Read(t.Context(), row); ok {
		t.Fatalf("expired snapshot returned %v", got)
	}
}

func TestSharedChatChannelRuntimeDeletesAndReplacesQR(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewChatChannelRuntimeCache(client)
	row := &entity.ChatChannel{ID: "wa", Channel: "whatsapp", Status: 1, Config: entity.JSONMap{}}
	sum := sha256.Sum256([]byte(`{"channel":"whatsapp","credential":{}}`))
	fingerprint := hex.EncodeToString(sum[:])
	for _, snapshot := range []map[string]any{{"status": "qr", "qr_data_url": "secret"}, {"status": "logged_out", "qr_data_url": nil}} {
		if err := cache.Publish(t.Context(), row.ID, fingerprint, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := cache.Read(t.Context(), row); !ok || got["qr_data_url"] != nil || got["status"] != "logged_out" {
		t.Fatalf("logout retained QR: %v, %v", got, ok)
	}
	if err := cache.Delete(t.Context(), row.ID); err != nil {
		t.Fatal(err)
	}
	if got, ok := cache.Read(t.Context(), row); ok {
		t.Fatalf("deleted snapshot returned %v", got)
	}
}

func TestChatChannelRuntimeChecksPermissionBeforeProvider(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	ownerCtx := serviceTestContext(t, "owner", "owner")
	otherCtx := serviceTestContext(t, "other", "other")
	createServiceTestChannel(t, db, &entity.ChatChannel{ID: "private", UserID: "owner", TenantID: "owner", Channel: "whatsapp", Config: entity.JSONMap{}, Status: 1})
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewChatChannelRuntimeCache(client)
	sum := sha256.Sum256([]byte(`{"channel":"whatsapp","credential":{}}`))
	if err := cache.Publish(t.Context(), "private", hex.EncodeToString(sum[:]), map[string]any{"status": "qr", "qr_data_url": "secret"}); err != nil {
		t.Fatal(err)
	}
	original := getChatChannelRuntimeProvider()
	t.Cleanup(func() { SetChatChannelRuntimeProvider(original) })
	calls := 0
	SetChatChannelRuntimeProvider(func(ctx context.Context, row *entity.ChatChannel) (map[string]any, bool) {
		calls++
		return cache.Read(ctx, row)
	})
	svc := NewChatChannelService()
	if got, code, err := svc.GetChatChannelRuntime(otherCtx, "private"); err == nil || code == common.CodeSuccess || got != nil || calls != 0 {
		t.Fatalf("unauthorized read = %v %v %v, calls=%d", got, code, err, calls)
	}
	if got, code, err := svc.GetChatChannelRuntime(ownerCtx, "private"); err != nil || code != common.CodeSuccess || got["qr_data_url"] != "secret" || calls != 1 {
		t.Fatalf("owner read = %v %v %v, calls=%d", got, code, err, calls)
	}
	mr.FastForward(time.Minute)
	if got, code, err := svc.GetChatChannelRuntime(ownerCtx, "private"); err != nil || code != common.CodeSuccess || got["status"] != "waiting" || got["qr_data_url"] != nil {
		t.Fatalf("expired runtime read = %v %v %v", got, code, err)
	}
}
