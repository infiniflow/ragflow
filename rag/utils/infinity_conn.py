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
from common.decorator import singleton
import pandas as pd
from common.file_utils import get_project_base_directory
from rag.nlp import is_english
from common.constants import PAGERANK_FLD, TAG_FLD
from common import settings
from rag.utils.doc_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)

logger = logging.getLogger("ragflow.infinity_conn")


def field_keyword(field_name: str):
    # The "docnm_kwd" field is always a string, not list.
    if field_name == "source_id" or (field_name.endswith("_kwd") and field_name != "docnm_kwd" and field_name != "knowledge_graph_kwd"):
        return True
    return False


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
        if not isinstance(k, str) or not v:
            continue
        if field_keyword(k):
            if isinstance(v, list):
                inCond = list()
                for item in v:
                    if isinstance(item, str):
                        item = item.replace("'", "''")
                    inCond.append(f"filter_fulltext('{k}', '{item}')")
                if inCond:
                    strInCond = " or ".join(inCond)
                    strInCond = f"({strInCond})"
                    cond.append(strInCond)
            else:
                cond.append(f"filter_fulltext('{k}', '{v}')")
        elif isinstance(v, list):
            inCond = list()
            for item in v:
                if isinstance(item, str):
                    item = item.replace("'", "''")
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


def concat_dataframes(df_list: list[pd.DataFrame], selectFields: list[str]) -> pd.DataFrame:
    df_list2 = [df for df in df_list if not df.empty]
    if df_list2:
        return pd.concat(df_list2, axis=0).reset_index(drop=True)

    schema = []
    for field_name in selectFields:
        if field_name == "score()":  # Workaround: fix schema is changed to score()
            schema.append("SCORE")
        elif field_name == "similarity()":  # Workaround: fix schema is changed to similarity()
            schema.append("SIMILARITY")
        else:
            schema.append(field_name)
    return pd.DataFrame(columns=schema)


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
                connPool = ConnectionPool(infinity_uri, max_size=32)
                inf_conn = connPool.get_conn()
                res = inf_conn.show_current_node()
                if res.error_code == ErrorCode.OK and res.server_status in ["started", "alive"]:
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
        fp_mapping = os.path.join(get_project_base_directory(), "conf", "infinity_mapping.json")
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
                logger.info(f"INFINITY added following column to table {table_name}: {field_name} {field_info}")
                if field_info["type"] != "varchar" or "analyzer" not in field_info:
                    continue
                inf_table.create_index(
                    f"text_idx_{field_name}",
                    IndexInfo(field_name, IndexType.FullText, {"ANALYZER": field_info["analyzer"]}),
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
            "status": "green" if res.error_code == 0 and res.server_status in ["started", "alive"] else "red",
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

        fp_mapping = os.path.join(get_project_base_directory(), "conf", "infinity_mapping.json")
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
                IndexInfo(field_name, IndexType.FullText, {"ANALYZER": field_info["analyzer"]}),
                ConflictType.Ignore,
            )
        self.connPool.release_conn(inf_conn)
        logger.info(f"INFINITY created table {table_name}, vector size {vectorSize}")

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
        aggFields: list[str] = [],
        rank_feature: dict | None = None,
    ) -> tuple[pd.DataFrame, int]:
        """
        BUG: Infinity returns empty for a highlight field if the query string doesn't use that field.
        """
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        table_list = list()
        output = selectFields.copy()
        for essential_field in ["id"] + aggFields:
            if essential_field not in output:
                output.append(essential_field)
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
            if score_func not in output:
                output.append(score_func)
            if PAGERANK_FLD not in output:
                output.append(PAGERANK_FLD)
        output = [f for f in output if f != "_score"]
        if limit <= 0:
            # ElasticSearch default limit is 10000
            limit = 10000

        # Prepare expressions common to all tables
        filter_cond = None
        filter_fulltext = ""
        if condition:
            table_found = False
            for indexName in indexNames:
                for kb_id in knowledgebaseIds:
                    table_name = f"{indexName}_{kb_id}"
                    try:
                        filter_cond = equivalent_condition_to_str(condition, db_instance.get_table(table_name))
                        table_found = True
                        break
                    except Exception:
                        pass
                if table_found:
                    break
            if not table_found:
                logger.error(f"No valid tables found for indexNames {indexNames} and knowledgebaseIds {knowledgebaseIds}")
                return pd.DataFrame(), 0

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

                # Add rank_feature support
                if rank_feature and "rank_features" not in matchExpr.extra_options:
                    # Convert rank_feature dict to Infinity's rank_features string format
                    # Format: "field^feature_name^weight,field^feature_name^weight"
                    rank_features_list = []
                    for feature_name, weight in rank_feature.items():
                        # Use TAG_FLD as the field containing rank features
                        rank_features_list.append(f"{TAG_FLD}^{feature_name}^{weight}")
                    if rank_features_list:
                        matchExpr.extra_options["rank_features"] = ",".join(rank_features_list)

                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                logger.debug(f"INFINITY search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, MatchDenseExpr):
                if filter_fulltext and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_fulltext})
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                similarity = matchExpr.extra_options.get("similarity")
                if similarity:
                    matchExpr.extra_options["threshold"] = similarity
                    del matchExpr.extra_options["similarity"]
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
                builder = table_instance.output(output)
                if len(matchExprs) > 0:
                    for matchExpr in matchExprs:
                        if isinstance(matchExpr, MatchTextExpr):
                            fields = ",".join(matchExpr.fields)
                            builder = builder.match_text(
                                fields,
                                matchExpr.matching_text,
                                matchExpr.topn,
                                matchExpr.extra_options.copy(),
                            )
                        elif isinstance(matchExpr, MatchDenseExpr):
                            builder = builder.match_dense(
                                matchExpr.vector_column_name,
                                matchExpr.embedding_data,
                                matchExpr.embedding_data_type,
                                matchExpr.distance_type,
                                matchExpr.topn,
                                matchExpr.extra_options.copy(),
                            )
                        elif isinstance(matchExpr, FusionExpr):
                            builder = builder.fusion(matchExpr.method, matchExpr.topn, matchExpr.fusion_params)
                else:
                    if filter_cond and len(filter_cond) > 0:
                        builder.filter(filter_cond)
                if orderBy.fields:
                    builder.sort(order_by_expr_list)
                builder.offset(offset).limit(limit)
                kb_res, extra_result = builder.option({"total_hits_count": True}).to_df()
                if extra_result:
                    total_hits_count += int(extra_result["total_hits_count"])
                logger.debug(f"INFINITY search table: {str(table_name)}, result: {str(kb_res)}")
                df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, output)
        if matchExprs:
            res["_score"] = res[score_column] + res[PAGERANK_FLD]
            res = res.sort_values(by="_score", ascending=False).reset_index(drop=True)
            res = res.head(limit)
        logger.debug(f"INFINITY search final result: {str(res)}")
        return res, total_hits_count

    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
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
                logger.warning(f"Table not found: {table_name}, this knowledge base isn't created in Infinity. Maybe it is created in other document engine.")
                continue
            kb_res, _ = table_instance.output(["*"]).filter(f"id = '{chunkId}'").to_df()
            logger.debug(f"INFINITY get table: {str(table_list)}, result: {str(kb_res)}")
            df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = concat_dataframes(df_list, ["id"])
        res_fields = self.get_fields(res, res.columns.tolist())
        return res_fields.get(chunkId, None)

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str = None) -> list[str]:
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
                if field_keyword(k):
                    if isinstance(v, list):
                        d[k] = "###".join(v)
                    else:
                        d[k] = v
                elif re.search(r"_feas$", k):
                    d[k] = json.dumps(v)
                elif k == "kb_id":
                    if isinstance(d[k], list):
                        d[k] = d[k][0]  # since d[k] is a list, but we need a str
                elif k == "position_int":
                    assert isinstance(v, list)
                    arr = [num for row in v for num in row]
                    d[k] = "_".join(f"{num:08x}" for num in arr)
                elif k in ["page_num_int", "top_int"]:
                    assert isinstance(v, list)
                    d[k] = "_".join(f"{num:08x}" for num in v)
                else:
                    d[k] = v

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

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        # if 'position_int' in newValue:
        #     logger.info(f"update position_int: {newValue['position_int']}")
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{indexName}_{knowledgebaseId}"
        table_instance = db_instance.get_table(table_name)
        # if "exists" in condition:
        #    del condition["exists"]

        clmns = {}
        if table_instance:
            for n, ty, de, _ in table_instance.show_columns().rows():
                clmns[n] = (ty, de)
        filter = equivalent_condition_to_str(condition, table_instance)
        removeValue = {}
        for k, v in list(newValue.items()):
            if field_keyword(k):
                if isinstance(v, list):
                    newValue[k] = "###".join(v)
                else:
                    newValue[k] = v
            elif re.search(r"_feas$", k):
                newValue[k] = json.dumps(v)
            elif k == "kb_id":
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
                if isinstance(v, str):
                    assert v in clmns, f"'{v}' should be in '{clmns}'."
                    ty, de = clmns[v]
                    if ty.lower().find("cha"):
                        if not de:
                            de = ""
                    newValue[v] = de
                else:
                    for kk, vv in v.items():
                        removeValue[kk] = vv
                    del newValue[k]
            else:
                newValue[k] = v

        remove_opt = {}  # "[k,new_value]": [id_to_update, ...]
        if removeValue:
            col_to_remove = list(removeValue.keys())
            row_to_opt = table_instance.output(col_to_remove + ["id"]).filter(filter).to_df()
            logger.debug(f"INFINITY search table {str(table_name)}, filter {filter}, result: {str(row_to_opt[0])}")
            row_to_opt = self.get_fields(row_to_opt, col_to_remove)
            for id, old_v in row_to_opt.items():
                for k, remove_v in removeValue.items():
                    if remove_v in old_v[k]:
                        new_v = old_v[k].copy()
                        new_v.remove(remove_v)
                        kv_key = json.dumps([k, new_v])
                        if kv_key not in remove_opt:
                            remove_opt[kv_key] = [id]
                        else:
                            remove_opt[kv_key].append(id)

        logger.debug(f"INFINITY update table {table_name}, filter {filter}, newValue {newValue}.")
        for update_kv, ids in remove_opt.items():
            k, v = json.loads(update_kv)
            table_instance.update(filter + " AND id in ({0})".format(",".join([f"'{id}'" for id in ids])), {k: "###".join(v)})

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
            logger.warning(f"Skipped deleting from table {table_name} since the table doesn't exist.")
            return 0
        filter = equivalent_condition_to_str(condition, table_instance)
        logger.debug(f"INFINITY delete table {table_name}, filter {filter}.")
        res = table_instance.delete(filter)
        self.connPool.release_conn(inf_conn)
        return res.deleted_rows

    """
    Helper functions for search result
    """

    def get_total(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> int:
        if isinstance(res, tuple):
            return res[1]
        return len(res)

    def get_chunk_ids(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return list(res["id"])

    def get_fields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        if isinstance(res, tuple):
            res = res[0]
        if not fields:
            return {}
        fieldsAll = fields.copy()
        fieldsAll.append("id")
        column_map = {col.lower(): col for col in res.columns}
        matched_columns = {column_map[col.lower()]: col for col in set(fieldsAll) if col.lower() in column_map}
        none_columns = [col for col in set(fieldsAll) if col.lower() not in column_map]

        res2 = res[matched_columns.keys()]
        res2 = res2.rename(columns=matched_columns)
        res2.drop_duplicates(subset=["id"], inplace=True)

        for column in res2.columns:
            k = column.lower()
            if field_keyword(k):
                res2[column] = res2[column].apply(lambda v: [kwd for kwd in v.split("###") if kwd])
            elif re.search(r"_feas$", k):
                res2[column] = res2[column].apply(lambda v: json.loads(v) if v else {})
            elif k == "position_int":

                def to_position_int(v):
                    if v:
                        arr = [int(hex_val, 16) for hex_val in v.split("_")]
                        v = [arr[i : i + 5] for i in range(0, len(arr), 5)]
                    else:
                        v = []
                    return v

                res2[column] = res2[column].apply(to_position_int)
            elif k in ["page_num_int", "top_int"]:
                res2[column] = res2[column].apply(lambda v: [int(hex_val, 16) for hex_val in v.split("_")] if v else [])
            else:
                pass
        for column in none_columns:
            res2[column] = None

        return res2.set_index("id").to_dict(orient="index")

    def get_highlight(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, keywords: list[str], fieldnm: str):
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
            if re.search(r"<em>[^<>]+</em>", txt, flags=re.IGNORECASE | re.MULTILINE):
                ans[id] = txt
                continue
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                if is_english([t]):
                    for w in keywords:
                        t = re.sub(
                            r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])" % re.escape(w),
                            r"\1<em>\2</em>\3",
                            t,
                            flags=re.IGNORECASE | re.MULTILINE,
                        )
                else:
                    for w in sorted(keywords, key=len, reverse=True):
                        t = re.sub(
                            re.escape(w),
                            f"<em>{w}</em>",
                            t,
                            flags=re.IGNORECASE | re.MULTILINE,
                        )
                if not re.search(r"<em>[^<>]+</em>", t, flags=re.IGNORECASE | re.MULTILINE):
                    continue
                txts.append(t)
            if txts:
                ans[id] = "...".join(txts)
            else:
                ans[id] = txt
        return ans

    def get_aggregation(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fieldnm: str):
        """
        Manual aggregation for tag fields since Infinity doesn't provide native aggregation
        """
        from collections import Counter

        # Extract DataFrame from result
        if isinstance(res, tuple):
            df, _ = res
        else:
            df = res

        if df.empty or fieldnm not in df.columns:
            return []

        # Aggregate tag counts
        tag_counter = Counter()

        for value in df[fieldnm]:
            if pd.isna(value) or not value:
                continue

            # Handle different tag formats
            if isinstance(value, str):
                # Split by ### for tag_kwd field or comma for other formats
                if fieldnm == "tag_kwd" and "###" in value:
                    tags = [tag.strip() for tag in value.split("###") if tag.strip()]
                else:
                    # Try comma separation as fallback
                    tags = [tag.strip() for tag in value.split(",") if tag.strip()]

                for tag in tags:
                    if tag:  # Only count non-empty tags
                        tag_counter[tag] += 1
            elif isinstance(value, list):
                # Handle list format
                for tag in value:
                    if tag and isinstance(tag, str):
                        tag_counter[tag.strip()] += 1

        # Return as list of [tag, count] pairs, sorted by count descending
        return [[tag, count] for tag, count in tag_counter.most_common()]

    """
    SQL
    """

    def sql(sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
