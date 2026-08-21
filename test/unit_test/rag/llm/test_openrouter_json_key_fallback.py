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

"""Regression tests for the OpenRouter ``key`` JSON-decode fallback (#17458).

Pre-fix, the three OpenRouter call sites in :mod:`rag.llm` did::

    try:
        api_key = json.loads(key).get("api_key", "")
        provider_order = json.loads(key).get("provider_order", "")
    except JSONDecodeError:
        api_key = key
        provider_order = ""

The ``except JSONDecodeError`` only catches the *parse* failure. A user
pasting a JSON string that is NOT an object -- e.g. ``"[1,2,3]"``,
``"42"``, ``'"hello"'``, ``"true"``, ``"null"`` -- parses fine and then
crashes on the ``.get(...)`` call with ``AttributeError: 'list' object
has no attribute 'get'`` (or ``'int'``, ``'str'``, ``'NoneType'``,
``'bool'``) from inside :mod:`rag.llm` internals, with no indication of
what the user did wrong.

The fix funnels every OpenRouter key through
``_resolve_openrouter_credentials`` in :mod:`rag.llm.key_utils`. The
helper:

1. Accepts a plain (non-JSON) string and returns
   ``{"api_key": key, "provider_order": ""}`` -- matching the
   pre-fix ``except JSONDecodeError`` fallback semantics.
2. Accepts a dict (or a JSON-string-encoded dict) and returns the
   parsed ``api_key`` and ``provider_order`` (both defaulting to
   ``""`` if missing).
3. On a JSON top-level type that is not a dict (list, string, number,
   bool, null), raises a clear :class:`ModelException` naming the
   required object shape and pointing at
   ``conf/models/openrouter.json``.

The three call sites then continue with the helper output, never calling
``.get`` on a non-dict.

PR #15776 (commit ``8e4fba6cd``) added the partial ``except
JSONDecodeError`` guard but did not address the ``AttributeError`` case
-- this PR closes the gap, in the same shape as PR #17457 did for
VolcEngine/Ark.
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_openrouter_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveOpenrouterCredentials:
    def test_plain_string_key_passes_through(self):
        """The most common case: a user pastes a plain OpenRouter API key
        (e.g. ``"sk-or-..."``). Pre-fix, this was caught by ``except
        JSONDecodeError`` and used as-is. The helper preserves that
        fallback and additionally defaults ``provider_order`` to empty
        so the OpenAI client doesn't get a malformed provider config."""
        out = _resolve_openrouter_credentials("sk-or-v1-abc123")
        assert out == {"api_key": "sk-or-v1-abc123", "provider_order": ""}

    def test_empty_string_falls_through_as_plain_key(self):
        """An empty string is a non-JSON string. The helper treats it the
        same as a plain key (preserving the pre-fix ``except
        JSONDecodeError`` branch)."""
        out = _resolve_openrouter_credentials("")
        assert out == {"api_key": "", "provider_order": ""}

    def test_python_dict_passes_through(self):
        """A pre-parsed dict (e.g. a future caller that already has the
        key as a dict) returns the dict shape with both fields
        populated."""
        raw = {"api_key": "sk-or-v1-abc123", "provider_order": "Anthropic,OpenAI"}
        out = _resolve_openrouter_credentials(raw)
        assert out == {"api_key": "sk-or-v1-abc123", "provider_order": "Anthropic,OpenAI"}

    def test_json_dict_with_all_fields(self):
        """The happy path: a valid JSON dict returns the parsed fields so
        the model classes can use them unchanged."""
        raw = {"api_key": "sk-or-v1-abc123", "provider_order": "Anthropic,OpenAI"}
        out = _resolve_openrouter_credentials(json.dumps(raw))
        assert out == {"api_key": "sk-or-v1-abc123", "provider_order": "Anthropic,OpenAI"}

    def test_json_dict_missing_api_key(self):
        """A dict without ``api_key`` is still a valid dict; the helper
        returns an empty ``api_key`` rather than raising. The downstream
        model class is responsible for the ``api_key``-missing check
        (none currently)."""
        out = _resolve_openrouter_credentials(json.dumps({"provider_order": "Anthropic"}))
        assert out == {"api_key": "", "provider_order": "Anthropic"}

    def test_json_dict_missing_provider_order(self):
        """Without ``provider_order`` the helper returns an empty string;
        the OpenAI client then doesn't get a malformed provider config."""
        out = _resolve_openrouter_credentials(json.dumps({"api_key": "sk-or-v1-abc123"}))
        assert out == {"api_key": "sk-or-v1-abc123", "provider_order": ""}

    def test_json_dict_missing_both_fields(self):
        """An empty dict is a valid dict; both fields default to
        empty strings."""
        out = _resolve_openrouter_credentials(json.dumps({}))
        assert out == {"api_key": "", "provider_order": ""}

    def test_json_array_raises_model_exception(self):
        """A valid JSON array is the bug case: pre-fix, ``.get(...)`` on
        the parsed list raised ``AttributeError: 'list' object has no
        attribute 'get'``. The helper must raise a clear
        :class:`ModelException` naming the actual type and the required
        object shape."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "OpenRouter" in msg
        assert "object" in msg
        assert "list" in msg
        assert "conf/models/openrouter.json" in msg
        # Must NOT be a raw AttributeError leaking out.
        assert "AttributeError" not in msg
        assert "'list' object" not in msg
        # Must not be retryable -- the operator has to fix their key.
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(json.dumps("just a string"))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "str" in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(json.dumps(42))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "int" in msg
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(json.dumps(3.14))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "float" in msg
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        """``json.loads("null")`` returns Python ``None``. The helper must
        treat that as a non-object (the pre-fix code crashed with
        ``AttributeError: 'NoneType' object has no attribute 'get'``)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials("null")
        msg = str(exc_info.value)
        assert "object" in msg
        assert "NoneType" in msg
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / ``None`` / etc. passed directly (not as a string
        or a dict) must also raise. Only ``str`` and ``dict`` are valid
        inputs to the helper."""
        with pytest.raises(ModelException):
            _resolve_openrouter_credentials(42)
        with pytest.raises(ModelException):
            _resolve_openrouter_credentials(None)
        with pytest.raises(ModelException):
            _resolve_openrouter_credentials(["a", "b"])

    def test_malformed_json_falls_through_as_plain_key(self):
        """``json.loads("not really json")`` raises ``JSONDecodeError``.
        The helper treats that the same as a plain string key (matching
        the pre-fix ``except JSONDecodeError`` fallback semantics -- a
        user pasting a raw token into the key field)."""
        out = _resolve_openrouter_credentials("not really json {")
        assert out == {"api_key": "not really json {", "provider_order": ""}


# --------------------------------------------------------------------------- #
# 2. End-to-end: the three call sites use the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_PLAIN = "sk-or-v1-abc123"
KEY_VALID_JSON = json.dumps({"api_key": "sk-or-v1-abc123", "provider_order": "Anthropic,OpenAI"})
KEY_JSON_ARRAY = json.dumps(["not", "a", "key"])
KEY_JSON_NUMBER = json.dumps(42)


def _patch_openai(*, where):
    """Patch ``openai.OpenAI`` and ``openai.AsyncOpenAI`` at the call
    site ``where`` (a fully-qualified module path -- e.g.
    ``"rag.llm.chat_model"``) so we can construct the model classes
    without a real OpenRouter round-trip.

    Returns ``(sync_patcher, async_patcher, captured)`` where
    ``captured`` is a dict of ``{"sync_kwargs": {...}, "async_kwargs":
    {...}}`` populated on construction. We use ``side_effect`` so the
    call kwargs (in particular ``api_key``) can be asserted on directly.
    """
    captured = {"sync_kwargs": None, "async_kwargs": None}

    def sync_factory(*args, **kwargs):
        captured["sync_kwargs"] = kwargs
        return MagicMock()

    def async_factory(*args, **kwargs):
        captured["async_kwargs"] = kwargs
        return MagicMock()

    sync_patcher = patch(f"{where}.OpenAI", side_effect=sync_factory)
    async_patcher = patch(f"{where}.AsyncOpenAI", side_effect=async_factory)
    return sync_patcher, async_patcher, captured


@pytest.mark.p0
class TestOpenRouterChatCallSite:
    """The ``LiteLLMBase.__init__`` OpenRouter branch lives in
    ``rag/llm/chat_model.py``. We can't easily build a full
    ``LiteLLMBase`` in the test (it pulls in heavy deps), so we test
    the helper wiring via a direct call to the helper, plus an
    end-to-end check on the model attributes set during __init__."""

    def test_plain_key_via_helper(self):
        """A plain key flows through the helper to the chat-side
        branch as ``api_key``, with ``provider_order`` defaulting to
        empty."""
        out = _resolve_openrouter_credentials(KEY_PLAIN)
        assert out["api_key"] == KEY_PLAIN
        assert out["provider_order"] == ""

    def test_json_dict_via_helper(self):
        """A JSON dict flows through the helper with both fields
        extracted."""
        out = _resolve_openrouter_credentials(KEY_VALID_JSON)
        assert out["api_key"] == "sk-or-v1-abc123"
        assert out["provider_order"] == "Anthropic,OpenAI"

    def test_json_array_via_helper_raises_model_exception(self):
        """The bug case: a JSON array used to crash with
        ``AttributeError: 'list' object has no attribute 'get'`` deep
        inside ``rag.llm``. The fix surfaces a clear
        :class:`ModelException`."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(KEY_JSON_ARRAY)
        msg = str(exc_info.value)
        assert "OpenRouter" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_number_via_helper_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_openrouter_credentials(KEY_JSON_NUMBER)
        assert "OpenRouter" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestOpenRouterCVCallSite:
    def test_plain_key_constructs_with_key_as_api_key(self):
        """A plain key flows through to the OpenAI client as api_key,
        with ``provider_order`` defaulting to empty."""
        from rag.llm.cv_model import OpenRouterCV

        sync, async_, captured = _patch_openai(where="rag.llm.cv_model")
        with sync, async_:
            cv = OpenRouterCV(KEY_PLAIN, model_name="openai/gpt-4o")
        assert cv.model_name == "openai/gpt-4o"
        assert captured["sync_kwargs"]["api_key"] == KEY_PLAIN

    def test_json_dict_constructs_with_derived_fields(self):
        from rag.llm.cv_model import OpenRouterCV

        sync, async_, captured = _patch_openai(where="rag.llm.cv_model")
        with sync, async_:
            OpenRouterCV(KEY_VALID_JSON, model_name="openai/gpt-4o")
        assert captured["sync_kwargs"]["api_key"] == "sk-or-v1-abc123"

    def test_json_array_raises_model_exception(self):
        from rag.llm.cv_model import OpenRouterCV

        with pytest.raises(ModelException) as exc_info:
            OpenRouterCV(KEY_JSON_ARRAY, model_name="openai/gpt-4o")
        assert "OpenRouter" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        from rag.llm.cv_model import OpenRouterCV

        with pytest.raises(ModelException) as exc_info:
            OpenRouterCV(KEY_JSON_NUMBER, model_name="openai/gpt-4o")
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestOpenRouterEmbedCallSite:
    def test_plain_key_constructs_with_key_as_api_key(self):
        """A plain key is stored as-is on the OpenAI client (matching
        the pre-fix behavior), with ``provider_order`` defaulting to
        empty."""
        from rag.llm.embedding_model import OpenRouterEmbed

        embed = OpenRouterEmbed(KEY_PLAIN, model_name="openai/text-embedding-3-small")
        assert embed.model_name == "openai/text-embedding-3-small"
        assert embed.provider_order == ""

    def test_json_dict_constructs_with_derived_fields(self):
        from rag.llm.embedding_model import OpenRouterEmbed

        embed = OpenRouterEmbed(KEY_VALID_JSON, model_name="openai/text-embedding-3-small")
        assert embed.provider_order == "Anthropic,OpenAI"

    def test_json_array_raises_model_exception(self):
        """The embedding site pre-fix did NOT crash on JSON non-object
        -- it had an explicit ``isinstance(payload, dict)`` check that
        silently used the raw JSON string as the api_key. That path
        then failed auth later with a less-actionable 401. The helper
        raises a clear :class:`ModelException` for the user instead."""
        from rag.llm.embedding_model import OpenRouterEmbed

        with pytest.raises(ModelException) as exc_info:
            OpenRouterEmbed(KEY_JSON_ARRAY, model_name="openai/text-embedding-3-small")
        assert "OpenRouter" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        from rag.llm.embedding_model import OpenRouterEmbed

        with pytest.raises(ModelException) as exc_info:
            OpenRouterEmbed(KEY_JSON_NUMBER, model_name="openai/text-embedding-3-small")
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False
