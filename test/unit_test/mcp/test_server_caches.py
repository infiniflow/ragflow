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


@pytest.fixture()
def mcp_server():
    return _load_mcp_server()


def _fresh_connector(mcp_server):
    connector = mcp_server.RAGFlowConnector(base_url=mcp_server.BASE_URL)
    connector._dataset_metadata_cache.clear()
    connector._document_metadata_cache.clear()
    return connector


def test_document_metadata_cache_is_bounded_like_the_dataset_cache(mcp_server):
    """Both caches are class-level (shared across connectors, i.e. across tenants in
    host mode); the document cache must apply the same LRU bound as the dataset
    cache or every dataset ever touched keeps its per-document metadata resident
    forever."""
    connector = _fresh_connector(mcp_server)
    limit = mcp_server.RAGFlowConnector._MAX_DOCUMENT_CACHE

    for idx in range(limit + 8):
        connector._set_cached_document_metadata_by_dataset(f"dataset-{idx}", [(f"doc-{idx}-1", {"name": f"doc-{idx}-1"})])

    assert len(connector._document_metadata_cache) == limit
    # LRU eviction order: the first 8 datasets were evicted, the most recent remain.
    assert "dataset-0" not in connector._document_metadata_cache
    assert f"dataset-{limit + 7}" in connector._document_metadata_cache


def test_document_metadata_cache_still_returns_cached_values(mcp_server):
    connector = _fresh_connector(mcp_server)

    connector._set_cached_document_metadata_by_dataset("dataset-1", [("doc-1", {"name": "doc-1"})])

    cached = connector._get_cached_document_metadata_by_dataset("dataset-1")

    assert cached == {"doc-1": {"name": "doc-1"}}
