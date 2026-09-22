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
import threading
from typing import Optional

import numpy as np
from psycopg2 import pool as pg_pool

from common import settings as common_settings
from common.decorator import singleton
from memory.utils.aggregation_utils import aggregate_by_field
from memory.utils.highlight_utils import get_highlight_from_messages
from common.doc_store.doc_store_base import MatchExpr, OrderByExpr, FusionExpr, MatchTextExpr, MatchDenseExpr
from common.float_utils import get_float
from rag.nlp import is_english
from rag.nlp.rag_tokenizer import tokenize, fine_grained_tokenize

logger = logging.getLogger('ragflow.memory_vastbase_conn')

ATTEMPT_TIME = 2


class SearchResult:
    def __init__(self, total=0, messages=None):
        self.total = total
        self.messages = messages or []


@singleton
class VBConnection:
    """Message store connection for Vastbase (PostgreSQL-compatible)."""

    def __init__(self):
        self.logger = logger
        self._init_connection_pool()
        self._table_exists_cache: set[str] = set()
        self._table_exists_cache_lock = threading.RLock()

    def _init_connection_pool(self):
        vb_config = common_settings.VB
        self.host = vb_config.get("host", "host.docker.internal")
        self.port = vb_config.get("port", 5432)
        self.user = vb_config.get("user", "ragflow")
        self.password = vb_config.get("password", "Infini_Rag@123")
        self.db_name = vb_config.get("db_name", "ragflow")

        dsn = (
            f"host={self.host} port={self.port} "
            f"dbname={self.db_name} user={self.user} password={self.password}"
        )
        self.pool = pg_pool.ThreadedConnectionPool(2, 10, dsn=dsn)
        self.logger.info(
            f"VBConnection pool initialized: {self.host}:{self.port}/{self.db_name}"
        )

    def _get_conn(self):
        return self.pool.getconn()

    def _put_conn(self, conn):
        self.pool.putconn(conn)

    @staticmethod
    def convert_field_name(field_name: str, use_tokenized_content=False) -> str:
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
    def map_message_to_vb_fields(message: dict) -> dict:
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
        content_embed = message.get("content_embed", [])
        if len(content_embed) > 0:
            storage_doc[f"q_{len(content_embed)}_vec"] = content_embed
        return storage_doc

    @staticmethod
    def get_message_from_vb_doc(doc: dict) -> dict:
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

    def _table_exists(self, table_name: str) -> bool:
        if table_name in self._table_exists_cache:
            return True
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = %s)",
                    (table_name,)
                )
                exists = cur.fetchone()[0]
            if exists:
                with self._table_exists_cache_lock:
                    self._table_exists_cache.add(table_name)
            return exists
        except Exception:
            return False
        finally:
            self._put_conn(conn)

    def _create_table(self, table_name: str, vector_size: int = 0):
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                columns = [
                    "id VARCHAR(256) PRIMARY KEY",
                    "message_id VARCHAR(256) NOT NULL",
                    "message_type_kwd VARCHAR(64)",
                    "source_id VARCHAR(256)",
                    "memory_id VARCHAR(256) NOT NULL",
                    "user_id VARCHAR(256) DEFAULT ''",
                    "agent_id VARCHAR(256)",
                    "session_id VARCHAR(256)",
                    "zone_id INTEGER DEFAULT 0",
                    "valid_at VARCHAR(64)",
                    "invalid_at VARCHAR(64)",
                    "forget_at VARCHAR(64)",
                    "status_int INTEGER NOT NULL DEFAULT 1",
                    "content_ltks TEXT",
                    "tokenized_content_ltks TEXT",
                ]
                if vector_size > 0:
                    columns.append(f"q_{vector_size}_vec vector({vector_size})")

                create_sql = (
                    f"CREATE TABLE IF NOT EXISTS {table_name} (\n  "
                    + ",\n  ".join(columns)
                    + "\n)"
                )
                cur.execute(create_sql)

                cur.execute(
                    f"CREATE INDEX IF NOT EXISTS idx_{table_name}_memory_id ON {table_name} (memory_id)"
                )
                cur.execute(
                    f"CREATE INDEX IF NOT EXISTS idx_{table_name}_message_id ON {table_name} (message_id)"
                )
                cur.execute(
                    f"CREATE INDEX IF NOT EXISTS idx_{table_name}_status ON {table_name} (status_int)"
                )
            conn.commit()
            with self._table_exists_cache_lock:
                self._table_exists_cache.add(table_name)
            self.logger.info(f"Created table {table_name} with vector_size={vector_size}")
        except Exception:
            conn.rollback()
            raise
        finally:
            self._put_conn(conn)

    def _ensure_vector_column(self, table_name: str, vector_size: int):
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                col_name = f"q_{vector_size}_vec"
                cur.execute(
                    "SELECT column_name FROM information_schema.columns "
                    "WHERE table_name = %s AND column_name = %s",
                    (table_name, col_name)
                )
                if not cur.fetchone():
                    cur.execute(
                        f"ALTER TABLE {table_name} ADD COLUMN IF NOT EXISTS "
                        f"{col_name} vector({vector_size})"
                    )
                    conn.commit()
                    self.logger.info(f"Added vector column {col_name} to {table_name}")
        except Exception:
            conn.rollback()
            raise
        finally:
            self._put_conn(conn)

    def _get_filters(self, condition: dict) -> list:
        filters = []
        must_not_exists = None
        for k, v in condition.items():
            if k == "must_not":
                if isinstance(v, dict) and "exists" in v:
                    must_not_exists = v["exists"]
                continue
            if k == "exists":
                filters.append(f"{v} IS NOT NULL")
                continue
            if isinstance(v, list):
                placeholders = ", ".join(f"'{_escape_value(str(item))}'" for item in v)
                filters.append(f"{k} IN ({placeholders})")
            elif isinstance(v, str):
                filters.append(f"{k} = '{_escape_value(v)}'")
            elif isinstance(v, int):
                filters.append(f"{k} = {v}")
            elif v is None:
                filters.append(f"{k} IS NULL")
        if must_not_exists:
            filters.append(f"{must_not_exists} IS NULL")
        return filters

    def _get_vector_column_from_table(self, table_name: str) -> Optional[str]:
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT column_name FROM information_schema.columns "
                    "WHERE table_name = %s AND column_name ~ '^q_[0-9]+_vec$'",
                    (table_name,)
                )
                row = cur.fetchone()
                return row[0] if row else None
        except Exception:
            return None
        finally:
            self._put_conn(conn)

    def _row_to_dict(self, row: tuple, fields: list[str]) -> dict:
        return dict(zip(fields, row))

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
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0

        result = SearchResult(total=0, messages=[])

        output_fields = select_fields.copy()
        if "id" not in output_fields:
            output_fields = ["id"] + output_fields
        if "_score" in output_fields:
            output_fields.remove("_score")

        has_content_embed = "content_embed" in output_fields
        actual_vector_column: Optional[str] = None
        if has_content_embed:
            output_fields = [f for f in output_fields if f != "content_embed"]
            for idx_name in index_names:
                if self._table_exists(idx_name):
                    actual_vector_column = self._get_vector_column_from_table(idx_name)
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
        filters = self._get_filters(condition_dict)
        filters_expr = " AND ".join(filters) if filters else "1=1"

        fulltext_query: Optional[str] = None
        fulltext_topn: Optional[int] = None
        vector_column_name: Optional[str] = None
        vector_data: Optional[list[float]] = None
        vector_topn: Optional[int] = None
        vector_similarity_threshold: Optional[float] = None
        vector_similarity_weight: Optional[float] = None

        for m in match_expressions:
            if isinstance(m, MatchTextExpr):
                assert "original_query" in m.extra_options, "'original_query' is missing in extra_options."
                fulltext_query = m.extra_options["original_query"]
                fulltext_topn = m.topn
            elif isinstance(m, MatchDenseExpr):
                vector_column_name = m.vector_column_name
                vector_data = m.embedding_data
                vector_topn = m.topn
                vector_similarity_threshold = m.extra_options.get("similarity", 0.0) if m.extra_options else 0.0
            elif isinstance(m, FusionExpr):
                weights = m.fusion_params.get("weights", "0.5,0.5") if m.fusion_params else "0.5,0.5"
                vector_similarity_weight = get_float(weights.split(",")[1])

        has_query = bool(fulltext_query)
        has_vector = bool(vector_data is not None)

        if has_query and has_vector:
            search_type = "fusion"
        elif has_query:
            search_type = "fulltext"
        elif has_vector:
            search_type = "vector"
        else:
            search_type = "filter"

        if limit:
            if vector_topn is not None:
                limit = min(vector_topn, limit)
            if fulltext_topn is not None:
                limit = min(fulltext_topn, limit)

        for index_name in index_names:
            if not self._table_exists(index_name):
                continue

            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    if search_type == "filter":
                        orders = []
                        if order_by and order_by.fields:
                            for field, order_dir in order_by.fields:
                                field_name = self.convert_field_name(field)
                                order_str = "ASC" if order_dir == 0 else "DESC"
                                orders.append(f"{field_name} {order_str}")
                        order_by_expr = "ORDER BY " + ", ".join(orders) if orders else ""
                        limit_expr = f"LIMIT {limit}" if limit > 0 else ""
                        offset_expr = f"OFFSET {offset}" if offset > 0 else ""
                        sql = (
                            f"SELECT {fields_expr} FROM {index_name} "
                            f"WHERE {filters_expr} "
                            f"{order_by_expr} {limit_expr} {offset_expr}"
                        )
                        cur.execute(sql)
                        rows = cur.fetchall()
                        for row in rows:
                            result.messages.append(self._row_to_dict(row, db_output_fields))
                            result.total += 1

                    elif search_type == "vector" and vector_data is not None and vector_column_name:
                        vector_str = "[" + ",".join(str(v) for v in vector_data) + "]"
                        score_expr = f"(1 - (cosine_distance({vector_column_name}, '{vector_str}'::vector)))"
                        limit_expr = f"LIMIT {limit}" if limit > 0 else ""
                        offset_expr = f"OFFSET {offset}" if offset > 0 else ""
                        sim_filter = f"AND {score_expr} >= {vector_similarity_threshold}" if vector_similarity_threshold > 0 else ""
                        sql = (
                            f"SELECT {fields_expr}, {score_expr} AS _score "
                            f"FROM {index_name} "
                            f"WHERE {filters_expr} {sim_filter} "
                            f"ORDER BY _score DESC "
                            f"{limit_expr} {offset_expr}"
                        )
                        cur.execute(sql)
                        rows = cur.fetchall()
                        out_fields = db_output_fields + ["_score"]
                        for row in rows:
                            result.messages.append(self._row_to_dict(row, out_fields))
                            result.total += 1

                    elif search_type == "fulltext" and fulltext_query:
                        tsquery = " & ".join(_escape_value(fulltext_query).split())
                        limit_expr = f"LIMIT {limit}" if limit > 0 else ""
                        offset_expr = f"OFFSET {offset}" if offset > 0 else ""
                        sql = (
                            f"SELECT {fields_expr}, "
                            f"  ts_rank(to_tsvector('simple', coalesce(content_ltks, '')), "
                            f"    to_tsquery('simple', '{tsquery}')) AS _score "
                            f"FROM {index_name} "
                            f"WHERE {filters_expr} "
                            f"  AND to_tsvector('simple', coalesce(content_ltks, '')) "
                            f"    @@ to_tsquery('simple', '{tsquery}') "
                            f"ORDER BY _score DESC "
                            f"{limit_expr} {offset_expr}"
                        )
                        cur.execute(sql)
                        rows = cur.fetchall()
                        out_fields = db_output_fields + ["_score"]
                        for row in rows:
                            result.messages.append(self._row_to_dict(row, out_fields))
                            result.total += 1

                    elif search_type == "fusion" and fulltext_query and vector_data is not None and vector_column_name:
                        vector_str = "[" + ",".join(str(v) for v in vector_data) + "]"
                        vector_score = f"(1 - cosine_distance({vector_column_name}, '{vector_str}'::vector))"
                        tsquery = " & ".join(_escape_value(fulltext_query).split())
                        num_candidates = (vector_topn or limit) + (fulltext_topn or limit)
                        score_expr = (
                            f"(ts_rank(to_tsvector('simple', coalesce(content_ltks, '')), "
                            f"  to_tsquery('simple', '{tsquery}')) * {1 - vector_similarity_weight} "
                            f"  + {vector_score} * {vector_similarity_weight})"
                        )
                        sql = (
                            f"WITH candidates AS ("
                            f"  SELECT {fields_expr}, {score_expr} AS _score "
                            f"  FROM {index_name} "
                            f"  WHERE {filters_expr} "
                            f"    AND to_tsvector('simple', coalesce(content_ltks, '')) "
                            f"      @@ to_tsquery('simple', '{tsquery}') "
                            f"  LIMIT {num_candidates}"
                            f") "
                            f"SELECT * FROM candidates "
                            f"WHERE {vector_score} >= {vector_similarity_threshold} "
                            f"ORDER BY _score DESC "
                            f"LIMIT {limit} OFFSET {offset}"
                        )
                        cur.execute(sql)
                        rows = cur.fetchall()
                        out_fields = db_output_fields + ["_score"]
                        for row in rows:
                            result.messages.append(self._row_to_dict(row, out_fields))
                            result.total += 1

            except Exception as e:
                self.logger.error(f"VBConnection.search error on {index_name}: {str(e)}")
            finally:
                self._put_conn(conn)

        if result.total == 0:
            result.total = len(result.messages)

        return result, result.total

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        if not self._table_exists(index_name):
            return None
        db_fields = [self.convert_field_name(f) for f in select_fields]
        fields_expr = ", ".join(db_fields)
        sql = (
            f"SELECT {fields_expr} FROM {index_name} "
            f"WHERE memory_id = '{_escape_value(memory_id)}' AND forget_at IS NOT NULL "
            f"ORDER BY forget_at ASC LIMIT {limit}"
        )
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(sql)
                rows = cur.fetchall()
            result = SearchResult(total=len(rows), messages=[])
            for row in rows:
                result.messages.append(self._row_to_dict(row, db_fields))
            return result
        except Exception as e:
            self.logger.error(f"VBConnection.get_forgotten_messages error: {str(e)}")
            return None
        finally:
            self._put_conn(conn)

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int = 512):
        if not self._table_exists(index_name):
            return None
        db_field = self.convert_field_name(field_name)
        db_fields = [self.convert_field_name(f) for f in select_fields]
        fields_expr = ", ".join(db_fields)
        sql = (
            f"SELECT {fields_expr} FROM {index_name} "
            f"WHERE memory_id = '{_escape_value(memory_id)}' AND {db_field} IS NULL "
            f"ORDER BY valid_at ASC LIMIT {limit}"
        )
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(sql)
                rows = cur.fetchall()
            result = SearchResult(total=len(rows), messages=[])
            for row in rows:
                result.messages.append(self._row_to_dict(row, db_fields))
            return result
        except Exception as e:
            self.logger.error(f"VBConnection.get_missing_field_message error: {str(e)}")
            return None
        finally:
            self._put_conn(conn)

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        if not self._table_exists(index_name):
            return None
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(f"SELECT * FROM {index_name} WHERE id = %s", (doc_id,))
                row = cur.fetchone()
                if not row:
                    return None
                cols = [desc[0] for desc in cur.description]
                doc = dict(zip(cols, row))
            return self.get_message_from_vb_doc(doc)
        except Exception as e:
            self.logger.error(f"VBConnection.get error: {str(e)}")
            return None
        finally:
            self._put_conn(conn)

    def insert(self, documents: list[dict], index_name: str, memory_id: str = None) -> list[str]:
        if not documents:
            return []

        vector_size = len(documents[0].get("content_embed", [])) if "content_embed" in documents[0] else 0

        if not self._table_exists(index_name):
            self._create_table(index_name, vector_size)
        elif vector_size > 0:
            self._ensure_vector_column(index_name, vector_size)

        docs = []
        ids = []
        for document in documents:
            d = self.map_message_to_vb_fields(document)
            ids.append(d["id"])
            docs.append(d)

        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                columns = list(docs[0].keys())
                col_list = ", ".join(columns)
                placeholders = ", ".join([f"%({col})s" for col in columns])
                update_cols = ", ".join(
                    f"{col} = EXCLUDED.{col}" for col in columns if col != "id"
                )
                insert_sql = (
                    f"INSERT INTO {index_name} ({col_list}) "
                    f"VALUES ({placeholders}) "
                    f"ON CONFLICT (id) DO UPDATE SET {update_cols}"
                )
                for d in docs:
                    cur.execute(insert_sql, d)
            conn.commit()
            return []
        except Exception as e:
            conn.rollback()
            self.logger.error(f"VBConnection.insert error: {str(e)}")
            return [str(e)]
        finally:
            self._put_conn(conn)

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        if not self._table_exists(index_name):
            return True

        condition["memory_id"] = memory_id
        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        filters = self._get_filters(condition_dict)

        update_dict = {self.convert_field_name(k): v for k, v in new_value.items()}
        if "content_ltks" in update_dict:
            update_dict["tokenized_content_ltks"] = fine_grained_tokenize(tokenize(update_dict["content_ltks"]))
        update_dict.pop("id", None)

        set_values = []
        params = {}
        for i, (k, v) in enumerate(update_dict.items()):
            if k == "remove":
                if isinstance(v, str):
                    set_values.append(f"{v} = NULL")
            elif k == "status":
                set_values.append(f"status_int = {1 if v else 0}")
            else:
                set_values.append(f"{k} = %(val_{i})s")
                params[f"val_{i}"] = v

        if not set_values or not filters:
            return True

        sql = f"UPDATE {index_name} SET {', '.join(set_values)} WHERE {' AND '.join(filters)}"
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(sql, params)
            conn.commit()
            return True
        except Exception as e:
            conn.rollback()
            self.logger.error(f"VBConnection.update error: {str(e)}")
            return False
        finally:
            self._put_conn(conn)

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        if not self._table_exists(index_name):
            return 0

        condition_dict = {self.convert_field_name(k): v for k, v in condition.items()}
        condition_dict["memory_id"] = memory_id
        filters = self._get_filters(condition_dict)
        if not filters:
            return 0

        sql = f"DELETE FROM {index_name} WHERE {' AND '.join(filters)}"
        conn = self._get_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(sql)
                deleted = cur.rowcount
            conn.commit()
            return deleted
        except Exception as e:
            conn.rollback()
            self.logger.error(f"VBConnection.delete error: {str(e)}")
            return 0
        finally:
            self._put_conn(conn)

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
        if isinstance(res, tuple):
            res = res[0]
        res_fields = {}
        if not fields:
            return {}
        messages = res.messages if hasattr(res, 'messages') else []
        for doc in messages:
            message = self.get_message_from_vb_doc(doc)
            m = {}
            for n, v in message.items():
                if n not in fields:
                    continue
                if isinstance(v, list):
                    m[n] = v
                    continue
                if n in ["message_id", "source_id", "valid_at", "invalid_at", "forget_at", "status"] and isinstance(v, (int, float, bool)):
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
        if isinstance(res, tuple):
            res = res[0]
        messages = getattr(res, "messages", None)
        return get_highlight_from_messages(
            messages, keywords, field_name, is_english_fn=lambda s: is_english([s])
        )

    def get_aggregation(self, res, field_name: str):
        if isinstance(res, tuple):
            res_obj = res[0]
        else:
            res_obj = res
        messages = getattr(res_obj, "messages", None)
        return aggregate_by_field(messages, field_name)


def _escape_value(value: str) -> str:
    if value is None:
        return ""
    return str(value).replace("'", "''")