import re
import logging
import json
import time
import copy
import elasticsearch
from elasticsearch import Elasticsearch
from elasticsearch_dsl import UpdateByQuery, Search, Index
from util import config

print("Elasticsearch version: ", elasticsearch.__version__)


def instance(env):
    CF = config.init(env)
    ES_DRESS = CF.get("es").split(",")

    ES = Elasticsearch(
        ES_DRESS,
        timeout=600
    )

    print("ES: ", ES_DRESS, ES.info())

    return ES


class HuEs:
    def __init__(self, env):
        self.env = env
        self.info = {}
        self.config = config.init(env)
        self.conn()
        self.idxnm = self.config.get("idx_nm")
        if not self.es.ping():
            raise Exception("Can't connect to ES cluster")

    def conn(self):
        for _ in range(10):
            try:
                c = instance(self.env)
                if c:
                    self.es = c
                    self.info = c.info()
                    logging.info("Connect to es.")
                    break
            except Exception as e:
                logging.error("Fail to connect to es: " + str(e))

    def version(self):
        v = self.info.get("version", {"number": "5.6"})
        v = v["number"].split(".")[0]
        return int(v) >= 7

    def upsert(self, df, idxnm=""):
        res = []
        for d in df:
            id = d["id"]
            del d["id"]
            d = {"doc": d, "doc_as_upsert": "true"}
            T = False
            for _ in range(10):
                try:
                    if not self.version():
                        r = self.es.update(
                            index=(
                                self.idxnm if not idxnm else idxnm),
                            body=d,
                            id=id,
                            doc_type="doc",
                            refresh=False,
                            retry_on_conflict=100)
                    else:
                        r = self.es.update(
                            index=(
                                self.idxnm if not idxnm else idxnm),
                            body=d,
                            id=id,
                            refresh=False,
                            doc_type="_doc",
                            retry_on_conflict=100)
                    logging.info("Successfully upsert: %s" % id)
                    T = True
                    break
                except Exception as e:
                    logging.warning("Fail to index: " +
                                    json.dumps(d, ensure_ascii=False) + str(e))
                    if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                        time.sleep(3)
                        continue
                    self.conn()
                    T = False

            if not T:
                res.append(d)
                logging.error(
                    "Fail to index: " +
                    re.sub(
                        "[\r\n]",
                        "",
                        json.dumps(
                            d,
                            ensure_ascii=False)))
                d["id"] = id
                d["_index"] = self.idxnm

        if not res:
            return True
        return False

    def bulk(self, df, idx_nm=None):
        ids, acts = {}, []
        for d in df:
            id = d["id"] if "id" in d else d["_id"]
            ids[id] = copy.deepcopy(d)
            ids[id]["_index"] = self.idxnm if not idx_nm else idx_nm
            if "id" in d:
                del d["id"]
            if "_id" in d:
                del d["_id"]
            acts.append(
                {"update": {"_id": id, "_index": ids[id]["_index"]}, "retry_on_conflict": 100})
            acts.append({"doc": d, "doc_as_upsert": "true"})
            logging.info("bulk upsert: %s" % id)

        res = []
        for _ in range(100):
            try:
                if elasticsearch.__version__[0] < 8:
                    r = self.es.bulk(
                        index=(
                            self.idxnm if not idx_nm else idx_nm),
                        body=acts,
                        refresh=False,
                        timeout="600s")
                else:
                    r = self.es.bulk(index=(self.idxnm if not idx_nm else
                                            idx_nm), operations=acts,
                                     refresh=False, timeout="600s")
                if re.search(r"False", str(r["errors"]), re.IGNORECASE):
                    return res

                for it in r["items"]:
                    if "error" in it["update"]:
                        res.append(str(it["update"]["_id"]) +
                                   ":" + str(it["update"]["error"]))

                return res
            except Exception as e:
                logging.warn("Fail to bulk: " + str(e))
                print(e)
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    time.sleep(3)
                    continue
                self.conn()

        return res

    def bulk4script(self, df):
        ids, acts = {}, []
        for d in df:
            id = d["id"]
            ids[id] = copy.deepcopy(d["raw"])
            acts.append({"update": {"_id": id, "_index": self.idxnm}})
            acts.append(d["script"])
            logging.info("bulk upsert: %s" % id)

        res = []
        for _ in range(10):
            try:
                if not self.version():
                    r = self.es.bulk(
                        index=self.idxnm,
                        body=acts,
                        refresh=False,
                        timeout="600s",
                        doc_type="doc")
                else:
                    r = self.es.bulk(
                        index=self.idxnm,
                        body=acts,
                        refresh=False,
                        timeout="600s")
                if re.search(r"False", str(r["errors"]), re.IGNORECASE):
                    return res

                for it in r["items"]:
                    if "error" in it["update"]:
                        res.append(str(it["update"]["_id"]))

                return res
            except Exception as e:
                logging.warning("Fail to bulk: " + str(e))
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    time.sleep(3)
                    continue
                self.conn()

        return res

    def rm(self, d):
        for _ in range(10):
            try:
                if not self.version():
                    r = self.es.delete(
                        index=self.idxnm,
                        id=d["id"],
                        doc_type="doc",
                        refresh=True)
                else:
                    r = self.es.delete(
                        index=self.idxnm,
                        id=d["id"],
                        refresh=True,
                        doc_type="_doc")
                logging.info("Remove %s" % d["id"])
                return True
            except Exception as e:
                logging.warn("Fail to delete: " + str(d) + str(e))
                if re.search(r"(Timeout|time out)", str(e), re.IGNORECASE):
                    time.sleep(3)
                    continue
                if re.search(r"(not_found)", str(e), re.IGNORECASE):
                    return True
                self.conn()

        logging.error("Fail to delete: " + str(d))

        return False

    def search(self, q, idxnm=None, src=False, timeout="2s"):
        print(json.dumps(q, ensure_ascii=False))
        for i in range(3):
            try:
                res = self.es.search(index=(self.idxnm if not idxnm else idxnm),
                                     body=q,
                                     timeout=timeout,
                                     # search_type="dfs_query_then_fetch",
                                     track_total_hits=True,
                                     _source=src)
                if str(res.get("timed_out", "")).lower() == "true":
                    raise Exception("Es Timeout.")
                return res
            except Exception as e:
                logging.error(
                    "ES search exception: " +
                    str(e) +
                    "【Q】：" +
                    str(q))
                if str(e).find("Timeout") > 0:
                    continue
                raise e
        logging.error("ES search timeout for 3 times!")
        raise Exception("ES search timeout.")

    def updateByQuery(self, q, d):
        ubq = UpdateByQuery(index=self.idxnm).using(self.es).query(q)
        scripts = ""
        for k, v in d.items():
            scripts += "ctx._source.%s = params.%s;" % (str(k), str(k))
        ubq = ubq.script(source=scripts, params=d)
        ubq = ubq.params(refresh=False)
        ubq = ubq.params(slices=5)
        ubq = ubq.params(conflicts="proceed")
        for i in range(3):
            try:
                r = ubq.execute()
                return True
            except Exception as e:
                logging.error("ES updateByQuery exception: " +
                              str(e) + "【Q】：" + str(q.to_dict()))
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue

        return False

    def deleteByQuery(self, query, idxnm=""):
        for i in range(3):
            try:
                r = self.es.delete_by_query(
                    index=idxnm if idxnm else self.idxnm,
                    body=Search().query(query).to_dict())
                return True
            except Exception as e:
                logging.error("ES updateByQuery deleteByQuery: " +
                              str(e) + "【Q】：" + str(query.to_dict()))
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue

        return False

    def update(self, id, script, routing=None):
        for i in range(3):
            try:
                if not self.version():
                    r = self.es.update(
                        index=self.idxnm,
                        id=id,
                        body=json.dumps(
                            script,
                            ensure_ascii=False),
                        doc_type="doc",
                        routing=routing,
                        refresh=False)
                else:
                    r = self.es.update(index=self.idxnm, id=id, body=json.dumps(script, ensure_ascii=False),
                                       routing=routing, refresh=False)  # , doc_type="_doc")
                return True
            except Exception as e:
                print(e)
                logging.error("ES update exception: " + str(e) + " id：" + str(id) + ", version:" + str(self.version()) +
                              json.dumps(script, ensure_ascii=False))
                if str(e).find("Timeout") > 0:
                    continue

        return False

    def indexExist(self, idxnm):
        s = Index(idxnm if idxnm else self.idxnm, self.es)
        for i in range(3):
            try:
                return s.exists()
            except Exception as e:
                logging.error("ES updateByQuery indexExist: " + str(e))
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue

        return False

    def docExist(self, docid, idxnm=None):
        for i in range(3):
            try:
                return self.es.exists(index=(idxnm if idxnm else self.idxnm),
                                      id=docid)
            except Exception as e:
                logging.error("ES Doc Exist: " + str(e))
                if str(e).find("Timeout") > 0 or str(e).find("Conflict") > 0:
                    continue
        return False

    def createIdx(self, idxnm, mapping):
        try:
            if elasticsearch.__version__[0] < 8:
                return self.es.indices.create(idxnm, body=mapping)
            from elasticsearch.client import IndicesClient
            return IndicesClient(self.es).create(index=idxnm,
                                                 settings=mapping["settings"],
                                                 mappings=mapping["mappings"])
        except Exception as e:
            logging.error("ES create index error %s ----%s" % (idxnm, str(e)))

    def deleteIdx(self, idxnm):
        try:
            return self.es.indices.delete(idxnm, allow_no_indices=True)
        except Exception as e:
            logging.error("ES delete index error %s ----%s" % (idxnm, str(e)))

    def getTotal(self, res):
        if isinstance(res["hits"]["total"], type({})):
            return res["hits"]["total"]["value"]
        return res["hits"]["total"]

    def getDocIds(self, res):
        return [d["_id"] for d in res["hits"]["hits"]]

    def getSource(self, res):
        rr = []
        for d in res["hits"]["hits"]:
            d["_source"]["id"] = d["_id"]
            d["_source"]["_score"] = d["_score"]
            rr.append(d["_source"])
        return rr

    def scrollIter(self, pagesize=100, scroll_time='2m', q={
        "query": {"match_all": {}}, "sort": [{"updated_at": {"order": "desc"}}]}):
        for _ in range(100):
            try:
                page = self.es.search(
                    index=self.idxnm,
                    scroll=scroll_time,
                    size=pagesize,
                    body=q,
                    _source=None
                )
                break
            except Exception as e:
                logging.error("ES scrolling fail. " + str(e))
                time.sleep(3)

        sid = page['_scroll_id']
        scroll_size = page['hits']['total']["value"]
        logging.info("[TOTAL]%d" % scroll_size)
        # Start scrolling
        while scroll_size > 0:
            yield page["hits"]["hits"]
            for _ in range(100):
                try:
                    page = self.es.scroll(scroll_id=sid, scroll=scroll_time)
                    break
                except Exception as e:
                    logging.error("ES scrolling fail. " + str(e))
                    time.sleep(3)

            # Update the scroll ID
            sid = page['_scroll_id']
            # Get the number of results that we returned in the last scroll
            scroll_size = len(page['hits']['hits'])
