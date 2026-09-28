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
import hashlib
import json
import logging
import re
from dataclasses import dataclass
from numbers import Integral
from timeit import default_timer as timer
from typing import Any

from common.doc_store.doc_store_base import DocStoreConnection, MatchExpr, OrderByExpr


_GAUSSDB_DSN_PATTERN = re.compile(r"(?i)\b(?:postgres(?:ql)?|gaussdb)://[^\s'\"<>]+")
_GAUSSDB_SECRET_PATTERN = re.compile(
    r"""
    (?P<prefix>
        (?P<key_quote>["']?)
        (?P<key>\b(?:password|passwd|access_token|token|api_key|secret|dsn)\b)
        (?P=key_quote)
        (?P<sep>\s*[:=]\s*)
    )
    (?P<value>
        "(?:\\.|""|[^"\\])*"
        | '(?:\\.|''|[^'\\])*'
        | "(?:\\.|[^"\\])*\Z
        | '(?:\\.|[^'\\])*\Z
        | [^\s]+
    )
    """,
    flags=re.IGNORECASE | re.VERBOSE,
)


def mask_gaussdb_text(value: Any) -> str:
    message = str(value)
    message = _GAUSSDB_DSN_PATTERN.sub("***", message)

    def replace_secret(match: re.Match[str]) -> str:
        secret = match.group("value")
        if len(secret) >= 2 and secret[0] in {"'", '"'} and secret[-1] == secret[0]:
            masked = f"{secret[0]}***{secret[0]}"
        else:
            masked = "***"
        return f"{match.group('prefix')}{masked}"

    return _GAUSSDB_SECRET_PATTERN.sub(replace_secret, message)


class InvalidGaussDBObjectName(ValueError):
    pass


class UnsafeGaussDBSQL(ValueError):
    pass


EXTRA_FIELD_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]{0,127}$")


def validate_extra_field(name: str) -> str:
    field = str(name or "")
    if not EXTRA_FIELD_RE.fullmatch(field):
        raise InvalidGaussDBObjectName(name)
    return field


def extra_field_expr(name: str, *, prefix: str | None = None, text: bool = False) -> str:
    field = validate_extra_field(name)
    source = f"{prefix}.extra" if prefix else "extra"
    operator = "->>" if text else "->"
    return f"({source} {operator} '{field}')"


def _parse_gaussdb_statements(sql: str, dialect: str = "postgres"):
    from sqlglot import exp, parse

    try:
        return exp, parse(sql, read=dialect)
    except Exception:
        raise UnsafeGaussDBSQL("SQL parse failed") from None


def is_gaussdb_aggregate_sql(sql: str) -> bool:
    exp, expressions = _parse_gaussdb_statements(sql)
    if len(expressions) != 1:
        raise UnsafeGaussDBSQL("exactly one SQL statement is allowed")
    return any(isinstance(node, exp.AggFunc) for node in expressions[0].walk())


_FIELD_DESCRIPTOR_TYPES = {
    "bool",
    "boolean",
    "date",
    "datetime",
    "double",
    "float",
    "integer",
    "json",
    "number",
    "string",
    "text",
}


def gaussdb_field_json_path_parts(field: str, descriptor: Any = None) -> tuple[str, ...]:
    candidate = str(descriptor or "").strip()
    if candidate and candidate.lower() not in _FIELD_DESCRIPTOR_TYPES:
        return tuple(part for part in candidate.split(".") if part)
    return tuple(part for part in str(field).split(".") if part)


@dataclass(frozen=True)
class ExposedGaussDBTable:
    logical_name: str
    physical_name: str
    allowed_columns: frozenset[str]
    json_fields: dict[str, tuple[str, ...]]
    required_kb_ids: tuple[str, ...]

    @classmethod
    def from_field_map(cls, physical_name: str, kb_ids: list[str] | tuple[str, ...], field_map: dict[str, Any]):
        json_fields = {}
        for field, descriptor in (field_map or {}).items():
            parts = gaussdb_field_json_path_parts(str(field), descriptor)
            if parts:
                json_fields[str(field)] = parts
        return cls(
            logical_name=physical_name,
            physical_name=physical_name,
            allowed_columns=frozenset({"doc_id", "docnm_kwd", "kb_id", "chunk_data"}),
            json_fields=json_fields,
            required_kb_ids=tuple(str(kid) for kid in kb_ids or () if str(kid)),
        )


@dataclass(frozen=True)
class ValidatedGaussDBSQL:
    sql: str
    columns: list[str] | None = None
    is_aggregation: bool = False


def jsonb_path_literal(parts: list[str] | tuple[str, ...]) -> str:
    if not isinstance(parts, (list, tuple)) or not parts:
        raise UnsafeGaussDBSQL("empty JSONB path")
    encoded = []
    for part in parts:
        if not isinstance(part, str):
            raise UnsafeGaussDBSQL("invalid JSONB path segment")
        segment = part
        if not segment:
            raise UnsafeGaussDBSQL("empty JSONB path segment")
        if re.fullmatch(r"[A-Za-z0-9_]+", segment):
            encoded.append(segment)
        else:
            escaped = segment.replace("\\", "\\\\").replace('"', '\\"')
            encoded.append(f'"{escaped}"')
    path = "{" + ",".join(encoded) + "}"
    return "'" + path.replace("'", "''") + "'"


def _parse_jsonb_path_literal(value: str) -> tuple[str, ...]:
    text = str(value or "").strip()
    if len(text) >= 2 and text[0] == "'" and text[-1] == "'":
        text = text[1:-1].replace("''", "'")
    if not (text.startswith("{") and text.endswith("}")):
        raise UnsafeGaussDBSQL("dynamic JSONB path is not allowed")
    body = text[1:-1]
    parts: list[str] = []
    buf: list[str] = []
    quoted = False
    escaped = False
    for char in body:
        if escaped:
            buf.append(char)
            escaped = False
            continue
        if quoted and char == "\\":
            escaped = True
            continue
        if char == '"':
            quoted = not quoted
            continue
        if char == "," and not quoted:
            parts.append("".join(buf))
            buf = []
            continue
        buf.append(char)
    if quoted or escaped:
        raise UnsafeGaussDBSQL("invalid JSONB path literal")
    parts.append("".join(buf))
    if not parts or any(part == "" for part in parts):
        raise UnsafeGaussDBSQL("invalid JSONB path literal")
    return tuple(parts)


class GaussDBSQLValidator:
    PARSE_DIALECT = "postgres"
    FORBIDDEN_FUNCTIONS = {
        "pg_sleep",
        "sleep",
        "now",
        "current_user",
        "current_date",
        "current_time",
        "current_timestamp",
        "current_database",
        "current_catalog",
        "localtime",
        "localtimestamp",
        "session_user",
        "user",
        "version",
        "current_schema",
        "json_extract",
        "json_extract_string",
        "json_extract_isnull",
        "jsonb_each",
        "jsonb_each_text",
        "jsonb_to_record",
        "jsonb_to_recordset",
        "jsonb_array_elements",
        "jsonb_array_elements_text",
    }

    def __init__(
        self,
        tables: dict[str, ExposedGaussDBTable] | set[str] | list[str] | tuple[str, ...] | None = None,
        kb_ids: list[str] | tuple[str, ...] | None = None,
        default_limit: int = 128,
        readonly_only: bool = False,
        runtime_readonly_guard: bool = False,
        execution_schema: str | None = None,
    ):
        if tables is None:
            self.tables = {}
        elif isinstance(tables, dict):
            self.tables = dict(tables)
        else:
            self.tables = {str(table): ExposedGaussDBTable.from_field_map(str(table), kb_ids or (), {}) for table in tables}
        self.default_limit = int(default_limit or 128)
        self.readonly_only = readonly_only
        self.runtime_readonly_guard = runtime_readonly_guard
        self.execution_schema = str(execution_schema).strip() if execution_schema else None

    @classmethod
    def readonly_guard(cls, default_limit: int = 128, execution_schema: str | None = None):
        return cls(
            default_limit=default_limit,
            readonly_only=True,
            runtime_readonly_guard=True,
            execution_schema=execution_schema,
        )

    def validate_and_patch(self, raw_sql: str) -> ValidatedGaussDBSQL:
        sql = self.normalize_sql(raw_sql)
        ast = self._parse_one(sql)
        self._validate_readonly_ast(ast)
        if self.runtime_readonly_guard:
            self._validate_runtime_readonly_context(ast)
        if not self.readonly_only:
            self._validate_tables(ast)
            self._validate_jsonb_paths(ast)
            self._validate_columns(ast)
            sql = self._enforce_kb_boundary(sql)
            ast = self._parse_one(sql)
            self._validate_tables(ast)
            self._validate_jsonb_paths(ast)
            self._validate_columns(ast)
        sql = self._enforce_limit(sql)
        ast = self._parse_one(sql)
        self._validate_readonly_ast(ast)
        if self.runtime_readonly_guard:
            self._validate_runtime_readonly_context(ast)
        if not self.readonly_only:
            self._validate_tables(ast)
            self._validate_jsonb_paths(ast)
            self._validate_columns(ast)
        if self.execution_schema:
            self._qualify_execution_tables(ast)
            sql = ast.sql(dialect=self.PARSE_DIALECT)
        return ValidatedGaussDBSQL(
            sql=sql,
            columns=self._select_columns(ast),
            is_aggregation=is_gaussdb_aggregate_sql(sql),
        )

    def normalize_sql(self, raw_sql: str) -> str:
        sql = str(raw_sql or "").strip()
        sql = re.sub(r"</think>\s*.*?\s*", "", sql, flags=re.DOTALL)
        sql = re.sub(r"```(?:sql)?\s*", "", sql, flags=re.IGNORECASE)
        sql = re.sub(r"```\s*$", "", sql, flags=re.IGNORECASE)
        sql = sql.strip().strip("`").strip().rstrip(";").strip()
        if not sql:
            raise UnsafeGaussDBSQL("empty SQL")
        if ";" in sql:
            raise UnsafeGaussDBSQL("multiple statements are not allowed")
        return sql

    def _parse_one(self, sql: str):
        exp, expressions = _parse_gaussdb_statements(sql, self.PARSE_DIALECT)
        if len(expressions) != 1:
            raise UnsafeGaussDBSQL("exactly one SQL statement is allowed")
        if not isinstance(expressions[0], exp.Select):
            raise UnsafeGaussDBSQL("only SELECT statements are allowed")
        return expressions[0]

    def _validate_readonly_ast(self, ast) -> None:
        from sqlglot import exp

        forbidden_types = (
            exp.Delete,
            exp.Update,
            exp.Insert,
            exp.Create,
            exp.Drop,
            exp.Command,
        )
        if any(isinstance(node, forbidden_types) for node in ast.walk()):
            raise UnsafeGaussDBSQL("SQL contains a non-read-only expression")
        if any(isinstance(node, exp.Lateral) for node in ast.walk()):
            raise UnsafeGaussDBSQL("LATERAL is not allowed")
        self._validate_limit_clauses(ast)
        self._validate_jsonb_empty_string_comparisons(ast)
        forbidden_system_types = tuple(
            cls
            for cls in (
                getattr(exp, "CurrentUser", None),
                getattr(exp, "CurrentDate", None),
                getattr(exp, "CurrentTime", None),
                getattr(exp, "CurrentTimestamp", None),
                getattr(exp, "CurrentDatabase", None),
                getattr(exp, "CurrentCatalog", None),
                getattr(exp, "CurrentSchema", None),
            )
            if cls
        )
        for node in ast.walk():
            if isinstance(node, exp.Star) and not isinstance(getattr(node, "parent", None), exp.Count):
                raise UnsafeGaussDBSQL("SELECT * is not allowed")
            if isinstance(node, exp.Window):
                raise UnsafeGaussDBSQL("window functions are not allowed")
            if isinstance(node, forbidden_system_types):
                raise UnsafeGaussDBSQL("system functions are not allowed")
        allowed_func_types = tuple(
            cls
            for cls in (
                exp.Count,
                exp.Sum,
                exp.Avg,
                exp.Max,
                exp.Min,
                exp.Cast,
                getattr(exp, "StrToDate", None),
                getattr(exp, "JSONBExtractScalar", None),
                getattr(exp, "JSONBExtract", None),
            )
            if cls
        )
        for node in ast.find_all(exp.Func):
            if isinstance(node, (exp.Binary, exp.Connector, exp.Predicate)) and not isinstance(node, allowed_func_types):
                continue
            name = str(getattr(node, "name", "") or "").lower()
            if not name:
                sql_name = str(node.sql_name() if hasattr(node, "sql_name") else "").lower()
                name = sql_name.removeprefix("exploding_") or node.__class__.__name__.lower()
            if isinstance(node, forbidden_system_types) or name in self.FORBIDDEN_FUNCTIONS:
                raise UnsafeGaussDBSQL(f"function {name} is not allowed")
            if not isinstance(node, allowed_func_types):
                raise UnsafeGaussDBSQL(f"function {name} is not allowed")

    def _validate_limit_clauses(self, ast) -> None:
        from sqlglot import exp

        for node in ast.walk():
            if isinstance(node, exp.Fetch):
                expression = node.args.get("count")
            elif isinstance(node, exp.Limit):
                expression = node.args.get("expression")
            else:
                continue
            self._positive_static_limit_value(expression)

    @staticmethod
    def _positive_static_limit_value(expression) -> int:
        from sqlglot import exp

        if not isinstance(expression, exp.Literal) or expression.is_string or not re.fullmatch(r"[0-9]+", str(expression.this)) or int(expression.this) <= 0:
            raise UnsafeGaussDBSQL("LIMIT must be a positive static integer")
        return int(expression.this)

    def _validate_jsonb_empty_string_comparisons(self, ast) -> None:
        from sqlglot import exp

        jsonb_text_types = (exp.JSONBExtractScalar,)

        def is_empty_string(expression) -> bool:
            while isinstance(expression, (exp.Cast, exp.Paren)):
                expression = expression.this
            return isinstance(expression, exp.Literal) and expression.is_string and expression.this == ""

        def contains_jsonb_text(expression) -> bool:
            return any(isinstance(node, jsonb_text_types) for node in expression.walk())

        comparison_types = tuple(cls for cls in (exp.EQ, exp.NEQ, getattr(exp, "NullSafeEQ", None), getattr(exp, "NullSafeNEQ", None)) if cls)
        for comparison in ast.walk():
            if not isinstance(comparison, comparison_types):
                continue
            left, right = comparison.this, comparison.expression
            if (is_empty_string(left) and contains_jsonb_text(right)) or (is_empty_string(right) and contains_jsonb_text(left)):
                raise UnsafeGaussDBSQL("JSONB text cannot be compared with an empty SQL string")

    def _validate_runtime_readonly_context(self, ast) -> None:
        base_tables = self._base_tables(ast)
        if not base_tables:
            raise UnsafeGaussDBSQL("SQL must read from a DocEngine table")
        for table in base_tables:
            if not re.fullmatch(r"ragflow_[A-Za-z0-9_]{1,56}", table):
                raise UnsafeGaussDBSQL(f"table {table} is not allowed")
        if self._has_complex_boundary(ast.sql(dialect=self.PARSE_DIALECT)):
            raise UnsafeGaussDBSQL("complex SQL must use a simpler single-table kb_id boundary")
        for select in self._selects_with_base_tables(ast):
            if not self._select_has_static_kb_boundary(select):
                raise UnsafeGaussDBSQL("each base table scope must include a static kb_id boundary")
        self._validate_runtime_columns(ast)
        self._validate_jsonb_paths(ast)

    def _validate_runtime_columns(self, ast) -> None:
        from sqlglot import exp

        allowed_columns = {"doc_id", "docnm_kwd", "kb_id", "chunk_data"}
        cte_outputs = self._cte_output_columns(ast)
        select_alias_refs = self._select_alias_reference_columns(ast)
        for column in ast.find_all(exp.Column):
            name = column.name
            if name in allowed_columns:
                continue
            if self._is_cte_output_column(column, cte_outputs):
                continue
            if id(column) in select_alias_refs:
                continue
            raise UnsafeGaussDBSQL(f"column {name} is not allowed")

    def _cte_names(self, ast) -> set[str]:
        from sqlglot import exp

        return {cte.alias_or_name for cte in ast.find_all(exp.CTE) if cte.alias_or_name}

    def _base_tables(self, ast) -> list[str]:
        from sqlglot import exp

        cte_names = self._cte_names(ast)
        tables = []
        for table in ast.find_all(exp.Table):
            if table.db or table.catalog:
                raise UnsafeGaussDBSQL("cross-schema SQL is not allowed")
            name = table.name
            if name and name not in cte_names:
                tables.append(name)
        return tables

    def _qualify_execution_tables(self, ast) -> None:
        from sqlglot import exp

        cte_names = self._cte_names(ast)
        schema = exp.to_identifier(self.execution_schema, quoted=True)
        for table in ast.find_all(exp.Table):
            if table.db or table.catalog or table.name in cte_names:
                continue
            table.set("db", schema.copy())

    def _validate_tables(self, ast) -> None:
        allowed = {table.physical_name for table in self.tables.values()} | set(self.tables)
        for table in self._base_tables(ast):
            if table not in allowed:
                raise UnsafeGaussDBSQL(f"table {table} is not allowed")

    def _validate_columns(self, ast) -> None:
        from sqlglot import exp

        allowed_columns = set()
        for table in self.tables.values():
            allowed_columns.update(table.allowed_columns)
        cte_outputs = self._cte_output_columns(ast)
        select_alias_refs = self._select_alias_reference_columns(ast)
        for column in ast.find_all(exp.Column):
            name = column.name
            if name in allowed_columns:
                continue
            if self._is_cte_output_column(column, cte_outputs):
                continue
            if id(column) in select_alias_refs:
                continue
            raise UnsafeGaussDBSQL(f"column {name} is not allowed")

    def _select_alias_reference_columns(self, ast) -> set[int]:
        from sqlglot import exp

        refs = set()
        for select in ast.find_all(exp.Select):
            aliases = {str(getattr(expression, "alias_or_name", "") or "") for expression in getattr(select, "expressions", []) or [] if getattr(expression, "alias", None)}
            aliases.discard("")
            if not aliases:
                continue
            for clause_name in ("group", "order"):
                clause = select.args.get(clause_name)
                if not clause:
                    continue
                for column in clause.find_all(exp.Column):
                    if not column.table and column.name in aliases:
                        refs.add(id(column))
        return refs

    def _cte_output_columns(self, ast) -> dict[str, set[str]]:
        from sqlglot import exp

        outputs: dict[str, set[str]] = {}
        for cte in ast.find_all(exp.CTE):
            name = cte.alias_or_name
            query = cte.this
            if not name or not isinstance(query, exp.Select):
                continue
            columns = set()
            for expression in query.expressions or []:
                alias = getattr(expression, "alias_or_name", None)
                if alias:
                    columns.add(str(alias))
            outputs[name] = columns
        return outputs

    def _is_cte_output_column(self, column, cte_outputs: dict[str, set[str]]) -> bool:
        from sqlglot import exp

        name = column.name
        table = column.table
        if table:
            return table in cte_outputs and name in cte_outputs[table]
        select = column.parent
        while select is not None and not isinstance(select, exp.Select):
            select = getattr(select, "parent", None)
        if select is None:
            return False
        source_tables = self._direct_source_tables(select)
        if not source_tables or not all(source in cte_outputs for source in source_tables):
            return False
        return any(name in cte_outputs[source] for source in source_tables)

    def _validate_jsonb_paths(self, ast) -> None:
        from sqlglot import exp

        allowed_paths = {path for table in self.tables.values() for path in table.json_fields.values()}
        json_classes = (exp.JSONExtractScalar, exp.JSONExtract)
        jsonb_classes = (exp.JSONBExtractScalar, exp.JSONBExtract)
        allowed_chunk_data_columns = set()
        for node in ast.walk():
            if isinstance(node, json_classes):
                raise UnsafeGaussDBSQL("only GaussDB #> / #>> JSONB operators are allowed")
            if not isinstance(node, jsonb_classes):
                continue
            source = node.this
            if not isinstance(source, exp.Column) or source.name != "chunk_data":
                raise UnsafeGaussDBSQL("only chunk_data JSONB paths are allowed")
            allowed_chunk_data_columns.add(id(source))
            expression = node.expression
            if not isinstance(expression, exp.Literal) or not expression.is_string:
                raise UnsafeGaussDBSQL("dynamic JSONB path is not allowed")
            path = _parse_jsonb_path_literal(expression.this)
            if not self.readonly_only and not allowed_paths:
                raise UnsafeGaussDBSQL(f"JSONB path {path} is not exposed")
            if allowed_paths and path not in allowed_paths:
                raise UnsafeGaussDBSQL(f"JSONB path {path} is not exposed")
        for column in ast.find_all(exp.Column):
            if column.name == "chunk_data" and id(column) not in allowed_chunk_data_columns:
                raise UnsafeGaussDBSQL("chunk_data may only be accessed through #> / #>>")

    def _required_kb_ids(self) -> tuple[str, ...]:
        kb_ids = []
        for table in self.tables.values():
            kb_ids.extend(table.required_kb_ids)
        return tuple(dict.fromkeys(kb_ids))

    def _enforce_kb_boundary(self, sql: str) -> str:
        kb_ids = self._required_kb_ids()
        if not kb_ids:
            raise UnsafeGaussDBSQL("kb_id boundary is required")
        ast = self._parse_one(sql)
        if not self._base_tables(ast):
            raise UnsafeGaussDBSQL("SQL must read from an exposed DocEngine table")
        if self._has_complex_boundary(sql):
            raise UnsafeGaussDBSQL("complex SQL must use a simpler single-table kb_id boundary")
        missing = []
        for select in self._selects_with_base_tables(ast):
            if self._select_has_allowed_kb_boundary(select, kb_ids):
                continue
            missing.append(select)
        if not missing:
            return sql
        if len(missing) == 1 and missing[0] is ast and len(self._selects_with_base_tables(ast)) == 1 and not ast.args.get("with_") and len(set(self._direct_base_tables(ast))) == 1:
            if self._where_mentions_kb_id(ast):
                raise UnsafeGaussDBSQL("kb_id boundary must be a positive top-level predicate")
            return self._insert_condition(sql, self._kb_condition(kb_ids))
        if len(set(self._base_tables(ast))) != 1:
            raise UnsafeGaussDBSQL("SQL must reference exactly one base table to inject kb_id")
        raise UnsafeGaussDBSQL("each base table scope must include a kb_id boundary")

    def _has_complex_boundary(self, sql: str) -> bool:
        from sqlglot import exp

        ast = self._parse_one(sql)
        if any(isinstance(node, exp.Or) for node in ast.walk()):
            return True
        if re.search(r"\b(join|union|intersect|except)\b", sql, flags=re.IGNORECASE):
            return True
        return False

    def _selects_with_base_tables(self, ast) -> list:
        from sqlglot import exp

        return [select for select in ast.find_all(exp.Select) if self._direct_base_tables(select)]

    def _direct_base_tables(self, select) -> list[str]:
        from sqlglot import exp

        cte_names = self._cte_names(select)
        tables = []
        from_expr = select.args.get("from_")
        if from_expr and isinstance(from_expr.this, exp.Table):
            if from_expr.this.name not in cte_names:
                tables.append(from_expr.this.name)
        for join in select.args.get("joins") or []:
            target = join.this
            if isinstance(target, exp.Table) and target.name not in cte_names:
                tables.append(target.name)
        return tables

    def _direct_source_tables(self, select) -> list[str]:
        from sqlglot import exp

        tables = []
        from_expr = select.args.get("from_")
        if from_expr and isinstance(from_expr.this, exp.Table):
            tables.append(from_expr.this.name)
        for join in select.args.get("joins") or []:
            target = join.this
            if isinstance(target, exp.Table):
                tables.append(target.name)
        return tables

    def _select_has_allowed_kb_boundary(self, select, allowed_kb_ids: tuple[str, ...]) -> bool:
        from sqlglot import exp

        where = select.args.get("where")
        if not where:
            return False
        allowed = set(allowed_kb_ids)
        for node in self._top_level_and_predicates(where.this):
            if isinstance(node, exp.EQ):
                values = self._kb_values_from_equality(node)
                if values is None:
                    continue
                if not values or not set(values).issubset(allowed):
                    raise UnsafeGaussDBSQL("SQL crosses the allowed kb_id boundary")
                return True
            elif isinstance(node, exp.In):
                values = self._kb_values_from_in(node)
                if values is None:
                    continue
                if not values or not set(values).issubset(allowed):
                    raise UnsafeGaussDBSQL("SQL crosses the allowed kb_id boundary")
                return True
        return False

    def _select_has_static_kb_boundary(self, select) -> bool:
        from sqlglot import exp

        where = select.args.get("where")
        if not where:
            return False
        for node in self._top_level_and_predicates(where.this):
            if isinstance(node, exp.EQ):
                values = self._kb_values_from_equality(node)
            elif isinstance(node, exp.In):
                values = self._kb_values_from_in(node)
            else:
                values = None
            if values is None:
                continue
            if not values:
                raise UnsafeGaussDBSQL("kb_id boundary is empty")
            self._validate_literal_values(values, "kb_id")
            return True
        return False

    def _top_level_and_predicates(self, node) -> list:
        from sqlglot import exp

        if isinstance(node, exp.And):
            return [*self._top_level_and_predicates(node.this), *self._top_level_and_predicates(node.expression)]
        return [node]

    def _where_mentions_kb_id(self, select) -> bool:
        from sqlglot import exp

        where = select.args.get("where")
        if not where:
            return False
        return any(self._is_kb_column(column) for column in where.find_all(exp.Column))

    def _kb_values_from_equality(self, node) -> list[str] | None:
        from sqlglot import exp

        left, right = node.this, node.expression
        if self._is_kb_column(left) and isinstance(right, exp.Literal) and right.is_string:
            return [str(right.this)]
        if self._is_kb_column(right) and isinstance(left, exp.Literal) and left.is_string:
            return [str(left.this)]
        return None

    def _kb_values_from_in(self, node) -> list[str] | None:
        from sqlglot import exp

        if not self._is_kb_column(node.this):
            return None
        values = []
        for item in node.expressions or []:
            if not isinstance(item, exp.Literal) or not item.is_string:
                raise UnsafeGaussDBSQL("kb_id IN must use static string literals")
            values.append(str(item.this))
        return values

    def _is_kb_column(self, node) -> bool:
        from sqlglot import exp

        return isinstance(node, exp.Column) and node.name.lower() == "kb_id"

    def _kb_condition(self, kb_ids: tuple[str, ...]) -> str:
        self._validate_literal_values(kb_ids, "kb_id")
        if len(kb_ids) == 1:
            return f"kb_id = '{kb_ids[0]}'"
        return "kb_id IN (" + ", ".join(f"'{kid}'" for kid in kb_ids) + ")"

    def _validate_literal_values(self, values: tuple[str, ...] | list[str], label: str) -> None:
        for value in values:
            if not re.fullmatch(r"[A-Za-z0-9_.:-]{1,256}", str(value)):
                raise UnsafeGaussDBSQL(f"unsafe {label} literal")

    def _insert_condition(self, sql: str, condition: str) -> str:
        from sqlglot import exp

        ast = self._parse_one(sql)
        condition_ast = self._parse_one(f"SELECT 1 WHERE {condition}").args["where"].this
        where = ast.args.get("where")
        if where:
            ast.set("where", exp.Where(this=exp.and_(where.this, condition_ast)))
        else:
            ast.set("where", exp.Where(this=condition_ast))
        return ast.sql(dialect=self.PARSE_DIALECT)

    def _enforce_limit(self, sql: str) -> str:
        from sqlglot import exp

        ast = self._parse_one(sql)
        limit = ast.args.get("limit")
        if not limit:
            if self.default_limit <= 0:
                return sql
            return ast.limit(self.default_limit).sql(dialect=self.PARSE_DIALECT)
        expression_key = "count" if isinstance(limit, exp.Fetch) else "expression"
        current = self._positive_static_limit_value(limit.args.get(expression_key))
        if self.default_limit <= 0 or current <= self.default_limit:
            return sql
        limit.set(expression_key, exp.Literal.number(self.default_limit))
        return ast.sql(dialect=self.PARSE_DIALECT)

    def _select_columns(self, ast) -> list[str]:
        columns = []
        for expression in getattr(ast, "expressions", []) or []:
            alias = getattr(expression, "alias_or_name", None)
            if alias:
                columns.append(str(alias))
        return columns


class GaussDBDDLBuilder:
    MAX_IDENTIFIER_LENGTH = 63
    REGULAR_INDEX_COLUMNS = (
        "doc_id",
        "available_int",
        "knowledge_graph_kwd",
        "entity_type_kwd",
        "removed_kwd",
    )
    FTS_COLUMNS = (
        "title_tks",
        "title_sm_tks",
        "important_tks",
        "question_tks",
        "content_ltks",
        "content_sm_ltks",
    )

    def __init__(self, schema: str):
        self.schema = self.validate_identifier(schema)

    def validate_identifier(self, name: str) -> str:
        if not re.fullmatch(r"(?:[A-Za-z_]|[^\x00-\x7F])(?:[A-Za-z0-9_#$]|[^\x00-\x7F]){0,62}", name or ""):
            raise InvalidGaussDBObjectName(name)
        return name

    def quote_identifier(self, name: str) -> str:
        escaped = self.validate_identifier(name).replace('"', '""')
        return f'"{escaped}"'

    def qualified_name(self, table: str) -> str:
        return f"{self.quote_identifier(self.schema)}.{self.quote_identifier(table)}"

    def index_name(self, table: str, suffix: str) -> str:
        name = f"idx_gdb_{self.validate_identifier(table)}_{self.validate_identifier(suffix)}"
        if len(name) <= self.MAX_IDENTIFIER_LENGTH:
            return name
        digest = hashlib.sha1(name.encode("utf-8")).hexdigest()[:10]
        prefix_len = self.MAX_IDENTIFIER_LENGTH - len(digest) - 1
        return f"{name[:prefix_len]}_{digest}"

    def build_chunk_table_ddl(self, table: str) -> str:
        name = self.qualified_name(table)
        return f"""CREATE TABLE IF NOT EXISTS {name} (
  id VARCHAR(256) NOT NULL,
  kb_id VARCHAR(256) NOT NULL,
  doc_id VARCHAR(256),
  docnm_kwd VARCHAR(256),
  doc_type_kwd VARCHAR(256),
  title_tks VARCHAR(256),
  title_sm_tks VARCHAR(256),
  content_with_weight TEXT,
  content_ltks TEXT,
  content_sm_ltks TEXT,
  important_kwd JSONB,
  important_tks TEXT,
  question_kwd JSONB,
  question_tks TEXT,
  tag_kwd JSONB,
  tag_feas JSONB,
  available_int INTEGER DEFAULT 1 NOT NULL,
  pagerank_fea INTEGER,
  create_time VARCHAR(19),
  create_timestamp_flt DOUBLE PRECISION,
  img_id VARCHAR(128),
  position_int JSONB,
  page_num_int JSONB,
  top_int JSONB,
  metadata JSONB,
  chunk_data JSONB,
  extra JSONB,
  _order_id INTEGER,
  group_id VARCHAR(256),
  mom_id VARCHAR(256),
  knowledge_graph_kwd VARCHAR(256),
  source_id JSONB,
  entity_kwd VARCHAR(256),
  entity_type_kwd VARCHAR(256),
  from_entity_kwd VARCHAR(256),
  to_entity_kwd VARCHAR(256),
  weight_int INTEGER,
  weight_flt DOUBLE PRECISION,
  entities_kwd JSONB,
  rank_flt DOUBLE PRECISION,
  n_hop_with_weight TEXT,
  removed_kwd VARCHAR(256) DEFAULT 'N',
  raptor_kwd VARCHAR(256),
  raptor_layer_int INTEGER,
  PRIMARY KEY (kb_id, id)
) WITH (storage_type=USTORE)"""

    def build_doc_meta_table_ddls(self, meta_table: str) -> list[str]:
        name = self.qualified_name(meta_table)
        idx = self.quote_identifier(self.index_name(meta_table, "kb_id"))
        return [
            f"""CREATE TABLE IF NOT EXISTS {name} (
  id VARCHAR(256) NOT NULL,
  kb_id VARCHAR(256) NOT NULL,
  meta_fields JSONB,
  PRIMARY KEY (id)
) WITH (storage_type=USTORE)""",
            f"CREATE INDEX IF NOT EXISTS {idx} ON {name} (kb_id)",
        ]

    def build_regular_index_ddls(self, table: str) -> list[str]:
        name = self.qualified_name(table)
        return [f"CREATE INDEX IF NOT EXISTS {self.quote_identifier(self.index_name(table, column))} ON {name} ({column})" for column in self.REGULAR_INDEX_COLUMNS]

    def build_fulltext_ugin_ddl(self, table: str) -> str:
        name = self.qualified_name(table)
        idx = self.quote_identifier(self.index_name(table, "fts_all"))
        expression = " || ' ' || ".join(f"coalesce({column}, ' ')" for column in self.FTS_COLUMNS)
        return f"""CREATE INDEX IF NOT EXISTS {idx}
  ON {name}
  USING ugin(to_tsvector('simple', {expression}))"""

    def build_ngram_fulltext_ugin_ddl(self, table: str) -> str:
        name = self.qualified_name(table)
        idx = self.quote_identifier(self.index_name(table, "fts_all_ngram"))
        expression = " || ' ' || ".join(f"coalesce({column}, ' ')" for column in self.FTS_COLUMNS)
        return f"""CREATE INDEX IF NOT EXISTS {idx}
  ON {name}
  USING ugin(to_tsvector('ngram', {expression}))"""

    def build_vector_column_ddls(self, table: str, dim: int) -> list[str]:
        dim = self.validate_vector_dim(dim)
        name = self.qualified_name(table)
        vector_col = self.vector_column_name(dim)
        valid_col = self.vector_valid_column_name(dim)
        return [
            f"ALTER TABLE {name} ADD COLUMN IF NOT EXISTS {vector_col} floatvector({dim}) DEFAULT (array_fill(0, ARRAY[{dim}])::text::floatvector({dim}))",
            f"ALTER TABLE {name} ADD COLUMN IF NOT EXISTS {valid_col} BOOLEAN DEFAULT FALSE NOT NULL",
        ]

    def build_diskann_index_ddl(self, table: str, dim: int) -> str:
        dim = self.validate_vector_dim(dim)
        name = self.qualified_name(table)
        vector_col = self.vector_column_name(dim)
        idx = self.quote_identifier(self.index_name(table, f"{vector_col}_diskann"))
        options = "subgraph_count=1"
        if dim > 1024:
            options += ", enable_vector_copy=false"
        return f"CREATE INDEX IF NOT EXISTS {idx} ON {name} USING gsdiskann ({vector_col} COSINE) WITH ({options})"

    def build_advisory_lock_sql(self, lock_name: str) -> tuple[str, list[str]]:
        return ("SELECT pg_advisory_xact_lock(hashtext(%s))", [str(lock_name)])

    def validate_vector_dim(self, dim: int) -> int:
        if isinstance(dim, bool) or not isinstance(dim, Integral):
            raise ValueError("vector dimension must be an integer")
        value = int(dim)
        if value <= 0:
            raise ValueError("vector dimension must be positive")
        if value > 4096:
            raise ValueError("GaussDB floatvector dimensions cannot exceed 4096")
        return value

    def vector_column_name(self, dim: int) -> str:
        return f"q_{self.validate_vector_dim(dim)}_vec"

    def vector_valid_column_name(self, dim: int) -> str:
        return f"q_{self.validate_vector_dim(dim)}_vec_valid"


class GaussDBSearchBuilder:
    CHUNK_COLUMNS = {
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
        "chunk_order_int",
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
        "compile_kwd",
        "row_id()",
    }
    JSONB_MULTI_VALUE_COLUMNS = {"important_kwd", "question_kwd", "tag_kwd", "source_id", "entities_kwd"}
    JSONB_ARRAY_AGG_COLUMNS = JSONB_MULTI_VALUE_COLUMNS | {"entities_kwd"}
    JSONB_EXTRA_SCALAR_COLUMNS = {"compile_kwd"}
    COLUMN_ALIASES = {"chunk_order_int": "_order_id"}
    FTS_WEIGHTS = {
        "title_tks": 10.0,
        "title_sm_tks": 5.0,
        "important_tks": 20.0,
        "question_tks": 20.0,
        "content_ltks": 2.0,
        "content_sm_ltks": 1.0,
    }
    VECTOR_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec$")
    VECTOR_VALID_COLUMN_RE = re.compile(r"^q_(?P<dim>\d+)_vec_valid$")
    CJK_RUN_RE = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]+")

    def __init__(self, schema: str):
        self.ddl = GaussDBDDLBuilder(schema=schema)

    def build_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        keywords: list[str] | None,
        vector: list[float] | None,
        vector_dim: int | None,
        vector_weight: float,
        offset: int,
        limit: int,
        similarity_threshold: float | None = None,
        minimum_should_match: float | str | None = None,
        topn: int | None = None,
        highlight_fields: list[str] | None = None,
        order_by: OrderByExpr | None = None,
        pagerank_weight: float = 0.0,
    ) -> tuple[str, list[Any]]:
        query_keywords = [str(keyword).strip() for keyword in keywords or [] if str(keyword).strip()]
        has_searchable_text = any(self.split_text_query_terms(query_keywords))
        query_vector = self._vector_param(vector, vector_dim) if vector is not None else None
        effective_limit = max(int(limit or 0), 0)
        effective_offset = max(int(offset or 0), 0)
        page_limit = effective_limit if effective_limit > 0 else max(int(topn or 0), 10000)
        candidate_limit = self._candidate_limit(page_limit, effective_offset, topn)

        if has_searchable_text and query_vector is not None:
            return self._build_hybrid_search_sql(
                table=table,
                select_fields=select_fields,
                condition=condition,
                keywords=query_keywords,
                vector=query_vector,
                vector_dim=vector_dim,
                vector_weight=vector_weight,
                similarity_threshold=similarity_threshold,
                minimum_should_match=minimum_should_match,
                candidate_limit=candidate_limit,
                offset=effective_offset,
                limit=page_limit,
                highlight_fields=highlight_fields,
                pagerank_weight=pagerank_weight,
            )
        if query_vector is not None:
            return self._build_vector_search_sql(
                table=table,
                select_fields=select_fields,
                condition=condition,
                vector=query_vector,
                vector_dim=vector_dim,
                similarity_threshold=similarity_threshold,
                candidate_limit=candidate_limit,
                offset=effective_offset,
                limit=page_limit,
                pagerank_weight=pagerank_weight,
            )
        if query_keywords:
            return self._build_fulltext_search_sql(
                table=table,
                select_fields=select_fields,
                condition=condition,
                keywords=query_keywords,
                minimum_should_match=minimum_should_match,
                offset=effective_offset,
                limit=page_limit,
                highlight_fields=highlight_fields,
                pagerank_weight=pagerank_weight,
            )
        return self._build_filter_search_sql(
            table=table,
            select_fields=select_fields,
            condition=condition,
            offset=effective_offset,
            limit=page_limit,
            order_by=order_by,
            pagerank_weight=pagerank_weight,
        )

    def build_condition_where(self, condition: dict | None) -> tuple[str, list[Any]]:
        fragments: list[str] = []
        params: list[Any] = []
        for key, value in (condition or {}).items():
            if key == "exists":
                column = self.validate_column(value)
                expression = self._column_expr(column)
                if self._is_jsonb_dynamic_column(column):
                    fragments.append(f"({expression} IS NOT NULL AND {expression} <> 'null'::jsonb)")
                else:
                    fragments.append(f"{expression} IS NOT NULL")
                continue
            if key == "must_not" and isinstance(value, dict) and "exists" in value:
                column = self.validate_column(value["exists"])
                expression = self._column_expr(column)
                if self._is_jsonb_dynamic_column(column):
                    fragments.append(f"({expression} IS NULL OR {expression} = 'null'::jsonb)")
                else:
                    fragments.append(f"{expression} IS NULL")
                continue

            column = self.validate_column(key)
            expression = self._column_expr(column)
            if self._is_jsonb_dynamic_column(column):
                values = self._list_values(value)
                predicates = []
                for item in values:
                    predicates.append(f"({expression} = %s::jsonb OR {expression} @> %s::jsonb)")
                    params.extend(
                        [
                            json.dumps(item, ensure_ascii=False),
                            json.dumps([item], ensure_ascii=False),
                        ]
                    )
                fragments.append("(" + " OR ".join(predicates) + ")")
                continue
            storage_column = self._storage_column(column)
            if storage_column in self.JSONB_MULTI_VALUE_COLUMNS:
                values = self._list_values(value)
                fragments.append("(" + " OR ".join([f"{storage_column} @> %s::jsonb"] * len(values)) + ")")
                params.extend(json.dumps([item], ensure_ascii=False) for item in values)
                continue
            if isinstance(value, (list, tuple, set)):
                values = self._list_values(value)
                fragments.append(f"{expression} IN ({', '.join(['%s'] * len(values))})")
                params.extend(values)
                continue
            if value is None:
                fragments.append(f"{expression} IS NULL")
                continue
            fragments.append(f"{expression} = %s")
            params.append(value)
        return " AND ".join(fragments), params

    def build_text_score_expr(
        self,
        keywords: list[str],
        minimum_should_match: float | str | None = None,
    ) -> tuple[str, list[Any]]:
        score_exprs: list[str] = []
        params: list[Any] = []
        text_queries = self._text_queries(keywords) if minimum_should_match is None else self._text_query_terms(keywords)
        for config, query_text in text_queries:
            weighted_score = " + ".join(
                f"{weight} * COALESCE(ts_rank(to_tsvector('{config}', coalesce({column}, ' ')), plainto_tsquery('{config}', %s)), 0)" for column, weight in self.FTS_WEIGHTS.items()
            )
            score_exprs.append(f"({weighted_score})")
            params.extend([query_text] * len(self.FTS_WEIGHTS))
        if not score_exprs:
            return "0.0", []
        if len(score_exprs) == 1:
            return score_exprs[0], params
        return f"(({' + '.join(score_exprs)}) / {len(score_exprs)}.0)", params

    def build_highlight_expr(self, field_name: str, keywords: list[str]) -> tuple[str, list[Any]]:
        field = self.validate_column(field_name)
        expression = self._column_expr(field, text=self._is_dynamic_column(field))
        _simple_terms, ngram_terms = self.split_text_query_terms(keywords)
        if ngram_terms:
            return f"COALESCE({expression}, ' ') AS _highlight_source", []
        return (
            f"ts_headline('simple', COALESCE({expression}, ' '), plainto_tsquery('simple', %s), 'StartSel=<em>, StopSel=</em>') AS _highlight",
            [self._text_query_param(keywords)],
        )

    def build_aggregation_sql(
        self,
        table: str,
        field_name: str,
        condition: dict | None,
        limit: int = 1000,
    ) -> tuple[str, list[Any]]:
        table_name = self.ddl.qualified_name(table)
        field = self.validate_column(field_name)
        field_expression = self._column_expr(field)
        where_sql, where_params = self.build_condition_where(condition)
        where_clause = where_sql or "TRUE"
        if field in self.JSONB_ARRAY_AGG_COLUMNS:
            sql = (
                "SELECT value, COUNT(1) AS count "
                "FROM ("
                f"SELECT jsonb_array_elements_text(COALESCE({field_expression}, '[]'::jsonb)) AS value "
                f"FROM {table_name} WHERE {where_clause}"
                ") AS expanded "
                "GROUP BY value ORDER BY count DESC, value ASC LIMIT %s"
            )
        else:
            sql = (
                f"SELECT {field_expression} AS value, COUNT(1) AS count "
                f"FROM {table_name} "
                f"WHERE {where_clause} AND {field_expression} IS NOT NULL "
                "GROUP BY value ORDER BY count DESC, value ASC LIMIT %s"
            )
        return sql, [*where_params, int(limit)]

    def build_position_order_sql(self) -> str:
        return "COALESCE((page_num_int #>> '{0}')::int, 100000000) ASC, COALESCE((position_int #>> '{0,3}')::int, 100000000) ASC, COALESCE((top_int #>> '{0}')::int, 100000000) ASC"

    def build_fts_vector_expr(self, config: str = "simple") -> str:
        expression = " || ' ' || ".join(f"coalesce({column}, ' ')" for column in self.FTS_WEIGHTS)
        return f"to_tsvector('{config}', {expression})"

    def validate_column(self, column: str) -> str:
        if column == "row_id()":
            return column
        column = str(column or "")
        if column in self.CHUNK_COLUMNS or self.VECTOR_COLUMN_RE.fullmatch(column) or self.VECTOR_VALID_COLUMN_RE.fullmatch(column):
            return column
        return validate_extra_field(column)

    def normalize_select_fields(self, fields: list[str] | None) -> list[str]:
        if not fields or "*" in fields:
            return ["id", "kb_id"]
        normalized: list[str] = ["id", "kb_id"]
        for field in fields:
            if field == "_score":
                continue
            column = self.validate_column(field)
            if column not in normalized:
                normalized.append(column)
            match = self.VECTOR_COLUMN_RE.fullmatch(column)
            if match:
                valid_column = self.ddl.vector_valid_column_name(int(match.group("dim")))
                if valid_column not in normalized:
                    normalized.append(valid_column)
        return normalized

    def _build_filter_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        offset: int,
        limit: int,
        order_by: OrderByExpr | None,
        pagerank_weight: float,
    ) -> tuple[str, list[Any]]:
        table_name = self.ddl.qualified_name(table)
        columns = self.normalize_select_fields(select_fields)
        where_sql, where_params = self.build_condition_where(condition)
        order_sql = self._build_order_by(order_by) or "kb_id ASC, id ASC"
        score_expr, score_params = self._score_with_pagerank("0.0", pagerank_weight)
        sql = f"SELECT {', '.join(self._select_exprs(columns))}, {score_expr} AS _score, COUNT(*) OVER() AS __total FROM {table_name}"
        if where_sql:
            sql += f" WHERE {where_sql}"
        sql += f" ORDER BY {order_sql} LIMIT %s OFFSET %s"
        return sql, [*score_params, *where_params, limit, offset]

    def _build_fulltext_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        keywords: list[str],
        minimum_should_match: float | str | None,
        offset: int,
        limit: int,
        highlight_fields: list[str] | None,
        pagerank_weight: float,
    ) -> tuple[str, list[Any]]:
        table_name = self.ddl.qualified_name(table)
        columns = self.normalize_select_fields(select_fields)
        score_expr, score_params = self.build_text_score_expr(keywords, minimum_should_match)
        score_expr, pagerank_params = self._score_with_pagerank(score_expr, pagerank_weight)
        match_expr, match_params = self._build_text_match_expr(keywords, minimum_should_match)
        where_sql, where_params = self.build_condition_where(condition)
        where_parts = [part for part in (where_sql, match_expr) if part]
        select_exprs = [*self._select_exprs(columns), f"{score_expr} AS _score", "COUNT(*) OVER() AS __total"]
        highlight_params: list[Any] = []
        if highlight_fields:
            highlight_expr, highlight_params = self.build_highlight_expr(highlight_fields[0], keywords)
            select_exprs.append(highlight_expr)
        sql = f"SELECT {', '.join(select_exprs)} FROM {table_name}"
        if where_parts:
            sql += f" WHERE {' AND '.join(where_parts)}"
        sql += " ORDER BY _score DESC, kb_id ASC, id ASC LIMIT %s OFFSET %s"
        return sql, [*score_params, *pagerank_params, *highlight_params, *where_params, *match_params, limit, offset]

    def _build_vector_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        vector: str,
        vector_dim: int,
        similarity_threshold: float | None,
        candidate_limit: int,
        offset: int,
        limit: int,
        pagerank_weight: float,
    ) -> tuple[str, list[Any]]:
        table_name = self.ddl.qualified_name(table)
        columns = self.normalize_select_fields(select_fields)
        dim = self.ddl.validate_vector_dim(vector_dim)
        vector_col = self.ddl.vector_column_name(dim)
        valid_col = self.ddl.vector_valid_column_name(dim)
        where_sql, where_params = self.build_condition_where(condition)
        where_parts = [part for part in (where_sql, f"{valid_col} = TRUE") if part]
        threshold = 0.0 if similarity_threshold is None else float(similarity_threshold)
        score_expr, score_params = self._score_with_pagerank(f"1 - ({vector_col} <+> %s::floatvector({dim}))", pagerank_weight)
        sql = (
            "WITH vec AS ("
            f" SELECT {', '.join(self._select_exprs(columns))}, "
            f"{vector_col} <+> %s::floatvector({dim}) AS distance, "
            f"{score_expr} AS _score "
            f"FROM {table_name} "
            f"WHERE {' AND '.join(where_parts)} "
            f"ORDER BY {vector_col} <+> %s::floatvector({dim}) ASC "
            "LIMIT %s"
            ") "
            "SELECT vec.*, COUNT(*) OVER() AS __total FROM vec "
            "WHERE _score >= %s "
            "ORDER BY distance ASC, kb_id ASC, id ASC LIMIT %s OFFSET %s"
        )
        return sql, [vector, vector, *score_params, *where_params, vector, candidate_limit, threshold, limit, offset]

    def _build_hybrid_search_sql(
        self,
        table: str,
        select_fields: list[str],
        condition: dict,
        keywords: list[str],
        vector: str,
        vector_dim: int,
        vector_weight: float,
        similarity_threshold: float | None,
        minimum_should_match: float | str | None,
        candidate_limit: int,
        offset: int,
        limit: int,
        highlight_fields: list[str] | None,
        pagerank_weight: float,
    ) -> tuple[str, list[Any]]:
        table_name = self.ddl.qualified_name(table)
        columns = self.normalize_select_fields(select_fields)
        joined_columns = ", ".join(self._select_exprs(columns, prefix="c"))
        dim = self.ddl.validate_vector_dim(vector_dim)
        vector_col = self.ddl.vector_column_name(dim)
        valid_col = self.ddl.vector_valid_column_name(dim)
        text_score_expr, text_score_params = self.build_text_score_expr(keywords, minimum_should_match)
        match_expr, match_params = self._build_text_match_expr(keywords, minimum_should_match)
        where_sql, where_params = self.build_condition_where(condition)
        base_where = where_sql or "TRUE"
        fts_where = " AND ".join([base_where, match_expr])
        vector_where = " AND ".join([base_where, f"{valid_col} = TRUE"])
        threshold = 0.0 if similarity_threshold is None else float(similarity_threshold)
        select_exprs = [joined_columns, "merged.score AS _score", "COUNT(*) OVER() AS __total"]
        highlight_params: list[Any] = []
        if highlight_fields:
            highlight_expr, highlight_params = self.build_highlight_expr(highlight_fields[0], keywords)
            select_exprs.append(highlight_expr)
        final_score_expr, final_score_params = self._score_with_pagerank("merged.score", pagerank_weight, table_alias="c")
        select_exprs[1] = f"{final_score_expr} AS _score"
        threshold_expr, threshold_score_params = self._score_with_pagerank("merged.score", pagerank_weight, table_alias="c")
        sql = (
            "WITH fts_raw AS ("
            f" SELECT kb_id, id, {text_score_expr} AS raw_fts_score "
            f"FROM {table_name} WHERE {fts_where} "
            "ORDER BY raw_fts_score DESC, kb_id ASC, id ASC LIMIT %s"
            "), fts AS ("
            " SELECT kb_id, id, COALESCE(raw_fts_score / NULLIF(MAX(raw_fts_score) OVER (), 0), 0) AS fts_score "
            "FROM fts_raw"
            "), vec AS ("
            f" SELECT kb_id, id, 1 - ({vector_col} <+> %s::floatvector({dim})) AS vector_score "
            f"FROM {table_name} WHERE {vector_where} "
            f"ORDER BY {vector_col} <+> %s::floatvector({dim}) ASC LIMIT %s"
            "), merged AS ("
            " SELECT COALESCE(fts.kb_id, vec.kb_id) AS kb_id, "
            "COALESCE(fts.id, vec.id) AS id, "
            "(1 - %s) * COALESCE(fts.fts_score, 0) + %s * COALESCE(vec.vector_score, 0) AS score "
            "FROM fts FULL OUTER JOIN vec ON fts.kb_id = vec.kb_id AND fts.id = vec.id"
            ") "
            f"SELECT {', '.join(select_exprs)} "
            f"FROM merged JOIN {table_name} c ON c.kb_id = merged.kb_id AND c.id = merged.id "
            f"WHERE {threshold_expr} >= %s "
            "ORDER BY _score DESC, merged.kb_id ASC, merged.id ASC LIMIT %s OFFSET %s"
        )
        return sql, [
            *text_score_params,
            *where_params,
            *match_params,
            candidate_limit,
            vector,
            *where_params,
            vector,
            candidate_limit,
            float(vector_weight),
            float(vector_weight),
            *final_score_params,
            *highlight_params,
            *threshold_score_params,
            threshold,
            limit,
            offset,
        ]

    def _build_text_match_expr(
        self,
        keywords: list[str],
        minimum_should_match: float | str | None = None,
    ) -> tuple[str, list[Any]]:
        if minimum_should_match is not None:
            text_query_terms = self._text_query_terms(keywords)
            predicates = [f"{self.build_fts_vector_expr(config)} @@ plainto_tsquery('{config}', %s)" for config, _query_text in text_query_terms]
            params = [query_text for _config, query_text in text_query_terms]
            required = self._minimum_should_match_count(minimum_should_match, len(predicates))
            if not predicates:
                return "FALSE", []
            if required <= 1:
                return "(" + " OR ".join(predicates) + ")", params
            if required >= len(predicates):
                return "(" + " AND ".join(predicates) + ")", params
            any_match = " OR ".join(predicates)
            match_count = " + ".join(f"CASE WHEN {predicate} THEN 1 ELSE 0 END" for predicate in predicates)
            return f"(({any_match}) AND ({match_count}) >= {required})", [*params, *params]

        match_exprs: list[str] = []
        params: list[Any] = []
        for config, query_text in self._text_queries(keywords):
            match_exprs.append(f"{self.build_fts_vector_expr(config)} @@ plainto_tsquery('{config}', %s)")
            params.append(query_text)
        return (" AND ".join(match_exprs), params) if match_exprs else ("FALSE", [])

    def _build_order_by(self, order_by: OrderByExpr | None) -> str:
        fields = getattr(order_by, "fields", None) or []
        parts: list[str] = []
        for field, direction in fields:
            column = self.validate_column(field)
            order = "DESC" if direction else "ASC"
            if column in {"page_num_int", "position_int", "top_int"}:
                parts.append(self.build_position_order_sql())
            else:
                parts.append(f"{self._column_expr(column, text=self._is_dynamic_column(column))} {order}")
        return ", ".join(parts)

    def _select_exprs(self, columns: list[str], prefix: str | None = None) -> list[str]:
        expressions = []
        for column in columns:
            if column == "row_id()":
                expressions.append('NULL AS "row_id()"')
            elif self._is_dynamic_column(column):
                expressions.append(f"{self._column_expr(column, prefix=prefix)} AS {column}")
            elif column in self.COLUMN_ALIASES:
                storage_column = self._storage_column(column)
                source = f"{prefix}.{storage_column}" if prefix else storage_column
                expressions.append(f"{source} AS {column}")
            elif prefix:
                expressions.append(f"{prefix}.{column}")
            else:
                expressions.append(column)
        return expressions

    def _is_dynamic_column(self, column: str) -> bool:
        return column in self.JSONB_EXTRA_SCALAR_COLUMNS or not (
            column in self.CHUNK_COLUMNS or column == "row_id()" or self.VECTOR_COLUMN_RE.fullmatch(column) or self.VECTOR_VALID_COLUMN_RE.fullmatch(column)
        )

    def _is_jsonb_dynamic_column(self, column: str) -> bool:
        return self._is_dynamic_column(column) and column not in self.JSONB_EXTRA_SCALAR_COLUMNS

    def _column_expr(
        self,
        column: str,
        *,
        prefix: str | None = None,
        text: bool = False,
    ) -> str:
        if self._is_dynamic_column(column):
            if column in self.JSONB_EXTRA_SCALAR_COLUMNS:
                source = f"{prefix}.extra" if prefix else "extra"
                return f"({source} #>> '{{{column}}}')"
            return extra_field_expr(
                column,
                prefix=prefix,
                text=text,
            )
        storage_column = self._storage_column(column)
        return f"{prefix}.{storage_column}" if prefix else storage_column

    def _storage_column(self, column: str) -> str:
        if column in self.JSONB_EXTRA_SCALAR_COLUMNS:
            return f"(extra #>> '{{{column}}}')"
        return self.COLUMN_ALIASES.get(column, column)

    def _score_with_pagerank(self, score_expr: str, pagerank_weight: float, table_alias: str | None = None) -> tuple[str, list[Any]]:
        weight = float(pagerank_weight or 0.0)
        if weight <= 0.0:
            return score_expr, []
        column = "pagerank_fea" if table_alias is None else f"{table_alias}.pagerank_fea"
        pagerank_expr = f"(COALESCE({column}, 0)::DOUBLE PRECISION / 100.0 * %s)"
        return f"({score_expr} + {pagerank_expr})", [weight]

    def _text_query_param(self, keywords: list[str]) -> str:
        return " ".join(str(keyword).strip() for keyword in keywords if str(keyword).strip())

    @classmethod
    def split_text_query_terms(cls, keywords: list[str]) -> tuple[list[str], list[str]]:
        simple_terms: list[str] = []
        ngram_terms: list[str] = []
        for keyword in keywords:
            text = str(keyword).strip()
            if not text:
                continue
            matches = list(cls.CJK_RUN_RE.finditer(text))
            if not matches:
                simple_terms.append(text)
                continue
            cursor = 0
            for match in matches:
                simple_part = text[cursor : match.start()].strip()
                if simple_part and any(char.isalnum() for char in simple_part):
                    simple_terms.append(simple_part)
                cjk_part = match.group()
                if len(cjk_part) >= 2:
                    ngram_terms.append(cjk_part)
                cursor = match.end()
            simple_part = text[cursor:].strip()
            if simple_part and any(char.isalnum() for char in simple_part):
                simple_terms.append(simple_part)
        return simple_terms, ngram_terms

    def _text_queries(self, keywords: list[str]) -> list[tuple[str, str]]:
        simple_terms, ngram_terms = self.split_text_query_terms(keywords)
        queries = []
        if simple_terms:
            queries.append(("simple", self._text_query_param(simple_terms)))
        if ngram_terms:
            queries.append(("ngram", self._text_query_param(ngram_terms)))
        return queries

    def _text_query_terms(self, keywords: list[str]) -> list[tuple[str, str]]:
        simple_terms, ngram_terms = self.split_text_query_terms(keywords)
        terms: list[tuple[str, str]] = []
        seen: set[tuple[str, str]] = set()
        for config, values in (("simple", simple_terms), ("ngram", ngram_terms)):
            for value in values:
                key = (config, value.casefold())
                if key in seen:
                    continue
                seen.add(key)
                terms.append((config, value))
        return terms

    @staticmethod
    def _minimum_should_match_count(minimum_should_match: float | str, term_count: int) -> int:
        if term_count <= 0:
            return 0

        value: float | str = minimum_should_match
        if isinstance(value, str):
            text = value.strip()
            try:
                if text.endswith("%"):
                    value = float(text[:-1]) / 100.0
                else:
                    value = float(text)
            except ValueError:
                return 1

        if isinstance(value, float) and 0.0 <= value <= 1.0:
            required = int(term_count * value)
        else:
            required = int(value)
        return min(term_count, max(1, required))

    def _vector_param(self, vector: list[float] | tuple[float, ...], vector_dim: int | None) -> str:
        if vector_dim is None:
            raise ValueError("vector_dim is required for vector search")
        dim = self.ddl.validate_vector_dim(vector_dim)
        values = list(vector)
        if len(values) != dim:
            raise ValueError(f"vector dimension mismatch: expected {dim}, got {len(values)}")
        return "[" + ",".join(str(float(value)) for value in values) + "]"

    def _list_values(self, value) -> list[Any]:
        values = list(value) if isinstance(value, (list, tuple, set)) else [value]
        if not values:
            raise ValueError("empty condition values are not supported")
        return values

    def _candidate_limit(self, limit: int, offset: int, topn: int | None) -> int:
        base = max(int(limit or 0) + int(offset or 0), int(limit or 0), 1)
        if topn and int(topn) > 0:
            base = max(base, int(topn))
        return base


class GaussDBConnectionBase(DocStoreConnection):
    def __init__(self, pool: Any | None = None, logger_name: str = "ragflow.gaussdb_conn"):
        if pool is None:
            from common.doc_store.gaussdb_conn_pool import GAUSSDB_CONN

            pool = GAUSSDB_CONN
        self.logger = logging.getLogger(logger_name)
        self.pool = pool
        self.masked_uri = self.pool.masked_uri
        self.resolved_schema = self.pool.resolved_schema
        self.schema = self.resolved_schema
        self.ddl = GaussDBDDLBuilder(schema=self.resolved_schema)
        self.pool.check_schema_access()
        self.logger.info("GaussDB %s connection initialized.", self.masked_uri)

    def db_type(self) -> str:
        return "gaussdb"

    def health(self) -> dict:
        result = {
            "status": "unhealthy",
            "uri": self.masked_uri,
            "version_comment": "unknown",
            "schema": self.resolved_schema,
            "server_encoding": "unknown",
            "client_encoding": "unknown",
        }
        try:
            result["version_comment"] = self._query_version()
            result["sql_compatibility"] = self._query_sql_compatibility()
            if result["sql_compatibility"] not in {"A", "ORA"}:
                result["error"] = f"unsupported GaussDB compatibility, expected A/ORA: sql_compatibility={result['sql_compatibility']}"
                return result
            result["server_encoding"] = self._query_server_encoding()
            result["client_encoding"] = self._query_client_encoding()
            normalized_client_encoding = result["client_encoding"].replace("-", "").replace("_", "")
            if normalized_client_encoding != "UTF8":
                result["error"] = f"unsupported GaussDB client encoding, expected UTF8: client_encoding={result['client_encoding']}"
                return result
            if result["server_encoding"] == "SQL_ASCII":
                result["warning"] = "server_encoding=SQL_ASCII does not validate or convert stored bytes; RAGFlow clients are forced to UTF8"
            result["status"] = "healthy"
            return result
        except Exception as exc:
            result["error"] = mask_gaussdb_text(exc)
            return result

    def _query_version(self) -> str:
        return self._query_required_scalar("SELECT version()", "version")

    def _query_sql_compatibility(self) -> str:
        return self._query_required_scalar("SHOW sql_compatibility", "sql_compatibility").upper()

    def _query_server_encoding(self) -> str:
        return self._query_required_scalar("SHOW server_encoding", "server_encoding").upper()

    def _query_client_encoding(self) -> str:
        return self._query_required_scalar("SHOW client_encoding", "client_encoding").upper()

    def _query_required_scalar(self, sql: str, field_name: str) -> str:
        row = self.pool.fetch_one(sql)
        if not row or row[0] is None or str(row[0]).strip() == "":
            raise RuntimeError(f"GaussDB {field_name} query returned no rows")
        return str(row[0]).strip()

    def get_performance_metrics(self) -> dict:
        st = timer()
        try:
            self.pool.fetch_one("SELECT 1")
            return {
                "connection": "connected",
                "latency_ms": round((timer() - st) * 1000.0, 3),
                "schema": self.resolved_schema,
            }
        except Exception as exc:
            return {
                "connection": "disconnected",
                "latency_ms": round((timer() - st) * 1000.0, 3),
                "error": mask_gaussdb_text(exc),
            }

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        raise NotImplementedError("GaussDB create_idx is implemented in the DDL task")

    def delete_idx(self, index_name: str, dataset_id: str):
        raise NotImplementedError("GaussDB delete_idx is implemented in the CRUD task")

    def index_exist(self, index_name: str, dataset_id: str) -> bool:
        raise NotImplementedError("GaussDB index_exist is implemented in the DDL task")

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
    ):
        raise NotImplementedError("GaussDB search is implemented in the search task")

    def get(self, data_id: str, index_name: str, dataset_ids: list[str]) -> dict | None:
        raise NotImplementedError("GaussDB get is implemented in the CRUD task")

    def insert(self, rows: list[dict], index_name: str, dataset_id: str = None) -> list[str]:
        raise NotImplementedError("GaussDB insert is implemented in the CRUD task")

    def update(self, condition: dict, new_value: dict, index_name: str, dataset_id: str) -> bool:
        raise NotImplementedError("GaussDB update is implemented in the CRUD task")

    def delete(self, condition: dict, index_name: str, dataset_id: str) -> int:
        raise NotImplementedError("GaussDB delete is implemented in the CRUD task")

    def get_total(self, res):
        raise NotImplementedError("GaussDB get_total is implemented in the adapter task")

    def get_doc_ids(self, res):
        raise NotImplementedError("GaussDB get_doc_ids is implemented in the adapter task")

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        raise NotImplementedError("GaussDB get_fields is implemented in the adapter task")

    def get_highlight(self, res, keywords: list[str], field_name: str):
        raise NotImplementedError("GaussDB get_highlight is implemented in the search task")

    def get_aggregation(self, res, field_name: str):
        raise NotImplementedError("GaussDB get_aggregation is implemented in the search task")

    def sql(self, sql: str, fetch_size: int, format: str):
        raise NotImplementedError("GaussDB sql is implemented in the Text-to-SQL task")
