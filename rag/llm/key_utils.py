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
    """Normalize an Azure-OpenAI credential string.

    Accepts a JSON object (``{"api_key": "...", "api_version": "..."}``), a
    plain string key, or anything else. Returns ``(api_key, api_version)``,
    defaulting ``api_version`` to ``"2024-02-01"`` when not specified.
    Falls back to the raw ``key`` value verbatim if ``json.loads`` fails so a
    user pasting the API key straight from the Azure Portal still validates
    instead of crashing with ``json.decoder.JSONDecodeError`` (see #17204).
    """
    try:
        key_obj = json.loads(key)
        if isinstance(key_obj, dict):
            return key_obj.get("api_key", ""), key_obj.get("api_version", "2024-02-01")
        logging.warning("Azure credential payload parsed as JSON but is not an object; using raw api_key string")
    except (json.JSONDecodeError, TypeError):
        logging.warning("Azure credential payload is not valid JSON; using raw api_key string")
    return key, "2024-02-01"


__all__ = ["_normalize_replicate_key", "_resolve_azure_credentials"]
