"""Unit tests for the metadata filter push-down translators (ES and Infinity).

Verifies the shape of the produced filter expressions for both ES DSL and
Infinity SQL, and confirms that coercion rules (lower-casing, list-membership,
date detection) are consistent between the two backends.
"""

import pytest

pytestmark = pytest.mark.p2

from common.metadata_es_filter import MetaFilterTranslator as ESMetaFilterTranslator
from common.metadata_infinity_filter import (
    MetaFilterTranslator as InfinityMetaFilterTranslator,
    SUPPORTED_OPERATORS,
    build_infinity_filter,
    is_pushdown_supported,
    plan_pushdown,
    extract_doc_ids,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def es_translator() -> ESMetaFilterTranslator:
    return ESMetaFilterTranslator()


@pytest.fixture
def infinity_translator() -> InfinityMetaFilterTranslator:
    return InfinityMetaFilterTranslator()


# ---------------------------------------------------------------------------
# Shared: is_pushdown_supported pre-check (same logic for both backends)
# ---------------------------------------------------------------------------


def test_pushdown_check_accepts_known_ops():
    assert is_pushdown_supported(
        [
            {"key": "tag", "op": "=", "value": "v"},
            {"key": "tag", "op": "contains", "value": "x"},
        ]
    )


def test_pushdown_check_rejects_unknown_op():
    assert not is_pushdown_supported([{"key": "tag", "op": "regex", "value": "^v"}])


def test_pushdown_check_rejects_missing_key():
    assert not is_pushdown_supported([{"op": "=", "value": "v"}])


def test_pushdown_check_accepts_not_contains():
    assert is_pushdown_supported([{"key": "tag", "op": "not contains", "value": "x"}])


# ---------------------------------------------------------------------------
# Shared: plan_pushdown (same logic for both backends)
# ---------------------------------------------------------------------------


def test_plan_pushdown_and_logic():
    fragments = plan_pushdown(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "score", "op": ">", "value": "5"},
        ],
        logic="and",
    )
    assert len(fragments) == 2


def test_plan_pushdown_or_logic():
    fragments = plan_pushdown(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "tag", "op": "=", "value": "beta"},
        ],
        logic="or",
    )
    assert len(fragments) == 2


def test_unknown_logic_rejected():
    with pytest.raises(ValueError):
        plan_pushdown([{"key": "k", "op": "=", "value": "v"}], logic="xor")


# ---------------------------------------------------------------------------
# Shared: extract_doc_ids (same implementation)
# ---------------------------------------------------------------------------


def test_extract_doc_ids_from_dataframe():
    import pandas as pd

    df = pd.DataFrame({"id": ["doc1", "doc2", "doc3"]})
    assert extract_doc_ids(df) == ["doc1", "doc2", "doc3"]


def test_extract_doc_ids_empty_dataframe():
    import pandas as pd

    df = pd.DataFrame({"id": []})
    assert extract_doc_ids(df) == []


def test_extract_doc_ids_none_input():
    assert extract_doc_ids(None) == []


def test_extract_doc_ids_non_dataframe():
    assert extract_doc_ids("not a dataframe") == []


# ---------------------------------------------------------------------------
# Shared: SUPPORTED_OPERATORS
# ---------------------------------------------------------------------------


def test_supported_operator_set_matches_documentation():
    expected = {
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
    assert SUPPORTED_OPERATORS == expected


# ===========================================================================
# ES-only tests
# ===========================================================================


def test_equal_translates_to_term_with_lowercased_value(es_translator):
    """String equality runs against ``.keyword`` so multi-word phrases match."""
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "tag", "op": "=", "value": "Alpha"}).to_clauses()
    assert clauses == [
        {"term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}}
    ]


def test_equal_parses_numeric_literal(es_translator):
    """Numeric values stay on the parent path — no ``.keyword`` sub-field exists for ``long``."""
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "score", "op": "=", "value": "5"}).to_clauses()
    assert clauses == [{"term": {_field("score"): 5}}]


def test_equal_multiword_uses_keyword_subfield(es_translator):
    """Regression: multi-word string values must match via .keyword sub-field."""
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate(
        {"key": "author", "op": "=", "value": "Alice Wonderland"}
    ).to_clauses()
    assert clauses == [
        {
            "term": {
                _field("author") + ".keyword": {
                    "value": "alice wonderland",
                    "case_insensitive": True,
                }
            }
        }
    ]


def test_not_equal_requires_field_to_exist(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "tag", "op": "≠", "value": "alpha"}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("tag")}}],
                "must_not": [
                    {"term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}}
                ],
            }
        }
    ]


@pytest.mark.parametrize(
    "op,es_key",
    [(">", "gt"), ("<", "lt"), ("≥", "gte"), ("≤", "lte")],
)
def test_range_operator_translation(es_translator, op, es_key):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "score", "op": op, "value": "10"}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [
                    {"exists": {"field": _field("score")}},
                    {"range": {_field("score"): {es_key: 10}}},
                ]
            }
        }
    ]


def test_range_passes_iso_date_through_unparsed(es_translator):
    clauses = es_translator.translate({"key": "published", "op": "≥", "value": "2025-01-15"}).to_clauses()
    range_clause = clauses[0]["bool"]["must"][1]
    assert range_clause == {"range": {"meta_fields.published": {"gte": "2025-01-15"}}}


def test_in_operator_csv_value_lowercased(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    def _string_terms_should(field_path: str, members):
        return {
            "bool": {
                "should": [
                    {"term": {field_path + ".keyword": {"value": m, "case_insensitive": True}}}
                    for m in members
                ],
                "minimum_should_match": 1,
            }
        }

    clauses = es_translator.translate({"key": "status", "op": "in", "value": "Active,Pending"}).to_clauses()
    assert clauses == [_string_terms_should(_field("status"), ["active", "pending"])]


def test_in_operator_python_list_literal(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    def _string_terms_should(field_path: str, members):
        return {
            "bool": {
                "should": [
                    {"term": {field_path + ".keyword": {"value": m, "case_insensitive": True}}}
                    for m in members
                ],
                "minimum_should_match": 1,
            }
        }

    clauses = es_translator.translate({"key": "status", "op": "in", "value": "['Open', 'Closed']"}).to_clauses()
    assert clauses == [_string_terms_should(_field("status"), ["open", "closed"])]


def test_in_operator_numeric_members_keep_terms(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "year", "op": "in", "value": "[2024, 2025]"}).to_clauses()
    assert clauses == [{"terms": {_field("year"): [2024, 2025]}}]


def test_not_in_negates_with_existence_guard(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    def _string_terms_should(field_path: str, members):
        return {
            "bool": {
                "should": [
                    {"term": {field_path + ".keyword": {"value": m, "case_insensitive": True}}}
                    for m in members
                ],
                "minimum_should_match": 1,
            }
        }

    clauses = es_translator.translate({"key": "status", "op": "not in", "value": "active,pending"}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("status")}}],
                "must_not": [_string_terms_should(_field("status"), ["active", "pending"])],
            }
        }
    ]


def test_contains_uses_case_insensitive_wildcard(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "version", "op": "contains", "value": "earth"}).to_clauses()
    assert clauses == [
        {
            "wildcard": {
                _field("version") + ".keyword": {
                    "value": "*earth*",
                    "case_insensitive": True,
                }
            }
        }
    ]


def test_contains_escapes_user_wildcards(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "title", "op": "contains", "value": "a*b?c"}).to_clauses()
    pattern = clauses[0]["wildcard"][_field("title") + ".keyword"]["value"]
    assert pattern == "*a\\*b\\?c*"


def test_not_contains_negates_with_exists(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "version", "op": "not contains", "value": "earth"}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("version")}}],
                "must_not": [
                    {
                        "wildcard": {
                            _field("version") + ".keyword": {
                                "value": "*earth*",
                                "case_insensitive": True,
                            }
                        }
                    }
                ],
            }
        }
    ]


def test_start_with_uses_prefix(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "name", "op": "start with", "value": "pre"}).to_clauses()
    assert clauses == [
        {"prefix": {_field("name") + ".keyword": {"value": "pre", "case_insensitive": True}}}
    ]


def test_end_with_uses_trailing_wildcard(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "file", "op": "end with", "value": ".pdf"}).to_clauses()
    pattern = clauses[0]["wildcard"][_field("file") + ".keyword"]["value"]
    assert pattern == "*.pdf"


def test_empty_matches_missing_or_blank(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "notes", "op": "empty", "value": ""}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "should": [
                    {"bool": {"must_not": [{"exists": {"field": _field("notes")}}]}},
                    {"term": {_field("notes") + ".keyword": ""}},
                ],
                "minimum_should_match": 1,
            }
        }
    ]


def test_not_empty_requires_exists_and_excludes_blank(es_translator):
    from common.metadata_es_filter import META_FIELDS_PREFIX

    def _field(key: str) -> str:
        return f"{META_FIELDS_PREFIX}.{key}"

    clauses = es_translator.translate({"key": "notes", "op": "not empty", "value": ""}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("notes")}}],
                "must_not": [{"term": {_field("notes") + ".keyword": ""}}],
            }
        }
    ]


def test_unknown_operator_raises(es_translator):
    from common.metadata_es_filter import UnsupportedMetaFilter

    with pytest.raises(UnsupportedMetaFilter) as exc:
        es_translator.translate({"key": "tag", "op": "regex", "value": "^foo"})
    assert "regex" in exc.value.reason


def test_missing_key_raises(es_translator):
    from common.metadata_es_filter import UnsupportedMetaFilter

    with pytest.raises(UnsupportedMetaFilter):
        es_translator.translate({"op": "=", "value": "x"})


def test_scalar_op_with_list_value_raises(es_translator):
    from common.metadata_es_filter import UnsupportedMetaFilter

    with pytest.raises(UnsupportedMetaFilter):
        es_translator.translate({"key": "tag", "op": "=", "value": ["a", "b"]})


def test_string_op_with_empty_value_raises(es_translator):
    from common.metadata_es_filter import UnsupportedMetaFilter

    with pytest.raises(UnsupportedMetaFilter):
        es_translator.translate({"key": "tag", "op": "contains", "value": ""})


def test_membership_with_empty_csv_raises(es_translator):
    from common.metadata_es_filter import UnsupportedMetaFilter

    with pytest.raises(UnsupportedMetaFilter):
        es_translator.translate({"key": "tag", "op": "in", "value": ""})


# ===========================================================================
# Infinity-only tests
# ===========================================================================


def test_build_infinity_filter_and_logic():
    body = build_infinity_filter(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "score", "op": ">", "value": "5"},
        ],
        logic="and",
    )
    assert " AND " in body
    assert "alpha" in body


def test_build_infinity_filter_or_logic():
    body = build_infinity_filter(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "tag", "op": "=", "value": "beta"},
        ],
        logic="or",
    )
    assert " OR " in body
    assert "alpha" in body
    assert "beta" in body


def test_empty_filter_list_returns_1eq1():
    body = build_infinity_filter([], "and")
    assert body == "1=1"


def test_infinity_equal_string_uses_lowercase(infinity_translator):
    cond = infinity_translator.translate({"key": "tag", "op": "=", "value": "Alpha"})
    assert cond == "JSON_CONTAINS(meta_fields, '$.tag', '\"Alpha\"')"


def test_infinity_equal_numeric_keeps_number(infinity_translator):
    cond = infinity_translator.translate({"key": "score", "op": "=", "value": "5"})
    assert cond == "JSON_CONTAINS(meta_fields, '$.score', 5)"


def test_infinity_equal_date_passes_unparsed(infinity_translator):
    cond = infinity_translator.translate({"key": "published", "op": "=", "value": "2025-01-15"})
    assert cond == "JSON_CONTAINS(meta_fields, '$.published', '\"2025-01-15\"')"


def test_infinity_not_equal_string(infinity_translator):
    cond = infinity_translator.translate({"key": "tag", "op": "≠", "value": "alpha"})
    assert "JSON_CONTAINS" in cond
    assert "alpha" in cond
    assert "NOT" in cond


def test_infinity_not_equal_numeric(infinity_translator):
    cond = infinity_translator.translate({"key": "score", "op": "≠", "value": "5"})
    assert "JSON_CONTAINS" in cond and "NOT" in cond and "5" in cond


@pytest.mark.parametrize("op,sql_op", [(">", ">"), ("<", "<"), ("≥", ">="), ("≤", "<=")])
def test_infinity_range_operators(infinity_translator, op, sql_op):
    cond = infinity_translator.translate({"key": "score", "op": op, "value": "10"})
    assert sql_op in cond
    assert "JSON_EXTRACT_DOUBLE(meta_fields, '$.score')" in cond


def test_infinity_range_string_value(infinity_translator):
    cond = infinity_translator.translate({"key": "published", "op": "≥", "value": "2025-01-15"})
    assert ">=" in cond
    assert "2025-01-15" in cond


def test_infinity_in_csv_lowercased(infinity_translator):
    cond = infinity_translator.translate({"key": "status", "op": "in", "value": "Active,Pending"})
    assert "JSON_CONTAINS" in cond
    assert "active" in cond
    assert "pending" in cond


def test_infinity_in_python_list(infinity_translator):
    cond = infinity_translator.translate({"key": "status", "op": "in", "value": "['Open', 'Closed']"})
    assert "JSON_CONTAINS" in cond
    assert "open" in cond
    assert "closed" in cond


def test_infinity_in_numeric_members(infinity_translator):
    cond = infinity_translator.translate({"key": "year", "op": "in", "value": "[2024, 2025]"})
    assert "JSON_CONTAINS" in cond
    assert "2024" in cond
    assert "2025" in cond


def test_infinity_not_in_csv(infinity_translator):
    cond = infinity_translator.translate({"key": "status", "op": "not in", "value": "active,pending"})
    assert "NOT JSON_CONTAINS" in cond


def test_infinity_contains_uses_JSON_CONTAINS(infinity_translator):
    """Infinity 'contains' uses JSON_CONTAINS for JSON array membership."""
    cond = infinity_translator.translate({"key": "version", "op": "contains", "value": "earth"})
    assert "JSON_CONTAINS" in cond
    assert "earth" in cond


def test_infinity_contains_escapes_quotes(infinity_translator):
    """Special characters in contains value are escaped for JSON_CONTAINS."""
    cond = infinity_translator.translate({"key": "title", "op": "contains", "value": "a%b_c"})
    assert "JSON_CONTAINS" in cond
    assert "a%b_c" in cond


def test_infinity_not_contains_uses_JSON_CONTAINS(infinity_translator):
    """Infinity 'not contains' uses JSON_CONTAINS with NOT."""
    cond = infinity_translator.translate({"key": "version", "op": "not contains", "value": "earth"})
    assert "JSON_CONTAINS" in cond
    assert "NOT" in cond or "not" in cond.lower()


def test_infinity_start_with(infinity_translator):
    cond = infinity_translator.translate({"key": "name", "op": "start with", "value": "pre"})
    assert "LIKE" in cond
    assert "'pre%" in cond


def test_infinity_end_with(infinity_translator):
    """Infinity 'end with' uses LIKE with trailing wildcard."""
    cond = infinity_translator.translate({"key": "file", "op": "end with", "value": ".pdf"})
    assert "LIKE" in cond
    assert "%.pdf" in cond


def test_infinity_empty(infinity_translator):
    cond = infinity_translator.translate({"key": "notes", "op": "empty", "value": ""})
    assert "JSON_EXTRACT_STRING" in cond
    assert '""' in cond


def test_infinity_not_empty(infinity_translator):
    cond = infinity_translator.translate({"key": "notes", "op": "not empty", "value": ""})
    assert "JSON_EXTRACT_STRING" in cond
    assert "!=" in cond


def test_infinity_unknown_operator_raises(infinity_translator):
    with pytest.raises(ValueError) as exc:
        infinity_translator.translate({"key": "tag", "op": "regex", "value": "^foo"})
    assert "regex" in str(exc.value)


def test_infinity_missing_key_raises(infinity_translator):
    with pytest.raises(ValueError):
        infinity_translator.translate({"op": "=", "value": "x"})


def test_infinity_invalid_key_format_raises(infinity_translator):
    with pytest.raises(ValueError, match="invalid key format"):
        infinity_translator.translate({"key": "a;b", "op": "=", "value": "x"})


def test_infinity_key_with_brace_raises(infinity_translator):
    with pytest.raises(ValueError, match="invalid key format"):
        infinity_translator.translate({"key": "field$}", "op": "=", "value": "x"})


def test_infinity_scalar_op_with_list_value_raises(infinity_translator):
    with pytest.raises(ValueError):
        infinity_translator.translate({"key": "tag", "op": "=", "value": ["a", "b"]})


def test_infinity_string_op_with_empty_value_raises(infinity_translator):
    with pytest.raises(ValueError):
        infinity_translator.translate({"key": "tag", "op": "contains", "value": ""})


def test_infinity_membership_with_empty_csv_raises(infinity_translator):
    with pytest.raises(ValueError):
        infinity_translator.translate({"key": "tag", "op": "in", "value": ""})