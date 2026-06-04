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

REST_API_MAX_PAGE_SIZE = 100

DEFAULT_PAGE_NUMBER = 1
DEFAULT_PAGE_SIZE = 30


def validate_rest_api_page_size(page_size: int) -> int:
    """Validate REST API page_size values against the public maximum."""
    if page_size > REST_API_MAX_PAGE_SIZE:
        raise ValueError(f"page_size must be less than or equal to {REST_API_MAX_PAGE_SIZE}")
    return page_size


def _coerce_non_negative_int(value, field_name: str) -> int:
    """Coerce a pagination argument to a non-negative int or raise ValueError.

    Raising ``ValueError`` keeps the same exception type the call sites already
    surface from ``int(...)``, so existing error handling is preserved while
    silently-accepted negatives are now rejected.
    """
    try:
        coerced = int(value)
    except (TypeError, ValueError):
        raise ValueError(f"{field_name} must be an integer")
    if coerced < 0:
        raise ValueError(f"{field_name} must be greater than or equal to 0")
    return coerced


def parse_page_number(args, default: int = DEFAULT_PAGE_NUMBER) -> int:
    """Parse and validate the ``page`` argument from a request-args mapping."""
    return _coerce_non_negative_int(args.get("page", default), "page")


def parse_page_size(args, default: int = DEFAULT_PAGE_SIZE) -> int:
    """Parse ``page_size`` and validate it against the public maximum."""
    page_size = _coerce_non_negative_int(args.get("page_size", default), "page_size")
    return validate_rest_api_page_size(page_size)


def parse_pagination(args, *, default_page: int = DEFAULT_PAGE_NUMBER, default_page_size: int = DEFAULT_PAGE_SIZE) -> tuple[int, int]:
    """Parse ``page`` and ``page_size`` from a request-args mapping in one call.

    ``args`` is any mapping exposing ``get(key, default)`` (e.g. ``request.args``
    or a plain dict). Returns a ``(page, page_size)`` tuple; both values are
    validated as non-negative integers and ``page_size`` is additionally capped
    at :data:`REST_API_MAX_PAGE_SIZE`.
    """
    return (
        parse_page_number(args, default_page),
        parse_page_size(args, default_page_size),
    )
