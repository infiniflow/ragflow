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
	defaultDiscordAPIBaseURL = "https://discord.com/api/v10"
	discordGatewayVersion    = "10"
	defaultDiscordTimeout    = 30 * time.Second
	discordHandshakeTimeout  = 30 * time.Second
	discordReadTimeout       = 2 * time.Minute
	discordWriteTimeout      = 10 * time.Second
	discordReconnectDelay    = 5 * time.Second
	discordMessageQueueSize  = 64
	discordMessageTTL        = time.Hour
	discordChatWorkerIdle    = 5 * time.Minute
	discordDefaultIntents    = 1<<0 | 1<<9 | 1<<12 | 1<<15
	discordSendMaxAttempts   = 3
	discordMaxRetryAfter     = 30 * time.Second
)

type discordAccount struct {
	AccountID  string
	Token      string
	APIBaseURL string
	GatewayURL string
	Timeout    time.Duration
	Intents    int
}

type discordChannel struct {
	account discordAccount

	mu      sync.Mutex
	cancel  context.CancelFunc
	conn    *websocket.Conn
	handler core.MessageHandler
	client  *http.Client
	selfID  string
	seen    map[string]time.Time
	workers map[string]*discordChatWorker
	writeMu sync.Mutex
	lastSeq atomic.Int64
	hasSeq  atomic.Bool
}

type discordChatWorker struct {
	queue chan core.IncomingMessage
}

type discordGatewayPayload struct {
	Op int             `json:"op"` // Gateway opcode, which indicates the payload type
	D  json.RawMessage `json:"d"`  // Event data
	S  *int64          `json:"s"`  // Sequence number of event used for resuming sessions and heartbeating
	T  string          `json:"t"`  // Event name
}

type discordGatewayBotResponse struct {
	URL string `json:"url"`
}

type discordGatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type discordGatewayReady struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
}

type discordMessageCreate struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channel_id"`
	GuildID     string `json:"guild_id"`
	Content     string `json:"content"`
	ChannelType *int   `json:"channel_type"`
	Author      struct {
		ID  string `json:"id"`
		Bot bool   `json:"bot"`
	} `json:"author"`
}

type discordRateLimitError struct {
	RetryAfter time.Duration
	Global     bool
	Response   string
}

// Error formats a Discord rate-limit response with retry metadata.
func (e *discordRateLimitError) Error() string {
	scope := "route"
	if e.Global {
		scope = "global"
	}
	return fmt.Sprintf("discord api rate limited (%s), retry_after=%s, response: %s", scope, e.RetryAfter, e.Response)
}

// newDiscordChannel creates a Discord bot channel with default REST and Gateway settings.
func newDiscordChannel(account discordAccount) *discordChannel {
	account.Token = discordBotToken(account.Token)
	if account.APIBaseURL == "" {
		account.APIBaseURL = defaultDiscordAPIBaseURL
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultDiscordTimeout
	}
	if account.Intents == 0 {
		account.Intents = discordDefaultIntents
	}
	return &discordChannel{
		account: account,
		client:  &http.Client{Timeout: account.Timeout},
		seen:    map[string]time.Time{},
		workers: map[string]*discordChatWorker{},
	}
}

// newDiscordChannelFromConfig builds a Discord channel from chat_channel.config.credential.
func newDiscordChannelFromConfig(accountID string, cfg map[string]any) (*discordChannel, error) {
	token := discordBotToken(firstString(cfg, "token", "bot_token"))
	if token == "" {
		return nil, fmt.Errorf("discord account %q is missing token", accountID)
	}

	timeout := defaultDiscordTimeout
	if raw, ok := cfg["timeout_secs"]; ok {
		if parsed := durationSeconds(raw); parsed > 0 {
			timeout = parsed
		}
	}

	return newDiscordChannel(discordAccount{
		AccountID:  accountID,
		Token:      token,
		APIBaseURL: firstString(cfg, "api_base_url", "api_url"),
		GatewayURL: firstString(cfg, "gateway_url"),
		Timeout:    timeout,
		Intents:    discordIntentsFromConfig(cfg),
	}), nil
}

// ChannelID returns the platform identifier used by the chat-channel runtime.
func (c *discordChannel) ChannelID() string {
	return "discord"
}

// AccountID returns the chat_channel.id bound to this Discord runtime instance.
func (c *discordChannel) AccountID() string {
	return c.account.AccountID
}

// SetMessageHandler installs the RAGFlow bridge invoked for inbound Discord messages.
func (c *discordChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start begins the Discord Gateway receive loop.
func (c *discordChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	go c.run(runCtx)
	return nil
}

// Stop cancels the Discord Gateway loop and closes the websocket connection.
func (c *discordChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	conn := c.conn
	c.cancel = nil
	c.conn = nil
	c.selfID = ""
	c.seen = map[string]time.Time{}
	c.workers = map[string]*discordChatWorker{}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	return nil
}

// Send posts an outgoing RAGFlow answer to a Discord channel.
func (c *discordChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	if strings.TrimSpace(msg.Text) == "" {
		return nil
	}
	payload := map[string]any{
		"content":          msg.Text,
		"allowed_mentions": map[string]any{"parse": []string{}},
	}
	if strings.TrimSpace(msg.ReplyToMessageID) != "" {
		payload["message_reference"] = map[string]any{
			"message_id":         msg.ReplyToMessageID,
			"channel_id":         msg.ChatID,
			"fail_if_not_exists": false,
		}
	}

	path := "/channels/" + url.PathEscape(msg.ChatID) + "/messages"
	var lastErr error
	for attempt := 1; attempt <= discordSendMaxAttempts; attempt++ {
		err := c.requestJSON(ctx, http.MethodPost, path, payload, nil)
		if err == nil {
			return nil
		}

		var rateLimitErr *discordRateLimitError
		if !errors.As(err, &rateLimitErr) {
			return err
		}
		lastErr = err
		if attempt == discordSendMaxAttempts || rateLimitErr.RetryAfter > discordMaxRetryAfter {
			return err
		}

		wait := rateLimitErr.RetryAfter
		if wait <= 0 {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// run reconnects to Discord Gateway until the channel is stopped.
func (c *discordChannel) run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.runGateway(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[discord:%s] gateway error: %v", c.account.AccountID, err)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(discordReconnectDelay):
		}
	}
}

// runGateway connects one Discord Gateway websocket session and reads events.
func (c *discordChannel) runGateway(ctx context.Context) error {
	// get websocket URL
	wsURL, err := c.gatewayURL(ctx)
	if err != nil {
		return err
	}

	// set websocket connection
	header := http.Header{}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = discordHandshakeTimeout

	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.setConn(conn)
	defer c.clearConn(conn)

	closeOnCancel := make(chan struct{})
	defer close(closeOnCancel)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closeOnCancel:
		}
	}()

	hello, err := c.readHello(conn)
	if err != nil {
		return err
	}
	if err := c.writeGatewayPayload(conn, 2, map[string]any{
		"token":   c.account.Token,
		"intents": c.account.Intents,
		"properties": map[string]string{
			"os":      "linux",
			"browser": "ragflow",
			"device":  "ragflow",
		},
	}); err != nil {
		return err
	}

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go c.heartbeatLoop(ctx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond, heartbeatDone)

	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(discordReadTimeout))
		var payload discordGatewayPayload
		if err := conn.ReadJSON(&payload); err != nil {
			return err
		}
		if payload.S != nil {
			c.lastSeq.Store(*payload.S)
			c.hasSeq.Store(true)
		}
		if err := c.handleGatewayPayload(ctx, conn, payload); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// readHello waits for the Discord Gateway hello frame that carries the heartbeat interval.
func (c *discordChannel) readHello(conn *websocket.Conn) (discordGatewayHello, error) {
	_ = conn.SetReadDeadline(time.Now().Add(discordReadTimeout))
	for {
		var payload discordGatewayPayload
		if err := conn.ReadJSON(&payload); err != nil {
			return discordGatewayHello{}, err
		}
		if payload.Op != 10 {
			continue
		}

		var hello discordGatewayHello
		if err := json.Unmarshal(payload.D, &hello); err != nil {
			return discordGatewayHello{}, err
		}
		if hello.HeartbeatInterval <= 0 {
			return discordGatewayHello{}, errors.New("discord gateway hello is missing heartbeat interval")
		}

		return hello, nil
	}
}

// heartbeatLoop keeps the Discord Gateway websocket session alive.
func (c *discordChannel) heartbeatLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration, done <-chan struct{}) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if err := c.writeHeartbeat(conn); err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

// handleGatewayPayload processes Discord Gateway opcodes and dispatch events.
func (c *discordChannel) handleGatewayPayload(ctx context.Context, conn *websocket.Conn, payload discordGatewayPayload) error {
	switch payload.Op {
	case 0:
		return c.handleDispatch(ctx, payload)
	case 1:
		return c.writeHeartbeat(conn)
	case 7:
		return errors.New("discord gateway requested reconnect")
	case 9:
		return errors.New("discord gateway invalid session")
	case 11:
		return nil
	default:
		return nil
	}
}

// handleDispatch converts supported Discord Gateway dispatches to chat-channel messages.
func (c *discordChannel) handleDispatch(ctx context.Context, payload discordGatewayPayload) error {
	switch payload.T {
	case "READY":
		var ready discordGatewayReady
		if err := json.Unmarshal(payload.D, &ready); err != nil {
			return err
		}

		c.mu.Lock()
		c.selfID = ready.User.ID
		c.mu.Unlock()

		log.Printf("[discord:%s] connected as bot id %s", c.account.AccountID, ready.User.ID)
	case "MESSAGE_CREATE":
		c.handleMessageCreate(ctx, payload.D)
	}
	return nil
}

// handleMessageCreate filters and queues inbound Discord messages for RAGFlow.
func (c *discordChannel) handleMessageCreate(ctx context.Context, raw json.RawMessage) {
	var message discordMessageCreate
	if err := json.Unmarshal(raw, &message); err != nil {
		return
	}

	if message.ID == "" || message.ChannelID == "" || message.Author.ID == "" {
		return
	}
	if message.Author.Bot {
		return
	}

	c.mu.Lock()
	selfID := c.selfID
	c.mu.Unlock()
	if selfID != "" && message.Author.ID == selfID {
		return
	}

	rawMap := map[string]any{}
	_ = json.Unmarshal(raw, &rawMap)
	incoming := core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    message.ChannelID,
		ChatType:  discordChatType(message.GuildID, message.ChannelType),
		MessageID: message.ID,
		SenderID:  message.Author.ID,
		Text:      message.Content,
		Raw:       rawMap,
	}
	_ = c.enqueueIncoming(ctx, incoming)
}

// enqueueIncoming schedules Discord message handling and marks it seen after a successful handoff.
func (c *discordChannel) enqueueIncoming(ctx context.Context, incoming core.IncomingMessage) bool {
	if ctx.Err() != nil {
		return false
	}

	now := time.Now()
	var worker *discordChatWorker
	startWorker := false
	queueFull := false

	c.mu.Lock()
	c.pruneSeenLocked(now)
	if _, ok := c.seen[incoming.MessageID]; ok {
		c.mu.Unlock()
		return false
	}

	if c.workers == nil {
		c.workers = map[string]*discordChatWorker{}
	}

	worker = c.workers[incoming.ChatID]
	if worker == nil {
		worker = &discordChatWorker{queue: make(chan core.IncomingMessage, discordMessageQueueSize)}
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
		log.Printf("[discord:%s] dropping message %s for chat %s: queue is full", c.account.AccountID, incoming.MessageID, incoming.ChatID)
	}

	return false
}

// runChatWorker processes one Discord chat's inbound messages sequentially.
func (c *discordChannel) runChatWorker(ctx context.Context, chatID string, worker *discordChatWorker) {
	idle := time.NewTimer(discordChatWorkerIdle)
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
			resetTimer(idle, discordChatWorkerIdle)
		case <-idle.C:
			if c.retireChatWorker(chatID, worker) {
				return
			}
			resetTimer(idle, discordChatWorkerIdle)
		}
	}
}

// retireChatWorker removes an idle Discord chat worker only while it is current and empty.
func (c *discordChannel) retireChatWorker(chatID string, worker *discordChatWorker) bool {
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

// handleIncoming invokes the installed RAGFlow bridge for one queued Discord message.
func (c *discordChannel) handleIncoming(ctx context.Context, incoming core.IncomingMessage) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()

	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[discord:%s] message handler error: %v", c.account.AccountID, err)
	}
}

// pruneSeenLocked removes expired duplicate-tracking entries while c.mu is held.
func (c *discordChannel) pruneSeenLocked(now time.Time) {
	for key, ts := range c.seen {
		if now.Sub(ts) > discordMessageTTL {
			delete(c.seen, key)
		}
	}
}

// requestJSON sends an authenticated JSON request to the Discord REST API.
func (c *discordChannel) requestJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.account.APIBaseURL, "/")+"/"+strings.TrimLeft(path, "/"), reader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", discordAuthHeader(c.account.Token))

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusTooManyRequests {
			return newDiscordRateLimitError(resp.Header, respBody)
		}
		return fmt.Errorf("discord api status: %d, response: %s", resp.StatusCode, string(respBody))
	}
	if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

// newDiscordRateLimitError extracts Discord 429 retry metadata from headers and response body.
func newDiscordRateLimitError(header http.Header, body []byte) *discordRateLimitError {
	var payload struct {
		RetryAfter any    `json:"retry_after"`
		Global     bool   `json:"global"`
		Message    string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)

	retryAfter := retryAfterFromHeader(header)
	if retryAfter <= 0 {
		retryAfter = retryAfterFromValue(payload.RetryAfter)
	}

	return &discordRateLimitError{
		RetryAfter: retryAfter,
		Global:     payload.Global,
		Response:   strings.TrimSpace(string(body)),
	}
}

// retryAfterFromHeader converts Discord Retry-After headers to a duration.
func retryAfterFromHeader(header http.Header) time.Duration {
	for _, key := range []string{"Retry-After", "X-RateLimit-Reset-After"} {
		if duration := retryAfterFromValue(header.Get(key)); duration > 0 {
			return duration
		}
	}
	return 0
}

// retryAfterFromValue converts Discord retry_after seconds into a duration.
func retryAfterFromValue(value any) time.Duration {
	switch v := value.(type) {
	case float64:
		return time.Duration(v * float64(time.Second))
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0
		}
		return time.Duration(n * float64(time.Second))
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return time.Duration(n * float64(time.Second))
	default:
		return 0
	}
}

// gatewayURL returns the websocket URL used to receive Discord Gateway events.
func (c *discordChannel) gatewayURL(ctx context.Context) (string, error) {
	raw := strings.TrimSpace(c.account.GatewayURL)
	if raw == "" {
		var response discordGatewayBotResponse
		if err := c.requestJSON(ctx, http.MethodGet, "/gateway/bot", nil, &response); err != nil {
			return "", err
		}
		raw = response.URL
	}
	if raw == "" {
		return "", errors.New("discord gateway url is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported discord gateway scheme %q", parsed.Scheme)
	}

	q := parsed.Query()
	q.Set("v", discordGatewayVersion)
	q.Set("encoding", "json")

	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// writeHeartbeat sends the latest acknowledged Discord Gateway sequence.
func (c *discordChannel) writeHeartbeat(conn *websocket.Conn) error {
	var data any
	if c.hasSeq.Load() {
		data = c.lastSeq.Load()
	}
	return c.writeGatewayPayload(conn, 1, data)
}

// writeGatewayPayload writes one JSON frame to Discord Gateway.
func (c *discordChannel) writeGatewayPayload(conn *websocket.Conn, op int, data any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(discordWriteTimeout))
	return conn.WriteJSON(map[string]any{
		"op": op,
		"d":  data,
	})
}

// setConn records the current websocket so Stop can close it.
func (c *discordChannel) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}

// clearConn removes a websocket only if it still belongs to the current Gateway run.
func (c *discordChannel) clearConn(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
}

// discordBotToken normalizes a configured Discord token for Gateway identify.
func discordBotToken(token string) string {
	token = strings.TrimSpace(token)
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "bot ") {
		return strings.TrimSpace(token[4:])
	}
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(token[7:])
	}
	return token
}

// discordAuthHeader formats the configured Discord bot token for REST API calls.
func discordAuthHeader(token string) string {
	token = discordBotToken(token)
	if token == "" {
		return ""
	}
	return "Bot " + token
}

// discordIntentsFromConfig reads optional Discord gateway intents from credential config.
func discordIntentsFromConfig(cfg map[string]any) int {
	for _, key := range []string{"intents", "gateway_intents"} {
		if value, ok := intValue(cfg[key]); ok && value > 0 {
			return value
		}
	}
	return discordDefaultIntents
}

// discordChatType maps Discord channel metadata onto RAGFlow chat-channel chat types.
func discordChatType(guildID string, channelType *int) string {
	if channelType != nil {
		switch *channelType {
		case 1:
			return "p2p"
		case 10, 11, 12:
			return "thread"
		case 0, 2, 5, 13, 15, 16:
			return "group"
		}
	}
	if strings.TrimSpace(guildID) == "" {
		return "p2p"
	}
	return "group"
}

// intValue converts common JSON-decoded config values to int.
func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}

// durationSeconds converts a config value measured in seconds to a duration.
func durationSeconds(value any) time.Duration {
	if seconds, ok := intValue(value); ok && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
