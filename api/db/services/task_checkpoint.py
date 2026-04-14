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
# CHECKPOINT: Checkpoint helpers for long-running tasks (GraphRAG, RAPTOR).
# Uses docEngine queries to detect previously completed work, enabling resume
# after crash/interruption without re-running expensive LLM extraction.

import json
import logging

import networkx as nx

from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.misc_utils import thread_pool_exec
from rag.nlp import search

_PAGE_SIZE = 256


async def load_subgraph_from_store(tenant_id: str, kb_id: str, doc_id: str):
    """Load a previously saved subgraph from the doc store.

    When generate_subgraph() completes for a document, it persists the subgraph
    to the doc store with knowledge_graph_kwd="subgraph". This function queries
    for that saved subgraph so that on resume, the expensive LLM entity/relation
    extraction can be skipped.

    Follows the same query pattern as does_graph_contains() in rag/graphrag/utils.py:
    filter by knowledge_graph_kwd only, then match source_id in Python. This avoids
    ambiguity in how the doc store backend (Elasticsearch / Infinity / OceanBase)
    handles list-valued fields in query conditions.

    Paginates through all stored subgraphs so that KBs with more than one page of
    subgraphs do not silently miss a match.

    Args:
        tenant_id: Tenant ID for index name resolution.
        kb_id: Knowledge base ID.
        doc_id: Document ID to look up.

    Returns:
        A networkx Graph if the subgraph was found, or None.
    """
    fields = ["content_with_weight", "source_id"]
    condition = {
        "knowledge_graph_kwd": ["subgraph"],
        "removed_kwd": "N",
    }
    offset = 0
    candidates_scanned = 0
    try:
        while True:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields, [], condition, [], OrderByExpr(),
                offset, _PAGE_SIZE, search.index_name(tenant_id), [kb_id]
            )
            field_map = settings.docStoreConn.get_fields(res, fields)
            if not field_map:
                break
            candidates_scanned += len(field_map)
            for cid, row in field_map.items():
                source_ids = row.get("source_id") or []
                if isinstance(source_ids, str):
                    source_ids = [source_ids]
                if doc_id not in source_ids:
                    continue
                content = row.get("content_with_weight", "")
                if not content:
                    continue
                try:
                    data = json.loads(content)
                    sg = nx.node_link_graph(data, edges="edges")
                    sg.graph["source_id"] = [doc_id]
                    logging.info(
                        "Checkpoint hit: subgraph for doc %s (tenant=%s kb=%s) found at chunk %s",
                        doc_id, tenant_id, kb_id, cid,
                    )
                    return sg
                except Exception:
                    logging.exception(
                        "Failed to parse subgraph JSON for doc %s chunk %s", doc_id, cid
                    )
                    continue
            if len(field_map) < _PAGE_SIZE:
                break
            offset += _PAGE_SIZE
    except Exception:
        logging.exception("Failed to load subgraph from store for doc %s", doc_id)
        return None
    logging.info(
        "Checkpoint miss: no subgraph for doc %s (tenant=%s kb=%s, scanned=%d candidates)",
        doc_id, tenant_id, kb_id, candidates_scanned,
    )
    return None


def has_raptor_chunks(doc_id: str, tenant_id: str, kb_id: str) -> bool:
    """Check whether a document already has RAPTOR chunks in the doc store.

    RAPTOR chunks are stored with raptor_kwd="raptor". If they exist for a
    given doc_id, the RAPTOR processing can be skipped on resume.

    Queries directly for raptor_kwd="raptor" rows to avoid false negatives
    when the first stored chunk for the document is a regular (non-RAPTOR) chunk.

    Args:
        doc_id: Document ID to check.
        tenant_id: Tenant ID.
        kb_id: Knowledge base ID.

    Returns:
        True if RAPTOR chunks exist for the document, False otherwise.
    """
    try:
        condition = {
            "doc_id": doc_id,
            "raptor_kwd": ["raptor"],
        }
        res = settings.docStoreConn.search(
            ["raptor_kwd"], [], condition, [], OrderByExpr(),
            0, 1, search.index_name(tenant_id), [kb_id]
        )
        field_map = settings.docStoreConn.get_fields(res, ["raptor_kwd"])
        found = bool(field_map)
        if found:
            logging.info(
                "Checkpoint hit: RAPTOR chunks for doc %s (tenant=%s kb=%s) already exist",
                doc_id, tenant_id, kb_id,
            )
        else:
            logging.info(
                "Checkpoint miss: no RAPTOR chunks for doc %s (tenant=%s kb=%s)",
                doc_id, tenant_id, kb_id,
            )
        return found
    except Exception:
        logging.exception("Failed to check RAPTOR chunks for doc %s", doc_id)
        return False
