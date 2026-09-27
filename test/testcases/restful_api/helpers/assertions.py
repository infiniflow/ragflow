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

from test.testcases.configs import IS_GO_PROXY


def assert_auth_error(payload: dict, scenario_name: str) -> None:
    assert payload["code"] == 401, (scenario_name, payload)
    if IS_GO_PROXY:
        if scenario_name == "missing token":
            assert payload["message"] == "Missing Authorization header", (scenario_name, payload)
        else:
            assert payload["message"] in {"Invalid access token", "Invalid auth credentials"}, (scenario_name, payload)
        return
    else:
        expected = "<Unauthorized '401: Unauthorized'>"
    assert payload["message"] == expected, (scenario_name, payload)
