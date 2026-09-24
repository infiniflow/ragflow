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
"""Regression tests for `_parse_retrieval_options` range-checking ``top_k`` (issue #19431).

`_parse_retrieval_options` accepts a `retrieval_setting` dict and returns
`(setting, similarity_threshold, top)`. The previous code never range-checked
`top_k`: a value <= 0 was forwarded to the retriever and surfaced as a 500,
arbitrarily large values triggered heavy search queries, and the route
returned `Internal server error` or a silent cost spike instead of a clear
argument error.

These tests pin the contract that `_parse_retrieval_options` raises a clear
`ValueError` for values outside the documented 1..1024 range and accepts
the boundary values.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


@pytest.fixture
def dify_retrieval_module(monkeypatch):
    """Load ``dify_retrieval_api.py`` with the heavy runtime stubbed.

    ``_parse_retrieval_options`` is a pure function — the only imports it
    touches at module load are ``logging``, ``quart``, and
    ``werkzeug.exceptions``, plus the project services used by the route
    decorator. Stubbing all of them keeps the test self-contained.
    """
    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(), login_required=lambda f: f)
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda f: f,
        build_error_result=lambda **kw: kw,
        get_request_json=lambda: SimpleNamespace(),
        get_json_result=lambda **kw: kw,
    )
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=lambda *a, **kw: SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *a, **kw: None,
        resolve_model_config=lambda *a, **kw: None,
    )
    _stub(monkeypatch, "common.metadata_utils", meta_filter=lambda *a, **kw: [], convert_conditions=lambda c: c)
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *a, **kw: {})
    # Stub common.settings so the module's `from common import settings` does
    # not pull in the heavy rag.utils.gcs_conn → google.cloud.storage chain
    # (which emits an ImportWarning that pytest escalates to an error).
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())

    quart_stub = ModuleType("quart")
    quart_stub.request = SimpleNamespace(method="POST", args={})
    quart_stub.jsonify = lambda payload: payload
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    werkzeug_exc = ModuleType("werkzeug.exceptions")
    werkzeug_exc.BadRequest = type("BadRequest", (Exception,), {})
    monkeypatch.setitem(sys.modules, "werkzeug.exceptions", werkzeug_exc)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "dify_retrieval_api.py"
    spec = importlib.util.spec_from_file_location("test_dify_retrieval_options", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_dify_retrieval_options", module)
    spec.loader.exec_module(module)
    return module


# --------------------------------------------------------------------------- #
# Tests
# --------------------------------------------------------------------------- #


@pytest.mark.p2
class TestParseRetrievalOptionsTopKRange:
    """Pin the documented 1..1024 contract on `top_k`."""

    def test_default_top_k_is_1024_when_omitted(self, dify_retrieval_module):
        _, _, top = dify_retrieval_module._parse_retrieval_options({})
        assert top == 1024

    def test_explicit_top_k_within_range_passes_through(self, dify_retrieval_module):
        _, _, top = dify_retrieval_module._parse_retrieval_options({"top_k": 7})
        assert top == 7

    def test_top_k_boundary_low_is_accepted(self, dify_retrieval_module):
        _, _, top = dify_retrieval_module._parse_retrieval_options({"top_k": 1})
        assert top == 1

    def test_top_k_boundary_high_is_accepted(self, dify_retrieval_module):
        _, _, top = dify_retrieval_module._parse_retrieval_options({"top_k": 1024})
        assert top == 1024

    def test_top_k_integral_float_is_accepted(self, dify_retrieval_module):
        """An integral float (``1024.0``) is still accepted — ``int(1024.0) == 1024``."""
        _, _, top = dify_retrieval_module._parse_retrieval_options({"top_k": 1024.0})
        assert top == 1024

    @pytest.mark.parametrize("bad_top_k", [0, -1, -100, 1025, 10_000, 1_000_000])
    def test_top_k_outside_range_raises_value_error(self, dify_retrieval_module, bad_top_k):
        """Out-of-range top_k must raise ValueError with a clear message so the
        route converts it to a 400 instead of forwarding to the retriever
        (top_k <= 0 → 500) or running a runaway query (top_k >> 1024)."""
        with pytest.raises(ValueError) as excinfo:
            dify_retrieval_module._parse_retrieval_options({"top_k": bad_top_k})
        assert "between 1 and 1024" in str(excinfo.value)

    @pytest.mark.parametrize("bad_top_k", [True, False, 1024.5, 1024.9, 0.5])
    def test_top_k_bool_and_fractional_float_are_rejected(self, dify_retrieval_module, bad_top_k):
        """``int(True) == 1`` and ``int(1024.9) == 1024`` would otherwise pass
        the range check; reject booleans and fractional floats explicitly."""
        with pytest.raises(ValueError) as excinfo:
            dify_retrieval_module._parse_retrieval_options({"top_k": bad_top_k})
        msg = str(excinfo.value)
        # Bool path produces "top_k must be integer and score_threshold must be
        # numeric"; fractional path produces the same message.
        assert "must be integer" in msg

    def test_top_k_zero_is_rejected_not_forwarded(self, dify_retrieval_module):
        """Regression for the documented `top_k = 0` 500: the guard fires
        before the retriever is touched."""
        with pytest.raises(ValueError):
            dify_retrieval_module._parse_retrieval_options({"top_k": 0})

    def test_top_k_million_does_not_reach_retriever(self, dify_retrieval_module):
        """Regression for the heavy-query scenario: top_k=10^6 must be
        rejected at the parser, not passed to the retriever."""
        with pytest.raises(ValueError):
            dify_retrieval_module._parse_retrieval_options({"top_k": 1_000_000})

    def test_top_k_explicit_none_raises(self, dify_retrieval_module):
        """`top_k=None` is rejected — `dict.get` only returns the default
        when the key is missing, not when the value is None. The route
        therefore surfaces this as the existing argument error."""
        with pytest.raises(ValueError):
            dify_retrieval_module._parse_retrieval_options({"top_k": None})

    def test_missing_setting_dict_uses_defaults(self, dify_retrieval_module):
        """Missing setting dict returns both defaults (1024 / 0.0)."""
        _, similarity, top = dify_retrieval_module._parse_retrieval_options(None)
        assert top == 1024
        assert similarity == 0.0

    def test_score_threshold_zero_with_valid_top_k(self, dify_retrieval_module):
        """score_threshold=0.0 and top_k=5 are both in range — happy path."""
        _, similarity, top = dify_retrieval_module._parse_retrieval_options({"score_threshold": 0.0, "top_k": 5})
        assert similarity == 0.0
        assert top == 5
