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
from pathlib import Path
from types import ModuleType, SimpleNamespace

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


class _DummyJwkClient:
    def __init__(self, _jwks_uri):
        self._key = "dummy-signing-key"

    def get_signing_key_from_jwt(self, _id_token):
        return SimpleNamespace(key=self._key)


def _load_auth_modules(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    auth_pkg = ModuleType("api.apps.auth")
    auth_pkg.__path__ = [str(repo_root / "api" / "apps" / "auth")]

    monkeypatch.setitem(sys.modules, "api", api_pkg)
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    monkeypatch.setitem(sys.modules, "api.apps.auth", auth_pkg)

    for mod_name in ["api.apps.auth.oauth", "api.apps.auth.oidc"]:
        sys.modules.pop(mod_name, None)

    oauth_path = repo_root / "api" / "apps" / "auth" / "oauth.py"
    oauth_spec = importlib.util.spec_from_file_location("api.apps.auth.oauth", oauth_path)
    oauth_module = importlib.util.module_from_spec(oauth_spec)
    monkeypatch.setitem(sys.modules, "api.apps.auth.oauth", oauth_module)
    oauth_spec.loader.exec_module(oauth_module)

    oidc_path = repo_root / "api" / "apps" / "auth" / "oidc.py"
    oidc_spec = importlib.util.spec_from_file_location("api.apps.auth.oidc", oidc_path)
    oidc_module = importlib.util.module_from_spec(oidc_spec)
    monkeypatch.setitem(sys.modules, "api.apps.auth.oidc", oidc_module)
    oidc_spec.loader.exec_module(oidc_module)

    return oauth_module, oidc_module


def _load_auth_init_module(monkeypatch):
    _load_auth_modules(monkeypatch)
    repo_root = Path(__file__).resolve().parents[4]

    github_mod = ModuleType("api.apps.auth.github")

    class _StubGithubOAuthClient:
        def __init__(self, config):
            self.config = config

    github_mod.GithubOAuthClient = _StubGithubOAuthClient
    monkeypatch.setitem(sys.modules, "api.apps.auth.github", github_mod)

    init_path = repo_root / "api" / "apps" / "auth" / "__init__.py"
    init_spec = importlib.util.spec_from_file_location(
        "api.apps.auth",
        init_path,
        submodule_search_locations=[str(repo_root / "api" / "apps" / "auth")],
    )
    init_module = importlib.util.module_from_spec(init_spec)
    monkeypatch.setitem(sys.modules, "api.apps.auth", init_module)
    init_spec.loader.exec_module(init_module)
    return init_module


def _base_config():
    return {
        "issuer": "https://issuer.example",
        "client_id": "client-1",
        "client_secret": "secret-1",
        "redirect_uri": "https://app.example/callback",
    }


def _metadata(issuer):
    return {
        "issuer": issuer,
        "jwks_uri": f"{issuer}/jwks",
        "authorization_endpoint": f"{issuer}/authorize",
        "token_endpoint": f"{issuer}/token",
        "userinfo_endpoint": f"{issuer}/userinfo",
    }


def _make_client(monkeypatch, oidc_module):
    monkeypatch.setattr(oidc_module.OIDCClient, "_load_oidc_metadata", staticmethod(lambda issuer: _metadata(issuer)))
    return oidc_module.OIDCClient(_base_config())


@pytest.mark.p2
def test_oidc_init_requires_issuer(monkeypatch):
    _, oidc_module = _load_auth_modules(monkeypatch)

    with pytest.raises(ValueError) as exc_info:
        oidc_module.OIDCClient({"client_id": "cid"})

    assert str(exc_info.value) == "Missing issuer in configuration."


@pytest.mark.p2
def test_oidc_init_loads_metadata_and_sets_endpoints(monkeypatch):
    _, oidc_module = _load_auth_modules(monkeypatch)
    monkeypatch.setattr(oidc_module.OIDCClient, "_load_oidc_metadata", staticmethod(lambda issuer: _metadata(issuer)))

    client = oidc_module.OIDCClient(_base_config())

    assert client.issuer == "https://issuer.example"
    assert client.jwks_uri == "https://issuer.example/jwks"
    assert client.authorization_url == "https://issuer.example/authorize"
    assert client.token_url == "https://issuer.example/token"
    assert client.userinfo_url == "https://issuer.example/userinfo"


@pytest.mark.p2
def test_load_oidc_metadata_success_and_wraps_failure(monkeypatch):
    _, oidc_module = _load_auth_modules(monkeypatch)

    calls = {}

    def _ok_sync_request(method, url, timeout):
        calls.update({"method": method, "url": url, "timeout": timeout})
        return _FakeResponse(_metadata("https://issuer.example"))

    monkeypatch.setattr(oidc_module, "sync_request", _ok_sync_request)
    metadata = oidc_module.OIDCClient._load_oidc_metadata("https://issuer.example")
    assert metadata["jwks_uri"] == "https://issuer.example/jwks"
    assert calls == {
        "method": "GET",
        "url": "https://issuer.example/.well-known/openid-configuration",
        "timeout": 7,
    }

    def _boom_sync_request(*_args, **_kwargs):
        raise RuntimeError("metadata boom")

    monkeypatch.setattr(oidc_module, "sync_request", _boom_sync_request)
    with pytest.raises(ValueError) as exc_info:
        oidc_module.OIDCClient._load_oidc_metadata("https://issuer.example")
    assert str(exc_info.value) == "Failed to fetch OIDC metadata: metadata boom"


@pytest.mark.p2
def test_parse_id_token_success_and_error(monkeypatch):
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)

    monkeypatch.setattr(oidc_module.jwt, "get_unverified_header", lambda _token: {})

    seen = {}

    class _JwkClient(_DummyJwkClient):
        def __init__(self, jwks_uri):
            super().__init__(jwks_uri)
            seen["jwks_uri"] = jwks_uri

        def get_signing_key_from_jwt(self, id_token):
            seen["id_token"] = id_token
            return super().get_signing_key_from_jwt(id_token)

    monkeypatch.setattr(oidc_module.jwt, "PyJWKClient", _JwkClient)

    def _decode(id_token, key, algorithms, audience, issuer):
        seen.update(
            {
                "decode_id_token": id_token,
                "decode_key": key,
                "algorithms": algorithms,
                "audience": audience,
                "issuer": issuer,
            }
        )
        return {"sub": "user-1", "email": "id@example.com"}

    monkeypatch.setattr(oidc_module.jwt, "decode", _decode)
    parsed = client.parse_id_token("id-token-1")

    assert parsed["sub"] == "user-1"
    assert seen["jwks_uri"] == "https://issuer.example/jwks"
    assert seen["decode_key"] == "dummy-signing-key"
    assert seen["algorithms"] == ["RS256"]
    assert seen["audience"] == "client-1"
    assert seen["issuer"] == "https://issuer.example"

    def _raise_decode(*_args, **_kwargs):
        raise RuntimeError("decode boom")

    monkeypatch.setattr(oidc_module.jwt, "decode", _raise_decode)
    with pytest.raises(ValueError) as exc_info:
        client.parse_id_token("id-token-2")
    assert str(exc_info.value) == "Error parsing ID Token: decode boom"


@pytest.mark.p2
def test_fetch_user_info_merges_id_token_and_oauth_userinfo(monkeypatch):
    oauth_module, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)

    monkeypatch.setattr(
        oidc_module.OIDCClient,
        "parse_id_token",
        lambda self, _id_token: {"picture": "id-picture", "email": "id@example.com"},
    )

    def _fake_parent_fetch(self, access_token, **_kwargs):
        assert access_token == "access-1"
        return oauth_module.UserInfo(
            email="oauth@example.com",
            username="oauth-user",
            nickname="oauth-nick",
            avatar_url=None,
        )

    monkeypatch.setattr(oauth_module.OAuthClient, "fetch_user_info", _fake_parent_fetch)

    info = client.fetch_user_info("access-1", id_token="id-token")

    assert info.email == "oauth@example.com"
    assert info.username == "oauth-user"
    assert info.nickname == "oauth-nick"
    assert info.avatar_url == "id-picture"


@pytest.mark.p2
def test_async_fetch_user_info_merges_id_token_and_oauth_userinfo(monkeypatch):
    oauth_module, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)

    monkeypatch.setattr(
        oidc_module.OIDCClient,
        "parse_id_token",
        lambda self, _id_token: {"picture": "id-picture-async", "email": "id-async@example.com"},
    )

    async def _fake_parent_async_fetch(self, access_token, **_kwargs):
        assert access_token == "access-2"
        return oauth_module.UserInfo(
            email="oauth-async@example.com",
            username="oauth-async-user",
            nickname="oauth-async-nick",
            avatar_url=None,
        )

    monkeypatch.setattr(oauth_module.OAuthClient, "async_fetch_user_info", _fake_parent_async_fetch)

    info = asyncio.run(client.async_fetch_user_info("access-2", id_token="id-token"))

    assert info.email == "oauth-async@example.com"
    assert info.username == "oauth-async-user"
    assert info.nickname == "oauth-async-nick"
    assert info.avatar_url == "id-picture-async"


@pytest.mark.p2
def test_normalize_user_info_passthrough(monkeypatch):
    oauth_module, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)

    result = client.normalize_user_info(
        {
            "email": "user@example.com",
            "username": "user",
            "nickname": "User",
            "picture": "picture-url",
        }
    )

    assert isinstance(result, oauth_module.UserInfo)
    assert result.to_dict() == {
        "email": "user@example.com",
        "username": "user",
        "nickname": "User",
        "avatar_url": "picture-url",
    }


@pytest.mark.p2
def test_get_auth_client_type_inference_and_unsupported(monkeypatch):
    auth_module = _load_auth_init_module(monkeypatch)

    class _FakeOAuth2Client:
        def __init__(self, config):
            self.config = config

    class _FakeOidcClient:
        def __init__(self, config):
            self.config = config

    class _FakeGithubClient:
        def __init__(self, config):
            self.config = config

    monkeypatch.setattr(
        auth_module,
        "CLIENT_TYPES",
        {
            "oauth2": _FakeOAuth2Client,
            "oidc": _FakeOidcClient,
            "github": _FakeGithubClient,
        },
    )

    oidc_client = auth_module.get_auth_client({"issuer": "https://issuer.example"})
    assert isinstance(oidc_client, _FakeOidcClient)

    oauth_client = auth_module.get_auth_client({})
    assert isinstance(oauth_client, _FakeOAuth2Client)

    with pytest.raises(ValueError, match="Unsupported type: invalid"):
        auth_module.get_auth_client({"type": "invalid"})
