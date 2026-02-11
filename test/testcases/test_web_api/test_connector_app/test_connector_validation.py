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


CONNECTOR_BASE_URL = f"{HOST_ADDRESS}/{VERSION}/connector"


def _assert_error(payload, *fragments):
    assert payload["code"] != 0, payload
    message = payload.get("message", "").lower()
    for fragment in fragments:
        assert fragment in message, payload


@pytest.mark.p2
def test_google_web_oauth_start_validation_errors(WebApiAuth):
    url = f"{CONNECTOR_BASE_URL}/google/oauth/web/start"

    res = requests.post(
        url,
        params={"type": "bad"},
        auth=WebApiAuth,
        json={"credentials": "{}"},
    )
    _assert_error(res.json(), "invalid", "type")

    res = requests.post(
        url,
        params={"type": "google-drive"},
        auth=WebApiAuth,
        json={"credentials": "not-json"},
    )
    _assert_error(res.json(), "credentials", "json")

    res = requests.post(
        url,
        params={"type": "google-drive"},
        auth=WebApiAuth,
        json={"credentials": {"installed": {}}},
    )
    _assert_error(res.json(), "web", "client")
