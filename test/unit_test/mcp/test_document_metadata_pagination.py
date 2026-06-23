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

"""Unit tests for ``RAGFlowConnector._get_document_metadata_cache`` pagination (issue #16248)."""

import importlib.util
import urllib.parse
from pathlib import Path

import pytest


def _load_mcp_server():
    """Import ``mcp/server/server.py`` as a standalone module for testing."""
    server_path = Path(__file__).resolve().parents[3] / "mcp" / "server" / "server.py"
    spec = importlib.util.spec_from_file_location("ragflow_mcp_server_unit_pagination", server_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class _FakeResponse:
    """Minimal stand-in for an ``httpx.Response`` returning a fixed JSON payload."""

    status_code = 200

    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


def _docs(count):
    """Build ``count`` minimal document records, each with a unique id."""
    return [{"id": f"doc-{idx}", "name": f"name-{idx}"} for idx in range(count)]


@pytest.fixture()
def mcp_server():
    """Load the MCP server module once per test."""
    return _load_mcp_server()


def _stub_documents(monkeypatch, connector, *, docs, code=0, total=None, cap=50):
    """Stub ``connector._get`` to page through ``docs`` for the documents endpoint.

    Returns the list of recorded ``/documents`` request paths so a test can assert
    how many pages were fetched. ``cap`` is a safety valve: before this fix an empty
    page would loop forever, so once the recorded count exceeds ``cap`` the stub
    raises. ``_get_document_metadata_cache`` swallows that exception, so the recorded
    request count still surfaces the runaway instead of hanging the test.
    """
    doc_requests = []
    total = len(docs) if total is None else total

    async def _get(path, params=None, api_key=""):
        if "/documents" in path:
            doc_requests.append(path)
            if len(doc_requests) > cap:
                raise RuntimeError(f"runaway pagination: {len(doc_requests)} requests for {path}")
            qs = urllib.parse.parse_qs(urllib.parse.urlparse(path).query)
            page = int(qs.get("page", ["1"])[0])
            page_size = int(qs.get("page_size", ["30"])[0])
            start = (page - 1) * page_size
            return _FakeResponse({"code": code, "data": {"docs": docs[start : start + page_size], "total": total}})
        # Dataset-info lookup (`/datasets?id=...&page_size=1`).
        return _FakeResponse({"code": 0, "data": [{"name": "ds", "description": "d"}]})

    monkeypatch.setattr(connector, "_get", _get)
    return doc_requests


@pytest.mark.p2
@pytest.mark.asyncio
async def test_empty_docs_page_breaks_instead_of_looping(monkeypatch, mcp_server):
    """A successful response with an empty ``docs`` list must stop, not refetch forever (#16248)."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    doc_requests = _stub_documents(monkeypatch, connector, docs=[], total=0)

    document_cache, _ = await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert document_cache == {}
    assert len(doc_requests) == 1


@pytest.mark.p2
@pytest.mark.asyncio
async def test_nonzero_code_stops_pagination(monkeypatch, mcp_server):
    """A non-success response code must stop the loop rather than spin on it."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    doc_requests = _stub_documents(monkeypatch, connector, docs=_docs(10), code=100)

    document_cache, _ = await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert document_cache == {}
    assert len(doc_requests) == 1


@pytest.mark.p2
@pytest.mark.asyncio
async def test_paginates_through_exact_multiple_last_page(monkeypatch, mcp_server):
    """60 docs at page_size 30 = two full pages; the final page must not be dropped.

    The previous termination math (``total - page * page_size <= 0`` after
    incrementing) stopped after page 1 and silently dropped the last page.
    """
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    doc_requests = _stub_documents(monkeypatch, connector, docs=_docs(60))

    document_cache, _ = await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert len(document_cache) == 60
    assert len(doc_requests) == 2


@pytest.mark.p2
@pytest.mark.asyncio
async def test_paginates_partial_last_page(monkeypatch, mcp_server):
    """A partial final page (45 docs over two pages) must be fetched in full."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    doc_requests = _stub_documents(monkeypatch, connector, docs=_docs(45))

    document_cache, _ = await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert len(document_cache) == 45
    assert len(doc_requests) == 2


@pytest.mark.p2
@pytest.mark.asyncio
async def test_request_includes_explicit_page_size(monkeypatch, mcp_server):
    """page_size must be sent so the server's page size matches the math used here."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    doc_requests = _stub_documents(monkeypatch, connector, docs=_docs(5))

    await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert doc_requests
    assert all("page_size=30" in path for path in doc_requests)


@pytest.mark.p2
@pytest.mark.asyncio
async def test_partial_pagination_failure_is_not_cached(monkeypatch, mcp_server):
    """If a later page fails, the partial result must not be cached and served as complete.

    Page 1 succeeds (30 of 60 docs) but page 2 returns a non-zero code. The call
    still surfaces what it fetched, but nothing is cached, so a subsequent lookup
    re-fetches instead of serving an incomplete list for the whole TTL.
    """
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    page_1 = _docs(30)

    async def _get(path, params=None, api_key=""):
        if "/documents" in path:
            qs = urllib.parse.parse_qs(urllib.parse.urlparse(path).query)
            page = int(qs.get("page", ["1"])[0])
            if page == 1:
                return _FakeResponse({"code": 0, "data": {"docs": page_1, "total": 60}})
            return _FakeResponse({"code": 100, "data": {}})  # later page fails
        return _FakeResponse({"code": 0, "data": [{"name": "ds", "description": "d"}]})

    monkeypatch.setattr(connector, "_get", _get)

    document_cache, _ = await connector._get_document_metadata_cache(["ds-1"], api_key="k")

    assert len(document_cache) == 30
    assert connector._get_cached_document_metadata_by_dataset("ds-1") is None
