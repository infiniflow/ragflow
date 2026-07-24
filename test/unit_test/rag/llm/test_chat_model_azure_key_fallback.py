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

"""Regression tests for issue #17204 - ``LiteLLMBase.__init__`` in
``rag/llm/chat_model.py`` must NOT crash with
``json.decoder.JSONDecodeError`` when the user pastes a plain (non-JSON)
API key straight from the Azure Portal into the Azure-OpenAI provider.

The fix funnels every Azure-credential decode through a single helper,
``rag.llm.key_utils._resolve_azure_credentials``, which falls back to the
raw string when JSON parsing fails.

The tests cover two surfaces:

1. The shared helper handles the four input shapes (plain string, JSON
   dict with both fields, JSON dict with one missing field, invalid JSON,
   valid JSON of the wrong shape).
2. Constructing a ``LiteLLMBase`` instance with ``provider="Azure-OpenAI"``
   and a plain string API key does NOT raise ``JSONDecodeError`` (the
   original v0.26.4 bug) and preserves the raw key as ``self.api_key``
   with the default ``api_version``.
"""

import json

import pytest

from rag.llm.key_utils import _resolve_azure_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveAzureCredentials:
    def test_plain_string_key_falls_back_to_raw(self):
        """The regression: a plain API key from the Azure Portal must NOT raise."""
        api_key, api_version = _resolve_azure_credentials("sk-azure-secret-token")
        assert api_key == "sk-azure-secret-token"
        assert api_version == "2024-02-01"

    def test_json_dict_payload_extracts_api_key_and_version(self):
        api_key, api_version = _resolve_azure_credentials(
            json.dumps({"api_key": "from-json", "api_version": "2024-08-01"})
        )
        assert api_key == "from-json"
        assert api_version == "2024-08-01"

    def test_json_dict_missing_api_key_uses_empty_string(self):
        api_key, api_version = _resolve_azure_credentials(json.dumps({"api_version": "2024-08-01"}))
        assert api_key == ""
        assert api_version == "2024-08-01"

    def test_json_dict_missing_api_version_uses_default(self):
        api_key, api_version = _resolve_azure_credentials(json.dumps({"api_key": "only-key"}))
        assert api_key == "only-key"
        assert api_version == "2024-02-01"

    def test_json_array_payload_falls_back_to_raw(self):
        """A JSON array is valid JSON but is not the expected object shape;
        the helper must fall back to using the raw string as the API key."""
        # json.loads succeeds -> falls into the `not isinstance(key_obj, dict)` branch.
        api_key, api_version = _resolve_azure_credentials(json.dumps(["not", "a", "key"]))
        assert api_key == json.dumps(["not", "a", "key"])
        assert api_version == "2024-02-01"

    def test_invalid_json_falls_back_to_raw(self):
        api_key, api_version = _resolve_azure_credentials("not really json {")
        assert api_key == "not really json {"
        assert api_version == "2024-02-01"


# --------------------------------------------------------------------------- #
# 2. The chat-model regression: constructing an ``Azure_OpenAI`` LLM with a
#    plain (non-JSON) API key must NOT crash with ``json.JSONDecodeError``.
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestChatModelAzureOpenAIPlainKey:
    """Reproduce the v0.26.4 traceback from issue #17204.

    The conftest stub for ``rag.llm`` only enumerates a handful of providers
    in its ``SupportedLiteLLMProvider`` enum (Azure_OpenAI, OpenAI, ...),
    so the test below deliberately exercises only the ``Azure_OpenAI``
    branch of ``LiteLLMBase.__init__``. Other providers' branches are
    tested elsewhere; our regression target is the previously unguarded
    ``json.loads(key).get("api_key", "")`` call.
    """

    def _make_minimal_chat_instance(self, key, provider):
        """Build a ``LiteLLMBase`` instance without touching the
        ``OpenAI(...)`` network client.

        ``LiteLLMBase`` is the class registered in ``rag.llm.ChatModel``
        for the ``"Azure-OpenAI"`` provider (see ``rag.llm.__init__``: it
        enumerates ``_FACTORY_NAME`` entries on the class itself and
        registers the class under each name). Its ``__init__`` is concrete
        (no abstract methods) so it is instantiable directly — that is
        exactly the constructor that previously crashed with
        ``json.JSONDecodeError`` on a plain-string key.

        The conftest stub for ``rag.llm`` only enumerates a handful of
        providers in its ``SupportedLiteLLMProvider`` enum; we extend it
        with ``OpenRouter``/``MiniMax`` here so the corresponding branches
        inside ``LiteLLMBase.__init__`` resolve without ``AttributeError``.
        """
        from unittest.mock import patch

        from rag.llm import chat_model

        # Augment the stub enum with the providers referenced by chat_model.
        chat_model.SupportedLiteLLMProvider.OpenRouter = "OpenRouter"
        chat_model.SupportedLiteLLMProvider.MiniMax = "MiniMax"

        with patch("rag.llm.chat_model.OpenAI"), patch("rag.llm.chat_model.AsyncOpenAI"):
            return chat_model.LiteLLMBase(
                key,
                "gpt-4o-mini",
                base_url="https://example.invalid/v1",
                provider=provider,
            )

    def test_plain_string_key_does_not_raise_json_decode_error(self):
        # Was: ``json.decoder.JSONDecodeError: Extra data: line 1 column 2 (char 1)``
        chat = self._make_minimal_chat_instance("azure-plain-api-key-string", "Azure-OpenAI")
        assert chat.api_key == "azure-plain-api-key-string"
        assert chat.api_version == "2024-02-01"

    def test_json_object_key_is_decoded(self):
        chat = self._make_minimal_chat_instance(
            json.dumps({"api_key": "from-json", "api_version": "2024-08-01"}),
            "Azure-OpenAI",
        )
        assert chat.api_key == "from-json"
        assert chat.api_version == "2024-08-01"
