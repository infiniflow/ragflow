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
from concurrent.futures import ThreadPoolExecutor

import pytest

from test.testcases.configs import INVALID_API_TOKEN, INVALID_ID_32, SESSION_WITH_CHAT_NAME_LIMIT
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import is_sorted


def _sse_events(response_text: str) -> list[str]:
    return [line[5:] for line in response_text.splitlines() if line.startswith("data:")]


def _session_names(payload):
    return [session["name"] for session in payload["data"]]


def _seed_sessions(rest_client, create_chat, prefix, count=5):
    chat_id = create_chat(f"{prefix}_chat")
    sessions = []
    for index in range(count):
        name = f"{prefix}_{index}"
        res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": name})
        assert res.status_code == 200, (prefix, index, res.text)
        payload = res.json()
        assert payload["code"] == 0, (prefix, index, payload)
        sessions.append(payload["data"])
    return chat_id, sessions


@pytest.mark.p1
def test_session_crud_cycle(rest_client, create_chat):
    chat_id = create_chat("restful_session_crud_chat")

    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_a"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    session_id = create_payload["data"]["id"]
    assert create_payload["data"]["chat_id"] == chat_id, create_payload

    list_res = rest_client.get(f"/chats/{chat_id}/sessions")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert any(item["id"] == session_id for item in list_payload["data"]), list_payload

    get_res = rest_client.get(f"/chats/{chat_id}/sessions/{session_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == session_id, get_payload

    patch_res = rest_client.patch(
        f"/chats/{chat_id}/sessions/{session_id}",
        json={"name": "session_a_updated"},
    )
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "session_a_updated", patch_payload

    delete_res = rest_client.delete(f"/chats/{chat_id}/sessions", json={"ids": [session_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    list_after_delete = rest_client.get(f"/chats/{chat_id}/sessions")
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert all(item["id"] != session_id for item in list_after_delete_payload["data"]), list_after_delete_payload


@pytest.mark.p2
def test_session_create_name_validation(rest_client, create_chat):
    chat_id = create_chat("restful_session_name_validation_chat")

    res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": " "})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "`name` can not be empty." in payload["message"], payload


@pytest.mark.p2
def test_session_update_blocks_messages_and_reference(rest_client, create_chat):
    chat_id = create_chat("restful_session_guard_chat")
    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_guard"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    session_id = create_payload["data"]["id"]

    msg_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"messages": []})
    assert msg_res.status_code == 200
    msg_payload = msg_res.json()
    assert msg_payload["code"] == 102, msg_payload
    assert "`messages` cannot be changed." in msg_payload["message"], msg_payload

    ref_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"reference": []})
    assert ref_res.status_code == 200
    ref_payload = ref_res.json()
    assert ref_payload["code"] == 102, ref_payload
    assert "`reference` cannot be changed." in ref_payload["message"], ref_payload


@pytest.mark.p1
def test_session_create_requires_auth_and_invalid_chat_contract():
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.post("/chats/chat_id/sessions", json={"name": "x"})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)



@pytest.mark.p2
def test_session_create_validation_and_deleted_chat_contract(rest_client, create_chat):
    chat_id = create_chat("restful_session_create_contract")

    empty_path_res = rest_client.post("/chats//sessions", json={"name": "valid_name"})
    assert empty_path_res.status_code == 200
    empty_path_payload = empty_path_res.json()
    assert empty_path_payload["code"] == 100, empty_path_payload
    assert empty_path_payload["message"] == "<MethodNotAllowed '405: Method Not Allowed'>", empty_path_payload

    invalid_chat_res = rest_client.post("/chats/invalid_chat_assistant_id/sessions", json={"name": "valid_name"})
    assert invalid_chat_res.status_code == 200
    invalid_chat_payload = invalid_chat_res.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert invalid_chat_payload["message"] == "No authorization.", invalid_chat_payload

    for scenario_name, payload in (
        ("valid", {"name": "valid_name"}),
        ("empty", {"name": ""}),
        ("space", {"name": " "}),
        ("numeric", {"name": 1}),
    ):
        res = rest_client.post(f"/chats/{chat_id}/sessions", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        if scenario_name == "valid":
            assert body["code"] == 0, (scenario_name, body)
            assert body["data"]["name"] == "valid_name", (scenario_name, body)
            assert body["data"]["chat_id"] == chat_id, (scenario_name, body)
        else:
            assert body["code"] == 102, (scenario_name, body)
            assert body["message"] == "`name` can not be empty.", (scenario_name, body)

    duplicate_first = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "duplicated_name"}).json()
    duplicate_second = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "duplicated_name"}).json()
    assert duplicate_first["code"] == 0, duplicate_first
    assert duplicate_second["code"] == 0, duplicate_second
    assert duplicate_second["data"]["name"] == "duplicated_name", duplicate_second

    upper_case = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "CASE INSENSITIVE"}).json()
    lower_case = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "case insensitive"}).json()
    assert upper_case["code"] == 0, upper_case
    assert lower_case["code"] == 0, lower_case
    assert upper_case["data"]["name"] == "CASE INSENSITIVE", upper_case
    assert lower_case["data"]["name"] == "case insensitive", lower_case

    long_name = "a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1)
    long_name_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": long_name})
    assert long_name_res.status_code == 200
    long_name_payload = long_name_res.json()
    assert long_name_payload["code"] == 0, long_name_payload
    assert long_name_payload["data"]["name"] == long_name[:SESSION_WITH_CHAT_NAME_LIMIT], long_name_payload

    delete_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    create_after_delete = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "after_delete"})
    assert create_after_delete.status_code == 200
    create_after_delete_payload = create_after_delete.json()
    assert create_after_delete_payload["code"] == 109, create_after_delete_payload
    assert create_after_delete_payload["message"] == "No authorization.", create_after_delete_payload


@pytest.mark.p2
def test_session_create_concurrent_contract(rest_client, create_chat):
    chat_id = create_chat("restful_session_create_concurrent")

    def _create(index):
        return rest_client.post(f"/chats/{chat_id}/sessions", json={"name": f"session create {index}"}).json()

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(_create, range(20)))

    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results

    list_res = rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert len(list_payload["data"]) == 20, list_payload


@pytest.mark.p1
def test_session_delete_requires_auth_and_invalid_target_contract(rest_client, create_chat):
    chat_id = create_chat("restful_session_delete_auth")
    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_a"})
    assert create_res.status_code == 200
    session_id = create_res.json()["data"]["id"]

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.delete(f"/chats/{chat_id}/sessions", json={"ids": [session_id]})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)

    invalid_chat_res = rest_client.delete("/chats/invalid_chat_assistant_id/sessions", json={"ids": [session_id]})
    assert invalid_chat_res.status_code == 200
    invalid_chat_payload = invalid_chat_res.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert invalid_chat_payload["message"] == "No authorization.", invalid_chat_payload


@pytest.mark.p2
def test_session_delete_basic_scenarios(rest_client, create_chat):
    cases = [
        ("none payload", None, 0, 5, {}),
        ("invalid only", {"ids": ["invalid_id"]}, 102, 5, "The chat doesn't own the session invalid_id"),
        ("not json", "not json", 100, 5, "<BadRequest '400: Bad Request'>"),
        ("single id", lambda sessions: {"ids": [sessions[0]["id"]]}, 0, 4, True),
        ("all ids", lambda sessions: {"ids": [session["id"] for session in sessions]}, 0, 0, True),
        ("delete all", {"delete_all": True}, 0, 0, True),
        ("empty ids", {"ids": []}, 0, 5, {}),
    ]

    for scenario_name, payload, expected_code, expected_remaining, expected_data in cases:
        chat_id, sessions = _seed_sessions(rest_client, create_chat, f"delete_basic_{scenario_name.replace(' ', '_')}")
        if callable(payload):
            payload = payload(sessions)
        if scenario_name == "not json":
            res = rest_client.delete(
                f"/chats/{chat_id}/sessions",
                headers={"Content-Type": "application/json"},
                data=payload,
            )
        else:
            res = rest_client.delete(f"/chats/{chat_id}/sessions", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code == 0:
            assert body["data"] == expected_data, (scenario_name, body)
        else:
            assert body["message"] == expected_data, (scenario_name, body)

        list_res = rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30})
        assert list_res.status_code == 200, (scenario_name, list_res.text)
        list_payload = list_res.json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert len(list_payload["data"]) == expected_remaining, (scenario_name, list_payload)


@pytest.mark.p2
def test_session_delete_error_and_repeat_contract(rest_client, create_chat):
    partial_cases = [
        ("invalid first", lambda sessions: {"ids": ["invalid_id"] + [session["id"] for session in sessions]}),
        ("invalid middle", lambda sessions: {"ids": [sessions[0]["id"], "invalid_id", *[session["id"] for session in sessions[1:]]]}),
        ("invalid last", lambda sessions: {"ids": [session["id"] for session in sessions] + ["invalid_id"]}),
    ]
    for scenario_name, payload_builder in partial_cases:
        chat_id, sessions = _seed_sessions(rest_client, create_chat, f"delete_partial_{scenario_name.replace(' ', '_')}")
        res = rest_client.delete(f"/chats/{chat_id}/sessions", json=payload_builder(sessions))
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        assert payload["data"]["success_count"] == len(sessions), (scenario_name, payload)
        assert payload["data"]["errors"] == ["The chat doesn't own the session invalid_id"], (scenario_name, payload)
        remaining = rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30}).json()
        assert remaining["code"] == 0, (scenario_name, remaining)
        assert remaining["data"] == [], (scenario_name, remaining)

    duplicate_chat_id, duplicate_sessions = _seed_sessions(rest_client, create_chat, "delete_duplicate")
    duplicate_id = duplicate_sessions[0]["id"]
    duplicate_res = rest_client.delete(f"/chats/{duplicate_chat_id}/sessions", json={"ids": [duplicate_id, duplicate_id]})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload
    assert duplicate_payload["data"]["success_count"] == 1, duplicate_payload
    assert duplicate_payload["data"]["errors"] == [f"Duplicate session ids: {duplicate_id}"], duplicate_payload

    repeated_chat_id, repeated_sessions = _seed_sessions(rest_client, create_chat, "delete_repeated")
    repeated_ids = [session["id"] for session in repeated_sessions]
    first_res = rest_client.delete(f"/chats/{repeated_chat_id}/sessions", json={"ids": repeated_ids})
    assert first_res.status_code == 200
    first_payload = first_res.json()
    assert first_payload["code"] == 0, first_payload
    assert first_payload["data"] is True, first_payload

    second_res = rest_client.delete(f"/chats/{repeated_chat_id}/sessions", json={"ids": repeated_ids})
    assert second_res.status_code == 200
    second_payload = second_res.json()
    assert second_payload["code"] == 102, second_payload
    for session_id in repeated_ids:
        assert f"The chat doesn't own the session {session_id}" in second_payload["message"], second_payload


@pytest.mark.p2
def test_session_delete_concurrent_and_bulk_contract(rest_client, create_chat):
    concurrent_chat_id, concurrent_sessions = _seed_sessions(rest_client, create_chat, "delete_concurrent", count=20)

    def _delete(session):
        return rest_client.delete(f"/chats/{concurrent_chat_id}/sessions", json={"ids": [session["id"]]}).json()

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(_delete, concurrent_sessions))

    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results
    assert all(result["data"] is True for result in results), results

    list_after_concurrent = rest_client.get(f"/chats/{concurrent_chat_id}/sessions", params={"page_size": 30}).json()
    assert list_after_concurrent["code"] == 0, list_after_concurrent
    assert list_after_concurrent["data"] == [], list_after_concurrent

    bulk_chat_id, bulk_sessions = _seed_sessions(rest_client, create_chat, "delete_bulk", count=100)
    bulk_res = rest_client.delete(
        f"/chats/{bulk_chat_id}/sessions",
        json={"ids": [session["id"] for session in bulk_sessions]},
    )
    assert bulk_res.status_code == 200
    bulk_payload = bulk_res.json()
    assert bulk_payload["code"] == 0, bulk_payload
    assert bulk_payload["data"] is True, bulk_payload


@pytest.mark.p1
def test_session_list_requires_auth_and_invalid_target_contract():
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get("/chats/chat_id/sessions")
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p2
def test_session_list_filter_and_deleted_chat_contract(rest_client, create_chat):
    chat_id, sessions = _seed_sessions(rest_client, create_chat, "list_filter")
    session_ids = [session["id"] for session in sessions]
    session_names = [session["name"] for session in sessions]

    default_res = rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30})
    assert default_res.status_code == 200
    default_payload = default_res.json()
    assert default_payload["code"] == 0, default_payload
    assert len(default_payload["data"]) == 5, default_payload

    for scenario_name, params, expected_names in (
        ("id none", {"id": None, "page_size": 30}, session_names),
        ("id empty", {"id": "", "page_size": 30}, session_names),
        ("valid id", {"id": session_ids[0], "page_size": 30}, [session_names[0]]),
        ("unknown id", {"id": "unknown", "page_size": 30}, []),
        ("name none", {"name": None, "page_size": 30}, session_names),
        ("name empty", {"name": "", "page_size": 30}, session_names),
        ("name exact", {"name": session_names[1], "page_size": 30}, [session_names[1]]),
        ("name unknown", {"name": "unknown", "page_size": 30}, []),
        ("name and id match", {"id": session_ids[0], "name": session_names[0], "page_size": 30}, [session_names[0]]),
        ("name and id mismatch", {"id": session_ids[0], "name": "session_with_chat_assistant_100", "page_size": 30}, []),
        ("name and invalid id", {"id": "id", "name": session_names[0], "page_size": 30}, []),
        ("invalid params ignored", {"a": "b", "page_size": 30}, session_names),
    ):
        res = rest_client.get(f"/chats/{chat_id}/sessions", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        assert set(_session_names(payload)) == set(expected_names), (scenario_name, payload)

    invalid_chat_res = rest_client.get(f"/chats/{INVALID_ID_32}/sessions")
    assert invalid_chat_res.status_code == 200
    invalid_chat_payload = invalid_chat_res.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert invalid_chat_payload["message"] == "No authorization.", invalid_chat_payload

    delete_chat_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_chat_res.status_code == 200
    delete_chat_payload = delete_chat_res.json()
    assert delete_chat_payload["code"] == 0, delete_chat_payload

    deleted_list_res = rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30})
    assert deleted_list_res.status_code == 200
    deleted_list_payload = deleted_list_res.json()
    assert deleted_list_payload["code"] == 109, deleted_list_payload
    assert deleted_list_payload["message"] == "No authorization.", deleted_list_payload


@pytest.mark.p2
def test_session_list_page_and_sort_contract(rest_client, create_chat):
    chat_id, sessions = _seed_sessions(rest_client, create_chat, "list_page_sort")
    created_names = [session["name"] for session in sessions]
    descending_names = list(reversed(created_names))

    page_cases = [
        ("page none", {"page": None, "page_size": 2}, 0, 2, ""),
        ("page zero", {"page": 0, "page_size": 2}, 0, 2, ""),
        ("page two", {"page": 2, "page_size": 2}, 0, 2, ""),
        ("page three", {"page": 3, "page_size": 2}, 0, 1, ""),
        ("page string", {"page": "3", "page_size": 2}, 0, 1, ""),
        ("page negative", {"page": -1, "page_size": 2}, 100, 0, "ProgrammingError(1064"),
        ("page alpha", {"page": "a", "page_size": 2}, 100, 0, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
        ("page_size none", {"page_size": None}, 0, 5, ""),
        ("page_size zero", {"page_size": 0}, 0, 0, ""),
        ("page_size one", {"page_size": 1}, 0, 1, ""),
        ("page_size six", {"page_size": 6}, 0, 5, ""),
        ("page_size negative", {"page_size": -1}, 0, 5, ""),
        ("page_size alpha", {"page_size": "a"}, 100, 0, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
    ]
    for scenario_name, params, expected_code, expected_count, expected_message in page_cases:
        res = rest_client.get(f"/chats/{chat_id}/sessions", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert len(payload["data"]) == expected_count, (scenario_name, payload)
        else:
            assert expected_message in payload["message"], (scenario_name, payload)

    sort_cases = [
        ("orderby none", {"orderby": None, "page_size": 30}, "create_time", True, descending_names, ""),
        ("orderby create", {"orderby": "create_time", "page_size": 30}, "create_time", True, descending_names, ""),
        ("orderby update", {"orderby": "update_time", "page_size": 30}, "update_time", True, descending_names, ""),
        ("orderby name ascending", {"orderby": "name", "desc": "False", "page_size": 30}, "name", False, created_names, ""),
        ("orderby unknown", {"orderby": "unknown", "page_size": 30}, None, None, None, "AttributeError(\"type object 'Conversation' has no attribute 'unknown'\")"),
        ("desc none", {"desc": None, "page_size": 30}, "create_time", True, descending_names, ""),
        ("desc true", {"desc": "true", "page_size": 30}, "create_time", True, descending_names, ""),
        ("desc True", {"desc": "True", "page_size": 30}, "create_time", True, descending_names, ""),
        ("desc false", {"desc": "false", "page_size": 30}, "create_time", False, created_names, ""),
        ("desc False", {"desc": "False", "page_size": 30}, "create_time", False, created_names, ""),
        ("desc false update_time", {"desc": "False", "orderby": "update_time", "page_size": 30}, "update_time", False, created_names, ""),
        ("desc unknown", {"desc": "unknown", "page_size": 30}, "create_time", True, descending_names, ""),
    ]
    for scenario_name, params, field, descending, expected_names, expected_message in sort_cases:
        res = rest_client.get(f"/chats/{chat_id}/sessions", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        expected_code = 0 if expected_names is not None else 100
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert is_sorted(payload["data"], field, descending), (scenario_name, payload)
            assert _session_names(payload) == expected_names, (scenario_name, payload)
        else:
            assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_session_list_concurrent_contract(rest_client, create_chat):
    chat_id, _sessions = _seed_sessions(rest_client, create_chat, "list_concurrent")

    def _list(_):
        return rest_client.get(f"/chats/{chat_id}/sessions", params={"page_size": 30}).json()

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(_list, range(10)))

    assert len(results) == 10, results
    assert all(result["code"] == 0 for result in results), results
    assert all(len(result["data"]) == 5 for result in results), results


@pytest.mark.p1
def test_session_update_requires_auth_and_invalid_target_contract(rest_client, create_chat):
    chat_id = create_chat("restful_session_update_auth")
    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_update_auth"})
    assert create_res.status_code == 200
    session_id = create_res.json()["data"]["id"]

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"name": "x"})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)

    invalid_chat_res = rest_client.patch(f"/chats/{INVALID_ID_32}/sessions/{session_id}", json={"name": "x"})
    assert invalid_chat_res.status_code == 200
    invalid_chat_payload = invalid_chat_res.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert invalid_chat_payload["message"] == "No authorization.", invalid_chat_payload

    empty_session_res = rest_client.patch(f"/chats/{chat_id}/sessions/", json={"name": "x"})
    assert empty_session_res.status_code == 200
    empty_session_payload = empty_session_res.json()
    assert empty_session_payload["code"] == 100, empty_session_payload
    assert empty_session_payload["message"] == "<MethodNotAllowed '405: Method Not Allowed'>", empty_session_payload

    invalid_session_res = rest_client.patch(f"/chats/{chat_id}/sessions/invalid_session_id", json={"name": "x"})
    assert invalid_session_res.status_code == 200
    invalid_session_payload = invalid_session_res.json()
    assert invalid_session_payload["code"] == 102, invalid_session_payload
    assert invalid_session_payload["message"] == "Session not found!", invalid_session_payload


@pytest.mark.p2
def test_session_update_name_and_param_contract(rest_client, create_chat):
    chat_id, sessions = _seed_sessions(rest_client, create_chat, "update_contract")
    session_id = sessions[0]["id"]

    for scenario_name, payload, expected_code, expected_name_or_message in (
        ("valid", {"name": "valid_name"}, 0, "valid_name"),
        ("empty", {"name": ""}, 102, "`name` can not be empty."),
        ("numeric", {"name": 1}, 102, "`name` can not be empty."),
        ("duplicate", {"name": "duplicated_name"}, 0, "duplicated_name"),
        ("case insensitive upper", {"name": "CASE INSENSITIVE UPDATE"}, 0, "CASE INSENSITIVE UPDATE"),
        ("case insensitive lower", {"name": "case insensitive update"}, 0, "case insensitive update"),
        ("long name", {"name": "a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1)}, 0, "a" * SESSION_WITH_CHAT_NAME_LIMIT),
    ):
        res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code == 0:
            assert body["data"]["name"] == expected_name_or_message, (scenario_name, body)
        else:
            assert body["message"] == expected_name_or_message, (scenario_name, body)

    for scenario_name, payload in (("empty payload", {}), ("none payload", None)):
        res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 0, (scenario_name, body)
        assert body["data"]["id"] == session_id, (scenario_name, body)

    delete_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    update_after_delete_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"name": "after_delete"})
    assert update_after_delete_res.status_code == 200
    update_after_delete_payload = update_after_delete_res.json()
    assert update_after_delete_payload["code"] == 109, update_after_delete_payload
    assert update_after_delete_payload["message"] == "No authorization.", update_after_delete_payload


@pytest.mark.p2
def test_session_update_repeated_and_concurrent_contract(rest_client, create_chat):
    chat_id, sessions = _seed_sessions(rest_client, create_chat, "update_repeated")
    session_ids = [session["id"] for session in sessions]

    first_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_ids[0]}", json={"name": "valid_name_1"})
    assert first_res.status_code == 200
    assert first_res.json()["code"] == 0, first_res.json()

    second_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_ids[0]}", json={"name": "valid_name_2"})
    assert second_res.status_code == 200
    assert second_res.json()["code"] == 0, second_res.json()

    def _update(index):
        return rest_client.patch(
            f"/chats/{chat_id}/sessions/{session_ids[index % len(session_ids)]}",
            json={"name": f"update session test {index}"},
        ).json()

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(_update, range(20)))

    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results


@pytest.mark.p2
def test_chat_recommendation_requires_question(rest_client):
    res = rest_client.post("/chat/recommendation", json={})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing: question" in payload["message"], payload


@pytest.mark.p2
def test_related_questions_compatibility_requires_auth(rest_client_noauth):
    # /api/v1/searchbots/related_questions is an SDK compatibility endpoint.
    res = rest_client_noauth.post(
        "/searchbots/related_questions",
        json={"question": "ragflow"},
        headers={"Authorization": "invalid"},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"].strip() in {
        "Authorization is not valid!",
        'Authentication error: API key is invalid!"',
        "Authentication error: API key is invalid!",
    }, payload


@pytest.mark.p2
def test_chat_completion_nonstream_with_session(rest_client, create_chat):
    chat_id = create_chat("restful_completion_nonstream_chat")
    create_session_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_for_completion"})
    assert create_session_res.status_code == 200
    create_session_payload = create_session_res.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    completion_res = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "session_id": session_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert completion_res.status_code == 200
    completion_payload = completion_res.json()
    assert completion_payload["code"] == 0, completion_payload
    assert isinstance(completion_payload["data"], dict), completion_payload
    for key in ["answer", "reference", "audio_binary", "id", "session_id"]:
        assert key in completion_payload["data"], completion_payload
    assert completion_payload["data"]["session_id"] == session_id, completion_payload


@pytest.mark.p2
def test_chat_completion_nonstream_with_chat_without_session(rest_client, create_chat):
    chat_id = create_chat("restful_completion_nonstream_without_session_chat")

    completion_res = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert completion_res.status_code == 200
    completion_payload = completion_res.json()
    assert completion_payload["code"] == 0, completion_payload
    assert isinstance(completion_payload["data"], dict), completion_payload
    assert completion_payload["data"]["session_id"], completion_payload


@pytest.mark.p2
def test_chat_completion_nonstream_without_chat(rest_client):
    completion_res = rest_client.post(
        "/chat/completions",
        json={
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert completion_res.status_code == 200
    completion_payload = completion_res.json()
    assert completion_payload["code"] == 0, completion_payload
    assert isinstance(completion_payload["data"], dict), completion_payload
    assert "answer" in completion_payload["data"], completion_payload


@pytest.mark.p2
def test_chat_completion_stream_events(rest_client, create_chat):
    chat_id = create_chat("restful_completion_stream_chat")
    stream_res = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": True,
        },
        timeout=60,
    )
    assert stream_res.status_code == 200
    content_type = stream_res.headers.get("Content-Type", "")
    assert "text/event-stream" in content_type, content_type

    events = _sse_events(stream_res.text)
    assert events, stream_res.text
    parsed_events = []
    for event in events:
        parsed = json.loads(event)
        parsed_events.append(parsed)

    assert any(evt.get("code") == 0 and isinstance(evt.get("data"), dict) for evt in parsed_events), parsed_events
    assert parsed_events[-1].get("data") is True, parsed_events[-1]


@pytest.mark.p2
def test_chat_completion_validation_errors(rest_client, create_chat):
    chat_id = create_chat("restful_completion_validation_chat")

    missing_messages = rest_client.post(
        "/chat/completions",
        json={"chat_id": chat_id, "stream": False},
    )
    assert missing_messages.status_code == 200
    missing_messages_payload = missing_messages.json()
    assert missing_messages_payload["code"] == 101, missing_messages_payload
    assert missing_messages_payload["message"] in {
        "required argument are missing: messages",
        "messages: is required",
    }, missing_messages_payload

    missing_chat_for_session = rest_client.post(
        "/chat/completions",
        json={
            "session_id": "some_session",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
    )
    assert missing_chat_for_session.status_code == 200
    missing_chat_for_session_payload = missing_chat_for_session.json()
    assert missing_chat_for_session_payload["code"] == 102, missing_chat_for_session_payload
    assert "`chat_id` is required when `session_id` is provided." in missing_chat_for_session_payload["message"], missing_chat_for_session_payload

    invalid_session = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
            "session_id": "invalid_session",
        },
    )
    assert invalid_session.status_code == 200
    invalid_session_payload = invalid_session.json()
    assert invalid_session_payload["code"] == 102, invalid_session_payload
    assert "Session not found!" in invalid_session_payload["message"], invalid_session_payload

    invalid_chat = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": "invalid_chat_id",
            "session_id": "invalid_session",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
    )
    assert invalid_chat.status_code == 200
    invalid_chat_payload = invalid_chat.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert "No authorization." in invalid_chat_payload["message"], invalid_chat_payload
