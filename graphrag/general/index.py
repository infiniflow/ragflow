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
import json
import logging
import networkx as nx
import trio

from api import settings
from api.utils import get_uuid
from graphrag.light.graph_extractor import GraphExtractor as LightKGExt
from graphrag.general.graph_extractor import GraphExtractor as GeneralKGExt
from graphrag.general.community_reports_extractor import CommunityReportsExtractor
from graphrag.entity_resolution import EntityResolution
from graphrag.general.extractor import Extractor
from graphrag.utils import (
    graph_merge,
    get_graph,
    set_graph,
    chunk_id,
    does_graph_contains,
    tidy_graph,
    GraphChange,
)
from rag.nlp import rag_tokenizer, search
from rag.utils.redis_conn import RedisDistributedLock


async def run_graphrag(
    row: dict,
    language,
    with_resolution: bool,
    with_community: bool,
    chat_model,
    embedding_model,
    callback,
):
    start = trio.current_time()
    tenant_id, kb_id, doc_id = row["tenant_id"], str(row["kb_id"]), row["doc_id"]
    chunks = []
    for d in settings.retrievaler.chunk_list(
        doc_id, tenant_id, [kb_id], fields=["content_with_weight", "doc_id"]
    ):
        chunks.append(d["content_with_weight"])

    subgraph = await generate_subgraph(
        LightKGExt
        if row["kb_parser_config"]["graphrag"]["method"] != "general"
        else GeneralKGExt,
        tenant_id,
        kb_id,
        doc_id,
        chunks,
        language,
        row["kb_parser_config"]["graphrag"]["entity_types"],
        chat_model,
        embedding_model,
        callback,
    )
    if not subgraph:
        return

    graphrag_task_lock = RedisDistributedLock(f"graphrag_task_{kb_id}", lock_value=doc_id, timeout=1200)
    await graphrag_task_lock.spin_acquire()
    callback(msg=f"run_graphrag {doc_id} graphrag_task_lock acquired")

    try:
        subgraph_nodes = set(subgraph.nodes())
        new_graph = await merge_subgraph(
            tenant_id,
            kb_id,
            doc_id,
            subgraph,
            embedding_model,
            callback,
        )
        assert new_graph is not None

        if not with_resolution or not with_community:
            return

        if with_resolution:
            await graphrag_task_lock.spin_acquire()
            callback(msg=f"run_graphrag {doc_id} graphrag_task_lock acquired")
            await resolve_entities(
                new_graph,
                subgraph_nodes,
                tenant_id,
                kb_id,
                doc_id,
                chat_model,
                embedding_model,
                callback,
            )
        if with_community:
            await graphrag_task_lock.spin_acquire()
            callback(msg=f"run_graphrag {doc_id} graphrag_task_lock acquired")
            await extract_community(
                new_graph,
                tenant_id,
                kb_id,
                doc_id,
                chat_model,
                embedding_model,
                callback,
            )
    finally:
        graphrag_task_lock.release()
    now = trio.current_time()
    callback(msg=f"GraphRAG for doc {doc_id} done in {now - start:.2f} seconds.")
    return


async def generate_subgraph(
    extractor: Extractor,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    chunks: list[str],
    language,
    entity_types,
    llm_bdl,
    embed_bdl,
    callback,
):
    contains = await does_graph_contains(tenant_id, kb_id, doc_id)
    if contains:
        callback(msg=f"Graph already contains {doc_id}")
        return None
    start = trio.current_time()
    ext = extractor(
        llm_bdl,
        language=language,
        entity_types=entity_types,
    )
    ents, rels = await ext(doc_id, chunks, callback)
    subgraph = nx.Graph()
    for ent in ents:
        assert "description" in ent, f"entity {ent} does not have description"
        ent["source_id"] = [doc_id]
        subgraph.add_node(ent["entity_name"], **ent)

    ignored_rels = 0
    for rel in rels:
        assert "description" in rel, f"relation {rel} does not have description"
        if not subgraph.has_node(rel["src_id"]) or not subgraph.has_node(rel["tgt_id"]):
            ignored_rels += 1
            continue
        rel["source_id"] = [doc_id]
        subgraph.add_edge(
            rel["src_id"],
            rel["tgt_id"],
            **rel,
        )
    if ignored_rels:
        callback(msg=f"ignored {ignored_rels} relations due to missing entities.")
    tidy_graph(subgraph, callback)

    subgraph.graph["source_id"] = [doc_id]
    chunk = {
        "content_with_weight": json.dumps(
            nx.node_link_data(subgraph, edges="edges"), ensure_ascii=False
        ),
        "knowledge_graph_kwd": "subgraph",
        "kb_id": kb_id,
        "source_id": [doc_id],
        "available_int": 0,
        "removed_kwd": "N",
    }
    cid = chunk_id(chunk)
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {"knowledge_graph_kwd": "subgraph", "source_id": doc_id}, search.index_name(tenant_id), kb_id
        )
    )
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.insert(
            [{"id": cid, **chunk}], search.index_name(tenant_id), kb_id
        )
    )
    now = trio.current_time()
    callback(msg=f"generated subgraph for doc {doc_id} in {now - start:.2f} seconds.")
    return subgraph

async def merge_subgraph(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    subgraph: nx.Graph,
    embedding_model,
    callback,
):
    start = trio.current_time()
    change = GraphChange()
    old_graph = await get_graph(tenant_id, kb_id)
    if old_graph is not None:
        logging.info("Merge with an exiting graph...................")
        tidy_graph(old_graph, callback)
        new_graph = graph_merge(old_graph, subgraph, change)
    else:
        new_graph = subgraph
        change.added_updated_nodes = set(new_graph.nodes())
        change.added_updated_edges = set(new_graph.edges())
    pr = nx.pagerank(new_graph)
    for node_name, pagerank in pr.items():
        new_graph.nodes[node_name]["pagerank"] = pagerank

    await set_graph(tenant_id, kb_id, embedding_model, new_graph, change, callback)
    now = trio.current_time()
    callback(
        msg=f"merging subgraph for doc {doc_id} into the global graph done in {now - start:.2f} seconds."
    )
    return new_graph


async def resolve_entities(
    graph,
    subgraph_nodes: set[str],
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
):
    start = trio.current_time()
    er = EntityResolution(
        llm_bdl,
    )
    reso = await er(graph, subgraph_nodes, callback=callback)
    graph = reso.graph
    change = reso.change
    callback(msg=f"Graph resolution removed {len(change.removed_nodes)} nodes and {len(change.removed_edges)} edges.")
    callback(msg="Graph resolution updated pagerank.")

    await set_graph(tenant_id, kb_id, embed_bdl, graph, change, callback)
    now = trio.current_time()
    callback(msg=f"Graph resolution done in {now - start:.2f}s.")


async def extract_community(
    graph,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
):
    start = trio.current_time()
    ext = CommunityReportsExtractor(
        llm_bdl,
    )
    cr = await ext(graph, callback=callback)
    community_structure = cr.structured_output
    community_reports = cr.output
    doc_ids = graph.graph["source_id"]

    now = trio.current_time()
    callback(
        msg=f"Graph extracted {len(cr.structured_output)} communities in {now - start:.2f}s."
    )
    start = now
    chunks = []
    for stru, rep in zip(community_structure, community_reports):
        obj = {
            "report": rep,
            "evidences": "\n".join([f.get("explanation", "") for f in stru["findings"]]),
        }
        chunk = {
            "id": get_uuid(),
            "docnm_kwd": stru["title"],
            "title_tks": rag_tokenizer.tokenize(stru["title"]),
            "content_with_weight": json.dumps(obj, ensure_ascii=False),
            "content_ltks": rag_tokenizer.tokenize(
                obj["report"] + " " + obj["evidences"]
            ),
            "knowledge_graph_kwd": "community_report",
            "weight_flt": stru["weight"],
            "entities_kwd": stru["entities"],
            "important_kwd": stru["entities"],
            "kb_id": kb_id,
            "source_id": list(doc_ids),
            "available_int": 0,
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(
            chunk["content_ltks"]
        )
        chunks.append(chunk)

    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {"knowledge_graph_kwd": "community_report", "kb_id": kb_id},
            search.index_name(tenant_id),
            kb_id,
        )
    )
    es_bulk_size = 4
    for b in range(0, len(chunks), es_bulk_size):
        doc_store_result = await trio.to_thread.run_sync(lambda: settings.docStoreConn.insert(chunks[b:b + es_bulk_size], search.index_name(tenant_id), kb_id))
        if doc_store_result:
            error_message = f"Insert chunk error: {doc_store_result}, please check log file and Elasticsearch/Infinity status!"
            raise Exception(error_message)

    now = trio.current_time()
    callback(
        msg=f"Graph indexed {len(cr.structured_output)} communities in {now - start:.2f}s."
    )
    return community_structure, community_reports
