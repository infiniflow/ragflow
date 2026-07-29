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

package channels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"
	"time"

	"ragflow/internal/channels/core"
	"ragflow/internal/channels/whatsapp"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

const reconcileInterval = 10 * time.Second

type runningChannel struct {
	channel core.Channel
	fp      string
}

type Runtime struct {
	mu      sync.Mutex
	running map[string]runningChannel
	failed  map[string]string
}

// NewRuntime creates an empty chat-channel runtime reconciler.
func NewRuntime() *Runtime {
	return &Runtime{
		running: map[string]runningChannel{},
		failed:  map[string]string{},
	}
}

// Start launches the chat-channel runtime reconciler in the background.
func Start(ctx context.Context) *Runtime {
	rt := NewRuntime()
	go rt.Run(ctx)
	return rt
}

// Run reconciles configured chat channels until the context is cancelled.
func (r *Runtime) Run(ctx context.Context) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	defer r.stopAll(context.Background())

	for {
		if err := r.Reconcile(ctx); err != nil {
			log.Printf("chat channel reconcile failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Reconcile starts, stops, and restarts channel instances to match the database state.
func (r *Runtime) Reconcile(ctx context.Context) error {
	desired, err := desiredChannels(ctx)
	if err != nil {
		return err
	}

	r.mu.Lock()
	for accountID, entry := range r.running {
		wanted, ok := desired[accountID]
		if !ok || wanted.fp != entry.fp {
			delete(r.running, accountID)
			go stopChannel(context.Background(), entry.channel)
		}
	}
	for accountID, fp := range r.failed {
		wanted, ok := desired[accountID]
		if !ok || wanted.fp != fp {
			delete(r.failed, accountID)
		}
	}
	r.mu.Unlock()

	activeWhatsApp := false
	for _, wanted := range desired {
		if wanted.channel == "whatsapp" {
			activeWhatsApp = true
			break
		}
	}
	if err := whatsapp.SyncGateway(ctx, activeWhatsApp); err != nil && activeWhatsApp {
		log.Printf("failed to sync WhatsApp gateway: %v", err)
	}

	for accountID, wanted := range desired {
		r.mu.Lock()
		_, isRunning := r.running[accountID]
		failedSameConfig := r.failed[accountID] == wanted.fp
		r.mu.Unlock()
		if isRunning || failedSameConfig {
			continue
		}
		if err := r.startChannel(ctx, accountID, wanted); err != nil {
			log.Printf("failed to start chat channel %s (%s): %v", accountID, wanted.channel, err)
			r.mu.Lock()
			r.failed[accountID] = wanted.fp
			r.mu.Unlock()
		}
	}
	return nil
}

type desiredChannel struct {
	channel    string
	credential map[string]any
	fp         string
}

// desiredChannels loads enabled chat-channel rows and reduces them to runtime configuration.
func desiredChannels(ctx context.Context) (map[string]desiredChannel, error) {
	rows, err := dao.NewChatChannel().ListActive(ctx, dao.DB)
	if err != nil {
		return nil, err
	}
	out := make(map[string]desiredChannel, len(rows))
	for _, row := range rows {
		credential := credentialFromConfig(row.Config)
		out[row.ID] = desiredChannel{
			channel:    row.Channel,
			credential: credential,
			fp:         fingerprint(row.Channel, credential),
		}
	}
	return out, nil
}

// credentialFromConfig extracts the platform credential block from chat_channel.config.
func credentialFromConfig(config entity.JSONMap) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	if raw, ok := config["credential"].(map[string]any); ok {
		return raw
	}
	if raw, ok := config["credential"].(entity.JSONMap); ok {
		return raw
	}
	return map[string]any{}
}

// fingerprint returns a stable hash for the configuration that requires a channel restart.
func fingerprint(channel string, credential map[string]any) string {
	payload, _ := json.Marshal(map[string]any{
		"channel":    channel,
		"credential": credential,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// startChannel builds a platform channel, attaches the RAG bridge, and starts it.
func (r *Runtime) startChannel(ctx context.Context, accountID string, wanted desiredChannel) error {
	ch, err := buildChannel(accountID, wanted)
	if err != nil {
		return err
	}
	if ch == nil {
		return nil
	}

	channelService := service.NewChatChannelService()
	ch.SetMessageHandler(func(ctx context.Context, msg core.IncomingMessage) error {
		answer, err := channelService.HandleIncomingMessage(ctx, service.ChatChannelIncomingMessage{
			Channel:   msg.Channel,
			AccountID: msg.AccountID,
			ChatID:    msg.ChatID,
			ChatType:  msg.ChatType,
			MessageID: msg.MessageID,
			SenderID:  msg.SenderID,
			Text:      msg.Text,
		})
		if err != nil || answer == "" {
			return err
		}
		return ch.Send(ctx, core.OutgoingMessage{
			ChatID:           msg.ChatID,
			Text:             answer,
			ReplyToMessageID: msg.MessageID,
		})
	})
	if err = ch.Start(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	r.running[accountID] = runningChannel{channel: ch, fp: wanted.fp}
	r.mu.Unlock()
	log.Printf("started chat channel %s:%s", ch.ChannelID(), accountID)
	return nil
}

// buildChannel constructs the platform-specific channel implementation for one chat_channel row.
func buildChannel(accountID string, wanted desiredChannel) (core.Channel, error) {
	switch wanted.channel {
	case "whatsapp":
		return whatsapp.NewChannelFromConfig(accountID, wanted.credential)
	default:
		return nil, nil
	}
}

// stopAll stops every running channel and shuts down shared gateway processes.
func (r *Runtime) stopAll(ctx context.Context) {
	r.mu.Lock()
	running := r.running
	r.running = map[string]runningChannel{}
	r.mu.Unlock()
	for _, entry := range running {
		stopChannel(ctx, entry.channel)
	}
	_ = whatsapp.SyncGateway(ctx, false)
}

// stopChannel stops one platform channel and logs any shutdown error.
func stopChannel(ctx context.Context, ch core.Channel) {
	if err := ch.Stop(ctx); err != nil {
		log.Printf("failed to stop chat channel %s:%s: %v", ch.ChannelID(), ch.AccountID(), err)
	}
}
