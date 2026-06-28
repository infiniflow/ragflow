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

import logging
from typing import Any, Mapping, Tuple

REST_API_MAX_PAGE_SIZE = 100

DEFAULT_PAGE_NUMBER = 1
DEFAULT_PAGE_SIZE = 30

logger = logging.getLogger(__name__)


def validate_rest_api_page_size(page_size: int) -> int:
    """Validate REST API page_size values against the public maximum."""
    if page_size > REST_API_MAX_PAGE_SIZE:
        raise ValueError(f"page_size must be less than or equal to {REST_API_MAX_PAGE_SIZE}")
    return page_size


def _parse_non_negative_int(args: Mapping[str, Any], key: str, default: int) -> int:
    """Parse ``args[key]`` as a non-negative integer, falling back to ``default``.

    ``0`` is intentionally allowed: REST handlers treat it as the "return all
    results" sentinel. Non-integer or negative inputs raise ``ValueError`` -- the
    same exception type the bare ``int(...)`` call sites already surface -- so
    existing error handling keeps working while ``?page=-5`` style values are no
    longer silently passed through to the service/DB layer.
    """
    raw = args.get(key, default)
    try:
        value = int(raw)
    except (TypeError, ValueError):
        logger.warning("Invalid pagination argument: %s=%r is not an integer", key, raw)
        raise ValueError(f"{key} must be an integer")
    if value < 0:
        logger.warning("Invalid pagination argument: %s=%r is negative", key, raw)
        raise ValueError(f"{key} must be greater than or equal to 0")
    return value


def parse_page_number(args: Mapping[str, Any], default: int = DEFAULT_PAGE_NUMBER) -> int:
    """Parse and validate the ``page`` argument from a request mapping."""
    return _parse_non_negative_int(args, "page", default)


def parse_page_size(args: Mapping[str, Any], default: int = DEFAULT_PAGE_SIZE) -> int:
    """Parse the ``page_size`` argument and validate it against the public maximum."""
    return validate_rest_api_page_size(_parse_non_negative_int(args, "page_size", default))


def parse_pagination(
    args: Mapping[str, Any],
    *,
    default_page: int = DEFAULT_PAGE_NUMBER,
    default_page_size: int = DEFAULT_PAGE_SIZE,
) -> Tuple[int, int]:
    """Return a validated ``(page, page_size)`` tuple from a request mapping.

    ``args`` may be ``request.args`` (query string) or a parsed JSON body / dict;
    both expose ``.get``. Per-call-site defaults are preserved by the caller.
    """
    return parse_page_number(args, default_page), parse_page_size(args, default_page_size)
