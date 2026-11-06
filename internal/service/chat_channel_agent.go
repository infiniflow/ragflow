// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

type chatChannelAgentRunner interface {
	RunAgent(context.Context, string, string, string, any, []map[string]any) (<-chan canvas.RunEvent, error)
}

func normalizedChannelTarget(id *string) *string {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	return new(strings.TrimSpace(*id))
}

func validateChannelBinding(ctx context.Context, chatID, agentID *string) error {
	if chatID != nil && agentID != nil {
		return errors.New("chat_id and agent_id are mutually exclusive")
	}
	ref := permission.ResourceRef{Kind: permission.ResourceKindChat}
	operation := permission.OperationUse
	switch {
	case chatID != nil:
		ref.ID = *chatID
	case agentID != nil:
		ref.Kind, ref.ID, operation = permission.ResourceKindCanvas, *agentID, permission.OperationRun
	default:
		return nil
	}
	return permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, ref, operation)
}

func (s *ChatChannelService) handleChannelAgentMessage(ctx context.Context, channel *entity.ChatChannel, msg ChatChannelIncomingMessage) (string, error) {
	sessionID := channelSessionID(*channel.AgentID, channel.ID, msg.ChatID)
	// RunAgent creates and authorizes the persisted session; this boundary only
	// supplies its stable identity for the first message and subsequent turns.
	ctx = WithAgentSessionID(ctx, sessionID)
	ctx = withAgentChannelName(ctx, channel.Name)
	events, err := s.agentRunner.RunAgent(ctx, *channel.AgentID, sessionID, "", msg.Text, nil)
	if err != nil {
		return "", err
	}
	var answer strings.Builder
	var runErr error
	thinking := false
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-events:
			if !ok {
				if runErr != nil {
					return "", runErr
				}
				return answer.String(), nil
			}
			switch ev.Type {
			case "message":
				var data canvas.MessageEvent
				if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
					return "", err
				}
				if data.StartToThink {
					thinking = true
				}
				if data.EndToThink {
					thinking = false
					continue
				}
				if !thinking {
					answer.WriteString(data.Content)
				}
			case "waiting_for_user":
				var data canvas.WaitingForUserEvent
				if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
					return "", err
				}
				if len(data.Inputs) > 1 {
					answer.Reset()
					answer.WriteString("This workflow requires structured input. Please continue in RAGFlow.")
					continue
				}
				if answer.Len() > 0 && data.Tips != "" {
					answer.WriteString("\n")
				}
				answer.WriteString(data.Tips)
				if answer.Len() == 0 {
					answer.WriteString("Please reply to continue.")
				}
			case "error":
				// Provider/internal errors may contain credentials or implementation
				// details; adapters receive an error, never that raw event payload.
				runErr = errors.New("channel workflow execution failed")
			case "cancelled":
				runErr = context.Canceled
			}
		}
	}
}
