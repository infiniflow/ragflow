# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
 - [LightRag](https://github.com/HKUDS/LightRAG)
"""

import html
import json
import logging
import re
import time
from collections import defaultdict
from hashlib import md5
from typing import Any, Callable
import os
import trio
from typing import Set, Tuple

import networkx as nx
import numpy as np
import xxhash
from networkx.readwrite import json_graph
import dataclasses

from api import settings
from api.utils import get_uuid
from rag.nlp import search, rag_tokenizer
from rag.utils.doc_store_conn import OrderByExpr
from rag.utils.redis_conn import REDIS_CONN

GRAPH_FIELD_SEP = "<SEP>"

ErrorHandlerFn = Callable[[BaseException | None, str | None, dict | None], None]

chat_limiter = trio.CapacityLimiter(int(os.environ.get('MAX_CONCURRENT_CHATS', 10)))

@dataclasses.dataclass
class GraphChange:
    removed_nodes: Set[str] = dataclasses.field(default_factory=set)
    added_updated_nodes: Set[str] = dataclasses.field(default_factory=set)
    removed_edges: Set[Tuple[str, str]] = dataclasses.field(default_factory=set)
    added_updated_edges: Set[Tuple[str, str]] = dataclasses.field(default_factory=set)

def perform_variable_replacements(
    input: str, history: list[dict] | None = None, variables: dict | None = None
) -> str:
    """Perform variable replacements on the input string and in a chat log."""
    if history is None:
        history = []
    if variables is None:
        variables = {}
    result = input

    def replace_all(input: str) -> str:
        result = input
        for k, v in variables.items():
            result = result.replace(f"{{{k}}}", v)
        return result

    result = replace_all(result)
    for i, entry in enumerate(history):
        if entry.get("role") == "system":
            entry["content"] = replace_all(entry.get("content") or "")

    return result


def clean_str(input: Any) -> str:
    """Clean an input string by removing HTML escapes, control characters, and other unwanted characters."""
    # If we get non-string input, just give it back
    if not isinstance(input, str):
        return input

    result = html.unescape(input.strip())
    # https://stackoverflow.com/questions/4324790/removing-control-characters-from-a-string-in-python
    return re.sub(r"[\"\x00-\x1f\x7f-\x9f]", "", result)


def dict_has_keys_with_types(
    data: dict, expected_fields: list[tuple[str, type]]
) -> bool:
    """Return True if the given dictionary has the given keys with the given types."""
    for field, field_type in expected_fields:
        if field not in data:
            return False

        value = data[field]
        if not isinstance(value, field_type):
            return False
    return True


def get_llm_cache(llmnm, txt, history, genconf):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))
    hasher.update(str(history).encode("utf-8"))
    hasher.update(str(genconf).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return bin


def set_llm_cache(llmnm, txt, v, history, genconf):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))
    hasher.update(str(history).encode("utf-8"))
    hasher.update(str(genconf).encode("utf-8"))

    k = hasher.hexdigest()
    REDIS_CONN.set(k, v.encode("utf-8"), 24*3600)


def get_embed_cache(llmnm, txt):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return np.array(json.loads(bin))


def set_embed_cache(llmnm, txt, arr):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))

    k = hasher.hexdigest()
    arr = json.dumps(arr.tolist() if isinstance(arr, np.ndarray) else arr)
    REDIS_CONN.set(k, arr.encode("utf-8"), 24*3600)


def get_tags_from_cache(kb_ids):
    hasher = xxhash.xxh64()
    hasher.update(str(kb_ids).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return bin


def set_tags_to_cache(kb_ids, tags):
    hasher = xxhash.xxh64()
    hasher.update(str(kb_ids).encode("utf-8"))

    k = hasher.hexdigest()
    REDIS_CONN.set(k, json.dumps(tags).encode("utf-8"), 600)

def tidy_graph(graph: nx.Graph, callback):
    """
    Ensure all nodes and edges in the graph have some essential attribute.
    """
    def is_valid_node(node_attrs: dict) -> bool:
        valid_node = True
        for attr in ["description", "source_id"]:
            if attr not in node_attrs:
                valid_node = False
                break
        return valid_node
    purged_nodes = []
    for node, node_attrs in graph.nodes(data=True):
        if not is_valid_node(node_attrs):
            purged_nodes.append(node)
    for node in purged_nodes:
        graph.remove_node(node)
    if purged_nodes and callback:
        callback(msg=f"Purged {len(purged_nodes)} nodes from graph due to missing essential attributes.")

    purged_edges = []
    for source, target, attr in graph.edges(data=True):
        if not is_valid_node(attr):
            purged_edges.append((source, target))
        if "keywords" not in attr:
            attr["keywords"] = []
    for source, target in purged_edges:
        graph.remove_edge(source, target)
    if purged_edges and callback:
        callback(msg=f"Purged {len(purged_edges)} edges from graph due to missing essential attributes.")

def get_from_to(node1, node2):
    if node1 < node2:
        return (node1, node2)
    else:
        return (node2, node1)

def graph_merge(g1: nx.Graph, g2: nx.Graph, change: GraphChange):
    """Merge graph g2 into g1 in place."""
    for node_name, attr in g2.nodes(data=True):
        change.added_updated_nodes.add(node_name)
        if not g1.has_node(node_name):
            g1.add_node(node_name, **attr)
            continue
        node = g1.nodes[node_name]
        node["description"] += GRAPH_FIELD_SEP + attr["description"]
        # A node's source_id indicates which chunks it came from.
        node["source_id"] += attr["source_id"]

    for source, target, attr in g2.edges(data=True):
        change.added_updated_edges.add(get_from_to(source, target))
        edge = g1.get_edge_data(source, target)
        if edge is None:
            g1.add_edge(source, target, **attr)
            continue
        edge["weight"] += attr.get("weight", 0)
        edge["description"] += GRAPH_FIELD_SEP + attr["description"]
        edge["keywords"] += attr["keywords"]
        # A edge's source_id indicates which chunks it came from.
        edge["source_id"] += attr["source_id"]

    for node_degree in g1.degree:
        g1.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])
    # A graph's source_id indicates which documents it came from.
    if "source_id" not in g1.graph:
        g1.graph["source_id"] = []
    g1.graph["source_id"] += g2.graph.get("source_id", [])
    return g1

def compute_args_hash(*args):
    return md5(str(args).encode()).hexdigest()


def handle_single_entity_extraction(
    record_attributes: list[str],
    chunk_key: str,
):
    if len(record_attributes) < 4 or record_attributes[0] != '"entity"':
        return None
    # add this record as a node in the G
    entity_name = clean_str(record_attributes[1].upper())
    if not entity_name.strip():
        return None
    entity_type = clean_str(record_attributes[2].upper())
    entity_description = clean_str(record_attributes[3])
    entity_source_id = chunk_key
    return dict(
        entity_name=entity_name.upper(),
        entity_type=entity_type.upper(),
        description=entity_description,
        source_id=entity_source_id,
    )


def handle_single_relationship_extraction(record_attributes: list[str], chunk_key: str):
    if len(record_attributes) < 5 or record_attributes[0] != '"relationship"':
        return None
    # add this record as edge
    source = clean_str(record_attributes[1].upper())
    target = clean_str(record_attributes[2].upper())
    edge_description = clean_str(record_attributes[3])

    edge_keywords = clean_str(record_attributes[4])
    edge_source_id = chunk_key
    weight = (
        float(record_attributes[-1]) if is_float_regex(record_attributes[-1]) else 1.0
    )
    pair = sorted([source.upper(), target.upper()])
    return dict(
        src_id=pair[0],
        tgt_id=pair[1],
        weight=weight,
        description=edge_description,
        keywords=edge_keywords,
        source_id=edge_source_id,
        metadata={"created_at": time.time()},
    )


def pack_user_ass_to_openai_messages(*args: str):
    roles = ["user", "assistant"]
    return [
        {"role": roles[i % 2], "content": content} for i, content in enumerate(args)
    ]


def split_string_by_multi_markers(content: str, markers: list[str]) -> list[str]:
    """Split a string by multiple markers"""
    if not markers:
        return [content]
    results = re.split("|".join(re.escape(marker) for marker in markers), content)
    return [r.strip() for r in results if r.strip()]


def is_float_regex(value):
    return bool(re.match(r"^[-+]?[0-9]*\.?[0-9]+$", value))


def chunk_id(chunk):
    return xxhash.xxh64((chunk["content_with_weight"] + chunk["kb_id"]).encode("utf-8")).hexdigest()


async def graph_node_to_chunk(kb_id, embd_mdl, ent_name, meta, chunks):
    chunk = {
        "id": get_uuid(),
        "important_kwd": [ent_name],
        "title_tks": rag_tokenizer.tokenize(ent_name),
        "entity_kwd": ent_name,
        "knowledge_graph_kwd": "entity",
        "entity_type_kwd": meta["entity_type"],
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "source_id": meta["source_id"],
        "kb_id": kb_id,
        "available_int": 0
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    ebd = get_embed_cache(embd_mdl.llm_name, ent_name)
    if ebd is None:
        ebd, _ = await trio.to_thread.run_sync(lambda: embd_mdl.encode([ent_name]))
        ebd = ebd[0]
        set_embed_cache(embd_mdl.llm_name, ent_name, ebd)
    assert ebd is not None
    chunk["q_%d_vec" % len(ebd)] = ebd
    chunks.append(chunk)


def get_relation(tenant_id, kb_id, from_ent_name, to_ent_name, size=1):
    ents = from_ent_name
    if isinstance(ents, str):
        ents = [from_ent_name]
    if isinstance(to_ent_name, str):
        to_ent_name = [to_ent_name]
    ents.extend(to_ent_name)
    ents = list(set(ents))
    conds = {
        "fields": ["content_with_weight"],
        "size": size,
        "from_entity_kwd": ents,
        "to_entity_kwd": ents,
        "knowledge_graph_kwd": ["relation"]
    }
    res = []
    es_res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id] if isinstance(kb_id, str) else kb_id)
    for id in es_res.ids:
        try:
            if size == 1:
                return json.loads(es_res.field[id]["content_with_weight"])
            res.append(json.loads(es_res.field[id]["content_with_weight"]))
        except Exception:
            continue
    return res


async def graph_edge_to_chunk(kb_id, embd_mdl, from_ent_name, to_ent_name, meta, chunks):
    chunk = {
        "id": get_uuid(),
        "from_entity_kwd": from_ent_name,
        "to_entity_kwd": to_ent_name,
        "knowledge_graph_kwd": "relation",
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "important_kwd": meta["keywords"],
        "source_id": meta["source_id"],
        "weight_int": int(meta["weight"]),
        "kb_id": kb_id,
        "available_int": 0
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    txt = f"{from_ent_name}->{to_ent_name}"
    ebd = get_embed_cache(embd_mdl.llm_name, txt)
    if ebd is None:
        ebd, _ = await trio.to_thread.run_sync(lambda: embd_mdl.encode([txt+f": {meta['description']}"]))
        ebd = ebd[0]
        set_embed_cache(embd_mdl.llm_name, txt, ebd)
    assert ebd is not None
    chunk["q_%d_vec" % len(ebd)] = ebd
    chunks.append(chunk)

async def does_graph_contains(tenant_id, kb_id, doc_id):
    # Get doc_ids of graph
    fields = ["source_id"]
    condition = {
        "knowledge_graph_kwd": ["graph"],
        "removed_kwd": "N",
    }
    res = await trio.to_thread.run_sync(lambda: settings.docStoreConn.search(fields, [], condition, [], OrderByExpr(), 0, 1, search.index_name(tenant_id), [kb_id]))
    fields2 = settings.docStoreConn.getFields(res, fields)
    graph_doc_ids = set()
    for chunk_id in fields2.keys():
        graph_doc_ids = set(fields2[chunk_id]["source_id"])
    return doc_id in graph_doc_ids

async def get_graph_doc_ids(tenant_id, kb_id) -> list[str]:
    conds = {
        "fields": ["source_id"],
        "removed_kwd": "N",
        "size": 1,
        "knowledge_graph_kwd": ["graph"]
    }
    res = await trio.to_thread.run_sync(lambda: settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id]))
    doc_ids = []
    if res.total == 0:
        return doc_ids
    for id in res.ids:
        doc_ids = res.field[id]["source_id"]
    return doc_ids


async def get_graph(tenant_id, kb_id):
    conds = {
        "fields": ["content_with_weight", "source_id"],
        "removed_kwd": "N",
        "size": 1,
        "knowledge_graph_kwd": ["graph"]
    }
    res = await trio.to_thread.run_sync(lambda: settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id]))
    if res.total == 0:
        return None
    for id in res.ids:
        try:
            g = json_graph.node_link_graph(json.loads(res.field[id]["content_with_weight"]), edges="edges")
            if "source_id" not in g.graph:
                g.graph["source_id"] = res.field[id]["source_id"]
            return g
        except Exception:
            continue
    result = await rebuild_graph(tenant_id, kb_id)
    return result


async def set_graph(tenant_id: str, kb_id: str, embd_mdl, graph: nx.Graph, change: GraphChange, callback):
    start = trio.current_time()

    await trio.to_thread.run_sync(lambda: settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph"]}, search.index_name(tenant_id), kb_id))

    if change.removed_nodes:
        await trio.to_thread.run_sync(lambda: settings.docStoreConn.delete({"knowledge_graph_kwd": ["entity"], "entity_kwd": sorted(change.removed_nodes)}, search.index_name(tenant_id), kb_id))

    if change.removed_edges:
        async with trio.open_nursery() as nursery:
            for from_node, to_node in change.removed_edges:
                nursery.start_soon(lambda: settings.docStoreConn.delete({"knowledge_graph_kwd": ["relation"], "from_entity_kwd": from_node, "to_entity_kwd": to_node}, search.index_name(tenant_id), kb_id))
    now = trio.current_time()
    if callback:
        callback(msg=f"set_graph removed {len(change.removed_nodes)} nodes and {len(change.removed_edges)} edges from index in {now - start:.2f}s.")
    start = now

    chunks = [{
        "id": get_uuid(),
        "content_with_weight": json.dumps(nx.node_link_data(graph, edges="edges"), ensure_ascii=False),
        "knowledge_graph_kwd": "graph",
        "kb_id": kb_id,
        "source_id": graph.graph.get("source_id", []),
        "available_int": 0,
        "removed_kwd": "N"
    }]
    async with trio.open_nursery() as nursery:
        for node in change.added_updated_nodes:
            node_attrs = graph.nodes[node]
            nursery.start_soon(lambda: graph_node_to_chunk(kb_id, embd_mdl, node, node_attrs, chunks))
        for from_node, to_node in change.added_updated_edges:
            edge_attrs = graph.edges[from_node, to_node]
            nursery.start_soon(lambda: graph_edge_to_chunk(kb_id, embd_mdl, from_node, to_node, edge_attrs, chunks))
    now = trio.current_time()
    if callback:
        callback(msg=f"set_graph converted graph change to {len(chunks)} chunks in {now - start:.2f}s.")
    start = now

    es_bulk_size = 4
    for b in range(0, len(chunks), es_bulk_size):
        doc_store_result = await trio.to_thread.run_sync(lambda: settings.docStoreConn.insert(chunks[b:b + es_bulk_size], search.index_name(tenant_id), kb_id))
        if doc_store_result:
            error_message = f"Insert chunk error: {doc_store_result}, please check log file and Elasticsearch/Infinity status!"
            raise Exception(error_message)
    now = trio.current_time()
    if callback:
        callback(msg=f"set_graph added/updated {len(change.added_updated_nodes)} nodes and {len(change.added_updated_edges)} edges from index in {now - start:.2f}s.")


def is_continuous_subsequence(subseq, seq):
    def find_all_indexes(tup, value):
        indexes = []
        start = 0
        while True:
            try:
                index = tup.index(value, start)
                indexes.append(index)
                start = index + 1
            except ValueError:
                break
        return indexes

    index_list = find_all_indexes(seq,subseq[0])
    for idx in index_list:
        if idx!=len(seq)-1:
            if seq[idx+1]==subseq[-1]:
                return True
    return False


def merge_tuples(list1, list2):
    result = []
    for tup in list1:
        last_element = tup[-1]
        if last_element in tup[:-1]:
            result.append(tup)
        else:
            matching_tuples = [t for t in list2 if t[0] == last_element]
            already_match_flag = 0
            for match in matching_tuples:
                matchh = (match[1], match[0])
                if is_continuous_subsequence(match, tup) or is_continuous_subsequence(matchh, tup):
                    continue
                already_match_flag = 1
                merged_tuple = tup + match[1:]
                result.append(merged_tuple)
            if not already_match_flag:
                result.append(tup)
    return result


async def get_entity_type2sampels(idxnms, kb_ids: list):
    es_res = await trio.to_thread.run_sync(lambda: settings.retrievaler.search({"knowledge_graph_kwd": "ty2ents", "kb_id": kb_ids,
                                       "size": 10000,
                                       "fields": ["content_with_weight"]},
                                      idxnms, kb_ids))

    res = defaultdict(list)
    for id in es_res.ids:
        smp = es_res.field[id].get("content_with_weight")
        if not smp:
            continue
        try:
            smp = json.loads(smp)
        except Exception as e:
            logging.exception(e)

        for ty, ents in smp.items():
            res[ty].extend(ents)
    return res


def flat_uniq_list(arr, key):
    res = []
    for a in arr:
        a = a[key]
        if isinstance(a, list):
            res.extend(a)
        else:
            res.append(a)
    return list(set(res))


async def rebuild_graph(tenant_id, kb_id):
    graph = nx.Graph()
    src_ids = set()
    flds = ["entity_kwd", "from_entity_kwd", "to_entity_kwd", "knowledge_graph_kwd", "content_with_weight", "source_id"]
    bs = 256
    for i in range(0, 1024*bs, bs):
        es_res = await trio.to_thread.run_sync(lambda: settings.docStoreConn.search(flds, [],
                                 {"kb_id": kb_id, "knowledge_graph_kwd": ["entity"]},
                                 [],
                                 OrderByExpr(),
                                 i, bs, search.index_name(tenant_id), [kb_id]
                                 ))
        tot = settings.docStoreConn.getTotal(es_res)
        if tot == 0:
            break

        es_res = settings.docStoreConn.getFields(es_res, flds)
        for id, d in es_res.items():
            assert d["knowledge_graph_kwd"] == "relation"
            src_ids.update(d.get("source_id", []))
            attrs = json.load(d["content_with_weight"])
            graph.add_node(d["entity_kwd"], **attrs)

    for i in range(0, 1024*bs, bs):
        es_res = await trio.to_thread.run_sync(lambda: settings.docStoreConn.search(flds, [],
                                 {"kb_id": kb_id, "knowledge_graph_kwd": ["relation"]},
                                 [],
                                 OrderByExpr(),
                                 i, bs, search.index_name(tenant_id), [kb_id]
                                 ))
        tot = settings.docStoreConn.getTotal(es_res)
        if tot == 0:
            return None

        es_res = settings.docStoreConn.getFields(es_res, flds)
        for id, d in es_res.items():
            assert d["knowledge_graph_kwd"] == "relation"
            src_ids.update(d.get("source_id", []))
            if graph.has_node(d["from_entity_kwd"]) and graph.has_node(d["to_entity_kwd"]):
                attrs = json.load(d["content_with_weight"])
                graph.add_edge(d["from_entity_kwd"], d["to_entity_kwd"], **attrs)

    src_ids = sorted(src_ids)
    graph.graph["source_id"] = src_ids
    return graph
