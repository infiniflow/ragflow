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

import json
import uuid

import pytest


@pytest.fixture
def search_resource(rest_client):
    name = f"restful_search_{uuid.uuid4().hex[:8]}"
    create_res = rest_client.post("/searches", json={"name": name, "description": "restful search"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    search_id = create_payload["data"]["search_id"]

    try:
        yield search_id
    finally:
        delete_res = rest_client.delete(f"/searches/{search_id}")
        assert delete_res.status_code == 200, delete_res.text
        delete_payload = delete_res.json()
        assert delete_payload["code"] in (0, 109), delete_payload


def _sse_events(response_text: str) -> list[str]:
    return [line[5:] for line in response_text.splitlines() if line.startswith("data:")]


@pytest.mark.p2
def test_search_routes_require_auth(rest_client_noauth):
    create_res = rest_client_noauth.post("/searches", json={"name": "search_noauth"})
    assert create_res.status_code == 401
    create_payload = create_res.json()
    assert create_payload["code"] == 401, create_payload

    list_res = rest_client_noauth.get("/searches")
    assert list_res.status_code == 401
    list_payload = list_res.json()
    assert list_payload["code"] == 401, list_payload


@pytest.mark.p2
def test_search_crud_contract(rest_client, search_resource):
    search_id = search_resource

    list_res = rest_client.get("/searches")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert any(item.get("id") == search_id for item in list_payload["data"]["search_apps"]), list_payload

    detail_res = rest_client.get(f"/searches/{search_id}")
    assert detail_res.status_code == 200
    detail_payload = detail_res.json()
    assert detail_payload["code"] == 0, detail_payload
    assert detail_payload["data"]["id"] == search_id, detail_payload

    new_name = f"search_updated_{uuid.uuid4().hex[:6]}"
    update_res = rest_client.put(
        f"/searches/{search_id}",
        json={"name": new_name, "search_config": {"top_k": 3}},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["name"] == new_name, update_payload


@pytest.mark.p2
def test_search_create_invalid_name(rest_client):
    res = rest_client.post("/searches", json={"name": ""})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "empty" in payload["message"], payload


@pytest.mark.p2
def test_search_update_invalid_search_id(rest_client):
    res = rest_client.put(
        "/searches/invalid_search_id",
        json={"name": "invalid", "search_config": {}},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 109, payload
    assert "No authorization" in payload["message"], payload


@pytest.mark.p2
def test_search_completion_requires_question(rest_client, search_resource):
    search_id = search_resource

    completion_res = rest_client.post(f"/searches/{search_id}/completion", json={})
    assert completion_res.status_code == 200
    completion_payload = completion_res.json()
    assert completion_payload["code"] == 101, completion_payload
    assert "required argument are missing: question" in completion_payload["message"], completion_payload

    completions_res = rest_client.post(f"/searches/{search_id}/completions", json={})
    assert completions_res.status_code == 200
    completions_payload = completions_res.json()
    assert completions_payload["code"] == 101, completions_payload
    assert "required argument are missing: question" in completions_payload["message"], completions_payload


@pytest.mark.p2
def test_search_completion_requires_kb_ids(rest_client, search_resource):
    search_id = search_resource
    for path in ("completion", "completions"):
        res = rest_client.post(
            f"/searches/{search_id}/{path}",
            json={"question": "what is coriander?"},
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 102, payload
        assert "`kb_ids` is required" in payload["message"], payload


@pytest.mark.p2
def test_search_completion_sse_shape_when_kb_ids_provided(rest_client, search_resource):
    search_id = search_resource
    # Even with kb_ids provided, runtime may return an error event in-stream, but
    # contract remains SSE with JSON data lines and terminal boolean event.
    res = rest_client.post(
        f"/searches/{search_id}/completion",
        json={"question": "what is coriander?", "kb_ids": ["nonexistent_dataset"]},
        timeout=60,
    )
    assert res.status_code == 200
    content_type = res.headers.get("Content-Type", "")
    assert "text/event-stream" in content_type, content_type

    events = _sse_events(res.text)
    assert events, res.text
    parsed = [json.loads(evt) for evt in events]
    assert isinstance(parsed[0], dict), parsed
    assert parsed[-1].get("data") is True, parsed[-1]
