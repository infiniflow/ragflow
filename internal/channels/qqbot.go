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
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/channels/core"
)

const (
	defaultQQBotBaseURL  = "https://api.sgroup.qq.com"
	defaultQQBotTokenURL = "https://bots.qq.com/app/getAppAccessToken"
	qqBotRequestTimeout  = 60 * time.Second
	qqBotReconnectDelay  = 3 * time.Second
	qqBotIntents         = (1 << 30) | (1 << 12) | (1 << 25) | (1 << 26)
)

type qqBotAccount struct {
	AccountID    string
	AppID        string
	ClientSecret string
	BaseURL      string
	TokenURL     string
}

type qqBotChannel struct {
	account qqBotAccount
	client  *http.Client
	dialer  *websocket.Dialer

	mu        sync.Mutex
	writeMu   sync.Mutex
	handler   core.MessageHandler
	cancel    context.CancelFunc
	done      chan struct{}
	sessionID string
	sequence  *int64
}

type qqBotGatewayPayload struct {
	Op       int             `json:"op"`
	Data     json.RawMessage `json:"d"`
	Sequence *int64          `json:"s"`
	Type     string          `json:"t"`
}

type qqBotHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

func newQQBotChannel(account qqBotAccount) *qqBotChannel {
	if strings.TrimSpace(account.BaseURL) == "" {
		account.BaseURL = defaultQQBotBaseURL
	}
	if strings.TrimSpace(account.TokenURL) == "" {
		account.TokenURL = defaultQQBotTokenURL
	}
	return &qqBotChannel{
		account: account,
		client:  &http.Client{Timeout: qqBotRequestTimeout},
		dialer:  websocket.DefaultDialer,
	}
}

func newQQBotChannelFromConfig(accountID string, cfg map[string]any) (*qqBotChannel, error) {
	appID := firstString(cfg, "app_id")
	clientSecret := firstString(cfg, "client_secret")
	if appID == "" || clientSecret == "" {
		return nil, fmt.Errorf("qqbot account %q is missing app_id or client_secret", accountID)
	}
	return newQQBotChannel(qqBotAccount{
		AccountID:    accountID,
		AppID:        appID,
		ClientSecret: clientSecret,
		BaseURL:      firstString(cfg, "base_url"),
	}), nil
}

func (c *qqBotChannel) ChannelID() string {
	return "qqbot"
}

func (c *qqBotChannel) AccountID() string {
	return c.account.AccountID
}

func (c *qqBotChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

func (c *qqBotChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.cancel = cancel
	c.done = done
	c.mu.Unlock()

	go func() {
		c.run(runCtx)
		c.mu.Lock()
		if c.done == done {
			c.cancel = nil
			c.done = nil
		}
		c.mu.Unlock()
		close(done)
	}()
	log.Printf("[qqbot:%s] starting gateway client", c.account.AccountID)
	return nil
}

func (c *qqBotChannel) Stop(ctx context.Context) error {
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

func (c *qqBotChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	chatType, peerID := normalizeQQBotTarget(msg.ChatID)
	if peerID == "" {
		return errors.New("chat_id is required")
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}
	var path string
	switch chatType {
	case "c2c":
		path = "/v2/users/" + url.PathEscape(peerID) + "/messages"
	case "group":
		path = "/v2/groups/" + url.PathEscape(peerID) + "/messages"
	case "dm":
		path = "/dms/" + url.PathEscape(peerID) + "/messages"
	case "channel":
		path = "/channels/" + url.PathEscape(peerID) + "/messages"
	default:
		return fmt.Errorf("unsupported outbound chat type %q", chatType)
	}
	body := map[string]any{
		"content":  msg.Text,
		"msg_type": 0,
		"msg_seq":  qqBotMessageSequence(),
	}
	if msg.ReplyToMessageID != "" {
		body["msg_id"] = msg.ReplyToMessageID
	}
	return c.requestJSON(ctx, token, http.MethodPost, path, body, true, false, nil)
}

func (c *qqBotChannel) run(ctx context.Context) {
	for ctx.Err() == nil {
		token, err := c.getAccessToken(ctx)
		if err == nil {
			var gatewayURL string
			gatewayURL, err = c.getGatewayURL(ctx, token)
			if err == nil {
				err = c.runGatewaySession(ctx, gatewayURL, token)
			}
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("[qqbot:%s] gateway loop error: %v", c.account.AccountID, err)
		}
		if !waitForContext(ctx, qqBotReconnectDelay) {
			return
		}
	}
}

func (c *qqBotChannel) runGatewaySession(ctx context.Context, gatewayURL string, token string) error {
	conn, _, err := c.dialer.DialContext(ctx, gatewayURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("[qqbot:%s] connected to gateway", c.account.AccountID)

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-sessionCtx.Done()
		_ = conn.Close()
	}()

	var heartbeatCancel context.CancelFunc
	defer func() {
		if heartbeatCancel != nil {
			heartbeatCancel()
		}
	}()

	for ctx.Err() == nil {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var payload qqBotGatewayPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			log.Printf("[qqbot:%s] invalid gateway payload: %.200s", c.account.AccountID, data)
			continue
		}
		if payload.Sequence != nil {
			c.setSequence(*payload.Sequence)
		}

		switch payload.Op {
		case 10:
			var hello qqBotHello
			if err := json.Unmarshal(payload.Data, &hello); err != nil {
				return fmt.Errorf("decode gateway hello: %w", err)
			}
			if err := c.sendIdentifyOrResume(conn, token); err != nil {
				return err
			}
			if heartbeatCancel != nil {
				heartbeatCancel()
			}
			heartbeatCtx, stopHeartbeat := context.WithCancel(sessionCtx)
			heartbeatCancel = stopHeartbeat
			go c.runHeartbeat(heartbeatCtx, conn, hello.HeartbeatInterval)
		case 11:
			continue
		case 7:
			return errors.New("gateway requested reconnect")
		case 9:
			var canResume bool
			_ = json.Unmarshal(payload.Data, &canResume)
			if !canResume {
				c.clearSession()
				if !waitForContext(ctx, qqBotReconnectDelay) {
					return ctx.Err()
				}
			}
			return fmt.Errorf("invalid gateway session (can_resume=%t)", canResume)
		case 0:
			if err := c.handleDispatch(ctx, payload.Type, payload.Data); err != nil {
				log.Printf("[qqbot:%s] ignored invalid %s event: %v", c.account.AccountID, payload.Type, err)
			}
		}
	}
	return ctx.Err()
}

func (c *qqBotChannel) runHeartbeat(ctx context.Context, conn *websocket.Conn, intervalMS int) {
	delay := time.Duration(intervalMS) * time.Millisecond
	if delay < time.Second {
		delay = time.Second
	}
	ticker := time.NewTicker(delay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload := map[string]any{"op": 1, "d": c.currentSequence()}
			if err := c.writeJSON(conn, payload); err != nil {
				log.Printf("[qqbot:%s] heartbeat failed: %v", c.account.AccountID, err)
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *qqBotChannel) sendIdentifyOrResume(conn *websocket.Conn, token string) error {
	c.mu.Lock()
	sessionID := c.sessionID
	var sequence *int64
	if c.sequence != nil {
		value := *c.sequence
		sequence = &value
	}
	c.mu.Unlock()

	if sessionID != "" && sequence != nil {
		return c.writeJSON(conn, map[string]any{
			"op": 6,
			"d": map[string]any{
				"token":      "QQBot " + token,
				"session_id": sessionID,
				"seq":        *sequence,
			},
		})
	}
	return c.writeJSON(conn, map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   "QQBot " + token,
			"intents": qqBotIntents,
			"shard":   []int{0, 1},
		},
	})
}

func (c *qqBotChannel) handleDispatch(ctx context.Context, eventType string, data json.RawMessage) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if eventType == "READY" {
		c.mu.Lock()
		c.sessionID = anyToString(raw["session_id"])
		c.mu.Unlock()
		return nil
	}
	if eventType == "RESUMED" {
		return nil
	}
	incoming := c.normalizeIncomingEvent(eventType, raw)
	if incoming == nil {
		return nil
	}
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[qqbot:%s] handler panic: %v\n%s", c.account.AccountID, r, debug.Stack())
				}
			}()
			if err := handler(ctx, *incoming); err != nil {
				log.Printf("[qqbot:%s] handler error: %v", c.account.AccountID, err)
			}
		}()
	}
	return nil
}

func (c *qqBotChannel) normalizeIncomingEvent(eventType string, data map[string]any) *core.IncomingMessage {
	author, _ := data["author"].(map[string]any)
	message := &core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.AccountID(),
		MessageID: anyToString(data["id"]),
		Text:      anyToString(data["content"]),
		Raw:       data,
	}
	switch eventType {
	case "C2C_MESSAGE_CREATE":
		message.SenderID = anyToString(author["user_openid"])
		if message.SenderID == "" {
			return nil
		}
		message.ChatID = incomingQQBotChatID("c2c", message.SenderID)
		message.ChatType = "p2p"
	case "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE":
		groupID := anyToString(data["group_openid"])
		message.SenderID = anyToString(author["member_openid"])
		if groupID == "" || message.SenderID == "" {
			return nil
		}
		message.ChatID = incomingQQBotChatID("group", groupID)
		message.ChatType = "group"
	case "DIRECT_MESSAGE_CREATE":
		message.SenderID = anyToString(author["id"])
		if message.SenderID == "" {
			return nil
		}
		chatID := anyToString(data["guild_id"])
		if chatID == "" {
			chatID = message.SenderID
		}
		message.ChatID = incomingQQBotChatID("dm", chatID)
		message.ChatType = "dm"
	case "AT_MESSAGE_CREATE":
		message.SenderID = anyToString(author["id"])
		channelID := anyToString(data["channel_id"])
		if message.SenderID == "" || channelID == "" {
			return nil
		}
		message.ChatID = incomingQQBotChatID("channel", channelID)
		message.ChatType = "channel"
	default:
		return nil
	}
	return message
}

func (c *qqBotChannel) getAccessToken(ctx context.Context) (string, error) {
	var response map[string]any
	err := c.requestJSON(ctx, "", http.MethodPost, c.account.TokenURL, map[string]any{
		"appId":        c.account.AppID,
		"clientSecret": c.account.ClientSecret,
	}, false, true, &response)
	if err != nil {
		return "", err
	}
	token := anyToString(response["access_token"])
	if token == "" {
		token = anyToString(response["accessToken"])
	}
	if token == "" {
		return "", fmt.Errorf("qqbot token response missing access_token: %v", response)
	}
	return token, nil
}

func (c *qqBotChannel) getGatewayURL(ctx context.Context, token string) (string, error) {
	var response map[string]any
	if err := c.requestJSON(ctx, token, http.MethodGet, "/gateway", nil, true, false, &response); err != nil {
		return "", err
	}
	gatewayURL := anyToString(response["url"])
	if gatewayURL == "" {
		return "", fmt.Errorf("qqbot gateway response missing url: %v", response)
	}
	return gatewayURL, nil
}

func (c *qqBotChannel) requestJSON(
	ctx context.Context,
	accessToken string,
	method string,
	path string,
	body map[string]any,
	auth bool,
	absoluteURL bool,
	out any,
) error {
	requestURL := path
	if !absoluteURL {
		requestURL = strings.TrimRight(c.account.BaseURL, "/") + path
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth && accessToken != "" {
		req.Header.Set("Authorization", "QQBot "+accessToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status: %d, response: %s", resp.StatusCode, payload)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	if out == nil {
		out = &map[string]any{}
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("invalid json response from %s: %.200s: %w", path, payload, err)
	}
	return nil
}

func (c *qqBotChannel) writeJSON(conn *websocket.Conn, value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(value)
}

func (c *qqBotChannel) setSequence(sequence int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence = &sequence
}

func (c *qqBotChannel) currentSequence() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sequence == nil {
		return nil
	}
	return *c.sequence
}

func (c *qqBotChannel) clearSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = ""
	c.sequence = nil
}

func normalizeQQBotTarget(chatID string) (string, string) {
	raw := strings.TrimSpace(chatID)
	raw = strings.TrimPrefix(raw, "qqbot:")
	for _, chatType := range []string{"group", "channel", "dm", "c2c"} {
		prefix := chatType + ":"
		if strings.HasPrefix(raw, prefix) {
			return chatType, strings.TrimPrefix(raw, prefix)
		}
	}
	return "c2c", raw
}

func incomingQQBotChatID(chatType string, peerID string) string {
	return "qqbot:" + chatType + ":" + peerID
}

func qqBotMessageSequence() int {
	sequence := int(time.Now().UnixMilli() % 65535)
	if sequence != 0 {
		return sequence
	}
	var value uint16
	if err := binary.Read(rand.Reader, binary.BigEndian, &value); err != nil || value == 0 {
		return 1
	}
	return int(value)
}

func anyToString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func waitForContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
