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

	"ragflow/internal/common"
	"ragflow/internal/server"
)

// SetLogger opens the configured log destinations and keeps startup INFO logs visible.
func SetLogger(serverName string, logLevel string) {
	// log filename
	logFileName := fmt.Sprintf("%s.log", serverName)

	globalConfig := server.GetConfig()
	logConfig := globalConfig.GetLogConfig()

	if common.LogLevelHigherThanInfo(logLevel) {
		logLevel = "info"
	}

	fileOut := common.FileOutput{
		Filename:   logFileName,
		Path:       logConfig.Path,
		MaxSize:    logConfig.MaxSize,
		MaxBackups: logConfig.MaxBackups,
		MaxAge:     logConfig.MaxAge,
		Compress:   logConfig.Compress,
	}

	common.SyncLog()

	common.InitLogger(logLevel, fileOut, serverName)
}

// SetIngestLog (re)initializes the ingestion lifecycle event sink for the given
// server name from the configured ingest_log settings. It mirrors SetLogger so
// every server mode that can emit (the API server creating/enqueuing tasks, the
// ingestor running them, etc.) shares one sink. Emission is best-effort: a sink
// init failure is logged but never fatal. The sink always writes to stdout; an
// ingestor.log.path only adds an optional rotated file on top of it.
func SetIngestLog(serverName string) {
	globalConfig := server.GetConfig()
	ingestLogConfig := globalConfig.GetIngestLogConfig()
	if err := common.InitIngestLog(ingestLogConfig.Type, ingestLogConfig.GetFileOutput(), serverName); err != nil {
		common.Error("Failed to initialize ingest log sink", err)
	}
}
