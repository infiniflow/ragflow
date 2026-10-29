<!--

 Copyright 2026 The InfiniFlow Authors. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.

-->

# Document SDK tests

This folder targets the current Python SDK and Go backend only. Its local pytest
configuration prevents inherited legacy backend fixtures and marker filters from
loading; it does not depend on the open dataset-audit PR or change other suites.
Run it serially from the repository root, without selecting a parent pytest
configuration or combining it with unaudited SDK folders.

## Service-free contracts

Install the SDK and its existing test dependencies into a Python 3.13 environment:

```sh
uv venv --python 3.13 /tmp/ragflow-sdk-document-tests
uv pip install --python /tmp/ragflow-sdk-document-tests/bin/python -e sdk/python --group sdk/python/pyproject.toml:test
/tmp/ragflow-sdk-document-tests/bin/python -m pytest test/testcases/test_sdk_api/test_file_management_within_dataset -m 'not integration'
/tmp/ragflow-sdk-document-tests/bin/python -m pytest test/testcases/test_sdk_api/test_file_management_within_dataset --collect-only -q
```

Unmarked contract tests cannot open network connections. Fixtures, payload mapping,
error propagation, download bytes and polling behavior are testable without Go,
credentials, model setup or external storage.

## Isolated Go integrations

Provide a disposable Go deployment and a dedicated **empty** test tenant. The API,
database, object store and configured search engine must work; parsing also needs
the ingestor, queue and the tenant's configured embedding model/provider. Do not
point these tests at a shared tenant or assume a running local service is isolated.

```sh
export RAGFLOW_SDK_ISOLATED=1
export RAGFLOW_SDK_BASE_URL=http://isolated-go-host:9380
export RAGFLOW_SDK_API_KEY=dedicated-test-tenant-key
/tmp/ragflow-sdk-document-tests/bin/python -m pytest test/testcases/test_sdk_api/test_file_management_within_dataset -m integration
```

Missing prerequisites fail setup rather than being mislabeled as product skips.
Before mutations, a read-only probe rejects a nonempty tenant. Each test creates
fresh datasets, and teardown deletes only tracked dataset IDs, including their
owned documents; no shared cleanup helper or tenant-wide deletion is used. HTTP
calls have connect/read timeouts; parsing waits have explicit deadlines.

`RAGFLOW_SDK_PARSE_TIMEOUT` optionally changes the parsing deadline (180 seconds by
default). A timeout or unavailable model/service is an environment/test failure,
not automatic evidence of a product defect. Targeted skips document independently
confirmed implementation defects in the affected tests. Python SDK CI execution
remains disabled; this PR does not change workflows or production code.
