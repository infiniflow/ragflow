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
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/channels/core"
)

func TestNewWeComChannelFromConfigValidatesConnectionFields(t *testing.T) {
	if _, err := newWeComChannelFromConfig("account-1", map[string]any{
		"connection_type": "websocket",
		"secret":          "secret",
	}); err == nil {
		t.Fatal("websocket config accepted a missing bot_id")
	}

	channel, err := newWeComChannelFromConfig("account-1", map[string]any{
		"connection_type": "websocket",
		"bot_id":          "bot-id",
		"secret":          "secret",
	})
	if err != nil {
		t.Fatalf("valid websocket config error = %v", err)
	}
	if channel.ChannelID() != "wecom" || channel.AccountID() != "account-1" {
		t.Fatalf("unexpected channel identity: %s:%s", channel.ChannelID(), channel.AccountID())
	}

	if _, err := newWeComChannelFromConfig("account-2", map[string]any{
		"corp_id":  "corp-id",
		"agent_id": "1000001",
		"secret":   "secret",
		"token":    "token",
		"aes_key":  "too-short",
	}); err == nil {
		t.Fatal("webhook config accepted an invalid aes_key")
	}
}

func TestNewWeComChannelFromConfigTreatsUnknownConnectionTypeAsWebhook(t *testing.T) {
	encodingAESKey, _ := testWeComAESKey()
	channel, err := newWeComChannelFromConfig("account-1", map[string]any{
		"connection_type": "unexpected",
		"corp_id":         "corp-id",
		"agent_id":        "1000001",
		"secret":          "secret",
		"token":           "token",
		"aes_key":         encodingAESKey,
	})
	if err != nil {
		t.Fatalf("unknown connection type error = %v", err)
	}
	if channel.account.ConnectionType != "webhook" || channel.crypto == nil {
		t.Fatalf("unknown connection type did not use webhook mode: %+v", channel.account)
	}
}

func TestNewWeComChannelFromConfigAcceptsIntegerAgentID(t *testing.T) {
	encodingAESKey, _ := testWeComAESKey()
	channel, err := newWeComChannelFromConfig("account-1", map[string]any{
		"corp_id":      "corp-id",
		"agent_id":     0,
		"secret":       "secret",
		"token":        "token",
		"aes_key":      encodingAESKey,
		"webhook_port": 0,
	})
	if err != nil {
		t.Fatalf("agent_id=0 error = %v", err)
	}
	if channel.account.AgentID != 0 {
		t.Fatalf("agent_id = %d, want 0", channel.account.AgentID)
	}
	if channel.account.WebhookPort != 0 {
		t.Fatalf("webhook_port = %d, want 0", channel.account.WebhookPort)
	}
}

func TestWeComCryptoDecryptsAndValidatesReceiveID(t *testing.T) {
	encodingAESKey, aesKey := testWeComAESKey()
	crypto, err := newWeComCrypto("token", encodingAESKey, "corp-id")
	if err != nil {
		t.Fatalf("newWeComCrypto() error = %v", err)
	}
	encrypted := encryptWeComTestPayload(t, aesKey, []byte("hello"), "corp-id")
	signature := testWeComSignature("token", "123", "nonce", encrypted)

	plaintext, err := crypto.decrypt(signature, "123", "nonce", encrypted)
	if err != nil {
		t.Fatalf("decrypt() error = %v", err)
	}
	if string(plaintext) != "hello" {
		t.Fatalf("decrypt() = %q, want hello", plaintext)
	}
	if _, err := crypto.decrypt("bad-signature", "123", "nonce", encrypted); err == nil {
		t.Fatal("decrypt() accepted an invalid signature")
	}

	wrongReceiveID := encryptWeComTestPayload(t, aesKey, []byte("hello"), "other-corp")
	wrongSignature := testWeComSignature("token", "123", "nonce", wrongReceiveID)
	if _, err := crypto.decrypt(wrongSignature, "123", "nonce", wrongReceiveID); err == nil {
		t.Fatal("decrypt() accepted a mismatched receive ID")
	}
}

func TestWeComWebhookHandlesVerificationAndTextMessage(t *testing.T) {
	encodingAESKey, aesKey := testWeComAESKey()
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "webhook",
		CorpID:         "corp-id",
		AgentID:        1000001,
		Secret:         "secret",
		Token:          "token",
		AESKey:         encodingAESKey,
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	messages := make(chan core.IncomingMessage, 1)
	channel.SetMessageHandler(func(_ context.Context, msg core.IncomingMessage) error {
		messages <- msg
		return nil
	})
	server := &wecomWebhookServer{channels: map[string]*wecomChannel{"account-1": channel}}

	echo := encryptWeComTestPayload(t, aesKey, []byte("verified"), "corp-id")
	echoQuery := url.Values{
		"msg_signature": {testWeComSignature("token", "123", "nonce", echo)},
		"timestamp":     {"123"},
		"nonce":         {"nonce"},
		"echostr":       {echo},
	}
	echoRequest := httptest.NewRequest(http.MethodGet, "/wecom/account-1/callback?"+echoQuery.Encode(), nil)
	echoResponse := httptest.NewRecorder()
	server.handleRequest(echoResponse, echoRequest)
	if echoResponse.Code != http.StatusOK || echoResponse.Body.String() != "verified" {
		t.Fatalf("verification response = %d %q", echoResponse.Code, echoResponse.Body.String())
	}

	plaintext := []byte(`<xml><ToUserName>corp-id</ToUserName><FromUserName>user-1</FromUserName><CreateTime>123</CreateTime><MsgType>text</MsgType><Content>hello wecom</Content><MsgId>message-1</MsgId></xml>`)
	encrypted := encryptWeComTestPayload(t, aesKey, plaintext, "corp-id")
	body, err := xml.Marshal(wecomEncryptedXML{Encrypt: encrypted})
	if err != nil {
		t.Fatalf("marshal encrypted XML: %v", err)
	}
	query := url.Values{
		"msg_signature": {testWeComSignature("token", "124", "nonce-2", encrypted)},
		"timestamp":     {"124"},
		"nonce":         {"nonce-2"},
	}
	request := httptest.NewRequest(http.MethodPost, "/wecom/account-1/callback?"+query.Encode(), bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.handleRequest(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("message response status = %d, body=%q", response.Code, response.Body.String())
	}

	select {
	case msg := <-messages:
		if msg.ChatID != "user-1" || msg.SenderID != "user-1" || msg.MessageID != "message-1" || msg.Text != "hello wecom" || msg.ChatType != "p2p" {
			t.Fatalf("unexpected webhook message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook message")
	}
}

func TestWeComWebhookAcceptsMalformedEncryptedMessage(t *testing.T) {
	encodingAESKey, _ := testWeComAESKey()
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "webhook",
		CorpID:         "corp-id",
		AgentID:        1000001,
		Secret:         "secret",
		Token:          "token",
		AESKey:         encodingAESKey,
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	server := &wecomWebhookServer{channels: map[string]*wecomChannel{"account-1": channel}}
	request := httptest.NewRequest(http.MethodPost, "/wecom/account-1/callback", strings.NewReader("not xml"))
	response := httptest.NewRecorder()
	server.handleRequest(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("malformed webhook response = %d, want 200", response.Code)
	}
}

func TestWeComWebhookServerSharesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve webhook port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release webhook port: %v", err)
	}

	encodingAESKey, _ := testWeComAESKey()
	newChannel := func(accountID string) *wecomChannel {
		channel, err := newWeComChannel(wecomAccount{
			AccountID:      accountID,
			ConnectionType: "webhook",
			CorpID:         "corp-id",
			AgentID:        1000001,
			Secret:         "secret",
			Token:          "token",
			AESKey:         encodingAESKey,
			WebhookHost:    "127.0.0.1",
			WebhookPort:    port,
		})
		if err != nil {
			t.Fatalf("newWeComChannel(%s) error = %v", accountID, err)
		}
		return channel
	}
	first := newChannel("account-1")
	second := newChannel("account-2")
	if err := first.Start(context.Background()); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer first.Stop(context.Background())
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer second.Stop(context.Background())

	first.mu.Lock()
	firstServer := first.server
	first.mu.Unlock()
	second.mu.Lock()
	secondServer := second.server
	second.mu.Unlock()
	if firstServer == nil || firstServer != secondServer {
		t.Fatal("webhook channels did not share one listener")
	}
	firstServer.mu.RLock()
	refs := firstServer.refs
	registered := len(firstServer.channels)
	firstServer.mu.RUnlock()
	if refs != 2 || registered != 2 {
		t.Fatalf("shared server refs=%d channels=%d, want 2 and 2", refs, registered)
	}

	if err := first.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	firstServer.mu.RLock()
	refs = firstServer.refs
	registered = len(firstServer.channels)
	firstServer.mu.RUnlock()
	if refs != 1 || registered != 1 {
		t.Fatalf("shared server after first stop refs=%d channels=%d, want 1 and 1", refs, registered)
	}
	if err := second.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestWeComApplicationSendCachesAccessToken(t *testing.T) {
	getTokenCalls := 0
	sendCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			getTokenCalls++
			if r.URL.Query().Get("corpid") != "corp-id" || r.URL.Query().Get("corpsecret") != "secret" {
				t.Errorf("unexpected gettoken query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"access-token","expires_in":7200}`))
		case "/message/send":
			sendCalls++
			if r.URL.Query().Get("access_token") != "access-token" {
				t.Errorf("unexpected access_token query: %s", r.URL.RawQuery)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode send payload: %v", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
			if payload["touser"] != "user-1" || payload["agentid"] != float64(1000001) || weComMapValue(payload["text"])["content"] != "answer" {
				t.Errorf("unexpected send payload: %+v", payload)
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	encodingAESKey, _ := testWeComAESKey()
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "webhook",
		CorpID:         "corp-id",
		AgentID:        1000001,
		Secret:         "secret",
		Token:          "token",
		AESKey:         encodingAESKey,
		APIBaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	message := core.OutgoingMessage{ChatID: "user-1", Text: "answer"}
	if err := channel.Send(context.Background(), message); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	if err := channel.Send(context.Background(), message); err != nil {
		t.Fatalf("second Send() error = %v", err)
	}
	if getTokenCalls != 1 || sendCalls != 2 {
		t.Fatalf("gettoken calls = %d, send calls = %d", getTokenCalls, sendCalls)
	}
}

func TestWeComWebSocketSubscribesHandlesAndSends(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	outgoing := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		var subscribe map[string]any
		if err := conn.ReadJSON(&subscribe); err != nil {
			t.Errorf("read subscribe: %v", err)
			return
		}
		if subscribe["cmd"] != "aibot_subscribe" || weComMapValue(subscribe["body"])["bot_id"] != "bot-id" {
			t.Errorf("unexpected subscribe payload: %+v", subscribe)
		}
		if err := conn.WriteJSON(map[string]any{"cmd": "aibot_subscribe", "errcode": 0, "errmsg": "ok"}); err != nil {
			t.Errorf("write subscribe response: %v", err)
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"cmd":     "aibot_msg_callback",
			"headers": map[string]any{"req_id": "request-1"},
			"body": map[string]any{
				"msgid":    "message-1",
				"msgtype":  "text",
				"chattype": "group",
				"chatid":   "chat-1",
				"from":     map[string]any{"userid": "user-1"},
				"text":     map[string]any{"content": "question"},
			},
		}); err != nil {
			t.Errorf("write callback: %v", err)
			return
		}
		var response map[string]any
		if err := conn.ReadJSON(&response); err != nil {
			t.Errorf("read outgoing response: %v", err)
			return
		}
		outgoing <- response
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "websocket",
		BotID:          "bot-id",
		Secret:         "secret",
		WSURL:          wsURL,
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	incoming := make(chan core.IncomingMessage, 1)
	channel.SetMessageHandler(func(ctx context.Context, msg core.IncomingMessage) error {
		incoming <- msg
		return channel.Send(ctx, core.OutgoingMessage{ChatID: msg.ChatID, Text: "answer", ReplyToMessageID: msg.MessageID})
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := channel.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case msg := <-incoming:
		if msg.ChatID != "chat-1" || msg.ChatType != "group" || msg.SenderID != "user-1" || msg.MessageID != "request-1" || msg.Text != "question" {
			t.Fatalf("unexpected websocket message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket message")
	}
	select {
	case response := <-outgoing:
		body := weComMapValue(response["body"])
		if response["cmd"] != "aibot_send_msg" || body["chatid"] != "chat-1" || weComMapValue(body["markdown"])["content"] != "answer" {
			t.Fatalf("unexpected websocket response: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket response")
	}
	if err := channel.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestWeComWebSocketMessagePreservesWhitespaceAndFallsBackToSender(t *testing.T) {
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "websocket",
		BotID:          "bot-id",
		Secret:         "secret",
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	messages := make(chan core.IncomingMessage, 1)
	channel.SetMessageHandler(func(_ context.Context, msg core.IncomingMessage) error {
		messages <- msg
		return nil
	})
	channel.handleWebSocketMessage(context.Background(), map[string]any{
		"headers": map[string]any{"req_id": "request-1"},
		"body": map[string]any{
			"msgtype": "text",
			"from":    map[string]any{"userid": "user-1"},
			"text":    map[string]any{"content": "  question\n"},
		},
	})
	select {
	case message := <-messages:
		if message.ChatID != "user-1" || message.Text != "  question\n" {
			t.Fatalf("unexpected websocket message: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket message")
	}
}

func TestWeComSendIgnoresTransportErrors(t *testing.T) {
	channel, err := newWeComChannel(wecomAccount{
		AccountID:      "account-1",
		ConnectionType: "websocket",
		BotID:          "bot-id",
		Secret:         "secret",
	})
	if err != nil {
		t.Fatalf("newWeComChannel() error = %v", err)
	}
	if err := channel.Send(context.Background(), core.OutgoingMessage{ChatID: "chat-1", Text: "answer"}); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
}

func testWeComAESKey() (string, []byte) {
	key := []byte("0123456789abcdef0123456789abcdef")
	return strings.TrimSuffix(base64.StdEncoding.EncodeToString(key), "="), key
}

func encryptWeComTestPayload(t *testing.T, key, message []byte, receiveID string) string {
	t.Helper()
	payload := make([]byte, 20+len(message)+len(receiveID))
	copy(payload[:16], []byte("0123456789abcdef"))
	binary.BigEndian.PutUint32(payload[16:20], uint32(len(message)))
	copy(payload[20:], message)
	copy(payload[20+len(message):], receiveID)
	padding := weComPKCS7BlockSize - len(payload)%weComPKCS7BlockSize
	payload = append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...)

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("create test cipher: %v", err)
	}
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, key[:block.BlockSize()]).CryptBlocks(ciphertext, payload)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func testWeComSignature(token, timestamp, nonce, encrypted string) string {
	parts := []string{token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	hash := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(hash[:])
}
