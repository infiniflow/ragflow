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
from pathlib import Path
from types import ModuleType
from urllib.parse import parse_qs, urlparse
import importlib.util
import sys
import types

import pytest


def _ensure_pkg(name: str) -> ModuleType:
    if name in sys.modules:
        return sys.modules[name]
    pkg = types.ModuleType(name)
    pkg.__path__ = []
    sys.modules[name] = pkg
    return pkg


def _load_module(module_name: str, file_path: Path) -> ModuleType:
    spec = importlib.util.spec_from_file_location(module_name, file_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


def _load_auth_modules():
    root = Path(__file__).resolve().parents[4]
    _ensure_pkg("api")
    _ensure_pkg("api.apps")
    _ensure_pkg("api.apps.auth")
    oauth_mod = _load_module("api.apps.auth.oauth", root / "api" / "apps" / "auth" / "oauth.py")
    github_mod = _load_module("api.apps.auth.github", root / "api" / "apps" / "auth" / "github.py")
    return oauth_mod, github_mod


def _load_oidc_module():
    root = Path(__file__).resolve().parents[4]
    _load_auth_modules()
    return _load_module("api.apps.auth.oidc", root / "api" / "apps" / "auth" / "oidc.py")


class _StubResponse:
    def __init__(self, payload):
        self._payload = payload

    def raise_for_status(self):
        return None

    def json(self):
        return self._payload


def _basic_oauth_config():
    return {
        "client_id": "client_id",
        "client_secret": "client_secret",
        "authorization_url": "https://example.com/auth",
        "token_url": "https://example.com/token",
        "userinfo_url": "https://example.com/userinfo",
        "redirect_uri": "https://example.com/callback",
        "scope": "email",
    }


@pytest.mark.p2
class TestGithubOAuthClient:
    def test_github_fetch_user_info_success(self, monkeypatch):
        config = {
            "client_id": "client_id",
            "client_secret": "client_secret",
            "redirect_uri": "https://example.com/callback",
        }
        oauth_mod, github_mod = _load_auth_modules()
        client = github_mod.GithubOAuthClient(config)

        user_payload = {"login": "octo", "name": "Octo Cat", "avatar_url": "http://avatar"}
        emails_payload = [
            {"email": "other@example.com", "primary": False},
            {"email": "primary@example.com", "primary": True},
        ]
        responses = [_StubResponse(user_payload), _StubResponse(emails_payload)]

        def _sync_request(*_args, **_kwargs):
            return responses.pop(0)

        monkeypatch.setattr(github_mod, "sync_request", _sync_request)

        user_info = client.fetch_user_info("token")
        assert user_info.email == "primary@example.com"
        assert user_info.username == "octo"
        assert user_info.nickname == "Octo Cat"
        assert user_info.avatar_url == "http://avatar"

    def test_github_fetch_user_info_error_wrapped(self, monkeypatch):
        config = {
            "client_id": "client_id",
            "client_secret": "client_secret",
            "redirect_uri": "https://example.com/callback",
        }
        oauth_mod, github_mod = _load_auth_modules()
        client = github_mod.GithubOAuthClient(config)

        def _sync_request(*_args, **_kwargs):
            raise Exception("boom")

        monkeypatch.setattr(github_mod, "sync_request", _sync_request)

        with pytest.raises(ValueError) as excinfo:
            client.fetch_user_info("token")
        assert "failed to fetch github user info" in str(excinfo.value).lower()


@pytest.mark.p2
class TestOAuthClient:
    def test_oauth_authorization_url_includes_state_and_scope(self):
        oauth_mod, github_mod = _load_auth_modules()
        client = oauth_mod.OAuthClient(_basic_oauth_config())
        url = client.get_authorization_url(state="s")
        params = parse_qs(urlparse(url).query)
        assert params.get("client_id") == ["client_id"]
        assert params.get("redirect_uri") == ["https://example.com/callback"]
        assert params.get("scope") == ["email"]
        assert params.get("state") == ["s"]

    def test_oauth_exchange_code_for_token_error_wrapped(self, monkeypatch):
        oauth_mod, github_mod = _load_auth_modules()
        client = oauth_mod.OAuthClient(_basic_oauth_config())

        def _sync_request(*_args, **_kwargs):
            raise Exception("boom")

        monkeypatch.setattr(oauth_mod, "sync_request", _sync_request)

        with pytest.raises(ValueError) as excinfo:
            client.exchange_code_for_token("code")
        message = str(excinfo.value).lower()
        assert "exchange" in message and "token" in message

    def test_oauth_normalize_user_info_picture_fallback(self):
        oauth_mod, github_mod = _load_auth_modules()
        client = oauth_mod.OAuthClient(_basic_oauth_config())
        info = client.normalize_user_info({"email": "a@b.com", "picture": "http://pic"})
        assert info.avatar_url == "http://pic"
        assert info.username == "a"


@pytest.mark.p2
class TestOIDCClient:
    def _metadata(self):
        return {
            "issuer": "https://issuer",
            "jwks_uri": "https://issuer/jwks",
            "authorization_endpoint": "https://issuer/auth",
            "token_endpoint": "https://issuer/token",
            "userinfo_endpoint": "https://issuer/userinfo",
        }

    def _base_config(self):
        return {
            "issuer": "https://issuer",
            "client_id": "client_id",
            "client_secret": "client_secret",
            "redirect_uri": "https://example.com/callback",
        }

    def test_oidc_client_missing_issuer_raises(self):
        oidc_mod = _load_oidc_module()
        with pytest.raises(ValueError) as excinfo:
            oidc_mod.OIDCClient({})
        assert "issuer" in str(excinfo.value).lower()

    def test_oidc_load_metadata_failure_wrapped(self, monkeypatch):
        oidc_mod = _load_oidc_module()

        def _boom(*_args, **_kwargs):
            raise Exception("boom")

        monkeypatch.setattr(oidc_mod, "sync_request", _boom)
        with pytest.raises(ValueError) as excinfo:
            oidc_mod.OIDCClient._load_oidc_metadata("https://issuer")
        message = str(excinfo.value).lower()
        assert "metadata" in message

    def test_oidc_parse_id_token_error_wrapped(self, monkeypatch):
        oidc_mod = _load_oidc_module()
        monkeypatch.setattr(oidc_mod.OIDCClient, "_load_oidc_metadata", lambda *_args, **_kwargs: self._metadata())
        client = oidc_mod.OIDCClient(self._base_config())

        def _bad_header(*_args, **_kwargs):
            raise Exception("bad-header")

        monkeypatch.setattr(oidc_mod.jwt, "get_unverified_header", _bad_header)
        with pytest.raises(ValueError) as excinfo:
            client.parse_id_token("token")
        message = str(excinfo.value).lower()
        assert "id token" in message

    def test_oidc_fetch_user_info_merges_token_and_userinfo(self, monkeypatch):
        oidc_mod = _load_oidc_module()
        monkeypatch.setattr(oidc_mod.OIDCClient, "_load_oidc_metadata", lambda *_args, **_kwargs: self._metadata())
        client = oidc_mod.OIDCClient(self._base_config())

        def _parse_id_token(_token):
            return {"email": "token@example.com"}

        class _StubUserInfo:
            def __init__(self, payload):
                self._payload = payload

            def to_dict(self):
                return self._payload

        monkeypatch.setattr(client, "parse_id_token", _parse_id_token)
        monkeypatch.setattr(
            oidc_mod.OAuthClient,
            "fetch_user_info",
            lambda *_args, **_kwargs: _StubUserInfo(
                {
                    "email": "userinfo@example.com",
                    "username": "user",
                    "nickname": "User",
                    "avatar_url": "http://avatar",
                }
            ),
        )
        info = client.fetch_user_info("access-token", id_token="id-token")
        assert info.email == "userinfo@example.com"
        assert info.username == "user"
