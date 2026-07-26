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


def _load_github_module(monkeypatch):
    _load_auth_modules(monkeypatch)
    repo_root = Path(__file__).resolve().parents[4]

    sys.modules.pop("api.apps.auth.github", None)
    github_path = repo_root / "api" / "apps" / "auth" / "github.py"
    github_spec = importlib.util.spec_from_file_location("api.apps.auth.github", github_path)
    github_module = importlib.util.module_from_spec(github_spec)
    monkeypatch.setitem(sys.modules, "api.apps.auth.github", github_module)
    github_spec.loader.exec_module(github_module)
    return github_module


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


def _metadata(issuer, signing_algs=None):
    md = {
        "issuer": issuer,
        "jwks_uri": f"{issuer}/jwks",
        "authorization_endpoint": f"{issuer}/authorize",
        "token_endpoint": f"{issuer}/token",
        "userinfo_endpoint": f"{issuer}/userinfo",
    }
    if signing_algs is not None:
        md["id_token_signing_alg_values_supported"] = signing_algs
    return md


def _make_client(monkeypatch, oidc_module, signing_algs=None):
    monkeypatch.setattr(
        oidc_module.OIDCClient,
        "_load_oidc_metadata",
        staticmethod(lambda issuer: _metadata(issuer, signing_algs=signing_algs)),
    )
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


# ===================================================================== #
# JWT algorithm-confusion regression tests                              #
#                                                                       #
# Before the fix, ``parse_id_token`` read the signing algorithm from    #
# the unverified JWT header. An attacker who presents a JWT with        #
# ``"alg": "none"`` would have signature verification disabled, and an  #
# attacker who presents ``"alg": "HS256"`` and signs the JWT with the   #
# public key bytes (RSA / HMAC confusion) would get the forged token    #
# accepted. The tests below pin the contract:                           #
#                                                                       #
#   - the algorithm allowlist is pinned at construction time from the   #
#     provider's discovery metadata intersected with the safe allowlist #
#   - the JWT header's ``alg`` claim is never read at decode time       #
# ===================================================================== #


@pytest.mark.p2
def test_id_token_signing_algs_default_to_rs256_when_metadata_missing(monkeypatch):
    """No ``id_token_signing_alg_values_supported`` in metadata → RS256 only.

    Crucially the fallback is RS256, never whatever the JWT header claims.
    """
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)
    assert client.id_token_signing_algs == ["RS256"]


@pytest.mark.p2
def test_id_token_signing_algs_intersect_metadata_with_safe_allowlist(monkeypatch):
    """Metadata advertises a mix of safe and unsafe algs — only safe ones kept."""
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(
        monkeypatch,
        oidc_module,
        signing_algs=["RS256", "ES256", "HS256", "none", "PS512"],
    )
    assert set(client.id_token_signing_algs) == {"RS256", "ES256", "PS512"}
    # The dangerous algorithms must not appear in the verification allowlist.
    assert "HS256" not in client.id_token_signing_algs
    assert "none" not in client.id_token_signing_algs


@pytest.mark.p2
def test_id_token_signing_algs_fall_back_when_only_unsafe_advertised(monkeypatch):
    """Provider advertises only HS256 / none → fall back to RS256, do not trust."""
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(
        monkeypatch,
        oidc_module,
        signing_algs=["HS256", "none", "bogus"],
    )
    assert client.id_token_signing_algs == ["RS256"]


@pytest.mark.p2
def test_id_token_signing_algs_ignores_non_string_entries(monkeypatch):
    """Malformed entries (None / dict / int) are filtered out, not crashed on."""
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(
        monkeypatch,
        oidc_module,
        signing_algs=["RS256", None, 42, {"x": 1}, "ES384"],
    )
    assert set(client.id_token_signing_algs) == {"RS256", "ES384"}


@pytest.mark.p2
def test_id_token_signing_algs_handles_non_list_metadata_field(monkeypatch):
    """If metadata gives a non-list type for the field, fall back to default."""
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module, signing_algs="RS256")
    assert client.id_token_signing_algs == ["RS256"]


@pytest.mark.p2
def test_parse_id_token_passes_pinned_algorithms_to_jwt_decode(monkeypatch):
    """``jwt.decode`` receives the pinned allowlist, regardless of JWT header."""
    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module, signing_algs=["RS256", "ES256"])

    # Even if the unverified header claims something dangerous, the
    # verification path must not consult it. We sabotage
    # ``jwt.get_unverified_header`` to prove the code never calls it.
    def _explode(_token):  # pragma: no cover - must not be called
        raise AssertionError("parse_id_token must not read the algorithm from the unverified JWT header")

    monkeypatch.setattr(oidc_module.jwt, "get_unverified_header", _explode)
    monkeypatch.setattr(oidc_module.jwt, "PyJWKClient", _DummyJwkClient)

    seen = {}

    def _decode(id_token, key, algorithms, audience, issuer):
        seen["algorithms"] = list(algorithms)
        return {"sub": "user-2"}

    monkeypatch.setattr(oidc_module.jwt, "decode", _decode)
    client.parse_id_token("malicious-header-token")

    assert set(seen["algorithms"]) == {"RS256", "ES256"}
    # Hard-stop: dangerous algorithms must never reach ``jwt.decode``.
    assert "none" not in seen["algorithms"]
    assert "HS256" not in seen["algorithms"]


@pytest.mark.p2
def test_parse_id_token_rejects_alg_none(monkeypatch):
    """End-to-end: an ``alg: "none"`` JWT must not authenticate.

    Uses the real PyJWT decoder so the test exercises the actual contract
    between ``parse_id_token`` and the upstream library.
    """
    import jwt as real_jwt

    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)  # defaults to RS256

    # PyJWT requires explicit opt-in to encode ``alg=none``; even then it
    # produces a token with no signature segment.
    forged = real_jwt.encode(
        {
            "sub": "victim-subject",
            "email": "admin@target.example",
            "aud": "client-1",
            "iss": "https://issuer.example",
        },
        key="",
        algorithm="none",
    )
    # Force the JWKS step into a no-op so we exercise *just* the alg gate.
    monkeypatch.setattr(oidc_module.jwt, "PyJWKClient", _DummyJwkClient)

    with pytest.raises(ValueError) as exc_info:
        client.parse_id_token(forged)
    assert "Error parsing ID Token" in str(exc_info.value)


@pytest.mark.p2
def test_parse_id_token_rejects_hs256_when_allowlist_is_asymmetric(monkeypatch):
    """End-to-end: a JWT whose header claims ``alg: HS256`` must not be
    accepted when the pinned allowlist is asymmetric-only.

    This is the algorithm half of the RSA / HMAC confusion attack
    (CWE-347). The attacker forges a JWT with ``"alg": "HS256"`` so the
    server picks the HMAC verifier; pre-fix the server would read that alg
    straight from the header and call
    ``jwt.decode(..., algorithms=["HS256"], key=public_key)`` which lets
    the attacker forge tokens with the public key bytes. After the fix the
    allowlist pinned at construction time wins — HS* is never in it — so
    PyJWT raises ``InvalidAlgorithmError`` before the HMAC verifier is
    ever invoked.

    Note: modern PyJWT (>=2.0) also independently refuses to use a
    PEM-encoded key as an HMAC secret, so the public-key-bytes leg of the
    full attack is partially mitigated at the library level. The fix here
    is defense in depth and the only mitigation for non-PEM key formats
    (raw bytes, DER, JWK octet keys, older PyJWT versions).
    """
    import jwt as real_jwt

    _, oidc_module = _load_auth_modules(monkeypatch)
    client = _make_client(monkeypatch, oidc_module)  # defaults to RS256

    # Use a non-PEM byte string so we exercise the alg gate (not PyJWT's
    # incidental PEM-as-HMAC-secret refusal).
    shared_secret = b"shared-secret-bytes-not-a-pem-key"
    forged = real_jwt.encode(
        {
            "sub": "victim-subject",
            "email": "admin@target.example",
            "aud": "client-1",
            "iss": "https://issuer.example",
        },
        key=shared_secret,
        algorithm="HS256",
    )

    class _SecretJwkClient(_DummyJwkClient):
        def get_signing_key_from_jwt(self, _id_token):
            return SimpleNamespace(key=shared_secret)

    monkeypatch.setattr(oidc_module.jwt, "PyJWKClient", _SecretJwkClient)

    with pytest.raises(ValueError) as exc_info:
        client.parse_id_token(forged)
    assert "Error parsing ID Token" in str(exc_info.value)


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


@pytest.mark.p2
def test_github_oauth_client_init_and_normalize_unit(monkeypatch):
    github_module = _load_github_module(monkeypatch)

    client = github_module.GithubOAuthClient(_base_config())
    assert client.authorization_url == "https://github.com/login/oauth/authorize"
    assert client.token_url == "https://github.com/login/oauth/access_token"
    assert client.userinfo_url == "https://api.github.com/user"
    assert client.scope == "user:email"

    normalized = client.normalize_user_info(
        {
            "email": "octo@example.com",
            "login": "octocat",
            "name": "Octo Cat",
            "avatar_url": "https://avatar.example/octocat.png",
        }
    )
    assert normalized.to_dict() == {
        "email": "octo@example.com",
        "username": "octocat",
        "nickname": "Octo Cat",
        "avatar_url": "https://avatar.example/octocat.png",
    }

    normalized_fallback = client.normalize_user_info({"email": "fallback@example.com"})
    assert normalized_fallback.to_dict() == {
        "email": "fallback@example.com",
        "username": "fallback",
        "nickname": "fallback",
        "avatar_url": "",
    }


@pytest.mark.p2
def test_github_fetch_user_info_sync_success_and_error_unit(monkeypatch):
    github_module = _load_github_module(monkeypatch)
    client = github_module.GithubOAuthClient(_base_config())

    calls = []

    def _fake_sync_request(method, url, headers=None, timeout=None):
        calls.append((method, url, headers, timeout))
        if url.endswith("/emails"):
            return _FakeResponse(
                [
                    {"email": "other@example.com", "primary": False},
                    {"email": "octo@example.com", "primary": True},
                ]
            )
        return _FakeResponse({"login": "octocat", "name": "Octo Cat", "avatar_url": "https://avatar.example/octocat.png"})

    monkeypatch.setattr(github_module, "sync_request", _fake_sync_request)
    info = client.fetch_user_info("sync-token")

    assert info.to_dict() == {
        "email": "octo@example.com",
        "username": "octocat",
        "nickname": "Octo Cat",
        "avatar_url": "https://avatar.example/octocat.png",
    }
    assert [call[1] for call in calls] == [
        "https://api.github.com/user",
        "https://api.github.com/user/emails",
    ]
    assert all(call[2]["Authorization"] == "Bearer sync-token" for call in calls)
    assert all(call[3] == 7 for call in calls)

    def _sync_request_raises(*_args, **_kwargs):
        return _FakeResponse(err=RuntimeError("status boom"))

    monkeypatch.setattr(github_module, "sync_request", _sync_request_raises)
    with pytest.raises(ValueError, match="Failed to fetch github user info: status boom"):
        client.fetch_user_info("sync-token")


@pytest.mark.p2
def test_github_fetch_user_info_async_success_and_error_unit(monkeypatch):
    github_module = _load_github_module(monkeypatch)
    client = github_module.GithubOAuthClient(_base_config())

    calls = []

    async def _fake_async_request(method, url, headers=None, **kwargs):
        calls.append((method, url, headers, kwargs.get("timeout")))
        if url.endswith("/emails"):
            return _FakeResponse(
                [
                    {"email": "other@example.com", "primary": False},
                    {"email": "octo-async@example.com", "primary": True},
                ]
            )
        return _FakeResponse({"login": "octocat-async", "name": "Octo Async", "avatar_url": "https://avatar.example/octo-async.png"})

    monkeypatch.setattr(github_module, "async_request", _fake_async_request)
    info = asyncio.run(client.async_fetch_user_info("async-token"))

    assert info.to_dict() == {
        "email": "octo-async@example.com",
        "username": "octocat-async",
        "nickname": "Octo Async",
        "avatar_url": "https://avatar.example/octo-async.png",
    }
    assert [call[1] for call in calls] == [
        "https://api.github.com/user",
        "https://api.github.com/user/emails",
    ]
    assert all(call[2]["Authorization"] == "Bearer async-token" for call in calls)
    assert all(call[3] == 7 for call in calls)

    async def _async_request_raises(*_args, **_kwargs):
        return _FakeResponse(err=RuntimeError("async status boom"))

    monkeypatch.setattr(github_module, "async_request", _async_request_raises)
    with pytest.raises(ValueError, match="Failed to fetch github user info: async status boom"):
        asyncio.run(client.async_fetch_user_info("async-token"))
