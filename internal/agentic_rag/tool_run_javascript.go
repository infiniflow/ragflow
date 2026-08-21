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

package agentic_rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// runJavascriptToolName is the tool the model calls to execute a sandbox-free
// ECMAScript 5.1 snippet.
const runJavascriptToolName = "run_javascript"

// runJavascriptToolDescription documents the strict ES5.1 sandbox contract:
// no module system, no external package, stdout is the output.
const runJavascriptToolDescription = "Execute a self-contained ECMAScript 5.1 (ES5.1) snippet and return its standard output.\n\n" +
	"## Strict grammar & capability rules (VIOLATIONS ARE REJECTED)\n" +
	"- **ECMAScript 5.1 ONLY.** Syntax from ES6+ is rejected: no let/const (use var), no arrow functions (=>), no template literals (`), no destructuring, no classes, no default/rest params, no for-of, no generators, no spread (...), no Promise/async/await, no Proxy, no Symbol, no Map/Set (use plain objects/arrays).\n" +
	"- **NO module system.** Do not use import, require, module.exports, exports, or define. There is NO Node.js/CommonJS runtime: external packages cannot be loaded. Only ES5.1 built-ins (Math, JSON, Array, Date, RegExp, String, Number, Object, etc.) are available.\n" +
	"- **Output is stdout.** Write results with console.log(...). The captured stdout (joined lines) is returned verbatim as the tool output. The snippet's return value is ignored unless you also console.log it.\n\n" +
	"## Input\n" +
	"- code: the complete ES5.1 source to run.\n\n" +
	"## Examples\n" +
	"1. Sum an array:\n" +
	"   var nums = [1,2,3,4]; var s = 0; for (var i = 0; i < nums.length; i++) { s += nums[i]; } console.log(\"sum=\" + s);\n" +
	"2. Parse JSON:\n" +
	"   var obj = JSON.parse('{\"a\":1,\"b\":2}'); console.log(obj.a + obj.b);"

// runJavascriptArgs is the JSON the model sends into InvokableRun.
type runJavascriptArgs struct {
	Code string `json:"code"`
}

// Bounds protecting the service from hostile / runaway snippets.
const (
	// runJavascriptMaxCodeBytes caps the source size accepted.
	runJavascriptMaxCodeBytes = 64 << 10 // 64 KiB
	// runJavascriptMaxStdoutBytes caps the captured stdout so a snippet cannot
	// exhaust memory by printing an unbounded stream.
	runJavascriptMaxStdoutBytes = 1 << 20 // 1 MiB
)

// runJavascriptTimeout is the hard wall-clock limit for a single snippet. It is
// enforced with a timer inside the tool rather than relying on the caller's
// context (an HTTP request context usually has no deadline, so a client that
// keeps the connection open would otherwise let `while(true){}` burn CPU
// forever). Declared as a var so tests can lower it.
var runJavascriptTimeout = 30 * time.Second

// RunJavascriptTool executes a strict ES5.1 snippet and returns its stdout.
type RunJavascriptTool struct{}

// NewRunJavascriptTool returns a RunJavascriptTool implementing eino's
// tool.InvokableTool.
func NewRunJavascriptTool() *RunJavascriptTool {
	return &RunJavascriptTool{}
}

// Info returns the tool's metadata for the chat model.
func (t *RunJavascriptTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: runJavascriptToolName,
		Desc: runJavascriptToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"code": {
				Type:     schema.String,
				Required: true,
				Desc:     "The complete ECMAScript 5.1 source to execute. ES6+ syntax and any module/require/import usage are rejected.",
			},
		}),
	}, nil
}

// es51ForbiddenPatterns are substrings that indicate ES6+ syntax or a module
// system, none of which the sandbox supports.
var es51ForbiddenPatterns = []string{
	"import ", "import(", "export ", "require(", "require (",
	"module.exports", "require ", "=>", "`",
	"let ", "const ", "=> ", "class ",
	"function*", "yield ", "async ", "await ",
	"...", "of ", "Proxy", "Symbol",
}

// InvokableRun validates the snippet against the ES5.1 contract, runs it in a
// fresh goja runtime, and returns the captured stdout. The snippet is bounded by
// the incoming context (deadline/cancellation triggers goja's interpreter
// interrupt, so e.g. `while(true){}` cannot hang the goroutine forever) and by
// code-size + stdout-size caps.
func (t *RunJavascriptTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args runJavascriptArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("run_javascript: parse arguments: %w", err)
	}

	code := strings.TrimSpace(args.Code)
	if code == "" {
		return "", fmt.Errorf("run_javascript: code is required and must be a non-empty string")
	}
	if len(code) > runJavascriptMaxCodeBytes {
		return "", fmt.Errorf("run_javascript: code too large (%d bytes > %d)", len(code), runJavascriptMaxCodeBytes)
	}
	if err := assertES51(code); err != nil {
		return "", err
	}

	vm := goja.New()

	// Capture console.log/stdout. goja has no console; the host injects it.
	// The buffer is size-capped so an unbounded print cannot exhaust memory.
	stdout := newBoundedBuffer(runJavascriptMaxStdoutBytes)
	vm.Set("console", map[string]func(goja.FunctionCall) goja.Value{
		"log": func(fc goja.FunctionCall) goja.Value {
			for i, arg := range fc.Arguments {
				if i > 0 {
					stdout.writeByte(' ')
				}
				stdout.writeString(arg.String())
			}
			stdout.writeByte('\n')
			return goja.Undefined()
		},
	})

	// Interrupt the interpreter on the earliest of: the request context being
	// done (deadline/cancellation) or the tool's own hard timeout. goja returns
	// an InterruptedError at the next instruction boundary, which turns a tight
	// loop into a bounded failure even when the caller never cancels the request.
	done := make(chan struct{})
	defer close(done)
	timeout := time.NewTimer(runJavascriptTimeout)
	defer timeout.Stop()
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt("run_javascript: interrupted: request context cancelled or timed out")
		case <-timeout.C:
			vm.Interrupt("run_javascript: interrupted: execution exceeded " + runJavascriptTimeout.String())
		case <-done:
		}
	}()

	if _, err := vm.RunString(code); err != nil {
		return "", fmt.Errorf("run_javascript: execution error: %w", err)
	}

	return stdout.String(), nil
}

// boundedBuffer is a bytes.Buffer that silently stops growing once it reaches
// maxBytes, so a snippet's console output cannot exhaust host memory.
type boundedBuffer struct {
	buf      bytes.Buffer
	max      int
	exceeded bool
}

func newBoundedBuffer(max int) *boundedBuffer {
	return &boundedBuffer{max: max}
}

func (b *boundedBuffer) writeString(s string) {
	if b.exceeded {
		return
	}
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.exceeded = true
		return
	}
	if len(s) > remaining {
		s = s[:remaining]
		b.exceeded = true
	}
	b.buf.WriteString(s)
}

func (b *boundedBuffer) writeByte(c byte) {
	if b.exceeded {
		return
	}
	if b.buf.Len() >= b.max {
		b.exceeded = true
		return
	}
	b.buf.WriteByte(c)
}

func (b *boundedBuffer) String() string { return b.buf.String() }

// assertES51 rejects snippets that obviously use ES6+ syntax or a module
// system. goja itself only implements ES5.1, but this gives a clear, early
// error message instead of a confusing parse failure.
func assertES51(code string) error {
	for _, p := range es51ForbiddenPatterns {
		if strings.Contains(code, p) {
			return fmt.Errorf("run_javascript: ES6+ or module syntax not allowed: found %q — only ECMAScript 5.1 is supported", p)
		}
	}
	return nil
}
