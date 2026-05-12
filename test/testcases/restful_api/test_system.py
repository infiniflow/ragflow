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


@pytest.mark.p1
def test_system_ping(rest_client):
    res = rest_client.get("/system/ping")
    assert res.status_code == 200
    assert res.text == "pong"


@pytest.mark.p1
def test_system_version(rest_client):
    res = rest_client.get("/system/version")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"], payload


@pytest.mark.p1
def test_system_status_requires_auth(rest_client_noauth):
    res = rest_client_noauth.get("/system/status")
    assert res.status_code == 401
    payload = res.json()
    assert payload["code"] == 401, payload
    assert "Unauthorized" in payload["message"], payload
