import logging

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
