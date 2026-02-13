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

import re
import json
import time
import logging
import os

import copy
from opensearchpy import OpenSearch, NotFoundError
from opensearchpy import UpdateByQuery, Q, Search, Index
from opensearchpy import ConnectionTimeout
from opensearchpy.client import IndicesClient
from common.decorator import singleton
from common.file_utils import get_project_base_directory
from common.doc_store.doc_store_base import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr, FusionExpr
from common.float_utils import get_float
from common.constants import PAGERANK_FLD, TAG_FLD
from rag.nlp.rag_tokenizer import tokenize, fine_grained_tokenize
from common import settings

ATTEMPT_TIME = 2
logger = logging.getLogger('ragflow.memory_opensearch_conn')


@singleton
class OSConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        logger.info(f"Use OpenSearch {settings.OS['hosts']} as the memory engine.")
        for _ in range(ATTEMPT_TIME):
            try:
                self.os = OpenSearch(
                    settings.OS["hosts"].split(","),
                    http_auth=(settings.OS["username"], settings.OS[
                        "password"]) if "username" in settings.OS and "password" in settings.OS else None,
                    verify_certs=False,
                    timeout=600
                )
                if self.os:
                    self.info = self.os.info()
                    break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting OpenSearch {settings.OS['hosts']} to be healthy.")
                time.sleep(5)
        if not self.os.ping():
            msg = f"OpenSearch {settings.OS['hosts']} is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "2.18.0"})
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
        with open(fp_mapping, "r") as f:
            self.mapping = json.load(f)
        logger.info(f"OpenSearch {settings.OS['hosts']} is healthy.")

    def _connect(self):
        if self.os.ping():
            return True
        try:
            self.os = OpenSearch(
                settings.OS["hosts"].split(","),
                http_auth=(settings.OS["username"], settings.OS[
                    "password"]) if "username" in settings.OS and "password" in settings.OS else None,
                verify_certs=False,
                timeout=600
            )
            return True
        except Exception:
            return False

    """
    Database operations
    """

    def db_type(self) -> str:
        return "opensearch"

    def health(self) -> dict:
        health_dict = dict(self.os.cluster.health())
        health_dict["type"] = "opensearch"
        return health_dict

    """
    Table operations
    """

    def create_idx(self, indexName: str, knowledgebaseId: str, vectorSize: int, parser_id: str = None):
        if self.index_exist(indexName, knowledgebaseId):
            return True
        try:
            return IndicesClient(self.os).create(index=indexName,
                                                 body=self.mapping)
        except Exception:
            logger.exception(f"OSConnection.create_idx({indexName}) got exception")
            return False

    def delete_idx(self, indexName: str, knowledgebaseId: str):
        try:
            self.os.indices.delete(index=indexName, allow_no_indices=True)
            return True
        except Exception:
            logger.exception(f"OSConnection.delete_idx({indexName}) got exception")
            return False

    def index_exist(self, indexName: str, knowledgebaseId: str = None) -> bool:
        try:
            return self.os.indices.exists(index=indexName)
        except Exception:
            logger.exception(f"OSConnection.index_exist({indexName}) got exception")
            return False

    @staticmethod
    def convert_field_name(field_name: str, use_tokenized_content=False) -> str:
        match field_name:
            case "message_type":
                return "message_type_kwd"
            case "status":
                return "status_int"
            case "content":
                if use_tokenized_content:
                    return "tokenized_content_ltks"
                return "content_ltks"
            case _:
                return field_name

    @staticmethod
    def map_message_to_os_fields(message: dict) -> dict:
        storage_doc = {
            "id": message.get("id"),
            "message_id": message["message_id"],
            "message_type_kwd": message["message_type"],
            "source_id": message["source_id"],
            "memory_id": message["memory_id"],
            "user_id": message["user_id"],
            "agent_id": message["agent_id"],
            "session_id": message["session_id"],
            "valid_at": message["valid_at"],
            "invalid_at": message["invalid_at"],
            "forget_at": message["forget_at"],
            "status_int": 1 if message["status"] else 0,
            "zone_id": message.get("zone_id", 0),
            "content_ltks": message["content"],
            "tokenized_content_ltks": fine_grained_tokenize(tokenize(message["content"])),
            f"q_{len(message['content_embed'])}_vec": message["content_embed"],
        }
        return storage_doc

    @staticmethod
    def get_message_from_os_doc(doc: dict) -> dict:
        embd_field_name = next((key for key in doc.keys() if re.match(r"q_\d+_vec", key)), None)
        message = {
            "message_id": doc["message_id"],
            "message_type": doc["message_type_kwd"],
            "source_id": doc["source_id"] if doc["source_id"] else None,
            "memory_id": doc["memory_id"],
            "user_id": doc.get("user_id", ""),
            "agent_id": doc["agent_id"],
            "session_id": doc["session_id"],
            "zone_id": doc.get("zone_id", 0),
            "valid_at": doc["valid_at"],
            "invalid_at": doc.get("invalid_at", "-"),
            "forget_at": doc.get("forget_at", "-"),
            "status": bool(int(doc["status_int"])),
            "content": doc.get("content_ltks", ""),
            "content_embed": doc.get(embd_field_name, []) if embd_field_name else [],
        }
        if doc.get("id"):
            message["id"] = doc["id"]
        return message

    """
    CRUD operations
    """

    def search(
            self, select_fields: list[str],
            highlight_fields: list[str],
            condition: dict,
            match_expressions: list[MatchExpr],
            order_by: OrderByExpr,
            offset: int,
            limit: int,
            index_names: str | list[str],
            memory_ids: list[str],
            agg_fields: list[str] | None = None,
            rank_feature: dict | None = None,
            hide_forgotten: bool = True
    ):
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        assert "_id" not in condition

        exist_index_list = [idx for idx in index_names if self.index_exist(idx)]
        if not exist_index_list:
            return None, 0

        bool_query = Q("bool", must=[], must_not=[])
        if hide_forgotten:
            bool_query.must_not.append(Q("exists", field="forget_at"))

        condition["memory_id"] = memory_ids
        for k, v in condition.items():
            field_name = self.convert_field_name(k)
            if field_name == "session_id" and v:
                bool_query.filter.append(Q("query_string", **{"query": f"*{v}*", "fields": ["session_id"], "analyze_wildcard": True}))
                continue
            if not v:
                continue
            if isinstance(v, list):
                bool_query.filter.append(Q("terms", **{field_name: v}))
            elif isinstance(v, str) or isinstance(v, int):
                bool_query.filter.append(Q("term", **{field_name: v}))
            else:
                raise Exception(
                    f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")
        
        s = Search()
        vector_similarity_weight = 0.5
        for m in match_expressions:
            if isinstance(m, FusionExpr) and m.method == "weighted_sum" and "weights" in m.fusion_params:
                weights = m.fusion_params["weights"]
                vector_similarity_weight = get_float(weights.split(",")[1])
        
        for m in match_expressions:
            if isinstance(m, MatchTextExpr):
                minimum_should_match = m.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                bool_query.must.append(Q("query_string", fields=[self.convert_field_name(f, use_tokenized_content=True) for f in m.fields],
                                   type="best_fields", query=m.matching_text,
                                   minimum_should_match=minimum_should_match,
                                   boost=1))
                bool_query.boost = 1.0 - vector_similarity_weight

            elif isinstance(m, MatchDenseExpr):
                assert (bool_query is not None)
                similarity = 0.0
                if "similarity" in m.extra_options:
                    similarity = m.extra_options["similarity"]
                
                # KNN in OpenSearch
                knn_query = {
                    "knn": {
                        self.convert_field_name(m.vector_column_name): {
                            "vector": list(m.embedding_data),
                            "k": m.topn,
                            "filter": bool_query.to_dict() if bool_query else None
                        }
                    }
                }
                # Search object in opensearch-py handles the dict directly or we can use DSL
                s = s.query(knn_query)

        if bool_query and not any(isinstance(m, MatchDenseExpr) for m in match_expressions):
            s = s.query(bool_query)
            
        for field in highlight_fields:
            s = s.highlight(field)

        if order_by:
            orders = list()
            for field, order in order_by.fields:
                order = "asc" if order == 0 else "desc"
                if field.endswith("_int") or field.endswith("_flt"):
                    order_info = {"order": order, "unmapped_type": "float"}
                else:
                    order_info = {"order": order, "unmapped_type": "text"}
                orders.append({field: order_info})
            s = s.sort(*orders)

        if agg_fields:
            for fld in agg_fields:
                s.aggs.bucket(f'aggs_{fld}', 'terms', field=fld, size=1000000)

        if limit > 0:
            s = s[offset:offset + limit]
        
        q = s.to_dict()
        logger.debug(f"OSConnection.search {str(exist_index_list)} query: " + json.dumps(q))

        for i in range(ATTEMPT_TIME):
            try:
                res = self.os.search(index=exist_index_list,
                                     body=q,
                                     timeout="600s")
                return res, self.get_total(res)
            except ConnectionTimeout:
                logger.exception("OpenSearch request timeout")
                self._connect()
                continue
            except NotFoundError:
                return None, 0
            except Exception as e:
                logger.exception(f"OSConnection.search {str(index_names)} query: {json.dumps(q)} got exception")
                raise e

        raise Exception("OSConnection.search timeout.")

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int=512):
        bool_query = Q("bool", must=[])
        bool_query.must.append(Q("exists", field="forget_at"))
        bool_query.filter.append(Q("term", memory_id=memory_id))
        order_by = OrderByExpr()
        order_by.asc("forget_at")
        s = Search().query(bool_query)
        orders = list()
        for field, order in order_by.fields:
            order = "asc" if order == 0 else "desc"
            if field.endswith("_int") or field.endswith("_flt"):
                order_info = {"order": order, "unmapped_type": "float"}
            else:
                order_info = {"order": order, "unmapped_type": "text"}
            orders.append({field: order_info})
        s = s.sort(*orders)
        s = s[:limit]
        q = s.to_dict()
        for i in range(ATTEMPT_TIME):
            try:
                res = self.os.search(index=index_name, body=q, timeout="600s")
                return res
            except ConnectionTimeout:
                logger.exception("OpenSearch request timeout")
                self._connect()
                continue
            except NotFoundError:
                return None
            except Exception as e:
                logger.exception(f"OSConnection.get_forgotten_messages query: {json.dumps(q)} got exception")
                raise e
        raise Exception("OSConnection.get_forgotten_messages timeout.")

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int=512):
        if not self.index_exist(index_name):
            return None
        bool_query = Q("bool", must=[])
        bool_query.must.append(Q("term", memory_id=memory_id))
        bool_query.must_not.append(Q("exists", field=field_name))
        order_by = OrderByExpr()
        order_by.asc("valid_at")
        s = Search().query(bool_query)
        orders = list()
        for field, order in order_by.fields:
            order = "asc" if order == 0 else "desc"
            if field.endswith("_int") or field.endswith("_flt"):
                order_info = {"order": order, "unmapped_type": "float"}
            else:
                order_info = {"order": order, "unmapped_type": "text"}
            orders.append({field: order_info})
        s = s.sort(*orders)
        s = s[:limit]
        q = s.to_dict()
        for i in range(ATTEMPT_TIME):
            try:
                res = self.os.search(index=index_name, body=q, timeout="600s")
                return res
            except ConnectionTimeout:
                logger.exception("OpenSearch request timeout")
                self._connect()
                continue
            except NotFoundError:
                return None
            except Exception as e:
                logger.exception(f"OSConnection.get_missing_field_message query: {json.dumps(q)} got exception")
                raise e
        raise Exception("OSConnection.get_missing_field_message timeout.")

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        for i in range(ATTEMPT_TIME):
            try:
                res = self.os.get(index=index_name, id=doc_id)
                message = res["_source"]
                message["id"] = doc_id
                return self.get_message_from_os_doc(message)
            except NotFoundError:
                return None
            except Exception as e:
                logger.exception(f"OSConnection.get({doc_id}) got exception")
                raise e
        raise Exception("OSConnection.get timeout.")

    def insert(self, documents: list[dict], index_name: str, memory_id: str = None) -> list[str]:
        operations = []
        for d in documents:
            assert "_id" not in d
            assert "id" in d
            d_copy = self.map_message_to_os_fields(copy.deepcopy(d))
            d_copy["memory_id"] = memory_id
            meta_id = d_copy.pop("id", "")
            operations.append({"index": {"_index": index_name, "_id": meta_id}})
            operations.append(d_copy)
        
        res = []
        for _ in range(ATTEMPT_TIME):
            try:
                r = self.os.bulk(index=index_name, body=operations, refresh=False, timeout="60s")
                if not r.get("errors"):
                    return res
                for item in r["items"]:
                    for action in ["create", "delete", "index", "update"]:
                        if action in item and "error" in item[action]:
                            res.append(str(item[action]["_id"]) + ":" + str(item[action]["error"]))
                return res
            except ConnectionTimeout:
                logger.exception("OpenSearch request timeout")
                time.sleep(3)
                self._connect()
                continue
            except Exception as e:
                res.append(str(e))
                logger.warning("OSConnection.insert got exception: " + str(e))
        return res

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        doc = copy.deepcopy(new_value)
        update_dict = {self.convert_field_name(k): v for k, v in doc.items()}
        if "content_ltks" in update_dict:
            update_dict["tokenized_content_ltks"] = fine_grained_tokenize(tokenize(update_dict["content_ltks"]))
        update_dict.pop("id", None)
        
        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        condition_dict["memory_id"] = memory_id
        
        if "id" in condition_dict and isinstance(condition_dict["id"], str):
            message_id = condition_dict["id"]
            try:
                self.os.update(index=index_name, id=message_id, body={"doc": update_dict})
                return True
            except Exception as e:
                logger.exception(f"OSConnection.update(index={index_name}, id={message_id}) got exception")
                return False

        bool_query = Q("bool")
        for k, v in condition_dict.items():
            if not isinstance(k, str) or not v: continue
            if k == "exists": bool_query.filter.append(Q("exists", field=v))
            elif isinstance(v, list): bool_query.filter.append(Q("terms", **{k: v}))
            elif isinstance(v, str) or isinstance(v, int): bool_query.filter.append(Q("term", **{k: v}))
        
        scripts = []
        params = {}
        for k, v in update_dict.items():
            if k == "remove": continue # Simplified for now
            if k == "add": continue # Simplified for now
            if (not isinstance(k, str) or not v) and k != "status_int": continue
            
            if isinstance(v, str):
                v = re.sub(r"(['\n\r]|\\.)", " ", v)
                params[f"pp_{k}"] = v
                scripts.append(f"ctx._source.{k}=params.pp_{k};")
            elif isinstance(v, (int, float)):
                scripts.append(f"ctx._source.{k}={v};")
            elif isinstance(v, list):
                scripts.append(f"ctx._source.{k}=params.pp_{k};")
                params[f"pp_{k}"] = v

        ubq = UpdateByQuery(index=index_name).using(self.os).query(bool_query)
        ubq = ubq.script(source="".join(scripts), params=params)
        try:
            ubq.execute()
            return True
        except Exception as e:
            logger.error(f"OSConnection.update got exception: {e}")
            return False

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        condition_dict["memory_id"] = memory_id
        
        if "id" in condition_dict:
            message_ids = condition_dict["id"]
            if not isinstance(message_ids, list): message_ids = [message_ids]
            qry = Q("ids", values=message_ids) if message_ids else Q("match_all")
        else:
            qry = Q("bool")
            for k, v in condition_dict.items():
                if k == "exists": qry.filter.append(Q("exists", field=v))
                elif isinstance(v, list): qry.must.append(Q("terms", **{k: v}))
                elif isinstance(v, str) or isinstance(v, int): qry.must.append(Q("term", **{k: v}))
        
        try:
            res = self.os.delete_by_query(index=index_name, body=Search().query(qry).to_dict(), refresh=True)
            return res.get("deleted", 0)
        except Exception as e:
            logger.warning(f"OSConnection.delete got exception: {e}")
            return 0

    def get_total(self, res):
        if isinstance(res["hits"]["total"], dict):
            return res["hits"]["total"]["value"]
        return res["hits"]["total"]

    def _get_source(self, res):
        if res and "hits" in res:
            for d in res["hits"]["hits"]:
                d["_source"]["id"] = d["_id"]
                yield d["_source"]

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        res_fields = {}
        if not fields: return {}
        for doc in self._get_source(res):
            message = self.get_message_from_os_doc(doc)
            m = {n: v for n, v in message.items() if n in fields}
            if m: res_fields[doc["id"]] = m
        return res_fields
