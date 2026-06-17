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

// TestWriteTXT_BodyOnly: minimal config — body only. The writer
// still emits a leading blank line (the separator that normally
// follows the header / timestamp). This matches the Python
// _generate_txt byte-for-byte when header/timestamp are absent.
func TestWriteTXT_BodyOnly(t *testing.T) {
	out := string(WriteTXT("hello world", TXTOptions{}))
	if out != "\nhello world" {
		t.Errorf("output = %q, want %q", out, "\nhello world")
	}
}

// TestWriteTXT_FullConfig: header + timestamp + body + footer.
// The fixed timestamp keeps the test deterministic.
func TestWriteTXT_FullConfig(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	out := string(WriteTXT("body text", TXTOptions{
		HeaderText:   "My Doc",
		FooterText:   "page 1",
		AddTimestamp: true,
		Now:          fixed,
	}))
	want := "My Doc\nGenerated: 2026-06-15T12:00:00Z\n\nbody text\npage 1"
	if out != want {
		t.Errorf("output mismatch:\n got  %q\n want %q", out, want)
	}
}

// TestWriteTXT_HeaderWithoutTimestamp: header-only path.
func TestWriteTXT_HeaderWithoutTimestamp(t *testing.T) {
	out := string(WriteTXT("body", TXTOptions{HeaderText: "H"}))
	if !strings.HasPrefix(out, "H\n\nbody") {
		t.Errorf("output = %q, want header-then-body layout", out)
	}
	if strings.Contains(out, "Generated:") {
		t.Errorf("unexpected timestamp line: %q", out)
	}
}

// TestWriteTXT_FooterOnly: footer-only path.
func TestWriteTXT_FooterOnly(t *testing.T) {
	out := string(WriteTXT("body", TXTOptions{FooterText: "F"}))
	if !strings.HasSuffix(out, "\nF") {
		t.Errorf("output = %q, want body-then-footer layout", out)
	}
}

// TestWriteTXT_EmptyContent: empty body produces just the
// header/footer scaffolding.
func TestWriteTXT_EmptyContent(t *testing.T) {
	out := string(WriteTXT("", TXTOptions{HeaderText: "H", FooterText: "F"}))
	if !strings.Contains(out, "H\n\n") {
		t.Errorf("output = %q, want header line followed by blank line", out)
	}
	if !strings.HasSuffix(out, "\nF") {
		t.Errorf("output = %q, want trailing footer", out)
	}
}
