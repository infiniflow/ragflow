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
import json
import logging
import os
import re
import threading
import time
from typing import Any, Optional

import numpy as np
from elasticsearch_dsl import Q, Search
from pydantic import BaseModel
from pymysql.converters import escape_string
from pyobvector import ObVecClient, FtsIndexParam, FtsParser, ARRAY, VECTOR
from pyobvector.client import ClusterVersionException
from pyobvector.client.hybrid_search import HybridSearch
from pyobvector.util import ObVersion
from sqlalchemy import text, Column, String, Integer, JSON, Double, Row, Table
from sqlalchemy.dialects.mysql import LONGTEXT, TEXT
from sqlalchemy.sql.type_api import TypeEngine

from common import settings
from common.constants import PAGERANK_FLD, TAG_FLD
from common.decorator import singleton
from common.float_utils import get_float
from common.doc_store.doc_store_base import DocStoreConnection, MatchExpr, OrderByExpr, FusionExpr, MatchTextExpr, \
    MatchDenseExpr
from rag.nlp import rag_tokenizer

ATTEMPT_TIME = 2
OB_QUERY_TIMEOUT = int(os.environ.get("OB_QUERY_TIMEOUT", "100_000_000"))

logger = logging.getLogger('ragflow.ob_conn')

column_order_id = Column("_order_id", Integer, nullable=True, comment="chunk order id for maintaining sequence")
column_group_id = Column("group_id", String(256), nullable=True, comment="group id for external retrieval")
column_mom_id = Column("mom_id", String(256), nullable=True, comment="parent chunk id")

column_definitions: list[Column] = [
    Column("id", String(256), primary_key=True, comment="chunk id"),
    Column("kb_id", String(256), nullable=False, index=True, comment="knowledge base id"),
    Column("doc_id", String(256), nullable=True, index=True, comment="document id"),
    Column("docnm_kwd", String(256), nullable=True, comment="document name"),
    Column("doc_type_kwd", String(256), nullable=True, comment="document type"),
    Column("title_tks", String(256), nullable=True, comment="title tokens"),
    Column("title_sm_tks", String(256), nullable=True, comment="fine-grained (small) title tokens"),
    Column("content_with_weight", LONGTEXT, nullable=True, comment="the original content"),
    Column("content_ltks", LONGTEXT, nullable=True, comment="long text tokens derived from content_with_weight"),
    Column("content_sm_ltks", LONGTEXT, nullable=True, comment="fine-grained (small) tokens derived from content_ltks"),
    Column("pagerank_fea", Integer, nullable=True, comment="page rank priority, usually set in kb level"),
    Column("important_kwd", ARRAY(String(256)), nullable=True, comment="keywords"),
    Column("important_tks", TEXT, nullable=True, comment="keyword tokens"),
    Column("question_kwd", ARRAY(String(1024)), nullable=True, comment="questions"),
    Column("question_tks", TEXT, nullable=True, comment="question tokens"),
    Column("tag_kwd", ARRAY(String(256)), nullable=True, comment="tags"),
    Column("tag_feas", JSON, nullable=True,
           comment="tag features used for 'rank_feature', format: [tag -> relevance score]"),
    Column("available_int", Integer, nullable=False, index=True, server_default="1",
           comment="status of availability, 0 for unavailable, 1 for available"),
    Column("create_time", String(19), nullable=True, comment="creation time in YYYY-MM-DD HH:MM:SS format"),
    Column("create_timestamp_flt", Double, nullable=True, comment="creation timestamp in float format"),
    Column("img_id", String(128), nullable=True, comment="image id"),
    Column("position_int", ARRAY(ARRAY(Integer)), nullable=True, comment="position"),
    Column("page_num_int", ARRAY(Integer), nullable=True, comment="page number"),
    Column("top_int", ARRAY(Integer), nullable=True, comment="rank from the top"),
    Column("knowledge_graph_kwd", String(256), nullable=True, index=True, comment="knowledge graph chunk type"),
    Column("source_id", ARRAY(String(256)), nullable=True, comment="source document id"),
    Column("entity_kwd", String(256), nullable=True, comment="entity name"),
    Column("entity_type_kwd", String(256), nullable=True, index=True, comment="entity type"),
    Column("from_entity_kwd", String(256), nullable=True, comment="the source entity of this edge"),
    Column("to_entity_kwd", String(256), nullable=True, comment="the target entity of this edge"),
    Column("weight_int", Integer, nullable=True, comment="the weight of this edge"),
    Column("weight_flt", Double, nullable=True, comment="the weight of community report"),
    Column("entities_kwd", ARRAY(String(256)), nullable=True, comment="node ids of entities"),
    Column("rank_flt", Double, nullable=True, comment="rank of this entity"),
    Column("removed_kwd", String(256), nullable=True, index=True, server_default="'N'",
           comment="whether it has been deleted"),
    Column("metadata", JSON, nullable=True, comment="metadata for this chunk"),
    Column("extra", JSON, nullable=True, comment="extra information of non-general chunk"),
    column_order_id,
    column_group_id,
    column_mom_id,
]

column_names: list[str] = [col.name for col in column_definitions]
column_types: dict[str, TypeEngine] = {col.name: col.type for col in column_definitions}
array_columns: list[str] = [col.name for col in column_definitions if isinstance(col.type, ARRAY)]

vector_column_pattern = re.compile(r"q_(?P<vector_size>\d+)_vec")

index_columns: list[str] = [
    "kb_id",
    "doc_id",
    "available_int",
    "knowledge_graph_kwd",
    "entity_type_kwd",
    "removed_kwd",
]

fts_columns_origin: list[str] = [
    "docnm_kwd^10",
    "content_with_weight",
    "important_tks^20",
    "question_tks^20",
]

fts_columns_tks: list[str] = [
    "title_tks^10",
    "title_sm_tks^5",
    "important_tks^20",
    "question_tks^20",
    "content_ltks^2",
    "content_sm_ltks",
]

index_name_template = "ix_%s_%s"
fulltext_index_name_template = "fts_idx_%s"
# MATCH AGAINST: https://www.oceanbase.com/docs/common-oceanbase-database-cn-1000000002017607
fulltext_search_template = "MATCH (%s) AGAINST ('%s' IN NATURAL LANGUAGE MODE)"
# cosine_distance: https://www.oceanbase.com/docs/common-oceanbase-database-cn-1000000002012938
vector_search_template = "cosine_distance(%s, '%s')"


class SearchResult(BaseModel):
    total: int
    chunks: list[dict]


def get_column_value(column_name: str, value: Any) -> Any:
    if column_name in column_types:
        column_type = column_types[column_name]
        if isinstance(column_type, String):
            return str(value)
        elif isinstance(column_type, Integer):
            return int(value)
        elif isinstance(column_type, Double):
            return float(value)
        elif isinstance(column_type, ARRAY) or isinstance(column_type, JSON):
            if isinstance(value, str):
                try:
                    return json.loads(value)
                except json.JSONDecodeError:
                    return value
            else:
                return value
        else:
            raise ValueError(f"Unsupported column type for column '{column_name}': {column_type}")
    elif vector_column_pattern.match(column_name):
        if isinstance(value, str):
            try:
                return json.loads(value)
            except json.JSONDecodeError:
                return value
        else:
            return value
    elif column_name == "_score":
        return float(value)
    else:
        raise ValueError(f"Unknown column '{column_name}' with value '{value}'.")


def get_default_value(column_name: str) -> Any:
    if column_name == "available_int":
        return 1
    elif column_name == "removed_kwd":
        return "N"
    elif column_name == "_order_id":
        return 0
    else:
        return None


def get_value_str(value: Any) -> str:
    if isinstance(value, str):
        cleaned_str = value.replace('\\', '\\\\')
        cleaned_str = cleaned_str.replace('\n', '\\n')
        cleaned_str = cleaned_str.replace('\r', '\\r')
        cleaned_str = cleaned_str.replace('\t', '\\t')
        return f"'{escape_string(cleaned_str)}'"
    elif isinstance(value, bool):
        return "true" if value else "false"
    elif value is None:
        return "NULL"
    elif isinstance(value, (list, dict)):
        json_str = json.dumps(value, ensure_ascii=False)
        return f"'{escape_string(json_str)}'"
    else:
        return str(value)


def get_metadata_filter_expression(metadata_filtering_conditions: dict) -> str:
    """
    Convert metadata filtering conditions to MySQL JSON path expression.

    Args:
        metadata_filtering_conditions: dict with 'conditions' and 'logical_operator' keys

    Returns:
        MySQL JSON path expression string
    """
    if not metadata_filtering_conditions:
        return ""

    conditions = metadata_filtering_conditions.get("conditions", [])
    logical_operator = metadata_filtering_conditions.get("logical_operator", "and").upper()

    if not conditions:
        return ""

    if logical_operator not in ["AND", "OR"]:
        raise ValueError(f"Unsupported logical operator: {logical_operator}. Only 'and' and 'or' are supported.")

    metadata_filters = []
    for condition in conditions:
        name = condition.get("name")
        comparison_operator = condition.get("comparison_operator")
        value = condition.get("value")

        if not all([name, comparison_operator]):
            continue

        expr = f"JSON_EXTRACT(metadata, '$.{name}')"
        value_str = get_value_str(value) if value else ""

        # Convert comparison operator to MySQL JSON path syntax
        if comparison_operator == "is":
            # JSON_EXTRACT(metadata, '$.field_name') = 'value'
            metadata_filters.append(f"{expr} = {value_str}")
        elif comparison_operator == "is not":
            metadata_filters.append(f"{expr} != {value_str}")
        elif comparison_operator == "contains":
            metadata_filters.append(f"JSON_CONTAINS({expr}, {value_str})")
        elif comparison_operator == "not contains":
            metadata_filters.append(f"NOT JSON_CONTAINS({expr}, {value_str})")
        elif comparison_operator == "start with":
            metadata_filters.append(f"{expr} LIKE CONCAT({value_str}, '%')")
        elif comparison_operator == "end with":
            metadata_filters.append(f"{expr} LIKE CONCAT('%', {value_str})")
        elif comparison_operator == "empty":
            metadata_filters.append(f"({expr} IS NULL OR {expr} = '' OR {expr} = '[]' OR {expr} = '{{}}')")
        elif comparison_operator == "not empty":
            metadata_filters.append(f"({expr} IS NOT NULL AND {expr} != '' AND {expr} != '[]' AND {expr} != '{{}}')")
        # Number operators
        elif comparison_operator == "=":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) = {value_str}")
        elif comparison_operator == "≠":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) != {value_str}")
        elif comparison_operator == ">":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) > {value_str}")
        elif comparison_operator == "<":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) < {value_str}")
        elif comparison_operator == "≥":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) >= {value_str}")
        elif comparison_operator == "≤":
            metadata_filters.append(f"CAST({expr} AS DECIMAL(20,10)) <= {value_str}")
        # Time operators
        elif comparison_operator == "before":
            metadata_filters.append(f"CAST({expr} AS DATETIME) < {value_str}")
        elif comparison_operator == "after":
            metadata_filters.append(f"CAST({expr} AS DATETIME) > {value_str}")
        else:
            logger.warning(f"Unsupported comparison operator: {comparison_operator}")
            continue

    if not metadata_filters:
        return ""

    return f"({f' {logical_operator} '.join(metadata_filters)})"


def get_filters(condition: dict) -> list[str]:
    filters: list[str] = []
    for k, v in condition.items():
        if not v:
            continue

        if k == "exists":
            filters.append(f"{v} IS NOT NULL")
        elif k == "must_not" and isinstance(v, dict) and "exists" in v:
            filters.append(f"{v.get('exists')} IS NULL")
        elif k == "metadata_filtering_conditions":
            # Handle metadata filtering conditions
            metadata_filter = get_metadata_filter_expression(v)
            if metadata_filter:
                filters.append(metadata_filter)
        elif k in array_columns:
            if isinstance(v, list):
                array_filters = []
                for vv in v:
                    array_filters.append(f"array_contains({k}, {get_value_str(vv)})")
                array_filter = " OR ".join(array_filters)
                filters.append(f"({array_filter})")
            else:
                filters.append(f"array_contains({k}, {get_value_str(v)})")
        elif isinstance(v, list):
            values: list[str] = []
            for item in v:
                values.append(get_value_str(item))
            value = ", ".join(values)
            filters.append(f"{k} IN ({value})")
        else:
            filters.append(f"{k} = {get_value_str(v)}")
    return filters


def _try_with_lock(lock_name: str, process_func, check_func, timeout: int = None):
    if not timeout:
        timeout = int(os.environ.get("OB_DDL_TIMEOUT", "60"))

    if not check_func():
        from rag.utils.redis_conn import RedisDistributedLock
        lock = RedisDistributedLock(lock_name)
        if lock.acquire():
            logger.info(f"acquired lock success: {lock_name}, start processing.")
            try:
                process_func()
                return
            except Exception as e:
                if "Duplicate" in str(e):
                    # In some cases, the schema may change after the lock is acquired, so if the error message
                    # indicates that the column or index is duplicated, it should be assumed that 'process_func'
                    # has been executed correctly.
                    logger.warning(f"Skip processing {lock_name} due to duplication: {str(e)}")
                    return
                raise
            finally:
                lock.release()

    if not check_func():
        logger.info(f"Waiting for process complete for {lock_name} on other task executors.")
        time.sleep(1)
        count = 1
        while count < timeout and not check_func():
            count += 1
            time.sleep(1)
        if count >= timeout and not check_func():
            raise Exception(f"Timeout to wait for process complete for {lock_name}.")


@singleton
class OBConnection(DocStoreConnection):
    def __init__(self):
        scheme: str = settings.OB.get("scheme")
        ob_config = settings.OB.get("config", {})

        if scheme and scheme.lower() == "mysql":
            mysql_config = settings.get_base_config("mysql", {})
            logger.info("Use MySQL scheme to create OceanBase connection.")
            host = mysql_config.get("host", "localhost")
            port = mysql_config.get("port", 2881)
            self.username = mysql_config.get("user", "root@test")
            self.password = mysql_config.get("password", "infini_rag_flow")
            max_connections = mysql_config.get("max_connections", 300)
        else:
            logger.info("Use customized config to create OceanBase connection.")
            host = ob_config.get("host", "localhost")
            port = ob_config.get("port", 2881)
            self.username = ob_config.get("user", "root@test")
            self.password = ob_config.get("password", "infini_rag_flow")
            max_connections = ob_config.get("max_connections", 300)

        self.db_name = ob_config.get("db_name", "test")
        self.uri = f"{host}:{port}"

        logger.info(f"Use OceanBase '{self.uri}' as the doc engine.")

        # Set the maximum number of connections that can be created above the pool_size.
        # By default, this is half of max_connections, but at least 10.
        # This allows the pool to handle temporary spikes in demand without exhausting resources.
        max_overflow = int(os.environ.get("OB_MAX_OVERFLOW", max(max_connections // 2, 10)))
        # Set the number of seconds to wait before giving up when trying to get a connection from the pool.
        # Default is 30 seconds, but can be overridden with the OB_POOL_TIMEOUT environment variable.
        pool_timeout = int(os.environ.get("OB_POOL_TIMEOUT", "30"))

        for _ in range(ATTEMPT_TIME):
            try:
                self.client = ObVecClient(
                    uri=self.uri,
                    user=self.username,
                    password=self.password,
                    db_name=self.db_name,
                    pool_pre_ping=True,
                    pool_recycle=3600,
                    pool_size=max_connections,
                    max_overflow=max_overflow,
                    pool_timeout=pool_timeout,
                )
                break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting OceanBase {self.uri} to be healthy.")
                time.sleep(5)

        if self.client is None:
            msg = f"OceanBase {self.uri} connection failed after {ATTEMPT_TIME} attempts."
            logger.error(msg)
            raise Exception(msg)

        self._load_env_vars()
        self._check_ob_version()
        self._try_to_update_ob_query_timeout()

        self.es = None
        if self.enable_hybrid_search:
            try:
                self.es = HybridSearch(
                    uri=self.uri,
                    user=self.username,
                    password=self.password,
                    db_name=self.db_name,
                    pool_pre_ping=True,
                    pool_recycle=3600,
                    pool_size=max_connections,
                    max_overflow=max_overflow,
                    pool_timeout=pool_timeout,
                )
                logger.info("OceanBase Hybrid Search feature is enabled")
            except ClusterVersionException as e:
                logger.info("Failed to initialize HybridSearch client, fallback to use SQL", exc_info=e)
                self.es = None

        if self.es is not None and self.search_original_content:
            logger.info("HybridSearch is enabled, forcing search_original_content to False")
            self.search_original_content = False
        # Determine which columns to use for full-text search dynamically:
        # If HybridSearch is enabled (self.es is not None), we must use tokenized columns (fts_columns_tks)
        # for compatibility and performance with HybridSearch. Otherwise, we use the original content columns
        # (fts_columns_origin), which may be controlled by an environment variable.
        self.fulltext_search_columns = fts_columns_origin if self.search_original_content else fts_columns_tks

        self._table_exists_cache: set[str] = set()
        self._table_exists_cache_lock = threading.RLock()

        logger.info(f"OceanBase {self.uri} is healthy.")

    def _check_ob_version(self):
        try:
            res = self.client.perform_raw_text_sql("SELECT OB_VERSION() FROM DUAL").fetchone()
            version_str = res[0] if res else None
            logger.info(f"OceanBase {self.uri} version is {version_str}")
        except Exception as e:
            raise Exception(f"Failed to get OceanBase version from {self.uri}, error: {str(e)}")

        if not version_str:
            raise Exception(f"Failed to get OceanBase version from {self.uri}.")

        ob_version = ObVersion.from_db_version_string(version_str)
        if ob_version < ObVersion.from_db_version_nums(4, 3, 5, 1):
            raise Exception(
                f"The version of OceanBase needs to be higher than or equal to 4.3.5.1, current version is {version_str}"
            )

    def _try_to_update_ob_query_timeout(self):
        try:
            val = self._get_variable_value("ob_query_timeout")
            if val and int(val) >= OB_QUERY_TIMEOUT:
                return
        except Exception as e:
            logger.warning("Failed to get 'ob_query_timeout' variable: %s", str(e))

        try:
            self.client.perform_raw_text_sql(f"SET GLOBAL ob_query_timeout={OB_QUERY_TIMEOUT}")
            logger.info("Set GLOBAL variable 'ob_query_timeout' to %d.", OB_QUERY_TIMEOUT)

            # refresh connection pool to ensure 'ob_query_timeout' has taken effect
            self.client.engine.dispose()
            if self.es is not None:
                self.es.engine.dispose()
            logger.info("Disposed all connections in engine pool to refresh connection pool")
        except Exception as e:
            logger.warning(f"Failed to set 'ob_query_timeout' variable: {str(e)}")

    def _load_env_vars(self):

        def is_true(var: str, default: str) -> bool:
            return os.getenv(var, default).lower() in ['true', '1', 'yes', 'y']

        self.enable_fulltext_search = is_true('ENABLE_FULLTEXT_SEARCH', 'true')
        logger.info(f"ENABLE_FULLTEXT_SEARCH={self.enable_fulltext_search}")

        self.use_fulltext_hint = is_true('USE_FULLTEXT_HINT', 'true')
        logger.info(f"USE_FULLTEXT_HINT={self.use_fulltext_hint}")

        self.search_original_content = is_true("SEARCH_ORIGINAL_CONTENT", 'true')
        logger.info(f"SEARCH_ORIGINAL_CONTENT={self.search_original_content}")

        self.enable_hybrid_search = is_true('ENABLE_HYBRID_SEARCH', 'false')
        logger.info(f"ENABLE_HYBRID_SEARCH={self.enable_hybrid_search}")

        self.use_fulltext_first_fusion_search = is_true('USE_FULLTEXT_FIRST_FUSION_SEARCH', 'true')
        logger.info(f"USE_FULLTEXT_FIRST_FUSION_SEARCH={self.use_fulltext_first_fusion_search}")

    """
    Database operations
    """

    def db_type(self) -> str:
        return "oceanbase"

    def health(self) -> dict:
        return {
            "uri": self.uri,
            "version_comment": self._get_variable_value("version_comment")
        }

    def _get_variable_value(self, var_name: str) -> Any:
        rows = self.client.perform_raw_text_sql(f"SHOW VARIABLES LIKE '{var_name}'")
        for row in rows:
            return row[1]
        raise Exception(f"Variable '{var_name}' not found.")

    def _check_table_exists_cached(self, table_name: str) -> bool:
        """
        Check table existence with cache to reduce INFORMATION_SCHEMA queries under high concurrency.
        Only caches when table exists. Does not cache when table does not exist.
        Thread-safe implementation: read operations are lock-free (GIL-protected),
        write operations are protected by RLock to ensure cache consistency.

        Args:
            table_name: Table name

        Returns:
            Whether the table exists with all required indexes and columns
        """
        if table_name in self._table_exists_cache:
            return True

        try:
            if not self.client.check_table_exists(table_name):
                return False
            for column_name in index_columns:
                if not self._index_exists(table_name, index_name_template % (table_name, column_name)):
                    return False
            for fts_column in self.fulltext_search_columns:
                column_name = fts_column.split("^")[0]
                if not self._index_exists(table_name, fulltext_index_name_template % column_name):
                    return False
            for column in [column_order_id, column_group_id, column_mom_id]:
                if not self._column_exist(table_name, column.name):
                    return False
        except Exception as e:
            raise Exception(f"OBConnection._check_table_exists_cached error: {str(e)}")

        with self._table_exists_cache_lock:
            if table_name not in self._table_exists_cache:
                self._table_exists_cache.add(table_name)
        return True

    """
    Table operations
    """

    def create_idx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        vector_field_name = f"q_{vectorSize}_vec"
        vector_index_name = f"{vector_field_name}_idx"

        try:
            _try_with_lock(
                lock_name=f"ob_create_table_{indexName}",
                check_func=lambda: self.client.check_table_exists(indexName),
                process_func=lambda: self._create_table(indexName),
            )

            for column_name in index_columns:
                _try_with_lock(
                    lock_name=f"ob_add_idx_{indexName}_{column_name}",
                    check_func=lambda: self._index_exists(indexName, index_name_template % (indexName, column_name)),
                    process_func=lambda: self._add_index(indexName, column_name),
                )

            for fts_column in self.fulltext_search_columns:
                column_name = fts_column.split("^")[0]
                _try_with_lock(
                    lock_name=f"ob_add_fulltext_idx_{indexName}_{column_name}",
                    check_func=lambda: self._index_exists(indexName, fulltext_index_name_template % column_name),
                    process_func=lambda: self._add_fulltext_index(indexName, column_name),
                )

            _try_with_lock(
                lock_name=f"ob_add_vector_column_{indexName}_{vector_field_name}",
                check_func=lambda: self._column_exist(indexName, vector_field_name),
                process_func=lambda: self._add_vector_column(indexName, vectorSize),
            )

            _try_with_lock(
                lock_name=f"ob_add_vector_idx_{indexName}_{vector_field_name}",
                check_func=lambda: self._index_exists(indexName, vector_index_name),
                process_func=lambda: self._add_vector_index(indexName, vector_field_name),
            )

            # new columns migration
            for column in [column_order_id, column_group_id, column_mom_id]:
                _try_with_lock(
                    lock_name=f"ob_add_{column.name}_{indexName}",
                    check_func=lambda: self._column_exist(indexName, column.name),
                    process_func=lambda: self._add_column(indexName, column),
                )
        except Exception as e:
            raise Exception(f"OBConnection.createIndex error: {str(e)}")
        finally:
            # always refresh metadata to make sure it contains the latest table structure
            self.client.refresh_metadata([indexName])

    def delete_idx(self, indexName: str, knowledgebaseId: str):
        if len(knowledgebaseId) > 0:
            # The index need to be alive after any kb deletion since all kb under this tenant are in one index.
            return
        try:
            if self.client.check_table_exists(table_name=indexName):
                self.client.drop_table_if_exist(indexName)
                logger.info(f"Dropped table '{indexName}'.")
        except Exception as e:
            raise Exception(f"OBConnection.deleteIndex error: {str(e)}")

    def index_exist(self, indexName: str, knowledgebaseId: str = None) -> bool:
        return self._check_table_exists_cached(indexName)

    def _get_count(self, table_name: str, filter_list: list[str] = None) -> int:
        where_clause = "WHERE " + " AND ".join(filter_list) if len(filter_list) > 0 else ""
        (count,) = self.client.perform_raw_text_sql(
            f"SELECT COUNT(*) FROM {table_name} {where_clause}"
        ).fetchone()
        return count

    def _column_exist(self, table_name: str, column_name: str) -> bool:
        return self._get_count(
            table_name="INFORMATION_SCHEMA.COLUMNS",
            filter_list=[
                f"TABLE_SCHEMA = '{self.db_name}'",
                f"TABLE_NAME = '{table_name}'",
                f"COLUMN_NAME = '{column_name}'",
            ]) > 0

    def _index_exists(self, table_name: str, index_name: str) -> bool:
        return self._get_count(
            table_name="INFORMATION_SCHEMA.STATISTICS",
            filter_list=[
                f"TABLE_SCHEMA = '{self.db_name}'",
                f"TABLE_NAME = '{table_name}'",
                f"INDEX_NAME = '{index_name}'",
            ]) > 0

    def _create_table(self, table_name: str):
        # remove outdated metadata for external changes
        if table_name in self.client.metadata_obj.tables:
            self.client.metadata_obj.remove(Table(table_name, self.client.metadata_obj))

        table_options = {
            "mysql_charset": "utf8mb4",
            "mysql_collate": "utf8mb4_unicode_ci",
            "mysql_organization": "heap",
        }

        self.client.create_table(
            table_name=table_name,
            columns=[c.copy() for c in column_definitions],
            **table_options,
        )
        logger.info(f"Created table '{table_name}'.")

    def _add_index(self, table_name: str, column_name: str):
        index_name = index_name_template % (table_name, column_name)
        self.client.create_index(
            table_name=table_name,
            is_vec_index=False,
            index_name=index_name,
            column_names=[column_name],
        )
        logger.info(f"Created index '{index_name}' on table '{table_name}'.")

    def _add_fulltext_index(self, table_name: str, column_name: str):
        fulltext_index_name = fulltext_index_name_template % column_name
        self.client.create_fts_idx_with_fts_index_param(
            table_name=table_name,
            fts_idx_param=FtsIndexParam(
                index_name=fulltext_index_name,
                field_names=[column_name],
                parser_type=FtsParser.IK,
            ),
        )
        logger.info(f"Created full text index '{fulltext_index_name}' on table '{table_name}'.")

    def _add_vector_column(self, table_name: str, vector_size: int):
        vector_field_name = f"q_{vector_size}_vec"

        self.client.add_columns(
            table_name=table_name,
            columns=[Column(vector_field_name, VECTOR(vector_size), nullable=True)],
        )
        logger.info(f"Added vector column '{vector_field_name}' to table '{table_name}'.")

    def _add_vector_index(self, table_name: str, vector_field_name: str):
        vector_index_name = f"{vector_field_name}_idx"
        self.client.create_index(
            table_name=table_name,
            is_vec_index=True,
            index_name=vector_index_name,
            column_names=[vector_field_name],
            vidx_params="distance=cosine, type=hnsw, lib=vsag",
        )
        logger.info(
            f"Created vector index '{vector_index_name}' on table '{table_name}' with column '{vector_field_name}'."
        )

    def _add_column(self, table_name: str, column: Column):
        try:
            self.client.add_columns(
                table_name=table_name,
                columns=[column.copy()],
            )
            logger.info(f"Added column '{column.name}' to table '{table_name}'.")
        except Exception as e:
            logger.warning(f"Failed to add column '{column.name}' to table '{table_name}': {str(e)}")

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
            **kwargs,
    ):
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        indexNames = list(set(indexNames))

        if len(matchExprs) == 3:
            if not self.enable_fulltext_search:
                # disable fulltext search in fusion search, which means fallback to vector search
                matchExprs = [m for m in matchExprs if isinstance(m, MatchDenseExpr)]
            else:
                for m in matchExprs:
                    if isinstance(m, FusionExpr):
                        weights = m.fusion_params["weights"]
                        vector_similarity_weight = get_float(weights.split(",")[1])
                        # skip the search if its weight is zero
                        if vector_similarity_weight <= 0.0:
                            matchExprs = [m for m in matchExprs if isinstance(m, MatchTextExpr)]
                        elif vector_similarity_weight >= 1.0:
                            matchExprs = [m for m in matchExprs if isinstance(m, MatchDenseExpr)]

        result: SearchResult = SearchResult(
            total=0,
            chunks=[],
        )

        # copied from es_conn.py
        if len(matchExprs) == 3 and self.es:
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
                if not v:
                    continue
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
                    assert len(matchExprs) == 3 and isinstance(matchExprs[0], MatchTextExpr) and isinstance(
                        matchExprs[1],
                        MatchDenseExpr) and isinstance(
                        matchExprs[2], FusionExpr)
                    weights = m.fusion_params["weights"]
                    vector_similarity_weight = get_float(weights.split(",")[1])
            for m in matchExprs:
                if isinstance(m, MatchTextExpr):
                    minimum_should_match = m.extra_options.get("minimum_should_match", 0.0)
                    if isinstance(minimum_should_match, float):
                        minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    bqry.must.append(Q("query_string", fields=fts_columns_tks,
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

            if bqry and rank_feature:
                for fld, sc in rank_feature.items():
                    if fld != PAGERANK_FLD:
                        fld = f"{TAG_FLD}.{fld}"
                    bqry.should.append(Q("rank_feature", field=fld, linear={}, boost=sc))

            if bqry:
                s = s.query(bqry)
            # for field in highlightFields:
            #     s = s.highlight(field)

            if orderBy:
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

            for fld in aggFields:
                s.aggs.bucket(f'aggs_{fld}', 'terms', field=fld, size=1000000)

            if limit > 0:
                s = s[offset:offset + limit]
            q = s.to_dict()
            logger.debug(f"OBConnection.hybrid_search {str(indexNames)} query: " + json.dumps(q))

            for index_name in indexNames:
                start_time = time.time()
                res = self.es.search(index=index_name,
                                     body=q,
                                     timeout="600s",
                                     track_total_hits=True,
                                     _source=True)
                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: hybrid, elapsed time: {elapsed_time:.3f} seconds,"
                    f" got count: {len(res)}"
                )
                for chunk in res:
                    result.chunks.append(self._es_row_to_entity(chunk))
                    result.total = result.total + 1
            return result

        output_fields = selectFields.copy()
        if "id" not in output_fields:
            output_fields = ["id"] + output_fields
        if "_score" in output_fields:
            output_fields.remove("_score")

        if highlightFields:
            for field in highlightFields:
                if field not in output_fields:
                    output_fields.append(field)

        fields_expr = ", ".join(output_fields)

        condition["kb_id"] = knowledgebaseIds
        filters: list[str] = get_filters(condition)
        filters_expr = " AND ".join(filters)

        fulltext_query: Optional[str] = None
        fulltext_topn: Optional[int] = None
        fulltext_search_weight: dict[str, float] = {}
        fulltext_search_expr: dict[str, str] = {}
        fulltext_search_idx_list: list[str] = []
        fulltext_search_score_expr: Optional[str] = None
        fulltext_search_filter: Optional[str] = None

        vector_column_name: Optional[str] = None
        vector_data: Optional[list[float]] = None
        vector_topn: Optional[int] = None
        vector_similarity_threshold: Optional[float] = None
        vector_similarity_weight: Optional[float] = None
        vector_search_expr: Optional[str] = None
        vector_search_score_expr: Optional[str] = None
        vector_search_filter: Optional[str] = None

        for m in matchExprs:
            if isinstance(m, MatchTextExpr):
                assert "original_query" in m.extra_options, "'original_query' is missing in extra_options."
                fulltext_query = m.extra_options["original_query"]
                fulltext_query = escape_string(fulltext_query.strip())
                fulltext_topn = m.topn

                # get fulltext match expression and weight values
                for field in self.fulltext_search_columns:
                    parts = field.split("^")
                    column_name: str = parts[0]
                    column_weight: float = float(parts[1]) if (len(parts) > 1 and parts[1]) else 1.0

                    fulltext_search_weight[column_name] = column_weight
                    fulltext_search_expr[column_name] = fulltext_search_template % (column_name, fulltext_query)
                    fulltext_search_idx_list.append(fulltext_index_name_template % column_name)

                # adjust the weight to 0~1
                weight_sum = sum(fulltext_search_weight.values())
                for column_name in fulltext_search_weight.keys():
                    fulltext_search_weight[column_name] = fulltext_search_weight[column_name] / weight_sum

            elif isinstance(m, MatchDenseExpr):
                assert m.embedding_data_type == "float", f"embedding data type '{m.embedding_data_type}' is not float."
                vector_column_name = m.vector_column_name
                vector_data = m.embedding_data
                vector_topn = m.topn
                vector_similarity_threshold = m.extra_options.get("similarity", 0.0)
            elif isinstance(m, FusionExpr):
                weights = m.fusion_params["weights"]
                vector_similarity_weight = get_float(weights.split(",")[1])

        if fulltext_query:
            fulltext_search_filter = f"({' OR '.join([expr for expr in fulltext_search_expr.values()])})"
            fulltext_search_score_expr = f"({' + '.join(f'{expr} * {fulltext_search_weight.get(col, 0)}' for col, expr in fulltext_search_expr.items())})"

        if vector_data:
            vector_data_str = "[" + ",".join([str(np.float32(v)) for v in vector_data]) + "]"
            vector_search_expr = vector_search_template % (vector_column_name, vector_data_str)
            # use (1 - cosine_distance) as score, which should be [-1, 1]
            # https://www.oceanbase.com/docs/common-oceanbase-database-standalone-1000000003577323
            vector_search_score_expr = f"(1 - {vector_search_expr})"
            vector_search_filter = f"{vector_search_score_expr} >= {vector_similarity_threshold}"

        pagerank_score_expr = f"(CAST(IFNULL({PAGERANK_FLD}, 0) AS DECIMAL(10, 2)) / 100)"

        # TODO use tag rank_feature in sorting
        # tag_rank_fea = {k: float(v) for k, v in (rank_feature or {}).items() if k != PAGERANK_FLD}

        if fulltext_query and vector_data:
            search_type = "fusion"
        elif fulltext_query:
            search_type = "fulltext"
        elif vector_data:
            search_type = "vector"
        elif len(aggFields) > 0:
            search_type = "aggregation"
        else:
            search_type = "filter"

        if search_type in ["fusion", "fulltext", "vector"] and "_score" not in output_fields:
            output_fields.append("_score")

        if limit:
            if vector_topn is not None:
                limit = min(vector_topn, limit)
            if fulltext_topn is not None:
                limit = min(fulltext_topn, limit)

        for index_name in indexNames:

            if not self._check_table_exists_cached(index_name):
                continue

            fulltext_search_hint = f"/*+ UNION_MERGE({index_name} {' '.join(fulltext_search_idx_list)}) */" if self.use_fulltext_hint else ""

            if search_type == "fusion":
                # fusion search, usually for chat
                num_candidates = vector_topn + fulltext_topn
                if self.use_fulltext_first_fusion_search:
                    count_sql = (
                        f"WITH fulltext_results AS ("
                        f"  SELECT {fulltext_search_hint} *, {fulltext_search_score_expr} AS relevance"
                        f"      FROM {index_name}"
                        f"      WHERE {filters_expr} AND {fulltext_search_filter}"
                        f"      ORDER BY relevance DESC"
                        f"      LIMIT {num_candidates}"
                        f")"
                        f"  SELECT COUNT(*) FROM fulltext_results WHERE {vector_search_filter}"
                    )
                else:
                    count_sql = (
                        f"WITH fulltext_results AS ("
                        f"  SELECT {fulltext_search_hint} id FROM {index_name}"
                        f"      WHERE {filters_expr} AND {fulltext_search_filter}"
                        f"      ORDER BY {fulltext_search_score_expr}"
                        f"      LIMIT {fulltext_topn}"
                        f"),"
                        f"vector_results AS ("
                        f"  SELECT id FROM {index_name}"
                        f"      WHERE {filters_expr} AND {vector_search_filter}"
                        f"      ORDER BY {vector_search_expr}"
                        f"      APPROXIMATE LIMIT {vector_topn}"
                        f")"
                        f"  SELECT COUNT(*) FROM fulltext_results f FULL OUTER JOIN vector_results v ON f.id = v.id"
                    )
                logger.debug("OBConnection.search with count sql: %s", count_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(count_sql)
                total_count = res.fetchone()[0] if res else 0
                result.total += total_count

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: fusion, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" vector column: '{vector_column_name}',"
                    f" query text: '{fulltext_query}',"
                    f" condition: '{condition}',"
                    f" vector_similarity_threshold: {vector_similarity_threshold},"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                if self.use_fulltext_first_fusion_search:
                    score_expr = f"(relevance * {1 - vector_similarity_weight} + {vector_search_score_expr} * {vector_similarity_weight} + {pagerank_score_expr})"
                    fusion_sql = (
                        f"WITH fulltext_results AS ("
                        f"  SELECT {fulltext_search_hint} *, {fulltext_search_score_expr} AS relevance"
                        f"      FROM {index_name}"
                        f"      WHERE {filters_expr} AND {fulltext_search_filter}"
                        f"      ORDER BY relevance DESC"
                        f"      LIMIT {num_candidates}"
                        f")"
                        f"  SELECT {fields_expr}, {score_expr} AS _score"
                        f"      FROM fulltext_results"
                        f"      WHERE {vector_search_filter}"
                        f"      ORDER BY _score DESC"
                        f"      LIMIT {offset}, {limit}"
                    )
                else:
                    pagerank_score_expr = f"(CAST(IFNULL(f.{PAGERANK_FLD}, 0) AS DECIMAL(10, 2)) / 100)"
                    score_expr = f"(f.relevance * {1 - vector_similarity_weight} + v.similarity * {vector_similarity_weight} + {pagerank_score_expr})"
                    fields_expr = ", ".join([f"t.{f} as {f}" for f in output_fields if f != "_score"])
                    fusion_sql = (
                        f"WITH fulltext_results AS ("
                        f"  SELECT {fulltext_search_hint} id, pagerank_fea, {fulltext_search_score_expr} AS relevance"
                        f"      FROM {index_name}"
                        f"      WHERE {filters_expr} AND {fulltext_search_filter}"
                        f"      ORDER BY relevance DESC"
                        f"      LIMIT {fulltext_topn}"
                        f"),"
                        f"vector_results AS ("
                        f"  SELECT id, pagerank_fea, {vector_search_score_expr} AS similarity"
                        f"      FROM {index_name}"
                        f"      WHERE {filters_expr} AND {vector_search_filter}"
                        f"      ORDER BY {vector_search_expr}"
                        f"      APPROXIMATE LIMIT {vector_topn}"
                        f"),"
                        f"combined_results AS ("
                        f"  SELECT COALESCE(f.id, v.id) AS id, {score_expr} AS score"
                        f"      FROM fulltext_results f"
                        f"      FULL OUTER JOIN vector_results v"
                        f"      ON f.id = v.id"
                        f")"
                        f"  SELECT {fields_expr}, c.score as _score"
                        f"      FROM combined_results c"
                        f"      JOIN {index_name} t"
                        f"      ON c.id = t.id"
                        f"      ORDER BY score DESC"
                        f"      LIMIT {offset}, {limit}"
                    )
                logger.debug("OBConnection.search with fusion sql: %s", fusion_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(fusion_sql)
                rows = res.fetchall()

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: fusion, step: 2-query, elapsed time: {elapsed_time:.3f} seconds,"
                    f" select fields: '{output_fields}',"
                    f" vector column: '{vector_column_name}',"
                    f" query text: '{fulltext_query}',"
                    f" condition: '{condition}',"
                    f" vector_similarity_threshold: {vector_similarity_threshold},"
                    f" vector_similarity_weight: {vector_similarity_weight},"
                    f" return rows count: {len(rows)}"
                )

                for row in rows:
                    result.chunks.append(self._row_to_entity(row, output_fields))
            elif search_type == "vector":
                # vector search, usually used for graph search
                count_sql = f"SELECT COUNT(id) FROM {index_name} WHERE {filters_expr} AND {vector_search_filter}"
                logger.debug("OBConnection.search with vector count sql: %s", count_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(count_sql)
                total_count = res.fetchone()[0] if res else 0
                result.total += total_count

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: vector, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" vector column: '{vector_column_name}',"
                    f" condition: '{condition}',"
                    f" vector_similarity_threshold: {vector_similarity_threshold},"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                vector_sql = (
                    f"SELECT {fields_expr}, {vector_search_score_expr} AS _score"
                    f"  FROM {index_name}"
                    f"  WHERE {filters_expr} AND {vector_search_filter}"
                    f"  ORDER BY {vector_search_expr}"
                    f"  APPROXIMATE LIMIT {limit if limit != 0 else vector_topn}"
                )
                if offset != 0:
                    vector_sql += f" OFFSET {offset}"
                logger.debug("OBConnection.search with vector sql: %s", vector_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(vector_sql)
                rows = res.fetchall()

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: vector, step: 2-query, elapsed time: {elapsed_time:.3f} seconds,"
                    f" select fields: '{output_fields}',"
                    f" vector column: '{vector_column_name}',"
                    f" condition: '{condition}',"
                    f" vector_similarity_threshold: {vector_similarity_threshold},"
                    f" return rows count: {len(rows)}"
                )

                for row in rows:
                    result.chunks.append(self._row_to_entity(row, output_fields))
            elif search_type == "fulltext":
                # fulltext search, usually used to search chunks in one dataset
                count_sql = f"SELECT {fulltext_search_hint} COUNT(id) FROM {index_name} WHERE {filters_expr} AND {fulltext_search_filter}"
                logger.debug("OBConnection.search with fulltext count sql: %s", count_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(count_sql)
                total_count = res.fetchone()[0] if res else 0
                result.total += total_count

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: fulltext, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" query text: '{fulltext_query}',"
                    f" condition: '{condition}',"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                fulltext_sql = (
                    f"SELECT {fulltext_search_hint} {fields_expr}, {fulltext_search_score_expr} AS _score"
                    f"  FROM {index_name}"
                    f"  WHERE {filters_expr} AND {fulltext_search_filter}"
                    f"  ORDER BY _score DESC"
                    f"  LIMIT {offset}, {limit if limit != 0 else fulltext_topn}"
                )
                logger.debug("OBConnection.search with fulltext sql: %s", fulltext_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(fulltext_sql)
                rows = res.fetchall()

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: fulltext, step: 2-query, elapsed time: {elapsed_time:.3f} seconds,"
                    f" select fields: '{output_fields}',"
                    f" query text: '{fulltext_query}',"
                    f" condition: '{condition}',"
                    f" return rows count: {len(rows)}"
                )

                for row in rows:
                    result.chunks.append(self._row_to_entity(row, output_fields))
            elif search_type == "aggregation":
                # aggregation search
                assert len(aggFields) == 1, "Only one aggregation field is supported in OceanBase."
                agg_field = aggFields[0]
                if agg_field in array_columns:
                    res = self.client.perform_raw_text_sql(
                        f"SELECT {agg_field} FROM {index_name}"
                        f" WHERE {agg_field} IS NOT NULL AND {filters_expr}"
                    )
                    counts = {}
                    for row in res:
                        if row[0]:
                            if isinstance(row[0], str):
                                try:
                                    arr = json.loads(row[0])
                                except json.JSONDecodeError:
                                    logger.warning(f"Failed to parse JSON array: {row[0]}")
                                    continue
                            else:
                                arr = row[0]

                            if isinstance(arr, list):
                                for v in arr:
                                    if isinstance(v, str) and v.strip():
                                        counts[v] = counts.get(v, 0) + 1

                    for v, count in counts.items():
                        result.chunks.append({
                            "value": v,
                            "count": count,
                        })
                    result.total += len(counts)
                else:
                    res = self.client.perform_raw_text_sql(
                        f"SELECT {agg_field}, COUNT(*) as count FROM {index_name}"
                        f" WHERE {agg_field} IS NOT NULL AND {filters_expr}"
                        f" GROUP BY {agg_field}"
                    )
                    for row in res:
                        result.chunks.append({
                            "value": row[0],
                            "count": int(row[1]),
                        })
                        result.total += 1
            else:
                # only filter
                orders: list[str] = []
                if orderBy:
                    for field, order in orderBy.fields:
                        if isinstance(column_types[field], ARRAY):
                            f = field + "_sort"
                            fields_expr += f", array_to_string({field}, ',') AS {f}"
                            field = f
                        order = "ASC" if order == 0 else "DESC"
                        orders.append(f"{field} {order}")
                count_sql = f"SELECT COUNT(id) FROM {index_name} WHERE {filters_expr}"
                logger.debug("OBConnection.search with normal count sql: %s", count_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(count_sql)
                total_count = res.fetchone()[0] if res else 0
                result.total += total_count

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: normal, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" condition: '{condition}',"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                order_by_expr = ("ORDER BY " + ", ".join(orders)) if len(orders) > 0 else ""
                limit_expr = f"LIMIT {offset}, {limit}" if limit != 0 else ""
                filter_sql = (
                    f"SELECT {fields_expr}"
                    f"  FROM {index_name}"
                    f"  WHERE {filters_expr}"
                    f"  {order_by_expr} {limit_expr}"
                )
                logger.debug("OBConnection.search with normal sql: %s", filter_sql)

                start_time = time.time()

                res = self.client.perform_raw_text_sql(filter_sql)
                rows = res.fetchall()

                elapsed_time = time.time() - start_time
                logger.info(
                    f"OBConnection.search table {index_name}, search type: normal, step: 2-query, elapsed time: {elapsed_time:.3f} seconds,"
                    f" select fields: '{output_fields}',"
                    f" condition: '{condition}',"
                    f" return rows count: {len(rows)}"
                )

                for row in rows:
                    result.chunks.append(self._row_to_entity(row, output_fields))

        if result.total == 0:
            result.total = len(result.chunks)

        return result

    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        if not self._check_table_exists_cached(indexName):
            return None

        try:
            res = self.client.get(
                table_name=indexName,
                ids=[chunkId],
            )
            row = res.fetchone()
            if row is None:
                raise Exception(f"ChunkId {chunkId} not found in index {indexName}.")

            return self._row_to_entity(row, fields=list(res.keys()))
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error when getting chunk {chunkId}: {str(e)}")
            return {
                "id": chunkId,
                "error": f"Failed to parse chunk data due to invalid JSON: {str(e)}"
            }
        except Exception as e:
            logger.error(f"Error getting chunk {chunkId}: {str(e)}")
            raise

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str = None) -> list[str]:
        if not documents:
            return []

        docs: list[dict] = []
        ids: list[str] = []
        for document in documents:
            d: dict = {}
            for k, v in document.items():
                if vector_column_pattern.match(k):
                    d[k] = v
                    continue
                if k not in column_names:
                    if "extra" not in d:
                        d["extra"] = {}
                    d["extra"][k] = v
                    continue
                if v is None:
                    d[k] = get_default_value(k)
                    continue

                if k == "kb_id" and isinstance(v, list):
                    d[k] = v[0]
                elif k == "content_with_weight" and isinstance(v, dict):
                    d[k] = json.dumps(v, ensure_ascii=False)
                elif k == "position_int":
                    d[k] = json.dumps([list(vv) for vv in v], ensure_ascii=False)
                elif isinstance(v, list):
                    # remove characters like '\t' for JSON dump and clean special characters
                    cleaned_v = []
                    for vv in v:
                        if isinstance(vv, str):
                            cleaned_str = vv.strip()
                            cleaned_str = cleaned_str.replace('\\', '\\\\')
                            cleaned_str = cleaned_str.replace('\n', '\\n')
                            cleaned_str = cleaned_str.replace('\r', '\\r')
                            cleaned_str = cleaned_str.replace('\t', '\\t')
                            cleaned_v.append(cleaned_str)
                        else:
                            cleaned_v.append(vv)
                    d[k] = json.dumps(cleaned_v, ensure_ascii=False)
                else:
                    d[k] = v

            ids.append(d["id"])
            # this is to fix https://github.com/sqlalchemy/sqlalchemy/issues/9703
            for column_name in column_names:
                if column_name not in d:
                    d[column_name] = get_default_value(column_name)

            metadata = d.get("metadata", {})
            if metadata is None:
                metadata = {}
            group_id = metadata.get("_group_id")
            title = metadata.get("_title")
            if d.get("doc_id"):
                if group_id:
                    d["group_id"] = group_id
                else:
                    d["group_id"] = d["doc_id"]
                if title:
                    d["docnm_kwd"] = title

            docs.append(d)

        logger.debug("OBConnection.insert chunks: %s", docs)

        res = []
        try:
            self.client.upsert(indexName, docs)
        except Exception as e:
            logger.error(f"OBConnection.insert error: {str(e)}")
            res.append(str(e))
        return res

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        if not self._check_table_exists_cached(indexName):
            return True

        condition["kb_id"] = knowledgebaseId
        filters = get_filters(condition)
        set_values: list[str] = []
        for k, v in newValue.items():
            if k == "remove":
                if isinstance(v, str):
                    set_values.append(f"{v} = NULL")
                else:
                    assert isinstance(v, dict), f"Expected str or dict for 'remove', got {type(newValue[k])}."
                    for kk, vv in v.items():
                        assert kk in array_columns, f"Column '{kk}' is not an array column."
                        set_values.append(f"{kk} = array_remove({kk}, {get_value_str(vv)})")
            elif k == "add":
                assert isinstance(v, dict), f"Expected str or dict for 'add', got {type(newValue[k])}."
                for kk, vv in v.items():
                    assert kk in array_columns, f"Column '{kk}' is not an array column."
                    set_values.append(f"{kk} = array_append({kk}, {get_value_str(vv)})")
            elif k == "metadata":
                assert isinstance(v, dict), f"Expected dict for 'metadata', got {type(newValue[k])}"
                set_values.append(f"{k} = {get_value_str(v)}")
                if v and "doc_id" in condition:
                    group_id = v.get("_group_id")
                    title = v.get("_title")
                    if group_id:
                        set_values.append(f"group_id = {get_value_str(group_id)}")
                    if title:
                        set_values.append(f"docnm_kwd = {get_value_str(title)}")
            else:
                set_values.append(f"{k} = {get_value_str(v)}")

        if not set_values:
            return True

        update_sql = (
            f"UPDATE {indexName}"
            f" SET {', '.join(set_values)}"
            f" WHERE {' AND '.join(filters)}"
        )
        logger.debug("OBConnection.update sql: %s", update_sql)

        try:
            self.client.perform_raw_text_sql(update_sql)
            return True
        except Exception as e:
            logger.error(f"OBConnection.update error: {str(e)}")
        return False

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        if not self._check_table_exists_cached(indexName):
            return 0

        condition["kb_id"] = knowledgebaseId
        try:
            res = self.client.get(
                table_name=indexName,
                ids=None,
                where_clause=[text(f) for f in get_filters(condition)],
                output_column_name=["id"],
            )
            rows = res.fetchall()
            if len(rows) == 0:
                return 0
            ids = [row[0] for row in rows]
            logger.debug(f"OBConnection.delete chunks, filters: {condition}, ids: {ids}")
            self.client.delete(
                table_name=indexName,
                ids=ids,
            )
            return len(ids)
        except Exception as e:
            logger.error(f"OBConnection.delete error: {str(e)}")
        return 0

    @staticmethod
    def _row_to_entity(data: Row, fields: list[str]) -> dict:
        entity = {}
        for i, field in enumerate(fields):
            value = data[i]
            if value is None:
                continue
            entity[field] = get_column_value(field, value)
        return entity

    @staticmethod
    def _es_row_to_entity(data: dict) -> dict:
        entity = {}
        for k, v in data.items():
            if v is None:
                continue
            entity[k] = get_column_value(k, v)
        return entity

    """
    Helper functions for search result
    """

    def get_total(self, res) -> int:
        return res.total

    def get_doc_ids(self, res) -> list[str]:
        return [row["id"] for row in res.chunks]

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        result = {}
        for row in res.chunks:
            data = {}
            for field in fields:
                v = row.get(field)
                if v is not None:
                    data[field] = v
            result[row["id"]] = data
        return result

    # copied from query.FulltextQueryer
    def is_chinese(self, line):
        arr = re.split(r"[ \t]+", line)
        if len(arr) <= 3:
            return True
        e = 0
        for t in arr:
            if not re.match(r"[a-zA-Z]+$", t):
                e += 1
        return e * 1.0 / len(arr) >= 0.7

    def highlight(self, txt: str, tks: str, question: str, keywords: list[str]) -> Optional[str]:
        if not txt or not keywords:
            return None

        highlighted_txt = txt

        if question and not self.is_chinese(question):
            highlighted_txt = re.sub(
                r"(^|\W)(%s)(\W|$)" % re.escape(question),
                r"\1<em>\2</em>\3", highlighted_txt,
                flags=re.IGNORECASE | re.MULTILINE,
            )
            if re.search(r"<em>[^<>]+</em>", highlighted_txt, flags=re.IGNORECASE | re.MULTILINE):
                return highlighted_txt

            for keyword in keywords:
                highlighted_txt = re.sub(
                    r"(^|\W)(%s)(\W|$)" % re.escape(keyword),
                    r"\1<em>\2</em>\3", highlighted_txt,
                    flags=re.IGNORECASE | re.MULTILINE,
                )
            if len(re.findall(r'</em><em>', highlighted_txt)) > 0 or len(
                    re.findall(r'</em>\s*<em>', highlighted_txt)) > 0:
                return highlighted_txt
            else:
                return None

        if not tks:
            tks = rag_tokenizer.tokenize(txt)
        tokens = tks.split()
        if not tokens:
            return None

        last_pos = len(txt)

        for i in range(len(tokens) - 1, -1, -1):
            token = tokens[i]
            token_pos = highlighted_txt.rfind(token, 0, last_pos)
            if token_pos != -1:
                if token in keywords:
                    highlighted_txt = (
                            highlighted_txt[:token_pos] +
                            f'<em>{token}</em>' +
                            highlighted_txt[token_pos + len(token):]
                    )
                last_pos = token_pos
        return re.sub(r'</em><em>', '', highlighted_txt)

    def get_highlight(self, res, keywords: list[str], fieldnm: str):
        ans = {}
        if len(res.chunks) == 0 or len(keywords) == 0:
            return ans

        for d in res.chunks:
            txt = d.get(fieldnm)
            if not txt:
                continue

            tks = d.get("content_ltks") if fieldnm == "content_with_weight" else ""
            highlighted_txt = self.highlight(txt, tks, " ".join(keywords), keywords)
            if highlighted_txt:
                ans[d["id"]] = highlighted_txt
        return ans

    def get_aggregation(self, res, fieldnm: str):
        if len(res.chunks) == 0:
            return []

        counts = {}
        result = []
        for d in res.chunks:
            if "value" in d and "count" in d:
                # directly use the aggregation result
                result.append((d["value"], d["count"]))
            elif fieldnm in d:
                # aggregate the values of specific field
                v = d[fieldnm]
                if isinstance(v, list):
                    for vv in v:
                        if isinstance(vv, str) and vv.strip():
                            counts[vv] = counts.get(vv, 0) + 1
                elif isinstance(v, str) and v.strip():
                    counts[v] = counts.get(v, 0) + 1

        if len(counts) > 0:
            for k, v in counts.items():
                result.append((k, v))

        return result

    """
    SQL
    """

    def sql(sql: str, fetch_size: int, format: str):
        # TODO: execute the sql generated by text-to-sql
        return None
