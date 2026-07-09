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
"""Translate RAGflow document-metadata filter lists into Infinity SQL filter expressions."""

from __future__ import annotations

import ast
import re
from typing import Any, Dict, List, Sequence

_KEY_PATTERN = re.compile(r"^[a-zA-Z_][a-zA-Z0-9_]*$")


def _validate_key(key: str, flt: Dict[str, Any]) -> None:
    if not _KEY_PATTERN.match(key):
        raise ValueError(f"invalid key format (must be identifier-like): {flt}")


SUPPORTED_OPERATORS: frozenset[str] = frozenset(
    {
        "=",
        "≠",
        ">",
        "<",
        "≥",
        "≤",
        "in",
        "not in",
        "contains",
        "not contains",
        "start with",
        "end with",
        "empty",
        "not empty",
    }
)

_RANGE_OPS: Dict[str, str] = {
    ">": ">",
    "<": "<",
    "≥": ">=",
    "≤": "<=",
}


class MetaFilterTranslator:
    """Translate one user filter clause at a time into Infinity SQL filter strings."""

    def translate(self, flt: Dict[str, Any]) -> str:
        op = flt.get("op")
        key = flt.get("key")
        value = flt.get("value")

        if not key or not isinstance(key, str):
            raise ValueError(f"filter is missing a string key: {flt}")
        _validate_key(key, flt)
        if op not in SUPPORTED_OPERATORS:
            raise ValueError(f"unknown operator: {op!r}, filter: {flt}")

        if op == "empty":
            return self._translate_empty(key)
        if op == "not empty":
            return self._translate_not_empty(key)
        if op == "=":
            return self._translate_equal(key, value, flt)
        if op == "≠":
            return self._translate_not_equal(key, value, flt)
        if op in _RANGE_OPS:
            return self._translate_range(key, op, value, flt)
        if op == "in":
            return self._translate_in(key, value, flt)
        if op == "not in":
            return self._translate_not_in(key, value, flt)
        if op == "contains":
            return self._translate_contains(key, value, flt)
        if op == "not contains":
            return self._translate_not_contains(key, value, flt)
        if op == "start with":
            return self._translate_start_with(key, value, flt)
        if op == "end with":
            return self._translate_end_with(key, value, flt)

        raise ValueError(f"no handler for operator: {op!r}, filter: {flt}")

    def _translate_empty(self, key: str) -> str:
        return f"JSON_EXTRACT_STRING(meta_fields, '$.{key}') = '\"\"'"

    def _translate_not_empty(self, key: str) -> str:
        return f"JSON_EXTRACT_STRING(meta_fields, '$.{key}') != '\"\"'"

    def _translate_equal(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        coerced = _coerce_scalar(value, flt)
        if isinstance(coerced, str):
            escaped = _escape_sql_string(coerced)
            return f"JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')"
        return f"JSON_CONTAINS(meta_fields, '$.{key}', {coerced})"

    def _translate_not_equal(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        coerced = _coerce_scalar(value, flt)
        if isinstance(coerced, str):
            escaped = _escape_sql_string(coerced)
            return f"NOT JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')"
        return f"NOT JSON_CONTAINS(meta_fields, '$.{key}', {coerced})"

    def _translate_range(self, key: str, op: str, value: Any, flt: Dict[str, Any]) -> str:
        coerced = _coerce_range_value(value, flt)
        sql_op = _RANGE_OPS.get(op, op)
        if isinstance(coerced, str):
            escaped = _escape_sql_string(coerced)
            return f"JSON_EXTRACT_STRING(meta_fields, '$.{key}') {sql_op} '{escaped}'"
        return f"JSON_EXTRACT_DOUBLE(meta_fields, '$.{key}') {sql_op} {coerced}"

    def _translate_in(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        members = _csv_or_list(value, flt)
        string_parts = []
        num_parts = []
        for m in members:
            # Use same coercion as range operators to detect numeric values
            coerced = _coerce_range_value(m, flt)
            if isinstance(coerced, (int, float)):
                num_parts.append(f"JSON_CONTAINS(meta_fields, '$.{key}', {coerced})")
            else:
                escaped = _escape_sql_string(coerced)
                string_parts.append(f"JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')")
        conditions = []
        if string_parts:
            conditions.append("(" + " OR ".join(string_parts) + ")")
        if num_parts:
            conditions.append("(" + " OR ".join(num_parts) + ")")
        return "(" + " OR ".join(conditions) + ")"

    def _translate_not_in(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        members = _csv_or_list(value, flt)
        string_parts = []
        num_parts = []
        for m in members:
            # Use same coercion as range operators to detect numeric values
            coerced = _coerce_range_value(m, flt)
            if isinstance(coerced, (int, float)):
                num_parts.append(f"NOT JSON_CONTAINS(meta_fields, '$.{key}', {coerced})")
            else:
                escaped = _escape_sql_string(coerced)
                string_parts.append(f"NOT JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')")
        conditions = []
        if string_parts:
            conditions.append("(" + " AND ".join(string_parts) + ")")
        if num_parts:
            conditions.append("(" + " AND ".join(num_parts) + ")")
        return " AND ".join(conditions)

    def _translate_contains(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        if not value and value != 0:
            raise ValueError(f"contains value is empty: {flt}")
        # Use same coercion as range operators to detect numeric values
        coerced = _coerce_range_value(value, flt)
        if isinstance(coerced, (int, float)):
            return f"JSON_CONTAINS(meta_fields, '$.{key}', {coerced})"
        escaped = _escape_sql_string(str(value))
        return f"JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')"

    def _translate_not_contains(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        text = _coerce_string(value, flt)
        escaped = _escape_sql_string(text)
        # Use Infinity's JSON_CONTAINS to check if value does NOT exist in JSON array
        return f"NOT JSON_CONTAINS(meta_fields, '$.{key}', '\"{escaped}\"')"

    def _translate_start_with(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        text = _coerce_string(value, flt)
        escaped = _escape_sql_string(_escape_likeWildcards(text))
        return f"JSON_EXTRACT_STRING(meta_fields, '$.{key}') LIKE '{escaped}%'"

    def _translate_end_with(self, key: str, value: Any, flt: Dict[str, Any]) -> str:
        text = _coerce_string(value, flt)
        escaped = _escape_sql_string(_escape_likeWildcards(text))
        return f"JSON_EXTRACT_STRING(meta_fields, '$.{key}') LIKE '%{escaped}'"


def plan_pushdown(filters: Sequence[Dict[str, Any]], logic: str) -> List[str]:
    if logic not in {"and", "or"}:
        raise ValueError(f"unknown logic {logic!r}")
    translator = MetaFilterTranslator()
    return [translator.translate(flt) for flt in filters]


def build_infinity_filter(filters: Sequence[Dict[str, Any]], logic: str) -> str:
    if not filters:
        return "1=1"
    fragments = plan_pushdown(filters, logic)
    joiner = " AND " if logic == "and" else " OR "
    result = "(" + joiner.join(fragments) + ")"
    return result


def is_pushdown_supported(filters: Sequence[Dict[str, Any]]) -> bool:
    for flt in filters:
        op = flt.get("op")
        if op not in SUPPORTED_OPERATORS:
            return False
        if not isinstance(flt.get("key"), str) or not flt.get("key"):
            return False
    return True


def extract_doc_ids(df) -> List[str]:
    if df is None or not hasattr(df, "iterrows"):
        return []
    return [str(row["id"]) for _, row in df.iterrows() if "id" in row]


# ---------------------------------------------------------------------------
# Value coercion helpers
# ---------------------------------------------------------------------------


def _coerce_scalar(value: Any, flt: Dict[str, Any]) -> Any:
    if value is None:
        raise ValueError(f"scalar comparison value is None: {flt}")
    if isinstance(value, (list, dict)):
        raise ValueError(f"scalar comparison value is non-scalar: {flt}")
    try:
        parsed = ast.literal_eval(str(value).strip())
        if isinstance(parsed, (int, float, bool)):
            return parsed
    except Exception:
        pass
    return str(value)


def _coerce_range_value(value: Any, flt: Dict[str, Any]) -> Any:
    if value is None:
        raise ValueError(f"range comparison value is None: {flt}")
    try:
        parsed = ast.literal_eval(str(value).strip())
        if isinstance(parsed, (int, float)):
            return parsed
    except Exception:
        pass
    return str(value)


def _coerce_string(value: Any, flt: Dict[str, Any]) -> str:
    if value is None:
        raise ValueError(f"string-operator value is None: {flt}")
    if isinstance(value, (list, dict)):
        raise ValueError(f"string-operator value must be a scalar: {flt}")
    s = str(value)
    if not s:
        raise ValueError(f"string-operator value is empty: {flt}")
    return s


def _csv_or_list(value: Any, flt: Dict[str, Any]) -> List[Any]:
    if value is None:
        raise ValueError(f"membership value is None: {flt}")
    if isinstance(value, (list, tuple)):
        members = list(value)
    elif isinstance(value, str):
        try:
            parsed = ast.literal_eval(value)
        except Exception:
            parsed = value
        if isinstance(parsed, (list, tuple)):
            members = list(parsed)
        else:
            members = [m.strip() for m in value.split(",") if m.strip()]
    else:
        members = [value]
    if not members:
        raise ValueError(f"membership value resolved to empty list: {flt}")
    normalised: List[Any] = []
    for m in members:
        if isinstance(m, str):
            normalised.append(m.lower().strip())
        else:
            normalised.append(m)
    return normalised


def _escape_sql_string(s: str) -> str:
    return s.replace("'", "''")


def _escape_likeWildcards(text: str) -> str:
    return text.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
