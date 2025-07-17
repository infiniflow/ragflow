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
from api.utils.api_utils import timeout
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
    graph_node_to_chunk,
    graph_edge_to_chunk,
    chat_limiter,
)
from rag.nlp import rag_tokenizer, search
from rag.utils.redis_conn import RedisDistributedLock


@timeout(30, 2)
async def _is_strong_enough(chat_model, embedding_model):
    _ = await trio.to_thread.run_sync(lambda: embedding_model.encode(["Are you strong enough!?"]))
    res =  await trio.to_thread.run_sync(lambda: chat_model.chat("Nothing special.", [{"role":"user", "content": "Are you strong enough!?"}]))
    if res.find("**ERROR**") >= 0:
        raise Exception(res)


async def run_graphrag(
    row: dict,
    language,
    with_resolution: bool,
    with_community: bool,
    chat_model,
    embedding_model,
    callback,
):
    # Pressure test for GraphRAG task
    async with trio.open_nursery() as nursery:
        for _ in range(12):
            nursery.start_soon(_is_strong_enough, chat_model, embedding_model)

    start = trio.current_time()
    tenant_id, kb_id, doc_id = row["tenant_id"], str(row["kb_id"]), row["doc_id"]
    chunks = []
    for d in settings.retrievaler.chunk_list(
        doc_id, tenant_id, [kb_id], fields=["content_with_weight", "doc_id"]
    ):
        chunks.append(d["content_with_weight"])

    subgraph = await generate_subgraph(
        LightKGExt
        if "method" not in row["kb_parser_config"].get("graphrag", {}) or row["kb_parser_config"]["graphrag"]["method"] != "general"
        else GeneralKGExt,
        tenant_id,
        kb_id,
        doc_id,
        chunks,
        language,
        row["kb_parser_config"]["graphrag"].get("entity_types", []),
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

        if not with_resolution and not with_community:
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


@timeout(60*60, 1)
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
    tidy_graph(subgraph, callback, check_attribute=False)

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


@timeout(60*3)
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
    old_graph = await get_graph(tenant_id, kb_id, subgraph.graph["source_id"])
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


@timeout(60*30, 1)
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
    reso = await er(graph, subgraph_nodes, callback=callback, kb_id=kb_id)
    graph = reso.graph
    change = reso.change
    callback(msg=f"Graph resolution removed {len(change.removed_nodes)} nodes and {len(change.removed_edges)} edges.")
    callback(msg="Graph resolution updated pagerank.")

    await set_graph(tenant_id, kb_id, embed_bdl, graph, change, callback)
    now = trio.current_time()
    callback(msg=f"Graph resolution done in {now - start:.2f}s.")


@timeout(60*30, 1)
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
    cr = await ext(graph, callback=callback, kb_id=kb_id)
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


async def extract_entities_only(
    tenant_id: str,
    kb_id: str,
    doc_ids: list[str],  # Process specific documents or all if empty
    language: str,
    entity_types: list[str],
    method: str,  # "light" or "general"
    llm_bdl,
    embed_bdl,
    callback
) -> dict:
    """
    Extract entities and relations from documents without building the full graph.
    Stores entities with knowledge_graph_kwd: "entity" and relations with "relation".
    Uses trio nursery for parallel processing.
    """
    start = trio.current_time()
    
    # Select extractor based on method
    extractor_class = LightKGExt if method == "light" else GeneralKGExt
    
    # Get document chunks to process
    if doc_ids:
        # Process specific documents
        chunks_data = []
        for doc_id in doc_ids:
            for d in settings.retrievaler.chunk_list(
                doc_id, tenant_id, [kb_id], fields=["content_with_weight", "doc_id"]
            ):
                chunks_data.append((doc_id, d["content_with_weight"]))
    else:
        # Process all documents in knowledge base
        chunks_data = []
        # Get all doc_ids for this kb
        doc_conds = {
            "size": 10000,  # Large number to get all docs
            "kb_id": kb_id
        }
        doc_res = await trio.to_thread.run_sync(
            lambda: settings.retrievaler.search(doc_conds, search.index_name(tenant_id), [kb_id])
        )
        processed_docs = set()
        for doc_id in doc_res.ids:
            if doc_id not in processed_docs:
                processed_docs.add(doc_id)
                for d in settings.retrievaler.chunk_list(
                    doc_id, tenant_id, [kb_id], fields=["content_with_weight", "doc_id"]
                ):
                    chunks_data.append((doc_id, d["content_with_weight"]))
    
    if not chunks_data:
        callback(msg="No documents found to process")
        return {
            "total_documents": 0,
            "processed_documents": 0,
            "entities_found": 0,
            "relations_found": 0,
            "status": "completed"
        }
    
    # Group chunks by document
    doc_chunks = {}
    for doc_id, chunk_content in chunks_data:
        if doc_id not in doc_chunks:
            doc_chunks[doc_id] = []
        doc_chunks[doc_id].append(chunk_content)
    
    total_documents = len(doc_chunks)
    callback(msg=f"Starting entity extraction for {total_documents} documents")
    
    # Shared state for results (safe with trio single-threading)
    processed_documents = 0
    total_entities = 0
    total_relations = 0
    results_lock = trio.Lock()
    
    async def extract_entities_for_document(doc_id, chunks):
        nonlocal processed_documents, total_entities, total_relations
        try:
            # Check if entities already exist for this document
            contains = await does_graph_contains(tenant_id, kb_id, doc_id)
            if contains:
                callback(msg=f"Entities already exist for document {doc_id}, skipping")
                async with results_lock:
                    processed_documents += 1
                return
            
            callback(msg=f"Extracting entities from document {doc_id}")
            
            # Extract entities and relations (uses chat_limiter internally)
            ext = extractor_class(
                llm_bdl,
                language=language,
                entity_types=entity_types,
            )
            ents, rels = await ext(doc_id, chunks, callback)
            
            # Store entities
            entity_chunks = []
            for ent in ents:
                assert "description" in ent, f"entity {ent} does not have description"
                ent["source_id"] = [doc_id]
                await graph_node_to_chunk(kb_id, embed_bdl, ent["entity_name"], ent, entity_chunks)
            
            # Store relations
            relation_chunks = []
            for rel in rels:
                assert "description" in rel, f"relation {rel} does not have description"
                rel["source_id"] = [doc_id]
                await graph_edge_to_chunk(kb_id, embed_bdl, rel["src_id"], rel["tgt_id"], rel, relation_chunks)
            
            # Bulk insert entities
            if entity_chunks:
                await trio.to_thread.run_sync(
                    lambda: settings.docStoreConn.insert(
                        entity_chunks, search.index_name(tenant_id), kb_id
                    )
                )
            
            # Bulk insert relations
            if relation_chunks:
                await trio.to_thread.run_sync(
                    lambda: settings.docStoreConn.insert(
                        relation_chunks, search.index_name(tenant_id), kb_id
                    )
                )
            
            # Update shared state thread-safely
            async with results_lock:
                total_entities += len(ents)
                total_relations += len(rels)
                processed_documents += 1
            
            callback(msg=f"Document {doc_id}: extracted {len(ents)} entities, {len(rels)} relations")
            
        except Exception as e:
            callback(msg=f"Error processing document {doc_id}: {str(e)}")
            async with results_lock:
                processed_documents += 1
    
    # Process all documents in parallel using trio nursery
    async with trio.open_nursery() as nursery:
        for doc_id, chunks in doc_chunks.items():
            nursery.start_soon(extract_entities_for_document, doc_id, chunks)
    
    now = trio.current_time()
    callback(msg=f"Entity extraction completed in {now - start:.2f} seconds")
    
    return {
        "total_documents": total_documents,
        "processed_documents": processed_documents,
        "entities_found": total_entities,
        "relations_found": total_relations,
        "status": "completed"
    }


async def build_graph_from_entities(
    tenant_id: str,
    kb_id: str,
    embed_bdl,
    callback
) -> dict:
    """
    Build complete NetworkX graph from pre-extracted entities and relations.
    Calculates PageRank and stores the final graph.
    """
    start = trio.current_time()
    
    callback(msg="Starting graph building from extracted entities")
    
    # Query all entities
    entity_conds = {
        "fields": ["entity_kwd", "content_with_weight", "source_id"],
        "size": 10000,  # Large number to get all entities
        "knowledge_graph_kwd": ["entity"],
        "kb_id": kb_id
    }
    
    entity_res = await trio.to_thread.run_sync(
        lambda: settings.retrievaler.search(entity_conds, search.index_name(tenant_id), [kb_id])
    )
    
    if entity_res.total == 0:
        callback(msg="No entities found to build graph")
        return {
            "total_entities": 0,
            "processed_entities": 0,
            "relationships_created": 0,
            "status": "failed",
            "error": "No entities found"
        }
    
    # Query all relations
    relation_conds = {
        "fields": ["from_entity_kwd", "to_entity_kwd", "content_with_weight", "source_id"],
        "size": 10000,  # Large number to get all relations
        "knowledge_graph_kwd": ["relation"],
        "kb_id": kb_id
    }
    
    relation_res = await trio.to_thread.run_sync(
        lambda: settings.retrievaler.search(relation_conds, search.index_name(tenant_id), [kb_id])
    )
    
    callback(msg=f"Found {entity_res.total} entities and {relation_res.total} relations")
    
    # Build NetworkX graph
    graph = nx.Graph()
    processed_entities = 0
    relationships_created = 0
    all_source_ids = set()
    
    # Add nodes from entities
    for entity_id in entity_res.ids:
        try:
            entity_data = entity_res.field[entity_id]
            entity_name = entity_data["entity_kwd"]
            entity_meta = json.loads(entity_data["content_with_weight"])
            
            # Add source_ids to track document origins
            if "source_id" in entity_data:
                source_ids = entity_data["source_id"]
                if isinstance(source_ids, str):
                    source_ids = [source_ids]
                entity_meta["source_id"] = source_ids
                all_source_ids.update(source_ids)
            
            graph.add_node(entity_name, **entity_meta)
            processed_entities += 1
            
        except Exception as e:
            callback(msg=f"Error processing entity {entity_id}: {str(e)}")
            continue
    
    # Add edges from relations
    for relation_id in relation_res.ids:
        try:
            relation_data = relation_res.field[relation_id]
            from_entity = relation_data["from_entity_kwd"]
            to_entity = relation_data["to_entity_kwd"]
            relation_meta = json.loads(relation_data["content_with_weight"])
            
            # Only add edge if both nodes exist
            if graph.has_node(from_entity) and graph.has_node(to_entity):
                # Add source_ids to track document origins
                if "source_id" in relation_data:
                    source_ids = relation_data["source_id"]
                    if isinstance(source_ids, str):
                        source_ids = [source_ids]
                    relation_meta["source_id"] = source_ids
                    all_source_ids.update(source_ids)
                
                graph.add_edge(from_entity, to_entity, **relation_meta)
                relationships_created += 1
            
        except Exception as e:
            callback(msg=f"Error processing relation {relation_id}: {str(e)}")
            continue
    
    if len(graph.nodes) == 0:
        callback(msg="No valid graph nodes created")
        return {
            "total_entities": entity_res.total,
            "processed_entities": 0,
            "relationships_created": 0,
            "status": "failed",
            "error": "No valid nodes created"
        }
    
    # Set graph metadata
    graph.graph["source_id"] = sorted(list(all_source_ids))
    
    callback(msg=f"Built graph with {len(graph.nodes)} nodes and {len(graph.edges)} edges")
    
    # Calculate PageRank on the complete graph
    callback(msg="Calculating PageRank...")
    pagerank_start = trio.current_time()
    
    pr = nx.pagerank(graph)
    for node_name, pagerank in pr.items():
        graph.nodes[node_name]["pagerank"] = pagerank
    
    pagerank_end = trio.current_time()
    callback(msg=f"PageRank calculation completed in {pagerank_end - pagerank_start:.2f} seconds")
    
    # Store the complete graph
    callback(msg="Storing complete graph...")
    store_start = trio.current_time()
    
    change = GraphChange()
    change.added_updated_nodes = set(graph.nodes())
    change.added_updated_edges = set(graph.edges())
    
    await set_graph(tenant_id, kb_id, embed_bdl, graph, change, callback)
    
    store_end = trio.current_time()
    callback(msg=f"Graph storage completed in {store_end - store_start:.2f} seconds")
    
    now = trio.current_time()
    callback(msg=f"Graph building completed in {now - start:.2f} seconds")
    
    return {
        "total_entities": entity_res.total,
        "processed_entities": processed_entities,
        "relationships_created": relationships_created,
        "total_nodes": len(graph.nodes),
        "total_edges": len(graph.edges),
        "status": "completed"
    }
