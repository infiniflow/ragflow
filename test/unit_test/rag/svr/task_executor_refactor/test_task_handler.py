#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

"""
Unit tests for TaskHandler module.

Mock strategy: external boundaries (LLMBundle, model config services, settings)
are mocked so that ``handle()`` and ``_bind_embedding_model`` execute their
real logic.  Heavy orchestration methods (``_run_standard_chunking``,
``_run_raptor``, ``_run_graphrag``) are mocked since they are tested
exhaustively in the integration test suite.

Stable pure helpers (_build_toc) are tested directly.
"""

import pytest
from unittest.mock import MagicMock, AsyncMock, patch

from rag.svr.task_executor_refactor.task_handler import TaskHandler

# Reuse shared helpers from conftest
from test.unit_test.rag.svr.task_executor_refactor.conftest import (
    patch_embedding_binding,
    create_mock_settings,
    make_task_context,
)


class TestTaskHandlerHandleTask:
    """Tests for the public handle_task() entry point."""

    @pytest.mark.asyncio
    async def test_handle_task_calls_handle(self):
        """Test handle_task delegates to handle()."""
        ctx = MagicMock()
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.doc_id = "doc_1"
        ctx.has_canceled_func = MagicMock(return_value=False)
        handler = TaskHandler(ctx=ctx)
        handler.handle = AsyncMock()
        await handler.handle_task()
        handler.handle.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_task_cleanup_on_cancel(self):
        """Test handle_task cleans up docStore when canceled."""
        from common import settings
        mock_doc_store = MagicMock()
        mock_doc_store.index_exist = MagicMock(return_value=True)
        mock_doc_store.delete = MagicMock(return_value=None)
        orig = settings.docStoreConn
        settings.docStoreConn = mock_doc_store
        try:
            ctx = MagicMock()
            ctx.id = "task_1"
            ctx.tenant_id = "tenant_1"
            ctx.kb_id = "kb_1"
            ctx.doc_id = "doc_1"
            ctx.has_canceled_func = MagicMock(return_value=True)
            ctx.recording_context = MagicMock()
            handler = TaskHandler(ctx=ctx)
            handler.handle = AsyncMock(side_effect=Exception("test error"))
            with pytest.raises(Exception, match="test error"):
                await handler.handle_task()
            mock_doc_store.delete.assert_called()
        finally:
            settings.docStoreConn = orig

    @pytest.mark.asyncio
    async def test_handle_task_cleanup_skips_when_index_missing(self):
        """Cancel cleanup should not call delete when the index doesn't exist."""
        from common import settings
        mock_doc_store = MagicMock()
        mock_doc_store.index_exist = MagicMock(return_value=False)
        mock_doc_store.delete = MagicMock()
        orig = settings.docStoreConn
        settings.docStoreConn = mock_doc_store
        try:
            ctx = MagicMock()
            ctx.id = "task_1"
            ctx.tenant_id = "tenant_1"
            ctx.kb_id = "kb_1"
            ctx.doc_id = "doc_1"
            ctx.has_canceled_func = MagicMock(return_value=True)
            ctx.recording_context = MagicMock()
            handler = TaskHandler(ctx=ctx)
            handler.handle = AsyncMock(side_effect=Exception("test error"))
            with pytest.raises(Exception, match="test error"):
                await handler.handle_task()
            mock_doc_store.delete.assert_not_called()
        finally:
            settings.docStoreConn = orig


class TestTaskHandlerHandle:
    """Tests for the public handle() method.

    External boundaries (LLMBundle, model config services, settings) are mocked
    so that ``_bind_embedding_model`` and ``_init_kb`` execute their real logic
    through ``handle()``.  Only the heavy orchestration methods
    (``_run_standard_chunking``, ``_run_raptor``, ``_run_graphrag``) are mocked.
    """

    # ── Context factory: make_task_context from conftest — see import above

    @pytest.mark.asyncio
    async def test_handle_memory_task(self):
        """Test handle returns after dispatching memory task — no further processing."""
        ctx = make_task_context(task_type="memory")
        ctx.raw_task = {"memory_id": "mem_1", "id": "task_1"}

        with patch("rag.svr.task_executor_refactor.task_handler.handle_save_to_memory_task",
                   new_callable=AsyncMock) as mock_handle:

            handler = TaskHandler(ctx=ctx)
            handler._run_standard_chunking = AsyncMock()
            handler._run_dataflow = AsyncMock()
            await handler.handle()

            mock_handle.assert_called_once_with(ctx.raw_task)
            # After memory task, should return immediately — no further routing
            handler._run_standard_chunking.assert_not_called()
            handler._run_dataflow.assert_not_called()

    @pytest.mark.asyncio
    async def test_handle_dataflow_task(self):
        """Test handle dispatches dataflow tasks (after embedding binding + init_kb)."""
        ctx = make_task_context(task_type="dataflow", doc_id="doc_1")

        with patch_embedding_binding(), \
             patch("rag.svr.task_executor_refactor.task_handler.settings", create_mock_settings()), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"):

            handler = TaskHandler(ctx=ctx)
            handler._run_dataflow = AsyncMock()
            await handler.handle()
            handler._run_dataflow.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_canceled_task(self):
        """Test handle returns early when task is canceled."""
        ctx = make_task_context(has_canceled_func=MagicMock(return_value=True))

        handler = TaskHandler(ctx=ctx)
        await handler.handle()
        ctx.progress_cb.assert_called_once_with(-1, msg="Task has been canceled.")

    @pytest.mark.asyncio
    async def test_handle_standard_chunking(self):
        """Test handle routes to standard chunking.

        ``_bind_embedding_model`` and ``_init_kb`` run their real code;
        only the external boundary (LLM API, settings) is mocked.
        """
        ctx = make_task_context()

        with patch_embedding_binding(), \
             patch("rag.svr.task_executor_refactor.task_handler.settings", create_mock_settings()), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"):

            handler = TaskHandler(ctx=ctx)
            handler._run_standard_chunking = AsyncMock()
            await handler.handle()
            handler._run_standard_chunking.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_raptor_task(self):
        """Test handle routes to RAPTOR with real embedding binding."""
        ctx = make_task_context(task_type="raptor")

        with patch_embedding_binding(), \
             patch("rag.svr.task_executor_refactor.task_handler.settings", create_mock_settings()), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"):

            handler = TaskHandler(ctx=ctx)
            handler._run_raptor = AsyncMock()
            await handler.handle()
            handler._run_raptor.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_graphrag_task(self):
        """Test handle routes to GraphRAG with real embedding binding."""
        ctx = make_task_context(task_type="graphrag")

        with patch_embedding_binding(), \
             patch("rag.svr.task_executor_refactor.task_handler.settings", create_mock_settings()), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"):

            handler = TaskHandler(ctx=ctx)
            handler._run_graphrag = AsyncMock()
            await handler.handle()
            handler._run_graphrag.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_embedding_model_failure(self):
        """Test handle returns early when embedding model binding fails.

        ``LLMBundle`` is patched to raise, so ``_bind_embedding_model``
        itself raises — no need to mock the private method.
        """
        ctx = make_task_context()

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_cfg, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_default, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle:

            mock_cfg.return_value = MagicMock()
            mock_default.return_value = MagicMock()
            mock_bundle.side_effect = RuntimeError("embedding service unavailable")

            handler = TaskHandler(ctx=ctx)
            with pytest.raises(RuntimeError, match="embedding service unavailable"):
                await handler.handle()

    @pytest.mark.asyncio
    async def test_handle_storage_binary_none_raises_file_not_found(self):
        """Verify that None binary from storage raises FileNotFoundError."""
        ctx = make_task_context()

        with patch_embedding_binding(), \
             patch("rag.svr.task_executor_refactor.task_handler.settings", create_mock_settings()), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"), \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService.get_storage_address",
                   return_value=("bucket_test", "name_test")), \
             patch.object(TaskHandler, "_get_storage_binary", new_callable=AsyncMock, return_value=None):

            handler = TaskHandler(ctx=ctx)
            # Do NOT mock _run_standard_chunking — we want real code path for the check
            with pytest.raises(FileNotFoundError, match="Can not find file <test.pdf> from minio"):
                await handler.handle()


class TestTaskHandlerBuildToc:
    """Tests for _build_toc — stable pure helper (requires LLM mocking)."""

    def test_build_toc_with_empty_docs(self):
        """Test _build_toc returns None when run_toc_from_text returns empty."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"

        docs = [{"id": "chunk_1", "content_with_weight": "text", "page_num_int": [1], "top_int": [0]}]

        def mock_asyncio_run(coro):
            coro.close()
            return []

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_cfg, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.asyncio.run", side_effect=mock_asyncio_run):

            mock_cfg.return_value = MagicMock()
            mock_msg = MagicMock()
            mock_bundle.return_value.__enter__.return_value = mock_msg

            result = TaskHandler._build_toc(ctx, docs, MagicMock())
            assert result is None

    def test_build_toc_with_results(self):
        """Test _build_toc builds TOC chunk when results exist."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"

        docs = [{"id": "chunk_0", "content_with_weight": "text", "doc_id": "doc_1", "page_num_int": [1], "top_int": [0]}]
        toc_result = [{"chunk_id": "0", "title": "Section 1"}]

        def mock_asyncio_run(coro):
            coro.close()
            return toc_result

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_cfg, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.asyncio.run", side_effect=mock_asyncio_run):

            mock_cfg.return_value = MagicMock()
            mock_msg = MagicMock()
            mock_bundle.return_value.__enter__.return_value = mock_msg

            result = TaskHandler._build_toc(ctx, docs, MagicMock())
            assert result is not None
            assert "toc_kwd" in result
            assert result["toc_kwd"] == "toc"
            assert result["available_int"] == 0


class TestTaskHandlerInit:
    """Tests for TaskHandler initialization."""

    def test_init_stores_context_and_hook(self):
        ctx = MagicMock()
        hook = MagicMock()
        handler = TaskHandler(ctx=ctx, billing_hook=hook)
        assert handler._task_context is ctx
        assert handler._billing_hook is hook

    def test_init_default_hook_none(self):
        ctx = MagicMock()
        handler = TaskHandler(ctx=ctx)
        assert handler._billing_hook is None
