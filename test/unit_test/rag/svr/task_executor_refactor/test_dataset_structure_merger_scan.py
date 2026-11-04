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
"""Regression tests for paged doc-store scans in ``dataset_structure_merger.py``.

Issue: infiniflow/ragflow#19649. With more than ``index.max_result_window``
doc-level rows, the unsorted scans hit ``from + size`` limits on
Elasticsearch, the store error was swallowed as an empty page, and the task
reported success after writing no dataset rows.
"""

import sys
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from rag.svr.task_executor_refactor import dataset_structure_merger as merger

pytestmark = [pytest.mark.p2, pytest.mark.asyncio]

_SERVICE_MODULES = (
    "api.apps.services.dataset_api_service",
    "api.db.services.compilation_template_service",
    "api.db.services.knowledgebase_service",
    "api.db.services.llm_service",
)


def _page(n: int, start: int = 0) -> dict:
    return {f"row{i}": {"id": f"row{i}", "deleted_doc_id": f"doc{i}", "compile_kwd": "c", "compilation_template_ids": ["t"]} for i in range(start, start + n)}


@pytest.fixture
def store():
    conn = MagicMock()
    conn.index_exist.return_value = True
    with patch.object(merger, "settings") as mock_settings, patch.object(merger, "search") as mock_search:
        mock_settings.docStoreConn = conn
        mock_settings.DOC_ENGINE = "elasticsearch"
        mock_search.index_name.return_value = "ragflow_tenant"
        yield mock_settings


def _order_fields(conn) -> list:
    return [call.args[4].fields for call in conn.search.call_args_list]


async def test_index_search_orders_by_id_on_elasticsearch(store):
    store.docStoreConn.get_fields.return_value = {}

    await merger._index_search("tenant", "kb", {}, ["id"], limit=1000, offset=10000)

    assert _order_fields(store.docStoreConn) == [[("id", 0)]]


async def test_index_search_keeps_engine_default_order_elsewhere(store):
    store.DOC_ENGINE = "opensearch"
    store.docStoreConn.get_fields.return_value = {}

    await merger._index_search("tenant", "kb", {}, ["id"])

    assert _order_fields(store.docStoreConn) == [[]]


async def test_index_search_propagates_store_errors(store):
    store.docStoreConn.search.side_effect = RuntimeError("Result window is too large")

    with pytest.raises(RuntimeError, match="Result window"):
        await merger._index_search("tenant", "kb", {}, ["id"], limit=1000, offset=10000)


async def test_collect_structure_pairs_propagates_a_failed_page(store):
    store.docStoreConn.search.side_effect = [MagicMock(), RuntimeError("store down")]
    store.docStoreConn.get_fields.return_value = _page(merger._PAGE_SIZE)

    with pytest.raises(RuntimeError, match="store down"):
        await merger._collect_structure_pairs("tenant", "kb")
    assert _order_fields(store.docStoreConn) == [[("id", 0)], [("id", 0)]]


async def test_cleanup_deleted_docs_propagates_a_failed_marker_page(store):
    store.docStoreConn.search.side_effect = [MagicMock(), RuntimeError("store down")]
    store.docStoreConn.get_fields.return_value = _page(merger._PAGE_SIZE)

    with pytest.raises(RuntimeError, match="store down"):
        await merger._cleanup_deleted_docs("tenant", "kb", "c", "t")
    store.docStoreConn.delete.assert_not_called()
    store.docStoreConn.update.assert_not_called()


async def test_consume_deletion_markers_keeps_markers_when_a_page_fails(store):
    store.docStoreConn.search.side_effect = [MagicMock(), RuntimeError("store down")]
    store.docStoreConn.get_fields.return_value = _page(merger._PAGE_SIZE)

    with pytest.raises(RuntimeError, match="store down"):
        await merger._consume_deletion_markers("tenant", "kb")
    store.docStoreConn.delete.assert_not_called()


async def test_run_structure_merge_fails_the_task_when_a_build_fails():
    ctx = MagicMock(task_type="structure_graph", tenant_id="tenant", kb_id="kb", id="task")
    ctx.has_canceled_func.return_value = False
    services = {name: MagicMock() for name in _SERVICE_MODULES}
    services["api.db.services.knowledgebase_service"].KnowledgebaseService.get_by_id.return_value = (True, MagicMock())
    services["api.db.services.compilation_template_service"].CompilationTemplateService.get_saved.return_value = {"kind": "knowledge_graph"}
    consume = AsyncMock()
    with (
        patch.dict(sys.modules, services),
        patch.object(merger, "_collect_structure_pairs", AsyncMock(return_value={("c", "t")})),
        patch.object(merger, "_disabled_doc_ids", AsyncMock(return_value=set())),
        patch.object(merger, "_do_build", AsyncMock(side_effect=RuntimeError("Result window is too large"))),
        patch.object(merger, "_refresh_index", AsyncMock()),
        patch.object(merger, "_consume_deletion_markers", consume),
    ):
        await merger.run_structure_merge(ctx)

    assert ctx.progress_cb.call_args.args[0] == -1
    consume.assert_not_called()
