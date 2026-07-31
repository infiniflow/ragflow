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
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/clientV2"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"

	"ragflow/internal/channels/core"
)

const (
	defaultDingTalkAPIBaseURL = "https://api.dingtalk.com"
	defaultDingTalkTimeout    = 30 * time.Second
	dingTalkMessageTTL        = time.Hour
	dingTalkQueueSize         = 64
	dingTalkWorkerIdle        = 5 * time.Minute
)

type dingTalkAccount struct {
	AccountID    string
	ClientID     string
	ClientSecret string
	APIBaseURL   string
	Timeout      time.Duration
}

type dingTalkChannel struct {
	account dingTalkAccount

	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	handler         core.MessageHandler
	client          *http.Client
	stream          clientV2.OpenDingTalkClient
	sessionWebhooks map[string]string
	processed       map[string]time.Time
	inflight        map[string]time.Time
	workers         map[string]*dingTalkWorker
}

type dingTalkWorker struct {
	queue chan dingTalkQueuedMessage
}

type dingTalkQueuedMessage struct {
	incoming core.IncomingMessage
	dedupKey string
}

// newDingTalkChannel creates a DingTalk stream channel backed by the official SDK.
func newDingTalkChannel(account dingTalkAccount) *dingTalkChannel {
	if account.APIBaseURL == "" {
		account.APIBaseURL = defaultDingTalkAPIBaseURL
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultDingTalkTimeout
	}
	ch := &dingTalkChannel{
		account:         account,
		client:          &http.Client{Timeout: account.Timeout},
		sessionWebhooks: map[string]string{},
		processed:       map[string]time.Time{},
		inflight:        map[string]time.Time{},
		workers:         map[string]*dingTalkWorker{},
	}
	ch.stream = clientV2.NewBuilder().
		Credential(&clientV2.AuthClientCredential{ClientId: account.ClientID, ClientSecret: account.ClientSecret}).
		RegisterCallbackHandler(payload.BotMessageCallbackTopic, ch.handleBotCallback).
		Build()
	return ch
}

// newDingTalkChannelFromConfig builds a DingTalk channel from chat_channel.config.credential.
func newDingTalkChannelFromConfig(accountID string, cfg map[string]any) (*dingTalkChannel, error) {
	clientID := firstString(cfg, "client_id", "clientId")
	clientSecret := firstString(cfg, "client_secret", "clientSecret")
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("dingtalk account %q is missing client_id or client_secret", accountID)
	}

	timeout := defaultDingTalkTimeout
	if raw, ok := cfg["timeout_secs"]; ok {
		if parsed := dingTalkDurationSeconds(raw); parsed > 0 {
			timeout = parsed
		}
	}

	return newDingTalkChannel(dingTalkAccount{
		AccountID:    accountID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		APIBaseURL:   firstString(cfg, "api_base_url", "api_url"),
		Timeout:      timeout,
	}), nil
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *dingTalkChannel) ChannelID() string {
	return "dingtalk"
}

// AccountID returns the chat_channel.id bound to this DingTalk runtime instance.
func (c *dingTalkChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound DingTalk messages.
func (c *dingTalkChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start begins the DingTalk official stream client.
func (c *dingTalkChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.ctx = runCtx
	c.cancel = cancel
	c.mu.Unlock()

	go c.run(runCtx)
	return nil
}

// Stop cancels the DingTalk stream client and clears transient runtime state.
func (c *dingTalkChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.ctx = nil
	c.sessionWebhooks = map[string]string{}
	c.processed = map[string]time.Time{}
	c.inflight = map[string]time.Time{}
	c.workers = map[string]*dingTalkWorker{}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

// Send posts an outgoing RAGFlow answer to DingTalk through the cached sessionWebhook.
func (c *dingTalkChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	if strings.TrimSpace(msg.Text) == "" {
		return nil
	}

	c.mu.Lock()
	sessionWebhook := c.sessionWebhooks[msg.ChatID]
	c.mu.Unlock()
	if strings.TrimSpace(sessionWebhook) == "" {
		log.Printf("[dingtalk:%s] no sessionWebhook cached for chat_id=%s; dropping reply", c.account.AccountID, msg.ChatID)
		return nil
	}

	p := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "RAGFlow",
			"text":  msg.Text,
		},
	}
	return c.postWebhook(ctx, sessionWebhook, p)
}

// run keeps the DingTalk SDK stream client active until the channel stops.
func (c *dingTalkChannel) run(ctx context.Context) {
	defer func() {
		var runCancel context.CancelFunc
		c.mu.Lock()
		if c.ctx == ctx {
			runCancel = c.cancel
		}
		c.mu.Unlock()
		if runCancel != nil {
			runCancel()
		}

		c.mu.Lock()
		if c.ctx == ctx {
			c.cancel = nil
			c.ctx = nil
		}
		c.mu.Unlock()
	}()
	if c.stream == nil {
		log.Printf("[dingtalk:%s] stream client is not initialized", c.account.AccountID)
		return
	}
	if err := c.stream.Start(ctx); err != nil && ctx.Err() == nil {
		log.Printf("[dingtalk:%s] stream client exited: %v", c.account.AccountID, err)
	}
}

// handleBotCallback receives DingTalk bot messages from the official stream SDK.
func (c *dingTalkChannel) handleBotCallback(data *chatbot.BotCallbackDataModel) (*chatbot.BotCallbackRespModel, error) {
	c.mu.Lock()
	ctx := c.ctx
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	incoming, dedupKey, ok := c.normalizeBotMessage(data)
	if !ok {
		return &chatbot.BotCallbackRespModel{}, nil
	}
	if dedupKey != "" && !c.beginMessage(dedupKey) {
		log.Printf("[dingtalk:%s] skipping duplicate message=%s", c.account.AccountID, dedupKey)
		return &chatbot.BotCallbackRespModel{}, nil
	}
	if !c.enqueueIncoming(ctx, dingTalkQueuedMessage{incoming: incoming, dedupKey: dedupKey}) && dedupKey != "" {
		c.cancelInflight(dedupKey)
	}
	return &chatbot.BotCallbackRespModel{}, nil
}

// normalizeBotMessage converts DingTalk SDK bot callback data to the chat-channel message model.
func (c *dingTalkChannel) normalizeBotMessage(data *chatbot.BotCallbackDataModel) (core.IncomingMessage, string, bool) {
	if data == nil {
		return core.IncomingMessage{}, "", false
	}
	chatID := dingTalkFirstNonEmpty(data.ConversationId)
	senderID := dingTalkFirstNonEmpty(data.SenderId, data.SenderStaffId)
	text := strings.TrimSpace(data.Text.Content)
	if text == "" {
		text = dingTalkExtractText(data.Content)
	}
	if data.SessionWebhook != "" && chatID != "" {
		c.saveSessionWebhook(chatID, data.SessionWebhook)
	}
	if text == "" || chatID == "" || senderID == "" {
		return core.IncomingMessage{}, "", false
	}

	raw := map[string]any{}
	if encoded, err := json.Marshal(data); err == nil {
		_ = json.Unmarshal(encoded, &raw)
	}
	dedupKey := dingTalkBotDedupKey(data, text)
	return core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    chatID,
		ChatType:  data.ConversationType,
		MessageID: dingTalkFirstNonEmpty(data.MsgId, dedupKey),
		SenderID:  senderID,
		Text:      text,
		Raw:       raw,
	}, dedupKey, true
}

// enqueueIncoming schedules DingTalk message handling and marks duplicates as processed after handling.
func (c *dingTalkChannel) enqueueIncoming(ctx context.Context, item dingTalkQueuedMessage) bool {
	if ctx.Err() != nil {
		return false
	}

	var worker *dingTalkWorker
	startWorker := false
	queueFull := false

	c.mu.Lock()
	if c.workers == nil {
		c.workers = map[string]*dingTalkWorker{}
	}
	worker = c.workers[item.incoming.ChatID]
	if worker == nil {
		worker = &dingTalkWorker{queue: make(chan dingTalkQueuedMessage, dingTalkQueueSize)}
		c.workers[item.incoming.ChatID] = worker
		startWorker = true
	}
	select {
	case worker.queue <- item:
		c.mu.Unlock()
		if startWorker {
			go c.runWorker(ctx, item.incoming.ChatID, worker)
		}
		return true
	case <-ctx.Done():
		c.mu.Unlock()
	default:
		queueFull = true
		c.mu.Unlock()
	}
	if startWorker {
		go c.runWorker(ctx, item.incoming.ChatID, worker)
	}
	if queueFull {
		log.Printf("[dingtalk:%s] dropping message %s for chat %s: queue is full", c.account.AccountID, item.incoming.MessageID, item.incoming.ChatID)
	}
	return false
}

// runWorker processes one DingTalk chat's inbound messages sequentially.
func (c *dingTalkChannel) runWorker(ctx context.Context, chatID string, worker *dingTalkWorker) {
	idle := time.NewTimer(dingTalkWorkerIdle)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-worker.queue:
			if ctx.Err() != nil {
				return
			}
			c.handleIncoming(ctx, item.incoming)
			if item.dedupKey != "" {
				c.finishMessage(item.dedupKey)
			}
			resetTimer(idle, dingTalkWorkerIdle)
		case <-idle.C:
			if c.retireWorker(chatID, worker) {
				return
			}
			resetTimer(idle, dingTalkWorkerIdle)
		}
	}
}

// retireWorker removes an idle DingTalk chat worker only while it is current and empty.
func (c *dingTalkChannel) retireWorker(chatID string, worker *dingTalkWorker) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.workers[chatID]
	if current != worker {
		return true
	}
	if len(worker.queue) > 0 {
		return false
	}
	delete(c.workers, chatID)
	return true
}

// handleIncoming invokes the installed RAGFlow bridge for one queued DingTalk message.
func (c *dingTalkChannel) handleIncoming(ctx context.Context, incoming core.IncomingMessage) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[dingtalk:%s] message handler error: %v", c.account.AccountID, err)
	}
}

// beginMessage marks a DingTalk message as in-flight if it is not a processed duplicate.
func (c *dingTalkChannel) beginMessage(dedupKey string) bool {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneMessageCacheLocked(now)
	if _, ok := c.processed[dedupKey]; ok {
		return false
	}
	if _, ok := c.inflight[dedupKey]; ok {
		return false
	}
	c.inflight[dedupKey] = now
	return true
}

// finishMessage moves a DingTalk message from in-flight to processed after handling.
func (c *dingTalkChannel) finishMessage(dedupKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, dedupKey)
	c.processed[dedupKey] = time.Now()
}

// cancelInflight removes an in-flight duplicate key when enqueue fails.
func (c *dingTalkChannel) cancelInflight(dedupKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, dedupKey)
}

// pruneMessageCacheLocked removes expired duplicate-tracking entries while c.mu is held.
func (c *dingTalkChannel) pruneMessageCacheLocked(now time.Time) {
	cutoff := now.Add(-dingTalkMessageTTL)
	for key, ts := range c.processed {
		if ts.Before(cutoff) {
			delete(c.processed, key)
		}
	}
	for key, ts := range c.inflight {
		if ts.Before(cutoff) {
			delete(c.inflight, key)
		}
	}
}

// saveSessionWebhook caches the DingTalk reply webhook for one conversation.
func (c *dingTalkChannel) saveSessionWebhook(chatID, sessionWebhook string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionWebhooks[chatID] = sessionWebhook
}

// postWebhook posts a markdown reply payload to a DingTalk sessionWebhook.
func (c *dingTalkChannel) postWebhook(ctx context.Context, sessionWebhook string, payload map[string]any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sessionWebhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk reply status: %d, response: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// dingTalkExtractText extracts user text from DingTalk message payload variants.
func dingTalkExtractText(data map[string]any) string {
	text := data["text"]
	if textMap := dingTalkMap(text); len(textMap) > 0 {
		if content := dingTalkString(textMap["content"]); content != "" {
			return content
		}
	}
	if content := dingTalkString(text); content != "" {
		return content
	}

	content := data["content"]
	if contentMap := dingTalkMap(content); len(contentMap) > 0 {
		for _, key := range []string{"text", "content", "recognition"} {
			if value := dingTalkString(contentMap[key]); strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return dingTalkString(content)
}

// dingTalkBotDedupKey builds a stable duplicate key for a DingTalk bot callback payload.
func dingTalkBotDedupKey(data *chatbot.BotCallbackDataModel, text string) string {
	if data == nil {
		return ""
	}
	if data.MsgId != "" {
		return "msg:" + data.MsgId
	}
	if data.ConversationId != "" && dingTalkFirstNonEmpty(data.SenderId, data.SenderStaffId) != "" && strings.TrimSpace(text) != "" {
		sum := sha1.Sum([]byte(text))
		eventTS := ""
		if data.CreateAt > 0 {
			eventTS = ":" + strconv.FormatInt(data.CreateAt, 10)
		}
		return fmt.Sprintf("fallback:%s:%s%s:%s", data.ConversationId, dingTalkFirstNonEmpty(data.SenderId, data.SenderStaffId), eventTS, hex.EncodeToString(sum[:])[:16])
	}
	return ""
}

// dingTalkDurationSeconds converts a config value measured in seconds to a duration.
func dingTalkDurationSeconds(value any) time.Duration {
	switch v := value.(type) {
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return time.Duration(n) * time.Second
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return time.Duration(n) * time.Second
	default:
		return 0
	}
}

// dingTalkString converts common JSON values to strings.
func dingTalkString(value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	return text
}

// dingTalkMap returns value as a string-keyed map when possible.
func dingTalkMap(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

// dingTalkFirstNonEmpty returns the first non-empty string.
func dingTalkFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
