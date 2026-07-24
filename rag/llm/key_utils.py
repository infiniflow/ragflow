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


_BEDROCK_KEY_HINT = (
    "Bedrock credentials must be a JSON object carrying at least 'auth_mode' and "
    "'bedrock_region', for example "
    '{"auth_mode": "access_key_secret", "bedrock_region": "us-east-1", '
    '"bedrock_ak": "...", "bedrock_sk": "..."}. '
    "See conf/models/bedrock.json for the full schema."
)


def _resolve_bedrock_credentials(key):
    """Return the credential dict encoded in a Bedrock ``key``.

    Every Bedrock connector reads its credentials from a JSON object. A bare
    string in that field - a plain AWS access key, say - used to reach
    ``json.loads`` unguarded and surface a ``JSONDecodeError`` raised from
    inside the connector, which tells the operator nothing about what to fix.
    """
    if isinstance(key, dict):
        payload = key
    else:
        try:
            payload = json.loads(key)
        except (TypeError, ValueError) as e:
            raise ValueError(_BEDROCK_KEY_HINT) from e
    if not isinstance(payload, dict):
        raise ValueError(_BEDROCK_KEY_HINT)
    return payload


__all__ = ["_normalize_replicate_key", "_resolve_bedrock_credentials"]
