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


def _resolve_google_service_account_key(key):
    """Resolve a Google Cloud Vertex AI service-account ``key`` into a dict.

    The Google Cloud Vertex provider (which covers both AnthropicVertex
    and GeminiVertex) requires a JSON object key with three fields,
    per the schema in ``api/apps/llm_app.py:291`` and the test fixture
    in ``test/testcases/test_web_api/test_llm_app/test_llm_list_unit.py:635-640``:

    - ``google_project_id``     -- the GCP project ID
    - ``google_region``         -- the region (e.g. ``"us-central1"``)
    - ``google_service_account_key`` -- base64-encoded service-account JSON

    Unlike Bedrock/BaiduYiyan/VolcEngine/OpenRouter, the Google Vertex
    provider does NOT accept a plain API key string -- it requires the
    full object. So this helper:

    1. Accepts a pre-parsed dict (returned verbatim) or a
       JSON-string-encoded dict (parsed and returned).
    2. On non-JSON input, on a JSON top-level type that is not a dict
       (list, string, number, bool, null), or on any other input type
       (None, int, list, ...), raises a clear
       :class:`ModelException(retryable=False)` naming the required
       object shape and pointing at the schema in
       ``api/apps/llm_app.py:291``.

    Returns a dict ``{"google_project_id": str, "google_region": str,
    "google_service_account_key": str}`` (all three default to ``""``
    if missing).
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError) as exc:
            raise ModelException(
                "Google Vertex AI key is not valid JSON. Expected an object with 'google_project_id', 'google_region', and 'google_service_account_key' (see api/apps/llm_app.py:291 for the schema).",
                retryable=False,
            ) from exc
    else:
        raise ModelException(
            f"Google Vertex AI key must be a string or dict, got {type(key).__name__}. "
            "Expected an object with 'google_project_id', 'google_region', and "
            "'google_service_account_key' (see api/apps/llm_app.py:291 for the schema).",
            retryable=False,
        )

    if not isinstance(payload, dict):
        # JSON parsed but the top-level type is not an object. The pre-fix
        # code did ``payload.get("google_service_account_key", "")`` here
        # and crashed with AttributeError on lists/strings/numbers/booleans/
        # null. Surface a clear error naming the actual type and the
        # required schema.
        raise ModelException(
            f"Google Vertex AI key must be a JSON object, got {type(payload).__name__}. "
            "Expected an object with 'google_project_id', 'google_region', and "
            "'google_service_account_key' (see api/apps/llm_app.py:291 for the schema).",
            retryable=False,
        )

    return {
        "google_project_id": payload.get("google_project_id", ""),
        "google_region": payload.get("google_region", ""),
        "google_service_account_key": payload.get("google_service_account_key", ""),
    }


__all__ = ["_normalize_replicate_key", "_resolve_google_service_account_key"]
