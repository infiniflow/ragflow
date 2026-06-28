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

func TestAkShare_StubsUnsupported(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    string
		wantSub string
	}{
		{
			name:    "well-formed args",
			args:    `{"symbol":"000001","indicator":"stock_zh_a_hist"}`,
			wantSub: "Python library",
		},
		{
			name:    "missing indicator",
			args:    `{"symbol":"000001"}`,
			wantSub: "Python library",
		},
		{
			name:    "missing symbol",
			args:    `{"indicator":"stock_zh_a_hist"}`,
			wantSub: "Python library",
		},
		{
			name:    "empty payload",
			args:    `{}`,
			wantSub: "Python library",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := NewAkShareTool()
			out, err := tool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil (out=%s)", out)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
			var env akshareEnvelope
			if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
				t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
			}
			if env.Error == "" {
				t.Errorf("env.Error = empty, want populated")
			}
			if !strings.Contains(env.Error, "Python") {
				t.Errorf("env.Error = %q, want to mention Python Canvas", env.Error)
			}
		})
	}
}

func TestAkShare_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	tool := NewAkShareTool()
	_, err := tool.InvokableRun(context.Background(), `{not json`)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "Python") {
		t.Errorf("err = %q, want to mention Python", err.Error())
	}
}

func TestAkShare_Info(t *testing.T) {
	t.Parallel()

	tool := NewAkShareTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "akshare" {
		t.Errorf("Name = %q, want akshare", info.Name)
	}
	if !strings.Contains(info.Desc, "AkShare") {
		t.Errorf("Desc = %q, want to mention AkShare", info.Desc)
	}
	if !strings.Contains(info.Desc, "STUB") && !strings.Contains(info.Desc, "Python") {
		t.Errorf("Desc = %q, want to flag stub status", info.Desc)
	}
}
