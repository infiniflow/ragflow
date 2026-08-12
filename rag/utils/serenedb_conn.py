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
"""SereneDB DocStoreConnection for RAGFlow.

Shape follows ob_conn.py (the plain-SQL sibling): ONE table per tenant index
(`ragflow_{tenant_id}`), kb_id is a filtered column, ES field names are kept verbatim so the
read path needs no renames (unlike infinity_conn). The engine-specific notes below were
verified against SereneDB 26.07.4:

  P1  ONE inverted index carries several text columns + the vector; `@@` works per column and
      OR across columns SUMS the per-column BM25 scores (natural field boosting — short
      title/keyword columns score higher via length norm).
  P2  VARCHAR[] columns + list_contains() filters work on base table and index relation;
      unnest() powers tag aggregation.
  P3  Weighted hybrid fusion is ONE SQL statement: BM25 branch normalized with
      `s / MAX(s) OVER ()` (window fn — on <26.07.4 a second CTE reference trips the
      iresearch_scan plan-copy bug; FIXED in 26.07.4 #962, but the window form is simpler AND
      version-portable so we keep it), FULL OUTER JOIN with the vector branch, weighted sum + pagerank.
  P4  Writes are immediately durable in the base table; the inverted index refreshes
      asynchronously (~1s converged in probe) — same contract as Elasticsearch's
      refresh_interval, which RAGFlow already tolerates.
  P6  psycopg2 returns native Python lists for arrays/vectors and dicts for JSON columns.
  P7  update()'s add/remove semantics map to list_append() / array_remove().

MINIMUM ENGINE VERSION: SereneDB 26.07.4. The vector-branch and fusion queries use the natural
forms that rely on the 26.07.4 fixes #964 (vector-op predicate in an ANN scan's WHERE) and #962
(multi-reference index CTE); on <26.07.4 both silently returned empty. Verified against the
released image 2026-07-23.

BM25 REQUIRES the dictionary to declare `frequency = true, norm = true` — without frequency the
scorer silently returns 0.0 for every row. Vectors are stored raw (ES mirror) AND as a
normalized shadow column indexed with metric='ip', quant='sq8': bge-m3 norms are 0.935-0.976,
not 1.0, so ip on unit vectors is the only way to get exact cosine AND quantization.

Deliberate first-cut simplifications (documented, revisit on in-app eval):
  - ES per-field boosts (^10 etc.) are approximated by the P1 summed-OR match over
    title/important/question/content token columns; content-only scoring reached ES-parity MRR
    on the gold eval, so this is headroom, not debt.
  - minimum_should_match is not enforced (OR semantics); IDF-noise is tamed by BM25 itself.
  - rank_feature tag boosting is skipped (parity: ob_conn TODOs it as well); pagerank IS applied.
"""

import json
import logging
import os
import re
import threading
import time
from dataclasses import dataclass, field
from urllib.parse import quote

import psycopg2
import psycopg2.extras
import psycopg2.pool

from common.doc_store.doc_store_base import (
    DocStoreConnection,
    FusionExpr,
    MatchDenseExpr,
    MatchExpr,
    MatchTextExpr,
    OrderByExpr,
)

logger = logging.getLogger("ragflow.serenedb_conn")

PAGERANK_FLD = "pagerank_fea"
ATTEMPT_TIME = 2

vector_column_pattern = re.compile(r"q_(?P<vector_size>\d+)_vec$")

# Column type map — ES mapping names verbatim. psycopg2 adapts python lists to arrays and
# json.dumps handles JSON columns on the write path; the read path gets native types (P6).
TEXT_COLUMNS = [
    "docnm_kwd",
    "doc_type_kwd",
    "title_tks",
    "title_sm_tks",
    "content_with_weight",
    "content_ltks",
    "content_sm_ltks",
    "important_tks",
    "question_tks",
    "create_time",
    "img_id",
    "knowledge_graph_kwd",
    "entity_kwd",
    "entity_type_kwd",
    "from_entity_kwd",
    "to_entity_kwd",
    "removed_kwd",
    "raptor_kwd",
    "group_id",
    "mom_id",
    "n_hop_with_weight",
]
ARRAY_COLUMNS = ["important_kwd", "question_kwd", "tag_kwd", "source_id", "entities_kwd"]
INT_COLUMNS = ["pagerank_fea", "available_int", "weight_int", "raptor_layer_int", "_order_id"]
FLOAT_COLUMNS = ["create_timestamp_flt", "weight_flt", "rank_flt"]
JSON_COLUMNS = ["tag_feas", "position_int", "page_num_int", "top_int", "chunk_data", "metadata", "extra", "meta_fields"]

COLUMN_DDL: dict[str, str] = {
    "id": "VARCHAR PRIMARY KEY",
    "kb_id": "VARCHAR",
    "doc_id": "VARCHAR",
    **{c: "TEXT" for c in TEXT_COLUMNS},
    **{c: "VARCHAR[]" for c in ARRAY_COLUMNS},
    **{c: "INTEGER" for c in INT_COLUMNS},
    **{c: "DOUBLE PRECISION" for c in FLOAT_COLUMNS},
    **{c: "JSON" for c in JSON_COLUMNS},
}
COLUMN_NAMES = list(COLUMN_DDL.keys())

# Text columns the inverted index carries (all indexable, used for @@ existence filters).
FTS_COLUMNS = ["title_tks", "important_tks", "question_tks", "content_ltks"]
# The SCORED lexical branch matches this ONE column. `ORDER BY BM25(idx.tableoid)` over a
# multi-column `@@` OR returns EMPTY (the WAND top-k iterator can't score a cross-column
# disjunction — same family as the other silent-empty scorer bugs). content_ltks is the
# dominant field and is what the parity eval scored on; ES-style field boosts (docnm^10 etc.)
# are deferred — reintroducing them needs per-column BM25 summed in Python, not an OR,
# precisely because of this bug.
LEX_SCORED_COL = "content_ltks"

DOC_META_DDL = {"id": "VARCHAR PRIMARY KEY", "kb_id": "VARCHAR", "meta_fields": "JSON"}

DEFAULTS = {"available_int": 1, "removed_kwd": "N", "_order_id": 0}

DICTIONARY_NAME = "rf_scored_delim"
# frequency/norm are what make BM25() score at all — see module docstring.
DICTIONARY_DDL = f"CREATE TEXT SEARCH DICTIONARY IF NOT EXISTS {DICTIONARY_NAME} (template = 'delimiter', delimiter = ' ', frequency = true, position = true, norm = true)"


def _index_relation(table_name: str) -> str:
    return f"idx_{table_name}"


def _norm_column(vector_size: int) -> str:
    return f"q_{vector_size}_vec_n"


def _escape(value) -> str:
    """SQL-literal encoding for the templated search statements (filters, aggregation)."""
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, (list, dict)):
        return "'" + json.dumps(value, ensure_ascii=False).replace("'", "''") + "'"
    return "'" + str(value).replace("'", "''") + "'"


def _strip_es_query(matching_text: str) -> str:
    """Fallback tokenization when extra_options lacks original_query: strip ES query_string
    syntax (boosts, quotes, boolean sugar) down to plain space-separated tokens for `@@`."""
    txt = re.sub(r"\^[0-9.]+", " ", matching_text)
    txt = re.sub(r'["()~*?:+\-]|\bAND\b|\bOR\b|\bNOT\b', " ", txt)
    toks = [t for t in txt.split() if t]
    seen, out = set(), []
    for t in toks:
        if t not in seen:
            seen.add(t)
            out.append(t)
    return " ".join(out)


def _l2_normalize(vec: list[float]) -> list[float]:
    s = sum(v * v for v in vec) ** 0.5
    if s == 0:
        return list(vec)
    return [v / s for v in vec]


@dataclass
class SearchResult:
    total: int = 0
    chunks: list[dict] = field(default_factory=list)


class SereneDBConnection(DocStoreConnection):
    _instance = None
    _instance_lock = threading.Lock()

    def __new__(cls, *args, **kwargs):
        with cls._instance_lock:
            if cls._instance is None:
                cls._instance = super().__new__(cls)
        return cls._instance

    def __init__(self):
        if getattr(self, "_initialized", False):
            return
        self._initialized = True
        dsn = None
        try:
            from common.settings import get_base_config  # inside RAGFlow

            cfg = get_base_config("serenedb", {}) or {}
            host = cfg.get("host", "serenedb")
            port = int(cfg.get("port", 7890))
            user = cfg.get("user", "postgres")
            password = cfg.get("password", "")
            dbname = cfg.get("db_name", "postgres")
            # URL-encode credentials so a password with @ / : / space does not
            # corrupt the DSN.
            dsn = f"postgresql://{quote(user, safe='')}:{quote(password, safe='')}@{host}:{port}/{quote(dbname, safe='')}"
            ssl_mode = cfg.get("ssl_mode")
            if ssl_mode:
                dsn += f"?sslmode={quote(str(ssl_mode), safe='')}"
        except Exception:
            pass
        dsn = os.environ.get("SERENEDB_DSN", dsn or "postgresql://postgres@serenedb:7890/")
        self._pool = psycopg2.pool.ThreadedConnectionPool(minconn=1, maxconn=8, dsn=dsn)
        self._known_tables: set[str] = set()
        self._known_lock = threading.Lock()
        self._dsn_display = re.sub(r":[^:@/]+@", ":***@", dsn)
        logger.info(f"SereneDB connection initialized: {self._dsn_display}")

    def _run(self, sql: str, params=None, fetch: bool = True):
        conn = self._pool.getconn()
        try:
            conn.autocommit = True
            with conn.cursor() as cur:
                cur.execute(sql, params)
                if fetch and cur.description is not None:
                    return cur.fetchall(), [d[0] for d in cur.description]
                return [], []
        finally:
            self._pool.putconn(conn)

    """
    Database operations
    """

    def db_type(self) -> str:
        return "serenedb"

    def health(self) -> dict:
        rows, _ = self._run("SELECT version()")
        return {"type": "serenedb", "status": "green", "dsn": self._dsn_display, "version": rows[0][0] if rows else "unknown"}

    """
    Table operations
    """

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        if index_name.startswith("ragflow_doc_meta_"):
            cols = ", ".join(f"{k} {t}" for k, t in DOC_META_DDL.items())
            self._run(f"CREATE TABLE IF NOT EXISTS {index_name} ({cols})", fetch=False)
            return
        cols = ", ".join(f"{k} {t}" for k, t in COLUMN_DDL.items())
        vec, vec_n = f"q_{vector_size}_vec", _norm_column(vector_size)
        self._run(f"CREATE TABLE IF NOT EXISTS {index_name} ({cols}, {vec} FLOAT[{vector_size}], {vec_n} FLOAT[{vector_size}])", fetch=False)
        self._run(DICTIONARY_DDL, fetch=False)
        fts = ", ".join(f"{c} {DICTIONARY_NAME}" for c in FTS_COLUMNS)
        self._run(
            f"CREATE INDEX IF NOT EXISTS {_index_relation(index_name)} ON {index_name} "
            f"USING inverted (id, {fts}, {vec_n} ivf (metric = 'ip', quant = 'sq8')) "
            f"WITH (optimize_top_k = 'bm25(1.2, 0.75)')",
            fetch=False,
        )
        with self._known_lock:
            self._known_tables.add(index_name)

    def create_doc_meta_idx(self, index_name: str):
        # RAGFlow calls this directly for the per-tenant metadata table (not in the ABC, but the
        # retriever/document service expects it — same as ob_conn_base.create_doc_meta_idx).
        self.create_idx(index_name, None, 0)
        return True

    def delete_idx(self, index_name: str, dataset_id: str):
        if dataset_id and not index_name.startswith("ragflow_doc_meta_"):
            # all KBs of a tenant share one table; a KB deletion must not drop the index
            return
        self._run(f"DROP INDEX IF EXISTS {_index_relation(index_name)}", fetch=False)
        self._run(f"DROP TABLE IF EXISTS {index_name}", fetch=False)
        with self._known_lock:
            self._known_tables.discard(index_name)

    def index_exist(self, index_name: str, dataset_id: str = None) -> bool:
        if index_name in self._known_tables:
            return True
        try:
            self._run(f"SELECT 1 FROM {index_name} LIMIT 0")
            with self._known_lock:
                self._known_tables.add(index_name)
            return True
        except Exception:
            return False

    """
    Filters
    """

    def _get_filters(self, condition: dict) -> list[str]:
        filters = []
        for k, v in condition.items():
            if not v:
                continue
            if k == "exists":
                if v in COLUMN_DDL:
                    filters.append(f"{v} IS NOT NULL")
            elif k == "must_not" and isinstance(v, dict) and "exists" in v:
                if v["exists"] in COLUMN_DDL:
                    filters.append(f"{v['exists']} IS NULL")
            elif k in ARRAY_COLUMNS:
                vals = v if isinstance(v, list) else [v]
                ors = " OR ".join(f"list_contains({k}, {_escape(x)})" for x in vals)
                filters.append(f"({ors})")
            elif k in COLUMN_DDL or vector_column_pattern.match(k):
                if isinstance(v, list):
                    filters.append(f"{k} IN ({', '.join(_escape(x) for x in v)})")
                else:
                    filters.append(f"{k} = {_escape(v)}")
        return filters

    """
    CRUD
    """

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
        **kwargs,
    ) -> SearchResult:
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        agg_fields = agg_fields or []

        output_fields = [f for f in (select_fields or []) if f != "_score"]
        if not output_fields or "*" in output_fields:
            output_fields = COLUMN_NAMES.copy()
        if "id" not in output_fields:
            output_fields = ["id"] + output_fields
        for f in highlight_fields or []:
            if f not in output_fields:
                output_fields.append(f)
        output_fields = [f for f in output_fields if f in COLUMN_DDL or vector_column_pattern.match(f)]
        if PAGERANK_FLD not in output_fields:
            output_fields.append(PAGERANK_FLD)
        fields_expr = ", ".join(output_fields)

        condition = dict(condition or {})
        condition["kb_id"] = dataset_ids
        filters = self._get_filters(condition)
        filters_expr = " AND ".join(filters) if filters else "TRUE"

        text_query = text_topn = None
        vec_col = vec_data = vec_topn = None
        vec_threshold = 0.0
        vector_weight = 0.5
        for m in match_expressions:
            if isinstance(m, MatchTextExpr):
                # matching_text is RAGFlow's TOKENIZED, ^-weighted query_string (e.g.
                # "(auto^0.5) (_^0.3) (ptr^0.4) "auto _"^0.9 ..."). The stored *_ltks columns are
                # tokenized the same way, so `@@` must see those tokens — the raw human question
                # in original_query would miss anything the tokenizer splits (auto_ptr -> auto _ ptr).
                text_query = _strip_es_query(m.matching_text or (m.extra_options or {}).get("original_query", ""))
                text_topn = m.topn
            elif isinstance(m, MatchDenseExpr):
                vec_col = m.vector_column_name
                vec_data = list(m.embedding_data)
                vec_topn = m.topn
                vec_threshold = float((m.extra_options or {}).get("similarity", 0.0))
            elif isinstance(m, FusionExpr):
                if m.method == "weighted_sum" and "weights" in (m.fusion_params or {}):
                    vector_weight = float(m.fusion_params["weights"].split(",")[1])

        result = SearchResult()
        pagerank_expr = f"COALESCE({PAGERANK_FLD}, 0) / 100.0"

        for index_name in index_names:
            if not self.index_exist(index_name):
                continue
            idx = _index_relation(index_name)
            t0 = time.time()

            if text_query and vec_data:
                rows = self._fusion_search(index_name, idx, fields_expr, output_fields, filters_expr, text_query, text_topn, vec_col, vec_data, vec_topn, vec_threshold, vector_weight, offset, limit)
                search_type = "fusion"
            elif text_query:
                match = f"{LEX_SCORED_COL} @@ {_escape(text_query)}"
                n = limit if limit > 0 else (text_topn or 10000)
                rows, _ = self._run(
                    f"SELECT {fields_expr}, BM25({idx}.tableoid) + {pagerank_expr} AS _score FROM {idx} WHERE {filters_expr} AND ({match}) ORDER BY _score DESC LIMIT {n} OFFSET {offset}"
                )
                search_type = "fulltext"
            elif vec_data:
                # Similarity threshold goes straight in the ANN scan's WHERE (relies on the
                # 26.07.4 fix #964 — on <26.07.4 a vector-op predicate here silently emptied the
                # result and had to be applied outside the scan).
                vec_n = _norm_column(len(vec_data))
                qv = "ARRAY[" + ",".join(str(float(x)) for x in _l2_normalize(vec_data)) + f"]::FLOAT[{len(vec_data)}]"
                n = limit if limit > 0 else (vec_topn or 10)
                rows, _ = self._run(
                    f"SELECT {fields_expr}, -({vec_n} <#> {qv}) + {pagerank_expr} AS _score "
                    f"FROM {idx} WHERE {filters_expr} AND -({vec_n} <#> {qv}) >= {vec_threshold} "
                    f"ORDER BY {vec_n} <#> {qv} LIMIT {n} OFFSET {offset}"
                )
                search_type = "vector"
            elif agg_fields:
                self._aggregation(result, index_name, agg_fields, filters_expr)
                logger.info(f"SereneDB search {index_name} type=aggregation took={time.time() - t0:.3f}s groups={result.total}")
                continue
            else:
                orders = []
                for f, o in order_by.fields if order_by else []:
                    if f in COLUMN_DDL:
                        orders.append(f"{f} {'ASC' if o == 0 else 'DESC'}")
                order_expr = ("ORDER BY " + ", ".join(orders)) if orders else ""
                limit_expr = f"LIMIT {limit} OFFSET {offset}" if limit else ""
                crows, _ = self._run(f"SELECT count(*) FROM {index_name} WHERE {filters_expr}")
                result.total += crows[0][0]
                rows, _ = self._run(f"SELECT {fields_expr} FROM {index_name} WHERE {filters_expr} {order_expr} {limit_expr}")
                for row in rows:
                    result.chunks.append(self._row_to_entity(row, output_fields))
                logger.info(f"SereneDB search {index_name} type=filter took={time.time() - t0:.3f}s rows={len(rows)}")
                continue

            for row in rows:
                result.chunks.append(self._row_to_entity(row, output_fields + ["_score"]))
            logger.info(f"SereneDB search {index_name} type={search_type} took={time.time() - t0:.3f}s rows={len(rows)} q={text_query!r}")

        if result.total == 0:
            result.total = len(result.chunks)
        return result

    def _fusion_search(self, table, idx, fields_expr, output_fields, filters_expr, text_query, text_topn, vec_col, vec_data, vec_topn, vec_threshold, vector_weight, offset, limit):
        """The P3 shape: one statement, window-normalized BM25 branch (a scalar-subquery
        normalizer would trip the iresearch_scan plan-copy bug), FULL OUTER JOIN, weighted sum.
        RAGFlow parity math: (1-vw) * bm25_norm + vw * cosine + pagerank/100."""
        vec_n = _norm_column(len(vec_data))
        qv = "ARRAY[" + ",".join(str(float(x)) for x in _l2_normalize(vec_data)) + f"]::FLOAT[{len(vec_data)}]"
        match = f"{LEX_SCORED_COL} @@ {_escape(text_query)}"
        lex_n = text_topn or 200
        v_n = vec_topn or 200
        n = limit if limit > 0 else (lex_n + v_n)
        prefixed = ", ".join(f"t.{f}" for f in output_fields)
        sql = f"""
WITH lex AS (
    SELECT id, BM25({idx}.tableoid) AS s
    FROM {idx} WHERE {filters_expr} AND ({match})
    ORDER BY s DESC LIMIT {lex_n}),
lexn AS (SELECT id, s / NULLIF(MAX(s) OVER (), 0) AS sn FROM lex),
vec AS (
    SELECT id, -({vec_n} <#> {qv}) AS sim
    FROM {idx} WHERE {filters_expr} AND -({vec_n} <#> {qv}) >= {vec_threshold}
    ORDER BY {vec_n} <#> {qv} LIMIT {v_n}),
fused AS (
    SELECT COALESCE(l.id, v.id) AS id,
           COALESCE(l.sn, 0) * {1.0 - vector_weight} + COALESCE(v.sim, 0) * {vector_weight} AS fs
    FROM lexn l FULL OUTER JOIN vec v ON l.id = v.id)
SELECT {prefixed}, f.fs + COALESCE(t.{PAGERANK_FLD}, 0) / 100.0 AS _score
FROM fused f JOIN {table} t ON t.id = f.id
ORDER BY _score DESC LIMIT {n} OFFSET {offset}"""
        rows, _ = self._run(sql)
        return rows

    def _aggregation(self, result: SearchResult, index_name: str, agg_fields, filters_expr):
        for agg_field in agg_fields:
            if agg_field not in COLUMN_DDL:
                # agg_field is interpolated into SQL; only aggregate real columns.
                continue
            if agg_field in ARRAY_COLUMNS:
                rows, _ = self._run(f"SELECT u.v, count(*) FROM (SELECT unnest({agg_field}) AS v FROM {index_name} WHERE {filters_expr} AND {agg_field} IS NOT NULL) u GROUP BY u.v")
            else:
                rows, _ = self._run(f"SELECT {agg_field}, count(*) FROM {index_name} WHERE {filters_expr} AND {agg_field} IS NOT NULL GROUP BY {agg_field}")
            for value, count in rows:
                result.chunks.append({"value": value, "count": int(count)})
                result.total += 1

    def get(self, chunk_id: str, index_name: str, dataset_ids: list[str]) -> dict | None:
        if not self.index_exist(index_name):
            return None
        rows, cols = self._run(f"SELECT * FROM {index_name} WHERE id = %s", (chunk_id,))
        if not rows:
            return None
        return self._row_to_entity(rows[0], cols)

    def insert(self, rows: list[dict], index_name: str, dataset_id: str = None) -> list[str]:
        if not rows:
            return []
        if index_name.startswith("ragflow_doc_meta_"):
            return self._insert_doc_meta(rows, index_name)
        if not self.index_exist(index_name):
            size = 0
            for k in rows[0]:
                m = vector_column_pattern.match(k)
                if m:
                    size = int(m.group("vector_size"))
            self.create_idx(index_name, dataset_id, size or 1024)

        # Batch by identical column tuple and multi-row upsert; per-row INSERTs would make
        # bulk ingest interminable.
        errors = []
        groups: dict[tuple, list[list]] = {}
        for doc in rows:
            d, extra = {}, {}
            vec_cols = {}
            for k, v in doc.items():
                m = vector_column_pattern.match(k)
                if m:
                    vec_cols[k] = v
                    continue
                if k not in COLUMN_DDL:
                    extra[k] = v
                    continue
                if k == "kb_id" and isinstance(v, list):
                    v = v[0]
                if k in JSON_COLUMNS and not isinstance(v, str):
                    v = json.dumps(v, ensure_ascii=False)
                if k == "content_with_weight" and isinstance(v, dict):
                    v = json.dumps(v, ensure_ascii=False)
                d[k] = v
            if extra:
                merged = d.get("extra")
                base = json.loads(merged) if isinstance(merged, str) and merged else {}
                base.update(extra)
                d["extra"] = json.dumps(base, ensure_ascii=False)
            for k, dv in DEFAULTS.items():
                d.setdefault(k, dv)

            cols, vals = list(d.keys()), list(d.values())
            for vc, vv in vec_cols.items():
                size = int(vector_column_pattern.match(vc).group("vector_size"))
                cols += [vc, _norm_column(size)]
                vals += [vv, _l2_normalize(vv)]
            groups.setdefault(tuple(cols), []).append(vals)

        conn = self._pool.getconn()
        try:
            conn.autocommit = True
            with conn.cursor() as cur:
                for cols, val_rows in groups.items():
                    updates = ", ".join(f"{c} = EXCLUDED.{c}" for c in cols if c != "id")
                    try:
                        psycopg2.extras.execute_values(cur, f"INSERT INTO {index_name} ({', '.join(cols)}) VALUES %s ON CONFLICT (id) DO UPDATE SET {updates}", val_rows, page_size=500)
                    except Exception as e:
                        logger.error(f"SereneDB insert error on {index_name}: {e}")
                        errors.append(str(e))
        finally:
            self._pool.putconn(conn)
        return errors

    def _insert_doc_meta(self, rows: list[dict], index_name: str) -> list[str]:
        if not self.index_exist(index_name):
            self.create_idx(index_name, None, 0)
        errors = []
        for doc in rows:
            meta = doc.get("meta_fields") or {}
            if not isinstance(meta, str):
                meta = json.dumps(meta, ensure_ascii=False)
            try:
                self._run(
                    f"INSERT INTO {index_name} (id, kb_id, meta_fields) VALUES (%s, %s, %s) ON CONFLICT (id) DO UPDATE SET kb_id = EXCLUDED.kb_id, meta_fields = EXCLUDED.meta_fields",
                    (doc.get("id"), doc.get("kb_id"), meta),
                    fetch=False,
                )
            except Exception as e:
                errors.append(str(e))
        return errors

    def update(self, condition: dict, new_value: dict, index_name: str, dataset_id: str) -> bool:
        if not self.index_exist(index_name):
            return True
        condition = dict(condition or {})
        if not index_name.startswith("ragflow_doc_meta_"):
            condition["kb_id"] = dataset_id
        filters = self._get_filters(condition)
        if not filters:
            return False
        sets = []
        for k, v in new_value.items():
            if k == "remove":
                items = {v: None} if isinstance(v, str) else v
                for kk, vv in items.items():
                    if kk not in COLUMN_DDL:
                        continue
                    if vv is None:
                        sets.append(f"{kk} = NULL")
                    elif kk in ARRAY_COLUMNS:
                        sets.append(f"{kk} = array_remove({kk}, {_escape(vv)})")
            elif k == "add":
                for kk, vv in v.items():
                    if kk in ARRAY_COLUMNS:
                        sets.append(f"{kk} = list_append({kk}, {_escape(vv)})")
            elif k in JSON_COLUMNS:
                sets.append(f"{k} = {_escape(json.dumps(v, ensure_ascii=False) if not isinstance(v, str) else v)}")
            elif k in COLUMN_DDL:
                sets.append(f"{k} = {_escape(v)}")
        if not sets:
            return True
        try:
            self._run(f"UPDATE {index_name} SET {', '.join(sets)} WHERE {' AND '.join(filters)}", fetch=False)
            return True
        except Exception as e:
            logger.error(f"SereneDB update error on {index_name}: {e}")
            return False

    def delete(self, condition: dict, index_name: str, dataset_id: str) -> int:
        if not self.index_exist(index_name):
            return 0
        condition = dict(condition or {})
        if not index_name.startswith("ragflow_doc_meta_"):
            condition["kb_id"] = dataset_id
        filters = self._get_filters(condition)
        if not filters:
            return 0
        where = " AND ".join(filters)
        rows, _ = self._run(f"SELECT count(*) FROM {index_name} WHERE {where}")
        n = rows[0][0]
        if n:
            self._run(f"DELETE FROM {index_name} WHERE {where}", fetch=False)
        return n

    """
    Result helpers
    """

    def _row_to_entity(self, row, cols) -> dict:
        entity = {}
        for c, v in zip(cols, row):
            if v is None:
                continue
            if c in JSON_COLUMNS and isinstance(v, str):
                try:
                    v = json.loads(v)
                except json.JSONDecodeError:
                    pass
            entity[c] = v
        return entity

    def get_scores(self, res: SearchResult) -> dict[str, float]:
        # chunk id -> fused/vector/bm25 score. NOT in the ABC, but RAGFlow's retriever calls it to
        # recover the first-stage score without re-reading vectors (see es_conn_base.get_scores).
        # search() stamps each chunk with "_score"; default 0.0 for filter-only results.
        return {c["id"]: float(c.get("_score", 0.0)) for c in res.chunks if "id" in c}

    def get_total(self, res: SearchResult) -> int:
        return res.total

    def get_doc_ids(self, res: SearchResult) -> list[str]:
        return [c["id"] for c in res.chunks]

    def get_fields(self, res: SearchResult, fields: list[str]) -> dict[str, dict]:
        out = {}
        for c in res.chunks:
            out[c["id"]] = {f: c[f] for f in fields if c.get(f) is not None}
        return out

    def get_highlight(self, res: SearchResult, keywords: list[str], field_name: str):
        # Same client-side strategy as ob_conn: emphasize keyword hits in the stored text.
        ans = {}
        if not res.chunks or not keywords:
            return ans
        pats = [re.compile(r"(^|\W)(%s)(\W|$)" % re.escape(k), re.IGNORECASE | re.MULTILINE) for k in keywords if k]
        for c in res.chunks:
            txt = c.get(field_name)
            if not txt:
                continue
            marked = txt
            for p in pats:
                marked = p.sub(r"\1<em>\2</em>\3", marked)
            if "<em>" in marked:
                ans[c["id"]] = re.sub(r"</em>\s*<em>", " ", marked)
        return ans

    def get_aggregation(self, res: SearchResult, field_name: str):
        out = []
        counts = {}
        for c in res.chunks:
            if "value" in c and "count" in c:
                out.append((c["value"], c["count"]))
            elif field_name in c:
                v = c[field_name]
                for vv in v if isinstance(v, list) else [v]:
                    if isinstance(vv, str) and vv.strip():
                        counts[vv] = counts.get(vv, 0) + 1
        out.extend(counts.items())
        return out

    """
    SQL passthrough (text-to-SQL feature)
    """

    def sql(self, sql: str, fetch_size: int = 1024, format: str = "json"):
        txt = sql.strip().rstrip(";")
        if fetch_size and re.match(r"^(select|with)\b", txt, re.IGNORECASE) and not re.search(r"\blimit\b", txt, re.IGNORECASE):
            txt = f"{txt} LIMIT {int(fetch_size)}"
        try:
            rows, cols = self._run(txt)
        except Exception:
            logger.exception("SereneDB sql passthrough failed")
            raise
        return {"columns": [{"name": c, "type": "text"} for c in cols], "rows": [list(r) for r in rows]}
