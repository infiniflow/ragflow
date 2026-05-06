"""Unit tests for the Elasticsearch push-down translator.

These tests cover the public surface of ``common.metadata_es_filter`` without
touching the live ES cluster. They verify the shape of the produced query DSL
operator-by-operator and confirm that the parity rules with the in-memory
``meta_filter`` (lower-casing, list-membership coercion, date detection) hold.
"""

import pytest

from common.metadata_es_filter import (
    META_FIELDS_PREFIX,
    MetaFilterPushdownPlan,
    MetaFilterTranslator,
    SUPPORTED_OPERATORS,
    UnsupportedMetaFilter,
    build_meta_filter_query,
    extract_doc_ids,
    is_pushdown_supported,
    plan_pushdown,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def translator() -> MetaFilterTranslator:
    return MetaFilterTranslator()


def _field(key: str) -> str:
    return f"{META_FIELDS_PREFIX}.{key}"


# ---------------------------------------------------------------------------
# Translator: per-operator shape
# ---------------------------------------------------------------------------


def test_equal_translates_to_term_with_lowercased_value(translator):
    """String equality runs against ``.keyword`` so multi-word phrases match.

    Querying the analyzed parent field with ``term`` only matches docs whose
    inverted index contains the literal phrase token, which never happens for
    multi-word values. The ``.keyword`` sub-field stores the unmodified string,
    and ``case_insensitive: true`` keeps the lower-cased compare semantics from
    the in-memory ``meta_filter``.
    """
    clauses = translator.translate({"key": "tag", "op": "=", "value": "Alpha"}).to_clauses()
    assert clauses == [
        {"term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}}
    ]


def test_equal_parses_numeric_literal(translator):
    """Numeric values stay on the parent path — no ``.keyword`` sub-field exists for ``long``."""
    clauses = translator.translate({"key": "score", "op": "=", "value": "5"}).to_clauses()
    assert clauses == [{"term": {_field("score"): 5}}]


def test_equal_multiword_uses_keyword_subfield(translator):
    """Regression for qinling0210's report: multi-word string values must match.

    Before the keyword-routing fix this emitted
    ``term: meta_fields.author = "alice wonderland"`` against an analyzed text
    field, which never matched (inverted index only contained per-token
    entries). Routing through ``.keyword`` preserves the full phrase.
    """
    clauses = translator.translate(
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


def test_not_equal_requires_field_to_exist(translator):
    clauses = translator.translate({"key": "tag", "op": "≠", "value": "alpha"}).to_clauses()
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
def test_range_operator_translation(translator, op, es_key):
    # Multi-clause positive filters wrap into a single bool so OR-logic
    # parents can't match on just the ``exists`` half of the range.
    clauses = translator.translate({"key": "score", "op": op, "value": "10"}).to_clauses()
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


def test_range_passes_iso_date_through_unparsed(translator):
    clauses = translator.translate({"key": "published", "op": "≥", "value": "2025-01-15"}).to_clauses()
    range_clause = clauses[0]["bool"]["must"][1]
    assert range_clause == {"range": {_field("published"): {"gte": "2025-01-15"}}}


def _string_terms_should(field_path: str, members):
    """``in``/``not in`` over string members expands per-element so each ``term``
    can carry ``case_insensitive`` (``terms`` does not accept that flag)."""
    return {
        "bool": {
            "should": [
                {"term": {field_path + ".keyword": {"value": m, "case_insensitive": True}}}
                for m in members
            ],
            "minimum_should_match": 1,
        }
    }


def test_in_operator_csv_value_lowercased(translator):
    clauses = translator.translate({"key": "status", "op": "in", "value": "Active,Pending"}).to_clauses()
    assert clauses == [_string_terms_should(_field("status"), ["active", "pending"])]


def test_in_operator_python_list_literal(translator):
    clauses = translator.translate({"key": "status", "op": "in", "value": "['Open', 'Closed']"}).to_clauses()
    assert clauses == [_string_terms_should(_field("status"), ["open", "closed"])]


def test_in_operator_numeric_members_keep_terms(translator):
    """All-numeric member lists keep the cheaper ``terms`` form on the parent path."""
    clauses = translator.translate({"key": "year", "op": "in", "value": "[2024, 2025]"}).to_clauses()
    assert clauses == [{"terms": {_field("year"): [2024, 2025]}}]


def test_not_in_negates_with_existence_guard(translator):
    clauses = translator.translate({"key": "status", "op": "not in", "value": "active,pending"}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("status")}}],
                "must_not": [_string_terms_should(_field("status"), ["active", "pending"])],
            }
        }
    ]


def test_contains_uses_case_insensitive_wildcard(translator):
    clauses = translator.translate({"key": "version", "op": "contains", "value": "earth"}).to_clauses()
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


def test_contains_escapes_user_wildcards(translator):
    clauses = translator.translate({"key": "title", "op": "contains", "value": "a*b?c"}).to_clauses()
    pattern = clauses[0]["wildcard"][_field("title") + ".keyword"]["value"]
    assert pattern == "*a\\*b\\?c*"


def test_not_contains_negates_with_exists(translator):
    clauses = translator.translate({"key": "version", "op": "not contains", "value": "earth"}).to_clauses()
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


def test_start_with_uses_prefix(translator):
    clauses = translator.translate({"key": "name", "op": "start with", "value": "pre"}).to_clauses()
    assert clauses == [
        {"prefix": {_field("name") + ".keyword": {"value": "pre", "case_insensitive": True}}}
    ]


def test_end_with_uses_trailing_wildcard(translator):
    clauses = translator.translate({"key": "file", "op": "end with", "value": ".pdf"}).to_clauses()
    pattern = clauses[0]["wildcard"][_field("file") + ".keyword"]["value"]
    assert pattern == "*.pdf"


def test_empty_matches_missing_or_blank(translator):
    clauses = translator.translate({"key": "notes", "op": "empty", "value": ""}).to_clauses()
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


def test_not_empty_requires_exists_and_excludes_blank(translator):
    clauses = translator.translate({"key": "notes", "op": "not empty", "value": ""}).to_clauses()
    assert clauses == [
        {
            "bool": {
                "must": [{"exists": {"field": _field("notes")}}],
                "must_not": [{"term": {_field("notes") + ".keyword": ""}}],
            }
        }
    ]


# ---------------------------------------------------------------------------
# Translator: validation paths
# ---------------------------------------------------------------------------


def test_unknown_operator_raises(translator):
    with pytest.raises(UnsupportedMetaFilter) as exc:
        translator.translate({"key": "tag", "op": "regex", "value": "^foo"})
    assert "regex" in exc.value.reason


def test_missing_key_raises(translator):
    with pytest.raises(UnsupportedMetaFilter):
        translator.translate({"op": "=", "value": "x"})


def test_scalar_op_with_list_value_raises(translator):
    with pytest.raises(UnsupportedMetaFilter):
        translator.translate({"key": "tag", "op": "=", "value": ["a", "b"]})


def test_string_op_with_empty_value_raises(translator):
    with pytest.raises(UnsupportedMetaFilter):
        translator.translate({"key": "tag", "op": "contains", "value": ""})


def test_membership_with_empty_csv_raises(translator):
    with pytest.raises(UnsupportedMetaFilter):
        translator.translate({"key": "tag", "op": "in", "value": ""})


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


# ---------------------------------------------------------------------------
# Plan composition
# ---------------------------------------------------------------------------


def test_plan_emits_must_clauses_for_and_logic():
    plan = plan_pushdown(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "score", "op": ">", "value": "5"},
        ],
        logic="and",
    )
    assert isinstance(plan, MetaFilterPushdownPlan)
    body = plan.to_query(["kb1"])
    bool_root = body["query"]["bool"]
    assert bool_root["filter"][0] == {"terms": {"kb_id": ["kb1"]}}
    inner = bool_root["filter"][1]["bool"]
    assert "must" in inner
    # Each translated filter contributes exactly one clause to the parent bool:
    # ``=`` is a single ``term``; ``>`` is wrapped into one atomic ``bool``.
    assert len(inner["must"]) == 2
    expected_tag_term = {
        "term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}
    }
    assert expected_tag_term in inner["must"]
    range_wrap = {
        "bool": {
            "must": [
                {"exists": {"field": _field("score")}},
                {"range": {_field("score"): {"gt": 5}}},
            ]
        }
    }
    assert range_wrap in inner["must"]


def test_range_filter_under_or_stays_atomic():
    """An OR'd range must not split into independent ``exists`` + ``range`` should branches."""
    body = build_meta_filter_query(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "score", "op": ">", "value": "5"},
        ],
        logic="or",
        kb_ids=["kb1"],
    )
    should = body["query"]["bool"]["filter"][1]["bool"]["should"]
    # Two filters → two should branches, not three or four.
    assert len(should) == 2
    assert {
        "term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}
    } in should


def test_plan_emits_should_clauses_for_or_logic():
    plan = plan_pushdown(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "tag", "op": "=", "value": "beta"},
        ],
        logic="or",
    )
    inner = plan.to_query(["kb1"])["query"]["bool"]["filter"][1]["bool"]
    assert inner["minimum_should_match"] == 1
    assert len(inner["should"]) == 2


def test_unknown_logic_rejected():
    with pytest.raises(UnsupportedMetaFilter):
        plan_pushdown([{"key": "k", "op": "=", "value": "v"}], logic="xor")


def test_empty_filter_list_returns_kb_only_query():
    body = build_meta_filter_query([], "and", ["kb1", "kb2"])
    assert body == {"query": {"bool": {"filter": [{"terms": {"kb_id": ["kb1", "kb2"]}}]}}}


def test_negative_filter_in_or_logic_keeps_negation_scope():
    """Wrapping ``≠`` in an OR should not let the ``must_not`` swallow other branches.

    ``≠`` is rejected by :func:`is_pushdown_supported` for multi-value safety, so
    this test exercises the translator directly to confirm the per-filter
    wrapping invariant. The same shape protects ``not contains`` (which IS
    pushed down) from leaking its ``must_not`` into a parent should.
    """
    body = build_meta_filter_query(
        [
            {"key": "tag", "op": "=", "value": "alpha"},
            {"key": "tag", "op": "≠", "value": "beta"},
        ],
        logic="or",
        kb_ids=["kb1"],
    )
    inner = body["query"]["bool"]["filter"][1]["bool"]
    should = inner["should"]
    assert should[0] == {
        "term": {_field("tag") + ".keyword": {"value": "alpha", "case_insensitive": True}}
    }
    # The ≠ branch is wrapped so its must_not does not bleed into the OR set.
    assert "bool" in should[1]
    assert "must_not" in should[1]["bool"]


# ---------------------------------------------------------------------------
# is_pushdown_supported pre-check
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


@pytest.mark.parametrize("op", ["≠", "not in"])
def test_pushdown_check_rejects_multivalue_unsafe_negatives(op):
    """Negatives that diverge on multi-valued fields force the in-memory fallback."""
    assert not is_pushdown_supported([{"key": "tag", "op": op, "value": "x"}])


def test_pushdown_check_one_unsafe_op_rejects_whole_request():
    """Mixing one unsafe op with safe ones still falls back, preserving correctness."""
    assert not is_pushdown_supported(
        [
            {"key": "tag", "op": "=", "value": "v"},
            {"key": "tag", "op": "≠", "value": "w"},
        ]
    )


def test_pushdown_check_accepts_not_contains():
    """``not contains`` stays in push-down; ``all(not contains)`` ≡ ``not any(contains)``."""
    assert is_pushdown_supported([{"key": "tag", "op": "not contains", "value": "x"}])


# ---------------------------------------------------------------------------
# extract_doc_ids
# ---------------------------------------------------------------------------


def test_extract_doc_ids_from_dict_response():
    response = {
        "hits": {
            "hits": [
                {"_id": "doc1", "_source": {"id": "doc1"}},
                {"_id": "doc2", "_source": {"id": "doc2"}},
            ]
        }
    }
    assert extract_doc_ids(response) == ["doc1", "doc2"]


def test_extract_doc_ids_falls_back_to_source_id():
    response = {"hits": {"hits": [{"_source": {"id": "src-id"}}]}}
    assert extract_doc_ids(response) == ["src-id"]


def test_extract_doc_ids_empty_response():
    assert extract_doc_ids({}) == []
    assert extract_doc_ids({"hits": {}}) == []
    assert extract_doc_ids({"hits": {"hits": []}}) == []
