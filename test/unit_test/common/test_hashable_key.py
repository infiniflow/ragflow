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
"""Tests for ``hashable_key``, the set-backed dedup key used by the RAG merge
paths (dataset_structure_merger).
Malformed provenance values (lists/dicts where a string was expected) must be
usable as set members without raising, and without merging values that aren't
actually equal."""

import pytest

from common.misc_utils import hashable_key


@pytest.mark.p1
def test_hashable_values_pass_through_unchanged():
    for value in ("c1", 7, None, ("a", "b"), frozenset({"x"})):
        assert hashable_key(value) == value


@pytest.mark.p1
def test_unhashable_values_are_usable_as_set_members():
    seen = {hashable_key(["bad", "id"]), hashable_key({"k": "v"})}
    assert hashable_key(["bad", "id"]) in seen
    assert hashable_key(["other", "id"]) not in seen


@pytest.mark.p1
def test_dict_key_order_is_irrelevant():
    assert hashable_key({"a": 1, "b": [2]}) == hashable_key({"b": [2], "a": 1})


@pytest.mark.p1
def test_nested_unequal_dicts_stay_distinct():
    assert hashable_key({"a": {"b": 1}}) != hashable_key({"a": {"b": 2}})


@pytest.mark.p1
def test_list_and_tuple_stay_distinct():
    # ["a"] != ("a",) in Python, so their keys must differ too
    assert hashable_key([{"a": 1}]) != hashable_key(({"a": 1},))


@pytest.mark.p1
def test_nested_set_and_frozenset_share_one_key():
    # {"a"} == frozenset({"a"}) in Python, so once canonicalized their keys must match
    assert hashable_key([{"a"}]) == hashable_key([frozenset({"a"})])


@pytest.mark.p1
def test_canonical_key_never_equals_a_plain_string():
    # the old repr()-based fallback merged an unhashable value with a genuine
    # string that happened to equal its repr()
    assert hashable_key(["bad", "id"]) != repr(["bad", "id"])
    assert hashable_key(["bad", "id"]) != str(["bad", "id"])
