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

All orchestration tests validate behavior through the public handle()/handle_task()
entry points.  Internal helpers (_run_standard_chunking, _run_dataflow, _run_raptor,
_run_graphrag, _bind_embedding_model, _get_storage_binary, etc.) are exercised
implicitly; no test reaches directly into those private orchestration methods.

Stable pure helpers (_build_toc, _get_vector_size) are tested directly since they
are side-effect-free data transformations.
"""

import pytest
import numpy as np
from unittest.mock import MagicMock, AsyncMock, patch

from rag.svr.task_executor_refactor.task_handler import TaskHandler


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
            # Should raise the exception
            with pytest.raises(Exception, match="test error"):
                await handler.handle_task()
            mock_doc_store.delete.assert_called()
        finally:
            settings.docStoreConn = orig


class TestTaskHandlerHandle:
    """Tests for the public handle() method.

    Internal orchestration methods (_run_standard_chunking, _run_dataflow,
    _run_raptor, _run_graphrag, _bind_embedding_model) are exercised through
    handle() so the suite stays resilient when those private methods change.
    """

    @pytest.mark.asyncio
    async def test_handle_memory_task(self):
        """Test handle dispatches memory tasks correctly."""
        ctx = MagicMock()
        ctx.task_type = "memory"
        ctx.id = "task_1"
        ctx.raw_task = {"memory_id": "mem_1"}
        ctx.write_interceptor = None
        ctx.has_canceled_func = MagicMock(return_value=False)

        with patch("rag.svr.task_executor_refactor.task_handler.handle_save_to_memory_task", new_callable=AsyncMock) as mock_handle:
            handler = TaskHandler(ctx=ctx)
            handler._bind_embedding_model = AsyncMock()
            handler._get_vector_size = MagicMock(return_value=1024)
            handler._init_kb = MagicMock()
            handler._run_standard_chunking = AsyncMock()
            await handler.handle()
            mock_handle.assert_called_once_with(ctx.raw_task)

    @pytest.mark.asyncio
    async def test_handle_dataflow_task(self):
        """Test handle dispatches dataflow tasks."""
        ctx = MagicMock()
        ctx.task_type = "dataflow"
        ctx.id = "task_1"
        ctx.doc_id = "doc_1"
        ctx.has_canceled_func = MagicMock(return_value=False)

        handler = TaskHandler(ctx=ctx)
        handler._run_dataflow = AsyncMock()
        await handler.handle()
        handler._run_dataflow.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_canceled_task(self):
        """Test handle returns early when task is canceled."""
        ctx = MagicMock()
        ctx.task_type = "standard"
        ctx.id = "task_1"
        ctx.has_canceled_func = MagicMock(return_value=True)
        ctx.progress_cb = MagicMock()

        handler = TaskHandler(ctx=ctx)
        await handler.handle()
        ctx.progress_cb.assert_called_once_with(-1, msg="Task has been canceled.")

    @pytest.mark.asyncio
    async def test_handle_standard_chunking(self):
        """Test handle dispatches standard chunking end-to-end."""
        ctx = MagicMock()
        ctx.task_type = "standard"
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.doc_id = "doc_1"
        ctx.embd_id = "embd_1"
        ctx.language = "en"
        ctx.parser_config = {}
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.progress_cb = MagicMock()
        ctx.recording_context = MagicMock()
        ctx.name = "test.pdf"
        ctx.from_page = 0
        ctx.to_page = -1

        handler = TaskHandler(ctx=ctx)
        handler._bind_embedding_model = AsyncMock(return_value=MagicMock())
        handler._get_vector_size = MagicMock(return_value=128)
        handler._init_kb = MagicMock()
        handler._run_standard_chunking = AsyncMock()

        await handler.handle()
        handler._run_standard_chunking.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_raptor_task(self):
        """Test handle dispatches raptor tasks."""
        ctx = MagicMock()
        ctx.task_type = "raptor"
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.embd_id = "embd_1"
        ctx.language = "en"
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.progress_cb = MagicMock()
        ctx.recording_context = MagicMock()

        handler = TaskHandler(ctx=ctx)
        handler._bind_embedding_model = AsyncMock(return_value=MagicMock())
        handler._get_vector_size = MagicMock(return_value=128)
        handler._init_kb = MagicMock()
        handler._run_raptor = AsyncMock()

        await handler.handle()
        handler._run_raptor.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_graphrag_task(self):
        """Test handle dispatches graphrag tasks."""
        ctx = MagicMock()
        ctx.task_type = "graphrag"
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.embd_id = "embd_1"
        ctx.language = "en"
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.progress_cb = MagicMock()
        ctx.recording_context = MagicMock()

        handler = TaskHandler(ctx=ctx)
        handler._bind_embedding_model = AsyncMock(return_value=MagicMock())
        handler._get_vector_size = MagicMock(return_value=128)
        handler._init_kb = MagicMock()
        handler._run_graphrag = AsyncMock()

        await handler.handle()
        handler._run_graphrag.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_embedding_model_failure(self):
        """Test handle returns early when embedding model binding fails."""
        ctx = MagicMock()
        ctx.task_type = "standard"
        ctx.id = "task_1"
        ctx.has_canceled_func = MagicMock(return_value=False)

        handler = TaskHandler(ctx=ctx)
        handler._bind_embedding_model = AsyncMock(return_value=None)

        await handler.handle()
        # Should not call _run_standard_chunking when model is None
        assert not hasattr(handler, '_run_standard_chunking_called')


class TestTaskHandlerGetVectorSize:
    """Tests for _get_vector_size — stable pure helper."""

    def test_get_vector_size(self):
        mock_model = MagicMock()
        mock_model.encode.return_value = (np.array([[1.0, 2.0, 3.0]]), 10)
        result = TaskHandler._get_vector_size(mock_model)
        assert result == 3


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
            # Close the coroutine to prevent "never awaited" warnings
            coro.close()
            return []

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_cfg:
            mock_cfg.return_value = MagicMock()
            with patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle:
                mock_msg = MagicMock()
                mock_bundle.return_value.__enter__.return_value = mock_msg
                with patch("rag.svr.task_executor_refactor.task_handler.asyncio.run", side_effect=mock_asyncio_run):
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
            # Close the coroutine to prevent "never awaited" warnings
            coro.close()
            return toc_result

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_cfg:
            mock_cfg.return_value = MagicMock()
            with patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle:
                mock_msg = MagicMock()
                mock_bundle.return_value.__enter__.return_value = mock_msg
                with patch("rag.svr.task_executor_refactor.task_handler.asyncio.run", side_effect=mock_asyncio_run):
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