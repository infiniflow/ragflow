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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/channels/core"
)

const (
	defaultLineWebhookHost = "0.0.0.0"
	defaultLineWebhookPort = 3001
	defaultLineTimeout     = 30 * time.Second
	lineAPIBaseURL         = "https://api.line.me/v2/bot/message"
	lineQueueSize          = 64
	lineWorkerIdle         = 5 * time.Minute
	lineReplyTokenTTL      = 2 * time.Minute
	lineMessageTTL         = 10 * time.Minute
)

type lineAccount struct {
	AccountID          string
	ChannelSecret      string
	ChannelAccessToken string
	WebhookHost        string
	WebhookPort        int
	Timeout            time.Duration
}

type lineChannel struct {
	account lineAccount

	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	handler     core.MessageHandler
	client      *http.Client
	server      *lineWebhookServer
	replyTokens map[string]lineReplyToken
	seen        map[string]time.Time
	workers     map[string]*lineWorker
}

type lineWorker struct {
	queue chan core.IncomingMessage
}

type lineReplyToken struct {
	token     string
	expiresAt time.Time
}

type lineWebhookServer struct {
	host string
	port int

	mu       sync.RWMutex
	server   *http.Server
	listener net.Listener
	channels map[string]*lineChannel
	refs     int
}

var (
	lineServersMu sync.Mutex
	lineServers   = map[string]*lineWebhookServer{}
)

// newLineChannel creates a LINE channel backed by the Messaging API and a shared webhook server.
func newLineChannel(account lineAccount) *lineChannel {
	if account.WebhookHost == "" {
		account.WebhookHost = defaultLineWebhookHost
	}
	if account.WebhookPort <= 0 {
		account.WebhookPort = defaultLineWebhookPort
	}
	if account.Timeout <= 0 {
		account.Timeout = defaultLineTimeout
	}
	return &lineChannel{
		account:     account,
		ctx:         context.Background(),
		client:      &http.Client{Timeout: account.Timeout},
		replyTokens: map[string]lineReplyToken{},
		seen:        map[string]time.Time{},
		workers:     map[string]*lineWorker{},
	}
}

// newLineChannelFromConfig builds a LINE channel from chat_channel.config.credential.
func newLineChannelFromConfig(accountID string, cfg map[string]any) (*lineChannel, error) {
	secret := firstString(cfg, "channel_secret", "channelSecret")
	token := firstString(cfg, "channel_access_token", "channelAccessToken")
	if secret == "" || token == "" {
		return nil, fmt.Errorf("line account %q missing channel_secret or channel_access_token", accountID)
	}

	port := defaultLineWebhookPort
	if raw := firstString(cfg, "webhook_port", "webhookPort"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("line account %q has invalid webhook_port %q", accountID, raw)
		}
		port = parsed
	}

	timeout := defaultLineTimeout
	if raw, ok := cfg["timeout_secs"]; ok {
		if parsed := lineDurationSeconds(raw); parsed > 0 {
			timeout = parsed
		}
	}

	return newLineChannel(lineAccount{
		AccountID:          accountID,
		ChannelSecret:      secret,
		ChannelAccessToken: token,
		WebhookHost:        valueOrDefault(firstString(cfg, "webhook_host", "webhookHost"), defaultLineWebhookHost),
		WebhookPort:        port,
		Timeout:            timeout,
	}), nil
}

func (c *lineChannel) ChannelID() string {
	return "line"
}

func (c *lineChannel) AccountID() string {
	return c.account.AccountID
}

func (c *lineChannel) SetMessageHandler(handler core.MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

func (c *lineChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.ctx = runCtx
	c.cancel = cancel
	c.mu.Unlock()

	server, err := acquireLineWebhookServer(c.account.WebhookHost, c.account.WebhookPort)
	if err != nil {
		c.mu.Lock()
		c.ctx = context.Background()
		c.cancel = nil
		c.mu.Unlock()
		cancel()
		return err
	}
	server.register(c)
	c.mu.Lock()
	c.server = server
	c.mu.Unlock()
	log.Printf("[line:%s] registered at path /line/%s/webhook", c.account.AccountID, c.account.AccountID)
	return nil
}

func (c *lineChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	server := c.server
	c.cancel = nil
	c.ctx = context.Background()
	c.server = nil
	c.replyTokens = map[string]lineReplyToken{}
	c.seen = map[string]time.Time{}
	c.workers = map[string]*lineWorker{}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if server != nil {
		server.unregister(c.account.AccountID)
		return releaseLineWebhookServer(ctx, c.account.WebhookHost, c.account.WebhookPort)
	}
	return nil
}

func (c *lineChannel) Send(ctx context.Context, msg core.OutgoingMessage) error {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}

	if msg.ReplyToMessageID != "" {
		if token := c.takeReplyToken(msg.ReplyToMessageID); token != "" {
			return c.postLineMessage(ctx, "/reply", map[string]any{
				"replyToken": token,
				"messages":   []map[string]string{{"type": "text", "text": text}},
			})
		}
	}
	if strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	return c.postLineMessage(ctx, "/push", map[string]any{
		"to":       msg.ChatID,
		"messages": []map[string]string{{"type": "text", "text": text}},
	})
}

func (c *lineChannel) handleWebhook(body []byte, signature string) (int, string) {
	if !validLineSignature(c.account.ChannelSecret, body, signature) {
		return http.StatusForbidden, "bad signature"
	}

	var payload lineWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return http.StatusBadRequest, "bad request"
	}
	for _, event := range payload.Events {
		incoming, ok := c.normalizeEvent(event)
		if !ok {
			continue
		}
		if event.ReplyToken != "" {
			c.storeReplyToken(incoming.MessageID, event.ReplyToken)
		}
		c.enqueueIncoming(incoming)
	}
	return http.StatusOK, "ok"
}

func (c *lineChannel) normalizeEvent(event lineEvent) (core.IncomingMessage, bool) {
	if event.Type != "message" || event.Message.Type != "text" || event.Message.ID == "" {
		return core.IncomingMessage{}, false
	}
	chatType, chatID := lineChatTypeAndID(event.Source)
	if chatID == "" {
		return core.IncomingMessage{}, false
	}
	raw := map[string]any{}
	if data, err := json.Marshal(event); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	return core.IncomingMessage{
		Channel:   c.ChannelID(),
		AccountID: c.account.AccountID,
		ChatID:    chatID,
		ChatType:  chatType,
		MessageID: event.Message.ID,
		SenderID:  event.Source.UserID,
		Text:      event.Message.Text,
		Raw:       raw,
	}, true
}

// enqueueIncoming schedules LINE message handling and marks it seen after a successful handoff.
func (c *lineChannel) enqueueIncoming(incoming core.IncomingMessage) bool {
	now := time.Now()
	var worker *lineWorker
	startWorker := false
	queueFull := false

	c.mu.Lock()
	c.pruneSeenLocked(now)
	if _, ok := c.seen[incoming.MessageID]; ok {
		c.mu.Unlock()
		return false
	}
	worker = c.workers[incoming.ChatID]
	if worker == nil {
		worker = &lineWorker{queue: make(chan core.IncomingMessage, lineQueueSize)}
		c.workers[incoming.ChatID] = worker
		startWorker = true
	}
	ctx := c.ctx
	select {
	case worker.queue <- incoming:
		c.seen[incoming.MessageID] = now
		c.mu.Unlock()
		if startWorker {
			go c.runWorker(ctx, incoming.ChatID, worker)
		}
		return true
	case <-ctx.Done():
		c.mu.Unlock()
	default:
		queueFull = true
		c.mu.Unlock()
	}

	if startWorker {
		go c.runWorker(ctx, incoming.ChatID, worker)
	}
	if queueFull {
		log.Printf("[line:%s] dropping message %s for chat %s: queue is full", c.account.AccountID, incoming.MessageID, incoming.ChatID)
	}
	return false
}

func (c *lineChannel) runWorker(ctx context.Context, chatID string, worker *lineWorker) {
	idle := time.NewTimer(lineWorkerIdle)
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
			resetTimer(idle, lineWorkerIdle)
		case <-idle.C:
			if c.retireWorker(chatID, worker) {
				return
			}
			resetTimer(idle, lineWorkerIdle)
		}
	}
}

func (c *lineChannel) retireWorker(chatID string, worker *lineWorker) bool {
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

func (c *lineChannel) handleIncoming(ctx context.Context, incoming core.IncomingMessage) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if err := handler(ctx, incoming); err != nil {
		log.Printf("[line:%s] message handler error: %v", c.account.AccountID, err)
	}
}

func (c *lineChannel) storeReplyToken(messageID, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneReplyTokensLocked(time.Now())
	c.replyTokens[messageID] = lineReplyToken{token: token, expiresAt: time.Now().Add(lineReplyTokenTTL)}
}

func (c *lineChannel) takeReplyToken(messageID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.pruneReplyTokensLocked(now)
	entry, ok := c.replyTokens[messageID]
	if !ok || now.After(entry.expiresAt) {
		delete(c.replyTokens, messageID)
		return ""
	}
	delete(c.replyTokens, messageID)
	return entry.token
}

func (c *lineChannel) pruneReplyTokensLocked(now time.Time) {
	for key, entry := range c.replyTokens {
		if now.After(entry.expiresAt) {
			delete(c.replyTokens, key)
		}
	}
}

func (c *lineChannel) pruneSeenLocked(now time.Time) {
	for key, ts := range c.seen {
		if now.Sub(ts) > lineMessageTTL {
			delete(c.seen, key)
		}
	}
}

func (c *lineChannel) postLineMessage(ctx context.Context, path string, body map[string]any) error {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lineAPIBaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.account.ChannelAccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("line send failed: status=%d response=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func acquireLineWebhookServer(host string, port int) (*lineWebhookServer, error) {
	key := lineServerKey(host, port)
	lineServersMu.Lock()
	defer lineServersMu.Unlock()
	if server := lineServers[key]; server != nil {
		server.refs++
		return server, nil
	}

	server := &lineWebhookServer{
		host:     host,
		port:     port,
		channels: map[string]*lineChannel{},
	}
	if err := server.start(); err != nil {
		return nil, err
	}
	server.refs = 1
	lineServers[key] = server
	return server, nil
}

func releaseLineWebhookServer(ctx context.Context, host string, port int) error {
	key := lineServerKey(host, port)
	lineServersMu.Lock()
	server := lineServers[key]
	if server == nil {
		lineServersMu.Unlock()
		return nil
	}
	server.refs--
	if server.refs > 0 {
		lineServersMu.Unlock()
		return nil
	}
	delete(lineServers, key)
	lineServersMu.Unlock()
	return server.stop(ctx)
}

func (s *lineWebhookServer) start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/line/", s.handleRequest)
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = listener
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[line] webhook server exited: %v", err)
		}
	}()
	log.Printf("[line] webhook listening on http://%s/line/<account_id>/webhook", addr)
	return nil
}

func (s *lineWebhookServer) stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *lineWebhookServer) register(ch *lineChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[ch.account.AccountID] = ch
}

func (s *lineWebhookServer) unregister(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, accountID)
}

func (s *lineWebhookServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	accountID, ok := lineWebhookAccountID(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	ch := s.channels[accountID]
	s.mu.RUnlock()
	if ch == nil {
		http.Error(w, "unknown account", http.StatusNotFound)
		return
	}
	status, text := ch.handleWebhook(body, r.Header.Get("X-Line-Signature"))
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}

type lineWebhookPayload struct {
	Events []lineEvent `json:"events"`
}

type lineEvent struct {
	Type       string      `json:"type"`
	ReplyToken string      `json:"replyToken"`
	Source     lineSource  `json:"source"`
	Message    lineMessage `json:"message"`
}

type lineSource struct {
	Type    string `json:"type"`
	UserID  string `json:"userId"`
	GroupID string `json:"groupId"`
	RoomID  string `json:"roomId"`
}

type lineMessage struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

func lineChatTypeAndID(source lineSource) (string, string) {
	switch source.Type {
	case "group":
		return "group", source.GroupID
	case "room":
		return "group", source.RoomID
	case "user":
		return "p2p", source.UserID
	default:
		return source.Type, source.UserID
	}
}

func validLineSignature(secret string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

func lineWebhookAccountID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "line" || parts[2] != "webhook" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func lineServerKey(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func lineDurationSeconds(value any) time.Duration {
	switch v := value.(type) {
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	case json.Number:
		n, _ := v.Int64()
		return time.Duration(n) * time.Second
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return time.Duration(n) * time.Second
	default:
		return 0
	}
}
