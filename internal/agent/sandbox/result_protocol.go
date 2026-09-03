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

// result_protocol.go is the Go port of `agent/sandbox/result_protocol.py`.
//
// The contract:
//
//   1. The user's code is expected to define a `main(**args)` function
//      (Python) or export a `main(args)` function (JavaScript).
//   2. The provider wraps the code in a small driver that calls main
//      with the agent-supplied arguments and emits a marker line
//      carrying main's return value as base64-JSON. The marker prefix
//      is `__RAGFLOW_RESULT__:`. This is the ONLY line the agent code
//      parser keeps from the synthesized output — the rest is the
//      user's stdout, surfaced verbatim.
//
// The marker protocol is a contract with `executor_manager`
// (Python FastAPI service that runs the actual code). Renaming the
// marker is a wire-format break — `executor_manager` parses for this
// exact prefix. See `agent/sandbox/executor_manager/services/execution.py`
// for the Python side that depends on it.

package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// resultMarkerPrefix is the wire-level marker the executor_manager
// Python service scans stdout for. Keep in sync with Python
// `agent/sandbox/result_protocol.py::RESULT_MARKER_PREFIX`.
const resultMarkerPrefix = "__RAGFLOW_RESULT__:"

// BuildPythonWrapper wraps a Python source so that:
//
//   - When executed as `python -c <wrapped>`, the user-defined
//     `main(**args)` is invoked with the JSON-decoded args.
//   - main's return value is JSON-encoded, prefixed with the
//     marker, and printed to stdout.
//
// argsJSON is base64-encoded and decoded inside Python via
// json.loads(base64.b64decode(...)). The base64 alphabet has no
// characters that conflict with Python syntax, so splicing the
// encoded string into a Python literal is safe. This avoids the
// fragility of embedding raw JSON directly (true/false/null vs
// Python's True/False/None) and removes the unsafe-quoting sink
// from CodeQL's view.
func BuildPythonWrapper(code, argsJSON string) string {
	argsB64 := base64.StdEncoding.EncodeToString([]byte(argsJSON))
	return code + `

if __name__ == "__main__":
    import base64
    import json

    result = main(**json.loads(base64.b64decode("` + argsB64 + `").decode("utf-8")))
    payload = json.dumps({"present": True, "value": result, "type": "json"}, ensure_ascii=False, separators=(",", ":"))
    print("` + resultMarkerPrefix + `" + base64.b64encode(payload.encode("utf-8")).decode("ascii"))
`
}

// BuildJavaScriptWrapper wraps a JavaScript source so that:
//
//   - When executed as `node -e <wrapped>`, the user-defined
//     `main(args)` (or `module.exports.main`) is awaited with the
//     JSON-decoded args object.
//   - main's return value is JSON-encoded, prefixed with the
//     marker, and printed to stdout.
//
// JavaScript lacks a "module" boundary in `node -e`, so we look for
// `main` in (a) the global scope and (b) `module.exports.main`,
// matching the Python wrapper.
//
// argsJSON is embedded as a base64 literal (alphabet contains no JS
// syntax-significant characters) and decoded at runtime via
// JSON.parse(Buffer.from(..., 'base64').toString('utf8')), so the
// only Go-side dataflow into the JS source is the base64 string.
func BuildJavaScriptWrapper(code, argsJSON string) string {
	argsB64 := base64.StdEncoding.EncodeToString([]byte(argsJSON))
	// Note: this string is *embedded inside* a Go raw string, but the
	// Go raw string and the JS source are independent languages. We
	// need the final JS to be valid; the doubled braces {{ }} are JS
	// template-literal escapes only on the JS side. We pass them
	// through as-is.
	return code + `

const __ragflowArgsB64 = "` + argsB64 + `";
const __ragflowArgs = JSON.parse(Buffer.from(__ragflowArgsB64, 'base64').toString('utf8'));

(async () => {
  const __ragflowMain = typeof main !== 'undefined' ? main : module.exports && module.exports.main;
  if (typeof __ragflowMain !== 'function') {
    throw new Error('main() must be defined or exported.');
  }
  const output = await Promise.resolve(__ragflowMain(__ragflowArgs));
  if (typeof output === 'undefined') {
    throw new Error('main() must return a value. Use null for an empty result.');
  }
  const payload = JSON.stringify({ present: true, value: output, type: 'json' });
  if (typeof payload === 'undefined') {
    throw new Error('main() returned a non-JSON-serializable value.');
  }
  console.log('` + resultMarkerPrefix + `' + Buffer.from(payload, 'utf8').toString('base64'));
})();
`
}

// ExtractStructuredResult scans stdout for the marker line, decodes
// the JSON payload after it, and returns the user-visible stdout
// (with the marker line removed) plus the parsed structured result.
//
// The Python side returns `(cleaned_stdout, structured_result_dict)`.
// On Go the dict is `map[string]any`.
//
// Edge cases (matching the Python implementation):
//   - empty stdout → ("", empty map).
//   - multiple marker lines → only the LAST one wins (later result
//     overrides earlier). The Python implementation does the same
//     because the loop overwrites `structured_result`.
//   - undecodable payload → the marker line is kept in the cleaned
//     stdout (the user gets to see the raw base64) and the map stays
//     empty. Python's `except Exception: cleaned_lines.append(line)`
//     does the same.
//   - the trailing newline is preserved if the input had one.
func ExtractStructuredResult(stdout string) (string, map[string]any) {
	if stdout == "" {
		return "", map[string]any{}
	}

	cleanedLines := []string{}
	structured := map[string]any{}

	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, resultMarkerPrefix) {
			payloadB64 := strings.TrimSpace(line[len(resultMarkerPrefix):])
			if payloadB64 == "" {
				cleanedLines = append(cleanedLines, line)
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(payloadB64)
			if err != nil {
				cleanedLines = append(cleanedLines, line)
				continue
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				cleanedLines = append(cleanedLines, line)
				continue
			}
			structured = decoded
			continue
		}
		cleanedLines = append(cleanedLines, line)
	}

	cleaned := strings.Join(cleanedLines, "\n")
	if strings.HasSuffix(stdout, "\n") && cleaned != "" && !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}
	return cleaned, structured
}

// argsToJSON is a small helper used by the providers to build the
// args string the wrapper expects. Empty/nil maps serialize to "{}"
// so the wrapper can always json.loads safely.
func argsToJSON(args map[string]any) (string, error) {
	if args == nil {
		return "{}", nil
	}
	// json.Marshal of a nil map produces "null" — replace with "{}"
	// so the wrappers see an object literal in both languages.
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("sandbox: marshal args: %w", err)
	}
	if string(b) == "null" {
		return "{}", nil
	}
	return string(b), nil
}
