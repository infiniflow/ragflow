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
"""``normalize_doc_id_filter`` must never hand ``[]`` to the retriever.

``Dealer.get_filters`` emits a ``doc_id`` condition whenever ``doc_ids is not
None``, and an empty condition value is useless at every backend: ES,
OpenSearch, Infinity, OceanBase and SereneDB drop the predicate (widening the
search to the whole dataset), and GaussDB raises. Only ``None`` means "no
document filter".
"""

import logging

import pytest

from common.metadata_utils import normalize_doc_id_filter

pytestmark = pytest.mark.p2


@pytest.mark.parametrize(
    "doc_ids",
    [
        None,
        [],
        [""],
        ["", ""],
        [None],
        ["", None],
    ],
    ids=["none", "empty", "one-blank", "all-blank", "one-none", "blank-and-none"],
)
def test_no_usable_ids_collapses_to_none(doc_ids):
    assert normalize_doc_id_filter(doc_ids) is None


@pytest.mark.parametrize(
    ("doc_ids", "expected"),
    [
        (["doc-1"], ["doc-1"]),
        (["doc-1", "doc-2"], ["doc-1", "doc-2"]),
        (["doc-1", "", "doc-2"], ["doc-1", "doc-2"]),
        (["", "doc-1"], ["doc-1"]),
        (["doc-1", None], ["doc-1"]),
    ],
    ids=["single", "pair", "blank-in-middle", "leading-blank", "trailing-none"],
)
def test_usable_ids_are_kept_in_order_without_blanks(doc_ids, expected):
    assert normalize_doc_id_filter(doc_ids) == expected


@pytest.mark.parametrize(
    ("doc_ids", "expected"),
    [
        (["  "], ["  "]),
        (["\t"], ["\t"]),
        (["\n"], ["\n"]),
        (["doc-1", "  "], ["doc-1", "  "]),
        (["  ", ""], ["  "]),
        ([" doc-2"], [" doc-2"]),
    ],
    ids=["spaces", "tab", "newline", "mixed-with-real", "whitespace-and-empty", "padded-id"],
)
def test_whitespace_only_ids_are_kept_not_dropped(doc_ids, expected):
    """Whitespace is a value, not an absence.

    Only *empty* ids are dropped, because those come from splitting a
    comma-separated field. A whitespace-only id is a real (if unmatchable) id:
    it yields no results, whereas dropping it to ``None`` would silently widen
    the request to every document in the dataset. Ids are also never rewritten,
    so a padded ``" doc-2"`` is forwarded exactly as supplied.
    """
    assert normalize_doc_id_filter(doc_ids) == expected


def test_dropping_ids_logs_counts_only(caplog):
    """Diagnosable without leaking caller data: counts are logged, ids are not."""
    with caplog.at_level(logging.DEBUG, logger="root"):
        assert normalize_doc_id_filter(["secret-doc-1", "", "secret-doc-2", ""]) == ["secret-doc-1", "secret-doc-2"]

    messages = [record.getMessage() for record in caplog.records]
    assert any("dropped 2 empty id(s) of 4 requested" in message for message in messages), messages
    assert not any("secret-doc" in message for message in messages), messages


def test_no_log_when_nothing_is_dropped(caplog):
    with caplog.at_level(logging.DEBUG, logger="root"):
        normalize_doc_id_filter(["doc-1", "doc-2"])

    assert not [record for record in caplog.records if "normalize_doc_id_filter" in record.getMessage()]


def test_result_is_never_an_empty_list():
    """The invariant the helper exists to hold, stated directly."""
    for doc_ids in (None, [], [""], ["", ""], ["a"], ["", "a"]):
        assert normalize_doc_id_filter(doc_ids) != []


def test_input_is_not_mutated():
    doc_ids = ["doc-1", "", "doc-2"]
    normalize_doc_id_filter(doc_ids)
    assert doc_ids == ["doc-1", "", "doc-2"]
