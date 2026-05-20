#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import binascii
import hashlib
import logging
import os
from typing import Any, Optional

from rag.utils.redis_conn import REDIS_CONN

_DEFAULT_TTL_SECONDS = 7 * 24 * 60 * 60
_KEY_PREFIX = "tts:cache:"


def _ttl_seconds() -> int:
    raw = os.environ.get("RAGFLOW_TTS_CACHE_TTL_SECONDS")
    if not raw:
        return _DEFAULT_TTL_SECONDS
    try:
        v = int(raw)
        return v if v > 0 else 0
    except ValueError:
        logging.warning("Invalid RAGFLOW_TTS_CACHE_TTL_SECONDS=%r, using default", raw)
        return _DEFAULT_TTL_SECONDS


def _model_id(tts_mdl: Any) -> Optional[str]:
    cfg = getattr(tts_mdl, "model_config", None)
    if isinstance(cfg, dict):
        mid = cfg.get("id")
        if mid is not None:
            return str(mid)
        name = cfg.get("llm_name") or cfg.get("model_name")
        if name:
            return str(name)
    return None


def _build_key(tts_mdl: Any, text: str) -> Optional[str]:
    mid = _model_id(tts_mdl)
    if not mid:
        return None
    digest = hashlib.sha256(text.encode("utf-8", "ignore")).hexdigest()
    return f"{_KEY_PREFIX}{mid}:{digest}"


def _to_hex_string(value: Any) -> Optional[str]:
    if value is None:
        return None
    if isinstance(value, bytes):
        try:
            return value.decode("utf-8")
        except Exception:
            return None
    if isinstance(value, str):
        return value
    return None


def synthesize_with_cache(tts_mdl: Any, cleaned_text: str) -> Optional[str]:
    """
    Synthesize ``cleaned_text`` through ``tts_mdl`` and return a hex-encoded
    audio blob, reusing a Redis-cached result when available.

    The cache key is derived from the TTS model identifier and a SHA-256 of the
    text, so different models keep separate caches and the same text on the
    same model resolves to the same key regardless of call site. Returns
    ``None`` on synthesis failure; callers should treat that as a no-op the
    same way they do today.
    """
    if not tts_mdl or not cleaned_text:
        return None

    key = _build_key(tts_mdl, cleaned_text)

    if key:
        try:
            cached = REDIS_CONN.get(key)
        except Exception as e:
            logging.warning("TTS cache lookup failed: %s", e)
            cached = None
        hex_cached = _to_hex_string(cached)
        if hex_cached:
            return hex_cached

    buf = b""
    try:
        for chunk in tts_mdl.tts(cleaned_text):
            if isinstance(chunk, (bytes, bytearray)):
                buf += bytes(chunk)
    except Exception as e:
        logging.error("TTS failed: %s (text length=%d)", e, len(cleaned_text))
        return None

    if not buf:
        return None

    hex_value = binascii.hexlify(buf).decode("utf-8")

    ttl = _ttl_seconds()
    if key and ttl > 0:
        try:
            REDIS_CONN.set(key, hex_value, exp=ttl)
        except Exception as e:
            logging.warning("TTS cache store failed: %s", e)

    return hex_value
