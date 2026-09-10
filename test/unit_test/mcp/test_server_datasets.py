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
import json
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


def _datasets(count, *, with_name=True):
    out = []
    for idx in range(count):
        row = {"id": f"dataset-{idx}", "description": f"description-{idx}"}
        if with_name:
            row["name"] = f"name-{idx}"
        out.append(row)
    return out


@pytest.fixture()
def mcp_server():
    return _load_mcp_server()


def _stub_dataset_pages(monkeypatch, connector, datasets):
    requests = []

    async def _get(path, params=None, api_key=""):
        requests.append({"path": path, "params": dict(params), "api_key": api_key})
        page = params["page"]
        page_size = params["page_size"]
        start = (page - 1) * page_size
        end = start + page_size
        # Mirrors the real REST envelope: api/utils/api_utils.py get_result() puts
        # the dataset total under "total_datasets", not "total".
        return _FakeResponse({"code": 0, "data": datasets[start:end], "total_datasets": len(datasets)})

    monkeypatch.setattr(connector, "_get", _get)
    return requests


@pytest.mark.asyncio
async def test_list_datasets_default_fetches_all_with_rest_page_size_limit(monkeypatch, mcp_server):
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    requests = _stub_dataset_pages(monkeypatch, connector, _datasets(250))

    result = await connector.list_datasets(api_key="unit-key")

    rows = [json.loads(line) for line in result.splitlines()]
    assert [row["id"] for row in rows] == [f"dataset-{idx}" for idx in range(250)]
    assert [request["params"]["page"] for request in requests] == [1, 2, 3]
    assert all(request["path"] == "/datasets" for request in requests)
    assert all(request["params"]["page_size"] == 100 for request in requests)


@pytest.mark.asyncio
async def test_list_datasets_stops_at_total_without_empty_page_request(monkeypatch, mcp_server):
    """When the fetched rows reach the reported total_datasets, the loop must stop
    without issuing the extra empty-page request. Reading the wrong key ("total")
    left this early-exit dead and always cost one more roundtrip per full listing."""
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    requests = _stub_dataset_pages(monkeypatch, connector, _datasets(200))

    result = await connector.list_datasets(api_key="unit-key")

    rows = [json.loads(line) for line in result.splitlines()]
    assert len(rows) == 200
    assert [request["params"]["page"] for request in requests] == [1, 2]


@pytest.mark.asyncio
async def test_resolve_dataset_ids_fetches_all_pages_and_deduplicates(monkeypatch, mcp_server):
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    datasets = _datasets(101) + [{"id": "dataset-100", "description": "duplicate"}]
    requests = _stub_dataset_pages(monkeypatch, connector, datasets)

    result = await connector.resolve_dataset_ids(api_key="unit-key")

    assert result == [f"dataset-{idx}" for idx in range(101)]
    assert [request["params"]["page"] for request in requests] == [1, 2]
    assert all(request["params"]["page_size"] == 100 for request in requests)


@pytest.mark.asyncio
async def test_list_datasets_clamps_explicit_page_size_to_rest_limit_and_preserves_filters(monkeypatch, mcp_server):
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    requests = _stub_dataset_pages(monkeypatch, connector, _datasets(150))

    result = await connector.list_datasets(
        api_key="unit-key",
        page=1,
        page_size=1000,
        orderby="name",
        desc=False,
        id="dataset-1",
        name="target",
    )

    assert len(result.splitlines()) == 100
    assert len(requests) == 1
    assert requests[0]["params"] == {
        "page": 1,
        "page_size": 100,
        "orderby": "name",
        "desc": False,
        "id": "dataset-1",
        "name": "target",
    }


@pytest.mark.asyncio
async def test_list_datasets_includes_name_field(monkeypatch, mcp_server):
    """``list_datasets`` must surface each dataset's ``name`` so MCP clients
    can show a human-readable label next to the opaque ``id``.

    Regression for #17133: pre-fix the per-row dict was
    ``{"description": ..., "id": ...}`` and dropped the ``name`` that
    the REST API returns. The sibling ``list_chats`` already
    includes ``id`` / ``name`` / ``description`` — the datasets
    branch was the inconsistency.
    """
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    _stub_dataset_pages(monkeypatch, connector, _datasets(3))

    result = await connector.list_datasets(api_key="unit-key")

    rows = [json.loads(line) for line in result.splitlines()]
    assert len(rows) == 3
    # Every row has the three fields the REST API returns.
    for row in rows:
        assert set(row.keys()) == {"id", "name", "description"}, f"row {row!r} is missing one of id/name/description"
    # Names round-trip in document order.
    assert [row["name"] for row in rows] == ["name-0", "name-1", "name-2"]
    assert [row["id"] for row in rows] == ["dataset-0", "dataset-1", "dataset-2"]
    assert [row["description"] for row in rows] == [
        "description-0",
        "description-1",
        "description-2",
    ]


@pytest.mark.asyncio
async def test_list_datasets_name_omitted_when_missing(monkeypatch, mcp_server):
    """If the REST API does not include ``name`` for some row (older
    dataset rows, partial records, migration backlog), the per-row
    dict must still serialise with ``name`` set to ``None`` so the
    shape is stable. The client can then show a fallback label.
    """
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    # No ``name`` field on any row — matches a real API response
    # where the field is absent.
    _stub_dataset_pages(monkeypatch, connector, _datasets(2, with_name=False))

    result = await connector.list_datasets(api_key="unit-key")

    rows = [json.loads(line) for line in result.splitlines()]
    assert len(rows) == 2
    for row in rows:
        assert "name" in row
        assert row["name"] is None
        assert set(row.keys()) == {"id", "name", "description"}
