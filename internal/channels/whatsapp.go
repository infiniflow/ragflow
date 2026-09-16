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
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/channels/core"
)

const (
	defaultGatewayBaseURL = "http://127.0.0.1:3005"
	defaultTimeout        = 30 * time.Second
	startTimeout          = 2 * time.Minute
	wsHandshakeTimeout    = 2 * time.Minute
	wsReadTimeout         = 2 * time.Minute
	wsPingInterval        = 30 * time.Second
	defaultReconnect      = 3 * time.Second
	messageTTL            = time.Hour
	messageQueueSize      = 64
	chatWorkerIdle        = 5 * time.Minute
)

type whatsappAccount struct {
	AccountID      string
	GatewayBaseURL string
	GatewayToken   string
	SessionKey     string
	Timeout        time.Duration
}

type whatsappRuntimeSnapshot struct {
	AccountID      string   `json:"account_id"`
	SessionKey     string   `json:"session_key"`
	Status         string   `json:"status"`
	ConnectedAt    *float64 `json:"connected_at"`
	QRUpdatedAt    *float64 `json:"qr_updated_at"`
	QRDataURL      *string  `json:"qr_data_url"`
	LastError      *string  `json:"last_error"`
	SessionID      *string  `json:"session_id"`
	LastSnapshotAt *float64 `json:"last_snapshot_at"`
	GatewayBaseURL *string  `json:"gateway_base_url"`
	EventCursor    int64    `json:"event_cursor"`
}

type whatsappChannel struct {
	account whatsappAccount

	mu             sync.Mutex
	cancel         context.CancelFunc
	handler        core.MessageHandler
	client         *http.Client
	status         string
	lastErr        string
	qrDataURL      string
	qrUpdatedAt    float64
	connectedAt    float64
	lastSnapshotAt float64
	sessionID      string
	eventCursor    int64
	seen           map[string]time.Time
	workers        map[string]*whatsappChatWorker
}

type whatsappChatWorker struct {
	queue chan core.IncomingMessage
}

var liveWhatsAppChannels sync.Map

// newWhatsAppChannel creates a WhatsApp channel instance with default runtime settings filled in.
func newWhatsAppChannel(account whatsappAccount) *whatsappChannel {
	if account.GatewayBaseURL == "" {
		account.GatewayBaseURL = defaultGatewayBaseURL
	}
	if account.SessionKey == "" {
		account.SessionKey = account.AccountID
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultTimeout
	}
	return &whatsappChannel{
		account: account,
		client:  &http.Client{Timeout: account.Timeout},
		status:  "stopped",
		seen:    map[string]time.Time{},
		workers: map[string]*whatsappChatWorker{},
	}
}

// newWhatsAppChannelFromConfig builds a WhatsApp channel from chat_channel.config.credential.
func newWhatsAppChannelFromConfig(accountID string, cfg map[string]any) (*whatsappChannel, error) {
	timeout := defaultTimeout
	if raw, ok := cfg["timeout_secs"]; ok {
		switch v := raw.(type) {
		case float64:
			timeout = time.Duration(v) * time.Second
		case int:
			timeout = time.Duration(v) * time.Second
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				timeout = time.Duration(n) * time.Second
			}
		}
	}
	baseURL := firstString(cfg, "gateway_base_url", "gateway_url", "control_url")
	token := firstString(cfg, "gateway_token", "token")
	sessionKey := firstString(cfg, "session_key", "session_id")
	if sessionKey == "" {
		sessionKey = accountID
	}
	return newWhatsAppChannel(whatsappAccount{
		AccountID:      accountID,
		GatewayBaseURL: baseURL,
		GatewayToken:   token,
		SessionKey:     sessionKey,
		Timeout:        timeout,
	}), nil
}

// getWhatsAppRuntimeSnapshot returns the in-memory runtime snapshot for a live WhatsApp channel.
func getWhatsAppRuntimeSnapshot(accountID string) (*whatsappRuntimeSnapshot, bool) {
	raw, ok := liveWhatsAppChannels.Load(accountID)
	if !ok {
		return nil, false
	}
	return raw.(*whatsappChannel).Snapshot(), true
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *whatsappChannel) ChannelID() string {
	return "whatsapp"
}

// AccountID returns the chat_channel.id bound to this WhatsApp runtime instance.
func (c *whatsappChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound WhatsApp messages.
func (c *whatsappChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start begins the WhatsApp gateway session and event websocket loop.
func (c *whatsappChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.status = "connecting"
	c.lastErr = ""
	c.mu.Unlock()

	liveWhatsAppChannels.Store(c.account.AccountID, c)
	go c.run(runCtx)
	return nil
}

// Stop cancels the WhatsApp event loop and asks the gateway to stop the session.
func (c *whatsappChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.status = "stopped"
	c.lastErr = ""
	c.qrDataURL = ""
	c.qrUpdatedAt = 0
	c.connectedAt = 0
	c.lastSnapshotAt = 0
	c.sessionID = ""
	c.eventCursor = 0
	c.seen = map[string]time.Time{}
	c.workers = map[string]*whatsappChatWorker{}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	liveWhatsAppChannels.Delete(c.account.AccountID)
	_ = c.requestJSON(ctx, http.MethodPost, fmt.Sprintf("whatsapp/%s/stop", url.PathEscape(c.sessionKey())), map[string]any{
		"account_id":  c.account.AccountID,
		"session_key": c.sessionKey(),
	}, nil)
	return nil
}

// Send posts an outgoing RAGFlow answer to the WhatsApp gateway.
func (c *whatsappChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	payload := map[string]any{
		"account_id":          c.account.AccountID,
		"session_key":         c.sessionKey(),
		"chat_id":             msg.ChatID,
		"text":                msg.Text,
		"reply_to_message_id": nil,
	}
	if msg.ReplyToMessageID != "" {
		payload["reply_to_message_id"] = msg.ReplyToMessageID
	}
	return c.requestJSON(ctx, http.MethodPost, fmt.Sprintf("whatsapp/%s/send", url.PathEscape(c.sessionKey())), payload, nil)
}

// Snapshot copies the current WhatsApp runtime state for the runtime API.
func (c *whatsappChannel) Snapshot() *whatsappRuntimeSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	baseURL := strings.TrimRight(c.account.GatewayBaseURL, "/")
	return &whatsappRuntimeSnapshot{
		AccountID:      c.account.AccountID,
		SessionKey:     c.sessionKey(),
		Status:         c.status,
		ConnectedAt:    floatPtr(c.connectedAt),
		QRUpdatedAt:    floatPtr(c.qrUpdatedAt),
		QRDataURL:      stringPtr(c.qrDataURL),
		LastError:      stringPtr(c.lastErr),
		SessionID:      stringPtr(c.sessionID),
		LastSnapshotAt: floatPtr(c.lastSnapshotAt),
		GatewayBaseURL: stringPtr(baseURL),
		EventCursor:    c.eventCursor,
	}
}

// run starts the gateway session and reconnects the websocket event stream when needed.
func (c *whatsappChannel) run(ctx context.Context) {
	if err := c.startSession(ctx); err != nil {
		if ctx.Err() == nil {
			c.setError(err)
		}
		return
	}

	for ctx.Err() == nil {
		if err := c.runEvents(ctx); err != nil && ctx.Err() == nil {
			c.setError(err)
			log.Printf("[whatsapp:%s] event loop error: %v", c.account.AccountID, err)
			time.Sleep(defaultReconnect)
		}
	}
}

// startSession asks the gateway to start the session and accepts an already-created session after timeout.
func (c *whatsappChannel) startSession(ctx context.Context) error {
	err := c.requestJSONWithTimeout(ctx, startTimeout, http.MethodPost, fmt.Sprintf("whatsapp/%s/start", url.PathEscape(c.sessionKey())), map[string]any{
		"account_id":       c.account.AccountID,
		"session_key":      c.sessionKey(),
		"gateway_base_url": strings.TrimRight(c.account.GatewayBaseURL, "/"),
	}, nil)
	if err == nil {
		return c.waitForStatus(ctx)
	}
	if statusErr := c.waitForStatus(ctx); statusErr == nil {
		log.Printf("[whatsapp:%s] start request returned error after gateway created the session: %v", c.account.AccountID, err)
		return nil
	}
	return err
}

// runEvents reads gateway websocket messages until the connection closes or the context ends.
func (c *whatsappChannel) runEvents(ctx context.Context) error {
	wsURL, err := c.eventsURL()
	if err != nil {
		return err
	}
	header := http.Header{}
	if auth := c.authHeader(); auth != "" {
		header.Set("Authorization", auth)
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = wsHandshakeTimeout
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	})
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pingDone:
				return
			case <-ticker.C:
				deadline := time.Now().Add(10 * time.Second)
				if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	for ctx.Err() == nil {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		c.handleWSPayload(ctx, payload)
	}
	return ctx.Err()
}

// waitForStatus waits briefly until the gateway exposes the started session status.
func (c *whatsappChannel) waitForStatus(ctx context.Context) error {
	deadline := time.Now().Add(startTimeout)
	for ctx.Err() == nil {
		var response map[string]any
		if err := c.requestJSON(ctx, http.MethodGet, fmt.Sprintf("whatsapp/%s/status", url.PathEscape(c.sessionKey())), nil, &response); err == nil {
			if data, ok := response["data"].(map[string]any); ok {
				c.applySnapshot(data)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("WhatsApp session status did not become available within %s", startTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(defaultReconnect):
		}
	}
	return ctx.Err()
}

// handleWSPayload dispatches a gateway websocket payload to snapshot or event handling.
func (c *whatsappChannel) handleWSPayload(ctx context.Context, payload []byte) {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return
	}
	data, _ := obj["data"].(map[string]any)
	switch strings.TrimSpace(fmt.Sprint(obj["type"])) {
	case "snapshot":
		c.applySnapshot(data)
	case "event":
		c.handleEvent(ctx, data)
	}
}

// handleEvent converts a WhatsApp message event and queues it for ordered processing.
func (c *whatsappChannel) handleEvent(ctx context.Context, item map[string]any) {
	if fmt.Sprint(item["kind"]) != "message" {
		return
	}
	messageID := strings.TrimSpace(fmt.Sprint(item["message_id"]))
	if messageID == "" {
		return
	}
	incoming := core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    fmt.Sprint(item["chat_id"]),
		ChatType:  valueOrDefault(item["chat_type"], "p2p"),
		MessageID: messageID,
		SenderID:  fmt.Sprint(item["sender_id"]),
		Text:      fmt.Sprint(item["text"]),
		Raw:       item,
	}
	_ = c.enqueueIncoming(ctx, incoming)
}

// enqueueIncoming schedules message handling and marks the message seen after a successful handoff.
func (c *whatsappChannel) enqueueIncoming(ctx context.Context, incoming core.IncomingMessage) bool {
	if ctx.Err() != nil {
		return false
	}

	now := time.Now()
	var worker *whatsappChatWorker
	startWorker := false
	queueFull := false

	c.mu.Lock()
	c.pruneSeenLocked(now)
	if _, ok := c.seen[incoming.MessageID]; ok {
		c.mu.Unlock()
		return false
	}
	if c.workers == nil {
		c.workers = map[string]*whatsappChatWorker{}
	}
	worker = c.workers[incoming.ChatID]
	if worker == nil {
		worker = &whatsappChatWorker{queue: make(chan core.IncomingMessage, messageQueueSize)}
		c.workers[incoming.ChatID] = worker
		startWorker = true
	}
	select {
	case worker.queue <- incoming:
		c.seen[incoming.MessageID] = now
		c.mu.Unlock()
		if startWorker {
			go c.runChatWorker(ctx, incoming.ChatID, worker)
		}
		return true
	case <-ctx.Done():
		c.mu.Unlock()
	default:
		queueFull = true
		c.mu.Unlock()
	}
	if startWorker {
		go c.runChatWorker(ctx, incoming.ChatID, worker)
	}
	if queueFull {
		log.Printf("[whatsapp:%s] dropping message %s for chat %s: queue is full", c.account.AccountID, incoming.MessageID, incoming.ChatID)
	}
	return false
}

// runChatWorker processes one chat's inbound messages sequentially.
func (c *whatsappChannel) runChatWorker(ctx context.Context, chatID string, worker *whatsappChatWorker) {
	idle := time.NewTimer(chatWorkerIdle)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-worker.queue:
			if ctx.Err() != nil {
				return
			}
			c.handleIncoming(ctx, msg)
			resetTimer(idle, chatWorkerIdle)
		case <-idle.C:
			if c.retireChatWorker(chatID, worker) {
				return
			}
			resetTimer(idle, chatWorkerIdle)
		}
	}
}

// retireChatWorker removes an idle worker only while it is still current and empty.
func (c *whatsappChannel) retireChatWorker(chatID string, worker *whatsappChatWorker) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.workers[chatID]
	if current != worker {
		return true
	}
	if len(worker.queue) > 0 {
		return false
	}
	if current == worker {
		delete(c.workers, chatID)
	}
	return true
}

// handleIncoming invokes the installed RAGFlow bridge for one queued message.
func (c *whatsappChannel) handleIncoming(ctx context.Context, incoming core.IncomingMessage) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[whatsapp:%s] message handler error: %v", c.account.AccountID, err)
	}
}

// pruneSeenLocked removes expired duplicate-tracking entries while c.mu is held.
func (c *whatsappChannel) pruneSeenLocked(now time.Time) {
	for key, ts := range c.seen {
		if now.Sub(ts) > messageTTL {
			delete(c.seen, key)
		}
	}
}

// applySnapshot updates the cached connection and QR-code state from the gateway.
func (c *whatsappChannel) applySnapshot(snapshot map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSnapshotAt = float64(time.Now().Unix())
	c.status = valueOrDefault(snapshot["status"], "connecting")
	c.lastErr = valueOrDefault(snapshot["last_error"], "")
	c.qrDataURL = valueOrDefault(snapshot["qr_data_url"], "")
	c.qrUpdatedAt = numeric(snapshot["qr_updated_at"])
	c.connectedAt = numeric(snapshot["connected_at"])
	c.sessionID = valueOrDefault(snapshot["session_id"], c.sessionID)
	if cursor := int64(numeric(snapshot["event_cursor"])); cursor > c.eventCursor {
		c.eventCursor = cursor
	}
	if c.status == "connected" {
		c.lastErr = ""
	}
}

// setError records a runtime failure in the snapshot exposed to the API.
func (c *whatsappChannel) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = "error"
	c.lastErr = err.Error()
	c.lastSnapshotAt = float64(time.Now().Unix())
}

// requestJSON sends an authenticated JSON request to the WhatsApp gateway.
func (c *whatsappChannel) requestJSON(ctx context.Context, method, path string, body map[string]any, out any) error {
	return c.requestJSONWithClient(ctx, c.client, method, path, body, out)
}

// requestJSONWithTimeout sends a JSON request with a one-off timeout override.
func (c *whatsappChannel) requestJSONWithTimeout(ctx context.Context, timeout time.Duration, method, path string, body map[string]any, out any) error {
	client := *c.client
	client.Timeout = timeout
	return c.requestJSONWithClient(ctx, &client, method, path, body, out)
}

// requestJSONWithClient sends an authenticated JSON request using the provided HTTP client.
func (c *whatsappChannel) requestJSONWithClient(ctx context.Context, client *http.Client, method, path string, body map[string]any, out any) error {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.account.GatewayBaseURL, "/")+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := c.authHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status: %d, response: %s", resp.StatusCode, string(respBody))
	}
	if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

// sessionKey returns the configured WhatsApp gateway session key.
func (c *whatsappChannel) sessionKey() string {
	if strings.TrimSpace(c.account.SessionKey) == "" {
		return "default"
	}
	return c.account.SessionKey
}

// authHeader formats the optional gateway token as an Authorization header.
func (c *whatsappChannel) authHeader() string {
	token := strings.TrimSpace(c.account.GatewayToken)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}
	return "Bearer " + token
}

// eventsURL builds the websocket URL used to consume gateway events after the cursor.
func (c *whatsappChannel) eventsURL() (string, error) {
	base, err := url.Parse(strings.TrimRight(c.account.GatewayBaseURL, "/"))
	if err != nil {
		return "", err
	}
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/whatsapp/" + url.PathEscape(c.sessionKey()) + "/events/ws"
	q := base.Query()
	c.mu.Lock()
	q.Set("after", strconv.FormatInt(c.eventCursor, 10))
	c.mu.Unlock()
	base.RawQuery = q.Encode()
	return base.String(), nil
}

// firstString returns the first non-empty string value among several config keys.
func firstString(cfg map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(cfg[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

// valueOrDefault converts a snapshot value to string with an empty-value fallback.
func valueOrDefault(value any, fallback string) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return fallback
	}
	return text
}

// numeric converts gateway numeric fields from common JSON-decoded Go types.
func numeric(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	default:
		return 0
	}
}

// stringPtr converts an empty string to nil for Python-compatible JSON null fields.
func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// floatPtr converts zero timestamps to nil for Python-compatible JSON null fields.
func floatPtr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

// resetTimer restarts a timer after draining any pending tick.
func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}
