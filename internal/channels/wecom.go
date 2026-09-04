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
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/channels/core"
)

const (
	defaultWeComAPIBaseURL  = "https://qyapi.weixin.qq.com/cgi-bin"
	defaultWeComWSURL       = "wss://openws.work.weixin.qq.com"
	defaultWeComWebhookHost = "0.0.0.0"
	defaultWeComWebhookPort = 3002
	weComHTTPTimeout        = 30 * time.Second
	weComWSHandshakeTimeout = 10 * time.Second
	weComSubscribeTimeout   = 10 * time.Second
	weComWSWriteTimeout     = 10 * time.Second
	weComReconnectDelay     = 3 * time.Second
	weComHeartbeatInterval  = 25 * time.Second
	weComTokenSafetyMargin  = 60 * time.Second
)

var (
	errWeComAuthentication = errors.New("WeCom websocket authentication failed")
	errWeComDisconnected   = errors.New("WeCom websocket disconnected by server event")
	weComRequestSequence   atomic.Uint64
)

type wecomAccount struct {
	AccountID      string
	ConnectionType string
	CorpID         string
	AgentID        int64
	Secret         string
	Token          string
	AESKey         string
	BotID          string
	WebhookHost    string
	WebhookPort    int
	APIBaseURL     string
	WSURL          string
}

type wecomChannel struct {
	account wecomAccount
	crypto  *wecomCrypto
	client  *http.Client

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	handler     core.MessageHandler
	cancel      context.CancelFunc
	done        chan struct{}
	ws          *websocket.Conn
	server      *wecomWebhookServer
	wsWriteMu   sync.Mutex
	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

type wecomAPIResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type wecomAccessTokenResponse struct {
	wecomAPIResponse
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// newWeComChannel creates a WeCom channel with default endpoints filled in.
func newWeComChannel(account wecomAccount) (*wecomChannel, error) {
	account.ConnectionType = strings.ToLower(strings.TrimSpace(account.ConnectionType))
	if account.ConnectionType == "" {
		account.ConnectionType = "webhook"
	}
	account.APIBaseURL = strings.TrimRight(strings.TrimSpace(account.APIBaseURL), "/")
	if account.APIBaseURL == "" {
		account.APIBaseURL = defaultWeComAPIBaseURL
	}
	account.WSURL = strings.TrimSpace(account.WSURL)
	if account.WSURL == "" {
		account.WSURL = defaultWeComWSURL
	}
	account.WebhookHost = strings.TrimSpace(account.WebhookHost)
	if account.WebhookHost == "" {
		account.WebhookHost = defaultWeComWebhookHost
	}
	if account.WebhookPort == 0 {
		account.WebhookPort = defaultWeComWebhookPort
	}

	channel := &wecomChannel{
		account: account,
		client:  &http.Client{Timeout: weComHTTPTimeout},
	}
	if account.ConnectionType == "webhook" {
		crypto, err := newWeComCrypto(account.Token, account.AESKey, account.CorpID)
		if err != nil {
			return nil, err
		}
		channel.crypto = crypto
	}
	return channel, nil
}

// newWeComChannelFromConfig builds a WeCom channel from chat_channel.config.credential.
func newWeComChannelFromConfig(accountID string, cfg map[string]any) (*wecomChannel, error) {
	connectionType := strings.ToLower(firstString(cfg, "connection_type"))
	if connectionType == "" {
		connectionType = "webhook"
	}
	if connectionType != "websocket" {
		// Python treats every non-websocket value as webhook. Keep the same
		// effective runtime path while still applying webhook validation.
		connectionType = "webhook"
	}

	account := wecomAccount{
		AccountID:      accountID,
		ConnectionType: connectionType,
		Secret:         firstString(cfg, "secret"),
		WebhookHost:    firstString(cfg, "webhook_host"),
		APIBaseURL:     firstString(cfg, "api_base_url"),
		WSURL:          firstString(cfg, "ws_url"),
	}
	if account.Secret == "" {
		return nil, fmt.Errorf("wecom account %q missing required field: secret", accountID)
	}

	if connectionType == "websocket" {
		account.BotID = firstString(cfg, "bot_id")
		if account.BotID == "" {
			return nil, fmt.Errorf("wecom account %q missing required field: bot_id", accountID)
		}
		return newWeComChannel(account)
	}

	account.CorpID = firstString(cfg, "corp_id")
	account.Token = firstString(cfg, "token")
	account.AESKey = firstString(cfg, "aes_key")
	missing := make([]string, 0, 3)
	if account.CorpID == "" {
		missing = append(missing, "corp_id")
	}
	if account.Token == "" {
		missing = append(missing, "token")
	}
	if account.AESKey == "" {
		missing = append(missing, "aes_key")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("wecom account %q missing required fields: %s", accountID, strings.Join(missing, ", "))
	}
	if len(account.AESKey) != 43 {
		return nil, fmt.Errorf("wecom account %q aes_key (EncodingAESKey) must be 43 characters, got %d", accountID, len(account.AESKey))
	}

	agentID, err := parseWeComInt(cfg["agent_id"], "agent_id")
	if err != nil {
		return nil, fmt.Errorf("wecom account %q %w", accountID, err)
	}
	account.AgentID = agentID
	var configuredWebhookPort *int
	if raw, ok := cfg["webhook_port"]; ok {
		port, err := parseWeComInt(raw, "webhook_port")
		if err != nil {
			return nil, fmt.Errorf("wecom account %q %w", accountID, err)
		}
		account.WebhookPort = int(port)
		configuredWebhookPort = &account.WebhookPort
	}
	channel, err := newWeComChannel(account)
	if err != nil {
		return nil, err
	}
	if configuredWebhookPort != nil {
		channel.account.WebhookPort = *configuredWebhookPort
	}
	return channel, nil
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *wecomChannel) ChannelID() string {
	return "wecom"
}

// AccountID returns the chat_channel.id bound to this WeCom runtime instance.
func (c *wecomChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound messages.
func (c *wecomChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start starts either the shared webhook listener or the WebSocket client.
func (c *wecomChannel) Start(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	if c.account.ConnectionType == "webhook" {
		c.mu.Lock()
		started := c.server != nil
		c.mu.Unlock()
		if started {
			return nil
		}
		server, err := acquireWeComWebhookServer(c)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.server = server
		c.mu.Unlock()
		log.Printf("[wecom:%s] registered webhook at /wecom/%s/callback", c.account.AccountID, c.account.AccountID)
		return nil
	}

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
		defer close(done)
		c.runWebSocket(runCtx)
		c.mu.Lock()
		if c.done == done {
			c.cancel = nil
			c.ws = nil
		}
		c.mu.Unlock()
	}()
	return nil
}

// Stop releases the webhook listener or stops the WebSocket reconnect loop.
func (c *wecomChannel) Stop(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	if c.account.ConnectionType == "webhook" {
		c.mu.Lock()
		server := c.server
		c.server = nil
		c.mu.Unlock()
		if server != nil {
			return releaseWeComWebhookServer(ctx, server, c.account.AccountID)
		}
		return nil
	}

	c.mu.Lock()
	cancel := c.cancel
	done := c.done
	conn := c.ws
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if conn != nil {
		_ = conn.Close()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send sends an application message or a WebSocket bot message.
func (c *wecomChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	if c.account.ConnectionType == "websocket" {
		if err := c.sendWebSocketMessage(msg); err != nil {
			log.Printf("[wecom:%s] websocket send failed: %v", c.account.AccountID, err)
		}
		return nil
	}
	if err := c.sendApplicationMessage(ctx, msg); err != nil {
		log.Printf("[wecom:%s] application send failed: %v", c.account.AccountID, err)
	}
	return nil
}

func (c *wecomChannel) runWebSocket(ctx context.Context) {
	for ctx.Err() == nil {
		err := c.runWebSocketOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errWeComAuthentication) || errors.Is(err, errWeComDisconnected) {
			log.Printf("[wecom:%s] websocket stopped: %v", c.account.AccountID, err)
			return
		}
		if err != nil {
			log.Printf("[wecom:%s] websocket loop error: %v", c.account.AccountID, err)
		}
		if !waitForWeComRetry(ctx) {
			return
		}
	}
}

func (c *wecomChannel) runWebSocketOnce(ctx context.Context) error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = weComWSHandshakeTimeout
	conn, _, err := dialer.DialContext(ctx, c.account.WSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.mu.Lock()
	c.ws = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.ws == conn {
			c.ws = nil
		}
		c.mu.Unlock()
	}()

	connectionDone := make(chan struct{})
	defer close(connectionDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-connectionDone:
		}
	}()

	if err := c.subscribeWebSocket(conn); err != nil {
		return err
	}
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go c.runWebSocketHeartbeat(ctx, conn, heartbeatDone)

	for ctx.Err() == nil {
		if err := conn.SetReadDeadline(time.Now().Add(2 * weComHeartbeatInterval)); err != nil {
			return err
		}
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		disconnect, err := c.handleWebSocketPayload(ctx, payload)
		if err != nil {
			log.Printf("[wecom:%s] invalid websocket payload: %v", c.account.AccountID, err)
			continue
		}
		if disconnect {
			return errWeComDisconnected
		}
	}
	return ctx.Err()
}

func (c *wecomChannel) subscribeWebSocket(conn *websocket.Conn) error {
	payload := map[string]any{
		"cmd":     "aibot_subscribe",
		"headers": map[string]any{"req_id": newWeComRequestID()},
		"body": map[string]any{
			"bot_id": c.account.BotID,
			"secret": c.account.Secret,
		},
	}
	if err := c.writeWebSocketJSON(conn, payload); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(weComSubscribeTimeout))
	defer conn.SetReadDeadline(time.Time{})
	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		return fmt.Errorf("WeCom websocket subscribe acknowledgement: %w", err)
	}
	if cmd := weComRawStringValue(response["cmd"]); cmd != "aibot_subscribe" {
		log.Printf("[wecom:%s] unexpected subscribe response command %q", c.account.AccountID, cmd)
	}
	errCode := weComIntValue(response["errcode"])
	if errCode == 853000 {
		return fmt.Errorf("%w: invalid bot_id or secret", errWeComAuthentication)
	}
	if errCode != 0 {
		return fmt.Errorf("WeCom websocket subscribe failed: errcode=%d errmsg=%s", errCode, weComStringValue(response["errmsg"]))
	}
	log.Printf("[wecom:%s] websocket subscribed", c.account.AccountID)
	return nil
}

func (c *wecomChannel) runWebSocketHeartbeat(ctx context.Context, conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(weComHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if err := c.writeWebSocketJSON(conn, map[string]any{
				"cmd":     "ping",
				"headers": map[string]any{"req_id": newWeComRequestID()},
			}); err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *wecomChannel) handleWebSocketPayload(ctx context.Context, payload []byte) (bool, error) {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return false, err
	}
	cmd := weComRawStringValue(object["cmd"])
	switch cmd {
	case "aibot_msg_callback":
		c.handleWebSocketMessage(ctx, object)
	case "aibot_event_callback":
		body := weComMapValue(object["body"])
		event := weComMapValue(body["event"])
		if weComRawStringValue(event["eventtype"]) == "disconnected_event" {
			return true, nil
		}
	case "aibot_subscribe", "aibot_respond_msg", "aibot_respond_welcome_msg", "aibot_send_msg", "ping":
		if code := weComIntValue(object["errcode"]); code != 0 {
			log.Printf("[wecom:%s] websocket response error: %s", c.account.AccountID, string(payload))
		}
	}
	return false, nil
}

func (c *wecomChannel) handleWebSocketMessage(ctx context.Context, raw map[string]any) {
	headers := weComMapValue(raw["headers"])
	body := weComMapValue(raw["body"])
	if weComRawStringValue(body["msgtype"]) != "text" {
		return
	}
	sender := weComMapValue(body["from"])
	senderID := weComRawStringValue(sender["userid"])
	if senderID == "" {
		return
	}
	chatID := weComRawStringValue(body["chatid"])
	if chatID == "" {
		chatID = senderID
	}
	messageID := weComRawStringValue(headers["req_id"])
	if messageID == "" {
		messageID = weComRawStringValue(body["msgid"])
	}
	chatType := "p2p"
	if weComRawStringValue(body["chattype"]) == "group" {
		chatType = "group"
	}
	text := weComRawStringValue(weComMapValue(body["text"])["content"])
	c.handleTextMessage(ctx, chatID, senderID, messageID, text, chatType, raw)
}

func (c *wecomChannel) sendWebSocketMessage(msg core.OutgoingMessage) error {
	if msg.Text == "" {
		return errors.New("WeCom websocket reply text is required")
	}
	c.mu.Lock()
	conn := c.ws
	c.mu.Unlock()
	if conn == nil {
		return errors.New("WeCom websocket is not connected")
	}
	return c.writeWebSocketJSON(conn, map[string]any{
		"cmd":     "aibot_send_msg",
		"headers": map[string]any{"req_id": newWeComRequestID()},
		"body": map[string]any{
			"chatid":  msg.ChatID,
			"msgtype": "markdown",
			"markdown": map[string]any{
				"content": msg.Text,
			},
		},
	})
}

func (c *wecomChannel) writeWebSocketJSON(conn *websocket.Conn, payload any) error {
	c.wsWriteMu.Lock()
	defer c.wsWriteMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(weComWSWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteJSON(payload)
}

func (c *wecomChannel) sendApplicationMessage(ctx context.Context, msg core.OutgoingMessage) error {
	if msg.ChatID == "" {
		return errors.New("WeCom chat_id is required")
	}
	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(c.account.APIBaseURL + "/message/send")
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("access_token", accessToken)
	endpoint.RawQuery = query.Encode()

	var response wecomAPIResponse
	if err := c.requestJSON(ctx, http.MethodPost, endpoint.String(), map[string]any{
		"touser":  msg.ChatID,
		"msgtype": "text",
		"agentid": c.account.AgentID,
		"text":    map[string]any{"content": msg.Text},
		"safe":    0,
	}, &response); err != nil {
		return err
	}
	if response.ErrCode != 0 {
		if response.ErrCode == 40014 || response.ErrCode == 42001 {
			c.clearAccessToken()
		}
		return fmt.Errorf("WeCom message/send failed: errcode=%d errmsg=%s", response.ErrCode, response.ErrMsg)
	}
	return nil
}

func (c *wecomChannel) getAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	endpoint, err := url.Parse(c.account.APIBaseURL + "/gettoken")
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("corpid", c.account.CorpID)
	query.Set("corpsecret", c.account.Secret)
	endpoint.RawQuery = query.Encode()
	var response wecomAccessTokenResponse
	if err := c.requestJSON(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || response.AccessToken == "" {
		return "", fmt.Errorf("WeCom gettoken failed: errcode=%d errmsg=%s", response.ErrCode, response.ErrMsg)
	}
	expiresIn := response.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 7200
	}
	c.accessToken = response.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn)*time.Second - weComTokenSafetyMargin)
	return c.accessToken, nil
}

func (c *wecomChannel) clearAccessToken() {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.accessToken = ""
	c.tokenExpiry = time.Time{}
}

func (c *wecomChannel) requestJSON(ctx context.Context, method, endpoint string, body map[string]any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("WeCom API returned invalid JSON: %w", err)
	}
	return nil
}

func (c *wecomChannel) handleTextMessage(ctx context.Context, chatID, senderID, messageID, text, chatType string, raw map[string]any) {
	if strings.TrimSpace(text) == "" {
		return
	}
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    chatID,
		ChatType:  chatType,
		MessageID: messageID,
		SenderID:  senderID,
		Text:      text,
		Raw:       raw,
	}); err != nil {
		log.Printf("[wecom:%s] message handler error: %v", c.account.AccountID, err)
	}
}

func parseWeComInt(value any, field string) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
		return int64(v), nil
	case json.Number:
		result, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
		return result, nil
	case string:
		result, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
		return result, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", field)
	}
}

func weComMapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func weComStringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// weComRawStringValue mirrors Python's str(value or "") conversion for
// message fields without changing user-visible whitespace.
func weComRawStringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func weComIntValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		result, _ := v.Int64()
		return int(result)
	case string:
		result, _ := strconv.Atoi(v)
		return result
	default:
		return 0
	}
}

func newWeComRequestID() string {
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), weComRequestSequence.Add(1))
}

func waitForWeComRetry(ctx context.Context) bool {
	timer := time.NewTimer(weComReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
