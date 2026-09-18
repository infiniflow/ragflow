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

package common

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// withTestLogger swaps in an observer-backed logger and restores the old one.
// atomicLevel must be initialized alongside Logger: IsDebugEnabled reads it,
// and its zero value panics (in production InitLog always runs first, but a
// unit test starts from the zero package state).
func withTestLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	// Debug-level observer so the DebugCtx call in the assertions is captured
	// too; production level filtering is orthogonal to field attachment.
	core, logs := observer.New(zapcore.DebugLevel)
	old := Logger
	oldLevel := atomicLevel
	Logger = zap.New(core)
	atomicLevel = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	t.Cleanup(func() { Logger = old; atomicLevel = oldLevel })
	return logs
}

// TestCtxLogOmitsSessionFieldWhenAbsent pins the nil-slice contract of
// sessionFields: a context without a session id must add NO session_id field
// at all — not an empty string, and certainly not the string "nil" (a nil
// []zap.Field appended into variadic fields is just the zero-length prefix,
// never a materialized field).
func TestCtxLogOmitsSessionFieldWhenAbsent(t *testing.T) {
	logs := withTestLogger(t)

	InfoCtx(context.Background(), "no-session")
	DebugCtx(WithSessionID(context.Background(), "abc"), "with-session")
	WarnCtx(context.Background(), "bare-warn")
	ErrorCtx(context.Background(), "bare-error", context.DeadlineExceeded)
	InfoCtx(WithSessionID(context.Background(), "def"), "with-extra", zap.Int("n", 1))

	entries := logs.All()
	if len(entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(entries))
	}

	type tc struct {
		msg    string
		wantID string // "" means the field must be absent
	}
	cases := []tc{
		{"no-session", ""},
		{"with-session", "abc"},
		{"bare-warn", ""},
		{"bare-error", ""},
		{"with-extra", "def"},
	}
	for i, c := range cases {
		entry := entries[i]
		// ErrorCtx folds the error into the message, mirroring common.Error.
		want := c.msg
		if c.msg == "bare-error" {
			want = "bare-error, context deadline exceeded"
		}
		if entry.Message != want {
			t.Fatalf("entry %d message = %q, want %q", i, entry.Message, want)
		}
		got, present := entry.ContextMap()["session_id"]
		if c.wantID == "" {
			if present {
				t.Errorf("entry %d (%q): session_id = %v, want the field absent", i, c.msg, got)
			}
			continue
		}
		if !present || got != c.wantID {
			t.Errorf("entry %d (%q): session_id = (%v, %v), want %q", i, c.msg, got, present, c.wantID)
		}
	}
}

// TestSessionIDFromContext covers the accessor's edge cases directly.
func TestSessionIDFromContext(t *testing.T) {
	if got := SessionIDFromContext(nil); got != "" {
		t.Errorf("nil context: got %q, want empty", got)
	}
	if got := SessionIDFromContext(context.Background()); got != "" {
		t.Errorf("plain context: got %q, want empty", got)
	}
	// Blank and whitespace-only ids are a no-op, per WithSessionID's guard.
	for _, blank := range []string{"", " ", "\t"} {
		ctx := WithSessionID(context.Background(), blank)
		if got := SessionIDFromContext(ctx); got != "" {
			t.Errorf("WithSessionID(%q): got %q, want the ctx untouched", blank, got)
		}
	}
	if got := SessionIDFromContext(WithSessionID(context.Background(), " s1 ")); got != "s1" {
		t.Errorf("trimmed id: got %q, want %q", got, "s1")
	}
}
