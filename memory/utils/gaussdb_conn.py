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
"""GaussDB A/ORA-compatible Memory Store adapter.

This module stores Memory messages only; it does not manage memory metadata in
the metadata database. The service constructs a logical index_name per tenant,
which this adapter maps to a physical GaussDB table. Rows within the shared
tenant table are isolated by memory_id.

The implementation reuses the connection pool, identifier validation, and
selected utilities from common.doc_store.gaussdb_conn_base. Its table schema,
field mapping, full-text/vector/hybrid queries, capacity accounting, and FIFO
operations remain independent so DocEngine chunk semantics never leak into
Memory messages.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
from datetime import datetime
from decimal import Decimal, InvalidOperation
from typing import Any, Iterable

import numpy as np
from pydantic import BaseModel

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchExpr, MatchTextExpr, OrderByExpr
from common.doc_store.gaussdb_conn_base import GaussDBConnectionBase, GaussDBDDLBuilder, InvalidGaussDBObjectName
from common.doc_store.gaussdb_conn_pool import GaussDBConnectionPool, classify_gaussdb_exception
from common.float_utils import get_float
from memory.utils.aggregation_utils import aggregate_by_field
from memory.utils.highlight_utils import get_highlight_from_messages
from rag.nlp import is_english
from rag.nlp.rag_tokenizer import fine_grained_tokenize, tokenize

logger = logging.getLogger("ragflow.memory_gaussdb_conn")

VECTOR_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec$")
VECTOR_EMPTY_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec_empty$")

# Base columns for Memory messages. Vector columns are added dynamically for
# each embedding dimension and are not listed here. These names also form the
# SQL allowlist: every external field must resolve to a base column or a
# q_{dim}_vec / q_{dim}_vec_empty dynamic column.
BASE_COLUMNS = (
    "id",
    "message_id",
    "message_type_kwd",
    "source_id",
    "memory_id",
    "user_id",
    "agent_id",
    "session_id",
    "zone_id",
    "valid_at",
    "invalid_at",
    "forget_at",
    "status_int",
    "content_ltks",
    "tokenized_content_ltks",
)
BASE_COLUMN_SET = set(BASE_COLUMNS)

TIME_COLUMNS = {"valid_at", "invalid_at", "forget_at"}
NUMERIC_COLUMNS = {"message_id", "source_id", "zone_id", "status_int"}
# MemoryService uses backend-neutral field names. GaussDB physical tables use
# suffixed names to avoid conflicts with full-text, vector, and status columns.
# Map every field before constructing SQL.
MEMORY_FIELD_MAP = {
    "message_type": "message_type_kwd",
    "status": "status_int",
    "content": "content_ltks",
}
REVERSE_MEMORY_FIELD_MAP = {
    "message_type_kwd": "message_type",
    "status_int": "status",
    "content_ltks": "content",
}
RESULT_FIELD_DEFAULTS = {
    "source_id": None,
    "user_id": "",
    "zone_id": 0,
    "invalid_at": "-",
    "forget_at": "-",
    "content": "",
    "content_embed": [],
}


def normalize_fulltext_query(text: Any) -> str:
    # Writes store fine_grained_tokenize(tokenize(content)). Apply the same
    # normalization before plainto_tsquery so short terms, English stems, and
    # mixed-language content do not produce false negatives from mismatched
    # write-time and query-time tokens.
    query = str(text or "").strip()
    if not query:
        return ""
    tokenized = fine_grained_tokenize(tokenize(query)).strip()
    return tokenized or query


class SearchResult(BaseModel):
    # Match other Memory Store adapters: search() returns (SearchResult, total),
    # while get_total/get_doc_ids/get_fields accept either that tuple or a bare
    # SearchResult.
    total: int
    messages: list[dict]


class GaussDBMemoryDDLBuilder(GaussDBDDLBuilder):
    """Build DDL for Memory message tables.

    The shared GaussDBDDLBuilder handles identifiers, safe table names, and the
    advisory lock. Memory tables additionally need A/ORA-compatible types,
    UStore, UGIN full-text indexes, gsdiskann vector indexes, and vector-empty
    markers, all of which are maintained here.
    """

    # Standard indexes cover listing, deletion, capacity accounting, FIFO,
    # recent-message access, and source/raw relationships. Vector and full-text
    # indexes are generated separately because they depend on dynamic dimensions
    # or expression indexes.
    REGULAR_INDEXES = (
        ("message_id", ("message_id",)),
        ("memory_id", ("memory_id",)),
        ("message_type", ("message_type_kwd",)),
        ("source_id", ("source_id",)),
        ("agent_session", ("agent_id", "session_id")),
        ("status_valid", ("status_int", "valid_at")),
        ("forget_at", ("forget_at",)),
    )

    def physical_table_name(self, index_name: str) -> str:
        logical = str(index_name or "").strip()
        if not logical:
            raise InvalidGaussDBObjectName(index_name)
        # A logical index_name may contain tenant IDs, hyphens, or other
        # service-generated characters. Expose only a hash suffix to keep the
        # physical name stable, short, and valid for GaussDB. memory_id is not
        # part of the table name; it isolates memories within the tenant table.
        digest = hashlib.sha1(logical.encode("utf-8")).hexdigest()[:32]
        return f"ragflow_mem_{digest}"

    def build_memory_table_ddl(self, table: str) -> str:
        # Use A/ORA-compatible VARCHAR2/NUMBER types for bounded text and
        # integers while retaining TEXT for full-text content. UStore is required
        # by the gsdiskann/vector retrieval path and is selected at table creation.
        name = self.qualified_name(table)
        pk = self.index_name(table, "pk")
        return f"""CREATE TABLE IF NOT EXISTS {name} (
  id VARCHAR2(96) NOT NULL,
  message_id NUMBER(19) NOT NULL,
  message_type_kwd VARCHAR2(64),
  source_id NUMBER(19),
  memory_id VARCHAR2(32) NOT NULL,
  user_id VARCHAR2(64),
  agent_id VARCHAR2(64),
  session_id VARCHAR2(128),
  zone_id NUMBER(10) DEFAULT 0,
  valid_at TIMESTAMP,
  invalid_at TIMESTAMP,
  forget_at TIMESTAMP,
  status_int NUMBER(10) DEFAULT 1 NOT NULL,
  content_ltks TEXT,
  tokenized_content_ltks TEXT,
  CONSTRAINT {pk} PRIMARY KEY (id)
) WITH (storage_type=USTORE)"""

    def build_regular_index_ddls(self, table: str) -> list[str]:
        name = self.qualified_name(table)
        return [f"CREATE INDEX IF NOT EXISTS {self.index_name(table, suffix)} ON {name} ({', '.join(columns)})" for suffix, columns in self.REGULAR_INDEXES]

    def build_fulltext_ugin_ddl(self, table: str) -> str:
        # Index the simple tsvector expression over tokenized_content_ltks. The
        # query path uses the same simple configuration with plainto_tsquery so
        # behavior does not depend on the database's default language.
        name = self.qualified_name(table)
        idx = self.index_name(table, "tokenized_ugin")
        return f"""CREATE INDEX IF NOT EXISTS {idx}
  ON {name}
  USING ugin (to_tsvector('simple', tokenized_content_ltks))"""

    def vector_empty_column_name(self, dim: int) -> str:
        return f"q_{self.validate_vector_dim(dim)}_vec_empty"

    def build_vector_column_ddls(self, table: str, dim: int) -> list[str]:
        # GaussDB floatvector columns cannot use NULL to represent a message
        # without a vector of this dimension. Pair every vector column with an
        # *_empty marker and use a zero-vector placeholder. Retrieval filters on
        # *_empty = FALSE, and reads restore only marked non-empty dimensions.
        dim = self.validate_vector_dim(dim)
        name = self.qualified_name(table)
        vector_col = self.vector_column_name(dim)
        empty_col = self.vector_empty_column_name(dim)
        return [
            f"ALTER TABLE {name} ADD COLUMN IF NOT EXISTS {vector_col} floatvector({dim}) DEFAULT (array_fill(0, ARRAY[{dim}])::text::floatvector({dim})) NOT NULL",
            f"ALTER TABLE {name} ADD COLUMN IF NOT EXISTS {empty_col} BOOLEAN DEFAULT TRUE NOT NULL",
        ]

    def build_vector_empty_index_ddl(self, table: str, dim: int) -> str:
        dim = self.validate_vector_dim(dim)
        name = self.qualified_name(table)
        empty_col = self.vector_empty_column_name(dim)
        idx = self.index_name(table, f"{empty_col}_idx")
        return f"CREATE INDEX IF NOT EXISTS {idx} ON {name} (memory_id, {empty_col})"

    def build_diskann_index_ddl(self, table: str, dim: int) -> str:
        # gsdiskann provides approximate nearest-neighbor lookup. Include the
        # dimension column in the index name so one tenant table can hold several
        # embedding dimensions during model migration.
        dim = self.validate_vector_dim(dim)
        name = self.qualified_name(table)
        vector_col = self.vector_column_name(dim)
        idx = self.index_name(table, f"{vector_col}_diskann")
        return f"CREATE INDEX IF NOT EXISTS {idx} ON {name} USING gsdiskann ({vector_col} cosine)"


class GaussDBMemoryConnection(GaussDBConnectionBase):
    """Store Memory messages in GaussDB.

    This class implements the Message Store interface expected by MemoryService.
    It exposes no arbitrary SQL and does not reuse DocEngine chunk queries. Every
    write, update, delete, and retrieval enforces a memory_id boundary so messages
    cannot cross memories within a shared tenant table.
    """

    def __init__(self, pool: GaussDBConnectionPool | None = None):
        super().__init__(pool=pool, logger_name="ragflow.memory_gaussdb_conn")
        # The base class initializes the shared pool and validates schema access.
        # Replace its DDL builder so create_idx and vector-column maintenance use
        # the message schema instead of the document chunk schema.
        self.ddl = GaussDBMemoryDDLBuilder(schema=self.resolved_schema)

    def create_idx(self, index_name: str, memory_id: str, vector_size: int, parser_id: str = None):
        table = self.physical_table(index_name)
        # Acquire the advisory lock before creating the base table, standard
        # indexes, full-text index, vector column, and vector-empty index. The
        # idempotent DDL supports repeated calls, concurrent initialization, and
        # first writes from multiple memories in one tenant.
        statements: list[str | tuple[str, list[Any]]] = [
            self.ddl.build_advisory_lock_sql(f"gaussdb_memory_create_table:{table}"),
            self.ddl.build_memory_table_ddl(table),
            self.ddl.build_advisory_lock_sql(f"gaussdb_memory_base_index:{table}"),
            *self.ddl.build_regular_index_ddls(table),
            self.ddl.build_advisory_lock_sql(f"gaussdb_memory_fulltext_index:{table}"),
            self.ddl.build_fulltext_ugin_ddl(table),
        ]
        statements.extend(self.ddl.build_vector_column_ddls(table, vector_size))
        statements.append(self.ddl.build_vector_empty_index_ddl(table, vector_size))
        self._execute_statements(statements)
        self._create_diskann_index_with_retry(table, vector_size)
        return True

    def delete_idx(self, index_name: str, memory_id: str):
        # The physical table boundary is the tenant-level index_name, not
        # memory_id. Tenant/user deletion may call delete_idx once per memory, so
        # DROP TABLE IF EXISTS must be repeatable. memory_id remains for interface
        # compatibility only.
        table = self.ddl.qualified_name(self.physical_table(index_name))
        self._execute_write(f"DROP TABLE IF EXISTS {table} PURGE", [])
        return True

    def index_exist(self, index_name: str, memory_id: str = None) -> bool:
        # has_index() checks the required base columns and indexes as well as the
        # table itself. Legacy or partially initialized tables therefore enter
        # the create/ensure path instead of failing later during a query.
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return False
        required_columns = set(BASE_COLUMNS)
        existing_columns = set(self._column_names(table))
        if not required_columns.issubset(existing_columns):
            return False
        required_indexes = {self.ddl.index_name(table, suffix) for suffix, _columns in self.ddl.REGULAR_INDEXES}
        required_indexes.add(self.ddl.index_name(table, "tokenized_ugin"))
        existing_indexes = set(self._index_names(table))
        return required_indexes.issubset(existing_indexes)

    def insert(self, documents: list[dict], index_name: str, memory_id: str = None) -> list[str]:
        if not documents:
            return []

        document_ids = [str(document.get("id") or "") for document in documents]
        errors: list[str] = []
        rows: list[dict] = []
        dim = None
        for document in documents:
            doc_id = str(document.get("id") or "")
            try:
                # One batch must use one embedding dimension because a MERGE can
                # bind only one q_{dim}_vec column. Reject mixed dimensions before
                # executing SQL and return the failed IDs.
                row, row_dim = self._message_to_row(document, memory_id)
                dim = row_dim if dim is None else dim
                if row_dim != dim:
                    raise ValueError(f"inconsistent content_embed dimension: expected {dim}, got {row_dim}")
                rows.append(row)
            except Exception as exc:
                logger.error("GaussDB memory normalize failed id=%s error=%s", doc_id, exc)
                errors.append(doc_id or str(exc))
        if errors:
            return [document_id for document_id in document_ids if document_id] or errors

        table = self.physical_table(index_name)
        if not self._table_exists(table):
            # The first write creates the tenant message table. memory_id stays
            # in row data and WHERE predicates as the intra-table boundary.
            self.create_idx(index_name, memory_id, dim)
        else:
            # An existing tenant table may lack the new column after an embedding
            # model dimension changes. Add the column and index on demand without
            # dropping older dimensions so historical rows remain readable via
            # their *_empty markers.
            self._ensure_vector_column_exists(table, dim)

        existing_dims = self._vector_dimensions(table)
        sql, params = self._build_merge_sql(table, dim, existing_dims, rows)
        try:
            self._execute_write(sql, params, many=True)
            return []
        except Exception as exc:
            ids = [row["id"] for row in rows]
            logger.error("GaussDB memory insert failed table=%s ids=%s error=%s", table, ids, exc)
            return ids or [str(exc)]

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        if not condition or not new_value:
            return False
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return True

        try:
            set_sql, set_params = self._build_update_set(table, index_name, memory_id, new_value)
            if not set_sql:
                return True
            where_sql, where_params = self._build_where_clause(condition, memory_ids=[memory_id], force_memory_filter=True)
            if not where_sql:
                return False
            # Always add the memory_id boundary, even when the caller supplies
            # only message_id, so an update cannot cross memories in one tenant.
            sql = f"UPDATE {self.ddl.qualified_name(table)} SET {set_sql} WHERE {where_sql}"
            self._execute_write(sql, [*set_params, *where_params])
            return True
        except Exception as exc:
            logger.error("GaussDB memory update failed table=%s condition=%s error=%s", table, condition, exc)
            return False

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        if not condition:
            return 0
        if self._has_empty_delete_list(condition):
            # An empty-list delete is a no-op. Building SQL from it could collapse
            # to a DELETE constrained only by memory_id and erase the memory.
            return 0
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return 0
        try:
            where_sql, where_params = self._build_where_clause(condition, memory_ids=[memory_id], force_memory_filter=True)
            if not where_sql:
                return 0
            # delete_message() often provides only message_id or source_id. Add
            # memory_id consistently to isolate rows in the shared tenant table.
            return self._execute_write(f"DELETE FROM {self.ddl.qualified_name(table)} WHERE {where_sql}", where_params)
        except Exception as exc:
            logger.error("GaussDB memory delete failed table=%s condition=%s error=%s", table, condition, exc)
            return 0

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        if not doc_id:
            return None
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return None
        # Read all base columns plus every existing vector and empty-marker
        # column. _message_from_row() restores the valid content_embed dimension.
        columns = [*BASE_COLUMNS]
        columns.extend(self._vector_columns_for_select(table))
        sql = f"SELECT {', '.join(columns)} FROM {self.ddl.qualified_name(table)} WHERE id = %s"
        row, description = self._fetch_one_with_description(sql, [doc_id])
        if row is None:
            return None
        return self._message_from_row(self._row_to_dict(row, description))

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
        hide_forgotten: bool = True,
        **kwargs,
    ):
        tables = [self.physical_table(name) for name in normalize_index_names(index_names)]
        memory_ids = clean_list_values(memory_ids)
        if not tables or not memory_ids:
            return SearchResult(total=0, messages=[]), 0

        parsed = self._parse_match_expressions(match_expressions)
        has_match = bool(parsed["text_query"] or parsed["vector"])
        # For a multi-tenant fan-out, fetch offset+limit candidates per table and
        # merge, sort, and slice them in memory. A single-table query pushes
        # offset and limit directly into SQL.
        collection_limit = max(int(offset or 0), 0) + max(int(limit or 0), 0)
        if collection_limit <= 0:
            collection_limit = 10000

        result = SearchResult(total=0, messages=[])
        for table in tables:
            if not self._table_exists(table):
                continue
            sql, params = self._build_search_sql(
                table=table,
                select_fields=select_fields,
                highlight_fields=highlight_fields,
                condition=condition,
                parsed=parsed,
                order_by=order_by,
                offset=0 if len(tables) > 1 else max(int(offset or 0), 0),
                limit=collection_limit if len(tables) > 1 else max(int(limit or 0), 0),
                memory_ids=memory_ids,
                hide_forgotten=hide_forgotten,
            )
            rows, description = self._fetch_all_with_description(sql, params)
            table_total, messages = self._rows_to_messages(rows, description)
            result.total += table_total
            result.messages.extend(messages)

        result.messages = self._sort_messages(result.messages, order_by, has_match)
        if len(tables) > 1:
            # Apply final pagination only after globally sorting the merged
            # tenant results; concatenating locally paged results is incorrect.
            effective_offset = max(int(offset or 0), 0)
            effective_limit = max(int(limit or 0), 0)
            if effective_limit:
                result.messages = result.messages[effective_offset : effective_offset + effective_limit]
        return result, result.total

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return None
        columns = self._select_columns(table, select_fields)
        sql = (
            f"SELECT {', '.join(columns)} "
            f"FROM {self.ddl.qualified_name(table)} "
            "WHERE memory_id = %s AND forget_at IS NOT NULL "
            # LIMIT is more reliable than FETCH FIRST for these queries on
            # A/ORA-compatible GaussDB and avoids dialect errors in maintenance.
            "ORDER BY forget_at ASC LIMIT %s"
        )
        rows, description = self._fetch_all_with_description(sql, [memory_id, int(limit)])
        _total, messages = self._rows_to_messages(rows, description)
        return SearchResult(total=len(messages), messages=messages)

    def get_missing_field_message(
        self,
        select_fields: list[str],
        index_name: str,
        memory_id: str,
        field_name: str,
        limit: int = 512,
    ):
        table = self.physical_table(index_name)
        if not self._table_exists(table):
            return None
        db_field = self.convert_field_name(field_name)
        self._validate_column(db_field)
        columns = self._select_columns(table, select_fields)
        sql = (
            f"SELECT {', '.join(columns)} "
            f"FROM {self.ddl.qualified_name(table)} "
            f"WHERE memory_id = %s AND {db_field} IS NULL "
            # Keep maintenance scans consistent with get_forgotten_messages.
            "ORDER BY valid_at ASC LIMIT %s"
        )
        rows, description = self._fetch_all_with_description(sql, [memory_id, int(limit)])
        _total, messages = self._rows_to_messages(rows, description)
        return SearchResult(total=len(messages), messages=messages)

    def get_total(self, res) -> int:
        if isinstance(res, tuple):
            return int(res[1] or 0)
        return int(getattr(res, "total", 0) or 0)

    def get_doc_ids(self, res) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return [row["id"] for row in getattr(res, "messages", []) if row.get("id")]

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        if isinstance(res, tuple):
            res = res[0]
        requested = set(fields or [])
        if not requested:
            return {}
        result: dict[str, dict] = {}
        for row in getattr(res, "messages", []) or []:
            message = self._message_from_row(row)
            doc_id = row.get("id") or message.get("id")
            if not doc_id:
                continue
            item = {}
            for field in fields:
                if field in message:
                    item[field] = message[field]
                elif field in RESULT_FIELD_DEFAULTS:
                    item[field] = RESULT_FIELD_DEFAULTS[field]
            if item:
                result[str(doc_id)] = item
        return result

    def get_highlight(self, res, keywords: list[str], field_name: str):
        if isinstance(res, tuple):
            res = res[0]
        return get_highlight_from_messages(
            getattr(res, "messages", None),
            keywords,
            field_name,
            is_english_fn=lambda s: is_english([s]),
        )

    def get_aggregation(self, res, field_name: str):
        if isinstance(res, tuple):
            res = res[0]
        return aggregate_by_field(getattr(res, "messages", None), field_name)

    def sql(self, sql: str, fetch_size: int = 128, format: str = "json"):
        # Memory Store exposes no arbitrary SQL. DocEngine SQL Q&A has its own
        # read-only validator, while Memory tables must never be accessed by
        # external SQL that can bypass the memory_id boundary.
        logger.warning("GaussDB Memory Store does not expose raw SQL execution.")
        return None

    def physical_table(self, index_name: str) -> str:
        return self.ddl.physical_table_name(index_name)

    @staticmethod
    def convert_field_name(field_name: str, use_tokenized_content: bool = False) -> str:
        # Normal reads and writes map content to the original-text column.
        # Full-text matching explicitly requests tokenized_content_ltks so
        # get_fields() does not return tokenized content.
        if field_name == "content" and use_tokenized_content:
            return "tokenized_content_ltks"
        return MEMORY_FIELD_MAP.get(field_name, field_name)

    def _message_to_row(self, message: dict, memory_id: str | None) -> tuple[dict, int]:
        content_embed = message.get("content_embed")
        if content_embed is None or len(content_embed) == 0:
            raise ValueError("content_embed is required for GaussDB memory insert")
        dim = self.ddl.validate_vector_dim(len(content_embed))
        target_memory_id = str(message.get("memory_id") or memory_id or "")
        if not target_memory_id:
            raise ValueError("memory_id is required")
        # Normalize each row by mapping application fields to physical columns,
        # converting empty user/agent/session IDs to NULL, and writing
        # content_embed to its dimension-specific vector column with *_empty set
        # to FALSE.
        row = {
            "id": str(message.get("id") or f"{target_memory_id}_{message['message_id']}"),
            "message_id": to_int_or_none(message.get("message_id")),
            "message_type_kwd": message.get("message_type"),
            "source_id": to_int_or_none(message.get("source_id")),
            "memory_id": target_memory_id,
            "user_id": none_if_empty(message.get("user_id")),
            "agent_id": none_if_empty(message.get("agent_id")),
            "session_id": none_if_empty(message.get("session_id")),
            "zone_id": to_int_or_none(message.get("zone_id", 0)) or 0,
            "valid_at": normalize_timestamp(message.get("valid_at")),
            "invalid_at": normalize_timestamp(message.get("invalid_at")),
            "forget_at": normalize_timestamp(message.get("forget_at")),
            "status_int": 1 if bool(message.get("status")) else 0,
            "content_ltks": message.get("content") or "",
            "tokenized_content_ltks": fine_grained_tokenize(tokenize(message.get("content") or "")),
            self.ddl.vector_column_name(dim): vector_literal(content_embed, dim),
            self.ddl.vector_empty_column_name(dim): False,
        }
        return row, dim

    def _message_from_row(self, row: dict) -> dict:
        # Restore backend-neutral field names and defaults. Empty invalid_at and
        # forget_at remain "-", while empty content and user_id remain "", so
        # callers do not need GaussDB-specific NULL handling.
        message = {
            "id": row.get("id"),
            "message_id": to_int_or_original(row.get("message_id")),
            "message_type": row.get("message_type_kwd"),
            "source_id": to_int_or_original(row.get("source_id")) if row.get("source_id") is not None else None,
            "memory_id": row.get("memory_id"),
            "user_id": row.get("user_id") or "",
            "agent_id": row.get("agent_id"),
            "session_id": row.get("session_id"),
            "zone_id": to_int_or_original(row.get("zone_id")) if row.get("zone_id") is not None else 0,
            "valid_at": format_timestamp(row.get("valid_at")),
            "invalid_at": format_timestamp(row.get("invalid_at")) or "-",
            "forget_at": format_timestamp(row.get("forget_at")) or "-",
            "status": bool(int(row.get("status_int") or 0)),
            "content": row.get("content_ltks") or "",
            "content_embed": self._content_embed_from_row(row),
        }
        if row.get("_score") is not None:
            message["_score"] = float(row.get("_score") or 0.0)
        return message

    def _content_embed_from_row(self, row: dict) -> list[float]:
        candidates = []
        for key, value in row.items():
            match = VECTOR_COLUMN_RE.fullmatch(str(key))
            if not match:
                continue
            dim = int(match.group("dim"))
            if row.get(self.ddl.vector_empty_column_name(dim)) is False:
                candidates.append((dim, parse_vector_value(value)))
        if not candidates:
            return []
        if len(candidates) > 1:
            # A message normally has one non-empty vector dimension. If legacy
            # data or a manual repair exposes several, log it and return the
            # highest dimension instead of failing the read.
            logger.warning("GaussDB memory row %s has multiple non-empty vector dimensions.", row.get("id"))
        return sorted(candidates, key=lambda item: item[0], reverse=True)[0][1]

    def _build_merge_sql(
        self,
        table: str,
        dim: int,
        existing_dims: list[int],
        rows: list[dict],
    ) -> tuple[str, list[list[Any]]]:
        vector_col = self.ddl.vector_column_name(dim)
        empty_col = self.ddl.vector_empty_column_name(dim)
        table_name = self.ddl.qualified_name(table)
        reset_other_dims = []
        for other_dim in existing_dims:
            if other_dim == dim:
                continue
            other_vector = self.ddl.vector_column_name(other_dim)
            other_empty = self.ddl.vector_empty_column_name(other_dim)
            reset_other_dims.append(f"{other_vector} = '{zero_vector_literal(other_dim)}'::floatvector")
            reset_other_dims.append(f"{other_empty} = TRUE")
        reset_clause = "".join(f",\n  {assignment}" for assignment in reset_other_dims)
        # A/ORA-compatible GaussDB uses MERGE ... USING (SELECT ... FROM dual)
        # for upserts instead of PostgreSQL ON CONFLICT. Updating the active
        # dimension resets all others to zero vectors with *_empty=TRUE so each
        # row exposes only one real content_embed.
        sql = f"""
MERGE INTO {table_name} t
USING (
  SELECT
    %s AS id,
    %s AS message_id,
    %s AS message_type_kwd,
    %s AS source_id,
    %s AS memory_id,
    %s AS user_id,
    %s AS agent_id,
    %s AS session_id,
    %s AS zone_id,
    %s::timestamp AS valid_at,
    %s::timestamp AS invalid_at,
    %s::timestamp AS forget_at,
    %s AS status_int,
    %s AS content_ltks,
    %s AS tokenized_content_ltks,
    %s::floatvector({dim}) AS {vector_col}
  FROM dual
) s
ON (t.id = s.id)
WHEN MATCHED THEN UPDATE SET
  message_id = s.message_id,
  message_type_kwd = s.message_type_kwd,
  source_id = s.source_id,
  memory_id = s.memory_id,
  user_id = s.user_id,
  agent_id = s.agent_id,
  session_id = s.session_id,
  zone_id = s.zone_id,
  valid_at = s.valid_at,
  invalid_at = s.invalid_at,
  forget_at = s.forget_at,
  status_int = s.status_int,
  content_ltks = s.content_ltks,
  tokenized_content_ltks = s.tokenized_content_ltks,
  {vector_col} = s.{vector_col},
  {empty_col} = FALSE{reset_clause}
WHEN NOT MATCHED THEN INSERT (
  id, message_id, message_type_kwd, source_id, memory_id, user_id, agent_id,
  session_id, zone_id, valid_at, invalid_at, forget_at, status_int,
  content_ltks, tokenized_content_ltks, {vector_col}, {empty_col}
) VALUES (
  s.id, s.message_id, s.message_type_kwd, s.source_id, s.memory_id, s.user_id,
  s.agent_id, s.session_id, s.zone_id, s.valid_at, s.invalid_at, s.forget_at,
  s.status_int, s.content_ltks, s.tokenized_content_ltks, s.{vector_col}, FALSE
)"""
        params = [
            [
                row.get("id"),
                row.get("message_id"),
                row.get("message_type_kwd"),
                row.get("source_id"),
                row.get("memory_id"),
                row.get("user_id"),
                row.get("agent_id"),
                row.get("session_id"),
                row.get("zone_id"),
                row.get("valid_at"),
                row.get("invalid_at"),
                row.get("forget_at"),
                row.get("status_int"),
                row.get("content_ltks"),
                row.get("tokenized_content_ltks"),
                row.get(vector_col),
            ]
            for row in rows
        ]
        return sql.strip(), params

    def _build_update_set(self, table: str, index_name: str, memory_id: str, new_value: dict) -> tuple[str, list[Any]]:
        fragments: list[str] = []
        params: list[Any] = []
        for field, value in (new_value or {}).items():
            if field == "remove":
                # remove clears fields. Vector columns cannot hold NULL, so use a
                # zero vector with the corresponding empty marker; set ordinary
                # columns directly to NULL.
                remove_fields = [value] if isinstance(value, str) else list(value or [])
                for remove_field in remove_fields:
                    db_field = self.convert_field_name(remove_field)
                    if VECTOR_COLUMN_RE.fullmatch(db_field):
                        dim = int(VECTOR_COLUMN_RE.fullmatch(db_field).group("dim"))
                        self._ensure_vector_column_exists(table, dim)
                        fragments.append(f"{db_field} = %s::floatvector({dim})")
                        params.append(zero_vector_literal(dim))
                        fragments.append(f"{self.ddl.vector_empty_column_name(dim)} = TRUE")
                    else:
                        self._validate_column(db_field)
                        fragments.append(f"{db_field} = NULL")
                        if db_field == "content_ltks":
                            # content_ltks owns its derived full-text tokens.
                            fragments.append("tokenized_content_ltks = NULL")
                continue
            if field == "content_embed":
                if value is None or len(value) == 0:
                    continue
                dim = self.ddl.validate_vector_dim(len(value))
                self._ensure_vector_column_exists(table, dim)
                vector_col = self.ddl.vector_column_name(dim)
                # As with insert, retain only the active dimension as a real
                # vector and reset every other dimension to prevent one message
                # from matching more than once.
                fragments.append(f"{vector_col} = %s::floatvector({dim})")
                params.append(vector_literal(value, dim))
                fragments.append(f"{self.ddl.vector_empty_column_name(dim)} = FALSE")
                for other_dim in self._vector_dimensions(table):
                    if other_dim == dim:
                        continue
                    fragments.append(f"{self.ddl.vector_column_name(other_dim)} = %s::floatvector({other_dim})")
                    params.append(zero_vector_literal(other_dim))
                    fragments.append(f"{self.ddl.vector_empty_column_name(other_dim)} = TRUE")
                continue

            db_field = self.convert_field_name(field)
            self._validate_column(db_field)
            if db_field == "content_ltks":
                # Refresh tokenized_content_ltks whenever the original content
                # changes so full-text tokens remain current.
                fragments.append("content_ltks = %s")
                params.append(value or "")
                fragments.append("tokenized_content_ltks = %s")
                params.append(fine_grained_tokenize(tokenize(value or "")))
            elif db_field in TIME_COLUMNS:
                fragments.append(f"{db_field} = %s::timestamp")
                params.append(normalize_timestamp(value))
            else:
                fragments.append(f"{db_field} = %s")
                if db_field in NUMERIC_COLUMNS:
                    params.append(to_int_or_none(value))
                else:
                    params.append(none_if_empty(value))
        return ", ".join(fragments), params

    def _build_search_sql(
        self,
        table: str,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        parsed: dict[str, Any],
        order_by: OrderByExpr,
        offset: int,
        limit: int,
        memory_ids: list[str],
        hide_forgotten: bool,
    ) -> tuple[str, list[Any]]:
        text_query = parsed["text_query"]
        vector = parsed["vector"]
        # Dispatch according to expression semantics: text plus vector uses
        # fusion, vector alone uses ANN ordering, text alone uses full-text
        # ordering, and no match expression falls back to filtering.
        if text_query and vector:
            return self._build_fusion_search_sql(table, select_fields, condition, parsed, offset, limit, memory_ids, hide_forgotten)
        if vector:
            return self._build_vector_search_sql(table, select_fields, condition, parsed, offset, limit, memory_ids, hide_forgotten)
        if text_query:
            return self._build_fulltext_search_sql(table, select_fields, highlight_fields, condition, parsed, offset, limit, memory_ids, hide_forgotten)
        return self._build_filter_search_sql(table, select_fields, condition, order_by, offset, limit, memory_ids, hide_forgotten)

    def _build_filter_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        order_by: OrderByExpr,
        offset: int,
        limit: int,
        memory_ids: list[str],
        hide_forgotten: bool,
    ) -> tuple[str, list[Any]]:
        columns = self._select_columns(table, select_fields)
        where_sql, where_params = self._build_where_clause(condition, memory_ids=memory_ids, hide_forgotten=hide_forgotten)
        order_sql = self._build_order_by(order_by) or "id ASC"
        # Filter-only lists, recent messages, and capacity accounting reuse
        # search(). COUNT(*) OVER() returns the total without another query.
        sql = f"SELECT {', '.join(columns)}, COUNT(*) OVER() AS __total FROM {self.ddl.qualified_name(table)}"
        if where_sql:
            sql += f" WHERE {where_sql}"
        sql += f" ORDER BY {order_sql}"
        params = [*where_params]
        if limit and int(limit) > 0:
            sql += " OFFSET %s LIMIT %s"
            params.extend([max(int(offset or 0), 0), int(limit)])
        return sql, params

    def _build_fulltext_search_sql(
        self,
        table: str,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        parsed: dict[str, Any],
        offset: int,
        limit: int,
        memory_ids: list[str],
        hide_forgotten: bool,
    ) -> tuple[str, list[Any]]:
        columns = self._select_columns(table, select_fields)
        text_query = parsed["text_query"]
        fts_expr = "to_tsvector('simple', tokenized_content_ltks)"
        # _parse_match_expressions already normalizes text_query with the
        # write-time tokenizer. This method only builds the GaussDB full-text
        # expression and ranking.
        score_expr = f"ts_rank({fts_expr}, plainto_tsquery('simple', %s))"
        match_expr = f"{fts_expr} @@ plainto_tsquery('simple', %s)"
        where_sql, where_params = self._build_where_clause(condition, memory_ids=memory_ids, hide_forgotten=hide_forgotten)
        where_parts = [part for part in (where_sql, match_expr) if part]
        sql = (
            f"SELECT {', '.join(columns)}, {score_expr} AS _score, COUNT(*) OVER() AS __total "
            f"FROM {self.ddl.qualified_name(table)} "
            f"WHERE {' AND '.join(where_parts)} "
            "ORDER BY _score DESC, valid_at DESC OFFSET %s LIMIT %s"
        )
        return sql, [text_query, *where_params, text_query, max(int(offset or 0), 0), effective_limit(limit, parsed["topn"])]

    def _build_vector_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        parsed: dict[str, Any],
        offset: int,
        limit: int,
        memory_ids: list[str],
        hide_forgotten: bool,
    ) -> tuple[str, list[Any]]:
        dim = parsed["vector_dim"]
        vector_col = self.ddl.vector_column_name(dim)
        empty_col = self.ddl.vector_empty_column_name(dim)
        columns = self._select_columns(table, select_fields)
        vector = parsed["vector"]
        threshold = float(parsed["similarity_threshold"] or 0.0)
        # `<+>` returns cosine distance, while RAGFlow supplies a similarity
        # threshold. Score and filter with 1 - distance, excluding zero-vector
        # placeholders through *_empty=FALSE.
        score_expr = f"1 - ({vector_col} <+> %s::floatvector({dim}))"
        distance_expr = f"{vector_col} <+> %s::floatvector({dim})"
        where_sql, where_params = self._build_where_clause(condition, memory_ids=memory_ids, hide_forgotten=hide_forgotten)
        where_parts = [part for part in (where_sql, f"{empty_col} = FALSE", f"1 - ({vector_col} <+> %s::floatvector({dim})) >= %s") if part]
        sql = (
            f"SELECT {', '.join(columns)}, {score_expr} AS _score, COUNT(*) OVER() AS __total "
            f"FROM {self.ddl.qualified_name(table)} "
            f"WHERE {' AND '.join(where_parts)} "
            f"ORDER BY {distance_expr} ASC OFFSET %s LIMIT %s"
        )
        return sql, [
            vector,
            *where_params,
            vector,
            threshold,
            vector,
            max(int(offset or 0), 0),
            effective_limit(limit, parsed["topn"]),
        ]

    def _build_fusion_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        parsed: dict[str, Any],
        offset: int,
        limit: int,
        memory_ids: list[str],
        hide_forgotten: bool,
    ) -> tuple[str, list[Any]]:
        dim = parsed["vector_dim"]
        vector_col = self.ddl.vector_column_name(dim)
        empty_col = self.ddl.vector_empty_column_name(dim)
        vector = parsed["vector"]
        threshold = float(parsed["similarity_threshold"] or 0.0)
        vector_weight = float(parsed["vector_weight"])
        text_weight = 1.0 - vector_weight
        text_query = parsed["text_query"]
        columns = self._select_columns(table, select_fields)
        inner_columns = unique_preserve_order([*columns, "tokenized_content_ltks", vector_col, empty_col, "valid_at"])
        fts_expr = "to_tsvector('simple', tokenized_content_ltks)"
        score_expr = f"ts_rank({fts_expr}, plainto_tsquery('simple', %s))"
        match_expr = f"{fts_expr} @@ plainto_tsquery('simple', %s)"
        where_sql, where_params = self._build_where_clause(condition, memory_ids=memory_ids, hide_forgotten=hide_forgotten)
        where_parts = [part for part in (where_sql, match_expr) if part]
        candidate_limit = max(int(parsed["topn"] or 0), effective_limit(limit, parsed["topn"]), 1)
        # Fusion first selects full-text candidates, then applies vector filtering
        # and a weighted sum within that set. This avoids computing vector scores
        # for rows without a text match and matches weighted_sum semantics.
        sql = (
            "WITH fulltext_results AS ("
            f" SELECT {', '.join(inner_columns)}, {score_expr} AS relevance "
            f"FROM {self.ddl.qualified_name(table)} "
            f"WHERE {' AND '.join(where_parts)} "
            "ORDER BY relevance DESC LIMIT %s"
            ") "
            f"SELECT {', '.join(columns)}, "
            f"relevance * %s + (1 - ({vector_col} <+> %s::floatvector({dim}))) * %s AS _score, "
            "COUNT(*) OVER() AS __total "
            "FROM fulltext_results "
            f"WHERE {empty_col} = FALSE "
            f"AND 1 - ({vector_col} <+> %s::floatvector({dim})) >= %s "
            "ORDER BY _score DESC, valid_at DESC OFFSET %s LIMIT %s"
        )
        return sql, [
            text_query,
            *where_params,
            text_query,
            candidate_limit,
            text_weight,
            vector,
            vector_weight,
            vector,
            threshold,
            max(int(offset or 0), 0),
            effective_limit(limit, parsed["topn"]),
        ]

    def _parse_match_expressions(self, match_expressions: list[MatchExpr] | None) -> dict[str, Any]:
        text_query = ""
        vector = None
        vector_dim = None
        topn = None
        similarity_threshold = 0.0
        vector_weight = 0.5

        for expr in match_expressions or []:
            if isinstance(expr, MatchTextExpr):
                # MsgTextQuery stores the original query in
                # extra_options["original_query"]. Prefer it and apply this
                # adapter's tokenizer instead of consuming matching_text syntax
                # generated for another backend.
                text_query = normalize_fulltext_query((expr.extra_options or {}).get("original_query") or expr.matching_text)
                topn = expr.topn if topn is None else min(topn, expr.topn)
            elif isinstance(expr, MatchDenseExpr):
                # vector_column_name may be q_{dim}_vec or the generic
                # content_embed field. Infer the latter's dimension from the
                # embedding data.
                vector_dim = parse_vector_dim(expr.vector_column_name) or len(expr.embedding_data)
                vector = vector_literal(expr.embedding_data, vector_dim)
                topn = expr.topn if topn is None else min(topn, expr.topn)
                similarity_threshold = float((expr.extra_options or {}).get("similarity", 0.0))
            elif isinstance(expr, FusionExpr):
                # FusionExpr orders weights as text_weight,vector_weight. Default
                # to 0.5 when the value is absent or incomplete.
                weights = (expr.fusion_params or {}).get("weights", "0.5,0.5")
                parts = str(weights).split(",")
                if len(parts) > 1:
                    vector_weight = get_float(parts[1])
                topn = expr.topn if topn is None else min(topn, expr.topn)

        return {
            "text_query": text_query,
            "vector": vector,
            "vector_dim": vector_dim,
            "topn": topn,
            "similarity_threshold": similarity_threshold,
            "vector_weight": vector_weight,
        }

    def _select_columns(self, table: str, select_fields: list[str] | None) -> list[str]:
        requested = select_fields or []
        columns = ["id"]
        wants_content_embed = False
        for field in requested:
            if field in {"_score", "id"}:
                continue
            if field == "content_embed":
                # content_embed is virtual; a physical table may contain several
                # q_{dim}_vec and *_empty columns. Expand it from the actual table
                # columns after processing the requested fields.
                wants_content_embed = True
                continue
            db_field = self.convert_field_name(field)
            self._validate_column(db_field)
            if db_field not in columns:
                columns.append(db_field)
        if wants_content_embed:
            columns.extend(column for column in self._vector_columns_for_select(table) if column not in columns)
        return columns

    def _vector_columns_for_select(self, table: str) -> list[str]:
        columns: list[str] = []
        # Read each vector with its empty marker to distinguish a real zero
        # vector from a placeholder.
        for dim in self._vector_dimensions(table):
            columns.append(self.ddl.vector_column_name(dim))
            columns.append(self.ddl.vector_empty_column_name(dim))
        return columns

    def _build_where_clause(
        self,
        condition: dict | None,
        memory_ids: list[str] | None = None,
        hide_forgotten: bool = False,
        force_memory_filter: bool = False,
    ) -> tuple[str, list[Any]]:
        fragments: list[str] = []
        params: list[Any] = []

        memory_values = clean_list_values(memory_ids)
        if memory_values:
            # memory_id is the mandatory boundary within a shared tenant table.
            # Add it from search/update/delete arguments even when the caller's
            # condition omits it.
            fragments.append(f"memory_id IN ({', '.join(['%s'] * len(memory_values))})")
            params.extend(memory_values)
        elif force_memory_filter:
            # Return an always-false predicate when a write lacks a memory_id
            # boundary so it cannot affect the entire tenant table.
            return "1=0", []

        if hide_forgotten:
            fragments.append("forget_at IS NULL")

        for field, value in (condition or {}).items():
            if field == "memory_id" and memory_values:
                continue
            if field == "exists":
                # Startup maintenance and missing-field scans use exists and
                # must_not-exists semantics. Map and validate the column instead
                # of interpolating external input directly.
                db_field = self.convert_field_name(str(value))
                self._validate_column(db_field)
                fragments.append(f"{db_field} IS NOT NULL")
                continue
            if field == "must_not" and isinstance(value, dict) and "exists" in value:
                db_field = self.convert_field_name(str(value["exists"]))
                self._validate_column(db_field)
                fragments.append(f"{db_field} IS NULL")
                continue
            if value is None or value == "":
                # Treat empty filter values as absent, matching the ES/OB Memory
                # adapters. Real empty strings are not searchable business data.
                continue
            db_field = self.convert_field_name(field)
            self._validate_column(db_field)
            if isinstance(value, (list, tuple, set)):
                values = clean_list_values(value)
                if not values:
                    continue
                fragments.append(f"{db_field} IN ({', '.join(['%s'] * len(values))})")
                params.extend(to_int_or_none(v) if db_field in NUMERIC_COLUMNS else v for v in values)
                continue
            fragments.append(f"{db_field} = %s")
            params.append(to_int_or_none(value) if db_field in NUMERIC_COLUMNS else value)
        return " AND ".join(fragments), params

    def _build_order_by(self, order_by: OrderByExpr | None) -> str:
        fields = getattr(order_by, "fields", None) or []
        parts = []
        for field, direction in fields:
            db_field = self.convert_field_name(field)
            # Apply the same allowlist to ORDER BY fields to prevent SQL injection.
            self._validate_column(db_field)
            parts.append(f"{db_field} {'DESC' if direction else 'ASC'}")
        return ", ".join(parts)

    def _validate_column(self, column: str) -> None:
        # SQL accepts only base columns or regex-protected dynamic vector columns.
        # This is the final guard before interpolating any dynamic field name.
        if column in BASE_COLUMN_SET or VECTOR_COLUMN_RE.fullmatch(column) or VECTOR_EMPTY_COLUMN_RE.fullmatch(column):
            return
        raise InvalidGaussDBObjectName(column)

    def _ensure_vector_column_exists(self, table: str, dim: int) -> None:
        dim = self.ddl.validate_vector_dim(dim)
        vector_col = self.ddl.vector_column_name(dim)
        empty_col = self.ddl.vector_empty_column_name(dim)
        columns_exist = self._column_exists(table, vector_col) and self._column_exists(table, empty_col)
        statements: list[str | tuple[str, list[Any]]] = [
            self.ddl.build_advisory_lock_sql(f"gaussdb_memory_vector_column:{table}:{dim}"),
        ]
        if not columns_exist:
            # Add columns only when absent. The empty-marker index uses IF NOT
            # EXISTS and can run on every ensure call to repair a partial state
            # where the columns exist but the index does not.
            statements.extend(self.ddl.build_vector_column_ddls(table, dim))
        statements.append(self.ddl.build_vector_empty_index_ddl(table, dim))
        self._execute_statements(statements)
        if not self._diskann_index_exists(table, dim):
            # Some GaussDB versions still perform costly gsdiskann initialization
            # for CREATE INDEX IF NOT EXISTS. Check pg_indexes first to avoid
            # changing work_mem on every write.
            self._create_diskann_index_with_retry(table, dim)

    def _create_diskann_index_with_retry(self, table: str, dim: int) -> None:
        ddl = self.ddl.build_diskann_index_ddl(table, dim)
        lock = self.ddl.build_advisory_lock_sql(f"gaussdb_memory_vector_index:{table}:{dim}")
        for work_mem in ("1GB", "2GB", "4GB"):
            try:
                # gsdiskann index creation depends on maintenance_work_mem. Retry
                # with bounded increases only for insufficient-memory errors;
                # propagate all other DDL failures.
                self._execute_statements([lock, f"SET LOCAL maintenance_work_mem = '{work_mem}'", ddl])
                return
            except Exception as exc:
                if not is_maintenance_work_mem_error(exc) or work_mem == "4GB":
                    raise
                logger.warning("Retrying GaussDB gsdiskann index with larger maintenance_work_mem after: %s", exc)

    def _table_exists(self, table: str) -> bool:
        row = self._fetch_one(
            """
            SELECT 1
              FROM information_schema.tables
             WHERE table_schema = %s
               AND table_name = %s
            """,
            [self.schema, table],
        )
        return bool(row)

    def _column_exists(self, table: str, column: str) -> bool:
        row = self._fetch_one(
            """
            SELECT 1
              FROM information_schema.columns
             WHERE table_schema = %s
               AND table_name = %s
               AND column_name = %s
            """,
            [self.schema, table, column],
        )
        return bool(row)

    def _column_names(self, table: str) -> list[str]:
        rows = self._fetch_all(
            """
            SELECT column_name
              FROM information_schema.columns
             WHERE table_schema = %s
               AND table_name = %s
            """,
            [self.schema, table],
        )
        return [row_value(row, "column_name", 0) for row in rows or []]

    def _index_names(self, table: str) -> list[str]:
        rows = self._fetch_all(
            """
            SELECT indexname
              FROM pg_indexes
             WHERE schemaname = %s
               AND tablename = %s
            """,
            [self.schema, table],
        )
        return [row_value(row, "indexname", 0) for row in rows or []]

    def _diskann_index_exists(self, table: str, dim: int) -> bool:
        dim = self.ddl.validate_vector_dim(dim)
        vector_col = self.ddl.vector_column_name(dim)
        expected_index = self.ddl.index_name(table, f"{vector_col}_diskann")
        # A matching name is insufficient: a historical index with another type
        # cannot support ANN retrieval. Confirm `using gsdiskann` in indexdef.
        row = self._fetch_one(
            """
            SELECT indexdef
              FROM pg_indexes
             WHERE schemaname = %s
               AND tablename = %s
               AND indexname = %s
            """,
            [self.schema, table, expected_index],
        )
        if not row:
            return False
        return "using gsdiskann" in str(row_value(row, "indexdef", 0)).lower()

    def _vector_dimensions(self, table: str) -> list[int]:
        dims = []
        # Infer every embedding dimension used by the tenant table from its
        # q_{dim}_vec catalog columns. Writes, reads, and content_embed expansion
        # all depend on this list.
        for column in self._column_names(table):
            match = VECTOR_COLUMN_RE.fullmatch(str(column or ""))
            if match:
                dims.append(int(match.group("dim")))
        return sorted(set(dims))

    def _execute_statements(self, statements: Iterable[str | tuple[str, list[Any]]]) -> None:
        # Commit DDL and maintenance statements as one transaction. Roll back the
        # group on any failure to avoid columns without their required indexes.
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            for statement in statements:
                if isinstance(statement, tuple):
                    cur.execute(statement[0], statement[1])
                else:
                    cur.execute(statement)
            conn.commit()
        except Exception as exc:
            conn.rollback()
            raise classify_gaussdb_exception(exc) from exc
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _execute_write(self, sql: str, params: list[Any], many: bool = False) -> int:
        # Centralize write commits and rollbacks, and classify psycopg2 failures
        # as GaussDB connection, permission, or authentication errors for logs
        # and health checks.
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            if many:
                cur.executemany(sql, params)
            else:
                cur.execute(sql, params)
            conn.commit()
            return int(getattr(cur, "rowcount", 0) or 0)
        except Exception as exc:
            conn.rollback()
            raise classify_gaussdb_exception(exc) from exc
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _fetch_one(self, sql: str, params: list[Any]):
        row, _description = self._fetch_one_with_description(sql, params)
        return row

    def _fetch_one_with_description(self, sql: str, params: list[Any]):
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(sql, params)
            return cur.fetchone(), getattr(cur, "description", None) or []
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _fetch_all(self, sql: str, params: list[Any]):
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(sql, params)
            return cur.fetchall()
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _fetch_all_with_description(self, sql: str, params: list[Any]):
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(sql, params)
            return cur.fetchall(), getattr(cur, "description", None) or []
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _row_to_dict(self, row, description) -> dict:
        columns = [desc[0] for desc in description]
        return dict(zip(columns, row)) if not isinstance(row, dict) else dict(row)

    def _rows_to_messages(self, rows, description) -> tuple[int, list[dict]]:
        total = 0
        messages = []
        for row in rows or []:
            raw = self._row_to_dict(row, description)
            if raw.get("__total") is not None:
                # Search SQL appends the total to each row through COUNT(*)
                # OVER(). Do not expose the internal __total column to callers.
                total = int(raw.pop("__total") or 0)
            messages.append(raw)
        if total == 0:
            total = len(messages)
        return total, messages

    def _sort_messages(self, messages: list[dict], order_by: OrderByExpr | None, has_match: bool) -> list[dict]:
        if has_match:
            # The database guarantees ordering only within each tenant table.
            # Merge match results stably by descending score, descending valid_at,
            # and ascending id to mirror the SQL ORDER BY.
            return sorted(
                messages,
                key=lambda row: (
                    -float(numeric_sort_value(row.get("_score"))),
                    descending_timestamp_sort_value(row.get("valid_at")),
                    str(row.get("id") or ""),
                ),
            )
        fields = getattr(order_by, "fields", None) or []
        if not fields:
            return sorted(messages, key=lambda row: str(row.get("id") or ""))
        sorted_messages = list(messages)
        for field, direction in reversed(fields):
            db_field = self.convert_field_name(field)
            # Preserve field types while merging: message_id/status sort
            # numerically and valid_at/forget_at sort chronologically rather than
            # as strings.
            sorted_messages.sort(key=lambda row, db_field=db_field: sortable_value(row.get(db_field), db_field), reverse=bool(direction))
        return sorted_messages

    @staticmethod
    def _has_empty_delete_list(condition: dict) -> bool:
        for key in ("message_id", "id"):
            if key in condition and isinstance(condition[key], (list, tuple, set)) and not clean_list_values(condition[key]):
                return True
        return False


def normalize_index_names(index_names: str | list[str]) -> list[str]:
    # Callers may pass one index_name, a comma-separated string, or a list.
    # Normalize each form to logical names before resolving physical tables.
    # List elements are stringified, so None becomes "None" under this contract.
    if isinstance(index_names, str):
        return [name.strip() for name in index_names.split(",") if name.strip()]
    return [str(name).strip() for name in index_names or [] if str(name).strip()]


def clean_list_values(values) -> list[Any]:
    # Remove None and empty strings to avoid IN () or treating an empty string as
    # business data. Wrap one string as a list for consistent ID handling.
    if values is None:
        return []
    if isinstance(values, (str, bytes)):
        values = [values]
    result = []
    for value in values:
        if value is None or value == "":
            continue
        result.append(value)
    return result


def effective_limit(limit: int, topn: int | None = None) -> int:
    # topn comes from the match expression and limit from pagination. Honor the
    # smaller positive constraint, or use a conservative cap when neither exists.
    candidates = [int(value) for value in (limit, topn) if value and int(value) > 0]
    return min(candidates) if candidates else 10000


def parse_vector_dim(column: str) -> int | None:
    match = VECTOR_COLUMN_RE.fullmatch(str(column or ""))
    return int(match.group("dim")) if match else None


def vector_literal(value, dim: int) -> str:
    # Bind GaussDB floatvectors as "[1.0,2.0]" and cast them explicitly to
    # floatvector(dim) in SQL. Reformat parseable strings; leave an unparseable
    # string unchanged so the database reports the actual input error.
    if isinstance(value, str):
        parsed = parse_vector_value(value)
        if parsed:
            value = parsed
        else:
            return value
    if isinstance(value, np.ndarray):
        value = value.tolist()
    if not isinstance(value, (list, tuple)) or len(value) != int(dim):
        raise ValueError(f"vector dimension mismatch: expected {dim}, got {len(value) if hasattr(value, '__len__') else 'unknown'}")
    return "[" + ",".join(str(float(item)) for item in value) + "]"


def parse_vector_value(value) -> list[float]:
    # Reads may return numpy arrays, lists, tuples, JSON arrays, or GaussDB
    # "[...]" text. Normalize them to a float list; return an empty list when no
    # usable content_embed can be decoded.
    if value is None:
        return []
    if isinstance(value, np.ndarray):
        return [float(item) for item in value.tolist()]
    if isinstance(value, (list, tuple)):
        return [float(item) for item in value]
    text = str(value).strip()
    if not text:
        return []
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        if text.startswith("[") and text.endswith("]"):
            body = text[1:-1].strip()
            if not body:
                return []
            return [float(item.strip()) for item in body.split(",")]
        return []
    if isinstance(parsed, list):
        return [float(item) for item in parsed]
    return []


def zero_vector_literal(dim: int) -> str:
    # Use a zero vector as the floatvector placeholder; q_{dim}_vec_empty carries
    # the actual empty-value semantics.
    return "[" + ",".join(["0"] * int(dim)) + "]"


def normalize_timestamp(value) -> str | None:
    # The Memory interface uses "-", "", and None for empty invalid_at/forget_at
    # values. Store each as SQL NULL.
    if value in (None, "", "-"):
        return None
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    return str(value)


def format_timestamp(value) -> str | None:
    # Format values without adding "-". _message_from_row owns field defaults so
    # other internal paths can still distinguish a real None.
    if value in (None, "", "-"):
        return None
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    return str(value)


def none_if_empty(value):
    # Optional user_id/agent_id/session_id fields do not need a stored empty
    # string. NULL better represents an omitted filter value.
    if value == "":
        return None
    return value


def to_int_or_none(value):
    if value in (None, ""):
        return None
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, (int, np.integer)):
        return int(value)
    if isinstance(value, Decimal):
        return int(value)
    return int(value)


def to_int_or_original(value):
    if value is None:
        return None
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, (int, np.integer)):
        return int(value)
    if isinstance(value, Decimal):
        return int(value)
    try:
        return int(value)
    except (TypeError, ValueError):
        return value


def row_value(row, key: str, index: int):
    if isinstance(row, dict):
        return row.get(key)
    return row[index]


def unique_preserve_order(values: list[str]) -> list[str]:
    result = []
    for value in values:
        if value not in result:
            result.append(value)
    return result


def sortable_value(value, field_name: str | None = None):
    # Cross-table merges cannot sort every value as text because numeric IDs such
    # as 100 and 99 would be ordered incorrectly. Use the physical column type
    # for numeric and timestamp fields, and text ordering for the rest.
    if field_name in NUMERIC_COLUMNS:
        return numeric_sort_value(value)
    if field_name in TIME_COLUMNS:
        return timestamp_sort_value(value)
    if value is None:
        return ""
    if isinstance(value, datetime):
        return value.isoformat()
    return str(value)


def numeric_sort_value(value) -> Decimal:
    # Decimal compares integers, numeric strings, and booleans consistently.
    # Map invalid or empty values to negative infinity so descending order places
    # them last.
    if value in (None, ""):
        return Decimal("-Infinity")
    if isinstance(value, bool):
        return Decimal(int(value))
    try:
        return Decimal(str(value))
    except (InvalidOperation, TypeError, ValueError):
        return Decimal("-Infinity")


def timestamp_sort_value(value) -> float:
    # Accept datetime objects, ISO strings, and the project's common
    # "%Y-%m-%d %H:%M:%S" format. Treat empty timestamps as negative infinity
    # for merged ascending or descending sorts.
    if value in (None, "", "-"):
        return float("-inf")
    if isinstance(value, datetime):
        return value.timestamp()
    text = str(value).strip()
    if not text:
        return float("-inf")
    try:
        return datetime.fromisoformat(text).timestamp()
    except ValueError:
        pass
    try:
        return datetime.strptime(text, "%Y-%m-%d %H:%M:%S").timestamp()
    except ValueError:
        return float("-inf")


def descending_timestamp_sort_value(value) -> float:
    # sorted() is ascending, so negate valid timestamps to obtain valid_at DESC.
    # Map empty values to positive infinity so they remain last.
    timestamp = timestamp_sort_value(value)
    if timestamp == float("-inf"):
        return float("inf")
    return -timestamp


def is_maintenance_work_mem_error(exc: Exception) -> bool:
    # Retry gsdiskann creation only when maintenance_work_mem is insufficient.
    # Dialect, permission, and extension errors must propagate.
    text = str(exc).lower()
    return "maintenance_work_mem" in text and ("required" in text or "below" in text or "insufficient" in text)


def close_cursor(cur) -> None:
    # Ignore cleanup failures so they do not hide the original SQL error.
    if cur is None:
        return
    try:
        cur.close()
    except Exception:
        pass
