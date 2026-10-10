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
	"testing"
)

// TestNavToolAvailabilityScopedToConversation proves the core invariant: a
// dataset-level absence is recorded per conversation and only blocks the single
// tool that reported it, never spilling into other conversations or other tools.
func TestNavToolAvailabilityScopedToConversation(t *testing.T) {
	ctxA := WithConversationKey(context.Background(), "convA")
	ctxB := WithConversationKey(context.Background(), "convB")
	ctxNone := context.Background()

	if !navToolEnabled(ctxNone, navigateTreeToolName) {
		t.Fatal("empty conversation key must leave every tool enabled")
	}

	markNavToolUnavailable(ctxA, navigateTreeToolName)

	if navToolEnabled(ctxA, navigateTreeToolName) {
		t.Fatal("expected navigate_tree unavailable for convA")
	}
	if !navToolEnabled(ctxB, navigateTreeToolName) {
		t.Fatal("convB must not be affected by convA's mark")
	}
	if !navToolEnabled(ctxA, navigateStructureToolName) {
		t.Fatal("only navigate_tree was marked, not navigate_structure")
	}
	if !navToolEnabled(ctxA, graphExploreToolName) {
		t.Fatal("only navigate_tree was marked, not graph_explore")
	}

	set := navUnavailableToolSet(ctxA)
	if !set[navigateTreeToolName] {
		t.Fatal("navUnavailableToolSet must contain the marked tool")
	}
	if set[navigateStructureToolName] {
		t.Fatal("navUnavailableToolSet must not contain tools that are still available")
	}
}

// TestNavUnavailableToolSetEmpty covers the no-op branch used by Run's tool-list
// filter when a conversation has nothing marked unavailable yet.
func TestNavUnavailableToolSetEmpty(t *testing.T) {
	if s := navUnavailableToolSet(context.Background()); s != nil {
		t.Fatalf("expected nil set for an empty conversation, got %v", s)
	}
}
