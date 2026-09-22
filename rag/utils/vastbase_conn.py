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
#  See the License for the specific language and the permissions and
#  limitations under the License.
#

import contextlib
import logging
import os
import re
import json
import time
import copy
from typing import Any
import psycopg2
from psycopg2 import sql
from psycopg2.extras import execute_values
import pandas as pd
from sqlalchemy import create_engine
from sqlalchemy.pool import QueuePool
from urllib.parse import quote_plus
import numpy as np

from common import settings
from common.constants import PAGERANK_FLD, TAG_FLD
from common.decorator import singleton
from common.doc_store.doc_store_base import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)
from common.file_utils import get_project_base_directory
from common.float_utils import get_float

logger = logging.getLogger('ragflow.vastbase_conn')
logger.setLevel(logging.INFO)


def get_table_exists(conn: psycopg2.extensions.connection, table_name: str) -> bool:
    """Get a table exists from a connection."""
    with conn.cursor() as cur:
        cur.execute("""
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = %s
        )
        """, (table_name,))
        return cur.fetchone()[0]


def get_table_instance(conn: psycopg2.extensions.connection, table_name: str):
    """Get a table columns from a connection."""
    with conn.cursor() as cur:
        check_table_exists_sql = """
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = %s
        )
        """
        cur.execute(check_table_exists_sql, (table_name,))
        table_exists = cur.fetchone()[0]
        if table_exists:
            table_columns_sql = """
            SELECT column_name, data_type, column_default, is_nullable
            FROM information_schema.columns
            WHERE table_name=%s
            """
            cur.execute(table_columns_sql, (table_name,))
            return cur.fetchall()
        else:
            return None


def field_keyword(field_name: str):
    # The "docnm_kwd" field is always a string, not list.
    if field_name == "source_id" or (field_name.endswith("_kwd") and field_name != "docnm_kwd" and field_name != "knowledge_graph_kwd"):
        return True
    return False


def quote_ident(name: str) -> str:
    """Quote a SQL identifier for use in raw SQL strings, handling embedded double quotes."""
    return '"' + name.replace('"', '""') + '"'


def equivalent_condition_to_str(condition: dict, table_instance=None) -> str | None:
    assert "_id" not in condition
    clmns = {}
    if table_instance:
        for n, ty, de, _ in table_instance:
            clmns[n] = (ty, de)

    def exists(cln):
        nonlocal clmns
        assert cln in clmns, f"'{cln}' should be in '{clmns}'."
        return f"{quote_ident(cln)} IS NOT NULL"

    cond = list()
    for k, v in condition.items():
        if not isinstance(k, str) or k in ["kb_id"] or not v:
            continue
        if field_keyword(k):
            pass
        elif isinstance(v, list):
            inCond = list()
            for item in v:
                if isinstance(item, str):
                    item = item.replace("'", "''")
                    inCond.append(f"'{item}'")
                else:
                    inCond.append(str(item))
            if inCond:
                strInCond = ", ".join(inCond)
                strInCond = f"{quote_ident(k)} IN ({strInCond})"
                cond.append(strInCond)
        elif k == "must_not":
            if isinstance(v, dict):
                for kk, vv in v.items():
                    if kk == "exists":
                        assert vv in clmns, f"'{vv}' should be in '{clmns}'."
                        cond.append(f"{quote_ident(vv)} IS NULL")
        elif k == "exists":
            cond.append(exists(v))
        elif isinstance(v, str):
            cond.append(f"{quote_ident(k)}='{v}'")
        else:
            cond.append(f"{quote_ident(k)}={str(v)}")
    return " AND ".join(cond) if cond else "1=1"


def select_identifier(field: str) -> sql.Composable:
    """Return a SQL identifier, handling * as a literal asterisk (not a quoted column name)."""
    if field == "*":
        return sql.SQL("*")
    return sql.Identifier(field)


def format_stage_preview(cols: list[str], rows: list[tuple], value_name: str, max_rows: int = 50) -> str:
    """Format standalone stage-recall rows as ``id=.. doc=.. <value>=..`` strings.

    Columns are located by name because the position of id/docnm_kwd depends
    on the caller's select_fields. ``value_name`` is the score/similarity
    column ("SCORE" or "SIMILARITY"); falls back to the last column.
    """
    lowered = [c.lower() for c in cols]
    id_i = lowered.index("id") if "id" in lowered else 0
    doc_i = lowered.index("docnm_kwd") if "docnm_kwd" in lowered else None
    val_i = next((i for i, c in enumerate(cols) if c.upper() == value_name.upper()), len(cols) - 1)
    label = value_name.lower()

    parts = []
    for r in rows[:max_rows]:
        s = f"id={r[id_i]}"
        if doc_i is not None:
            s += f" doc={r[doc_i]}"
        s += f" {label}={r[val_i]}"
        parts.append(s)
    return "\n".join(parts)


def concat_dataframes(df_list: list[pd.DataFrame], select_fields: list[str]) -> pd.DataFrame:
    df_list2 = [df for df in df_list if not df.empty]
    if df_list2:
        return pd.concat(df_list2, axis=0).reset_index(drop=True)

    schema = []
    for field_name in select_fields:
        if field_name == 'score()':
            schema.append('SCORE')
        elif field_name == 'similarity()':
            schema.append('SIMILARITY')
        else:
            schema.append(field_name)
    return pd.DataFrame(columns=schema)


@singleton
class VBConnection(DocStoreConnection):
    def __init__(self):
        self.dbName = settings.VB.get("db_name", "rag_flow")
        vb_host = settings.VB.get("host", "vastbase")
        vb_port = settings.VB.get("port", 5432)
        vb_user = settings.VB.get("user", "rag_flow")
        vb_password = settings.VB.get("password", "infini_rag_flow")
        self.db_compatibility = settings.VB.get("dbcompatibility", "PG").upper()

        # Connection-budget sizing. Vastbase is usually shared with the peewee
        # metadata DB (service_conf `vastbase.max_connections`, default 50) and
        # the pool lives per process (ragflow_server + WS task_executors). The
        # server's max_connections must cover, across ALL processes:
        #     (VB_POOL_SIZE + VB_MAX_OVERFLOW + 50) * (WS + 1)  (+ headroom)
        # e.g. defaults 50+100=150/process, WS=1 -> 2 processes -> 2*(150+50)=400
        # connections, so the server needs max_connections well above that, or
        # you'll hit "FATAL: Too many clients already". Lower these env vars
        # and/or raise the server's max_connections to fit.
        pool_size = int(os.getenv("VB_POOL_SIZE", "50"))
        max_overflow = int(os.getenv("VB_MAX_OVERFLOW", "100"))
        pool_timeout = int(os.getenv("VB_POOL_TIMEOUT", "30"))
        # Recycle connections periodically so the server / middleboxes can't
        # silently close idle ones underneath us. pool_pre_ping also pings on
        # every checkout, but recycle bounds how stale a conn can get.
        pool_recycle = int(os.getenv("VB_POOL_RECYCLE", "1800"))

        self.engine = None

        logger.info(f"Use Vastbase with floatvector at {vb_host}:{vb_port} as the doc engine.")

        url = (
            f"postgresql+psycopg2://{quote_plus(vb_user)}:{quote_plus(vb_password)}"
            f"@{vb_host}:{vb_port}/{self.dbName}"
        )
        # keepalives + statement_timeout mirror the old psycopg2 pool settings.
        connect_args = {
            "keepalives": 1,
            "keepalives_idle": 60,
            "keepalives_interval": 30,
            "keepalives_count": 10,
            "options": "-c statement_timeout=300000",
        }

        # Try to connect to Vastbase
        for _ in range(24):
            try:
                engine = create_engine(
                    url,
                    poolclass=QueuePool,
                    pool_size=pool_size,
                    max_overflow=max_overflow,
                    pool_timeout=pool_timeout,
                    pool_recycle=pool_recycle,
                    pool_pre_ping=True,
                    connect_args=connect_args,
                )
                # Test connection
                raw = engine.raw_connection()
                try:
                    cur = raw.cursor()
                    cur.execute("SELECT 1")
                    cur.fetchone()
                    cur.close()
                finally:
                    raw.close()
                self.engine = engine
                break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting Vastbase {vb_host}:{vb_port} to be healthy.")
                time.sleep(5)

        if self.engine is None:
            msg = f"Vastbase {vb_host}:{vb_port} is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)

        logger.info(f"Vastbase {vb_host}:{vb_port} is healthy.")

    @contextlib.contextmanager
    def get_conn(self):
        """Yield a live psycopg2 connection checked out from the SQLAlchemy pool.

        The engine is built with ``pool_pre_ping=True`` and ``pool_recycle``:
        on every checkout SQLAlchemy pings the connection and transparently
        discards + replaces any that the server has closed, so dead/stale
        connections can no longer be handed back out. That removes the need
        for the manual ``SELECT 1`` probe, the retry loop, and the background
        health-check thread that the old ``ThreadedConnectionPool`` path
        required (and which returned dead connections to the pool via
        ``putconn`` instead of discarding them).
        """
        # raw_connection() returns a pool proxy; .dbapi_connection is the real
        # psycopg2 connection, so psycopg2.sql.* composition (as_string(conn)),
        # cursors, commit/rollback all keep working unchanged. `raw` owns the
        # return-to-pool lifecycle via its close().
        raw = self.engine.raw_connection()
        try:
            yield raw.dbapi_connection
        except Exception as e:
            logger.error(f"Error in Vastbase connection: {str(e)}")
            try:
                raw.rollback()
            except Exception:
                pass
            raise
        finally:
            try:
                raw.close()
            except Exception:
                pass

    def dispose(self):
        """Close all pooled connections. Safe to call multiple times."""
        engine = getattr(self, "engine", None)
        if engine is not None:
            try:
                engine.dispose()
            except Exception:
                pass

    def __del__(self):
        self.dispose()

    """
    Database operations
    """

    def db_type(self) -> str:
        return "vastbase"

    def _vector_distance_op(self) -> str:
        """Return the cosine distance operator based on db compatibility mode."""
        return "<=>" if self.db_compatibility == "PG" else "<+>"

    def health(self) -> dict:
        """Return the health status of the database."""
        with self.get_conn() as vb_conn:
            try:
                with vb_conn.cursor() as cur:
                    cur.execute("SELECT vb_version()")
                    cur.fetchone()
                res = {
                    "type": "vastbase",
                    "status": "green",
                    "error": ""
                }
            except Exception as e:
                res = {
                    "type": "vastbase",
                    "status": "red",
                    "error": str(e)
                }
            return res

    """
    Table operations
    """

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        """Create a table and necessary indexes for vector storage"""
        table_name = f"{index_name}_{dataset_id}"
        with self.get_conn() as vb_conn:
            fp_mapping = os.path.join(
                get_project_base_directory(), "conf", "vastbase_mapping.json"
            )
            if not os.path.exists(fp_mapping):
                raise Exception(f"Mapping file not found at {fp_mapping}")
            schema = json.load(open(fp_mapping))
            vector_name = f"q_{vector_size}_vec"

            columns = []
            # Process field definitions from mapping
            for field_name, field_info in schema.items():
                field_type = field_info["type"]
                field_default = field_info['default']
                columns.append(sql.SQL("{field_name} {field_type} DEFAULT {field_default}").format(
                    field_name=sql.Identifier(field_name),
                    field_type=sql.SQL(field_type),
                    field_default=sql.Literal(field_default)
                ))

            # Add vector field
            columns.append(sql.SQL("{vector_name} floatvector({vectorSize})").format(
                vector_name=sql.Identifier(vector_name),
                vectorSize=sql.SQL(str(vector_size))
            ))

            # Create table
            create_table_sql = sql.SQL("""
            CREATE TABLE IF NOT EXISTS {table_name} (
                {columns}
            )
            """).format(
                table_name=sql.Identifier(table_name),
                columns=sql.SQL(", ").join(columns)
            )

            with vb_conn.cursor() as cur:
                cur.execute(create_table_sql)
                logging.debug(f"VASTBASE create table SQL: {create_table_sql.as_string(vb_conn)}")
                # Create vector index using Graph Index
                create_q_vex_idx_sql = sql.SQL("""
                CREATE INDEX IF NOT EXISTS {index_name}
                ON {table_name} USING graph_index ({vector_name} floatvector_cosine_ops)
                WITH (m=16, ef_construction=50)
                """).format(
                    index_name=sql.Identifier(f'q_vec_idx_{table_name}'),
                    table_name=sql.Identifier(table_name),
                    vector_name=sql.Identifier(vector_name)
                )
                cur.execute(create_q_vex_idx_sql)
                vb_conn.commit()

                # Determine Vastbase compatibility mode from settings.
                #   - "PG" — PG-compatible mode: use GIN + to_tsvector for fulltext indexes.
                #   - "B"  — MySQL-compatible mode: use ALTER TABLE ... ADD FULLTEXT INDEX.
                db_compatibility = self.db_compatibility

                text_idx_fields = [
                    "title_tks",
                    "title_sm_tks",
                    "important_kwd",
                    "important_tks",
                    "question_tks",
                    "content_ltks",
                    "content_sm_ltks"
                ]

                if db_compatibility == "PG":
                    # PG-compatible: single GIN index with to_tsvector.
                    try:
                        field_list = sql.SQL(', ').join(
                            sql.SQL("to_tsvector('cn_tokenizer', {})").format(sql.Identifier(f))
                            for f in text_idx_fields
                        )
                        pg_fts_sql = sql.SQL("""
                            CREATE INDEX IF NOT EXISTS {index_name}
                            ON {table_name} USING gin({field_list})
                        """).format(
                            index_name=sql.Identifier(f'text_gin_idx_{table_name}'),
                            table_name=sql.Identifier(table_name),
                            field_list=field_list
                        )
                        logging.debug(f"VASTBASE create PG fulltext index SQL: {pg_fts_sql.as_string(vb_conn)}")
                        cur.execute(pg_fts_sql)
                        vb_conn.commit()
                        logger.info(
                            f"VASTBASE created PG fulltext index for table {table_name}"
                        )
                    except Exception as e:
                        logging.warning(
                            f"VASTBASE PG fulltext index failed, "
                            f"vector search will work without it: {e}"
                        )
                        vb_conn.rollback()
                elif db_compatibility == "B":
                    # MySQL-compatible: ALTER TABLE ... ADD FULLTEXT INDEX per field.
                    for f in text_idx_fields:
                        try:
                            mysql_fts_sql = sql.SQL("""
                                ALTER TABLE {table_name}
                                ADD INDEX {index_name} USING "fulltext" ({field_name})
                            """).format(
                                table_name=sql.Identifier(table_name),
                                index_name=sql.Identifier(f'{f}_fulltext_idx_{table_name}'),
                                field_name=sql.Identifier(f)
                            )
                            cur.execute(mysql_fts_sql)
                            vb_conn.commit()
                        except Exception as e2:
                            if "already exists" not in str(e2):
                                logging.warning(
                                    f"VASTBASE failed to create fulltext index for {f}: {e2}, "
                                    f"vector search will work without it"
                                )
                            vb_conn.rollback()
                    logger.info(
                        f"VASTBASE created MySQL fulltext indexes for table {table_name}"
                    )
                else:
                    logger.warning(
                        f"VASTBASE unknown dbcompatibility '{db_compatibility}', "
                        f"skipping fulltext index creation"
                    )

    def delete_idx(self, index_name: str, dataset_id: str):
        """Drop the table for the given index and knowledgebase"""
        table_name = f"{index_name}_{dataset_id}"
        with self.get_conn() as vb_conn:
            with vb_conn.cursor() as cur:
                drop_index_sql = sql.SQL("DROP TABLE IF EXISTS {table_name}").format(
                    table_name=sql.Identifier(table_name)
                )
                cur.execute(drop_index_sql)
                vb_conn.commit()

    def create_doc_meta_idx(self, index_name: str):
        """
        Create a document metadata table.

        Table name pattern: ragflow_doc_meta_{tenant_id}
        - Per-tenant metadata table for storing document metadata fields
        """
        table_name = index_name
        with self.get_conn() as vb_conn:
            if get_table_exists(vb_conn, table_name):
                return True
            fp_mapping = os.path.join(
                get_project_base_directory(), "conf", "doc_meta_vastbase_mapping.json"
            )
            if not os.path.exists(fp_mapping):
                logger.error(f"Document metadata mapping file not found at {fp_mapping}")
                return False
            schema = json.load(open(fp_mapping))
            columns = []
            for field_name, field_info in schema.items():
                field_type = field_info["type"]
                field_default = field_info['default']
                columns.append(sql.SQL("{field_name} {field_type} DEFAULT {field_default}").format(
                    field_name=sql.Identifier(field_name),
                    field_type=sql.SQL(field_type),
                    field_default=sql.Literal(field_default)
                ))
            create_table_sql = sql.SQL("""
            CREATE TABLE IF NOT EXISTS {table_name} (
                {columns}
            )
            """).format(
                table_name=sql.Identifier(table_name),
                columns=sql.SQL(", ").join(columns)
            )
            with vb_conn.cursor() as cur:
                cur.execute(create_table_sql)
                vb_conn.commit()
            return True

    def index_exist(self, index_name: str, dataset_id: str) -> bool:
        """Check if the table exists for the given index and knowledgebase"""
        if index_name.startswith("ragflow_doc_meta_"):
            table_name = index_name
        else:
            table_name = f"{index_name}_{dataset_id}"
        with self.get_conn() as vb_conn:
            exists = get_table_exists(vb_conn, table_name)
            return exists

    """
    CRUD operations
    """

    def search(
            self, select_fields: list[str],
            highlight_fields: list[str],
            condition: dict,
            match_expressions: list[MatchExpr],
            order_by: OrderByExpr,
            offset: int,
            limit: int,
            index_names: str | list[str],
            knowledgebase_ids: list[str],
            agg_fields: list[str] = [],
            rank_feature: dict | None = None
    ) -> tuple[pd.DataFrame, int]:
        """
        TODO: Vastbase doesn't provide highlight
        """
        # logger.info(
        #     f"VASTBASE search called: index_names={index_names}, "
        #     f"kb_ids={knowledgebase_ids}, select_fields={select_fields}, "
        #     f"condition_keys={list(condition.keys()) if condition else None}, "
        #     f"match_expr_count={len(match_expressions)}, offset={offset}, limit={limit}"
        # )
        # for i, expr in enumerate(match_expressions):
        #     logger.debug(f"VASTBASE match_expr[{i}]: type={type(expr).__name__}, "
        #                 f"content={json.dumps(expr.__dict__, default=str)[:500]}")
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        _search_t0 = time.time()
        with self.get_conn() as vb_conn:
            df_list = list()
            table_list = list()
            output = select_fields.copy()
            # When * is selected, it already includes all columns — don't add individual fields.
            if "*" not in output:
                for essential_field in ["id"]:
                    if essential_field not in select_fields:
                        output.append(essential_field)
            score_func = ""
            score_column = ""
            for matchExpr in match_expressions:
                if isinstance(matchExpr, MatchTextExpr):
                    score_func = "score()"
                    score_column = "SCORE"
                    break
            if not score_func:
                for matchExpr in match_expressions:
                    if isinstance(matchExpr, MatchDenseExpr):
                        score_func = "similarity()"
                        score_column = "SIMILARITY"
                        break
            if match_expressions:
                if PAGERANK_FLD not in output:
                    output.append(PAGERANK_FLD)
            output = [f for f in output if f not in ["_score", "row_id()"]]

            # Prepare expressions common to all tables
            filter_cond = None
            filter_fulltext = None
            # Per-column @~@ BM25 expressions. When more than one field is searched,
            # these are combined via UNION ALL (not OR) at query time — see note below.
            fulltext_ft_parts: list = []
            filter_vector = None
            if condition:
                for indexName in index_names:
                    if indexName.startswith("ragflow_doc_meta_"):
                        table_name = indexName
                    else:
                        table_name = f"{indexName}_{knowledgebase_ids[0]}"
                    table_instance = get_table_instance(vb_conn, table_name)
                    if table_instance:
                        filter_cond = equivalent_condition_to_str(condition, table_instance)
                        break

            vector_similarity_weight = 0.5
            vector_query_data = None  # (vec_col, vec_data, vec_topn) for debug logging
            for matchExpr in match_expressions:
                if isinstance(matchExpr, MatchTextExpr):
                    minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                    if isinstance(minimum_should_match, float):
                        minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    if filter_cond and "filter" not in matchExpr.extra_options:
                        matchExpr.extra_options.update({"filter": filter_cond})
                    pattern = r'[~^]\d+(?:\.\d+)?'
                    matching_text = re.sub(pattern, '', matchExpr.matching_text)
                    fields = list()
                    pattern = r'^(.+?)(?:\^(\d+(?:\.\d+)?))?$'
                    for field in matchExpr.fields:
                        match = re.match(pattern, field)
                        if match:
                            field_name = match.group(1)
                            field_weight = match.group(2) if match.group(2) else "1"
                            fields.append((field_name, field_weight))
                    if fields:
                        # B mode: @~@ for fulltext search with parameters.
                        # NOTE: joining multiple @~@ operators with OR makes the
                        # PostgreSQL/ParadeDB planner pick a BitmapOr node, which
                        # drops the BM25 scan context so bm25_score() returns NULL
                        # (ParadeDB #2038, Neon #12853). We therefore keep the
                        # per-column expressions separate here and let the query
                        # builder run one BM25 scan per column, combining them with
                        # UNION ALL instead of OR.
                        for field_name, field_weight in fields:
                            fulltext_ft_parts.append(sql.SQL("{column} @~@ {matching_text}").format(
                                column=sql.Identifier(field_name),
                                matching_text=sql.Literal(f"{matching_text} @<PARAM:MINIMUM_SHOULD_MATCH={minimum_should_match} PARAM:BOOST={field_weight}>@")
                            ))
                        # Single-column search can stay a plain WHERE predicate.
                        if len(fulltext_ft_parts) == 1:
                            filter_fulltext = fulltext_ft_parts[0]
                            if filter_cond:
                                filter_fulltext = sql.SQL("({filter_cond}) AND ({filter_fulltext})").format(
                                    filter_cond=sql.SQL(filter_cond),
                                    filter_fulltext=filter_fulltext
                                )
                elif isinstance(matchExpr, MatchDenseExpr):
                    similarity = matchExpr.extra_options.get("similarity")
                    vector_name = matchExpr.vector_column_name
                    vec_len = len(matchExpr.embedding_data) if matchExpr.embedding_data else 0
                    vector_query_data = (matchExpr.vector_column_name, matchExpr.embedding_data, matchExpr.topn)
                    if similarity is not None:
                        filter_vector = sql.SQL("1 - ({vec_col} " + self._vector_distance_op() + " {vec}) >= {similarity}").format(
                            vec_col=sql.Identifier(matchExpr.vector_column_name),
                            vec=sql.Literal([float(v) for v in matchExpr.embedding_data]),
                            similarity=sql.Literal(similarity),
                        )
                elif isinstance(matchExpr, FusionExpr):
                    if isinstance(matchExpr, FusionExpr) and matchExpr.method == "weighted_sum" and "weights" in matchExpr.fusion_params:
                        assert len(match_expressions) == 3 and isinstance(match_expressions[0], MatchTextExpr) and isinstance(
                            match_expressions[1],
                            MatchDenseExpr) and isinstance(
                            match_expressions[2], FusionExpr)
                        weights = matchExpr.fusion_params["weights"]
                        vector_similarity_weight = float(weights.split(",")[1])
                        fulltext_weight = 1 - vector_similarity_weight
                        # logger.info(f"VASTBASE fusion weighted_sum: vector_similarity_weight={vector_similarity_weight}, fulltext_weight={fulltext_weight}")

            order_by_expr_list = list()
            if order_by.fields:
                for order_field in order_by.fields:
                    if order_field[1] == 0:
                        order_by_expr_list.append((order_field[0], "ASC"))
                    else:
                        order_by_expr_list.append((order_field[0], "DESC"))

            total_hits_count = 0
            # Scatter search tables and gather the results
            for indexName in index_names:
                is_meta = indexName.startswith("ragflow_doc_meta_")
                for knowledgebaseId in knowledgebase_ids:
                    # doc_meta tables don't have kb_id suffix
                    table_name = indexName if is_meta else f"{indexName}_{knowledgebaseId}"
                    try:
                        table_exists = get_table_exists(vb_conn, table_name)
                        if not table_exists:
                            continue
                    except Exception:
                        logger.warning(f"Error checking table {table_name}, skipping...")
                        continue
                    table_list.append(table_name)
                    select_fields_sql = sql.SQL(', ').join([select_identifier(field) for field in output])
                    sql_expr = None
                    filter_fulltext_expr = None
                    filter_vector_expr = None
                    fused_query = False
                    if len(match_expressions) > 0:
                        for matchExpr in match_expressions:
                            if isinstance(matchExpr, MatchTextExpr):
                                if filter_fulltext is None and not fulltext_ft_parts:
                                    continue
                                # The text expr's topn (100, fixed by FulltextQueryer)
                                # caps fulltext recall well below the vector side when
                                # the request topk is large (topk=1024 recalls 1024
                                # vectors but only 100 bm25 rows, starving the weighted
                                # sum). Scale the fulltext CTE limit up to the fusion
                                # topn; never below the text expr's own topn.
                                fulltext_limit = matchExpr.topn
                                for expr in match_expressions:
                                    if isinstance(expr, FusionExpr):
                                        fulltext_limit = max(fulltext_limit, expr.topn)
                                if len(fulltext_ft_parts) > 1:
                                    # Multi-field BM25: one @~@ scan per column (each keeps its
                                    # own bm25_score context), UNION ALL'd and deduplicated so a
                                    # row matching several fields is kept once with its best score.
                                    # Avoids the BitmapOr plan that nulls out bm25_score().
                                    per_column_limit = max(fulltext_limit * 2, 1)
                                    union_branches = []
                                    for part in fulltext_ft_parts:
                                        branch_where = part
                                        if filter_cond:
                                            branch_where = sql.SQL("({filter_cond}) AND ({part})").format(
                                                filter_cond=sql.SQL(filter_cond),
                                                part=part,
                                            )
                                        union_branches.append(sql.SQL("""
                                        (SELECT {select_fields}, bm25_score() AS bm25_score
                                        FROM {table_name}
                                        WHERE {branch_where}
                                        ORDER BY bm25_score DESC
                                        LIMIT {limit})
                                        """).format(
                                            select_fields=select_fields_sql,
                                            table_name=sql.Identifier(table_name),
                                            branch_where=branch_where,
                                            limit=sql.Literal(per_column_limit),
                                        ))
                                    filter_fulltext_expr = sql.SQL("""
                                    SELECT {select_fields}, "SCORE"
                                    FROM (
                                        SELECT DISTINCT ON (id) {select_fields}, bm25_score AS "SCORE"
                                        FROM (
                                            {union_all}
                                        ) AS unioned
                                        ORDER BY id, "SCORE" DESC
                                    ) AS deduped
                                    ORDER BY "SCORE" DESC
                                    LIMIT {limit}
                                    """).format(
                                        select_fields=select_fields_sql,
                                        union_all=sql.SQL(" UNION ALL ").join(union_branches),
                                        limit=sql.Literal(fulltext_limit),
                                    )
                                else:
                                    filter_fulltext_expr = sql.SQL("""
                                    SELECT {select_fields}, bm25_score as "SCORE"
                                    FROM (SELECT {select_fields}, bm25_score() as bm25_score
                                    FROM {table_name}
                                    WHERE {filter_fulltext}
                                    ORDER BY bm25_score DESC
                                    LIMIT {limit})
                                    """).format(
                                        select_fields=select_fields_sql,
                                        table_name=sql.Identifier(table_name),
                                        filter_fulltext=filter_fulltext,
                                        limit=sql.Literal(fulltext_limit)
                                    )
                                sql_expr = filter_fulltext_expr
                            elif isinstance(matchExpr, MatchDenseExpr):
                                if filter_vector is None:
                                    continue
                                # Pure k-NN: ORDER BY vec <+> q LIMIT n only.
                                # Do NOT put a vector-distance expression in WHERE —
                                # that makes the planner fall back to Seq Scan and
                                # bypass the graph_index (HNSW), turning this into a
                                # brute-force exact KNN (seconds vs milliseconds).
                                # The similarity threshold is intentionally dropped
                                # here; it is loose (default 0.2) and topn already
                                # returns the most similar rows.
                                filter_vector_expr = sql.SQL("""
                                SELECT {select_fields}, (1-({vec_col} """ + self._vector_distance_op() + """ {vec})) AS "SIMILARITY"
                                FROM {table_name}
                                ORDER BY {vec_col} """ + self._vector_distance_op() + """ {vec}
                                LIMIT {limit}
                                """).format(
                                    select_fields=select_fields_sql,
                                    vec_col=sql.Identifier(matchExpr.vector_column_name),
                                    vec=sql.Literal([float(v) for v in matchExpr.embedding_data]),
                                    table_name=sql.Identifier(table_name),
                                    limit=sql.Literal(matchExpr.topn)
                                )
                                if not sql_expr:
                                    sql_expr = filter_vector_expr
                            elif isinstance(matchExpr, FusionExpr):
                                fused_query = True
                                # Normalize the fulltext bm25 SCORE to [0,1] before
                                # the weighted sum. Raw bm25 (tens~hundreds, further
                                # inflated by field PARAM:BOOST) fused directly with
                                # the [0,1] cosine SIMILARITY lets ANY fulltext hit
                                # outrank EVERY vector-only row, so the fusion
                                # degenerates into fulltext-only recall and the
                                # vector stage never contributes candidates.
                                # Mirrors Infinity, which also normalizes each
                                # way's score before fusion (see rag/nlp/search.py).
                                # Divisor falls back to 1 when fulltext is empty or
                                # all-zero so vector-only rows keep a numeric score.
                                fused_score = sql.SQL(
                                    '(COALESCE(a."SCORE", 0) / COALESCE(NULLIF((SELECT MAX("SCORE") FROM filter_fulltext), 0), 1) * {fulltext_weight}'
                                    ' + COALESCE(b."SIMILARITY", 0) * {vector_similarity_weight})'
                                ).format(
                                    fulltext_weight=sql.Literal(1 - vector_similarity_weight),
                                    vector_similarity_weight=sql.Literal(vector_similarity_weight),
                                )
                                sql_expr = sql.SQL("""
                                WITH filter_fulltext AS ({filter_fulltext_expr}),
                                     filter_vector AS ({filter_vector_expr})
                                SELECT {select_fields}, {fused_score} AS "SCORE"
                                FROM filter_fulltext a
                                FULL OUTER JOIN filter_vector b
                                ON a.id = b.id
                                ORDER BY {fused_score} DESC
                                LIMIT {limit}
                                """).format(
                                    filter_fulltext_expr=filter_fulltext_expr,
                                    filter_vector_expr=filter_vector_expr,
                                    select_fields=sql.SQL(', ').join([sql.SQL("COALESCE(a.{field},b.{field}) AS {field}").format(
                                        field=sql.Identifier(field)
                                        ) for field in output]),
                                    fused_score=fused_score,
                                    score_column=sql.Identifier(score_column),
                                    limit=sql.Literal(matchExpr.topn)
                                )
                    else:
                        if filter_cond and len(filter_cond) > 0:
                            sql_expr = sql.SQL("""
                            SELECT {select_fields}
                            FROM {table_name}
                            WHERE {filter_clause}
                            """).format(
                                select_fields=select_fields_sql,
                                table_name=sql.Identifier(table_name),
                                filter_clause=sql.SQL(filter_cond)
                            )
                    if sql_expr is None:
                        logger.warning(
                            "VBConnection.search skip table=%s because sql_expr is None. match_expressions=%s filter_cond=%s filter_fulltext=%s filter_vector=%s",
                            table_name,
                            [type(m).__name__ for m in match_expressions],
                            bool(filter_cond),
                            filter_fulltext_expr is not None,
                            filter_vector_expr is not None,
                        )
                        continue
                    sql_query = sql.SQL("""
                    SELECT *
                    FROM ({sub_query})
                    """).format(
                        sub_query=sql_expr
                    )
                    if order_by.fields:
                        sql_query = sql.SQL("""
                        {sql_query}
                        ORDER BY {order_clause}
                        """).format(
                            sql_query=sql_query,
                            order_clause=sql.SQL(', ').join([sql.SQL('{field} {sort}').format(
                                field=sql.Identifier(field),
                                sort=sql.SQL(sort)
                            ) for field, sort in order_by_expr_list])
                        )
                    sql_query = sql.SQL("""
                    {sql_query}
                    LIMIT {limit} OFFSET {offset}""").format(
                        sql_query=sql_query,
                        limit=sql.Literal(limit),
                        offset=sql.Literal(offset)
                    )
                    sql_str = sql_query.as_string(vb_conn)
                    _t0 = time.time()
                    with vb_conn.cursor() as cur:
                        cur.execute(sql_query)
                        column_names = [desc[0] for desc in cur.description]
                        rows = cur.fetchall()
                        if rows:
                            total_hits_count += cur.rowcount
                        kb_res = pd.DataFrame(rows, columns=column_names)
                        df_list.append(kb_res)
                    logger.debug(
                        "VBConnection.search [main] table=%s rows=%d elapsed=%.3fs | sql: %s",
                        table_name, len(rows), time.time() - _t0, sql_str,
                    )
                    # Fusion result log (INFO): the main query IS the fused
                    # result when FusionExpr ran, so log its rows alongside the
                    # standalone fulltext/vector stage logs below for a
                    # three-way comparison (same id= doc= value= format).

                    # if fused_query:
                    #     logger.info(
                    #         "VBConnection.search [stage=fusion] table=%s rows=%d "
                    #         "elapsed=%.3fs | fused score per row: %s | sql: %s",
                    #         table_name, len(rows),
                    #         time.time() - _t0,
                    #         format_stage_preview(column_names, rows, "SCORE"),
                    #         sql_str,
                    #     )

                    # Stage-by-stage recall debug: re-run the fulltext and vector
                    # sub-queries standalone so we can see which side is missing
                    # rows or scoring NULL (e.g. bm25_score() nulling out under OR,
                    # or vector returning nothing). COALESCE in the fusion hides
                    # these, so the sub-queries must be inspected on their own.
                    # try:
                    #     if filter_fulltext_expr is not None:
                    #         _t0 = time.time()
                    #         with vb_conn.cursor() as cur:
                    #             cur.execute(filter_fulltext_expr)
                    #             ft_cols = [d[0] for d in cur.description]
                    #             ft_rows = cur.fetchall() or []
                    #         logger.info(
                    #             "VBConnection.search [stage=fulltext] table=%s rows=%d "
                    #             "null_score=%d elapsed=%.3fs | score per row: %s | sql: %s",
                    #             table_name, len(ft_rows),
                    #             sum(1 for r in ft_rows if r[-1] is None),
                    #             time.time() - _t0,
                    #             format_stage_preview(ft_cols, ft_rows, "SCORE"),
                    #             filter_fulltext_expr.as_string(vb_conn),
                    #         )
                    #     if filter_vector_expr is not None:
                    #         _t0 = time.time()
                    #         with vb_conn.cursor() as cur:
                    #             cur.execute(filter_vector_expr)
                    #             vec_cols = [d[0] for d in cur.description]
                    #             vec_rows = cur.fetchall() or []
                    #         logger.info(
                    #             "VBConnection.search [stage=vector] table=%s rows=%d "
                    #             "null_similarity=%d elapsed=%.3fs | similarity per row: %s | sql: %s",
                    #             table_name, len(vec_rows),
                    #             sum(1 for r in vec_rows if r[-1] is None),
                    #             time.time() - _t0,
                    #             format_stage_preview(vec_cols, vec_rows, "SIMILARITY"),
                    #             filter_vector_expr.as_string(vb_conn),
                    #         )
                    # except Exception as e:
                    #     logger.warning("VBConnection.search stage debug failed: %s", e)

        res = concat_dataframes(df_list, output)

        # Total search time: connection checkout (incl. pre_ping) + all SQL +
        # result concat. Per-table breakdown stays at DEBUG below; this TOTAL
        # is INFO so it's visible without raising the log level. Compare it to
        # end-to-end retrieval latency to tell DB-bound from embed-bound.
        logger.info(
            "VBConnection.search TOTAL index_names=%s kb_ids=%s tables=%d hits=%d elapsed=%.3fs",
            index_names, knowledgebase_ids, len(table_list), total_hits_count, time.time() - _search_t0,
        )

        if match_expressions:
            # Use whichever score column is actually present in the result
            # PostgreSQL unquoted aliases are lowercased; check all variants
            score_col = score_column if score_column in res.columns else (
                "SIMILARITY" if "SIMILARITY" in res.columns else (
                    "SCORE" if "SCORE" in res.columns else (
                        "score" if "score" in res.columns else (
                            "similarity" if "similarity" in res.columns else None
                        )
                    )
                )
            )
            if score_col is None:
                return res, total_hits_count

            res['Sum'] = res[score_col] + res[PAGERANK_FLD]
            res = res.sort_values(by='Sum', ascending=False).reset_index(drop=True).drop(columns=['Sum'])
            res = res.head(limit)
        # Print summary: id, doc name, score (skip pagerank_fea vector)
        # if match_expressions and score_col in res.columns and 'docnm_kwd' in res.columns:
        #     score_cols = ['id', 'docnm_kwd', score_col]
        #     if 'SIMILARITY' in res.columns:
        #         score_cols.append('SIMILARITY')
        #     summary = res[score_cols].to_string(index=False)
        #     logger.info(f"VASTBASE result summary:\n{summary}")

        # Debug: run a standalone vector similarity query and log results
        # if vector_query_data and table_list:
        #     vec_col, vec_data, vec_topn = vector_query_data
        #     try:
        #         with self.get_conn() as vb_conn:
        #             for tbl in table_list:
        #                 sim_sql = sql.SQL("""
        #                     SELECT id, docnm_kwd, ({vec_col} """ + self._vector_distance_op() + """ {vec}) AS "VEC_DIST", (1-({vec_col} """ + self._vector_distance_op() + """ {vec})) AS "SIMILARITY"
        #                     FROM {table_name}
        #                     ORDER BY {vec_col} """ + self._vector_distance_op() + """ {vec}
        #                     LIMIT {limit}
        #                 """).format(
        #                     vec_col=sql.Identifier(vec_col),
        #                     vec=sql.Literal([float(v) for v in vec_data]),
        #                     table_name=sql.Identifier(tbl),
        #                     limit=sql.Literal(vec_topn),
        #                 )
        #                 with vb_conn.cursor() as cur:
        #                     cur.execute(sim_sql)
        #                     cols = [desc[0] for desc in cur.description]
        #                     rows = cur.fetchall()
        #                     if rows:
        #                         sim_df = pd.DataFrame(rows, columns=cols)
        #     except Exception as e:
        #         logger.warning(f"VASTBASE standalone vector similarity query failed: {e}")
        return res, total_hits_count

    def get(
            self, data_id: str, index_name: str, dataset_ids: list[str]
    ) -> dict | None:
        with self.get_conn() as vb_conn:
            df_list = list()
            assert isinstance(dataset_ids, list)
            table_list = list()
            for knowledgebaseId in dataset_ids:
                if index_name.startswith("ragflow_doc_meta_"):
                    table_name = index_name
                else:
                    table_name = f"{index_name}_{knowledgebaseId}"
                table_list.append(table_name)
                table_exists = get_table_exists(vb_conn, table_name)
                if not table_exists:
                    logger.warning(
                        f"Table not found: {table_name}, this knowledge base isn't created in Vastbase. Maybe it is created in other document engine.")
                    continue
                with vb_conn.cursor() as cur:
                    cur.execute(sql.SQL("SELECT * FROM {table_name} WHERE id = %s").format(
                        table_name=sql.Identifier(table_name)
                    ), (data_id,))
                    column_names = [desc[0] for desc in cur.description]
                    rows = cur.fetchall()
                kb_res = pd.DataFrame(rows, columns=column_names)
                df_list.append(kb_res)
            res = concat_dataframes(df_list, ["id"])
            res_fields = self.get_fields(res, res.columns.tolist())
            return res_fields.get(data_id, None)

    def insert(
            self, documents: list[dict], index_name: str, dataset_id: str = None
    ) -> list[str]:
        with self.get_conn() as vb_conn:
            if index_name.startswith("ragflow_doc_meta_"):
                table_name = index_name
            else:
                table_name = f"{index_name}_{dataset_id}"
            table_instance = get_table_instance(vb_conn, table_name)
            if not table_instance:
                # Need to create the table
                vector_size = 0
                patt = re.compile(r"q_(?P<vector_size>\d+)_vec")
                for k in documents[0].keys():
                    m = patt.match(k)
                    if m:
                        vector_size = int(m.group("vector_size"))
                        break
                if vector_size == 0:
                    raise ValueError("Cannot infer vector size from documents")
                self.create_idx(index_name, dataset_id, vector_size)
                table_instance = get_table_instance(vb_conn, table_name)

            if not table_instance:
                raise ValueError(f"Table {table_name} does not exist in Vastbase.")
            # embedding fields can't have a default value....
            embedding_clmns = []
            for n, ty, _, _ in table_instance:
                r = re.search(r"Embedding\([a-z]+,([0-9]+)\)", ty)
                if not r:
                    continue
                embedding_clmns.append((n, int(r.group(1))))

            docs = copy.deepcopy(documents)
            for d in docs:
                assert "_id" not in d
                assert "id" in d
                for k, v in d.items():
                    if field_keyword(k):
                        if isinstance(v, list):
                            d[k] = "###".join(v)
                        else:
                            d[k] = v
                    elif re.search(r"_feas$", k):
                        d[k] = json.dumps(v)
                    elif k == 'kb_id':
                        if isinstance(d[k], list):
                            d[k] = d[k][0]
                    elif k == "position_int":
                        assert isinstance(v, list)
                        d[k] = [num for row in v for num in row]
                    elif k in ["page_num_int", "top_int"]:
                        assert isinstance(v, list)
                        d[k] = v
                    else:
                        if isinstance(v, dict):
                            d[k] = json.dumps(v)
                        elif isinstance(v, np.ndarray):
                            d[k] = v.tolist()
                        else:
                            d[k] = v

                    for n, vs in embedding_clmns:
                        if n in d:
                            continue
                        d[n] = [0] * vs
            ids = [d["id"] for d in docs]
            with vb_conn.cursor() as cur:
                cur.execute(sql.SQL("DELETE FROM {} WHERE id IN %s").format(
                    sql.Identifier(table_name)
                ), (tuple(ids),))
                column_names = list(docs[0].keys())
                values = [tuple(doc.get(col, None) for col in column_names) for doc in docs]
                insert_sql = sql.SQL("INSERT INTO {table_name} ({column_names}) VALUES %s").format(
                    table_name=sql.Identifier(table_name),
                    column_names=sql.SQL(', ').join([sql.Identifier(col) for col in column_names])
                )
                execute_values(cur, insert_sql, values)
                vb_conn.commit()
            return []

    def update(
            self, condition: dict, new_value: dict, index_name: str, dataset_id: str
    ) -> bool:
        with self.get_conn() as vb_conn:
            table_name = f"{index_name}_{dataset_id}"
            table_instance = get_table_instance(vb_conn, table_name)

            clmns = {}
            if table_instance:
                for n, ty, de, _ in table_instance:
                    clmns[n] = (ty, de)
            filter = equivalent_condition_to_str(condition, table_instance)
            removeValue = {}
            for k, v in list(new_value.items()):
                if field_keyword(k):
                    if isinstance(v, list):
                        new_value[k] = "###".join(v)
                    else:
                        new_value[k] = v
                elif re.search(r"_feas$", k):
                    new_value[k] = json.dumps(v)
                elif k == 'kb_id':
                    if isinstance(new_value[k], list):
                        new_value[k] = new_value[k][0]
                elif k == "position_int":
                    assert isinstance(v, list)
                    new_value[k] = [num for row in v for num in row]
                elif k in ["page_num_int", "top_int"]:
                    assert isinstance(v, list)
                    new_value[k] = v
                elif k == "remove":
                    if isinstance(v, str):
                        assert v in clmns, f"'{v}' should be in '{clmns}'."
                        ty, de = clmns[v]
                        if ty.lower().find("cha"):
                            if not de:
                                de = ""
                        new_value[v] = de
                    else:
                        for kk, vv in v.items():
                            removeValue[kk] = vv
                        del new_value[k]
                else:
                    new_value[k] = v

            remove_opt = {}
            with vb_conn.cursor() as cur:
                if removeValue:
                    col_to_remove = list(removeValue.keys())
                    col_to_remove.append('id')
                    cur.execute(sql.SQL("SELECT {columns} FROM {table_name} WHERE {filter_clause}").format(
                        columns=sql.SQL(', ').join([sql.Identifier(col) for col in col_to_remove]),
                        table_name=sql.Identifier(table_name),
                        filter_clause=sql.SQL(filter)
                    ))
                    column_names = [desc[0] for desc in cur.description]
                    rows = cur.fetchall()
                    row_to_opt = pd.DataFrame(rows, columns=column_names)
                    row_to_opt = self.get_fields(row_to_opt, col_to_remove)
                    for id, old_v in row_to_opt.items():
                        for k, remove_v in removeValue.items():
                            if remove_v in old_v[k]:
                                new_v = old_v[k].copy()
                                new_v.remove(remove_v)
                                kv_key = json.dumps([k, new_v])
                                if kv_key not in remove_opt:
                                    remove_opt[kv_key] = [id]
                                else:
                                    remove_opt[kv_key].append(id)

                for update_kv, ids in remove_opt.items():
                    k, v = json.loads(update_kv)
                    cur.execute(sql.SQL("UPDATE {table_name} SET {k}=%s WHERE {filter_clause} AND id in %s").format(
                        table_name=sql.Identifier(table_name),
                        k=sql.Identifier(k),
                        filter_clause=sql.SQL(filter)
                    ), ("###".join(v), tuple(ids)))

                cur.execute(sql.SQL("UPDATE {table_name} SET {set_clause} WHERE {filter_clause}").format(
                    table_name=sql.Identifier(table_name),
                    set_clause=sql.SQL(', ').join([sql.SQL("{k}={v}").format(
                        k=sql.Identifier(k),
                        v=sql.Literal(v)
                    ) for k, v in new_value.items()]),
                    filter_clause=sql.SQL(filter)
                ))
                vb_conn.commit()
            return True

    def delete(self, condition: dict, index_name: str, dataset_id: str) -> int:
        with self.get_conn() as vb_conn:
            if index_name.startswith("ragflow_doc_meta_"):
                table_name = index_name
            else:
                table_name = f"{index_name}_{dataset_id}"
            table_instance = get_table_instance(vb_conn, table_name)
            if not table_instance:
                logger.warning(
                    "VBConnection.delete skipped (table missing) table=%s condition=%s",
                    table_name, condition,
                )
                return 0
            filter = equivalent_condition_to_str(condition, table_instance)
            delete_sql = sql.SQL("DELETE FROM {table_name} WHERE {filter_clause}").format(
                table_name=sql.Identifier(table_name),
                filter_clause=sql.SQL(filter)
            )
            sql_str = delete_sql.as_string(vb_conn)
            _t0 = time.time()
            with vb_conn.cursor() as cur:
                cur.execute(delete_sql)
                deleted_rows = cur.rowcount
                vb_conn.commit()
            logger.debug(
                "VBConnection.delete table=%s condition=%s deleted_rows=%d elapsed=%.3fs | sql: %s",
                table_name, condition, deleted_rows, time.time() - _t0, sql_str,
            )
            return deleted_rows

    def fetch_metadata_doc_ids(
        self,
        index_name: str,
        kb_ids: list[str],
        sql_filter: str,
        filter_params: list[Any],
        limit: int,
    ) -> list[str]:
        if not str(index_name or "").startswith("ragflow_doc_meta_"):
            raise ValueError(f"invalid Vastbase document metadata table name: {index_name}")
        scoped_kb_ids = [str(kb_id) for kb_id in (kb_ids or []) if kb_id]
        if not scoped_kb_ids or not sql_filter:
            return []
        adapted_filter = sql_filter.replace("meta_fields", "(meta_fields::jsonb)")
        placeholders = ", ".join(["%s"] * len(scoped_kb_ids))
        effective_limit = limit if limit and limit > 0 else 10000
        query = (
            f'SELECT id FROM {quote_ident(index_name)} '
            f"WHERE kb_id IN ({placeholders}) AND ({adapted_filter}) "
            f"ORDER BY id LIMIT %s"
        )
        params = [*scoped_kb_ids, *(filter_params or []), effective_limit]
        with self.get_conn() as vb_conn:
            with vb_conn.cursor() as cur:
                cur.execute(query, params)
                rows = cur.fetchall()
        return [str(row[0]) for row in rows if row and row[0] is not None]

    """
    Helper functions for search result
    """

    def get_total(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> int:
        if isinstance(res, tuple):
            return res[1]
        return len(res)

    def get_scores(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> dict[str, float]:
        """
        Map chunk id to its similarity score from a Vastbase search result.
        The score is stored in the SIMILARITY column when a MatchDenseExpr
        is used (e.g. by _knn_scores).
        """
        if isinstance(res, tuple):
            res = res[0]
        if res.empty:
            return {}
        if "SIMILARITY" in res.columns:
            return dict(zip(res["id"], res["SIMILARITY"].fillna(0.0).astype(float)))
        return {row["id"]: 0.0 for _, row in res.iterrows()}

    def get_doc_ids(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return list(res["id"])

    def get_fields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        if isinstance(res, tuple):
            res = res[0]
        if not fields:
            return {}
        fieldsAll = fields.copy()
        fieldsAll.append('id')
        column_map = {col.lower(): col for col in res.columns}
        # Map _score to the actual score column in Vastbase results
        score_aliases = {"_score": ["score", "similarity"]}
        for alias_field, candidates in score_aliases.items():
            if alias_field in fieldsAll and alias_field not in column_map:
                for c in candidates:
                    if c in column_map:
                        column_map[alias_field] = column_map[c]
                        break
        matched_columns = {column_map[col.lower()]: col for col in set(fieldsAll) if col.lower() in column_map}
        none_columns = [col for col in set(fieldsAll) if col.lower() not in column_map]

        res2 = res[matched_columns.keys()]
        res2 = res2.rename(columns=matched_columns)
        res2.drop_duplicates(subset=['id'], inplace=True)

        for column in res2.columns:
            if res2[column] is None:
                res2[column] = ""
                continue
            k = column.lower()
            if field_keyword(k):
                res2[column] = res2[column].apply(lambda v: [kwd for kwd in (v or '').split("###") if kwd])
            elif k == "position_int":
                def to_position_int(v):
                    if v:
                        # v is already a flat int list from integer[]
                        v = [v[i:i + 5] for i in range(0, len(v), 5)]
                    else:
                        v = []
                    return v
                res2[column] = res2[column].apply(to_position_int)
            elif k in ["page_num_int", "top_int"]:
                # v is already a list from integer[]
                res2[column] = res2[column].apply(lambda v: list(v) if v else [])
            elif k in ("metadata", "extra") or k.endswith("_feas"):
                # JSON stored as text → parse to dict
                res2[column] = res2[column].apply(
                    lambda v: json.loads(v) if isinstance(v, str) and v.strip().startswith("{") else (v or {})
                )
            else:
                pass
        for column in none_columns:
            res2[column] = None

        # Convert to dict, then strip None/NaN values per row (like OB does).
        result = res2.set_index("id").to_dict(orient="index")
        for row_id, row in result.items():
            for k in list(row.keys()):
                v = row[k]
                if v is None or (isinstance(v, float) and pd.isna(v)):
                    del row[k]
        return result

    def get_highlight(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, keywords: list[str], fieldnm: str):
        if isinstance(res, tuple):
            res = res[0]
        ans = {}
        num_rows = len(res)
        column_id = res["id"]
        if fieldnm not in res:
            return {}
        for i in range(num_rows):
            id = column_id[i]
            txt = res[fieldnm][i]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                for w in keywords:
                    t = re.sub(
                        r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])"
                        % re.escape(w),
                        r"\1<em>\2</em>\3",
                        t,
                        flags=re.IGNORECASE | re.MULTILINE,
                    )
                if not re.search(
                        r"<em>[^<>]+</em>", t, flags=re.IGNORECASE | re.MULTILINE
                ):
                    continue
                txts.append(t)
            ans[id] = "...".join(txts)
        return ans

    def get_aggregation(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fieldnm: str):
        """
        TODO: Vastbase doesn't provide aggregation
        """
        return list()

    """
    SQL
    """

    def sql(self, sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
