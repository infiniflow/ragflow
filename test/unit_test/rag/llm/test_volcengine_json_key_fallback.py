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

"""Regression tests for the VolcEngine/Ark ``key`` JSON-decode fallback (#17456).

Pre-fix, the three VolcEngine call sites in :mod:`rag.llm` did::

    try:
        ark_api_key = json.loads(key).get("ark_api_key", "")
        model_name = json.loads(key).get("ep_id", "") + json.loads(key).get("endpoint_id", "")
        super().__init__(ark_api_key, model_name, base_url, **kwargs)
    except JSONDecodeError:
        super().__init__(key, model_name, base_url, **kwargs)

The ``except JSONDecodeError`` only catches the *parse* failure. A user
pasting a JSON string that is NOT an object -- e.g. ``"[1,2,3]"``,
``"42"``, ``'"hello"'``, ``"true"``, ``"null"`` -- parses fine and then
crashes on the ``.get(...)`` call with ``AttributeError: 'list' object
has no attribute 'get'`` (or ``'int'``, ``'str'``, ``'NoneType'``,
``'bool'``) from inside :mod:`rag.llm` internals, with no indication of
what the user did wrong.

The fix funnels every VolcEngine key through
``_resolve_volcengine_credentials`` in :mod:`rag.llm.key_utils`. The
helper:

1. Accepts a plain (non-JSON) string and returns
   ``{"ark_api_key": key, "model_name": None}`` -- matching the
   pre-fix ``except JSONDecodeError`` fallback semantics.
2. Accepts a dict (returned with ``ark_api_key`` and an optionally
   derived ``model_name`` from ``ep_id`` + ``endpoint_id``).
3. On a JSON top-level type that is not a dict (list, string, number,
   bool, null), raises a clear :class:`ModelException` naming the
   required object shape and pointing at ``conf/models/volcengine.json``.

The three call sites then continue with the helper output, never calling
``.get`` on a non-dict.
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_volcengine_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveVolcengineCredentials:
    def test_plain_string_key_passes_through(self):
        """The most common case: a user pastes a plain VolcEngine API key
        (e.g. ``"abc123-very-long-key"``). Pre-fix, this was caught by
        ``except JSONDecodeError`` and used as-is. The helper preserves
        that fallback and additionally tags ``model_name`` as ``None`` so
        the caller keeps the ``model_name`` it was passed in
        ``__init__``."""
        out = _resolve_volcengine_credentials("abc123-very-long-key")
        assert out == {"ark_api_key": "abc123-very-long-key", "model_name": None}

    def test_empty_string_falls_through_as_plain_key(self):
        """An empty string is a non-JSON string. The helper treats it the
        same as a plain key (preserving the pre-fix ``except
        JSONDecodeError`` branch)."""
        out = _resolve_volcengine_credentials("")
        assert out == {"ark_api_key": "", "model_name": None}

    def test_python_dict_passes_through(self):
        """A pre-parsed dict (e.g. a future caller that already has the
        key as a dict) returns the dict shape with ``ark_api_key`` and an
        optional ``model_name`` derived from ``ep_id`` + ``endpoint_id``."""
        raw = {
            "ark_api_key": "abc123",
            "ep_id": "ep-abc",
            "endpoint_id": "-250115",
        }
        out = _resolve_volcengine_credentials(raw)
        assert out == {"ark_api_key": "abc123", "model_name": "ep-abc-250115"}

    def test_json_dict_with_all_fields(self):
        """The happy path: a valid JSON dict returns the parsed fields so
        the model classes can use them unchanged."""
        raw = {"ark_api_key": "abc123", "ep_id": "ep-abc", "endpoint_id": "-250115"}
        out = _resolve_volcengine_credentials(json.dumps(raw))
        assert out == {"ark_api_key": "abc123", "model_name": "ep-abc-250115"}

    def test_json_dict_missing_ark_api_key(self):
        """A dict without ``ark_api_key`` is still a valid dict; the
        helper returns an empty ``ark_api_key`` rather than raising. The
        downstream model class is responsible for the
        ``ark_api_key``-missing check (none currently)."""
        out = _resolve_volcengine_credentials(json.dumps({"ep_id": "ep-abc"}))
        assert out == {"ark_api_key": "", "model_name": "ep-abc"}

    def test_json_dict_missing_both_ep_id_and_endpoint_id(self):
        """Without ``ep_id`` and ``endpoint_id`` the helper cannot derive
        a model_name; it returns ``None`` so the caller keeps the
        ``model_name`` parameter it was passed in ``__init__``."""
        out = _resolve_volcengine_credentials(json.dumps({"ark_api_key": "abc123"}))
        assert out == {"ark_api_key": "abc123", "model_name": None}

    def test_json_dict_with_only_ep_id(self):
        out = _resolve_volcengine_credentials(json.dumps({"ark_api_key": "abc123", "ep_id": "ep-abc"}))
        assert out == {"ark_api_key": "abc123", "model_name": "ep-abc"}

    def test_json_dict_with_only_endpoint_id(self):
        out = _resolve_volcengine_credentials(json.dumps({"ark_api_key": "abc123", "endpoint_id": "-250115"}))
        assert out == {"ark_api_key": "abc123", "model_name": "-250115"}

    def test_json_array_raises_model_exception(self):
        """A valid JSON array is the bug case: pre-fix, ``.get(...)`` on
        the parsed list raised ``AttributeError: 'list' object has no
        attribute 'get'``. The helper must raise a clear
        :class:`ModelException` naming the actual type and the required
        object shape."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "VolcEngine" in msg
        assert "object" in msg
        assert "list" in msg
        assert "conf/models/volcengine.json" in msg
        # Must NOT be a raw AttributeError leaking out.
        assert "AttributeError" not in msg
        assert "'list' object" not in msg
        # Must not be retryable -- the operator has to fix their key.
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials(json.dumps("just a string"))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "str" in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials(json.dumps(42))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "int" in msg
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials(json.dumps(3.14))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "float" in msg
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        """``json.loads("null")`` returns Python ``None``. The helper must
        treat that as a non-object (the pre-fix code crashed with
        ``AttributeError: 'NoneType' object has no attribute 'get'``)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials("null")
        msg = str(exc_info.value)
        assert "object" in msg
        assert "NoneType" in msg
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_volcengine_credentials("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / ``None`` / etc. passed directly (not as a string
        or a dict) must also raise. Only ``str`` and ``dict`` are valid
        inputs to the helper."""
        with pytest.raises(ModelException):
            _resolve_volcengine_credentials(42)
        with pytest.raises(ModelException):
            _resolve_volcengine_credentials(None)
        with pytest.raises(ModelException):
            _resolve_volcengine_credentials(["a", "b"])

    def test_malformed_json_falls_through_as_plain_key(self):
        """``json.loads("not really json")`` raises ``JSONDecodeError``.
        The helper treats that the same as a plain string key (matching
        the pre-fix ``except JSONDecodeError`` fallback semantics -- a
        user pasting a raw token into the key field)."""
        out = _resolve_volcengine_credentials("not really json {")
        assert out == {"ark_api_key": "not really json {", "model_name": None}


# --------------------------------------------------------------------------- #
# 2. End-to-end: the three call sites use the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_PLAIN = "abc123-very-long-key"
KEY_VALID_JSON = json.dumps({"ark_api_key": "abc123", "ep_id": "ep-abc", "endpoint_id": "-250115"})
KEY_JSON_ARRAY = json.dumps(["not", "a", "key"])
KEY_JSON_NUMBER = json.dumps(42)


def _patch_openai(*, where):
    """Patch ``openai.OpenAI`` and ``openai.AsyncOpenAI`` at the call
    site ``where`` (a fully-qualified module path -- e.g.
    ``"rag.llm.chat_model"``) so we can construct ``VolcEngineChat`` /
    ``VolcEngineCV`` without a real Ark/Voyage round-trip.

    Returns ``(sync_patcher, async_patcher, captured)`` where
    ``captured`` is a dict of ``{"sync_kwargs": {...}, "async_kwargs":
    {...}}`` populated on construction. We use ``side_effect`` so the
    call kwargs (in particular ``api_key``) can be asserted on directly.

    Patching at the import location is required because both
    ``chat_model.py`` and ``cv_model.py`` do ``from openai import
    AsyncOpenAI, OpenAI`` at the top of the module; patching
    ``openai.OpenAI`` does not affect the already-bound names in those
    modules.
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
class TestVolcEngineChatCallSite:
    def test_plain_key_constructs_with_key_as_api_key(self):
        """A plain key should be passed through to ``Base.__init__`` as
        the api_key, with the model_name parameter preserved."""
        from rag.llm.chat_model import VolcEngineChat

        sync, async_, captured = _patch_openai(where="rag.llm.chat_model")
        with sync, async_:
            chat = VolcEngineChat(KEY_PLAIN, model_name="doubao-1.5-pro")
        assert chat.model_name == "doubao-1.5-pro"
        # Base.__init__ stored the api_key on the OpenAI client.
        assert captured["sync_kwargs"]["api_key"] == KEY_PLAIN

    def test_json_dict_constructs_with_derived_model_name(self):
        """A JSON dict with both ep_id and endpoint_id should override
        the passed model_name with the derived one."""
        from rag.llm.chat_model import VolcEngineChat

        sync, async_, captured = _patch_openai(where="rag.llm.chat_model")
        with sync, async_:
            chat = VolcEngineChat(KEY_VALID_JSON, model_name="ignored")
        assert chat.model_name == "ep-abc-250115"
        assert captured["sync_kwargs"]["api_key"] == "abc123"

    def test_json_array_raises_model_exception(self):
        """The bug case: a JSON array used to crash with
        ``AttributeError: 'list' object has no attribute 'get'`` deep
        inside ``rag.llm``. The fix surfaces a clear
        :class:`ModelException`."""
        from rag.llm.chat_model import VolcEngineChat

        with pytest.raises(ModelException) as exc_info:
            VolcEngineChat(KEY_JSON_ARRAY, model_name="doubao-1.5-pro")
        msg = str(exc_info.value)
        assert "VolcEngine" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        from rag.llm.chat_model import VolcEngineChat

        with pytest.raises(ModelException) as exc_info:
            VolcEngineChat(KEY_JSON_NUMBER, model_name="doubao-1.5-pro")
        assert "VolcEngine" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestVolcEngineCVCallSite:
    def test_plain_key_constructs_with_key_as_api_key(self):
        """A plain key flows through to the OpenAI client as api_key,
        with the passed model_name preserved."""
        from rag.llm.cv_model import VolcEngineCV

        sync, async_, captured = _patch_openai(where="rag.llm.cv_model")
        with sync, async_:
            cv = VolcEngineCV(KEY_PLAIN, model_name="doubao-1.5-vision-pro")
        assert cv.model_name == "doubao-1.5-vision-pro"
        assert captured["sync_kwargs"]["api_key"] == KEY_PLAIN

    def test_json_dict_constructs_with_derived_model_name(self):
        from rag.llm.cv_model import VolcEngineCV

        sync, async_, captured = _patch_openai(where="rag.llm.cv_model")
        with sync, async_:
            cv = VolcEngineCV(KEY_VALID_JSON, model_name="ignored")
        assert cv.model_name == "ep-abc-250115"
        assert captured["sync_kwargs"]["api_key"] == "abc123"

    def test_json_array_raises_model_exception(self):
        from rag.llm.cv_model import VolcEngineCV

        with pytest.raises(ModelException) as exc_info:
            VolcEngineCV(KEY_JSON_ARRAY, model_name="doubao-1.5-vision-pro")
        assert "VolcEngine" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        from rag.llm.cv_model import VolcEngineCV

        with pytest.raises(ModelException) as exc_info:
            VolcEngineCV(KEY_JSON_NUMBER, model_name="doubao-1.5-vision-pro")
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestVolcEngineEmbedCallSite:
    def test_plain_key_constructs_with_key_as_ark_api_key(self):
        """A plain key is stored as-is on ``self.ark_api_key`` (matching
        the pre-fix ``except JSONDecodeError`` branch)."""
        from rag.llm.embedding_model import VolcEngineEmbed

        embed = VolcEngineEmbed(KEY_PLAIN, model_name="doubao-embedding")
        assert embed.ark_api_key == KEY_PLAIN
        assert embed.model_name == "doubao-embedding"

    def test_json_dict_constructs_with_ark_api_key(self):
        from rag.llm.embedding_model import VolcEngineEmbed

        embed = VolcEngineEmbed(KEY_VALID_JSON, model_name="doubao-embedding")
        assert embed.ark_api_key == "abc123"
        # The embedding class only consumes ark_api_key, NOT model_name.
        assert embed.model_name == "doubao-embedding"

    def test_json_array_raises_model_exception(self):
        from rag.llm.embedding_model import VolcEngineEmbed

        with pytest.raises(ModelException) as exc_info:
            VolcEngineEmbed(KEY_JSON_ARRAY, model_name="doubao-embedding")
        assert "VolcEngine" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        from rag.llm.embedding_model import VolcEngineEmbed

        with pytest.raises(ModelException) as exc_info:
            VolcEngineEmbed(KEY_JSON_NUMBER, model_name="doubao-embedding")
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False
