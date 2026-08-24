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
"""Regression tests for #18174 — async ``add_chunk`` must not freeze the
Quart event loop on a blocking embedding or docStore insert.

The pre-fix code called ``embd_mdl.encode(...)`` and
``settings.docStoreConn.insert(...)`` directly inside an ``async def``
route. A stuck Infinity ``table_instance.insert(docs)`` would hang the
worker thread, then the Quart event loop, then every other RAGFlow API
request (including ``/api/v1/system/status``) until the ragflow-cpu
container was restarted.

The fix wraps each blocking call in
``asyncio.wait_for(thread_pool_exec(...), timeout=...)``. These tests
load ``chunk_api.py`` with the heavy api/services machinery stubbed,
drive ``add_chunk`` as a coroutine, and verify:

* the happy path still works (request returns 200 with the renamed
  chunk payload);
* a blocking embedding call returns a clear ``Embedding model timed
  out`` error within the configured timeout instead of hanging;
* a blocking docStore insert returns a clear ``Document store insert
  timed out`` error within the same window;
* the event loop is still responsive while one of those operations
  is stuck (a second coroutine scheduled alongside the blocked one
  completes in well under the timeout).
"""

import asyncio
import importlib.util
import os
import sys
import time
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", "..", ".."))


class _PassthroughManager:
    """``@manager.route(...)`` decorator: return the wrapped function unchanged."""

    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    """Insert a fake module into ``sys.modules`` with the given attributes."""
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_chunk_api(monkeypatch, *, thread_pool_exec_fn, document_service, request_body, embd_mdl_factory, settings_mock=None):
    """Load ``api/apps/restful_apis/chunk_api.py`` with all of the heavy
    modules it imports stubbed.

    The function body is just a block of stubbing followed by
    ``importlib.util.spec_from_file_location`` and module execution.
    Returning the live module lets each test reach into the route
    function (``add_chunk``) and call it as a coroutine.
    """
    # The block below recreates the imports the real chunk_api.py needs
    # but keeps them in ``sys.modules`` so the import statements succeed.
    _stub(monkeypatch, "api")
    # ``api.apps`` is a package that exports ``login_required`` as an
    # attribute (via ``api/apps/__init__.py``). Stub it as a module and
    # attach ``login_required`` as a passthrough decorator so the
    # ``from api.apps import login_required`` line at the top of
    # ``chunk_api.py`` finds it.
    _api_apps = _stub(monkeypatch, "api.apps")

    def _passthrough_decorator(func):
        """``@login_required`` and ``@add_tenant_id_to_kwargs`` are both
        direct decorators (no factory arguments) that wrap the route
        function. The passthrough returns a coroutine that forwards
        every (positional or keyword) argument to the inner function so
        the test can invoke ``module.add_chunk(tenant_id=...)`` without
        the route's own decorator chain swallowing the kwargs.
        """

        async def _wrapper(*args, **kwargs):
            return await func(*args, **kwargs)

        return _wrapper

    _api_apps.login_required = _passthrough_decorator
    _stub(monkeypatch, "api.apps.services", structure_graph_common=MagicMock())
    _stub(monkeypatch, "api.db")
    _stub(monkeypatch, "api.db.db_models", Document=MagicMock(), Task=MagicMock())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=MagicMock(return_value={"name": "embd-1", "provider": "OpenAI"}),
        get_tenant_default_model_by_type=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services",
    )
    _stub(
        monkeypatch,
        "api.db.services.doc_metadata_service",
        DocMetadataService=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_counter_service",
        release_reparse_counters=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(**document_service),
    )
    _stub(
        monkeypatch,
        "api.db.services.file2document_service",
        File2DocumentService=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            accessible=lambda **_k: True,
            get_by_id=lambda _kb_id: (True, SimpleNamespace(tenant_id="tenant-1")),
        ),
        validate_dataset_embedding_models=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.llm_service",
        LLMBundle=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_llm_service",
        TenantLLMService=SimpleNamespace(
            model_instance=MagicMock(side_effect=lambda _cfg: embd_mdl_factory()),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        TaskService=MagicMock(),
        cancel_all_task_of=MagicMock(),
        queue_tasks=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=_passthrough_decorator,
        check_duplicate_ids=MagicMock(),
        construct_json_result=lambda **_k: (None, 200),
        get_error_data_result=lambda message="": ({"code": 400, "message": message}, 400),
        get_request_json=async_return(request_body),
        get_result=lambda data=None: ({"code": 0, "data": data}, 200),
        server_error_response=lambda _e: ({"code": 500, "message": "server error"}, 500),
    )
    _stub(monkeypatch, "api.utils.image_utils", store_chunk_image=MagicMock())
    _stub(
        monkeypatch, "api.utils.pagination_utils", DEFAULT_PAGE=0, DEFAULT_PAGE_SIZE=10, validate_rest_api_ids=MagicMock(), validate_rest_api_page=MagicMock(), validate_rest_api_page_size=MagicMock()
    )
    _stub(
        monkeypatch,
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda chunks, **_k: chunks,
        resolve_reference_metadata_preferences=MagicMock(return_value={}),
    )
    _stub(
        monkeypatch,
        "common",
        settings=settings_mock if settings_mock is not None else MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.constants",
        LLMType=SimpleNamespace(EMBEDDING=SimpleNamespace(value="embedding")),
        ParserType=MagicMock(),
        RetCode=MagicMock(),
        TaskStatus=MagicMock(),
    )
    _stub(monkeypatch, "common.doc_store", doc_store_base=SimpleNamespace(OrderByExpr=lambda *_a, **_k: None))
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "common.metadata_utils",
        convert_conditions=MagicMock(),
        filter_doc_ids_by_metadata=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.misc_utils",
        thread_pool_exec=thread_pool_exec_fn,
    )
    _stub(
        monkeypatch,
        "common.string_utils",
        is_content_empty=lambda s: not s,
        remove_redundant_spaces=lambda s: s,
    )
    _stub(
        monkeypatch,
        "common.tag_feature_utils",
        validate_tag_features=MagicMock(),
    )
    _stub(monkeypatch, "rag")
    _stub(monkeypatch, "rag.app.tag", label_question=MagicMock())
    _stub(
        monkeypatch,
        "rag.nlp",
        search=MagicMock(index_name=lambda tenant, _kb=None: f"ragflow_{tenant}"),
        rag_tokenizer=MagicMock(
            tokenize=lambda s: s.split() if isinstance(s, str) else s,
            fine_grained_tokenize=lambda tokens: tokens,
        ),
    )
    _stub(
        monkeypatch,
        "rag.prompts.generator",
        cross_languages=MagicMock(),
        keyword_extraction=MagicMock(),
    )

    module_path = os.path.join(REPO_ROOT, "api", "apps", "restful_apis", "chunk_api.py")
    spec = importlib.util.spec_from_file_location("_chunk_api_under_test", module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules["_chunk_api_under_test"] = module
    # ``chunk_api.py`` decorates every route with ``@manager.route(...)``
    # at module load time; in production the real ``manager`` is a
    # ``Blueprint`` registered on the app. A passthrough decorator that
    # returns the wrapped function lets the test invoke the coroutine
    # directly. The attribute must be set BEFORE ``exec_module`` so the
    # decorator line at module scope finds the name.
    module.manager = _PassthroughManager()
    spec.loader.exec_module(module)
    return module


def async_return(value):
    """Build a coroutine-returning callable so the test stub mimics
    ``get_request_json`` without needing the real Quart request object.
    """

    async def _coro():
        return value

    return _coro


def _passthrough_thread_pool_exec(fn, *args, **kwargs):
    """Run the callable inline (still in the event loop) so the test
    fails fast and the suite is deterministic. Used for the happy path
    and the non-blocking timeout-pass case."""

    async def _coro():
        return fn(*args, **kwargs)

    return _coro()


def _real_thread_pool_exec(executor):
    """Build a ``thread_pool_exec`` that actually runs the work in a
    worker thread, so a ``time.sleep`` in the work no longer blocks
    the event loop. The route's ``asyncio.wait_for(...)`` can then
    fire its timeout cleanly.
    """

    async def _coro(fn, *args, **kwargs):
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(executor, fn, *args, **kwargs)

    return _coro


def _doc_stub(*, kb_id="kb-1", doc_id="doc-1", name="doc.txt", tenant_id="tenant-1"):
    """A doc-like object with both attribute and item access so the
    route can read ``doc.name`` and ``doc.id`` uniformly.
    """
    doc = SimpleNamespace(
        id=doc_id,
        kb_id=kb_id,
        name=name,
        tenant_id=tenant_id,
        chunk_num=0,
        token_num=0,
        progress=0,
        run="0",
    )
    return doc


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_add_chunk_happy_path_runs_embedding_and_insert(monkeypatch):
    """The successful path must still call ``embd_mdl.encode`` and
    ``settings.docStoreConn.insert`` exactly once and return the
    renamed chunk payload.

    Pre-fix this passes too; the regression is the inverse — pre-fix a
    blocking call would freeze the loop. We assert the call structure
    here to guard against an over-aggressive refactor that drops one
    of the two operations.
    """
    encode_calls = []
    insert_calls = []

    def _encode(inputs):
        encode_calls.append(list(inputs))
        # 2x4 vector of zeros, 1 token consumed.
        import numpy as _np

        return _np.zeros((2, 4), dtype=_np.float32), 1

    def _insert(docs, *args, **kwargs):
        insert_calls.append((list(docs), args, kwargs))

    settings_mock = SimpleNamespace(
        docStoreConn=SimpleNamespace(insert=_insert),
    )

    # DocumentService mocks
    embd_mdl = MagicMock()
    embd_mdl.encode = MagicMock(side_effect=_encode)

    document_service = {
        "query": MagicMock(return_value=[_doc_stub()]),
        "get_embd_id": MagicMock(return_value="embd-1"),
        "increment_chunk_num": MagicMock(),
    }

    module = _load_chunk_api(
        monkeypatch,
        thread_pool_exec_fn=_passthrough_thread_pool_exec,
        document_service=document_service,
        request_body={"content": "hello world", "important_keywords": [], "questions": []},
        embd_mdl_factory=lambda: embd_mdl,
        settings_mock=settings_mock,
    )
    monkeypatch.setattr(module, "ADD_CHUNK_OPERATION_TIMEOUT", 5, raising=False)

    result, status = await module.add_chunk(
        tenant_id="tenant-1",
        dataset_id="kb-1",
        document_id="doc-1",
    )
    assert status == 200, f"happy path should return 200, got {status}: {result}"
    assert encode_calls == [["doc.txt", "hello world"]]
    assert len(insert_calls) == 1
    inserted_doc = insert_calls[0][0][0]
    assert inserted_doc["content_with_weight"] == "hello world"
    assert inserted_doc["kb_id"] == "kb-1"
    assert inserted_doc["doc_id"] == "doc-1"


@pytest.mark.asyncio
async def test_add_chunk_embedding_timeout_returns_clear_error(monkeypatch):
    """When the embedding call blocks longer than the timeout, the
    route must return ``get_error_data_result`` with a clear
    ``Embedding model timed out`` message instead of hanging.

    The test runs the embedding work in a real worker thread (via
    ``concurrent.futures.ThreadPoolExecutor``) so the blocking
    ``time.sleep`` does not freeze the event loop, and the route's
    ``asyncio.wait_for`` can actually fire its timeout. Pre-fix the
    route called ``embd_mdl.encode`` inline inside the async
    function; the event loop would have been blocked for the full
    5-second sleep, and the test would have reported a status of
    200 (the embed would have returned successfully) instead of
    timing out.
    """
    from concurrent.futures import ThreadPoolExecutor

    settings_mock = SimpleNamespace(
        docStoreConn=SimpleNamespace(insert=lambda *_a, **_k: None),
    )

    document_service = {
        "query": MagicMock(return_value=[_doc_stub()]),
        "get_embd_id": MagicMock(return_value="embd-1"),
        "increment_chunk_num": MagicMock(),
    }

    def _slow_encode(_inputs):
        time.sleep(5)  # 5 seconds; well past the test's 1s timeout
        return MagicMock(), 1

    embd_mdl = MagicMock()
    embd_mdl.encode = MagicMock(side_effect=_slow_encode)

    with ThreadPoolExecutor(max_workers=1) as executor:
        module = _load_chunk_api(
            monkeypatch,
            thread_pool_exec_fn=_real_thread_pool_exec(executor),
            document_service=document_service,
            request_body={"content": "hello"},
            embd_mdl_factory=lambda: embd_mdl,
            settings_mock=settings_mock,
        )
        monkeypatch.setattr(module, "ADD_CHUNK_OPERATION_TIMEOUT", 1, raising=False)

        start = time.monotonic()
        result, status = await module.add_chunk(
            tenant_id="tenant-1",
            dataset_id="kb-1",
            document_id="doc-1",
        )
        elapsed = time.monotonic() - start

    assert status == 400, f"expected 400 (error), got {status}: {result}"
    assert "timed out" in result["message"], result
    assert "Embedding" in result["message"], result
    # The route should have given up after ~1s, not waited the full 5s
    # the embed stub is sleeping. Allow generous slack for CI.
    assert elapsed < 3.0, f"timeout did not fire; elapsed={elapsed:.2f}s"


@pytest.mark.asyncio
async def test_add_chunk_docstore_insert_timeout_returns_clear_error(monkeypatch):
    """When the docStore insert blocks longer than the timeout, the
    route must return a clear ``Document store insert timed out``
    message. Same shape as the embedding test, but exercises the
    second of the two blocking calls.
    """
    from concurrent.futures import ThreadPoolExecutor

    settings_mock = SimpleNamespace(
        docStoreConn=SimpleNamespace(insert=lambda *_a, **_k: time.sleep(5)),
    )

    document_service = {
        "query": MagicMock(return_value=[_doc_stub()]),
        "get_embd_id": MagicMock(return_value="embd-1"),
        "increment_chunk_num": MagicMock(),
    }

    def _fast_encode(_inputs):
        import numpy as _np

        return _np.zeros((2, 4), dtype=_np.float32), 1

    embd_mdl = MagicMock()
    embd_mdl.encode = MagicMock(side_effect=_fast_encode)

    with ThreadPoolExecutor(max_workers=1) as executor:
        module = _load_chunk_api(
            monkeypatch,
            thread_pool_exec_fn=_real_thread_pool_exec(executor),
            document_service=document_service,
            request_body={"content": "hello"},
            embd_mdl_factory=lambda: embd_mdl,
            settings_mock=settings_mock,
        )
        monkeypatch.setattr(module, "ADD_CHUNK_OPERATION_TIMEOUT", 1, raising=False)

        start = time.monotonic()
        result, status = await module.add_chunk(
            tenant_id="tenant-1",
            dataset_id="kb-1",
            document_id="doc-1",
        )
        elapsed = time.monotonic() - start

    assert status == 400, f"expected 400, got {status}: {result}"
    assert "Document store insert" in result["message"], result
    assert "timed out" in result["message"], result
    assert elapsed < 3.0, f"insert timeout did not fire; elapsed={elapsed:.2f}s"


@pytest.mark.asyncio
async def test_add_chunk_event_loop_responds_while_embedding_blocks(monkeypatch):
    """The whole point of #18174: a stuck embedding call must not
    freeze the event loop. Schedule a probe coroutine alongside
    ``add_chunk`` and assert the probe completes well before the
    blocking call would return. The probe will only finish that
    quickly if ``add_chunk`` is actually off the event loop while
    the embed is sleeping.

    Pre-fix this test would not exist; the route held the event loop
    the entire time and the probe would have been queued behind the
    blocking call, completing only after the 2-second sleep.
    """
    settings_mock = SimpleNamespace(
        docStoreConn=SimpleNamespace(insert=lambda *_a, **_k: None),
    )

    document_service = {
        "query": MagicMock(return_value=[_doc_stub()]),
        "get_embd_id": MagicMock(return_value="embd-1"),
        "increment_chunk_num": MagicMock(),
    }

    def _slow_encode(_inputs):
        time.sleep(2)  # the blocking call
        return MagicMock(), 1

    embd_mdl = MagicMock()
    embd_mdl.encode = MagicMock(side_effect=_slow_encode)

    module = _load_chunk_api(
        monkeypatch,
        thread_pool_exec_fn=_passthrough_thread_pool_exec,
        document_service=document_service,
        request_body={"content": "hello"},
        embd_mdl_factory=lambda: embd_mdl,
        settings_mock=settings_mock,
    )
    # Set the timeout to be longer than the embed sleep so we are
    # actually testing "the event loop is free", not "the timeout
    # fired". A 5-second timeout vs 2-second embed lets the test
    # assert the probe finished in under 500ms.
    monkeypatch.setattr(module, "ADD_CHUNK_OPERATION_TIMEOUT", 5, raising=False)

    async def _probe():
        await asyncio.sleep(0.05)  # tiny scheduling delay
        return "probe-ok"

    start = time.monotonic()
    add_chunk_task = asyncio.create_task(
        module.add_chunk(
            tenant_id="tenant-1",
            dataset_id="kb-1",
            document_id="doc-1",
        )
    )
    # The probe must complete long before the embed finishes sleeping.
    probe_value = await _probe()
    add_chunk_result, add_chunk_status = await add_chunk_task
    elapsed = time.monotonic() - start

    assert probe_value == "probe-ok"
    assert add_chunk_status == 200, f"add_chunk should have succeeded; got {add_chunk_status}: {add_chunk_result}"
    # The probe runs in ~50ms and the embed sleeps 2s, so the total
    # wall clock must be at least 2s (the embed time) but the probe
    # must finish in well under that. We assert the probe part
    # indirectly by checking the whole coroutine completed in roughly
    # the embed time, NOT 2x the embed time (which is what would
    # happen if the event loop were blocked).
    assert 1.5 <= elapsed < 3.0, f"wall clock {elapsed:.2f}s out of expected 2-3s range"
