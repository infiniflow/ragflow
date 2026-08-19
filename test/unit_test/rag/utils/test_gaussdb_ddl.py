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

from common.doc_store.gaussdb_conn_base import (
    GaussDBDDLBuilder,
    InvalidGaussDBObjectName,
)


def _normalize_ddl(ddl):
    return " ".join(ddl.split())


@pytest.mark.parametrize("identifier", ["ragflow_unit", "schema#tenant$1", "中文标识符"])
def test_tc_wrt_901_identifier_helpers_return_exact_valid_and_quoted_names(identifier):
    builder = GaussDBDDLBuilder(schema="test_schema")

    assert builder.validate_identifier(identifier) == identifier
    assert builder.quote_identifier(identifier) == f'"{identifier}"'
    assert builder.qualified_name(identifier) == f'"test_schema"."{identifier}"'


INVALID_IDENTIFIERS = ["", "a" * 64, "1abc", "a b", "ta ble", "kb_id;drop"]


@pytest.mark.parametrize(
    "identifier",
    INVALID_IDENTIFIERS,
    ids=["empty", "overlong", "numeric-prefix", "embedded-space", "table-space", "sql-punctuation"],
)
def test_tc_wrt_008_index_name_rejects_invalid_identifiers(identifier):
    builder = GaussDBDDLBuilder(schema="test_schema")

    with pytest.raises(InvalidGaussDBObjectName) as exc_info:
        builder.index_name(identifier, "chunk")

    assert exc_info.type is InvalidGaussDBObjectName
    assert exc_info.value.args == (identifier,)
    assert str(exc_info.value) == identifier


@pytest.mark.parametrize(
    "identifier",
    INVALID_IDENTIFIERS,
    ids=["empty", "overlong", "numeric-prefix", "embedded-space", "table-space", "sql-punctuation"],
)
def test_tc_wrt_902_validate_identifier_rejects_invalid_identifiers(identifier):
    builder = GaussDBDDLBuilder(schema="test_schema")

    with pytest.raises(InvalidGaussDBObjectName) as exc_info:
        builder.validate_identifier(identifier)

    assert exc_info.type is InvalidGaussDBObjectName
    assert exc_info.value.args == (identifier,)
    assert str(exc_info.value) == identifier


def test_tc_wrt_903_chunk_table_ddl_matches_normalized_snapshot():
    builder = GaussDBDDLBuilder(schema="test_schema")

    ddl = builder.build_chunk_table_ddl("ragflow_unit")

    expected = """CREATE TABLE IF NOT EXISTS "test_schema"."ragflow_unit" (
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
    assert _normalize_ddl(ddl) == _normalize_ddl(expected)


def test_tc_wrt_904_vector_column_ddls_match_exact_snapshot():
    builder = GaussDBDDLBuilder(schema="test_schema")

    statements = builder.build_vector_column_ddls("ragflow_unit", 1024)

    assert statements == [
        'ALTER TABLE "test_schema"."ragflow_unit" ADD COLUMN IF NOT EXISTS q_1024_vec floatvector(1024) DEFAULT (array_fill(0, ARRAY[1024])::text::floatvector(1024))',
        'ALTER TABLE "test_schema"."ragflow_unit" ADD COLUMN IF NOT EXISTS q_1024_vec_valid BOOLEAN DEFAULT FALSE NOT NULL',
    ]


@pytest.mark.parametrize(
    ("dim", "diskann_options"),
    [
        (1, "subgraph_count=1"),
        (4096, "subgraph_count=1, enable_vector_copy=false"),
    ],
    ids=["minimum", "maximum"],
)
def test_tc_wrt_004_validate_vector_dim_accepts_gaussdb_boundaries(dim, diskann_options):
    builder = GaussDBDDLBuilder(schema="test_schema")

    assert builder.validate_vector_dim(dim) == dim
    assert builder.vector_column_name(dim) == f"q_{dim}_vec"
    assert builder.vector_valid_column_name(dim) == f"q_{dim}_vec_valid"
    assert builder.build_vector_column_ddls("ragflow_t1", dim) == [
        f'ALTER TABLE "test_schema"."ragflow_t1" ADD COLUMN IF NOT EXISTS q_{dim}_vec floatvector({dim}) DEFAULT (array_fill(0, ARRAY[{dim}])::text::floatvector({dim}))',
        f'ALTER TABLE "test_schema"."ragflow_t1" ADD COLUMN IF NOT EXISTS q_{dim}_vec_valid BOOLEAN DEFAULT FALSE NOT NULL',
    ]
    assert builder.build_diskann_index_ddl("ragflow_t1", dim) == (
        f'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_t1_q_{dim}_vec_diskann" ON "test_schema"."ragflow_t1" USING gsdiskann (q_{dim}_vec COSINE) WITH ({diskann_options})'
    )


@pytest.mark.parametrize("dim", [0, -1], ids=["zero", "negative"])
def test_tc_wrt_005_validate_vector_dim_rejects_non_positive_dimensions(dim):
    builder = GaussDBDDLBuilder(schema="public")

    with pytest.raises(ValueError) as exc_info:
        builder.validate_vector_dim(dim)

    assert str(exc_info.value) == "vector dimension must be positive"


def test_tc_wrt_006_vector_size_above_gaussdb_limit_is_rejected():
    builder = GaussDBDDLBuilder(schema="public")

    with pytest.raises(ValueError) as exc_info:
        builder.validate_vector_dim(4097)

    assert str(exc_info.value) == "GaussDB floatvector dimensions cannot exceed 4096"


@pytest.mark.parametrize(
    "dim",
    [
        "not-int",
        1024.5,
    ],
    ids=["non-numeric", "float"],
)
def test_tc_wrt_007_non_integer_vector_size_raises_exact_error(dim):
    builder = GaussDBDDLBuilder(schema="test_schema")

    with pytest.raises(ValueError) as exc_info:
        builder.validate_vector_dim(dim)

    assert str(exc_info.value) == "vector dimension must be an integer"


@pytest.mark.parametrize(
    ("dim", "expected_options"),
    [
        (768, "subgraph_count=1"),
        (1024, "subgraph_count=1"),
        (1025, "subgraph_count=1, enable_vector_copy=false"),
        (3072, "subgraph_count=1, enable_vector_copy=false"),
    ],
    ids=["below-threshold", "threshold", "above-threshold", "high-dimension"],
)
def test_tc_wrt_009_diskann_dimension_matrix_matches_exact_ddl(dim, expected_options):
    builder = GaussDBDDLBuilder(schema="public")

    ddl = builder.build_diskann_index_ddl("ragflow_tenant_a", dim)

    assert ddl == (f'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_tenant_a_q_{dim}_vec_diskann" ON "public"."ragflow_tenant_a" USING gsdiskann (q_{dim}_vec COSINE) WITH ({expected_options})')


def test_tc_wrt_909_advisory_lock_sql_is_exact_and_parameterized():
    builder = GaussDBDDLBuilder(schema="test_schema")

    sql, params = builder.build_advisory_lock_sql("create_idx:schema:table")

    assert sql == "SELECT pg_advisory_xact_lock(hashtext(%s))"
    assert params == ["create_idx:schema:table"]


def test_tc_wrt_911_long_table_identifier_produces_bounded_distinct_index_names():
    builder = GaussDBDDLBuilder(schema="test_schema")
    table = "ragflow_" + "a" * 32
    index_columns = ("doc_id", "available_int", "knowledge_graph_kwd", "entity_type_kwd", "removed_kwd")
    suffixes = (*index_columns, "q_3072_vec_diskann")

    expected_names = [
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_doc_id",
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_available_int",
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_kno_79293b137f",
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_ent_2da2f5b85e",
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_removed_kwd",
        "idx_gdb_ragflow_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_q_3_83afb848d3",
    ]
    names = [builder.index_name(table, suffix) for suffix in suffixes]
    repeated_names = [builder.index_name(table, suffix) for suffix in suffixes]
    regular_ddls = builder.build_regular_index_ddls(table)
    diskann_ddl = builder.build_diskann_index_ddl(table, 3072)

    assert names == expected_names
    assert repeated_names == expected_names
    assert len(set(names)) == len(suffixes)
    assert all(len(name) <= 63 for name in names)
    assert all(name.startswith("idx_gdb_ragflow_") for name in names)
    assert len(regular_ddls) == 5
    for column, index_name, statement in zip(index_columns, expected_names[:5], regular_ddls, strict=True):
        assert statement == (f'CREATE INDEX IF NOT EXISTS "{index_name}" ON "test_schema"."{table}" ({column})')
    assert diskann_ddl == (f'CREATE INDEX IF NOT EXISTS "{expected_names[-1]}" ON "test_schema"."{table}" USING gsdiskann (q_3072_vec COSINE) WITH (subgraph_count=1, enable_vector_copy=false)')


def test_tc_wrt_912_doc_meta_table_ddl_uses_id_pk_ustore_and_kb_index():
    builder = GaussDBDDLBuilder(schema="public")

    statements = builder.build_doc_meta_table_ddls("ragflow_doc_meta_tenant_a")

    assert len(statements) == 2
    assert [_normalize_ddl(statement) for statement in statements] == [
        _normalize_ddl(
            """CREATE TABLE IF NOT EXISTS "public"."ragflow_doc_meta_tenant_a" (
              id VARCHAR(256) NOT NULL,
              kb_id VARCHAR(256) NOT NULL,
              meta_fields JSONB,
              PRIMARY KEY (id)
            ) WITH (storage_type=USTORE)"""
        ),
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_doc_meta_tenant_a_kb_id" ON "public"."ragflow_doc_meta_tenant_a" (kb_id)',
    ]
    assert "PRIMARY KEY (kb_id, id)" not in _normalize_ddl(statements[0])


def test_tc_wrt_913_regular_index_ddls_match_exact_column_matrix():
    builder = GaussDBDDLBuilder(schema="public")

    statements = builder.build_regular_index_ddls("ragflow_tenant_a")

    assert statements == [
        f'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_tenant_a_{column}" ON "public"."ragflow_tenant_a" ({column})'
        for column in ("doc_id", "available_int", "knowledge_graph_kwd", "entity_type_kwd", "removed_kwd")
    ]


def test_tc_wrt_914_fulltext_ugin_ddl_matches_normalized_snapshot():
    builder = GaussDBDDLBuilder(schema="public")

    simple_ddl = builder.build_fulltext_ugin_ddl("ragflow_tenant_a")
    ngram_ddl = builder.build_ngram_fulltext_ugin_ddl("ragflow_tenant_a")

    expected_simple = """CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_tenant_a_fts_all"
      ON "public"."ragflow_tenant_a"
      USING ugin(to_tsvector('simple', coalesce(title_tks, ' ') || ' ' || coalesce(title_sm_tks, ' ') || ' ' ||
      coalesce(important_tks, ' ') || ' ' || coalesce(question_tks, ' ') || ' ' || coalesce(content_ltks, ' ') || ' ' ||
      coalesce(content_sm_ltks, ' ')))"""
    expected_ngram = """CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_tenant_a_fts_all_ngram"
      ON "public"."ragflow_tenant_a"
      USING ugin(to_tsvector('ngram', coalesce(title_tks, ' ') || ' ' || coalesce(title_sm_tks, ' ') || ' ' ||
      coalesce(important_tks, ' ') || ' ' || coalesce(question_tks, ' ') || ' ' || coalesce(content_ltks, ' ') || ' ' ||
      coalesce(content_sm_ltks, ' ')))"""
    assert _normalize_ddl(simple_ddl) == _normalize_ddl(expected_simple)
    assert _normalize_ddl(ngram_ddl) == _normalize_ddl(expected_ngram)
