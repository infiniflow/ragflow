#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import pytest

from ragflow_sdk.modules.memory import Memory


class _StubResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


class _StubRag:
    def __init__(self, response):
        self._response = response

    def get(self, *_args, **_kwargs):
        return self._response


@pytest.mark.p2
def test_memory_get_config_error_raises():
    rag = _StubRag(_StubResponse({"code": 1, "message": "boom"}))
    memory = Memory(rag, {"id": "memory_id"})
    with pytest.raises(Exception) as excinfo:
        memory.get_config()
    assert "boom" in str(excinfo.value)
