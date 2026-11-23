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
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/channels/core"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

const (
	reconcileInterval       = 10 * time.Second
	channelFinalSendTimeout = 30 * time.Second
	channelShutdownTimeout  = 30 * time.Second
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
	completionClient *service.ChatChannelRESTClient
	mu               sync.Mutex
	running          map[string]runningChannel
	failed           map[string]failedChannel
	cancel           context.CancelFunc
	done             chan struct{}
	stopping         bool
	messages         sync.WaitGroup
	statusCache      *service.ChatChannelRuntimeCache

	gatewayAttempts    int
	gatewayNextRetryAt time.Time
}

// NewRuntime creates an empty chat-channel runtime reconciler.
func NewRuntime() *Runtime {
	return &Runtime{
		running: map[string]runningChannel{},
		failed:  map[string]failedChannel{},
		done:    make(chan struct{}),
	}
}

// Start launches reconciliation using the API for channel execution.
func Start(ctx context.Context, apiURL string) (*Runtime, error) {
	channelRuntime := NewRuntime()
	channelRuntime.completionClient = service.NewChatChannelRESTClient(apiURL)
	if kvrocks.IsEnabled() {
		channelRuntime.statusCache = service.NewChatChannelRuntimeCache(kvrocks.Get().GetClient())
	}

	if _, err := desiredChannels(ctx); err != nil {
		return channelRuntime, fmt.Errorf("failed to load chat channels: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	channelRuntime.cancel = cancel
	go channelRuntime.Run(runCtx)
	return channelRuntime, nil
}

// Stop cancels reconciliation and waits for adapter cleanup and active messages.
func (r *Runtime) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	<-r.done
}

// Run reconciles configured chat channels until the context is cancelled.
func (r *Runtime) Run(ctx context.Context) {
	defer close(r.done)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelShutdownTimeout)
		defer cancel()
		r.stopAll(shutdownCtx)
	}()

	// check channel's status periodically
	for {
		if ctx.Err() != nil {
			return
		}
		if err := r.Reconcile(ctx); err != nil {
			common.Error("chat channel reconcile failed", err)
		}
		r.publishRuntimeSnapshots(ctx)
		delay := reconcileInterval
		r.mu.Lock()
		if remaining := time.Until(r.gatewayNextRetryAt); remaining > 0 && remaining < delay {
			delay = remaining
		}
		for _, failure := range r.failed {
			if remaining := time.Until(failure.nextRetryAt); remaining > 0 && remaining < delay {
				delay = remaining
			}
		}
		r.mu.Unlock()
		if !waitForContext(ctx, delay) {
			return
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
	var removedIDs []string
	r.mu.Lock()
	for accountID, entry := range r.running {
		wanted, ok := desired[accountID]
		if !ok || wanted.fingerprint != entry.fingerprint {
			delete(r.running, accountID)
			toStop = append(toStop, entry.channel)
			removedIDs = append(removedIDs, accountID)
		}
	}

	for accountID, failure := range r.failed {
		wanted, ok := desired[accountID]
		if !ok || wanted.fingerprint != failure.fingerprint {
			delete(r.failed, accountID)
		}
	}
	r.mu.Unlock()

	for _, accountID := range removedIDs {
		r.deleteRuntimeSnapshot(ctx, accountID)
	}
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
	r.mu.Lock()
	gatewayRetryPending := time.Now().Before(r.gatewayNextRetryAt)
	r.mu.Unlock()
	if !activeWhatsApp || !gatewayRetryPending {
		err := syncWhatsAppGateway(ctx, activeWhatsApp)
		r.mu.Lock()
		if err != nil && activeWhatsApp {
			r.gatewayAttempts++
			r.gatewayNextRetryAt = time.Now().Add(channelRetryDelay(r.gatewayAttempts))
		} else {
			r.gatewayAttempts = 0
			r.gatewayNextRetryAt = time.Time{}
		}
		r.mu.Unlock()
		if err != nil && activeWhatsApp {
			common.Error("failed to sync WhatsApp gateway", err)
			reconcileErr = fmt.Errorf("sync WhatsApp gateway: %w", err)
		}
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
		if err := r.startChannel(ctx, accountID, wanted); err != nil {
			common.Error("failed to start chat channel", err, zap.String("account_id", accountID), zap.String("channel", wanted.channel))
			r.recordStartFailure(accountID, wanted.fingerprint, time.Now())
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
		nextRetryAt: now.Add(channelRetryDelay(attempts)),
	}
}

// clearStartFailure removes a stale failed-start record after a successful start.
func (r *Runtime) clearStartFailure(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failed, accountID)
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
			fingerprint: service.ChatChannelRuntimeFingerprint(row.Channel, credential),
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

// startChannel builds a platform channel, attaches the RAG bridge, and starts it.
func (r *Runtime) startChannel(ctx context.Context, accountID string, wanted desiredChannel) error {
	ch, err := buildChannel(accountID, wanted)
	if err != nil {
		return err
	}
	if ch == nil {
		return nil
	}

	ch.SetMessageHandler(func(ctx context.Context, msg core.IncomingMessage) error {
		r.mu.Lock()
		if r.stopping {
			r.mu.Unlock()
			return context.Canceled
		}
		r.messages.Add(1)
		r.mu.Unlock()
		defer r.messages.Done()
		if err := ctx.Err(); err != nil {
			return err
		}
		outgoing := core.OutgoingMessage{ChatID: msg.ChatID, ReplyToMessageID: msg.MessageID}

		// A streaming channel owns its own message lifecycle: cumulative text goes
		// to SendStream, and the bridge sends the complete answer if streaming fails.
		var onDelta func(string)
		var text chan string
		var sent chan error
		if streamer, ok := ch.(core.StreamingChannel); ok && streamer.StreamEnabled() {
			text = make(chan string, 1)
			sent = make(chan error, 1)
			go func() {
				sent <- streamer.SendStream(ctx, outgoing, text)
			}()
			onDelta = func(value string) {
				// Keep only the newest cumulative snapshot; the channel must not
				// build a backlog while the platform renders older text.
				select {
				case text <- value:
				default:
					select {
					case <-text:
					default:
					}
					select {
					case text <- value:
					default:
					}
				}
			}
		}

		answer, err := r.completionClient.HandleIncomingMessage(ctx, service.ChatChannelIncomingMessage{
			Channel:   msg.Channel,
			AccountID: msg.AccountID,
			ChatID:    msg.ChatID,
			ChatType:  msg.ChatType,
			MessageID: msg.MessageID,
			SenderID:  msg.SenderID,
			Text:      msg.Text,
		}, onDelta)

		if text != nil {
			// Streaming updates may have been coalesced. Make the final snapshot
			// available before closing so a successful stream always has the full answer.
			if answer != "" {
				onDelta(answer)
			}
			close(text)
			streamErr := <-sent
			if streamErr == nil {
				return err
			}
			if err == nil && answer == "" {
				// The model produced nothing, so there is nothing to deliver.
				return nil
			}
			common.Warn("chat channel stream failed, sending the answer as one message", zap.Error(streamErr), zap.String("channel", ch.ChannelID()), zap.String("account_id", accountID))
		}
		if err != nil || answer == "" {
			return err
		}
		// The turn's context may already be past its deadline, so the fallback send gets a
		// context that cannot be cancelled by it.
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelFinalSendTimeout)
		defer cancel()
		outgoing.Text = answer
		return ch.Send(sendCtx, outgoing)
	})
	if err = ch.Start(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	r.running[accountID] = runningChannel{channel: ch, fingerprint: wanted.fingerprint}
	r.mu.Unlock()
	common.Info("started chat channel", zap.String("channel", ch.ChannelID()), zap.String("account_id", accountID))
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
	r.stopping = true
	r.mu.Unlock()
	for accountID, entry := range running {
		r.deleteRuntimeSnapshot(ctx, accountID)
		stopChannel(ctx, entry.channel)
	}
	_ = syncWhatsAppGateway(ctx, false)
	drained := make(chan struct{})
	go func() {
		r.messages.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
		common.Warn("chat channel shutdown timed out waiting for active messages")
	}
}

func (r *Runtime) publishRuntimeSnapshots(ctx context.Context) {
	if r.statusCache == nil {
		return
	}
	r.mu.Lock()
	running := make(map[string]string, len(r.running))
	for accountID, entry := range r.running {
		if entry.channel.ChannelID() == "whatsapp" {
			running[accountID] = entry.fingerprint
		}
	}
	r.mu.Unlock()
	for accountID, configFingerprint := range running {
		snapshot, ok := whatsappRuntimeSnapshotMap(accountID)
		if !ok {
			r.deleteRuntimeSnapshot(ctx, accountID)
			continue
		}
		if err := r.statusCache.Publish(ctx, accountID, configFingerprint, snapshot); err != nil {
			common.Warn("failed to publish chat-channel runtime status", zap.Error(err), zap.String("account_id", accountID))
		}
	}
}

func (r *Runtime) deleteRuntimeSnapshot(ctx context.Context, accountID string) {
	if r.statusCache != nil {
		if err := r.statusCache.Delete(ctx, accountID); err != nil {
			common.Warn("failed to clear chat-channel runtime status", zap.Error(err), zap.String("account_id", accountID))
		}
	}
}

// stopChannel stops one platform channel and logs any shutdown error.
func stopChannel(ctx context.Context, ch core.Channel) {
	if err := ch.Stop(ctx); err != nil {
		common.Error("failed to stop chat channel", err, zap.String("channel", ch.ChannelID()), zap.String("account_id", ch.AccountID()))
	}
}
