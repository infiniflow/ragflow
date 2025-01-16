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
from functools import reduce, partial
import networkx as nx

from api import settings
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from graphrag.general.community_reports_extractor import CommunityReportsExtractor
from graphrag.entity_resolution import EntityResolution
from graphrag.general.extractor import Extractor
from graphrag.general.graph_extractor import GraphExtractor, DEFAULT_ENTITY_TYPES
from graphrag.utils import graph_merge, set_entity, get_relation, set_relation, get_entity, get_graph, set_graph, \
    chunk_id
from rag.nlp import rag_tokenizer, search

class Dealer:
    def __init__(self,
                 extractor: Extractor,
                 tenant_id: str,
                 kb_id: str,
                 chunks: list[tuple[str, str]],
                 language,
                 callback=None,
                 entity_types=DEFAULT_ENTITY_TYPES
                 ):
        _, tenant = TenantService.get_by_id(tenant_id)
        self.llm_bdl = LLMBundle(tenant_id, LLMType.CHAT, tenant.llm_id)
        ext = extractor(self.llm_bdl, language=language,
                             entity_types=entity_types,
                             get_entity=partial(get_entity, tenant_id, kb_id),
                             set_entity=partial(set_entity, tenant_id, kb_id),
                             get_relation=partial(get_relation, tenant_id, kb_id),
                             set_relation=partial(set_relation, tenant_id, kb_id)
                             )
        ents, rels = ext(chunks, callback)
        self.graph = nx.Graph()
        for en in ents:
            self.graph.add_node(en["entity_name"],
                           entity_type=en["entity_type"],
                           description=en["description"])
        for rel in rels:
            self.graph.add_edge(
                rel["src_id"],
                rel["tgt_id"],
                weight=rel["weight"],
                description=rel["description"]
            )

        old_graph = get_graph(tenant_id, kb_id)
        if old_graph is not None:
            self.graph = reduce(graph_merge, [old_graph, self.graph])


class WithCommunity(Dealer):
    def __init__(self,
                 tenant_id: str,
                 kb_id: str,
                 chunks: list[tuple[str, str]],
                 callback,
                 language,
                 entity_types=DEFAULT_ENTITY_TYPES
                 ):
        super().__init__(GraphExtractor, tenant_id, kb_id, chunks, callback, language, entity_types)

        er = EntityResolution(self.llm_bdl)
        graph = er(self.graph).output

        set_graph(tenant_id, kb_id, graph)

        for node_degree in graph.degree:
            graph.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])

        callback(0.6, "Extracting community reports.")
        cr = CommunityReportsExtractor(self.llm_bdl)
        cr = cr(graph, callback=callback)
        self.community_structure = cr.structured_output
        self.community_reports = cr.output

        for community, desc in zip(cr.structured_output, cr.output):
            chunk = {
                "title_tks": rag_tokenizer.tokenize(community["title"]),
                "content_with_weight": desc,
                "content_ltks": rag_tokenizer.tokenize(desc),
                "knowledge_graph_kwd": "community_report",
                "weight_flt": community["weight"],
                "entities_kwd": community["entities"],
                "important_kwd": community["entities"],
                "kb_id": kb_id
            }
            chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
            settings.docStoreConn.insert([{"id": chunk_id(chunk), **chunk}], search.index_name(tenant_id))

        set_graph(tenant_id, kb_id, graph)








