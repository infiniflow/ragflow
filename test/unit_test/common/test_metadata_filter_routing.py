from common import metadata_utils


def test_supported_filter_uses_pushdown_without_loading_metadata(monkeypatch):
    conditions = [{"key": "department", "op": "=", "value": "engineering"}]

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: ["doc-1"])

    def fail_if_loaded():
        raise AssertionError("metadata must stay lazy when push-down succeeds")

    assert metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", fail_if_loaded) == ["doc-1"]


def test_empty_pushdown_result_does_not_load_metadata(monkeypatch):
    conditions = [{"key": "department", "op": "=", "value": "legal"}]

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: [])

    def fail_if_loaded():
        raise AssertionError("an empty push-down result is definitive")

    assert metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", fail_if_loaded) == []


def test_unsupported_filter_preserves_multivalue_fallback_semantics(monkeypatch):
    conditions = [{"key": "tag", "op": "≠", "value": "alpha"}]
    metas = {"tag": {"alpha": ["doc-1"], "beta": ["doc-1", "doc-2"]}}
    load_count = 0

    monkeypatch.setattr(metadata_utils, "_try_meta_pushdown", lambda kb_ids, filters, logic: None)

    def load_metas():
        nonlocal load_count
        load_count += 1
        return metas

    result = metadata_utils.filter_doc_ids_by_metadata(["kb-1"], conditions, "and", load_metas)

    assert set(result) == {"doc-1", "doc-2"}
    assert load_count == 1
