#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Unit tests for api.utils.pagination_utils."""

import pytest

from api.utils.pagination_utils import (
    REST_API_MAX_PAGE_SIZE,
    parse_page_number,
    parse_page_size,
    parse_pagination,
    validate_rest_api_page_size,
)


# --- validate_rest_api_page_size (existing behaviour preserved) ---


@pytest.mark.p2
def test_validate_rest_api_page_size_within_limit():
    assert validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE) == REST_API_MAX_PAGE_SIZE


@pytest.mark.p2
def test_validate_rest_api_page_size_above_limit_raises():
    with pytest.raises(ValueError):
        validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE + 1)


# --- parse_page_number ---


@pytest.mark.p2
def test_parse_page_number_default_when_absent():
    assert parse_page_number({}, default=1) == 1
    assert parse_page_number({}, default=0) == 0


@pytest.mark.p2
def test_parse_page_number_parses_string():
    assert parse_page_number({"page": "5"}) == 5


@pytest.mark.p2
def test_parse_page_number_zero_is_valid_sentinel():
    assert parse_page_number({"page": "0"}) == 0


@pytest.mark.p2
def test_parse_page_number_negative_raises():
    with pytest.raises(ValueError):
        parse_page_number({"page": "-5"})


@pytest.mark.p2
def test_parse_page_number_non_integer_raises():
    with pytest.raises(ValueError):
        parse_page_number({"page": "abc"})


# --- parse_page_size ---


@pytest.mark.p2
def test_parse_page_size_default_when_absent():
    assert parse_page_size({}, default=30) == 30
    assert parse_page_size({}, default=0) == 0


@pytest.mark.p2
def test_parse_page_size_parses_string():
    assert parse_page_size({"page_size": "15"}) == 15


@pytest.mark.p2
def test_parse_page_size_zero_is_valid_sentinel():
    assert parse_page_size({"page_size": "0"}) == 0


@pytest.mark.p2
def test_parse_page_size_negative_raises():
    with pytest.raises(ValueError):
        parse_page_size({"page_size": "-1"})


@pytest.mark.p2
def test_parse_page_size_non_integer_raises():
    with pytest.raises(ValueError):
        parse_page_size({"page_size": "20.5"})


@pytest.mark.p2
def test_parse_page_size_above_max_raises():
    with pytest.raises(ValueError):
        parse_page_size({"page_size": str(REST_API_MAX_PAGE_SIZE + 1)})


@pytest.mark.p2
def test_parse_page_size_at_max_allowed():
    assert parse_page_size({"page_size": str(REST_API_MAX_PAGE_SIZE)}) == REST_API_MAX_PAGE_SIZE


# --- parse_pagination ---


@pytest.mark.p2
def test_parse_pagination_returns_tuple_with_defaults():
    assert parse_pagination({}, default_page=1, default_page_size=30) == (1, 30)


@pytest.mark.p2
def test_parse_pagination_parses_both_values():
    assert parse_pagination({"page": "2", "page_size": "50"}, default_page=1, default_page_size=30) == (2, 50)


@pytest.mark.p2
def test_parse_pagination_preserves_zero_sentinel_defaults():
    assert parse_pagination({}, default_page=0, default_page_size=0) == (0, 0)


@pytest.mark.p2
def test_parse_pagination_rejects_negative_page():
    with pytest.raises(ValueError):
        parse_pagination({"page": "-1"}, default_page=1, default_page_size=30)


@pytest.mark.p2
def test_parse_pagination_rejects_oversized_page_size():
    with pytest.raises(ValueError):
        parse_pagination({"page_size": str(REST_API_MAX_PAGE_SIZE + 1)}, default_page=1, default_page_size=30)


@pytest.mark.p2
def test_parse_pagination_accepts_int_valued_args():
    # JSON-parsed request bodies may already contain ints rather than strings.
    assert parse_pagination({"page": 3, "page_size": 25}, default_page=1, default_page_size=30) == (3, 25)
