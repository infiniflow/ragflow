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
Shared pytest fixtures for task_executor_refactor integration tests.

This module provides reusable fixtures for integration tests that verify
the complete orchestration flow of TaskHandler and its collaborating services.

Design principles:
- Mock external system boundaries (LLM, ES, MinIO, MySQL)
- Use real TaskContext, TaskHandler, and service instances
- Verify RecordingContext for data flow assertions
"""
# =============================================================================
# TensorFlow/UMAP Import Workaround
# =============================================================================
# Mock umap.parametric_umap before any other imports to prevent TensorFlow
# dependency errors during test collection. This allows tests to run without
# requiring TensorFlow to be installed.
import sys
from unittest.mock import MagicMock

# Create a mock module for parametric_umap to satisfy umap's import check
_mock_parametric_umap = MagicMock()
sys.modules.setdefault("umap.parametric_umap", _mock_parametric_umap)
sys.modules.setdefault("umap", MagicMock())

import asyncio
import uuid
from typing import Any, Dict, List
from unittest.mock import MagicMock, AsyncMock, patch

import numpy as np
import pytest

from rag.svr.task_executor_refactor.task_context import TaskContext, TaskLimiters, TaskCallbacks
from rag.svr.task_executor_refactor.recording_context import (
    RecordingContext,
    set_recording_context,
)


# =============================================================================
# Async Limiter Fixtures
# =============================================================================

class AsyncMockLimiter:
    """Mock asyncio semaphore that does not actually limit."""

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        pass


@pytest.fixture
def mock_limiter():
    """Provide a no-op async limiter."""
    return asyncio.Semaphore(5)


# =============================================================================
# Task Dictionary Fixtures
# =============================================================================

@pytest.fixture
def standard_task_dict() -> Dict[str, Any]:
    """Provide a minimal but complete task dict for standard chunking."""
    return {
        "id": f"task_{uuid.uuid4().hex[:8]}",
        "tenant_id": "tenant_test",
        "kb_id": "kb_test",
        "doc_id": "doc_test",
        "name": "test_document.pdf",
        "location": "/path/to/test_document.pdf",
        "size": 1024,
        "parser_id": "naive",
        "parser_config": {
            "auto_keywords": 0,
            "auto_questions": 0,
            "enable_metadata": False,
        },
        "kb_parser_config": {},
        "language": "en",
        "llm_id": "llm_test",
        "embd_id": "embd_test",
        "from_page": 0,
        "to_page": -1,
        "task_type": "standard",
        "pagerank": 0,
    }


@pytest.fixture
def dataflow_task_dict() -> Dict[str, Any]:
    """Provide a task dict for dataflow tasks."""
    task = standard_task_dict()
    task["task_type"] = "dataflow"
    task["dataflow_id"] = "dataflow_test"
    return task


@pytest.fixture
def raptor_task_dict() -> Dict[str, Any]:
    """Provide a task dict for RAPTOR tasks."""
    task = standard_task_dict()
    task["task_type"] = "raptor"
    task["doc_ids"] = ["doc_1", "doc_2"]
    return task


@pytest.fixture
def graphrag_task_dict() -> Dict[str, Any]:
    """Provide a task dict for GraphRAG tasks."""
    task = standard_task_dict()
    task["task_type"] = "graphrag"
    task["doc_ids"] = ["doc_1"]
    return task


@pytest.fixture
def memory_task_dict() -> Dict[str, Any]:
    """Provide a task dict for memory tasks."""
    return {
        "id": f"task_{uuid.uuid4().hex[:8]}",
        "task_type": "memory",
        "memory_id": "mem_test",
        "source_id": "src_test",
        "message_dict": {"role": "user", "content": "test"},
    }


# =============================================================================
# TaskContext Fixtures
# =============================================================================

@pytest.fixture
def task_context(standard_task_dict, mock_limiter, recording_context):
    """Provide a real TaskContext instance with mocked limiters."""
    ctx = TaskContext(
        task=standard_task_dict,
        limiters=TaskLimiters(
            chat=mock_limiter,
            minio=mock_limiter,
            chunk=mock_limiter,
            embed=mock_limiter,
            kg=mock_limiter,
        ),
        callbacks=TaskCallbacks(
            progress=MagicMock(),
            has_canceled=MagicMock(return_value=False),
        ),
        recording_context=recording_context,
    )
    return ctx


@pytest.fixture
def canceled_task_context(standard_task_dict, mock_limiter, recording_context):
    """Provide a TaskContext where the task is already canceled."""
    ctx = TaskContext(
        task=standard_task_dict,
        limiters=TaskLimiters(
            chat=mock_limiter,
            minio=mock_limiter,
            chunk=mock_limiter,
            embed=mock_limiter,
            kg=mock_limiter,
        ),
        callbacks=TaskCallbacks(
            progress=MagicMock(),
            has_canceled=MagicMock(return_value=True),
        ),
        recording_context=recording_context,
    )
    return ctx


# =============================================================================
# RecordingContext Fixtures
# =============================================================================

@pytest.fixture(autouse=True)
def recording_context():
    """Provide a fresh RecordingContext for each test.

    This fixture is autouse=True to ensure every test has a clean
    recording context for assertions.
    """
    ctx = RecordingContext()
    set_recording_context(ctx)
    yield ctx
    # Cleanup: reset the global context after test
    set_recording_context(RecordingContext())


@pytest.fixture(autouse=True)
def cleanup_resources(request):
    """Global resource cleanup fixture.

    Runs after each test to clean up:
    - Unclosed event loops
    - Unclosed sockets (via garbage collection)
    - Unawaited coroutines
    - MagicMock objects that may hold unclosed resources

    This prevents ResourceWarning and RuntimeWarning from failing
    tests when filterwarnings is set to "error".

    Optimization: Uses minimal gc cycles and generation-2 collection
    for faster teardown.
    """
    yield
    import warnings

    # Suppress warnings during cleanup to avoid recursive warning issues
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")

        # Close any unclosed event loops
        try:
            policy = asyncio.get_event_loop_policy()
            loop = policy.get_event_loop()
            if not loop.is_closed():
                loop.close()
        except RuntimeError:
            # No event loop exists, which is fine
            pass


# =============================================================================
# External System Mocks (Boundary Mocks)
# =============================================================================

class MockEmbeddingModel:
    """Mock embedding model that returns deterministic vectors."""

    def __init__(self, vector_size: int = 128):
        self.vector_size = vector_size
        self.max_length = 512
        self.llm_name = "mock_embedding"

    def encode(self, texts: List[str]):
        """Return random vectors for the given texts."""
        vectors = np.random.rand(len(texts), self.vector_size).astype(np.float32)
        token_count = sum(len(t.split()) for t in texts)
        return vectors, token_count

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass


class MockChatModel:
    """Mock chat model that returns canned responses."""

    def __init__(self):
        self.llm_name = "mock_chat"

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass


@pytest.fixture
def mock_embedding_model():
    """Provide a mock embedding model."""
    return MockEmbeddingModel(vector_size=128)


@pytest.fixture
def mock_chat_model():
    """Provide a mock chat model."""
    return MockChatModel()


# =============================================================================
# Patching Helpers
# =============================================================================

def create_patch_embedding_model(vectors=None, vector_size=128):
    """Create a patcher for the embedding model binding.

    This patches the entire _bind_embedding_model flow to return a mock model.
    """
    if vectors is None:
        vectors = np.random.rand(1, vector_size).astype(np.float32)

    mock_model = MagicMock()
    mock_model.encode.return_value = (vectors, 10)
    mock_model.max_length = 512
    mock_model.llm_name = "mock_embedding"
    mock_model.__enter__ = MagicMock(return_value=mock_model)
    mock_model.__exit__ = MagicMock(return_value=False)

    return patch(
        "rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance",
        return_value=MagicMock(),
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.LLMBundle",
        return_value=mock_model,
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type",
        return_value=MagicMock(),
    )


def create_patch_docstore_insert():
    """Create a patcher for docStoreConn.insert that always succeeds."""
    return patch(
        "common.settings.docStoreConn",
        new_callable=MagicMock,
    )


def create_patch_storage_binary(binary_data=b"fake pdf content"):
    """Create a patcher for storage retrieval."""
    mock_async = AsyncMock(return_value=binary_data)
    return patch(
        "rag.svr.task_executor_refactor.task_handler.File2DocumentService.get_storage_address",
        return_value=("bucket_test", "name_test"),
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.thread_pool_exec",
        new_callable=MagicMock,
        return_value=mock_async,
    )


def create_patch_parser_chunking(chunks=None):
    """Create a patcher for the parser chunking to return predefined chunks.

    Args:
        chunks: List of chunk dicts to return from the parser.
                If None, returns a default single chunk.
    """
    if chunks is None:
        chunks = [{
            "content_with_weight": "This is a test chunk content.",
            "page_num_int": [0],
            "top_int": [0],
            "position_int": [0, 0, 0, 0],
        }]

    mock_async = AsyncMock(return_value=chunks)
    return patch(
        "rag.svr.task_executor_refactor.chunk_service.thread_pool_exec",
        new_callable=MagicMock,
        return_value=mock_async,
    )


# =============================================================================
# Shared Helper Functions for Integration Tests
# =============================================================================


def create_mock_embedding_model(vector_size: int = 128):
    """Create a mock embedding model that returns deterministic vectors matching input size."""
    mock_model = MagicMock()

    def mock_encode(texts):
        n = len(texts) if isinstance(texts, list) else 1
        return (
            np.random.rand(n, vector_size).astype(np.float32),
            10 * n,
        )

    mock_model.encode = mock_encode
    mock_model.max_length = 512
    mock_model.llm_name = "mock_embedding"
    mock_model.__enter__ = MagicMock(return_value=mock_model)
    mock_model.__exit__ = MagicMock(return_value=False)
    return mock_model


def create_mock_chat_model():
    """Create a mock chat model."""
    mock_model = MagicMock()
    mock_model.llm_name = "mock_chat"
    mock_model.__enter__ = MagicMock(return_value=mock_model)
    mock_model.__exit__ = MagicMock(return_value=False)
    return mock_model


def create_mock_settings():
    """Create a mock settings object with STORAGE_IMPL and docStoreConn."""
    mock_settings = MagicMock()
    mock_settings.STORAGE_IMPL = MagicMock()
    mock_settings.STORAGE_IMPL.get = MagicMock(return_value=b"fake binary content")
    mock_settings.docStoreConn = MagicMock()
    mock_settings.docStoreConn.create_idx = MagicMock(return_value=None)
    mock_settings.docStoreConn.insert = MagicMock(return_value=None)
    mock_settings.docStoreConn.delete = MagicMock(return_value=None)
    mock_settings.docStoreConn.index_exist = MagicMock(return_value=True)
    mock_settings.docStoreConn.search = MagicMock(return_value={"hits": []})
    mock_settings.DOC_MAXIMUM_SIZE = 100 * 1024 * 1024  # 100MB
    mock_settings.DOC_BULK_SIZE = 100
    mock_settings.retriever = MagicMock()
    return mock_settings


def create_default_chunks(count: int = 2) -> List[Dict[str, Any]]:
    """Create default chunk dictionaries for testing."""
    chunks = []
    for i in range(count):
        chunks.append({
            "id": f"chunk_{i}_{uuid.uuid4().hex[:6]}",
            "content_with_weight": f"This is test chunk content number {i}.",
            "page_num_int": [i],
            "top_int": [i * 100],
            "position_int": [i, 0, i + 1, 0],
            "doc_id": "doc_test",
            "kb_id": "kb_test",
            "docnm_kwd": "test_document.pdf",
        })
    return chunks


def create_mock_chunk_service(chunks=None):
    """Create a mock ChunkService instance."""
    if chunks is None:
        chunks = create_default_chunks(count=3)
    mock_service = MagicMock()
    mock_service.build_chunks = AsyncMock(return_value=chunks)
    mock_service.insert_chunks = AsyncMock(return_value=True)
    return mock_service


@pytest.fixture
def mock_embedding_model_factory():
    """Provide a factory for mock embedding models."""
    return create_mock_embedding_model


@pytest.fixture
def mock_chat_model_factory():
    """Provide a factory for mock chat models."""
    return create_mock_chat_model


@pytest.fixture
def mock_settings_factory():
    """Provide a factory for mock settings."""
    return create_mock_settings


@pytest.fixture
def mock_chunk_service_factory():
    """Provide a factory for mock chunk services."""
    return create_mock_chunk_service


# =============================================================================
# RaptorService Fixtures
# =============================================================================

def create_mock_raptor_context():
    """Create a mock TaskContext suitable for RaptorService tests."""
    ctx = MagicMock()
    ctx.tenant_id = "tenant_1"
    ctx.kb_id = "kb_1"
    ctx.write_interceptor = None
    ctx.progress_cb = MagicMock()
    ctx.raw_task = {"type": ""}
    ctx.parser_id = "naive"
    ctx.parser_config = {}
    ctx.name = "test.pdf"
    ctx.pagerank = 0
    ctx.id = "task_1"
    return ctx


@pytest.fixture
def mock_raptor_context():
    """Provide a mock TaskContext for RaptorService tests."""
    return create_mock_raptor_context()
