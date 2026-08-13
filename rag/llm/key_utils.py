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
import json
import logging

from common.exceptions import ModelException


def _normalize_replicate_key(key):
    if isinstance(key, dict):
        if "api_key" in key:
            return key.get("api_key")
        return json.dumps(key)
    if isinstance(key, str):
        try:
            payload = json.loads(key)
            if isinstance(payload, dict) and "api_key" in payload:
                return payload.get("api_key")
        except (json.JSONDecodeError, TypeError):
            pass
    return key


def _resolve_provider_credentials(key):
    """Parse a TTS / Seq2txt provider ``key`` and return the config dict.

    Used by ``FishAudioTTS``, ``SparkTTS``, and ``TencentCloudSeq2txt``
    (3 sites that previously did a bare ``key = json.loads(key)`` with
    no try/except, no helper, and no fallback -- so a plain key
    produced a raw ``JSONDecodeError`` and a JSON non-object produced
    a raw ``AttributeError``).

    The expected ``key`` is one of:

    1. A falsy value (``None`` or empty string) -- returns ``{}``.
    2. A Python ``dict`` -- returned verbatim. Covers the API verify
       path that passes the form dict directly without JSON-encoding.
    3. A JSON-string-encoded object -- parsed and returned.

    On non-JSON input -- for example a user pasting a plain API key
    like ``"sk-..."`` -- raises a clear :class:`ModelException`
    pointing at the expected schema, instead of letting
    ``json.loads`` bubble up as ``json.decoder.JSONDecodeError`` from
    inside the provider init.

    On a JSON top-level type that is not a dict (list, string, number,
    bool, null), raises the same clear :class:`ModelException`
    instead of calling ``.get(...)`` on the value and getting
    ``AttributeError``.

    Returns a ``dict`` (possibly empty) on success.

    This is the 8th helper in the JSON-decode family in this file
    (alongside ``_resolve_bedrock_credentials``,
    ``_resolve_qianfan_credentials``, ``_resolve_volcengine_credentials``,
    ``_resolve_openrouter_credentials``,
    ``_resolve_google_service_account_key``,
    ``_resolve_azure_credentials``, ``_resolve_ocr_credentials``).
    Unlike the OCR helper, this one does not have a separate
    ``_is_raw_secret_key`` raw-key fallback -- the 3 sites covered
    here (FishAudio, Spark, Tencent) do not support a raw key as a
    documented api_key fallback, so a plain key is always a user
    mistake and should surface as a clear error.
    """
    if not key:
        return {}
    if isinstance(key, dict):
        return key
    if isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError) as exc:
            logging.warning(
                'TTS/Seq2txt provider key is not valid JSON; expected a JSON object (e.g. {"fish_audio_ak": "..."} for Fish Audio, or {"spark_app_id": "..."} for XunFei Spark). See conf/llm_factories.json for supported TTS / Seq2txt factories.',
            )
            raise ModelException(
                'TTS/Seq2txt provider key must be a JSON object. Example: {"fish_audio_ak": "..."} or {"spark_app_id": "..."} or {"tencent_cloud_sid": "..."}. See conf/llm_factories.json for supported TTS / Seq2txt factories.',
                retryable=False,
            ) from exc
    else:
        logging.warning(
            "TTS/Seq2txt provider key is not a string or dict (got %s); expected a JSON object.",
            type(key).__name__,
        )
        raise ModelException(
            f"TTS/Seq2txt provider key must be a JSON object or string, got {type(key).__name__}. See conf/llm_factories.json for supported TTS / Seq2txt factories.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        logging.warning(
            "TTS/Seq2txt provider key parsed as JSON but is not a dict (got %s).",
            type(payload).__name__,
        )
        raise ModelException(
            f"TTS/Seq2txt provider key must be a JSON object, got {type(payload).__name__}. See conf/llm_factories.json for supported TTS / Seq2txt factories.",
            retryable=False,
        )

    return payload


__all__ = ["_normalize_replicate_key", "_resolve_provider_credentials"]
