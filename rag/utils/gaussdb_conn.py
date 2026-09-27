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
from __future__ import annotations

import json
import logging
import re
from math import isfinite
from typing import Any, Iterable

from psycopg2 import Error as PsycopgError, errorcodes
from pydantic import BaseModel

from common.constants import PAGERANK_FLD
from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr
from common.doc_store.gaussdb_conn_base import (
    GaussDBConnectionBase,
    GaussDBSQLValidator,
    GaussDBSearchBuilder,
    extra_field_expr,
    validate_extra_field,
)

logger = logging.getLogger("ragflow.gaussdb_conn")

SQL_QUERY_TIMEOUT_MS = 30000
VECTOR_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec$")
VECTOR_VALID_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec_valid$")


def _tokenize_query_terms(query_text: Any) -> list[str]:
    from rag.nlp import rag_tokenizer

    return [term for term in rag_tokenizer.tokenize(str(query_text or "")).split() if term]


def _application_highlight(text: Any, keywords: list[str]) -> str | None:
    if text is None:
        return None
    simple_terms, ngram_terms = GaussDBSearchBuilder.split_text_query_terms(keywords)
    unique_terms: dict[str, str] = {}
    for term in [*simple_terms, *ngram_terms]:
        unique_terms.setdefault(term.casefold(), term)
    terms = sorted(unique_terms.values(), key=len, reverse=True)
    if not terms:
        return None
    pattern = re.compile("|".join(re.escape(term) for term in terms), re.IGNORECASE)
    highlighted, count = pattern.subn(lambda match: f"<em>{match.group(0)}</em>", str(text))
    return highlighted if count else None


class GaussDBError(Exception):
    pass


CHUNK_COLUMNS = (
    "id",
    "kb_id",
    "doc_id",
    "docnm_kwd",
    "doc_type_kwd",
    "title_tks",
    "title_sm_tks",
    "content_with_weight",
    "content_ltks",
    "content_sm_ltks",
    "important_kwd",
    "important_tks",
    "question_kwd",
    "question_tks",
    "tag_kwd",
    "tag_feas",
    "available_int",
    "pagerank_fea",
    "create_time",
    "create_timestamp_flt",
    "img_id",
    "position_int",
    "page_num_int",
    "top_int",
    "metadata",
    "chunk_data",
    "extra",
    "_order_id",
    "group_id",
    "mom_id",
    "knowledge_graph_kwd",
    "source_id",
    "entity_kwd",
    "entity_type_kwd",
    "from_entity_kwd",
    "to_entity_kwd",
    "weight_int",
    "weight_flt",
    "entities_kwd",
    "rank_flt",
    "n_hop_with_weight",
    "removed_kwd",
    "raptor_kwd",
    "raptor_layer_int",
)
CHUNK_COLUMN_SET = set(CHUNK_COLUMNS)
JSONB_COLUMNS = {
    "important_kwd",
    "question_kwd",
    "tag_kwd",
    "tag_feas",
    "position_int",
    "page_num_int",
    "top_int",
    "metadata",
    "chunk_data",
    "extra",
    "source_id",
    "entities_kwd",
}
JSONB_MULTI_VALUE_COLUMNS = {
    "important_kwd",
    "question_kwd",
    "tag_kwd",
    "source_id",
    "entities_kwd",
}
KEY_COLUMNS = {"id", "kb_id"}
DOC_META_COLUMNS = ("id", "kb_id", "meta_fields")
DOC_META_COLUMN_SET = set(DOC_META_COLUMNS)
DOC_META_JSONB_COLUMNS = {"meta_fields"}
DEFAULT_VALUES = {
    "available_int": 1,
    "removed_kwd": "N",
}
_UNDEFINED_TABLE_SQLSTATE = "42P01"


def _is_undefined_table_error(exc: Exception) -> bool:
    return getattr(exc, "pgcode", None) == _UNDEFINED_TABLE_SQLSTATE or getattr(exc, "sqlstate", None) == _UNDEFINED_TABLE_SQLSTATE


def _infer_chunk_vector_size(documents: list[dict]) -> int:
    dimensions = {int(match.group("dim")) for document in documents for column in document if (match := VECTOR_COLUMN_RE.fullmatch(str(column)))}
    if len(dimensions) != 1:
        raise ValueError("cannot infer a single GaussDB vector dimension from chunk documents")
    return dimensions.pop()


class SearchResult(BaseModel):
    total: int
    chunks: list[dict]


class GaussDBConnection(GaussDBConnectionBase):
    def db_type(self) -> str:
        return "gaussdb"

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        # Keep the functional table even when the optional gsdiskann build
        # fails, while still propagating the index error to the caller.
        statements = [
            self.ddl.build_advisory_lock_sql(f"create_idx:{self.schema}:{index_name}"),
            self.ddl.build_chunk_table_ddl(index_name),
        ]
        statements.extend(self.ddl.build_regular_index_ddls(index_name))
        statements.append(self.ddl.build_fulltext_ugin_ddl(index_name))
        statements.append(self.ddl.build_ngram_fulltext_ugin_ddl(index_name))
        statements.extend(self.ddl.build_vector_column_ddls(index_name, vector_size))
        self._execute_statements(statements)
        self._create_diskann_index(index_name, vector_size)
        return True

    def _create_diskann_index(self, index_name: str, vector_size: int) -> None:
        statements = [
            self.ddl.build_advisory_lock_sql(f"create_vector_idx:{self.schema}:{index_name}:{vector_size}"),
            "SET LOCAL maintenance_work_mem = '1GB'",
            self.ddl.build_diskann_index_ddl(index_name, vector_size),
        ]
        self._execute_statements(statements)

    def create_doc_meta_idx(self, index_name: str):
        if not is_doc_meta_table(index_name):
            raise ValueError(f"invalid GaussDB document metadata table name: {index_name}")
        statements = [
            self.ddl.build_advisory_lock_sql(f"create_doc_meta_idx:{self.schema}:{index_name}"),
            *self.ddl.build_doc_meta_table_ddls(index_name),
        ]
        self._execute_statements(statements)
        return True

    def delete_idx(self, index_name: str, dataset_id: str | None):
        table = self.ddl.qualified_name(index_name)
        if dataset_id:
            try:
                self._execute_write(f"DELETE FROM {table} WHERE kb_id = %s", [dataset_id])
            except PsycopgError as exc:
                if exc.pgcode != errorcodes.UNDEFINED_TABLE:
                    raise
        else:
            self._execute_write(f"DROP TABLE IF EXISTS {table}", [])

    def index_exist(self, index_name: str, dataset_id: str | None = None) -> bool:
        self.ddl.validate_identifier(index_name)
        row = self._fetch_one(
            """
            SELECT 1
              FROM information_schema.tables
             WHERE table_schema = %s
               AND table_name = %s
             LIMIT 1
            """,
            [self.schema, index_name],
        )
        return bool(row)

    def insert(self, documents: list[dict], index_name: str, knowledgebase_id: str = None, refresh: str | bool = "wait_for") -> list[str]:
        if not documents:
            return []
        document_ids = [str(document.get("id") or "") for document in documents]
        try:
            is_meta = is_doc_meta_table(index_name)
            if is_meta:
                sql, params = self._build_doc_meta_upsert(index_name, documents, knowledgebase_id)
            else:
                normalized, errors = self._normalize_chunk_rows(index_name, documents, knowledgebase_id)
                if errors:
                    return [document_id for document_id in document_ids if document_id] or errors
                sql, params = self._build_chunk_upsert(index_name, normalized)
            try:
                self._execute_write(sql, params, many=True)
            except Exception as exc:
                if is_meta or not _is_undefined_table_error(exc) or self.index_exist(index_name, knowledgebase_id):
                    raise
                vector_size = _infer_chunk_vector_size(documents)
                logger.info(
                    "GaussDB chunk table missing during insert; creating table=%s vector_size=%s",
                    index_name,
                    vector_size,
                )
                self.create_idx(index_name, knowledgebase_id, vector_size)
                self._execute_write(sql, params, many=True)
            return []
        except Exception as exc:
            logger.error("GaussDB insert failed for table=%s ids=%s error=%s", index_name, document_ids, exc)
            return [document_id for document_id in document_ids if document_id] or [str(exc)]

    def get(self, chunk_id: str, index_name: str, knowledgebase_ids: list[str]) -> dict | None:
        if not chunk_id:
            return None
        is_meta = is_doc_meta_table(index_name)
        kb_ids = normalize_kb_ids(knowledgebase_ids)
        if not is_meta and not kb_ids:
            return None
        if not self.index_exist(index_name):
            return None
        table = self.ddl.qualified_name(index_name)
        params = [chunk_id]
        where = "id = %s"
        if kb_ids:
            placeholders = ", ".join(["%s"] * len(kb_ids))
            where += f" AND kb_id IN ({placeholders})"
            params.extend(kb_ids)
        if not is_meta and len(kb_ids) > 1:
            sql = f"SELECT * FROM {table} WHERE {where} ORDER BY kb_id ASC LIMIT 2"
            rows, description = self._fetch_all_with_description(sql, params)
            chunks = [self._row_to_chunk(row, description) for row in rows or []]
            self._check_cross_kb_duplicate_chunk_ids(chunks)
            return chunks[0] if chunks else None
        sql = f"SELECT * FROM {table} WHERE {where} LIMIT 1"
        row, description = self._fetch_one_with_description(sql, params)
        if row is None:
            return None
        return self._row_to_chunk(row, description)

    def search(
        self,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        match_expressions: list,
        order_by,
        offset: int,
        limit: int,
        index_names: str | list[str],
        knowledgebase_ids: list[str] | None = None,
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
        **kwargs,
    ) -> SearchResult:
        if knowledgebase_ids is None:
            knowledgebase_ids = kwargs.get("dataset_ids") or []
        tables = normalize_table_names(index_names)
        if not tables:
            return SearchResult(total=0, chunks=[])
        if len(tables) == 1 and is_doc_meta_table(tables[0]) and not match_expressions and not agg_fields:
            return self._search_doc_meta_table(
                select_fields=select_fields,
                condition=condition,
                order_by=order_by,
                offset=offset,
                limit=limit,
                index_name=tables[0],
                knowledgebase_ids=knowledgebase_ids,
            )
        if any(is_doc_meta_table(table) for table in tables):
            raise ValueError("document metadata tables cannot be mixed with chunk search")

        if agg_fields:
            return self._search_chunk_aggregation(
                tables=tables,
                condition=condition,
                knowledgebase_ids=knowledgebase_ids,
                agg_fields=agg_fields,
            )

        parsed = self._parse_match_expressions(match_expressions, rank_feature)
        scoped_condition = self._scoped_search_condition(condition, knowledgebase_ids)
        if len(tables) == 1:
            return self._search_chunk_table(
                table=tables[0],
                select_fields=select_fields,
                highlight_fields=highlight_fields,
                condition=scoped_condition,
                parsed=parsed,
                order_by=order_by,
                offset=offset,
                limit=limit,
            )

        effective_offset = max(int(offset or 0), 0)
        effective_limit = max(int(limit or 0), 0)
        if effective_limit == 0:
            effective_limit = 10000
        collection_limit = effective_offset + effective_limit
        total = 0
        chunks: list[dict] = []
        for table in tables:
            result = self._search_chunk_table(
                table=table,
                select_fields=select_fields,
                highlight_fields=highlight_fields,
                condition=scoped_condition,
                parsed=parsed,
                order_by=order_by,
                offset=0,
                limit=collection_limit,
            )
            total += result.total
            chunks.extend(result.chunks)
        chunks = self._sort_search_chunks(chunks, order_by, has_match=bool(parsed["keywords"] or parsed["vector"]))
        chunks = chunks[effective_offset : effective_offset + effective_limit]
        return SearchResult(total=total, chunks=chunks)

    def update(self, condition: dict, new_value: dict, index_name: str, knowledgebase_id: str) -> bool:
        if not condition or not new_value:
            return False
        if not (normalize_kb_id(knowledgebase_id) or normalize_kb_id(condition.get("kb_id"))):
            return False
        try:
            table = self.ddl.qualified_name(index_name)
            is_meta = is_doc_meta_table(index_name)
            dynamic_values = {validate_extra_field(key): value for key, value in new_value.items() if key not in {"add", "remove"} and is_dynamic_chunk_field(key, is_meta)}
            if dynamic_values:
                fixed_values = {key: value for key, value in new_value.items() if key not in dynamic_values}
                return self._update_with_dynamic_values(
                    table=table,
                    condition=condition,
                    knowledgebase_id=knowledgebase_id,
                    fixed_values=fixed_values,
                    dynamic_values=dynamic_values,
                )
            set_sql, set_params = self._build_set_clause(new_value, is_meta=is_meta, condition=condition)
            where_sql, where_params = self._build_where_clause(condition, knowledgebase_id, is_meta=is_meta)
            sql = f"UPDATE {table} SET {set_sql} WHERE {where_sql}"
            self._execute_write(sql, [*set_params, *where_params])
            return True
        except Exception as exc:
            logger.error("GaussDB update failed for table=%s condition=%s error=%s", index_name, condition, exc)
            return False

    def _update_with_dynamic_values(
        self,
        *,
        table: str,
        condition: dict,
        knowledgebase_id: str,
        fixed_values: dict,
        dynamic_values: dict[str, Any],
    ) -> bool:
        supplied_extra = fixed_values.pop("extra", None)
        set_sql, set_params = self._build_set_clause(
            fixed_values,
            is_meta=False,
            condition=condition,
        )
        where_sql, where_params = self._build_where_clause(
            condition,
            knowledgebase_id,
            is_meta=False,
        )
        if not where_sql:
            return False

        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(
                f"SELECT id, kb_id, extra FROM {table} WHERE {where_sql} FOR UPDATE",
                where_params,
            )
            rows = cur.fetchall()
            for row in rows or []:
                row_id, kb_id, stored_extra = row
                base_extra = supplied_extra if supplied_extra is not None else stored_extra
                merged_extra = dict(parse_json_dict(base_extra))
                merged_extra.update(dynamic_values)
                assignments = [part for part in (set_sql, "extra = %s::jsonb") if part]
                cur.execute(
                    f"UPDATE {table} SET {', '.join(assignments)} WHERE kb_id = %s AND id = %s",
                    [
                        *set_params,
                        json.dumps(merged_extra, ensure_ascii=False),
                        kb_id,
                        row_id,
                    ],
                )
            conn.commit()
            return True
        except Exception:
            conn.rollback()
            raise
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def delete(self, condition: dict, index_name: str, knowledgebase_id: str) -> int:
        if not condition:
            return 0
        if not (normalize_kb_id(knowledgebase_id) or normalize_kb_id(condition.get("kb_id"))):
            return 0
        try:
            table = self.ddl.qualified_name(index_name)
            where_sql, where_params = self._build_where_clause(
                condition,
                knowledgebase_id,
                is_meta=is_doc_meta_table(index_name),
            )
            return self._execute_write(f"DELETE FROM {table} WHERE {where_sql}", where_params)
        except Exception as exc:
            logger.error("GaussDB delete failed for table=%s condition=%s error=%s", index_name, condition, exc)
            return 0

    def fetch_metadata_doc_ids(
        self,
        index_name: str,
        kb_ids: list[str],
        sql_filter: str,
        filter_params: list[Any],
        limit: int,
    ) -> list[str]:
        if not is_doc_meta_table(index_name):
            raise ValueError(f"invalid GaussDB document metadata table name: {index_name}")
        scoped_kb_ids = normalize_kb_ids(kb_ids)
        if not scoped_kb_ids or not sql_filter:
            return []
        table = self.ddl.qualified_name(index_name)
        placeholders = ", ".join(["%s"] * len(scoped_kb_ids))
        effective_limit = limit if limit and limit > 0 else 10000
        sql = f"SELECT id FROM {table} WHERE kb_id IN ({placeholders}) AND ({sql_filter}) ORDER BY id LIMIT %s"
        rows = self._fetch_all(sql, [*scoped_kb_ids, *(filter_params or []), effective_limit])
        doc_ids = []
        for row in rows or []:
            if isinstance(row, dict):
                value = row.get("id")
            elif isinstance(row, (list, tuple)) and row:
                value = row[0]
            else:
                value = None
            if value is not None:
                doc_ids.append(str(value))
        return doc_ids

    def sql(self, sql: str, fetch_size: int = 128, format: str = "json"):
        self.logger.debug("GaussDBConnection.sql get sql: %s", sql)
        fetch_size = int(fetch_size or 128)
        validated = GaussDBSQLValidator.readonly_guard(
            default_limit=fetch_size,
            execution_schema=self.schema,
        ).validate_and_patch(sql)
        rows, description = self._fetch_all_with_description(
            validated.sql,
            [],
            statement_timeout_ms=SQL_QUERY_TIMEOUT_MS,
        )
        columns = [desc[0] for desc in description]

        def coerce_value(value):
            if isinstance(value, bytes):
                return value.decode("utf-8", errors="ignore")
            if isinstance(value, (dict, list)):
                return json.dumps(value, ensure_ascii=False)
            return value

        rows_list = [[coerce_value(value) for value in list(row)] for row in rows or []]
        result = {
            "columns": [{"name": column, "type": "text"} for column in columns],
            "rows": rows_list,
        }
        if format == "markdown":
            header = "|" + "|".join(columns) + "|" if columns else ""
            separator = "|" + "|".join(["---" for _ in columns]) + "|" if columns else ""
            body = "\n".join(["|" + "|".join([str(value) for value in row]) + "|" for row in rows_list])
            result["markdown"] = "\n".join([line for line in [header, separator, body] if line])
        return result

    def get_total(self, res) -> int:
        return int(res.total)

    def get_doc_ids(self, res) -> list[str]:
        return [row["id"] for row in res.chunks if "id" in row]

    @staticmethod
    def _check_cross_kb_duplicate_chunk_ids(rows) -> None:
        seen_kb_ids = {}
        for row in rows or []:
            chunk_id = row.get("id")
            if chunk_id is None:
                continue
            kb_id = row.get("kb_id")
            if kb_id is None:
                continue
            previous_kb_id = seen_kb_ids.get(chunk_id)
            if previous_kb_id is not None and previous_kb_id != kb_id:
                raise GaussDBError(f"cross-KB duplicate chunk id: {chunk_id}")
            seen_kb_ids[chunk_id] = kb_id

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        self._check_cross_kb_duplicate_chunk_ids(res.chunks)
        result = {}
        for row in res.chunks:
            chunk_id = row.get("id")
            if chunk_id is None:
                continue
            result[chunk_id] = {field: row[field] for field in fields if row.get(field) is not None}
        return result

    def get_highlight(self, res, keywords: list[str], field_name: str):
        self._check_cross_kb_duplicate_chunk_ids(res.chunks)
        highlights = {}
        for row in res.chunks:
            chunk_id = row.get("id")
            if row.get("_highlight_source") is not None:
                value = _application_highlight(row["_highlight_source"], keywords)
            else:
                value = row.get("_highlight") or row.get("highlight")
            if chunk_id and value:
                highlights[chunk_id] = value
        return highlights

    def get_aggregation(self, res, field_name: str):
        counts = {}
        result = []
        for row in res.chunks:
            if "value" in row and "count" in row:
                result.append((row["value"], row["count"]))
                continue
            value = row.get(field_name)
            if isinstance(value, list):
                for item in value:
                    if isinstance(item, str) and item.strip():
                        counts[item] = counts.get(item, 0) + 1
            elif isinstance(value, str) and value.strip():
                counts[value] = counts.get(value, 0) + 1
        return result or list(counts.items())

    def get_scores(self, res) -> dict[str, float]:
        self._check_cross_kb_duplicate_chunk_ids(res.chunks)
        return {row["id"]: float(row.get("_score") or 0.0) for row in res.chunks if row.get("id") is not None}

    def get_vector_dimensions(self, index_name: str) -> list[int]:
        self.ddl.validate_identifier(index_name)
        sql = """
            SELECT column_name
              FROM information_schema.columns
             WHERE table_schema = %s AND table_name = %s
        """
        rows = self._fetch_all(sql, [self.schema, index_name])
        dims = []
        for row in rows:
            col = row[0] if isinstance(row, (list, tuple)) else row.get("column_name")
            match = VECTOR_COLUMN_RE.fullmatch(str(col or ""))
            if match:
                dims.append(int(match.group("dim")))
        return sorted(set(dims))

    def adjust_chunk_pagerank_fea(
        self,
        chunk_id: str,
        index_name: str,
        dataset_id: str,
        delta: int,
        min_w: int = 0,
        max_w: int = 100,
        **kwargs,
    ) -> bool:
        if not chunk_id or not dataset_id:
            return False
        table = self.ddl.qualified_name(index_name)
        sql = f"UPDATE {table} SET pagerank_fea = GREATEST(%s, LEAST(%s, COALESCE(pagerank_fea, 0) + %s)) WHERE kb_id = %s AND id = %s"
        return self._execute_write(sql, [min_w, max_w, delta, dataset_id, chunk_id]) > 0

    def _normalize_chunk_rows(self, index_name: str, documents: list[dict], knowledgebase_id: str | None):
        errors = []
        rows = []
        batch_dims = sorted({int(VECTOR_COLUMN_RE.fullmatch(key).group("dim")) for document in documents for key in document if VECTOR_COLUMN_RE.fullmatch(key)})
        for document in documents:
            doc_id = str(document.get("id") or "")
            try:
                rows.append(self._normalize_chunk_row(index_name, document, knowledgebase_id, batch_dims))
            except Exception as exc:
                logger.error("GaussDB normalize chunk failed id=%s error=%s", doc_id, exc)
                errors.append(doc_id or str(exc))
        return rows, errors

    def _normalize_chunk_row(self, index_name: str, document: dict, knowledgebase_id: str | None, batch_dims: list[int]) -> dict:
        chunk_id = document.get("id")
        if not chunk_id:
            raise ValueError("chunk id is required")
        row = {"id": str(chunk_id)}

        kb_id = normalize_kb_id(document.get("kb_id")) or knowledgebase_id
        if not kb_id:
            raise ValueError("kb_id is required")
        if knowledgebase_id and kb_id != knowledgebase_id:
            raise ValueError(f"kb_id {kb_id} does not match dataset_id {knowledgebase_id}")
        row["kb_id"] = kb_id

        extra = {}
        for key, value in document.items():
            if key in {"id", "kb_id"} or VECTOR_COLUMN_RE.fullmatch(key) or VECTOR_VALID_COLUMN_RE.fullmatch(key):
                continue
            if key not in CHUNK_COLUMN_SET:
                extra[key] = value
                continue
            row[key] = normalize_column_value(key, value)

        if extra:
            existing_extra = row.get("extra")
            if isinstance(existing_extra, str):
                try:
                    existing_extra = json.loads(existing_extra)
                except json.JSONDecodeError:
                    existing_extra = {}
            if not isinstance(existing_extra, dict):
                existing_extra = {}
            existing_extra.update(extra)
            row["extra"] = json.dumps(existing_extra, ensure_ascii=False)

        metadata = row.get("metadata")
        metadata_dict = parse_json_dict(metadata)
        if metadata_dict:
            if metadata_dict.get("_group_id"):
                row["group_id"] = metadata_dict["_group_id"]
            elif row.get("doc_id"):
                row.setdefault("group_id", row.get("doc_id"))
            if metadata_dict.get("_title"):
                row["docnm_kwd"] = metadata_dict["_title"]
        elif row.get("doc_id"):
            row.setdefault("group_id", row.get("doc_id"))

        vector_columns = [key for key in document if VECTOR_COLUMN_RE.fullmatch(key)]
        if vector_columns:
            for vector_col in vector_columns:
                dim = int(VECTOR_COLUMN_RE.fullmatch(vector_col).group("dim"))
                row[vector_col] = vector_literal(document[vector_col], dim)
                row[self.ddl.vector_valid_column_name(dim)] = True
        else:
            dims = batch_dims or self.get_vector_dimensions(index_name)
            if len(dims) != 1:
                raise ValueError("cannot infer GaussDB vector dimension")
            dim = dims[0]
            row[self.ddl.vector_column_name(dim)] = zero_vector_literal(dim)
            row[self.ddl.vector_valid_column_name(dim)] = False

        for column, default in DEFAULT_VALUES.items():
            row.setdefault(column, default)
        return row

    def _build_chunk_upsert(self, index_name: str, rows: list[dict]) -> tuple[str, list[list[Any]]]:
        if not rows:
            raise ValueError("rows are required")
        columns = ordered_columns(rows)
        table = self.ddl.qualified_name(index_name)
        placeholders = ", ".join(self._placeholder(column) for column in columns)
        update_columns = [column for column in columns if column not in {"id", "kb_id"}]
        update_clause = ", ".join(f"{column} = VALUES({column})" for column in update_columns)
        sql = f"INSERT INTO {table} ({', '.join(columns)}) VALUES ({placeholders}) ON DUPLICATE KEY UPDATE {update_clause}"
        params = [[row.get(column) for column in columns] for row in rows]
        return sql, params

    def _build_doc_meta_upsert(
        self,
        index_name: str,
        documents: list[dict],
        knowledgebase_id: str | None,
    ) -> tuple[str, list[list[Any]]]:
        table = self.ddl.qualified_name(index_name)
        params = []
        for document in documents:
            doc_id = document.get("id")
            kb_id = normalize_kb_id(document.get("kb_id")) or knowledgebase_id
            if not doc_id or not kb_id:
                raise ValueError("doc metadata id and kb_id are required")
            if knowledgebase_id and kb_id != knowledgebase_id:
                raise ValueError(f"kb_id {kb_id} does not match dataset_id {knowledgebase_id}")
            params.append([str(doc_id), kb_id, json.dumps(document.get("meta_fields") or {}, ensure_ascii=False)])
        sql = f"INSERT INTO {table} (id, kb_id, meta_fields) VALUES (%s, %s, %s::jsonb) ON DUPLICATE KEY UPDATE meta_fields = VALUES(meta_fields)"
        return sql, params

    def _search_doc_meta_table(
        self,
        select_fields: list[str],
        condition: dict,
        order_by,
        offset: int,
        limit: int,
        index_name: str,
        knowledgebase_ids: list[str],
    ) -> SearchResult:
        effective_condition = dict(condition or {})
        kb_ids = normalize_kb_ids(knowledgebase_ids)
        condition_kb_ids = normalize_kb_ids(effective_condition.get("kb_id"))
        if kb_ids:
            if condition_kb_ids and not set(condition_kb_ids).issubset(set(kb_ids)):
                raise ValueError("condition kb_id must stay within knowledgebase_ids")
            if not condition_kb_ids:
                effective_condition["kb_id"] = kb_ids
        if not normalize_kb_ids(effective_condition.get("kb_id")):
            raise ValueError("GaussDB document metadata search requires a kb_id boundary")

        table = self.ddl.qualified_name(index_name)
        columns = select_doc_meta_columns(select_fields)
        where_sql, where_params = self._build_where_clause(effective_condition, None, is_meta=True)
        sql = f"SELECT {', '.join(columns)}, COUNT(*) OVER() AS __total FROM {table}"
        if where_sql:
            sql += f" WHERE {where_sql}"
        order_sql = build_doc_meta_order_by(order_by)
        if order_sql:
            sql += f" ORDER BY {order_sql}"
        effective_limit = limit if limit and limit > 0 else 10000
        effective_offset = max(int(offset or 0), 0)
        sql += " LIMIT %s OFFSET %s"
        rows, description = self._fetch_all_with_description(sql, [*where_params, effective_limit, effective_offset])
        total = 0
        chunks = []
        for row in rows or []:
            chunk = self._row_to_chunk(row, description)
            total = int(chunk.pop("__total", total or 0) or 0)
            chunks.append(chunk)
        if not chunks and effective_offset:
            count_sql = f"SELECT COUNT(*) FROM {table}"
            if where_sql:
                count_sql += f" WHERE {where_sql}"
            row = self._fetch_one(count_sql, where_params)
            total = int(row[0]) if row else 0
        return SearchResult(total=total, chunks=chunks)

    def _search_chunk_aggregation(
        self,
        tables: list[str],
        condition: dict,
        knowledgebase_ids: list[str] | None,
        agg_fields: list[str],
    ) -> SearchResult:
        if len(agg_fields) != 1:
            raise ValueError("GaussDB search supports one aggregation field per request")
        counts: dict[Any, int] = {}
        scoped_condition = self._scoped_search_condition(condition, knowledgebase_ids)
        for table in tables:
            sql, params = self._search_builder().build_aggregation_sql(
                table=table,
                field_name=agg_fields[0],
                condition=scoped_condition,
            )
            rows, description = self._fetch_all_with_description(sql, params)
            for row in rows or []:
                chunk = self._row_to_chunk(row, description)
                if chunk.get("count") is not None:
                    chunk["count"] = int(chunk["count"])
                value = chunk.get("value")
                if value is not None:
                    counts[value] = counts.get(value, 0) + int(chunk.get("count") or 0)
        chunks = [{"value": value, "count": count} for value, count in sorted(counts.items(), key=lambda item: (-item[1], str(item[0])))][:1000]
        return SearchResult(total=len(chunks), chunks=chunks)

    def _search_chunk_table(
        self,
        table: str,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        parsed: dict[str, Any],
        order_by,
        offset: int,
        limit: int,
    ) -> SearchResult:
        sql, params = self._search_builder().build_search_sql(
            table=table,
            select_fields=select_fields,
            condition=condition,
            keywords=parsed["keywords"],
            vector=parsed["vector"],
            vector_dim=parsed["vector_dim"],
            vector_weight=parsed["vector_weight"],
            similarity_threshold=parsed["similarity_threshold"],
            minimum_should_match=parsed["minimum_should_match"],
            topn=parsed["topn"],
            offset=offset,
            limit=limit,
            highlight_fields=highlight_fields,
            order_by=order_by,
            pagerank_weight=parsed["pagerank_weight"],
        )
        rows, description = self._fetch_all_with_description(sql, params)
        result = self._rows_to_search_result(rows, description)
        if result.chunks or not max(int(offset or 0), 0):
            return result

        count_sql, count_params = self._search_builder().build_search_sql(
            table=table,
            select_fields=["id"],
            condition=condition,
            keywords=parsed["keywords"],
            vector=parsed["vector"],
            vector_dim=parsed["vector_dim"],
            vector_weight=parsed["vector_weight"],
            similarity_threshold=parsed["similarity_threshold"],
            minimum_should_match=parsed["minimum_should_match"],
            topn=parsed["topn"],
            offset=0,
            limit=1,
            highlight_fields=[],
            order_by=order_by,
            pagerank_weight=parsed["pagerank_weight"],
        )
        count_rows, count_description = self._fetch_all_with_description(count_sql, count_params)
        count_result = self._rows_to_search_result(count_rows, count_description)
        return SearchResult(total=count_result.total, chunks=[])

    def _rows_to_search_result(self, rows, description) -> SearchResult:
        total = 0
        chunks = []
        for row in rows or []:
            chunk = self._row_to_chunk(row, description)
            total = int(chunk.pop("__total", total or 0) or 0)
            chunks.append(chunk)
        return SearchResult(total=total, chunks=chunks)

    def _sort_search_chunks(self, chunks: list[dict], order_by, has_match: bool) -> list[dict]:
        fields = getattr(order_by, "fields", None) or []
        if not fields:
            if has_match:
                return sorted(
                    chunks,
                    key=lambda row: (
                        -(float(row.get("_score") or 0.0)),
                        str(row.get("kb_id") or ""),
                        str(row.get("id") or ""),
                    ),
                )
            return sorted(chunks, key=lambda row: (str(row.get("kb_id") or ""), str(row.get("id") or "")))
        sorted_chunks = list(chunks)
        for field, direction in reversed(fields):
            sorted_chunks.sort(
                key=lambda row, field=field: sortable_search_value(row.get(field), field),
                reverse=bool(direction),
            )
        return sorted_chunks

    def _parse_match_expressions(
        self,
        match_expressions: list,
        rank_feature: dict | None,
    ) -> dict[str, Any]:
        keywords: list[str] = []
        vector = None
        vector_dim = None
        topn = None
        vector_weight = None
        similarity_threshold = None
        minimum_should_match = None
        pagerank_weight = 10.0

        for expr in match_expressions or []:
            if isinstance(expr, MatchTextExpr):
                query_text = (expr.extra_options or {}).get("original_query") or expr.matching_text or ""
                keywords = _tokenize_query_terms(query_text)
                minimum_should_match = (expr.extra_options or {}).get("minimum_should_match")
                topn = expr.topn if topn is None else min(topn, expr.topn)
            elif isinstance(expr, MatchDenseExpr):
                if expr.embedding_data_type != "float":
                    raise ValueError(f"unsupported GaussDB vector data type: {expr.embedding_data_type}")
                vector = list(expr.embedding_data)
                vector_dim = parse_vector_dim(expr.vector_column_name) or len(vector)
                topn = expr.topn if topn is None else min(topn, expr.topn)
                similarity_threshold = float((expr.extra_options or {}).get("similarity", 0.0))
            elif isinstance(expr, FusionExpr):
                vector_weight = parse_fusion_vector_weight(expr)
                topn = expr.topn if topn is None else min(topn, expr.topn)

        if rank_feature and rank_feature.get(PAGERANK_FLD) is not None:
            pagerank_weight = float(rank_feature[PAGERANK_FLD])

        if vector is None:
            vector_weight = 0.0
        elif not keywords:
            vector_weight = 1.0 if vector_weight is None else float(vector_weight)
        else:
            vector_weight = 0.5 if vector_weight is None else float(vector_weight)

        return {
            "keywords": keywords,
            "vector": vector,
            "vector_dim": vector_dim,
            "vector_weight": vector_weight,
            "similarity_threshold": similarity_threshold,
            "minimum_should_match": minimum_should_match,
            "topn": topn,
            "pagerank_weight": pagerank_weight,
        }

    def _scoped_search_condition(self, condition: dict | None, knowledgebase_ids: list[str] | None) -> dict:
        effective = dict(condition or {})
        kb_ids = normalize_kb_ids(knowledgebase_ids)
        if kb_ids:
            condition_kb_ids = normalize_kb_ids(effective.get("kb_id"))
            if condition_kb_ids:
                if not set(condition_kb_ids).issubset(set(kb_ids)):
                    raise ValueError("condition kb_id must stay within knowledgebase_ids")
            else:
                effective["kb_id"] = kb_ids
        if "doc_ids" in effective and "doc_id" not in effective:
            effective["doc_id"] = effective.pop("doc_ids")
        if isinstance(effective.get("doc_id"), (list, tuple, set)) and not effective["doc_id"]:
            effective.pop("doc_id")
        if not normalize_kb_ids(effective.get("kb_id")):
            raise ValueError("GaussDB chunk search requires a kb_id boundary")
        return effective

    def _search_builder(self) -> GaussDBSearchBuilder:
        builder = getattr(self, "search_builder", None)
        if builder is None:
            builder = GaussDBSearchBuilder(schema=self.schema)
            self.search_builder = builder
        return builder

    def _build_set_clause(self, new_value: dict, is_meta: bool, condition: dict) -> tuple[str, list[Any]]:
        fragments = []
        params = []
        allowed_columns = DOC_META_COLUMN_SET if is_meta else CHUNK_COLUMN_SET
        jsonb_columns = DOC_META_JSONB_COLUMNS if is_meta else JSONB_COLUMNS
        jsonb_operations: dict[str, dict[str, Any]] = {}
        updated_vector_dims = {int(match.group("dim")) for key in new_value if not is_meta and (match := VECTOR_COLUMN_RE.fullmatch(str(key)))}
        for operation, value in new_value.items():
            if operation in {"remove", "add"} and isinstance(value, dict):
                for column, item in value.items():
                    jsonb_operations.setdefault(column, {})[operation] = item
        emitted_jsonb_columns = set()

        def append_jsonb_update(column: str) -> None:
            if column in emitted_jsonb_columns:
                return
            emitted_jsonb_columns.add(column)
            operations = jsonb_operations[column]
            remove_expr = None
            remove_param = None
            if "remove" in operations:
                escaped_item = str(operations["remove"]).replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
                remove_expr = f"json_remove({column}, json_unquote(json_search({column}, 'one', jsonb_array_element_text(%s::jsonb, 0))))::jsonb"
                remove_param = json.dumps([escaped_item], ensure_ascii=False)
            if "add" in operations:
                base_expr = f"COALESCE({remove_expr}, {column}, '[]'::jsonb)" if remove_expr is not None else f"COALESCE({column}, '[]'::jsonb)"
                expression = (
                    f"(SELECT CASE WHEN candidate @> %s::jsonb THEN candidate ELSE json_array_append(candidate, '$', %s::jsonb)::jsonb END FROM (SELECT {base_expr} AS candidate) AS jsonb_update)"
                )
                params.extend(
                    [
                        json.dumps([operations["add"]], ensure_ascii=False),
                        json.dumps(operations["add"], ensure_ascii=False),
                    ]
                )
                if remove_param is not None:
                    params.append(remove_param)
            else:
                expression = f"COALESCE({remove_expr}, {column})"
                params.append(remove_param)
            fragments.append(f"{column} = {expression}")

        for key, value in new_value.items():
            if key == "remove":
                if isinstance(value, str):
                    if value in KEY_COLUMNS:
                        raise ValueError(f"key column cannot be updated: {value}")
                    if value not in allowed_columns:
                        raise ValueError(f"unsupported remove target: {value}")
                    fragments.append(f"{value} = NULL")
                    continue
                if not isinstance(value, dict):
                    raise ValueError(f"unsupported remove target: {value}")
                for column, item in value.items():
                    if column not in JSONB_MULTI_VALUE_COLUMNS:
                        raise ValueError(f"unsupported JSONB remove target: {column}")
                    append_jsonb_update(column)
                continue
            if key == "add":
                if not isinstance(value, dict):
                    raise ValueError(f"unsupported add target: {value}")
                for column, item in value.items():
                    if column not in JSONB_MULTI_VALUE_COLUMNS:
                        raise ValueError(f"unsupported JSONB add target: {column}")
                    append_jsonb_update(column)
                continue
            if key == "metadata" and not is_meta and isinstance(value, dict):
                fragments.append("metadata = %s::jsonb")
                params.append(json.dumps(value, ensure_ascii=False))
                if value.get("_group_id"):
                    fragments.append("group_id = %s")
                    params.append(value["_group_id"])
                if value.get("_title"):
                    fragments.append("docnm_kwd = %s")
                    params.append(value["_title"])
                continue
            if key in KEY_COLUMNS:
                if key == "id" and isinstance(value, str) and value == condition.get("id"):
                    continue
                raise ValueError(f"key column cannot be updated: {key}")
            if vector_match := VECTOR_COLUMN_RE.fullmatch(key):
                dim = int(vector_match.group("dim"))
                fragments.append(f"{key} = {self._placeholder(key)}")
                params.append(vector_literal(value, dim))
                fragments.append(f"{self.ddl.vector_valid_column_name(dim)} = TRUE")
                continue
            if valid_match := VECTOR_VALID_COLUMN_RE.fullmatch(str(key)):
                dim = self.ddl.validate_vector_dim(int(valid_match.group("dim")))
                if dim in updated_vector_dims:
                    continue
                fragments.append(f"{key} = %s")
                params.append(bool(value))
                continue
            if key not in allowed_columns:
                raise ValueError(f"unknown column for update: {key}")
            if key in jsonb_columns:
                fragments.append(f"{key} = %s::jsonb")
                params.append(json.dumps(value, ensure_ascii=False))
            else:
                fragments.append(f"{key} = %s")
                params.append(value)
        return ", ".join(fragments), params

    def _build_where_clause(self, condition: dict, knowledgebase_id: str | None, is_meta: bool) -> tuple[str, list[Any]]:
        effective = dict(condition or {})
        if knowledgebase_id:
            existing_kb = normalize_kb_id(effective.get("kb_id"))
            if existing_kb and existing_kb != knowledgebase_id:
                raise ValueError(f"condition kb_id {existing_kb} does not match dataset_id {knowledgebase_id}")
            effective["kb_id"] = knowledgebase_id

        fragments = []
        params = []

        def column_expression(column: str) -> str:
            validate_filter_column(column, is_meta)
            return filter_column_expr(column, is_meta)

        for key, value in effective.items():
            if key == "exists":
                column = column_expression(value)
                if is_jsonb_dynamic_chunk_field(value, is_meta):
                    fragments.append(f"({column} IS NOT NULL AND {column} <> 'null'::jsonb)")
                else:
                    fragments.append(f"{column} IS NOT NULL")
                continue
            if key == "must_not" and isinstance(value, dict) and "exists" in value:
                column = column_expression(value["exists"])
                if is_jsonb_dynamic_chunk_field(value["exists"], is_meta):
                    fragments.append(f"({column} IS NULL OR {column} = 'null'::jsonb)")
                else:
                    fragments.append(f"{column} IS NULL")
                continue
            column = column_expression(key)
            if is_jsonb_dynamic_chunk_field(key, is_meta):
                values = list(value) if isinstance(value, (list, tuple, set)) else [value]
                if not values:
                    raise ValueError(f"empty list condition for {key}")
                predicates = []
                for item in values:
                    predicates.append(f"({column} = %s::jsonb OR {column} @> %s::jsonb)")
                    params.extend(
                        [
                            json.dumps(item, ensure_ascii=False),
                            json.dumps([item], ensure_ascii=False),
                        ]
                    )
                fragments.append("(" + " OR ".join(predicates) + ")")
                continue
            if key in JSONB_MULTI_VALUE_COLUMNS:
                values = list(value) if isinstance(value, (list, tuple, set)) else [value]
                if not values:
                    raise ValueError(f"empty list condition for {key}")
                fragments.append("(" + " OR ".join([f"{column} @> %s::jsonb"] * len(values)) + ")")
                params.extend(json.dumps([item], ensure_ascii=False) for item in values)
            elif isinstance(value, (list, tuple, set)):
                values = list(value)
                if not values:
                    raise ValueError(f"empty list condition for {key}")
                fragments.append(f"{column} IN ({', '.join(['%s'] * len(values))})")
                params.extend(values)
            else:
                fragments.append(f"{column} = %s")
                params.append(value)
        return " AND ".join(fragments), params

    def _placeholder(self, column: str) -> str:
        match = VECTOR_COLUMN_RE.fullmatch(column)
        if match:
            return f"%s::floatvector({match.group('dim')})"
        if column in JSONB_COLUMNS or column in DOC_META_JSONB_COLUMNS:
            return "%s::jsonb"
        return "%s"

    def _row_to_chunk(self, row, description) -> dict:
        columns = [desc[0] for desc in description]
        raw = dict(zip(columns, row)) if not isinstance(row, dict) else dict(row)
        result = {}
        dynamic_fields = {}
        invalid_vectors = {f"q_{match.group('dim')}_vec" for key, value in raw.items() if (match := VECTOR_VALID_COLUMN_RE.fullmatch(key)) and value is False}
        for key, value in raw.items():
            if key in invalid_vectors:
                continue
            decoded = decode_column_value(key, value)
            result[key] = decoded
            if key == "extra" and isinstance(decoded, dict):
                dynamic_fields.update(decoded)
        for key, value in dynamic_fields.items():
            result.setdefault(key, value)
        return result

    def _execute_statements(self, statements: Iterable[str | tuple[str, list[Any]]]) -> None:
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
        except Exception:
            conn.rollback()
            raise
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)

    def _execute_write(self, sql: str, params: list[Any], many: bool = False) -> int:
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
        except Exception:
            conn.rollback()
            raise
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

    def _fetch_all_with_description(self, sql: str, params: list[Any], statement_timeout_ms: int | None = None):
        conn = self.pool.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            if statement_timeout_ms and statement_timeout_ms > 0:
                cur.execute(f"SET LOCAL statement_timeout = {int(statement_timeout_ms)}")
            cur.execute(sql, params)
            return cur.fetchall(), getattr(cur, "description", None) or []
        finally:
            close_cursor(cur)
            self.pool.put_conn(conn)


def is_doc_meta_table(index_name: str) -> bool:
    return str(index_name or "").startswith("ragflow_doc_meta_")


def normalize_kb_id(value) -> str | None:
    if isinstance(value, list):
        return str(value[0]) if value else None
    if value is None:
        return None
    return str(value)


def normalize_kb_ids(values) -> list[str]:
    if values is None:
        return []
    if isinstance(values, (str, bytes)):
        raw_values = [values]
    else:
        raw_values = list(values)
    normalized = []
    seen = set()
    for value in raw_values:
        if value in (None, ""):
            continue
        text = str(value)
        if text in seen:
            continue
        seen.add(text)
        normalized.append(text)
    return normalized


def normalize_table_names(index_names: str | list[str]) -> list[str]:
    if isinstance(index_names, str):
        return [name.strip() for name in index_names.split(",") if name.strip()]
    return [str(name).strip() for name in index_names or [] if str(name).strip()]


def parse_vector_dim(vector_column_name: str) -> int | None:
    match = VECTOR_COLUMN_RE.fullmatch(str(vector_column_name or ""))
    return int(match.group("dim")) if match else None


def parse_fusion_vector_weight(expr: FusionExpr) -> float | None:
    params = expr.fusion_params or {}
    weights = params.get("weights")
    if not weights:
        return None
    try:
        return float(str(weights).split(",")[-1])
    except (TypeError, ValueError):
        return None


def sortable_search_value(value: Any, field: str):
    if field == "page_num_int":
        return nested_numeric_value(value, [0])
    if field == "position_int":
        return nested_numeric_value(value, [0, 3])
    if field == "top_int":
        return nested_numeric_value(value, [0])
    if isinstance(value, (int, float)):
        return value
    if value is None:
        return ""
    return str(value)


def nested_numeric_value(value: Any, path: list[int]) -> float:
    current = value
    try:
        for index in path:
            current = current[index]
        numeric = float(current)
        return numeric if isfinite(numeric) else 100000000.0
    except (TypeError, ValueError, IndexError, KeyError, OverflowError):
        return 100000000.0


def select_doc_meta_columns(select_fields: list[str]) -> list[str]:
    if not select_fields or "*" in select_fields:
        return list(DOC_META_COLUMNS)
    columns = []
    for field in select_fields:
        validate_filter_column(field, is_meta=True)
        if field not in columns:
            columns.append(field)
    return columns


def build_doc_meta_order_by(order_by) -> str:
    fields = getattr(order_by, "fields", None) or []
    parts = []
    for field, direction in fields:
        validate_filter_column(field, is_meta=True)
        parts.append(f"{field} {'DESC' if direction else 'ASC'}")
    return ", ".join(parts)


def normalize_column_value(column: str, value: Any) -> Any:
    if value is None:
        return None
    if column in JSONB_COLUMNS:
        return json.dumps(value, ensure_ascii=False) if not isinstance(value, str) else value
    return value


def parse_json_dict(value) -> dict:
    if isinstance(value, dict):
        return value
    if isinstance(value, str):
        try:
            parsed = json.loads(value)
        except json.JSONDecodeError:
            return {}
        return parsed if isinstance(parsed, dict) else {}
    return {}


def vector_literal(value, dim: int) -> str:
    if isinstance(value, str):
        parsed = parse_vector_literal(value)
        if parsed is None:
            raise ValueError("invalid vector literal")
        value = parsed
    if not isinstance(value, (list, tuple)) or len(value) != dim:
        raise ValueError(f"vector dimension mismatch: expected {dim}, got {len(value) if hasattr(value, '__len__') else 'unknown'}")
    return "[" + ",".join(str(item) for item in value) + "]"


def parse_vector_literal(value: str):
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError:
        return None
    return parsed if isinstance(parsed, list) else None


def zero_vector_literal(dim: int) -> str:
    return "[" + ",".join(["0"] * dim) + "]"


def ordered_columns(rows: list[dict]) -> list[str]:
    present = set()
    for row in rows:
        present.update(row)
    dynamic_vectors = sorted(column for column in present if VECTOR_COLUMN_RE.fullmatch(column))
    dynamic_valid = sorted(column for column in present if VECTOR_VALID_COLUMN_RE.fullmatch(column))
    ordered = [column for column in CHUNK_COLUMNS if column in present]
    ordered.extend(column for column in dynamic_vectors + dynamic_valid if column not in ordered)
    return ordered


def validate_filter_column(column: str, is_meta: bool) -> None:
    if is_meta:
        if column not in DOC_META_COLUMN_SET:
            raise ValueError(f"unsupported metadata filter column: {column}")
        return
    if column not in CHUNK_COLUMN_SET and not VECTOR_COLUMN_RE.fullmatch(str(column)) and not VECTOR_VALID_COLUMN_RE.fullmatch(str(column)):
        validate_extra_field(column)


def is_dynamic_chunk_field(column: str, is_meta: bool = False) -> bool:
    return not is_meta and column not in CHUNK_COLUMN_SET and not VECTOR_COLUMN_RE.fullmatch(str(column)) and not VECTOR_VALID_COLUMN_RE.fullmatch(str(column))


def is_jsonb_dynamic_chunk_field(column: str, is_meta: bool = False) -> bool:
    return is_dynamic_chunk_field(column, is_meta) and column not in GaussDBSearchBuilder.JSONB_EXTRA_SCALAR_COLUMNS


def filter_column_expr(column: str, is_meta: bool = False) -> str:
    validate_filter_column(column, is_meta)
    if not is_dynamic_chunk_field(column, is_meta):
        return str(column)
    if column in GaussDBSearchBuilder.JSONB_EXTRA_SCALAR_COLUMNS:
        return f"(extra #>> '{{{column}}}')"
    return extra_field_expr(column)


def decode_column_value(column: str, value: Any) -> Any:
    if value is None:
        return None
    if column in JSONB_COLUMNS or column in DOC_META_JSONB_COLUMNS:
        if isinstance(value, str):
            try:
                return json.loads(value)
            except json.JSONDecodeError as exc:
                raise ValueError(f"invalid JSONB value for {column}") from exc
        return value
    if VECTOR_COLUMN_RE.fullmatch(column):
        if isinstance(value, str):
            parsed = parse_vector_literal(value)
            if parsed is None:
                raise ValueError(f"invalid vector value for {column}")
            return parsed
        return value
    return value


def close_cursor(cur) -> None:
    if cur is not None and hasattr(cur, "close"):
        cur.close()
