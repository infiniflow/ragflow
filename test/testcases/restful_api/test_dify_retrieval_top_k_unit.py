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

"""Unit tests for /dify/retrieval top_k validation.

_parse_retrieval_options must reject out-of-range top_k values (the route
turns the ValueError into a 400) instead of forwarding them to the
retriever, where top_k <= 0 raises and huge values trigger heavy queries.
"""

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


@pytest.fixture()
def dify_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", jsonify=lambda x: x, request=SimpleNamespace(args={})))
    monkeypatch.setitem(sys.modules, "werkzeug.exceptions", _module_stub("werkzeug.exceptions", BadRequest=Exception))
    monkeypatch.setitem(
        sys.modules,
        "api.apps",
        _module_stub(
            "api.apps",
            login_required=lambda *a, **k: (a[0] if a and callable(a[0]) else (lambda f: f)),
        ),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", _module_stub("api.db.services.document_service", DocumentService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", _module_stub("api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", _module_stub("api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", _module_stub("api.db.services.llm_service", LLMBundle=None))
    monkeypatch.setitem(
        sys.modules,
        "api.db.joint_services.tenant_model_service",
        _module_stub("api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=None, resolve_model_config=None),
    )
    monkeypatch.setitem(sys.modules, "common.metadata_utils", _module_stub("common.metadata_utils", convert_conditions=None, meta_filter=None))
    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub("api.utils.api_utils", add_tenant_id_to_kwargs=lambda f: f, build_error_result=lambda **kw: kw, get_request_json=None, get_json_result=lambda **kw: kw),
    )
    monkeypatch.setitem(sys.modules, "rag.app.tag", _module_stub("rag.app.tag", label_question=None))
    monkeypatch.setitem(sys.modules, "common", _module_stub("common", settings=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "common.constants", _module_stub("common.constants", LLMType=SimpleNamespace(), RetCode=SimpleNamespace(ARGUMENT_ERROR=101, NOT_FOUND=104, AUTHENTICATION_ERROR=109)))

    module_name = "test_dify_retrieval_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "dify_retrieval_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
@pytest.mark.parametrize("bad_top_k", [0, -1, -1024, 1025, 10**6])
def test_out_of_range_top_k_rejected(dify_module, bad_top_k):
    with pytest.raises(ValueError):
        dify_module._parse_retrieval_options({"top_k": bad_top_k})


@pytest.mark.p2
@pytest.mark.parametrize("good_top_k", [1, 8, 1024])
def test_in_range_top_k_accepted(dify_module, good_top_k):
    _, threshold, top = dify_module._parse_retrieval_options({"top_k": good_top_k})
    assert top == good_top_k
    assert threshold == 0.0


@pytest.mark.p2
def test_default_top_k(dify_module):
    _, _, top = dify_module._parse_retrieval_options({})
    assert top == 1024


@pytest.mark.p2
def test_non_numeric_top_k_rejected(dify_module):
    with pytest.raises(ValueError):
        dify_module._parse_retrieval_options({"top_k": "not-a-number"})
