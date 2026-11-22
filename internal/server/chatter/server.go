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

// Package chatter runs chat-channel adapters independently of the API listeners.
package chatter

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/channels"
	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/service"
)

// Run owns the channel runtime until cancellation, including adapter shutdown.
func Run(ctx context.Context, args *bootstrap.ServerArgs, services *bootstrap.AgentServices) error {
	if !kvrocks.IsEnabled() {
		return fmt.Errorf("chatter requires Kvrocks for shared channel runtime status")
	}
	var closeTokenizer func()
	var runtimeMu sync.Mutex
	stopped := false
	defer func() {
		runtimeMu.Lock()
		defer runtimeMu.Unlock()
		stopped = true
		if closeTokenizer != nil {
			closeTokenizer()
		}
	}()
	getAgentService := sync.OnceValues(func() (*service.AgentService, error) {
		if services != nil {
			return services.AgentService, nil
		}
		// Adapter shutdown can time out while a first message is initializing.
		runtimeMu.Lock()
		defer runtimeMu.Unlock()
		if stopped {
			return nil, context.Canceled
		}
		// A standalone chatter needs the dictionaries only when a message arrives.
		common.Info("Initializing chat-channel agent runtime")
		var err error
		closeTokenizer, err = bootstrap.InitializeAgentTokenizer()
		if err != nil {
			return nil, err
		}
		return bootstrap.NewAgentServices().AgentService, nil
	})
	runtime, err := channels.Start(ctx, getAgentService)
	if err != nil {
		return fmt.Errorf("start chat-channel: %w", err)
	}
	defer runtime.Stop()

	if common.GetRAGFlowType() == common.EnterpriseEdition {
		common.Info("\n  ____ _           _   _\n" +
			" / ___| |__   __ _| |_| |_ ___ _ __\n" +
			"| |   | '_ \\ / _` | __| __/ _ \\ '__|\n" +
			"| |___| | | | (_| | |_| ||  __/ |\n" +
			" \\____|_| |_|\\__,_|\\__|\\__\\___|_|\n")
	}

	if hb := bootstrap.StartHeartbeat(common.ServerTypeChatter, fmt.Sprintf("chatter-%s", common.GenerateUUID()[:8]), 0, server.GetConfig().GetHeartbeatInterval()); hb != nil {
		defer hb.Wait()
		defer hb.Stop()
	}
	common.Info("Chat-channel service started")
	if args.Ready != nil {
		if err := args.Ready(); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return nil
}
