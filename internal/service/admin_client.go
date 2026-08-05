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
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"time"

	"go.uber.org/zap"
)

var licenseStatusCode common.ErrorCode
var AdminServiceClient *AdminClient

// AdminClient is responsible for sending heartbeat reports to the admin server
type AdminClient struct {
	client       *utility.HTTPClient
	serverType   common.ServerType
	serverName   string
	host         string
	port         int
	version      string
	lastSuccess  bool
	attemptCount int
	clusterInfo  *utility.ClusterInfo
}

// NewAdminClient creates a new heartbeat service instance
func NewAdminClient(serverType common.ServerType, serverName, host string, port int) *AdminClient {
	licenseStatusCode = common.CodeSuccess
	return &AdminClient{
		serverType:   serverType,
		serverName:   serverName,
		host:         host,
		port:         port,
		version:      common.GetRAGFlowVersion(),
		lastSuccess:  false,
		attemptCount: 0,
	}
}

// InitHTTPClient initializes the HTTP client with admin server configuration
func (h *AdminClient) InitHTTPClient() error {
	config := server.GetConfig()
	adminConfig := config.GetAdminServerConfig()
	if adminConfig.Host == "" || adminConfig.HTTPPort == 0 {
		return fmt.Errorf("admin configuration not found")
	}

	h.client = utility.NewHTTPClientBuilder().
		WithHost(adminConfig.Host).
		WithPort(adminConfig.HTTPPort).
		WithTimeout(10 * time.Second).
		Build()

	common.Info("Heartbeat HTTP client initialized",
		zap.String("admin_host", adminConfig.Host),
		zap.Int("admin_port", adminConfig.HTTPPort),
	)

	err := h.InitHTTPClientEE()
	if err != nil {
		common.Fatal(fmt.Sprintf("Fail to init enterprise service: %v", err))
	}

	return nil
}

// SendHeartbeat sends a heartbeat message to the admin server
func (h *AdminClient) SendHeartbeat() error {

	if h.attemptCount < 10 {
		if h.lastSuccess {
			h.attemptCount++
			return nil
		}
	}
	h.attemptCount = 0
	h.lastSuccess = false

	if h.client == nil {
		if err := h.InitHTTPClient(); err != nil {
			common.Error("Failed to initialize HTTP client", err)
			return err
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

	message.Ext = h.clusterInfo

	jsonData, err := json.Marshal(message)
	if err != nil {
		common.Error("Failed to marshal heartbeat message", err)
		return err
	}

	resp, err := h.client.PostJSON("/api/v1/admin/reports", jsonData)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// extract the Code and Message field of the response
	var responseBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		return err
	}
	code, ok := responseBody["code"].(float64)
	if !ok {
		return fmt.Errorf("unexpected heartbeat response (status %d): missing or non-numeric \"code\" field", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error HTTP status code: %d", resp.StatusCode)
	}

	responseCode := common.ErrorCode(code)
	if responseCode != common.CodeLicenseValid {
		if responseCode != licenseStatusCode {
			licenseStatusCode = responseCode
			common.Warn(fmt.Sprintf("Heartbeat response error: %s, code: %d", responseCode.Message(), responseCode))
		}

		return errors.New(responseCode.Message())
	}
	licenseStatusCode = responseCode

	common.Debug("Heartbeat sent successfully",
		zap.String("server_id", h.serverName),
		zap.String("server_type", string(h.serverType)),
	)

	h.lastSuccess = true

	return nil
}
