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

import re
from typing import Optional

import numpy as np
from pydantic import BaseModel
from pymysql.converters import escape_string
from sqlalchemy import Column, String, Integer
from sqlalchemy.dialects.mysql import LONGTEXT

from common.decorator import singleton
from common.doc_store.doc_store_base import MatchExpr, OrderByExpr, FusionExpr, MatchTextExpr, MatchDenseExpr
from common.doc_store.ob_conn_base import OBConnectionBase, get_value_str, vector_search_template
from common.float_utils import get_float
from rag.nlp.rag_tokenizer import tokenize, fine_grained_tokenize

# Column definitions for memory message table
COLUMN_DEFINITIONS: list[Column] = [
    Column("id", String(256), primary_key=True, comment="unique record id"),
    Column("message_id", String(256), nullable=False, index=True, comment="message id"),
    Column("message_type_kwd", String(64), nullable=True, comment="message type"),
    Column("source_id", String(256), nullable=True, comment="source message id"),
    Column("memory_id", String(256), nullable=False, index=True, comment="memory id"),
    Column("user_id", String(256), nullable=True, comment="user id"),
    Column("agent_id", String(256), nullable=True, comment="agent id"),
    Column("session_id", String(256), nullable=True, comment="session id"),
    Column("zone_id", Integer, nullable=True, server_default="0", comment="zone id"),
    Column("valid_at", String(64), nullable=True, comment="valid at timestamp string"),
    Column("invalid_at", String(64), nullable=True, comment="invalid at timestamp string"),
    Column("forget_at", String(64), nullable=True, comment="forget at timestamp string"),
    Column("status_int", Integer, nullable=False, server_default="1", comment="status: 1 for active, 0 for inactive"),
    Column("content_ltks", LONGTEXT, nullable=True, comment="content with tokenization"),
    Column("tokenized_content_ltks", LONGTEXT, nullable=True, comment="fine-grained tokenized content"),
]

COLUMN_NAMES: list[str] = [col.name for col in COLUMN_DEFINITIONS]

# Index columns for creating indexes
INDEX_COLUMNS: list[str] = [
    "message_id",
    "memory_id",
    "status_int",
]

# Full-text search columns
FTS_COLUMNS: list[str] = [
    "content_ltks",
    "tokenized_content_ltks",
]


class SearchResult(BaseModel):
    total: int
    messages: list[dict]


@singleton
class OBConnection(OBConnectionBase):
    def __init__(self):
        super().__init__(logger_name='ragflow.memory_ob_conn')
        self._fulltext_search_columns = FTS_COLUMNS

    """
    Template method implementations
    """

    def get_index_columns(self) -> list[str]:
        return INDEX_COLUMNS

    def get_fulltext_columns(self) -> list[str]:
        """Return list of column names that need fulltext indexes (without weight suffix)."""
        return [col.split("^")[0] for col in self._fulltext_search_columns]

    def get_column_definitions(self) -> list[Column]:
        return COLUMN_DEFINITIONS

    def get_lock_prefix(self) -> str:
        return "ob_memory_"

    def _get_dataset_id_field(self) -> str:
        return "memory_id"

    def _get_vector_column_name_from_table(self, table_name: str) -> Optional[str]:
        """Get the vector column name from the table (q_{size}_vec pattern)."""
        sql = f"""
            SELECT COLUMN_NAME 
            FROM INFORMATION_SCHEMA.COLUMNS 
            WHERE TABLE_SCHEMA = '{self.db_name}' 
              AND TABLE_NAME = '{table_name}' 
              AND COLUMN_NAME REGEXP '^q_[0-9]+_vec$'
            LIMIT 1
        """
        try:
            res = self.client.perform_raw_text_sql(sql)
            row = res.fetchone()
            return row[0] if row else None
        except Exception:
            return None

    """
    Field conversion methods
    """

    @staticmethod
    def convert_field_name(field_name: str, use_tokenized_content=False) -> str:
        """Convert message field name to database column name."""
        match field_name:
            case "message_type":
                return "message_type_kwd"
            case "status":
                return "status_int"
            case "content":
                if use_tokenized_content:
                    return "tokenized_content_ltks"
                return "content_ltks"
            case _:
                return field_name

    @staticmethod
    def map_message_to_ob_fields(message: dict) -> dict:
        """Map message dictionary fields to OceanBase document fields."""
        storage_doc = {
            "id": message.get("id"),
            "message_id": message["message_id"],
            "message_type_kwd": message["message_type"],
            "source_id": message.get("source_id"),
            "memory_id": message["memory_id"],
            "user_id": message.get("user_id", ""),
            "agent_id": message["agent_id"],
            "session_id": message["session_id"],
            "valid_at": message["valid_at"],
            "invalid_at": message.get("invalid_at"),
            "forget_at": message.get("forget_at"),
            "status_int": 1 if message["status"] else 0,
            "zone_id": message.get("zone_id", 0),
            "content_ltks": message["content"],
            "tokenized_content_ltks": fine_grained_tokenize(tokenize(message["content"])),
        }
        # Handle vector embedding
        content_embed = message.get("content_embed", [])
        if len(content_embed) > 0:
            storage_doc[f"q_{len(content_embed)}_vec"] = content_embed
        return storage_doc

    @staticmethod
    def get_message_from_ob_doc(doc: dict) -> dict:
        """Convert an OceanBase document back to a message dictionary."""
        embd_field_name = next((key for key in doc.keys() if re.match(r"q_\d+_vec", key)), None)
        content_embed = doc.get(embd_field_name, []) if embd_field_name else []
        if isinstance(content_embed, np.ndarray):
            content_embed = content_embed.tolist()
        message = {
            "message_id": doc.get("message_id"),
            "message_type": doc.get("message_type_kwd"),
            "source_id": doc.get("source_id") if doc.get("source_id") else None,
            "memory_id": doc.get("memory_id"),
            "user_id": doc.get("user_id", ""),
            "agent_id": doc.get("agent_id"),
            "session_id": doc.get("session_id"),
            "zone_id": doc.get("zone_id", 0),
            "valid_at": doc.get("valid_at"),
            "invalid_at": doc.get("invalid_at", "-"),
            "forget_at": doc.get("forget_at", "-"),
            "status": bool(int(doc.get("status_int", 0))),
            "content": doc.get("content_ltks", ""),
            "content_embed": content_embed,
        }
        if doc.get("id"):
            message["id"] = doc["id"]
        return message

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
        memory_ids: list[str],
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
        hide_forgotten: bool = True
    ):
        """Search messages in memory storage."""
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0

        result: SearchResult = SearchResult(total=0, messages=[])

        output_fields = select_fields.copy()
        if "id" not in output_fields:
            output_fields = ["id"] + output_fields
        if "_score" in output_fields:
            output_fields.remove("_score")

        # Handle content_embed field - resolve to actual vector column name
        has_content_embed = "content_embed" in output_fields
        actual_vector_column: Optional[str] = None
        if has_content_embed:
            output_fields = [f for f in output_fields if f != "content_embed"]
            # Try to get vector column name from first available table
            for idx_name in index_names:
                if self._check_table_exists_cached(idx_name):
                    actual_vector_column = self._get_vector_column_name_from_table(idx_name)
                    if actual_vector_column:
                        output_fields.append(actual_vector_column)
                        break

        if highlight_fields:
            for field in highlight_fields:
                field_name = self.convert_field_name(field)
                if field_name not in output_fields:
                    output_fields.append(field_name)

        db_output_fields = [self.convert_field_name(f) for f in output_fields]
        fields_expr = ", ".join(db_output_fields)

        condition["memory_id"] = memory_ids
        if hide_forgotten:
            condition["must_not"] = {"exists": "forget_at"}

        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        filters: list[str] = self._get_filters(condition_dict)
        filters_expr = " AND ".join(filters) if filters else "1=1"

        # Parse match expressions
        fulltext_query: Optional[str] = None
        fulltext_topn: Optional[int] = None
        fulltext_search_expr: dict[str, str] = {}
        fulltext_search_weight: dict[str, float] = {}
        fulltext_search_filter: Optional[str] = None
        fulltext_search_score_expr: Optional[str] = None

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
            elif isinstance(m, MatchDenseExpr):
                vector_column_name = m.vector_column_name
                vector_data = m.embedding_data
                vector_topn = m.topn
                vector_similarity_threshold = m.extra_options.get("similarity", 0.0) if m.extra_options else 0.0
            elif isinstance(m, FusionExpr):
                weights = m.fusion_params.get("weights", "0.5,0.5") if m.fusion_params else "0.5,0.5"
                vector_similarity_weight = get_float(weights.split(",")[1])

        if fulltext_query:
            fulltext_search_filter = f"({' OR '.join([expr for expr in fulltext_search_expr.values()])})"
            fulltext_search_score_expr = f"({' + '.join(f'{expr} * {fulltext_search_weight.get(col, 0)}' for col, expr in fulltext_search_expr.items())})"

        if vector_data:
            vector_data_str = "[" + ",".join([str(np.float32(v)) for v in vector_data]) + "]"
            vector_search_expr = vector_search_template % (vector_column_name, vector_data_str)
            vector_search_score_expr = f"(1 - {vector_search_expr})"
            vector_search_filter = f"{vector_search_score_expr} >= {vector_similarity_threshold}"

        # Determine search type
        if fulltext_query and vector_data:
            search_type = "fusion"
        elif fulltext_query:
            search_type = "fulltext"
        elif vector_data:
            search_type = "vector"
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
            table_name = index_name

            if not self._check_table_exists_cached(table_name):
                continue

            if search_type == "fusion":
                num_candidates = (vector_topn or limit) + (fulltext_topn or limit)
                score_expr = f"(relevance * {1 - vector_similarity_weight} + {vector_search_score_expr} * {vector_similarity_weight})"
                fusion_sql = (
                    f"WITH fulltext_results AS ("
                    f"  SELECT *, {fulltext_search_score_expr} AS relevance"
                    f"      FROM {table_name}"
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
                self.logger.debug("OBConnection.search with fusion sql: %s", fusion_sql)
                rows, elapsed_time = self._execute_search_sql(fusion_sql)
                self.logger.info(
                    f"OBConnection.search table {table_name}, search type: fusion, elapsed time: {elapsed_time:.3f}s, rows: {len(rows)}"
                )

                for row in rows:
                    result.messages.append(self._row_to_entity(row, db_output_fields + ["_score"]))
                    result.total += 1

            elif search_type == "vector":
                vector_sql = self._build_vector_search_sql(
                    table_name, fields_expr, vector_search_score_expr, filters_expr,
                    vector_search_filter, vector_search_expr, limit, vector_topn, offset
                )
                self.logger.debug("OBConnection.search with vector sql: %s", vector_sql)
                rows, elapsed_time = self._execute_search_sql(vector_sql)
                self.logger.info(
                    f"OBConnection.search table {table_name}, search type: vector, elapsed time: {elapsed_time:.3f}s, rows: {len(rows)}"
                )

                for row in rows:
                    result.messages.append(self._row_to_entity(row, db_output_fields + ["_score"]))
                    result.total += 1

            elif search_type == "fulltext":
                fulltext_sql = self._build_fulltext_search_sql(
                    table_name, fields_expr, fulltext_search_score_expr, filters_expr,
                    fulltext_search_filter, offset, limit, fulltext_topn
                )
                self.logger.debug("OBConnection.search with fulltext sql: %s", fulltext_sql)
                rows, elapsed_time = self._execute_search_sql(fulltext_sql)
                self.logger.info(
                    f"OBConnection.search table {table_name}, search type: fulltext, elapsed time: {elapsed_time:.3f}s, rows: {len(rows)}"
                )

                for row in rows:
                    result.messages.append(self._row_to_entity(row, db_output_fields + ["_score"]))
                    result.total += 1

            else:
                orders: list[str] = []
                if order_by and order_by.fields:
                    for field, order_dir in order_by.fields:
                        field_name = self.convert_field_name(field)
                        order_str = "ASC" if order_dir == 0 else "DESC"
                        orders.append(f"{field_name} {order_str}")

                order_by_expr = ("ORDER BY " + ", ".join(orders)) if orders else ""
                limit_expr = f"LIMIT {offset}, {limit}" if limit != 0 else ""
                filter_sql = self._build_filter_search_sql(
                    table_name, fields_expr, filters_expr, order_by_expr, limit_expr
                )
                self.logger.debug("OBConnection.search with filter sql: %s", filter_sql)
                rows, elapsed_time = self._execute_search_sql(filter_sql)
                self.logger.info(
                    f"OBConnection.search table {table_name}, search type: filter, elapsed time: {elapsed_time:.3f}s, rows: {len(rows)}"
                )

                for row in rows:
                    result.messages.append(self._row_to_entity(row, db_output_fields))
                    result.total += 1

        if result.total == 0:
            result.total = len(result.messages)

        return result, result.total

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        """Get forgotten messages (messages with forget_at set)."""
        if not self._check_table_exists_cached(index_name):
            return None

        db_output_fields = [self.convert_field_name(f) for f in select_fields]
        fields_expr = ", ".join(db_output_fields)

        sql = (
            f"SELECT {fields_expr}"
            f"  FROM {index_name}"
            f"  WHERE memory_id = {get_value_str(memory_id)} AND forget_at IS NOT NULL"
            f"  ORDER BY forget_at ASC"
            f"  LIMIT {limit}"
        )
        self.logger.debug("OBConnection.get_forgotten_messages sql: %s", sql)

        res = self.client.perform_raw_text_sql(sql)
        rows = res.fetchall()

        result = SearchResult(total=len(rows), messages=[])
        for row in rows:
            result.messages.append(self._row_to_entity(row, db_output_fields))

        return result

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str,
                                  limit: int = 512):
        """Get messages missing a specific field."""
        if not self._check_table_exists_cached(index_name):
            return None

        db_field_name = self.convert_field_name(field_name)
        db_output_fields = [self.convert_field_name(f) for f in select_fields]
        fields_expr = ", ".join(db_output_fields)

        sql = (
            f"SELECT {fields_expr}"
            f"  FROM {index_name}"
            f"  WHERE memory_id = {get_value_str(memory_id)} AND {db_field_name} IS NULL"
            f"  ORDER BY valid_at ASC"
            f"  LIMIT {limit}"
        )
        self.logger.debug("OBConnection.get_missing_field_message sql: %s", sql)

        res = self.client.perform_raw_text_sql(sql)
        rows = res.fetchall()

        result = SearchResult(total=len(rows), messages=[])
        for row in rows:
            result.messages.append(self._row_to_entity(row, db_output_fields))

        return result

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        """Get single message by id."""
        doc = super().get(doc_id, index_name, memory_ids)
        if doc is None:
            return None
        return self.get_message_from_ob_doc(doc)

    def insert(self, documents: list[dict], index_name: str, memory_id: str = None) -> list[str]:
        """Insert messages into memory storage."""
        if not documents:
            return []

        vector_size = len(documents[0].get("content_embed", [])) if "content_embed" in documents[0] else 0

        if not self._check_table_exists_cached(index_name):
            if vector_size == 0:
                raise ValueError("Cannot infer vector size from documents")
            self.create_idx(index_name, memory_id, vector_size)
        elif vector_size > 0:
            # Table exists but may not have the required vector column
            self._ensure_vector_column_exists(index_name, vector_size)

        docs: list[dict] = []
        ids: list[str] = []

        for document in documents:
            d = self.map_message_to_ob_fields(document)
            ids.append(d["id"])

            for column_name in COLUMN_NAMES:
                if column_name not in d:
                    d[column_name] = None

            docs.append(d)

        self.logger.debug("OBConnection.insert messages: %s", ids)

        res = []
        try:
            self.client.upsert(index_name, docs)
        except Exception as e:
            self.logger.error(f"OBConnection.insert error: {str(e)}")
            res.append(str(e))
        return res

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        """Update messages with given condition."""
        if not self._check_table_exists_cached(index_name):
            return True

        condition["memory_id"] = memory_id
        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        filters = self._get_filters(condition_dict)

        update_dict = {self.convert_field_name(k): v for k, v in new_value.items()}
        if "content_ltks" in update_dict:
            update_dict["tokenized_content_ltks"] = fine_grained_tokenize(tokenize(update_dict["content_ltks"]))
        update_dict.pop("id", None)

        set_values: list[str] = []
        for k, v in update_dict.items():
            if k == "remove":
                if isinstance(v, str):
                    set_values.append(f"{v} = NULL")
            elif k == "status":
                set_values.append(f"status_int = {1 if v else 0}")
            else:
                set_values.append(f"{k} = {get_value_str(v)}")

        if not set_values:
            return True

        update_sql = (
            f"UPDATE {index_name}"
            f" SET {', '.join(set_values)}"
            f" WHERE {' AND '.join(filters)}"
        )
        self.logger.debug("OBConnection.update sql: %s", update_sql)

        try:
            self.client.perform_raw_text_sql(update_sql)
            return True
        except Exception as e:
            self.logger.error(f"OBConnection.update error: {str(e)}")
        return False

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        """Delete messages with given condition."""
        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        return super().delete(condition_dict, index_name, memory_id)

    """
    Helper functions for search result
    """

    def get_total(self, res) -> int:
        if isinstance(res, tuple):
            return res[1]
        if hasattr(res, 'total'):
            return res.total
        return 0

    def get_doc_ids(self, res) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        if hasattr(res, 'messages'):
            return [row.get("id") for row in res.messages if row.get("id")]
        return []

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        """Get fields from search result."""
        if isinstance(res, tuple):
            res = res[0]

        res_fields = {}
        if not fields:
            return {}

        messages = res.messages if hasattr(res, 'messages') else []

        for doc in messages:
            message = self.get_message_from_ob_doc(doc)
            m = {}
            for n, v in message.items():
                if n not in fields:
                    continue
                if isinstance(v, list):
                    m[n] = v
                    continue
                if n in ["message_id", "source_id", "valid_at", "invalid_at", "forget_at", "status"] and isinstance(v,
                                                                                                                    (int,
                                                                                                                     float,
                                                                                                                     bool)):
                    m[n] = v
                    continue
                if not isinstance(v, str):
                    m[n] = str(v) if v is not None else ""
                else:
                    m[n] = v

            doc_id = doc.get("id") or message.get("id")
            if m and doc_id:
                res_fields[doc_id] = m

        return res_fields

    def get_highlight(self, res, keywords: list[str], field_name: str):
        """Get highlighted text for search results."""
        # TODO: Implement highlight functionality for OceanBase memory
        return {}

    def get_aggregation(self, res, field_name: str):
        """Get aggregation for search results."""
        # TODO: Implement aggregation functionality for OceanBase memory
        return []
