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
"""JSON-decode helpers for LLM / OCR / Rerank / TTS / Seq2txt / CV / Embed / ModelMeta providers.

These helpers normalize the user-supplied ``key`` into a predictable shape
(plain string OR a JSON-object dict), and raise a clear
:class:`common.exceptions.ModelException` (retryable=False) on input that
parses to JSON but is not a dict at the top level -- the pre-fix call
sites in chat_model.py / cv_model.py / embedding_model.py / ocr_model.py /
tts_model.py / sequence2txt_model.py / rerank_model.py / model_meta.py
either crashed with ``AttributeError`` from the subsequent ``.get(...)``
call or silently used the raw JSON string as the API key, in both cases
producing a less-actionable 401 from the upstream API.

The helpers in this file are added in multiple PRs (cycles 2, 3, 7, 8, 9,
10, 12, 13, 15, 16, and 19). Some are still on open PRs and not yet
merged on ``upstream/main``; consumers in dependent PRs re-add the helpers
they need so the PR is self-contained, and the maintainer can drop the
duplicate on rebase. See the individual helper docstrings for the
specific PR overlap and the caller set.
"""

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


def _resolve_volcengine_credentials(key):
    """Resolve a VolcEngine/Ark ``key`` into a dict with ``ark_api_key`` and
    optionally ``model_name``.

    Overlap with PR #17457 (cycle 7). See that PR for the full
    background; this re-addition keeps the model_meta.py fix in PR
    #18250 self-contained so the maintainer can drop the duplicate on
    rebase.

    Returns a dict ``{"ark_api_key": str, "model_name": str | None}``:

    - ``None`` or empty string              -> ``{"ark_api_key": "", "model_name": None}``
    - Plain (non-JSON) string               -> ``{"ark_api_key": key, "model_name": None}``
    - ``dict``                              -> verbatim (with field defaults)
    - JSON-string-encoded object            -> parsed dict
    - JSON non-object (list, int, ...)      -> :class:`ModelException`
    """
    if not key:
        return {"ark_api_key": "", "model_name": None}
    if isinstance(key, dict):
        return {
            "ark_api_key": key.get("ark_api_key", key.get("api_key", "")),
            "model_name": (key.get("ep_id", "") + key.get("endpoint_id", "")) or None,
        }
    if isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            # Plain non-JSON string: treat as a bare API key (matches the
            # historical ``except JSONDecodeError`` fallback in
            # chat_model.py / cv_model.py / embedding_model.py).
            return {"ark_api_key": key, "model_name": None}
    else:
        raise ModelException(
            f"VolcEngine/Ark key must be a JSON object or a plain string, got {type(key).__name__}. See conf/models/volcengine.json for the schema.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        raise ModelException(
            f"VolcEngine/Ark key must be a JSON object, got {type(payload).__name__}. See conf/models/volcengine.json for the schema.",
            retryable=False,
        )
    return {
        "ark_api_key": payload.get("ark_api_key", payload.get("api_key", "")),
        "model_name": (payload.get("ep_id", "") + payload.get("endpoint_id", "")) or None,
    }


def _resolve_openrouter_credentials(key):
    """Resolve an OpenRouter or NewAPI ``key`` into a dict with
    ``api_key`` and ``provider_order``.

    Overlap with PR #17459 (cycle 8). See that PR for the full
    background; this re-addition keeps the model_meta.py fix in PR
    #18250 self-contained so the maintainer can drop the duplicate on
    rebase. NewAPI reuses this helper because the model_meta.py
    NewAPI site has the same shape (plain string OR JSON dict with
    ``api_key``).

    Returns a dict ``{"api_key": str, "provider_order": str}``:

    - ``None`` or empty string              -> ``{"api_key": "", "provider_order": ""}``
    - Plain (non-JSON) string               -> ``{"api_key": key, "provider_order": ""}``
    - ``dict``                              -> verbatim (with field defaults)
    - JSON-string-encoded object            -> parsed dict
    - JSON non-object (list, int, ...)      -> :class:`ModelException`
    """
    if not key:
        return {"api_key": "", "provider_order": ""}
    if isinstance(key, dict):
        return {
            "api_key": key.get("api_key", ""),
            "provider_order": key.get("provider_order", ""),
        }
    if isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            return {"api_key": key, "provider_order": ""}
    else:
        raise ModelException(
            f"OpenRouter/NewAPI key must be a JSON object or a plain string, got {type(key).__name__}. See conf/models/openrouter.json for the schema.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        raise ModelException(
            f"OpenRouter/NewAPI key must be a JSON object, got {type(payload).__name__}. See conf/models/openrouter.json for the schema.",
            retryable=False,
        )
    return {
        "api_key": payload.get("api_key", ""),
        "provider_order": payload.get("provider_order", ""),
    }


__all__ = [
    "_normalize_replicate_key",
    "_resolve_volcengine_credentials",
    "_resolve_openrouter_credentials",
]
