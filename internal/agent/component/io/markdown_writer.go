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

// Package io — Markdown writer.
//
// Round-trips markdown content: header / footer become HTML
// comments (so they don't affect rendering), and an optional
// front-matter-style timestamp comment goes at the top.

package io

import (
	"bytes"
	"time"
)

// MarkdownOptions is the public contract for the Markdown writer.
type MarkdownOptions struct {
	HeaderText   string
	FooterText   string
	AddTimestamp bool
	// Now overrides time.Now() for deterministic tests.
	Now time.Time
}

// WriteMarkdown renders content as Markdown with header/footer as
// HTML comments. Round-tripping Markdown → Markdown is a no-op
// apart from the comment-based metadata.
func WriteMarkdown(content string, opts MarkdownOptions) []byte {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var b bytes.Buffer
	if opts.AddTimestamp {
		b.WriteString("<!-- generated: ")
		b.WriteString(now.Format(time.RFC3339))
		b.WriteString(" -->\n\n")
	}
	if opts.HeaderText != "" {
		b.WriteString("<!-- header: ")
		b.WriteString(opts.HeaderText)
		b.WriteString(" -->\n\n")
	}
	b.WriteString(content)
	if opts.FooterText != "" {
		b.WriteString("\n\n<!-- footer: ")
		b.WriteString(opts.FooterText)
		b.WriteString(" -->\n")
	}
	return b.Bytes()
}
