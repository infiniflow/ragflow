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


@pytest.fixture()
def mcp_server():
    return _load_mcp_server()


def _stub_retrieval(monkeypatch, connector):
    captured = {}

    async def _post(path, json=None, api_key=""):
        captured["path"] = path
        captured["payload"] = dict(json)
        return _FakeResponse({"code": 0, "data": {"chunks": [], "total": 0, "doc_aggs": []}})

    async def _get_document_metadata_cache(*args, **kwargs):
        return {}, {}

    monkeypatch.setattr(connector, "_post", _post)
    monkeypatch.setattr(connector, "_get_document_metadata_cache", _get_document_metadata_cache)
    return captured


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "page,page_size",
    [
        (1, 10),  # default page_size
        (1, 50),  # recommended page_size
        (2, 50),  # paging keeps the same candidate window
        (7, 10),  # default page_size paged past the backend's default 64
        (1, 100),  # schema maximum
    ],
)
async def test_retrieval_sends_fixed_rerank_candidates_window(monkeypatch, mcp_server, page, page_size):
    """The candidate window is a fixed constant rather than page * page_size, so
    the rerank pool — and therefore the ranking — stays identical on every page
    of a pagination sequence; a growing window could reorder results between
    pages, duplicating or skipping chunks."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    captured = _stub_retrieval(monkeypatch, connector)

    await connector.retrieval(
        api_key="unit-key",
        dataset_ids=["dataset-1"],
        question="unit question",
        page=page,
        page_size=page_size,
    )

    assert captured["path"] == "/retrieval"
    payload = captured["payload"]
    assert payload["rerank_candidates_count"] == mcp_server.RAGFlowConnector._RERANK_CANDIDATES_COUNT
    assert payload["rerank_candidates_count"] >= page * page_size


@pytest.mark.asyncio
async def test_retrieval_rejects_window_beyond_fixed_candidates(monkeypatch, mcp_server):
    """Requests that cannot fit inside the fixed candidate window fail up front
    with an actionable message instead of surfacing the backend's rejection (or
    worse, silently serving drifting pages)."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    captured = _stub_retrieval(monkeypatch, connector)

    window = mcp_server.RAGFlowConnector._RERANK_CANDIDATES_COUNT
    with pytest.raises(Exception, match="exceeds the fixed rerank candidate window"):
        await connector.retrieval(
            api_key="unit-key",
            dataset_ids=["dataset-1"],
            question="unit question",
            page=window // 100 + 1,
            page_size=100,
        )

    assert "payload" not in captured  # rejected before any request was sent
