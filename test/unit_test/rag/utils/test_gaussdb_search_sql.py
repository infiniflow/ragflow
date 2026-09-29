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

from common.doc_store.gaussdb_conn_base import GaussDBSearchBuilder, InvalidGaussDBObjectName
from rag.utils.gaussdb_conn import GaussDBConnection, SearchResult


def test_tc_ret_513_dynamic_chunk_field_exists_filter_reads_extra_jsonb():
    builder = GaussDBSearchBuilder(schema="public")

    sql, params = builder.build_condition_where({"kb_id": "kb1", "must_not": {"exists": "custom_kwd"}})

    assert "(extra -> 'custom_kwd') IS NULL" in sql
    assert "(extra -> 'custom_kwd') = 'null'::jsonb" in sql
    assert params == ["kb1"]


def test_tc_ret_513_dynamic_chunk_field_select_and_filter_are_backed_by_extra_jsonb():
    builder = GaussDBSearchBuilder(schema="public")

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id", "custom_kwd"],
        condition={"kb_id": "kb1", "custom_kwd": ["artifact_page"]},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert "(extra -> 'custom_kwd') AS custom_kwd" in sql
    assert "(extra -> 'custom_kwd') = %s::jsonb" in sql
    assert params[:3] == ["kb1", '"artifact_page"', '["artifact_page"]']


def test_tc_ret_513_dynamic_chunk_field_rejects_unsafe_json_key():
    builder = GaussDBSearchBuilder(schema="public")

    with pytest.raises(InvalidGaussDBObjectName):
        builder.build_condition_where({"must_not": {"exists": "custom_kwd'; DROP TABLE user; --"}})


def test_tc_ret_101_fulltext_builder_parameterizes_simple_query_and_filter():
    builder = GaussDBSearchBuilder(schema="public")
    malicious_keyword = "contract'); DROP TABLE ragflow_tenant; --"
    query_text = f"{malicious_keyword} risk"
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id", "content_with_weight"],
        condition={"available_int": 1},
        keywords=[malicious_keyword, "risk"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert "to_tsvector('simple'" in sql
    assert "ts_rank" in sql
    assert "WHERE available_int = %s AND to_tsvector" in sql
    assert "plainto_tsquery('simple', %s)" in sql
    assert "ORDER BY _score DESC, kb_id ASC, id ASC LIMIT %s OFFSET %s" in sql
    assert malicious_keyword not in sql
    assert query_text not in sql
    assert params == [query_text] * 6 + [1, query_text, 10, 0]


def test_tc_ret_109_chinese_ngram_query_preserves_english_semantics_and_highlight_text():
    builder = GaussDBSearchBuilder(schema="public")

    ngram_ddl = builder.ddl.build_ngram_fulltext_ugin_ddl("ragflow_tenant")
    assert '"idx_gdb_ragflow_tenant_fts_all_ngram"' in ngram_ddl
    assert "USING ugin(to_tsvector('ngram'" in ngram_ddl

    chinese_sql, chinese_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id", "content_with_weight"],
        condition={"kb_id": "kb1"},
        keywords=["深圳"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
        highlight_fields=["content_with_weight"],
    )
    assert "to_tsvector('ngram'" in chinese_sql
    assert "plainto_tsquery('ngram', %s)" in chinese_sql
    assert "to_tsvector('simple'" not in chinese_sql
    assert "COALESCE(content_with_weight, ' ') AS _highlight_source" in chinese_sql
    assert "ts_headline('ngram'" not in chinese_sql
    assert chinese_params == ["深圳"] * 6 + ["kb1", "深圳", 10, 0]

    mixed_sql, mixed_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id", "content_with_weight"],
        condition={"kb_id": "kb1"},
        keywords=["上海", "RAGFlow", "database"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
        highlight_fields=["content_with_weight"],
    )
    assert "to_tsvector('simple'" in mixed_sql
    assert "to_tsvector('ngram'" in mixed_sql
    assert "plainto_tsquery('simple', %s)" in mixed_sql
    assert "plainto_tsquery('ngram', %s)" in mixed_sql
    assert " / 2.0)" in mixed_sql
    assert ") @@ plainto_tsquery('simple', %s) AND to_tsvector('ngram'" in mixed_sql
    assert "COALESCE(content_with_weight, ' ') AS _highlight_source" in mixed_sql
    assert mixed_params == ["RAGFlow database"] * 6 + ["上海"] * 6 + [
        "kb1",
        "RAGFlow database",
        "上海",
        10,
        0,
    ]

    single_character_sql, single_character_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["中"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )
    assert "0.0 AS _score" in single_character_sql
    assert "WHERE kb_id = %s AND FALSE" in single_character_sql
    assert single_character_params == ["kb1", 10, 0]

    vector_only_sql, _vector_only_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["中"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.3,
        offset=0,
        limit=10,
    )
    assert "WITH vec AS" in vector_only_sql
    assert "WITH fts_raw AS" not in vector_only_sql

    conn = GaussDBConnection.__new__(GaussDBConnection)
    result = SearchResult(
        total=2,
        chunks=[
            {"id": "mixed", "_highlight_source": "上海 RAGFlow database 审计"},
            {"id": "overlap", "_highlight_source": "深圳数据库审计"},
        ],
    )
    assert conn.get_highlight(result, ["上海", "RAGFlow", "database"], "content_with_weight") == {"mixed": "<em>上海</em> <em>RAGFlow</em> <em>database</em> 审计"}
    assert conn.get_highlight(result, ["深圳数据", "深圳"], "content_with_weight") == {"overlap": "<em>深圳数据</em>库审计"}


def test_tc_ret_111_minimum_should_match_uses_partial_unique_terms_and_term_scores():
    builder = GaussDBSearchBuilder(schema="public")
    keywords = ["你好", "一下", "王楠", "简历", "大概", "介绍", "一下"]

    match_sql, match_params = builder._build_text_match_expr(keywords, minimum_should_match=0.3)
    score_sql, score_params = builder.build_text_score_expr(keywords, minimum_should_match=0.3)

    unique_terms = ["你好", "一下", "王楠", "简历", "大概", "介绍"]
    assert " OR " in match_sql
    assert match_params == unique_terms
    assert " / 6.0)" in score_sql
    assert score_params == [term for term in unique_terms for _field in builder.FTS_WEIGHTS]
    assert builder._minimum_should_match_count(0.3, 6) == 1
    assert builder._minimum_should_match_count("50%", 6) == 3
    assert builder._minimum_should_match_count(2, 6) == 2

    two_match_sql, two_match_params = builder._build_text_match_expr(keywords, minimum_should_match=2)
    assert "CASE WHEN" in two_match_sql
    assert ">= 2" in two_match_sql
    assert two_match_params == unique_terms * 2


def test_tc_ret_203_vector_search_filters_invalid_placeholder_vectors():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"available_int": 1},
        keywords=[],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=1.0,
        offset=0,
        limit=5,
    )

    vector_param = "[0.1,0.2,0.3,0.4]"
    assert "WHERE available_int = %s AND q_4_vec_valid = TRUE" in sql
    assert sql.count("q_4_vec <+> %s::floatvector(4)") == 3
    assert "WHERE _score >= %s" in sql
    assert params == [vector_param, vector_param, 1, vector_param, 5, 0.0, 5, 0]


def test_tc_ret_301_hybrid_search_uses_configured_vector_weight():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"available_int": 1},
        keywords=["budget"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.7,
        offset=0,
        limit=5,
    )

    assert "%s * COALESCE(vec.vector_score, 0)" in sql
    assert "(1 - %s) * COALESCE(fts.fts_score, 0)" in sql
    assert params == [
        "budget",
        "budget",
        "budget",
        "budget",
        "budget",
        "budget",
        1,
        "budget",
        5,
        "[0.1,0.2,0.3,0.4]",
        1,
        "[0.1,0.2,0.3,0.4]",
        5,
        0.7,
        0.7,
        0.0,
        5,
        0,
    ]


def test_tc_ret_303_hybrid_search_full_outer_joins_text_and_vector_candidates():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["budget"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.3,
        offset=0,
        limit=5,
    )

    assert "COALESCE(fts.kb_id, vec.kb_id) AS kb_id" in sql
    assert "COALESCE(fts.id, vec.id) AS id" in sql
    assert "FROM fts FULL OUTER JOIN vec ON fts.kb_id = vec.kb_id AND fts.id = vec.id" in sql
    assert 'FROM merged JOIN "public"."ragflow_tenant" c ON c.kb_id = merged.kb_id AND c.id = merged.id' in sql
    assert params[-5:] == [0.3, 0.3, 0.0, 5, 0]


def test_tc_ret_310_hybrid_pagerank_is_applied_to_projection_and_threshold():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["budget"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.7,
        offset=0,
        limit=5,
        pagerank_weight=10.0,
    )

    pagerank_expr = "(COALESCE(c.pagerank_fea, 0)::DOUBLE PRECISION / 100.0 * %s)"
    assert f"(merged.score + {pagerank_expr}) AS _score" in sql
    assert f"WHERE (merged.score + {pagerank_expr}) >= %s" in sql
    assert params == ["budget"] * 6 + [
        "kb1",
        "budget",
        5,
        "[0.1,0.2,0.3,0.4]",
        "kb1",
        "[0.1,0.2,0.3,0.4]",
        5,
        0.7,
        0.7,
        10.0,
        10.0,
        0.0,
        5,
        0,
    ]


def test_tc_ret_701_pagerank_feature_adds_bound_score_component():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["risk"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=5,
        pagerank_weight=10.0,
    )

    assert "COALESCE(pagerank_fea, 0)::DOUBLE PRECISION / 100.0 * %s" in sql
    assert "ORDER BY _score DESC, kb_id ASC, id ASC" in sql
    assert params == ["risk"] * 6 + [10.0, "kb1", "risk", 5, 0]


def test_tc_ret_201_vector_only_query_omits_fulltext_predicates():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"available_int": 1},
        keywords=[],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=1.0,
        offset=0,
        limit=5,
    )

    assert "to_tsvector" not in sql
    assert "WITH vec AS" in sql
    assert "ORDER BY q_4_vec <+> %s::floatvector(4) ASC LIMIT %s" in sql
    assert "ORDER BY distance ASC, kb_id ASC, id ASC LIMIT %s OFFSET %s" in sql
    assert "[0.1,0.2,0.3,0.4]" not in sql
    assert "[0.1,0.2,0.3,0.4]" in params
    assert params[-4:] == [5, 0.0, 5, 0]


def test_tc_ret_605_fulltext_search_projects_parameterized_highlight_column():
    builder = GaussDBSearchBuilder(schema="public")
    malicious_keyword = "risk'); DROP TABLE ragflow_tenant; --"
    query_text = f"{malicious_keyword} audit"
    vector = [0.123456789, 0.234567891, 0.345678912, 0.456789123]
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=[malicious_keyword, "audit"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=5,
        highlight_fields=["content_with_weight"],
    )

    assert ("ts_headline('simple', COALESCE(content_with_weight, ' '), plainto_tsquery('simple', %s), 'StartSel=<em>, StopSel=</em>') AS _highlight") in sql
    assert malicious_keyword not in sql
    assert query_text not in sql
    assert params == [query_text] * 7 + ["kb1", query_text, 5, 0]

    hybrid_sql, hybrid_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=[malicious_keyword, "audit"],
        vector=vector,
        vector_dim=4,
        vector_weight=0.3,
        offset=0,
        limit=5,
        highlight_fields=["content_with_weight"],
    )
    vector_param = "[0.123456789,0.234567891,0.345678912,0.456789123]"
    assert "FULL OUTER JOIN" in hybrid_sql
    assert ("ts_headline('simple', COALESCE(content_with_weight, ' '), plainto_tsquery('simple', %s), 'StartSel=<em>, StopSel=</em>') AS _highlight") in hybrid_sql
    assert malicious_keyword not in hybrid_sql
    assert query_text not in hybrid_sql
    assert vector_param not in hybrid_sql
    assert hybrid_params == [query_text] * 6 + [
        "kb1",
        query_text,
        5,
        vector_param,
        "kb1",
        vector_param,
        5,
        0.3,
        0.3,
        query_text,
        0.0,
        5,
        0,
    ]


def test_tc_ret_601_aggregation_sql_groups_field_values():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_aggregation_sql(
        table="ragflow_tenant",
        field_name="docnm_kwd",
        condition={"kb_id": "kb1"},
    )

    assert sql == ('SELECT docnm_kwd AS value, COUNT(1) AS count FROM "public"."ragflow_tenant" WHERE kb_id = %s AND docnm_kwd IS NOT NULL GROUP BY value ORDER BY count DESC, value ASC LIMIT %s')
    assert params == ["kb1", 1000]


def test_tc_ret_602_aggregation_sql_expands_jsonb_array_fields_without_lateral():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_aggregation_sql(
        table="ragflow_tenant",
        field_name="tag_kwd",
        condition={"kb_id": "k1"},
    )

    assert sql == (
        "SELECT value, COUNT(1) AS count "
        "FROM (SELECT jsonb_array_elements_text(COALESCE(tag_kwd, '[]'::jsonb)) AS value "
        'FROM "public"."ragflow_tenant" WHERE kb_id = %s) AS expanded '
        "GROUP BY value ORDER BY count DESC, value ASC LIMIT %s"
    )
    assert "LATERAL" not in sql.upper()
    assert params == ["k1", 1000]


def test_tc_ret_306_hybrid_threshold_is_applied_after_merged_score():
    builder = GaussDBSearchBuilder(schema="public")

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["risk"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.75,
        similarity_threshold=0.2,
        offset=0,
        limit=5,
    )

    assert "WHERE merged.score >= %s" in sql
    assert sql.count(">= %s") == 1
    assert params[-5:] == [0.75, 0.75, 0.2, 5, 0]


def test_tc_ret_311_hybrid_zero_vector_weight_is_bound_in_merged_score():
    builder = GaussDBSearchBuilder(schema="public")

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=["risk"],
        vector=[0.1, 0.2, 0.3, 0.4],
        vector_dim=4,
        vector_weight=0.0,
        similarity_threshold=0.8,
        offset=0,
        limit=5,
    )

    assert "(1 - %s) * COALESCE(fts.fts_score, 0) + %s * COALESCE(vec.vector_score, 0) AS score" in sql
    assert "WHERE merged.score >= %s" in sql
    assert params[-5:] == [0.0, 0.0, 0.8, 5, 0]


def test_tc_ret_010_filter_only_search_uses_collection_limit_when_limit_is_zero():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "kb1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=0,
    )

    assert "ORDER BY kb_id ASC, id ASC LIMIT %s OFFSET %s" in sql
    assert params == ["kb1", 10000, 0]


def test_tc_ret_108_filter_only_search_parameterizes_kb_limit_and_offset():
    builder = GaussDBSearchBuilder(schema="public")
    kb_id = "k1' OR TRUE --"
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id", "docnm_kwd"],
        condition={"kb_id": kb_id},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=5,
        limit=10,
        order_by=type("Order", (), {"fields": [("docnm_kwd", True)]})(),
    )

    assert sql == ('SELECT id, kb_id, docnm_kwd, 0.0 AS _score, COUNT(*) OVER() AS __total FROM "public"."ragflow_tenant" WHERE kb_id = %s ORDER BY docnm_kwd DESC LIMIT %s OFFSET %s')
    assert params == [kb_id, 10, 5]
    assert kb_id not in sql
    assert all(expression not in sql for expression in ("to_tsvector", "plainto_tsquery", "ts_rank", "@@"))


def test_tc_ret_405_condition_builder_supports_exists_must_not_lists_jsonb_and_null():
    builder = GaussDBSearchBuilder(schema="public")

    where_sql, params = builder.build_condition_where(
        {
            "exists": "doc_id",
            "must_not": {"exists": "img_id"},
            "doc_id": ["d1", "d2"],
            "tag_kwd": ["risk", "audit"],
            "mom_id": None,
        }
    )

    assert "doc_id IS NOT NULL" in where_sql
    assert "img_id IS NULL" in where_sql
    assert "doc_id IN (%s, %s)" in where_sql
    assert "(tag_kwd @> %s::jsonb OR tag_kwd @> %s::jsonb)" in where_sql
    assert "mom_id IS NULL" in where_sql
    assert params == ["d1", "d2", '["risk"]', '["audit"]']


def test_tc_ret_513_compile_kwd_uses_extra_jsonb_mapping():
    builder = GaussDBSearchBuilder(schema="public")

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["compile_kwd"],
        condition={"kb_id": "kb1", "must_not": {"exists": "compile_kwd"}},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert "(extra #>> '{compile_kwd}') AS compile_kwd" in sql
    assert "(extra #>> '{compile_kwd}') IS NULL" in sql
    assert params == ["kb1", 10, 0]


def test_tc_ret_406_condition_builder_rejects_empty_list_conditions():
    builder = GaussDBSearchBuilder(schema="public")

    with pytest.raises(ValueError, match="empty condition values"):
        builder.build_condition_where({"id": []})


def test_tc_ret_208_vector_search_rejects_missing_or_mismatched_dimensions():
    builder = GaussDBSearchBuilder(schema="public")

    with pytest.raises(ValueError, match="^vector_dim is required for vector search$"):
        builder.build_search_sql(
            table="ragflow_tenant",
            select_fields=["id"],
            condition={"kb_id": "kb1"},
            keywords=[],
            vector=[0.1],
            vector_dim=None,
            vector_weight=1.0,
            offset=0,
            limit=5,
        )

    with pytest.raises(ValueError, match="^vector dimension mismatch: expected 2, got 1$"):
        builder.build_search_sql(
            table="ragflow_tenant",
            select_fields=["id"],
            condition={"kb_id": "kb1"},
            keywords=[],
            vector=[0.1],
            vector_dim=2,
            vector_weight=1.0,
            offset=0,
            limit=5,
        )


def test_tc_ret_501_select_empty_defaults_to_id_kb_id():
    builder = GaussDBSearchBuilder(schema="public")

    assert builder.normalize_select_fields([]) == ["id", "kb_id"]

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=[],
        condition={"kb_id": "k1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert sql == ('SELECT id, kb_id, 0.0 AS _score, COUNT(*) OVER() AS __total FROM "public"."ragflow_tenant" WHERE kb_id = %s ORDER BY kb_id ASC, id ASC LIMIT %s OFFSET %s')
    assert "SELECT *" not in sql
    assert params == ["k1", 10, 0]


def test_tc_ret_502_select_fields_add_ids_skip_score_and_deduplicate():
    builder = GaussDBSearchBuilder(schema="public")

    assert builder.normalize_select_fields(["_score", "content_with_weight", "content_with_weight"]) == [
        "id",
        "kb_id",
        "content_with_weight",
    ]

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["_score", "content_with_weight", "content_with_weight"],
        condition={"kb_id": "k1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert sql.startswith("SELECT id, kb_id, content_with_weight, 0.0 AS _score")
    assert sql.count("content_with_weight") == 1
    assert params == ["k1", 10, 0]


def test_tc_ret_503_select_star_defaults_to_id_kb_id():
    builder = GaussDBSearchBuilder(schema="public")

    assert builder.normalize_select_fields(["*"]) == ["id", "kb_id"]

    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["*"],
        condition={"kb_id": "k1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
    )

    assert sql == ('SELECT id, kb_id, 0.0 AS _score, COUNT(*) OVER() AS __total FROM "public"."ragflow_tenant" WHERE kb_id = %s ORDER BY kb_id ASC, id ASC LIMIT %s OFFSET %s')
    assert "SELECT *" not in sql
    assert params == ["k1", 10, 0]


def test_tc_ret_504_selecting_vector_column_auto_includes_valid_flag():
    builder = GaussDBSearchBuilder(schema="public")

    assert builder.normalize_select_fields(["q_1024_vec"]) == ["id", "kb_id", "q_1024_vec", "q_1024_vec_valid"]


def test_tc_ret_702_zero_pagerank_weight_does_not_add_pagerank_score_component():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["id"],
        condition={"kb_id": "k1"},
        keywords=["hello"],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=10,
        pagerank_weight=0.0,
    )

    assert "pagerank_fea" not in sql
    assert "COALESCE(pagerank_fea" not in sql
    assert params == ["hello"] * 6 + ["k1", "hello", 10, 0]


def test_tc_ret_512_compatibility_fields_use_safe_mapping_and_deterministic_orders():
    builder = GaussDBSearchBuilder(schema="public")
    sql, params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["row_id()", "chunk_order_int"],
        condition={"kb_id": "kb1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=5,
        order_by=type("Order", (), {"fields": [("position_int", False)]})(),
    )

    assert builder.build_position_order_sql() == (
        "COALESCE((page_num_int #>> '{0}')::int, 100000000) ASC, COALESCE((position_int #>> '{0,3}')::int, 100000000) ASC, COALESCE((top_int #>> '{0}')::int, 100000000) ASC"
    )
    assert sql == (
        'SELECT id, kb_id, NULL AS "row_id()", _order_id AS chunk_order_int, '
        '0.0 AS _score, COUNT(*) OVER() AS __total FROM "public"."ragflow_tenant" '
        "WHERE kb_id = %s ORDER BY "
        "COALESCE((page_num_int #>> '{0}')::int, 100000000) ASC, "
        "COALESCE((position_int #>> '{0,3}')::int, 100000000) ASC, "
        "COALESCE((top_int #>> '{0}')::int, 100000000) ASC LIMIT %s OFFSET %s"
    )
    assert params == ["kb1", 5, 0]
    assert "row_id() AS" not in sql
    assert "chunk_order_int AS" not in sql

    default_sql, default_params = builder.build_search_sql(
        table="ragflow_tenant",
        select_fields=["row_id()", "chunk_order_int"],
        condition={"kb_id": "kb1"},
        keywords=[],
        vector=None,
        vector_dim=None,
        vector_weight=0.0,
        offset=0,
        limit=5,
    )
    assert "ORDER BY kb_id ASC, id ASC LIMIT %s OFFSET %s" in default_sql
    assert default_params == ["kb1", 5, 0]

    with pytest.raises(ValueError, match="unsafe_field"):
        builder.build_search_sql(
            table="ragflow_tenant",
            select_fields=["unsafe_field; DROP TABLE chunks"],
            condition={"kb_id": "kb1"},
            keywords=[],
            vector=None,
            vector_dim=None,
            vector_weight=0.0,
            offset=0,
            limit=5,
        )
