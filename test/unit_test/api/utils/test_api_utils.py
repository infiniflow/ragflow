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

import importlib
import sys
from types import ModuleType, SimpleNamespace

import pytest

from common.constants import RetCode


def _load_api_utils(monkeypatch):
    settings_stub = ModuleType("common.settings")
    settings_stub.ALLOWED_LLM_FACTORIES = None
    settings_stub.STRONG_TEST_COUNT = 0
    monkeypatch.setitem(sys.modules, "common.settings", settings_stub)

    mcp_stub = ModuleType("common.mcp_tool_call_conn")
    mcp_stub.MCPToolCallSession = object
    mcp_stub.close_multiple_mcp_toolcall_sessions = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "common.mcp_tool_call_conn", mcp_stub)

    tenant_llm_stub = ModuleType("api.db.services.tenant_llm_service")
    tenant_llm_stub.LLMFactoriesService = SimpleNamespace(get_all=lambda **_kwargs: [])
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_stub)

    monkeypatch.delitem(sys.modules, "api.utils.api_utils", raising=False)
    return importlib.import_module("api.utils.api_utils")


@pytest.mark.p2
class TestBuildErrorResultHttpStatus:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "code, expected",
        [
            (RetCode.ARGUMENT_ERROR, RetCode.BAD_REQUEST),
            (RetCode.AUTHENTICATION_ERROR, RetCode.UNAUTHORIZED),
            (RetCode.PERMISSION_ERROR, RetCode.FORBIDDEN),
            (RetCode.NOT_FOUND, RetCode.NOT_FOUND),
            (RetCode.SERVER_ERROR, RetCode.SERVER_ERROR),
            ("oops", RetCode.BAD_REQUEST),
            (None, RetCode.BAD_REQUEST),
        ],
    )
    def test_resolve_error_http_status(self, monkeypatch, code, expected):
        api_utils = _load_api_utils(monkeypatch)
        assert api_utils._resolve_error_http_status(code) == expected

    @pytest.mark.p2
    def test_build_error_result_uses_normalized_http_status(self, monkeypatch):
        api_utils = _load_api_utils(monkeypatch)

        class _DummyResponse:
            def __init__(self, payload):
                self.payload = payload
                self.status_code = None

        monkeypatch.setattr(api_utils, "_safe_jsonify", lambda payload: _DummyResponse(payload))

        response = api_utils.build_error_result(code=RetCode.ARGUMENT_ERROR, message="required arguments are missing")

        assert response.payload["code"] == RetCode.ARGUMENT_ERROR
        assert response.status_code == RetCode.BAD_REQUEST

    @pytest.mark.p2
    def test_build_error_result_maps_authentication_to_401(self, monkeypatch):
        api_utils = _load_api_utils(monkeypatch)

        class _DummyResponse:
            def __init__(self, payload):
                self.payload = payload
                self.status_code = None

        monkeypatch.setattr(api_utils, "_safe_jsonify", lambda payload: _DummyResponse(payload))

        response = api_utils.build_error_result(code=RetCode.AUTHENTICATION_ERROR, message="No authorization.")

        assert response.payload["code"] == RetCode.AUTHENTICATION_ERROR
        assert response.status_code == RetCode.UNAUTHORIZED
