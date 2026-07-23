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


@pytest.mark.p2
def test_task_routes_require_auth(rest_client_noauth):
    cancel_res = rest_client_noauth.post("/tasks/missing_task/cancel")
    assert cancel_res.status_code == 401
    cancel_payload = cancel_res.json()
    assert cancel_payload["code"] == 401, cancel_payload

    patch_res = rest_client_noauth.patch("/tasks/missing_task", json={"action": "stop"})
    assert patch_res.status_code == 401
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 401, patch_payload


@pytest.mark.p2
def test_patch_task_rejects_unsupported_action(rest_client):
    res = rest_client.patch("/tasks/missing_task", json={"action": "pause"})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "Only 'stop' is supported" in payload["message"], payload


@pytest.mark.p2
def test_cancel_missing_task_sets_cancel_contract(rest_client):
    res = rest_client.post("/tasks/missing_task/cancel")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
