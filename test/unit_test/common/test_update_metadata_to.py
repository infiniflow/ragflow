#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#

from common.metadata_utils import update_metadata_to


def test_update_metadata_to_merges_string_lists():
    base = {"tags": ["a"]}
    update_metadata_to(base, {"tags": ["a", "b"]})
    assert base["tags"] == ["a", "b"]


def test_update_metadata_to_preserves_bool_int_when_merging_existing():
    # Same call shape as outline persist: new fields first, existing doc meta second.
    merged = update_metadata_to(
        {"outline": [{"title": "Ch1", "depth": 0}]},
        {
            "_isCurrent": True,
            "_version": 1,
            "_processStatus": 4,
            "category_code": "02",
        },
    )
    assert merged["_isCurrent"] is True
    assert merged["_version"] == 1
    assert merged["_processStatus"] == 4
    assert merged["category_code"] == "02"
    assert merged["outline"] == [{"title": "Ch1", "depth": 0}]


def test_update_metadata_to_keeps_first_outline_when_existing_also_has_outline():
    merged = update_metadata_to(
        {"outline": [{"title": "new", "depth": 0}]},
        {"outline": [{"title": "old", "depth": 1}], "_version": 2},
    )
    assert merged["outline"] == [{"title": "new", "depth": 0}]
    assert merged["_version"] == 2


def test_update_metadata_to_skips_empty_string_list():
    base = {"tags": ["a"]}
    update_metadata_to(base, {"tags": [], "other": []})
    assert base == {"tags": ["a"]}


def test_update_metadata_to_preserves_float_and_none():
    merged = update_metadata_to({}, {"score": 1.5, "note": None})
    assert merged["score"] == 1.5
    assert merged["note"] is None


def test_update_metadata_to_preserves_dict_when_absent():
    merged = update_metadata_to({}, {"extra": {"a": 1}})
    assert merged["extra"] == {"a": 1}


def test_update_metadata_to_skips_dict_when_present():
    base = {"extra": {"a": 1}}
    update_metadata_to(base, {"extra": {"b": 2}})
    assert base["extra"] == {"a": 1}


def test_update_metadata_to_inserts_structured_list_when_absent():
    merged = update_metadata_to({}, {"outline": [{"title": "Ch1", "depth": 0}]})
    assert merged["outline"] == [{"title": "Ch1", "depth": 0}]


def test_update_metadata_to_does_not_append_string_to_structured_list():
    base = {"outline": [{"title": "Ch1", "depth": 0}]}
    update_metadata_to(base, {"outline": "chapter"})
    assert base["outline"] == [{"title": "Ch1", "depth": 0}]


def test_update_metadata_to_does_not_extend_structured_list_with_strings():
    base = {"outline": [{"title": "Ch1", "depth": 0}]}
    update_metadata_to(base, {"outline": ["a", "b"]})
    assert base["outline"] == [{"title": "Ch1", "depth": 0}]
