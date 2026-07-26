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

// Package io — TXT writer.
//
// The TXT writer is the trivial plain-text path: header / footer /
// timestamp are wrapped as plain text lines around the body. No
// encoding negotiation (UTF-8 only); no markdown stripping
// (downstream tools that don't understand markdown can consume TXT
// when the source content is markdown-flavoured — the writer does
// not rewrite the body).

package io

import (
	"bytes"
	"fmt"
	"time"
)

// TXTOptions is the public contract for the TXT writer.
type TXTOptions struct {
	HeaderText   string
	FooterText   string
	AddTimestamp bool
	// Now overrides time.Now() for deterministic tests. Zero value
	// means "use the real clock".
	Now time.Time
}

// WriteTXT renders plain text per the Python `_generate_txt`
// contract: optional header line, optional "Generated: ..." line,
// body, optional footer line. The shape is byte-exact stable so
// downstream byte-equality tests can pin it.
func WriteTXT(content string, opts TXTOptions) []byte {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var b bytes.Buffer
	if opts.HeaderText != "" {
		b.WriteString(opts.HeaderText)
		b.WriteString("\n")
	}
	if opts.AddTimestamp {
		fmt.Fprintf(&b, "Generated: %s\n", now.Format(time.RFC3339))
	}
	b.WriteString("\n")
	b.WriteString(content)
	if opts.FooterText != "" {
		b.WriteString("\n")
		b.WriteString(opts.FooterText)
	}
	return b.Bytes()
}
