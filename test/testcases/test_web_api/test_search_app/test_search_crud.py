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
import uuid

import pytest
from common import search_create, search_detail, search_list, search_rm, search_update
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


INVALID_AUTH_CASES = [
    (None, 401, "Unauthorized"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
]


def _search_name(prefix="search"):
    return f"{prefix}_{uuid.uuid4().hex[:8]}"


def _find_tenant_id(WebApiAuth, search_id):
    res = search_list(WebApiAuth, payload={})
    assert res["code"] == 0, res
    for search_app in res["data"]["search_apps"]:
        if search_app.get("id") == search_id:
            return search_app.get("tenant_id")
    assert False, res


@pytest.fixture
def search_app(WebApiAuth):
    name = _search_name()
    create_res = search_create(WebApiAuth, {"name": name, "description": "test search"})
    assert create_res["code"] == 0, create_res
    search_id = create_res["data"]["search_id"]
    yield search_id
    rm_res = search_rm(WebApiAuth, {"search_id": search_id})
    assert rm_res["code"] == 0, rm_res
    assert rm_res["data"] is True, rm_res


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_create(self, invalid_auth, expected_code, expected_fragment):
        res = search_create(invalid_auth, {"name": "dummy"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_list(self, invalid_auth, expected_code, expected_fragment):
        res = search_list(invalid_auth, payload={})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_detail(self, invalid_auth, expected_code, expected_fragment):
        res = search_detail(invalid_auth, {"search_id": "dummy_search_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_update(self, invalid_auth, expected_code, expected_fragment):
        res = search_update(invalid_auth, {"search_id": "dummy", "name": "dummy", "search_config": {}, "tenant_id": "dummy"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_rm(self, invalid_auth, expected_code, expected_fragment):
        res = search_rm(invalid_auth, {"search_id": "dummy_search_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res


class TestSearchCrud:
    @pytest.mark.p2
    def test_create_and_rm(self, WebApiAuth):
        name = _search_name("create")
        create_res = search_create(WebApiAuth, {"name": name, "description": "test search"})
        assert create_res["code"] == 0, create_res
        search_id = create_res["data"]["search_id"]

        rm_res = search_rm(WebApiAuth, {"search_id": search_id})
        assert rm_res["code"] == 0, rm_res
        assert rm_res["data"] is True, rm_res

    @pytest.mark.p2
    def test_list(self, WebApiAuth, search_app):
        res = search_list(WebApiAuth, payload={})
        assert res["code"] == 0, res
        assert any(app.get("id") == search_app for app in res["data"]["search_apps"]), res

    @pytest.mark.p2
    def test_detail(self, WebApiAuth, search_app):
        res = search_detail(WebApiAuth, {"search_id": search_app})
        assert res["code"] == 0, res
        assert res["data"].get("id") == search_app, res

    @pytest.mark.p2
    def test_update(self, WebApiAuth, search_app):
        tenant_id = _find_tenant_id(WebApiAuth, search_app)
        new_name = _search_name("updated")
        payload = {
            "search_id": search_app,
            "name": new_name,
            "search_config": {"top_k": 3},
            "tenant_id": tenant_id,
        }
        res = search_update(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"].get("name") == new_name, res

    @pytest.mark.p3
    def test_create_invalid_name(self, WebApiAuth):
        res = search_create(WebApiAuth, {"name": ""})
        assert res["code"] == 102, res
        assert "empty" in res["message"], res

    @pytest.mark.p3
    def test_update_invalid_search_id(self, WebApiAuth):
        create_res = search_create(WebApiAuth, {"name": _search_name("invalid"), "description": "test search"})
        assert create_res["code"] == 0, create_res
        search_id = create_res["data"]["search_id"]
        tenant_id = _find_tenant_id(WebApiAuth, search_id)
        try:
            payload = {
                "search_id": "invalid_search_id",
                "name": "invalid",
                "search_config": {},
                "tenant_id": tenant_id,
            }
            res = search_update(WebApiAuth, payload)
            assert res["code"] == 109, res
            assert "No authorization" in res["message"], res
        finally:
            rm_res = search_rm(WebApiAuth, {"search_id": search_id})
            assert rm_res["code"] == 0, rm_res
