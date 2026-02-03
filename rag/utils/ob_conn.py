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
import re
import time
from typing import Any, Optional

import numpy as np
from elasticsearch_dsl import Q, Search
from pydantic import BaseModel
from pymysql.converters import escape_string
from pyobvector import ARRAY
from sqlalchemy import Column, String, Integer, JSON, Double, Row
from sqlalchemy.dialects.mysql import LONGTEXT, TEXT
from sqlalchemy.sql.type_api import TypeEngine

from common.constants import PAGERANK_FLD, TAG_FLD
from common.decorator import singleton
from common.doc_store.doc_store_base import MatchExpr, OrderByExpr, FusionExpr, MatchTextExpr, MatchDenseExpr
from common.doc_store.ob_conn_base import (
    OBConnectionBase, get_value_str,
    vector_search_template, vector_column_pattern,
    fulltext_index_name_template,
)
from common.float_utils import get_float
from rag.nlp import rag_tokenizer

logger = logging.getLogger('ragflow.ob_conn')

column_order_id = Column("_order_id", Integer, nullable=True, comment="chunk order id for maintaining sequence")
column_group_id = Column("group_id", String(256), nullable=True, comment="group id for external retrieval")
column_mom_id = Column("mom_id", String(256), nullable=True, comment="parent chunk id")
column_chunk_data = Column("chunk_data", JSON, nullable=True, comment="table parser row data")

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
    column_chunk_data,
    Column("metadata", JSON, nullable=True, comment="metadata for this chunk"),
    Column("extra", JSON, nullable=True, comment="extra information of non-general chunk"),
    column_order_id,
    column_group_id,
    column_mom_id,
]

column_names: list[str] = [col.name for col in column_definitions]
column_types: dict[str, TypeEngine] = {col.name: col.type for col in column_definitions}
array_columns: list[str] = [col.name for col in column_definitions if isinstance(col.type, ARRAY)]

# Index columns for RAG chunk table
INDEX_COLUMNS: list[str] = [
    "kb_id",
    "doc_id",
    "available_int",
    "knowledge_graph_kwd",
    "entity_type_kwd",
    "removed_kwd",
]

# Full-text search columns (with weight) - original content
FTS_COLUMNS_ORIGIN: list[str] = [
    "docnm_kwd^10",
    "content_with_weight",
    "important_tks^20",
    "question_tks^20",
]

# Full-text search columns (with weight) - tokenized content
FTS_COLUMNS_TKS: list[str] = [
    "title_tks^10",
    "title_sm_tks^5",
    "important_tks^20",
    "question_tks^20",
    "content_ltks^2",
    "content_sm_ltks",
]

# Extra columns to add after table creation (for migration)
EXTRA_COLUMNS: list[Column] = [column_order_id, column_group_id, column_mom_id]


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
        value_str = get_value_str(value)

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


@singleton
class OBConnection(OBConnectionBase):
    def __init__(self):
        super().__init__(logger_name='ragflow.ob_conn')
        # Determine which columns to use for full-text search dynamically
        self._fulltext_search_columns = FTS_COLUMNS_ORIGIN if self.search_original_content else FTS_COLUMNS_TKS

    """
    Template method implementations
    """

    def get_index_columns(self) -> list[str]:
        return INDEX_COLUMNS

    def get_column_definitions(self) -> list[Column]:
        return column_definitions

    def get_extra_columns(self) -> list[Column]:
        return EXTRA_COLUMNS

    def get_lock_prefix(self) -> str:
        return "ob_"

    def _get_filters(self, condition: dict) -> list[str]:
        return get_filters(condition)

    def get_fulltext_columns(self) -> list[str]:
        """Return list of column names that need fulltext indexes (without weight suffix)."""
        return [col.split("^")[0] for col in self._fulltext_search_columns]

    def delete_idx(self, index_name: str, dataset_id: str):
        if dataset_id:
            # The index need to be alive after any kb deletion since all kb under this tenant are in one index.
            return
        super().delete_idx(index_name, dataset_id)

    """
    Performance monitoring
    """

    def get_performance_metrics(self) -> dict:
        """
        Get comprehensive performance metrics for OceanBase.

        Returns:
            dict: Performance metrics including latency, storage, QPS, and slow queries
        """
        metrics = {
            "connection": "connected",
            "latency_ms": 0.0,
            "storage_used": "0B",
            "storage_total": "0B",
            "query_per_second": 0,
            "slow_queries": 0,
            "active_connections": 0,
            "max_connections": 0
        }

        try:
            # Measure connection latency
            start_time = time.time()
            self.client.perform_raw_text_sql("SELECT 1").fetchone()
            metrics["latency_ms"] = round((time.time() - start_time) * 1000, 2)

            # Get storage information
            try:
                storage_info = self._get_storage_info()
                metrics.update(storage_info)
            except Exception as e:
                logger.warning(f"Failed to get storage info: {str(e)}")

            # Get connection pool statistics
            try:
                pool_stats = self._get_connection_pool_stats()
                metrics.update(pool_stats)
            except Exception as e:
                logger.warning(f"Failed to get connection pool stats: {str(e)}")

            # Get slow query statistics
            try:
                slow_queries = self._get_slow_query_count()
                metrics["slow_queries"] = slow_queries
            except Exception as e:
                logger.warning(f"Failed to get slow query count: {str(e)}")

            # Get QPS (Queries Per Second) - approximate from processlist
            try:
                qps = self._estimate_qps()
                metrics["query_per_second"] = qps
            except Exception as e:
                logger.warning(f"Failed to estimate QPS: {str(e)}")

        except Exception as e:
            metrics["connection"] = "disconnected"
            metrics["error"] = str(e)
            logger.error(f"Failed to get OceanBase performance metrics: {str(e)}")

        return metrics

    def _get_storage_info(self) -> dict:
        """
        Get storage space usage information.

        Returns:
            dict: Storage information with used and total space
        """
        try:
            # Get database size
            result = self.client.perform_raw_text_sql(
                f"SELECT ROUND(SUM(data_length + index_length) / 1024 / 1024, 2) AS 'size_mb' "
                f"FROM information_schema.tables WHERE table_schema = '{self.db_name}'"
            ).fetchone()

            size_mb = float(result[0]) if result and result[0] else 0.0

            # Try to get total available space (may not be available in all OceanBase versions)
            try:
                result = self.client.perform_raw_text_sql(
                    "SELECT ROUND(SUM(total_size) / 1024 / 1024 / 1024, 2) AS 'total_gb' "
                    "FROM oceanbase.__all_disk_stat"
                ).fetchone()
                total_gb = float(result[0]) if result and result[0] else None
            except Exception:
                # Fallback: estimate total space (100GB default if not available)
                total_gb = 100.0

            return {
                "storage_used": f"{size_mb:.2f}MB",
                "storage_total": f"{total_gb:.2f}GB" if total_gb else "N/A"
            }
        except Exception as e:
            logger.warning(f"Failed to get storage info: {str(e)}")
            return {
                "storage_used": "N/A",
                "storage_total": "N/A"
            }

    def _get_connection_pool_stats(self) -> dict:
        """
        Get connection pool statistics.

        Returns:
            dict: Connection pool statistics
        """
        try:
            # Get active connections from processlist
            result = self.client.perform_raw_text_sql("SHOW PROCESSLIST")
            active_connections = len(list(result.fetchall()))

            # Get max_connections setting
            max_conn_result = self.client.perform_raw_text_sql(
                "SHOW VARIABLES LIKE 'max_connections'"
            ).fetchone()
            max_connections = int(max_conn_result[1]) if max_conn_result and max_conn_result[1] else 0

            # Get pool size from client if available
            pool_size = getattr(self.client, 'pool_size', None) or 0

            return {
                "active_connections": active_connections,
                "max_connections": max_connections if max_connections > 0 else pool_size,
                "pool_size": pool_size
            }
        except Exception as e:
            logger.warning(f"Failed to get connection pool stats: {str(e)}")
            return {
                "active_connections": 0,
                "max_connections": 0,
                "pool_size": 0
            }

    def _get_slow_query_count(self, threshold_seconds: int = 1) -> int:
        """
        Get count of slow queries (queries taking longer than threshold).

        Args:
            threshold_seconds: Threshold in seconds for slow queries (default: 1)

        Returns:
            int: Number of slow queries
        """
        try:
            result = self.client.perform_raw_text_sql(
                f"SELECT COUNT(*) FROM information_schema.processlist "
                f"WHERE time > {threshold_seconds} AND command != 'Sleep'"
            ).fetchone()
            return int(result[0]) if result and result[0] else 0
        except Exception as e:
            logger.warning(f"Failed to get slow query count: {str(e)}")
            return 0

    def _estimate_qps(self) -> int:
        """
        Estimate queries per second from processlist.

        Returns:
            int: Estimated queries per second
        """
        try:
            # Count active queries (non-Sleep commands)
            result = self.client.perform_raw_text_sql(
                "SELECT COUNT(*) FROM information_schema.processlist WHERE command != 'Sleep'"
            ).fetchone()
            active_queries = int(result[0]) if result and result[0] else 0

            # Rough estimate: assume average query takes 0.1 seconds
            # This is a simplified estimation
            estimated_qps = max(0, active_queries * 10)

            return estimated_qps
        except Exception as e:
            logger.warning(f"Failed to estimate QPS: {str(e)}")
            return 0

    """
    CRUD operations
    """

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
        agg_fields: list[str] = [],
        rank_feature: dict | None = None,
        **kwargs,
    ):
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        index_names = list(set(index_names))

        if len(match_expressions) == 3:
            if not self.enable_fulltext_search:
                # disable fulltext search in fusion search, which means fallback to vector search
                match_expressions = [m for m in match_expressions if isinstance(m, MatchDenseExpr)]
            else:
                for m in match_expressions:
                    if isinstance(m, FusionExpr):
                        weights = m.fusion_params["weights"]
                        vector_similarity_weight = get_float(weights.split(",")[1])
                        # skip the search if its weight is zero
                        if vector_similarity_weight <= 0.0:
                            match_expressions = [m for m in match_expressions if isinstance(m, MatchTextExpr)]
                        elif vector_similarity_weight >= 1.0:
                            match_expressions = [m for m in match_expressions if isinstance(m, MatchDenseExpr)]

        result: SearchResult = SearchResult(
            total=0,
            chunks=[],
        )

        # copied from es_conn.py
        if len(match_expressions) == 3 and self.es:
            bqry = Q("bool", must=[])
            condition["kb_id"] = knowledgebase_ids
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
            for m in match_expressions:
                if isinstance(m, FusionExpr) and m.method == "weighted_sum" and "weights" in m.fusion_params:
                    assert len(match_expressions) == 3 and isinstance(match_expressions[0], MatchTextExpr) and isinstance(
                        match_expressions[1],
                        MatchDenseExpr) and isinstance(
                        match_expressions[2], FusionExpr)
                    weights = m.fusion_params["weights"]
                    vector_similarity_weight = get_float(weights.split(",")[1])
            for m in match_expressions:
                if isinstance(m, MatchTextExpr):
                    minimum_should_match = m.extra_options.get("minimum_should_match", 0.0)
                    if isinstance(minimum_should_match, float):
                        minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    bqry.must.append(Q("query_string", fields=FTS_COLUMNS_TKS,
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

            if order_by:
                orders = list()
                for field, order in order_by.fields:
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

            for fld in agg_fields:
                s.aggs.bucket(f'aggs_{fld}', 'terms', field=fld, size=1000000)

            if limit > 0:
                s = s[offset:offset + limit]
            q = s.to_dict()
            logger.debug(f"OBConnection.hybrid_search {str(index_names)} query: " + json.dumps(q))

            for index_name in index_names:
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

        output_fields = select_fields.copy()
        if "id" not in output_fields:
            output_fields = ["id"] + output_fields
        if "_score" in output_fields:
            output_fields.remove("_score")

        if highlight_fields:
            for field in highlight_fields:
                if field not in output_fields:
                    output_fields.append(field)

        fields_expr = ", ".join(output_fields)

        condition["kb_id"] = knowledgebase_ids
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

        for m in match_expressions:
            if isinstance(m, MatchTextExpr):
                assert "original_query" in m.extra_options, "'original_query' is missing in extra_options."
                fulltext_query = m.extra_options["original_query"]
                fulltext_query = escape_string(fulltext_query.strip())
                fulltext_topn = m.topn

                fulltext_search_expr, fulltext_search_weight = self._parse_fulltext_columns(
                    fulltext_query, self._fulltext_search_columns
                )
                for column_name in fulltext_search_expr.keys():
                    fulltext_search_idx_list.append(fulltext_index_name_template % column_name)

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
        elif len(agg_fields) > 0:
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

        for index_name in index_names:

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
                rows, elapsed_time = self._execute_search_sql(count_sql)
                total_count = rows[0][0] if rows else 0
                result.total += total_count
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
                rows, elapsed_time = self._execute_search_sql(fusion_sql)
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
                count_sql = self._build_count_sql(index_name, filters_expr, vector_search_filter)
                logger.debug("OBConnection.search with vector count sql: %s", count_sql)
                rows, elapsed_time = self._execute_search_sql(count_sql)
                total_count = rows[0][0] if rows else 0
                result.total += total_count
                logger.info(
                    f"OBConnection.search table {index_name}, search type: vector, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" vector column: '{vector_column_name}',"
                    f" condition: '{condition}',"
                    f" vector_similarity_threshold: {vector_similarity_threshold},"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                vector_sql = self._build_vector_search_sql(
                    index_name, fields_expr, vector_search_score_expr, filters_expr,
                    vector_search_filter, vector_search_expr, limit, vector_topn, offset
                )
                logger.debug("OBConnection.search with vector sql: %s", vector_sql)
                rows, elapsed_time = self._execute_search_sql(vector_sql)
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
                count_sql = self._build_count_sql(index_name, filters_expr, fulltext_search_filter, fulltext_search_hint)
                logger.debug("OBConnection.search with fulltext count sql: %s", count_sql)
                rows, elapsed_time = self._execute_search_sql(count_sql)
                total_count = rows[0][0] if rows else 0
                result.total += total_count
                logger.info(
                    f"OBConnection.search table {index_name}, search type: fulltext, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" query text: '{fulltext_query}',"
                    f" condition: '{condition}',"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                fulltext_sql = self._build_fulltext_search_sql(
                    index_name, fields_expr, fulltext_search_score_expr, filters_expr,
                    fulltext_search_filter, offset, limit, fulltext_topn, fulltext_search_hint
                )
                logger.debug("OBConnection.search with fulltext sql: %s", fulltext_sql)
                rows, elapsed_time = self._execute_search_sql(fulltext_sql)
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
                assert len(agg_fields) == 1, "Only one aggregation field is supported in OceanBase."
                agg_field = agg_fields[0]
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
                if order_by:
                    for field, order in order_by.fields:
                        if isinstance(column_types[field], ARRAY):
                            f = field + "_sort"
                            fields_expr += f", array_to_string({field}, ',') AS {f}"
                            field = f
                        order = "ASC" if order == 0 else "DESC"
                        orders.append(f"{field} {order}")
                count_sql = self._build_count_sql(index_name, filters_expr)
                logger.debug("OBConnection.search with normal count sql: %s", count_sql)
                rows, elapsed_time = self._execute_search_sql(count_sql)
                total_count = rows[0][0] if rows else 0
                result.total += total_count
                logger.info(
                    f"OBConnection.search table {index_name}, search type: normal, step: 1-count, elapsed time: {elapsed_time:.3f} seconds,"
                    f" condition: '{condition}',"
                    f" got count: {total_count}"
                )

                if total_count == 0:
                    continue

                order_by_expr = ("ORDER BY " + ", ".join(orders)) if len(orders) > 0 else ""
                limit_expr = f"LIMIT {offset}, {limit}" if limit != 0 else ""
                filter_sql = self._build_filter_search_sql(
                    index_name, fields_expr, filters_expr, order_by_expr, limit_expr
                )
                logger.debug("OBConnection.search with normal sql: %s", filter_sql)
                rows, elapsed_time = self._execute_search_sql(filter_sql)
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

    def get(self, chunk_id: str, index_name: str, knowledgebase_ids: list[str]) -> dict | None:
        try:
            doc = super().get(chunk_id, index_name, knowledgebase_ids)
            if doc is None:
                return None
            return doc
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error when getting chunk {chunk_id}: {str(e)}")
            return {
                "id": chunk_id,
                "error": f"Failed to parse chunk data due to invalid JSON: {str(e)}"
            }
        except Exception as e:
            logger.exception(f"OBConnection.get({chunk_id}) got exception")
            raise e

    def insert(self, documents: list[dict], index_name: str, knowledgebase_id: str = None) -> list[str]:
        if not documents:
            return []

        # For doc_meta tables, use simple insert without field transformation
        if index_name.startswith("ragflow_doc_meta_"):
            return self._insert_doc_meta(documents, index_name)

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
            self.client.upsert(index_name, docs)
        except Exception as e:
            logger.error(f"OBConnection.insert error: {str(e)}")
            res.append(str(e))
        return res

    def _insert_doc_meta(self, documents: list[dict], index_name: str) -> list[str]:
        """Insert documents into doc_meta table with simple field handling."""
        docs: list[dict] = []
        for document in documents:
            d = {
                "id": document.get("id"),
                "kb_id": document.get("kb_id"),
            }
            # Handle meta_fields - store as JSON
            meta_fields = document.get("meta_fields")
            if meta_fields is not None:
                if isinstance(meta_fields, dict):
                    d["meta_fields"] = json.dumps(meta_fields, ensure_ascii=False)
                elif isinstance(meta_fields, str):
                    d["meta_fields"] = meta_fields
                else:
                    d["meta_fields"] = "{}"
            else:
                d["meta_fields"] = "{}"
            docs.append(d)

        logger.debug("OBConnection._insert_doc_meta: %s", docs)

        res = []
        try:
            self.client.upsert(index_name, docs)
        except Exception as e:
            logger.error(f"OBConnection._insert_doc_meta error: {str(e)}")
            res.append(str(e))
        return res

    def update(self, condition: dict, new_value: dict, index_name: str, knowledgebase_id: str) -> bool:
        if not self._check_table_exists_cached(index_name):
            return True

        # For doc_meta tables, don't force kb_id in condition
        if not index_name.startswith("ragflow_doc_meta_"):
            condition["kb_id"] = knowledgebase_id
        filters = get_filters(condition)
        set_values: list[str] = []
        for k, v in new_value.items():
            if k == "remove":
                if isinstance(v, str):
                    set_values.append(f"{v} = NULL")
                else:
                    assert isinstance(v, dict), f"Expected str or dict for 'remove', got {type(new_value[k])}."
                    for kk, vv in v.items():
                        assert kk in array_columns, f"Column '{kk}' is not an array column."
                        set_values.append(f"{kk} = array_remove({kk}, {get_value_str(vv)})")
            elif k == "add":
                assert isinstance(v, dict), f"Expected str or dict for 'add', got {type(new_value[k])}."
                for kk, vv in v.items():
                    assert kk in array_columns, f"Column '{kk}' is not an array column."
                    set_values.append(f"{kk} = array_append({kk}, {get_value_str(vv)})")
            elif k == "metadata":
                assert isinstance(v, dict), f"Expected dict for 'metadata', got {type(new_value[k])}"
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
            f"UPDATE {index_name}"
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

    def _row_to_entity(self, data: Row, fields: list[str]) -> dict:
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

    def sql(self, sql: str, fetch_size: int = 1024, format: str = "json"):
        logger.debug("OBConnection.sql get sql: %s", sql)

        def normalize_sql(sql_text: str) -> str:
            cleaned = sql_text.strip().rstrip(";")
            cleaned = re.sub(r"[`]+", "", cleaned)
            cleaned = re.sub(
                r"json_extract_string\s*\(\s*([^,]+?)\s*,\s*([^)]+?)\s*\)",
                r"JSON_UNQUOTE(JSON_EXTRACT(\1, \2))",
                cleaned,
                flags=re.IGNORECASE,
            )
            cleaned = re.sub(
                r"json_extract_isnull\s*\(\s*([^,]+?)\s*,\s*([^)]+?)\s*\)",
                r"(JSON_EXTRACT(\1, \2) IS NULL)",
                cleaned,
                flags=re.IGNORECASE,
            )
            return cleaned

        def coerce_value(value: Any) -> Any:
            if isinstance(value, np.generic):
                return value.item()
            if isinstance(value, bytes):
                return value.decode("utf-8", errors="ignore")
            return value

        sql_text = normalize_sql(sql)
        if fetch_size and fetch_size > 0:
            sql_lower = sql_text.lstrip().lower()
            if re.match(r"^(select|with)\b", sql_lower) and not re.search(r"\blimit\b", sql_lower):
                sql_text = f"{sql_text} LIMIT {int(fetch_size)}"

        logger.debug("OBConnection.sql to ob: %s", sql_text)

        try:
            res = self.client.perform_raw_text_sql(sql_text)
        except Exception:
            logger.exception("OBConnection.sql got exception")
            raise

        if res is None:
            return None

        columns = list(res.keys()) if hasattr(res, "keys") else []
        try:
            rows = res.fetchmany(fetch_size) if fetch_size and fetch_size > 0 else res.fetchall()
        except Exception:
            rows = res.fetchall()

        rows_list = [[coerce_value(v) for v in list(row)] for row in rows]
        result = {
            "columns": [{"name": col, "type": "text"} for col in columns],
            "rows": rows_list,
        }

        if format == "markdown":
            header = "|" + "|".join(columns) + "|" if columns else ""
            separator = "|" + "|".join(["---" for _ in columns]) + "|" if columns else ""
            body = "\n".join(["|" + "|".join([str(v) for v in row]) + "|" for row in rows_list])
            result["markdown"] = "\n".join([line for line in [header, separator, body] if line])

        return result
