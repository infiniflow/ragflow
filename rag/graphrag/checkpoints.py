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

from __future__ import annotations

import hashlib
import json
import logging
from typing import Any

from common.misc_utils import thread_pool_exec
from rag.utils.redis_conn import REDIS_CONN


COMMUNITY_CHECKPOINT = "graphrag_checkpoint_community"
RESOLUTION_CHECKPOINT = "graphrag_checkpoint_resolution"
CHECKPOINT_PAGE_SIZE = 1000
CHECKPOINT_TTL_SECONDS = 7 * 24 * 3600


def stable_checkpoint_key(*parts: Any) -> str:
    payload = json.dumps(parts, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def community_checkpoint_key(level: str, community_id: str, nodes: list[str]) -> str:
    return stable_checkpoint_key("community", str(level), str(community_id), sorted(nodes))


def resolution_checkpoint_key(entity_type: str, pairs: list[tuple[str, str]]) -> str:
    normalized_pairs = sorted([sorted([a, b]) for a, b in pairs])
    return stable_checkpoint_key("resolution", entity_type, normalized_pairs)


def _checkpoint_index_key(tenant_id: str, kb_id: str, checkpoint_type: str) -> str:
    return f"graphrag:checkpoint:{tenant_id}:{kb_id}:{checkpoint_type}:keys"


def _checkpoint_data_key(tenant_id: str, kb_id: str, checkpoint_type: str, checkpoint_key: str) -> str:
    return f"graphrag:checkpoint:{tenant_id}:{kb_id}:{checkpoint_type}:{checkpoint_key}"


def _decode_redis_value(value: Any) -> Any:
    if isinstance(value, bytes):
        return value.decode("utf-8")
    return value


def _checkpoint_page_size(page_size: int | None) -> int:
    return page_size if page_size and page_size > 0 else CHECKPOINT_PAGE_SIZE


def _iter_checkpoint_keys(index_key: str, page_size: int | None):
    redis_client = getattr(REDIS_CONN, "REDIS", None)
    if redis_client is None or not hasattr(redis_client, "sscan_iter"):
        raise RuntimeError("Redis SSCAN is unavailable for GraphRAG checkpoint index iteration")
    return redis_client.sscan_iter(index_key, count=_checkpoint_page_size(page_size))


def _load_checkpoints_sync(tenant_id: str, kb_id: str, checkpoint_type: str, page_size: int | None) -> dict[str, Any]:
    checkpoints: dict[str, Any] = {}
    index_key = _checkpoint_index_key(tenant_id, kb_id, checkpoint_type)
    try:
        checkpoint_keys = _iter_checkpoint_keys(index_key, page_size)
    except Exception:
        logging.exception("Failed to load GraphRAG checkpoint index type=%s kb=%s", checkpoint_type, kb_id)
        return checkpoints

    for checkpoint_key in checkpoint_keys:
        checkpoint_key = _decode_redis_value(checkpoint_key)
        try:
            value = REDIS_CONN.get(_checkpoint_data_key(tenant_id, kb_id, checkpoint_type, checkpoint_key))
            value = _decode_redis_value(value)
            if not value:
                continue
            checkpoints[checkpoint_key] = json.loads(value)
        except Exception:
            logging.exception("Failed to parse GraphRAG checkpoint type=%s kb=%s key=%s", checkpoint_type, kb_id, checkpoint_key)
    logging.info("Loaded %d GraphRAG checkpoints type=%s kb=%s", len(checkpoints), checkpoint_type, kb_id)
    return checkpoints


async def load_checkpoints(tenant_id: str, kb_id: str, checkpoint_type: str, *, page_size: int | None = None) -> dict[str, Any]:
    return await thread_pool_exec(_load_checkpoints_sync, tenant_id, kb_id, checkpoint_type, page_size)


async def save_checkpoint(tenant_id: str, kb_id: str, checkpoint_type: str, checkpoint_key: str, payload: Any) -> bool:
    index_key = _checkpoint_index_key(tenant_id, kb_id, checkpoint_type)
    data_key = _checkpoint_data_key(tenant_id, kb_id, checkpoint_type, checkpoint_key)
    try:
        redis_client = getattr(REDIS_CONN, "REDIS", None)
        if redis_client is None or not hasattr(redis_client, "pipeline"):
            logging.warning("GraphRAG checkpoint Redis client unavailable type=%s kb=%s key=%s", checkpoint_type, kb_id, checkpoint_key)
            return False
        pipeline = redis_client.pipeline(transaction=True)
        pipeline.set(data_key, json.dumps(payload, ensure_ascii=False), ex=CHECKPOINT_TTL_SECONDS)
        pipeline.sadd(index_key, checkpoint_key)
        pipeline.expire(index_key, CHECKPOINT_TTL_SECONDS)
        pipeline.execute()
        logging.info("Saved GraphRAG checkpoint type=%s kb=%s key=%s", checkpoint_type, kb_id, checkpoint_key)
        return True
    except Exception:
        logging.exception("Failed to save GraphRAG checkpoint type=%s kb=%s key=%s", checkpoint_type, kb_id, checkpoint_key)
        return False


async def cleanup_checkpoints(tenant_id: str, kb_id: str, checkpoint_type: str, *, page_size: int | None = None) -> bool:
    index_key = _checkpoint_index_key(tenant_id, kb_id, checkpoint_type)
    try:
        cleaned_count = 0
        checkpoint_keys = _iter_checkpoint_keys(index_key, page_size)
        for checkpoint_key in checkpoint_keys:
            checkpoint_key = _decode_redis_value(checkpoint_key)
            REDIS_CONN.delete(_checkpoint_data_key(tenant_id, kb_id, checkpoint_type, checkpoint_key))
            cleaned_count += 1
        REDIS_CONN.delete(index_key)
        logging.info("Cleaned up %d GraphRAG checkpoints type=%s kb=%s", cleaned_count, checkpoint_type, kb_id)
        return True
    except Exception:
        logging.exception("Failed to cleanup GraphRAG checkpoints type=%s kb=%s", checkpoint_type, kb_id)
        return False
