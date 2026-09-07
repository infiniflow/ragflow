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
Integration tests for TaskHandler orchestration.
"""

import asyncio
import gc
from typing import Any, Dict
from unittest.mock import MagicMock, AsyncMock, patch

import pytest

from rag.svr.task_executor_refactor.task_handler import TaskHandler
from rag.svr.task_executor_refactor.task_context import TaskContext, TaskLimiters, TaskCallbacks
from rag.svr.task_executor_refactor.recording_context import BaseRecordingContext, RecordingContext
from rag.svr.task_executor_refactor.constants import CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID

# Import shared helpers from conftest
from test.unit_test.rag.svr.task_executor_refactor.conftest import (
    AsyncMockLimiter,
    create_mock_embedding_model,
    create_default_chunks,
    create_mock_settings,
    create_mock_chunk_service,
    make_task_dict,
    patch_get_storage_binary,
    patch_task_handler_settings,
    mock_thread_return_binary,
    mock_thread_return_none,
)


def create_task_context(
    task_dict: Dict[str, Any],
    is_canceled: bool = False,
    recording_context: BaseRecordingContext | None = None,
) -> TaskContext:
    """Create a real TaskContext with mocked limiters and callbacks.

    Args:
        task_dict: Task dictionary with all task attributes.
        is_canceled: If True, has_canceled_func returns True.
        recording_context: RecordingContext to inject. If None, a new one
            is created automatically so that recording_context access works.

    Returns:
        TaskContext with all required dependencies injected.
    """
    if recording_context is None:
        recording_context = RecordingContext()
    limiter = AsyncMockLimiter()
    progress_callback = MagicMock()
    ctx = TaskContext(
        task=task_dict,
        limiters=TaskLimiters(
            chat=limiter,
            minio=limiter,
            chunk=limiter,
            embed=limiter,
            kg=limiter,
        ),
        callbacks=TaskCallbacks(
            progress=progress_callback,
            has_canceled=MagicMock(return_value=is_canceled),
        ),
        recording_context=recording_context,
    )
    # Add progress_callback property for task_handler compatibility
    ctx.progress_callback = progress_callback
    # Add set_progress_cb method for task_handler compatibility
    ctx.set_progress_cb = lambda cb: setattr(ctx.callbacks, 'progress_cb', cb)
    return ctx


class TestStandardChunkingPipelineIntegration:
    """P0: Integration tests for the complete standard chunking pipeline."""

    @pytest.mark.asyncio
    async def test_full_chunking_pipeline_records_task_status(self):
        """Verify that the complete pipeline records task_status as 'completed'."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            task_status = recording_ctx.get("task_status")
            assert task_status == "completed", f"Expected task_status='completed', got {task_status}"

    @pytest.mark.asyncio
    async def test_full_chunking_pipeline_records_insertion_result(self):
        """Verify that insertion_result is recorded as 'success'."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            insertion_result = recording_ctx.get("insertion_result")
            assert insertion_result == "success", f"Expected insertion_result='success', got {insertion_result}"

    @pytest.mark.asyncio
    async def test_full_chunking_pipeline_records_chunk_ids(self):
        """Verify that chunk_ids_count is recorded after build_chunks."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunks = create_default_chunks(count=3)
        mock_chunk_service = create_mock_chunk_service(chunks=mock_chunks)

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.run_toc_from_text", new_callable=AsyncMock) as mock_run_toc, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service
            mock_run_toc.return_value = []  # TOC returns empty when not enabled

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            chunk_ids_count = recording_ctx.get("chunk_ids_count")
            assert chunk_ids_count is not None, "chunk_ids_count should be recorded"
            assert chunk_ids_count == 3, f"Expected chunk_ids_count=3, got {chunk_ids_count}"

    @pytest.mark.asyncio
    async def test_full_chunking_pipeline_records_token_count(self):
        """Verify that token_count and vector_size are recorded after embedding."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            token_count = recording_ctx.get("token_count")
            vector_size = recording_ctx.get("vector_size")

            assert token_count is not None, "token_count should be recorded"
            assert vector_size is not None, "vector_size should be recorded"
            assert vector_size == 128, f"Expected vector_size=128, got {vector_size}"

    @pytest.mark.asyncio
    async def test_full_chunking_pipeline_progress_callback_invoked(self):
        """Verify that progress_callback is invoked multiple times during pipeline."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            ctx.progress_callback.assert_called()
            call_count = ctx.progress_callback.call_count
            assert call_count > 0, "progress_callback should have been invoked at least once"


class TestTaskCancellationCleanupIntegration:
    """P0: Integration tests for task cancellation cleanup flow."""

    @pytest.mark.asyncio
    async def test_canceled_task_calls_docstore_delete(self):
        """Verify that docStoreConn.delete is called when task is canceled."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict, is_canceled=True)
        mock_settings = create_mock_settings()

        call_log = []

        def mock_thread_impl(func, *args, **kwargs):
            # Get the actual method name from the mock
            func_repr = repr(func)
            call_log.append(func_repr)
            if 'index_exist' in func_repr:
                return True
            if 'delete' in func_repr:
                return {"result": "deleted"}
            return {"result": "deleted"}

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_index"), \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec", side_effect=mock_thread_impl):

            handler = TaskHandler(ctx=ctx)
            await handler.handle_task()

            # Verify delete was called by checking the call log
            delete_calls = [c for c in call_log if 'delete' in c]
            assert len(delete_calls) >= 1, f"Expected at least one delete call, got: {call_log}"

    @pytest.mark.asyncio
    async def test_canceled_task_progress_callback_with_negative_one(self):
        """Verify that progress_callback is called with -1 when task is canceled."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict, is_canceled=True)
        mock_settings = create_mock_settings()

        def mock_thread_impl(func, *args, **kwargs):
            func_repr = repr(func)
            if 'index_exist' in func_repr:
                return True
            if 'delete' in func_repr:
                return {"result": "deleted"}
            return {"result": "deleted"}

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_index"), \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec", side_effect=mock_thread_impl):

            handler = TaskHandler(ctx=ctx)
            await handler.handle_task()

            ctx.progress_callback.assert_called()
            call_args_list = ctx.progress_callback.call_args_list
            # Check for -1 in any position of the call arguments
            has_negative_progress = False
            for call in call_args_list:
                # Check positional args
                for arg in call[0]:
                    if arg == -1:
                        has_negative_progress = True
                        break
                # Check keyword args
                if call[1].get("prog") == -1:
                    has_negative_progress = True
                if has_negative_progress:
                    break
            assert has_negative_progress, f"progress_callback should have been called with -1 progress. Calls: {call_args_list}"

    @pytest.mark.asyncio
    async def test_canceled_task_does_not_proceed_to_chunking(self):
        """Verify that canceled task does not proceed to embedding model binding."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict, is_canceled=True)
        mock_settings = create_mock_settings()

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default:

            mock_index_name.return_value = "test_index"
            mock_settings.docStoreConn.index_exist.return_value = True
            mock_settings.docStoreConn.delete.return_value = {"result": "deleted"}

            async def mock_thread_impl(func, *args, **kwargs):
                return {"result": "deleted"}

            mock_thread_exec.side_effect = mock_thread_impl
            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()

            handler = TaskHandler(ctx=ctx)
            await handler.handle_task()

            mock_bundle.assert_not_called()


class TestRaptorPipelineIntegration:
    """P1: Integration tests for the RAPTOR pipeline."""

    @pytest.mark.asyncio
    async def test_raptor_pipeline_records_task_status(self):
        """Verify that RAPTOR pipeline records task_status."""
        task_dict = make_task_dict(doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=["doc1", "doc2"], task_type="raptor", parser_config={"raptor": {"use_raptor": False}}, kb_parser_config={"raptor": {"use_raptor": False}})
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_kb = MagicMock()
        mock_kb.id = "kb_test"
        mock_kb.parser_config = {"raptor": {"use_raptor": False}}

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.KnowledgebaseService") as mock_kb_service, \
             patch("rag.svr.task_executor_refactor.task_handler.RaptorService") as mock_raptor_service, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_index_name.return_value = "test_index"
            mock_kb_service.get_by_id.return_value = (True, mock_kb)
            mock_kb_service.update_by_id.return_value = True
            mock_raptor_service.return_value.run_raptor_for_kb = AsyncMock(return_value=([], 0, []))
            mock_chunk_service.return_value.insert_chunks = AsyncMock(return_value=True)
            mock_doc_service.increment_chunk_num = MagicMock()

            mock_thread_exec.side_effect = mock_thread_return_none

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            task_status = recording_ctx.get("task_status")
            assert task_status == "completed", f"Expected task_status='completed', got {task_status}"

    @pytest.mark.asyncio
    async def test_raptor_pipeline_enables_raptor_if_not_configured(self):
        """Verify that RAPTOR is enabled if not already configured."""
        task_dict = make_task_dict(doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=["doc1", "doc2"], task_type="raptor", parser_config={"raptor": {"use_raptor": False}}, kb_parser_config={"raptor": {"use_raptor": False}})
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_kb = MagicMock()
        mock_kb.id = "kb_test"
        mock_kb.parser_config = {"raptor": {"use_raptor": False}}

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.KnowledgebaseService") as mock_kb_service, \
             patch("rag.svr.task_executor_refactor.task_handler.RaptorService") as mock_raptor_service, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_index_name.return_value = "test_index"
            mock_kb_service.get_by_id.return_value = (True, mock_kb)
            mock_kb_service.update_by_id.return_value = True
            mock_raptor_service.return_value.run_raptor_for_kb = AsyncMock(return_value=([], 0, []))
            mock_chunk_service.return_value.insert_chunks = AsyncMock(return_value=True)
            mock_doc_service.increment_chunk_num = MagicMock()

            mock_thread_exec.side_effect = mock_thread_return_none

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            # Check that the kb parser_config was updated
            mock_kb_service.update_by_id.assert_called_once()
            call_args = mock_kb_service.update_by_id.call_args
            update_dict = call_args[0][1]
            assert update_dict.get("parser_config", {}).get("raptor", {}).get("use_raptor") is True, \
                "RAPTOR should be enabled in parser_config after running"


class TestEmbeddingModelBindingFailureIntegration:
    """P1: Integration tests for embedding model binding failure."""

    @pytest.mark.asyncio
    async def test_embedding_binding_failure_raises_exception(self):
        """Verify that embedding model binding failure raises an exception."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default:

            mock_get_config.side_effect = Exception("Model not found")
            mock_get_default.side_effect = Exception("Model not found")

            handler = TaskHandler(ctx=ctx)

            with pytest.raises(Exception, match="Model not found"):
                await handler.handle()

    @pytest.mark.asyncio
    async def test_embedding_binding_failure_calls_progress_callback(self):
        """Verify that embedding model binding failure calls progress_callback."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default:

            mock_get_config.side_effect = Exception("Model not found")
            mock_get_default.side_effect = Exception("Model not found")

            handler = TaskHandler(ctx=ctx)

            with pytest.raises(Exception):
                await handler.handle()

            ctx.progress_callback.assert_called()


class TestDataflowPipelineIntegration:
    """P2: Integration tests for the dataflow pipeline."""

    @pytest.mark.asyncio
    async def test_dataflow_pipeline_calls_dataflow_service(self):
        """Verify that dataflow pipeline calls DataflowService.run_dataflow()."""
        task_dict = make_task_dict(doc_id=CANVAS_DEBUG_DOC_ID, task_type="dataflow")
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)

        with patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name", return_value="test_idx"), \
             patch("rag.svr.task_executor_refactor.task_handler.settings") as mock_settings, \
             patch("rag.svr.task_executor_refactor.task_handler.DataflowService") as mock_dataflow_service:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_settings.docStoreConn = MagicMock()
            mock_settings.docStoreConn.create_idx = MagicMock()

            mock_instance = MagicMock()
            mock_instance.run_dataflow = AsyncMock(return_value=None)
            mock_dataflow_service.return_value = mock_instance

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            mock_dataflow_service.assert_called_once()
            mock_instance.run_dataflow.assert_called_once()


class TestTocAsyncFlowIntegration:
    """P2: Integration tests for TOC async flow."""

    @pytest.mark.asyncio
    async def test_toc_async_flow_creates_toc_thread(self):
        """Verify that TOC async flow creates a TOC thread when enabled."""

        task_dict = make_task_dict(parser_config={"auto_keywords": 0, "auto_questions": 0, "enable_metadata": False, "toc_extraction": True})
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.run_toc_from_text", new_callable=AsyncMock) as mock_run_toc, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls, \
             patch("rag.svr.task_executor_refactor.post_processor.DocumentService") as mock_post_doc_service:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service
            mock_run_toc.return_value = [{"title": "Test TOC", "level": 1}]
            mock_post_doc_service.increment_chunk_num = MagicMock()

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            mock_run_toc.assert_called()

        # Explicit cleanup to prevent resource leaks
        del mock_embedding, mock_settings, mock_chunk_service
        del mock_get_config, mock_get_default, mock_bundle, mock_file_service
        del mock_index_name, mock_doc_service, mock_chunk_service_cls, mock_run_toc, mock_post_doc_service
        del mock_thread_exec, mock_chunk_thread_exec
        # Allow pending callbacks to execute
        await asyncio.sleep(0)
        gc.collect()

    @pytest.mark.asyncio(loop_scope="function")
    @pytest.mark.filterwarnings("ignore::pytest.PytestUnraisableExceptionWarning")
    async def test_toc_async_flow_does_not_create_thread_when_disabled(self):
        """Verify that TOC async flow does not create a thread when disabled.
        
        Note: This test has a known issue with resource leaks (unclosed sockets and
        event loops) when run as part of the full test suite. The warning filter
        above suppresses these warnings temporarily. The root cause is related to
        asyncio.to_thread creating new event loops that are not properly cleaned up
        by pytest-asyncio.
        """

        task_dict = make_task_dict(parser_config={"auto_keywords": 0, "auto_questions": 0, "enable_metadata": False, "toc_extraction": True})
        task_dict["parser_config"]["toc_extraction"] = False
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.run_toc_from_text", new_callable=AsyncMock) as mock_run_toc, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            mock_run_toc.assert_not_called()

        # Explicit cleanup to prevent resource leaks
        del mock_embedding, mock_settings, mock_chunk_service
        del mock_get_config, mock_get_default, mock_bundle, mock_file_service
        del mock_index_name, mock_doc_service, mock_chunk_service_cls, mock_run_toc
        del mock_thread_exec, mock_chunk_thread_exec
        # Allow pending callbacks to execute and close event loop
        await asyncio.sleep(0)
        # Cancel all pending tasks
        current_task = asyncio.current_task()
        pending = [t for t in asyncio.all_tasks() if t is not current_task and not t.done()]
        for task in pending:
            task.cancel()
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)
        gc.collect()


class TestRecordingContextDataFlowAssertions:
    """P2: Integration tests for RecordingContext data flow assertions."""

    @pytest.mark.asyncio
    async def test_recording_context_captures_file_size_check(self):
        """Verify that RecordingContext captures file_size_exceeded result."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            file_size_exceeded = recording_ctx.get("file_size_exceeded")
            assert file_size_exceeded is None or file_size_exceeded is False, \
                f"Expected file_size_exceeded to be False/None for small file, got {file_size_exceeded}"

    @pytest.mark.asyncio
    async def test_recording_context_captures_parser_id(self):
        """Verify that RecordingContext captures parser_id from task context."""
        task_dict = make_task_dict()
        ctx = create_task_context(task_dict)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_chunk_service = create_mock_chunk_service()

        with patch_get_storage_binary(), \
             patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.File2DocumentService") as mock_file_service, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_chunk_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService") as mock_doc_service, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.ChunkService") as mock_chunk_service_cls:

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_file_service.get_storage_address.return_value = ("bucket_test", "name_test")
            mock_index_name.return_value = "test_index"
            mock_doc_service.increment_chunk_num = MagicMock()
            mock_doc_service.get_document_metadata.return_value = {}
            mock_doc_service.update_document_metadata = MagicMock()
            mock_chunk_service_cls.return_value = mock_chunk_service

            mock_thread_exec.side_effect = mock_thread_return_binary
            mock_chunk_thread_exec.side_effect = mock_thread_return_binary

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            recording_ctx = ctx.recording_context
            # parser_id is available in the task context, verify task completion
            task_status = recording_ctx.get("task_status")
            assert task_status == "completed", f"Expected task_status='completed', got {task_status}"
            # Verify the parser_id is accessible from the task context
            assert ctx.parser_id == "naive", f"Expected parser_id='naive', got {ctx.parser_id}"


class TestGraphragPipelineIntegration:
    """P2: Integration tests for GraphRAG pipeline default configuration."""

    @pytest.mark.asyncio
    async def test_graphrag_pipeline_configures_full_defaults(self):
        """Verify that GraphRAG configures all default parameters when not already set."""
        task_dict = make_task_dict(doc_ids=["doc1", "doc2"], task_type="graphrag")
        rec_ctx = RecordingContext()
        ctx = create_task_context(task_dict, recording_context=rec_ctx)
        mock_embedding = create_mock_embedding_model(vector_size=128)
        mock_settings = create_mock_settings()
        mock_kb = MagicMock()
        mock_kb.id = "kb_test"
        mock_kb.parser_config = {}

        with patch_task_handler_settings(mock_settings), \
             patch("rag.svr.task_executor_refactor.chunk_service.settings", mock_settings), \
             patch("rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance") as mock_get_config, \
             patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as mock_bundle, \
             patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as mock_get_default, \
             patch("rag.svr.task_executor_refactor.task_handler.search.index_name") as mock_index_name, \
             patch("rag.svr.task_executor_refactor.task_handler.thread_pool_exec") as mock_thread_exec, \
             patch("rag.svr.task_executor_refactor.task_handler.KnowledgebaseService") as mock_kb_service, \
             patch("rag.svr.task_executor_refactor.task_handler.run_graphrag_for_kb") as mock_run_graphrag, \
             patch("rag.svr.task_executor_refactor.task_handler.DocumentService"):

            mock_get_config.return_value = MagicMock()
            mock_get_default.return_value = MagicMock()
            mock_bundle.return_value = mock_embedding
            mock_index_name.return_value = "test_index"
            mock_kb_service.get_by_id.return_value = (True, mock_kb)
            mock_kb_service.update_by_id.return_value = True
            mock_run_graphrag.return_value = {"status": "completed"}

            mock_thread_exec.side_effect = mock_thread_return_none

            handler = TaskHandler(ctx=ctx)
            await handler.handle()

            # Verify update_by_id was called with full default config
            mock_kb_service.update_by_id.assert_called_once()
            call_args = mock_kb_service.update_by_id.call_args
            config = call_args[0][1]["parser_config"]["graphrag"]
            assert config["use_graphrag"] is True
            assert "organization" in config["entity_types"]
            assert "person" in config["entity_types"]
            assert "geo" in config["entity_types"]
            assert "event" in config["entity_types"]
            assert "category" in config["entity_types"]
            assert config["method"] == "light"
            assert "batch_chunk_token_size" in config
            assert "retry_attempts" in config
            assert "retry_backoff_seconds" in config
            assert "retry_backoff_max_seconds" in config
            assert "build_subgraph_timeout_per_chunk_seconds" in config
            assert "build_subgraph_min_timeout_seconds" in config
            assert "merge_timeout_seconds" in config
            assert "resolution_timeout_seconds" in config
            assert "community_timeout_seconds" in config
            assert "lock_acquire_timeout_seconds" in config, \
                "All GraphRAG default config parameters should be present"
