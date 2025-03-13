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
from copy import deepcopy
from hashlib import md5
from typing import Any, Callable
import os
import trio

import networkx as nx
import numpy as np
import xxhash
from networkx.readwrite import json_graph

from api import settings
from rag.nlp import search, rag_tokenizer
from rag.utils.doc_store_conn import OrderByExpr
from rag.utils.redis_conn import REDIS_CONN

ErrorHandlerFn = Callable[[BaseException | None, str | None, dict | None], None]

chat_limiter = trio.CapacityLimiter(int(os.environ.get('MAX_CONCURRENT_CHATS', 10)))

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


def graph_merge(g1, g2):
    g = g2.copy()
    for n, attr in g1.nodes(data=True):
        if n not in g2.nodes():
            g.add_node(n, **attr)
            continue

    for source, target, attr in g1.edges(data=True):
        if g.has_edge(source, target):
            g[source][target].update({"weight": attr.get("weight", 0)+1})
            continue
        g.add_edge(source, target)#, **attr)

    for node_degree in g.degree:
        g.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])
    return g


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

def get_entity_cache(tenant_id, kb_id, ent_name) -> str | list[str]:
    hasher = xxhash.xxh64()
    hasher.update(str(tenant_id).encode("utf-8"))
    hasher.update(str(kb_id).encode("utf-8"))
    hasher.update(str(ent_name).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return json.loads(bin)


def set_entity_cache(tenant_id, kb_id, ent_name, content_with_weight):
    hasher = xxhash.xxh64()
    hasher.update(str(tenant_id).encode("utf-8"))
    hasher.update(str(kb_id).encode("utf-8"))
    hasher.update(str(ent_name).encode("utf-8"))

    k = hasher.hexdigest()
    REDIS_CONN.set(k, content_with_weight.encode("utf-8"), 3600)


def get_entity(tenant_id, kb_id, ent_name):
    cache = get_entity_cache(tenant_id, kb_id, ent_name)
    if cache:
        return cache
    conds = {
        "fields": ["content_with_weight"],
        "entity_kwd": ent_name,
        "size": 10000,
        "knowledge_graph_kwd": ["entity"]
    }
    res = []
    es_res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id])
    for id in es_res.ids:
        try:
            if isinstance(ent_name, str):
                set_entity_cache(tenant_id, kb_id, ent_name, es_res.field[id]["content_with_weight"])
                return json.loads(es_res.field[id]["content_with_weight"])
            res.append(json.loads(es_res.field[id]["content_with_weight"]))
        except Exception:
            continue

    return res


def set_entity(tenant_id, kb_id, embd_mdl, ent_name, meta):
    chunk = {
        "important_kwd": [ent_name],
        "title_tks": rag_tokenizer.tokenize(ent_name),
        "entity_kwd": ent_name,
        "knowledge_graph_kwd": "entity",
        "entity_type_kwd": meta["entity_type"],
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "source_id": list(set(meta["source_id"])),
        "kb_id": kb_id,
        "available_int": 0
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    set_entity_cache(tenant_id, kb_id, ent_name, chunk["content_with_weight"])
    res = settings.retrievaler.search({"entity_kwd": ent_name, "size": 1, "fields": []},
                                      search.index_name(tenant_id), [kb_id])
    if res.ids:
        settings.docStoreConn.update({"entity_kwd": ent_name}, chunk, search.index_name(tenant_id), kb_id)
    else:
        ebd = get_embed_cache(embd_mdl.llm_name, ent_name)
        if ebd is None:
            try:
                ebd, _ = embd_mdl.encode([ent_name])
                ebd = ebd[0]
                set_embed_cache(embd_mdl.llm_name, ent_name, ebd)
            except Exception as e:
                logging.exception(f"Fail to embed entity: {e}")
        if ebd is not None:
            chunk["q_%d_vec" % len(ebd)] = ebd
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id), kb_id)


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


def set_relation(tenant_id, kb_id, embd_mdl, from_ent_name, to_ent_name, meta):
    chunk = {
        "from_entity_kwd": from_ent_name,
        "to_entity_kwd": to_ent_name,
        "knowledge_graph_kwd": "relation",
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "important_kwd": meta["keywords"],
        "source_id": list(set(meta["source_id"])),
        "weight_int": int(meta["weight"]),
        "kb_id": kb_id,
        "available_int": 0
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    res = settings.retrievaler.search({"from_entity_kwd": to_ent_name, "to_entity_kwd": to_ent_name, "size": 1, "fields": []},
                                      search.index_name(tenant_id), [kb_id])

    if res.ids:
        settings.docStoreConn.update({"from_entity_kwd": from_ent_name, "to_entity_kwd": to_ent_name},
                                 chunk,
                                 search.index_name(tenant_id), kb_id)
    else:
        txt = f"{from_ent_name}->{to_ent_name}"
        ebd = get_embed_cache(embd_mdl.llm_name, txt)
        if ebd is None:
            try:
                ebd, _ = embd_mdl.encode([txt+f": {meta['description']}"])
                ebd = ebd[0]
                set_embed_cache(embd_mdl.llm_name, txt, ebd)
            except Exception as e:
                logging.exception(f"Fail to embed entity relation: {e}")
        if ebd is not None:
            chunk["q_%d_vec" % len(ebd)] = ebd
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id), kb_id)

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
        return None, []
    for id in res.ids:
        try:
            return json_graph.node_link_graph(json.loads(res.field[id]["content_with_weight"]), edges="edges"), \
                   res.field[id]["source_id"]
        except Exception:
            continue
    result = await rebuild_graph(tenant_id, kb_id)
    return result


async def set_graph(tenant_id, kb_id, graph, docids):
    chunk = {
        "content_with_weight": json.dumps(nx.node_link_data(graph, edges="edges"), ensure_ascii=False,
                                          indent=2),
        "knowledge_graph_kwd": "graph",
        "kb_id": kb_id,
        "source_id": list(docids),
        "available_int": 0,
        "removed_kwd": "N"
    }     
    res = await trio.to_thread.run_sync(lambda: settings.retrievaler.search({"knowledge_graph_kwd": "graph", "size": 1, "fields": []}, search.index_name(tenant_id), [kb_id]))
    if res.ids:
        await trio.to_thread.run_sync(lambda: settings.docStoreConn.update({"knowledge_graph_kwd": "graph"}, chunk,
                                     search.index_name(tenant_id), kb_id))
    else:
        await trio.to_thread.run_sync(lambda: settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id), kb_id))


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


async def update_nodes_pagerank_nhop_neighbour(tenant_id, kb_id, graph, n_hop):
    def n_neighbor(id):
        nonlocal graph, n_hop
        count = 0
        source_edge = list(graph.edges(id))
        if not source_edge:
            return []
        count = count + 1
        while count < n_hop:
            count = count + 1
            sc_edge = deepcopy(source_edge)
            source_edge = []
            for pair in sc_edge:
                append_edge = list(graph.edges(pair[-1]))
                for tuples in merge_tuples([pair], append_edge):
                    source_edge.append(tuples)
        nbrs = []
        for path in source_edge:
            n = {"path": path, "weights": []}
            wts = nx.get_edge_attributes(graph, 'weight')
            for i in range(len(path)-1):
                f, t = path[i], path[i+1]
                n["weights"].append(wts.get((f, t), 0))
            nbrs.append(n)
        return nbrs

    pr = nx.pagerank(graph)
    try:
        async with trio.open_nursery() as nursery:
            for n, p in pr.items():
                graph.nodes[n]["pagerank"] = p
                nursery.start_soon(lambda: trio.to_thread.run_sync(lambda: settings.docStoreConn.update({"entity_kwd": n, "kb_id": kb_id},
                                                {"rank_flt": p,
                                                "n_hop_with_weight": json.dumps((n), ensure_ascii=False)},
                                                search.index_name(tenant_id), kb_id)))
    except Exception as e:
        logging.exception(e)

    ty2ents = defaultdict(list)
    for p, r in sorted(pr.items(), key=lambda x: x[1], reverse=True):
        ty = graph.nodes[p].get("entity_type")
        if not ty or len(ty2ents[ty]) > 12:
            continue
        ty2ents[ty].append(p)

    chunk = {
        "content_with_weight": json.dumps(ty2ents, ensure_ascii=False),
        "kb_id": kb_id,
        "knowledge_graph_kwd": "ty2ents",
        "available_int": 0
    }
    res = await trio.to_thread.run_sync(lambda: settings.retrievaler.search({"knowledge_graph_kwd": "ty2ents", "size": 1, "fields": []},
                                      search.index_name(tenant_id), [kb_id]))
    if res.ids:
        await trio.to_thread.run_sync(lambda: settings.docStoreConn.update({"knowledge_graph_kwd": "ty2ents"},
                                     chunk,
                                     search.index_name(tenant_id), kb_id))
    else:
        await trio.to_thread.run_sync(lambda: settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id), kb_id))


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
    src_ids = []
    flds = ["entity_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd", "weight_int", "knowledge_graph_kwd", "source_id"]
    bs = 256
    for i in range(0, 39*bs, bs):
        es_res = await trio.to_thread.run_sync(lambda: settings.docStoreConn.search(flds, [],
                                 {"kb_id": kb_id, "knowledge_graph_kwd": ["entity", "relation"]},
                                 [],
                                 OrderByExpr(),
                                 i, bs, search.index_name(tenant_id), [kb_id]
                                 ))
        tot = settings.docStoreConn.getTotal(es_res)
        if tot == 0:
            return None, None

        es_res = settings.docStoreConn.getFields(es_res, flds)
        for id, d in es_res.items():
            src_ids.extend(d.get("source_id", []))
            if d["knowledge_graph_kwd"] == "entity":
                graph.add_node(d["entity_kwd"], entity_type=d["entity_type_kwd"])
            elif "from_entity_kwd" in d and "to_entity_kwd" in d:
                graph.add_edge(
                    d["from_entity_kwd"],
                    d["to_entity_kwd"],
                    weight=int(d["weight_int"])
                )

        if len(es_res.keys()) < 128:
            return graph, list(set(src_ids))

    return graph, list(set(src_ids))
