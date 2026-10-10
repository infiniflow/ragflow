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
	"sync"
	"time"
)

// conversationKey carries the per-conversation identifier through the request
// context so the compiled-knowledge tools (navigate_tree / navigate_structure /
// graph_explore) can record, and later honor, a per-conversation availability
// decision without changing their constructors or the tool registry. The caller
// (the chat pipeline) sets it from the conversation id; when it is absent Run
// assigns a unique per-run key so availability still holds within a single ReAct
// turn.
type conversationKeyT struct{}

// WithConversationKey returns a copy of ctx carrying the conversation id. An empty
// key is a no-op (a stable identity is what makes the availability tracking
// meaningful).
func WithConversationKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, conversationKeyT{}, key)
}

// conversationKeyFrom returns the conversation id carried by ctx, or "".
func conversationKeyFrom(ctx context.Context) string {
	if k, ok := ctx.Value(conversationKeyT{}).(string); ok {
		return k
	}
	return ""
}

// navAvailabilityStore records, per conversation, which compiled-knowledge tools
// have been PROVEN unavailable for the bound datasets, so the agent stops
// spending calls on them for the rest of the conversation. Every tool starts out
// enabled; a tool leaves that set only on evidence.
//
// A tool is marked unavailable only on a DATASET-LEVEL absence - navigate_tree
// reporting "no compiled navigation tree", navigate_structure "no structure",
// graph_explore "no compiled knowledge graph" - which is a fact that does not
// change within a conversation. A query-level miss (the structure exists, but
// THIS query reached nothing) is NOT a trigger: a better-phrased query may still
// hit.
//
// The store is process-local and best-effort: a process restart or a TTL expiry
// simply makes the tool re-detect on its next call (one cheap backend read), not
// a correctness regression.
type navAvailabilityStore struct {
	mu      sync.Mutex
	unavail map[string]map[string]time.Time // convKey -> tool -> unavailable-until
	ttl     time.Duration
}

var navAvailability = &navAvailabilityStore{
	unavail: map[string]map[string]time.Time{},
	ttl:     30 * time.Minute,
}

// markNavToolUnavailable records tool as unavailable for the conversation carried
// by ctx. A missing/empty conversation key or tool is a no-op.
func markNavToolUnavailable(ctx context.Context, tool string) {
	key := conversationKeyFrom(ctx)
	if key == "" || tool == "" {
		return
	}
	navAvailability.mu.Lock()
	defer navAvailability.mu.Unlock()
	if navAvailability.unavail[key] == nil {
		navAvailability.unavail[key] = map[string]time.Time{}
	}
	navAvailability.unavail[key][tool] = time.Now().Add(navAvailability.ttl)
}

// navToolEnabled reports whether tool is still usable for the conversation
// carried by ctx: true unless it was marked unavailable and that mark has not yet
// expired. A missing/empty conversation key leaves every tool enabled.
func navToolEnabled(ctx context.Context, tool string) bool {
	key := conversationKeyFrom(ctx)
	if key == "" || tool == "" {
		return true
	}
	navAvailability.mu.Lock()
	defer navAvailability.mu.Unlock()
	until, ok := navAvailability.unavail[key][tool]
	if !ok {
		return true
	}
	if time.Now().After(until) {
		delete(navAvailability.unavail[key], tool)
		return true
	}
	return false
}

// navUnavailableToolSet returns the set of compiled-knowledge tools proven
// unavailable for the conversation carried by ctx (honoring the TTL). It is used
// to drop those tools from the agent's tool list so the model cannot keep calling
// them.
func navUnavailableToolSet(ctx context.Context) map[string]bool {
	key := conversationKeyFrom(ctx)
	if key == "" {
		return nil
	}
	navAvailability.mu.Lock()
	defer navAvailability.mu.Unlock()
	raw, ok := navAvailability.unavail[key]
	if !ok {
		return nil
	}
	now := time.Now()
	out := make(map[string]bool, len(raw))
	for tool, until := range raw {
		if now.After(until) {
			delete(navAvailability.unavail[key], tool)
			continue
		}
		out[tool] = true
	}
	if len(out) == 0 {
		delete(navAvailability.unavail, key)
	}
	return out
}
