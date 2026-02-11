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
import importlib.util
import sys
import types

import pytest


def _ensure_pkg(name: str):
    if name in sys.modules:
        return sys.modules[name]
    pkg = types.ModuleType(name)
    pkg.__path__ = []
    sys.modules[name] = pkg
    return pkg


def _load_module(module_name: str, file_path: Path):
    spec = importlib.util.spec_from_file_location(module_name, file_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


def _load_auth_init_module():
    root = Path(__file__).resolve().parents[4]
    _ensure_pkg("api")
    _ensure_pkg("api.apps")
    _ensure_pkg("api.apps.auth")
    _load_module("api.apps.auth.oauth", root / "api" / "apps" / "auth" / "oauth.py")
    _load_module("api.apps.auth.oidc", root / "api" / "apps" / "auth" / "oidc.py")
    _load_module("api.apps.auth.github", root / "api" / "apps" / "auth" / "github.py")
    return _load_module("api.apps.auth", root / "api" / "apps" / "auth" / "__init__.py")


def _oauth_config():
    return {
        "client_id": "client_id",
        "client_secret": "client_secret",
        "authorization_url": "https://example.com/auth",
        "token_url": "https://example.com/token",
        "userinfo_url": "https://example.com/userinfo",
        "redirect_uri": "https://example.com/callback",
    }


def _oidc_metadata(issuer):
    return {
        "issuer": issuer,
        "jwks_uri": f"{issuer}/jwks",
        "authorization_endpoint": f"{issuer}/auth",
        "token_endpoint": f"{issuer}/token",
        "userinfo_endpoint": f"{issuer}/userinfo",
    }


@pytest.mark.p2
class TestAuthInit:
    def test_get_auth_client_type_routing(self, monkeypatch):
        auth_mod = _load_auth_init_module()
        monkeypatch.setattr(
            auth_mod.OIDCClient,
            "_load_oidc_metadata",
            staticmethod(_oidc_metadata),
        )

        client = auth_mod.get_auth_client(
            {
                "type": "github",
                "client_id": "client_id",
                "client_secret": "client_secret",
                "redirect_uri": "https://example.com/callback",
            }
        )
        assert isinstance(client, auth_mod.GithubOAuthClient)

        client = auth_mod.get_auth_client({"type": "oauth2", **_oauth_config()})
        assert isinstance(client, auth_mod.OAuthClient)

        client = auth_mod.get_auth_client(
            {
                "type": "oidc",
                "issuer": "https://issuer.example",
                "client_id": "client_id",
                "client_secret": "client_secret",
                "redirect_uri": "https://example.com/callback",
            }
        )
        assert isinstance(client, auth_mod.OIDCClient)

        client = auth_mod.get_auth_client(
            {
                "issuer": "https://issuer.example",
                "client_id": "client_id",
                "client_secret": "client_secret",
                "redirect_uri": "https://example.com/callback",
            }
        )
        assert isinstance(client, auth_mod.OIDCClient)

        client = auth_mod.get_auth_client(_oauth_config())
        assert isinstance(client, auth_mod.OAuthClient)

    def test_get_auth_client_unsupported_type(self):
        auth_mod = _load_auth_init_module()
        with pytest.raises(ValueError) as excinfo:
            auth_mod.get_auth_client({"type": "bad"})
        message = str(excinfo.value).lower()
        assert "unsupported" in message and "type" in message
