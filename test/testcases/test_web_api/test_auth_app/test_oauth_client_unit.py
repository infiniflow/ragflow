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

import asyncio
import importlib.util
import sys
import urllib.parse
from pathlib import Path
from types import ModuleType

import pytest


class _FakeResponse:
    def __init__(self, payload=None, err=None):
        self._payload = payload or {}
        self._err = err

    def raise_for_status(self):
        if self._err:
            raise self._err

    def json(self):
        return self._payload


def _base_config(scope="openid profile"):
    return {
        "client_id": "client-1",
        "client_secret": "secret-1",
        "authorization_url": "https://issuer.example/authorize",
        "token_url": "https://issuer.example/token",
        "userinfo_url": "https://issuer.example/userinfo",
        "redirect_uri": "https://app.example/callback",
        "scope": scope,
    }


def _load_oauth_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    http_client_mod = ModuleType("common.http_client")

    async def _default_async_request(*_args, **_kwargs):
        return _FakeResponse({})

    def _default_sync_request(*_args, **_kwargs):
        return _FakeResponse({})

    http_client_mod.async_request = _default_async_request
    http_client_mod.sync_request = _default_sync_request
    monkeypatch.setitem(sys.modules, "common.http_client", http_client_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    auth_pkg = ModuleType("api.apps.auth")
    auth_pkg.__path__ = [str(repo_root / "api" / "apps" / "auth")]

    monkeypatch.setitem(sys.modules, "api", api_pkg)
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    monkeypatch.setitem(sys.modules, "api.apps.auth", auth_pkg)

    sys.modules.pop("api.apps.auth.oauth", None)
    oauth_path = repo_root / "api" / "apps" / "auth" / "oauth.py"
    oauth_spec = importlib.util.spec_from_file_location("api.apps.auth.oauth", oauth_path)
    oauth_module = importlib.util.module_from_spec(oauth_spec)
    monkeypatch.setitem(sys.modules, "api.apps.auth.oauth", oauth_module)
    oauth_spec.loader.exec_module(oauth_module)
    return oauth_module


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.mark.p2
def test_oauth_client_sync_matrix_unit(monkeypatch):
    oauth_module = _load_oauth_module(monkeypatch)
    client = oauth_module.OAuthClient(_base_config())

    assert client.client_id == "client-1"
    assert client.client_secret == "secret-1"
    assert client.authorization_url.endswith("/authorize")
    assert client.token_url.endswith("/token")
    assert client.userinfo_url.endswith("/userinfo")
    assert client.redirect_uri.endswith("/callback")
    assert client.scope == "openid profile"
    assert client.http_request_timeout == 7

    info = oauth_module.UserInfo("u@example.com", "user1", "User One", "avatar-url")
    assert info.to_dict() == {
        "email": "u@example.com",
        "username": "user1",
        "nickname": "User One",
        "avatar_url": "avatar-url",
    }

    auth_url = client.get_authorization_url(state="s p/a?ce")
    parsed = urllib.parse.urlparse(auth_url)
    query = urllib.parse.parse_qs(parsed.query)
    assert parsed.scheme == "https"
    assert query["client_id"] == ["client-1"]
    assert query["redirect_uri"] == ["https://app.example/callback"]
    assert query["response_type"] == ["code"]
    assert query["scope"] == ["openid profile"]
    assert query["state"] == ["s p/a?ce"]

    no_scope_client = oauth_module.OAuthClient(_base_config(scope=None))
    no_scope_query = urllib.parse.parse_qs(urllib.parse.urlparse(no_scope_client.get_authorization_url()).query)
    assert "scope" not in no_scope_query

    call_log = []

    def _sync_ok(method, url, data=None, headers=None, timeout=None):
        call_log.append((method, url, data, headers, timeout))
        if url.endswith("/token"):
            return _FakeResponse({"access_token": "token-1"})
        return _FakeResponse({"email": "user@example.com", "picture": "id-picture"})

    monkeypatch.setattr(oauth_module, "sync_request", _sync_ok)
    token = client.exchange_code_for_token("code-1")
    assert token["access_token"] == "token-1"
    user_info = client.fetch_user_info("access-1")
    assert isinstance(user_info, oauth_module.UserInfo)
    assert user_info.to_dict() == {
        "email": "user@example.com",
        "username": "user",
        "nickname": "user",
        "avatar_url": "id-picture",
    }
    assert call_log[0][0] == "POST"
    assert call_log[0][3]["Accept"] == "application/json"
    assert call_log[1][0] == "GET"
    assert call_log[1][3]["Authorization"] == "Bearer access-1"

    normalized = client.normalize_user_info(
        {"email": "fallback@example.com", "username": "fallback-user", "nickname": "fallback-nick", "avatar_url": "direct-avatar"}
    )
    assert normalized.to_dict()["avatar_url"] == "direct-avatar"

    monkeypatch.setattr(oauth_module, "sync_request", lambda *_args, **_kwargs: _FakeResponse(err=RuntimeError("status boom")))
    with pytest.raises(ValueError, match="Failed to exchange authorization code for token: status boom"):
        client.exchange_code_for_token("code-2")
    with pytest.raises(ValueError, match="Failed to fetch user info: status boom"):
        client.fetch_user_info("access-2")


@pytest.mark.p2
def test_oauth_client_async_matrix_unit(monkeypatch):
    oauth_module = _load_oauth_module(monkeypatch)
    client = oauth_module.OAuthClient(_base_config())

    async def _async_ok(method, url, data=None, headers=None, **kwargs):
        _ = (method, data, headers, kwargs.get("timeout"))
        if url.endswith("/token"):
            return _FakeResponse({"access_token": "token-async"})
        return _FakeResponse({"email": "async@example.com", "username": "async-user", "nickname": "Async User", "avatar_url": "async-avatar"})

    monkeypatch.setattr(oauth_module, "async_request", _async_ok)
    token = asyncio.run(client.async_exchange_code_for_token("code-a"))
    assert token["access_token"] == "token-async"
    info = asyncio.run(client.async_fetch_user_info("async-token"))
    assert info.to_dict() == {
        "email": "async@example.com",
        "username": "async-user",
        "nickname": "Async User",
        "avatar_url": "async-avatar",
    }

    async def _async_fail(*_args, **_kwargs):
        return _FakeResponse(err=RuntimeError("async boom"))

    monkeypatch.setattr(oauth_module, "async_request", _async_fail)
    with pytest.raises(ValueError, match="Failed to exchange authorization code for token: async boom"):
        asyncio.run(client.async_exchange_code_for_token("code-b"))
    with pytest.raises(ValueError, match="Failed to fetch user info: async boom"):
        asyncio.run(client.async_fetch_user_info("async-token-2"))
