from common.metadata_utils import meta_filter


def test_contains():
    # returns chunk where the metadata contains the value
    metas = {"version": {"hello earth": ["doc1"], "hello mars": ["doc2"]}}
    filters = [{"key": "version", "op": "contains", "value": "earth"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_contains():
    # returns chunk where the metadata does not contain the value
    metas = {"version": {"hello earth": ["doc1"], "hello mars": ["doc2"]}}
    filters = [{"key": "version", "op": "not contains", "value": "earth"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_in_operator():
    # returns chunk where the metadata is in the value
    metas = {"status": {"active": ["doc1"], "pending": ["doc2"], "done": ["doc3"]}}
    filters = [{"key": "status", "op": "in", "value": "active,pending"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc2"}


def test_not_in_operator():
    # returns chunk where the metadata is not in the value
    metas = {"status": {"active": ["doc1"], "pending": ["doc2"], "done": ["doc3"]}}
    filters = [{"key": "status", "op": "not in", "value": "active,pending"}]

    assert meta_filter(metas, filters) == ["doc3"]


def test_start_with():
    # returns chunk where the metadata starts with the value
    metas = {"name": {"prefix_value": ["doc1"], "other": ["doc2"]}}
    filters = [{"key": "name", "op": "start with", "value": "pre"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_end_with():
    # returns chunk where the metadata ends with the value
    metas = {"file": {"report.pdf": ["doc1"], "image.png": ["doc2"]}}
    filters = [{"key": "file", "op": "end with", "value": ".pdf"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_empty():
    # returns chunk where the metadata is empty
    metas = {"notes": {"": ["doc1"], "non-empty": ["doc2"]}}
    filters = [{"key": "notes", "op": "empty", "value": ""}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_empty():
    # returns chunk where the metadata is not empty
    metas = {"notes": {"": ["doc1"], "non-empty": ["doc2"]}}
    filters = [{"key": "notes", "op": "not empty", "value": ""}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_equal():
    # returns chunk where the metadata is equal to the value
    metas = {"score": {"5": ["doc1"], "6": ["doc2"]}}
    filters = [{"key": "score", "op": "=", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_not_equal():
    # returns chunk where the metadata is not equal to the value
    metas = {"score": {"5": ["doc1"], "6": ["doc2"]}}
    filters = [{"key": "score", "op": "≠", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_greater_than():
    # returns chunk where the metadata is greater than the value
    metas = {"score": {"10": ["doc1"], "2": ["doc2"]}}
    filters = [{"key": "score", "op": ">", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc1"]


def test_less_than():
    # returns chunk where the metadata is less than the value
    metas = {"score": {"10": ["doc1"], "2": ["doc2"]}}
    filters = [{"key": "score", "op": "<", "value": "5"}]

    assert meta_filter(metas, filters) == ["doc2"]


def test_greater_than_or_equal():
    # returns chunk where the metadata is greater than or equal to the value
    metas = {"score": {"5": ["doc1"], "6": ["doc2"], "4": ["doc3"]}}
    filters = [{"key": "score", "op": "≥", "value": "5"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc2"}


def test_less_than_or_equal():
    # returns chunk where the metadata is less than or equal to the value
    metas = {"score": {"5": ["doc1"], "6": ["doc2"], "4": ["doc3"]}}
    filters = [{"key": "score", "op": "≤", "value": "5"}]

    assert set(meta_filter(metas, filters)) == {"doc1", "doc3"}
