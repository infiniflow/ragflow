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


def _resolve_volcengine_credentials(key):
    """Resolve a VolcEngine/Ark ``key`` into a dict with ``ark_api_key`` and
    optionally ``model_name``.

    Unlike Bedrock, VolcEngine/Ark accepts BOTH a plain API key string and a
    JSON object (the JSON form carries ``ark_api_key`` plus ``ep_id`` /
    ``endpoint_id`` so the model_name can be derived server-side without a
    follow-up ``/models`` call). See ``conf/models/volcengine.json`` for
    the full schema.

    Pre-fix, the three call sites in ``chat_model.py``, ``cv_model.py`` and
    ``embedding_model.py`` did ``json.loads(key).get("ark_api_key", "")``
    inside a ``try: ... except JSONDecodeError:`` block. JSON parsing
    succeeded for top-level non-objects (``"[1,2,3]"``, ``"42"``,
    ``'"hi"'``, ``"true"``, ``"null"``) and the subsequent ``.get(...)``
    call then crashed with ``AttributeError: 'list' object has no
    attribute 'get'`` from inside ``rag/llm`` internals -- no indication
    that the user pasted a non-object JSON. This helper closes that gap by
    raising a clear :class:`ModelException` on JSON-but-not-object input,
    while still accepting a plain (non-JSON) string as a valid bare API
    key, matching the pre-fix fallback semantics.

    Returns a dict ``{"ark_api_key": str, "model_name": str | None}``:

    - Plain (non-JSON) string key   -> ``{"ark_api_key": key, "model_name": None}``
      so the caller keeps using the ``model_name`` parameter passed to
      ``__init__`` (the historical behavior of the ``except JSONDecodeError``
      branch).
    - JSON dict                     -> ``{"ark_api_key": payload["ark_api_key"], "model_name": payload["ep_id"] + payload["endpoint_id"]}``
      (both fields default to ``""`` if missing; the model_name is
      ``None`` when both are missing, so the caller's parameter wins).
    - JSON top-level non-object     -> raises ``ModelException`` with the
      type name and a pointer at ``conf/models/volcengine.json``.
    - Anything else (None, int, list, ...) -> raises ``ModelException``.
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            # Plain (non-JSON) string key. Pre-fix behavior: use the
            # string as-is for ``ark_api_key`` and let the caller keep
            # the ``model_name`` parameter it passed in. Preserve.
            return {"ark_api_key": key, "model_name": None}
    else:
        logging.error(
            "VolcEngine key must be a string or dict, got %s",
            type(key).__name__,
        )
        raise ModelException(
            f"VolcEngine key must be a string or dict, got {type(key).__name__}. See conf/models/volcengine.json for the full schema.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        # JSON parsed but the top-level type is not an object. The pre-fix
        # code did ``payload.get("ark_api_key", "")`` here and crashed with
        # AttributeError on lists/strings/numbers/booleans/null. Surface a
        # clear error naming the actual type and the required schema.
        logging.error(
            "VolcEngine key JSON top-level type must be an object, got %s",
            type(payload).__name__,
        )
        raise ModelException(
            f"VolcEngine key must be a JSON object, got {type(payload).__name__}. "
            "Expected an object with 'ark_api_key' (and optionally 'ep_id' / 'endpoint_id'); "
            "see conf/models/volcengine.json for the full schema.",
            retryable=False,
        )

    ark_api_key = payload.get("ark_api_key", "")
    # Coerce ep_id / endpoint_id to str before concat: JSON values like
    # 12345 or null would otherwise raise TypeError when concatenated.
    ep_id = payload.get("ep_id", "")
    endpoint_id = payload.get("endpoint_id", "")
    model_name = (str(ep_id) if ep_id is not None else "") + (str(endpoint_id) if endpoint_id is not None else "")
    return {"ark_api_key": ark_api_key, "model_name": model_name or None}


__all__ = ["_normalize_replicate_key", "_resolve_volcengine_credentials"]
