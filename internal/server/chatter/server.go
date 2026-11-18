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

	"ragflow/internal/channels"
	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/server/local"
)

// Run owns the channel runtime until cancellation, including adapter shutdown.
func Run(ctx context.Context, _ context.CancelFunc, serverName string, args *bootstrap.ServerArgs) error {
	if !kvrocks.IsEnabled() {
		return fmt.Errorf("chatter requires Kvrocks for shared channel runtime status")
	}
	bootstrap.SetLogger(serverName, args)
	closeTokenizer, err := bootstrap.InitializeAgentTokenizer()
	if err != nil {
		return err
	}
	defer closeTokenizer()
	bootstrap.SetIngestLog(serverName)
	agentServices := bootstrap.NewAgentServices()
	runtime, err := channels.Start(ctx, agentServices.AgentService)
	if err != nil {
		return fmt.Errorf("start chat-channel: %w", err)
	}
	defer runtime.Stop()
	local.InitAdminStatus()
	if hb := bootstrap.StartHeartbeat(common.ServerTypeChatter, fmt.Sprintf("chatter-%s", common.GenerateUUID()[:8]), 0, server.GetConfig().GetHeartbeatInterval()); hb != nil {
		defer hb.Stop()
	}
	common.Info("Chat-channel process started")
	<-ctx.Done()
	return nil
}
