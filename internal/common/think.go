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

package common

import "strings"

// StripThinkTrailing cuts everything through the LAST `</think>` marker on the
// input and returns what follows, mirroring Python's
//
//	re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
//
// The Python regex is greedy: with re.DOTALL the leading `.*` matches as much
// as possible, so for an input with more than one `</think>` everything up to
// and including the LAST marker is stripped. A non-greedy form would strip only
// the first marker and leave a residual think block in the response. Go's
// strings.LastIndex has the same greedy semantics, so it is used instead of a
// regex. When no `</think>` is present, s is returned unchanged. Note the marker
// is matched regardless of whether an opening `<think>` exists — parity with
// Python, which does not require one.
func StripThinkTrailing(s string) string {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		return s[i+len("</think>"):]
	}
	return s
}
