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

"""Regression tests for the Azure-OpenAI key JSON-decode gap across
4 sites (#17675).

Pre-fix, the Azure-OpenAI provider in :mod:`rag.llm` had **4 unfixed
call sites** with **3 different patterns** for parsing the API key:

- **chat** (chat_model.py:1649-1651) -- bare ``json.loads(key).get(...)``
  with no try/except, crashes with ``JSONDecodeError`` on plain API key
  and ``AttributeError`` on JSON non-object.
- **vision / CV** (cv_model.py:380) -- local helper with silent
  fallback for non-object JSON, silently uses the raw key.
- **embed** (embedding_model.py:325) -- identical local helper (a
  duplicate copy of the CV one).
- **seq2txt** (sequence2txt_model.py:386) -- uses raw ``key`` directly
  with NO JSON parsing at all.

The fix unifies all 4 sites through a single
``_resolve_azure_credentials(key)`` helper in :mod:`rag.llm.key_utils`
that follows the same pattern as the other 5 helpers in the
JSON-decode family (Bedrock, BaiduYiyan, VolcEngine, OpenRouter,
GoogleCV):

1. Accepts a dict (returned verbatim) or a JSON-string-encoded dict.
2. On non-JSON input, raises a clear :class:`ModelException` with
   ``retryable=False`` naming the required fields and pointing at
   ``conf/models/azure.json``.
3. On a JSON top-level type that is not a dict (list, string, number,
   bool, null), raises the same :class:`ModelException`.

Returns ``(api_key, api_version)`` where ``api_version`` defaults to
``"2024-02-01"`` and ``api_key`` defaults to ``""`` if missing.

The local helper duplicates in ``cv_model.py`` and
``embedding_model.py`` are removed in this PR.

This test file covers:

- ``_resolve_azure_credentials`` directly (12 helper tests: 4
  happy-path + 6 non-object raises + 2 non-string input raises).
- The 4 call sites (AzureGptV4, AzureEmbed, AzureSeq2txt) end-to-end
  (one happy-path + one bug-case each). The LiteLLMBase Azure-OpenAI
  branch in ``chat_model.py`` is tested via the helper directly because
  ``LiteLLMBase`` pulls in heavy deps and is hard to construct in a
  unit test (same approach as the prior cycle's tests).
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_azure_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveAzureCredentials:
    def test_python_dict_passes_through(self):
        """A pre-parsed dict (e.g. a future caller that already has the
        key as a dict) returns the ``(api_key, api_version)`` shape."""
        out = _resolve_azure_credentials({"api_key": "sk-direct", "api_version": "2025-01-01"})
        assert out == ("sk-direct", "2025-01-01")

    def test_python_dict_missing_api_key(self):
        """A dict without ``api_key`` returns ``("", default_version)``."""
        out = _resolve_azure_credentials({"api_version": "2025-01-01"})
        assert out == ("", "2025-01-01")

    def test_python_dict_missing_api_version(self):
        """A dict without ``api_version`` returns the default version."""
        out = _resolve_azure_credentials({"api_key": "sk-direct"})
        assert out == ("sk-direct", "2024-02-01")

    def test_json_dict_with_all_fields(self):
        """The happy path: a valid JSON dict returns the parsed fields."""
        raw = {"api_key": "sk-123", "api_version": "2025-01-01"}
        out = _resolve_azure_credentials(json.dumps(raw))
        assert out == ("sk-123", "2025-01-01")

    def test_json_array_raises_model_exception(self):
        """The bug case: a valid JSON array parsed, then ``.get(...)``
        would raise ``AttributeError: 'list' object has no attribute
        'get'`` from inside ``rag.llm``. The helper raises a clear
        :class:`ModelException`."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "Azure-OpenAI" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials(json.dumps("just a string"))
        assert "object" in str(exc_info.value)
        assert "str" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials(json.dumps(42))
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials(json.dumps(3.14))
        assert "object" in str(exc_info.value)
        assert "float" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials("null")
        assert "object" in str(exc_info.value)
        assert "NoneType" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        """The most common user mistake: pasting a plain API key from
        the Azure Portal (e.g. ``"abc...123"``). Pre-fix this crashed
        with ``json.decoder.JSONDecodeError: Expecting value: line 1
        column 1 (char 0)``. The fix raises a clear
        :class:`ModelException`."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials("abc-12345-very-long-azure-api-key")
        msg = str(exc_info.value)
        assert "Azure-OpenAI" in msg
        assert "api_key" in msg
        assert "JSONDecodeError" not in msg
        assert "Expecting value" not in msg
        assert exc_info.value.retryable is False

    def test_empty_string_raises_model_exception(self):
        with pytest.raises(ModelException):
            _resolve_azure_credentials("")

    def test_invalid_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_azure_credentials("not really json {")
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / tuple / etc. passed directly (not as a string
        or dict) must also raise. Only ``str`` and ``dict`` are valid
        inputs to the helper."""
        with pytest.raises(ModelException):
            _resolve_azure_credentials(42)
        with pytest.raises(ModelException):
            _resolve_azure_credentials(None)
        with pytest.raises(ModelException):
            _resolve_azure_credentials(["a", "b"])


# --------------------------------------------------------------------------- #
# 2. End-to-end: the 3 constructible call sites
# --------------------------------------------------------------------------- #


def _install_azure_openai_stub(monkeypatch, where):
    """Patch ``openai.lib.azure.AzureOpenAI`` and
    ``openai.lib.azure.AsyncAzureOpenAI`` at the call site so we can
    construct the model classes without a real Azure round-trip.

    Same pattern as the prior cycle's ``_patch_openai`` helper, but
    scoped to the Azure import path.
    """
    captured = {"sync_kwargs": None, "async_kwargs": None}

    def sync_factory(*args, **kwargs):
        captured["sync_kwargs"] = kwargs
        return MagicMock()

    def async_factory(*args, **kwargs):
        captured["async_kwargs"] = kwargs
        return MagicMock()

    sync_patcher = patch(f"{where}.AzureOpenAI", side_effect=sync_factory)
    async_patcher = patch(f"{where}.AsyncAzureOpenAI", side_effect=async_factory)
    return sync_patcher, async_patcher, captured


@pytest.mark.p0
class TestAzureGptV4CallSite:
    def test_json_dict_constructs_with_parsed_fields(self, monkeypatch):
        """Happy path: a valid JSON dict passes through the helper and
        ``AzureGptV4.__init__`` completes (with the Azure SDK stubbed).
        The stubbed AzureOpenAI is called with the parsed api_key +
        api_version."""
        from rag.llm.cv_model import AzureGptV4

        sync, async_, captured = _install_azure_openai_stub(monkeypatch, "rag.llm.cv_model")
        with sync, async_:
            AzureGptV4(
                json.dumps({"api_key": "sk-123", "api_version": "2025-01-01"}),
                model_name="gpt-4o",
                base_url="https://example.azure.com/openai/deployments/gpt-4o",
            )
        # The Azure SDK was called with the parsed api_key + api_version.
        called_kwargs = captured["sync_kwargs"]
        assert called_kwargs["api_key"] == "sk-123"
        assert called_kwargs["api_version"] == "2025-01-01"
        # base_url was normalized through ensure_v1.
        assert "azure_endpoint" in called_kwargs

    def test_json_array_raises_model_exception(self, monkeypatch):
        """The bug case: a JSON array used to crash with
        ``AttributeError: 'list' object has no attribute 'get'`` deep
        inside ``rag.llm``. The fix surfaces a clear
        :class:`ModelException`."""
        from rag.llm.cv_model import AzureGptV4

        sync, async_, _ = _install_azure_openai_stub(monkeypatch, "rag.llm.cv_model")
        with sync, async_:
            with pytest.raises(ModelException) as exc_info:
                AzureGptV4(
                    json.dumps(["not", "a", "key"]),
                    model_name="gpt-4o",
                    base_url="https://example.azure.com/openai/deployments/gpt-4o",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self, monkeypatch):
        """The most common user mistake: pasting a plain Azure API key
        from the Portal. Pre-fix this crashed with
        ``json.decoder.JSONDecodeError: Expecting value`` from inside
        ``rag.llm``. The fix raises a clear :class:`ModelException`."""
        from rag.llm.cv_model import AzureGptV4

        sync, async_, _ = _install_azure_openai_stub(monkeypatch, "rag.llm.cv_model")
        with sync, async_:
            with pytest.raises(ModelException) as exc_info:
                AzureGptV4(
                    "abc-12345-very-long-azure-api-key",
                    model_name="gpt-4o",
                    base_url="https://example.azure.com/openai/deployments/gpt-4o",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "api_key" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestAzureEmbedCallSite:
    def test_json_dict_constructs_with_parsed_fields(self, monkeypatch):
        """Happy path: a valid JSON dict passes through the helper and
        ``AzureEmbed.__init__`` completes (with the Azure SDK stubbed).
        """
        from rag.llm.embedding_model import AzureEmbed

        # AzureEmbed is a subclass of OpenAIEmbed and uses a lazy
        # ``from openai.lib.azure import AzureOpenAI`` inside __init__.
        # The ``openai`` module itself is the import location.
        captured = {"sync_kwargs": None}

        def azure_factory(*args, **kwargs):
            captured["sync_kwargs"] = kwargs
            return MagicMock()

        with patch("openai.lib.azure.AzureOpenAI", side_effect=azure_factory):
            AzureEmbed(
                json.dumps({"api_key": "sk-123", "api_version": "2025-01-01"}),
                model_name="text-embedding-3-small",
                base_url="https://example.azure.com/openai/deployments/text-embedding-3-small",
            )
        called_kwargs = captured["sync_kwargs"]
        assert called_kwargs["api_key"] == "sk-123"
        assert called_kwargs["api_version"] == "2025-01-01"

    def test_json_array_raises_model_exception(self, monkeypatch):
        from rag.llm.embedding_model import AzureEmbed

        with patch("openai.lib.azure.AzureOpenAI", MagicMock()):
            with pytest.raises(ModelException) as exc_info:
                AzureEmbed(
                    json.dumps(["not", "a", "key"]),
                    model_name="text-embedding-3-small",
                    base_url="https://example.azure.com/openai/deployments/text-embedding-3-small",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self, monkeypatch):
        from rag.llm.embedding_model import AzureEmbed

        with patch("openai.lib.azure.AzureOpenAI", MagicMock()):
            with pytest.raises(ModelException) as exc_info:
                AzureEmbed(
                    "abc-12345-very-long-azure-api-key",
                    model_name="text-embedding-3-small",
                    base_url="https://example.azure.com/openai/deployments/text-embedding-3-small",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "api_key" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


@pytest.mark.p0
class TestAzureSeq2txtCallSite:
    def test_json_dict_constructs_with_parsed_fields(self, monkeypatch):
        """Happy path: a valid JSON dict passes through the helper and
        ``AzureSeq2txt.__init__`` completes (with the Azure SDK
        stubbed). Pre-fix, the raw ``key`` was passed straight to the
        AzureOpenAI client, so a JSON string was used as the api_key
        and the call silently failed with a 401.

        ``AzureSeq2txt`` only uses the sync ``AzureOpenAI`` (no
        ``AsyncAzureOpenAI``), so the patch is sync-only here.
        """
        from rag.llm.sequence2txt_model import AzureSeq2txt

        captured = {"sync_kwargs": None}

        def sync_factory(*args, **kwargs):
            captured["sync_kwargs"] = kwargs
            return MagicMock()

        sync_patcher = patch("rag.llm.sequence2txt_model.AzureOpenAI", side_effect=sync_factory)
        with sync_patcher:
            AzureSeq2txt(
                json.dumps({"api_key": "sk-123", "api_version": "2025-01-01"}),
                model_name="whisper",
                base_url="https://example.azure.com/openai/deployments/whisper",
            )
        called_kwargs = captured["sync_kwargs"]
        assert called_kwargs["api_key"] == "sk-123"
        assert called_kwargs["api_version"] == "2025-01-01"

    def test_json_array_raises_model_exception(self, monkeypatch):
        from rag.llm.sequence2txt_model import AzureSeq2txt

        with patch("rag.llm.sequence2txt_model.AzureOpenAI", MagicMock()):
            with pytest.raises(ModelException) as exc_info:
                AzureSeq2txt(
                    json.dumps(["not", "a", "key"]),
                    model_name="whisper",
                    base_url="https://example.azure.com/openai/deployments/whisper",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self, monkeypatch):
        from rag.llm.sequence2txt_model import AzureSeq2txt

        with patch("rag.llm.sequence2txt_model.AzureOpenAI", MagicMock()):
            with pytest.raises(ModelException) as exc_info:
                AzureSeq2txt(
                    "abc-12345-very-long-azure-api-key",
                    model_name="whisper",
                    base_url="https://example.azure.com/openai/deployments/whisper",
                )
        assert "Azure-OpenAI" in str(exc_info.value)
        assert "api_key" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False
