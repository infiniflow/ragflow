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

    monkeypatch.setattr(connector, "_post", _post)
    return captured


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "page,page_size,expected_candidates",
    [
        (1, 10, 64),  # default window stays at the backend default
        (1, 50, 64),  # recommended page_size, still within the default candidate pool
        (2, 50, 100),  # advertised paging past 64 results
        (7, 10, 70),  # default page_size paged past 64 results
        (1, 100, 100),  # schema maximum
    ],
)
async def test_retrieval_grows_rerank_candidates_with_requested_window(monkeypatch, mcp_server, page, page_size, expected_candidates):
    """The backend defaults rerank_candidates_count to 64 and rejects any request
    where it is smaller than page * page_size, so the MCP tool must send a candidate
    count that covers the requested window — otherwise every page past the first 64
    results fails with an error about a parameter the tool never exposes."""
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
    assert payload["rerank_candidates_count"] == expected_candidates
    assert payload["rerank_candidates_count"] >= page * page_size
