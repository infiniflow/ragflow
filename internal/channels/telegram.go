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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/channels/core"
)

const (
	defaultTelegramAPIBaseURL = "https://api.telegram.org"
	telegramPollTimeout       = 30 * time.Second
	telegramHTTPTimeout       = 40 * time.Second
	telegramReconnectDelay    = 3 * time.Second
)

type telegramAccount struct {
	AccountID  string
	Token      string
	APIBaseURL string
}

type telegramChannel struct {
	account telegramAccount

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	handler core.MessageHandler
	client  *http.Client
	offset  int64
}

type telegramAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

type telegramTransportError struct {
	method string
	err    error
}

func (e *telegramTransportError) Error() string {
	return fmt.Sprintf("Telegram API %s transport request failed", e.method)
}

func (e *telegramTransportError) Unwrap() error {
	return e.err
}

type telegramUpdate struct {
	UpdateID          int64            `json:"update_id"`
	Message           *telegramMessage `json:"message"`
	EditedMessage     *telegramMessage `json:"edited_message"`
	ChannelPost       *telegramMessage `json:"channel_post"`
	EditedChannelPost *telegramMessage `json:"edited_channel_post"`
	Raw               map[string]any
}

type telegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *telegramUser `json:"from"`
	Chat      telegramChat  `json:"chat"`
	Text      string        `json:"text"`
	Caption   string        `json:"caption"`
}

type telegramUser struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// newTelegramChannel creates a Telegram channel with default API settings.
func newTelegramChannel(account telegramAccount) *telegramChannel {
	account.Token = strings.TrimSpace(account.Token)
	account.APIBaseURL = strings.TrimSpace(account.APIBaseURL)
	if account.APIBaseURL == "" {
		account.APIBaseURL = defaultTelegramAPIBaseURL
	}
	return &telegramChannel{
		account: account,
		client:  &http.Client{Timeout: telegramHTTPTimeout},
	}
}

// newTelegramChannelFromConfig builds a Telegram channel from chat_channel.config.credential.
func newTelegramChannelFromConfig(accountID string, cfg map[string]any) (*telegramChannel, error) {
	token := strings.TrimSpace(fmt.Sprint(cfg["token"]))
	if token == "" || token == "<nil>" {
		return nil, fmt.Errorf("telegram account %q is missing token", accountID)
	}

	// APIBaseURL is intentionally optional and is useful for Bot API-compatible
	// proxies and tests. The Python implementation defaults to api.telegram.org.
	baseURL := firstString(cfg, "api_base_url", "base_url")
	return newTelegramChannel(telegramAccount{
		AccountID:  accountID,
		Token:      token,
		APIBaseURL: baseURL,
	}), nil
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *telegramChannel) ChannelID() string {
	return "telegram"
}

// AccountID returns the chat_channel.id bound to this Telegram runtime instance.
func (c *telegramChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound messages.
func (c *telegramChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start starts Telegram long polling in the background.
func (c *telegramChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.cancel = cancel
	c.done = done
	c.offset = 0
	c.mu.Unlock()

	go func() {
		defer close(done)
		c.run(runCtx)

		c.mu.Lock()
		if c.done == done {
			c.cancel = nil
		}
		c.mu.Unlock()
	}()
	return nil
}

// Stop cancels Telegram long polling and waits for the poll request to finish.
func (c *telegramChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	done := c.done
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send sends a Telegram message and optionally replies to an existing message.
// Ai -> user
func (c *telegramChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	chatID, err := strconv.ParseInt(strings.TrimSpace(msg.ChatID), 10, 64)
	if err != nil {
		log.Printf("[telegram:%s] invalid chat_id %q: %v", c.account.AccountID, msg.ChatID, err)
		return nil
	}

	payload := map[string]any{
		"chat_id": chatID,
		"text":    msg.Text,
	}
	if replyID := strings.TrimSpace(msg.ReplyToMessageID); replyID != "" {
		if messageID, parseErr := strconv.ParseInt(replyID, 10, 64); parseErr == nil {
			payload["reply_parameters"] = map[string]any{
				"message_id":                  messageID,
				"allow_sending_without_reply": true,
			}
		}
	}
	if err := c.callAPI(ctx, "sendMessage", payload, nil); err != nil {
		log.Printf("[telegram:%s] send failed: %v", c.account.AccountID, err)
	}
	return nil
}

// run drops queued updates and consumes Telegram updates until stopped.
func (c *telegramChannel) run(ctx context.Context) {
	for ctx.Err() == nil {
		err := c.callAPI(ctx, "deleteWebhook", map[string]any{
			"drop_pending_updates": true,
		}, nil)
		if err == nil {
			break
		}
		log.Printf("[telegram:%s] failed to start polling: %v", c.account.AccountID, err)
		if !waitForTelegramRetry(ctx) {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}

	for ctx.Err() == nil {
		updates, err := c.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[telegram:%s] polling failed: %v", c.account.AccountID, err)
			if !waitForTelegramRetry(ctx) {
				return
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= c.offset {
				c.offset = update.UpdateID + 1
			}
			c.handleUpdate(ctx, update)
		}
	}
}

// getUpdates performs one Telegram long-poll request and decodes raw updates.
func (c *telegramChannel) getUpdates(ctx context.Context) ([]telegramUpdate, error) {
	params := map[string]any{
		"offset":  c.offset,
		"timeout": int(telegramPollTimeout / time.Second),
	}
	var rawUpdates []json.RawMessage
	if err := c.callAPI(ctx, "getUpdates", params, &rawUpdates); err != nil {
		return nil, err
	}

	updates := make([]telegramUpdate, 0, len(rawUpdates))
	for _, raw := range rawUpdates {
		var update telegramUpdate
		if err := json.Unmarshal(raw, &update); err != nil {
			log.Printf("[telegram:%s] ignored invalid update: %v", c.account.AccountID, err)
			continue
		}
		_ = json.Unmarshal(raw, &update.Raw)
		updates = append(updates, update)
	}
	return updates, nil
}

// handleUpdate maps Telegram's effective message into the common channel model.
// Telegram -> AI
func (c *telegramChannel) handleUpdate(ctx context.Context, update telegramUpdate) {
	msg := update.effectiveMessage()
	if msg == nil {
		return
	}
	if msg.From == nil && msg != update.ChannelPost && msg != update.EditedChannelPost {
		return
	}
	if msg.From != nil && msg.From.IsBot {
		return
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	senderID := strconv.FormatInt(msg.Chat.ID, 10)
	if msg.From != nil {
		senderID = strconv.FormatInt(msg.From.ID, 10)
	}
	incoming := core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    strconv.FormatInt(msg.Chat.ID, 10),
		ChatType:  telegramChatType(msg.Chat.Type),
		MessageID: strconv.FormatInt(msg.MessageID, 10),
		SenderID:  senderID,
		Text:      text,
		Raw:       update.Raw,
	}

	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[telegram:%s] message handler error: %v", c.account.AccountID, err)
	}
}

func (u telegramUpdate) effectiveMessage() *telegramMessage {
	if u.Message != nil {
		return u.Message
	}
	if u.EditedMessage != nil {
		return u.EditedMessage
	}
	if u.ChannelPost != nil {
		return u.ChannelPost
	}
	return u.EditedChannelPost
}

func telegramChatType(value string) string {
	switch value {
	case "private":
		return "p2p"
	case "group", "supergroup":
		return "group"
	case "channel":
		return "channel"
	default:
		if value == "" {
			return "unknown"
		}
		return value
	}
}

// callAPI invokes a Telegram Bot API method using the configured token.
func (c *telegramChannel) callAPI(ctx context.Context, method string, params map[string]any, out any) error {
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(c.account.APIBaseURL, "/")
	endpoint := baseURL + "/bot" + url.PathEscape(c.account.Token) + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return &telegramTransportError{method: method, err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("Telegram API %s returned status %d: %s", method, resp.StatusCode, string(body))
	}

	var response telegramAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("Telegram API %s returned invalid JSON: %w", method, err)
	}
	if !response.OK {
		return fmt.Errorf("Telegram API %s failed: %s", method, response.Description)
	}
	if out != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, out); err != nil {
			return fmt.Errorf("Telegram API %s returned invalid result: %w", method, err)
		}
	}
	return nil
}

func waitForTelegramRetry(ctx context.Context) bool {
	timer := time.NewTimer(telegramReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
