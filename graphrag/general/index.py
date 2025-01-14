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
import logging
import os
from concurrent.futures import ThreadPoolExecutor
import json
from functools import reduce
import networkx as nx
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from graphrag.general.community_reports_extractor import CommunityReportsExtractor
from graphrag.general.entity_resolution import EntityResolution
from graphrag.general.graph_extractor import GraphExtractor, DEFAULT_ENTITY_TYPES
from graphrag.general.mind_map_extractor import MindMapExtractor
from graphrag.utils import graph_merge
from rag.nlp import rag_tokenizer
from rag.utils import num_tokens_from_string


def build_knowledge_graph_chunks(tenant_id: str, chunks: list[str], callback, entity_types=DEFAULT_ENTITY_TYPES):
    _, tenant = TenantService.get_by_id(tenant_id)
    llm_bdl = LLMBundle(tenant_id, LLMType.CHAT, tenant.llm_id)
    ext = GraphExtractor(llm_bdl)
    left_token_count = llm_bdl.max_length - ext.prompt_token_count - 1024
    left_token_count = max(llm_bdl.max_length * 0.6, left_token_count)

    assert left_token_count > 0, f"The LLM context length({llm_bdl.max_length}) is smaller than prompt({ext.prompt_token_count})"

    BATCH_SIZE=4
    texts, graphs = [], []
    cnt = 0
    max_workers = int(os.environ.get('GRAPH_EXTRACTOR_MAX_WORKERS', 50))
    with ThreadPoolExecutor(max_workers=max_workers) as exe:
        threads = []
        for i in range(len(chunks)):
            tkn_cnt = num_tokens_from_string(chunks[i])
            if cnt+tkn_cnt >= left_token_count and texts:
                for b in range(0, len(texts), BATCH_SIZE):
                    threads.append(exe.submit(ext, ["\n".join(texts[b:b+BATCH_SIZE])], {"entity_types": entity_types}, callback))
                texts = []
                cnt = 0
            texts.append(chunks[i])
            cnt += tkn_cnt
        if texts:
            for b in range(0, len(texts), BATCH_SIZE):
                threads.append(exe.submit(ext, ["\n".join(texts[b:b+BATCH_SIZE])], {"entity_types": entity_types}, callback))

        callback(0.5, "Extracting entities.")
        graphs = []
        for i, _ in enumerate(threads):
            graphs.append(_.result().output)
            callback(0.5 + 0.1*i/len(threads), f"Entities extraction progress ... {i+1}/{len(threads)}")

    graph = reduce(graph_merge, graphs) if graphs else nx.Graph()
    er = EntityResolution(llm_bdl)
    graph = er(graph).output

    _chunks = chunks
    chunks = []
    for n, attr in graph.nodes(data=True):
        if attr.get("rank", 0) == 0:
            logging.debug(f"Ignore entity: {n}")
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
    cr = cr(graph, callback=callback)
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
    if not len(mg.keys()):
        return chunks

    logging.debug(json.dumps(mg, ensure_ascii=False, indent=2))
    chunks.append(
        {
            "content_with_weight": json.dumps(mg, ensure_ascii=False, indent=2),
            "knowledge_graph_kwd": "mind_map"
        })

    return chunks






