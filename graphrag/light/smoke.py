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

import argparse
import json
from functools import partial
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api import settings
import networkx as nx
from api.db.services.document_service import DocumentService
from graphrag.light.graph_extractor import GraphExtractor
from graphrag.light.index import get_entity, set_entity, get_relation, set_relation

settings.init_settings()

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('-t', '--tenant_id', default=False, help="Tenant ID", action='store', required=True)
    parser.add_argument('-d', '--doc_id', default=False, help="Document ID", action='store', required=True)
    args = parser.parse_args()

    e, doc = DocumentService.get_by_id(args.doc_id)
    if not e:
        raise LookupError("Document not found.")
    kb_id = doc.kb_id

    chunks = [d["content_with_weight"] for d in
              settings.retrievaler.chunk_list(args.doc_id, args.tenant_id, [kb_id], max_count=6,
                                              fields=["content_with_weight"])]

    ext = GraphExtractor(LLMBundle(args.tenant_id, LLMType.CHAT), language="English",
                         get_entity=partial(get_entity, args.tenant_id, kb_id),
                         set_entity=partial(set_entity, args.tenant_id, kb_id),
                         get_relation=partial(get_relation, args.tenant_id, kb_id),
                         set_relation=partial(set_relation, args.tenant_id, kb_id)
                         )
    ents, rels = ext([("x", c) for c in chunks])

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

    print(json.dumps(nx.node_link_data(graph), ensure_ascii=False, indent=2))
