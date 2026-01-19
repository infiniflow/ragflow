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
from abc import abstractmethod

import infinity
from infinity.common import ConflictType
from infinity.index import IndexInfo, IndexType
from infinity.errors import ErrorCode
import pandas as pd
from common.file_utils import get_project_base_directory
from rag.nlp import is_english
from common import settings
from common.doc_store.doc_store_base import DocStoreConnection, MatchExpr, OrderByExpr


class InfinityConnectionBase(DocStoreConnection):
    def __init__(self, mapping_file_name: str="infinity_mapping.json", logger_name: str="ragflow.infinity_conn"):
        from common.doc_store.infinity_conn_pool import INFINITY_CONN

        self.dbName = settings.INFINITY.get("db_name", "default_db")
        self.mapping_file_name = mapping_file_name
        self.logger = logging.getLogger(logger_name)
        infinity_uri = settings.INFINITY["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            infinity_uri = infinity.common.NetworkAddress(host, int(port))
        self.connPool = None
        self.logger.info(f"Use Infinity {infinity_uri} as the doc engine.")
        conn_pool = INFINITY_CONN.get_conn_pool()
        for _ in range(24):
            try:
                inf_conn = conn_pool.get_conn()
                res = inf_conn.show_current_node()
                if res.error_code == ErrorCode.OK and res.server_status in ["started", "alive"]:
                    self._migrate_db(inf_conn)
                    self.connPool = conn_pool
                    conn_pool.release_conn(inf_conn)
                    break
                conn_pool.release_conn(inf_conn)
                self.logger.warning(f"Infinity status: {res.server_status}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
            except Exception as e:
                conn_pool = INFINITY_CONN.refresh_conn_pool()
                self.logger.warning(f"{str(e)}. Waiting Infinity {infinity_uri} to be healthy.")
                time.sleep(5)
        if self.connPool is None:
            msg = f"Infinity {infinity_uri} is unhealthy in 120s."
            self.logger.error(msg)
            raise Exception(msg)
        self.logger.info(f"Infinity {infinity_uri} is healthy.")

    def _migrate_db(self, inf_conn):
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)
        fp_mapping = os.path.join(get_project_base_directory(), "conf", self.mapping_file_name)
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
                self.logger.info(f"INFINITY added following column to table {table_name}: {field_name} {field_info}")
                if field_info["type"] != "varchar" or "analyzer" not in field_info:
                    continue
                analyzers = field_info["analyzer"]
                if isinstance(analyzers, str):
                    analyzers = [analyzers]
                for analyzer in analyzers:
                    inf_table.create_index(
                        f"ft_{re.sub(r'[^a-zA-Z0-9]', '_', field_name)}_{re.sub(r'[^a-zA-Z0-9]', '_', analyzer)}",
                        IndexInfo(field_name, IndexType.FullText, {"ANALYZER": analyzer}),
                        ConflictType.Ignore,
                    )

    """
    Dataframe and fields convert
    """

    @staticmethod
    @abstractmethod
    def field_keyword(field_name: str):
        # judge keyword or not, such as "*_kwd" tag-like columns.
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def convert_select_fields(self, output_fields: list[str]) -> list[str]:
        # rm _kwd, _tks, _sm_tks, _with_weight suffix in field name.
        raise NotImplementedError("Not implemented")

    @staticmethod
    @abstractmethod
    def convert_matching_field(field_weight_str: str) -> str:
        # convert matching field to
        raise NotImplementedError("Not implemented")

    @staticmethod
    def list2str(lst: str | list, sep: str = " ") -> str:
        if isinstance(lst, str):
            return lst
        return sep.join(lst)

    def equivalent_condition_to_str(self, condition: dict, table_instance=None) -> str | None:
        assert "_id" not in condition
        columns = {}
        if table_instance:
            for n, ty, de, _ in table_instance.show_columns().rows():
                columns[n] = (ty, de)

        def exists(cln):
            nonlocal columns
            assert cln in columns, f"'{cln}' should be in '{columns}'."
            ty, de = columns[cln]
            if ty.lower().find("cha"):
                if not de:
                    de = ""
                return f" {cln}!='{de}' "
            return f"{cln}!={de}"

        cond = list()
        for k, v in condition.items():
            if not isinstance(k, str) or not v:
                continue
            if self.field_keyword(k):
                if isinstance(v, list):
                    inCond = list()
                    for item in v:
                        if isinstance(item, str):
                            item = item.replace("'", "''")
                        inCond.append(f"filter_fulltext('{self.convert_matching_field(k)}', '{item}')")
                    if inCond:
                        strInCond = " or ".join(inCond)
                        strInCond = f"({strInCond})"
                        cond.append(strInCond)
                else:
                    cond.append(f"filter_fulltext('{self.convert_matching_field(k)}', '{v}')")
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

    @staticmethod
    def concat_dataframes(df_list: list[pd.DataFrame], select_fields: list[str]) -> pd.DataFrame:
        df_list2 = [df for df in df_list if not df.empty]
        if df_list2:
            return pd.concat(df_list2, axis=0).reset_index(drop=True)

        schema = []
        for field_name in select_fields:
            if field_name == "score()":  # Workaround: fix schema is changed to score()
                schema.append("SCORE")
            elif field_name == "similarity()":  # Workaround: fix schema is changed to similarity()
                schema.append("SIMILARITY")
            else:
                schema.append(field_name)
        return pd.DataFrame(columns=schema)

    """
    Database operations
    """

    def db_type(self) -> str:
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

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int):
        table_name = f"{index_name}_{dataset_id}"
        inf_conn = self.connPool.get_conn()
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)

        fp_mapping = os.path.join(get_project_base_directory(), "conf", self.mapping_file_name)
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        schema = json.load(open(fp_mapping))
        vector_name = f"q_{vector_size}_vec"
        schema[vector_name] = {"type": f"vector,{vector_size},float"}
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
            analyzers = field_info["analyzer"]
            if isinstance(analyzers, str):
                analyzers = [analyzers]
            for analyzer in analyzers:
                inf_table.create_index(
                    f"ft_{re.sub(r'[^a-zA-Z0-9]', '_', field_name)}_{re.sub(r'[^a-zA-Z0-9]', '_', analyzer)}",
                    IndexInfo(field_name, IndexType.FullText, {"ANALYZER": analyzer}),
                    ConflictType.Ignore,
                )
        self.connPool.release_conn(inf_conn)
        self.logger.info(f"INFINITY created table {table_name}, vector size {vector_size}")
        return True

    def delete_idx(self, index_name: str, dataset_id: str):
        table_name = f"{index_name}_{dataset_id}"
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        db_instance.drop_table(table_name, ConflictType.Ignore)
        self.connPool.release_conn(inf_conn)
        self.logger.info(f"INFINITY dropped table {table_name}")

    def index_exist(self, index_name: str, dataset_id: str) -> bool:
        table_name = f"{index_name}_{dataset_id}"
        try:
            inf_conn = self.connPool.get_conn()
            db_instance = inf_conn.get_database(self.dbName)
            _ = db_instance.get_table(table_name)
            self.connPool.release_conn(inf_conn)
            return True
        except Exception as e:
            self.logger.warning(f"INFINITY indexExist {str(e)}")
        return False

    """
    CRUD operations
    """

    @abstractmethod
    def search(
        self,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        match_expressions: list[MatchExpr],
        order_by: OrderByExpr,
        offset: int,
        limit: int,
        index_names: str | list[str],
        dataset_ids: list[str],
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
    ) -> tuple[pd.DataFrame, int]:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get(self, doc_id: str, index_name: str, knowledgebase_ids: list[str]) -> dict | None:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def insert(self, documents: list[dict], index_name: str, dataset_ids: str = None) -> list[str]:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def update(self, condition: dict, new_value: dict, index_name: str, dataset_id: str) -> bool:
        raise NotImplementedError("Not implemented")

    def delete(self, condition: dict, index_name: str, dataset_id: str) -> int:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{index_name}_{dataset_id}"
        try:
            table_instance = db_instance.get_table(table_name)
        except Exception:
            self.logger.warning(f"Skipped deleting from table {table_name} since the table doesn't exist.")
            return 0
        filter = self.equivalent_condition_to_str(condition, table_instance)
        self.logger.debug(f"INFINITY delete table {table_name}, filter {filter}.")
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

    def get_doc_ids(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return list(res["id"])

    @abstractmethod
    def get_fields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        raise NotImplementedError("Not implemented")

    def get_highlight(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, keywords: list[str], field_name: str):
        if isinstance(res, tuple):
            res = res[0]
        ans = {}
        num_rows = len(res)
        column_id = res["id"]
        if field_name not in res:
            return {}
        for i in range(num_rows):
            id = column_id[i]
            txt = res[field_name][i]
            if re.search(r"<em>[^<>]+</em>", txt, flags=re.IGNORECASE | re.MULTILINE):
                ans[id] = txt
                continue
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txt_list = []
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
                txt_list.append(t)
            if txt_list:
                ans[id] = "...".join(txt_list)
            else:
                ans[id] = txt
        return ans

    def get_aggregation(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, field_name: str):
        """
        Manual aggregation for tag fields since Infinity doesn't provide native aggregation
        """
        from collections import Counter

        # Extract DataFrame from result
        if isinstance(res, tuple):
            df, _ = res
        else:
            df = res

        if df.empty or field_name not in df.columns:
            return []

        # Aggregate tag counts
        tag_counter = Counter()

        for value in df[field_name]:
            if pd.isna(value) or not value:
                continue

            # Handle different tag formats
            if isinstance(value, str):
                # Split by ### for tag_kwd field or comma for other formats
                if field_name == "tag_kwd" and "###" in value:
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

    def sql(self, sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
