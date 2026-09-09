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

"""Unit tests for DELETE /chats/<id>/sessions/<sid>/messages/<msg_id>.

The handler must drop the QA pair AND its matching reference entry: when
the session has no assistant prologue, deleting the first QA pair must not
remove the LAST reference, unknown/unpaired message ids must not raise,
and an empty reference list must not raise.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


def _module_stub(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


def _conv_obj(messages, references):
    return SimpleNamespace(
        id="sess_1",
        dialog_id="chat_1",
        to_dict=lambda: {"id": "sess_1", "message": [dict(m) for m in messages], "reference": list(references)},
    )


def _envelope(data=None, message="", code=0):
    return {"code": code, "message": message, "data": data}


def _load_chat_api(monkeypatch, conv):
    repo_root = Path(__file__).resolve().parents[3]
    updated = {}

    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", Response=None, request=SimpleNamespace(args={})))
    monkeypatch.setitem(sys.modules, "werkzeug.exceptions", _module_stub("werkzeug.exceptions", BadRequest=Exception))
    monkeypatch.setitem(
        sys.modules,
        "api.apps",
        _module_stub(
            "api.apps",
            current_user=SimpleNamespace(id="tenant_1"),
            login_required=lambda *a, **k: (a[0] if a and callable(a[0]) else (lambda f: f)),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.apps.restful_apis._generation_params",
        _module_stub("api.apps.restful_apis._generation_params", merge_generation_config=None, pop_generation_config=None),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", _module_stub("api.db.services.llm_service", resolve_llm_setting=None, LLMBundle=None))
    monkeypatch.setitem(
        sys.modules,
        "api.db.joint_services.tenant_model_service",
        _module_stub("api.db.joint_services.tenant_model_service", get_api_key=None, get_composite_model_name_by_id=None, get_model_config_by_id=None, get_tenant_default_model_by_type=None, resolve_model_config=None, resolve_model_id=None),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.conversation_service",
        _module_stub(
            "api.db.services.conversation_service",
            ConversationService=SimpleNamespace(
                get_by_id=lambda session_id: (True, conv),
                update_by_id=lambda cid, data: updated.update(data),
            ),
            structure_answer=lambda *a, **k: a[0] if a else None,
        ),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", _module_stub("api.db.services.dialog_service", DialogService=SimpleNamespace(model=SimpleNamespace(_meta=SimpleNamespace(fields={"id": None}))), gen_mindmap=None, rag_agent=None))
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", _module_stub("api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace(), validate_dataset_embedding_models=None))
    monkeypatch.setitem(sys.modules, "api.db.services.chunk_feedback_service", _module_stub("api.db.services.chunk_feedback_service", ChunkFeedbackService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", _module_stub("api.db.services.search_service", SearchService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", _module_stub("api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace()))
    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub(
            "api.utils.api_utils",
            check_duplicate_ids=None,
            get_data_error_result=lambda *a, **kw: _envelope(None, kw.get("message", a[0] if a else ""), 102),
            get_json_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
            get_request_json=None,
            server_error_response=lambda e: _envelope(None, str(e), 100),
            validate_request=lambda *_a, **_k: (lambda f: f),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub("api.utils.pagination_utils", DEFAULT_PAGE=1, DEFAULT_PAGE_SIZE=30, validate_rest_api_ids=lambda ids, name: ids, validate_rest_api_page=lambda p: int(p), validate_rest_api_page_size=lambda s: int(s)),
    )
    monkeypatch.setitem(
        sys.modules,
        "common.constants",
        _module_stub("common.constants", LLMType=SimpleNamespace(), RetCode=SimpleNamespace(OPERATING_ERROR=103, AUTHENTICATION_ERROR=109), StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1"))),
    )
    monkeypatch.setitem(sys.modules, "common", _module_stub("common", settings=SimpleNamespace()))

    async def _tpe(fn, *args):
        return fn(*args)

    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_tpe))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", _module_stub("rag.prompts.generator", chunks_format=None))
    monkeypatch.setitem(sys.modules, "rag.prompts.template", _module_stub("rag.prompts.template", load_prompt=None))

    module_name = "test_chat_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    async def _owned(chat_id):
        return True
    module._ensure_owned_chat = _owned
    module._build_session_response = lambda c: {"id": c["id"], "message": c["message"], "reference": c["reference"]}
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    module._ensure_owned_chat = _owned
    module._build_session_response = lambda c: {"id": c["id"], "message": c["message"], "reference": c["reference"]}
    return module, updated


def _run(coro):
    return asyncio.get_event_loop().run_until_complete(coro)


def _qa(mid):
    return [
        {"role": "user", "id": mid, "content": "q"},
        {"role": "assistant", "id": mid, "content": "a"},
    ]


@pytest.mark.p2
def test_delete_first_qa_pair_without_prologue_keeps_reference_alignment(monkeypatch):
    messages = _qa("m1") + _qa("m2")
    conv = _conv_obj(messages, [{"chunks": "ref-m1"}, {"chunks": "ref-m2"}])
    module, updated = _load_chat_api(monkeypatch, conv)
    res = _run(module.delete_session_message(chat_id="chat_1", session_id="sess_1", msg_id="m1"))
    assert res["data"]["reference"] == [{"chunks": "ref-m2"}]
    assert [m["id"] for m in res["data"]["message"]] == ["m2", "m2"]


@pytest.mark.p2
def test_delete_first_qa_pair_with_prologue(monkeypatch):
    # The prologue keeps one message slot but no reference entry: reference
    # entries follow QA pairs only.
    messages = [{"role": "assistant", "id": "prologue", "content": "hi"}] + _qa("m1") + _qa("m2")
    conv = _conv_obj(messages, [{"chunks": "ref-m1"}, {"chunks": "ref-m2"}])
    module, updated = _load_chat_api(monkeypatch, conv)
    res = _run(module.delete_session_message(chat_id="chat_1", session_id="sess_1", msg_id="m1"))
    assert res["data"]["reference"] == [{"chunks": "ref-m2"}]
    assert [m["id"] for m in res["data"]["message"]] == ["prologue", "m2", "m2"]


@pytest.mark.p2
def test_delete_unpaired_message_id_returns_error_not_500(monkeypatch):
    messages = [
        {"role": "user", "id": "m1", "content": "q"},
        {"role": "assistant", "id": "mX", "content": "a"},
    ]
    conv = _conv_obj(messages, [{"chunks": "ref-m1"}])
    module, updated = _load_chat_api(monkeypatch, conv)
    res = _run(module.delete_session_message(chat_id="chat_1", session_id="sess_1", msg_id="m1"))
    assert res["code"] == 102
    assert updated == {}


@pytest.mark.p2
def test_delete_with_empty_reference_does_not_raise(monkeypatch):
    messages = _qa("m1")
    conv = _conv_obj(messages, [])
    module, updated = _load_chat_api(monkeypatch, conv)
    res = _run(module.delete_session_message(chat_id="chat_1", session_id="sess_1", msg_id="m1"))
    assert res["data"]["message"] == []
    assert res["data"]["reference"] == []
