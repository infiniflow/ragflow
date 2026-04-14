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

    Args:
        tenant_id: Tenant ID for index name resolution.
        kb_id: Knowledge base ID.
        doc_id: Document ID to look up.

    Returns:
        A networkx Graph if the subgraph was found, or None.
    """
    try:
        fields = ["content_with_weight", "source_id"]
        condition = {
            "knowledge_graph_kwd": ["subgraph"],
            "removed_kwd": "N",
        }
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields, [], condition, [], OrderByExpr(),
            0, 1000, search.index_name(tenant_id), [kb_id]
        )
        field_map = settings.docStoreConn.get_fields(res, fields)
        for cid in field_map:
            source_ids = field_map[cid].get("source_id") or []
            if isinstance(source_ids, str):
                source_ids = [source_ids]
            if doc_id not in source_ids:
                continue
            content = field_map[cid].get("content_with_weight", "")
            if content:
                data = json.loads(content)
                sg = nx.node_link_graph(data, edges="edges")
                sg.graph["source_id"] = [doc_id]
                return sg
    except Exception:
        logging.exception(f"Failed to load subgraph from store for doc {doc_id}")
    return None


def has_raptor_chunks(doc_id: str, tenant_id: str, kb_id: str) -> bool:
    """Check whether a document already has RAPTOR chunks in the doc store.

    RAPTOR chunks are stored with raptor_kwd="raptor". If they exist for a
    given doc_id, the RAPTOR processing can be skipped on resume.

    Args:
        doc_id: Document ID to check.
        tenant_id: Tenant ID.
        kb_id: Knowledge base ID.

    Returns:
        True if RAPTOR chunks exist for the document, False otherwise.
    """
    try:
        existing = list(settings.retriever.chunk_list(
            doc_id, tenant_id, [str(kb_id)],
            fields=["raptor_kwd"], max_count=1,
        ))
        return any(d.get("raptor_kwd") == "raptor" for d in existing)
    except Exception:
        logging.exception(f"Failed to check RAPTOR chunks for doc {doc_id}")
        return False
