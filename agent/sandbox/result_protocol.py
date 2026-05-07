#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import base64
import json
from typing import Any


RESULT_MARKER_PREFIX = "__RAGFLOW_RESULT__:"


def build_python_wrapper(code: str, args_json: str) -> str:
    return f'''{code}

if __name__ == "__main__":
    import base64
    import json

    result = main(**{args_json})
    payload = json.dumps({{"present": True, "value": result, "type": "json"}}, ensure_ascii=False, separators=(",", ":"))
    print("{RESULT_MARKER_PREFIX}" + base64.b64encode(payload.encode("utf-8")).decode("ascii"))
'''


def build_javascript_wrapper(code: str, args_json: str) -> str:
    return f'''{code}

const __ragflowArgs = {args_json};

(async () => {{
  const __ragflowMain = typeof main !== 'undefined' ? main : module.exports && module.exports.main;
  if (typeof __ragflowMain !== 'function') {{
    throw new Error('main() must be defined or exported.');
  }}
  const output = await Promise.resolve(__ragflowMain(__ragflowArgs));
  if (typeof output === 'undefined') {{
    throw new Error('main() must return a value. Use null for an empty result.');
  }}
  const payload = JSON.stringify({{ present: true, value: output, type: 'json' }});
  if (typeof payload === 'undefined') {{
    throw new Error('main() returned a non-JSON-serializable value.');
  }}
  console.log('{RESULT_MARKER_PREFIX}' + Buffer.from(payload, 'utf8').toString('base64'));
}})();
'''


def extract_structured_result(stdout: str) -> tuple[str, dict[str, Any]]:
    if not stdout:
        return "", {}

    cleaned_lines: list[str] = []
    structured_result: dict[str, Any] = {}

    for line in str(stdout).splitlines():
        if line.startswith(RESULT_MARKER_PREFIX):
            payload_b64 = line[len(RESULT_MARKER_PREFIX) :].strip()
            if not payload_b64:
                cleaned_lines.append(line)
                continue
            try:
                payload = base64.b64decode(payload_b64, validate=True).decode("utf-8")
                structured_result = json.loads(payload)
            except Exception:
                cleaned_lines.append(line)
            continue
        cleaned_lines.append(line)

    cleaned_stdout = "\n".join(cleaned_lines)
    if stdout.endswith("\n") and cleaned_stdout and not cleaned_stdout.endswith("\n"):
        cleaned_stdout += "\n"
    return cleaned_stdout, structured_result
