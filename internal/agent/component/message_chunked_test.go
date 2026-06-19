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
	"strings"
	"testing"
)

// TestSplitSentences_Empty: empty input returns a single empty
// element (never nil, callers can always range).
func TestSplitSentences_Empty(t *testing.T) {
	out := splitSentences("")
	if len(out) != 1 {
		t.Fatalf("expected 1 element, got %d", len(out))
	}
	if out[0] != "" {
		t.Errorf("expected empty string, got %q", out[0])
	}
}

// TestSplitSentences_NoBoundary: text without sentence boundaries
// returns the whole text as a single element.
func TestSplitSentences_NoBoundary(t *testing.T) {
	out := splitSentences("just a fragment")
	if len(out) != 1 {
		t.Fatalf("expected 1 element, got %d", len(out))
	}
	if out[0] != "just a fragment" {
		t.Errorf("got %q, want full text", out[0])
	}
}

// TestSplitSentences_DotBoundary: a period+space split.
func TestSplitSentences_DotBoundary(t *testing.T) {
	out := splitSentences("first. second. third.")
	if len(out) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %+v", len(out), out)
	}
	if out[0] != "first." || out[1] != "second." || out[2] != "third." {
		t.Errorf("unexpected split: %+v", out)
	}
}

// TestSplitSentences_MixedPunctuation: "!" and "?" are also
// sentence boundaries.
func TestSplitSentences_MixedPunctuation(t *testing.T) {
	out := splitSentences("wow! really? ok.")
	if len(out) != 3 {
		t.Fatalf("expected 3, got %d: %+v", len(out), out)
	}
	if !strings.HasSuffix(out[0], "!") || !strings.HasSuffix(out[1], "?") || !strings.HasSuffix(out[2], ".") {
		t.Errorf("wrong trailing punctuation: %+v", out)
	}
}

// TestMessage_Stream_MultiSentence: a resolved content with
// multiple sentences emits N content chunks + 1 done chunk.
// The end-to-end message_test.go already exercises the single-
// sentence happy path; this test pins the multi-sentence path
// through splitSentences directly (Stream integration is via the
// existing test).
func TestMessage_Stream_MultiSentence(t *testing.T) {
	// Direct unit test of splitSentences; the Stream() method uses
	// this internally. The full Stream() integration is covered by
	// TestMessage_Stream in message_test.go.
	sentences := splitSentences("First sentence. Second sentence. Third.")
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %+v", len(sentences), sentences)
	}
}
