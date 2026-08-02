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


def _resolve_ocr_credentials(key):
    """Parse an OCR model ``key`` and return the raw config dict.

    Used by the 5 OCR provider classes in :mod:`rag.llm.ocr_model`
    (``MinerUOcrModel``, ``PaddleOCROcrModel``, ``OpenDataLoaderOcrModel``,
    ``SoMarkOcrModel``, ``MistralOcrModel``).

    The expected ``key`` is one of:

    1. A falsy value (``None`` or empty string) -- returns ``{}``.
       Matches the existing ``if key:`` short-circuit in every class.
    2. A Python ``dict`` -- returned verbatim. Covers the API verify
       path that passes the form dict directly without JSON-encoding.
    3. A JSON-string-encoded object -- parsed and returned.

    On non-JSON input -- for example a user pasting a plain API key
    like ``"sk-..."`` -- raises a clear :class:`ModelException`
    pointing at the expected schema, instead of letting
    ``json.loads`` bubble up as ``json.decoder.JSONDecodeError`` from
    inside the provider init (which the previous code path silently
    swallowed via ``except Exception: raw_config = {}``, dropping the
    user's key on the floor and falling back to host env vars).

    On a JSON top-level type that is not a dict (list, string, number,
    bool, null), raises the same clear :class:`ModelException`
    instead of calling ``.get(...)`` on the value and getting
    ``AttributeError``.

    Returns a ``dict`` (possibly empty) on success.
    """
    if not key:
        return {}
    if isinstance(key, dict):
        return key
    if isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            logging.warning(
                'OCR provider key is not valid JSON; expected a JSON object (e.g. nested {"api_key": {...}} from the UI, or flat {"PROVIDER_*": "..."} auto-provisioned from env vars). See conf/llm_factories.json for supported OCR factories.',
            )
            raise ModelException(
                'OCR provider key must be a JSON object. Example: {"api_key": {"mineru_apiserver": "..."}} or {"MINERU_API_KEY": "..."}. See conf/llm_factories.json for supported OCR factories.',
                retryable=False,
            )
    else:
        logging.warning(
            "OCR provider key is not a string or dict (got %s); expected a JSON object.",
            type(key).__name__,
        )
        raise ModelException(
            f"OCR provider key must be a JSON object or string, got {type(key).__name__}. See conf/llm_factories.json for supported OCR factories.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        logging.warning(
            "OCR provider key parsed as JSON but is not a dict (got %s).",
            type(payload).__name__,
        )
        raise ModelException(
            f"OCR provider key must be a JSON object, got {type(payload).__name__}. See conf/llm_factories.json for supported OCR factories.",
            retryable=False,
        )

    return payload


__all__ = ["_normalize_replicate_key", "_resolve_ocr_credentials"]
