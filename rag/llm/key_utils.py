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


def _resolve_openrouter_credentials(key):
    """Resolve an OpenRouter ``key`` into a dict with ``api_key`` and
    ``provider_order``.

    OpenRouter accepts BOTH a plain API key string and a JSON object (the
    JSON form carries ``api_key`` plus ``provider_order`` so the user can
    pin a list of providers and disable fallbacks). See
    ``conf/models/openrouter.json`` for the model list and
    ``conf/llm_factories.json`` for the factory entry.

    Pre-fix, the three call sites in ``chat_model.py`` (the OpenRouter
    branch of ``LiteLLMBase.__init__``), ``cv_model.py`` and
    ``embedding_model.py`` did ``json.loads(key).get("api_key", "")``
    inside a ``try: ... except JSONDecodeError:`` block. The chat and CV
    sites crashed with ``AttributeError: 'list' object has no attribute
    'get'`` (or ``'int'``, ``'str'``, ``'NoneType'``, ``'bool'``) when
    the user pasted a JSON non-object; the embedding site silently used
    the raw JSON string as the API key, which then failed auth later
    with a less-actionable 401. PR #15776 added the partial
    ``except JSONDecodeError`` guard but did not address the
    ``AttributeError`` case.

    This helper:

    1. Accepts a plain (non-JSON) string and returns
       ``{"api_key": key, "provider_order": ""}`` -- matching the
       pre-fix ``except JSONDecodeError`` fallback semantics.
    2. Accepts a dict (or a JSON-string-encoded dict) and returns the
       parsed ``api_key`` and ``provider_order`` (both defaulting to
       ``""`` if missing).
    3. On a JSON top-level type that is not a dict (list, string,
       number, bool, null), raises a clear :class:`ModelException`
       naming the actual type and pointing at
       ``conf/models/openrouter.json``.

    Returns a dict ``{"api_key": str, "provider_order": str}``.
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            # Plain (non-JSON) string key. Pre-fix behavior: use the
            # string as-is for ``api_key`` and default ``provider_order``
            # to empty. Preserve.
            return {"api_key": key, "provider_order": ""}
    else:
        raise ModelException(
            f"OpenRouter key must be a string or dict, got {type(key).__name__}. See conf/models/openrouter.json for the full schema.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        # JSON parsed but the top-level type is not an object. The pre-fix
        # code did ``payload.get("api_key", "")`` here and crashed with
        # AttributeError on lists/strings/numbers/booleans/null. Surface a
        # clear error naming the actual type and the required schema.
        raise ModelException(
            f"OpenRouter key must be a JSON object, got {type(payload).__name__}. "
            "Expected an object with 'api_key' (and optionally 'provider_order'); "
            "see conf/models/openrouter.json for the full schema.",
            retryable=False,
        )

    return {
        "api_key": payload.get("api_key", ""),
        "provider_order": payload.get("provider_order", ""),
    }


__all__ = ["_normalize_replicate_key", "_resolve_openrouter_credentials"]
