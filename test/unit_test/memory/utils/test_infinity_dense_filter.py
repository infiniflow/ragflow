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

import importlib
import sys
import types

import pytest

from common.doc_store.doc_store_base import MatchDenseExpr

pytestmark = pytest.mark.p2


@pytest.fixture
def infinity_module(monkeypatch):
    """Import the Memory Infinity adapter without connecting to Infinity."""
    infinity_pkg = types.ModuleType("infinity")
    infinity_pkg.__path__ = []
    infinity_common = types.ModuleType("infinity.common")
    infinity_common.InfinityException = RuntimeError
    infinity_common.SortType = types.SimpleNamespace(Asc="asc", Desc="desc")
    infinity_errors = types.ModuleType("infinity.errors")
    infinity_errors.ErrorCode = types.SimpleNamespace()
    infinity_base = types.ModuleType("common.doc_store.infinity_conn_base")
    infinity_base.InfinityConnectionBase = type("InfinityConnectionBase", (), {})
    time_utils = types.ModuleType("common.time_utils")
    time_utils.date_string_to_timestamp = lambda value: value

    monkeypatch.setitem(sys.modules, "infinity", infinity_pkg)
    monkeypatch.setitem(sys.modules, "infinity.common", infinity_common)
    monkeypatch.setitem(sys.modules, "infinity.errors", infinity_errors)
    monkeypatch.setitem(sys.modules, "common.doc_store.infinity_conn_base", infinity_base)
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils)
    sys.modules.pop("memory.utils.infinity_conn", None)
    module = importlib.import_module("memory.utils.infinity_conn")
    yield module
    sys.modules.pop("memory.utils.infinity_conn", None)


def _dense(options=None):
    return MatchDenseExpr("q_3_vec", [0.1, 0.2, 0.3], "float", "cosine", 10, options)


def test_dense_only_search_keeps_scope_filter(infinity_module):
    dense = _dense({"similarity": 0.2})

    infinity_module._apply_dense_filter(dense, "status_int = 1", "")

    assert dense.extra_options["filter"] == "status_int = 1"


def test_hybrid_search_keeps_fulltext_filter(infinity_module):
    dense = _dense({"similarity": 0.2})

    infinity_module._apply_dense_filter(dense, "status_int = 1", "(status_int = 1) AND filter_fulltext('content', 'query')")

    assert dense.extra_options["filter"] == "(status_int = 1) AND filter_fulltext('content', 'query')"


def test_explicit_dense_filter_is_not_overwritten(infinity_module):
    dense = _dense({"filter": "memory_id = 'memory-1'"})

    infinity_module._apply_dense_filter(dense, "status_int = 1", "fulltext")

    assert dense.extra_options["filter"] == "memory_id = 'memory-1'"
