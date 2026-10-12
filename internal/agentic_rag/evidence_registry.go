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
	"sync"
)

// EvidenceRegistry assigns each passage the model is SHOWN a stable [ID:n]
// handle, in first-seen order.
//
// The model can only cite what it was shown, so the number a citation names has
// to be decided at the moment the passage reaches it, not later in a renderer
// the session never sees. A tool result therefore stamps its passages with the
// handle the model is expected to write back ([ID:n], from the passage's
// ref attribute), and this registry is what those handles resolve against.
//
// A passage that reaches the model twice keeps its first number, so the same
// chunk cited on two lines is one place, cited the same way.
type EvidenceRegistry struct {
	mu    sync.Mutex
	order []string
	numOf map[string]int
}

// NewEvidenceRegistry returns an empty registry.
func NewEvidenceRegistry() *EvidenceRegistry {
	return &EvidenceRegistry{numOf: map[string]int{}}
}

// Stamp returns the stable handle for a chunk id, allocating the next one the
// first time the passage is seen. An empty id (or a nil registry) yields -1,
// which callers read as "not citable".
func (r *EvidenceRegistry) Stamp(id string) int {
	if r == nil {
		return -1
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.numOf[id]; ok {
		return n
	}
	n := len(r.order)
	r.order = append(r.order, id)
	r.numOf[id] = n
	return n
}

// Resolve returns the chunk id a handle names, and whether it is published.
func (r *EvidenceRegistry) Resolve(n int) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if n < 0 || n >= len(r.order) {
		return "", false
	}
	return r.order[n], true
}

// IDs returns a snapshot of the registry in first-seen order: IDs[n] is the
// chunk the model was shown as [ID:n].
func (r *EvidenceRegistry) IDs() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

// evidenceRegistryKey carries the run's registry through the request context,
// the same per-run channel web_search uses: the tools read it when they render,
// so concurrent turns never share numbering.
type evidenceRegistryKey struct{}

// WithEvidenceRegistry returns a copy of ctx carrying the run's evidence
// registry. A nil registry is a no-op.
func WithEvidenceRegistry(ctx context.Context, r *EvidenceRegistry) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, evidenceRegistryKey{}, r)
}

// evidenceRegistryFrom returns the registry carried by ctx, or nil when the
// caller published none (rendering then stamps no ref attribute).
func evidenceRegistryFrom(ctx context.Context) *EvidenceRegistry {
	r, _ := ctx.Value(evidenceRegistryKey{}).(*EvidenceRegistry)
	return r
}
