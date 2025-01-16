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
from graphrag.light.graph_prompt import PROMPTS
from graphrag.utils import graph_merge, get_entity, set_entity, get_relation, set_relation, get_graph, set_graph
from rag.nlp import rag_tokenizer, search


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

