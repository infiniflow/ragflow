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

# Chat assistant SDK tests

Run from the repository root with Python 3.13, the current `sdk/python` dependencies,
pytest and Pillow installed. These tests deliberately exclude ancestor fixtures
that provision models, require provider keys, or rewrite marker selection.
Both `-c` and `--confcutdir` are required:

```sh
folder=test/testcases/test_sdk_api/test_chat_assistant_management
python -m pytest -c "$folder/pytest.ini" --confcutdir="$folder" "$folder" --collect-only -q
python -m pytest -c "$folder/pytest.ini" --confcutdir="$folder" "$folder" -m 'not integration' -q
```

Mocked tests need no backend or credentials and block unmocked network requests.
Live tests fail setup unless all three environment variables are configured:

- `RAGFLOW_SDK_ISOLATED=1`: confirms a dedicated disposable Go deployment and tenant.
- `RAGFLOW_SDK_BASE_URL`: the deployment base URL.
- `RAGFLOW_SDK_API_KEY`: the dedicated tenant owner's API key.

Provision that environment independently; a reachable server does not establish
isolation. Use an empty tenant without shared resources, with a configured default
chat model, embedding model, document engine and storage. Dataset association tests
upload documents and add manual chunks; they never start parsing or poll jobs.
Run serially, without xdist or another client sharing the tenant. The initial
preflight rejects accessible chats/datasets; delete-all cases additionally check
the complete current chat IDs immediately before deletion.

Run the live suite by replacing `-m 'not integration'` with `-m integration`.
Cleanup deletes only IDs created by each test and reports cleanup errors. No model
configuration or tenant-wide cleanup runs automatically. Targeted source-defect
skips retain their expected assertions and need rechecking when their owning
SDK or Go implementation changes; a skip is not a live reproduction.
