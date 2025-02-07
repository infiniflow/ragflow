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
import os
import re
import json
import time
import copy
import infinity
from infinity.common import ConflictType, InfinityException, SortType
from infinity.index import IndexInfo, IndexType
from infinity.connection_pool import ConnectionPool
from infinity.errors import ErrorCode
from rag import settings
from rag.settings import PAGERANK_FLD
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

logger = logging.getLogger('ragflow.infinity_conn')


def equivalent_condition_to_str(condition: dict, table_instance=None) -> str | None:
    assert "_id" not in condition
    clmns = {}
    if table_instance:
        for n, ty, de, _ in table_instance.show_columns().rows():
            clmns[n] = (ty, de)

    def exists(cln):
        nonlocal clmns
        assert cln in clmns, f"'{cln}' should be in '{clmns}'."
        ty, de = clmns[cln]
        if ty.lower().find("cha"):
            if not de:
                de = ""
            return f" {cln}!='{de}' "
        return f"{cln}!={de}"

    cond = list()
    for k, v in condition.items():
        if not isinstance(k, str) or k in ["kb_id"] or not v:
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
        elif k == "must_not":
            if isinstance(v, dict):
                for kk, vv in v.items():
                    if kk == "exists":
                        cond.append("NOT (%s)" % exists(vv))
        elif isinstance(v, str):
            cond.append(f"{k}='{v}'")
        elif k == "exists":
            cond.append(exists(v))
        else:
            cond.append(f"{k}={str(v)}")
    return " AND ".join(cond) if cond else "1=1"


def concat_dataframes(df_list: list[pl.DataFrame], selectFields: list[str]) -> pl.DataFrame:
    """
    Concatenate multiple dataframes into one.
    """
    df_list = [df for df in df_list if not df.is_empty()]
    if df_list:
        return pl.concat(df_list)
    schema = dict()
    for field_name in selectFields:
        if field_name == 'score()':  # Workaround: fix schema is changed to score()
            schema['SCORE'] = str
        else:
            schema[field_name] = str
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
        logger.info(f"Use Infinity {infinity_uri} as the doc engine.")
        for _ in range(24):
            try:
                connPool = ConnectionPool(infinity_uri)
                inf_conn = connPool.get_conn()
                res = inf_conn.show_current_node()
                if res.error_code == ErrorCode.OK and res.server_status == "started":
                    self._migrate_db(inf_conn)
                    self.connPool = connPool
                    connPool.release_conn(inf_conn)
                    break
                connPool.release_conn(inf_conn)
                logger.warn(f"Infinity status: {res.server_status}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
        if self.connPool is None:
            msg = f"Infinity {infinity_uri} is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)
        logger.info(f"Infinity {infinity_uri} is healthy.")

    def _migrate_db(self, inf_conn):
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)
        fp_mapping = os.path.join(
            get_project_base_directory(), "conf", "infinity_mapping.json"
        )
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        schema = json.load(open(fp_mapping))
        table_names = inf_db.list_tables().table_names
        for table_name in table_names:
            inf_table = inf_db.get_table(table_name)
            index_names = inf_table.list_indexes().index_names
            if "q_vec_idx" not in index_names:
                # Skip tables not created by me
                continue
            column_names = inf_table.show_columns()["name"]
            column_names = set(column_names)
            for field_name, field_info in schema.items():
                if field_name in column_names:
                    continue
                res = inf_table.add_columns({field_name: field_info})
                assert res.error_code == infinity.ErrorCode.OK
                logger.info(
                    f"INFINITY added following column to table {table_name}: {field_name} {field_info}"
                )
                if field_info["type"] != "varchar" or "analyzer" not in field_info:
                    continue
                inf_table.create_index(
                    f"text_idx_{field_name}",
                    IndexInfo(
                        field_name, IndexType.FullText, {"ANALYZER": field_info["analyzer"]}
                    ),
                    ConflictType.Ignore,
                )

    """
    Database operations
    """

    def dbType(self) -> str:
        return "infinity"

    def health(self) -> dict:
        """
        Return the health status of the database.
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
        for field_name, field_info in schema.items():
            if field_info["type"] != "varchar" or "analyzer" not in field_info:
                continue
            inf_table.create_index(
                f"text_idx_{field_name}",
                IndexInfo(
                    field_name, IndexType.FullText, {"ANALYZER": field_info["analyzer"]}
                ),
                ConflictType.Ignore,
            )
        self.connPool.release_conn(inf_conn)
        logger.info(
            f"INFINITY created table {table_name}, vector size {vectorSize}"
        )

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        table_name = f"{indexName}_{knowledgebaseId}"
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        db_instance.drop_table(table_name, ConflictType.Ignore)
        self.connPool.release_conn(inf_conn)
        logger.info(f"INFINITY dropped table {table_name}")

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            inf_conn = self.connPool.get_conn()
            db_instance = inf_conn.get_database(self.dbName)
            _ = db_instance.get_table(table_name)
            self.connPool.release_conn(inf_conn)
            return True
        except Exception as e:
            logger.warning(f"INFINITY indexExist {str(e)}")
        return False

    """
    CRUD operations
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
        for essential_field in ["id"]:
            if essential_field not in selectFields:
                selectFields.append(essential_field)
        score_func = ""
        score_column = ""
        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                score_func = "score()"
                score_column = "SCORE"
                break
        if not score_func:
            for matchExpr in matchExprs:
                if isinstance(matchExpr, MatchDenseExpr):
                    score_func = "similarity()"
                    score_column = "SIMILARITY"
                    break
        if matchExprs:
            selectFields.append(score_func)
            selectFields.append(PAGERANK_FLD)
        selectFields = [f for f in selectFields if f != "_score"]

        # Prepare expressions common to all tables
        filter_cond = None
        filter_fulltext = ""
        if condition:
            for indexName in indexNames:
                table_name = f"{indexName}_{knowledgebaseIds[0]}"
                filter_cond = equivalent_condition_to_str(condition, db_instance.get_table(table_name))
                break

        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                if filter_cond and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_cond})
                fields = ",".join(matchExpr.fields)
                filter_fulltext = f"filter_fulltext('{fields}', '{matchExpr.matching_text}')"
                if filter_cond:
                    filter_fulltext = f"({filter_cond}) AND {filter_fulltext}"
                minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    str_minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                logger.debug(f"INFINITY search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, MatchDenseExpr):
                if filter_fulltext and filter_cond and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_fulltext})
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                logger.debug(f"INFINITY search MatchDenseExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, FusionExpr):
                logger.debug(f"INFINITY search FusionExpr: {json.dumps(matchExpr.__dict__)}")

        order_by_expr_list = list()
        if orderBy.fields:
            for order_field in orderBy.fields:
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field[0], SortType.Asc))
                else:
                    order_by_expr_list.append((order_field[0], SortType.Desc))

        total_hits_count = 0
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
                kb_res, extra_result = builder.option({"total_hits_count": True}).to_pl()
                if extra_result:
                    total_hits_count += int(extra_result["total_hits_count"])
                logger.debug(f"INFINITY search table: {str(table_name)}, result: {str(kb_res)}")
                df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, selectFields)
        if matchExprs:
            res = res.sort(pl.col(score_column) + pl.col(PAGERANK_FLD), descending=True, maintain_order=True)
            if score_column and score_column != "SCORE":
                res = res.rename({score_column: "_score"})
        res = res.limit(limit)
        logger.debug(f"INFINITY search final result: {str(res)}")
        return res, total_hits_count

    def get(
            self, chunkId: str, indexName: str, knowledgebaseIds: list[str]
    ) -> dict | None:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        assert isinstance(knowledgebaseIds, list)
        table_list = list()
        for knowledgebaseId in knowledgebaseIds:
            table_name = f"{indexName}_{knowledgebaseId}"
            table_list.append(table_name)
            table_instance = None
            try:
                table_instance = db_instance.get_table(table_name)
            except Exception:
                logger.warning(
                    f"Table not found: {table_name}, this knowledge base isn't created in Infinity. Maybe it is created in other document engine.")
                continue
            kb_res, _ = table_instance.output(["*"]).filter(f"id = '{chunkId}'").to_pl()
            logger.debug(f"INFINITY get table: {str(table_list)}, result: {str(kb_res)}")
            df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, ["id"])
        res_fields = self.getFields(res, res.columns)
        return res_fields.get(chunkId, None)

    def insert(
            self, documents: list[dict], indexName: str, knowledgebaseId: str = None
    ) -> list[str]:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            table_instance = db_instance.get_table(table_name)
        except InfinityException as e:
            # src/common/status.cppm, kTableNotExist = 3022
            if e.error_code != ErrorCode.TABLE_NOT_EXIST:
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

        # embedding fields can't have a default value....
        embedding_clmns = []
        clmns = table_instance.show_columns().rows()
        for n, ty, _, _ in clmns:
            r = re.search(r"Embedding\([a-z]+,([0-9]+)\)", ty)
            if not r:
                continue
            embedding_clmns.append((n, int(r.group(1))))

        docs = copy.deepcopy(documents)
        for d in docs:
            assert "_id" not in d
            assert "id" in d
            for k, v in d.items():
                if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                    assert isinstance(v, list)
                    d[k] = "###".join(v)
                elif re.search(r"_feas$", k):
                    d[k] = json.dumps(v)
                elif k == 'kb_id':
                    if isinstance(d[k], list):
                        d[k] = d[k][0]  # since d[k] is a list, but we need a str
                elif k == "position_int":
                    assert isinstance(v, list)
                    arr = [num for row in v for num in row]
                    d[k] = "_".join(f"{num:08x}" for num in arr)
                elif k in ["page_num_int", "top_int"]:
                    assert isinstance(v, list)
                    d[k] = "_".join(f"{num:08x}" for num in v)

            for n, vs in embedding_clmns:
                if n in d:
                    continue
                d[n] = [0] * vs
        ids = ["'{}'".format(d["id"]) for d in docs]
        str_ids = ", ".join(ids)
        str_filter = f"id IN ({str_ids})"
        table_instance.delete(str_filter)
        # for doc in documents:
        #     logger.info(f"insert position_int: {doc['position_int']}")
        # logger.info(f"InfinityConnection.insert {json.dumps(documents)}")
        table_instance.insert(docs)
        self.connPool.release_conn(inf_conn)
        logger.debug(f"INFINITY inserted into {table_name} {str_ids}.")
        return []

    def update(
            self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str
    ) -> bool:
        # if 'position_int' in newValue:
        #     logger.info(f"update position_int: {newValue['position_int']}")
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        table_instance = db_instance.get_table(table_name)
        #if "exists" in condition:
        #    del condition["exists"]
        filter = equivalent_condition_to_str(condition, table_instance)
        for k, v in list(newValue.items()):
            if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                assert isinstance(v, list)
                newValue[k] = "###".join(v)
            elif re.search(r"_feas$", k):
                newValue[k] = json.dumps(v)
            elif k.endswith("_kwd") and isinstance(v, list):
                newValue[k] = " ".join(v)
            elif k == 'kb_id':
                if isinstance(newValue[k], list):
                    newValue[k] = newValue[k][0]  # since d[k] is a list, but we need a str
            elif k == "position_int":
                assert isinstance(v, list)
                arr = [num for row in v for num in row]
                newValue[k] = "_".join(f"{num:08x}" for num in arr)
            elif k in ["page_num_int", "top_int"]:
                assert isinstance(v, list)
                newValue[k] = "_".join(f"{num:08x}" for num in v)
            elif k == "remove":
                del newValue[k]
                if v in [PAGERANK_FLD]:
                    newValue[v] = 0

        logger.debug(f"INFINITY update table {table_name}, filter {filter}, newValue {newValue}.")
        table_instance.update(filter, newValue)
        self.connPool.release_conn(inf_conn)
        return True

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            table_instance = db_instance.get_table(table_name)
        except Exception:
            logger.warning(
                f"Skipped deleting from table {table_name} since the table doesn't exist."
            )
            return 0
        filter = equivalent_condition_to_str(condition, table_instance)
        logger.debug(f"INFINITY delete table {table_name}, filter {filter}.")
        res = table_instance.delete(filter)
        self.connPool.release_conn(inf_conn)
        return res.deleted_rows

    """
    Helper functions for search result
    """

    def getTotal(self, res: tuple[pl.DataFrame, int] | pl.DataFrame) -> int:
        if isinstance(res, tuple):
            return res[1]
        return len(res)

    def getChunkIds(self, res: tuple[pl.DataFrame, int] | pl.DataFrame) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return list(res["id"])

    def getFields(self, res: tuple[pl.DataFrame, int] | pl.DataFrame, fields: list[str]) -> list[str, dict]:
        if isinstance(res, tuple):
            res = res[0]
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
                elif fieldnm in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                    assert isinstance(v, str)
                    v = [kwd for kwd in v.split("###") if kwd]
                elif fieldnm == "position_int":
                    assert isinstance(v, str)
                    if v:
                        arr = [int(hex_val, 16) for hex_val in v.split('_')]
                        v = [arr[i:i + 5] for i in range(0, len(arr), 5)]
                    else:
                        v = []
                elif fieldnm in ["page_num_int", "top_int"]:
                    assert isinstance(v, str)
                    if v:
                        v = [int(hex_val, 16) for hex_val in v.split('_')]
                    else:
                        v = []
                else:
                    if not isinstance(v, str):
                        v = str(v)
                    # if fieldnm.endswith("_tks"):
                    #     v = rmSpace(v)
                m[fieldnm] = v
            res_fields[id] = m
        return res_fields

    def getHighlight(self, res: tuple[pl.DataFrame, int] | pl.DataFrame, keywords: list[str], fieldnm: str):
        if isinstance(res, tuple):
            res = res[0]
        ans = {}
        num_rows = len(res)
        column_id = res["id"]
        if fieldnm not in res:
            return {}
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

    def getAggregation(self, res: tuple[pl.DataFrame, int] | pl.DataFrame, fieldnm: str):
        """
        TODO: Infinity doesn't provide aggregation
        """
        return list()

    """
    SQL
    """

    def sql(sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
