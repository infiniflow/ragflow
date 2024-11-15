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
from graphrag import leiden
from graphrag.community_reports_extractor import CommunityReportsExtractor
from graphrag.entity_resolution import EntityResolution
from graphrag.graph_extractor import GraphExtractor
from graphrag.leiden import add_community_info2graph

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('-t', '--tenant_id', default=False, help="Tenant ID", action='store', required=True)
    parser.add_argument('-d', '--doc_id', default=False, help="Document ID", action='store', required=True)
    args = parser.parse_args()

    from api.db import LLMType
    from api.db.services.llm_service import LLMBundle
    from api import settings
    from api.db.services.knowledgebase_service import KnowledgebaseService

    kb_ids = KnowledgebaseService.get_kb_ids(args.tenant_id)

    ex = GraphExtractor(LLMBundle(args.tenant_id, LLMType.CHAT))
    docs = [d["content_with_weight"] for d in
            settings.retrievaler.chunk_list(args.doc_id, args.tenant_id, kb_ids, max_count=6, fields=["content_with_weight"])]
    graph = ex(docs)

    er = EntityResolution(LLMBundle(args.tenant_id, LLMType.CHAT))
    graph = er(graph.output)

    comm = leiden.run(graph.output, {})
    add_community_info2graph(graph.output, comm)

    # print(json.dumps(nx.node_link_data(graph.output), ensure_ascii=False,indent=2))
    print(json.dumps(comm, ensure_ascii=False, indent=2))

    cr = CommunityReportsExtractor(LLMBundle(args.tenant_id, LLMType.CHAT))
    cr = cr(graph.output)
    print("------------------ COMMUNITY REPORT ----------------------\n", cr.output)
    print(json.dumps(cr.structured_output, ensure_ascii=False, indent=2))