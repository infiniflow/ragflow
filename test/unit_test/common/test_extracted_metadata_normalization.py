#
# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

from common.metadata_utils import normalize_extracted_metadata


def test_normalize_extracted_metadata_keeps_flat_string_values():
    result = normalize_extracted_metadata(
        """```json
        {
          "name": "robot",
          "year": 2026,
          "ratio": 1.25,
          "approved": true,
          "tags": ["finance", 7, "robotics"],
          "nested": {"ignored": true},
          "missing": null
        }
        ```"""
    )

    assert result == {
        "name": "robot",
        "year": "2026",
        "ratio": "1.25",
        "approved": "True",
        "tags": ["finance", "robotics"],
    }


def test_normalize_extracted_metadata_rejects_non_object_json():
    assert normalize_extracted_metadata("{}") == {}
    assert normalize_extracted_metadata("[]") == {}
    assert normalize_extracted_metadata("not-json") == {}


def test_normalize_extracted_metadata_rejects_non_string_keys():
    assert normalize_extracted_metadata({1: "ignored", "name": "robot"}) == {"name": "robot"}
