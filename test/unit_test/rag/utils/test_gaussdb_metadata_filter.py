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
import pytest

from common import metadata_gaussdb_filter as gaussdb_filter
from common.metadata_gaussdb_filter import (
    GaussDBMetaFilterTranslator,
    SUPPORTED_OPERATORS,
    UnsupportedGaussDBMetaFilter,
    build_gaussdb_filter,
    fetch_gaussdb_metadata_doc_ids,
    is_pushdown_supported,
    jsonb_path_literal,
    normalize_gaussdb_meta_operator,
    plan_pushdown,
)

OP_NE = "\u2260"
OP_GE = "\u2265"
OP_LE = "\u2264"


def _assert_filter(filters, logic, expected_sql, expected_params):
    sql, params = build_gaussdb_filter(filters, logic)

    assert sql == expected_sql
    assert params == expected_params
    return sql


def _assert_rejected_filter(flt, expected_reason, logic="and", hidden_inputs=()):
    with pytest.raises(UnsupportedGaussDBMetaFilter) as exc_info:
        build_gaussdb_filter([flt], logic)

    assert exc_info.value.reason == expected_reason
    for hidden_input in hidden_inputs:
        assert hidden_input not in exc_info.value.reason


def test_tc_mgf_002_multiple_filters_join_with_and():
    sql = _assert_filter(
        [
            {"key": "status", "op": "=", "value": "active"},
            {"key": "amount", "op": ">", "value": 100},
        ],
        "and",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) AND "
        "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' "
        "THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
        ["active", "active", 100],
    )

    assert "active" not in sql
    assert "100" not in sql


def test_tc_mgf_003_multiple_filters_join_with_or():
    _assert_filter(
        [
            {"key": "status", "op": "=", "value": "a"},
            {"key": "status", "op": "=", "value": "b"},
        ],
        "or",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
        ["a", "a", "b", "b"],
    )


def test_tc_mgf_004_empty_filter_list_returns_tautology():
    _assert_filter([], "and", "1=1", [])


def test_tc_mgf_005_nested_key_path_uses_jsonb_path_literal():
    sql = _assert_filter(
        [{"key": "profile.name", "op": "=", "value": "Alice"}],
        "and",
        "(lower(meta_fields #>> '{profile,name}') = %s OR jsonb_exists(meta_fields #> '{profile,name}', %s))",
        ["alice", "alice"],
    )

    assert jsonb_path_literal("profile.name") == "'{profile,name}'"
    assert "Alice" not in sql
    assert "alice" not in sql


def test_tc_mgf_006_custom_jsonb_column_is_used_for_translation():
    translated = GaussDBMetaFilterTranslator(jsonb_column="chunk_data").translate({"key": "amount", "op": "=", "value": 5})

    assert translated.sql == "chunk_data #> '{amount}' @> %s::jsonb"
    assert translated.params == ["5"]
    assert "meta_fields" not in translated.sql


def test_tc_mgf_101_equal_string_translates_to_scalar_or_array_match():
    sql = _assert_filter(
        [{"key": "status", "op": "=", "value": "active"}],
        "and",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
        ["active", "active"],
    )

    assert "active" not in sql


def test_tc_mgf_102_not_equal_string_requires_existing_key_and_negated_match():
    sql = _assert_filter(
        [{"key": "status", "op": OP_NE, "value": "active"}],
        "and",
        "((meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)",
        ["active", "active"],
    )

    assert "active" not in sql


def test_tc_mgf_103_greater_than_number_uses_numeric_cast():
    sql = _assert_filter(
        [{"key": "amount", "op": ">", "value": 100}],
        "and",
        "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
        [100],
    )

    assert "100" not in sql


def test_tc_mgf_104_numeric_comparisons_use_canonical_sql_operators():
    cases = [
        (OP_GE, ">="),
        ("<", "<"),
        (OP_LE, "<="),
    ]

    for filter_op, sql_op in cases:
        sql = _assert_filter(
            [{"key": "amount", "op": filter_op, "value": 50}],
            "and",
            f"(CASE WHEN meta_fields #>> '{{amount}}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{{amount}}')::DOUBLE PRECISION {sql_op} %s ELSE FALSE END)",
            [50],
        )
        assert "50" not in sql


def test_tc_mgf_105_date_iso_prefix_uses_exact_timestamp_predicate():
    sql = _assert_filter(
        [{"key": "dt", "op": ">", "value": "2026-07"}],
        "and",
        "(CASE "
        "WHEN meta_fields #>> '{dt}' ~ '^[0-9]{4}-[0-9]{2}$' "
        "THEN to_timestamp(meta_fields #>> '{dt}' || '-01 00:00:00', 'YYYY-MM-DD HH24:MI:SS') "
        "WHEN meta_fields #>> '{dt}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' "
        "THEN to_timestamp(meta_fields #>> '{dt}' || ' 00:00:00', 'YYYY-MM-DD HH24:MI:SS') "
        "WHEN meta_fields #>> '{dt}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}$' "
        "THEN to_timestamp(replace(meta_fields #>> '{dt}', 'T', ' ') || ':00:00', 'YYYY-MM-DD HH24:MI:SS') "
        "WHEN meta_fields #>> '{dt}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}$' "
        "THEN to_timestamp(replace(meta_fields #>> '{dt}', 'T', ' ') || ':00', 'YYYY-MM-DD HH24:MI:SS') "
        "WHEN meta_fields #>> '{dt}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}$' "
        "THEN to_timestamp(replace(meta_fields #>> '{dt}', 'T', ' '), 'YYYY-MM-DD HH24:MI:SS') "
        "END > to_timestamp(%s, 'YYYY-MM-DD HH24:MI:SS'))",
        ["2026-07-01 00:00:00"],
    )

    assert gaussdb_filter.normalize_datetime_prefix("2026-07") == "2026-07-01 00:00:00"
    assert "2026-07" not in sql


def test_tc_mgf_106_contains_string_matches_array_member_or_substring():
    sql = _assert_filter(
        [{"key": "cust", "op": "contains", "value": "Acme"}],
        "and",
        "(jsonb_exists(meta_fields #> '{cust}', %s) OR lower(meta_fields #>> '{cust}') LIKE %s ESCAPE '\\')",
        ["acme", "%acme%"],
    )

    assert "Acme" not in sql
    assert "acme" not in sql


def test_tc_mgf_107_not_contains_requires_existing_key_and_negated_contains():
    sql = _assert_filter(
        [{"key": "cust", "op": "not contains", "value": "Acme"}],
        "and",
        "((meta_fields #> '{cust}') IS NOT NULL AND (jsonb_exists(meta_fields #> '{cust}', %s) OR lower(meta_fields #>> '{cust}') LIKE %s ESCAPE '\\') IS NOT TRUE)",
        ["acme", "%acme%"],
    )

    assert "Acme" not in sql
    assert "acme" not in sql


def test_tc_mgf_108_start_with_uses_escaped_like_prefix():
    sql = _assert_filter(
        [{"key": "cust", "op": "start with", "value": "Acme"}],
        "and",
        "(lower(meta_fields #>> '{cust}') LIKE %s ESCAPE '\\')",
        ["acme%"],
    )

    assert "Acme" not in sql
    assert "acme" not in sql


def test_tc_mgf_109_end_with_uses_escaped_like_suffix():
    sql = _assert_filter(
        [{"key": "cust", "op": "end with", "value": "Corp"}],
        "and",
        "(lower(meta_fields #>> '{cust}') LIKE %s ESCAPE '\\')",
        ["%corp"],
    )

    assert "Corp" not in sql
    assert "corp" not in sql


def test_tc_mgf_110_in_operator_expands_members_with_or():
    _assert_filter(
        [{"key": "status", "op": "in", "value": ["a", "b"]}],
        "and",
        "((lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)))",
        ["a", "a", "b", "b"],
    )


def test_tc_mgf_111_not_in_operator_requires_key_and_ands_members():
    _assert_filter(
        [{"key": "status", "op": "not in", "value": ["a", "b"]}],
        "and",
        "((meta_fields #> '{status}') IS NOT NULL AND "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE AND "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)",
        ["a", "a", "b", "b"],
    )


def test_tc_mgf_112_empty_operator_covers_all_empty_states_and_nested_missing_parent():
    status_sql = _assert_filter(
        [{"key": "status", "op": "empty", "value": None}],
        "and",
        "((meta_fields #> '{status}') IS NULL OR meta_fields #> '{status}' = 'null'::jsonb OR "
        "meta_fields #> '{status}' = '\"\"'::jsonb OR meta_fields #> '{status}' = '[]'::jsonb OR "
        "meta_fields #> '{status}' = '{}'::jsonb)",
        [],
    )
    nested_sql = _assert_filter(
        [{"key": "vendor.name", "op": "empty", "value": None}],
        "and",
        "((meta_fields #> '{vendor,name}') IS NULL OR "
        "meta_fields #> '{vendor,name}' = 'null'::jsonb OR "
        "meta_fields #> '{vendor,name}' = '\"\"'::jsonb OR "
        "meta_fields #> '{vendor,name}' = '[]'::jsonb OR "
        "meta_fields #> '{vendor,name}' = '{}'::jsonb)",
        [],
    )
    for sql in (status_sql, nested_sql):
        assert "= ''" not in sql
        assert "<> ''" not in sql


def test_tc_mgf_113_not_empty_operator_excludes_all_empty_states():
    sql = _assert_filter(
        [{"key": "status", "op": "not empty", "value": None}],
        "and",
        "((meta_fields #> '{status}') IS NOT NULL AND (meta_fields #> '{status}' = 'null'::jsonb) IS NOT TRUE AND "
        "(meta_fields #> '{status}' = '\"\"'::jsonb) IS NOT TRUE AND "
        "(meta_fields #> '{status}' = '[]'::jsonb) IS NOT TRUE AND "
        "(meta_fields #> '{status}' = '{}'::jsonb) IS NOT TRUE)",
        [],
    )
    assert "= ''" not in sql
    assert "<> ''" not in sql


def test_tc_mgf_201_equal_none_generates_json_null_equality():
    _assert_filter(
        [{"key": "status", "op": "=", "value": None}],
        "and",
        "((meta_fields #> '{status}') IS NOT NULL AND meta_fields #> '{status}' = 'null'::jsonb)",
        [],
    )
    assert gaussdb_filter.coerce_scalar_value(None, {"key": "status", "op": "=", "value": None}) is None


def test_tc_mgf_202_equal_empty_string_uses_jsonb_empty_string_literal():
    sql = _assert_filter(
        [{"key": "status", "op": "=", "value": ""}],
        "and",
        "(meta_fields #> '{status}' = '\"\"'::jsonb)",
        [],
    )

    assert "= ''" not in sql


def test_tc_mgf_301_equal_integer_uses_jsonb_containment():
    sql = _assert_filter(
        [{"key": "amount", "op": "=", "value": 5}],
        "and",
        "(meta_fields #> '{amount}' @> %s::jsonb)",
        ["5"],
    )

    assert gaussdb_filter.coerce_scalar_value(5, {"key": "amount", "op": "=", "value": 5}) == 5
    assert gaussdb_filter.jsonb_param(5) == "5"
    assert "5" not in sql


def test_tc_mgf_302_equal_boolean_uses_jsonb_containment():
    sql = _assert_filter(
        [{"key": "flag", "op": "=", "value": True}],
        "and",
        "(meta_fields #> '{flag}' @> %s::jsonb)",
        ["true"],
    )

    assert "true" not in sql
    assert "True" not in sql


def test_tc_mgf_303_equal_numeric_string_uses_jsonb_numeric_containment():
    flt = {"key": "n", "op": "=", "value": "123"}
    sql = _assert_filter(
        [flt],
        "and",
        "(meta_fields #> '{n}' @> %s::jsonb)",
        ["123"],
    )

    assert gaussdb_filter.coerce_scalar_value("123", flt) == 123
    assert "123" not in sql


def test_tc_mgf_304_equal_python_literals_differ_from_lowercase_json_words():
    cases = [
        ("True", True, "(meta_fields #> '{n}' @> %s::jsonb)", ["true"]),
        ("False", False, "(meta_fields #> '{n}' @> %s::jsonb)", ["false"]),
        (
            "None",
            None,
            "((meta_fields #> '{n}') IS NOT NULL AND meta_fields #> '{n}' = 'null'::jsonb)",
            [],
        ),
        (
            "true",
            "true",
            "(lower(meta_fields #>> '{n}') = %s OR jsonb_exists(meta_fields #> '{n}', %s))",
            ["true", "true"],
        ),
        (
            "null",
            "null",
            "(lower(meta_fields #>> '{n}') = %s OR jsonb_exists(meta_fields #> '{n}', %s))",
            ["null", "null"],
        ),
    ]

    for value, expected_value, expected_sql, expected_params in cases:
        flt = {"key": "n", "op": "=", "value": value}
        sql = _assert_filter([flt], "and", expected_sql, expected_params)
        coerced = gaussdb_filter.coerce_scalar_value(value, flt)

        if isinstance(expected_value, str):
            assert coerced == expected_value
        else:
            assert coerced is expected_value
        assert value not in sql


def test_tc_mgf_305_range_accepts_negative_number():
    sql = _assert_filter(
        [{"key": "amount", "op": ">", "value": -10}],
        "and",
        "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
        [-10],
    )

    assert "-10" not in sql


def test_tc_mgf_306_range_accepts_decimal_number():
    sql = _assert_filter(
        [{"key": "amount", "op": ">", "value": 1.5}],
        "and",
        "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
        [1.5],
    )

    assert "1.5" not in sql


def test_tc_mgf_307_range_numeric_strings_stay_on_numeric_branch():
    for value, expected_param in (("100", 100), ("2026", 2026), ("01", 1), ("01.5", 1.5)):
        flt = {"key": "amount", "op": ">", "value": value}
        sql = _assert_filter(
            [flt],
            "and",
            "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
            [expected_param],
        )
        assert gaussdb_filter.coerce_range_value(value, flt) == ("number", expected_param)
        assert "to_timestamp" not in sql
        assert value not in sql


def test_tc_mgf_308_date_iso_prefixes_normalize_to_exact_timestamp_parameters():
    cases = [
        ("2026-07", "2026-07-01 00:00:00"),
        ("2026-07-08", "2026-07-08 00:00:00"),
        ("2026-07-08T10", "2026-07-08 10:00:00"),
        ("2026-07-08 10", "2026-07-08 10:00:00"),
        ("2026-07-08T10:30", "2026-07-08 10:30:00"),
        ("2026-07-08 10:30:45", "2026-07-08 10:30:45"),
    ]

    for value, expected_param in cases:
        _assert_filter(
            [{"key": "published_at", "op": ">", "value": value}],
            "and",
            "(CASE "
            "WHEN meta_fields #>> '{published_at}' ~ '^[0-9]{4}-[0-9]{2}$' "
            "THEN to_timestamp(meta_fields #>> '{published_at}' || '-01 00:00:00', 'YYYY-MM-DD HH24:MI:SS') "
            "WHEN meta_fields #>> '{published_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' "
            "THEN to_timestamp(meta_fields #>> '{published_at}' || ' 00:00:00', 'YYYY-MM-DD HH24:MI:SS') "
            "WHEN meta_fields #>> '{published_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}$' "
            "THEN to_timestamp(replace(meta_fields #>> '{published_at}', 'T', ' ') || ':00:00', 'YYYY-MM-DD HH24:MI:SS') "
            "WHEN meta_fields #>> '{published_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}$' "
            "THEN to_timestamp(replace(meta_fields #>> '{published_at}', 'T', ' ') || ':00', 'YYYY-MM-DD HH24:MI:SS') "
            "WHEN meta_fields #>> '{published_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}$' "
            "THEN to_timestamp(replace(meta_fields #>> '{published_at}', 'T', ' '), 'YYYY-MM-DD HH24:MI:SS') "
            "END > to_timestamp(%s, 'YYYY-MM-DD HH24:MI:SS'))",
            [expected_param],
        )
        assert gaussdb_filter.normalize_datetime_prefix(value) == expected_param


def test_tc_mgf_309_invalid_datetime_prefix_is_rejected_as_range_value():
    flt = {"key": "dt", "op": ">", "value": "2026-13-01"}

    _assert_rejected_filter(
        flt,
        "unsupported range comparison value",
        hidden_inputs=("2026-13-01",),
    )


def test_tc_mgf_310_in_operator_splits_comma_separated_string_members():
    sql = _assert_filter(
        [{"key": "status", "op": "in", "value": "a,b,c"}],
        "and",
        "((lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)))",
        ["a", "a", "b", "b", "c", "c"],
    )

    assert "a,b,c" not in sql


def test_tc_mgf_311_in_operator_parses_json_array_string_members():
    sql = _assert_filter(
        [{"key": "n", "op": "in", "value": "[1,2]"}],
        "and",
        "((meta_fields #> '{n}' @> %s::jsonb) OR (meta_fields #> '{n}' @> %s::jsonb))",
        ["1", "2"],
    )

    assert "[1,2]" not in sql


def test_tc_mgf_312_equal_decimal_string_uses_jsonb_numeric_containment():
    sql = _assert_filter(
        [{"key": "n", "op": "=", "value": "1.5"}],
        "and",
        "(meta_fields #> '{n}' @> %s::jsonb)",
        ["1.5"],
    )

    assert "1.5" not in sql


def test_tc_mgf_313_in_operator_accepts_single_scalar_member():
    sql = _assert_filter(
        [{"key": "n", "op": "in", "value": 7}],
        "and",
        "((meta_fields #> '{n}' @> %s::jsonb))",
        ["7"],
    )

    assert sql.count("@> %s::jsonb") == 1
    assert "7" not in sql


def test_tc_mgf_401_and_logic_joins_three_mixed_operators_in_order():
    sql = _assert_filter(
        [
            {"key": "status", "op": "=", "value": "active"},
            {"key": "amount", "op": ">", "value": 100},
            {"key": "cust", "op": "contains", "value": "acme"},
        ],
        "and",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) AND "
        "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' "
        "THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END) AND "
        "(jsonb_exists(meta_fields #> '{cust}', %s) OR "
        "lower(meta_fields #>> '{cust}') LIKE %s ESCAPE '\\')",
        ["active", "active", 100, "acme", "%acme%"],
    )

    assert "active" not in sql
    assert "100" not in sql
    assert "acme" not in sql


def test_tc_mgf_402_or_logic_joins_three_filters_in_order():
    _assert_filter(
        [
            {"key": "status", "op": "=", "value": "a"},
            {"key": "status", "op": "=", "value": "b"},
            {"key": "status", "op": "=", "value": "c"},
        ],
        "or",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
        ["a", "a", "b", "b", "c", "c"],
    )


def test_tc_mgf_403_and_logic_combines_not_in_with_positive_filter():
    _assert_filter(
        [
            {"key": "status", "op": "not in", "value": ["a", "b"]},
            {"key": "type", "op": "=", "value": "doc"},
        ],
        "and",
        "((meta_fields #> '{status}') IS NOT NULL AND "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE AND "
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE) AND "
        "(lower(meta_fields #>> '{type}') = %s OR jsonb_exists(meta_fields #> '{type}', %s))",
        ["a", "a", "b", "b", "doc", "doc"],
    )


def test_tc_mgf_404_nested_key_exists_and_jsonb_number_predicate_join_with_and():
    sql = _assert_filter(
        [
            {"key": "a.b", "op": OP_NE, "value": "x"},
            {"key": "a.c", "op": "=", "value": 1},
        ],
        "and",
        "((meta_fields #> '{a,b}') IS NOT NULL AND (lower(meta_fields #>> '{a,b}') = %s OR jsonb_exists(meta_fields #> '{a,b}', %s)) IS NOT TRUE) AND (meta_fields #> '{a,c}' @> %s::jsonb)",
        ["x", "x", "1"],
    )

    assert jsonb_path_literal("a.c") == "'{a,c}'"
    assert "'x'" not in sql


def test_tc_mgf_405_special_key_segments_are_encoded_without_sql_injection():
    malicious_segment = "owner') OR TRUE --"
    key = f"root.a,b.{malicious_segment}"
    path = "'{root,\"a,b\",\"owner'') OR TRUE --\"}'"

    sql = _assert_filter(
        [{"key": key, "op": OP_NE, "value": "x"}],
        "and",
        f"((meta_fields #> {path}) IS NOT NULL AND (lower(meta_fields #>> {path}) = %s OR jsonb_exists(meta_fields #> {path}, %s)) IS NOT TRUE)",
        ["x", "x"],
    )

    assert jsonb_path_literal(key) == path
    assert malicious_segment not in sql
    assert sql.count(path) == 3


def test_tc_mgf_501_is_alias_translates_to_equal_sql():
    sql = _assert_filter(
        [{"key": "status", "op": "is", "value": "active"}],
        "and",
        "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
        ["active", "active"],
    )

    assert normalize_gaussdb_meta_operator("is") == "="
    assert "active" not in sql


def test_tc_mgf_502_not_equal_aliases_translate_to_identical_sql():
    expected_sql = "((meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)"

    for op in ("is not", "not is", "!=", "<>"):
        sql = _assert_filter(
            [{"key": "status", "op": op, "value": "active"}],
            "and",
            expected_sql,
            ["active", "active"],
        )
        assert normalize_gaussdb_meta_operator(op) == OP_NE
        assert "active" not in sql


def test_tc_mgf_503_range_aliases_normalize_and_translate_to_canonical_operators():
    cases = [(">=", OP_GE, ">="), ("<=", OP_LE, "<=")]

    for alias, canonical, sql_op in cases:
        sql = _assert_filter(
            [{"key": "amount", "op": alias, "value": 50}],
            "and",
            f"(CASE WHEN meta_fields #>> '{{amount}}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{{amount}}')::DOUBLE PRECISION {sql_op} %s ELSE FALSE END)",
            [50],
        )
        assert normalize_gaussdb_meta_operator(alias) == canonical
        assert "50" not in sql


def test_tc_mgf_504_operator_normalization_tolerates_case_and_whitespace():
    cases = [
        (
            "  IS  NOT ",
            "((meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)",
            ["acme", "acme"],
        ),
        (
            "Contains",
            "(jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
            ["acme", "%acme%"],
        ),
        (
            "START WITH",
            "(lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
            ["acme%"],
        ),
    ]

    for op, expected_sql, expected_params in cases:
        sql = _assert_filter(
            [{"key": "status", "op": op, "value": "acme"}],
            "and",
            expected_sql,
            expected_params,
        )
        assert "acme" not in sql


def test_tc_mgf_505_before_after_operators_are_not_supported():
    for op in ("before", "after"):
        _assert_rejected_filter(
            {"key": "date", "op": op, "value": "2026-01-01"},
            f"unsupported metadata filter operator '{op}'",
        )


def test_tc_mgf_601_missing_key_is_rejected_with_stable_reason():
    _assert_rejected_filter(
        {"op": "=", "value": "private-metadata-value"},
        "invalid metadata key",
        hidden_inputs=("private-metadata-value",),
    )


def test_tc_mgf_602_missing_operator_is_rejected_with_exact_message():
    _assert_rejected_filter(
        {"key": "status", "value": "x"},
        "metadata filter operator is missing",
    )


def test_tc_mgf_603_unknown_operator_is_rejected_with_exact_message():
    _assert_rejected_filter(
        {"key": "status", "op": "regex", "value": "x"},
        "unsupported metadata filter operator 'regex'",
    )


def test_tc_mgf_604_numeric_leading_key_segment_is_encoded():
    _assert_filter(
        [{"key": "1abc", "op": "=", "value": 1}],
        "and",
        "(meta_fields #> '{1abc}' @> %s::jsonb)",
        ["1"],
    )
    assert jsonb_path_literal("1abc") == "'{1abc}'"


def test_tc_mgf_605_empty_or_nul_key_segments_are_rejected():
    cases = [
        ("a..b", "metadata key contains an empty path segment"),
        ("a.", "metadata key contains an empty path segment"),
        (".a", "metadata key contains an empty path segment"),
        ("a.\x00b", "metadata key contains NUL"),
    ]

    for key, expected_reason in cases:
        _assert_rejected_filter(
            {"key": key, "op": "=", "value": "x"},
            expected_reason,
            hidden_inputs=(key,),
        )


def test_tc_mgf_606_key_segment_with_space_is_encoded():
    path = "'{a,\"b c\"}'"
    _assert_filter(
        [{"key": "a.b c", "op": "=", "value": 1}],
        "and",
        f"(meta_fields #> {path} @> %s::jsonb)",
        ["1"],
    )
    assert jsonb_path_literal("a.b c") == path


def test_tc_mgf_607_equal_operator_rejects_non_scalar_value():
    _assert_rejected_filter(
        {"key": "private-key", "op": "=", "value": ["private-value"]},
        "scalar comparison value is non-scalar",
        hidden_inputs=("private-key", "private-value"),
    )


def test_tc_mgf_608_range_operator_rejects_none_value():
    _assert_rejected_filter(
        {"key": "private-key", "op": ">", "value": None},
        "range comparison value is None",
        hidden_inputs=("private-key",),
    )


def test_tc_mgf_609_range_operator_rejects_boolean_value():
    _assert_rejected_filter(
        {"key": "private-key", "op": ">", "value": True},
        "range comparison value is boolean",
        hidden_inputs=("private-key",),
    )


def test_tc_mgf_610_range_operator_rejects_non_numeric_non_date_string():
    _assert_rejected_filter(
        {"key": "private-key", "op": ">", "value": "private-range-value"},
        "unsupported range comparison value",
        hidden_inputs=("private-key", "private-range-value"),
    )


def test_tc_mgf_611_contains_operator_rejects_none_and_container_values():
    for value in (None, ["private-value"], {"private-value": True}):
        _assert_rejected_filter(
            {"key": "private-key", "op": "contains", "value": value},
            "string operator value must be a scalar",
            hidden_inputs=("private-key", "private-value"),
        )


def test_tc_mgf_612_contains_operator_rejects_empty_string():
    _assert_rejected_filter(
        {"key": "private-key", "op": "contains", "value": ""},
        "string operator value is empty",
        hidden_inputs=("private-key",),
    )


def test_tc_mgf_613_in_operator_rejects_none_value():
    _assert_rejected_filter(
        {"key": "private-key", "op": "in", "value": None},
        "membership value is None",
        hidden_inputs=("private-key",),
    )


def test_tc_mgf_614_in_operator_rejects_every_empty_resolved_member_list():
    for value in ([], "", ",,"):
        _assert_rejected_filter(
            {"key": "private-key", "op": "in", "value": value},
            "membership value resolved to empty list",
            hidden_inputs=("private-key",),
        )


def test_tc_mgf_615_unknown_logic_is_rejected():
    with pytest.raises(UnsupportedGaussDBMetaFilter) as exc_info:
        build_gaussdb_filter([], "xor")

    assert str(exc_info.value) == "unknown logic 'xor'"


def test_tc_mgf_616_invalid_jsonb_column_is_rejected():
    with pytest.raises(UnsupportedGaussDBMetaFilter) as exc_info:
        GaussDBMetaFilterTranslator(jsonb_column="chunk_data;drop")

    assert str(exc_info.value) == "invalid JSONB column 'chunk_data;drop'"


def test_tc_mgf_617_value_with_sql_metacharacters_is_bound_and_like_escaped():
    value = r"x%_\'; DROP TABLE--"
    sql = _assert_filter(
        [{"key": "status", "op": "contains", "value": value}],
        "and",
        "(jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
        [r"x%_\'; drop table--", r"%x\%\_\\'; drop table--%"],
    )

    assert "DROP TABLE" not in sql
    assert "drop table" not in sql
    assert "x%_" not in sql


def test_tc_mgf_618_non_ascii_and_at_sign_key_segments_are_encoded():
    cases = [
        ("字段", "'{\"字段\"}'"),
        ("field@name", "'{\"field@name\"}'"),
    ]

    for key, path in cases:
        _assert_filter(
            [{"key": key, "op": "=", "value": 1}],
            "and",
            f"(meta_fields #> {path} @> %s::jsonb)",
            ["1"],
        )
        assert jsonb_path_literal(key) == path


def test_tc_mgf_701_is_pushdown_supported_returns_true_for_fully_translatable_filter():
    filters = [{"key": "a", "op": "=", "value": "x"}]

    assert is_pushdown_supported(filters) is True
    plan = plan_pushdown(filters, "and")
    assert plan.logic == "and"
    assert len(plan.translated) == 1
    assert plan.translated[0].sql == "lower(meta_fields #>> '{a}') = %s OR jsonb_exists(meta_fields #> '{a}', %s)"
    assert plan.translated[0].params == ["x", "x"]
    _assert_filter(
        filters,
        "and",
        "(lower(meta_fields #>> '{a}') = %s OR jsonb_exists(meta_fields #> '{a}', %s))",
        ["x", "x"],
    )


def test_tc_mgf_702_is_pushdown_supported_returns_false_without_raising():
    filters = [{"key": "a", "op": "before", "value": "x"}]

    assert is_pushdown_supported(filters) is False
    _assert_rejected_filter(
        filters[0],
        "unsupported metadata filter operator 'before'",
    )


def test_tc_mgf_704_all_supported_operators_normalize_and_translate():
    expected_operators = frozenset(
        {
            "=",
            "\u2260",
            ">",
            "\u2265",
            "<",
            "\u2264",
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
    cases = [
        (
            "=",
            "status",
            "x",
            "lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)",
            ["x", "x"],
        ),
        (
            OP_NE,
            "status",
            "x",
            "(meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE",
            ["x", "x"],
        ),
        (
            ">",
            "amount",
            1,
            "CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END",
            [1],
        ),
        (
            OP_GE,
            "amount",
            1,
            "CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION >= %s ELSE FALSE END",
            [1],
        ),
        (
            "<",
            "amount",
            1,
            "CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION < %s ELSE FALSE END",
            [1],
        ),
        (
            OP_LE,
            "amount",
            1,
            "CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION <= %s ELSE FALSE END",
            [1],
        ),
        (
            "in",
            "status",
            ["x"],
            "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
            ["x", "x"],
        ),
        (
            "not in",
            "status",
            ["x"],
            "(meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE",
            ["x", "x"],
        ),
        (
            "contains",
            "status",
            "x",
            "jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\'",
            ["x", "%x%"],
        ),
        (
            "not contains",
            "status",
            "x",
            "(meta_fields #> '{status}') IS NOT NULL AND (jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\') IS NOT TRUE",
            ["x", "%x%"],
        ),
        (
            "start with",
            "status",
            "x",
            "lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\'",
            ["x%"],
        ),
        (
            "end with",
            "status",
            "x",
            "lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\'",
            ["%x"],
        ),
        (
            "empty",
            "status",
            None,
            "(meta_fields #> '{status}') IS NULL OR meta_fields #> '{status}' = 'null'::jsonb OR "
            "meta_fields #> '{status}' = '\"\"'::jsonb OR meta_fields #> '{status}' = '[]'::jsonb OR "
            "meta_fields #> '{status}' = '{}'::jsonb",
            [],
        ),
        (
            "not empty",
            "status",
            None,
            "(meta_fields #> '{status}') IS NOT NULL AND (meta_fields #> '{status}' = 'null'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '\"\"'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '[]'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '{}'::jsonb) IS NOT TRUE",
            [],
        ),
    ]

    case_operators = {op for op, _, _, _, _ in cases}
    assert len(expected_operators) == 14
    assert len(cases) == 14
    assert SUPPORTED_OPERATORS == expected_operators
    assert case_operators == expected_operators
    translator = GaussDBMetaFilterTranslator()
    for op, key, value, expected_sql, expected_params in cases:
        assert normalize_gaussdb_meta_operator(op) == op
        translated = translator.translate({"key": key, "op": op, "value": value})
        assert translated.sql == expected_sql
        assert translated.params == expected_params


def test_tc_mgf_707_coerce_scalar_value_covers_python_literal_matrix():
    cases = [
        ("123", 123),
        ("1.5", 1.5),
        ("true", "true"),
        ("false", "false"),
        ("null", "null"),
        ("True", True),
        ("False", False),
        ("None", None),
        ("", ""),
        ("  ", "  "),
        ("[1,2]", "[1,2]"),
        ("abc", "abc"),
        (5, 5),
        (None, None),
    ]

    for value, expected in cases:
        assert gaussdb_filter.coerce_scalar_value(value, {"key": "a", "op": "=", "value": value}) == expected


def test_tc_mgf_708_normalize_datetime_prefix_covers_valid_and_invalid_matrix():
    cases = [
        ("2026", None),
        ("2026-07", "2026-07-01 00:00:00"),
        ("2026-07-08 10:30:45", "2026-07-08 10:30:45"),
        ("2026-13-01", None),
        ("2026-07-32", None),
        ("abc", None),
    ]

    for value, expected in cases:
        assert gaussdb_filter.normalize_datetime_prefix(value) == expected


def test_tc_mgf_709_fetch_gaussdb_metadata_doc_ids_delegates_public_boundary():
    expected_doc_ids = ["doc-1", "doc-2"]

    class RecordingDocStore:
        def __init__(self):
            self.calls = []

        def fetch_metadata_doc_ids(self, *args):
            self.calls.append(args)
            return expected_doc_ids

    doc_store = RecordingDocStore()
    kb_ids = ["kb-1", "kb-2"]
    filter_params = ["active", "active"]
    result = fetch_gaussdb_metadata_doc_ids(
        doc_store,
        "ragflow_doc_meta_tenant",
        kb_ids,
        "lower(meta_fields #>> '{status}') = %s",
        filter_params,
        101,
    )

    assert result is expected_doc_ids
    assert doc_store.calls == [
        (
            "ragflow_doc_meta_tenant",
            kb_ids,
            "lower(meta_fields #>> '{status}') = %s",
            filter_params,
            101,
        )
    ]


def test_tc_mgf_810_all_operators_avoid_bare_empty_string_sql():
    cases = [
        (
            {"key": "status", "op": "=", "value": "x"},
            "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s))",
            ["x", "x"],
        ),
        (
            {"key": "status", "op": OP_NE, "value": "x"},
            "((meta_fields #> '{status}') IS NOT NULL AND (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)",
            ["x", "x"],
        ),
        (
            {"key": "amount", "op": ">", "value": 100},
            "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION > %s ELSE FALSE END)",
            [100],
        ),
        (
            {"key": "amount", "op": OP_GE, "value": 100},
            "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION >= %s ELSE FALSE END)",
            [100],
        ),
        (
            {"key": "amount", "op": "<", "value": 100},
            "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION < %s ELSE FALSE END)",
            [100],
        ),
        (
            {"key": "amount", "op": OP_LE, "value": 100},
            "(CASE WHEN meta_fields #>> '{amount}' ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN (meta_fields #>> '{amount}')::DOUBLE PRECISION <= %s ELSE FALSE END)",
            [100],
        ),
        (
            {"key": "status", "op": "contains", "value": "x"},
            "(jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
            ["x", "%x%"],
        ),
        (
            {"key": "status", "op": "not contains", "value": "x"},
            "((meta_fields #> '{status}') IS NOT NULL AND (jsonb_exists(meta_fields #> '{status}', %s) OR lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\') IS NOT TRUE)",
            ["x", "%x%"],
        ),
        (
            {"key": "status", "op": "start with", "value": "x"},
            "(lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
            ["x%"],
        ),
        (
            {"key": "status", "op": "end with", "value": "x"},
            "(lower(meta_fields #>> '{status}') LIKE %s ESCAPE '\\')",
            ["%x"],
        ),
        (
            {"key": "status", "op": "empty", "value": None},
            "((meta_fields #> '{status}') IS NULL OR meta_fields #> '{status}' = 'null'::jsonb OR "
            "meta_fields #> '{status}' = '\"\"'::jsonb OR meta_fields #> '{status}' = '[]'::jsonb OR "
            "meta_fields #> '{status}' = '{}'::jsonb)",
            [],
        ),
        (
            {"key": "status", "op": "not empty", "value": None},
            "((meta_fields #> '{status}') IS NOT NULL AND (meta_fields #> '{status}' = 'null'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '\"\"'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '[]'::jsonb) IS NOT TRUE AND "
            "(meta_fields #> '{status}' = '{}'::jsonb) IS NOT TRUE)",
            [],
        ),
        (
            {"key": "status", "op": "in", "value": ["x", "y"]},
            "((lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) OR (lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)))",
            ["x", "x", "y", "y"],
        ),
        (
            {"key": "status", "op": "not in", "value": ["x", "y"]},
            "((meta_fields #> '{status}') IS NOT NULL AND "
            "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE AND "
            "(lower(meta_fields #>> '{status}') = %s OR jsonb_exists(meta_fields #> '{status}', %s)) IS NOT TRUE)",
            ["x", "x", "y", "y"],
        ),
    ]

    assert len(cases) == 14
    for flt, expected_sql, expected_params in cases:
        sql = _assert_filter([flt], "and", expected_sql, expected_params)
        assert "= ''" not in sql
        assert "<> ''" not in sql
