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

package bootstrap

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// SetLogger (re)initializes the process logger for the given server name using
// the configured log settings plus any CLI overrides carried by args. It was
// previously a private helper in package main; it lives here so the per-mode
// runners (now in their own packages) can call it without reaching into main.
func SetLogger(serverName string, arguments *ServerArgs) {
	// log filename
	logFileName := fmt.Sprintf("%s.log", serverName)

	globalConfig := server.GetConfig()
	logConfig := globalConfig.GetLogConfig()

	// Reinitialize logger with the configured level and CLI overrides.
	logLevel := SelectedLogLevel(arguments, logConfig.Level)

	globalConfig.SetLogLevel(logLevel)

	fileOut := common.FileOutput{
		Filename:   logFileName,
		Path:       logConfig.Path,
		MaxSize:    logConfig.MaxSize,
		MaxBackups: logConfig.MaxBackups,
		MaxAge:     logConfig.MaxAge,
		Compress:   logConfig.Compress,
	}

	common.SyncLog()
	if err := common.InitLogger(logLevel, fileOut, serverName); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
	}
}

// StartHeartbeat initializes and starts the heartbeat reporter to the admin
// server. It is shared by the API, ingestor, and syncer server modes. The
// caller must defer the returned *utility.ScheduledTask's Stop() method.
func StartHeartbeat(serverType common.ServerType, serverID string, port int, heartBeatInterval time.Duration) *utility.ScheduledTask {
	localIP, err := utility.GetLocalIP()
	if err != nil {
		common.Fatal("fail to get local ip address")
	}

	service.AdminServiceClient = service.NewAdminClient(
		serverType,
		serverID,
		localIP,
		port,
	)
	if err = service.AdminServiceClient.InitHTTPClient(); err != nil {
		common.Warn("Failed to initialize heartbeat service", zap.Error(err))
		return nil
	}

	heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", heartBeatInterval, func() {
		if err = service.AdminServiceClient.SendHeartbeat(); err == nil {
			local.SetAdminStatus(0, "")
		} else {
			local.SetAdminStatus(1, err.Error())
		}
	})
	heartbeatReporter.Start()
	return heartbeatReporter
}
