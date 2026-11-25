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

// Package syncer wires and runs the RAGFlow file syncer. It was extracted from
// package main so that main stays a thin dispatch layer.
package syncer

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/syncer"
)

// Run starts the RAGFlow file syncer. It blocks until ctx is cancelled (OS
// shutdown signal) or the syncer receives a shutdown command from the admin,
// then gracefully stops the syncer.
func Run(ctx context.Context, serverType common.ServerType, cancel context.CancelFunc, arguments *bootstrap.ServerArgs) error {
	globalConfig := server.GetConfig()
	syncerConfig := globalConfig.GetSyncerConfig()
	fileSyncer := syncer.NewSyncer(syncerConfig.MaxConcurrentSyncs)

	if err := fileSyncer.StartContext(ctx); err != nil {
		common.Error("Failed to initialize file syncer", err)
		return err
	}

	if serverType == common.ServerTypeFileSyncer {
		common.Info("\n     _______ __        _____\n" +
			"    / ____(_) /__     / ___/__  ______  ________  _____\n" +
			"   / /_  / / / _ \\    \\__ \\/ / / / __ \\/ ___/ _ \\/ ___/\n" +
			"  / __/ / / /  __/   ___/ / /_/ / / / / /__/  __/ /\n" +
			" /_/   /_/_/\\___/   /____/\\__, /_/ /_/\\___/\\___/_/\n" +
			"                           /____/    \n")
	}

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow file syncer service version: %s", common.GetRAGFlowVersion()))

	// Start heartbeat reporter to admin server
	if hb := bootstrap.StartHeartbeat(
		common.ServerTypeFileSyncer,
		fmt.Sprintf("syncer-%s", fileSyncer.ID()[:8]),
		0,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Wait()
		defer hb.Stop()
	}

	var readyErr error
	if arguments.Ready != nil {
		readyErr = arguments.Ready()
		if readyErr != nil {
			cancel()
		}
	}

	// Wait for either an OS shutdown signal or a shutdown command from the admin
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
		common.Info(fmt.Sprintf("Shutting down RAGFlow file syncer %s ...", *arguments.Name))
	case <-fileSyncer.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping file syncer %s ...", *arguments.Name))
		cancel()
	}

	fileSyncer.Stop()
	common.Info(fmt.Sprintf("File syncer %s shutdown complete", *arguments.Name))

	return readyErr
}
