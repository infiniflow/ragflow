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
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	weComWebhookBodyLimit       = 1024 * 1024
	weComWebhookShutdownTimeout = 5 * time.Second
	weComPKCS7BlockSize         = 32
)

type wecomCrypto struct {
	token     string
	aesKey    []byte
	receiveID string
}

type wecomEncryptedXML struct {
	Encrypt string `xml:"Encrypt"`
}

type wecomXMLMessage struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgID        string `xml:"MsgId"`
}

type wecomWebhookServerKey struct {
	host string
	port int
}

type wecomWebhookServer struct {
	key      wecomWebhookServerKey
	server   *http.Server
	listener net.Listener

	mu       sync.RWMutex
	channels map[string]*wecomChannel
	refs     int
}

var sharedWeComWebhookServers = struct {
	sync.Mutex
	servers map[wecomWebhookServerKey]*wecomWebhookServer
}{
	servers: map[wecomWebhookServerKey]*wecomWebhookServer{},
}

func newWeComCrypto(token, encodingAESKey, receiveID string) (*wecomCrypto, error) {
	if len(encodingAESKey) != 43 {
		return nil, fmt.Errorf("WeCom EncodingAESKey must be 43 characters, got %d", len(encodingAESKey))
	}
	aesKey, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("decode WeCom EncodingAESKey: %w", err)
	}
	if len(aesKey) != 32 {
		return nil, fmt.Errorf("decoded WeCom AES key must be 32 bytes, got %d", len(aesKey))
	}
	return &wecomCrypto{
		token:     token,
		aesKey:    aesKey,
		receiveID: receiveID,
	}, nil
}

func (c *wecomCrypto) decrypt(signature, timestamp, nonce, encrypted string) ([]byte, error) {
	if signature == "" || timestamp == "" || nonce == "" || encrypted == "" {
		return nil, errors.New("missing WeCom signature parameters")
	}
	if !c.hasValidSignature(signature, timestamp, nonce, encrypted) {
		return nil, errors.New("invalid WeCom message signature")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decode WeCom ciphertext: %w", err)
	}
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("invalid WeCom ciphertext length")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, c.aesKey[:block.BlockSize()]).CryptBlocks(plaintext, ciphertext)
	plaintext, err = removeWeComPKCS7Padding(plaintext)
	if err != nil {
		return nil, err
	}
	if len(plaintext) < 20 {
		return nil, errors.New("invalid WeCom decrypted payload")
	}
	messageLength := int(binary.BigEndian.Uint32(plaintext[16:20]))
	messageEnd := 20 + messageLength
	if messageLength < 0 || messageEnd > len(plaintext) {
		return nil, errors.New("invalid WeCom decrypted message length")
	}
	if receiveID := string(plaintext[messageEnd:]); c.receiveID != "" && receiveID != c.receiveID {
		return nil, fmt.Errorf("WeCom receive ID mismatch: got %q", receiveID)
	}
	return plaintext[20:messageEnd], nil
}

func (c *wecomCrypto) hasValidSignature(signature, timestamp, nonce, encrypted string) bool {
	parts := []string{c.token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	hash := sha1.Sum([]byte(strings.Join(parts, "")))
	expected := hex.EncodeToString(hash[:])
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) == 1
}

func removeWeComPKCS7Padding(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty WeCom padded payload")
	}
	padding := int(payload[len(payload)-1])
	if padding < 1 || padding > weComPKCS7BlockSize || padding > len(payload) {
		return nil, errors.New("invalid WeCom PKCS#7 padding")
	}
	for _, value := range payload[len(payload)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid WeCom PKCS#7 padding bytes")
		}
	}
	return payload[:len(payload)-padding], nil
}

func acquireWeComWebhookServer(channel *wecomChannel) (*wecomWebhookServer, error) {
	key := wecomWebhookServerKey{
		host: channel.account.WebhookHost,
		port: channel.account.WebhookPort,
	}
	sharedWeComWebhookServers.Lock()
	defer sharedWeComWebhookServers.Unlock()

	if server := sharedWeComWebhookServers.servers[key]; server != nil {
		server.mu.Lock()
		server.channels[channel.account.AccountID] = channel
		server.refs++
		server.mu.Unlock()
		return server, nil
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(key.host, strconv.Itoa(key.port)))
	if err != nil {
		return nil, fmt.Errorf("start WeCom webhook listener: %w", err)
	}
	server := &wecomWebhookServer{
		key:      key,
		listener: listener,
		channels: map[string]*wecomChannel{
			channel.account.AccountID: channel,
		},
		refs: 1,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/wecom/", server.handleRequest)
	server.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	sharedWeComWebhookServers.servers[key] = server

	go func() {
		if err := server.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[wecom] webhook server %s stopped: %v", listener.Addr(), err)
		}
	}()
	log.Printf("[wecom] webhook listening on http://%s/wecom/<account_id>/callback", listener.Addr())
	return server, nil
}

func releaseWeComWebhookServer(ctx context.Context, server *wecomWebhookServer, accountID string) error {
	sharedWeComWebhookServers.Lock()
	server.mu.Lock()
	delete(server.channels, accountID)
	if server.refs > 0 {
		server.refs--
	}
	remaining := server.refs
	server.mu.Unlock()
	if remaining > 0 {
		sharedWeComWebhookServers.Unlock()
		return nil
	}
	if sharedWeComWebhookServers.servers[server.key] == server {
		delete(sharedWeComWebhookServers.servers, server.key)
	}
	sharedWeComWebhookServers.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, weComWebhookShutdownTimeout)
	defer cancel()
	return server.server.Shutdown(shutdownCtx)
}

func (s *wecomWebhookServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	accountID, ok := weComAccountIDFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	channel := s.channels[accountID]
	s.mu.RUnlock()
	if channel == nil {
		http.Error(w, "unknown account", http.StatusNotFound)
		return
	}

	query := r.URL.Query()
	signature := query.Get("msg_signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	if r.Method == http.MethodGet {
		plaintext, err := channel.crypto.decrypt(signature, timestamp, nonce, query.Get("echostr"))
		if err != nil {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(plaintext)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, weComWebhookBodyLimit))
	if err != nil {
		log.Printf("[wecom:%s] failed to read request body: %v", accountID, err)
		w.WriteHeader(http.StatusOK)
		return
	}
	var envelope wecomEncryptedXML
	if err := xml.Unmarshal(body, &envelope); err != nil || strings.TrimSpace(envelope.Encrypt) == "" {
		log.Printf("[wecom:%s] invalid encrypted message: %v", accountID, err)
		w.WriteHeader(http.StatusOK)
		return
	}
	plaintext, err := channel.crypto.decrypt(signature, timestamp, nonce, envelope.Encrypt)
	if err != nil {
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}
	var message wecomXMLMessage
	if err := xml.Unmarshal(plaintext, &message); err != nil {
		log.Printf("[wecom:%s] failed to parse decrypted message: %v", accountID, err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if message.MsgType == "text" && message.FromUserName != "" {
		channel.handleTextMessage(r.Context(), message.FromUserName, message.FromUserName, message.MsgID, message.Content, "p2p", map[string]any{
			"to_user_name":   message.ToUserName,
			"from_user_name": message.FromUserName,
			"create_time":    message.CreateTime,
			"msg_type":       message.MsgType,
			"content":        message.Content,
			"msg_id":         message.MsgID,
			"xml":            string(plaintext),
		})
	}
	w.WriteHeader(http.StatusOK)
}

func weComAccountIDFromPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "wecom" || parts[2] != "callback" {
		return "", false
	}
	accountID, err := url.PathUnescape(parts[1])
	return accountID, err == nil && accountID != ""
}
