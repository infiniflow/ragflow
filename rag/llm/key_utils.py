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


def _resolve_qianfan_credentials(key):
    """Parse a BaiduYiyan (Baidu Qianfan) ``key`` and return it as a dict.

    The BaiduYiyan / Qianfan API requires a JSON object with at least
    ``yiyan_ak`` and ``yiyan_sk``. See ``conf/models/baidu.json`` for the
    model class.

    On non-JSON input -- for example a user pasting a plain Baidu API key
    like ``"bce-v3/ALTAK-.../..."`` -- we raise a clear
    :class:`ModelException` pointing at the required schema, instead of
    letting ``json.loads`` bubble up as ``json.decoder.JSONDecodeError:
    Expecting value: line 1 column 1 (char 0)`` from inside ``rag/llm``
    internals. Mirrors the fix shape of ``_resolve_bedrock_credentials``
    for #17373 and the Azure fix for #17204 / #17215.

    On a JSON top-level type that is not a dict (e.g. a list, string, or
    number), we also raise the same clear error rather than calling
    ``.get("yiyan_ak")`` on the value and getting an ``AttributeError``.
    """
    if isinstance(key, dict):
        payload = key
    elif isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError) as exc:
            logging.warning(
                "BaiduYiyan key is not valid JSON; expected a JSON object with 'yiyan_ak' and 'yiyan_sk' (see conf/models/baidu.json).",
            )
            raise ModelException(
                'BaiduYiyan requires a JSON key with at least \'yiyan_ak\' and \'yiyan_sk\'. Example: {"yiyan_ak": "...", "yiyan_sk": "..."}. See conf/models/baidu.json for the model class.',
                retryable=False,
            ) from exc
    else:
        logging.warning(
            "BaiduYiyan key is not a string or dict (got %s); expected a JSON object with 'yiyan_ak' and 'yiyan_sk'.",
            type(key).__name__,
        )
        raise ModelException(
            f"BaiduYiyan requires a JSON key, got {type(key).__name__}. See conf/models/baidu.json for the model class.",
            retryable=False,
        )

    if not isinstance(payload, dict):
        logging.warning(
            "BaiduYiyan key parsed as JSON but is not a dict (got %s).",
            type(payload).__name__,
        )
        raise ModelException(
            f"BaiduYiyan key must be a JSON object, got {type(payload).__name__}. See conf/models/baidu.json for the model class.",
            retryable=False,
        )

    return payload


__all__ = ["_normalize_replicate_key", "_resolve_qianfan_credentials"]
