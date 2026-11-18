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

package agentic_rag

import (
	"context"
	"strings"
	"testing"
)

// TestUnknownToolResultKeepsTheRunAlive pins the contract eino needs: a call to
// a tool this run never registered comes back as an ordinary tool result, not
// as an error that aborts the turn.
func TestUnknownToolResultKeepsTheRunAlive(t *testing.T) {
	out, err := unknownToolResult(context.Background(), webSearchToolName, `{"queries":["x"]}`)
	if err != nil {
		t.Fatalf("an unregistered tool must not fail the run: %v", err)
	}
	if !strings.Contains(out, webSearchToolName) {
		t.Errorf("the model needs the offending name to stop retrying it, got %q", out)
	}
	if !strings.Contains(out, "not registered") {
		t.Errorf("the result should say the capability is absent, got %q", out)
	}
}
