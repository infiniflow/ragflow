import builtins
import logging
import sys
from types import SimpleNamespace

import pytest

from common import metadata_utils


def test_supported_filter_uses_pushdown_without_loading_metadata(monkeypatch, caplog):
    conditions = [{"key": "department", "op": "=", "value": "engineering"}]

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: ["doc-1"])

    def fail_if_loaded():
        raise AssertionError("metadata must stay lazy when push-down succeeds")

    with caplog.at_level(logging.DEBUG):
        result = metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", fail_if_loaded)

    assert result == ["doc-1"]
    assert "Metadata filter used push-down: kb_count=1 condition_count=1 matched_doc_count=1" in caplog.messages
    assert "doc-1" not in "\n".join(caplog.messages)


def test_empty_pushdown_result_does_not_load_metadata(monkeypatch, caplog):
    conditions = [{"key": "department", "op": "=", "value": "legal"}]

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: [])

    def fail_if_loaded():
        raise AssertionError("an empty push-down result is definitive")

    with caplog.at_level(logging.DEBUG):
        result = metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", fail_if_loaded)

    assert result == []
    assert "Metadata filter used push-down: kb_count=1 condition_count=1 matched_doc_count=0" in caplog.messages


def test_unsupported_filter_preserves_multivalue_fallback_semantics(monkeypatch, caplog):
    conditions = [{"key": "tag", "op": "≠", "value": "alpha"}]
    metas = {"tag": {"alpha": ["doc-1"], "beta": ["doc-1", "doc-2"]}}
    load_count = 0

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: None)

    def load_metas():
        nonlocal load_count
        load_count += 1
        return metas

    with caplog.at_level(logging.DEBUG):
        result = metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", load_metas)

    assert set(result) == {"doc-1", "doc-2"}
    assert load_count == 1
    assert "Metadata filter uses in-memory fallback: kb_count=1 condition_count=1" in caplog.messages
    assert "doc-1" not in "\n".join(caplog.messages)


def test_filter_without_kb_scope_skips_pushdown(monkeypatch):
    conditions = [{"key": "department", "op": "=", "value": "engineering"}]

    def fail_if_called(*args):
        raise AssertionError("push-down requires a KB scope")

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", fail_if_called)

    result = metadata_utils.filter_doc_ids_by_metadata(
        [],
        conditions,
        "and",
        lambda: {"department": {"engineering": ["doc-1"]}},
    )

    assert result == ["doc-1"]


def test_pushdown_service_errors_are_not_masked(monkeypatch):
    class FailingDocMetadataService:
        @staticmethod
        def filter_doc_ids_by_meta_pushdown(kb_ids, conditions, logic):
            raise RuntimeError("unexpected service failure")

    real_import = builtins.__import__

    def import_with_failing_service(name, module_globals=None, module_locals=None, fromlist=(), level=0):
        if name == "api.db.services.doc_metadata_service":
            return SimpleNamespace(DocMetadataService=FailingDocMetadataService)
        return real_import(name, module_globals, module_locals, fromlist, level)

    monkeypatch.setattr(builtins, "__import__", import_with_failing_service)

    with pytest.raises(RuntimeError, match="unexpected service failure"):
        metadata_utils._try_meta_pushdown(
            ["kb-1"],
            [{"key": "department", "op": "=", "value": "engineering"}],
            "and",
        )


def test_missing_pushdown_service_uses_fallback(monkeypatch):
    real_import = builtins.__import__

    def import_without_service(name, module_globals=None, module_locals=None, fromlist=(), level=0):
        if name == "api.db.services.doc_metadata_service":
            raise ImportError("service is unavailable")
        return real_import(name, module_globals, module_locals, fromlist, level)

    monkeypatch.setattr(builtins, "__import__", import_without_service)

    assert (
        metadata_utils._try_meta_pushdown(
            ["kb-1"],
            [{"key": "department", "op": "=", "value": "engineering"}],
            "and",
        )
        is None
    )


@pytest.mark.asyncio
async def test_apply_metadata_filter_uses_shared_router(monkeypatch):
    conditions = [{"key": "department", "op": "=", "value": "engineering"}]
    calls = []

    monkeypatch.setitem(
        sys.modules,
        "rag.prompts.generator",
        SimpleNamespace(gen_meta_filter=None),
    )

    def shared_router(kb_ids, routed_conditions, logic, metas_loader):
        calls.append((kb_ids, routed_conditions, logic, metas_loader))
        return ["doc-1"]

    monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", shared_router)

    result = await metadata_utils.apply_meta_data_filter(
        {"method": "manual", "manual": conditions},
        kb_ids=["kb-1"],
        metas_loader=dict,
    )

    assert result == ["doc-1"]
    assert len(calls) == 1
    assert calls[0][:3] == (["kb-1"], conditions, "and")
