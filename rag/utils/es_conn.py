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
from rag.utils import singleton
from api.utils.file_utils import get_project_base_directory
import polars as pl
from rag.utils.doc_store_conn import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr, \
    FusionExpr
from rag.nlp import is_english, rag_tokenizer

ATTEMPT_TIME = 2


@singleton
class ESConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        logging.info(f"Use Elasticsearch {settings.ES['hosts']} as the doc engine.")
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
                logging.warning(f"{str(e)}. Waiting Elasticsearch {settings.ES['hosts']} to be healthy.")
                time.sleep(5)
        if not self.es.ping():
            msg = f"Elasticsearch {settings.ES['hosts']} didn't become healthy in 120s."
            logging.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "8.11.3"})
        v = v["number"].split(".")[0]
        if int(v) < 8:
            msg = f"Elasticsearch version must be greater than or equal to 8, current version: {v}"
            logging.error(msg)
            raise Exception(msg)
        fp_mapping = os.path.join(get_project_base_directory(), "conf", "mapping.json")
        if not os.path.exists(fp_mapping):
            msg = f"Elasticsearch mapping file not found at {fp_mapping}"
            logging.error(msg)
            raise Exception(msg)
        self.mapping = json.load(open(fp_mapping, "r"))
        logging.info(f"Elasticsearch {settings.ES['hosts']} is healthy.")

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
        try:
            from elasticsearch.client import IndicesClient
            return IndicesClient(self.es).create(index=indexName,
                                                 settings=self.mapping["settings"],
                                                 mappings=self.mapping["mappings"])
        except Exception:
            logging.exception("ESConnection.createIndex error %s" % (indexName))

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        try:
            self.es.indices.delete(index=indexName, allow_no_indices=True)
        except NotFoundError:
            pass
        except Exception:
            logging.exception("ESConnection.deleteIdx error %s" % (indexName))

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        s = Index(indexName, self.es)
        for i in range(ATTEMPT_TIME):
            try:
                return s.exists()
            except Exception as e:
                logging.exception("ESConnection.indexExist got exception")
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue
        return False

    """
    CRUD operations
    """

    def search(self, selectFields: list[str], highlightFields: list[str], condition: dict, matchExprs: list[MatchExpr],
               orderBy: OrderByExpr, offset: int, limit: int, indexNames: str | list[str],
               knowledgebaseIds: list[str]) -> list[dict] | pl.DataFrame:
        """
        Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl.html
        """
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        assert "_id" not in condition

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
            if not v: continue
            if isinstance(v, list):
                bqry.filter.append(Q("terms", **{k: v}))
            elif isinstance(v, str) or isinstance(v, int):
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
                vector_similarity_weight = float(weights.split(",")[1])
        for m in matchExprs:
            if isinstance(m, MatchTextExpr):
                minimum_should_match = "0%"
                if "minimum_should_match" in m.extra_options:
                    minimum_should_match = str(int(m.extra_options["minimum_should_match"] * 100)) + "%"
                bqry.must.append(Q("query_string", fields=m.fields,
                                   type="best_fields", query=m.matching_text,
                                   minimum_should_match=minimum_should_match,
                                   boost=1))
                bqry.boost = 1.0 - vector_similarity_weight

            elif isinstance(m, MatchDenseExpr):
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

        if bqry:
            s = s.query(bqry)
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
        logging.debug(f"ESConnection.search {str(indexNames)} query: " + json.dumps(q))

        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.search(index=indexNames,
                                     body=q,
                                     timeout="600s",
                                     # search_type="dfs_query_then_fetch",
                                     track_total_hits=True,
                                     _source=True)
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                logging.debug(f"ESConnection.search {str(indexNames)} res: " + str(res))
                return res
            except Exception as e:
                logging.exception(f"ESConnection.search {str(indexNames)} query: " + str(q))
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logging.error("ESConnection.search timeout for 3 times!")
        raise Exception("ESConnection.search timeout.")

    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.get(index=(indexName),
                                  id=chunkId, source=True, )
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                if not res.get("found"):
                    return None
                chunk = res["_source"]
                chunk["id"] = chunkId
                return chunk
            except Exception as e:
                logging.exception(f"ESConnection.get({chunkId}) got exception")
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logging.error("ESConnection.get timeout for 3 times!")
        raise Exception("ESConnection.get timeout.")

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str) -> list[str]:
        # Refers to https://www.elastic.co/guide/en/elasticsearch/reference/current/docs-bulk.html
        operations = []
        for d in documents:
            assert "_id" not in d
            assert "id" in d
            d_copy = copy.deepcopy(d)
            meta_id = d_copy.pop("id", "")
            operations.append(
                {"index": {"_index": indexName, "_id": meta_id}})
            operations.append(d_copy)

        res = []
        for _ in range(ATTEMPT_TIME):
            try:
                res = []
                r = self.es.bulk(index=(indexName), operations=operations,
                                 refresh=False, timeout="60s")
                if re.search(r"False", str(r["errors"]), re.IGNORECASE):
                    return res

                for item in r["items"]:
                    for action in ["create", "delete", "index", "update"]:
                        if action in item and "error" in item[action]:
                            res.append(str(item[action]["_id"]) + ":" + str(item[action]["error"]))
                return res
            except Exception as e:
                res.append(str(e))
                logging.warning("ESConnection.insert got exception: " + str(e))
                res = []
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    res.append(str(e))
                    time.sleep(3)
                    continue
        return res

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        doc = copy.deepcopy(newValue)
        doc.pop("id", None)
        if "id" in condition and isinstance(condition["id"], str):
            # update specific single document
            chunkId = condition["id"]
            for i in range(ATTEMPT_TIME):
                try:
                    self.es.update(index=indexName, id=chunkId, doc=doc)
                    return True
                except Exception as e:
                    logging.exception(
                        f"ESConnection.update(index={indexName}, id={id}, doc={json.dumps(condition, ensure_ascii=False)}) got exception")
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
                    raise Exception(
                        f"Condition `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str or list.")
            scripts = []
            for k, v in newValue.items():
                if not isinstance(k, str) or not v:
                    continue
                if isinstance(v, str):
                    scripts.append(f"ctx._source.{k} = '{v}'")
                elif isinstance(v, int):
                    scripts.append(f"ctx._source.{k} = {v}")
                else:
                    raise Exception(
                        f"newValue `{str(k)}={str(v)}` value type is {str(type(v))}, expected to be int, str.")
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
                    logging.error("ESConnection.update got exception: " + str(e))
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
        logging.debug("ESConnection.delete query: " + json.dumps(qry.to_dict()))
        for _ in range(ATTEMPT_TIME):
            try:
                res = self.es.delete_by_query(
                    index=indexName,
                    body=Search().query(qry).to_dict(),
                    refresh=True)
                return res["deleted"]
            except Exception as e:
                logging.warning("ESConnection.delete got exception: " + str(e))
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
            if not is_english(txt.split(" ")):
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
        logging.debug(f"ESConnection.sql get sql: {sql}")
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
        logging.debug(f"ESConnection.sql to es: {sql}")

        for i in range(ATTEMPT_TIME):
            try:
                res = self.es.sql.query(body={"query": sql, "fetch_size": fetch_size}, format=format,
                                        request_timeout="2s")
                return res
            except ConnectionTimeout:
                logging.exception("ESConnection.sql timeout")
                continue
            except Exception:
                logging.exception("ESConnection.sql got exception")
                return None
        logging.error("ESConnection.sql timeout for 3 times!")
        return None
