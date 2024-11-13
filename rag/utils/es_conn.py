import re
import json
import time
import os
from typing import List, Dict

import elasticsearch
import copy
from elasticsearch import Elasticsearch
from elasticsearch_dsl import UpdateByQuery, Q, Search, Index
from elastic_transport import ConnectionTimeout
from api.utils.log_utils import logger
from rag import settings
from rag.utils import singleton
from api.utils.file_utils import get_project_base_directory
import polars as pl
from rag.utils.doc_store_conn import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr, FusionExpr
from rag.nlp import is_english, rag_tokenizer

logger.info("Elasticsearch sdk version: "+str(elasticsearch.__version__))


@singleton
class ESConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        for _ in range(10):
            try:
                self.es = Elasticsearch(
                    settings.ES["hosts"].split(","),
                    basic_auth=(settings.ES["username"], settings.ES["password"]) if "username" in settings.ES and "password" in settings.ES else None,
                    verify_certs=False,
                    timeout=600
                )
                if self.es:
                    self.info = self.es.info()
                    logger.info("Connect to es.")
                    break
            except Exception:
                logger.exception("Fail to connect to es")
                time.sleep(1)
        if not self.es.ping():
            raise Exception("Can't connect to ES cluster")
        v = self.info.get("version", {"number": "5.6"})
        v = v["number"].split(".")[0]
        if int(v) < 8:
            raise Exception(f"ES version must be greater than or equal to 8, current version: {v}")
        fp_mapping = os.path.join(get_project_base_directory(), "conf", "mapping.json")
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        self.mapping = json.load(open(fp_mapping, "r"))

    """
    Database operations
    """
    def dbType(self) -> str:
        return "elasticsearch"

    def health(self) -> dict:
        return dict(self.es.cluster.health()) + {"type": "elasticsearch"}

    """
    Table operations
    """
    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        if self.indexExist(indexName, knowledgebaseId):
            return True
        try:
            from elasticsearch.client import IndicesClient
            return IndicesClient(self.es).create(index=indexName,
                                                 settings=self.mapping["settings"],
                                                 mappings=self.mapping["mappings"])
        except Exception:
            logger.exception("ES create index error %s" % (indexName))

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        try:
            return self.es.indices.delete(indexName, allow_no_indices=True)
        except Exception:
            logger.exception("ES delete index error %s" % (indexName))

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        s = Index(indexName, self.es)
        for i in range(3):
            try:
                return s.exists()
            except Exception as e:
                logger.exception("ES indexExist")
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue
        return False

    """
    CRUD operations
    """
    def search(self, selectFields: list[str], highlightFields: list[str], condition: dict, matchExprs: list[MatchExpr], orderBy: OrderByExpr, offset: int, limit: int, indexNames: str|list[str], knowledgebaseIds: list[str]) -> list[dict] | pl.DataFrame:
        """
        Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl.html
        """
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        assert "_id" not in condition
        s = Search()
        bqry = None
        vector_similarity_weight = 0.5
        for m in matchExprs:
            if isinstance(m, FusionExpr) and m.method=="weighted_sum" and "weights" in m.fusion_params:
                assert len(matchExprs)==3 and isinstance(matchExprs[0], MatchTextExpr) and isinstance(matchExprs[1], MatchDenseExpr) and isinstance(matchExprs[2], FusionExpr)
                weights = m.fusion_params["weights"]
                vector_similarity_weight = float(weights.split(",")[1])
        for m in matchExprs:
            if isinstance(m, MatchTextExpr):
                minimum_should_match = "0%"
                if "minimum_should_match" in m.extra_options:
                    minimum_should_match = str(int(m.extra_options["minimum_should_match"] * 100)) + "%"
                bqry = Q("bool",
                            must=Q("query_string", fields=m.fields,
                                type="best_fields", query=m.matching_text,
                                minimum_should_match = minimum_should_match,
                                boost=1),
                            boost = 1.0 - vector_similarity_weight,
                        )
                if condition:
                    for k, v in condition.items():
                        if not isinstance(k, str) or not v:
                            continue
                        if isinstance(v, list):
                            bqry.filter.append(Q("terms", **{k: v}))
                        elif isinstance(v, str) or isinstance(v, int):
                            bqry.filter.append(Q("term", **{k: v}))
                        else:
                            raise Exception(f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")
            elif isinstance(m, MatchDenseExpr):
                assert(bqry is not None)
                similarity = 0.0
                if "similarity" in m.extra_options:
                    similarity = m.extra_options["similarity"]
                s = s.knn(m.vector_column_name,
                    m.topn,
                    m.topn * 2,
                    query_vector = list(m.embedding_data),
                    filter = bqry.to_dict(),
                    similarity = similarity,
                )
        if matchExprs:
            s.query = bqry
        for field in highlightFields:
            s = s.highlight(field)

        if orderBy:
            orders = list()
            for field, order in orderBy.fields:
                order = "asc" if order == 0 else "desc"
                orders.append({field: {"order": order, "unmapped_type": "float",
                                 "mode": "avg", "numeric_type": "double"}})
            s = s.sort(*orders)

        if limit > 0:
            s = s[offset:limit]
        q = s.to_dict()
        # logger.info("ESConnection.search [Q]: " + json.dumps(q))

        for i in range(3):
            try:
                res = self.es.search(index=indexNames,
                                     body=q,
                                     timeout="600s",
                                     # search_type="dfs_query_then_fetch",
                                     track_total_hits=True,
                                     _source=True)
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                logger.info("ESConnection.search res: " + str(res))
                return res
            except Exception as e:
                logger.exception("ES search [Q]: " + str(q))
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logger.error("ES search timeout for 3 times!")
        raise Exception("ES search timeout.")

    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        for i in range(3):
            try:
                res = self.es.get(index=(indexName),
                                id=chunkId, source=True,)
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                if not res.get("found"):
                    return None
                chunk = res["_source"]
                chunk["id"] = chunkId
                return chunk
            except Exception as e:
                logger.exception(f"ES get({chunkId}) got exception")
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logger.error("ES search timeout for 3 times!")
        raise Exception("ES search timeout.")

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str) -> list[str]:
        # Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/docs-bulk.html
        operations = []
        for d in documents:
            assert "_id" not in d
            assert "id" in d
            d_copy = copy.deepcopy(d)
            meta_id = d_copy["id"]
            del d_copy["id"]
            operations.append(
                {"index": {"_index": indexName, "_id": meta_id}})
            operations.append(d_copy)

        res = []
        for _ in range(100):
            try:
                r = self.es.bulk(index=(indexName), operations=operations,
                                     refresh=False, timeout="600s")
                if re.search(r"False", str(r["errors"]), re.IGNORECASE):
                    return res

                for item in r["items"]:
                    for action in ["create", "delete", "index", "update"]:
                        if action in item and "error" in item[action]:
                            res.append(str(item[action]["_id"]) + ":" + str(item[action]["error"]))
                return res
            except Exception as e:
                logger.warning("Fail to bulk: " + str(e))
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    time.sleep(3)
                    continue
        return res

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        doc = copy.deepcopy(newValue)
        del doc['id']
        if "id" in condition and isinstance(condition["id"], str):
            # update specific single document
            chunkId = condition["id"]
            for i in range(3):
                try:
                    self.es.update(index=indexName, id=chunkId, doc=doc)
                    return True
                except Exception as e:
                    logger.exception(f"ES failed to update(index={indexName}, id={id}, doc={json.dumps(condition, ensure_ascii=False)})")
                    if str(e).find("Timeout") > 0:
                        continue
        else:
            # update unspecific maybe-multiple documents
            bqry = Q("bool")
            for k, v in condition.items():
                if not isinstance(k, str) or not v:
                    continue
                if isinstance(v, list):
                    bqry.filter.append(Q("terms", **{k: v}))
                elif isinstance(v, str) or isinstance(v, int):
                    bqry.filter.append(Q("term", **{k: v}))
                else:
                    raise Exception(f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")
            scripts = []
            for k, v in newValue.items():
                if not isinstance(k, str) or not v:
                    continue
                if isinstance(v, str):
                    scripts.append(f"ctx._source.{k} = '{v}'")
                elif isinstance(v, int):
                    scripts.append(f"ctx._source.{k} = {v}")
                else:
                    raise Exception(f"newValue `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str.")
            ubq = UpdateByQuery(
                index=indexName).using(
                self.es).query(bqry)
            ubq = ubq.script(source="; ".join(scripts))
            ubq = ubq.params(refresh=True)
            ubq = ubq.params(slices=5)
            ubq = ubq.params(conflicts="proceed")
            for i in range(3):
                try:
                    _ = ubq.execute()
                    return True
                except Exception as e:
                    logger.error("ES update exception: " + str(e) + "[Q]:" + str(bqry.to_dict()))
                    if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                        continue
        return False

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        qry = None
        assert "_id" not in condition
        if "id" in condition:
            chunk_ids = condition["id"]
            if not isinstance(chunk_ids, list):
                chunk_ids = [chunk_ids]
            qry = Q("ids", values=chunk_ids)
        else:
            qry = Q("bool")
            for k, v in condition.items():
                if isinstance(v, list):
                    qry.must.append(Q("terms", **{k: v}))
                elif isinstance(v, str) or isinstance(v, int):
                    qry.must.append(Q("term", **{k: v}))
                else:
                    raise Exception("Condition value must be int, str or list.")
        logger.info("ESConnection.delete [Q]: " + json.dumps(qry.to_dict()))
        for _ in range(10):
            try:
                res = self.es.delete_by_query(
                    index=indexName,
                    body = Search().query(qry).to_dict(),
                    refresh=True)
                return res["deleted"]
            except Exception as e:
                logger.warning("Fail to delete: " + str(filter) + str(e))
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
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

    def getFields(self, res, fields: List[str]) -> Dict[str, dict]:
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

    def getHighlight(self, res, keywords: List[str], fieldnm: str):
        ans = {}
        for d in res["hits"]["hits"]:
            hlts = d.get("highlight")
            if not hlts:
                continue
            txt = "...".join([a for a in list(hlts.items())[0][1]])
            if not is_english(txt.split(" ")):
                ans[d["_id"]] = txt
                continue

            txt = d["_source"][fieldnm]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE|re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                for w in keywords:
                    t = re.sub(r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])"%re.escape(w), r"\1<em>\2</em>\3", t, flags=re.IGNORECASE|re.MULTILINE)
                if not re.search(r"<em>[^<>]+</em>", t, flags=re.IGNORECASE|re.MULTILINE):
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
        logger.info(f"ESConnection.sql get sql: {sql}")
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
        logger.info(f"ESConnection.sql to es: {sql}")

        for i in range(3):
            try:
                res = self.es.sql.query(body={"query": sql, "fetch_size": fetch_size}, format=format, request_timeout="2s")
                return res
            except ConnectionTimeout:
                logger.exception("ESConnection.sql timeout [Q]: " + sql)
                continue
            except Exception:
                logger.exception("ESConnection.sql got exception [Q]: " + sql)
                return None
        logger.error("ESConnection.sql timeout for 3 times!")
        return None
