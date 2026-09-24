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
"""Regression tests for `delete_session_message` (issue #19430).

The route handles ``DELETE /chats/<chat_id>/sessions/<session_id>/messages/<msg_id>``.
The previous implementation computed the matching reference index as
``(i - 1) // 2`` — which assumes the session has an assistant prologue at
message index 0. In a session without one, deleting the first QA pair
resolved to ``ref_index = -1`` and removed the LAST reference instead of
the first, silently corrupting the session's citations and shifting every
later pair by one.

These tests pin:

1. Sessions without a prologue delete the correct reference (index 0).
2. Sessions with a prologue still work (no regression).
3. An unpaired ``msg_id`` (no assistant follow-up) returns a data error
   instead of raising AssertionError → 500.
4. Sessions with no references don't raise IndexError.
"""

import copy
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None and not hasattr(parent_mod, child_name):
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


class _FakeAwaitable:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _FakeConv:
    """Quacks like a ConversationService row enough for delete_session_message."""

    def __init__(self, conv):
        self._conv = conv

    @property
    def dialog_id(self):
        return self._conv["dialog_id"]

    def to_dict(self):
        return self._conv


def _build_session(*, with_prologue: bool, n_pairs: int = 2):
    """Build a minimal session dict with paired user/assistant messages.

    ``with_prologue=True`` puts a leading assistant message (the prompt
    prologue) at index 0 — pair indices 0..n_pairs-1 then map to message
    indices 1..2n_pairs. ``with_prologue=False`` skips the prologue, so
    pair 0 is at message indices 0..1 instead.
    """
    messages = []
    if with_prologue:
        messages.append({"role": "assistant", "content": "Hi, how can I help?", "id": "prologue"})
    pair_id = 0
    for pair_id in range(n_pairs):
        messages.append({"role": "user", "content": f"q{pair_id}", "id": f"u{pair_id}"})
        messages.append({"role": "assistant", "content": f"a{pair_id}", "id": f"u{pair_id}"})
    references = [{"doc_id": f"d{i}", "content": f"ref{i}"} for i in range(n_pairs)]
    return {"id": "s-1", "dialog_id": "c-1", "name": "test", "message": messages, "reference": references}


def _load_chat_api(monkeypatch, session_conv):
    """Load chat_api.py with transitive imports stubbed, expose the
    route handler and ``ConversationService.update_by_id`` capture."""

    update_calls = []

    def _update_by_id(_conv_id, conv):
        update_calls.append({"id": _conv_id, "conv": conv})
        return True

    def _get_by_id(session_id):
        if session_id != session_conv["id"]:
            return False, None
        return True, _FakeConv(session_conv)

    _stub(
        monkeypatch,
        "api.apps",
        current_user=SimpleNamespace(id="tenant-1", is_superuser=True),
        login_required=lambda func=None, **_kwargs: (lambda f: f) if func is None else func,
        __path__=[],
    )
    _stub(monkeypatch, "api.apps.restful_apis", __path__=[])
    _stub(
        monkeypatch,
        "api.apps.restful_apis._generation_params",
        merge_generation_config=lambda *a, **k: None,
        pop_generation_config=lambda *a, **k: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.conversation_service",
        ConversationService=SimpleNamespace(
            get_by_id=_get_by_id,
            update_by_id=_update_by_id,
        ),
        structure_answer=lambda *a, **k: "",
    )
    _stub(
        monkeypatch,
        "api.db.services.dialog_service",
        DialogService=SimpleNamespace(
            query=lambda **_k: True,
            model=SimpleNamespace(_meta=SimpleNamespace(fields=[])),
        ),
        gen_mindmap=lambda *a, **k: "",
        rag_agent=lambda *a, **k: "",
    )
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace(), validate_dataset_embedding_models=lambda kbs: None)
    _stub(
        monkeypatch,
        "common.constants",
        RetCode=SimpleNamespace(
            AUTHENTICATION_ERROR=401,
            ARGUMENT_ERROR=400,
            DATA_ERROR=500,
            NOT_FOUND=404,
            SERVER_ERROR=500,
        ),
        StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="valid")),
        LLMType=SimpleNamespace(),
    )
    _stub(monkeypatch, "common.settings", docStoreConn=SimpleNamespace(), retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "rag.prompts", __path__=[])
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda *a, **k: "")
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *a, **k: "")
    _stub(
        monkeypatch,
        "common.misc_utils",
        get_uuid=lambda: "uuid",
        hash_str2int=lambda *a, **k: 0,
        thread_pool_exec=lambda func, *a, **kw: _FakeAwaitable(func(*a, **kw)),
        thread_pool_exec_long_time=lambda *a, **k: _FakeAwaitable(None),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        check_duplicate_ids=lambda *a, **k: None,
        get_data_error_result=lambda message="", code=500: {"code": code, "message": message, "data": False},
        get_json_result=lambda data=False, message="", code=0: {"code": code, "message": message, "data": data},
        get_request_json=lambda: _FakeAwaitable({}),
        server_error_response=lambda ex: {"code": 500, "message": str(ex), "data": False},
        validate_request=lambda *_a, **_k: lambda f: f,
    )
    _stub(
        monkeypatch,
        "api.utils.pagination_utils",
        validate_rest_api_ids=lambda *a, **k: None,
        validate_rest_api_page=lambda *a, **k: None,
        validate_rest_api_page_size=lambda *a, **k: None,
        DEFAULT_PAGE=1,
        DEFAULT_PAGE_SIZE=20,
    )
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace(), TenantLLMService=SimpleNamespace(), resolve_llm_setting=lambda *a, **k: {})
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.canvas_service",
        CanvasTemplateService=SimpleNamespace(),
        UserCanvasService=SimpleNamespace(),
        completion=lambda *a, **k: None,
        completion_openai=lambda *a, **k: None,
    )
    _stub(monkeypatch, "api.db.services.canvas_replica_service", CanvasReplicaService=SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", Task=SimpleNamespace())
    _stub(monkeypatch, "api.db", CanvasCategory=SimpleNamespace(), __path__=[])
    _stub(monkeypatch, "api.db.joint_services", __path__=[])
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *a, **k: {},
        resolve_llm_setting=lambda *a, **k: {},
        get_composite_model_name_by_ids=lambda *a, **k: "",
        get_composite_model_name_by_id=lambda *a, **k: "",
        resolve_model_id=lambda *a, **k: "",
        get_api_key=lambda *a, **k: "",
        get_model_config_by_id=lambda *a, **k: {},
        get_tenant_default_model_by_type=lambda *a, **k: {},
    )
    _stub(monkeypatch, "api.db.services.chunk_feedback_service", ChunkFeedbackService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace())

    quart_stub = ModuleType("quart")
    quart_stub.request = SimpleNamespace(method="DELETE", args={}, json={})
    quart_stub.jsonify = lambda payload: payload
    quart_stub.Response = lambda *a, **k: SimpleNamespace()
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    werkzeug_exc = ModuleType("werkzeug.exceptions")
    werkzeug_exc.BadRequest = type("BadRequest", (Exception,), {})
    monkeypatch.setitem(sys.modules, "werkzeug.exceptions", werkzeug_exc)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"
    spec = importlib.util.spec_from_file_location("test_chat_api_delete_msg", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_chat_api_delete_msg", module)
    spec.loader.exec_module(module)
    return module, update_calls


# --------------------------------------------------------------------------- #
# Tests
# --------------------------------------------------------------------------- #


@pytest.mark.p1
class TestDeleteSessionMessage:
    """Pin the #19430 fix: correct reference index regardless of prologue."""

    @pytest.mark.asyncio
    async def test_deletes_first_pair_reference_in_session_without_prologue(self, monkeypatch):
        """Regression: deleting the first QA pair in a session WITHOUT a
        prologue must remove reference[0], not reference[-1]."""
        conv = _build_session(with_prologue=False, n_pairs=2)
        assert conv["message"][0]["id"] == "u0"
        assert conv["message"][1]["id"] == "u0"
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "u0")

        assert result["code"] != 500, result
        # The first pair (user/assistant at indices 0..1) is gone, pair 1
        # shifts to indices 0..1.
        assert len(update_calls) == 1
        saved = update_calls[0]["conv"]
        assert [m["id"] for m in saved["message"]] == ["u1", "u1"]
        # The deleted pair's reference (ref0) is gone; ref1 remains.
        assert saved["reference"] == [{"doc_id": "d1", "content": "ref1"}]

    @pytest.mark.asyncio
    async def test_deletes_first_pair_reference_in_session_with_prologue(self, monkeypatch):
        """The with-prologue case must still work — same code path, just
        pair indices shift by 1."""
        conv = _build_session(with_prologue=True, n_pairs=2)
        assert conv["message"][0]["id"] == "prologue"
        module, update_calls = _load_chat_api(monkeypatch, conv)

        # The first user message is at index 1; deleting it should
        # remove the assistant at index 2 and reference[0].
        result = await module.delete_session_message("c-1", "s-1", "u0")

        assert result["code"] != 500, result
        saved = update_calls[0]["conv"]
        assert [m["id"] for m in saved["message"]] == ["prologue", "u1", "u1"]
        assert saved["reference"] == [{"doc_id": "d1", "content": "ref1"}]

    @pytest.mark.asyncio
    async def test_deletes_second_pair_correctly(self, monkeypatch):
        """Deleting the second user message (no-prologue layout) must
        remove pair index 1, not pair index 0."""
        conv = _build_session(with_prologue=False, n_pairs=2)
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "u1")

        assert result["code"] != 500, result
        saved = update_calls[0]["conv"]
        assert [m["id"] for m in saved["message"]] == ["u0", "u0"]
        # ref1 deleted, ref0 retained.
        assert saved["reference"] == [{"doc_id": "d0", "content": "ref0"}]

    @pytest.mark.asyncio
    async def test_unpaired_msg_id_returns_data_error(self, monkeypatch):
        """A user message with no assistant follow-up must return a clean
        data error instead of raising AssertionError → 500."""
        conv = _build_session(with_prologue=False, n_pairs=1)
        # Drop the assistant half of the only pair to leave an unpaired user msg.
        conv["message"].pop()
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "u0")

        assert result["code"] == 500  # DATA_ERROR
        assert "not paired" in result["message"]
        # No save happens on bad input.
        assert update_calls == []

    @pytest.mark.asyncio
    async def test_same_id_non_assistant_follow_up_is_not_deleted(self, monkeypatch):
        conv = _build_session(with_prologue=False, n_pairs=1)
        conv["message"][1]["role"] = "user"
        expected_messages = copy.deepcopy(conv["message"])
        expected_references = copy.deepcopy(conv["reference"])
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "u0")

        assert result["code"] == 500  # DATA_ERROR
        assert "not paired" in result["message"]
        assert conv["message"] == expected_messages
        assert conv["reference"] == expected_references
        assert update_calls == []

    @pytest.mark.asyncio
    async def test_session_with_empty_references_does_not_raise(self, monkeypatch):
        """A session whose message list is non-empty but ``reference`` is
        empty must not raise IndexError. The route must succeed (delete
        the messages) and leave the empty reference list alone."""
        conv = _build_session(with_prologue=False, n_pairs=2)
        conv["reference"] = []
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "u0")

        assert result["code"] != 500, result
        saved = update_calls[0]["conv"]
        # Pair 0 deleted, pair 1 remains.
        assert [m["id"] for m in saved["message"]] == ["u1", "u1"]
        assert saved["reference"] == []

    @pytest.mark.asyncio
    async def test_unknown_msg_id_is_a_no_op(self, monkeypatch):
        """A msg_id that doesn't match any message leaves the session
        unchanged and still returns 200 (existing behavior)."""
        import copy

        conv = _build_session(with_prologue=False, n_pairs=2)
        # Snapshot the conv BEFORE the call so we can assert it wasn't
        # mutated (CodeRabbit review: comparing update_calls[0]["conv"] to
        # the same mutable object vacuously passes — both are the same dict).
        expected_message_ids = [m["id"] for m in conv["message"]]
        expected_references = copy.deepcopy(conv["reference"])
        module, update_calls = _load_chat_api(monkeypatch, conv)

        result = await module.delete_session_message("c-1", "s-1", "does-not-exist")

        assert result["code"] != 500, result
        # update_by_id still called (matches the existing pre-fix behavior —
        # the route unconditionally calls update_by_id even when the loop
        # found no match). The session state itself is unchanged.
        assert len(update_calls) == 1
        assert [m["id"] for m in update_calls[0]["conv"]["message"]] == expected_message_ids
        assert update_calls[0]["conv"]["reference"] == expected_references
