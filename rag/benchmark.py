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
import os
import sys
import time
import argparse
from collections import defaultdict

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.knowledgebase_service import KnowledgebaseService
from api import settings
from api.utils import get_uuid
from rag.nlp import tokenize, search
from ranx import evaluate
from ranx import Qrels, Run
import pandas as pd
from tqdm import tqdm

global max_docs
max_docs = sys.maxsize


class Benchmark:
    def __init__(self, kb_id):
        self.kb_id = kb_id
        e, self.kb = KnowledgebaseService.get_by_id(kb_id)
        self.similarity_threshold = self.kb.similarity_threshold
        self.vector_similarity_weight = self.kb.vector_similarity_weight
        self.embd_mdl = LLMBundle(self.kb.tenant_id, LLMType.EMBEDDING, llm_name=self.kb.embd_id, lang=self.kb.language)
        self.tenant_id = ''
        self.index_name = ''
        self.initialized_index = False

    def _get_retrieval(self, qrels):
        # Need to wait for the ES and Infinity index to be ready
        time.sleep(20)
        run = defaultdict(dict)
        query_list = list(qrels.keys())
        for query in query_list:
            ranks = settings.retrievaler.retrieval(query, self.embd_mdl, self.tenant_id, [self.kb.id], 1, 30,
                                            0.0, self.vector_similarity_weight)
            if len(ranks["chunks"]) == 0:
                print(f"deleted query: {query}")
                del qrels[query]
                continue
            for c in ranks["chunks"]:
                c.pop("vector", None)
                run[query][c["chunk_id"]] = c["similarity"]
        return run

    def embedding(self, docs):
        texts = [d["content_with_weight"] for d in docs]
        embeddings, _ = self.embd_mdl.encode(texts)
        assert len(docs) == len(embeddings)
        vector_size = 0
        for i, d in enumerate(docs):
            v = embeddings[i]
            vector_size = len(v)
            d["q_%d_vec" % len(v)] = v
        return docs, vector_size

    def init_index(self, vector_size: int):
        if self.initialized_index:
            return
        if settings.docStoreConn.indexExist(self.index_name, self.kb_id):
            settings.docStoreConn.deleteIdx(self.index_name, self.kb_id)
        settings.docStoreConn.createIdx(self.index_name, self.kb_id, vector_size)
        self.initialized_index = True

    def ms_marco_index(self, file_path, index_name):
        qrels = defaultdict(dict)
        texts = defaultdict(dict)
        docs_count = 0
        docs = []
        filelist = sorted(os.listdir(file_path))

        for fn in filelist:
            if docs_count >= max_docs:
                break
            if not fn.endswith(".parquet"):
                continue
            data = pd.read_parquet(os.path.join(file_path, fn))
            for i in tqdm(range(len(data)), colour="green", desc="Tokenizing:" + fn):
                if docs_count >= max_docs:
                    break
                query = data.iloc[i]['query']
                for rel, text in zip(data.iloc[i]['passages']['is_selected'], data.iloc[i]['passages']['passage_text']):
                    d = {
                        "id": get_uuid(),
                        "kb_id": self.kb.id,
                        "docnm_kwd": "xxxxx",
                        "doc_id": "ksksks"
                    }
                    tokenize(d, text, "english")
                    docs.append(d)
                    texts[d["id"]] = text
                    qrels[query][d["id"]] = int(rel)
                if len(docs) >= 32:
                    docs_count += len(docs)
                    docs, vector_size = self.embedding(docs)
                    self.init_index(vector_size)
                    settings.docStoreConn.insert(docs, self.index_name, self.kb_id)
                    docs = []

        if docs:
            docs, vector_size = self.embedding(docs)
            self.init_index(vector_size)
            settings.docStoreConn.insert(docs, self.index_name, self.kb_id)
        return qrels, texts

    def trivia_qa_index(self, file_path, index_name):
        qrels = defaultdict(dict)
        texts = defaultdict(dict)
        docs_count = 0
        docs = []
        filelist = sorted(os.listdir(file_path))
        for fn in filelist:
            if docs_count >= max_docs:
                break
            if not fn.endswith(".parquet"):
                continue
            data = pd.read_parquet(os.path.join(file_path, fn))
            for i in tqdm(range(len(data)), colour="green", desc="Indexing:" + fn):
                if docs_count >= max_docs:
                    break
                query = data.iloc[i]['question']
                for rel, text in zip(data.iloc[i]["search_results"]['rank'],
                                     data.iloc[i]["search_results"]['search_context']):
                    d = {
                        "id": get_uuid(),
                        "kb_id": self.kb.id,
                        "docnm_kwd": "xxxxx",
                        "doc_id": "ksksks"
                    }
                    tokenize(d, text, "english")
                    docs.append(d)
                    texts[d["id"]] = text
                    qrels[query][d["id"]] = int(rel)
                if len(docs) >= 32:
                    docs_count += len(docs)
                    docs, vector_size = self.embedding(docs)
                    self.init_index(vector_size)
                    settings.docStoreConn.insert(docs,self.index_name)
                    docs = []

        docs, vector_size = self.embedding(docs)
        self.init_index(vector_size)
        settings.docStoreConn.insert(docs, self.index_name)
        return qrels, texts

    def miracl_index(self, file_path, corpus_path, index_name):
        corpus_total = {}
        for corpus_file in os.listdir(corpus_path):
            tmp_data = pd.read_json(os.path.join(corpus_path, corpus_file), lines=True)
            for index, i in tmp_data.iterrows():
                corpus_total[i['docid']] = i['text']

        topics_total = {}
        for topics_file in os.listdir(os.path.join(file_path, 'topics')):
            if 'test' in topics_file:
                continue
            tmp_data = pd.read_csv(os.path.join(file_path, 'topics', topics_file), sep='\t', names=['qid', 'query'])
            for index, i in tmp_data.iterrows():
                topics_total[i['qid']] = i['query']

        qrels = defaultdict(dict)
        texts = defaultdict(dict)
        docs_count = 0
        docs = []
        for qrels_file in os.listdir(os.path.join(file_path, 'qrels')):
            if 'test' in qrels_file:
                continue
            if docs_count >= max_docs:
                break

            tmp_data = pd.read_csv(os.path.join(file_path, 'qrels', qrels_file), sep='\t',
                                   names=['qid', 'Q0', 'docid', 'relevance'])
            for i in tqdm(range(len(tmp_data)), colour="green", desc="Indexing:" + qrels_file):
                if docs_count >= max_docs:
                    break
                query = topics_total[tmp_data.iloc[i]['qid']]
                text = corpus_total[tmp_data.iloc[i]['docid']]
                rel = tmp_data.iloc[i]['relevance']
                d = {
                    "id": get_uuid(),
                    "kb_id": self.kb.id,
                    "docnm_kwd": "xxxxx",
                    "doc_id": "ksksks"
                }
                tokenize(d, text, 'english')
                docs.append(d)
                texts[d["id"]] = text
                qrels[query][d["id"]] = int(rel)
                if len(docs) >= 32:
                    docs_count += len(docs)
                    docs, vector_size = self.embedding(docs)
                    self.init_index(vector_size)
                    settings.docStoreConn.insert(docs, self.index_name)
                    docs = []

        docs, vector_size = self.embedding(docs)
        self.init_index(vector_size)
        settings.docStoreConn.insert(docs, self.index_name)
        return qrels, texts

    def save_results(self, qrels, run, texts, dataset, file_path):
        keep_result = []
        run_keys = list(run.keys())
        for run_i in tqdm(range(len(run_keys)), desc="Calculating ndcg@10 for single query"):
            key = run_keys[run_i]
            keep_result.append({'query': key, 'qrel': qrels[key], 'run': run[key],
                                'ndcg@10': evaluate({key: qrels[key]}, {key: run[key]}, "ndcg@10")})
        keep_result = sorted(keep_result, key=lambda kk: kk['ndcg@10'])
        with open(os.path.join(file_path, dataset + 'result.md'), 'w', encoding='utf-8') as f:
            f.write('## Score For Every Query\n')
            for keep_result_i in keep_result:
                f.write('### query: ' + keep_result_i['query'] + ' ndcg@10:' + str(keep_result_i['ndcg@10']) + '\n')
                scores = [[i[0], i[1]] for i in keep_result_i['run'].items()]
                scores = sorted(scores, key=lambda kk: kk[1])
                for score in scores[:10]:
                    f.write('- text: ' + str(texts[score[0]]) + '\t qrel: ' + str(score[1]) + '\n')
        json.dump(qrels, open(os.path.join(file_path, dataset + '.qrels.json'), "w+", encoding='utf-8'), indent=2)
        json.dump(run, open(os.path.join(file_path, dataset + '.run.json'), "w+", encoding='utf-8'), indent=2)
        print(os.path.join(file_path, dataset + '_result.md'), 'Saved!')

    def __call__(self, dataset, file_path, miracl_corpus=''):
        if dataset == "ms_marco_v1.1":
            self.tenant_id = "benchmark_ms_marco_v11"
            self.index_name = search.index_name(self.tenant_id)
            qrels, texts = self.ms_marco_index(file_path, "benchmark_ms_marco_v1.1")
            run = self._get_retrieval(qrels)
            print(dataset, evaluate(Qrels(qrels), Run(run), ["ndcg@10", "map@5", "mrr@10"]))
            self.save_results(qrels, run, texts, dataset, file_path)
        if dataset == "trivia_qa":
            self.tenant_id = "benchmark_trivia_qa"
            self.index_name = search.index_name(self.tenant_id)
            qrels, texts = self.trivia_qa_index(file_path, "benchmark_trivia_qa")
            run = self._get_retrieval(qrels)
            print(dataset, evaluate(Qrels(qrels), Run(run), ["ndcg@10", "map@5", "mrr@10"]))
            self.save_results(qrels, run, texts, dataset, file_path)
        if dataset == "miracl":
            for lang in ['ar', 'bn', 'de', 'en', 'es', 'fa', 'fi', 'fr', 'hi', 'id', 'ja', 'ko', 'ru', 'sw', 'te', 'th',
                         'yo', 'zh']:
                if not os.path.isdir(os.path.join(file_path, 'miracl-v1.0-' + lang)):
                    print('Directory: ' + os.path.join(file_path, 'miracl-v1.0-' + lang) + ' not found!')
                    continue
                if not os.path.isdir(os.path.join(file_path, 'miracl-v1.0-' + lang, 'qrels')):
                    print('Directory: ' + os.path.join(file_path, 'miracl-v1.0-' + lang, 'qrels') + 'not found!')
                    continue
                if not os.path.isdir(os.path.join(file_path, 'miracl-v1.0-' + lang, 'topics')):
                    print('Directory: ' + os.path.join(file_path, 'miracl-v1.0-' + lang, 'topics') + 'not found!')
                    continue
                if not os.path.isdir(os.path.join(miracl_corpus, 'miracl-corpus-v1.0-' + lang)):
                    print('Directory: ' + os.path.join(miracl_corpus, 'miracl-corpus-v1.0-' + lang) + ' not found!')
                    continue
                self.tenant_id = "benchmark_miracl_" + lang
                self.index_name = search.index_name(self.tenant_id)
                self.initialized_index = False
                qrels, texts = self.miracl_index(os.path.join(file_path, 'miracl-v1.0-' + lang),
                                                 os.path.join(miracl_corpus, 'miracl-corpus-v1.0-' + lang),
                                                 "benchmark_miracl_" + lang)
                run = self._get_retrieval(qrels)
                print(dataset, evaluate(Qrels(qrels), Run(run), ["ndcg@10", "map@5", "mrr@10"]))
                self.save_results(qrels, run, texts, dataset, file_path)


if __name__ == '__main__':
    print('*****************RAGFlow Benchmark*****************')
    parser = argparse.ArgumentParser(usage="benchmark.py <max_docs> <kb_id> <dataset> <dataset_path> [<miracl_corpus_path>])", description='RAGFlow Benchmark')
    parser.add_argument('max_docs', metavar='max_docs', type=int, help='max docs to evaluate')
    parser.add_argument('kb_id', metavar='kb_id', help='knowledgebase id')
    parser.add_argument('dataset', metavar='dataset', help='dataset name, shall be one of ms_marco_v1.1(https://huggingface.co/datasets/microsoft/ms_marco), trivia_qa(https://huggingface.co/datasets/mandarjoshi/trivia_qa>), miracl(https://huggingface.co/datasets/miracl/miracl')
    parser.add_argument('dataset_path', metavar='dataset_path', help='dataset path')
    parser.add_argument('miracl_corpus_path', metavar='miracl_corpus_path', nargs='?', default="", help='miracl corpus path. Only needed when dataset is miracl')

    args = parser.parse_args()
    max_docs = args.max_docs
    kb_id = args.kb_id
    ex = Benchmark(kb_id)

    dataset = args.dataset
    dataset_path = args.dataset_path

    if dataset == "ms_marco_v1.1" or dataset == "trivia_qa":
        ex(dataset, dataset_path)
    elif dataset == "miracl":
        if len(args) < 5:
            print('Please input the correct parameters!')
            exit(1)
        miracl_corpus_path = args[4]
        ex(dataset, dataset_path, miracl_corpus=args.miracl_corpus_path)
    else:
        print("Dataset: ", dataset, "not supported!")
