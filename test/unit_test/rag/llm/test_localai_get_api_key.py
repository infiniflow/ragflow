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
"""Unit tests for ``rag.llm.model_meta.LocalAI._get_api_key``.

Regression for #17757: when a user adds a LocalAI provider with an api_key
that the API service has stored as a JSON dict (the format used for several
other LLM providers -- see ``api/apps/services/provider_api_service.py:313``
which calls ``json.dumps(api_key)`` for non-string values), the
``Base._get_api_key`` default returns the raw JSON string. The
``LocalAI.get_model_list`` verify path then sends
``Authorization: Bearer {"api_key": "sk-xxx", "endpoint": "..."}`` which
LocalAI rejects with 401, and the UI surfaces "102 no models found for
provider LocalAI".

LocalAI typically does not require auth, so the right behavior is:

* JSON dict with an ``api_key`` field  -> the inner key (matches
  ``VolcEngine._get_api_key`` / ``OpenRouter._get_api_key`` / ``NewAPI._get_api_key``
  in the same file).
* JSON dict without an ``api_key`` field  -> ``""`` so ``get_model_list``
  takes the no-auth path (LocalAI's normal case).
* Plain string  -> returned as-is (the historical pre-fix passthrough).
* JSON parse error, JSON non-object, or non-string api_key  -> the base
  default (raw passthrough) so we do not regress any caller depending on it.
"""

import pytest

from rag.llm.model_meta import LocalAI

pytestmark = pytest.mark.p2


def _make(api_key):
    return LocalAI(api_key=api_key, base_url="http://127.0.0.1:8080")


# --- JSON dict with an api_key field (the bug fix) --------------------------


def test_json_dict_with_api_key_returns_inner_key():
    """The api_key is extracted from the JSON dict so the Bearer header is valid."""
    provider = _make('{"api_key": "sk-local-1234", "endpoint": "http://x"}')
    assert provider._get_api_key() == "sk-local-1234"


def test_json_dict_with_empty_api_key_returns_inner_key():
    """An empty inner key is preserved (caller's intent) and surfaces as no-auth header."""
    provider = _make('{"api_key": "", "endpoint": "http://x"}')
    assert provider._get_api_key() == ""


def test_json_dict_with_only_whitespace_api_key_returns_inner_key():
    provider = _make('{"api_key": "   "}')
    assert provider._get_api_key() == "   "


# --- JSON dict without an api_key field (LocalAI's normal no-auth case) ------


def test_json_dict_without_api_key_field_returns_empty_string():
    """A user entered ``{"endpoint": "http://x"}`` -- no key field.

    Returning ``""`` (not the raw JSON) means ``get_model_list`` takes its
    no-auth path: ``if self.api_key:`` is False, so the LocalAI server is
    called without an ``Authorization`` header -- which it accepts by
    default and the verify endpoint returns the model list.
    """
    provider = _make('{"endpoint": "http://x", "model": "llama3"}')
    assert provider._get_api_key() == ""


def test_json_empty_dict_returns_empty_string():
    provider = _make("{}")
    assert provider._get_api_key() == ""


# --- Plain string (the historical passthrough -- pre-fix behaviour) ----------


def test_plain_string_returns_as_is():
    """A plain string api_key is the historical Base default. No JSON, no change."""
    provider = _make("sk-plain-key")
    assert provider._get_api_key() == "sk-plain-key"


def test_empty_string_returns_empty_string():
    """Empty string is falsy in ``get_model_list``; no Authorization header is sent."""
    provider = _make("")
    assert provider._get_api_key() == ""


# --- JSON non-object (the historical passthrough) ----------------------------


def test_json_list_returns_raw_to_preserve_pre_fix_behavior():
    """A JSON list would produce a malformed Bearer header either way.

    Pre-fix returned the raw JSON string. Post-fix keeps that exact behavior
    so we do not silently regress any caller that depends on the raw
    passthrough -- the malformed-Bearer path is a separate, pre-existing
    concern, not in scope of #17757.
    """
    provider = _make('["sk-1", "sk-2"]')
    assert provider._get_api_key() == '["sk-1", "sk-2"]'


def test_json_string_returns_raw_to_preserve_pre_fix_behavior():
    """A JSON string (e.g. ``"\"sk-xxx\""``) falls back to the raw passthrough."""
    provider = _make('"sk-quoted"')
    assert provider._get_api_key() == '"sk-quoted"'


# --- Malformed JSON (the historical passthrough) -----------------------------


def test_malformed_json_returns_raw_to_preserve_pre_fix_behavior():
    """A string that looks like JSON but cannot be parsed falls back to raw."""
    provider = _make('{"api_key": "unterminated')
    assert provider._get_api_key() == '{"api_key": "unterminated'


# --- Integration: get_model_list takes the no-auth path on empty resolved key


def test_get_model_list_skips_authorization_header_when_resolved_key_is_empty():
    """The Authorization header is omitted when the resolved api_key is empty.

    Verified by inspecting the ``headers`` kwarg of the mocked
    ``session.get`` call: the dict must not contain ``Authorization``.
    """
    from unittest.mock import AsyncMock, MagicMock, patch

    async def _run():
        # ``{"endpoint": "..."}`` -- the JSON-dict case that pre-fix produced
        # the malformed-Bearer 401. Post-fix resolves to ``""`` and the
        # verify path sends no Authorization header at all.
        provider = LocalAI(
            api_key='{"endpoint": "http://127.0.0.1:8080"}',
            base_url="http://127.0.0.1:8080",
        )
        assert provider._get_api_key() == ""

        tags_payload = {"models": [{"name": "llama3", "model": "llama3"}]}
        show_payload = {
            "model_info": {"general.context_length": 8192},
            "capabilities": ["completion"],
        }

        def _mock_resp(payload, status=200):
            resp = MagicMock()
            resp.status = status
            resp.json = AsyncMock(return_value=payload)
            return resp

        tags_resp = _mock_resp(tags_payload)
        show_resp = _mock_resp(show_payload)
        wrapper = MagicMock()
        wrapper.__aenter__ = AsyncMock(return_value=tags_resp)
        wrapper.__aexit__ = AsyncMock(return_value=None)
        show_wrapper = MagicMock()
        show_wrapper.__aenter__ = AsyncMock(return_value=show_resp)
        show_wrapper.__aexit__ = AsyncMock(return_value=None)

        session = MagicMock()
        session.get.return_value = wrapper
        session.post.return_value = show_wrapper
        session_cls = MagicMock()
        session_cls.__aenter__ = AsyncMock(return_value=session)
        session_cls.__aexit__ = AsyncMock(return_value=None)

        with patch("aiohttp.ClientSession", return_value=session_cls):
            return await provider.get_model_list(), session

    import asyncio

    models, session = asyncio.run(_run())
    assert [m["name"] for m in models] == ["llama3"]

    get_headers = session.get.call_args.kwargs["headers"]
    post_headers = session.post.call_args.kwargs["headers"]
    assert "Authorization" not in get_headers
    assert "Authorization" not in post_headers


def test_get_model_list_sends_valid_bearer_when_json_dict_has_api_key():
    """The Bearer header is the inner key, not the raw JSON dict.

    Pre-fix: ``Bearer {"api_key": "sk-local", "endpoint": "..."}`` -> 401.
    Post-fix: ``Bearer sk-local`` -> 200.
    """
    from unittest.mock import AsyncMock, MagicMock, patch

    async def _run():
        provider = LocalAI(
            api_key='{"api_key": "sk-local", "endpoint": "http://x"}',
            base_url="http://127.0.0.1:8080",
        )
        assert provider._get_api_key() == "sk-local"

        tags_payload = {"models": [{"name": "llama3", "model": "llama3"}]}
        show_payload = {
            "model_info": {"general.context_length": 8192},
            "capabilities": ["completion"],
        }

        def _mock_resp(payload, status=200):
            resp = MagicMock()
            resp.status = status
            resp.json = AsyncMock(return_value=payload)
            return resp

        tags_resp = _mock_resp(tags_payload)
        show_resp = _mock_resp(show_payload)
        wrapper = MagicMock()
        wrapper.__aenter__ = AsyncMock(return_value=tags_resp)
        wrapper.__aexit__ = AsyncMock(return_value=None)
        show_wrapper = MagicMock()
        show_wrapper.__aenter__ = AsyncMock(return_value=show_resp)
        show_wrapper.__aexit__ = AsyncMock(return_value=None)

        session = MagicMock()
        session.get.return_value = wrapper
        session.post.return_value = show_wrapper
        session_cls = MagicMock()
        session_cls.__aenter__ = AsyncMock(return_value=session)
        session_cls.__aexit__ = AsyncMock(return_value=None)

        with patch("aiohttp.ClientSession", return_value=session_cls):
            return await provider.get_model_list(), session

    import asyncio

    models, session = asyncio.run(_run())
    assert [m["name"] for m in models] == ["llama3"]
    assert session.get.call_args.kwargs["headers"]["Authorization"] == "Bearer sk-local"
    assert session.post.call_args.kwargs["headers"]["Authorization"] == "Bearer sk-local"


# --- Pre-fix behaviour preserved for plain strings (regression guard) -------


def test_get_model_list_sends_bearer_for_plain_string_api_key():
    """A plain-string api_key still produces a plain Bearer header."""
    from unittest.mock import AsyncMock, MagicMock, patch

    async def _run():
        provider = LocalAI(
            api_key="sk-plain",
            base_url="http://127.0.0.1:8080",
        )
        assert provider._get_api_key() == "sk-plain"

        tags_payload = {"models": [{"name": "llama3", "model": "llama3"}]}
        show_payload = {
            "model_info": {"general.context_length": 8192},
            "capabilities": ["completion"],
        }

        def _mock_resp(payload, status=200):
            resp = MagicMock()
            resp.status = status
            resp.json = AsyncMock(return_value=payload)
            return resp

        tags_resp = _mock_resp(tags_payload)
        show_resp = _mock_resp(show_payload)
        wrapper = MagicMock()
        wrapper.__aenter__ = AsyncMock(return_value=tags_resp)
        wrapper.__aexit__ = AsyncMock(return_value=None)
        show_wrapper = MagicMock()
        show_wrapper.__aenter__ = AsyncMock(return_value=show_resp)
        show_wrapper.__aexit__ = AsyncMock(return_value=None)

        session = MagicMock()
        session.get.return_value = wrapper
        session.post.return_value = show_wrapper
        session_cls = MagicMock()
        session_cls.__aenter__ = AsyncMock(return_value=session)
        session_cls.__aexit__ = AsyncMock(return_value=None)

        with patch("aiohttp.ClientSession", return_value=session_cls):
            return await provider.get_model_list(), session

    import asyncio

    models, session = asyncio.run(_run())
    assert [m["name"] for m in models] == ["llama3"]
    assert session.get.call_args.kwargs["headers"]["Authorization"] == "Bearer sk-plain"


@pytest.mark.parametrize("api_key", ["", None])
def test_get_model_list_skips_authorization_for_empty_or_none_api_key(api_key):
    """``None`` and ``""`` both resolve to no-auth -- ``if self.api_key:`` is False."""
    from unittest.mock import AsyncMock, MagicMock, patch

    async def _run():
        provider = LocalAI(api_key=api_key, base_url="http://127.0.0.1:8080")
        assert provider._get_api_key() == ""

        tags_payload = {"models": [{"name": "llama3", "model": "llama3"}]}
        show_payload = {
            "model_info": {"general.context_length": 8192},
            "capabilities": ["completion"],
        }

        def _mock_resp(payload, status=200):
            resp = MagicMock()
            resp.status = status
            resp.json = AsyncMock(return_value=payload)
            return resp

        tags_resp = _mock_resp(tags_payload)
        show_resp = _mock_resp(show_payload)
        wrapper = MagicMock()
        wrapper.__aenter__ = AsyncMock(return_value=tags_resp)
        wrapper.__aexit__ = AsyncMock(return_value=None)
        show_wrapper = MagicMock()
        show_wrapper.__aenter__ = AsyncMock(return_value=show_resp)
        show_wrapper.__aexit__ = AsyncMock(return_value=None)

        session = MagicMock()
        session.get.return_value = wrapper
        session.post.return_value = show_wrapper
        session_cls = MagicMock()
        session_cls.__aenter__ = AsyncMock(return_value=session)
        session_cls.__aexit__ = AsyncMock(return_value=None)

        with patch("aiohttp.ClientSession", return_value=session_cls):
            return await provider.get_model_list(), session

    import asyncio

    models, session = asyncio.run(_run())
    assert [m["name"] for m in models] == ["llama3"]
    assert "Authorization" not in session.get.call_args.kwargs["headers"]
    assert "Authorization" not in session.post.call_args.kwargs["headers"]
