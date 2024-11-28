import logging
import os
import re
import json
import time
import infinity
from infinity.common import ConflictType, InfinityException, SortType
from infinity.index import IndexInfo, IndexType
from infinity.connection_pool import ConnectionPool
from infinity.errors import ErrorCode
from rag import settings
from rag.utils import singleton
import polars as pl
from polars.series.series import Series
from api.utils.file_utils import get_project_base_directory

from rag.utils.doc_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)


def equivalent_condition_to_str(condition: dict) -> str:
    assert "_id" not in condition
    cond = list()
    for k, v in condition.items():
        if not isinstance(k, str) or not v:
            continue
        if isinstance(v, list):
            inCond = list()
            for item in v:
                if isinstance(item, str):
                    inCond.append(f"'{item}'")
                else:
                    inCond.append(str(item))
            if inCond:
                strInCond = ", ".join(inCond)
                strInCond = f"{k} IN ({strInCond})"
                cond.append(strInCond)
        elif isinstance(v, str):
            cond.append(f"{k}='{v}'")
        else:
            cond.append(f"{k}={str(v)}")
    return " AND ".join(cond)


def concat_dataframes(df_list: list[pl.DataFrame], selectFields: list[str]) -> pl.DataFrame:
    """
    Concatenate multiple dataframes into one.
    """
    if df_list:
        return pl.concat(df_list)
    schema = dict()
    for fieldnm in selectFields:
        schema[fieldnm] = str
    return pl.DataFrame(schema=schema)

@singleton
class InfinityConnection(DocStoreConnection):
    def __init__(self):
        self.dbName = settings.INFINITY.get("db_name", "default_db")
        infinity_uri = settings.INFINITY["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            infinity_uri = infinity.common.NetworkAddress(host, int(port))
        self.connPool = None
        logging.info(f"Use Infinity {infinity_uri} as the doc engine.")
        for _ in range(24):
            try:
                connPool = ConnectionPool(infinity_uri)
                inf_conn = connPool.get_conn()
                res = inf_conn.show_current_node()
                connPool.release_conn(inf_conn)
                self.connPool = connPool
                if res.error_code == ErrorCode.OK and res.server_status=="started":
                    break
                logging.warn(f"Infinity status: {res.server_status}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
            except Exception as e:
                logging.warning(f"{str(e)}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
        if self.connPool is None:
            msg = f"Infinity {infinity_uri} didn't become healthy in 120s."
            logging.error(msg)
            raise Exception(msg)
        logging.info(f"Infinity {infinity_uri} is healthy.")

    """
    Database operations
    """

    def dbType(self) -> str:
        return "infinity"

    def health(self) -> dict:
        """
        Return the health status of the database.
        TODO: Infinity-sdk provides health() to wrap `show global variables` and `show tables`
        """
        inf_conn = self.connPool.get_conn()
        res = inf_conn.show_current_node()
        self.connPool.release_conn(inf_conn)
        res2 = {
            "type": "infinity",
            "status": "green" if res.error_code == 0 and res.server_status == "started" else "red",
            "error": res.error_msg,
        }
        return res2

    """
    Table operations
    """

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        table_name = f"{indexName}_{knowledgebaseId}"
        inf_conn = self.connPool.get_conn()
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)

        fp_mapping = os.path.join(
            get_project_base_directory(), "conf", "infinity_mapping.json"
        )
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        schema = json.load(open(fp_mapping))
        vector_name = f"q_{vectorSize}_vec"
        schema[vector_name] = {"type": f"vector,{vectorSize},float"}
        inf_table = inf_db.create_table(
            table_name,
            schema,
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "q_vec_idx",
            IndexInfo(
                vector_name,
                IndexType.Hnsw,
                {
                    "M": "16",
                    "ef_construction": "50",
                    "metric": "cosine",
                    "encode": "lvq",
                },
            ),
            ConflictType.Ignore,
        )
        text_suffix = ["_tks", "_ltks", "_kwd"]
        for field_name, field_info in schema.items():
            if field_info["type"] != "varchar":
                continue
            for suffix in text_suffix:
                if field_name.endswith(suffix):
                    inf_table.create_index(
                        f"text_idx_{field_name}",
                        IndexInfo(
                            field_name, IndexType.FullText, {"ANALYZER": "standard"}
                        ),
                        ConflictType.Ignore,
                    )
                    break
        self.connPool.release_conn(inf_conn)
        logging.info(
            f"INFINITY created table {table_name}, vector size {vectorSize}"
        )

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        table_name = f"{indexName}_{knowledgebaseId}"
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        db_instance.drop_table(table_name, ConflictType.Ignore)
        self.connPool.release_conn(inf_conn)
        logging.info(f"INFINITY dropped table {table_name}")

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            inf_conn = self.connPool.get_conn()
            db_instance = inf_conn.get_database(self.dbName)
            _ = db_instance.get_table(table_name)
            self.connPool.release_conn(inf_conn)
            return True
        except Exception as e:
            logging.warning(f"INFINITY indexExist {str(e)}")
        return False

    """
    CRUD operations
    """

    def search(
            self,
            selectFields: list[str],
            highlightFields: list[str],
            condition: dict,
            matchExprs: list[MatchExpr],
            orderBy: OrderByExpr,
            offset: int,
            limit: int,
            indexNames: str | list[str],
            knowledgebaseIds: list[str],
    ) -> list[dict] | pl.DataFrame:
        """
        TODO: Infinity doesn't provide highlight
        """
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        table_list = list()
        if "id" not in selectFields:
            selectFields.append("id")

        # Prepare expressions common to all tables
        filter_cond = ""
        filter_fulltext = ""
        if condition:
            filter_cond = equivalent_condition_to_str(condition)
        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                if len(filter_cond) != 0 and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_cond})
                fields = ",".join(matchExpr.fields)
                filter_fulltext = (
                    f"filter_fulltext('{fields}', '{matchExpr.matching_text}')"
                )
                if len(filter_cond) != 0:
                    filter_fulltext = f"({filter_cond}) AND {filter_fulltext}"
                logging.debug(f"filter_fulltext: {filter_fulltext}")
                minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    str_minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
            elif isinstance(matchExpr, MatchDenseExpr):
                if len(filter_cond) != 0 and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_fulltext})
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)

        order_by_expr_list = list()
        if orderBy.fields:
            for order_field in orderBy.fields:
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field[0], SortType.Asc))
                else:
                    order_by_expr_list.append((order_field[0], SortType.Desc))

        # Scatter search tables and gather the results
        for indexName in indexNames:
            for knowledgebaseId in knowledgebaseIds:
                table_name = f"{indexName}_{knowledgebaseId}"
                try:
                    table_instance = db_instance.get_table(table_name)
                except Exception:
                    continue
                table_list.append(table_name)
                builder = table_instance.output(selectFields)
                if len(matchExprs) > 0:
                    for matchExpr in matchExprs:
                        if isinstance(matchExpr, MatchTextExpr):
                            fields = ",".join(matchExpr.fields)
                            builder = builder.match_text(
                                fields,
                                matchExpr.matching_text,
                                matchExpr.topn,
                                matchExpr.extra_options,
                            )
                        elif isinstance(matchExpr, MatchDenseExpr):
                            builder = builder.match_dense(
                                matchExpr.vector_column_name,
                                matchExpr.embedding_data,
                                matchExpr.embedding_data_type,
                                matchExpr.distance_type,
                                matchExpr.topn,
                                matchExpr.extra_options,
                            )
                        elif isinstance(matchExpr, FusionExpr):
                            builder = builder.fusion(
                                matchExpr.method, matchExpr.topn, matchExpr.fusion_params
                            )
                else:
                    if len(filter_cond) > 0:
                        builder.filter(filter_cond)
                if orderBy.fields:
                    builder.sort(order_by_expr_list)
                builder.offset(offset).limit(limit)
                kb_res = builder.to_pl()
                df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, selectFields)
        logging.debug("INFINITY search tables: " + str(table_list))
        return res

    def get(
            self, chunkId: str, indexName: str, knowledgebaseIds: list[str]
    ) -> dict | None:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        assert isinstance(knowledgebaseIds, list)
        for knowledgebaseId in knowledgebaseIds:
            table_name = f"{indexName}_{knowledgebaseId}"
            table_instance = db_instance.get_table(table_name)
            kb_res = table_instance.output(["*"]).filter(f"id = '{chunkId}'").to_pl()
            if len(kb_res) != 0 and kb_res.shape[0] > 0:
                df_list.append(kb_res)

        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, ["id"])
        res_fields = self.getFields(res, res.columns)
        return res_fields.get(chunkId, None)

    def insert(
            self, documents: list[dict], indexName: str, knowledgebaseId: str
    ) -> list[str]:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            table_instance = db_instance.get_table(table_name)
        except InfinityException as e:
            # src/common/status.cppm, kTableNotExist = 3022
            if e.error_code != 3022:
                raise
            vector_size = 0
            patt = re.compile(r"q_(?P<vector_size>\d+)_vec")
            for k in documents[0].keys():
                m = patt.match(k)
                if m:
                    vector_size = int(m.group("vector_size"))
                    break
            if vector_size == 0:
                raise ValueError("Cannot infer vector size from documents")
            self.createIdx(indexName, knowledgebaseId, vector_size)
            table_instance = db_instance.get_table(table_name)

        for d in documents:
            assert "_id" not in d
            assert "id" in d
            for k, v in d.items():
                if k.endswith("_kwd") and isinstance(v, list):
                    d[k] = " ".join(v)
        ids = ["'{}'".format(d["id"]) for d in documents]
        str_ids = ", ".join(ids)
        str_filter = f"id IN ({str_ids})"
        table_instance.delete(str_filter)
        # for doc in documents:
        #     logging.info(f"insert position_list: {doc['position_list']}")
        # logging.info(f"InfinityConnection.insert {json.dumps(documents)}")
        table_instance.insert(documents)
        self.connPool.release_conn(inf_conn)
        logging.debug(f"inserted into {table_name} {str_ids}.")
        return []

    def update(
            self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str
    ) -> bool:
        # if 'position_list' in newValue:
        #     logging.info(f"upsert position_list: {newValue['position_list']}")
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        table_instance = db_instance.get_table(table_name)
        filter = equivalent_condition_to_str(condition)
        for k, v in newValue.items():
            if k.endswith("_kwd") and isinstance(v, list):
                newValue[k] = " ".join(v)
        table_instance.update(filter, newValue)
        self.connPool.release_conn(inf_conn)
        return True

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        filter = equivalent_condition_to_str(condition)
        try:
            table_instance = db_instance.get_table(table_name)
        except Exception:
            logging.warning(
                f"Skipped deleting `{filter}` from table {table_name} since the table doesn't exist."
            )
            return 0
        res = table_instance.delete(filter)
        self.connPool.release_conn(inf_conn)
        return res.deleted_rows

    """
    Helper functions for search result
    """

    def getTotal(self, res):
        return len(res)

    def getChunkIds(self, res):
        return list(res["id"])

    def getFields(self, res, fields: list[str]) -> list[str, dict]:
        res_fields = {}
        if not fields:
            return {}
        num_rows = len(res)
        column_id = res["id"]
        for i in range(num_rows):
            id = column_id[i]
            m = {"id": id}
            for fieldnm in fields:
                if fieldnm not in res:
                    m[fieldnm] = None
                    continue
                v = res[fieldnm][i]
                if isinstance(v, Series):
                    v = list(v)
                elif fieldnm == "important_kwd":
                    assert isinstance(v, str)
                    v = v.split(" ")
                else:
                    if not isinstance(v, str):
                        v = str(v)
                    # if fieldnm.endswith("_tks"):
                    #     v = rmSpace(v)
                m[fieldnm] = v
            res_fields[id] = m
        return res_fields

    def getHighlight(self, res, keywords: list[str], fieldnm: str):
        ans = {}
        num_rows = len(res)
        column_id = res["id"]
        for i in range(num_rows):
            id = column_id[i]
            txt = res[fieldnm][i]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                for w in keywords:
                    t = re.sub(
                        r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])"
                        % re.escape(w),
                        r"\1<em>\2</em>\3",
                        t,
                        flags=re.IGNORECASE | re.MULTILINE,
                    )
                if not re.search(
                        r"<em>[^<>]+</em>", t, flags=re.IGNORECASE | re.MULTILINE
                ):
                    continue
                txts.append(t)
            ans[id] = "...".join(txts)
        return ans

    def getAggregation(self, res, fieldnm: str):
        """
        TODO: Infinity doesn't provide aggregation
        """
        return list()

    """
    SQL
    """

    def sql(sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
