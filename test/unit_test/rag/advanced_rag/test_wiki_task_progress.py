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

"""Task-level Wiki progress must not inherit a nested compiler's completion."""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock

import pytest


@pytest.fixture
def wiki_generator(monkeypatch):
    def stub(name, **attrs):
        module = ModuleType(name)
        module.__dict__.update(attrs)
        monkeypatch.setitem(sys.modules, name, module)
        return module

    async def thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    stub("common", settings=SimpleNamespace())
    stub("common.constants", LLMType=SimpleNamespace(CHAT="chat"))
    stub("common.misc_utils", thread_pool_exec=thread_pool_exec)
    stub("rag.nlp", search=SimpleNamespace())
    stub("rag.advanced_rag.knowlege_compile.structure", LLMCallPool=object)
    stub("rag.advanced_rag.knowlege_compile._common", env_int=lambda name, default, **kwargs: default)
    wiki = stub("rag.advanced_rag.knowlege_compile.wiki", WIKI_MAP_STATE_COMPILE_KWD="map", WIKI_MAP_STATE_META_COMPILE_KWD="meta")
    for name in (
        "_wiki_commit_active_map_state",
        "_wiki_load_active_map_state",
        "_wiki_load_map_extracts_for_state",
        "_wiki_scan_current_chunk_state",
    ):
        setattr(wiki, name, AsyncMock(return_value=[]))
    wiki._wiki_compare_chunk_states = lambda *args: {
        "new_chunk_ids": {"c1", "c2"},
        "changed_chunk_ids": set(),
        "deleted_chunk_ids": set(),
        "unchanged_chunk_ids": set(),
    }
    wiki.wiki_map_from_chunks = AsyncMock()
    stub("rag.svr.task_executor_refactor.task_context", TaskContext=object)
    stub("api.db.services.document_service", DocumentService=SimpleNamespace(get_by_kb_id=lambda **kwargs: ([{"id": "d0"}, {"id": "d1"}], 2)))
    stub("api.db.services.compilation_template_service", CompilationTemplateService=SimpleNamespace(get_saved=lambda *args: {"config": {"mode": "entity"}}))
    stub("api.db.services.llm_service", LLMBundle=lambda *args, **kwargs: object())
    stub("api.db.joint_services.tenant_model_service", resolve_model_config=lambda *args: {})
    stub("rag.advanced_rag.knowlege_compile.wiki_incremental", wiki_compile_incremental=AsyncMock(), _wiki_load_pages_for_graph=AsyncMock(return_value=[]))

    path = Path(__file__).resolve().parents[4] / "rag/svr/task_executor_refactor/dataset_wiki_generator.py"
    spec = importlib.util.spec_from_file_location("wiki_task_progress_under_test", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    docs = [({"id": f"d{i}"}, "template") for i in range(2)]
    monkeypatch.setattr(module, "_wiki_eligible_docs", lambda *args, **kwargs: docs)
    monkeypatch.setattr(module, "_validate_wiki_eligible_docs", lambda *args: {"d0": "llm", "d1": "llm"})
    monkeypatch.setattr(module, "_wiki_existing_map_doc_ids", AsyncMock(return_value=set()))
    monkeypatch.setattr(module, "_wiki_load_mode", AsyncMock(return_value="entity"))
    monkeypatch.setattr(module, "_wiki_load_embedding_fingerprint", AsyncMock(return_value=None))
    monkeypatch.setattr(module, "_wiki_embedding_fingerprint", lambda *args: "embedding")
    monkeypatch.setattr(module, "_wiki_save_mode", AsyncMock())
    monkeypatch.setattr(module, "_wiki_load_map_extracts_for_state", AsyncMock(return_value=[{"_map_version": {"chunk_id": c}} for c in ("c1", "c2")]))
    monkeypatch.setattr(module, "_create_wiki_llm_pool", lambda *args: SimpleNamespace(wrap=lambda model, **kwargs: model))
    return module


@pytest.mark.parametrize("existing", [False, True], ids=["first-build", "incremental"])
@pytest.mark.parametrize("failed", [False, True], ids=["success", "failure"])
def test_nested_completion_does_not_finish_wiki_task(wiki_generator, monkeypatch, existing, failed):
    module = wiki_generator
    monkeypatch.setattr(module, "_wiki_existing_map_doc_ids", AsyncMock(return_value={"d0", "d1"} if existing else set()))
    events = []
    observed = []
    state = {"progress": 0}

    def progress(p=None, msg=""):
        if p is not None:
            state["progress"] = p
        events.append((p, msg))

    ctx = SimpleNamespace(tenant_id="tenant", kb_id="kb", language="English", progress_cb=progress)

    async def load_chunks(*args, **kwargs):
        yield [{"id": "chunk"}]

    async def run():
        completed = asyncio.Event()

        async def map_batch(**kwargs):
            # One worker finishes while a sibling is still running.
            if kwargs["doc_id"] == "d0":
                kwargs["callback"](1.0, "MAP batch complete")
                completed.set()
            else:
                await completed.wait()
                observed.append(state["progress"])
                kwargs["callback"](0.2, "MAP sibling still running")
            observed.append(state["progress"])

        async def compile_pages(**kwargs):
            assert kwargs["incremental"] is existing
            observed.append(state["progress"])
            kwargs["callback"](1.0, "nested compiler complete")
            observed.append(state["progress"])
            return {"errors": ["page failed"] if failed else [], "pages_created": 2}

        monkeypatch.setattr(module, "wiki_map_from_chunks", map_batch)
        compiler = sys.modules["rag.advanced_rag.knowlege_compile.wiki_incremental"]
        monkeypatch.setattr(compiler, "wiki_compile_incremental", compile_pages)

        async def before_persistence(*args, **kwargs):
            observed.append(state["progress"])
            return []

        monkeypatch.setattr(compiler, "_wiki_load_pages_for_graph", before_persistence)
        module._wiki_commit_active_map_state.side_effect = before_persistence
        await module.run_wiki_incremental(ctx, object(), load_chunks)

    asyncio.run(run())
    assert observed and all(0 <= p < 1 for p in observed)
    assert state["progress"] == (-1 if failed else 1)
    assert events[-1][1].startswith("Wiki incomplete:" if failed else "Wiki done:")
    assert [p for p, _ in events[:-1] if p is not None and p >= 1] == []
    assert any(msg == "MAP batch complete" for _, msg in events)
    assert any(msg == "MAP sibling still running" for _, msg in events)
    assert any(msg == "nested compiler complete" for _, msg in events)
    assert module._wiki_commit_active_map_state.await_count == (0 if failed else 1)


@pytest.mark.parametrize("failure_progress", [-1, -0.5])
def test_nested_failure_progress_is_preserved(wiki_generator, monkeypatch, failure_progress):
    module = wiki_generator
    events = []
    ctx = SimpleNamespace(tenant_id="tenant", kb_id="kb", language="English", progress_cb=lambda p=None, msg="": events.append((p, msg)))

    async def load_chunks(*args, **kwargs):
        yield [{"id": "chunk"}]

    async def map_batch(**kwargs):
        kwargs["callback"](failure_progress, "MAP failure")

    async def compile_pages(**kwargs):
        kwargs["callback"](failure_progress, "compiler failure")
        return {"errors": ["failed"]}

    monkeypatch.setattr(module, "wiki_map_from_chunks", map_batch)
    compiler = sys.modules["rag.advanced_rag.knowlege_compile.wiki_incremental"]
    monkeypatch.setattr(compiler, "wiki_compile_incremental", compile_pages)
    asyncio.run(module.run_wiki_incremental(ctx, object(), load_chunks))
    assert (failure_progress, "MAP failure") in events
    assert (failure_progress, "compiler failure") in events
    assert events[-1][0] == -1
    module._wiki_commit_active_map_state.assert_not_awaited()


def test_cancelled_compilation_never_reports_success(wiki_generator, monkeypatch):
    module = wiki_generator
    events = []
    ctx = SimpleNamespace(tenant_id="tenant", kb_id="kb", language="English", progress_cb=lambda p=None, msg="": events.append((p, msg)))

    async def load_chunks(*args, **kwargs):
        yield [{"id": "chunk"}]

    async def compile_pages(**kwargs):
        kwargs["callback"](1.0, "inner phase complete")
        raise asyncio.CancelledError

    compiler = sys.modules["rag.advanced_rag.knowlege_compile.wiki_incremental"]
    monkeypatch.setattr(compiler, "wiki_compile_incremental", compile_pages)
    with pytest.raises(asyncio.CancelledError):
        asyncio.run(module.run_wiki_incremental(ctx, object(), load_chunks))
    assert not any(p is not None and p >= 1 for p, _ in events)
    module._wiki_commit_active_map_state.assert_not_awaited()
