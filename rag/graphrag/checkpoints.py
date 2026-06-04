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

from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.misc_utils import thread_pool_exec
from rag.nlp import search


COMMUNITY_CHECKPOINT = "graphrag_checkpoint_community"
RESOLUTION_CHECKPOINT = "graphrag_checkpoint_resolution"
CHECKPOINT_PAGE_SIZE = 1000


def stable_checkpoint_key(*parts: Any) -> str:
    payload = json.dumps(parts, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def community_checkpoint_key(level: str, community_id: str, nodes: list[str]) -> str:
    return stable_checkpoint_key("community", str(level), str(community_id), sorted(nodes))


def resolution_checkpoint_key(entity_type: str, pairs: list[tuple[str, str]]) -> str:
    normalized_pairs = sorted([sorted([a, b]) for a, b in pairs])
    return stable_checkpoint_key("resolution", entity_type, normalized_pairs)


def checkpoint_chunk_id(kb_id: str, checkpoint_type: str, checkpoint_key: str) -> str:
    return stable_checkpoint_key("checkpoint", kb_id, checkpoint_type, checkpoint_key)


async def load_checkpoints(tenant_id: str, kb_id: str, checkpoint_type: str, *, page_size: int = CHECKPOINT_PAGE_SIZE) -> dict[str, Any]:
    checkpoints: dict[str, Any] = {}
    offset = 0
    fields = ["id", "content_with_weight"]
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields,
                [],
                {"knowledge_graph_kwd": [checkpoint_type]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                search.index_name(tenant_id),
                [kb_id],
            )
            rows = settings.docStoreConn.get_fields(res, fields)
        except Exception:
            logging.exception("Failed to load GraphRAG checkpoints type=%s kb=%s offset=%s", checkpoint_type, kb_id, offset)
            return checkpoints

        if not rows:
            return checkpoints

        for row in rows.values():
            try:
                stored = json.loads(row.get("content_with_weight") or "{}")
                key = stored.get("key")
                if key:
                    checkpoints[key] = stored.get("payload")
            except Exception:
                logging.exception("Failed to parse GraphRAG checkpoint row type=%s kb=%s", checkpoint_type, kb_id)

        if len(rows) < page_size:
            return checkpoints
        offset += page_size


async def save_checkpoint(tenant_id: str, kb_id: str, checkpoint_type: str, checkpoint_key: str, payload: Any) -> bool:
    stored = {
        "type": checkpoint_type,
        "key": checkpoint_key,
        "payload": payload,
    }
    chunk = {
        "id": checkpoint_chunk_id(kb_id, checkpoint_type, checkpoint_key),
        "content_with_weight": json.dumps(stored, ensure_ascii=False),
        "knowledge_graph_kwd": checkpoint_type,
        "kb_id": kb_id,
        "source_id": [checkpoint_key],
        "available_int": 0,
    }
    try:
        result = await thread_pool_exec(settings.docStoreConn.insert, [chunk], search.index_name(tenant_id), kb_id)
        if result:
            logging.warning("GraphRAG checkpoint insert returned errors type=%s kb=%s key=%s errors=%s", checkpoint_type, kb_id, checkpoint_key, result)
            return False
        return True
    except Exception:
        logging.exception("Failed to save GraphRAG checkpoint type=%s kb=%s key=%s", checkpoint_type, kb_id, checkpoint_key)
        return False


async def cleanup_checkpoints(tenant_id: str, kb_id: str, checkpoint_type: str) -> bool:
    try:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"knowledge_graph_kwd": [checkpoint_type], "kb_id": kb_id},
            search.index_name(tenant_id),
            kb_id,
        )
        return True
    except Exception:
        logging.exception("Failed to cleanup GraphRAG checkpoints type=%s kb=%s", checkpoint_type, kb_id)
        return False
