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

"""
RAPTOR chunk management utilities.

Provides functions for managing RAPTOR summary chunks,
including detection, retrieval, and deletion.
"""

import logging

from common.misc_utils import thread_pool_exec
from common import settings
from rag.nlp import search as nlp_search
from rag.utils.raptor_utils import (
    collect_raptor_chunk_ids,
)

RAPTOR_METHOD_SEARCH_LIMIT = 10000


async def get_raptor_chunk_field_map(doc_id: str, tenant_id: str, kb_id: str) -> dict:
    """Return stored RAPTOR marker fields for a document."""
    from common.doc_store.doc_store_base import OrderByExpr

    async def search_fields(fields: list[str], condition: dict, order_by=None):
        """Search chunk fields in the current knowledge base."""
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields, [], condition, [], order_by or OrderByExpr(),
            0, RAPTOR_METHOD_SEARCH_LIMIT, nlp_search.index_name(tenant_id), [kb_id]
        )
        return settings.docStoreConn.get_fields(res, fields)

    primary = await search_fields(["raptor_kwd", "extra"], {"doc_id": doc_id, "raptor_kwd": ["raptor"]})
    if collect_raptor_chunk_ids(primary):
        return primary

    try:
        return await search_fields(
            ["raptor_kwd", "extra"],
            {"doc_id": doc_id},
            OrderByExpr().desc("create_timestamp_flt"),
        )
    except Exception:
        logging.debug("RAPTOR fallback method lookup with extra field failed for doc %s", doc_id, exc_info=True)
        return primary


async def delete_raptor_chunks(doc_id: str, tenant_id: str, kb_id: str, keep_method: str | None = None) -> int:
    """Delete RAPTOR summaries for doc_id, optionally preserving one method."""
    if keep_method is None:
        logging.info(
            "delete_raptor_chunks: removing all RAPTOR summaries (doc=%s tenant=%s kb=%s)",
            doc_id, tenant_id, kb_id,
        )
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"doc_id": doc_id, "raptor_kwd": ["raptor"]},
            nlp_search.index_name(tenant_id),
            kb_id,
        )
        return 0

    field_map = await get_raptor_chunk_field_map(doc_id, tenant_id, kb_id)
    chunk_ids = collect_raptor_chunk_ids(field_map, exclude_methods={keep_method})
    if not chunk_ids:
        logging.debug(
            "delete_raptor_chunks: no stale RAPTOR chunks to remove (doc=%s tenant=%s kb=%s keep=%s)",
            doc_id, tenant_id, kb_id, keep_method,
        )
        return 0

    logging.info(
        "delete_raptor_chunks: removing %d stale RAPTOR chunks (doc=%s tenant=%s kb=%s keep=%s)",
        len(chunk_ids), doc_id, tenant_id, kb_id, keep_method,
    )
    await thread_pool_exec(
        settings.docStoreConn.delete,
        {"id": list(chunk_ids)},
        nlp_search.index_name(tenant_id),
        kb_id,
    )
    return len(chunk_ids)
