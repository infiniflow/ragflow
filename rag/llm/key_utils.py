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


def _resolve_bedrock_credentials(key):
    """Parse a Bedrock ``key`` and return it as a dict.

    Unlike Azure-OpenAI, Bedrock REQUIRES a JSON object — the underlying
    boto3 client needs ``auth_mode`` and ``bedrock_region`` at minimum, plus
    provider-specific fields (e.g. ``bedrock_ak``/``bedrock_sk`` for
    ``access_key_secret`` mode, ``aws_role_arn`` for ``iam_role`` mode). See
    ``conf/models/bedrock.json`` for the full schema.

    On non-JSON input — for example a user pasting a plain AWS access key
    like ``"AKIAIOSFODNN7EXAMPLE"`` — we raise a clear :class:`ModelException`
    pointing at the required schema, instead of letting ``json.loads`` bubble
    up as ``json.decoder.JSONDecodeError: Expecting value: line 1 column 1
    (char 0)`` from inside ``rag/llm`` internals. Mirrors the fix shape of
    ``_resolve_azure_credentials`` for #17204 / #17373.

    On a JSON top-level type that is not a dict (e.g. a list, string, or
    number), we also raise the same clear error rather than calling
    ``.get("auth_mode")`` on the value and getting an ``AttributeError``.
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            logging.warning(
                "Bedrock key is not valid JSON; expected a JSON object with 'auth_mode' and 'bedrock_region' (see conf/models/bedrock.json).",
            )
            raise ModelException(
                "Bedrock requires a JSON key with at least 'auth_mode' and "
                '\'bedrock_region\'. Example: {"auth_mode": "access_key_secret", '
                '"bedrock_region": "us-east-1", "bedrock_ak": "...", '
                '"bedrock_sk": "..."}. See conf/models/bedrock.json for the '
                "full schema.",
                retryable=False,
            )
    else:
        logging.warning(
            "Bedrock key is not a string or dict (got %s); expected a JSON object with 'auth_mode' and 'bedrock_region'.",
            type(key).__name__,
        )
        raise ModelException(
            f"Bedrock requires a JSON key, got {type(key).__name__}. See conf/models/bedrock.json for the full schema.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        logging.warning(
            "Bedrock key parsed as JSON but is not a dict (got %s).",
            type(payload).__name__,
        )
        raise ModelException(
            f"Bedrock key must be a JSON object, got {type(payload).__name__}. See conf/models/bedrock.json for the full schema.",
            retryable=False,
        )

    return payload


__all__ = ["_normalize_replicate_key", "_resolve_bedrock_credentials"]
