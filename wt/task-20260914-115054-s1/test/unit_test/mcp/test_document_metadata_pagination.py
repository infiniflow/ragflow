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

import importlib.util
import re
from collections import OrderedDict
from pathlib import Path

import pytest


def _load_mcp_server():
    server_path = Path(__file__).resolve().parents[3] / "mcp" / "server" / "server.py"
    spec = importlib.util.spec_from_file_location("ragflow_mcp_server_unit", server_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class _FakeResponse:
    status_code = 200

    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


class _LoopGuard(BaseException):
    """Raised when the pagination loop fails to terminate.

    Subclasses BaseException so it escapes the method's ``except Exception`` and
    fails the test instead of hanging the suite forever (the #16248 regression).
    """


@pytest.fixture()
def mcp_server():
    return _load_mcp_server()


def _fresh_connector(mcp_server):
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    # The metadata caches are class-level OrderedDicts shared across instances;
    # shadow them per test so cases don't pollute one another.
    connector._dataset_metadata_cache = OrderedDict()
    connector._document_metadata_cache = OrderedDict()
    return connector


def _stub_get(monkeypatch, connector, total, *, code=0, page_size=30, guard=100):
    """Mock RAGFlowConnector._get: serves the dataset-info call and a paginated
    documents endpoint backed by ``total`` synthetic documents."""
    all_docs = [{"id": f"doc-{i}", "name": f"name-{i}"} for i in range(total)]
    doc_requests = []

    async def _get(path, params=None, api_key=""):
        if "/documents?" in path:
            doc_requests.append(path)
            if len(doc_requests) > guard:
                raise _LoopGuard(f"pagination did not terminate after {guard} requests")
            page = int(re.search(r"[?&]page=(\d+)", path).group(1))
            requested_size = int(re.search(r"[?&]page_size=(\d+)", path).group(1))
            start = (page - 1) * requested_size
            return _FakeResponse({"code": code, "data": {"docs": all_docs[start : start + requested_size], "total": total}})
        # dataset-info lookup (/datasets?id=...)
        return _FakeResponse({"code": 0, "data": [{"name": "DS", "description": "d"}]})

    monkeypatch.setattr(connector, "_get", _get)
    return doc_requests


@pytest.mark.asyncio
async def test_empty_dataset_terminates_without_infinite_loop(monkeypatch, mcp_server):
    # Regression for #16268's sibling #16248: an empty docs page used to loop forever.
    connector = _fresh_connector(mcp_server)
    doc_requests = _stub_get(monkeypatch, connector, total=0)

    document_cache, _ = await connector._get_document_metadata_cache(["ds-empty"], api_key="k")

    assert document_cache == {}
    assert len(doc_requests) == 1  # one request, then stop -- no re-request loop


@pytest.mark.asyncio
async def test_api_error_terminates(monkeypatch, mcp_server):
    connector = _fresh_connector(mcp_server)
    doc_requests = _stub_get(monkeypatch, connector, total=50, code=100)

    document_cache, _ = await connector._get_document_metadata_cache(["ds-err"], api_key="k")

    assert document_cache == {}
    assert len(doc_requests) == 1  # error response stops pagination immediately


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "total, expected_pages",
    [(15, 1), (30, 2), (31, 2), (45, 2), (60, 3), (90, 4)],
)
async def test_fetches_every_document_across_pages(monkeypatch, mcp_server, total, expected_pages):
    # The old `total - page * page_size` check stopped one page early and dropped
    # documents (e.g. 60 docs -> only 30 cached). Verify every document is fetched.
    connector = _fresh_connector(mcp_server)
    doc_requests = _stub_get(monkeypatch, connector, total=total)

    document_cache, _ = await connector._get_document_metadata_cache(["ds"], api_key="k")

    assert len(document_cache) == total
    assert {f"doc-{i}" for i in range(total)} == set(document_cache)
    assert len(doc_requests) == expected_pages


@pytest.mark.asyncio
async def test_documents_request_sends_explicit_page_size(monkeypatch, mcp_server):
    # page_size is now sent explicitly so the client/server page sizes can't drift.
    connector = _fresh_connector(mcp_server)
    doc_requests = _stub_get(monkeypatch, connector, total=5)

    await connector._get_document_metadata_cache(["ds"], api_key="k")

    assert doc_requests
    assert "page_size=30" in doc_requests[0]
