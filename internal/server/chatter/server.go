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
	"net"
	"strconv"

	"ragflow/internal/channels"
	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

// Run owns the channel runtime until cancellation, including adapter shutdown.
func Run(ctx context.Context, args *bootstrap.ServerArgs) error {
	if !kvrocks.IsEnabled() {
		return fmt.Errorf("chatter requires Kvrocks for shared channel runtime status")
	}
	api := server.GetConfig().GetAPIServerConfig()
	host := api.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	runtime, err := channels.Start(ctx, "http://"+net.JoinHostPort(host, strconv.Itoa(api.HTTPPort)))
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
