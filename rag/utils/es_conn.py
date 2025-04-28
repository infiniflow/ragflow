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
from elasticsearch import Elasticsearch, NotFoundError
from elasticsearch_dsl import UpdateByQuery, Q, Search, Index
from elastic_transport import ConnectionTimeout
from rag import settings
from rag.settings import TAG_FLD, PAGERANK_FLD
from rag.utils import singleton, get_float
from api.utils.file_utils import get_project_base_directory
from rag.utils.doc_store_conn import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr, \
    FusionExpr
from rag.nlp import is_english, rag_tokenizer

ATTEMPT_TIME = 2

logger = logging.getLogger('ragflow.es_conn')


@singleton
class ESConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        logger.info(f"Use Elasticsearch {settings.ES['hosts']} as the doc engine.")
        for _ in range(ATTEMPT_TIME):
            try:
                self.es = Elasticsearch(
                    settings.ES["hosts"].split(","),
                    basic_auth=(settings.ES["username"], settings.ES[
                        "password"]) if "username" in settings.ES and "password" in settings.ES else None,
                    verify_certs=False,
                    timeout=600
                )
                if self.es:
                    self.info = self.es.info()
                    break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting Elasticsearch {settings.ES['hosts']} to be healthy.")
                time.sleep(5)
        if not self.es.ping():
            msg = f"Elasticsearch {settings.ES['hosts']} is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "8.17.4"})
        v = v["number"].split(".")[0]
        if int(v) < 8:
            msg = f"Elasticsearch version must be greater than or equal to 8, current version: {v}"
            logger.error(msg)
            raise Exception(msg)
        fp_mapping = os.path.join(get_project_base_directory(), "conf", "mapping.json")
        if not os.path.exists(fp_mapping):
            msg = f"Elasticsearch mapping file not found at {fp_mapping}"
            logger.error(msg)
            raise Exception(msg)
        self.mapping = json.load(open(fp_mapping, "r"))
        logger.info(f"Elasticsearch {settings.ES['hosts']} is healthy.")

    """
    Database operations
    """

    def dbType(self) -> str:
        return "elasticsearch"

    def health(self) -> dict:
        health_dict = dict(self.es.cluster.health())
        health_dict["type"] = "elasticsearch"
        return health_dict

    """
    Table operations
    """

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        if self.indexExist(indexName, knowledgebaseId):
            return True
        try: # 首次执行时根据配置文件conf/mapping.json创建索引,具体在此类的 __init__()方法中
            from elasticsearch.client import IndicesClient
            return IndicesClient(self.es).create(index=indexName,
                                                 settings=self.mapping["settings"],
                                                 mappings=self.mapping["mappings"])
        except Exception:
            logger.exception("ESConnection.createIndex error %s" % (indexName))

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        if len(knowledgebaseId) > 0:
            # The index need to be alive after any kb deletion since all kb under this tenant are in one index.
            return
        try:
            self.es.indices.delete(index=indexName, allow_no_indices=True)
        except NotFoundError:
            pass
        except Exception:
            logger.exception("ESConnection.deleteIdx error %s" % (indexName))

    def indexExist(self, indexName: str, knowledgebaseId: str = None) -> bool:
        s = Index(indexName, self.es)
        for i in range(ATTEMPT_TIME):
            try:
                return s.exists()
            except Exception as e:
                logger.exception("ESConnection.indexExist got exception")
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue
                break
        return False

    """
    CRUD operations
    """
    """
    Search
        结合了多种查询条件、排序、高亮显示、聚合等高级功能, 并且支持向量相似性查询
            selectFields: 需要返回的字段列表。
            highlightFields: 需要高亮显示的字段列表。
            condition: 过滤条件, 用于筛选文档。
            matchExprs: 匹配表达式列表, 用于定义如何匹配文档。
            orderBy: 排序表达式, 用于定义排序规则。
            offset: 分页偏移量。
            limit: 分页大小。
            indexNames: Elasticsearch索引名称, 可以是单个字符串或字符串列表。
            knowledgebaseIds: 知识库ID列表, 用于过滤文档。
            aggFields: 需要聚合的字段列表, 默认为空。
            rank_feature: 排名特征字典, 用于调整搜索结果的排名, 可选
    """
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
        """
        Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl.html
        """
        if isinstance(indexNames, str): # 如果indexNames是字符串，则按逗号分割成列表
            indexNames = indexNames.split(",") # 确保indexNames是一个非空列表
        assert isinstance(indexNames, list) and len(indexNames) > 0
        assert "_id" not in condition # 确保condition中不包含_id字段，因为_id通常用于文档的唯一标识，而不是作为过滤条件

        bqry = Q("bool", must=[])
        condition["kb_id"] = knowledgebaseIds
        for k, v in condition.items():
            if k == "available_int":
                if v == 0:
                    bqry.filter.append(Q("range", available_int={"lt": 1}))
                else:
                    bqry.filter.append(
                        Q("bool", must_not=Q("range", available_int={"lt": 1})))
                continue
            if not v: # 空跳过空条件
                continue
            if isinstance(v, list): # 列表，则添加一个terms查询
                bqry.filter.append(Q("terms", **{k: v}))
            elif isinstance(v, str) or isinstance(v, int): # 字符串或整数，则添加一个term查询
                bqry.filter.append(Q("term", **{k: v}))
            else:
                raise Exception(
                    f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")

        s = Search()
        vector_similarity_weight = 0.5
        for m in matchExprs:
            if isinstance(m, FusionExpr) and m.method == "weighted_sum" and "weights" in m.fusion_params:
                assert len(matchExprs) == 3 and isinstance(matchExprs[0], MatchTextExpr) and isinstance(matchExprs[1],
                                                                                                        MatchDenseExpr) and isinstance(
                    matchExprs[2], FusionExpr)
                weights = m.fusion_params["weights"]
                vector_similarity_weight = get_float(weights.split(",")[1])
        for m in matchExprs:
            if isinstance(m, MatchTextExpr): # 文本匹配
                minimum_should_match = m.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                bqry.must.append(Q("query_string", fields=m.fields,
                                   type="best_fields", query=m.matching_text,
                                   minimum_should_match=minimum_should_match,
                                   boost=1))
                bqry.boost = 1.0 - vector_similarity_weight

            elif isinstance(m, MatchDenseExpr): # 向量匹配
                assert (bqry is not None)
                similarity = 0.0
                if "similarity" in m.extra_options:
                    similarity = m.extra_options["similarity"]
                s = s.knn(m.vector_column_name,
                          m.topn,
                          m.topn * 2,
                          query_vector=list(m.embedding_data),
                          filter=bqry.to_dict(),
                          similarity=similarity,
                          )

        if bqry and rank_feature: # 排序特征
            for fld, sc in rank_feature.items():
                if fld != PAGERANK_FLD:
                    fld = f"{TAG_FLD}.{fld}"
                bqry.should.append(Q("rank_feature", field=fld, linear={}, boost=sc))

        if bqry: # 布尔查询到搜索对象
            s = s.query(bqry)
        for field in highlightFields: # 高亮字段
            s = s.highlight(field)

        if orderBy: # 排序
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

        for fld in aggFields: # 聚合字段
            s.aggs.bucket(f'aggs_{fld}', 'terms', field=fld, size=1000000)

        if limit > 0: # 分页
            s = s[offset:offset + limit]
        q = s.to_dict() # 将搜索对象转换为字典格式
        logger.debug(f"ESConnection.search {str(indexNames)} query: " + json.dumps(q))

        for i in range(ATTEMPT_TIME):
            try:
                #print(json.dumps(q, ensure_ascii=False))
                res = self.es.search(index=indexNames,
                                     body=q,
                                     timeout="600s",
                                     # search_type="dfs_query_then_fetch",
                                     track_total_hits=True,
                                     _source=True)
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                logger.debug(f"ESConnection.search {str(indexNames)} res: " + str(res))
                return res
            except Exception as e:
                logger.exception(f"ESConnection.search {str(indexNames)} query: " + str(q))
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logger.error("ESConnection.search timeout for 3 times!")
        raise Exception("ESConnection.search timeout.")

    """
        从Elasticsearch中检索指定的文档，并处理以下情况：
        成功获取文档：返回文档内容，并添加文档ID。
        文档不存在：返回 None。
        超时或网络问题：重试多次，直到成功或达到最大重试次数。
    """ 
    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.get(index=(indexName),
                                  id=chunkId, source=True, )
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                chunk = res["_source"]
                chunk["id"] = chunkId
                return chunk
            except NotFoundError:
                return None
            except Exception as e:
                logger.exception(f"ESConnection.get({chunkId}) got exception")
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logger.error("ESConnection.get timeout for 3 times!")
        raise Exception("ESConnection.get timeout.")
    
    """
    将多个文档批量插入到 Elasticsearch 索引中。它利用了 Elasticsearch 的 Bulk API 来高效地执行多个索引操作
    参数说明：
        documents：一个包含多个文档的列表，每个文档是一个字典。
        indexName：Elasticsearch 的索引名称，文档将被插入到该索引中。
        knowledgebaseId：可选参数，知识库ID，用于在文档中添加一个 kb_id 字段
    """
    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str = None) -> list[str]:
        # Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/docs-bulk.html
        operations = []
        for d in documents:
            assert "_id" not in d # 确保文档中没有 _id 字段（因为 _id 是 Elasticsearch 的保留字段）
            assert "id" in d  # 确保文档中有 id 字段（用于唯一标识文档）
            d_copy = copy.deepcopy(d)
            d_copy["kb_id"] = knowledgebaseId # knowledgebaseId 添加到文档中，作为 kb_id 字段
            meta_id = d_copy.pop("id", "") # 文档中移除 id 字段，并将其值存储到 meta_id 中
            operations.append(
                {"index": {"_index": indexName, "_id": meta_id}})
            operations.append(d_copy)

        res = [] # 初始化一个空列表 res，用于存储失败的文档ID及其错误信息
        for _ in range(ATTEMPT_TIME):
            try:
                res = []
                r = self.es.bulk(index=(indexName), operations=operations,
                                 refresh=False, timeout="60s")
                if re.search(r"False", str(r["errors"]), re.IGNORECASE):
                    return res

                for item in r["items"]: # 遍历响应中的 items 列表，检查每个操作的结果
                    for action in ["create", "delete", "index", "update"]:
                        if action in item and "error" in item[action]:
                            res.append(str(item[action]["_id"]) + ":" + str(item[action]["error"]))
                return res
            except Exception as e:
                res.append(str(e))
                logger.warning("ESConnection.insert got exception: " + str(e))
                res = []
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    res.append(str(e))
                    time.sleep(3)
                    continue
        return res
    
    """
        根据给定的条件和新值, 更新Elasticsearch中的文档。它支持两种更新方式:
        更新单个指定文档:通过文档ID进行更新, 支持移除特定字段或更新字段值。
        更新多个未指定文档:通过条件过滤文档, 支持添加、移除字段值或更新字段值
        condition: 字典,表示更新条件。
        newValue: 字典,表示要更新的新值。
        indexName: Elasticsearch的索引名称。
        knowledgebaseId: 知识库ID,用于过滤文档
    """
    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        doc = copy.deepcopy(newValue)
        doc.pop("id", None) # 从doc中移除id键（如果存在）, 因为id通常用于标识文档, 而不是更新内容
        condition["kb_id"] = knowledgebaseId
        if "id" in condition and isinstance(condition["id"], str):
            # update specific single document
            chunkId = condition["id"]
            for i in range(ATTEMPT_TIME):
                for k in doc.keys(): # 遍历doc中的所有键
                    if "feas" != k.split("_")[-1]: # 筛选出字段名以_feas结尾的字段, 只有这些字段需要被移除
                        continue
                    try:
                        self.es.update(index=indexName, id=chunkId, script=f"ctx._source.remove(\"{k}\");")
                    except Exception:
                        logger.exception(f"ESConnection.update(index={indexName}, id={chunkId}, doc={json.dumps(condition, ensure_ascii=False)}) got exception")
                try:
                    self.es.update(index=indexName, id=chunkId, doc=doc)
                    return True
                except Exception as e:
                    logger.exception(
                        f"ESConnection.update(index={indexName}, id={chunkId}, doc={json.dumps(condition, ensure_ascii=False)}) got exception")
                    if re.search(r"(timeout|connection)", str(e).lower()):
                        continue
                    break
            return False

        # update unspecific maybe-multiple documents
        bqry = Q("bool")
        for k, v in condition.items():
            if not isinstance(k, str) or not v:
                continue
            if k == "exists": # 则添加一个exists查询, 检查字段是否存在
                bqry.filter.append(Q("exists", field=v))
                continue
            if isinstance(v, list):
                bqry.filter.append(Q("terms", **{k: v}))
            elif isinstance(v, str) or isinstance(v, int): # 如果值是字符串或整数, 则添加一个term查询, 匹配字段的单个值
                bqry.filter.append(Q("term", **{k: v}))
            else:
                raise Exception(
                    f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")
        scripts = []
        params = {}
        for k, v in newValue.items():
            if k == "remove":
                if isinstance(v, str):
                    scripts.append(f"ctx._source.remove('{v}');")
                if isinstance(v, dict):
                    for kk, vv in v.items():
                        scripts.append(f"int i=ctx._source.{kk}.indexOf(params.p_{kk});ctx._source.{kk}.remove(i);")
                        params[f"p_{kk}"] = vv
                continue
            if k == "add":
                if isinstance(v, dict):
                    for kk, vv in v.items():
                        scripts.append(f"ctx._source.{kk}.add(params.pp_{kk});")
                        params[f"pp_{kk}"] = vv.strip()
                continue
            if (not isinstance(k, str) or not v) and k != "available_int":
                continue
            if isinstance(v, str):
                v = re.sub(r"(['\n\r]|\\.)", " ", v)
                params[f"pp_{k}"] = v
                scripts.append(f"ctx._source.{k}=params.pp_{k};")
            elif isinstance(v, int) or isinstance(v, float):
                scripts.append(f"ctx._source.{k}={v};")
            elif isinstance(v, list):
                scripts.append(f"ctx._source.{k}=params.pp_{k};")
                params[f"pp_{k}"] = json.dumps(v, ensure_ascii=False)
            else:
                raise Exception(
                    f"newValue `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str.")
        ubq = UpdateByQuery(
            index=indexName).using(
            self.es).query(bqry)
        ubq = ubq.script(source="".join(scripts), params=params)
        ubq = ubq.params(refresh=True)
        ubq = ubq.params(slices=5)
        ubq = ubq.params(conflicts="proceed")

        for _ in range(ATTEMPT_TIME):
            try:
                _ = ubq.execute()
                return True
            except Exception as e:
                logger.error("ESConnection.update got exception: " + str(e) + "\n".join(scripts))
                if re.search(r"(timeout|connection|conflict)", str(e).lower()):
                    continue
                break
        return False

    """
        从 Elasticsearch 索引中删除满足特定条件的文档。它利用了 Elasticsearch 的 delete_by_query API 来实现这一功能
    """
    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        qry = None
        assert "_id" not in condition # 确保条件字典中没有 _id 字段
        condition["kb_id"] = knowledgebaseId
        if "id" in condition:
            chunk_ids = condition["id"]
            if not isinstance(chunk_ids, list): # 单值转换为列表
                chunk_ids = [chunk_ids]
            if not chunk_ids:  # when chunk_ids is empty, delete all
                qry = Q("match_all")
            else:
                qry = Q("ids", values=chunk_ids)
        else:
            qry = Q("bool")
            for k, v in condition.items():
                if k == "exists":
                    qry.filter.append(Q("exists", field=v))

                elif k == "must_not":
                    if isinstance(v, dict):
                        for kk, vv in v.items():
                            if kk == "exists":
                                qry.must_not.append(Q("exists", field=vv))

                elif isinstance(v, list):
                    qry.must.append(Q("terms", **{k: v}))
                elif isinstance(v, str) or isinstance(v, int):
                    qry.must.append(Q("term", **{k: v}))
                else:
                    raise Exception("Condition value must be int, str or list.")
        logger.debug("ESConnection.delete query: " + json.dumps(qry.to_dict()))
        for _ in range(ATTEMPT_TIME):
            try:
                #print(Search().query(qry).to_dict(), flush=True)
                res = self.es.delete_by_query(
                    index=indexName,
                    body=Search().query(qry).to_dict(),
                    refresh=True)
                return res["deleted"]
            except Exception as e:
                logger.warning("ESConnection.delete got exception: " + str(e))
                if re.search(r"(timeout|connection)", str(e).lower()):
                    time.sleep(3)
                    continue
                if re.search(r"(not_found)", str(e), re.IGNORECASE):
                    return 0
        return 0

    """
    Helper functions for search result
    """

    def getTotal(self, res):
        if isinstance(res["hits"]["total"], type({})):
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
                # if n.find("tks") > 0:
                #     m[n] = rmSpace(m[n])

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

    """
    SQL
    """

    def sql(self, sql: str, fetch_size: int, format: str):
        logger.debug(f"ESConnection.sql get sql: {sql}")
        sql = re.sub(r"[ `]+", " ", sql)
        sql = sql.replace("%", "")
        replaces = []
        for r in re.finditer(r" ([a-z_]+_l?tks)( like | ?= ?)'([^']+)'", sql):
            fld, v = r.group(1), r.group(3)
            match = " MATCH({}, '{}', 'operator=OR;minimum_should_match=30%') ".format(
                fld, rag_tokenizer.fine_grained_tokenize(rag_tokenizer.tokenize(v)))
            replaces.append(
                ("{}{}'{}'".format(
                    r.group(1),
                    r.group(2),
                    r.group(3)),
                 match))

        for p, r in replaces:
            sql = sql.replace(p, r, 1)
        logger.debug(f"ESConnection.sql to es: {sql}")

        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.sql.query(body={"query": sql, "fetch_size": fetch_size}, format=format,
                                        request_timeout="2s")
                return res
            except ConnectionTimeout:
                logger.exception("ESConnection.sql timeout")
                continue
            except Exception:
                logger.exception("ESConnection.sql got exception")
                return None
        logger.error("ESConnection.sql timeout for 3 times!")
        return None
