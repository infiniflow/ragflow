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

package component

import (
	"context"
	"errors"
	"testing"
)

// TestStubMemorySaver_DefaultReturnsError: the default stub
// returns ErrMemoryServiceMissing so callers can detect the
// deferred state.
func TestStubMemorySaver_DefaultReturnsError(t *testing.T) {
	SetMemorySaver(nil)
	saver := GetMemorySaver()
	err := saver.Save(context.Background(), MemorySaveRequest{
		MemoryIDs: []string{"m1"},
		AgentID:   "a1",
	})
	if !errors.Is(err, ErrMemoryServiceMissing) {
		t.Errorf("got %v, want ErrMemoryServiceMissing", err)
	}
}

// TestSetMemorySaver_Roundtrip: a custom saver set via
// SetMemorySaver is returned by GetMemorySaver and gets called.
func TestSetMemorySaver_Roundtrip(t *testing.T) {
	var called bool
	custom := &fakeSaver{called: &called}
	SetMemorySaver(custom)
	defer SetMemorySaver(nil)
	got := GetMemorySaver()
	if got != custom {
		t.Fatalf("saver not registered")
	}
	if err := got.Save(context.Background(), MemorySaveRequest{
		MemoryIDs:     []string{"m1"},
		AgentResponse: "hi",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !called {
		t.Errorf("custom Save not called")
	}
}

type fakeSaver struct {
	called *bool
}

func (f *fakeSaver) Save(_ context.Context, req MemorySaveRequest) error {
	if f.called != nil {
		*f.called = true
	}
	if req.AgentResponse == "" {
		return errors.New("missing agent response")
	}
	return nil
}
