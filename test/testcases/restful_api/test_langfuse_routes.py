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
def test_langfuse_api_key_routes_require_auth(rest_client_noauth):
    for method in ("get", "post", "put", "delete"):
        requester = getattr(rest_client_noauth, method)
        kwargs = {"json": {"secret_key": "s", "public_key": "p", "host": "http://example.com"}} if method in {"post", "put"} else {}
        res = requester("/langfuse/api-key", **kwargs)
        assert res.status_code == 401
        payload = res.json()
        assert payload["code"] == 401, (method, payload)


@pytest.mark.p2
def test_langfuse_api_key_missing_required_fields(rest_client):
    res = rest_client.post("/langfuse/api-key", json={"secret_key": "", "public_key": "pub", "host": "http://host"})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] in (101, 102), payload
    assert "required" in payload["message"].lower() or "missing" in payload["message"].lower(), payload
