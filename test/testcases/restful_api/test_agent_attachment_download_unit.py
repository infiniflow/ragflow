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

"""Unit tests for GET /agents/attachments/{id}/download (#15502)."""

from types import SimpleNamespace

import pytest
from testcases.test_http_api.test_session_management.test_session_sdk_routes_unit import (
    _load_agent_api_module,
    _run,
)


async def _immediate_thread_pool(func, *args, **kwargs):
    return func(*args, **kwargs)


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


@pytest.mark.p2
def test_download_attachment_denied_when_not_accessible(monkeypatch):
    module = _load_agent_api_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"ext": "markdown"}))
    monkeypatch.setattr(module, "thread_pool_exec", _immediate_thread_pool)
    monkeypatch.setattr(module, "_agent_attachment_accessible", lambda *_args, **_kwargs: False)

    res = _run(module.download_attachment(tenant_id="tenant-1", attachment_id="att-denied"))

    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Document not found!" in res["message"], res


@pytest.mark.p2
def test_download_attachment_empty_blob_returns_error(monkeypatch):
    module = _load_agent_api_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"ext": "markdown"}))
    monkeypatch.setattr(module, "thread_pool_exec", _immediate_thread_pool)
    monkeypatch.setattr(module, "_agent_attachment_accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(
        module.settings,
        "STORAGE_IMPL",
        SimpleNamespace(get=lambda *_args, **_kwargs: None),
    )

    res = _run(module.download_attachment(tenant_id="tenant-1", attachment_id="att-missing"))

    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Document not found!" in res["message"], res


@pytest.mark.p2
def test_download_attachment_success_returns_bytes(monkeypatch):
    module = _load_agent_api_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"ext": "pdf"}))
    monkeypatch.setattr(module, "thread_pool_exec", _immediate_thread_pool)
    monkeypatch.setattr(module, "_agent_attachment_accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(
        module.settings,
        "STORAGE_IMPL",
        SimpleNamespace(get=lambda *_args, **_kwargs: b"%PDF-1.4 ok"),
    )

    async def fake_make_response(data):
        return _DummyResponse(data)

    monkeypatch.setattr(module, "make_response", fake_make_response)
    monkeypatch.setattr(
        module,
        "apply_safe_file_response_headers",
        lambda response, content_type, extension: response.headers.update(
            {"content_type": content_type, "extension": extension}
        ),
    )

    res = _run(module.download_attachment(tenant_id="tenant-1", attachment_id="att-ok"))

    assert isinstance(res, _DummyResponse), res
    assert res.data == b"%PDF-1.4 ok", res
    assert res.headers["content_type"] == "application/pdf", res.headers


@pytest.mark.p2
def test_agent_attachment_accessible_uses_document_service_first(monkeypatch):
    module = _load_agent_api_module(monkeypatch)
    calls = []

    monkeypatch.setattr(
        module.DocumentService,
        "accessible",
        lambda doc_id, user_id: calls.append(("accessible", doc_id, user_id)) or True,
    )
    monkeypatch.setattr(module, "_agent_attachment_in_user_conversation", lambda *_: calls.append(("conv",)) or False)

    assert module._agent_attachment_accessible("doc-1", "user-1") is True
    assert calls == [("accessible", "doc-1", "user-1")]


@pytest.mark.p2
def test_agent_attachment_accessible_falls_back_to_conversation(monkeypatch):
    module = _load_agent_api_module(monkeypatch)
    calls = []

    monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
    monkeypatch.setattr(
        module,
        "_agent_attachment_in_user_conversation",
        lambda doc_id, user_id: calls.append((doc_id, user_id)) or True,
    )

    assert module._agent_attachment_accessible("doc-2", "user-2") is True
    assert calls == [("doc-2", "user-2")]
