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

package service

import (
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"time"

	"go.uber.org/zap"
)

// HeartbeatSender is responsible for sending heartbeat reports to the admin server
type HeartbeatSender struct {
	client       *utility.HTTPClient
	logger       *zap.Logger
	serverType   common.ServerType
	serverName   string
	host         string
	port         int
	version      string
	lastSuccess  bool
	attemptCount int
}

// NewHeartbeatSender creates a new heartbeat service instance
func NewHeartbeatSender(logger *zap.Logger, serverType common.ServerType, serverName, host string, port int) *HeartbeatSender {
	return &HeartbeatSender{
		logger:       logger,
		serverType:   serverType,
		serverName:   serverName,
		host:         host,
		port:         port,
		version:      utility.GetRAGFlowVersion(),
		lastSuccess:  false,
		attemptCount: 0,
	}
}

// InitHTTPClient initializes the HTTP client with admin server configuration
func (h *HeartbeatSender) InitHTTPClient() error {
	adminConfig := server.GetAdminConfig()
	if adminConfig == nil {
		return fmt.Errorf("admin configuration not found")
	}

	h.client = utility.NewHTTPClientBuilder().
		WithHost(adminConfig.Host).
		WithPort(adminConfig.Port).
		WithTimeout(10 * time.Second).
		Build()

	h.logger.Info("Heartbeat HTTP client initialized",
		zap.String("admin_host", adminConfig.Host),
		zap.Int("admin_port", adminConfig.Port+2),
	)

	return nil
}

// SendHeartbeat sends a heartbeat message to the admin server
func (h *HeartbeatSender) SendHeartbeat() (error, string) {

	if h.attemptCount < 10 {
		if h.lastSuccess {
			h.attemptCount++
			return nil, ""
		}
	}
	h.attemptCount = 0
	h.lastSuccess = false

	if h.client == nil {
		if err := h.InitHTTPClient(); err != nil {
			h.logger.Error("Failed to initialize HTTP client", zap.Error(err))
			return err, "internal error, fail to initialize HTTP client"
		}
	}

	message := &common.BaseMessage{
		MessageID:   time.Now().UnixNano(),
		MessageType: common.MessageHeartbeat,
		ServerName:  h.serverName,
		ServerType:  h.serverType,
		Host:        h.host,
		Port:        h.port,
		Version:     h.version,
		Timestamp:   time.Now(),
		Ext:         nil,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		h.logger.Error("Failed to marshal heartbeat message", zap.Error(err))
		return err, "fail to parse the message"
	}

	resp, err := h.client.PostJSON("/api/v1/admin/reports", jsonData)
	if err != nil {
		return err, "can't connect with admin server"
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errMsg := fmt.Errorf("Heartbeat request failed with status code: %d", resp.StatusCode)
		h.logger.Warn(errMsg.Error())
		return errMsg, errMsg.Error()
	}

	h.logger.Debug("Heartbeat sent successfully",
		zap.String("server_id", h.serverName),
		zap.String("server_type", string(h.serverType)),
	)

	h.lastSuccess = true

	return nil, ""
}
