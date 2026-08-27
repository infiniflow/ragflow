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
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"ragflow/internal/channels/core"
)

const (
	defaultFeishuDomain  = "feishu"
	defaultFeishuTimeout = 30 * time.Second
	feishuMessageTTL     = time.Hour
	feishuQueueSize      = 64
	feishuWorkerIdle     = 5 * time.Minute
)

type feishuAccount struct {
	AccountID string
	AppID     string
	AppSecret string
	Domain    string
	Timeout   time.Duration
}

type feishuChannel struct {
	account feishuAccount

	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	handler  core.MessageHandler
	rest     *lark.Client
	wsClient *larkws.Client
	seen     map[string]time.Time
	workers  map[string]*feishuWorker
}

type feishuWorker struct {
	queue chan core.IncomingMessage
}

// newFeishuChannel creates a Feishu channel backed by the official Lark SDK.
func newFeishuChannel(account feishuAccount) *feishuChannel {
	if account.Domain == "" {
		account.Domain = defaultFeishuDomain
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultFeishuTimeout
	}
	ch := &feishuChannel{
		account: account,
		ctx:     context.Background(),
		seen:    map[string]time.Time{},
		workers: map[string]*feishuWorker{},
	}
	ch.rest = lark.NewClient(
		account.AppID,
		account.AppSecret,
		lark.WithOpenBaseUrl(feishuBaseURL(account.Domain)),
		lark.WithLogLevel(larkcore.LogLevelInfo),
		lark.WithReqTimeout(account.Timeout),
	)
	ch.wsClient = larkws.NewClient(
		account.AppID,
		account.AppSecret,
		larkws.WithDomain(feishuBaseURL(account.Domain)),
		larkws.WithEventHandler(ch.eventDispatcher()),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	return ch
}

// newFeishuChannelFromConfig builds a Feishu channel from chat_channel.config.credential.
func newFeishuChannelFromConfig(accountID string, cfg map[string]any) (*feishuChannel, error) {
	appID := firstString(cfg, "app_id", "appId")
	appSecret := firstString(cfg, "app_secret", "appSecret")
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("feishu account %q is missing app_id or app_secret", accountID)
	}

	timeout := defaultFeishuTimeout
	if raw, ok := cfg["timeout_secs"]; ok {
		if parsed := feishuDurationSeconds(raw); parsed > 0 {
			timeout = parsed
		}
	}

	return newFeishuChannel(feishuAccount{
		AccountID: accountID,
		AppID:     appID,
		AppSecret: appSecret,
		Domain:    valueOrDefault(firstString(cfg, "domain"), defaultFeishuDomain),
		Timeout:   timeout,
	}), nil
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *feishuChannel) ChannelID() string {
	return "feishu"
}

// AccountID returns the chat_channel.id bound to this Feishu runtime instance.
func (c *feishuChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound Feishu messages.
func (c *feishuChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start begins the Feishu WebSocket event client.
func (c *feishuChannel) Start(ctx context.Context) error {
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

// Stop cancels the Feishu WebSocket client and clears transient workers.
func (c *feishuChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	wsClient := c.wsClient

	c.cancel = nil
	c.seen = map[string]time.Time{}
	c.workers = map[string]*feishuWorker{}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if wsClient != nil {
		wsClient.Close()
	}
	return nil
}

// Send posts an outgoing RAGFlow answer to Feishu.
func (c *feishuChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	if strings.TrimSpace(msg.Text) == "" {
		return nil
	}

	content, _ := json.Marshal(map[string]string{"text": msg.Text})
	contentText := string(content)
	msgType := "text"

	if strings.TrimSpace(msg.ReplyToMessageID) != "" {
		resp, err := c.rest.Im.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().
			MessageId(msg.ReplyToMessageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(contentText).
				Build()).
			Build())
		if err != nil {
			return err
		}
		return feishuResponseError("reply", resp.Code, resp.Msg)
	}

	resp, err := c.rest.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(msgType).
			Content(contentText).
			Build()).
		Build())
	if err != nil {
		return err
	}
	return feishuResponseError("create", resp.Code, resp.Msg)
}

// run keeps the SDK WebSocket client active until the channel stops.
func (c *feishuChannel) run(ctx context.Context) {
	if c.wsClient == nil {
		log.Printf("[feishu:%s] WebSocket client is not initialized", c.account.AccountID)
		return
	}
	if err := c.wsClient.Start(ctx); err != nil && ctx.Err() == nil {
		log.Printf("[feishu:%s] WebSocket client exited: %v", c.account.AccountID, err)
	}
}

// eventDispatcher builds the Feishu event handler used by the WebSocket client.
func (c *feishuChannel) eventDispatcher() *dispatcher.EventDispatcher {
	return dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(
		func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			incoming, ok := c.normalizeMessage(event)
			if !ok {
				return nil
			}
			c.enqueueIncoming(incoming)
			return nil
		},
	)
}

// normalizeMessage converts a Feishu SDK message event to the chat-channel message model.
func (c *feishuChannel) normalizeMessage(event *larkim.P2MessageReceiveV1) (core.IncomingMessage, bool) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return core.IncomingMessage{}, false
	}

	message := event.Event.Message
	messageID := stringValue(message.MessageId)
	if messageID == "" {
		return core.IncomingMessage{}, false
	}

	raw := map[string]any{}
	if data, err := json.Marshal(event); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	return core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    stringValue(message.ChatId),
		ChatType:  stringValue(message.ChatType),
		MessageID: messageID,
		SenderID:  feishuSenderID(event.Event.Sender),
		Text:      feishuMessageText(message.Content),
		Raw:       raw,
	}, true
}

// enqueueIncoming schedules Feishu message handling and marks it seen after a successful handoff.
func (c *feishuChannel) enqueueIncoming(incoming core.IncomingMessage) bool {
	now := time.Now()
	var worker *feishuWorker
	startWorker := false
	queueFull := false

	c.mu.Lock()
	c.pruneSeenLocked(now)
	if _, ok := c.seen[incoming.MessageID]; ok {
		c.mu.Unlock()
		return false
	}

	if c.workers == nil {
		c.workers = map[string]*feishuWorker{}
	}
	worker = c.workers[incoming.ChatID]
	if worker == nil {
		worker = &feishuWorker{queue: make(chan core.IncomingMessage, feishuQueueSize)}
		c.workers[incoming.ChatID] = worker
		startWorker = true
	}

	select {
	case worker.queue <- incoming:
		c.seen[incoming.MessageID] = now
		c.mu.Unlock()
		if startWorker {
			go c.runWorker(c.ctx, incoming.ChatID, worker)
		}
		return true
	case <-c.ctx.Done():
		c.mu.Unlock()
	default:
		queueFull = true
		c.mu.Unlock()
	}

	if startWorker {
		go c.runWorker(c.ctx, incoming.ChatID, worker)
	}
	if queueFull {
		log.Printf("[feishu:%s] dropping message %s for chat %s: queue is full", c.account.AccountID, incoming.MessageID, incoming.ChatID)
	}
	return false
}

// runWorker processes one Feishu chat's inbound messages sequentially.
func (c *feishuChannel) runWorker(ctx context.Context, chatID string, worker *feishuWorker) {
	idle := time.NewTimer(feishuWorkerIdle)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-worker.queue:
			if ctx.Err() != nil {
				return
			}
			handlerCtx, cancel := context.WithTimeout(ctx, c.account.Timeout)
			c.handleIncoming(handlerCtx, msg)
			cancel()
			resetTimer(idle, feishuWorkerIdle)
		case <-idle.C:
			if c.retireWorker(chatID, worker) {
				return
			}
			resetTimer(idle, feishuWorkerIdle)
		}
	}
}

// retireWorker removes an idle Feishu chat worker only while it is current and empty.
func (c *feishuChannel) retireWorker(chatID string, worker *feishuWorker) bool {
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

// handleIncoming invokes the installed RAGFlow bridge for one queued Feishu message.
func (c *feishuChannel) handleIncoming(ctx context.Context, incoming core.IncomingMessage) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[feishu:%s] message handler error: %v", c.account.AccountID, err)
	}
}

// pruneSeenLocked removes expired duplicate-tracking entries while c.mu is held.
func (c *feishuChannel) pruneSeenLocked(now time.Time) {
	for key, ts := range c.seen {
		if now.Sub(ts) > feishuMessageTTL {
			delete(c.seen, key)
		}
	}
}

// feishuMessageText extracts plain text from Feishu's JSON-encoded message content.
func feishuMessageText(content *string) string {
	raw := stringValue(content)
	if raw == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	if text, ok := payload["text"].(string); ok {
		return text
	}
	return ""
}

// feishuSenderID returns the sender open_id from a Feishu event sender.
func feishuSenderID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	return stringValue(sender.SenderId.OpenId)
}

// feishuBaseURL maps Feishu/Lark domain config to the SDK base URL.
func feishuBaseURL(domain string) string {
	if strings.EqualFold(strings.TrimSpace(domain), "lark") {
		return lark.LarkBaseUrl
	}
	return lark.FeishuBaseUrl
}

// feishuResponseError returns an error when a Feishu SDK response has a non-zero code.
func feishuResponseError(action string, code int, message string) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("feishu %s failed: code=%d msg=%s", action, code, message)
}

// feishuDurationSeconds converts a config value measured in seconds to a duration.
func feishuDurationSeconds(value any) time.Duration {
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

// stringValue dereferences an optional string.
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
