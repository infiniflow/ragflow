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
	"fmt"
	"log"
	"sync"
	"time"

	"ragflow/internal/channels/core"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

const (
	reconcileInterval      = 10 * time.Second
	initialStartRetryDelay = 10 * time.Second
	maxStartRetryDelay     = 5 * time.Minute
)

type runningChannel struct {
	channel     core.Channel
	fingerprint string
}

type failedChannel struct {
	fingerprint string
	attempts    int
	nextRetryAt time.Time
}

type Runtime struct {
	mu      sync.Mutex
	running map[string]runningChannel
	failed  map[string]failedChannel
}

// NewRuntime creates an empty chat-channel runtime reconciler.
func NewRuntime() *Runtime {
	return &Runtime{
		running: map[string]runningChannel{},
		failed:  map[string]failedChannel{},
	}
}

// Start launches the chat-channel runtime reconciler in the background.
func Start(ctx context.Context) (*Runtime, error) {
	channelRuntime := NewRuntime()
	service.SetChatChannelRuntimeProvider(whatsappRuntimeSnapshotMap)

	if err := channelRuntime.Reconcile(ctx); err != nil {
		return channelRuntime, fmt.Errorf("failed to reconcile channel: %w", err)
	}

	go channelRuntime.Run(ctx)
	return channelRuntime, nil
}

// Run reconciles configured chat channels until the context is cancelled.
func (r *Runtime) Run(ctx context.Context) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	defer r.stopAll(ctx)

	// check channel's status periodically
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
	var reconcileErr error
	desired, err := desiredChannels(ctx)
	if err != nil {
		return err
	}

	var toStop []core.Channel
	r.mu.Lock()
	for accountID, entry := range r.running {
		wanted, ok := desired[accountID]
		if !ok || wanted.fingerprint != entry.fingerprint {
			delete(r.running, accountID)
			toStop = append(toStop, entry.channel)
		}
	}

	for accountID, failure := range r.failed {
		wanted, ok := desired[accountID]
		if !ok || wanted.fingerprint != failure.fingerprint {
			delete(r.failed, accountID)
		}
	}
	r.mu.Unlock()

	for _, ch := range toStop {
		stopChannel(ctx, ch)
	}

	activeWhatsApp := false
	for _, wanted := range desired {
		if wanted.channel == "whatsapp" {
			activeWhatsApp = true
			break
		}
	}
	if err = syncWhatsAppGateway(ctx, activeWhatsApp); err != nil && activeWhatsApp {
		log.Printf("failed to sync WhatsApp gateway: %v", err)
		reconcileErr = fmt.Errorf("sync WhatsApp gateway: %w", err)
	}

	now := time.Now()
	for accountID, wanted := range desired {
		r.mu.Lock()
		_, isRunning := r.running[accountID]
		failure, failed := r.failed[accountID]
		retryPending := failed && failure.fingerprint == wanted.fingerprint && now.Before(failure.nextRetryAt)
		r.mu.Unlock()
		if isRunning || retryPending {
			continue
		}
		if err = r.startChannel(ctx, accountID, wanted); err != nil {
			log.Printf("failed to start chat channel %s (%s): %v", accountID, wanted.channel, err)
			r.recordStartFailure(accountID, wanted.fingerprint, now)
			if reconcileErr == nil {
				reconcileErr = fmt.Errorf("failed to start chat channel %s: %w", accountID, err)
			}
			continue
		}
		r.clearStartFailure(accountID)
	}
	return reconcileErr
}

// recordStartFailure saves the next retry window for a failed channel start.
func (r *Runtime) recordStartFailure(accountID string, fingerprint string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	attempts := 1
	if failure, ok := r.failed[accountID]; ok && failure.fingerprint == fingerprint {
		attempts = failure.attempts + 1
	}
	r.failed[accountID] = failedChannel{
		fingerprint: fingerprint,
		attempts:    attempts,
		nextRetryAt: now.Add(startRetryDelay(attempts)),
	}
}

// clearStartFailure removes a stale failed-start record after a successful start.
func (r *Runtime) clearStartFailure(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failed, accountID)
}

// startRetryDelay returns the bounded exponential backoff for a start attempt.
func startRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := initialStartRetryDelay
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= maxStartRetryDelay {
			return maxStartRetryDelay
		}
	}
	return delay
}

type desiredChannel struct {
	channel     string
	credential  map[string]any
	fingerprint string
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
			channel:     row.Channel,
			credential:  credential,
			fingerprint: fingerprint(row.Channel, credential),
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
	r.running[accountID] = runningChannel{channel: ch, fingerprint: wanted.fingerprint}
	r.mu.Unlock()
	log.Printf("started chat channel %s:%s", ch.ChannelID(), accountID)
	return nil
}

// buildChannel constructs the platform-specific channel implementation for one chat_channel row.
func buildChannel(accountID string, wanted desiredChannel) (core.Channel, error) {
	switch wanted.channel {
	case "feishu":
		return newFeishuChannelFromConfig(accountID, wanted.credential)
	case "discord":
		return newDiscordChannelFromConfig(accountID, wanted.credential)
	case "qqbot":
		return newQQBotChannelFromConfig(accountID, wanted.credential)
	case "whatsapp":
		return newWhatsAppChannelFromConfig(accountID, wanted.credential)
	case "line":
		return newLineChannelFromConfig(accountID, wanted.credential)
	case "telegram":
		return newTelegramChannelFromConfig(accountID, wanted.credential)
	case "wecom":
		return newWeComChannelFromConfig(accountID, wanted.credential)
	case "dingtalk":
		return newDingTalkChannelFromConfig(accountID, wanted.credential)
	default:
		return nil, fmt.Errorf("unknown channel: %s", wanted.channel)
	}
}

// whatsappRuntimeSnapshotMap returns the API payload for a live WhatsApp runtime snapshot.
func whatsappRuntimeSnapshotMap(accountID string) (map[string]any, bool) {
	snapshot, ok := getWhatsAppRuntimeSnapshot(accountID)
	if !ok {
		return nil, false
	}
	return map[string]any{
		"account_id":       snapshot.AccountID,
		"session_key":      snapshot.SessionKey,
		"status":           snapshot.Status,
		"connected_at":     snapshot.ConnectedAt,
		"qr_updated_at":    snapshot.QRUpdatedAt,
		"qr_data_url":      snapshot.QRDataURL,
		"last_error":       snapshot.LastError,
		"session_id":       snapshot.SessionID,
		"last_snapshot_at": snapshot.LastSnapshotAt,
		"gateway_base_url": snapshot.GatewayBaseURL,
		"event_cursor":     snapshot.EventCursor,
	}, true
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
	_ = syncWhatsAppGateway(ctx, false)
}

// stopChannel stops one platform channel and logs any shutdown error.
func stopChannel(ctx context.Context, ch core.Channel) {
	if err := ch.Stop(ctx); err != nil {
		log.Printf("failed to stop chat channel %s:%s: %v", ch.ChannelID(), ch.AccountID(), err)
	}
}
