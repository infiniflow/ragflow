import re
from typing import List, Dict
import infinity
from infinity.common import ConflictType
from infinity.index import IndexInfo, IndexType
from infinity.connection_pool import ConnectionPool
from rag import settings
from rag.settings import doc_store_logger
from rag.utils import singleton
import polars as pl
from . import rmSpace

from rag.utils.doc_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)


def equivalent_condition_to_str(condition: dict) -> str:
    cond = list()
    for k, v in condition.items():
        if not isinstance(k, str):
            continue
        if isinstance(v, list):
            inCond = list()
            for item in v:
                if isinstance(item, str):
                    inCond.append(f"'{v}'", v)
                else:
                    inCond.append({str(v)})
            if inCond:
                strInCond = ', '.join(inCond)
                strInCond = f'{k} IN ({strInCond})'
                cond.append(strInCond)
        elif isinstance(v, str):
            cond.append(f"{k}='{v}'")
        else:
            cond.append(f"{k}={str(v)}")
    return " AND ".join(cond)


@singleton
class InfinityConnection(DocStoreConnection):
    def __init__(self):
        self.dbName = settings.INFINITY.get("db_name", "default_db")
        infinity_uri = settings.INFINITY["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            infinity_uri = infinity.common.NetworkAddress(host, int(port))
        self.connPool = ConnectionPool(infinity_uri)
        doc_store_logger.info(f"Connected to infinity {infinity_uri}.")

    """
    Database operations
    """

    def health(self) -> dict:
        """
        Return the health status of the database.
        TODO: Infinity-sdk provides health() to wrap `show global variables` and `show tables`
        """
        return dict()

    """
    Table operations
    """

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        table_name = f'{indexName}_{knowledgebaseId}'
        inf_conn = self.connPool.get_conn()
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)
        vector_name = f'q_{vectorSize}_vec'
        inf_table = inf_db.create_table(
            table_name,
            {
                "chunk_id": {
                    "type": "varchar",
                    "default": "",
                },  # ES `_id` for each ES document
                "doc_id": {"type": "varchar", "default": ""},
                "kb_id": {"type": "varchar", "default": ""},
                "create_time": {"type": "varchar", "default": ""},
                "create_timestamp_flt": {"type": "float", "default": 0.0},
                "img_id": {"type": "varchar", "default": ""},
                "docnm_kwd": {"type": "varchar", "default": ""},
                "title_tks": {"type": "varchar", "default": ""},
                "title_sm_tks": {"type": "varchar", "default": ""},
                "name_kwd": {"type": "varchar", "default": ""},
                "important_kwd": {"type": "varchar", "default": ""},
                "important_tks": {"type": "varchar", "default": ""},
                "content_with_weight": {
                    "type": "varchar",
                    "default": "",
                },  # The raw chunk text
                "content_ltks": {"type": "varchar", "default": ""},
                "content_sm_ltks": {"type": "varchar", "default": ""},
                vector_name: {"type": f"vector,{vectorSize},float"},
                "page_num_int": {"type": "varchar", "default": ""},
                "top_int": {"type": "varchar", "default": ""},
                "position_int": {"type": "varchar", "default": ""},
                "weight_int": {"type": "integer", "default": 0},
                "weight_flt": {"type": "float", "default": 0.0},
                "rank_int": {"type": "integer", "default": 0},
                "available_int": {"type": "integer", "default": 0},
            },
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "q_vec_idx",
            IndexInfo(
                vector_name,
                IndexType.Hnsw,
                {
                    "M": "16",
                    "ef_construction": "50",
                    "metric": "cosine",
                    "encode": "lvq",
                },
            ),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx0",
            IndexInfo("title_tks", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx1",
            IndexInfo("title_sm_tks", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx2",
            IndexInfo("important_kwd", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx3",
            IndexInfo("important_tks", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx4",
            IndexInfo("content_ltks", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        inf_table.create_index(
            "text_idx5",
            IndexInfo("content_sm_ltks", IndexType.FullText, {"ANALYZER": "standard"}),
            ConflictType.Ignore,
        )
        self.connPool.release_conn(inf_conn)

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        table_name = f'{indexName}_{knowledgebaseId}'
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        db_instance.drop_table(table_name, ConflictType.Ignore)
        self.connPool.release_conn(inf_conn)

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f'{indexName}_{knowledgebaseId}'
        try:
            inf_conn = self.connPool.get_conn()
            db_instance = inf_conn.get_database(self.dbName)
            _ = db_instance.get_table(table_name)
            self.connPool.release_conn(inf_conn)
            return True
        except Exception as e:
            doc_store_logger.error("INFINITY indexExist: " + str(e))
        return False

    """
    CRUD operations
    """

    def search(
        self, selectFields: list[str], highlightFields: list[str], condition: dict, matchExprs: list[MatchExpr], orderBy: OrderByExpr, offset: int, limit: int, indexName: str, knowledgebaseIds: list[str]
    ) -> list[dict] | pl.DataFrame:
        """
        TODO: Infinity doesn't provide highlight
        """
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        for knowledgebaseId in knowledgebaseIds:
            table_name = f'{indexName}_{knowledgebaseId}'
            table_instance = db_instance.get_table(table_name)
            if 'chunk_id' not in selectFields:
                selectFields.append('chunk_id')
            builder = table_instance.output(selectFields)
            filter_cond = ''
            filter_fulltext = ''
            if condition:
                filter_cond = equivalent_condition_to_str(condition)
            for matchExpr in matchExprs:
                if isinstance(matchExpr, MatchTextExpr):
                    if len(filter_cond)!=0 and 'filter' not in matchExpr.extra_options:
                        matchExpr.extra_options.update({'filter': f'"{filter_cond}"'})
                    filter_fulltext = f'MatchText({matchExpr.fields}, {matchExpr.matching_text})'
                    if len(filter_cond)!=0:
                        filter_fulltext = f'({filter_cond}) AND {filter_fulltext}'
                    minimum_should_match = "0%"
                    if "minimum_should_match" in matchExpr.extra_options:
                        minimum_should_match = str(int(matchExpr.extra_options["minimum_should_match"] * 100)) + "%"
                        matchExpr.extra_options.update({'minimum_should_match': {minimum_should_match}})
                    builder = builder.match_text(
                        matchExpr.fields,
                        matchExpr.matching_text,
                        matchExpr.topn,
                        matchExpr.extra_options,
                    )
                elif isinstance(matchExpr, MatchDenseExpr):
                    if len(filter_cond)!=0 and 'filter' not in matchExpr.extra_options:
                        matchExpr.extra_options.update({'filter': f'"{filter_fulltext}"'})
                    builder = builder.match_dense(
                        matchExpr.vector_column_name,
                        matchExpr.embedding_data,
                        matchExpr.embedding_data_type,
                        matchExpr.distance_type,
                        matchExpr.topn,
                        matchExpr.extra_options,
                    )
                elif isinstance(matchExpr, FusionExpr):
                    builder = builder.fusion(
                        matchExpr.method, matchExpr.topn, matchExpr.fusion_params
                    )
            order_by_expr_list = list()
            for order_field in orderBy.fields:
                order_by_expr_list.append((order_field[0], order_field[1]==0))
            builder.sort(order_by_expr_list)
            builder.offset(offset).limit(limit)
            kb_res = builder.to_pl()
            df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = pl.concat(df_list)
        return res

    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | pl.DataFrame:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        for knowledgebaseId in knowledgebaseIds:
            table_name = f'{indexName}_{knowledgebaseId}'
            table_instance = db_instance.get_table(table_name)
            kb_res = table_instance.output(["*"]).filter(f"chunk_id = '{chunkId}'").to_pl()
            df_list.append(kb_res)
        self.connPool.release(inf_conn)
        res = pl.concat(df_list)
        return res

    def upsertBulk(self, documents: list[dict], indexName: str, knowledgebaseId: str):
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f'{indexName}_{knowledgebaseId}'
        table_instance = db_instance.get_table(table_name)
        for d in documents:
            if '_id' in d:
                d["chunk_id"] = d["_id"]
                del d["_id"]
        ids = [f"'{d["chunk_id"]}'" for d in documents]
        str_ids = ', '.join(ids)
        str_filter = f'chunk_id IN ({str_ids})'
        table_instance.delete(str_filter)
        table_instance.insert(documents)
        self.connPool.release_conn(inf_conn)

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str):
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f'{indexName}_{knowledgebaseId}'
        table_instance = db_instance.get_table(table_name)
        filter = equivalent_condition_to_str(condition)
        table_instance.update(filter, newValue)
        self.connPool.release_conn(inf_conn)

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str):
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f'{indexName}_{knowledgebaseId}'
        filter = equivalent_condition_to_str(condition)
        try:
            table_instance = db_instance.get_table(table_name)
        except Exception:
            doc_store_logger.warning(f"Skipped deleting `{filter}` from table {table_name} since the table doesn't exist.")
            return
        table_instance.delete(filter)
        self.connPool.release_conn(inf_conn)


    """
    Helper functions for search result
    """
    def getTotal(self, res):
        return len(res)

    def getChunkIds(self, res):
        return res["chunk_id"]

    def getFields(self, res, fields: List[str]) -> Dict[str, dict]:
        res_fields = {}
        if not fields:
            return {}
        num_rows = len(res)
        column_id = res["chunk_id"]
        for i in range(num_rows):
            chunk_id = column_id[i]
            m = {"id": chunk_id}
            for fieldnm in fields:
                if fieldnm not in res:
                    m[fieldnm] = None
                    continue
                v = res[fieldnm][i]
                if isinstance(v, list):
                    m[fieldnm] = "\t".join([str(vv) if not isinstance(
                        vv, list) else "\t".join([str(vvv) for vvv in vv]) for vv in v])
                    continue
                if not isinstance(v, str):
                    v = str(v)
                if fieldnm.find("tks") > 0:
                    v = rmSpace(v)
                m[fieldnm] = v
            res_fields[chunk_id] = m
        return res_fields

    def getHighlight(self, res, keywords: List[str], fieldnm: str):
        ans = {}
        num_rows = len(res)
        column_id = res["chunk_id"]
        for i in range(num_rows):
            chunk_id = column_id[i]
            txt = res[fieldnm][i]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE|re.MULTILINE)
            txts = []
            for t in re.split(r"[.?!;\n]", txt):
                for w in keywords:
                    t = re.sub(r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])"%re.escape(w), r"\1<em>\2</em>\3", t, flags=re.IGNORECASE|re.MULTILINE)
                if not re.search(r"<em>[^<>]+</em>", t, flags=re.IGNORECASE|re.MULTILINE):
                    continue
                txts.append(t)
            ans[chunk_id] = "...".join(txts)
        return ans

    def getAggregation(self, res, fieldnm: str):
        """
        TODO: Infinity doesn't provide aggregation
        """
        return list()

    """
    SQL
    """
    def sql(sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")
