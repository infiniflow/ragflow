#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import re
import json
import time
import os

from contextlib import contextmanager
import copy
import psycopg2.extras  # type: ignore
import psycopg2.pool  # type: ignore
from rag import settings
from rag.settings import TAG_FLD, PAGERANK_FLD
from rag.utils import singleton

from rag.utils.doc_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)

ATTEMPT_TIME = 5

logger = logging.getLogger('ragflow.gauss_conn')

SQL_CREATE_TABLE = """
CREATE TABLE IF NOT EXISTS {table_name} (
    id VARCHAR NOT NULL,
    kb_id VARCHAR NOT NULL,
    doc_id VARCHAR NOT NULL,
    docnm_kwd VARCHAR NOT NULL,
    title_tks VARCHAR NOT NULL,
    content_with_weight TEXT NOT NULL,
    content_ltks TEXT NOT NULL,
    embedding floatvector({dimension}) NOT NULL
);
"""

SQL_CREATE_VECTOR_INDEX = """
CREATE INDEX IF NOT EXISTS idx_{table_name} ON {table_name} 
USING gsdiskann (embedding COSINE) WITH (
enable_pq=true,
pq_nseg=128,
pq_nclus=16,
num_parallels = 32,
quantization_type='pq',
subgraph_count = 1
)
"""

SQL_CREATE_BM25_INDEX = """
CREATE INDEX IF NOT EXISTS idx_bm25_{table_name} ON {table_name} 
USING bm25 (content_with_weight)
"""

@singleton
class GaussConnection(DocStoreConnection):
    def __init__(self):
        self.pool = None
        logger.info(f"Use GaussVector as the doc engine.")
        logger.info(settings.GAUSS["host"])
        for _ in range(ATTEMPT_TIME):
            try:
                self.pool = self._create_connection_pool()
                logger.info(f"GaussVector is healthy.")
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting GaussVector to be healthy.")
                time.sleep(5)

    def _create_connection_pool(self):
        return psycopg2.pool.SimpleConnectionPool(10, 50,
            host=settings.GAUSS["host"],
            port=settings.GAUSS["port"],
            user=settings.GAUSS["user"],
            password=settings.GAUSS["password"],
            database=settings.GAUSS["database"]
        )

    @contextmanager
    def _get_cursor(self):
        conn = self.pool.getconn()
        cur = conn.cursor()
        try:
            yield cur
        finally:
            cur.close()
            conn.commit()
            self.pool.putconn(conn)

    """
    Database operations
    """

    def dbType(self) -> str:
        logger.info(f"get dbType: gaussvector")
        return "gaussvector"

    def health(self) -> dict:
        logger.info(f"check GaussVector status")
        health_dict = {
            "type": "gaussvector",
            "status": "green",
            "error": "",
        }
        return health_dict

    """
    Table operations
    """

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        tableName = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.createIndex {tableName}")

        if self.indexExist(indexName, knowledgebaseId):
            return

        try:
            with self._get_cursor() as cur:
                cur.execute(SQL_CREATE_TABLE.format(table_name=tableName, dimension=vectorSize))
                cur.execute(SQL_CREATE_VECTOR_INDEX.format(table_name=tableName))
                cur.execute(SQL_CREATE_BM25_INDEX.format(table_name=tableName))
            return True
        except Exception:
            logger.exception("GaussConnection.createIndex error %s" % (indexName))

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        tableName = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.deleteIdx {tableName}")
        try:
            with self._get_cursor() as cur:
                cur.execute(f"DROP TABLE {tableName}")
                return True
        except Exception:
            logger.exception("GaussConnection.deleteIdx error %s" % (indexName))

    def indexExist(self, indexName: str, knowledgebaseId: str = None) -> bool:
        tableName = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.indexExist {tableName}")
        try:
            with self._get_cursor() as cur:
                cur.execute(f"SELECT id FROM {tableName}")
                logger.info(f"GaussConnection.indexExist is existed.")
                return True
        except Exception as e:
            logger.info(f"GaussConnection.indexExist is not existing.")
            return False

    """
    CRUD operations
    """
    def search_by_doc_id(self, table_name, doc_ids: list[str], limit, offset):
        logging.info(f"GaussConnection.search by doc_id {doc_ids}")
        doc_list_str = ", ".join([str("'" + doc_id + "'") for doc_id in doc_ids])
        start_time = time.perf_counter()
        with self._get_cursor() as cur:
            cur.execute(
                f"SELECT id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, embedding FROM {table_name}"
                f" WHERE doc_id in ({doc_list_str}) LIMIT {limit} OFFSET {offset}"
            )
            end_time = time.perf_counter()
            logging.info(f"GaussConnection.search by doc_id cost: ({round((end_time - start_time) * 1000, 2)} ms)")

            docs = []
            for record in cur:
                id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, vector = record
                vec_list = vector.strip("[").strip("]").split(",")
                docs.append({
                    "id": id,
                    "kb_id": kb_id,
                    "doc_id": doc_id,
                    "docnm_kwd": docnm_kwd,
                    "title_tks": title_tks,
                    "content_ltks": content_ltks,
                    "content_with_weight": content_with_weight,
                    "q_1024_vec": [float(v) for v in vec_list]
                    })

            return docs

    def search_by_vector(self, table_name, query_vector: list[float], score_threshold, top_k):
        top_k = min(top_k, 10)
        logging.info(f"GaussConnection.search vector topk: {top_k}")
        start_time = time.perf_counter()
        with self._get_cursor() as cur:
            cur.execute(
                f"""SELECT id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, 
                embedding, embedding <-> %s AS distance  FROM {table_name} ORDER BY distance LIMIT {top_k}
                """,
                (json.dumps(query_vector),),
            )
            end_time = time.perf_counter()
            logging.info(f"GaussConnection.search vector cost: ({round((end_time - start_time) * 1000, 2)} ms)")

            docs = []
            for record in cur:
                id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, vector, distance = record
                score = 1 - distance
                vec_list = vector.strip("[").strip("]").split(",")
                docs.append({
                    "id": id,
                    "kb_id": kb_id,
                    "doc_id": doc_id,
                    "docnm_kwd": docnm_kwd,
                    "title_tks": title_tks,
                    "content_ltks": content_ltks,
                    "content_with_weight": content_with_weight,
                    "q_1024_vec": [float(v) for v in vec_list]
                    })
                if score > score_threshold:
                    logger.info(f"GaussConnection.vector text {str(id)} ")

            return docs

    def search_by_full_text(self, table_name, query: str, score_threshold, top_k):
        top_k = min(top_k, 10)
        logging.info(f"GaussConnection.search bm25 topk: {top_k}")
        start_time = time.perf_counter()

        with self._get_cursor() as cur:
            cur.execute(
                f"""SELECT /*+ indexscan({table_name} idx_bm25_{table_name}) */ 
                id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, embedding,
                content_with_weight ### %s AS score FROM {table_name} ORDER BY score DESC LIMIT {top_k}""",
                (f"'{query}'",),
            )
            end_time = time.perf_counter()
            logging.info(f"GaussConnection.search bm25 cost: ({round((end_time - start_time) * 1000, 2)} ms)")

            docs = []

            for record in cur:
                id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, vector, score = record
                vec_list = vector.strip("[").strip("]").split(",")
                docs.append({
                    "id": id,
                    "kb_id": kb_id,
                    "doc_id": doc_id,
                    "docnm_kwd": docnm_kwd,
                    "title_tks": title_tks,
                    "content_ltks": content_ltks,
                    "content_with_weight": content_with_weight,
                    "q_1024_vec": [float(v) for v in vec_list]
                    })

            return docs

    def search(
            self, selectFields: list[str],
            highlightFields: list[str],
            condition: dict,
            matchExprs: list[MatchExpr],
            orderBy: OrderByExpr,
            offset: int,
            limit: int,
            indexNames: str | list[str],
            knowledgebaseIds: list[str],
            aggFields: list[str] = [],
            rank_feature: dict | None = None
    ):
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0

        logger.info(f"GaussConnection.search indexNames: {indexNames} ")
        logger.info(f"GaussConnection.search knowledgebaseIds: {knowledgebaseIds} ")
        logger.info(f"GaussConnection.search selectFields: {selectFields} ")
        logger.info(f"GaussConnection.search highlightFields: {highlightFields} ")
        logger.info(f"GaussConnection.search condition: {condition} ")
        logger.info(f"GaussConnection.search matchExprs: {matchExprs} ")
        logger.info(f"GaussConnection.search orderBy: {orderBy} ")
        logger.info(f"GaussConnection.search offset: {offset} ")
        logger.info(f"GaussConnection.search limit: {limit} ")
        logger.info(f"GaussConnection.search aggFields: {aggFields} ")
        logger.info(f"GaussConnection.search rank_feature: {rank_feature} ")

        table_name = ""
        res = []

        if "doc_id" in condition and len(condition["doc_id"]) > 0:
            for indexName in indexNames:
                for knowledgebaseId in knowledgebaseIds:
                    table_name = f"{indexName}_{knowledgebaseId}"
                    if self.indexExist(indexName, knowledgebaseId):                  
                        logger.info(f"GaussConnection.search by doc_id {table_name} ")
                        res.extend(self.search_by_doc_id(table_name, condition["doc_id"], limit, offset))
            return res        
        
        for indexName in indexNames:
            for knowledgebaseId in knowledgebaseIds:
                table_name = f"{indexName}_{knowledgebaseId}"
                logger.info(f"GaussConnection.search table_name {table_name} ")

                for matchExpr in matchExprs:
                    if isinstance(matchExpr, MatchTextExpr):
                        minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                        text_res = self.search_by_full_text(table_name, matchExpr.matching_text, minimum_should_match, matchExpr.topn)
                        logger.info(f"GaussConnection.search MatchTextExpr count: {len(text_res)} ")
                        res.extend(text_res)
                    elif isinstance(matchExpr, MatchDenseExpr):
                        similarity = matchExpr.extra_options.get("similarity", 0.0)
                        vector_res = self.search_by_vector(table_name, matchExpr.embedding_data, similarity, matchExpr.topn)
                        logger.info(f"GaussConnection.search MatchDenseExpr count: {len(vector_res)} ")
                        res.extend(vector_res)

        logger.info(f"GaussConnection.search success.")
        return res


    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        logger.info(f"GaussConnection.get {indexName} ")
        try:
            with self._get_cursor() as cur:
                cur.execute(f"SELECT * FROM {indexName} WHERE id = %s", (chunkId,))
                row = cur.fetchone()
                if row:
                    columns = [desc[0] for desc in cur.description]
                    result = dict(zip(columns, row))
                    return result
                return None
        except Exception as e:
            logger.error(f"Error in get operation: {str(e)}")
            return None


    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str = None) -> list[str]:
        if self.indexExist(indexName, knowledgebaseId) == False:
            self.createIdx(indexName, knowledgebaseId, 1024)

        table_name = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.insert {table_name} ")
        
        values = []
        for doc in documents:
            assert "id" in doc
            values.append(
                (
                    str(doc["id"]),
                    str(doc["kb_id"]),
                    str(doc["doc_id"]),
                    str(doc["docnm_kwd"]),
                    str(doc["title_tks"]),
                    str(doc["content_ltks"]),
                    str(doc["content_with_weight"]),
                    str(doc["q_1024_vec"]),
                )
            )
        try:
            with self._get_cursor() as cur:
                psycopg2.extras.execute_values(
                    cur, f"INSERT INTO {table_name} (id, kb_id, doc_id, docnm_kwd, title_tks, content_ltks, content_with_weight, embedding) VALUES %s", values
                )
        except Exception as e:
            logger.error(f"{str(e)}.")
            return []
        logger.info(f"GaussConnection inserted success.")
        return []

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.update {table_name} ")

        logger.info(f"GaussConnection.update condition {condition} ")
        logger.info(f"GaussConnection.update newValue {newValue} ")

        try:
            with self._get_cursor() as cur:
                return True
        except Exception as e:
            logger.error(f"GaussConnection.update ERROR: {str(e)}")
            return False

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        table_name = f"{indexName}_{knowledgebaseId}"
        logger.info(f"GaussConnection.delete {table_name} ")

        logger.info(f"GaussConnection.delete condition {condition} ")
        if "doc_id" not in condition:
            return 0

        try:
            with self._get_cursor() as cur:
                cur.execute(f"DELETE FROM {table_name} WHERE doc_id = %s", (condition["doc_id"],))
                return 0
        except Exception as e:
            logger.error(f"GaussConnection.delete ERROR: {str(e)}")
            return 0


    """
    Helper functions for search result
    """

    def getTotal(self, res):
        if isinstance(res, list):
            return len(res)
        return 0

    def getChunkIds(self, res):
        logger.info(f"GaussConnection.getTotal {len(res)} ")
        chunk_ids = []
        for item in res:
            chunk_ids.append(str(item["id"]))
        return chunk_ids

    def getFields(self, res, fields: list[str]) -> dict[str, dict]:
        res_fields = {}
        for item in res:
            chunk_id = str(item["id"])
            if isinstance(item, dict):
                res_fields.update({chunk_id: item})
        return res_fields

    def getHighlight(self, res, keywords: list[str], fieldnm: str):
        ans = {}
        return ans

    def getAggregation(self, res, fieldnm: str):
        return []

    """
    SQL
    """

    def sql(self, sql: str, fetch_size: int, format: str):
        logger.info(f"GaussConnection.sql get sql: {sql}")
        return None