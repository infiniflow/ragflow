#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from sqlglot import exp, parse_one

from common.doc_store.gaussdb_conn_base import (
    ExposedGaussDBTable,
    GaussDBSQLValidator,
    gaussdb_field_json_path_parts,
    is_gaussdb_aggregate_sql,
    jsonb_path_literal,
)


logger = logging.getLogger(__name__)

_SQL_RULES = """RULES:
1. Use only the table and fields listed below.
2. Select doc_id and docnm_kwd for non-aggregate data queries.
3. Do not add kb_id to SELECT or WHERE; the system injects and validates the KB scope.
4. Use only the static JSONB path literals listed under Fields.
5. Do not construct dynamic JSONB paths.
6. Do not use json_extract, json_extract_string, or json_extract_isnull; use #>> / #> instead.
7. A/ORA treats '' as NULL; never compare with = '' or <> ''; use IS NULL or IS NOT NULL.
8. Return exactly one read-only SELECT statement and no other text.
9. Do not use SELECT *, window functions, or system functions.
10. Do not use LATERAL, JOIN, UNION, INTERSECT, EXCEPT, or OR.
11. Do not expand JSON with jsonb_each, jsonb_each_text, jsonb_to_record, jsonb_to_recordset, jsonb_array_elements, or jsonb_array_elements_text.
12. Do not use DML or DDL, including INSERT, UPDATE, DELETE, MERGE, CREATE, ALTER, DROP, TRUNCATE, GRANT, REVOKE, CALL, COPY, or DO."""


def build_validator(table_name, kb_ids, field_map):
    table = ExposedGaussDBTable.from_field_map(table_name, kb_ids, field_map)
    return GaussDBSQLValidator({table.logical_name: table}, default_limit=128)


def is_aggregate_sql(sql):
    return is_gaussdb_aggregate_sql(sql)


def _json_path_literal(field, field_type=None):
    return jsonb_path_literal(gaussdb_field_json_path_parts(str(field), field_type))


def _field_lines(field_map):
    return "\n".join(f"  - {field} ({descriptor}): chunk_data #>> {_json_path_literal(field, descriptor)}" for field, descriptor in (field_map or {}).items())


def build_sql_prompt(table_name, field_map, question):
    return """You are a Database Administrator. Write SQL for GaussDB A/ORA mode for a table with JSONB 'chunk_data' column.

Table: {table_name}
JSONB text extraction: chunk_data #>> '{{FieldName}}'
JSONB value extraction: chunk_data #> '{{FieldName}}'
Numeric cast: CAST(chunk_data #>> '{{FieldName}}' AS DOUBLE PRECISION)
Date cast: to_date(chunk_data #>> '{{FieldName}}', 'YYYY-MM-DD')
NULL check: (chunk_data #>> '{{FieldName}}') IS NOT NULL

{rules}

Fields:
{fields}

Question: {question}""".format(
        table_name=table_name,
        rules=_SQL_RULES,
        fields=_field_lines(field_map),
        question=question,
    )


def build_user_prompt(table_name, field_map, question):
    return """Table: {table_name}
Fields:
{fields}
Question: {question}
Write SQL using GaussDB JSONB #>> / #> syntax. Include doc_id and docnm_kwd for data queries. Only SQL.""".format(
        table_name=table_name,
        fields=_field_lines(field_map),
        question=question,
    )


def build_repair_prompt(table_name, field_map, question, previous_sql):
    return """Table name: {table_name};
GaussDB JSONB fields available in chunk_data:
{fields}

Question: {question}
Previous SQL:
{previous_sql}

The previous SQL result is missing required source columns for citations.
Rewrite SQL to keep the same query intent and include doc_id and docnm_kwd in the SELECT list.
Use chunk_data #>> '{{field}}' or chunk_data #> '{{field}}' for JSONB fields.
For date fields, use to_date(chunk_data #>> '{{field}}', 'YYYY-MM-DD').

{rules}""".format(
        table_name=table_name,
        fields=_field_lines(field_map),
        question=question,
        previous_sql=previous_sql,
        rules=_SQL_RULES,
    )


def build_retry_prompt(table_name, field_map, question, error):
    return """
Table name: {table_name};
GaussDB JSONB fields available in chunk_data:
{fields}

Question: {question}
Please write SQL using chunk_data #>> '{{field}}' / chunk_data #> '{{field}}' syntax. Include doc_id and docnm_kwd for data queries. Only SQL.
For date fields, use to_date(chunk_data #>> '{{field}}', 'YYYY-MM-DD').

The previous SQL error is:
{error}

Correct the SQL for GaussDB A/ORA mode.

{rules}
""".format(
        table_name=table_name,
        fields=_field_lines(field_map),
        question=question,
        error=error,
        rules=_SQL_RULES,
    )


def build_aggregate_source_sql(sql, doc_name_column, include_kb_id):
    statement = parse_one(sql, read=GaussDBSQLValidator.PARSE_DIALECT)
    if statement.args.get("having"):
        raise ValueError("GaussDB aggregate source lookup cannot preserve HAVING safely")

    columns = [exp.column("doc_id"), exp.column(doc_name_column)]
    if include_kb_id:
        columns.append(exp.column("kb_id"))
    statement.set("expressions", columns)
    statement.set("distinct", None)
    statement.set("group", None)
    statement.set("order", None)
    statement.set("limit", None)
    statement.set("offset", None)
    return statement.sql(dialect=GaussDBSQLValidator.PARSE_DIALECT)


def build_source_reference(table, kb_ids):
    rows = table.get("rows", [])
    if not rows:
        return None

    columns = table.get("columns", [])
    doc_idx = next((i for i, column in enumerate(columns) if column["name"].lower() == "doc_id"), None)
    name_idx = next((i for i, column in enumerate(columns) if column["name"].lower() in ["docnm_kwd", "docnm"]), None)
    kb_idx = next((i for i, column in enumerate(columns) if column["name"].lower() in ["kb_id", "kb_id_kwd"]), None)
    if doc_idx is None or name_idx is None:
        return None

    chunks = []
    doc_aggs = {}
    for row in rows:
        chunk = {"doc_id": row[doc_idx], "docnm_kwd": row[name_idx]}
        if len(kb_ids or []) == 1:
            chunk["kb_id"] = kb_ids[0]
        elif kb_idx is not None:
            chunk["kb_id"] = row[kb_idx]
        chunks.append(chunk)

        doc_id = row[doc_idx]
        if doc_id not in doc_aggs:
            doc_aggs[doc_id] = {"doc_name": row[name_idx], "count": 0}
        doc_aggs[doc_id]["count"] += 1

    return {
        "chunks": chunks,
        "doc_aggs": [{"doc_id": doc_id, "doc_name": values["doc_name"], "count": values["count"]} for doc_id, values in doc_aggs.items()],
    }


def complete_reference_kb_ids(result, table_name, kb_ids, validator, sql_retrieval):
    if len(kb_ids or []) <= 1:
        return

    chunks = result["reference"]["chunks"]
    doc_ids = list(dict.fromkeys(str(chunk["doc_id"]) for chunk in chunks if chunk.get("doc_id") and not chunk.get("kb_id")))
    if not doc_ids:
        return

    quoted_doc_ids = ", ".join("'" + doc_id.replace("'", "''") + "'" for doc_id in doc_ids)
    lookup_sql = f"SELECT doc_id, kb_id FROM {table_name} WHERE doc_id IN ({quoted_doc_ids})"
    try:
        lookup_sql = validator.validate_and_patch(lookup_sql).sql
        lookup_tbl = sql_retrieval(lookup_sql, format="json")
        doc_idx = next((i for i, column in enumerate(lookup_tbl.get("columns", [])) if column["name"].lower() == "doc_id"), None)
        kb_idx = next((i for i, column in enumerate(lookup_tbl.get("columns", [])) if column["name"].lower() in ["kb_id", "kb_id_kwd"]), None)
        if doc_idx is None or kb_idx is None:
            return
        mapping = {row[doc_idx]: row[kb_idx] for row in lookup_tbl.get("rows", []) if len(row) > max(doc_idx, kb_idx)}
        for chunk in chunks:
            if not chunk.get("kb_id") and chunk.get("doc_id") in mapping:
                chunk["kb_id"] = mapping[chunk["doc_id"]]
    except Exception:
        logger.warning("use_sql: Failed to complete GaussDB reference kb_id values", exc_info=True)
