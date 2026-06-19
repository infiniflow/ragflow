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

"""Unit tests for api.utils.pagination_utils REST pagination parsing."""

import pytest

from api.utils.pagination_utils import (
    REST_API_MAX_PAGE_SIZE,
    parse_page_number,
    parse_page_size,
    parse_pagination,
    validate_rest_api_page_size,
)


class TestValidateRestApiPageSize:
    def test_within_limit_is_returned(self):
        assert validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE) == REST_API_MAX_PAGE_SIZE

    def test_above_limit_raises(self):
        with pytest.raises(ValueError, match="less than or equal to"):
            validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE + 1)


class TestParsePageNumber:
    def test_present_value_is_parsed(self):
        assert parse_page_number({"page": "5"}) == 5

    def test_missing_value_uses_default(self):
        assert parse_page_number({}, default=0) == 0
        assert parse_page_number({}) == 1

    def test_zero_is_allowed(self):
        assert parse_page_number({"page": "0"}, default=1) == 0

    def test_negative_raises(self):
        with pytest.raises(ValueError, match="greater than or equal to 0"):
            parse_page_number({"page": "-1"})

    @pytest.mark.parametrize("bad", ["abc", "", "1.5", None])
    def test_non_integer_raises(self, bad):
        with pytest.raises(ValueError, match="must be an integer"):
            parse_page_number({"page": bad})


class TestParsePageSize:
    def test_present_value_is_parsed(self):
        assert parse_page_size({"page_size": "50"}) == 50

    def test_missing_value_uses_default(self):
        assert parse_page_size({}, default=15) == 15

    def test_zero_sentinel_is_allowed(self):
        assert parse_page_size({"page_size": "0"}, default=0) == 0

    def test_above_max_raises(self):
        with pytest.raises(ValueError, match="less than or equal to"):
            parse_page_size({"page_size": str(REST_API_MAX_PAGE_SIZE + 1)})

    def test_negative_raises(self):
        with pytest.raises(ValueError, match="greater than or equal to 0"):
            parse_page_size({"page_size": "-1"})


class TestParsePagination:
    def test_returns_page_and_page_size_tuple(self):
        assert parse_pagination({"page": "2", "page_size": "15"}, default_page=1, default_page_size=30) == (2, 15)

    def test_applies_per_call_defaults(self):
        assert parse_pagination({}, default_page=0, default_page_size=0) == (0, 0)

    def test_page_size_is_capped(self):
        with pytest.raises(ValueError, match="less than or equal to"):
            parse_pagination({"page_size": str(REST_API_MAX_PAGE_SIZE + 1)})
