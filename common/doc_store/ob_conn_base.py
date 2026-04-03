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
from abc import abstractmethod
from typing import Any

from pymysql.converters import escape_string
from pyobvector import ObVecClient, FtsIndexParam, FtsParser, VECTOR
from sqlalchemy import Column, Table

from common.doc_store.doc_store_base import DocStoreConnection, MatchExpr, OrderByExpr

ATTEMPT_TIME = 2

# Common templates for OceanBase
index_name_template = "ix_%s_%s"
fulltext_index_name_template = "fts_idx_%s"
fulltext_search_template = "MATCH (%s) AGAINST ('%s' IN NATURAL LANGUAGE MODE)"
vector_search_template = "cosine_distance(%s, '%s')"
vector_column_pattern = re.compile(r"q_(?P<vector_size>\d+)_vec")


def get_value_str(value: Any) -> str:
    """Convert value to SQL string representation."""
    if isinstance(value, str):
        # escape_string already handles all necessary escaping for MySQL/OceanBase
        # including backslashes, quotes, newlines, etc.
        return f"'{escape_string(value)}'"
    elif isinstance(value, bool):
        return "true" if value else "false"
    elif value is None:
        return "NULL"
    elif isinstance(value, (list, dict)):
        json_str = json.dumps(value, ensure_ascii=False)
        return f"'{escape_string(json_str)}'"
    else:
        return str(value)


def _try_with_lock(lock_name: str, process_func, check_func, timeout: int = None):
    """Execute function with distributed lock."""
    if not timeout:
        timeout = int(os.environ.get("OB_DDL_TIMEOUT", "60"))

    if not check_func():
        from rag.utils.redis_conn import RedisDistributedLock
        lock = RedisDistributedLock(lock_name)
        if lock.acquire():
            try:
                process_func()
                return
            except Exception as e:
                if "Duplicate" in str(e):
                    return
                raise
            finally:
                lock.release()

    if not check_func():
        time.sleep(1)
        count = 1
        while count < timeout and not check_func():
            count += 1
            time.sleep(1)
        if count >= timeout and not check_func():
            raise Exception(f"Timeout to wait for process complete for {lock_name}.")


class OBConnectionBase(DocStoreConnection):
    """Base class for OceanBase document store connections."""

    def __init__(self, logger_name: str = 'ragflow.ob_conn'):
        from common.doc_store.ob_conn_pool import OB_CONN

        self.logger = logging.getLogger(logger_name)
        self.client: ObVecClient = OB_CONN.get_client()
        self.es = OB_CONN.get_hybrid_search_client()
        self.db_name = OB_CONN.get_db_name()
        self.uri = OB_CONN.get_uri()

        self._load_env_vars()

        self._table_exists_cache: set[str] = set()
        self._table_exists_cache_lock = threading.RLock()

        # Cache for vector columns: stores (table_name, vector_size) tuples
        self._vector_column_cache: set[tuple[str, int]] = set()
        self._vector_column_cache_lock = threading.RLock()

        self.logger.info(f"OceanBase {self.uri} connection initialized.")

    def _load_env_vars(self):
        def is_true(var: str, default: str) -> bool:
            return os.getenv(var, default).lower() in ['true', '1', 'yes', 'y']

        self.enable_fulltext_search = is_true('ENABLE_FULLTEXT_SEARCH', 'true')
        self.use_fulltext_hint = is_true('USE_FULLTEXT_HINT', 'true')
        self.search_original_content = is_true("SEARCH_ORIGINAL_CONTENT", 'true')
        self.enable_hybrid_search = is_true('ENABLE_HYBRID_SEARCH', 'false')
        self.use_fulltext_first_fusion_search = is_true('USE_FULLTEXT_FIRST_FUSION_SEARCH', 'true')

        # Adjust settings based on hybrid search availability
        if self.es is not None and self.search_original_content:
            self.logger.info("HybridSearch is enabled, forcing search_original_content to False")
            self.search_original_content = False

    """
    Template methods - must be implemented by subclasses
    """

    @abstractmethod
    def get_index_columns(self) -> list[str]:
        """Return list of column names that need regular indexes."""
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_fulltext_columns(self) -> list[str]:
        """Return list of column names that need fulltext indexes (without weight suffix)."""
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_column_definitions(self) -> list[Column]:
        """Return list of column definitions for table creation."""
        raise NotImplementedError("Not implemented")

    def get_extra_columns(self) -> list[Column]:
        """Return list of extra columns to add after table creation. Override if needed."""
        return []

    def get_table_name(self, index_name: str, dataset_id: str) -> str:
        """Return the actual table name given index_name and dataset_id."""
        return index_name

    @abstractmethod
    def get_lock_prefix(self) -> str:
        """Return the lock name prefix for distributed locking."""
        raise NotImplementedError("Not implemented")

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

    """
    Table operations - common implementation using template methods
    """

    def _check_table_exists_cached(self, table_name: str) -> bool:
        """
        Check table existence with cache to reduce INFORMATION_SCHEMA queries.
        Thread-safe implementation using RLock.
        """
        if table_name in self._table_exists_cache:
            return True

        try:
            if not self.client.check_table_exists(table_name):
                return False

            # Check regular indexes
            for column_name in self.get_index_columns():
                if not self._index_exists(table_name, index_name_template % (table_name, column_name)):
                    return False

            # Check fulltext indexes
            for column_name in self.get_fulltext_columns():
                if not self._index_exists(table_name, fulltext_index_name_template % column_name):
                    return False

            # Check extra columns
            for column in self.get_extra_columns():
                if not self._column_exist(table_name, column.name):
                    return False

        except Exception as e:
            raise Exception(f"OBConnection._check_table_exists_cached error: {str(e)}")

        with self._table_exists_cache_lock:
            if table_name not in self._table_exists_cache:
                self._table_exists_cache.add(table_name)
        return True

    def _create_table(self, table_name: str):
        """Create table using column definitions from subclass."""
        self._create_table_with_columns(table_name, self.get_column_definitions())

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        """Create index/table with all necessary indexes."""
        table_name = self.get_table_name(index_name, dataset_id)
        lock_prefix = self.get_lock_prefix()

        try:
            _try_with_lock(
                lock_name=f"{lock_prefix}create_table_{table_name}",
                check_func=lambda: self.client.check_table_exists(table_name),
                process_func=lambda: self._create_table(table_name),
            )

            for column_name in self.get_index_columns():
                _try_with_lock(
                    lock_name=f"{lock_prefix}add_idx_{table_name}_{column_name}",
                    check_func=lambda cn=column_name: self._index_exists(table_name,
                                                                         index_name_template % (table_name, cn)),
                    process_func=lambda cn=column_name: self._add_index(table_name, cn),
                )

            for column_name in self.get_fulltext_columns():
                _try_with_lock(
                    lock_name=f"{lock_prefix}add_fulltext_idx_{table_name}_{column_name}",
                    check_func=lambda cn=column_name: self._index_exists(table_name, fulltext_index_name_template % cn),
                    process_func=lambda cn=column_name: self._add_fulltext_index(table_name, cn),
                )

            # Add vector column and index (skip metadata refresh, will be done in finally)
            self._ensure_vector_column_exists(table_name, vector_size, refresh_metadata=False)

            # Add extra columns if any
            for column in self.get_extra_columns():
                _try_with_lock(
                    lock_name=f"{lock_prefix}add_{column.name}_{table_name}",
                    check_func=lambda c=column: self._column_exist(table_name, c.name),
                    process_func=lambda c=column: self._add_column(table_name, c),
                )

        except Exception as e:
            raise Exception(f"OBConnection.create_idx error: {str(e)}")
        finally:
            self.client.refresh_metadata([table_name])

    def create_doc_meta_idx(self, index_name: str):
        """
        Create a document metadata table.

        Table name pattern: ragflow_doc_meta_{tenant_id}
        - Per-tenant metadata table for storing document metadata fields
        """
        from sqlalchemy import JSON
        from sqlalchemy.dialects.mysql import VARCHAR

        table_name = index_name
        lock_prefix = self.get_lock_prefix()

        # Define columns for document metadata table
        doc_meta_columns = [
            Column("id", VARCHAR(256), primary_key=True, comment="document id"),
            Column("kb_id", VARCHAR(256), nullable=False, comment="knowledge base id"),
            Column("meta_fields", JSON, nullable=True, comment="document metadata fields"),
        ]

        try:
            # Create table with distributed lock
            _try_with_lock(
                lock_name=f"{lock_prefix}create_doc_meta_table_{table_name}",
                check_func=lambda: self.client.check_table_exists(table_name),
                process_func=lambda: self._create_table_with_columns(table_name, doc_meta_columns),
            )

            # Create index on kb_id for better query performance
            _try_with_lock(
                lock_name=f"{lock_prefix}add_idx_{table_name}_kb_id",
                check_func=lambda: self._index_exists(table_name, index_name_template % (table_name, "kb_id")),
                process_func=lambda: self._add_index(table_name, "kb_id"),
            )

            self.logger.info(f"Created document metadata table '{table_name}'.")
            return True

        except Exception as e:
            self.logger.error(f"OBConnection.create_doc_meta_idx error: {str(e)}")
            return False
        finally:
            self.client.refresh_metadata([table_name])

    def delete_idx(self, index_name: str, dataset_id: str):
        """Delete index/table."""
        # For doc_meta tables, use index_name directly as table name
        if index_name.startswith("ragflow_doc_meta_"):
            table_name = index_name
        else:
            table_name = self.get_table_name(index_name, dataset_id)
        try:
            if self.client.check_table_exists(table_name=table_name):
                self.client.drop_table_if_exist(table_name)
                self.logger.info(f"Dropped table '{table_name}'.")
        except Exception as e:
            raise Exception(f"OBConnection.delete_idx error: {str(e)}")

    def index_exist(self, index_name: str, dataset_id: str = None) -> bool:
        """Check if index/table exists."""
        # For doc_meta tables, use index_name directly as table name
        if index_name.startswith("ragflow_doc_meta_"):
            table_name = index_name
        else:
            table_name = self.get_table_name(index_name, dataset_id) if dataset_id else index_name
        return self._check_table_exists_cached(table_name)

    """
    Table operations - helper methods
    """

    def _get_count(self, table_name: str, filter_list: list[str] = None) -> int:
        where_clause = "WHERE " + " AND ".join(filter_list) if filter_list and len(filter_list) > 0 else ""
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

    def _index_exists(self, table_name: str, idx_name: str) -> bool:
        return self._get_count(
            table_name="INFORMATION_SCHEMA.STATISTICS",
            filter_list=[
                f"TABLE_SCHEMA = '{self.db_name}'",
                f"TABLE_NAME = '{table_name}'",
                f"INDEX_NAME = '{idx_name}'",
            ]) > 0

    def _create_table_with_columns(self, table_name: str, columns: list[Column]):
        """Create table with specified columns."""
        if table_name in self.client.metadata_obj.tables:
            self.client.metadata_obj.remove(Table(table_name, self.client.metadata_obj))

        table_options = {
            "mysql_charset": "utf8mb4",
            "mysql_collate": "utf8mb4_unicode_ci",
            "mysql_organization": "heap",
        }

        self.client.create_table(
            table_name=table_name,
            columns=[c.copy() for c in columns],
            **table_options,
        )
        self.logger.info(f"Created table '{table_name}'.")

    def _add_index(self, table_name: str, column_name: str):
        idx_name = index_name_template % (table_name, column_name)
        self.client.create_index(
            table_name=table_name,
            is_vec_index=False,
            index_name=idx_name,
            column_names=[column_name],
        )
        self.logger.info(f"Created index '{idx_name}' on table '{table_name}'.")

    def _add_fulltext_index(self, table_name: str, column_name: str):
        fulltext_idx_name = fulltext_index_name_template % column_name
        self.client.create_fts_idx_with_fts_index_param(
            table_name=table_name,
            fts_idx_param=FtsIndexParam(
                index_name=fulltext_idx_name,
                field_names=[column_name],
                parser_type=FtsParser.IK,
            ),
        )
        self.logger.info(f"Created full text index '{fulltext_idx_name}' on table '{table_name}'.")

    def _add_vector_column(self, table_name: str, vector_size: int):
        vector_field_name = f"q_{vector_size}_vec"
        self.client.add_columns(
            table_name=table_name,
            columns=[Column(vector_field_name, VECTOR(vector_size), nullable=True)],
        )
        self.logger.info(f"Added vector column '{vector_field_name}' to table '{table_name}'.")

    def _add_vector_index(self, table_name: str, vector_field_name: str):
        vector_idx_name = f"{vector_field_name}_idx"
        self.client.create_index(
            table_name=table_name,
            is_vec_index=True,
            index_name=vector_idx_name,
            column_names=[vector_field_name],
            vidx_params="distance=cosine, type=hnsw, lib=vsag",
        )
        self.logger.info(
            f"Created vector index '{vector_idx_name}' on table '{table_name}' with column '{vector_field_name}'."
        )

    def _add_column(self, table_name: str, column: Column):
        try:
            self.client.add_columns(
                table_name=table_name,
                columns=[column.copy()],
            )
            self.logger.info(f"Added column '{column.name}' to table '{table_name}'.")
        except Exception as e:
            self.logger.warning(f"Failed to add column '{column.name}' to table '{table_name}': {str(e)}")

    def _ensure_vector_column_exists(self, table_name: str, vector_size: int, refresh_metadata: bool = True):
        """
        Ensure vector column and index exist for the given vector size.
        This method is safe to call multiple times - it will skip if already exists.
        Uses cache to avoid repeated INFORMATION_SCHEMA queries.

        Args:
            table_name: Name of the table
            vector_size: Size of the vector column
            refresh_metadata: Whether to refresh SQLAlchemy metadata after changes (default True)
        """
        if vector_size <= 0:
            return

        cache_key = (table_name, vector_size)

        # Check cache first
        if cache_key in self._vector_column_cache:
            return

        lock_prefix = self.get_lock_prefix()
        vector_field_name = f"q_{vector_size}_vec"
        vector_index_name = f"{vector_field_name}_idx"

        # Check if already exists (may have been created by another process)
        column_exists = self._column_exist(table_name, vector_field_name)
        index_exists = self._index_exists(table_name, vector_index_name)

        if column_exists and index_exists:
            # Already exists, add to cache and return
            with self._vector_column_cache_lock:
                self._vector_column_cache.add(cache_key)
            return

        # Create column if needed
        if not column_exists:
            _try_with_lock(
                lock_name=f"{lock_prefix}add_vector_column_{table_name}_{vector_field_name}",
                check_func=lambda: self._column_exist(table_name, vector_field_name),
                process_func=lambda: self._add_vector_column(table_name, vector_size),
            )

        # Create index if needed
        if not index_exists:
            _try_with_lock(
                lock_name=f"{lock_prefix}add_vector_idx_{table_name}_{vector_field_name}",
                check_func=lambda: self._index_exists(table_name, vector_index_name),
                process_func=lambda: self._add_vector_index(table_name, vector_field_name),
            )

        if refresh_metadata:
            self.client.refresh_metadata([table_name])

        # Add to cache after successful creation
        with self._vector_column_cache_lock:
            self._vector_column_cache.add(cache_key)

    def _execute_search_sql(self, sql: str) -> tuple[list, float]:
        start_time = time.time()
        res = self.client.perform_raw_text_sql(sql)
        rows = res.fetchall()
        elapsed_time = time.time() - start_time
        return rows, elapsed_time

    def _parse_fulltext_columns(
        self,
        fulltext_query: str,
        fulltext_columns: list[str]
    ) -> tuple[dict[str, str], dict[str, float]]:
        """
        Parse fulltext search columns with optional weight suffix and build search expressions.

        Args:
            fulltext_query: The escaped fulltext query string
            fulltext_columns: List of column names, optionally with weight suffix (e.g., "col^0.5")

        Returns:
            Tuple of (fulltext_search_expr dict, fulltext_search_weight dict)
            where weights are normalized to 0~1
        """
        fulltext_search_expr: dict[str, str] = {}
        fulltext_search_weight: dict[str, float] = {}

        # get fulltext match expression and weight values
        for field in fulltext_columns:
            parts = field.split("^")
            column_name: str = parts[0]
            column_weight: float = float(parts[1]) if (len(parts) > 1 and parts[1]) else 1.0

            fulltext_search_weight[column_name] = column_weight
            fulltext_search_expr[column_name] = fulltext_search_template % (column_name, fulltext_query)

        # adjust the weight to 0~1
        weight_sum = sum(fulltext_search_weight.values())
        n = len(fulltext_search_weight)
        if weight_sum <= 0 < n:
            # All weights are 0 (e.g. "col^0"); use equal weights to avoid ZeroDivisionError
            for column_name in fulltext_search_weight:
                fulltext_search_weight[column_name] = 1.0 / n
        else:
            for column_name in fulltext_search_weight:
                fulltext_search_weight[column_name] = fulltext_search_weight[column_name] / weight_sum

        return fulltext_search_expr, fulltext_search_weight

    def _build_vector_search_sql(
        self,
        table_name: str,
        fields_expr: str,
        vector_search_score_expr: str,
        filters_expr: str,
        vector_search_filter: str,
        vector_search_expr: str,
        limit: int,
        vector_topn: int,
        offset: int = 0
    ) -> str:
        sql = (
            f"SELECT {fields_expr}, {vector_search_score_expr} AS _score"
            f"  FROM {table_name}"
            f"  WHERE {filters_expr} AND {vector_search_filter}"
            f"  ORDER BY {vector_search_expr}"
            f"  APPROXIMATE LIMIT {limit if limit != 0 else vector_topn}"
        )
        if offset != 0:
            sql += f" OFFSET {offset}"
        return sql

    def _build_fulltext_search_sql(
        self,
        table_name: str,
        fields_expr: str,
        fulltext_search_score_expr: str,
        filters_expr: str,
        fulltext_search_filter: str,
        offset: int,
        limit: int,
        fulltext_topn: int,
        hint: str = ""
    ) -> str:
        hint_expr = f"{hint} " if hint else ""
        return (
            f"SELECT {hint_expr}{fields_expr}, {fulltext_search_score_expr} AS _score"
            f"  FROM {table_name}"
            f"  WHERE {filters_expr} AND {fulltext_search_filter}"
            f"  ORDER BY _score DESC"
            f"  LIMIT {offset}, {limit if limit != 0 else fulltext_topn}"
        )

    def _build_filter_search_sql(
        self,
        table_name: str,
        fields_expr: str,
        filters_expr: str,
        order_by_expr: str = "",
        limit_expr: str = ""
    ) -> str:
        return (
            f"SELECT {fields_expr}"
            f"  FROM {table_name}"
            f"  WHERE {filters_expr}"
            f"  {order_by_expr} {limit_expr}"
        )

    def _build_count_sql(
        self,
        table_name: str,
        filters_expr: str,
        extra_filter: str = "",
        hint: str = ""
    ) -> str:
        hint_expr = f"{hint} " if hint else ""
        where_clause = f"{filters_expr} AND {extra_filter}" if extra_filter else filters_expr
        return f"SELECT {hint_expr}COUNT(id) FROM {table_name} WHERE {where_clause}"

    def _row_to_entity(self, data, fields: list[str]) -> dict:
        entity = {}
        for i, field in enumerate(fields):
            value = data[i]
            if value is None:
                continue
            entity[field] = value
        return entity

    def _get_dataset_id_field(self) -> str:
        return "kb_id"

    def _get_filters(self, condition: dict) -> list[str]:
        filters: list[str] = []
        for k, v in condition.items():
            if not v:
                continue
            if k == "exists":
                filters.append(f"{v} IS NOT NULL")
            elif k == "must_not" and isinstance(v, dict) and "exists" in v:
                filters.append(f"{v.get('exists')} IS NULL")
            elif isinstance(v, list):
                values: list[str] = []
                for item in v:
                    values.append(get_value_str(item))
                value = ", ".join(values)
                filters.append(f"{k} IN ({value})")
            else:
                filters.append(f"{k} = {get_value_str(v)}")
        return filters

    def get(self, doc_id: str, index_name: str, dataset_ids: list[str]) -> dict | None:
        if not self._check_table_exists_cached(index_name):
            return None
        try:
            res = self.client.get(
                table_name=index_name,
                ids=[doc_id],
            )
            row = res.fetchone()
            if row is None:
                return None
            return self._row_to_entity(row, fields=list(res.keys()))
        except Exception as e:
            self.logger.exception(f"OBConnectionBase.get({doc_id}) got exception")
            raise e

    def delete(self, condition: dict, index_name: str, dataset_id: str) -> int:
        if not self._check_table_exists_cached(index_name):
            return 0
        # For doc_meta tables, don't add dataset_id to condition
        if not index_name.startswith("ragflow_doc_meta_"):
            condition[self._get_dataset_id_field()] = dataset_id
        try:
            from sqlalchemy import text
            res = self.client.get(
                table_name=index_name,
                ids=None,
                where_clause=[text(f) for f in self._get_filters(condition)],
                output_column_name=["id"],
            )
            rows = res.fetchall()
            if len(rows) == 0:
                return 0
            ids = [row[0] for row in rows]
            self.logger.debug(f"OBConnection.delete, filters: {condition}, ids: {ids}")
            self.client.delete(
                table_name=index_name,
                ids=ids,
            )
            return len(ids)
        except Exception as e:
            self.logger.error(f"OBConnection.delete error: {str(e)}")
        return 0

    """
    Abstract CRUD methods that must be implemented by subclasses
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
        knowledgebase_ids: list[str],
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
        **kwargs,
    ):
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def insert(self, documents: list[dict], index_name: str, dataset_id: str = None) -> list[str]:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def update(self, condition: dict, new_value: dict, index_name: str, dataset_id: str) -> bool:
        raise NotImplementedError("Not implemented")

    """
    Helper functions for search result - abstract methods
    """

    @abstractmethod
    def get_total(self, res) -> int:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_doc_ids(self, res) -> list[str]:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_highlight(self, res, keywords: list[str], field_name: str):
        raise NotImplementedError("Not implemented")

    @abstractmethod
    def get_aggregation(self, res, field_name: str):
        raise NotImplementedError("Not implemented")

    """
    SQL - can be overridden by subclasses
    """

    def sql(self, sql: str, fetch_size: int, format: str):
        """Execute SQL query - default implementation."""
        return None
