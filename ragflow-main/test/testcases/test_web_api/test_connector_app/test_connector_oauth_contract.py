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
import os

import pytest
import requests

from configs import HOST_ADDRESS, VERSION

CONNECTOR_BASE_URL = f"{HOST_ADDRESS}/{VERSION}/connector"
LLM_API_KEY_URL = f"{HOST_ADDRESS}/{VERSION}/llm/set_api_key"
LANGFUSE_API_KEY_URL = f"{HOST_ADDRESS}/{VERSION}/langfuse/api_key"

pytestmark = pytest.mark.p3


@pytest.fixture(autouse=True)
def _require_oauth_env(require_env_flag):
    require_env_flag("RAGFLOW_E2E_OAUTH")


def _skip_unless_provider(allowed):
    provider = os.getenv("RAGFLOW_OAUTH_PROVIDER")
    if provider and provider not in allowed:
        pytest.skip(f"RAGFLOW_OAUTH_PROVIDER={provider} not in {sorted(allowed)}")


def _assert_unauthorized(payload):
    assert payload["code"] == 401, payload
    assert "Unauthorized" in payload["message"], payload


def _assert_unauthorized_response(res, *, allow_405=False):
    if allow_405 and res.status_code == 405:
        pytest.skip("method not supported in this deployment")
    content_type = res.headers.get("Content-Type", "")
    payload = None
    if "json" in content_type:
        payload = res.json()
    else:
        try:
            payload = res.json()
        except ValueError:
            assert False, f"Expected JSON response, status={res.status_code}, content_type={content_type}"
    _assert_unauthorized(payload)


def _assert_callback_response(res, expected_fragment):
    assert res.status_code in {200, 302}, {"status": res.status_code, "headers": dict(res.headers)}
    if res.status_code == 200:
        assert "text/html" in res.headers.get("Content-Type", ""), res.headers
        assert expected_fragment in res.text
    else:
        location = res.headers.get("Location", "")
        assert location, res.headers
        markers = ("error", "oauth", "callback", "state", "code")
        assert any(marker in location for marker in markers), location


def test_google_oauth_start_requires_auth():
    _skip_unless_provider({"google", "google-drive", "gmail"})
    res = requests.post(f"{CONNECTOR_BASE_URL}/google/oauth/web/start")
    _assert_unauthorized(res.json())


def test_google_oauth_start_missing_credentials(WebApiAuth):
    _skip_unless_provider({"google", "google-drive", "gmail"})
    res = requests.post(f"{CONNECTOR_BASE_URL}/google/oauth/web/start", auth=WebApiAuth, json={})
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing" in payload["message"], payload
    assert "credentials" in payload["message"], payload


@pytest.mark.parametrize("path", ["google-drive/oauth/web/callback", "gmail/oauth/web/callback"])
def test_google_oauth_callback_missing_state(path):
    _skip_unless_provider({"google", "google-drive", "gmail"})
    res = requests.get(f"{CONNECTOR_BASE_URL}/{path}", allow_redirects=False)
    _assert_callback_response(res, "Missing OAuth state parameter.")


def test_google_oauth_result_missing_flow_id(WebApiAuth):
    _skip_unless_provider({"google", "google-drive", "gmail"})
    res = requests.post(
        f"{CONNECTOR_BASE_URL}/google/oauth/web/result",
        params={"type": "google-drive"},
        auth=WebApiAuth,
        json={},
    )
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing" in payload["message"], payload
    assert "flow_id" in payload["message"], payload


def test_box_oauth_start_missing_params(WebApiAuth):
    _skip_unless_provider({"box"})
    res = requests.post(f"{CONNECTOR_BASE_URL}/box/oauth/web/start", auth=WebApiAuth, json={})
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "client_id" in payload["message"], payload
    assert "client_secret" in payload["message"], payload


def test_box_oauth_callback_missing_state():
    _skip_unless_provider({"box"})
    res = requests.get(f"{CONNECTOR_BASE_URL}/box/oauth/web/callback", allow_redirects=False)
    _assert_callback_response(res, "Missing OAuth parameters.")


def test_box_oauth_result_missing_flow_id(WebApiAuth):
    _skip_unless_provider({"box"})
    res = requests.post(f"{CONNECTOR_BASE_URL}/box/oauth/web/result", auth=WebApiAuth, json={})
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing" in payload["message"], payload
    assert "flow_id" in payload["message"], payload


def test_langfuse_api_key_requires_auth():
    res = requests.post(LANGFUSE_API_KEY_URL, json={})
    _assert_unauthorized_response(res)


def test_langfuse_api_key_requires_auth_get():
    res = requests.get(LANGFUSE_API_KEY_URL)
    _assert_unauthorized_response(res, allow_405=True)


def test_langfuse_api_key_requires_auth_put():
    res = requests.put(LANGFUSE_API_KEY_URL, json={})
    _assert_unauthorized_response(res, allow_405=True)


def test_llm_set_api_key_requires_auth():
    res = requests.post(LLM_API_KEY_URL, json={})
    _assert_unauthorized_response(res)
