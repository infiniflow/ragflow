from common.metadata_utils import meta_filter


def test_contains():
    metas = {"version": {"hello world": ["doc1"]}}
    filters = [{"key": "version", "op": "contains", "value": "world"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_contains():
    metas = {"version": {"foo": ["doc1"], "bar": ["doc2"]}}
    filters = [{"key": "version", "op": "not contains", "value": "foo"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_in_operator():
    metas = {"status": {"active": ["doc1"], "pending": ["doc2"]}}
    filters = [{"key": "status", "op": "in", "value": "active,pending"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc2"}


def test_not_in_operator():
    metas = {"status": {"active": ["doc1"], "pending": ["doc2"]}}
    filters = [{"key": "status", "op": "not in", "value": "pending"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_start_with():
    metas = {"name": {"prefix_value": ["doc1"], "other": ["doc2"]}}
    filters = [{"key": "name", "op": "start with", "value": "pre"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_end_with():
    metas = {"file": {"report.pdf": ["doc1"], "image.png": ["doc2"]}}
    filters = [{"key": "file", "op": "end with", "value": ".pdf"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_empty():
    metas = {"notes": {"": ["doc1"], "non-empty": ["doc2"]}}
    filters = [{"key": "notes", "op": "empty", "value": ""}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_empty():
    metas = {"notes": {"": ["doc1"], "non-empty": ["doc2"]}}
    filters = [{"key": "notes", "op": "not empty", "value": ""}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_equal():
    metas = {"score": {"5": ["doc1"], "6": ["doc2"]}}
    filters = [{"key": "score", "op": "=", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_equal():
    metas = {"score": {"5": ["doc1"], "6": ["doc2"]}}
    filters = [{"key": "score", "op": "≠", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_greater_than():
    metas = {"score": {"10": ["doc1"], "2": ["doc2"]}}
    filters = [{"key": "score", "op": ">", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_less_than():
    metas = {"score": {"10": ["doc1"], "2": ["doc2"]}}
    filters = [{"key": "score", "op": "<", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_greater_than_or_equal():
    metas = {"score": {"5": ["doc1"], "6": ["doc2"], "4": ["doc3"]}}
    filters = [{"key": "score", "op": "≥", "value": "5"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc2"}


def test_less_than_or_equal():
    metas = {"score": {"5": ["doc1"], "6": ["doc2"], "4": ["doc3"]}}
    filters = [{"key": "score", "op": "≤", "value": "5"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc3"}
