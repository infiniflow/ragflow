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
from functools import partial
import networkx as nx
import trio

from api import settings
from graphrag.light.graph_extractor import GraphExtractor as LightKGExt
from graphrag.general.graph_extractor import GraphExtractor as GeneralKGExt
from graphrag.general.community_reports_extractor import CommunityReportsExtractor
from graphrag.entity_resolution import EntityResolution
from graphrag.general.extractor import Extractor
from graphrag.utils import (
    graph_merge,
    set_entity,
    get_relation,
    set_relation,
    get_entity,
    get_graph,
    set_graph,
    chunk_id,
    update_nodes_pagerank_nhop_neighbour,
    does_graph_contains,
    get_graph_doc_ids,
)
from rag.nlp import rag_tokenizer, search
from rag.utils.redis_conn import REDIS_CONN


def graphrag_task_set(tenant_id, kb_id, doc_id) -> bool:
    key = f"graphrag:{tenant_id}:{kb_id}"
    ok = REDIS_CONN.set(key, doc_id, exp=3600 * 24)
    if not ok:
        raise Exception(f"Faild to set the {key} to {doc_id}")


def graphrag_task_get(tenant_id, kb_id) -> str | None:
    key = f"graphrag:{tenant_id}:{kb_id}"
    doc_id = REDIS_CONN.get(key)
    return doc_id


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

    graph, doc_ids = await update_graph(
        LightKGExt
        if row["parser_config"]["graphrag"]["method"] != "general"
        else GeneralKGExt,
        tenant_id,
        kb_id,
        doc_id,
        chunks,
        language,
        row["parser_config"]["graphrag"]["entity_types"],
        chat_model,
        embedding_model,
        callback,
    )
    if not graph:
        return
    if with_resolution or with_community:
        graphrag_task_set(tenant_id, kb_id, doc_id)
    if with_resolution:
        await resolve_entities(
            graph,
            doc_ids,
            tenant_id,
            kb_id,
            doc_id,
            chat_model,
            embedding_model,
            callback,
        )
    if with_community:
        await extract_community(
            graph,
            doc_ids,
            tenant_id,
            kb_id,
            doc_id,
            chat_model,
            embedding_model,
            callback,
        )
    now = trio.current_time()
    callback(msg=f"GraphRAG for doc {doc_id} done in {now - start:.2f} seconds.")
    return


async def update_graph(
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
        callback(msg=f"Graph already contains {doc_id}, cancel myself")
        return None, None
    start = trio.current_time()
    ext = extractor(
        llm_bdl,
        language=language,
        entity_types=entity_types,
        get_entity=partial(get_entity, tenant_id, kb_id),
        set_entity=partial(set_entity, tenant_id, kb_id, embed_bdl),
        get_relation=partial(get_relation, tenant_id, kb_id),
        set_relation=partial(set_relation, tenant_id, kb_id, embed_bdl),
    )
    ents, rels = await ext(doc_id, chunks, callback)
    subgraph = nx.Graph()
    for en in ents:
        subgraph.add_node(en["entity_name"], entity_type=en["entity_type"])

    for rel in rels:
        subgraph.add_edge(
            rel["src_id"],
            rel["tgt_id"],
            weight=rel["weight"],
            # description=rel["description"]
        )
    # TODO: infinity doesn't support array search
    chunk = {
        "content_with_weight": json.dumps(
            nx.node_link_data(subgraph, edges="edges"), ensure_ascii=False, indent=2
        ),
        "knowledge_graph_kwd": "subgraph",
        "kb_id": kb_id,
        "source_id": [doc_id],
        "available_int": 0,
        "removed_kwd": "N",
    }
    cid = chunk_id(chunk)
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.insert(
            [{"id": cid, **chunk}], search.index_name(tenant_id), kb_id
        )
    )
    now = trio.current_time()
    callback(msg=f"generated subgraph for doc {doc_id} in {now - start:.2f} seconds.")
    start = now

    while True:
        new_graph = subgraph
        now_docids = set([doc_id])
        old_graph, old_doc_ids = await get_graph(tenant_id, kb_id)
        if old_graph is not None:
            logging.info("Merge with an exiting graph...................")
            new_graph = graph_merge(old_graph, subgraph)
        await update_nodes_pagerank_nhop_neighbour(tenant_id, kb_id, new_graph, 2)
        if old_doc_ids:
            for old_doc_id in old_doc_ids:
                now_docids.add(old_doc_id)
        old_doc_ids2 = await get_graph_doc_ids(tenant_id, kb_id)
        delta_doc_ids = set(old_doc_ids2) - set(old_doc_ids)
        if delta_doc_ids:
            callback(
                msg="The global graph has changed during merging, try again"
            )
            await trio.sleep(1)
            continue
        break
    await set_graph(tenant_id, kb_id, new_graph, list(now_docids))
    now = trio.current_time()
    callback(
        msg=f"merging subgraph for doc {doc_id} into the global graph done in {now - start:.2f} seconds."
    )
    return new_graph, now_docids


async def resolve_entities(
    graph,
    doc_ids,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
):
    working_doc_id = graphrag_task_get(tenant_id, kb_id)
    if doc_id != working_doc_id:
        callback(
            msg=f"Another graphrag task of doc_id {working_doc_id} is working on this kb, cancel myself"
        )
        return
    start = trio.current_time()
    er = EntityResolution(
        llm_bdl,
        get_entity=partial(get_entity, tenant_id, kb_id),
        set_entity=partial(set_entity, tenant_id, kb_id, embed_bdl),
        get_relation=partial(get_relation, tenant_id, kb_id),
        set_relation=partial(set_relation, tenant_id, kb_id, embed_bdl),
    )
    reso = await er(graph, callback=callback)
    graph = reso.graph
    callback(msg=f"Graph resolution removed {len(reso.removed_entities)} nodes.")
    await update_nodes_pagerank_nhop_neighbour(tenant_id, kb_id, graph, 2)
    callback(msg="Graph resolution updated pagerank.")

    working_doc_id = graphrag_task_get(tenant_id, kb_id)
    if doc_id != working_doc_id:
        callback(
            msg=f"Another graphrag task of doc_id {working_doc_id} is working on this kb, cancel myself"
        )
        return
    await set_graph(tenant_id, kb_id, graph, doc_ids)

    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {
                "knowledge_graph_kwd": "relation",
                "kb_id": kb_id,
                "from_entity_kwd": reso.removed_entities,
            },
            search.index_name(tenant_id),
            kb_id,
        )
    )
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {
                "knowledge_graph_kwd": "relation",
                "kb_id": kb_id,
                "to_entity_kwd": reso.removed_entities,
            },
            search.index_name(tenant_id),
            kb_id,
        )
    )
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {
                "knowledge_graph_kwd": "entity",
                "kb_id": kb_id,
                "entity_kwd": reso.removed_entities,
            },
            search.index_name(tenant_id),
            kb_id,
        )
    )
    now = trio.current_time()
    callback(msg=f"Graph resolution done in {now - start:.2f}s.")


async def extract_community(
    graph,
    doc_ids,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
):
    working_doc_id = graphrag_task_get(tenant_id, kb_id)
    if doc_id != working_doc_id:
        callback(
            msg=f"Another graphrag task of doc_id {working_doc_id} is working on this kb, cancel myself"
        )
        return
    start = trio.current_time()
    ext = CommunityReportsExtractor(
        llm_bdl,
        get_entity=partial(get_entity, tenant_id, kb_id),
        set_entity=partial(set_entity, tenant_id, kb_id, embed_bdl),
        get_relation=partial(get_relation, tenant_id, kb_id),
        set_relation=partial(set_relation, tenant_id, kb_id, embed_bdl),
    )
    cr = await ext(graph, callback=callback)
    community_structure = cr.structured_output
    community_reports = cr.output
    working_doc_id = graphrag_task_get(tenant_id, kb_id)
    if doc_id != working_doc_id:
        callback(
            msg=f"Another graphrag task of doc_id {working_doc_id} is working on this kb, cancel myself"
        )
        return
    await set_graph(tenant_id, kb_id, graph, doc_ids)

    now = trio.current_time()
    callback(
        msg=f"Graph extracted {len(cr.structured_output)} communities in {now - start:.2f}s."
    )
    start = now
    await trio.to_thread.run_sync(
        lambda: settings.docStoreConn.delete(
            {"knowledge_graph_kwd": "community_report", "kb_id": kb_id},
            search.index_name(tenant_id),
            kb_id,
        )
    )
    for stru, rep in zip(community_structure, community_reports):
        obj = {
            "report": rep,
            "evidences": "\n".join([f["explanation"] for f in stru["findings"]]),
        }
        chunk = {
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
            "source_id": doc_ids,
            "available_int": 0,
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(
            chunk["content_ltks"]
        )
        # try:
        #    ebd, _ = embed_bdl.encode([", ".join(community["entities"])])
        #    chunk["q_%d_vec" % len(ebd[0])] = ebd[0]
        # except Exception as e:
        #    logging.exception(f"Fail to embed entity relation: {e}")
        await trio.to_thread.run_sync(
            lambda: settings.docStoreConn.insert(
                [{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id)
            )
        )

    now = trio.current_time()
    callback(
        msg=f"Graph indexed {len(cr.structured_output)} communities in {now - start:.2f}s."
    )
    return community_structure, community_reports
