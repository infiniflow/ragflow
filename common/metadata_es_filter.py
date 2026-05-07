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
"""Translate RAGflow document-metadata filter lists into Elasticsearch DSL.

The legacy ``common.metadata_utils.meta_filter`` evaluates user-defined
metadata conditions in Python after loading every document's metadata into
memory. That works for small knowledge bases but degrades badly past a few
thousand documents. This module produces an equivalent ES bool query so the
filtering can be pushed down to the search engine.

Operators handled here mirror ``meta_filter`` exactly. When a filter cannot be
translated (unknown operator, malformed value, list-typed input that the
in-memory code special-cases) the translator raises
:class:`UnsupportedMetaFilter` so callers fall back to the in-memory path
without silently changing semantics.
"""

from __future__ import annotations

import ast
import re
from dataclasses import dataclass, field
from typing import Any, Dict, Iterable, List, Optional, Sequence

# Field prefix in the doc-metadata ES index. Every user metadata key lives at
# ``meta_fields.<key>`` thanks to the dynamic object mapping in
# ``conf/doc_meta_es_mapping.json``.
META_FIELDS_PREFIX = "meta_fields"

# Strict ``YYYY-MM-DD`` recogniser, kept consistent with the legacy in-memory
# path. Mismatched-type comparisons (string vs date, list vs scalar) fall back
# to in-memory semantics rather than guess at the right ES coercion.
_DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")

# Operators that the legacy filter exposes. Anything outside this set is a bug
# elsewhere; surface it instead of silently no-op'ing.
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

# ES range comparators keyed by RAGflow operator.
_RANGE_OPS: Dict[str, str] = {
    ">": "gt",
    "<": "lt",
    "≥": "gte",
    "≤": "lte",
}

# Negative operators that diverge from ``meta_filter`` on multi-valued metadata
# fields. The in-memory path checks each value bucket independently, so a doc
# whose field is ``[a, b]`` matches ``≠ a`` (because the ``b`` bucket satisfies
# the predicate). ``must_not term: a`` in ES would exclude that doc outright.
# Without a cheap way to prove a field is single-valued at query time we refuse
# push-down for these operators and let the in-memory fallback handle them.
# ``not contains`` is not in this set: ``all(not contains)`` is equivalent to
# ``not any(contains)``, so ``must_not wildcard *X*`` matches the legacy
# semantics on both single- and multi-valued fields.
MULTIVALUE_UNSAFE_NEGATIVE_OPS: frozenset[str] = frozenset({"≠", "not in"})


class UnsupportedMetaFilter(Exception):
    """Raised when a metadata filter cannot be expressed as ES DSL.

    Carries the filter that failed so callers can log a precise reason and the
    in-memory fallback can pick up unchanged.
    """

    def __init__(self, reason: str, filter_clause: Optional[Dict[str, Any]] = None) -> None:
        super().__init__(reason)
        self.reason = reason
        self.filter_clause = filter_clause


@dataclass
class TranslatedFilter:
    """A single user filter rendered as one or more ES bool clauses.

    A clause that wants the field to be present (``≠``, ``not in``, range,
    ``not contains``) goes into ``must`` so the negation does not accidentally
    match documents missing the key. ``must_not`` carries the actual rejection.
    Pure positive filters (``=``, ``contains``, ``in``, ``exists``) fill
    ``must`` only.
    """

    must: List[Dict[str, Any]] = field(default_factory=list)
    must_not: List[Dict[str, Any]] = field(default_factory=list)

    def to_clauses(self) -> List[Dict[str, Any]]:
        """Collapse to the ES clauses this filter contributes to a parent bool.

        Always emits a single atomic clause when there is anything to emit:
        a multi-clause ``must`` (e.g. range = ``exists`` + ``range``) gets
        wrapped in its own ``bool`` so an OR-logic parent ``should`` can't
        match on just one half of the filter. A pure single positive clause
        is returned unwrapped because there is nothing to break apart.
        """
        if not self.must and not self.must_not:
            return []
        if not self.must_not:
            if len(self.must) == 1:
                return list(self.must)
            # Multi-clause positive filter — keep it atomic for OR parents.
            return [{"bool": {"must": list(self.must)}}]
        # Negative semantics always need wrapping so they survive being OR'd
        # with siblings.
        return [{"bool": {"must": list(self.must), "must_not": list(self.must_not)}}]


@dataclass
class MetaFilterPushdownPlan:
    """Composed ES bool query body for an entire RAGflow filter request."""

    logic: str
    translated: List[TranslatedFilter] = field(default_factory=list)

    def is_empty(self) -> bool:
        return not self.translated

    def to_query(self, kb_ids: Sequence[str]) -> Dict[str, Any]:
        """Render the full ES query body, scoped to the given KB ids.

        The KB filter is always a ``terms`` clause so the query can serve any
        number of knowledge bases without rewriting the caller.
        """
        kb_clause = {"terms": {"kb_id": list(kb_ids)}}

        if self.is_empty():
            return {"query": {"bool": {"filter": [kb_clause]}}}

        sub_clauses = [t.to_clauses() for t in self.translated]
        flat_clauses: List[Dict[str, Any]] = [c for group in sub_clauses for c in group]

        if self.logic == "or":
            inner = {
                "bool": {
                    "should": flat_clauses,
                    "minimum_should_match": 1,
                }
            }
        else:
            inner = {"bool": {"must": flat_clauses}}

        return {
            "query": {
                "bool": {
                    "filter": [kb_clause, inner],
                }
            }
        }


class MetaFilterTranslator:
    """Translate one user filter clause at a time into ES DSL fragments.

    Stateless aside from configuration; safe to instantiate once per request
    or share at module scope.
    """

    def __init__(self, prefix: str = META_FIELDS_PREFIX) -> None:
        self.prefix = prefix

    def field_name(self, key: str) -> str:
        """Compose the dotted ES field path for a user metadata key."""
        return f"{self.prefix}.{key}"

    def translate(self, flt: Dict[str, Any]) -> TranslatedFilter:
        """Translate a single filter dict into ES bool clauses.

        Raises ``UnsupportedMetaFilter`` for malformed input or operator/value
        combinations the legacy in-memory path treats as a special case (e.g.
        list-of-strings membership in ``in``/``not in``).
        """
        op = flt.get("op")
        key = flt.get("key")
        value = flt.get("value")

        if not key or not isinstance(key, str):
            raise UnsupportedMetaFilter("filter is missing a string key", flt)
        if op not in SUPPORTED_OPERATORS:
            raise UnsupportedMetaFilter(f"unknown operator {op!r}", flt)

        field_path = self.field_name(key)

        if op == "empty":
            return self._translate_empty(field_path)
        if op == "not empty":
            return self._translate_not_empty(field_path)
        if op == "=":
            return self._translate_equal(field_path, value, flt)
        if op == "≠":
            return self._translate_not_equal(field_path, value, flt)
        if op in _RANGE_OPS:
            return self._translate_range(field_path, op, value, flt)
        if op == "in":
            return self._translate_in(field_path, value, flt)
        if op == "not in":
            return self._translate_not_in(field_path, value, flt)
        if op == "contains":
            return self._translate_contains(field_path, value, flt)
        if op == "not contains":
            return self._translate_not_contains(field_path, value, flt)
        if op == "start with":
            return self._translate_start_with(field_path, value, flt)
        if op == "end with":
            return self._translate_end_with(field_path, value, flt)

        # Unreachable: SUPPORTED_OPERATORS gate above covers every branch.
        raise UnsupportedMetaFilter(f"no handler for operator {op!r}", flt)

    def _translate_empty(self, field_path: str) -> TranslatedFilter:
        # "empty" matches documents whose value is missing OR equals "" — same
        # falsy semantics the in-memory ``not input`` check enforces. The
        # blank-string check has to target ``.keyword`` because the analyzed
        # text field drops empty values during tokenisation, leaving no token
        # for ``term: ""`` to match.
        return TranslatedFilter(
            must=[
                {
                    "bool": {
                        "should": [
                            {"bool": {"must_not": [{"exists": {"field": field_path}}]}},
                            {"term": {_keyword_path(field_path): ""}},
                        ],
                        "minimum_should_match": 1,
                    }
                }
            ]
        )

    def _translate_not_empty(self, field_path: str) -> TranslatedFilter:
        return TranslatedFilter(
            must=[{"exists": {"field": field_path}}],
            must_not=[{"term": {_keyword_path(field_path): ""}}],
        )

    def _translate_equal(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        coerced = _coerce_scalar(value, flt)
        return TranslatedFilter(must=[_term_or_match(field_path, coerced)])

    def _translate_not_equal(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        coerced = _coerce_scalar(value, flt)
        return TranslatedFilter(
            must=[{"exists": {"field": field_path}}],
            must_not=[_term_or_match(field_path, coerced)],
        )

    def _translate_range(self, field_path: str, op: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        coerced = _coerce_range_value(value, flt)
        return TranslatedFilter(
            must=[
                {"exists": {"field": field_path}},
                {"range": {field_path: {_RANGE_OPS[op]: coerced}}},
            ]
        )

    def _translate_in(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        members = _csv_or_list(value, flt)
        return TranslatedFilter(must=[_terms_string_or_numeric(field_path, members)])

    def _translate_not_in(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        members = _csv_or_list(value, flt)
        return TranslatedFilter(
            must=[{"exists": {"field": field_path}}],
            must_not=[_terms_string_or_numeric(field_path, members)],
        )

    def _translate_contains(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        text = _coerce_string(value, flt)
        return TranslatedFilter(must=[_wildcard(field_path, f"*{_escape_wildcard(text)}*")])

    def _translate_not_contains(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        text = _coerce_string(value, flt)
        return TranslatedFilter(
            must=[{"exists": {"field": field_path}}],
            must_not=[_wildcard(field_path, f"*{_escape_wildcard(text)}*")],
        )

    def _translate_start_with(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        text = _coerce_string(value, flt)
        return TranslatedFilter(
            must=[{"prefix": {_keyword_path(field_path): {"value": text, "case_insensitive": True}}}]
        )

    def _translate_end_with(self, field_path: str, value: Any, flt: Dict[str, Any]) -> TranslatedFilter:
        text = _coerce_string(value, flt)
        return TranslatedFilter(must=[_wildcard(field_path, f"*{_escape_wildcard(text)}")])


def build_meta_filter_query(
    filters: Sequence[Dict[str, Any]],
    logic: str,
    kb_ids: Sequence[str],
    translator: Optional[MetaFilterTranslator] = None,
) -> Dict[str, Any]:
    """Top-level helper: translate every filter and render the ES query body.

    Raises ``UnsupportedMetaFilter`` if any filter cannot be expressed.
    """
    plan = plan_pushdown(filters, logic, translator=translator)
    return plan.to_query(kb_ids)


def plan_pushdown(
    filters: Sequence[Dict[str, Any]],
    logic: str,
    translator: Optional[MetaFilterTranslator] = None,
) -> MetaFilterPushdownPlan:
    """Translate every filter in turn, building a single composed plan.

    Separated from ``build_meta_filter_query`` so callers can inspect or
    augment the plan before binding it to a KB scope.
    """
    if logic not in {"and", "or"}:
        raise UnsupportedMetaFilter(f"unknown logic {logic!r}")

    t = translator or MetaFilterTranslator()
    plan = MetaFilterPushdownPlan(logic=logic)
    for flt in filters:
        plan.translated.append(t.translate(flt))
    return plan


def is_pushdown_supported(filters: Sequence[Dict[str, Any]]) -> bool:
    """Cheap pre-check: do all filters look translatable without coercion?

    Used by the routing layer to skip the heavier ``plan_pushdown`` call when
    the request obviously needs the in-memory fallback.

    Operators in :data:`MULTIVALUE_UNSAFE_NEGATIVE_OPS` are rejected here so a
    single such filter forces the whole request to in-memory evaluation, which
    is the only place we can replicate the per-bucket semantics over
    multi-valued metadata fields.
    """
    for flt in filters:
        op = flt.get("op")
        if op not in SUPPORTED_OPERATORS:
            return False
        if op in MULTIVALUE_UNSAFE_NEGATIVE_OPS:
            return False
        if not isinstance(flt.get("key"), str) or not flt.get("key"):
            return False
    return True


def extract_doc_ids(es_response: Dict[str, Any]) -> List[str]:
    """Pull doc IDs out of an ES search response shaped like ``{hits:{hits:[...]}}``.

    Tolerates both the dict-typed ES 7+ response and the dict-coerced
    ``ObjectApiResponse`` returned by the elasticsearch python client.
    """
    hits_root = es_response.get("hits") if isinstance(es_response, dict) else None
    if not hits_root:
        # ``ObjectApiResponse`` is dict-like; ``.get`` works at both levels.
        try:
            hits_root = es_response["hits"]
        except Exception:
            return []

    raw_hits: Iterable[Dict[str, Any]]
    if isinstance(hits_root, dict):
        raw_hits = hits_root.get("hits", []) or []
    else:
        raw_hits = []

    out: List[str] = []
    for hit in raw_hits:
        if not isinstance(hit, dict):
            continue
        # ``id`` is mirrored into ``_source`` by the metadata writer; ``_id``
        # is the canonical identifier. Prefer ``_id`` so renames in the source
        # field name don't break us.
        doc_id = hit.get("_id")
        if not doc_id:
            source = hit.get("_source") or {}
            doc_id = source.get("id") or source.get("doc_id")
        if doc_id:
            out.append(str(doc_id))
    return out


# ---------------------------------------------------------------------------
# Value coercion helpers
# ---------------------------------------------------------------------------


def _coerce_scalar(value: Any, flt: Dict[str, Any]) -> Any:
    """Mirror the legacy ``ast.literal_eval`` then ``str.lower()`` flow.

    The in-memory filter parses values as Python literals when possible (so
    ``"5"`` becomes ``5``) and lower-cases strings. For ES ``term`` queries we
    need the same coercion or numeric data won't match.
    """
    if value is None:
        raise UnsupportedMetaFilter("scalar comparison value is None", flt)
    if isinstance(value, (list, dict)):
        raise UnsupportedMetaFilter("scalar comparison value is non-scalar", flt)

    s = str(value).strip()
    if _DATE_RE.match(s):
        return s
    try:
        parsed = ast.literal_eval(s)
    except Exception:
        parsed = s
    if isinstance(parsed, str):
        return parsed.lower()
    if isinstance(parsed, (int, float, bool)):
        return parsed
    return s.lower()


def _coerce_range_value(value: Any, flt: Dict[str, Any]) -> Any:
    """Range comparisons accept dates verbatim and numbers parsed via literal_eval.

    Strings that aren't numeric or ISO dates are pushed through as-is — ES
    will compare them lexically against keyword fields, which is the same
    behaviour as the in-memory ``input >= value`` Python comparison after the
    original ``ast.literal_eval`` failure path.
    """
    if value is None:
        raise UnsupportedMetaFilter("range comparison value is None", flt)
    s = str(value).strip()
    if _DATE_RE.match(s):
        return s
    try:
        parsed = ast.literal_eval(s)
    except Exception:
        return s
    if isinstance(parsed, (int, float)):
        return parsed
    return s


def _coerce_string(value: Any, flt: Dict[str, Any]) -> str:
    """String operators (contains/start with/end with) need a non-empty string."""
    if value is None:
        raise UnsupportedMetaFilter("string-operator value is None", flt)
    if isinstance(value, (list, dict)):
        raise UnsupportedMetaFilter("string-operator value must be a scalar", flt)
    s = str(value)
    if not s:
        raise UnsupportedMetaFilter("string-operator value is empty", flt)
    return s


def _csv_or_list(value: Any, flt: Dict[str, Any]) -> List[Any]:
    """``in`` / ``not in`` accept either a real list or a comma-separated string.

    The legacy in-memory path applies ``ast.literal_eval`` to the value too.
    Mirror that for parity, then trim whitespace and lower-case any strings.
    """
    if value is None:
        raise UnsupportedMetaFilter("membership value is None", flt)

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
        raise UnsupportedMetaFilter("membership value resolved to empty list", flt)

    normalised: List[Any] = []
    for m in members:
        if isinstance(m, str):
            normalised.append(m.lower().strip())
        else:
            normalised.append(m)
    return normalised


def _keyword_path(field_path: str) -> str:
    """Sub-field used for exact-match string queries.

    Dynamic mapping under ``meta_fields`` indexes string values as ``text``
    with a ``.keyword`` multi-field. ``term``/``terms``/``prefix``/``wildcard``
    against the analyzed parent breaks for any multi-word value because the
    inverted index stores per-token entries, not the original phrase. Routing
    string queries through ``<field>.keyword`` keeps semantics aligned with the
    in-memory ``meta_filter`` (full-string compare after lower-casing).
    """
    return f"{field_path}.keyword"


def _term_or_match(field_path: str, value: Any) -> Dict[str, Any]:
    """Exact-match clause that respects how dynamic mapping indexes the value.

    String values target the ``.keyword`` sub-field with ``case_insensitive``
    so phrase values still match (the in-memory path lower-cases before
    comparing). Numeric / bool values target the parent path because numeric
    fields have no ``.keyword`` sub-field under default dynamic mapping.
    """
    if isinstance(value, str):
        return {
            "term": {
                _keyword_path(field_path): {
                    "value": value,
                    "case_insensitive": True,
                }
            }
        }
    return {"term": {field_path: value}}


def _terms_string_or_numeric(field_path: str, members: List[Any]) -> Dict[str, Any]:
    """``in``/``not in`` payload that mirrors ``_term_or_match`` per element.

    ES ``terms`` does not accept ``case_insensitive``, so for string members we
    expand into a ``bool: should`` of case-insensitive ``term`` queries on the
    keyword sub-field. Pure-numeric / bool member lists keep the cheaper
    ``terms`` form on the parent path.
    """
    if all(not isinstance(m, str) for m in members):
        return {"terms": {field_path: members}}
    return {
        "bool": {
            "should": [_term_or_match(field_path, m) for m in members],
            "minimum_should_match": 1,
        }
    }


def _wildcard(field_path: str, pattern: str) -> Dict[str, Any]:
    """Wildcard runs against ``.keyword`` so the original phrase is searched.

    ``wildcard`` against an analyzed text field walks per-token entries, which
    drops phrase context (``Alice Wonderland`` becomes tokens ``alice``,
    ``wonderland``). The ``.keyword`` sub-field preserves the full original
    string, matching the in-memory ``str.find`` semantics.
    """
    return {
        "wildcard": {
            _keyword_path(field_path): {
                "value": pattern,
                "case_insensitive": True,
            }
        }
    }


def _escape_wildcard(text: str) -> str:
    """Escape the two ES wildcard metacharacters so user input stays literal."""
    return text.replace("\\", "\\\\").replace("*", "\\*").replace("?", "\\?")
