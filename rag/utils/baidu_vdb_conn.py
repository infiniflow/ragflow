#
#  Copyright 2025 The Baidu Authors. All Rights Reserved.
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

import logging
import re
import json
import os

import copy

import pymochow
from pymochow.configuration import Configuration
from pymochow.auth.bce_credentials import BceCredentials
from pymochow.model.schema import (
    Schema, Field, IndexField,
    VectorIndex, 
    FilteringIndex, IndexStructureType,
    InvertedIndex, InvertedIndexAnalyzer, InvertedIndexParams, InvertedIndexFieldAttribute, InvertedIndexParseMode,
    HNSWParams, 
    AutoBuildRowCountIncrement
)
from pymochow.model.enum import (
    FieldType, IndexType, MetricType, ElementType,
    ServerErrCode
)
from pymochow.model.database import Database
from pymochow.model.table import (
    Table, Partition, Row,
    VectorTopkSearchRequest, BM25SearchRequest, HybridSearchRequest,
    VectorSearchConfig
)
from pymochow.exception import ClientError, ServerError

from rag import settings
from rag.utils import singleton
import pandas as pd
from api.utils.file_utils import get_project_base_directory
from rag.utils.doc_store_conn import DocStoreConnection, MatchExpr, OrderByExpr, MatchTextExpr, MatchDenseExpr, \
    FusionExpr

ATTEMPT_TIME = 2
logger = logging.getLogger('ragflow.baidu_vdb_conn')


filter_idx_filed_pattern = re.compile(r"^(.*_(kwd|id|ids|uid|uids)|uid)$")
invertes_idx_filedpattern = re.compile(r".*_(tks|ltks)$")

@singleton
class BaiduVDBConnection(DocStoreConnection):
    def __init__(self):
        self.db_name = "ragflow"
        self.tks_inverted_idx = "tks_inverted_idx"
        self.kwd_filter_idx = "kwd_filter_idx"
        logger.info(f"User BaiduVDB {settings.BAIDUVDB['endpoint']} as the doc engine")
        config = Configuration(credentials=BceCredentials(settings.BAIDUVDB['username'], settings.BAIDUVDB['password']),
            endpoint=settings.BAIDUVDB['endpoint'])
        self.client = pymochow.MochowClient(config)
        fp_mapping = os.path.join(
            get_project_base_directory(), "conf", "mochow_mapping.json"
        )
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        self.mapping = json.load(open(fp_mapping))
        healthy = self.health()
        self.query_fields_boosts = {
            "title_tks": 10,
            "title_sm_tks": 5,
            "important_tks": 20,
            "question_tks": 20,
            "content_ltks": 2,
            "content_sm_ltks": 1,
        }
        if healthy["err"] == "":
            logger.info(f"BaiduVDB {settings.BAIDUVDB['endpoint']} is healthy.")
        else:
            logger.warning(f"BaiduVDB {settings.BAIDUVDB['endpoint']} is not healthy. error: {healthy.get('err')}")
        

    """
    Database operations
    """

    def dbType(self) -> str:
        return "BaiduVDB"

    def health(self) -> dict:
        res = {
            "type": "BaiduVDB"
        }
        try:
            self.client.list_databases()
            res["status"] = "normal"
            res["err"] = ""
        except Exception as e:
            res["status"] = "invalid"
            res["err"] = str(e)
        return res

    """
    Table operations
    """

    def _get_table_schema(self) -> Schema:
        fields: list[Field] = []
        indexes: list[IndexField] = []

        for field, field_attribute in self.mapping.items():
            assert isinstance(field, str)
            assert isinstance(field_attribute, dict)
            if field == "id":
                fields.append(Field(field, FieldType.STRING, primary_key=True, partition_key=True, not_null=True))
            elif field_attribute["fieldType"] == FieldType.ARRAY.value:
                fields.append(Field(field_name=field, 
                                    field_type=FieldType(field_attribute["fieldType"]),
                                    element_type=ElementType(field_attribute["elementType"])))
            else:
                fields.append(Field(field_name=field, field_type=FieldType(field_attribute["fieldType"])))

        dimension_list: list[int] = [512, 768, 1024, 1536]

        for _, dimension in enumerate(dimension_list):
            fields.append(Field(
                field_name=f"q_{dimension}_vec", 
                field_type=FieldType.FLOAT_VECTOR, 
                dimension=dimension,
                not_null=False,
            ))

            
            vector_index = VectorIndex(
                index_name=f"q_{dimension}_vec_idx",
                index_type=IndexType.HNSW,
                field=f"q_{dimension}_vec", 
                metric_type=MetricType.COSINE,
                params=HNSWParams(m=16,efconstruction=50),
                auto_build=True,
                auto_build_index_policy=AutoBuildRowCountIncrement(row_count_increment=10000, row_count_increment_ratio=0.2),
            )
            
            indexes.append(vector_index)

        filter_index_fileds: list[str] = [field.field_name for field in fields if filter_idx_filed_pattern.match(field.field_name)]
        indexes.append(FilteringIndex(
            index_name=self.kwd_filter_idx,
            fields=[{"field": field_name, "indexStructureType": IndexStructureType.BITMAP} for field_name in filter_index_fileds]
        ))

        invertes_index_fields: list[str] = [field.field_name for field in fields if invertes_idx_filedpattern.match(field.field_name)]
        indexes.append(InvertedIndex(
            index_name=self.tks_inverted_idx,
            fields=invertes_index_fields,
            params=InvertedIndexParams(
                analyzer=InvertedIndexAnalyzer.DEFAULT_ANALYZER,
                parse_mode=InvertedIndexParseMode.COARSE_MODE,
            ),
            field_attributes=[InvertedIndexFieldAttribute.ANALYZED] * len(invertes_index_fields),
        ))

        schema = Schema(fields=fields, indexes=indexes)
        logger.debug(f"create table schema: {str(schema.to_dict())}")
        return schema

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        if self.indexExist(indexName=indexName, knowledgebaseId=knowledgebaseId):
            return
        db_list: list[Database] = self.client.list_databases()
        db_name_list: list[str] = [db.database_name for db in db_list]
        has_db = self.db_name in db_name_list
        if not has_db:
            self.client.create_database(self.db_name)

        table_name = indexName
        db = self.client.database(self.db_name)
        try:
            db.create_table(
                table_name=table_name,
                replication=settings.BAIDUVDB['replication'],
                partition=Partition(partition_num=3),
                schema=self._get_table_schema()
            )
        except Exception as e:
            logger.warning(f"BaiduVDB create index {indexName} failed, error: {str(e)}")
        logger.info(f"BaiduVDB create index {indexName} succeed")
        

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        if len(knowledgebaseId) > 0:
            # The index need to be alive after any kb deletion since all kb under this tenant are in one index.
            return
        try:
            db = self.client.database(self.db_name)
            table_name = indexName
            db.drop_table(table_name=table_name)
        except ServerError as e:
            if e.code == ServerErrCode.DB_NOT_EXIST:
                return
            else:
                logger.warning(f"BaiduVDB deleteIdx {str(e)}")
        except Exception as e:
            logger.warning(f"BaiduVDB deleteIdx {str(e)}")


    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        try:
            db = self.client.database(self.db_name)
            table_name = indexName
            table_list: list[Table] = db.list_table()
            table_name_list = [table.table_name for table in table_list]
            return table_name in table_name_list
        except ClientError:
            return False
        except Exception as e:
            logger.warning(f"BaiduVDB indexExist {str(e)}")
            return False
        return True

    """
    CRUD operations
    """

    def search(
        self, 
        selectFields: list[str], 
        highlightFields: list[str], 
        condition: dict, 
        matchExprs: list[MatchExpr], 
        orderBy: OrderByExpr, 
        offset: int, 
        limit: int, 
        indexNames: str | list[str], 
        knowledgebaseIds: list[str], 
        aggFields: list[str] = ..., 
        rank_feature: dict | None = None
    ) -> tuple[pd.DataFrame, int]:
        """
        TODO: vdb not support highlight, agg, rank
        """
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        projections = selectFields.copy()
        if len(projections) != 0:
            for essential_field in ["id"]:
                if essential_field not in projections:
                    projections.append(essential_field)
                
        assert "_id" not in condition
        condition["kb_id"] = knowledgebaseIds
        filter = self._condition_to_filter(condition=condition)
        db = self.client.database(self.db_name)

        res = list()
        search_req = None
        if len(matchExprs) == 0:
            # select
            select_res = self._select(db, indexNames=indexNames, projections=projections, filter=filter if filter != "" else None)
            res.extend(select_res)
        elif len(matchExprs) and isinstance(matchExprs[0], MatchTextExpr):
            # bm25_search
            search_req = self._matchTextExpr2Bm25SearchReq(matchExprs[0], filter if filter != "" else None)
        elif len(matchExprs) and isinstance(matchExprs[0], MatchDenseExpr):
            # vector_search
            search_req = self._matchDenseExpr2VectorSearchReq(matchExprs[0], filter if filter != "" else None)
        else:
            # hybird_search
            is_bybird_search = False
            vector_weight = 0.5
            for m in matchExprs:
                if isinstance(m, FusionExpr) and m.method == "weighted_sum" and "weights" in m.fusion_params:
                    assert len(matchExprs) == 3 \
                    and isinstance(matchExprs[0], MatchTextExpr) \
                    and isinstance(matchExprs[1], MatchDenseExpr) \
                    and isinstance(matchExprs[2], FusionExpr)
                    weights = m.fusion_params["weights"]
                    vector_weight = float(weights.split(",")[1])
                    is_bybird_search = True
            if is_bybird_search:
                bm25_search_req = self._matchTextExpr2Bm25SearchReq(matchExprs[0], None)
                vector_search_req = self._matchDenseExpr2VectorSearchReq(matchExprs[1], None)
                search_req = HybridSearchRequest(
                    vector_request=vector_search_req,
                    bm25_request=bm25_search_req,
                    vector_weight=vector_weight,
                    bm25_weight=1-vector_weight,
                    filter=filter if filter != "" else None,
                    limit=matchExprs[1].topn,
                )
        
        if search_req is not None:
            search_res = self._Search(db=db, indexNames=indexNames, req=search_req, projections=projections)
            res.extend(search_res)

        if len(orderBy.fields) > 0:
            sort_fields = list()
            for order_field in orderBy.fields:
                field, desc = order_field[0], order_field[1]
                if field in ["page_num_int","top_int"]:
                    sort_fields.append((field, min, bool(desc)))
                if field == "create_timestamp_flt":
                    def f(x):
                        return -x if desc else x
                    sort_fields.append((field, f, bool(desc)))
            def build_sort_key(entry):
                return [func(entry[field]) if callable(func) else entry[field] for field, func, _ in sort_fields]

            if len(sort_fields) > 0:
                res = sorted(res, key=build_sort_key)

        if limit > 0:
            res = res[offset:offset+limit]
        return res

    def _select(self, db: Database, indexNames: list[str], projections: list[str], filter: str):
        res_list = list()
        for table_name in indexNames:
            table = db.table(table_name=table_name)
            marker = None
            while True:
                res = table.select(marker=marker, filter=filter, projections=projections, limit=50)
                assert isinstance(res.rows, list)
                res_list.extend(res.rows)
                if res.is_truncated is False:
                    break
                else:
                    marker = res.next_marker
        return res_list

    def _matchTextExpr2Bm25SearchReq(self, m: MatchTextExpr, filter: str) -> BM25SearchRequest:
        search_text_cond = list()
        for field in m.fields:
            boost = 1
            if field in self.query_fields_boosts:
                boost = self.query_fields_boosts[field]
            if boost != 1:
                search_text_cond.append(f"{field}:{m.matching_text}^{boost}")
            else:
                search_text_cond.append(f"{field}:{m.matching_text}")
        search_text = " OR ".join(search_text_cond)
        return BM25SearchRequest(
            index_name=self.tks_inverted_idx,
            search_text=search_text,
            filter=filter,
            limit=m.topn,
        )

    def _matchDenseExpr2VectorSearchReq(self, m: MatchDenseExpr, filter: str) -> VectorTopkSearchRequest:
        config = VectorSearchConfig(ef=m.topn*2)
        return VectorTopkSearchRequest(
            vector_field=m.vector_column_name,
            limit=m.topn,
            vector=list(m.embedding_data),
            filter=filter,
            config=config
        )

    def _Search(self, db: Database, indexNames: list[str], req: BM25SearchRequest|VectorTopkSearchRequest|HybridSearchRequest, projections: list[str]):
        res_list = list()
        for table_name in indexNames:
            table = db.table(table_name=table_name)
            res = None
            if (isinstance(req, BM25SearchRequest)):
                res = table.bm25_search(request=req, projections=projections)
            elif (isinstance(req, VectorTopkSearchRequest)):
                res = table.vector_search(request=req, projections=projections)
            elif (isinstance(req, HybridSearchRequest)):
                res = table.hybrid_search(request=req, projections=projections)
            if res is None:
                continue
            assert isinstance(res.rows, list)
            for row in res.rows:
                res_list.append(row['row'])
        return  res_list
    
    def get(self, chunkId: str, indexName: str, knowledgebaseIds: list[str]) -> dict | None:
        db = self.client.database(self.db_name)
        table_name = indexName
        table_instance = db.table(table_name=table_name)
        kb_res = table_instance.query(primary_key={'id': chunkId})
        return kb_res.row

    def insert(self, documents: list[dict], indexName: str, knowledgebaseId: str = None) -> list[str]:
        # Refers https://cloud.baidu.com/doc/VDB/s/8lrsob128#%E6%9B%B4%E6%96%B0%E6%8F%92%E5%85%A5%E8%AE%B0%E5%BD%95
        docs = copy.deepcopy(documents)
        for d in docs:
            assert "id" in d
            for k, v in d.items():
                if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                    assert isinstance(v, list)
                elif re.search(r"_feas$", k):
                    d[k] = json.dumps(v)
                elif k == 'kb_id':
                    if isinstance(v, str):
                        d[k] = [v]
                elif k == "position_int":
                    assert isinstance(v, list)
                    d[k] = [num for row in v for num in row]
            if 'kb_id' not in d:
                d["kb_id"] = [knowledgebaseId]
            # set default value for scalar data
            for field, field_attribute in self.mapping.items():
                assert isinstance(field, str)
                assert isinstance(field_attribute, dict)
                if field not in d:
                    d[field] = field_attribute["default"]

        db = self.client.database(self.db_name)
        table_name = indexName
        table_instance = db.table(table_name=table_name)
        chunk_rows_list = [docs[i:i+300] for i in range(0, len(docs), 300)]

        res = []
        for chunk_rows in chunk_rows_list:
            for _ in range(ATTEMPT_TIME):
                try:
                    table_instance.upsert(rows=[Row(**c_row) for c_row in chunk_rows])
                except Exception as e:
                    res.append(str(e))
                    logger.warning("BaiduVDB.upsert got error: " + str(e))
                    continue
        return res

    def update(self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str) -> bool:
        db = self.client.database(self.db_name)
        table_name = indexName
        table = db.table(table_name=table_name)
        doc = copy.deepcopy(newValue)
        doc.pop("id", None)
        for k, v in doc.items():
            if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                assert isinstance(v, list)
            elif re.search(r"_feas$", k):
                doc[k] = json.dumps(v)
            elif k == "position_int":
                assert isinstance(v, list)
                doc[k] = [num for row in v for num in row]
            elif k == 'kb_id':
                if isinstance(v, str):
                    doc[k] = [v]
            elif k == "remove":
                del doc[k]
                # replace value with default value
                assert k in self.mapping
                k_attr = self.mapping[k]
                assert isinstance(k_attr, dict)
                doc[k] = k_attr["default"]
        if "id" in condition and isinstance(condition["id"], str):
            chunkId = condition["id"]
            try:
                table.update(
                    primary_key={"id": chunkId},
                    update_fields=doc,
                )
            except Exception as e:
                logging.warning(f"BaiduVDB update row from table by primary_key: id: {chunkId} failed, error: {str(e)}")
                return False
            return True
        condition["kb_id"] = [knowledgebaseId]
        filter = self._condition_to_filter(condition=condition)
        logger.debug(f"BaiduVDB update row from table {table_name} start")
        marker = None
        projection = ["id"]
        all_chunkIds = []
        # get ids by filter
        try:
            while True:
                res = table.select(marker=marker, projections=projection, filter=filter, limit=50)
                assert isinstance(res.rows, list)
                for row in res.rows:
                    all_chunkIds.append(row['id'])
                if res.is_truncated is False:
                    break
                else:
                    marker = res.next_marker
        except Exception as e:
            logger.warning(f"BaiduVDB: Fail to update row from table {table_name}, filter: {filter}, due to get ids by select failed: error: {str(e)}")
            return False
        # update row by primary_key
        try:
            for chunkId in all_chunkIds:
                table.update(
                    primary_key={"id": chunkId},
                    update_fields=doc,
                )
        except Exception as e:
            logging.warning(f"BaiduVDB update row from table by primiary_ley got by filter: {filter} failed, error: {str(e)}")
            return False
        logger.debug(f"BaiduVDB update row from table {table_name}, filter: {filter}")
        return True

    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        if not self.indexExist(indexName, knowledgebaseId):
            return 0
        condition["kb_id"] = [knowledgebaseId]
        filter = self._condition_to_filter(condition=condition)
        db = self.client.database(self.db_name)
        table_name = indexName
        table = db.table(table_name=table_name)
        try:
            table.delete(filter=filter)
        except Exception as e:
            logger.warning(f"BaiduVDB delete row from table {table_name} failed, filter: {filter}, error: {str(e)}")
            return 0
        logger.debug(f"BaiduVDB delete row from table {table_name}, filter: {filter}")
        return 0

    def _condition_exisist(self, field: str) ->str:
        assert field in self.mapping
        field_attribute = self.mapping[field]
        assert field_attribute["fieldType"] != FieldType.TEXT
        assert isinstance(field_attribute, dict)
        field_default = field_attribute["default"]
        if field_attribute["fieldType"] == FieldType.ARRAY.value:
            assert isinstance(field_default, list)
            field_default_first = field_default[0]
            if isinstance(field_default_first, str):
                return f"{field}[0] != '{field_default_first}'"
            else:
                return f"{field}[0] != {field_default}"
        else:
            if isinstance(field_default, str):
                return f"{field} != '{field_default}'"
            else:
                return f"{field} != {field_default}"

    def _condition_to_filter(self, condition: dict) -> str:
        if "id" in condition:
            id_value = condition["id"]
            if isinstance(id_value, str):
                return f"id == '{id_value}'"
            elif isinstance(id_value, list):
                inCond = [f'{item}' for item in id_value]
                strInCond = ", ".join(inCond)
                return f"id IN ({strInCond})"
        cond = list()
        for k, v in condition.items():
            if not isinstance(k, str) or not v:
                continue
            if k == "must_not":
                if isinstance(v, dict):
                    for kk, vv in v.items():
                        if kk == "exists":
                            cond.append("NOT (%s)" % self._condition_exisist(vv))
                continue
            elif k == "exists":
                cond.append(self._condition_exisist(v))
                continue

            assert k in self.mapping
            field_attribute = self.mapping[k]
            assert field_attribute["fieldType"] != FieldType.TEXT
            assert isinstance(field_attribute, dict)
            if isinstance(v, list):
                inCond = list()
                for item in v:
                    if isinstance(item, str):
                        inCond.append(f"'{item}'")
                    else:
                        inCond.append(str(item))
                if inCond:
                    strInCond = ", ".join(inCond)
                    if field_attribute["fieldType"] == FieldType.ARRAY.value:
                        cond.append(f"array_contains_any({k}, [{strInCond}])")
                    else:
                        cond.append( f"{k} IN ({strInCond})")
            elif isinstance(v, str):
                if field_attribute["fieldType"] == FieldType.ARRAY.value:
                    cond.append(f"array_contains({k}, '{v}')")
                else:
                    cond.append(f"{k} == '{v}'")
            else:
                if field_attribute["fieldType"] == FieldType.ARRAY.value:
                    cond.append(f"array_contains({k}, {str(v)})")
                else:
                    cond.append(f"{k} == {str(v)}")

        if len(cond) == 0:
            return ""
        return " AND ".join(cond)


    """
    Helper functions for search result
    """

    def getTotal(self, rows):
        return len(rows)

    def getChunkIds(self, rows):
        return [row["id"] for row in rows]

    def getFields(self, rows, fields: list[str]) -> dict[str, dict]:
        res_fields = {}
        for row in rows:
            m = {n: row.get(n) for n in fields if row.get(n) is not None}
            if "position_int" in m and isinstance(m["position_int"], list):
                m["position_int"] = [m["position_int"][i: i+5] for i in range(0, len(m["position_int"]), 5)]
            if m:
                res_fields[row["id"]] = m
        return res_fields

    def getHighlight(self, rows, keywords: list[str], fieldnm: str):
        ans = {}
        for row in rows:
            txt = row[fieldnm]
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
            ans[row["id"]] = "...".join(txts)
        return ans

    def getAggregation(self, res, fieldnm: str):
        return list()

    """
    SQL
    """
    def sql(sql: str, fetch_size: int, format: str):
        """
        Run the sql generated by text-to-sql
        """
        raise NotImplementedError("BaiduVDB not support sql")

    




