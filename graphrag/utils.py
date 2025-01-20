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
from hashlib import md5
from typing import Any, Callable

import networkx as nx
import numpy as np
import xxhash
from networkx.readwrite import json_graph

from api import settings
from rag.nlp import search, rag_tokenizer
from rag.utils.redis_conn import REDIS_CONN

ErrorHandlerFn = Callable[[BaseException | None, str | None, dict | None], None]


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

        continue
        if not attr.get("description"):
            continue

        if "description" not in g.nodes[n]:
            g.nodes[n]["description"] = attr["description"]

        if g.nodes[n]["description"].lower().find(attr["description"][:32].lower()) < 0:
            g.nodes[n]["description"] += "\n" + attr["description"]

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


def get_entity(tenant_id, kb_id, ent_name):
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
                return json.loads(es_res.field[id]["content_with_weight"])
            res.append(json.loads(es_res.field[id]["content_with_weight"]))
        except Exception:
            continue

    return res


def set_entity(tenant_id, kb_id, embd_mdl, ent_name, meta):
    chunk = {
        "important_kwd": [ent_name],
        "title_tks": rag_tokenizer.tokenize(ent_name),
        "entity_kwd": [ent_name],
        "knowledge_graph_kwd": "entity",
        "entity_type_kwd": meta["entity_type"],
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "source_id": meta["source_id"],
        "kb_id": kb_id
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    res = settings.retrievaler.search({"entity_kwd": ent_name, "size": 1, "fields": []},
                                      search.index_name(tenant_id), [kb_id])
    if res.ids:
        settings.docStoreConn.update({"entity_kwd": ent_name}, chunk, search.index_name(tenant_id), kb_id)
    else:
        try:
            ebd, _ = embd_mdl.encode([ent_name])
            chunk["q_%d_vec" % len(ebd[0])] = ebd[0]
        except Exception as e:
            logging.exception(f"Fail to embed entity: {e}")
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id))


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
    es_res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id])
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
        "from_entity_kwd": [from_ent_name],
        "to_entity_kwd": [to_ent_name],
        "knowledge_graph_kwd": "relation",
        "content_with_weight": json.dumps(meta, ensure_ascii=False),
        "content_ltks": rag_tokenizer.tokenize(meta["description"]),
        "important_kwd": meta["keywords"],
        "source_id": meta["source_id"],
        "weight_int": int(meta["weight"]),
        "kb_id": kb_id
    }
    chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
    res = settings.retrievaler.search({"from_entity_kwd": to_ent_name, "to_entity_kwd": to_ent_name, "size": 1, "fields": []},
                                      search.index_name(tenant_id), [kb_id])
    if res.ids:
        settings.docStoreConn.update({"from_entity_kwd": to_ent_name, "to_entity_kwd": to_ent_name},
                                 chunk,
                                 search.index_name(tenant_id), kb_id)
    else:
        try:
            ebd, _ = embd_mdl.encode([f"{from_ent_name}, {to_ent_name}"])
            chunk["q_%d_vec" % len(ebd[0])] = ebd[0]
        except Exception as e:
            logging.exception(f"Fail to embed entity relation: {e}")
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id))


def get_graph(tenant_id, kb_id):
    conds = {
        "fields": ["content_with_weight"],
        "size": 1,
        "knowledge_graph_kwd": ["graph"]
    }
    res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id])
    for id in res.ids:
        try:
            return json_graph.node_link_graph(json.loads(res.field[id]["content_with_weight"]), edges="edges")
        except Exception:
            continue


def set_graph(tenant_id, kb_id, graph):
    chunk = {
        "content_with_weight": json.dumps(nx.node_link_data(graph, edges="edges"), ensure_ascii=False,
                                          indent=2),
        "knowledge_graph_kwd": "graph",
        "kb_id": kb_id
    }
    res = settings.retrievaler.search({"knowledge_graph_kwd": "graph", "size": 1, "fields": []}, search.index_name(tenant_id), [kb_id])
    if res.ids:
        settings.docStoreConn.update({"knowledge_graph_kwd": "graph"}, chunk,
                                     search.index_name(tenant_id), kb_id)
    else:
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id))


def flat_uniq_list(arr, key):
    res = []
    for a in arr:
        a = a[key]
        if isinstance(a, list):
            res.extend(a)
        else:
            res.append(a)
    return list(set(res))