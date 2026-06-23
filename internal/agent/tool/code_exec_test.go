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
	"errors"
	"strings"
	"testing"
)

func TestCodeExec_StubsErrorWhenClientMissing(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	out, err := c.InvokableRun(context.Background(), `{"language":"python","code":"def main(): return {}"}`)
	if !errors.Is(err, ErrCodeExecSandboxMissing) {
		t.Fatalf("err = %v, want ErrCodeExecSandboxMissing", err)
	}

	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if !got.Stub {
		t.Errorf("Stub = false, want true")
	}
	if !strings.Contains(got.Error, "sandbox") {
		t.Errorf("Error = %q, want to mention 'sandbox'", got.Error)
	}
}

func TestCodeExec_RejectsEmptyCode(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(), `{"language":"python","code":""}`)
	if err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("err = %v, want to mention empty code", err)
	}
}

func TestCodeExec_RejectsBadLanguage(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(), `{"language":"brainfuck","code":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "language") {
		t.Fatalf("err = %v, want to reject unsupported language", err)
	}
}

func TestCodeExec_AcceptsLangAlias(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	// Python tool also accepts "lang" as the field name; the Go shell
	// should still reach the stub branch.
	_, err := c.InvokableRun(context.Background(), `{"lang":"nodejs","script":"async function main() {}"}`)
	if !errors.Is(err, ErrCodeExecSandboxMissing) {
		t.Fatalf("err = %v, want ErrCodeExecSandboxMissing", err)
	}
}

func TestCodeExec_Info(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "execute_code" {
		t.Errorf("Name = %q, want execute_code", info.Name)
	}
	if !strings.Contains(info.Desc, "Python") {
		t.Errorf("Desc = %q, want to mention Python", info.Desc)
	}
}
