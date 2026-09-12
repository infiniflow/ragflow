#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
from types import SimpleNamespace

from agent.tools.retrieval import Retrieval


class TestNormalizeDocumentIds:
    def test_empty_values(self):
        assert Retrieval._normalize_document_ids(None) == []
        assert Retrieval._normalize_document_ids("") == []
        assert Retrieval._normalize_document_ids([]) == []

    def test_list(self):
        assert Retrieval._normalize_document_ids(["a", "b"]) == ["a", "b"]
        assert Retrieval._normalize_document_ids(["a", "", "b"]) == ["a", "b"]

    def test_single_string(self):
        assert Retrieval._normalize_document_ids("doc-1") == ["doc-1"]


class TestResolveDocumentIds:
    def test_prefers_kwargs(self):
        tool = Retrieval.__new__(Retrieval)
        tool._param = SimpleNamespace(document_ids="doc-config")
        assert tool._resolve_document_ids(["doc-runtime"]) == ["doc-runtime"]

    def test_falls_back_to_param(self):
        tool = Retrieval.__new__(Retrieval)
        tool._param = SimpleNamespace(document_ids=["doc-config"])
        tool._canvas = SimpleNamespace(is_reff=lambda _exp: False)
        assert tool._resolve_document_ids(None) == ["doc-config"]
