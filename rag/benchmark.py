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
from collections import defaultdict
from api.db import FileType, TaskStatus, ParserType, LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.settings import retrievaler
from api.utils import get_uuid
from rag.nlp import tokenize, search
from rag.utils.es_conn import ELASTICSEARCH
from ranx import evaluate


class benchmark_ndcg10:
    def __init__(self, kb_id):
        e, kb = KnowledgebaseService.get_by_id(kb_id)
        self.similarity_threshold = kb.similarity_threshold
        self.vector_similarity_weight = kb.vector_similarity_weight
        self.embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=kb.embd_id, lang=kb.language)

    def _get_benchmarks(self, query, count=16):
        req = {"question": query, "size": count, "vector": True, "similarity": self.similarity_threshold}
        sres = retrievaler.search(req, search.index_name("benchmark"), self.embd_mdl)
        return sres

    def _get_retrieval(self, qrels):
        run = defaultdict(dict)
        query_list = list(qrels.keys())
        for query in query_list:
            sres = self._get_benchmarks(query)
            sim, _, _ = retrievaler.rerank(sres, query, 1 - self.vector_similarity_weight,
                                           self.vector_similarity_weight)
            for index, id in enumerate(sres.ids):
                run[query][id] = sim[index]
        return run

    def embedding(self, docs, batch_size=16):
        vects = []
        cnts = [d["content_with_weight"] for d in docs]
        for i in range(0, len(cnts), batch_size):
            vts, c = self.embd_mdl.encode(cnts[i: i + batch_size])
            vects.extend(vts.tolist())
        assert len(docs) == len(vects)
        for i, d in enumerate(docs):
            v = vects[i]
            d["q_%d_vec" % len(v)] = v
        return docs

    def __call__(self, file_path):
        qrels = defaultdict(dict)

        docs = []
        with open(file_path) as f:
            for line in f:
                query, text, rel = line.strip('\n').split()
                d = {
                    "id": get_uuid()
                }
                tokenize(d, text)
                docs.append(d)
                if len(docs) >= 32:
                    ELASTICSEARCH.bulk(docs, search.index_name("benchmark"))
                    docs = []
                qrels[query][d["id"]] = float(rel)
            docs = self.embedding(docs)
            ELASTICSEARCH.bulk(docs, search.index_name("benchmark"))

        run = self._get_retrieval(qrels)
        return evaluate(qrels, run, "ndcg@10")


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('-f', '--filepath', default='', help="file path", action='store', required=True)
    parser.add_argument('-k', '--kb_id', default='', help="kb_id", action='store', required=True)
    args = parser.parse_args()

    ex = benchmark_ndcg10(args.kb_id)
    print(ex(args.filepath))
