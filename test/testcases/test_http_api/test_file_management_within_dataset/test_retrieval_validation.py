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
import pytest
from common import retrieval_chunks


@pytest.mark.p2
@pytest.mark.parametrize(
    "payload, expected_fragment",
    [
        ({}, "required"),
        ({"dataset_ids": "bad"}, "list"),
    ],
    ids=["missing_dataset_ids", "dataset_ids_not_list"],
)
def test_retrieval_validation_missing_dataset_ids(HttpApiAuth, payload, expected_fragment):
    res = retrieval_chunks(HttpApiAuth, payload)
    assert res["code"] != 0, res
    message = res.get("message", "").lower()
    assert "dataset_ids" in message, res
    assert expected_fragment in message, res
