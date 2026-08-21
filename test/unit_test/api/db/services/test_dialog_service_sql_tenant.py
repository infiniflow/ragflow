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
"""Regression tests for issue #18514.

When a chat references a team-shared Table dataset owned by another tenant,
``async_chat`` runs SQL retrieval against the chat tenant's
``ragflow_<chat_tenant>`` index instead of the dataset owner's
``ragflow_<kb_owner_tenant>`` index. Chunks are stored in the owner's index
-- the chat tenant's index may not even exist -- so SQL retrieval returns
empty results for a chat that has data.

The fix derives the SQL target index from ``kbs[0].tenant_id`` (parallel to
the vector retrieval path's ``tenant_ids = list(set([kb.tenant_id for kb in
kbs]))``), not from ``dialog.tenant_id``.

These tests drive the real ``dialog_service.async_chat`` (with heavy
dependencies stubbed) and assert that the ``use_sql`` call receives the KB
owner's tenant_id, not the dialog's tenant_id.
"""

import asyncio
import ast
import sys
import types
import warnings
from types import SimpleNamespace

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.services import dialog_service  # noqa: E402


# ---------------------------------------------------------------------------
# Stubs
# ---------------------------------------------------------------------------


class _StubChatModel:
    def __init__(self):
        self.calls = []

    def bind_tools(self, *args, **kwargs):
        return None


def _build_kb(tenant_id, embd_id="embd-1@OpenAI"):
    return SimpleNamespace(
        id="kb-1",
        embd_id=embd_id,
        tenant_embd_id=embd_id,
        tenant_id=tenant_id,
        chunk_num=1,
        name="Test KB",
        parser_id="general",
    )


def _build_dialog(tenant_id, kb_ids, prompt_config=None):
    return SimpleNamespace(
        tenant_id=tenant_id,
        kb_ids=kb_ids,
        prompt_config=prompt_config or {"system": "", "parameters": [], "quote": True},
        meta_data_filter={},
        llm_id="llm-1@OpenAI",
        tenant_llm_id=None,
        llm_setting=None,
        name="Test Dialog",
        language="English",
        prompt_type="simple",
    )


def _collect(async_gen):
    async def _run():
        return [ev async for ev in async_gen]

    return asyncio.run(_run())


def _patch_async_chat_dependencies(monkeypatch, *, kbs, field_map, use_sql_capture):
    """Apply the standard set of stubs for driving ``async_chat`` up to the
    ``use_sql`` call. Returns the chat_mdl stub so individual tests can
    customise it.
    """
    chat_mdl = _StubChatModel()

    # get_models is stubbed wholesale so the test does not need to satisfy
    # the dialog's full attribute surface (rerank_id, tenant_rerank_id,
    # tts_id, ...). The unpacking on the call site still happens, so the
    # rest of async_chat sees the (kbs, ...) values we return.
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda *_a, **_k: (list(kbs), None, None, chat_mdl, None),
    )

    # field_map triggers the SQL path on a non-empty value
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService,
        "get_field_map",
        lambda _ids: dict(field_map),
    )

    # Model config / LLMBundle plumbing (the chat_mdl itself is unused in
    # the SQL path; only the tenant_id argument to use_sql is asserted).
    monkeypatch.setattr(
        dialog_service,
        "resolve_model_config",
        lambda _tid, _type, _name: {"llm_factory": "OpenAI", "max_tokens": 8192, "model_type": "chat"},
    )
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_by_id",
        lambda _tid, _type, _name: {"llm_factory": "OpenAI", "max_tokens": 8192, "model_type": "chat"},
    )
    monkeypatch.setattr(
        dialog_service,
        "get_tenant_default_model_by_type",
        lambda _tid, _type: {"llm_factory": "OpenAI", "max_tokens": 8192, "model_type": "chat"},
    )
    # resolve_model_type is called BEFORE get_models in async_chat (line ~608)
    # to decide chat vs vision; return a value that takes the chat branch.
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda *_a, **_k: ["chat"])
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda *_a, **_k: chat_mdl)
    monkeypatch.setattr(dialog_service, "validate_dataset_embedding_models", lambda _kbs: None)

    # Langfuse / langfuse stub: no langfuse keys => skip the tracer.
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", staticmethod(lambda tenant_id: None))

    # Meta filter is empty by default; apply_meta_data_filter is not called.
    monkeypatch.setattr(dialog_service.DocMetadataService, "get_flatted_meta_by_kbs", staticmethod(lambda _ids: {}))

    # Capturing use_sql
    async def _use_sql(*args, **kwargs):
        use_sql_capture.append({"args": args, "kwargs": kwargs})
        return {
            "answer": "ok",
            "reference": {"chunks": [], "doc_aggs": []},
            "prompt": "",
        }

    monkeypatch.setattr(dialog_service, "use_sql", _use_sql)
    return chat_mdl


def test_async_chat_sql_uses_kb_owner_tenant_not_chat_tenant(monkeypatch):
    """A chat owned by tenant B references a KB owned by tenant A. The
    ``use_sql`` call must receive tenant A's id, not tenant B's.
    """
    kb_owner_tenant = "tenant-A-uuid"
    chat_tenant = "tenant-B-uuid"
    kbs = [_build_kb(tenant_id=kb_owner_tenant)]
    use_sql_calls = []

    _patch_async_chat_dependencies(
        monkeypatch,
        kbs=kbs,
        field_map={"col_name": "col_name_tks"},
        use_sql_capture=use_sql_calls,
    )

    dialog = _build_dialog(tenant_id=chat_tenant, kb_ids=["kb-1"])
    messages = [{"role": "user", "content": "how many rows?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=False))

    assert len(use_sql_calls) == 1, f"expected 1 use_sql call, got {len(use_sql_calls)}"
    args = use_sql_calls[0]["args"]
    # Signature: use_sql(question, field_map, tenant_id, chat_mdl, quota, kb_ids, doc_ids)
    assert args[2] == kb_owner_tenant, f"SQL retrieval must target the KB owner tenant ({kb_owner_tenant!r}); got {args[2]!r} (chat tenant was {chat_tenant!r})"
    # Sanity: question and kb_ids still forwarded correctly
    assert args[0] == "how many rows?"
    assert args[5] == ["kb-1"]
    # The function exits after a successful use_sql, so at least one event
    # is yielded (the SQL answer).
    assert events, "async_chat should yield the use_sql result"


def test_async_chat_sql_uses_single_kb_tenant_in_multi_kb_dialog(monkeypatch):
    """When the dialog references a single KB, the SQL target is that KB's
    tenant -- the multi-KB tenant_ids branch of the vector retrieval path
    is paralleled by kbs[0].tenant_id here.
    """
    kbs = [_build_kb(tenant_id="kb-owner")]
    use_sql_calls = []

    _patch_async_chat_dependencies(
        monkeypatch,
        kbs=kbs,
        field_map={"col": "col_tks"},
        use_sql_capture=use_sql_calls,
    )

    dialog = _build_dialog(tenant_id="chat-owner", kb_ids=["kb-1"])
    messages = [{"role": "user", "content": "q"}]

    _collect(dialog_service.async_chat(dialog, messages, stream=False))

    assert use_sql_calls[0]["args"][2] == "kb-owner"


def test_async_chat_sql_uses_dialog_tenant_when_no_kbs_loaded(monkeypatch):
    """Defensive: if ``kbs`` is empty (an edge case where ``get_models``
    loaded zero KBs but ``field_map`` was non-empty), fall back to
    ``dialog.tenant_id`` -- the pre-fix behaviour, preserved as the
    last-resort index name.
    """
    use_sql_calls = []

    _patch_async_chat_dependencies(
        monkeypatch,
        kbs=[],
        field_map={"col": "col_tks"},
        use_sql_capture=use_sql_calls,
    )

    dialog = _build_dialog(tenant_id="chat-tenant", kb_ids=["kb-1"])
    messages = [{"role": "user", "content": "q"}]

    _collect(dialog_service.async_chat(dialog, messages, stream=False))

    assert use_sql_calls[0]["args"][2] == "chat-tenant"


def test_async_chat_does_not_call_use_sql_when_field_map_is_empty(monkeypatch):
    """Sanity: the SQL path is gated on a non-empty ``field_map``. The
    pre-fix code had the same gate; this test pins that the gate is
    preserved by the fix. (The actual non-empty path is covered by the
    three tests above; this just guards against the gate being moved by
    accident.)
    """
    # Patch get_field_map to return empty -- the gate at ``if field_map:``
    # must skip the SQL path.
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService,
        "get_field_map",
        lambda _ids: {},
    )

    # Pin the gate's contract directly via the AST: the async_chat function
    # must read ``if field_map:`` before the use_sql call. The full
    # async_chat flow when field_map is empty drives the vector retrieval
    # path, which has a different and unrelated setup.
    src_path = dialog_service.__file__
    with open(src_path) as f:
        tree = ast.parse(f.read())
    for node in ast.walk(tree):
        if isinstance(node, ast.AsyncFunctionDef) and node.name == "async_chat":
            for stmt in ast.walk(node):
                if isinstance(stmt, ast.If):
                    test_src = ast.unparse(stmt.test)
                    if test_src.strip() == "field_map":
                        for sub in ast.walk(stmt):
                            if isinstance(sub, ast.Call):
                                func = sub.func
                                if isinstance(func, ast.Name) and func.id == "use_sql":
                                    return
                        pytest.fail("``if field_map:`` branch must contain a use_sql call")
            pytest.fail("``if field_map:`` gate not found in async_chat")
    pytest.fail("async_chat function not found in dialog_service")
