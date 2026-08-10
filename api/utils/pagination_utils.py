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

DEFAULT_PAGE = 1
DEFAULT_PAGE_SIZE = 30
REST_API_MAX_PAGE_SIZE = 100


def validate_rest_api_page(page) -> int:
    """Validate page, if invalid, silent fallback to default page"""
    try:
        int_page = int(page)
    except (TypeError, ValueError):
        return DEFAULT_PAGE
    if int_page < 1:
        return DEFAULT_PAGE
    return int_page


def validate_rest_api_page_size(page_size) -> int:
    """Validate page_size, if invalid, silent fallback to default page_size, and validate it against the public maximum."""
    try:
        int_page_size = int(page_size)
    except (TypeError, ValueError):
        return DEFAULT_PAGE_SIZE
    if int_page_size < 1:
        return DEFAULT_PAGE_SIZE
    if int_page_size > REST_API_MAX_PAGE_SIZE:
        raise ValueError(f"page_size must be less than or equal to {REST_API_MAX_PAGE_SIZE}")
    return int_page_size
