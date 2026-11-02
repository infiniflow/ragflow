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

package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type variableStoreStub struct {
	value    string
	getErr   error
	setNX    bool
	setNXErr error
}

func (s *variableStoreStub) Get(context.Context, string) (string, error) {
	return s.value, s.getErr
}

func (s *variableStoreStub) Set(context.Context, string, string, time.Duration) bool {
	return true
}

func (s *variableStoreStub) SetNX(context.Context, string, string, time.Duration) (bool, error) {
	return s.setNX, s.setNXErr
}

func TestGetOrCreateKeyReturnsSetNXError(t *testing.T) {
	transportErr := errors.New("kvrocks unavailable")
	store := &variableStoreStub{setNXErr: transportErr}

	value, err := GetOrCreateKey(t.Context(), store, "ragflow:key", "generated")
	if err == nil {
		t.Fatal("GetOrCreateKey error = nil, want SetNX error")
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("GetOrCreateKey error = %v, want wrapped transport error", err)
	}
	if value != "" {
		t.Fatalf("GetOrCreateKey value = %q, want empty string", value)
	}
	if !strings.Contains(err.Error(), "ragflow:key") {
		t.Fatalf("GetOrCreateKey error = %q, want key context", err)
	}
}

func TestGetOrCreateKeyHandlesSetNXContention(t *testing.T) {
	// The first read misses. After SetNX reports contention, the second read
	// observes the value created by the competing process.
	storeWithSequence := &sequenceVariableStore{values: []string{"", "competing"}}
	value, err := GetOrCreateKey(t.Context(), storeWithSequence, "ragflow:key", "generated")
	if err != nil {
		t.Fatalf("GetOrCreateKey returned error: %v", err)
	}
	if value != "competing" {
		t.Fatalf("GetOrCreateKey value = %q, want %q", value, "competing")
	}
}

type sequenceVariableStore struct {
	values []string
}

func (s *sequenceVariableStore) Get(context.Context, string) (string, error) {
	value := s.values[0]
	s.values = s.values[1:]
	return value, nil
}

func (s *sequenceVariableStore) Set(context.Context, string, string, time.Duration) bool {
	return true
}

func (s *sequenceVariableStore) SetNX(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}
