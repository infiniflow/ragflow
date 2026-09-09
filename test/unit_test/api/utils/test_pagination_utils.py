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
"""Unit tests for the shared REST pagination helpers (#15657).

Every REST list endpoint routes its `page`, `page_size` and `*_ids` arguments
through `api/utils/pagination_utils.py`, so the module's contract is the
contract of the whole public list surface. That contract is deliberately
asymmetric: unusable input falls back to the default silently, while input that
exceeds a public cap raises. Only the raising half is covered today
(`test_doc_validation.py` for page_size, `test_list_memory_filters.py` for ids,
both through other layers); these tests pin the module itself, fallbacks
included.
"""

import pytest

from api.utils.pagination_utils import (
    DEFAULT_PAGE,
    DEFAULT_PAGE_SIZE,
    REST_API_MAX_IDS,
    REST_API_MAX_PAGE_SIZE,
    validate_rest_api_ids,
    validate_rest_api_page,
    validate_rest_api_page_size,
)

pytestmark = pytest.mark.p2


@pytest.mark.parametrize(
    "raw, expected",
    [
        ("7", 7),
        (7, 7),
        (" 7 ", 7),
        (2.9, 2),  # floats truncate rather than round
    ],
)
def test_validate_rest_api_page_accepts_usable_values(raw, expected):
    assert validate_rest_api_page(raw) == expected


@pytest.mark.parametrize("raw", [0, -3, "0", "-3", "abc", "", None, [], {}])
def test_validate_rest_api_page_falls_back_to_the_default(raw):
    """Unparseable or out-of-range pages fall back instead of raising.

    Query strings are user input, so `?page=abc` and `?page=-5` must not reach
    the service layer as an error or as a negative offset.
    """
    assert validate_rest_api_page(raw) == DEFAULT_PAGE


@pytest.mark.parametrize(
    "raw, expected",
    [
        ("50", 50),
        (50, 50),
        (1, 1),
        (REST_API_MAX_PAGE_SIZE, REST_API_MAX_PAGE_SIZE),
    ],
)
def test_validate_rest_api_page_size_accepts_usable_values(raw, expected):
    assert validate_rest_api_page_size(raw) == expected


@pytest.mark.parametrize("raw", [0, -1, "0", "abc", "", None, [], {}])
def test_validate_rest_api_page_size_falls_back_to_the_default(raw):
    assert validate_rest_api_page_size(raw) == DEFAULT_PAGE_SIZE


def test_validate_rest_api_page_size_rejects_values_above_the_cap():
    """The cap is the one case that raises: silently shrinking an oversized
    page_size would hand back fewer rows than asked for with no signal."""
    with pytest.raises(ValueError, match=f"page_size must be less than or equal to {REST_API_MAX_PAGE_SIZE}"):
        validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE + 1)


def test_validate_rest_api_ids_passes_through_within_the_cap():
    assert validate_rest_api_ids(None) is None
    assert validate_rest_api_ids([]) == []

    at_limit = [f"id-{i}" for i in range(REST_API_MAX_IDS)]
    assert validate_rest_api_ids(at_limit) is at_limit  # returned unchanged, not copied


def test_validate_rest_api_ids_rejects_lists_above_the_cap():
    over_limit = [f"id-{i}" for i in range(REST_API_MAX_IDS + 1)]
    with pytest.raises(ValueError, match=f"ids must contain at most {REST_API_MAX_IDS} IDs"):
        validate_rest_api_ids(over_limit)


def test_validate_rest_api_ids_names_the_offending_field():
    """Endpoints validate several id lists per request, so the message has to
    say which one was too long."""
    over_limit = [f"id-{i}" for i in range(REST_API_MAX_IDS + 1)]
    with pytest.raises(ValueError, match=f"owner_ids must contain at most {REST_API_MAX_IDS} IDs"):
        validate_rest_api_ids(over_limit, field_name="owner_ids")
