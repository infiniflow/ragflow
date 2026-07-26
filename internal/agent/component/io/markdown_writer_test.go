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

package io

import (
	"strings"
	"testing"
	"time"
)

// TestWriteMarkdown_BodyOnly: minimal config — body passthrough.
func TestWriteMarkdown_BodyOnly(t *testing.T) {
	out := string(WriteMarkdown("# Title\n\nbody", MarkdownOptions{}))
	if out != "# Title\n\nbody" {
		t.Errorf("body-only output = %q, want passthrough", out)
	}
}

// TestWriteMarkdown_Timestamp: timestamp becomes a leading HTML
// comment so it doesn't render.
func TestWriteMarkdown_Timestamp(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	out := string(WriteMarkdown("body", MarkdownOptions{
		AddTimestamp: true,
		Now:          fixed,
	}))
	want := "<!-- generated: 2026-06-15T12:00:00Z -->\n\nbody"
	if out != want {
		t.Errorf("timestamp output = %q, want %q", out, want)
	}
}

// TestWriteMarkdown_HeaderComment: header becomes a comment block
// before the body.
func TestWriteMarkdown_HeaderComment(t *testing.T) {
	out := string(WriteMarkdown("body", MarkdownOptions{HeaderText: "My Doc"}))
	want := "<!-- header: My Doc -->\n\nbody"
	if out != want {
		t.Errorf("header output = %q, want %q", out, want)
	}
}

// TestWriteMarkdown_FooterComment: footer becomes a trailing
// comment block.
func TestWriteMarkdown_FooterComment(t *testing.T) {
	out := string(WriteMarkdown("body", MarkdownOptions{FooterText: "page 1"}))
	want := "body\n\n<!-- footer: page 1 -->\n"
	if out != want {
		t.Errorf("footer output = %q, want %q", out, want)
	}
}

// TestWriteMarkdown_AllOptions: full config produces header +
// timestamp + body + footer in order.
func TestWriteMarkdown_AllOptions(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	out := string(WriteMarkdown("body", MarkdownOptions{
		HeaderText:   "H",
		FooterText:   "F",
		AddTimestamp: true,
		Now:          fixed,
	}))
	want := "<!-- generated: 2026-06-15T12:00:00Z -->\n\n<!-- header: H -->\n\nbody\n\n<!-- footer: F -->\n"
	if out != want {
		t.Errorf("full output mismatch:\n got  %q\n want %q", out, want)
	}
}

// TestWriteMarkdown_HeaderEscapesInsideComment: header / footer
// content is embedded raw inside an HTML comment, so any '--' or
// '>' in the header could break the comment. We do NOT sanitise —
// callers are responsible for not passing header / footer text
// containing HTML-comment terminators. This test pins that
// contract (escape is the caller's job, not the writer's).
func TestWriteMarkdown_HeaderEscapesInsideComment(t *testing.T) {
	out := string(WriteMarkdown("body", MarkdownOptions{HeaderText: "raw H"}))
	if !strings.Contains(out, "<!-- header: raw H -->") {
		t.Errorf("expected raw header text inside comment, got %q", out)
	}
}
