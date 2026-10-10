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

package agentic_rag

import (
	"context"
	"testing"
)

// TestDisableNavToolScopedToConversation proves the core invariant of A: a
// dataset-level disable is recorded per conversation and only blocks the single
// tool that reported the absence, never spilling into other conversations or
// other tools.
func TestDisableNavToolScopedToConversation(t *testing.T) {
	ctxA := WithConversationKey(context.Background(), "convA")
	ctxB := WithConversationKey(context.Background(), "convB")
	ctxNone := context.Background()

	if navToolDisabled(ctxNone, navigateTreeToolName) {
		t.Fatal("empty conversation key must never report a disable")
	}

	disableNavTool(ctxA, navigateTreeToolName)

	if !navToolDisabled(ctxA, navigateTreeToolName) {
		t.Fatal("expected navigate_tree disabled for convA")
	}
	if navToolDisabled(ctxB, navigateTreeToolName) {
		t.Fatal("convB must not be affected by convA's disable")
	}
	if navToolDisabled(ctxA, navigateStructureToolName) {
		t.Fatal("only navigate_tree was disabled, not navigate_structure")
	}
	if navToolDisabled(ctxA, graphExploreToolName) {
		t.Fatal("only navigate_tree was disabled, not graph_explore")
	}

	set := navDisabledToolSet(ctxA)
	if !set[navigateTreeToolName] {
		t.Fatal("navDisabledToolSet must contain the disabled tool")
	}
	if set[navigateStructureToolName] {
		t.Fatal("navDisabledToolSet must not contain tools that were not disabled")
	}
}

// TestNavDisabledToolSetEmpty covers the no-op branch used by Run's tool-list
// filter when a conversation has nothing disabled yet.
func TestNavDisabledToolSetEmpty(t *testing.T) {
	if s := navDisabledToolSet(context.Background()); s != nil {
		t.Fatalf("expected nil set for an empty conversation, got %v", s)
	}
}
