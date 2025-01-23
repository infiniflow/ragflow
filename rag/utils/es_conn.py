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
import copy
from opensearchpy import OpenSearch, NotFoundError
from opensearchpy import Index
from elastic_transport import ConnectionTimeout
from rag import settings
from rag.settings import TAG_FLD, PAGERANK_FLD
from rag.utils import singleton
from api.utils.file_utils import get_project_base_directory
import polars as pl
from rag.utils.doc_store_conn import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr
from rag.nlp import is_english

ATTEMPT_TIME = 2

logger = logging.getLogger('ragflow.es_conn')


@singleton
class OSConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        logger.info(f"Use OpenSearch {settings.ES['hosts']} as the doc engine.")
        for _ in range(ATTEMPT_TIME):
            try:
                self.es = OpenSearch(
                    hosts=["https://vpc-opensearch-ai-cog-use01-inriygymoxoihklcebbajsljhu.us-east-1.es.amazonaws.com"],
                    use_ssl=True,
                    verify_certs=False,
                    timeout=600,
                )
                if self.es:
                    self.info = self.es.info()
                    break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting for OpenSearch {settings.ES['hosts']} to be healthy.")
                time.sleep(5)
        if not self.es.ping():
            msg = f"OpenSearch {settings.ES['hosts']} is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "2.11.0"})
        v = v["number"].split(".")[0]
        if int(v) < 2:
            msg = f"OpenSearch version must be greater than or equal to 2, current version: {v}"
            logger.error(msg)
            raise Exception(msg)
        fp_mapping = os.path.join(get_project_base_directory(), "conf", "mapping.json")
        if not os.path.exists(fp_mapping):
            msg = f"OpenSearch mapping file not found at {fp_mapping}"
            logger.error(msg)
            raise Exception(msg)
        self.mapping = json.load(open(fp_mapping, "r"))
        logger.info(f"OpenSearch {settings.ES['hosts']} is healthy.")

    def dbType(self) -> str:
        return "opensearch"

    def health(self) -> dict:
        health_dict = dict(self.es.cluster.health())
        health_dict["type"] = "opensearch"
        return health_dict

    def createIdx(self, indexName: str, knowledgebaseId: str, embedding_dim: int):
        """
        Ensure the index is created with the correct mapping using both the provided mapping
        and KNN settings. If the index exists, delete it and recreate.
        """
        try:
            from opensearchpy.client.indices import IndicesClient

            logger.info(f"Creating index '{indexName}' with combined settings and KNN mappings.")
            index_body = {
                "settings": {
                    **self.mapping.get("settings", {}),
                    "index": {
                        "knn": True,
                        "knn.algo_param.ef_search": 100
                    },
                },
                "mappings": {
                    **self.mapping.get("mappings", {}),
                    "properties": {
                        **self.mapping["mappings"].get("properties", {}),
                        "vector_field": {  # Adjust field name as per your context
                            "type": "knn_vector",
                            "dimension": embedding_dim,
                            "method": {
                                "name": "hnsw",
                                "space_type": "l2",
                                "engine": "nmslib",
                                "parameters": {
                                    "ef_construction": 128,
                                    "m": 24
                                }
                            },
                        },
                    },
                },
            }

            IndicesClient(self.es).create(index=indexName, body=index_body)
            logger.info(f"Index '{indexName}' created successfully.")
            return True

        except Exception as e:
            logger.exception(f"Failed to create index '{indexName}': {e}")
            return False

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        if len(knowledgebaseId) > 0:
            return
        try:
            self.es.indices.delete(index=indexName, allow_no_indices=True)
        except NotFoundError:
            pass
        except Exception:
            logger.exception("ESConnection.deleteIdx error %s" % (indexName))

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        s = Index(indexName, self.es)
        for i in range(ATTEMPT_TIME):
            try:
                return s.exists()
            except Exception as e:
                logger.exception("ESConnection.indexExist got exception")
                if "Timeout" in str(e) or "Conflict" in str(e):
                    continue
                break
        return False

    def search(self, selectFields: list[str], highlightFields: list[str], condition: dict, matchExprs: list[MatchExpr],
            orderBy: OrderByExpr, offset: int, limit: int, indexNames: str | list[str],
            knowledgebaseIds: list[str]) -> list[dict] | pl.DataFrame:
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        assert "_id" not in condition

        must_clauses = []
        filter_clauses = []

        condition["kb_id"] = knowledgebaseIds
        for k, v in condition.items():
            if k == "available_int":
                if v == 0:
                    filter_clauses.append({"range": {"available_int": {"lt": 1}}})
                else:
                    filter_clauses.append({"bool": {"must_not": {"range": {"available_int": {"lt": 1}}}}})
                continue
            if not v:
                continue
            if isinstance(v, list):
                filter_clauses.append({"terms": {k: v}})
            elif isinstance(v, (str, int)):
                filter_clauses.append({"term": {k: v}})
            else:
                raise Exception(f"Condition `{str(k)}={str(v)}` has unexpected type {type(v)}.")

        knn_query = None
        for m in matchExprs:
            if isinstance(m, MatchTextExpr):
                must_clauses.append({
                    "query_string": {
                        "fields": m.fields,
                        "query": m.matching_text,
                        "minimum_should_match": m.extra_options.get("minimum_should_match", "0%"),
                        "boost": 1
                    }
                })
            elif isinstance(m, MatchDenseExpr):
                knn_query = {
                    "knn": {
                        m.vector_column_name: {
                            "vector": list(m.embedding_data),
                            "k": m.topn
                        }
                    }
                }

        query = {"query": {"bool": {}}}
        if must_clauses:
            query["query"]["bool"]["must"] = must_clauses
        if filter_clauses:
            query["query"]["bool"]["filter"] = filter_clauses

        if knn_query:
            query["query"] = knn_query

        if highlightFields:
            for field in highlightFields:
                s = s.highlight(field)
            query["highlight"] = {"fields": {field: {} for field in highlightFields}}

        if orderBy:
            orders = list()
            for field, order in orderBy.fields:
                order = "asc" if order == 0 else "desc"
                if field in ["page_num_int", "top_int"]:
                    order_info = {"order": order, "unmapped_type": "float",
                                  "mode": "avg", "numeric_type": "double"}
                elif field.endswith("_int") or field.endswith("_flt"):
                    order_info = {"order": order, "unmapped_type": "float"}
                else:
                    order_info = {"order": order, "unmapped_type": "text"}
                orders.append({field: order_info})
            s = s.sort(*orders)

            query["sort"] = [{field: {"order": "asc" if order == 0 else "desc", "unmapped_type": "float"}}
                            for field, order in orderBy.fields]

        if limit > 0:
            s = s[offset:offset + limit]
        q = s.to_dict()
        logger.debug(f"ESConnection.search {str(indexNames)} query: " + json.dumps(q))
        query["from"] = offset
        query["size"] = limit

        logger.debug(f"Generated Query: {json.dumps(query, indent=2)}")

        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.search(index=",".join(indexNames), body=query, timeout=600, track_total_hits=True, _source=True)
                return res
            except Exception as e:
                logger.exception(f"ESConnection.search {str(indexNames)} query failed.")
                if "Timeout" in str(e):
                    continue
                raise e




    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.get(index=indexName, id=chunkId, _source=True)
                return res["_source"]
            except NotFoundError:
                return None
            except Exception as e:
                logger.exception(f"ESConnection.get({chunkId}) encountered an error.")
                if "Timeout" in str(e):
                    continue

    @staticmethod
    def transform_dict(input_dict):
        transformed_dict = {
            "vector_field": input_dict.get("vector_field"),
            "text": input_dict.get("text"),
        }
        
        metadata = {k: v for k, v in input_dict.items() if k not in ["vector_field", "text"]}
        if metadata:
            transformed_dict["metadata"] = metadata
        
        return transformed_dict

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str) -> list[str]:
        operations = []
        for doc in documents:
            assert "_id" not in doc
            assert "id" in doc
            
            doc_copy = copy.deepcopy(doc)
            doc_copy["source"] = doc_copy.pop("docnm_kwd")
            doc_copy = self.transform_dict(doc_copy)
            print('doc_copy: ', doc_copy)
            
            meta_id = doc_copy.get("id")
            
            operations.append(
                {"index": {"_index": indexName, "_id": meta_id}}
            )
            operations.append(doc_copy)

        for attempt in range(ATTEMPT_TIME):
            try:
                response = self.es.bulk(
                    body=operations,
                    refresh=False,
                    timeout=60
                )

                if not response["errors"]:
                    return []

                errors = [
                    f'{item["index"]["_id"]}: {item["index"]["error"]}'
                    for item in response["items"]
                    if "error" in item["index"]
                ]
                return errors
            except Exception as e:
                logger.warning(f"ESConnection.insert encountered an error: {e}")
                time.sleep(3)
        return ["Failed to insert after multiple attempts"]

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        doc = copy.deepcopy(newValue)
        doc.pop("id", None)

        if "id" in condition and isinstance(condition["id"], str):
            chunkId = condition["id"]
            for i in range(ATTEMPT_TIME):
                try:
                    self.es.update(index=indexName, id=chunkId, body={"doc": doc})
                    return True
                except Exception as e:
                    logger.exception(
                        f"ESConnection.update(index={indexName}, id={chunkId}, doc={json.dumps(condition, ensure_ascii=False)}) got exception"
                    )
                    if "Timeout" in str(e):
                        continue
                    break
            return False
        else:
            query = {"bool": {"must": []}}

            for k, v in condition.items():
                if not isinstance(k, str) or not v:
                    continue
                if k == "exist":
                    query["must"].append({"exists": {"field": v}})
                    continue
                if isinstance(v, list):
                    query["must"].append({"terms": {k: v}})
                elif isinstance(v, (str, int)):
                    query["must"].append({"term": {k: v}})
                else:
                    raise Exception(
                        f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str, or list."
                    )

            scripts = []
            for k, v in newValue.items():
                if k == "remove":
                    scripts.append(f"ctx._source.remove('{v}');")
                    continue
                if not isinstance(k, str) or not v:
                    continue
                if isinstance(v, str):
                    scripts.append(f"ctx._source.{k} = '{v}'")
                elif isinstance(v, int):
                    scripts.append(f"ctx._source.{k} = {v}")
                else:
                    raise Exception(
                        f"newValue `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int or str."
                    )

            body = {
                "script": {
                    "source": "; ".join(scripts),
                    "lang": "painless"
                },
                "query": query
            }

            for i in range(ATTEMPT_TIME):
                try:
                    res = self.es.update_by_query(index=indexName, body=body, refresh=True, conflicts="proceed")
                    return res.get("updated", 0) > 0
                except Exception as e:
                    logger.exception(f"ESConnection.update_by_query got exception: {str(e)}")
                    if "Timeout" in str(e):
                        continue
            return False


    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        query = None

        if "id" in condition:
            chunk_ids = condition["id"]
            if not isinstance(chunk_ids, list):
                chunk_ids = [chunk_ids]
            query = {"ids": {"values": chunk_ids}}
        else:
            query = {"bool": {"must": []}}
            for k, v in condition.items():
                if isinstance(v, list):
                    query["bool"]["must"].append({"terms": {k: v}})
                elif isinstance(v, (str, int)):
                    query["bool"]["must"].append({"term": {k: v}})
                else:
                    raise Exception("Condition value must be int, str, or list.")

        body = {"query": query}

        for _ in range(ATTEMPT_TIME):
            try:
                res = self.es.delete_by_query(index=indexName, body=body, refresh=True)
                return res["deleted"]
            except Exception as e:
                logger.warning(f"ESConnection.delete got exception: {e}")
                if "Timeout" in str(e):
                    time.sleep(3)
                    continue
                if "not_found" in str(e).lower():
                    return 0
        return 0


    def getTotal(self, res):
        if isinstance(res["hits"]["total"], dict):
            return res["hits"]["total"]["value"]
        return res["hits"]["total"]


    def getChunkIds(self, res):
        return [d["_id"] for d in res["hits"]["hits"]]


    def __getSource(self, res):
        rr = []
        for d in res["hits"]["hits"]:
            d["_source"]["id"] = d["_id"]
            d["_source"]["_score"] = d["_score"]
            rr.append(d["_source"])
        return rr


    def getFields(self, res, fields: list[str]) -> dict[str, dict]:
        res_fields = {}
        if not fields:
            return {}
        for d in self.__getSource(res):
            m = {n: d.get(n) for n in fields if d.get(n) is not None}
            for n, v in m.items():
                if isinstance(v, list):
                    m[n] = v
                    continue
                if not isinstance(v, str):
                    m[n] = str(m[n])
            if m:
                res_fields[d["id"]] = m
        return res_fields


    def getHighlight(self, res, keywords: list[str], fieldnm: str):
        ans = {}
        for d in res["hits"]["hits"]:
            hlts = d.get("highlight")
            if not hlts:
                continue
            txt = "...".join([a for a in list(hlts.items())[0][1]])
            if not is_english(txt.split()):
                ans[d["_id"]] = txt
                continue

            txt = d["_source"][fieldnm]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                for w in keywords:
                    t = re.sub(r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])" % re.escape(w), r"\1<em>\2</em>\3", t,
                            flags=re.IGNORECASE | re.MULTILINE)
                if not re.search(r"<em>[^<>]+</em>", t, flags=re.IGNORECASE | re.MULTILINE):
                    continue
                txts.append(t)
            ans[d["_id"]] = "...".join(txts) if txts else "...".join([a for a in list(hlts.items())[0][1]])
        return ans


    def getAggregation(self, res, fieldnm: str):
        agg_field = "aggs_" + fieldnm
        if "aggregations" not in res or agg_field not in res["aggregations"]:
            return list()
        bkts = res["aggregations"][agg_field]["buckets"]
        return [(b["key"], b["doc_count"]) for b in bkts]


    def sql(self, sql: str, fetch_size: int, format: str):
        logger.debug(f"ESConnection.sql get sql: {sql}")
        sql = re.sub(r"[ `]+", " ", sql)
        sql = sql.replace("%", "")
        body = {"query": sql, "fetch_size": fetch_size}

        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.sql.query(body=body, format=format, request_timeout=2)
                return res
            except ConnectionTimeout:
                logger.exception("ESConnection.sql timeout")
                continue
            except Exception:
                logger.exception("ESConnection.sql got exception")
                return None
        logger.error("ESConnection.sql timeout for 3 times!")
        return None

