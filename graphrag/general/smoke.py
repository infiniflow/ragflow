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

import networkx as nx

from api import settings
from api.db import LLMType
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from graphrag.general.index import WithCommunity, Dealer, WithResolution
from graphrag.light.graph_extractor import GraphExtractor
from rag.utils.redis_conn import RedisDistributedLock

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
    chunks = [("x", c) for c in chunks]

    RedisDistributedLock.clean_lock(kb_id)

    _, tenant = TenantService.get_by_id(args.tenant_id)
    llm_bdl = LLMBundle(args.tenant_id, LLMType.CHAT, tenant.llm_id)
    _, kb = KnowledgebaseService.get_by_id(kb_id)
    embed_bdl = LLMBundle(args.tenant_id, LLMType.EMBEDDING, kb.embd_id)

    dealer = Dealer(GraphExtractor, args.tenant_id, kb_id, llm_bdl, chunks, "English", embed_bdl=embed_bdl)
    print(json.dumps(nx.node_link_data(dealer.graph), ensure_ascii=False, indent=2))

    dealer = WithResolution(args.tenant_id, kb_id, llm_bdl, embed_bdl)
    dealer = WithCommunity(args.tenant_id, kb_id, llm_bdl, embed_bdl)

    print("------------------ COMMUNITY REPORT ----------------------\n", dealer.community_reports)
    print(json.dumps(dealer.community_structure, ensure_ascii=False, indent=2))
