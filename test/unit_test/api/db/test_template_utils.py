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

import pytest

from api.db.template_utils import normalize_canvas_template_categories


@pytest.mark.p2
def test_normalize_canvas_template_categories_legacy_canvas_type():
    payload = {"id": 1, "canvas_type": "Recommended"}

    normalized = normalize_canvas_template_categories(payload)

    assert normalized["canvas_type"] == "Recommended"
    assert normalized["canvas_types"] == ["Recommended"]


@pytest.mark.p2
def test_normalize_canvas_template_categories_with_canvas_types_only():
    payload = {
        "id": 1,
        "canvas_types": ["Recommended", "Agent", "Agent", "  ", 1, None],
    }

    normalized = normalize_canvas_template_categories(payload)

    assert normalized["canvas_type"] == "Recommended"
    assert normalized["canvas_types"] == ["Recommended", "Agent"]


@pytest.mark.p2
def test_normalize_canvas_template_categories_merges_legacy_and_new_field():
    payload = {
        "id": 1,
        "canvas_type": "Marketing",
        "canvas_types": ["Recommended", "Marketing", "Agent"],
    }

    normalized = normalize_canvas_template_categories(payload)

    assert normalized["canvas_type"] == "Marketing"
    assert normalized["canvas_types"] == ["Marketing", "Recommended", "Agent"]


@pytest.mark.p2
def test_normalize_canvas_template_categories_no_valid_categories():
    payload = {"id": 1, "canvas_type": "   ", "canvas_types": [None, 3, "  "]}

    normalized = normalize_canvas_template_categories(payload)

    assert normalized["canvas_type"] is None
    assert normalized["canvas_types"] == []
