// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

const (
	feishuMaxMessageSize     = 16 << 20
	feishuMaxPendingMessages = 128
	feishuMaxMessageParts    = 1024
)

type feishuMessageParts struct {
	chunks    [][]byte
	expiresAt time.Time
	size      int
}

// feishuWebSocketSession owns its fragment cache and workers. The SDK client
// starts a cache cleanup goroutine that its public lifecycle cannot release.
type feishuWebSocketSession struct {
	conn         *websocket.Conn
	handler      *dispatcher.EventDispatcher
	accountID    string
	writeMu      sync.Mutex
	pingInterval atomic.Int64
	parts        map[string]*feishuMessageParts
	pendingBytes int
}

func (c *feishuChannel) runWebSocketSession(ctx context.Context) (bool, error) {
	endpoint, err := c.getWebSocketEndpoint(ctx)
	if err != nil {
		return false, err
	}
	gatewayURL, err := url.Parse(endpoint.Url)
	if err != nil {
		return false, redactURLError(err)
	}
	var serviceID int64
	if raw := gatewayURL.Query().Get(larkws.ServiceID); raw != "" {
		serviceID, err = strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return false, fmt.Errorf("invalid Feishu WebSocket service ID: %w", err)
		}
	}
	conn, _, err := dialSafeChannelWebSocket(ctx, endpoint.Url, nil, c.account.Timeout, channelURLPolicy{exactHosts: []string{"msg-frontier.feishu.cn", "msg-frontier.larksuite.com"}})
	if err != nil {
		return false, redactURLError(err)
	}
	defer conn.Close()
	conn.SetReadLimit(feishuMaxMessageSize)
	session := &feishuWebSocketSession{
		conn:      conn,
		handler:   c.eventDispatcher(),
		accountID: c.account.AccountID,
		parts:     map[string]*feishuMessageParts{},
	}
	session.pingInterval.Store(int64(2 * time.Minute))
	if endpoint.ClientConfig != nil && endpoint.ClientConfig.PingInterval > 0 {
		session.pingInterval.Store(int64(time.Duration(endpoint.ClientConfig.PingInterval) * time.Second))
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-sessionCtx.Done()
		_ = conn.Close()
	}()
	go func() {
		defer workers.Done()
		for sessionCtx.Err() == nil {
			if err := session.writeFrame(larkws.NewPingFrame(int32(serviceID))); err != nil {
				common.Warn("feishu: WebSocket heartbeat failed", zap.String("account_id", c.account.AccountID), zap.Error(err))
				_ = conn.Close()
				return
			}
			if !waitForContext(sessionCtx, time.Duration(session.pingInterval.Load())) {
				return
			}
		}
	}()
	defer func() {
		cancel()
		workers.Wait()
	}()
	for ctx.Err() == nil {
		if err := conn.SetReadDeadline(time.Now().Add(2*time.Duration(session.pingInterval.Load()) + 5*time.Second)); err != nil {
			return true, err
		}
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return true, err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		var frame larkws.Frame
		if err := frame.Unmarshal(data); err != nil {
			common.Warn("feishu: invalid WebSocket frame", zap.String("account_id", c.account.AccountID), zap.Error(err))
			continue
		}
		if err := session.handleFrame(ctx, &frame); err != nil {
			return true, err
		}
	}
	return true, ctx.Err()
}

func (c *feishuChannel) getWebSocketEndpoint(ctx context.Context) (*larkws.Endpoint, error) {
	body, err := json.Marshal(larkws.BootstrapRequest{AppID: c.account.AppID, AppSecret: c.account.AppSecret})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuBaseURL(c.account.Domain)+larkws.GenEndpointUri, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("locale", "zh")
	req.Header.Set("User-Agent", larkcore.UserAgent(""))
	response, err := c.client.Do(req)
	if err != nil {
		return nil, redactURLError(err)
	}
	defer response.Body.Close()
	var result larkws.EndpointResp
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Feishu WebSocket endpoint: %w", err)
	}
	if response.StatusCode != http.StatusOK || result.Code != larkws.OK {
		return nil, fmt.Errorf("Feishu WebSocket endpoint failed: status=%d code=%d message=%s", response.StatusCode, result.Code, result.Msg)
	}
	if result.Data == nil || result.Data.Url == "" {
		return nil, fmt.Errorf("Feishu WebSocket endpoint is missing")
	}
	return result.Data, nil
}

func (s *feishuWebSocketSession) writeFrame(frame *larkws.Frame) error {
	data, err := frame.Marshal()
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (s *feishuWebSocketSession) handleFrame(ctx context.Context, frame *larkws.Frame) error {
	headers := larkws.Headers(frame.Headers)
	if larkws.FrameType(frame.Method) == larkws.FrameTypeControl {
		if headers.GetString(larkws.HeaderType) == string(larkws.MessageTypePong) && len(frame.Payload) > 0 {
			var config larkws.ClientConfig
			if err := json.Unmarshal(frame.Payload, &config); err != nil {
				common.Warn("feishu: invalid WebSocket config", zap.String("account_id", s.accountID), zap.Error(err))
			} else if config.PingInterval > 0 {
				s.pingInterval.Store(int64(time.Duration(config.PingInterval) * time.Second))
			}
		}
		return nil
	}
	if larkws.FrameType(frame.Method) != larkws.FrameTypeData || headers.GetString(larkws.HeaderType) != string(larkws.MessageTypeEvent) {
		return nil
	}
	data, err := s.combineMessage(headers, frame.Payload)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	startedAt := time.Now()
	result, handlerErr := s.handler.Do(ctx, data)
	response := larkws.NewResponseByCode(http.StatusOK)
	if handlerErr != nil {
		common.Warn("feishu: WebSocket event handler failed", zap.String("account_id", s.accountID), zap.Error(handlerErr))
		response.StatusCode = http.StatusInternalServerError
	} else if result != nil {
		response.Data, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	headers.Add(larkws.HeaderBizRt, strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10))
	frame.Headers = headers
	frame.Payload, err = json.Marshal(response)
	if err != nil {
		return err
	}
	return s.writeFrame(frame)
}

func (s *feishuWebSocketSession) combineMessage(headers larkws.Headers, data []byte) ([]byte, error) {
	for id, parts := range s.parts {
		if time.Now().After(parts.expiresAt) {
			s.pendingBytes -= parts.size
			delete(s.parts, id)
		}
	}
	sum := headers.GetInt(larkws.HeaderSum)
	if sum <= 1 {
		return data, nil
	}
	seq := headers.GetInt(larkws.HeaderSeq)
	id := headers.GetString(larkws.HeaderMessageID)
	if sum > feishuMaxMessageParts || seq < 0 || seq >= sum || id == "" {
		return nil, fmt.Errorf("invalid Feishu WebSocket message fragments")
	}
	parts := s.parts[id]
	if parts == nil {
		if len(s.parts) >= feishuMaxPendingMessages {
			return nil, fmt.Errorf("too many pending Feishu WebSocket messages")
		}
		parts = &feishuMessageParts{chunks: make([][]byte, sum)}
		s.parts[id] = parts
	}
	if len(parts.chunks) != sum {
		return nil, fmt.Errorf("inconsistent Feishu WebSocket message fragments")
	}
	sizeChange := len(data) - len(parts.chunks[seq])
	if s.pendingBytes+sizeChange > feishuMaxMessageSize {
		return nil, fmt.Errorf("pending Feishu WebSocket messages exceed size limit")
	}
	parts.chunks[seq] = data
	parts.size += sizeChange
	s.pendingBytes += sizeChange
	parts.expiresAt = time.Now().Add(5 * time.Second)
	for _, chunk := range parts.chunks {
		if chunk == nil {
			return nil, nil
		}
	}
	delete(s.parts, id)
	s.pendingBytes -= parts.size
	return bytes.Join(parts.chunks, nil), nil
}
