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
from functools import reduce, partial
import networkx as nx
import xxhash
from networkx.readwrite import json_graph
from api import settings
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from graphrag.light.graph_extractor import GraphExtractor
from graphrag.light.prompt import PROMPTS
from graphrag.utils import graph_merge
from rag.nlp import rag_tokenizer, search


def chunk_id(chunk):
    return xxhash.xxh64((chunk["content_with_weight"] + chunk["kb_id"]).encode("utf-8")).hexdigest()


def get_entity(tenant_id, kb_id, ent_name):
    conds = {
        "fields": ["content_with_weight"],
        "size": 1,
        "entity_kwd": [ent_name],
        "knowledge_graph_kwd": ["entity"]
    }
    res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id])
    for id in res.ids:
        try:
            return json.loads(res.field[id]["content_with_weight"])
        except Exception:
            continue


def set_entity(tenant_id, kb_id, ent_name, meta):
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
        settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id))


def get_relation(tenant_id, kb_id, from_ent_name, to_ent_name):
    conds = {
        "fields": ["content_with_weight"],
        "size": 1,
        "from_entity_kwd": [from_ent_name, to_ent_name],
        "to_entity_kwd": [from_ent_name, to_ent_name],
        "knowledge_graph_kwd": ["relation"]
    }
    res = settings.retrievaler.search(conds, search.index_name(tenant_id), [kb_id])
    for id in res.ids:
        try:
            return json.loads(res.field[id]["content_with_weight"])
        except Exception:
            continue


def set_relation(tenant_id, kb_id, from_ent_name, to_ent_name, meta):
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
            return json_graph.node_link_graph(json.loads(res.field[id]["content_with_weight"]))
        except Exception:
            continue


def set_graph(tenant_id, kb_id, graph):
    chunk = {
        "content_with_weight": json.dumps(nx.node_link_data(graph), ensure_ascii=False,
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


def build_knowledge_graph_with_chunks(tenant_id: str,
                                      kb_id: str,
                                      chunks: list[tuple[str, str]],
                                      callback,
                                      language,
                                      entity_types=PROMPTS["DEFAULT_ENTITY_TYPES"]):
    _, tenant = TenantService.get_by_id(tenant_id)
    llm_bdl = LLMBundle(tenant_id, LLMType.CHAT, tenant.llm_id)
    ext = GraphExtractor(llm_bdl, language=language,
                         entity_types=entity_types,
                         get_entity=partial(get_entity, tenant_id, kb_id),
                         set_entity=partial(set_entity, tenant_id, kb_id),
                         get_relation=partial(get_relation, tenant_id, kb_id),
                         set_relation=partial(set_relation, tenant_id, kb_id)
                         )
    ents, rels = ext(chunks, callback)

    graph = nx.Graph()
    for en in ents:
        graph.add_node(en["entity_name"],
                       entity_type=en["entity_type"],
                       description=en["description"])
    for rel in rels:
        graph.add_edge(
            rel["src_id"],
            rel["tgt_id"],
            weight=rel["weight"],
            description=rel["description"]
        )

    old_graph = get_graph(tenant_id, kb_id)
    if old_graph is not None:
        graph = reduce(graph_merge, [old_graph, graph])
    set_graph(tenant_id, kb_id, graph)

