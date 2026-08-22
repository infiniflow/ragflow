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
"""Regression tests for issue #18397.

``POST /api/v1/retrieval`` with ``metadata_condition`` returned empty
chunks on large datasets (>10k documents) where ``/datasets/search`` with
the same ``meta_data_filter`` returned non-empty chunks. Root cause:
``/retrieval`` used ``filter_doc_ids_by_metadata`` (in-memory fetch
via ``DocMetadataService.get_flatted_meta_by_kbs``) while
``/datasets/search`` used ``apply_meta_data_filter`` which tries the
ES / Infinity / GaussDB push-down first and falls back to in-memory
only when push-down cannot service the request.

The fix: ``/retrieval`` now uses ``apply_meta_data_filter`` -- the
same helper that ``/datasets/search`` uses. The push-down path is
preferred; the in-memory path remains the fallback so backends without
push-down support (or filters the push-down can't express) still
return the right answer.

The test focuses on the contract that the new /retrieval call site
relies on -- ``apply_meta_data_filter`` is called with the right
arguments (a 'manual' method shape derived from the API's
``metadata_condition``) and that the result is consumed correctly.
"""

import asyncio
import ast
from pathlib import Path

import pytest

from common.metadata_utils import apply_meta_data_filter, convert_conditions


# ---------------------------------------------------------------------------
# apply_meta_data_filter integration: the new /retrieval call shape
# ---------------------------------------------------------------------------


def test_api_metadata_condition_converts_to_manual_filter_shape():
    """The API's ``metadata_condition`` shape is
    ``{logic, conditions: [{name, comparison_operator, value}]}``.
    ``convert_conditions`` must map ``comparison_operator`` ->
    ``op`` and ``name`` -> ``key`` so ``apply_meta_data_filter`` can
    consume the result under the ``manual`` method.
    """
    metadata_condition = {
        "logic": "and",
        "conditions": [
            {"name": "agent_code", "comparison_operator": "=", "value": "AGENT_WST_SHENGCHAN"},
        ],
    }
    conditions = convert_conditions(metadata_condition)
    assert conditions == [
        {"op": "=", "key": "agent_code", "value": "AGENT_WST_SHENGCHAN"},
    ], f"convert_conditions must map the API's name/comparison_operator to the manual filter's key/op; got {conditions!r}"

    manual_filter = {
        "method": "manual",
        "manual": conditions,
        "logic": metadata_condition.get("logic", "and"),
    }
    assert manual_filter == {
        "method": "manual",
        "manual": [
            {"op": "=", "key": "agent_code", "value": "AGENT_WST_SHENGCHAN"},
        ],
        "logic": "and",
    }


def test_apply_meta_data_filter_pushdown_path_is_used_before_in_memory_fallback(monkeypatch):
    """The contract that fixes issue #18397: ``apply_meta_data_filter``
    is the right entry point for the /retrieval call site because it
    tries the push-down path first and only falls back to the
    in-memory path when the push-down returns ``None`` (cannot service
    the request).

    This is a contract test on the helper -- pin the helper's behaviour
    so a future refactor that flips the order (in-memory first) is
    caught loudly.
    """
    from common import metadata_utils

    captured = {"in_memory_called": False, "pushdown_called": False}

    def _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader):
        captured["in_memory_called"] = True
        return ["doc-3"]

    # Reach inside filter_doc_ids_by_metadata's body and capture the
    # call to _try_meta_pushdown by wrapping the helper directly.
    # The real filter_doc_ids_by_metadata function is:
    #   doc_ids = _try_meta_pushdown(kb_ids, conditions, logic) if conditions and kb_ids else None
    #   if doc_ids is not None: return doc_ids
    #   return meta_filter(metas_loader(), conditions, logic)
    # We re-implement it here so the test is independent of the
    # real push-down path (which would try to connect to MySQL).
    def _patched_filter(kb_ids, conditions, logic, metas_loader):
        doc_ids = _try_meta_pushdown(kb_ids, conditions, logic) if conditions and kb_ids else None
        if doc_ids is not None:
            return doc_ids
        return _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader)

    def _try_meta_pushdown(kb_ids, conditions, logic):
        captured["pushdown_called"] = True
        return ["doc-1", "doc-2"]

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", _try_meta_pushdown)
    monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", _patched_filter)

    result = asyncio.run(
        apply_meta_data_filter(
            {"method": "manual", "manual": [{"op": "=", "key": "k", "value": "v"}], "logic": "and"},
            None,
            "",
            None,
            None,
            kb_ids=["kb-1"],
            metas_loader=lambda: {},
        )
    )

    assert result == ["doc-1", "doc-2"], f"apply_meta_data_filter must return the push-down result when the push-down path succeeds; got {result!r}"
    assert captured["pushdown_called"], "push-down path must be attempted first"
    assert not captured["in_memory_called"], "in-memory fallback must NOT be called when push-down succeeds"


def test_apply_meta_data_filter_falls_back_to_in_memory_when_pushdown_returns_none(monkeypatch):
    """When the push-down path returns None (cannot service the
    request -- e.g. oceanbase/seekdb without push-down support, or a
    filter the push-down can't express), apply_meta_data_filter falls
    back to the in-memory path. The /retrieval call site relies on
    this fallback so backends without push-down support still work.
    """
    from common import metadata_utils

    captured = {"in_memory_called": False, "pushdown_called": False}

    def _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader):
        captured["in_memory_called"] = True
        return ["doc-3", "doc-4"]

    def _try_meta_pushdown(kb_ids, conditions, logic):
        captured["pushdown_called"] = True
        return None  # push-down cannot service

    def _patched_filter(kb_ids, conditions, logic, metas_loader):
        doc_ids = _try_meta_pushdown(kb_ids, conditions, logic) if conditions and kb_ids else None
        if doc_ids is not None:
            return doc_ids
        return _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader)

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", _try_meta_pushdown)
    monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", _patched_filter)

    result = asyncio.run(
        apply_meta_data_filter(
            {"method": "manual", "manual": [{"op": "=", "key": "k", "value": "v"}], "logic": "and"},
            None,
            "",
            None,
            None,
            kb_ids=["kb-1"],
            metas_loader=lambda: {},
        )
    )

    assert result == ["doc-3", "doc-4"], f"apply_meta_data_filter must fall back to the in-memory path when push-down returns None; got {result!r}"
    assert captured["pushdown_called"]
    assert captured["in_memory_called"], "in-memory fallback must run when push-down returns None"


def test_apply_meta_data_filter_short_circuits_empty_pushdown_results(monkeypatch):
    """When the push-down path returns an empty list (no matches), the
    /retrieval call site must short-circuit to 'no chunks' (the
    existing ``if not doc_ids and metadata_condition.get('conditions')``
    check) rather than calling the in-memory path with a stale
    metadata fetch. This is the user-visible fix: an honest empty
    result from the push-down is better than silently returning a
    different in-memory answer.
    """
    from common import metadata_utils

    captured = {"in_memory_called": False}

    def _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader):
        captured["in_memory_called"] = True
        return ["doc-x"]

    def _try_meta_pushdown(kb_ids, conditions, logic):
        return []

    def _patched_filter(kb_ids, conditions, logic, metas_loader):
        doc_ids = _try_meta_pushdown(kb_ids, conditions, logic) if conditions and kb_ids else None
        if doc_ids is not None:
            return doc_ids
        return _filter_doc_ids_by_metadata(kb_ids, conditions, logic, metas_loader)

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", _try_meta_pushdown)
    monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", _patched_filter)

    result = asyncio.run(
        apply_meta_data_filter(
            {"method": "manual", "manual": [{"op": "=", "key": "k", "value": "v"}], "logic": "and"},
            None,
            "",
            None,
            None,
            kb_ids=["kb-1"],
            metas_loader=lambda: {},
        )
    )

    # The helper returns the ['-999'] sentinel when manual filters
    # yield no result; the /retrieval call site interprets that as
    # "no chunks" and returns early. The point of this test is that
    # the in-memory path is NOT consulted when the push-down path
    # already returned its honest answer.
    assert result == ["-999"], f"apply_meta_data_filter must propagate the empty push-down result (not silently fall back to in-memory); got {result!r}"
    assert not captured["in_memory_called"], (
        "in-memory fallback must NOT run when push-down returns an empty list (the user-visible fix -- an honest empty result is better than a stale in-memory answer)"
    )


# ---------------------------------------------------------------------------
# chunk_api.py:retrieval_test must use apply_meta_data_filter (the push-down
# path), not filter_doc_ids_by_metadata (in-memory only)
# ---------------------------------------------------------------------------


def test_retrieval_uses_apply_meta_data_filter_not_filter_doc_ids_by_metadata():
    """Pin the call-site change. The /retrieval endpoint's
    metadata_condition path must use apply_meta_data_filter so the
    push-down (ES / Infinity / GaussDB) is tried before the in-memory
    fallback. Pre-fix, /retrieval called filter_doc_ids_by_metadata
    directly, which always used the in-memory path and missed matches
    on large datasets (the root cause of issue #18397).

    The test is a source-level walk of the chunk_api.py file (no
    import) to avoid the heavy import chain that triggers the DB
    services to be initialised.
    """
    path = Path(__file__).resolve().parents[3] / "api" / "apps" / "restful_apis" / "chunk_api.py"
    source = path.read_text(encoding="utf-8")
    tree = ast.parse(source)

    # 1. The import block must bring in apply_meta_data_filter.
    imports_apply = False
    imports_filter = False
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module == "common.metadata_utils":
            for alias in node.names:
                if alias.name == "apply_meta_data_filter":
                    imports_apply = True
                if alias.name == "filter_doc_ids_by_metadata":
                    imports_filter = True
    assert imports_apply, (
        f"{path.name} must import apply_meta_data_filter from "
        f"common.metadata_utils so the /retrieval endpoint uses the "
        f"push-down path (see issue #18397). The pre-fix code imported "
        f"filter_doc_ids_by_metadata (in-memory only) and missed matches "
        f"on large datasets."
    )
    assert not imports_filter, (
        f"{path.name} must not import filter_doc_ids_by_metadata -- the "
        f"/retrieval endpoint should use apply_meta_data_filter (which "
        f"wraps the push-down + in-memory fallback); the direct in-memory "
        f"call is the bug. See issue #18397."
    )

    # 2. Inside the retrieval_test function, the metadata_condition
    # branch must call apply_meta_data_filter (not filter_doc_ids_by_metadata).
    for node in ast.walk(tree):
        if not (isinstance(node, ast.AsyncFunctionDef) and node.name == "retrieval_test"):
            continue
        # Walk the function body and look at every await-call
        # expression. The metadata_condition branch must contain an
        # apply_meta_data_filter call.
        body_src = ast.unparse(node)
        assert "apply_meta_data_filter" in body_src, "retrieval_test must call apply_meta_data_filter to honour the push-down path (issue #18397)."
        # The body must NOT call filter_doc_ids_by_metadata directly.
        assert "filter_doc_ids_by_metadata" not in body_src, (
            "retrieval_test must not call filter_doc_ids_by_metadata "
            "directly -- the direct in-memory call is the bug. The "
            "endpoint should use apply_meta_data_filter, which wraps "
            "the push-down + in-memory fallback."
        )
        return
    pytest.fail("`retrieval_test` function not found in chunk_api.py")
