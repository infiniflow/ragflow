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

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWencai_StubsUnsupported(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args string
	}{
		{
			name: "well-formed args",
			args: `{"query":"近期涨停股","page":1,"per_page":20}`,
		},
		{
			name: "minimal args",
			args: `{"query":"高股息低估值"}`,
		},
		{
			name: "missing query",
			args: `{"page":1}`,
		},
		{
			name: "empty payload",
			args: `{}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := NewWencaiTool()
			out, err := tool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil (out=%s)", out)
			}
			if !strings.Contains(err.Error(), "同花顺") {
				t.Errorf("err = %q, want to mention 同花顺", err.Error())
			}
			var env wencaiEnvelope
			if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
				t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
			}
			if env.Error == "" {
				t.Errorf("env.Error = empty, want populated")
			}
			if !strings.Contains(env.Error, "同花顺") {
				t.Errorf("env.Error = %q, want to mention 同花顺", env.Error)
			}
		})
	}
}

func TestWencai_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	tool := NewWencaiTool()
	_, err := tool.InvokableRun(context.Background(), `{not json`)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "同花顺") {
		t.Errorf("err = %q, want to mention 同花顺", err.Error())
	}
}

func TestWencai_Info(t *testing.T) {
	t.Parallel()

	tool := NewWencaiTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "wencai" {
		t.Errorf("Name = %q, want wencai", info.Name)
	}
	if !strings.Contains(info.Desc, "Wencai") {
		t.Errorf("Desc = %q, want to mention Wencai", info.Desc)
	}
	if !strings.Contains(info.Desc, "STUB") && !strings.Contains(info.Desc, "Python") {
		t.Errorf("Desc = %q, want to flag stub status", info.Desc)
	}
}
