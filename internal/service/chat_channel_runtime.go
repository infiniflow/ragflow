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
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

const chatChannelRuntimeTTL = 30 * time.Second

// ChatChannelRuntimeCache shares transient channel metadata with the API process.
type ChatChannelRuntimeCache struct {
	client *redis.Client
}

type chatChannelRuntimeEntry struct {
	Fingerprint string         `json:"fingerprint"`
	Snapshot    map[string]any `json:"snapshot"`
}

func NewChatChannelRuntimeCache(client *redis.Client) *ChatChannelRuntimeCache {
	return &ChatChannelRuntimeCache{client: client}
}

func chatChannelRuntimeKey(channelID string) string {
	return "chat_channel:runtime:" + channelID
}

// Publish refreshes the snapshot lease; a crashed runtime loses its metadata automatically.
func (c *ChatChannelRuntimeCache) Publish(ctx context.Context, channelID, fingerprint string, snapshot map[string]any) error {
	if c == nil || c.client == nil {
		return errors.New("chat-channel runtime cache is unavailable")
	}
	payload, err := json.Marshal(chatChannelRuntimeEntry{Fingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		return err
	}
	return c.client.Set(ctx, chatChannelRuntimeKey(channelID), payload, chatChannelRuntimeTTL).Err()
}

// Read accepts only metadata from the currently enabled WhatsApp configuration.
// Permission checks belong to GetChatChannelRuntime, before it invokes this provider.
func (c *ChatChannelRuntimeCache) Read(ctx context.Context, channel *entity.ChatChannel) (map[string]any, bool) {
	if c == nil || c.client == nil || channel.Status != 1 || channel.Channel != "whatsapp" {
		return nil, false
	}
	payload, err := c.client.Get(ctx, chatChannelRuntimeKey(channel.ID)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			common.Warn("Failed to read chat-channel runtime status", zap.String("channel_id", channel.ID), zap.Error(err))
		}
		return nil, false
	}
	var entry chatChannelRuntimeEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		common.Warn("Invalid chat-channel runtime status", zap.String("channel_id", channel.ID))
		return nil, false
	}
	credential := map[string]any{}
	switch config := channel.Config["credential"].(type) {
	case map[string]any:
		credential = config
	case entity.JSONMap:
		credential = config
	}
	if entry.Fingerprint != ChatChannelRuntimeFingerprint(channel.Channel, credential) || entry.Snapshot == nil {
		return nil, false
	}
	return entry.Snapshot, true
}

func (c *ChatChannelRuntimeCache) Delete(ctx context.Context, channelID string) error {
	if c == nil || c.client == nil {
		return errors.New("chat-channel runtime cache is unavailable")
	}
	return c.client.Del(ctx, chatChannelRuntimeKey(channelID)).Err()
}
