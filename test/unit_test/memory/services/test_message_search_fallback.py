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

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr

pytestmark = pytest.mark.p2


class _Store:
    def __init__(self, responses, mutate_first_dense=False):
        self.responses = list(responses)
        self.mutate_first_dense = mutate_first_dense
        self.calls = []

    def search(self, **kwargs):
        self.calls.append(kwargs)
        if self.mutate_first_dense and len(self.calls) == 1:
            dense = next(expr for expr in kwargs["match_expressions"] if isinstance(expr, MatchDenseExpr))
            dense.extra_options["filter"] = "lexical predicate"
            dense.extra_options["similarity"] = "0.2"
        return self.responses.pop(0)

    @staticmethod
    def get_fields(result, _fields):
        return result


@pytest.fixture
def messages_module(monkeypatch):
    """Import the service with a lightweight settings module."""
    settings_stub = types.ModuleType("common.settings")
    monkeypatch.setitem(sys.modules, "common.settings", settings_stub)
    import common

    monkeypatch.setattr(common, "settings", settings_stub, raising=False)
    sys.modules.pop("memory.services.messages", None)
    module = importlib.import_module("memory.services.messages")
    yield module, settings_stub
    sys.modules.pop("memory.services.messages", None)


def _hybrid_expressions():
    text = MatchTextExpr(["content"], "zzqx", 100, {"minimum_should_match": 0.2})
    dense = MatchDenseExpr("q_3_vec", [0.1, 0.2, 0.3], "float", "cosine", 10, {"similarity": 0.2})
    fusion = FusionExpr("weighted_sum", 10, {"weights": "0.7,0.3"})
    return [text, dense, fusion]


def test_zero_result_hybrid_search_retries_with_clean_dense_expression(messages_module):
    module, settings_stub = messages_module
    result_mapping = {"message-1": {"message_id": "message-1", "content": "semantic match"}}
    store = _Store([(None, 0), (result_mapping, 1)], mutate_first_dense=True)
    settings_stub.msgStoreConn = store

    result = module.MessageService.search_message(
        ["memory-1"],
        {},
        ["tenant-1"],
        _hybrid_expressions(),
        10,
        allow_dense_fallback=True,
    )

    assert result == list(result_mapping.values())
    assert len(store.calls) == 2
    assert len(store.calls[1]["match_expressions"]) == 1
    fallback = store.calls[1]["match_expressions"][0]
    assert isinstance(fallback, MatchDenseExpr)
    assert fallback.extra_options == {"similarity": 0.2}
    assert store.calls[1]["condition"]["status"] == 1
    assert store.calls[1]["memory_ids"] == ["memory-1"]


def test_hybrid_hit_does_not_run_dense_fallback(messages_module):
    module, settings_stub = messages_module
    result_mapping = {"message-1": {"message_id": "message-1", "content": "lexical match"}}
    store = _Store([(result_mapping, 1)])
    settings_stub.msgStoreConn = store

    result = module.MessageService.search_message(
        ["memory-1"],
        {},
        ["tenant-1"],
        _hybrid_expressions(),
        10,
        allow_dense_fallback=True,
    )

    assert result == list(result_mapping.values())
    assert len(store.calls) == 1


def test_keyword_only_search_does_not_run_dense_fallback(messages_module):
    module, settings_stub = messages_module
    store = _Store([(None, 0)])
    settings_stub.msgStoreConn = store

    result = module.MessageService.search_message(
        ["memory-1"],
        {},
        ["tenant-1"],
        _hybrid_expressions(),
        10,
        allow_dense_fallback=False,
    )

    assert result == []
    assert len(store.calls) == 1
