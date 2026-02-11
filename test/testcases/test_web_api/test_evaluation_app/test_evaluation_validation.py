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
import pytest
import requests

from configs import HOST_ADDRESS, VERSION


EVALUATION_BASE_URL = f"{HOST_ADDRESS}/{VERSION}/evaluation"


def _assert_error(payload, *fragments):
    assert payload["code"] != 0, payload
    message = payload.get("message", "").lower()
    for fragment in fragments:
        assert fragment in message, payload


@pytest.mark.p2
def test_evaluation_validation_errors(WebApiAuth):
    res = requests.post(
        f"{EVALUATION_BASE_URL}/dataset/create",
        auth=WebApiAuth,
        json={"name": " ", "kb_ids": []},
    )
    _assert_error(res.json(), "name")

    res = requests.post(
        f"{EVALUATION_BASE_URL}/dataset/create",
        auth=WebApiAuth,
        json={"name": "valid", "kb_ids": "bad"},
    )
    _assert_error(res.json(), "kb_ids")

    res = requests.post(
        f"{EVALUATION_BASE_URL}/dataset/invalid/case/import",
        auth=WebApiAuth,
        json={"cases": ""},
    )
    _assert_error(res.json(), "cases")

    res = requests.post(
        f"{EVALUATION_BASE_URL}/compare",
        auth=WebApiAuth,
        json={"run_ids": ["only-one"]},
    )
    _assert_error(res.json(), "run_ids")
