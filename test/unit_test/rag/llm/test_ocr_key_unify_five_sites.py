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

"""Regression tests for the OCR-provider key JSON-decode gap across 5
sites (#17681).

The 5 OCR provider classes in :mod:`rag.llm.ocr_model`
(``MinerUOcrModel``, ``PaddleOCROcrModel``, ``OpenDataLoaderOcrModel``,
``SoMarkOcrModel``, ``MistralOcrModel``) all parsed their ``key`` with
the same ``try: json.loads(key) except Exception: raw_config = {}``
silent-fallback pattern. When a user pasted a plain key (e.g. a
Mistral API key like ``"sk-abc123"``), the JSON parse failed, the
exception was silently swallowed, and the provider fell back to host
env vars -- the user's pasted key was dropped on the floor.

This test file covers three layers:

  1. The shared helper ``_resolve_ocr_credentials`` in
     :mod:`rag.llm.key_utils` (~16 tests): Python dict passthrough
     with/without fields, JSON dict happy path, JSON array / string /
     number / float / null / bool all raise ``ModelException``, plain
     string (most common user mistake) raises ``ModelException``,
     empty string returns ``{}``, invalid JSON raises
     ``ModelException``, non-string non-dict (int, None, list) raises
     ``ModelException``.
  2. The local ``_is_raw_secret_key`` helper in :mod:`rag.llm.ocr_model`
     (~5 tests): detects plain secret strings (Mistral / SoMark raw
     key path) vs valid JSON (the helper's strict path).
  3. End-to-end on the 5 call sites (3 tests per site = 15 total):
     JSON dict constructs with parsed fields, JSON non-object raises,
     and -- for the 2 sites that support it (SoMark, Mistral) -- plain
     key uses the raw-key fallback path.

The 3 sites that do NOT support a raw key (MinerU, PaddleOCR,
OpenDataLoader) raise ``ModelException`` for a plain key, which is the
bug fix. The 2 sites that DO support a raw key (SoMark, Mistral) keep
the existing behavior via the ``key_as_secret`` fallback.
"""

import json

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_ocr_credentials
from rag.llm.ocr_model import (
    _is_raw_secret_key,
    MinerUOcrModel,
    MistralOcrModel,
    OpenDataLoaderOcrModel,
    PaddleOCROcrModel,
    SoMarkOcrModel,
)


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #


class TestResolveOcrCredentials:
    """``_resolve_ocr_credentials`` parses an OCR-provider key into a
    config ``dict``. Falsy / dict / JSON-string-encoded dict inputs
    succeed; everything else raises ``ModelException`` with a clear
    message pointing at the expected schema.
    """

    def test_falsy_key_returns_empty_dict(self):
        assert _resolve_ocr_credentials("") == {}
        assert _resolve_ocr_credentials(None) == {}

    def test_python_dict_passes_through(self):
        out = _resolve_ocr_credentials({"MINERU_API_KEY": "x"})
        assert out == {"MINERU_API_KEY": "x"}

    def test_json_dict_with_all_fields(self):
        raw = {"MINERU_API_KEY": "x", "MINERU_APISERVER": "https://example"}
        out = _resolve_ocr_credentials(json.dumps(raw))
        assert out == raw

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "OCR provider key" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials(json.dumps("just a string"))
        assert "object" in str(exc_info.value)
        assert "str" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials(json.dumps(42))
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials(json.dumps(3.14))
        assert "object" in str(exc_info.value)
        assert "float" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials("null")
        assert "object" in str(exc_info.value)
        assert "NoneType" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        """The most common user mistake: pasting a plain API key from
        the provider's portal. Pre-fix this was silently dropped via
        ``except Exception: raw_config = {}`` and the provider fell
        back to host env vars. The fix raises a clear
        ``ModelException``.
        """
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials("sk-abc12345-very-long-api-key")
        msg = str(exc_info.value)
        assert "OCR provider key" in msg
        assert "JSON object" in msg
        assert "JSONDecodeError" not in msg
        assert "Expecting value" not in msg
        assert exc_info.value.retryable is False

    def test_invalid_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_ocr_credentials("not really json {")
        assert "OCR provider key" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / tuple / None (non-empty) / etc. passed directly
        (not as a string or dict) must also raise. Only ``str`` and
        ``dict`` are valid inputs to the helper.
        """
        for bad in (42, ["a", "b"], (1, 2), 3.14, True):
            with pytest.raises(ModelException):
                _resolve_ocr_credentials(bad)

    def test_empty_dict_returns_empty_dict(self):
        """An empty dict is a valid input (no config) and returns an
        empty dict -- the caller is responsible for falling back to
        host env vars in that case.
        """
        assert _resolve_ocr_credentials({}) == {}

    def test_nested_ui_config_passes_through(self):
        """The nested ``{"api_key": {...}}`` shape from the UI is
        returned verbatim; the per-class wrapper is responsible for
        unwrapping it.
        """
        nested = {"api_key": {"mineru_apiserver": "https://example"}}
        assert _resolve_ocr_credentials(nested) == nested
        assert _resolve_ocr_credentials(json.dumps(nested)) == nested

    def test_flat_env_auto_provision_config_passes_through(self):
        """The flat ``{"PROVIDER_*": "..."}`` shape from env-var
        auto-provisioning is returned verbatim.
        """
        flat = {"MINERU_API_KEY": "x", "MINERU_APISERVER": "https://example"}
        assert _resolve_ocr_credentials(flat) == flat
        assert _resolve_ocr_credentials(json.dumps(flat)) == flat

    def test_helper_lists_in_its_module_all(self):
        """The helper must be exported from ``rag.llm.key_utils``."""
        from rag.llm import key_utils

        assert "_resolve_ocr_credentials" in key_utils.__all__


# --------------------------------------------------------------------------- #
# 2. The local _is_raw_secret_key helper
# --------------------------------------------------------------------------- #


class TestIsRawSecretKey:
    """``_is_raw_secret_key`` is used by ``SoMarkOcrModel`` and
    ``MistralOcrModel`` to detect a non-JSON plain string and route
    it to the raw-key fallback (``key_as_secret``) instead of the
    helper's strict JSON validation.
    """

    def test_plain_string_is_raw(self):
        assert _is_raw_secret_key("sk-abc123") is True

    def test_invalid_json_string_is_raw(self):
        assert _is_raw_secret_key("not really json {") is True

    def test_json_object_is_not_raw(self):
        assert _is_raw_secret_key(json.dumps({"a": 1})) is False

    def test_json_array_is_not_raw(self):
        # This is the case the old `not startswith("{")` heuristic
        # misclassified: ``[`` doesn't start with ``{`` but is valid
        # JSON. The new check uses ``json.loads`` validation.
        assert _is_raw_secret_key(json.dumps(["a", "b"])) is False

    def test_json_string_is_not_raw(self):
        assert _is_raw_secret_key(json.dumps("just a string")) is False

    def test_dict_is_not_raw(self):
        assert _is_raw_secret_key({"a": 1}) is False

    def test_empty_string_is_not_raw(self):
        assert _is_raw_secret_key("") is False
        assert _is_raw_secret_key(None) is False

    def test_non_string_non_dict_is_not_raw(self):
        assert _is_raw_secret_key(42) is False
        assert _is_raw_secret_key([1, 2]) is False


# --------------------------------------------------------------------------- #
# 3. End-to-end: 3 sites that do NOT support raw key
# --------------------------------------------------------------------------- #


class TestMinerUOcrModelCallSite:
    """``MinerUOcrModel`` requires a JSON config (no raw-key fallback).
    Pre-fix, a plain key was silently dropped via
    ``except Exception: raw_config = {}``. The fix raises a clear
    ``ModelException`` for a plain key.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = MinerUOcrModel(
            key=json.dumps({"mineru_apiserver": "https://example", "MINERU_APISERVER": "https://example-env"}),
            model_name="MinerU-model",
        )
        # The MinerU class maps mineru_api from the JSON's
        # ``mineru_apiserver`` key (or ``MINERU_APISERVER`` as the
        # env-auto-provision fallback). The helper was used to parse
        # the dict, so the values come from the config we passed,
        # not from host env vars.
        assert mdl.mineru_api == "https://example"
        assert mdl.mineru_backend == "pipeline"  # default

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            MinerUOcrModel(key=json.dumps(["not", "a", "key"]), model_name="MinerU-model")
        assert "OCR provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        """The bug-fix case: a plain API key used to be silently
        dropped. Now it raises a clear ``ModelException``.
        """
        with pytest.raises(ModelException) as exc_info:
            MinerUOcrModel(
                key="sk-abc12345-very-long-mineru-api-key",
                model_name="MinerU-model",
            )
        assert "OCR provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


class TestPaddleOCROcrModelCallSite:
    """``PaddleOCROcrModel`` requires a JSON config (no raw-key fallback).
    Pre-fix, a plain key was silently dropped. The fix raises.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = PaddleOCROcrModel(
            key=json.dumps({"PADDLEOCR_ACCESS_TOKEN": "pat", "PADDLEOCR_BASE_URL": "https://example"}),
            model_name="PaddleOCR-model",
        )
        assert mdl.access_token == "pat"

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            PaddleOCROcrModel(key=json.dumps(["not", "a", "key"]), model_name="PaddleOCR-model")
        assert "OCR provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            PaddleOCROcrModel(
                key="sk-abc12345-very-long-paddleocr-token",
                model_name="PaddleOCR-model",
            )
        assert "OCR provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


class TestOpenDataLoaderOcrModelCallSite:
    """``OpenDataLoaderOcrModel`` requires a JSON config (no raw-key
    fallback). Pre-fix, a plain key was silently dropped. The fix
    raises.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = OpenDataLoaderOcrModel(
            key=json.dumps({"OPENDATALOADER_API_KEY": "odk", "OPENDATALOADER_APISERVER": "https://example"}),
            model_name="OpenDataLoader-model",
        )
        assert mdl.api_key == "odk"
        assert mdl.api_url == "https://example"

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            OpenDataLoaderOcrModel(
                key=json.dumps(["not", "a", "key"]),
                model_name="OpenDataLoader-model",
            )
        assert "OCR provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            OpenDataLoaderOcrModel(
                key="sk-abc12345-very-long-opendataloader-key",
                model_name="OpenDataLoader-model",
            )
        assert "OCR provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


# --------------------------------------------------------------------------- #
# 4. End-to-end: 2 sites that DO support raw key
# --------------------------------------------------------------------------- #


class TestSoMarkOcrModelCallSite:
    """``SoMarkOcrModel`` supports a raw secret key as a documented
    fallback for the api_key (the most common user input). Pre-fix,
    the raw key silently fell through ``except Exception: raw_config =
    {}`` and was recovered via the ``key_as_secret`` fallback to
    ``SOMARK_API_KEY``. Post-fix, the same behavior is preserved via
    ``_is_raw_secret_key``.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = SoMarkOcrModel(
            key=json.dumps({"SOMARK_API_KEY": "sk-somark", "SOMARK_BASE_URL": "https://example"}),
            model_name="SoMark-model",
        )
        assert mdl.api_key == "sk-somark"
        assert mdl.base_url.rstrip("/").endswith("example")

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            SoMarkOcrModel(key=json.dumps(["not", "a", "key"]), model_name="SoMark-model")
        assert "OCR provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_uses_raw_key_fallback(self):
        """The documented raw-key feature: a plain string is treated
        as the api_key, falling back through the env-var chain.
        """
        mdl = SoMarkOcrModel(key="sk-somark-raw", model_name="SoMark-model")
        # The raw key is passed as the ``key_as_secret`` fallback to
        # ``SOMARK_API_KEY``; with no env var set, it ends up as the
        # api_key.
        assert mdl.api_key == "sk-somark-raw"


class TestMistralOcrModelCallSite:
    """``MistralOcrModel`` supports a raw secret key as a documented
    fallback for the api_key. Pre-fix, the raw key silently fell
    through and was recovered via ``key_as_secret``. Post-fix, the
    same behavior is preserved via ``_is_raw_secret_key``.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = MistralOcrModel(
            key=json.dumps({"MISTRAL_OCR_API_KEY": "sk-mistral", "MISTRAL_OCR_BASE_URL": "https://example"}),
            model_name="mistral-ocr-latest",
        )
        assert mdl.api_key == "sk-mistral"

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            MistralOcrModel(key=json.dumps(["not", "a", "key"]), model_name="mistral-ocr-latest")
        assert "OCR provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_uses_raw_key_fallback(self):
        """The documented raw-key feature: a plain Mistral API key is
        used as the api_key via the ``key_as_secret`` fallback.
        """
        mdl = MistralOcrModel(key="sk-mistral-raw", model_name="mistral-ocr-latest")
        assert mdl.api_key == "sk-mistral-raw"

    def test_flat_string_api_key_still_works(self):
        """Regression: a flat config whose ``api_key`` is a plain string
        (not a nested config object) must keep the key rather than
        discard it. Pre-existing test (test_mistral_ocr_model.py)
        covered this; re-asserted here after the helper refactor.
        """
        mdl = MistralOcrModel(
            key=json.dumps({"api_key": "sk-flat", "mistral_ocr_base_url": "https://api.mistral.ai/v1"}),
            model_name="mistral-ocr-latest",
        )
        assert mdl.api_key == "sk-flat"
        assert mdl.base_url.rstrip("/").endswith("api.mistral.ai/v1")
