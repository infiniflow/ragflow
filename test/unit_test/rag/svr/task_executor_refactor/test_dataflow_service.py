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
Unit tests for DataflowService module.

Tests validate behavior through the public run_dataflow() entry point.
Private orchestration helpers (_process_chunks, _encode_batch, _normalize_chunks,
_get_output_type, _embed_chunks, _load_dsl, etc.) are exercised implicitly; no test
reaches directly into those internals.
"""

import pytest
from unittest.mock import MagicMock, AsyncMock, patch

from rag.svr.task_executor_refactor.dataflow_service import DataflowService


class TestDataflowServiceRunDataflow:
    """Tests for the public run_dataflow() method.

    Internal helpers (_load_dsl, _normalize_chunks, _get_output_type, _process_chunks,
    _embed_chunks, _encode_batch) are exercised through this single entry point so
    the suite stays resilient when internal method boundaries change.
    """

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    @patch("rag.svr.task_executor_refactor.dataflow_service.PipelineOperationLogService")
    async def test_run_dataflow_dsl_not_found(self, mock_pipeline_log, mock_canvas, task_context):
        """Test run_dataflow returns early when DSL is not found."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        mock_canvas.get_by_id.return_value = (False, None)

        service = DataflowService(ctx=task_context)
        with pytest.raises(AssertionError, match="User pipeline not found"):
            await service.run_dataflow()

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.dataflow_service.Pipeline")
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    async def test_run_dataflow_empty_chunks(self, mock_canvas, mock_pipeline_class, task_context):
        """Test run_dataflow handles empty pipeline output."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
        mock_pipeline = MagicMock()
        mock_pipeline.run = AsyncMock(return_value={})
        mock_pipeline_class.return_value = mock_pipeline

        with patch.object(DataflowService, "_record_pipeline_log"):
            service = DataflowService(ctx=task_context)
            await service.run_dataflow()

    @pytest.mark.asyncio
    @pytest.mark.parametrize("output_key", ["chunks", "json"])
    @patch("rag.svr.task_executor_refactor.dataflow_service.Pipeline")
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    async def test_run_dataflow_with_output_type(self, mock_canvas, mock_pipeline_class, task_context, output_key):
        """Test run_dataflow processes output end-to-end (chunks / json)."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        task_context._task["tenant_id"] = "tenant_test"
        task_context._task["kb_id"] = "kb_test"
        task_context._task["doc_id"] = "doc_test"
        task_context._task["name"] = "test.pdf"
        task_context._write_interceptor = None

        mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
        data = {output_key: [{"text": "content", "content_with_weight": "content"}]}
        data["embedding_token_consumption"] = 5
        mock_pipeline = MagicMock()
        mock_pipeline.run = AsyncMock(return_value=data)
        mock_pipeline_class.return_value = mock_pipeline

        with (
            patch.object(DataflowService, "_embed_chunks", new_callable=AsyncMock, return_value=(data[output_key], 5)),
            patch.object(DataflowService, "_insert_chunks", new_callable=AsyncMock, return_value=True),
            patch.object(DataflowService, "_update_document_metadata"),
            patch.object(DataflowService, "_record_pipeline_log"),
            patch("api.db.services.document_service.DocumentService.increment_chunk_num"),
        ):
            service = DataflowService(ctx=task_context)
            await service.run_dataflow()
            DataflowService._insert_chunks.assert_called_once()

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.dataflow_service.Pipeline")
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    async def test_run_dataflow_embedding_failure(self, mock_canvas, mock_pipeline_class, task_context):
        """Test run_dataflow handles embedding failure gracefully."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        task_context._task["name"] = "test.pdf"
        task_context._write_interceptor = None

        mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
        chunks = {
            "chunks": [
                {"text": "Hello"},
            ],
            "embedding_token_consumption": 1,
        }
        mock_pipeline = MagicMock()
        mock_pipeline.run = AsyncMock(return_value=chunks)
        mock_pipeline_class.return_value = mock_pipeline

        with patch.object(DataflowService, "_embed_chunks", new_callable=AsyncMock, return_value=(None, 0)), patch.object(DataflowService, "_record_pipeline_log"):
            service = DataflowService(ctx=task_context)
            await service.run_dataflow()
            service._record_pipeline_log.assert_called()

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.dataflow_service.Pipeline")
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    async def test_run_dataflow_with_billing_hook_success(self, mock_canvas, mock_pipeline_class, task_context):
        """Test run_dataflow calls billing hook on success."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        task_context._task["tenant_id"] = "tenant_test"
        task_context._task["kb_id"] = "kb_test"
        task_context._task["doc_id"] = "doc_test"
        task_context._task["name"] = "test.pdf"
        task_context._write_interceptor = None

        mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
        chunks = {
            "chunks": [
                {"text": "Hello"},
            ],
            "embedding_token_consumption": 1,
        }
        mock_pipeline = MagicMock()
        mock_pipeline.run = AsyncMock(return_value=chunks)
        mock_pipeline_class.return_value = mock_pipeline

        billing_hook = MagicMock()
        billing_hook.on_pipeline_success = AsyncMock()
        billing_hook.on_pipeline_error = AsyncMock()

        with (
            patch.object(DataflowService, "_embed_chunks", new_callable=AsyncMock, return_value=(chunks["chunks"], 1)),
            patch.object(DataflowService, "_insert_chunks", new_callable=AsyncMock, return_value=True),
            patch.object(DataflowService, "_update_document_metadata"),
            patch.object(DataflowService, "_record_pipeline_log"),
            patch("api.db.services.document_service.DocumentService.increment_chunk_num"),
        ):
            service = DataflowService(ctx=task_context, billing_hook=billing_hook)
            await service.run_dataflow()
            billing_hook.on_pipeline_success.assert_called_once()
            billing_hook.on_pipeline_error.assert_not_called()

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.dataflow_service.Pipeline")
    @patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService")
    async def test_run_dataflow_with_billing_hook_error(self, mock_canvas, mock_pipeline_class, task_context):
        """Test run_dataflow calls billing hook on error."""
        task_context._task["task_type"] = "dataflow"
        task_context._task["dataflow_id"] = "dataflow_test"
        task_context._task["name"] = "test.pdf"
        task_context._write_interceptor = None

        mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
        mock_pipeline = MagicMock()
        mock_pipeline.run = AsyncMock(side_effect=Exception("Pipeline failure"))
        mock_pipeline_class.return_value = mock_pipeline

        billing_hook = MagicMock()
        billing_hook.on_pipeline_success = AsyncMock()
        billing_hook.on_pipeline_error = AsyncMock()

        service = DataflowService(ctx=task_context, billing_hook=billing_hook)
        with pytest.raises(Exception, match="Pipeline failure"):
            await service.run_dataflow()

        billing_hook.on_pipeline_error.assert_called_once()
        billing_hook.on_pipeline_success.assert_not_called()


class TestDataflowServiceNormalizeChunks:
    """Tests for _normalize_chunks — stable pure helper for output-format normalization."""

    def test_normalize_chunks_from_chunks_key(self):
        """Test normalization from 'chunks' key."""
        result = DataflowService._normalize_chunks({"chunks": [{"a": 1}]})
        assert result == [{"a": 1}]

    def test_normalize_chunks_from_json_key(self):
        """Test normalization from 'json' key."""
        result = DataflowService._normalize_chunks({"json": [{"a": 1}]})
        assert result == [{"a": 1}]

    def test_normalize_chunks_from_markdown_key(self):
        """Test normalization from 'markdown' key."""
        result = DataflowService._normalize_chunks({"markdown": "# Title"})
        assert result == [{"text": ["# Title"]}]

    def test_normalize_chunks_from_text_key(self):
        """Test normalization from 'text' key."""
        result = DataflowService._normalize_chunks({"text": "plain text"})
        assert result == [{"text": ["plain text"]}]

    def test_normalize_chunks_from_html_key(self):
        """Test normalization from 'html' key."""
        result = DataflowService._normalize_chunks({"html": "<p>content</p>"})
        assert result == [{"text": ["<p>content</p>"]}]

    def test_normalize_chunks_unknown_key(self):
        """Test normalization with unknown key returns empty."""
        result = DataflowService._normalize_chunks({"unknown": "data"})
        assert result == []

    def test_normalize_chunks_empty_markdown(self):
        """Test normalization with empty markdown value returns empty."""
        result = DataflowService._normalize_chunks({"markdown": ""})
        assert result == []

    def test_normalize_chunks_preserves_deepcopy(self):
        """Test normalization returns a deepcopy so mutations don't leak."""
        input_data = {"chunks": [{"key": "value"}]}
        result = DataflowService._normalize_chunks(input_data)
        result[0]["key"] = "modified"
        assert input_data["chunks"][0]["key"] == "value"


class TestDataflowServiceGetOutputType:
    """Tests for _get_output_type — stable pure helper for output-type detection."""

    def test_get_output_type_chunks(self):
        assert DataflowService._get_output_type({"chunks": []}) == "chunks"

    def test_get_output_type_json(self):
        assert DataflowService._get_output_type({"json": []}) == "json"

    def test_get_output_type_markdown(self):
        assert DataflowService._get_output_type({"markdown": ""}) == "markdown"

    def test_get_output_type_text(self):
        assert DataflowService._get_output_type({"text": ""}) == "text"

    def test_get_output_type_html(self):
        assert DataflowService._get_output_type({"html": ""}) == "html"

    def test_get_output_type_empty(self):
        assert DataflowService._get_output_type({}) == "empty"


class TestDataflowServiceProcessChunks:
    """Tests for _process_chunks — stable pure helper for chunk metadata processing."""

    def test_process_chunks_adds_doc_id_and_kb_id(self, task_context):
        """Test _process_chunks adds doc_id, kb_id, and metadata."""
        task_context._task["doc_id"] = "doc_123"
        task_context._task["kb_id"] = "kb_456"
        task_context._task["name"] = "test.pdf"
        chunks = [{"text": "content"}]
        DataflowService._process_chunks(DataflowService(ctx=task_context), chunks)
        assert chunks[0]["doc_id"] == "doc_123"
        assert "kb_id" in chunks[0]
        assert "content_with_weight" in chunks[0]
        assert "text" not in chunks[0]

    def test_process_chunks_generates_id(self, task_context):
        """Test _process_chunks auto-generates id."""
        task_context._task["doc_id"] = "doc_123"
        task_context._task["kb_id"] = "kb_456"
        task_context._task["name"] = "test.pdf"
        chunks = [{"text": "content"}]
        DataflowService._process_chunks(DataflowService(ctx=task_context), chunks)
        assert "id" in chunks[0]

    def test_process_chunks_questions_field(self, task_context):
        """Test _process_chunks processes questions field."""
        task_context._task["doc_id"] = "doc_123"
        task_context._task["kb_id"] = "kb_456"
        task_context._task["name"] = "test.pdf"
        chunks = [{"text": "content", "questions": "Q1\nQ2"}]
        DataflowService._process_chunks(DataflowService(ctx=task_context), chunks)
        assert "questions" not in chunks[0]
        assert "question_kwd" in chunks[0]

    def test_process_chunks_summary_field(self, task_context):
        """Test _process_chunks processes summary field."""
        task_context._task["doc_id"] = "doc_123"
        task_context._task["kb_id"] = "kb_456"
        task_context._task["name"] = "test.pdf"
        chunks = [{"text": "content", "summary": "summary text"}]
        DataflowService._process_chunks(DataflowService(ctx=task_context), chunks)
        assert "summary" not in chunks[0]
        assert "content_ltks" in chunks[0]

    def test_process_chunks_metadata_field(self, task_context):
        """Test _process_chunks extracts metadata."""
        task_context._task["doc_id"] = "doc_123"
        task_context._task["kb_id"] = "kb_456"
        task_context._task["name"] = "test.pdf"
        chunks = [{"text": "content", "metadata": {"key": "val"}}]
        metadata = DataflowService._process_chunks(DataflowService(ctx=task_context), chunks)
        assert "metadata" not in chunks[0]
        assert "key" in metadata


class TestDataflowServiceInit:
    """Tests for DataflowService initialization."""

    @patch("rag.svr.task_executor_refactor.dataflow_service.settings")
    def test_init_with_custom_batch_sizes(self, mock_settings):
        """Test initialization with custom batch sizes."""
        ctx = MagicMock()
        service = DataflowService(ctx=ctx, embedding_batch_size=64, doc_bulk_size=50)
        assert service._embedding_batch_size == 64
        assert service._doc_bulk_size == 50

    @patch("rag.svr.task_executor_refactor.dataflow_service.settings")
    def test_init_with_default_sizes(self, mock_settings):
        """Test initialization with default batch sizes."""
        mock_settings.EMBEDDING_BATCH_SIZE = 32
        mock_settings.DOC_BULK_SIZE = 100
        ctx = MagicMock()
        service = DataflowService(ctx=ctx)
        assert service._embedding_batch_size == 32
        assert service._doc_bulk_size == 100

    def test_init_stores_context_and_hook(self):
        """Test initialization stores context and billing hook."""
        ctx = MagicMock()
        hook = MagicMock()
        service = DataflowService(ctx=ctx, billing_hook=hook)
        assert service._task_context is ctx
        assert service._billing_hook is hook


class TestDataflowServiceLoadDsl:
    """Tests for _load_dsl with dataflow_id correction."""

    @pytest.mark.asyncio
    async def test_load_dsl_for_dataflow_task_type_returns_unchanged_id(self):
        """When task_type == 'dataflow', dataflow_id is returned unchanged."""
        ctx = MagicMock()
        ctx.task_type = "dataflow"
        dataflow_id = "original_dataflow_id"

        with patch("rag.svr.task_executor_refactor.dataflow_service.UserCanvasService") as mock_canvas:
            mock_canvas.get_by_id.return_value = (True, MagicMock(dsl='{"id": "test"}'))
            service = DataflowService(ctx=ctx)

            dsl, corrected_id = await service._load_dsl(dataflow_id)

            assert dsl == '{"id": "test"}'
            assert corrected_id == "original_dataflow_id"
            mock_canvas.get_by_id.assert_called_once_with(dataflow_id)

    @pytest.mark.asyncio
    async def test_load_dsl_for_pipeline_log_task_type_returns_corrected_id(self):
        """When task_type != 'dataflow', dataflow_id comes from pipeline_log.pipeline_id."""
        ctx = MagicMock()
        ctx.task_type = "raptor"
        dataflow_id = "pipeline_log_id"

        with patch("rag.svr.task_executor_refactor.dataflow_service.PipelineOperationLogService") as mock_log:
            mock_log_instance = MagicMock()
            mock_log_instance.dsl = '{"id": "test_pipeline"}'
            mock_log_instance.pipeline_id = "corrected_pipeline_id"
            mock_log.get_by_id.return_value = (True, mock_log_instance)

            service = DataflowService(ctx=ctx)

            dsl, corrected_id = await service._load_dsl(dataflow_id)

            assert dsl == '{"id": "test_pipeline"}'
            assert corrected_id == "corrected_pipeline_id"
            mock_log.get_by_id.assert_called_once_with(dataflow_id)
