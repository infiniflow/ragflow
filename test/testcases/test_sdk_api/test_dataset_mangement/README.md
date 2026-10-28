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

# Dataset SDK tests

Run from the repository root with Python 3.13 and the current SDK installed:

```sh
uv venv --python 3.13 /tmp/ragflow-sdk-tests
uv pip install --python /tmp/ragflow-sdk-tests/bin/python -e sdk/python pytest
/tmp/ragflow-sdk-tests/bin/python -m pytest test/testcases/test_sdk_api/test_dataset_mangement --collect-only
/tmp/ragflow-sdk-tests/bin/python -m pytest test/testcases/test_sdk_api/test_dataset_mangement -m 'not integration'
```

The folder's `pytest.ini` keeps collection and mocked contracts independent of the
legacy parent conftests, provider keys, backend setup, and priority filtering.
For an explicit configuration path, use `-c test/testcases/test_sdk_api/test_dataset_mangement/pytest.ini`.
Do not collect this folder together with unaudited SDK folders under the old parent configuration.

Integration tests require a dedicated **Go** test deployment and an initially empty
test tenant. Do not use a shared deployment or production credentials. Provision
services and a default embedding model before running; the fixtures do not register
users, configure providers, or change tenant defaults. Run serially, without xdist.

Set these environment variables:

- `RAGFLOW_SDK_ISOLATED=1`: confirms the deployment and tenant are dedicated to this run.
- `RAGFLOW_SDK_BASE_URL`: the Go HTTP API base URL.
- `RAGFLOW_SDK_API_KEY`: an owner API key for the empty test tenant.
- `RAGFLOW_SDK_EMBEDDING_MODEL`: an accessible canonical `model@instance@provider` identifier for explicit model tests.
- `RAGFLOW_SDK_PIPELINE_ID` and `RAGFLOW_SDK_PIPELINE_VERSION_ID`: an accessible ingestion pipeline and its published version for parser-mode update tests.
- `RAGFLOW_SDK_DOC_ENGINE`: the configured document engine for pagerank capability tests.
- `RAGFLOW_SDK_OTHER_API_KEY`: a valid key belonging to an unrelated test tenant for resource permission tests.

```sh
/tmp/ragflow-sdk-tests/bin/python -m pytest test/testcases/test_sdk_api/test_dataset_mangement -m 'integration and not p3'
# Includes 1,000-dataset capacity and concurrent request scenarios:
/tmp/ragflow-sdk-tests/bin/python -m pytest test/testcases/test_sdk_api/test_dataset_mangement -m integration
```

Missing environment requirements fail setup with a prerequisite message; they are
not product-defect skips. Cleanup only deletes IDs returned by this test's creates,
including successful creates before a later failure, and preserves unrelated IDs.
No live test or fixture calls `delete_all=True`; its payload is checked with mocks.

Four explicit skips preserve supported expectations blocked by confirmed source defects:

- Create and update accept 128 BMP characters, but Go counts UTF-8 bytes in
  `internal/service/dataset/crud.go` and `update.go`.
- Pipeline creation is documented in the SDK, but `RAGFlow.create_dataset` cannot
  send `pipeline_version_id`, which Go requires (`empty_version_test.go`).
- Negative pagerank should report the lower bound; `update.go` reports the upper
  bound for both range violations.

The suite does not enable CI execution or change production implementations.
