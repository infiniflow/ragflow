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

package pipeline

import (
	"context"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/engine"
	"ragflow/internal/ingestion/testutil"
)

// TestResolveStoreInjectedWins verifies that an explicitly injected store is
// returned unchanged and the NATS resolver is not consulted.
func TestResolveStoreInjectedWins(t *testing.T) {
	engine.SetNatsEngine(testutil.SetupNatsEngine(t))
	t.Cleanup(func() { engine.SetNatsEngine(nil) })

	injected := &canvas.RedisCheckPointStore{}
	p := &Pipeline{store: injected}

	got, err := p.resolveStore(context.Background())
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if got != canvas.CheckPointStore(injected) {
		t.Fatalf("resolveStore: injected store not returned (got %T)", got)
	}
}

// TestResolveStoreNatsPresent verifies the hard cutover: with a NATS engine
// installed, resolveStore returns a NATS-backed CheckPointStore (never Redis).
func TestResolveStoreNatsPresent(t *testing.T) {
	ne := testutil.SetupNatsEngine(t)
	engine.SetNatsEngine(ne)
	t.Cleanup(func() { engine.SetNatsEngine(nil) })

	p := &Pipeline{}
	got, err := p.resolveStore(context.Background())
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if got == nil {
		t.Fatal("resolveStore: expected a non-nil NATS store")
	}
	if _, ok := got.(*canvas.NatsCheckPointStore); !ok {
		t.Fatalf("resolveStore: expected *canvas.NatsCheckPointStore, got %T", got)
	}
}

// TestResolveStoreNatsAbsent verifies graceful degradation: with no NATS engine
// (and no injected store), resolveStore returns (nil, nil). A resume-required
// run is then rejected by the requireResume guard in Run — this test only
// asserts the store resolution itself does not error.
func TestResolveStoreNatsAbsent(t *testing.T) {
	engine.SetNatsEngine(nil)
	p := &Pipeline{}

	got, err := p.resolveStore(context.Background())
	if err != nil {
		t.Fatalf("resolveStore: expected nil error without NATS, got %v", err)
	}
	if got != nil {
		t.Fatalf("resolveStore: expected nil store without NATS, got %T", got)
	}
}
