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
from typing import Any

logger = logging.getLogger(__name__)


def _collect_canvas_types(canvas_type: Any, canvas_types: Any) -> list[str]:
    categories: list[str] = []

    if isinstance(canvas_type, str):
        category = canvas_type.strip()
        if category:
            categories.append(category)

    iterable_types: list[Any]
    if isinstance(canvas_types, list):
        iterable_types = canvas_types
    elif canvas_types is None:
        iterable_types = []
    else:
        iterable_types = [canvas_types]

    for item in iterable_types:
        if not isinstance(item, str):
            continue
        category = item.strip()
        if not category:
            continue
        categories.append(category)

    deduplicated: list[str] = []
    seen: set[str] = set()
    for category in categories:
        if category in seen:
            continue
        seen.add(category)
        deduplicated.append(category)

    return deduplicated


def normalize_canvas_template_categories(template: dict[str, Any]) -> dict[str, Any]:
    normalized = dict(template)
    raw_canvas_type = normalized.get("canvas_type")
    raw_canvas_types = normalized.get("canvas_types")
    canvas_types = _collect_canvas_types(
        raw_canvas_type,
        raw_canvas_types,
    )
    normalized["canvas_types"] = canvas_types
    normalized["canvas_type"] = canvas_types[0] if canvas_types else None
    if raw_canvas_type != normalized["canvas_type"] or raw_canvas_types != normalized["canvas_types"]:
        logger.debug(
            "Normalized canvas categories for template_id=%s: canvas_type=%r -> %r, canvas_types=%r -> %r",
            normalized.get("id"),
            raw_canvas_type,
            normalized["canvas_type"],
            raw_canvas_types,
            normalized["canvas_types"],
        )
    return normalized
