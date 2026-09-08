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

// memory_extractor_test.go — timezone-semantics tests for the memory
// extractor's timestamp handling (valid_at / invalid_at persistence).

package service

import (
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// pinMemoryNow fixes the memory wall clock at the given instant for the
// duration of a test, restoring it afterwards.
func pinMemoryNow(t *testing.T, instant time.Time) {
	t.Helper()
	orig := memoryNow
	memoryNow = func() time.Time { return instant }
	t.Cleanup(func() { memoryNow = orig })
}

// TestFormatMemoryTimeKeepsParsedWallClock: an offset-bearing or
// Z-suffixed ISO timestamp must keep its own wall clock — never be
// converted to UTC the way time.Time.UTC would.
func TestFormatMemoryTimeKeepsParsedWallClock(t *testing.T) {
	fallback := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "offset iso", value: "2026-08-20T10:05:00+08:00", want: "2026-08-20 10:05:00"},
		{name: "offset iso with fractional seconds", value: "2026-08-20T10:05:00.123+08:00", want: "2026-08-20 10:05:00"},
		{name: "zulu iso", value: "2026-08-20T02:05:00Z", want: "2026-08-20 02:05:00"},
		{name: "naive iso", value: "2026-08-20T10:05:00", want: "2026-08-20 10:05:00"},
		{name: "storage layout", value: "2026-08-20 10:05:00", want: "2026-08-20 10:05:00"},
		{name: "date only", value: "2026-08-20", want: "2026-08-20 00:00:00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := formatMemoryTime(test.value, fallback); got != test.want {
				t.Fatalf("formatMemoryTime(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestFormatMemoryTimeFallbackKeepsFallbackLocation: empty or unparseable
// input falls back to the supplied time formatted in its own location, so
// callers passing time.Now() persist server-local wall clock.
func TestFormatMemoryTimeFallbackKeepsFallbackLocation(t *testing.T) {
	fallback := time.Date(2026, 8, 20, 10, 5, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for _, value := range []string{"", "   ", "not a timestamp"} {
		if got := formatMemoryTime(value, fallback); got != "2026-08-20 10:05:00" {
			t.Fatalf("formatMemoryTime(%q) = %q, want fallback wall clock %q", value, got, "2026-08-20 10:05:00")
		}
	}
}

// TestBuildExtractedMessageValidAtSemantics: extracted messages persist the
// LLM-supplied wall clock as-is and fall back to the (server-local) now.
func TestBuildExtractedMessageValidAtSemantics(t *testing.T) {
	msg := MemoryMessage{UserID: "u1", AgentID: "a1", SessionID: "s1"}
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.Local)

	withLLMTime := buildExtractedMessage(8, 42, "mem-1", msg, extractedMemory{
		MessageType: "fact",
		Content:     "likes coffee",
		ValidAt:     "2026-08-20T10:05:00+08:00",
	}, now)
	if got := withLLMTime["valid_at"]; got != "2026-08-20 10:05:00" {
		t.Fatalf("valid_at = %v, want LLM wall clock %q", got, "2026-08-20 10:05:00")
	}

	fallbackOnly := buildExtractedMessage(9, 42, "mem-1", msg, extractedMemory{
		MessageType: "fact",
		Content:     "likes coffee",
	}, now)
	if got := fallbackOnly["valid_at"]; got != now.Format(memoryTimeLayout) {
		t.Fatalf("valid_at = %v, want fallback %q", got, now.Format(memoryTimeLayout))
	}
}

// TestBuildExtractedMessageUsesStableDocumentIDAcrossRetries ensures fallback
// timestamps do not become part of the chunk-store identity. A retry may run
// at a different time and allocate a new logical message id, but it must still
// overwrite the same extracted document.
func TestBuildExtractedMessageUsesStableDocumentIDAcrossRetries(t *testing.T) {
	msg := MemoryMessage{UserID: "u1", AgentID: "a1", SessionID: "s1"}
	firstAttempt := time.Date(2026, 8, 20, 10, 5, 0, 0, time.Local)
	retryAttempt := firstAttempt.Add(10 * time.Second)

	for _, test := range []struct {
		name string
		item extractedMemory
	}{
		{
			name: "omitted valid at",
			item: extractedMemory{MessageType: "fact", Content: "likes coffee"},
		},
		{
			name: "invalid timestamps",
			item: extractedMemory{
				MessageType: "fact",
				Content:     "likes coffee",
				ValidAt:     "not a timestamp",
				InvalidAt:   "also not a timestamp",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := buildExtractedMessage(8, 42, "mem-1", msg, test.item, firstAttempt)
			retry := buildExtractedMessage(9, 42, "mem-1", msg, test.item, retryAttempt)

			firstID, ok := first["id"].(string)
			if !ok || firstID == "" {
				t.Fatalf("first extracted message id = %#v, want a stable non-empty string", first["id"])
			}
			if got, ok := retry["id"].(string); !ok || got != firstID {
				t.Fatalf("retry extracted message id = %#v, want %q", retry["id"], firstID)
			}
		})
	}
}

// TestUpdateTaskProgressReturnsPersistenceError ensures callers can keep the
// broker message retryable when they cannot persist the terminal task state.
func TestUpdateTaskProgressReturnsPersistenceError(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.IngestionTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	svc := &MemoryMessageService{taskDAO: dao.NewTaskDAO()}
	if err := svc.updateTaskProgress(t.Context(), "mem-task-1", 1.0, "completed"); err == nil {
		t.Fatal("updateTaskProgress() error = nil, want persistence error")
	}
}

// TestTypeInstructionsStayInLockstepWithPython pins the rules that
// memory/utils/prompt_util.py carries and the Go map historically dropped:
// per-type Examples lines, the full "Timestamp Rules for <Type> Knowledge:"
// titles, and the semantic Default line. If a substring fails here, the two
// implementations are handing the extracting LLM different contracts (the
// drift this map was merged with — see #18415).
func TestTypeInstructionsStayInLockstepWithPython(t *testing.T) {
	want := map[string][]string{
		"semantic": {
			"- Examples: \"The capital of France is Paris\", \"Water boils at 100°C\"",
			"**Timestamp Rules for Semantic Knowledge:**",
			"- valid_at: When the fact became true (e.g., law enactment, discovery)",
			"- invalid_at: When it becomes false (e.g., repeal, disproven) or empty if still true",
			"- Default: valid_at = conversation time, invalid_at = \"\" for timeless facts",
		},
		"episodic": {
			"- Examples: \"Yesterday I fixed the bug\", \"User reported issue last week\"",
			"**Timestamp Rules for Episodic Knowledge:**",
			"- Extract explicit times: \"at 3 PM\", \"last Monday\", \"from X to Y\"",
		},
		"procedural": {
			"- Examples: \"To reset password, click...\", \"Debugging steps: 1)...\"",
			"**Timestamp Rules for Procedural Knowledge:**",
			"- For version-specific: use release dates",
			"- For best practices: invalid_at = \"\"",
		},
	}
	for memoryType, lines := range want {
		got, ok := TYPE_INSTRUCTIONS[memoryType]
		if !ok {
			t.Fatalf("TYPE_INSTRUCTIONS has no entry for %q", memoryType)
		}
		for _, line := range lines {
			if !strings.Contains(got, line) {
				t.Errorf("TYPE_INSTRUCTIONS[%q] is missing the Python-side rule: %s", memoryType, line)
			}
		}
	}
}
