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
from configs import VERSION


@pytest.mark.p1
def test_route_not_found_returns_json(rest_client_noauth):
    res = rest_client_noauth.get("/__missing_route__")
    assert res.status_code == 404
    payload = res.json()
    assert payload["code"] == 404, payload
    assert payload["error"] == "Not Found", payload
    assert payload["message"] == f"Not Found: /api/{VERSION}/__missing_route__", payload
