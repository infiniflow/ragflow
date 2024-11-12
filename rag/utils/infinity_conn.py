import os
import re
import json
from typing import List, Dict
import infinity
from infinity.common import ConflictType, InfinityException
from infinity.index import IndexInfo, IndexType
from infinity.connection_pool import ConnectionPool
from rag import settings
from rag.settings import doc_store_logger
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


@singleton
class InfinityConnection(DocStoreConnection):
    def __init__(self):
        self.dbName = settings.INFINITY.get("db_name", "default_db")
        infinity_uri = settings.INFINITY["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            infinity_uri = infinity.common.NetworkAddress(host, int(port))
        self.connPool = ConnectionPool(infinity_uri)
        doc_store_logger.info(f"Connected to infinity {infinity_uri}.")

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
        res = infinity.show_current_node()
        self.connPool.release_conn(inf_conn)
        color = "green" if res.error_code == 0 else "red"
        res2 = {
            "type": "infinity",
            "status": f"{res.role} {color}",
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
        doc_store_logger.info(
            f"INFINITY created table {table_name}, vector size {vectorSize}"
        )

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        table_name = f"{indexName}_{knowledgebaseId}"
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        db_instance.drop_table(table_name, ConflictType.Ignore)
        self.connPool.release_conn(inf_conn)
        doc_store_logger.info(f"INFINITY dropped table {table_name}")

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            inf_conn = self.connPool.get_conn()
            db_instance = inf_conn.get_database(self.dbName)
            _ = db_instance.get_table(table_name)
            self.connPool.release_conn(inf_conn)
            return True
        except Exception as e:
            doc_store_logger.error("INFINITY indexExist: " + str(e))
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
        indexNames: str|list[str],
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
                # doc_store_logger.info(f"filter_fulltext: {filter_fulltext}")
                minimum_should_match = "0%"
                if "minimum_should_match" in matchExpr.extra_options:
                    minimum_should_match = (
                        str(int(matchExpr.extra_options["minimum_should_match"] * 100))
                        + "%"
                    )
                    matchExpr.extra_options.update(
                        {"minimum_should_match": minimum_should_match}
                    )
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
            elif isinstance(matchExpr, MatchDenseExpr):
                if len(filter_cond) != 0 and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_fulltext})
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
        if orderBy.fields:
            order_by_expr_list = list()
            for order_field in orderBy.fields:
                order_by_expr_list.append((order_field[0], order_field[1] == 0))

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
                if orderBy.fields:
                    builder.sort(order_by_expr_list)
                builder.offset(offset).limit(limit)
                kb_res = builder.to_pl()
                df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = pl.concat(df_list)
        doc_store_logger.info("INFINITY search tables: " + str(table_list))
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
            df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = pl.concat(df_list)
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
        ids = [f"'{d["id"]}'" for d in documents]
        str_ids = ", ".join(ids)
        str_filter = f"id IN ({str_ids})"
        table_instance.delete(str_filter)
        # for doc in documents:
        #     doc_store_logger.info(f"insert position_list: {doc['position_list']}")
        # doc_store_logger.info(f"InfinityConnection.insert {json.dumps(documents)}")
        table_instance.insert(documents)
        self.connPool.release_conn(inf_conn)
        doc_store_logger.info(f"inserted into {table_name} {str_ids}.")
        return []

    def update(
        self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str
    ) -> bool:
        # if 'position_list' in newValue:
        #     doc_store_logger.info(f"update position_list: {newValue['position_list']}")
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
            doc_store_logger.warning(
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

    def getFields(self, res, fields: List[str]) -> Dict[str, dict]:
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

    def getHighlight(self, res, keywords: List[str], fieldnm: str):
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
