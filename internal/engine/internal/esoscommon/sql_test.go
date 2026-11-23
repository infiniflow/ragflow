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

package esoscommon_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/engine/internal/esoscommon"
	"ragflow/internal/tokenizer"
)

func TestMain(m *testing.M) {
	tokenizer.SetEngineType("infinity")
	m.Run()
}

func TestPreprocessSQL_WhitespaceAndBackticks(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a  b", "a b"},
		{"a   b   c", "a b c"},
		{"a`b`c", "a b c"},
		{"a `` b", "a b"},
		{"  leading and trailing  ", " leading and trailing "},
	}
	for _, c := range cases {
		if got := esoscommon.PreprocessSQL(c.in); got != c.want {
			t.Errorf("PreprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocessSQL_StripsPercent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"count > 0 %", "count > 0 "},
		{"100% match", "100 match"},
		{"%%%", ""},
	}
	for _, c := range cases {
		if got := esoscommon.PreprocessSQL(c.in); got != c.want {
			t.Errorf("PreprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocessSQL_LktksRewrite(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		field  string
		expect string
	}{
		{
			"like with single-token value (ltks suffix)",
			"select content_ltks like 'weather'",
			"content_ltks",
			"MATCH(content_ltks,",
		},
		{
			"= with multi-word value (ltks suffix)",
			"select content_ltks = 'final report'",
			"content_ltks",
			"MATCH(content_ltks,",
		},
		{
			"tks (no l) suffix",
			"select title_tks = 'hello'",
			"title_tks",
			"MATCH(title_tks,",
		},
		{
			"leading-space anchor: no leading space means no match (mirrors Python regex)",
			"content_ltks like 'weather'",
			"content_ltks",
			"content_ltks like 'weather'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := esoscommon.PreprocessSQL(c.in)
			isAnchorTest := c.expect == c.in
			if isAnchorTest {
				if got != c.in {
					t.Errorf("PreprocessSQL(%q) = %q, want unchanged (leading-space anchor should prevent match)", c.in, got)
				}
				return
			}
			if strings.Contains(got, c.field+" ") {
				pattern := regexp.MustCompile(c.field + `( like | ?= ?)`)
				if pattern.MatchString(got) {
					t.Errorf("PreprocessSQL(%q) = %q, still contains the original `<field> like/=` pattern", c.in, got)
				}
			}
			if !strings.Contains(got, c.expect) {
				t.Errorf("PreprocessSQL(%q) = %q, want substring %q", c.in, got, c.expect)
			}
			if !strings.Contains(got, "minimum_should_match=30") {
				t.Errorf("PreprocessSQL(%q) = %q, want substring minimum_should_match=30", c.in, got)
			}
		})
	}
}

func TestPreprocessSQL_NoMatchLeavesSQLAlone(t *testing.T) {
	in := "SELECT doc_id FROM ragflow_t1"
	got := esoscommon.PreprocessSQL(in)
	if got != in {
		t.Errorf("PreprocessSQL(%q) = %q, want unchanged", in, got)
	}
}

// fakeNetTimeoutErr implements net.Error with Timeout()==true.
type fakeNetTimeoutErr struct{}

func (fakeNetTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeNetTimeoutErr) Timeout() bool   { return true }
func (fakeNetTimeoutErr) Temporary() bool { return true }

func TestIsTimeoutError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("wrap: %w", context.DeadlineExceeded), true},
		{"net.Error.Timeout()==true", fakeNetTimeoutErr{}, true},
		{"wrapped net.Error.Timeout", fmt.Errorf("wrap: %w", fakeNetTimeoutErr{}), true},
		{"plain string 'i/o timeout'", errors.New("read tcp: i/o timeout"), true},
		{"plain string 'deadline exceeded'", errors.New("context deadline exceeded"), true},
		{"plain string 'connection timeout'", errors.New("connection timeout while reading"), true},
		{"unrelated error", errors.New("parse: invalid character"), false},
		{"EOF is not a timeout", errors.New("EOF"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := esoscommon.IsTimeoutError(c.err); got != c.want {
				t.Errorf("IsTimeoutError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsTimeoutError_NonTimeoutNetError(t *testing.T) {
	e := &net.OpError{
		Op:  "dial",
		Err: errors.New("connection refused"),
	}
	if esoscommon.IsTimeoutError(e) {
		t.Errorf("IsTimeoutError(connection-refused) = true, want false")
	}
}
