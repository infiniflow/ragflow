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

"""Regression tests for the TTS / Seq2txt provider key JSON-decode gap
across 3 sites (#17687).

The 3 provider classes -- ``FishAudioTTS`` (TTS),
``SparkTTS`` (TTS), and ``TencentCloudSeq2txt`` (Seq2txt) -- all
parsed their ``key`` with a bare ``key = json.loads(key)``: no
try/except, no helper, no fallback. When a user pasted a plain key,
``json.loads`` raised ``JSONDecodeError`` and the exception
propagated as a raw stack trace. When the user pasted a JSON
non-object (list, string, number, null, bool),
``key.get(field, default)`` raised ``AttributeError`` from inside
the provider init.

This test file covers two layers:

  1. The shared helper ``_resolve_provider_credentials`` in
     :mod:`rag.llm.key_utils` (~14 tests): falsy key returns
     ``{}``; Python dict passthrough; JSON dict happy path; JSON
     array / string / number / float / null / bool all raise
     ``ModelException``; plain string (most common user mistake)
     raises ``ModelException``; empty string raises
     ``ModelException``; invalid JSON raises ``ModelException``;
     non-string non-dict (int, None, list) raises
     ``ModelException``; helper is exported in ``__all__``.
  2. End-to-end on the 3 call sites (3 tests per site = 9 total):
     JSON dict constructs with parsed fields, JSON array raises
     ``ModelException``, and plain string raises
     ``ModelException`` (the bug-fix case).

The 3 sites do NOT support a raw secret key as a documented
fallback (unlike Mistral/SoMark in the OCR family) -- a plain key
is always a user mistake and should surface as a clear error.
"""

import json

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_provider_credentials
from rag.llm.sequence2txt_model import TencentCloudSeq2txt
from rag.llm.tts_model import FishAudioTTS, SparkTTS


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #


class TestResolveProviderCredentials:
    """``_resolve_provider_credentials`` parses a TTS / Seq2txt
    provider key into a config ``dict``. Falsy / dict /
    JSON-string-encoded dict inputs succeed; everything else
    raises ``ModelException`` with a clear message pointing at the
    expected schema.
    """

    def test_falsy_key_returns_empty_dict(self):
        assert _resolve_provider_credentials("") == {}
        assert _resolve_provider_credentials(None) == {}

    def test_python_dict_passes_through(self):
        out = _resolve_provider_credentials({"fish_audio_ak": "x"})
        assert out == {"fish_audio_ak": "x"}

    def test_json_dict_with_all_fields(self):
        raw = {"fish_audio_ak": "x", "fish_audio_refid": "r"}
        out = _resolve_provider_credentials(json.dumps(raw))
        assert out == raw

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "TTS/Seq2txt provider key" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials(json.dumps("just a string"))
        assert "object" in str(exc_info.value)
        assert "str" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials(json.dumps(42))
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials(json.dumps(3.14))
        assert "object" in str(exc_info.value)
        assert "float" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials("null")
        assert "object" in str(exc_info.value)
        assert "NoneType" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        """The most common user mistake: pasting a plain API key from
        the provider's portal. Pre-fix this was a raw
        ``JSONDecodeError: Expecting value: line 1 column 1 (char
        0)``. The fix raises a clear ``ModelException``.
        """
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials("sk-abc12345-very-long-api-key")
        msg = str(exc_info.value)
        assert "TTS/Seq2txt provider key" in msg
        assert "JSON object" in msg
        assert "JSONDecodeError" not in msg
        assert "Expecting value" not in msg
        assert exc_info.value.retryable is False

    def test_invalid_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_provider_credentials("not really json {")
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / tuple / None (non-empty) / etc. passed directly
        (not as a string or dict) must also raise. Only ``str`` and
        ``dict`` are valid inputs to the helper.
        """
        for bad in (42, ["a", "b"], (1, 2), 3.14, True):
            with pytest.raises(ModelException):
                _resolve_provider_credentials(bad)

    def test_empty_dict_returns_empty_dict(self):
        """An empty dict is a valid input (no config) and returns an
        empty dict -- the caller is responsible for handling it.
        """
        assert _resolve_provider_credentials({}) == {}

    def test_helper_lists_in_its_module_all(self):
        """The helper must be exported from ``rag.llm.key_utils``."""
        from rag.llm import key_utils

        assert "_resolve_provider_credentials" in key_utils.__all__


# --------------------------------------------------------------------------- #
# 2. End-to-end: 3 call sites
# --------------------------------------------------------------------------- #


class TestFishAudioTTSCallSite:
    """``FishAudioTTS`` (TTS) requires a JSON config. Pre-fix, a
    bare ``key = json.loads(key)`` raised raw ``JSONDecodeError`` on
    a plain key and raw ``AttributeError`` on a JSON non-object.
    The fix raises a clear ``ModelException``.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = FishAudioTTS(
            key=json.dumps({"fish_audio_ak": "ak-value", "fish_audio_refid": "ref-value"}),
            model_name="fish-audio-model",
        )
        assert mdl.headers["api-key"] == "ak-value"
        assert mdl.ref_id == "ref-value"

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            FishAudioTTS(key=json.dumps(["not", "a", "key"]), model_name="m")
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        """The bug-fix case: a plain API key used to raise a raw
        ``JSONDecodeError``. Now it raises a clear ``ModelException``.
        """
        with pytest.raises(ModelException) as exc_info:
            FishAudioTTS(
                key="sk-abc12345-very-long-fish-audio-key",
                model_name="m",
            )
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


class TestSparkTTSCallSite:
    """``SparkTTS`` (XunFei Spark TTS) requires a JSON config.
    Pre-fix, a bare ``key = json.loads(key)`` raised raw
    ``JSONDecodeError`` on a plain key. The fix raises.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = SparkTTS(
            key=json.dumps(
                {
                    "spark_app_id": "app-id",
                    "spark_api_secret": "api-secret",
                    "spark_api_key": "api-key",
                }
            ),
            model_name="spark-model",
        )
        assert mdl.APPID == "app-id"
        assert mdl.APISecret == "api-secret"
        assert mdl.APIKey == "api-key"

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            SparkTTS(key=json.dumps(["not", "a", "key"]), model_name="m")
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            SparkTTS(
                key="sk-abc12345-very-long-spark-api-key",
                model_name="m",
            )
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False


class TestTencentCloudSeq2txtCallSite:
    """``TencentCloudSeq2txt`` (Tencent Cloud ASR) requires a JSON
    config. Pre-fix, a bare ``key = json.loads(key)`` raised raw
    ``JSONDecodeError`` on a plain key. The fix raises.
    """

    def test_json_dict_constructs_with_parsed_fields(self):
        mdl = TencentCloudSeq2txt(
            key=json.dumps(
                {
                    "tencent_cloud_sid": "sid-value",
                    "tencent_cloud_sk": "sk-value",
                }
            ),
            model_name="16k_zh",
        )
        assert mdl.model_name == "16k_zh"
        # The Tencent SDK stores credentials internally; we just
        # check that construction succeeded without raising, which
        # implies the JSON was parsed and the credentials were
        # extracted without AttributeError.
        assert mdl.client is not None

    def test_json_array_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            TencentCloudSeq2txt(
                key=json.dumps(["not", "a", "key"]),
                model_name="16k_zh",
            )
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "list" in str(exc_info.value)
        assert "AttributeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            TencentCloudSeq2txt(
                key="sk-abc12345-very-long-tencent-sid",
                model_name="16k_zh",
            )
        assert "TTS/Seq2txt provider key" in str(exc_info.value)
        assert "JSON object" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False
