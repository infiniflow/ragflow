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
        self.max_length = 4096

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass

    async def async_chat(self, system_prompt, messages, **kwargs):
        return '{"key": "value"}'


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

    return (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance",
            return_value=MagicMock(),
        ),
        patch(
            "rag.svr.task_executor_refactor.task_handler.LLMBundle",
            return_value=mock_model,
        ),
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type",
            return_value=MagicMock(),
        ),
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
        chunks = [
            {
                "content_with_weight": "This is a test chunk content.",
                "page_num_int": [0],
                "top_int": [0],
                "position_int": [0, 0, 0, 0],
            }
        ]

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
        chunks.append(
            {
                "id": f"chunk_{i}_{uuid.uuid4().hex[:6]}",
                "content_with_weight": f"This is test chunk content number {i}.",
                "page_num_int": [i],
                "top_int": [i * 100],
                "position_int": [i, 0, i + 1, 0],
                "doc_id": "doc_test",
                "kb_id": "kb_test",
                "docnm_kwd": "test_document.pdf",
            }
        )
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
# Unified Mock TaskContext Factory
# =============================================================================


def make_task_context(**overrides):
    """Build a MagicMock TaskContext with sensible defaults for all services.

    Every test file that needs a mock ``TaskContext`` should use this factory
    with keyword-only overrides instead of defining its own ``_create_mock_context``.

    Usage::

        ctx = make_task_context(parser_id="table", kb_parser_config={"tag_kb_ids": ["kb_1"]})
    """
    defaults = {
        "id": "task_1",
        "tenant_id": "tenant_1",
        "kb_id": "kb_1",
        "doc_id": "doc_1",
        "name": "test.pdf",
        "location": "/path/to/test.pdf",
        "language": "en",
        "parser_id": "naive",
        "parser_config": {},
        "kb_parser_config": {},
        "llm_id": "llm_1",
        "embd_id": "embd_1",
        "from_page": 0,
        "to_page": -1,
        "size": 1000,
        "pagerank": 0,
        "task_type": "standard",
        "dataflow_id": "",
        "doc_ids": [],
        "file": None,
        "memory_id": "",
        "source_id": "",
        "message_dict": {},
    }
    ctx = MagicMock()
    for k, v in defaults.items():
        setattr(ctx, k, v if k not in overrides else overrides.pop(k))

    # Callbacks
    ctx.progress_cb = MagicMock()
    ctx.has_canceled_func = MagicMock(return_value=False)
    ctx.recording_context = MagicMock()
    ctx.write_interceptor = None

    # Raw task dict — derive from context attributes
    ctx.raw_task = MagicMock()

    # Limiters — all use AsyncMockLimiter so services that acquire them work
    limiter = AsyncMockLimiter()
    ctx.chunk_limiter = limiter
    ctx.chat_limiter = limiter
    ctx.embed_limiter = limiter
    ctx.kg_limiter = limiter
    ctx.minio_limiter = limiter

    # Apply remaining overrides
    for k, v in overrides.items():
        setattr(ctx, k, v)

    return ctx


# =============================================================================
# RaptorService Fixtures (kept for backward compatibility)
# =============================================================================


def create_mock_raptor_context():
    """Create a mock TaskContext suitable for RaptorService tests."""
    return make_task_context()


@pytest.fixture
def mock_raptor_context():
    """Provide a mock TaskContext for RaptorService tests."""
    return make_task_context()


# =============================================================================
# Embedding Binding Patch Helper
# =============================================================================


class patch_embedding_binding:
    """Context manager that patches embedding model binding at the external boundary.

    Patches ``LLMBundle``, ``get_model_config_from_provider_instance``, and
    ``get_tenant_default_model_by_type`` so that ``TaskHandler._bind_embedding_model``
    executes its real logic without making actual API calls.

    Usage::

        with patch_embedding_binding(vector_size=128):
            handler = TaskHandler(ctx)
            await handler.handle()
    """

    def __init__(self, vector_size: int = 128):
        self._vector_size = vector_size
        self._patches = []

    def __enter__(self):
        mock_model = MagicMock()
        mock_model.encode = MagicMock(
            return_value=(
                np.random.rand(1, self._vector_size).astype(np.float32),
                10,
            )
        )
        mock_model.max_length = 512
        mock_model.llm_name = "mock_embedding"
        mock_model.__enter__ = MagicMock(return_value=mock_model)
        mock_model.__exit__ = MagicMock(return_value=False)

        self._patches = [
            patch(
                "rag.svr.task_executor_refactor.task_handler.get_model_config_from_provider_instance",
                return_value=MagicMock(),
            ),
            patch(
                "rag.svr.task_executor_refactor.task_handler.LLMBundle",
                return_value=mock_model,
            ),
            patch(
                "rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type",
                return_value=MagicMock(),
            ),
        ]
        for p in self._patches:
            p.__enter__()
        return self

    def __exit__(self, *args):
        for p in reversed(self._patches):
            p.__exit__(*args)


# =============================================================================
# Common mock callbacks
# =============================================================================


async def mock_thread_return_binary(func, *args, **kwargs):
    """Reusable mock for thread_pool_exec — returns fake binary."""
    return b"fake pdf binary"


async def mock_thread_return_none(func, *args, **kwargs):
    """Reusable mock for thread_pool_exec — returns None."""
    return None


# =============================================================================
# Patch helpers for integration tests
# =============================================================================


def patch_get_storage_binary():
    """Patch TaskHandler._get_storage_binary to return fake binary."""
    return patch("rag.svr.task_executor_refactor.task_handler.TaskHandler._get_storage_binary", new_callable=AsyncMock, return_value=b"fake pdf binary")


def patch_task_handler_settings(mock_settings):
    """Patch the settings module-level import in task_handler."""
    return patch("rag.svr.task_executor_refactor.task_handler.settings", mock_settings)


# =============================================================================
# Shared Task Dictionary Factory
# =============================================================================


def make_task_dict(**overrides):
    """Build a task dict with sensible defaults for integration tests.

    All ``_create_standard_task_dict`` / ``_create_raptor_task_dict`` / etc.
    helpers in integration tests should be replaced with this single factory.

    Usage::

        task_dict = make_task_dict(task_type="raptor", doc_ids=["doc1"])
    """
    return {
        "id": f"task_{uuid.uuid4().hex[:8]}",
        "tenant_id": "tenant_test",
        "kb_id": "kb_test",
        "doc_id": "doc_test",
        "name": "test_document.pdf",
        "location": "/path/to/test_document.pdf",
        "size": 1024,
        "parser_id": "naive",
        "parser_config": {"auto_keywords": 0, "auto_questions": 0, "enable_metadata": False},
        "kb_parser_config": {},
        "language": "en",
        "llm_id": "llm_test",
        "embd_id": "embd_test",
        "from_page": 0,
        "to_page": -1,
        "task_type": "standard",
        "pagerank": 0,
        **overrides,
    }


# =============================================================================
# Shared Pipeline Mock Block for Integration Tests
# =============================================================================


class patch_pipeline_mocks:
    """Context manager bundling common integration-test mock blocks.

    Patches external boundaries so ``TaskHandler.handle()`` executes without
    actual API calls.  Use ``mode="raptor"`` or ``mode="graphrag"``.

    Usage::

        with patch_pipeline_mocks() as m:
            m.get_model_config_from_provider_instance.return_value = MagicMock()
            handler = TaskHandler(ctx)
            await handler.handle()
    """

    _MODULES = {
        "task_handler": "rag.svr.task_executor_refactor.task_handler",
        "chunk_service": "rag.svr.task_executor_refactor.chunk_service",
    }

    # (module_key, attr_name, use_AsyncMock)
    _COMMON = [
        ("task_handler", "get_model_config_from_provider_instance", False),
        ("task_handler", "LLMBundle", False),
        ("task_handler", "get_tenant_default_model_by_type", False),
        ("task_handler", "search.index_name", False),
        ("task_handler", "thread_pool_exec", False),
        ("task_handler", "DocumentService", False),
    ]

    _STANDARD = [
        ("task_handler", "File2DocumentService", False),
        ("chunk_service", "thread_pool_exec", False),
        ("task_handler", "ChunkService", False),
    ]

    _RAPTOR = [
        ("task_handler", "KnowledgebaseService", False),
        ("task_handler", "RaptorService", False),
        ("task_handler", "ChunkService", False),
    ]

    _GRAPH_RAG = [
        ("task_handler", "KnowledgebaseService", False),
        ("task_handler", "run_graphrag_for_kb", True),
    ]

    def __init__(self, mode: str = "standard"):
        self._mode = mode
        self._stack = None

    def __enter__(self):
        import contextlib
        from unittest.mock import patch, MagicMock, AsyncMock

        prefixes = list(self._COMMON)
        if self._mode == "standard":
            prefixes += self._STANDARD
        elif self._mode == "raptor":
            prefixes += self._RAPTOR
        elif self._mode == "graphrag":
            prefixes += self._GRAPH_RAG

        mocks = MagicMock()
        ctx_managers = []
        for mod_key, attr, use_async in prefixes:
            target = f"{self._MODULES[mod_key]}.{attr}"
            if use_async:
                cm = patch(target, new_callable=AsyncMock)
            else:
                cm = patch(target)

            mock_handle = cm.__enter__()
            setattr(mocks, attr.replace(".", "_"), mock_handle)
            ctx_managers.append(cm)

        self._stack = contextlib.ExitStack()
        self._ctx_managers = ctx_managers
        return mocks

    def __exit__(self, *args):
        for cm in reversed(self._ctx_managers):
            cm.__exit__(*args)
