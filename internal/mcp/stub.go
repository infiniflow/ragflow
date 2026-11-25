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

//go:build !ee

package mcp

import "net/http"

// CodexTickets is the mode 8 MCP authorization store. The community edition has
// no mode 8, so the type carries no state and every operation denies.
type CodexTickets struct{}

// DefaultCodexTickets returns the community edition's always-closed store.
func DefaultCodexTickets() *CodexTickets { return &CodexTickets{} }

// NewCodexHandler returns the mode 8 MCP bridge. The community edition has no
// Codex engine to call it, so the route exists but every request is a 404.
func NewCodexHandler(tickets *CodexTickets) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}
