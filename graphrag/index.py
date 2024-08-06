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
import re
from concurrent.futures import ThreadPoolExecutor
import json
from functools import reduce
from typing import List
import networkx as nx
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from graphrag.community_reports_extractor import CommunityReportsExtractor
from graphrag.entity_resolution import EntityResolution
from graphrag.graph_extractor import GraphExtractor
from graphrag.mind_map_extractor import MindMapExtractor
from rag.nlp import rag_tokenizer
from rag.utils import num_tokens_from_string


def be_children(obj: dict, keyset:set):
    if isinstance(obj, str):
        obj = [obj]
    if isinstance(obj, list):
        for i in obj: keyset.add(i)
        return [{"id": i, "children":[]} for i in obj]
    arr = []
    for k,v in obj.items():
        k = re.sub(r"\*+", "", k)
        if not k or k in keyset:continue
        keyset.add(k)
        arr.append({
            "id": k,
            "children": be_children(v, keyset) if isinstance(v, dict) else []
        })
    return arr


def graph_merge(g1, g2):
    g = g2.copy()
    for n, attr in g1.nodes(data=True):
        if n not in g2.nodes():
            g.add_node(n, **attr)
            continue

        g.nodes[n]["weight"] += 1
        if g.nodes[n]["description"].lower().find(attr["description"][:32].lower()) < 0:
            g.nodes[n]["description"] += "\n" + attr["description"]

    for source, target, attr in g1.edges(data=True):
        if g.has_edge(source, target):
            g[source][target].update({"weight": attr["weight"]+1})
            continue
        g.add_edge(source, target, **attr)

    for node_degree in g.degree:
        g.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])
    return g


def build_knowlege_graph_chunks(tenant_id: str, chunks: List[str], callback, entity_types=["organization", "person", "location", "event", "time"]):
    _, tenant = TenantService.get_by_id(tenant_id)
    llm_bdl = LLMBundle(tenant_id, LLMType.CHAT, tenant.llm_id)
    ext = GraphExtractor(llm_bdl)
    left_token_count = llm_bdl.max_length - ext.prompt_token_count - 1024
    left_token_count = max(llm_bdl.max_length * 0.8, left_token_count)

    assert left_token_count > 0, f"The LLM context length({llm_bdl.max_length}) is smaller than prompt({ext.prompt_token_count})"

    texts, graphs = [], []
    cnt = 0
    threads = []
    exe = ThreadPoolExecutor(max_workers=12)
    for i in range(len(chunks)):
        tkn_cnt = num_tokens_from_string(chunks[i])
        if cnt+tkn_cnt >= left_token_count and texts:
            threads.append(exe.submit(ext, texts, {"entity_types": entity_types}))
            texts = []
            cnt = 0
        texts.append(chunks[i])
        cnt += tkn_cnt
    if texts:
        threads.append(exe.submit(ext, texts))

    callback(0.5, "Extracting entities.")
    graphs = []
    for i, _ in enumerate(threads):
        graphs.append(_.result().output)
        callback(0.5 + 0.1*i/len(threads))

    graph = reduce(graph_merge, graphs)
    er = EntityResolution(llm_bdl)
    graph = er(graph).output

    _chunks = chunks
    chunks = []
    for n, attr in graph.nodes(data=True):
        if attr.get("rank", 0) == 0:
            print(f"Ignore entity: {n}")
            continue
        chunk = {
            "name_kwd": n,
            "important_kwd": [n],
            "title_tks": rag_tokenizer.tokenize(n),
            "content_with_weight": json.dumps({"name": n, **attr}, ensure_ascii=False),
            "content_ltks": rag_tokenizer.tokenize(attr["description"]),
            "knowledge_graph_kwd": "entity",
            "rank_int": attr["rank"],
            "weight_int": attr["weight"]
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
        chunks.append(chunk)

    callback(0.6, "Extracting community reports.")
    cr = CommunityReportsExtractor(llm_bdl)
    cr = cr(graph)
    for community, desc in zip(cr.structured_output, cr.output):
        chunk = {
            "title_tks": rag_tokenizer.tokenize(community["title"]),
            "content_with_weight": desc,
            "content_ltks": rag_tokenizer.tokenize(desc),
            "knowledge_graph_kwd": "community_report",
            "weight_flt": community["weight"],
            "entities_kwd": community["entities"],
            "important_kwd": community["entities"]
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
        chunks.append(chunk)

    chunks.append(
        {
            "content_with_weight": json.dumps(nx.node_link_data(graph), ensure_ascii=False, indent=2),
            "knowledge_graph_kwd": "graph"
        })

    callback(0.75, "Extracting mind graph.")
    mindmap = MindMapExtractor(llm_bdl)
    mg = mindmap(_chunks).output
    if not len(mg.keys()): return chunks

    if len(mg.keys()) > 1:
        keyset = set([re.sub(r"\*+", "", k) for k,v in mg.items() if isinstance(v, dict) and re.sub(r"\*+", "", k)])
        md_map = {"id": "root", "children": [{"id": re.sub(r"\*+", "", k), "children": be_children(v, keyset)} for k,v in mg.items() if isinstance(v, dict) and re.sub(r"\*+", "", k)]}
    else:
        k = re.sub(r"\*+", "", list(mg.keys())[0])
        md_map = {"id": k, "children": be_children(list(mg.items())[0][1], set([k]))}
    print(json.dumps(md_map, ensure_ascii=False, indent=2))
    chunks.append(
        {
            "content_with_weight": json.dumps(md_map, ensure_ascii=False, indent=2),
            "knowledge_graph_kwd": "mind_map"
        })

    return chunks






