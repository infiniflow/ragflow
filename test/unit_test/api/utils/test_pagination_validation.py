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

"""Unit tests for REST API pagination guards in api/utils/pagination_utils.py."""

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


class TestValidateRestApiPage:
    def test_valid_int_and_numeric_string_pass_through(self):
        """Valid page values pass through without coercion surprises."""
        assert validate_rest_api_page(1) == 1
        assert validate_rest_api_page(7) == 7
        assert validate_rest_api_page("3") == 3

    def test_zero_and_negative_fall_back_to_default(self):
        """Non-positive page values use the documented default."""
        assert validate_rest_api_page(0) == DEFAULT_PAGE
        assert validate_rest_api_page(-4) == DEFAULT_PAGE
        assert validate_rest_api_page("-2") == DEFAULT_PAGE

    def test_non_numeric_falls_back_to_default(self):
        """Invalid page values use the documented default."""
        assert validate_rest_api_page("abc") == DEFAULT_PAGE
        assert validate_rest_api_page("") == DEFAULT_PAGE
        assert validate_rest_api_page(None) == DEFAULT_PAGE
        assert validate_rest_api_page([1]) == DEFAULT_PAGE

    def test_default_page_is_one(self):
        """The REST API default page remains the first page."""
        assert DEFAULT_PAGE == 1


class TestValidateRestApiPageSize:
    def test_valid_sizes_pass_through(self):
        """Valid page sizes pass through unchanged."""
        assert validate_rest_api_page_size(1) == 1
        assert validate_rest_api_page_size(30) == 30
        assert validate_rest_api_page_size("25") == 25

    def test_boundary_max_is_accepted(self):
        """The documented maximum page size is accepted."""
        assert REST_API_MAX_PAGE_SIZE == 100
        assert validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE) == REST_API_MAX_PAGE_SIZE

    def test_over_max_raises(self):
        """Page sizes above the documented maximum are rejected."""
        with pytest.raises(ValueError, match="less than or equal to"):
            validate_rest_api_page_size(REST_API_MAX_PAGE_SIZE + 1)
        with pytest.raises(ValueError, match="less than or equal to"):
            validate_rest_api_page_size(1000)

    def test_non_positive_and_non_numeric_fall_back_to_default(self):
        """Invalid page-size values use the documented default."""
        assert validate_rest_api_page_size(0) == DEFAULT_PAGE_SIZE
        assert validate_rest_api_page_size(-10) == DEFAULT_PAGE_SIZE
        assert validate_rest_api_page_size("abc") == DEFAULT_PAGE_SIZE
        assert validate_rest_api_page_size(None) == DEFAULT_PAGE_SIZE

    def test_default_page_size_is_thirty(self):
        """The REST API default page size remains thirty."""
        assert DEFAULT_PAGE_SIZE == 30


class TestValidateRestApiIds:
    def test_none_and_empty_pass_through(self):
        """Missing and empty ID filters remain valid."""
        assert validate_rest_api_ids(None) is None
        assert validate_rest_api_ids([]) == []

    def test_within_limit_passes_through_unchanged(self):
        """An ID list at the limit is returned unchanged."""
        ids = [f"id-{i}" for i in range(REST_API_MAX_IDS)]
        assert validate_rest_api_ids(ids) == ids

    def test_over_limit_raises_with_default_field_name(self):
        """An oversized ID list reports the default field name."""
        ids = [f"id-{i}" for i in range(REST_API_MAX_IDS + 1)]
        with pytest.raises(ValueError, match="ids must contain at most"):
            validate_rest_api_ids(ids)

    def test_over_limit_raises_with_custom_field_name(self):
        """An oversized ID list honors a custom field name."""
        ids = [f"id-{i}" for i in range(REST_API_MAX_IDS + 1)]
        with pytest.raises(ValueError, match="documents must contain at most"):
            validate_rest_api_ids(ids, field_name="documents")

    def test_max_ids_is_one_hundred(self):
        """The REST API ID-list limit remains one hundred."""
        assert REST_API_MAX_IDS == 100
