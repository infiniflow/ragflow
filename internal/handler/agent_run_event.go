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

package handler

import (
	"encoding/json"
	"strings"

	"ragflow/internal/agent/canvas"
)

func agentRunEventMessage(ev canvas.RunEvent, fallback string) string {
	var payload canvas.ErrorEvent
	if err := json.Unmarshal([]byte(ev.Data), &payload); err == nil {
		if payload.Kind == canvas.RunErrorKindInternal {
			return canvas.InternalRunErrorMessage
		}
		if payload.Message != "" {
			return payload.Message
		}
	}
	if message := strings.TrimSpace(ev.Data); message != "" {
		return message
	}
	return fallback
}

func agentWaitingForUser(ev canvas.RunEvent) canvas.WaitingForUserEvent {
	var waiting canvas.WaitingForUserEvent
	if err := json.Unmarshal([]byte(ev.Data), &waiting); err != nil {
		return canvas.WaitingForUserEvent{}
	}
	return waiting
}

func agentWaitingForUserMessage(ev canvas.RunEvent) string {
	waiting := agentWaitingForUser(ev)
	message := "Agent is waiting for user input."
	if waiting.CpnID != "" {
		message += " cpn_id: " + waiting.CpnID
	}
	if waiting.Tips != "" {
		message += " " + waiting.Tips
	}
	return message
}
