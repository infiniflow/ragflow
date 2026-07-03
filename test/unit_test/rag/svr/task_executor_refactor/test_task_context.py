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
Unit tests for TaskContext module.
"""

from unittest.mock import MagicMock
from rag.svr.task_executor_refactor.task_context import TaskContext, TaskLimiters, TaskCallbacks


def _make_ctx(task, **kwargs):
    """Helper to create TaskContext with default limiters and callbacks."""
    return TaskContext(
        task=task,
        limiters=kwargs.get("limiters", TaskLimiters()),
        callbacks=kwargs.get("callbacks", TaskCallbacks()),
        write_interceptor=kwargs.get("write_interceptor", None),
    )


class TestTaskContextInit:
    """Tests for TaskContext initialization."""

    def test_init_with_minimal_task(self):
        """Test initialization with minimal task dict."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.id == "task_1"

    def test_init_with_all_parameters(self):
        """Test initialization with all parameters."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        chat_limiter = MagicMock()
        minio_limiter = MagicMock()
        chunk_limiter = MagicMock()
        embed_limiter = MagicMock()
        kg_limiter = MagicMock()
        write_interceptor = MagicMock()
        progress_callback = MagicMock()
        has_canceled_func = MagicMock()

        ctx = TaskContext(
            task=task,
            limiters=TaskLimiters(
                chat=chat_limiter,
                minio=minio_limiter,
                chunk=chunk_limiter,
                embed=embed_limiter,
                kg=kg_limiter,
            ),
            callbacks=TaskCallbacks(
                progress=progress_callback,
                has_canceled=has_canceled_func,
            ),
            write_interceptor=write_interceptor,
        )

        assert ctx.chat_limiter is chat_limiter
        assert ctx.minio_limiter is minio_limiter
        assert ctx.chunk_limiter is chunk_limiter
        assert ctx.embed_limiter is embed_limiter
        assert ctx.kg_limiter is kg_limiter
        assert ctx.write_interceptor is write_interceptor
        assert ctx.callbacks.progress is progress_callback
        assert ctx.has_canceled_func is has_canceled_func

    def test_init_defaults_for_callbacks(self):
        """Test that callbacks default to no-op functions."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        # Should not raise
        ctx.callbacks.progress()
        assert ctx.has_canceled_func("task_1") is False


class TestTaskContextIdentityProperties:
    """Tests for task identity properties."""

    def test_id(self):
        """Test id property."""
        task = {"id": "task_123", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.id == "task_123"

    def test_tenant_id(self):
        """Test tenant_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.tenant_id == "tenant_1"

    def test_kb_id_default(self):
        """Test kb_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.kb_id == ""

    def test_kb_id(self):
        """Test kb_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "kb_id": "kb_1"}
        ctx = _make_ctx(task=task)
        assert ctx.kb_id == "kb_1"

    def test_doc_id_default(self):
        """Test doc_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.doc_id == ""

    def test_doc_id(self):
        """Test doc_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "doc_id": "doc_1"}
        ctx = _make_ctx(task=task)
        assert ctx.doc_id == "doc_1"

    def test_doc_ids_default(self):
        """Test doc_ids property defaults to empty list."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.doc_ids == []

    def test_doc_ids(self):
        """Test doc_ids property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "doc_ids": ["doc_1", "doc_2"]}
        ctx = _make_ctx(task=task)
        assert ctx.doc_ids == ["doc_1", "doc_2"]


class TestTaskContextDocumentMetadataProperties:
    """Tests for document metadata properties."""

    def test_name_default(self):
        """Test name property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.name == ""

    def test_name(self):
        """Test name property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "name": "test.pdf"}
        ctx = _make_ctx(task=task)
        assert ctx.name == "test.pdf"

    def test_location_default(self):
        """Test location property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.location == ""

    def test_size_default(self):
        """Test size property defaults to 0."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.size == 0

    def test_size(self):
        """Test size property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "size": 1024}
        ctx = _make_ctx(task=task)
        assert ctx.size == 1024


class TestTaskContextParserProperties:
    """Tests for parser configuration properties."""

    def test_parser_id_default(self):
        """Test parser_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.parser_id == ""

    def test_parser_id(self):
        """Test parser_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "parser_id": "naive"}
        ctx = _make_ctx(task=task)
        assert ctx.parser_id == "naive"

    def test_parser_config_default(self):
        """Test parser_config property defaults to empty dict."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.parser_config == {}

    def test_parser_config(self):
        """Test parser_config property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "parser_config": {"chunk_size": 512}}
        ctx = _make_ctx(task=task)
        assert ctx.parser_config == {"chunk_size": 512}

    def test_kb_parser_config_default(self):
        """Test kb_parser_config property defaults to empty dict."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.kb_parser_config == {}

    def test_kb_parser_config(self):
        """Test kb_parser_config property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "kb_parser_config": {"language": "en"}}
        ctx = _make_ctx(task=task)
        assert ctx.kb_parser_config == {"language": "en"}


class TestTaskContextLanguageAndModelProperties:
    """Tests for language and model properties."""

    def test_language_default(self):
        """Test language property defaults to 'Chinese'."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.language == "Chinese"

    def test_language(self):
        """Test language property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "language": "zh"}
        ctx = _make_ctx(task=task)
        assert ctx.language == "zh"

    def test_llm_id_default(self):
        """Test llm_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.llm_id == ""

    def test_llm_id(self):
        """Test llm_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "llm_id": "gpt-4"}
        ctx = _make_ctx(task=task)
        assert ctx.llm_id == "gpt-4"

    def test_embd_id_default(self):
        """Test embd_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.embd_id == ""

    def test_embd_id(self):
        """Test embd_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "embd_id": "text-embedding-ada-002"}
        ctx = _make_ctx(task=task)
        assert ctx.embd_id == "text-embedding-ada-002"


class TestTaskContextPageRangeProperties:
    """Tests for page range properties."""

    def test_from_page_default(self):
        """Test from_page property defaults to 0."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.from_page == 0

    def test_from_page(self):
        """Test from_page property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "from_page": 10}
        ctx = _make_ctx(task=task)
        assert ctx.from_page == 10

    def test_to_page_default(self):
        """Test to_page property defaults to -1."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.to_page == -1

    def test_to_page(self):
        """Test to_page property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "to_page": 100}
        ctx = _make_ctx(task=task)
        assert ctx.to_page == 100


class TestTaskContextTaskTypeAndRoutingProperties:
    """Tests for task type and routing properties."""

    def test_task_type_default(self):
        """Test task_type property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.task_type == ""

    def test_task_type(self):
        """Test task_type property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "task_type": "raptor"}
        ctx = _make_ctx(task=task)
        assert ctx.task_type == "raptor"

    def test_dataflow_id_default(self):
        """Test dataflow_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.dataflow_id == ""

    def test_dataflow_id(self):
        """Test dataflow_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "dataflow_id": "flow_1"}
        ctx = _make_ctx(task=task)
        assert ctx.dataflow_id == "flow_1"


class TestTaskContextAdditionalProperties:
    """Tests for additional properties."""

    def test_pagerank_default(self):
        """Test pagerank property defaults to 0."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.pagerank == 0

    def test_pagerank(self):
        """Test pagerank property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "pagerank": 10}
        ctx = _make_ctx(task=task)
        assert ctx.pagerank == 10

    def test_file_default(self):
        """Test file property defaults to None."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.file is None

    def test_file(self):
        """Test file property."""
        file_obj = MagicMock()
        task = {"id": "task_1", "tenant_id": "tenant_1", "file": file_obj}
        ctx = _make_ctx(task=task)
        assert ctx.file is file_obj


class TestTaskContextMemoryProperties:
    """Tests for memory task properties."""

    def test_memory_id_default(self):
        """Test memory_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.memory_id == ""

    def test_memory_id(self):
        """Test memory_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "memory_id": "mem_1"}
        ctx = _make_ctx(task=task)
        assert ctx.memory_id == "mem_1"

    def test_source_id_default(self):
        """Test source_id property defaults to empty string."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.source_id == ""

    def test_source_id(self):
        """Test source_id property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "source_id": "src_1"}
        ctx = _make_ctx(task=task)
        assert ctx.source_id == "src_1"

    def test_message_dict_default(self):
        """Test message_dict property defaults to empty dict."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.message_dict == {}

    def test_message_dict(self):
        """Test message_dict property."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "message_dict": {"key": "value"}}
        ctx = _make_ctx(task=task)
        assert ctx.message_dict == {"key": "value"}


class TestTaskContextRawTask:
    """Tests for raw_task property and get method."""

    def test_raw_task_returns_original_dict(self):
        """Test raw_task returns the original task dict."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "custom_key": "value"}
        ctx = _make_ctx(task=task)
        assert ctx.raw_task is task

    def test_get_existing_key(self):
        """Test get method with existing key."""
        task = {"id": "task_1", "tenant_id": "tenant_1", "custom_key": "value"}
        ctx = _make_ctx(task=task)
        assert ctx.get("custom_key") == "value"

    def test_get_nonexistent_key_returns_none(self):
        """Test get method with nonexistent key returns None."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.get("missing") is None

    def test_get_with_default(self):
        """Test get method with default value."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        assert ctx.get("missing", "default") == "default"


class TestTaskContextProgressCallback:
    """Tests for progress callback functionality."""

    def test_progress_cb_is_set_in_init(self):
        """Test that _progress_cb is set during initialization."""
        task = {"id": "task_1", "tenant_id": "tenant_1"}
        ctx = _make_ctx(task=task)
        # _progress_cb should be set in __init__
        assert hasattr(ctx, "_progress_cb")
        assert ctx._progress_cb is not None
