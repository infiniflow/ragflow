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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"go.uber.org/zap"

	"ragflow/internal/channels/core"
	"ragflow/internal/common"
)

const (
	defaultDingTalkAPIBaseURL = "https://api.dingtalk.com"
	defaultDingTalkTimeout    = 30 * time.Second
	dingTalkMessageTTL        = time.Hour
	dingTalkQueueSize         = 64
	dingTalkWorkerIdle        = 5 * time.Minute
	dingTalkStreamTick        = 100 * time.Millisecond
	dingTalkTokenMargin       = 60 * time.Second
)

// errDingTalkNothingStreamed reports that no text was available for a streamed card, so
// the caller still has to deliver the answer.
var errDingTalkNothingStreamed = errors.New("dingtalk: no streamed text to display")

type dingTalkAccount struct {
	AccountID        string
	ClientID         string
	ClientSecret     string
	APIBaseURL       string
	Timeout          time.Duration
	Stream           bool
	CardTemplateID   string
	StreamContentKey string
}

// dingTalkPeer remembers where a conversation lives so an AI card can be delivered to it.
type dingTalkPeer struct {
	chatType      string
	senderStaffID string
}

type dingTalkChannel struct {
	account dingTalkAccount

	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	handler         core.MessageHandler
	client          *http.Client
	stream          *client.StreamClient
	sessionWebhooks map[string]string
	peers           map[string]dingTalkPeer
	processed       map[string]time.Time
	inflight        map[string]time.Time
	workers         map[string]*dingTalkWorker
	tokenMu         sync.Mutex
	accessToken     string
	tokenExpiry     time.Time
}

type dingTalkWorker struct {
	queue chan dingTalkQueuedMessage
}

type dingTalkQueuedMessage struct {
	incoming core.IncomingMessage
	dedupKey string
}

// newDingTalkChannel creates a DingTalk stream channel backed by the official SDK.
func newDingTalkChannel(account dingTalkAccount) (*dingTalkChannel, error) {
	if account.APIBaseURL == "" {
		account.APIBaseURL = defaultDingTalkAPIBaseURL
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultDingTalkTimeout
	}
	if account.StreamContentKey == "" {
		account.StreamContentKey = "content"
	}
	proxyURL, err := dingTalkSafeProxyURL()
	if err != nil {
		return nil, err
	}
	ch := &dingTalkChannel{
		account:         account,
		client:          newSafeChannelHTTPClient(account.Timeout, channelURLPolicy{exactHosts: []string{"oapi.dingtalk.com", "api.dingtalk.com"}}),
		sessionWebhooks: map[string]string{},
		peers:           map[string]dingTalkPeer{},
		processed:       map[string]time.Time{},
		inflight:        map[string]time.Time{},
		workers:         map[string]*dingTalkWorker{},
	}
	ch.stream = client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(account.ClientID, account.ClientSecret)),
		client.WithProxy(proxyURL),
	)
	ch.stream.RegisterChatBotCallbackRouter(ch.handleBotCallback)
	return ch, nil
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
		AccountID:        accountID,
		ClientID:         clientID,
		ClientSecret:     clientSecret,
		APIBaseURL:       firstString(cfg, "api_base_url", "api_url"),
		Timeout:          timeout,
		Stream:           cfg["stream"] == true,
		CardTemplateID:   firstString(cfg, "card_template_id"),
		StreamContentKey: firstString(cfg, "stream_content_key"),
	})
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
	c.peers = map[string]dingTalkPeer{}
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
		common.Warn("dingtalk: no sessionWebhook cached, dropping reply", zap.String("account_id", c.account.AccountID), zap.String("chat_id", msg.ChatID))
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
		common.Warn("dingtalk: stream client is not initialized", zap.String("account_id", c.account.AccountID))
		return
	}
	if err := c.stream.Start(ctx); err != nil && ctx.Err() == nil {
		common.Error("dingtalk: stream client exited", err, zap.String("account_id", c.account.AccountID))
	}
}

// handleBotCallback receives DingTalk bot messages from the official stream SDK.
func (c *dingTalkChannel) handleBotCallback(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	c.mu.Lock()
	runCtx := c.ctx
	c.mu.Unlock()
	if runCtx != nil {
		ctx = runCtx
	} else if ctx == nil {
		ctx = context.Background()
	}
	incoming, dedupKey, ok := c.normalizeBotMessage(data)
	if !ok {
		return []byte(`{"response":{}}`), nil
	}
	if dedupKey != "" && !c.beginMessage(dedupKey) {
		common.Info("dingtalk: skipping duplicate message", zap.String("account_id", c.account.AccountID), zap.String("dedup_key", dedupKey))
		return []byte(`{"response":{}}`), nil
	}
	if !c.enqueueIncoming(ctx, dingTalkQueuedMessage{incoming: incoming, dedupKey: dedupKey}) && dedupKey != "" {
		c.cancelInflight(dedupKey)
	}
	return []byte(`{"response":{}}`), nil
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
	if chatID != "" {
		c.savePeer(chatID, data.ConversationType, data.SenderStaffId)
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
		common.Warn("dingtalk: dropping message, queue is full", zap.String("account_id", c.account.AccountID), zap.String("message_id", item.incoming.MessageID), zap.String("chat_id", item.incoming.ChatID))
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
		common.Error("dingtalk: message handler error", err, zap.String("account_id", c.account.AccountID))
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

// savePeer caches where a conversation lives so streamed cards can be delivered to it.
func (c *dingTalkChannel) savePeer(chatID, chatType, senderStaffID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers[chatID] = dingTalkPeer{chatType: chatType, senderStaffID: senderStaffID}
}

// chatPeer returns the cached delivery target of one conversation.
func (c *dingTalkChannel) chatPeer(chatID string) dingTalkPeer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peers[chatID]
}

// StreamEnabled reports whether this account asked for streamed card replies. Streaming
// needs the AI card template that carries the streaming markdown variable.
func (c *dingTalkChannel) StreamEnabled() bool {
	return c.account.Stream && c.account.CardTemplateID != ""
}

// SendStream replies with an AI card: the card is created on the first text, the answer is
// appended to its streaming variable, and the last update closes the card.
func (c *dingTalkChannel) SendStream(ctx context.Context, msg core.OutgoingMessage, text <-chan string) error {
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}

	var first string
	select {
	case <-ctx.Done():
		return ctx.Err()
	case value, open := <-text:
		if !open || strings.TrimSpace(value) == "" {
			return errDingTalkNothingStreamed
		}
		first = value
	}

	outTrackID, err := c.createAICard(ctx, msg.ChatID)
	if err != nil {
		return err
	}

	latest, sent := first, ""
	ticker := time.NewTicker(dingTalkStreamTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case value, open := <-text:
			if !open {
				// A closed channel returns immediately forever, so stop selecting on it and
				// let the ticker drain what is still unsent.
				text = nil
				continue
			}
			latest = value
		case <-ticker.C:
			if sent != latest {
				if err := c.pushCardDelta(ctx, outTrackID, dingTalkStreamDelta(sent, latest), false); err != nil {
					return err
				}
				sent = latest
				continue
			}
			if text == nil {
				return c.pushCardDelta(ctx, outTrackID, "", true)
			}
		}
	}
}

// dingTalkStreamDelta returns the part of the answer the card has not received yet. A
// rewritten answer is sent whole so the card cannot stall on a stale prefix.
func dingTalkStreamDelta(shown, target string) string {
	if strings.HasPrefix(target, shown) {
		return target[len(shown):]
	}
	return target
}

// createAICard delivers an empty AI card to the conversation and returns its instance id.
func (c *dingTalkChannel) createAICard(ctx context.Context, chatID string) (string, error) {
	peer := c.chatPeer(chatID)
	outTrackID := "ragflow_" + common.GenerateToken()
	body := map[string]any{
		"cardTemplateId": c.account.CardTemplateID,
		"outTrackId":     outTrackID,
		"cardData": map[string]any{"cardParamMap": map[string]any{
			"flowStatus":               "1",
			c.account.StreamContentKey: "",
		}},
		"callbackType":          "STREAM",
		"imGroupOpenSpaceModel": map[string]any{"supportForward": true},
		"imRobotOpenSpaceModel": map[string]any{"supportForward": true},
	}
	if peer.chatType == "2" {
		body["openSpaceId"] = "dtv1.card//IM_GROUP." + chatID
		body["imGroupOpenDeliverModel"] = map[string]any{"robotCode": c.account.ClientID}
	} else {
		if peer.senderStaffID == "" {
			return "", errors.New("dingtalk stream card needs the sender staff id of a one-to-one chat")
		}
		body["openSpaceId"] = "dtv1.card//IM_ROBOT." + peer.senderStaffID
		body["imRobotOpenDeliverModel"] = map[string]any{"spaceType": "IM_ROBOT"}
	}
	token, err := c.openAPIToken(ctx)
	if err != nil {
		return "", err
	}
	if err := c.doOpenAPI(ctx, http.MethodPost, "/v1.0/card/instances/createAndDeliver", token, body, nil); err != nil {
		return "", err
	}
	return outTrackID, nil
}

// pushCardDelta appends content to the card's streaming variable and optionally closes it.
func (c *dingTalkChannel) pushCardDelta(ctx context.Context, outTrackID, content string, finalize bool) error {
	token, err := c.openAPIToken(ctx)
	if err != nil {
		return err
	}
	return c.doOpenAPI(ctx, http.MethodPut, "/v1.0/card/streaming", token, map[string]any{
		"outTrackId": outTrackID,
		"guid":       common.GenerateToken(),
		"key":        c.account.StreamContentKey,
		"content":    content,
		"isFull":     false,
		"isFinalize": finalize,
		"isError":    false,
	}, nil)
}

// openAPIToken returns a cached DingTalk open API access token.
func (c *dingTalkChannel) openAPIToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}
	var response struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	if err := c.doOpenAPI(ctx, http.MethodPost, "/v1.0/oauth2/accessToken", "", map[string]any{
		"appKey":    c.account.ClientID,
		"appSecret": c.account.ClientSecret,
	}, &response); err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", errors.New("dingtalk open api returned no access token")
	}
	expiry := time.Duration(response.ExpireIn) * time.Second
	if expiry <= 0 {
		expiry = time.Hour
	}
	c.accessToken = response.AccessToken
	c.tokenExpiry = time.Now().Add(expiry - dingTalkTokenMargin)
	return c.accessToken, nil
}

// doOpenAPI calls the DingTalk open API and reports the platform error payload.
func (c *dingTalkChannel) doOpenAPI(ctx context.Context, method, path, token string, body map[string]any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.account.APIBaseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("x-acs-dingtalk-access-token", token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk open api %s status: %d, response: %s", path, resp.StatusCode, string(data))
	}
	var failure struct {
		Success *bool  `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &failure)
	if failure.Success != nil && !*failure.Success {
		return fmt.Errorf("dingtalk open api %s failed: code=%s message=%s", path, failure.Code, failure.Message)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// postWebhook posts a Markdown reply payload to a DingTalk sessionWebhook.
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
func dingTalkExtractText(data any) string {
	dataMap := dingTalkMap(data)
	text := dataMap["text"]
	if textMap := dingTalkMap(text); len(textMap) > 0 {
		if content := dingTalkString(textMap["content"]); content != "" {
			return content
		}
	}
	if content := dingTalkString(text); content != "" {
		return content
	}

	content := dataMap["content"]
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
