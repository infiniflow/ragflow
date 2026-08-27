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

package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRunnerRedactsInternalRunError(t *testing.T) {
	cause := errors.New("dial mysql.internal:3306 password=secret")
	runErr := NewInternalRunError(fmt.Errorf("persist session: %w", cause))
	if !errors.Is(runErr, cause) {
		t.Fatal("NewInternalRunError must preserve the original cause")
	}

	runner := NewRunner()
	events := runner.Run(t.Context(), func(context.Context, map[string]any) (*CanvasState, error) {
		return nil, runErr
	}, "canvas", "session", nil, map[string]any{})

	ev, ok := <-events
	if !ok {
		t.Fatal("runner closed without an error event")
	}
	if ev.Type != "error" {
		t.Fatalf("event type = %q, want error", ev.Type)
	}
	var payload ErrorEvent
	if err := json.Unmarshal([]byte(ev.Data), &payload); err != nil {
		t.Fatalf("decode error event: %v", err)
	}
	if payload.Kind != RunErrorKindInternal {
		t.Errorf("error kind = %q, want %q", payload.Kind, RunErrorKindInternal)
	}
	if payload.Message != InternalRunErrorMessage {
		t.Errorf("error message = %q, want %q", payload.Message, InternalRunErrorMessage)
	}
	if strings.Contains(ev.Data, "mysql.internal") || strings.Contains(ev.Data, "password=secret") {
		t.Fatalf("error event leaked its internal cause: %s", ev.Data)
	}
	if _, open := <-events; open {
		t.Fatal("runner should close after the terminal error event")
	}
}

func TestRunErrorEventPreservesPublicMessage(t *testing.T) {
	payload := runErrorEvent(errors.New("invalid variable reference"))
	if payload.Message != "invalid variable reference" {
		t.Errorf("message = %q, want public runtime error", payload.Message)
	}
	if payload.Kind != "" {
		t.Errorf("kind = %q, want empty for public runtime error", payload.Kind)
	}
}
