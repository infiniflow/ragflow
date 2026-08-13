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


def _resolve_azure_credentials(key):
    """Parse an Azure-OpenAI ``key`` and return ``(api_key, api_version)``.

    The Azure-OpenAI provider requires a JSON object key with at least
    ``api_key``; ``api_version`` defaults to ``"2024-02-01"`` if missing.
    See ``conf/llm_factories.json`` for the factory entry and
    ``conf/models/azure.json`` for the model class.

    On non-JSON input -- for example a user pasting a plain Azure API
    key like ``"abc...123"`` from the Azure Portal -- we raise a clear
    :class:`ModelException` pointing at the required schema, instead of
    letting ``json.loads`` bubble up as ``json.decoder.JSONDecodeError:
    Expecting value: line 1 column 1 (char 0)`` from inside ``rag/llm``
    internals. Mirrors the fix shape of ``_resolve_bedrock_credentials``
    for #17373 and the Azure fix for #17204 / #17215.

    On a JSON top-level type that is not a dict (e.g. a list, string, or
    number), we also raise the same clear error rather than calling
    ``.get("api_key")`` on the value and getting an ``AttributeError``.

    Returns ``(api_key, api_version)``. Both default to ``""`` and
    ``"2024-02-01"`` respectively if missing from a valid dict.
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError) as exc:
            logging.warning(
                "Azure-OpenAI key is not valid JSON; expected a JSON object with 'api_key' (and optionally 'api_version') (see conf/models/azure.json).",
            )
            raise ModelException(
                'Azure-OpenAI requires a JSON key with at least \'api_key\'. Example: {"api_key": "...", "api_version": "2024-02-01"}. See conf/models/azure.json for the model class.',
                retryable=False,
            ) from exc
    else:
        logging.warning(
            "Azure-OpenAI key is not a string or dict (got %s); expected a JSON object with 'api_key' (and optionally 'api_version').",
            type(key).__name__,
        )
        raise ModelException(
            f"Azure-OpenAI requires a JSON key, got {type(key).__name__}. See conf/models/azure.json for the model class.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        logging.warning(
            "Azure-OpenAI key parsed as JSON but is not a dict (got %s).",
            type(payload).__name__,
        )
        raise ModelException(
            f"Azure-OpenAI key must be a JSON object, got {type(payload).__name__}. See conf/models/azure.json for the model class.",
            retryable=False,
        )

    return payload.get("api_key", ""), payload.get("api_version", "2024-02-01")


__all__ = ["_normalize_replicate_key", "_resolve_azure_credentials"]
