import infinity
from infinity.common import ConflictType
from infinity.index import IndexInfo, IndexType
from infinity.connection_pool import ConnectionPool
from rag import settings
from rag.settings import doc_store_logger
from rag.utils import singleton
import polars as pl

from rag.utils.data_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    MatchSparseExpr,
    MatchTensorExpr,
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
                    inCond.append(f'{k}="{v}"', k, v)
                else:
                    inCond.append(f'{k}={str(v)}', k, v)
            if inCond:
                strInCond = f'({" OR ".join(inCond)})'
                cond.append(strInCond)
        elif isinstance(v, str):
            cond.append(f'{k}="{v}"', k, v)
        else:
            cond.append(f"{k}={str(v)}", k, v)
    return " AND ".join(cond)


@singleton
class InfinityConnection(DocStoreConnection):
    def __init__(self):
        self.dbName = settings.INFINITY.get("db_name", "default_db")
        infinity_uri = settings.INFINITY["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            infinity_uri = infinity.common.NetworkAddress(host, int(port))
        self.inf = ConnectionPool(infinity_uri)
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

    def createIdx(self, vectorSize: int, indexName: str):
        inf_conn = self.inf.get_conn()
        inf_db = inf_conn.create_database(self.dbName, ConflictType.Ignore)
        inf_table = inf_db.create_table(
            indexName,
            {
                "_id": {
                    "type": "varchar",
                    "default": "",
                },  # ES has this meta field as the primary key for every index
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
                "q_vec": {"type": f"vector,{vectorSize},float"},
                "page_num_int": {"type": "varchar", "default": 0},
                "top_int": {"type": "varchar", "default": 0},
                "position_int": {"type": "varchar", "default": 0},
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
                "q_vec",
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
        self.inf.release_conn(inf_conn)

    def deleteIdx(self, indexName: str):
        inf_conn = self.inf.get_conn()
        db = inf_conn.get_database(self.dbName)
        db.drop_table(indexName, ConflictType.Ignore)
        self.inf.release_conn(inf_conn)

    def indexExist(self, indexName: str) -> bool:
        try:
            inf_conn = self.inf.get_conn()
            _ = inf_conn.get_table(self.dbName, indexName)
            self.inf.release_conn(inf_conn)
            return True
        except Exception as e:
            doc_store_logger.error("INFINITY indexExist: " + str(e))
        return False

    """
    CRUD operations
    """

    def search(
        self, selectFields: list[str], condition: dict, matchExprs: list[MatchExpr], orderBy: OrderByExpr, offset: int, limit: int, indexName: str
    ) -> list[dict] | pl.DataFrame:
        """
        TODO: convert result to dict?
        """
        inf_conn = self.inf.get_conn()
        table_instance = inf_conn.get_table(self.dbName, indexName)
        builder = table_instance.output(selectFields)
        if condition:
            builder = builder.filter(equivalent_condition_to_str(condition))
        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                builder = builder.match_text(
                    matchExpr.fields,
                    matchExpr.matching_text,
                    matchExpr.topn,
                    matchExpr.extra_options,
                )
            elif isinstance(matchExpr, MatchDenseExpr):
                builder = builder.match_dense(
                    matchExpr.vector_column_name,
                    matchExpr.embedding_data,
                    matchExpr.embedding_data_type,
                    matchExpr.distance_type,
                    matchExpr.topn,
                    matchExpr.knn_params,
                )
            elif isinstance(matchExpr, MatchSparseExpr):
                builder = builder.match_sparse(
                    matchExpr.vector_column_name,
                    matchExpr.sparse_data,
                    matchExpr.distance_type,
                    matchExpr.topn,
                    matchExpr.opt_params,
                )
            elif isinstance(matchExpr, MatchTensorExpr):
                builder = builder.match_tensor(
                    matchExpr.column_name,
                    matchExpr.query_data,
                    matchExpr.query_data_type,
                    matchExpr.topn,
                    matchExpr.extra_option,
                )
            elif isinstance(matchExpr, FusionExpr):
                builder = builder.fusion(
                    matchExpr.method, matchExpr.topn, matchExpr.fusion_params
                )
        builder.offset(offset).limit(limit)
        res = builder.to_pl()
        self.inf.release_conn(inf_conn)
        return res

    def get(self, docId: str, indexName: str) -> dict | pl.DataFrame:
        inf_conn = self.inf.get_conn()
        table_instance = inf_conn.get_table(self.dbName, indexName)
        res = table_instance.output(["*"]).filter(f"doc_id = '{docId}'").to_pl()
        self.inf.release(inf_conn)
        return res

    def upsertBulk(self, documents: list[dict], indexName: str):
        ids = [f"_id={d['_id']}" for d in documents]
        del_filter = " OR ".join(ids)
        inf_conn = self.inf.get_conn()
        table_instance = inf_conn.get_table(self.dbName, indexName)
        table_instance.delete(del_filter)
        table_instance.insert(documents)
        self.inf.release_conn(inf_conn)

    def update(self, condition: dict, newValue: dict, indexName: str):
        inf_conn = self.inf.get_conn()
        table_instance = inf_conn.get_table(self.dbName, indexName)
        filter = equivalent_condition_to_str(condition)
        table_instance.update(filter, newValue)
        self.inf.release_conn(inf_conn)

    def delete(self, condition: dict, indexName: str):
        inf_conn = self.inf.get_conn()
        table_instance = inf_conn.get_table(self.dbName, indexName)
        filter = equivalent_condition_to_str(condition)
        table_instance.delete(filter)
        self.inf.release_conn(inf_conn)
