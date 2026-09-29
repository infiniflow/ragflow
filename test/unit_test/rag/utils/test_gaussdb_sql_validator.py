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
import pytest

from common.doc_store import gaussdb_conn_base as gaussdb_base
from common.doc_store.gaussdb_conn_base import (
    ExposedGaussDBTable,
    GaussDBSQLValidator,
    UnsafeGaussDBSQL,
    is_gaussdb_aggregate_sql,
    jsonb_path_literal,
    mask_gaussdb_text,
)


def _validator(kb_ids=None, field_map=None, fetch_size=128):
    table = ExposedGaussDBTable.from_field_map(
        physical_name="ragflow_t1",
        kb_ids=["kb-1"] if kb_ids is None else kb_ids,
        field_map={"amount": "number", "dept": "string", "customer,name": "string"} if field_map is None else field_map,
    )
    return GaussDBSQLValidator({table.logical_name: table}, default_limit=fetch_size)


def _assert_unsafe_sql(sql, message, validator=None):
    with pytest.raises(UnsafeGaussDBSQL) as exc_info:
        (validator or _validator()).validate_and_patch(sql)

    assert str(exc_info.value) == message


def test_tc_cfg_711_mask_gaussdb_text_redacts_quoted_unquoted_and_delimited_secrets():
    raw = (
        "password=plain,semi;value "
        "passwd='quoted, semi; value' "
        'token="token, semi; value" '
        '"access_token": "access, semi; value", '
        "api_key=api,semi;value "
        "secret='secret, semi; value' "
        "dsn=postgresql://user:dsn-secret@db.internal:5432/ragflow?options=a,b;c"
    )

    assert mask_gaussdb_text(raw) == ('password=*** passwd=\'***\' token="***" "access_token": "***", api_key=*** secret=\'***\' dsn=***')
    assert mask_gaussdb_text("uri=gaussdb://user:secret@host/db?x=a,b;c") == "uri=***"
    assert mask_gaussdb_text("password='quoted ''value,semi;tail' next=ok") == "password='***' next=ok"
    assert mask_gaussdb_text('token="unterminated, semi; value') == "token=***"


def test_tc_sql_412_jsonb_path_literal_encodes_special_keys():
    assert jsonb_path_literal(["amount"]) == "'{amount}'"
    assert jsonb_path_literal(["customer", "name"]) == "'{customer,name}'"
    assert jsonb_path_literal(["customer,name"]) == "'{\"customer,name\"}'"
    assert jsonb_path_literal(['customer"name']) == '\'{"customer\\"name"}\''
    assert jsonb_path_literal(["customer\\name"]) == "'{\"customer\\\\name\"}'"
    assert jsonb_path_literal(["customer name"]) == "'{\"customer name\"}'"
    assert jsonb_path_literal(["\u5ba2\u6237"]) == "'{\"\u5ba2\u6237\"}'"
    assert jsonb_path_literal(["O'Brien"]) == "'{\"O''Brien\"}'"


def test_tc_sql_412_jsonb_path_literal_rejects_empty_path_or_segment():
    with pytest.raises(UnsafeGaussDBSQL) as empty_path:
        jsonb_path_literal([])
    with pytest.raises(UnsafeGaussDBSQL) as empty_segment:
        jsonb_path_literal(["amount", ""])
    with pytest.raises(UnsafeGaussDBSQL) as invalid_segment:
        jsonb_path_literal([None])

    assert str(empty_path.value) == "empty JSONB path"
    assert str(empty_segment.value) == "empty JSONB path segment"
    assert str(invalid_segment.value) == "invalid JSONB path segment"


def test_tc_sql_412_jsonb_path_literal_parser_handles_escapes_and_invalid_shapes():
    assert gaussdb_base._parse_jsonb_path_literal("'{\"customer,name\",dept}'") == ("customer,name", "dept")
    assert gaussdb_base._parse_jsonb_path_literal("'{\"customer\\\\name\"}'") == ("customer\\name",)

    invalid_literals = {
        "amount": "dynamic JSONB path is not allowed",
        "'{amount'": "dynamic JSONB path is not allowed",
        "'{amount,}'": "invalid JSONB path literal",
        "'{\"amount}'": "invalid JSONB path literal",
    }
    for literal, message in invalid_literals.items():
        with pytest.raises(UnsafeGaussDBSQL) as exc_info:
            gaussdb_base._parse_jsonb_path_literal(literal)
        assert str(exc_info.value) == message


def test_tc_sql_101_validator_allows_single_table_select_and_returns_columns():
    validated = _validator(field_map={}).validate_and_patch("SELECT doc_id, docnm_kwd FROM ragflow_t1 WHERE kb_id = 'kb-1' ORDER BY doc_id LIMIT 5")

    assert validated.sql == ("SELECT doc_id, docnm_kwd FROM ragflow_t1 WHERE kb_id = 'kb-1' ORDER BY doc_id LIMIT 5")
    assert validated.columns == ["doc_id", "docnm_kwd"]
    assert validated.is_aggregation is False


def test_tc_sql_102_validator_allows_readonly_cte_and_exposed_output_aliases():
    baseline = _validator(field_map={}).validate_and_patch("WITH x AS (SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT doc_id FROM x")
    alias = _validator(field_map={"amount": "number"}).validate_and_patch(
        "WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT amount FROM rows"
    )
    qualified_alias = _validator(field_map={"amount": "number"}).validate_and_patch(
        "WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT rows.amount FROM rows"
    )

    assert baseline.sql == ("WITH x AS (SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT doc_id FROM x LIMIT 128")
    assert baseline.columns == ["doc_id"]
    assert baseline.is_aggregation is False
    assert alias.sql == ("WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT amount FROM rows LIMIT 128")
    assert alias.columns == ["amount"]
    assert alias.is_aggregation is False
    assert qualified_alias.sql == ("WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT rows.amount FROM rows LIMIT 128")
    assert qualified_alias.columns == ["amount"]
    assert qualified_alias.is_aggregation is False

    validator = _validator(field_map={})
    list_validator = GaussDBSQLValidator(["ragflow_t1"], kb_ids=["kb-1"])
    assert list_validator.tables["ragflow_t1"].required_kb_ids == ("kb-1",)
    values_cte = validator._parse_one("WITH x AS (SELECT 1 UNION SELECT 2) SELECT 1")
    assert validator._cte_output_columns(values_cte) == {}
    from sqlglot import exp

    assert validator._is_cte_output_column(exp.column("doc_id"), {}) is False


def test_tc_sql_103_validator_rejects_multiple_statements_and_wraps_parser_failures():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'; SELECT 1",
        "multiple statements are not allowed",
    )

    validator = _validator()
    with pytest.raises(UnsafeGaussDBSQL, match="exactly one SQL statement is allowed"):
        validator._parse_one("SELECT 1; SELECT 2")

    with pytest.raises(UnsafeGaussDBSQL) as parse_error:
        validator._parse_one("SELECT (")
    with pytest.raises(UnsafeGaussDBSQL) as aggregate_parse_error:
        is_gaussdb_aggregate_sql("SELECT (")

    assert str(parse_error.value) == "SQL parse failed"
    assert str(aggregate_parse_error.value) == "SQL parse failed"


def test_tc_sql_104_validator_rejects_dml_including_nested_cte():
    scenarios = {
        "INSERT INTO ragflow_t1 VALUES (1)": "only SELECT statements are allowed",
        "UPDATE ragflow_t1 SET doc_id = 'new'": "only SELECT statements are allowed",
        "DELETE FROM ragflow_t1": "only SELECT statements are allowed",
        "WITH x AS (DELETE FROM ragflow_t1 RETURNING doc_id) SELECT doc_id FROM x": ("SQL contains a non-read-only expression"),
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message)


def test_tc_sql_105_validator_rejects_ddl():
    for sql in (
        "CREATE TABLE test (id int)",
        "DROP TABLE ragflow_t1",
        "ALTER TABLE ragflow_t1 ADD COLUMN x int",
    ):
        _assert_unsafe_sql(sql, "only SELECT statements are allowed")


def test_tc_sql_106_validator_rejects_call():
    _assert_unsafe_sql("CALL my_proc()", "only SELECT statements are allowed")


def test_tc_sql_107_validator_rejects_copy():
    _assert_unsafe_sql("COPY ragflow_t1 TO '/tmp/out.csv'", "only SELECT statements are allowed")


@pytest.mark.parametrize(
    "sql",
    [
        "MERGE INTO ragflow_t1 t USING source_table s ON t.doc_id = s.doc_id WHEN MATCHED THEN DELETE",
        "TRUNCATE TABLE ragflow_t1",
        "ANALYZE ragflow_t1",
        "VACUUM ragflow_t1",
        "GRANT SELECT ON ragflow_t1 TO app_user",
        "REVOKE SELECT ON ragflow_t1 FROM app_user",
        "LOCK TABLE ragflow_t1 IN ACCESS EXCLUSIVE MODE",
        "DO $$ BEGIN NULL; END $$",
    ],
    ids=["merge", "truncate", "analyze", "vacuum", "grant", "revoke", "lock", "do"],
)
def test_tc_sql_130_validator_rejects_additional_write_and_admin_statements(sql):
    with pytest.raises(UnsafeGaussDBSQL) as exc_info:
        _validator(field_map={}).validate_and_patch(sql)

    assert str(exc_info.value) in {
        "only SELECT statements are allowed",
        "multiple statements are not allowed",
        "SQL parse failed",
    }


def test_tc_sql_108_validator_rejects_select_star():
    _assert_unsafe_sql("SELECT * FROM ragflow_t1 WHERE kb_id = 'kb-1'", "SELECT * is not allowed")


def test_tc_sql_109_validator_allows_count_star_and_marks_aggregation():
    validated = _validator(field_map={}).validate_and_patch("SELECT COUNT(*) AS cnt FROM ragflow_t1")

    assert validated.sql == "SELECT COUNT(*) AS cnt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert validated.columns == ["cnt"]
    assert validated.is_aggregation is True


def test_tc_sql_110_validator_rejects_rank_and_row_number_windows():
    scenarios = (
        "SELECT doc_id, RANK() OVER (ORDER BY chunk_data #>> '{amount}') AS rnk FROM ragflow_t1",
        "SELECT doc_id, ROW_NUMBER() OVER (PARTITION BY kb_id ORDER BY doc_id) AS rn FROM ragflow_t1",
    )
    for sql in scenarios:
        _assert_unsafe_sql(sql, "window functions are not allowed")


def test_tc_sql_111_validator_rejects_system_table_and_cross_schema_table():
    for sql in (
        "SELECT doc_id FROM pg_catalog.tables",
        "SELECT doc_id FROM other_schema.ragflow_t1",
    ):
        _assert_unsafe_sql(sql, "cross-schema SQL is not allowed")


def test_tc_sql_112_validator_rejects_non_whitelisted_table():
    _assert_unsafe_sql(
        "SELECT doc_id FROM unknown_table WHERE kb_id = 'kb-1'",
        "table unknown_table is not allowed",
    )


def test_tc_sql_113_validator_rejects_non_whitelisted_column():
    _assert_unsafe_sql(
        "SELECT content_with_weight FROM ragflow_t1 WHERE kb_id = 'kb-1'",
        "column content_with_weight is not allowed",
    )


def test_tc_sql_114_validator_rejects_forbidden_system_functions():
    scenarios = {
        "SELECT pg_sleep(1) FROM ragflow_t1": "function pg_sleep is not allowed",
        "SELECT now() FROM ragflow_t1": "system functions are not allowed",
        "SELECT current_user FROM ragflow_t1": "system functions are not allowed",
        "SELECT version() FROM ragflow_t1": "function version is not allowed",
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message, _validator(field_map={}))


def test_tc_sql_115_validator_rejects_oceanbase_json_functions():
    scenarios = {
        "SELECT json_extract_string(chunk_data, '$.amount') FROM ragflow_t1": ("function json_extract_string is not allowed"),
        "SELECT json_extract(chunk_data, '$.amount') FROM ragflow_t1": ("only GaussDB #> / #>> JSONB operators are allowed"),
        "SELECT json_extract_isnull(chunk_data, '$.amount') FROM ragflow_t1": ("function json_extract_isnull is not allowed"),
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message)


def test_tc_sql_116_validator_rejects_forbidden_jsonb_set_returning_functions():
    scenarios = {
        "SELECT jsonb_each(chunk_data) FROM ragflow_t1": "function jsonb_each is not allowed",
        "SELECT jsonb_each_text(chunk_data) FROM ragflow_t1": "function jsonb_each_text is not allowed",
        "SELECT jsonb_array_elements(chunk_data) FROM ragflow_t1": ("function jsonb_array_elements is not allowed"),
        "SELECT jsonb_to_recordset(chunk_data) FROM ragflow_t1": ("function jsonb_to_recordset is not allowed"),
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message, _validator(field_map={}))


def test_tc_sql_117_validator_rejects_join_even_when_both_aliases_are_scoped():
    _assert_unsafe_sql(
        "SELECT a.doc_id FROM ragflow_t1 a JOIN ragflow_t1 b ON a.doc_id = b.doc_id WHERE a.kb_id = 'kb-1' AND b.kb_id = 'kb-1'",
        "complex SQL must use a simpler single-table kb_id boundary",
        _validator(field_map={}),
    )


def test_tc_sql_118_validator_rejects_union_as_non_select_root():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' UNION SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-2'",
        "only SELECT statements are allowed",
        _validator(field_map={}),
    )


def test_tc_sql_119_validator_rejects_or_predicate():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' OR chunk_data #>> '{amount}' = '100'",
        "complex SQL must use a simpler single-table kb_id boundary",
    )


def test_tc_sql_120_validator_rejects_dynamic_or_wrong_source_jsonb_paths():
    scenarios = {
        "SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> ('{' || param || '}') = '100'": ("dynamic JSONB path is not allowed"),
        "SELECT doc_id, chunk_data #>> path_col FROM ragflow_t1 WHERE kb_id = 'kb-1'": ("dynamic JSONB path is not allowed"),
        "SELECT doc_id, other_data #>> '{amount}' FROM ragflow_t1 WHERE kb_id = 'kb-1'": ("only chunk_data JSONB paths are allowed"),
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message)


def test_tc_sql_121_validator_rejects_empty_sql_variants():
    for sql in ("", "   ", "```sql```"):
        _assert_unsafe_sql(sql, "empty SQL")


def test_tc_sql_122_validator_rejects_explain():
    _assert_unsafe_sql(
        "EXPLAIN SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'",
        "only SELECT statements are allowed",
    )


def test_tc_sql_123_validator_rejects_set_command():
    _assert_unsafe_sql("SET statement_timeout = 30000", "only SELECT statements are allowed")


def test_tc_sql_124_validator_rejects_show_command():
    _assert_unsafe_sql("SHOW statement_timeout", "only SELECT statements are allowed")


@pytest.mark.parametrize(
    "sql",
    [
        "SELECT doc_id FROM ragflow_t1, LATERAL jsonb_each(chunk_data) AS j",
        "SELECT doc_id FROM ragflow_t1 CROSS JOIN LATERAL (SELECT 1) AS j",
        "SELECT doc_id FROM ragflow_t1 LEFT JOIN LATERAL (SELECT 1) AS j ON TRUE",
    ],
    ids=["set-returning-function", "cross-join-subquery", "left-join-subquery"],
)
def test_tc_sql_125_validator_rejects_every_lateral_shape(sql):
    _assert_unsafe_sql(
        sql,
        "LATERAL is not allowed",
        _validator(field_map={}),
    )


def test_tc_sql_126_validator_rejects_jsonb_path_query():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE jsonb_path_query(chunk_data, '$.amount') IS NOT NULL",
        "function jsonb_path_query is not allowed",
        _validator(field_map={}),
    )


def test_tc_sql_127_validator_rejects_jsonb_to_record():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1, jsonb_to_record(chunk_data) AS t(amount text)",
        "function jsonb_to_record is not allowed",
        _validator(field_map={}),
    )


def test_tc_sql_128_validator_rejects_generate_series():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1, generate_series(1, 10) AS g",
        "function generate_series is not allowed",
        _validator(field_map={}),
    )


def test_tc_sql_129_validator_rejects_unnest():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1, unnest(ARRAY[1, 2, 3]) AS u",
        "function unnest is not allowed",
        _validator(field_map={}),
    )


def test_tc_sql_201_validator_injects_kb_id_for_simple_single_table_query():
    validator = _validator(field_map={})
    validated = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1")

    assert validated.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_202_validator_injects_multi_kb_boundary():
    validated = _validator(kb_ids=["kb-1", "kb-2"], field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id IN ('kb-1', 'kb-2') LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_203_validator_preserves_allowed_kb_boundary_shapes():
    equality = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'")
    reversed_equality = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE 'kb-1' = kb_id")
    in_boundary = _validator(kb_ids=["kb-1", "kb-2"], field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id IN ('kb-1', 'kb-2')")
    predicate_before_boundary = _validator(kb_ids=["kb-1", "kb-2"], field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE doc_id IN ('doc1', 'doc2') AND kb_id IN ('kb-1')")

    assert equality.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert reversed_equality.sql == "SELECT doc_id FROM ragflow_t1 WHERE 'kb-1' = kb_id LIMIT 128"
    assert in_boundary.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id IN ('kb-1', 'kb-2') LIMIT 128")
    assert predicate_before_boundary.sql == ("SELECT doc_id FROM ragflow_t1 WHERE doc_id IN ('doc1', 'doc2') AND kb_id IN ('kb-1') LIMIT 128")
    for validated in (equality, reversed_equality, in_boundary, predicate_before_boundary):
        assert validated.columns == ["doc_id"]
        assert validated.is_aggregation is False


def test_tc_sql_204_validator_rejects_cross_kb_boundary():
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-other'",
        "SQL crosses the allowed kb_id boundary",
        _validator(field_map={}),
    )


def test_tc_sql_205_validator_rejects_non_positive_or_dynamic_kb_predicates():
    scenarios = {
        "SELECT doc_id FROM ragflow_t1 WHERE NOT kb_id = 'kb-1'": ("kb_id boundary must be a positive top-level predicate"),
        "SELECT doc_id FROM ragflow_t1 WHERE (kb_id = 'kb-1') IS FALSE": ("kb_id boundary must be a positive top-level predicate"),
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id IN (doc_id)": ("kb_id IN must use static string literals"),
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id IN ()": "SQL crosses the allowed kb_id boundary",
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message, _validator(field_map={}))

    validator = _validator()
    from sqlglot import exp

    in_ast = validator._parse_one("SELECT doc_id FROM ragflow_t1 WHERE kb_id IN ('kb-1')")
    assert validator._select_has_static_kb_boundary(in_ast) is True
    no_boundary_ast = validator._parse_one("SELECT doc_id FROM ragflow_t1 WHERE doc_id = 'd1'")
    assert validator._select_has_static_kb_boundary(no_boundary_ast) is False
    non_boundary_ast = validator._parse_one("SELECT doc_id FROM ragflow_t1 WHERE NOT doc_id = 'd1'")
    assert validator._select_has_static_kb_boundary(non_boundary_ast) is False
    empty_in = exp.In(this=exp.column("kb_id"), expressions=[])
    empty_ast = exp.select("doc_id").from_("ragflow_t1").where(empty_in)
    with pytest.raises(UnsafeGaussDBSQL, match="kb_id boundary is empty"):
        validator._select_has_static_kb_boundary(empty_ast)


def test_tc_sql_206_validator_rejects_complex_sql_when_scope_cannot_be_patched():
    _assert_unsafe_sql(
        "SELECT a.doc_id FROM ragflow_t1 a JOIN ragflow_t1 b ON a.doc_id = b.doc_id",
        "complex SQL must use a simpler single-table kb_id boundary",
        _validator(field_map={}),
    )


def test_tc_sql_207_validator_rejects_second_non_whitelisted_table():
    _assert_unsafe_sql(
        "SELECT t1.doc_id FROM ragflow_t1 t1 JOIN ragflow_t2 t2 ON t1.doc_id = t2.doc_id",
        "table ragflow_t2 is not allowed",
        _validator(field_map={}),
    )

    validator = _validator()
    with pytest.raises(UnsafeGaussDBSQL, match="must read from an exposed"):
        validator._enforce_kb_boundary("SELECT 1")
    with pytest.raises(UnsafeGaussDBSQL, match="exactly one base table"):
        validator._enforce_kb_boundary("SELECT doc_id FROM ragflow_t1, ragflow_t2")

    ast = validator._parse_one("SELECT a.doc_id FROM ragflow_t1 a JOIN ragflow_t2 b ON a.id = b.id")
    assert validator._direct_base_tables(ast) == ["ragflow_t1", "ragflow_t2"]
    assert validator._direct_source_tables(ast) == ["ragflow_t1", "ragflow_t2"]


def test_tc_sql_208_validator_rejects_missing_or_unsafe_required_kb_boundary():
    missing_table = ExposedGaussDBTable.from_field_map("ragflow_t1", [], {})
    missing_validator = GaussDBSQLValidator({"ragflow_t1": missing_table})

    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1",
        "kb_id boundary is required",
        missing_validator,
    )
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1",
        "unsafe kb_id literal",
        _validator(kb_ids=["kb-1;drop"], field_map={}),
    )


def test_tc_sql_209_validator_requires_each_cte_base_scope_to_prove_kb_boundary():
    validator = _validator(field_map={})
    _assert_unsafe_sql(
        "WITH x AS (SELECT doc_id FROM ragflow_t1) SELECT doc_id FROM x",
        "each base table scope must include a kb_id boundary",
        validator,
    )
    _assert_unsafe_sql(
        "WITH rows AS (SELECT doc_id, 'kb-1' AS kb_id FROM ragflow_t1) SELECT doc_id FROM rows WHERE kb_id = 'kb-1'",
        "each base table scope must include a kb_id boundary",
        validator,
    )

    validated = validator.validate_and_patch("WITH x AS (SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT doc_id FROM x")
    assert validated.sql == ("WITH x AS (SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT doc_id FROM x LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


@pytest.mark.parametrize(
    "correlation_predicate",
    ["", "inner_t.doc_id = outer_t.doc_id AND "],
    ids=["non-correlated", "correlated"],
)
def test_tc_sql_209_validator_requires_each_subquery_scope_to_prove_kb_boundary(correlation_predicate):
    validator = _validator(field_map={})
    outer_missing = f"SELECT outer_t.doc_id FROM ragflow_t1 AS outer_t WHERE EXISTS (SELECT 1 FROM ragflow_t1 AS inner_t WHERE {correlation_predicate}inner_t.kb_id = 'kb-1')"
    inner_missing = (
        f"SELECT outer_t.doc_id FROM ragflow_t1 AS outer_t WHERE outer_t.kb_id = 'kb-1' AND EXISTS (SELECT 1 FROM ragflow_t1 AS inner_t WHERE {correlation_predicate}inner_t.doc_id = 'doc-1')"
    )
    legal = f"SELECT outer_t.doc_id FROM ragflow_t1 AS outer_t WHERE outer_t.kb_id = 'kb-1' AND EXISTS (SELECT 1 FROM ragflow_t1 AS inner_t WHERE {correlation_predicate}inner_t.kb_id = 'kb-1')"
    cross_kb = f"SELECT outer_t.doc_id FROM ragflow_t1 AS outer_t WHERE outer_t.kb_id = 'kb-1' AND EXISTS (SELECT 1 FROM ragflow_t1 AS inner_t WHERE {correlation_predicate}inner_t.kb_id = 'kb-other')"

    for sql in (outer_missing, inner_missing):
        _assert_unsafe_sql(sql, "each base table scope must include a kb_id boundary", validator)
    _assert_unsafe_sql(cross_kb, "SQL crosses the allowed kb_id boundary", validator)

    validated = validator.validate_and_patch(legal)
    assert validated.sql.endswith("LIMIT 128")
    assert validated.sql.count("kb_id = 'kb-1'") == 2
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_301_validator_adds_default_limit_when_missing():
    validated = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'")

    assert validated.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False
    no_limit_validator = GaussDBSQLValidator(_validator().tables, default_limit=-1)
    assert "LIMIT" not in no_limit_validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'").sql


def test_tc_sql_302_validator_caps_large_limit_to_default_limit():
    validated = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 200")

    assert validated.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_303_validator_preserves_limit_under_default_limit():
    validated = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 100")

    assert validated.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 100"
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


@pytest.mark.parametrize(
    "sql",
    [
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 0",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT -1",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 1.5",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT $1",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT doc_id",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT '1'",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' FETCH FIRST 0 ROWS ONLY",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND EXISTS (SELECT 1 LIMIT 0)",
    ],
    ids=[
        "zero",
        "negative",
        "decimal",
        "parameter",
        "column",
        "string",
        "zero-fetch",
        "nested-zero",
    ],
)
def test_tc_sql_304_validator_rejects_non_positive_dynamic_or_non_integer_limits(sql):
    _assert_unsafe_sql(sql, "LIMIT must be a positive static integer", _validator(field_map={}))


def test_tc_sql_305_validator_rejects_complex_scope_before_limit_patch():
    validator = _validator(field_map={"status": "string"})
    scenarios = (
        "SELECT a.doc_id FROM ragflow_t1 a JOIN ragflow_t1 b ON a.doc_id = b.doc_id LIMIT 500",
        "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' OR chunk_data #>> '{status}' = 'active' LIMIT 500",
    )
    for sql in scenarios:
        _assert_unsafe_sql(
            sql,
            "complex SQL must use a simpler single-table kb_id boundary",
            validator,
        )


def test_tc_sql_306_readonly_guard_uses_fetch_size_as_default_limit():
    validator = GaussDBSQLValidator.readonly_guard(default_limit=64)
    validated = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'")

    assert validator.default_limit == 64
    assert validated.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 64"
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_307_validator_caps_fetch_first_to_default_limit():
    validated = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' FETCH FIRST 500 ROWS ONLY")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' FETCH FIRST 128 ROWS ONLY")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_308_clause_tokens_in_literals_or_subqueries_do_not_block_ast_patches():
    limit_field = _validator(field_map={"limit": "string"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #>> '{limit}' = '100'")
    subquery_limit = _validator(field_map={}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND EXISTS (SELECT 1 LIMIT 1)")
    forbidden_word_literal = _validator(field_map={"dept": "string"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #>> '{dept}' = 'update'")
    order_word_literal = _validator(field_map={"dept": "string"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{dept}' = 'order by dept' ORDER BY doc_id")

    assert limit_field.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #>> '{limit}' = '100' LIMIT 128")
    assert subquery_limit.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND EXISTS(SELECT 1 LIMIT 1) LIMIT 128")
    assert forbidden_word_literal.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #>> '{dept}' = 'update' LIMIT 128")
    assert order_word_literal.sql == ("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{dept}' = 'order by dept' AND kb_id = 'kb-1' ORDER BY doc_id LIMIT 128")
    for validated in (limit_field, subquery_limit, forbidden_word_literal, order_word_literal):
        assert validated.columns == ["doc_id"]
        assert validated.is_aggregation is False


def test_tc_sql_401_validator_allows_single_key_jsonb_text_path():
    validated = _validator(field_map={"amount": "number"}).validate_and_patch("SELECT chunk_data #>> '{amount}' AS amount FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 10")

    assert validated.sql == ("SELECT chunk_data #>> '{amount}' AS amount FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 10")
    assert validated.columns == ["amount"]
    assert validated.is_aggregation is False


def test_tc_sql_402_validator_allows_multi_key_jsonb_text_path():
    validated = _validator(field_map={"profile.name": "string"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{profile,name}' = 'Alice'")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{profile,name}' = 'Alice' AND kb_id = 'kb-1' LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_403_validator_requires_quotes_for_jsonb_key_containing_comma():
    validator = _validator(field_map={"a,b": "string"})
    validated = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{\"a,b\"}' = 'value'")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{\"a,b\"}' = 'value' AND kb_id = 'kb-1' LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False
    _assert_unsafe_sql(
        "SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{a,b}' = 'value'",
        "JSONB path ('a', 'b') is not exposed",
        validator,
    )


def test_tc_sql_404_validator_allows_jsonb_object_extraction_operator():
    validated = _validator(field_map={"profile": "json"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #> '{profile}' @> '{\"name\": \"Alice\"}'::jsonb")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #> '{profile}' @> CAST('{\"name\": \"Alice\"}' AS JSONB) AND kb_id = 'kb-1' LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_405_validator_allows_numeric_cast_on_exposed_jsonb_field():
    validated = _validator(field_map={"amount": "number"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION) > 100")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION) > 100 AND kb_id = 'kb-1' LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_406_validator_allows_to_date_on_exposed_jsonb_field():
    validated = _validator(field_map={"dt": "date"}).validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE to_date(chunk_data #>> '{dt}', 'YYYY-MM-DD') > '2026-01-01'")

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE TO_DATE(chunk_data #>> '{dt}', 'YYYY-MM-DD') > '2026-01-01' AND kb_id = 'kb-1' LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_407_validator_allows_jsonb_text_null_checks():
    validator = _validator(field_map={"status": "string"})
    is_null = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE (chunk_data #>> '{status}') IS NULL")
    is_not_null = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE (chunk_data #>> '{status}') IS NOT NULL")

    assert is_null.sql == ("SELECT doc_id FROM ragflow_t1 WHERE (chunk_data #>> '{status}') IS NULL AND kb_id = 'kb-1' LIMIT 128")
    assert is_not_null.sql == ("SELECT doc_id FROM ragflow_t1 WHERE NOT (chunk_data #>> '{status}') IS NULL AND kb_id = 'kb-1' LIMIT 128")
    for validated in (is_null, is_not_null):
        assert validated.columns == ["doc_id"]
        assert validated.is_aggregation is False


def test_tc_sql_408_validator_rejects_unexposed_or_empty_map_jsonb_path():
    _assert_unsafe_sql(
        "SELECT doc_id, chunk_data #>> '{non_exposed_field}' FROM ragflow_t1 WHERE kb_id = 'kb-1'",
        "JSONB path ('non_exposed_field',) is not exposed",
    )
    _assert_unsafe_sql(
        "SELECT doc_id, chunk_data #>> '{amount}' AS amount FROM ragflow_t1 WHERE kb_id = 'kb-1'",
        "JSONB path ('amount',) is not exposed",
        _validator(field_map={}),
    )


def test_tc_sql_409_validator_rejects_bare_or_non_gaussdb_chunk_data_access():
    scenarios = {
        "SELECT doc_id, chunk_data FROM ragflow_t1 WHERE kb_id = 'kb-1'": ("chunk_data may only be accessed through #> / #>>"),
        "SELECT chunk_data ->> 'amount' FROM ragflow_t1": ("only GaussDB #> / #>> JSONB operators are allowed"),
        "SELECT chunk_data['amount'] FROM ragflow_t1": ("chunk_data may only be accessed through #> / #>>"),
        "SELECT chunk_data @> '{\"amount\":1}'::jsonb FROM ragflow_t1": ("chunk_data may only be accessed through #> / #>>"),
        "SELECT chunk_data ? 'amount' FROM ragflow_t1": ("chunk_data may only be accessed through #> / #>>"),
        "SELECT chunk_data #- '{amount}' FROM ragflow_t1": ("chunk_data may only be accessed through #> / #>>"),
    }
    for sql, message in scenarios.items():
        _assert_unsafe_sql(sql, message)


@pytest.mark.parametrize(
    "predicate",
    [
        "chunk_data #>> '{status}' = ''",
        "'' = chunk_data #>> '{status}'",
        "chunk_data #>> '{status}' <> ''",
        "'' != chunk_data #>> '{status}'",
        "CAST(chunk_data #>> '{status}' AS TEXT) = ''",
        "chunk_data #>> '{status}' = CAST('' AS TEXT)",
        "chunk_data #>> '{status}' IS DISTINCT FROM ''",
        "chunk_data #>> '{status}' IS NOT DISTINCT FROM ''",
    ],
    ids=[
        "equal",
        "reversed-equal",
        "not-equal",
        "reversed-not-equal",
        "cast-jsonb",
        "cast-empty-string",
        "distinct",
        "not-distinct",
    ],
)
def test_tc_sql_410_validator_rejects_jsonb_text_comparisons_with_sql_empty_string(predicate):
    _assert_unsafe_sql(
        f"SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND {predicate}",
        "JSONB text cannot be compared with an empty SQL string",
        _validator(field_map={"status": "string"}),
    )


def test_tc_sql_410_validator_allows_jsonb_null_literal_comparison():
    sql = "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #> '{status}' = 'null'::jsonb"

    validated = _validator(field_map={"status": "string"}).validate_and_patch(sql)

    assert validated.sql == ("SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' AND chunk_data #> '{status}' = CAST('null' AS JSONB) LIMIT 128")
    assert validated.columns == ["doc_id"]
    assert validated.is_aggregation is False


def test_tc_sql_512_runtime_readonly_guard_enforces_table_and_preserves_safe_scopes():
    validator = GaussDBSQLValidator.readonly_guard(execution_schema="tenant_schema")
    _assert_unsafe_sql(
        "SELECT 1",
        "SQL must read from a DocEngine table",
        validator,
    )

    cte_alias = validator.validate_and_patch("WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM ragflow_t1 WHERE kb_id = 'kb-1') SELECT amount FROM rows")
    predicate_before_boundary = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE doc_id = 'doc1' AND kb_id = 'kb-1'")
    correlated = validator.validate_and_patch(
        "SELECT outer_t.doc_id FROM ragflow_t1 AS outer_t WHERE outer_t.kb_id = 'kb-1' AND EXISTS ("
        "SELECT 1 FROM ragflow_t1 AS inner_t WHERE inner_t.doc_id = outer_t.doc_id "
        "AND inner_t.kb_id = 'kb-1')"
    )
    assert cte_alias.sql == ("WITH rows AS (SELECT chunk_data #>> '{amount}' AS amount, kb_id FROM \"tenant_schema\".ragflow_t1 WHERE kb_id = 'kb-1') SELECT amount FROM rows LIMIT 128")
    assert cte_alias.columns == ["amount"]
    assert cte_alias.is_aggregation is False
    assert predicate_before_boundary.sql == ("SELECT doc_id FROM \"tenant_schema\".ragflow_t1 WHERE doc_id = 'doc1' AND kb_id = 'kb-1' LIMIT 128")
    assert predicate_before_boundary.columns == ["doc_id"]
    assert predicate_before_boundary.is_aggregation is False
    assert correlated.sql.count('FROM "tenant_schema".ragflow_t1') == 2
    assert correlated.sql.count("kb_id = 'kb-1'") == 2
    assert correlated.columns == ["doc_id"]
    assert correlated.is_aggregation is False


@pytest.mark.parametrize(
    "sql",
    [
        "SELECT 'sum(' AS marker, doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 5",
        "SELECT doc_id /* sum( */ FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 5",
        "SELECT doc_id AS \"sum(\" FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 5",
    ],
    ids=["literal", "comment", "alias"],
)
def test_tc_sql_705_aggregate_detection_ignores_sum_text_outside_ast_functions(sql):
    assert is_gaussdb_aggregate_sql(sql) is False
    assert _validator(field_map={}).validate_and_patch(sql).is_aggregation is False


def test_tc_sql_705_aggregate_detection_rejects_multiple_statements():
    with pytest.raises(UnsafeGaussDBSQL) as exc_info:
        is_gaussdb_aggregate_sql("SELECT COUNT(*) FROM ragflow_t1; SELECT 1")

    assert str(exc_info.value) == "exactly one SQL statement is allowed"


def test_tc_sql_705_validator_detects_all_supported_aggregations_and_non_aggregate():
    validator = _validator(field_map={"amount": "number"})
    scenarios = {
        "SELECT COUNT(*) AS cnt FROM ragflow_t1": (
            "SELECT COUNT(*) AS cnt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["cnt"],
            True,
        ),
        "SELECT SUM(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS total FROM ragflow_t1": (
            "SELECT SUM(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS total FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["total"],
            True,
        ),
        "SELECT AVG(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS avg_amt FROM ragflow_t1": (
            "SELECT AVG(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS avg_amt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["avg_amt"],
            True,
        ),
        "SELECT MAX(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS max_amt FROM ragflow_t1": (
            "SELECT MAX(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS max_amt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["max_amt"],
            True,
        ),
        "SELECT MIN(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS min_amt FROM ragflow_t1": (
            "SELECT MIN(CAST(chunk_data #>> '{amount}' AS DOUBLE PRECISION)) AS min_amt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["min_amt"],
            True,
        ),
        "SELECT doc_id FROM ragflow_t1": (
            "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128",
            ["doc_id"],
            False,
        ),
    }
    for sql, expected in scenarios.items():
        validated = validator.validate_and_patch(sql)
        assert (validated.sql, validated.columns, validated.is_aggregation) == expected
        assert is_gaussdb_aggregate_sql(validated.sql) is expected[2]


def test_tc_sql_706_validator_returns_selected_column_names_and_aliases():
    validator = _validator(field_map={"amount": "number"})
    aliased = validator.validate_and_patch("SELECT doc_id, chunk_data #>> '{amount}' AS amt FROM ragflow_t1")
    simple = validator.validate_and_patch("SELECT doc_id FROM ragflow_t1")
    literal = validator.validate_and_patch("SELECT 1, doc_id FROM ragflow_t1")

    assert aliased.sql == ("SELECT doc_id, chunk_data #>> '{amount}' AS amt FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128")
    assert aliased.columns == ["doc_id", "amt"]
    assert aliased.is_aggregation is False
    assert simple.sql == "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert simple.columns == ["doc_id"]
    assert simple.is_aggregation is False
    assert literal.sql == "SELECT 1, doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1' LIMIT 128"
    assert literal.columns == ["1", "doc_id"]
    assert literal.is_aggregation is False


def test_tc_sql_707_select_alias_is_allowed_in_group_and_order_by_for_both_guards():
    sql = "SELECT chunk_data #>> '{dept}' AS dept, COUNT(*) AS cnt FROM ragflow_t1 WHERE kb_id = 'kb-1' GROUP BY dept ORDER BY dept"
    validated = _validator(field_map={"dept": "string"}).validate_and_patch(sql)
    readonly = GaussDBSQLValidator.readonly_guard().validate_and_patch(sql)
    expected_sql = "SELECT chunk_data #>> '{dept}' AS dept, COUNT(*) AS cnt FROM ragflow_t1 WHERE kb_id = 'kb-1' GROUP BY dept ORDER BY dept LIMIT 128"

    assert validated.sql == expected_sql
    assert validated.columns == ["dept", "cnt"]
    assert validated.is_aggregation is True
    assert readonly.sql == expected_sql
    assert readonly.columns == ["dept", "cnt"]
    assert readonly.is_aggregation is True
