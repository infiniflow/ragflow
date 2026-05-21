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


@pytest.mark.p3
def test_route_not_found_returns_json():
    url = f"{HOST_ADDRESS}/api/{VERSION}/__missing_route__"
    res = requests.get(url)
    assert res.status_code == 404
    payload = res.json()
    assert payload["error"] == "Not Found"
    assert f"/api/{VERSION}/__missing_route__" in payload["message"]
