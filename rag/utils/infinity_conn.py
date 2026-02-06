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

import re
import json
import copy
from infinity.common import InfinityException, SortType
from infinity.errors import ErrorCode
from common.decorator import singleton
import pandas as pd
from common.constants import PAGERANK_FLD, TAG_FLD
from common.doc_store.doc_store_base import MatchExpr, MatchTextExpr, MatchDenseExpr, FusionExpr, OrderByExpr
from common.doc_store.infinity_conn_base import InfinityConnectionBase


@singleton
class InfinityConnection(InfinityConnectionBase):
    """
    Dataframe and fields convert
    """

    @staticmethod
    def field_keyword(field_name: str):
        # Treat "*_kwd" tag-like columns as keyword lists except knowledge_graph_kwd; source_id is also keyword-like.
        if field_name == "source_id" or (
                field_name.endswith("_kwd") and field_name not in ["knowledge_graph_kwd", "docnm_kwd", "important_kwd",
                                                                   "question_kwd"]):
            return True
        return False

    def convert_select_fields(self, output_fields: list[str]) -> list[str]:
        need_empty_count = "important_kwd" in output_fields
        for i, field in enumerate(output_fields):
            if field in ["docnm_kwd", "title_tks", "title_sm_tks"]:
                output_fields[i] = "docnm"
            elif field in ["important_kwd", "important_tks"]:
                output_fields[i] = "important_keywords"
            elif field in ["question_kwd", "question_tks"]:
                output_fields[i] = "questions"
            elif field in ["content_with_weight", "content_ltks", "content_sm_ltks"]:
                output_fields[i] = "content"
            elif field in ["authors_tks", "authors_sm_tks"]:
                output_fields[i] = "authors"
        if need_empty_count and "important_kwd_empty_count" not in output_fields:
            output_fields.append("important_kwd_empty_count")
        return list(set(output_fields))

    @staticmethod
    def convert_matching_field(field_weight_str: str) -> str:
        tokens = field_weight_str.split("^")
        field = tokens[0]
        if field == "docnm_kwd" or field == "title_tks":
            field = "docnm@ft_docnm_rag_coarse"
        elif field == "title_sm_tks":
            field = "docnm@ft_docnm_rag_fine"
        elif field == "important_kwd":
            field = "important_keywords@ft_important_keywords_rag_coarse"
        elif field == "important_tks":
            field = "important_keywords@ft_important_keywords_rag_fine"
        elif field == "question_kwd":
            field = "questions@ft_questions_rag_coarse"
        elif field == "question_tks":
            field = "questions@ft_questions_rag_fine"
        elif field == "content_with_weight" or field == "content_ltks":
            field = "content@ft_content_rag_coarse"
        elif field == "content_sm_ltks":
            field = "content@ft_content_rag_fine"
        elif field == "authors_tks":
            field = "authors@ft_authors_rag_coarse"
        elif field == "authors_sm_tks":
            field = "authors@ft_authors_rag_fine"
        tokens[0] = field
        return "^".join(tokens)

    """
    CRUD operations
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
            knowledgebase_ids: list[str],
            agg_fields: list[str] | None = None,
            rank_feature: dict | None = None,
    ) -> tuple[pd.DataFrame, int]:
        """
        BUG: Infinity returns empty for a highlight field if the query string doesn't use that field.
        """
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        table_list = list()
        output = select_fields.copy()
        output = self.convert_select_fields(output)
        if agg_fields is None:
            agg_fields = []
        for essential_field in ["id"] + agg_fields:
            if essential_field not in output:
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
            if score_func not in output:
                output.append(score_func)
            if PAGERANK_FLD not in output:
                output.append(PAGERANK_FLD)
        output = [f for f in output if f != "_score"]
        if limit <= 0:
            # ElasticSearch default limit is 10000
            limit = 10000

        # Prepare expressions common to all tables
        filter_cond = None
        filter_fulltext = ""
        if condition:
            # Remove kb_id filter for Infinity (it uses table separation instead)
            condition = {k: v for k, v in condition.items() if k != "kb_id"}

            table_found = False
            for indexName in index_names:
                if indexName.startswith("ragflow_doc_meta_"):
                    table_names_to_search = [indexName]
                else:
                    table_names_to_search = [f"{indexName}_{kb_id}" for kb_id in knowledgebase_ids]
                for table_name in table_names_to_search:
                    try:
                        filter_cond = self.equivalent_condition_to_str(condition, db_instance.get_table(table_name))
                        table_found = True
                        break
                    except Exception:
                        pass
                if table_found:
                    break
            if not table_found:
                self.logger.error(
                    f"No valid tables found for indexNames {index_names} and knowledgebaseIds {knowledgebase_ids}")
                return pd.DataFrame(), 0

        for matchExpr in match_expressions:
            if isinstance(matchExpr, MatchTextExpr):
                if filter_cond and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_cond})
                matchExpr.fields = [self.convert_matching_field(field) for field in matchExpr.fields]
                fields = ",".join(matchExpr.fields)
                filter_fulltext = f"filter_fulltext('{fields}', '{matchExpr.matching_text}')"
                if filter_cond:
                    filter_fulltext = f"({filter_cond}) AND {filter_fulltext}"
                minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    str_minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match

                # Add rank_feature support
                if rank_feature and "rank_features" not in matchExpr.extra_options:
                    # Convert rank_feature dict to Infinity's rank_features string format
                    # Format: "field^feature_name^weight,field^feature_name^weight"
                    rank_features_list = []
                    for feature_name, weight in rank_feature.items():
                        # Use TAG_FLD as the field containing rank features
                        rank_features_list.append(f"{TAG_FLD}^{feature_name}^{weight}")
                    if rank_features_list:
                        matchExpr.extra_options["rank_features"] = ",".join(rank_features_list)

                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                self.logger.debug(f"INFINITY search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, MatchDenseExpr):
                if filter_fulltext and "filter" not in matchExpr.extra_options:
                    matchExpr.extra_options.update({"filter": filter_fulltext})
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                similarity = matchExpr.extra_options.get("similarity")
                if similarity:
                    matchExpr.extra_options["threshold"] = similarity
                    del matchExpr.extra_options["similarity"]
                self.logger.debug(f"INFINITY search MatchDenseExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, FusionExpr):
                if matchExpr.method == "weighted_sum":
                    # The default is "minmax" which gives a zero score for the last doc.
                    matchExpr.fusion_params["normalize"] = "atan"
                self.logger.debug(f"INFINITY search FusionExpr: {json.dumps(matchExpr.__dict__)}")

        order_by_expr_list = list()
        if order_by.fields:
            for order_field in order_by.fields:
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field[0], SortType.Asc))
                else:
                    order_by_expr_list.append((order_field[0], SortType.Desc))

        total_hits_count = 0
        # Scatter search tables and gather the results
        for indexName in index_names:
            if indexName.startswith("ragflow_doc_meta_"):
                table_names_to_search = [indexName]
            else:
                table_names_to_search = [f"{indexName}_{kb_id}" for kb_id in knowledgebase_ids]
            for table_name in table_names_to_search:
                try:
                    table_instance = db_instance.get_table(table_name)
                except Exception:
                    continue
                table_list.append(table_name)
                builder = table_instance.output(output)
                if len(match_expressions) > 0:
                    for matchExpr in match_expressions:
                        if isinstance(matchExpr, MatchTextExpr):
                            fields = ",".join(matchExpr.fields)
                            builder = builder.match_text(
                                fields,
                                matchExpr.matching_text,
                                matchExpr.topn,
                                matchExpr.extra_options.copy(),
                            )
                        elif isinstance(matchExpr, MatchDenseExpr):
                            builder = builder.match_dense(
                                matchExpr.vector_column_name,
                                matchExpr.embedding_data,
                                matchExpr.embedding_data_type,
                                matchExpr.distance_type,
                                matchExpr.topn,
                                matchExpr.extra_options.copy(),
                            )
                        elif isinstance(matchExpr, FusionExpr):
                            builder = builder.fusion(matchExpr.method, matchExpr.topn, matchExpr.fusion_params)
                else:
                    if filter_cond and len(filter_cond) > 0:
                        builder.filter(filter_cond)
                if order_by.fields:
                    builder.sort(order_by_expr_list)
                builder.offset(offset).limit(limit)
                kb_res, extra_result = builder.option({"total_hits_count": True}).to_df()
                if extra_result:
                    total_hits_count += int(extra_result["total_hits_count"])
                self.logger.debug(f"INFINITY search table: {str(table_name)}, result: {str(kb_res)}")
                df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = self.concat_dataframes(df_list, output)
        if match_expressions:
            res["_score"] = res[score_column] + res[PAGERANK_FLD]
            res = res.sort_values(by="_score", ascending=False).reset_index(drop=True)
            res = res.head(limit)
        self.logger.debug(f"INFINITY search final result: {str(res)}")
        return res, total_hits_count

    def get(self, chunk_id: str, index_name: str, knowledgebase_ids: list[str]) -> dict | None:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        assert isinstance(knowledgebase_ids, list)
        table_list = list()
        if index_name.startswith("ragflow_doc_meta_"):
            table_names_to_search = [index_name]
        else:
            table_names_to_search = [f"{index_name}_{kb_id}" for kb_id in knowledgebase_ids]
        for table_name in table_names_to_search:
            table_list.append(table_name)
            try:
                table_instance = db_instance.get_table(table_name)
            except Exception:
                self.logger.warning(
                    f"Table not found: {table_name}, this dataset isn't created in Infinity. Maybe it is created in other document engine.")
                continue
            kb_res, _ = table_instance.output(["*"]).filter(f"id = '{chunk_id}'").to_df()
            self.logger.debug(f"INFINITY get table: {str(table_list)}, result: {str(kb_res)}")
            df_list.append(kb_res)
        self.connPool.release_conn(inf_conn)
        res = self.concat_dataframes(df_list, ["id"])
        fields = set(res.columns.tolist())
        for field in ["docnm_kwd", "title_tks", "title_sm_tks", "important_kwd", "important_tks", "question_kwd",
                      "question_tks", "content_with_weight", "content_ltks", "content_sm_ltks", "authors_tks",
                      "authors_sm_tks"]:
            fields.add(field)
        res_fields = self.get_fields(res, list(fields))
        return res_fields.get(chunk_id, None)

    def insert(self, documents: list[dict], index_name: str, knowledgebase_id: str = None) -> list[str]:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        if index_name.startswith("ragflow_doc_meta_"):
            table_name = index_name
        else:
            table_name = f"{index_name}_{knowledgebase_id}"
        try:
            table_instance = db_instance.get_table(table_name)
        except InfinityException as e:
            # src/common/status.cppm, kTableNotExist = 3022
            if e.error_code != ErrorCode.TABLE_NOT_EXIST:
                raise
            vector_size = 0
            patt = re.compile(r"q_(?P<vector_size>\d+)_vec")
            for k in documents[0].keys():
                m = patt.match(k)
                if m:
                    vector_size = int(m.group("vector_size"))
                    break
            if vector_size == 0:
                raise ValueError("Cannot infer vector size from documents")

            # Determine parser_id from document structure
            # Table parser documents have 'chunk_data' field
            parser_id = None
            if "chunk_data" in documents[0] and isinstance(documents[0].get("chunk_data"), dict):
                from common.constants import ParserType
                parser_id = ParserType.TABLE.value
                self.logger.debug("Detected TABLE parser from document structure")

            # Fallback: Create table with base schema (shouldn't normally happen as init_kb() creates it)
            self.logger.debug(f"Fallback: Creating table {table_name} with base schema, parser_id: {parser_id}")
            self.create_idx(index_name, knowledgebase_id, vector_size, parser_id)
            table_instance = db_instance.get_table(table_name)

        # embedding fields can't have a default value....
        embedding_clmns = []
        clmns = table_instance.show_columns().rows()
        for n, ty, _, _ in clmns:
            r = re.search(r"Embedding\([a-z]+,([0-9]+)\)", ty)
            if not r:
                continue
            embedding_clmns.append((n, int(r.group(1))))

        docs = copy.deepcopy(documents)
        for d in docs:
            assert "_id" not in d
            assert "id" in d
            for k, v in list(d.items()):
                if k == "docnm_kwd":
                    d["docnm"] = v
                elif k == "title_kwd":
                    if not d.get("docnm_kwd"):
                        d["docnm"] = self.list2str(v)
                elif k == "title_sm_tks":
                    if not d.get("docnm_kwd"):
                        d["docnm"] = self.list2str(v)
                elif k == "important_kwd":
                    if isinstance(v, list):
                        empty_count = sum(1 for kw in v if kw == "")
                        tokens = [kw for kw in v if kw != ""]
                        d["important_keywords"] = self.list2str(tokens, ",")
                        d["important_kwd_empty_count"] = empty_count
                    else:
                        d["important_keywords"] = self.list2str(v, ",")
                elif k == "important_tks":
                    if not d.get("important_kwd"):
                        d["important_keywords"] = v
                elif k == "content_with_weight":
                    d["content"] = v
                elif k == "content_ltks":
                    if not d.get("content_with_weight"):
                        d["content"] = v
                elif k == "content_sm_ltks":
                    if not d.get("content_with_weight"):
                        d["content"] = v
                elif k == "authors_tks":
                    d["authors"] = v
                elif k == "authors_sm_tks":
                    if not d.get("authors_tks"):
                        d["authors"] = v
                elif k == "question_kwd":
                    d["questions"] = self.list2str(v, "\n")
                elif k == "question_tks":
                    if not d.get("question_kwd"):
                        d["questions"] = self.list2str(v)
                elif self.field_keyword(k):
                    if isinstance(v, list):
                        d[k] = "###".join(v)
                    else:
                        d[k] = v
                elif re.search(r"_feas$", k):
                    d[k] = json.dumps(v)
                elif k == "chunk_data":
                    # Convert data dict to JSON string for storage
                    if isinstance(v, dict):
                        d[k] = json.dumps(v)
                    else:
                        d[k] = v
                elif k == "kb_id":
                    if isinstance(d[k], list):
                        d[k] = d[k][0]  # since d[k] is a list, but we need a str
                elif k == "position_int":
                    assert isinstance(v, list)
                    arr = [num for row in v for num in row]
                    d[k] = "_".join(f"{num:08x}" for num in arr)
                elif k in ["page_num_int", "top_int"]:
                    assert isinstance(v, list)
                    d[k] = "_".join(f"{num:08x}" for num in v)
                elif k == "meta_fields":
                    if isinstance(v, dict):
                        d[k] = json.dumps(v, ensure_ascii=False)
                    else:
                        d[k] = v if v else "{}"
                else:
                    d[k] = v
            for k in ["docnm_kwd", "title_tks", "title_sm_tks", "important_kwd", "important_tks", "content_with_weight",
                      "content_ltks", "content_sm_ltks", "authors_tks", "authors_sm_tks", "question_kwd",
                      "question_tks"]:
                if k in d:
                    del d[k]

            for n, vs in embedding_clmns:
                if n in d:
                    continue
                d[n] = [0] * vs
        ids = ["'{}'".format(d["id"]) for d in docs]
        str_ids = ", ".join(ids)
        str_filter = f"id IN ({str_ids})"
        table_instance.delete(str_filter)
        # for doc in documents:
        #     logger.info(f"insert position_int: {doc['position_int']}")
        # logger.info(f"InfinityConnection.insert {json.dumps(documents)}")
        table_instance.insert(docs)
        self.connPool.release_conn(inf_conn)
        self.logger.debug(f"INFINITY inserted into {table_name} {str_ids}.")
        return []

    def update(self, condition: dict, new_value: dict, index_name: str, knowledgebase_id: str) -> bool:
        # if 'position_int' in newValue:
        #     logger.info(f"update position_int: {newValue['position_int']}")
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        if index_name.startswith("ragflow_doc_meta_"):
            table_name = index_name
        else:
            table_name = f"{index_name}_{knowledgebase_id}"
        table_instance = db_instance.get_table(table_name)
        # if "exists" in condition:
        #    del condition["exists"]

        clmns = {}
        if table_instance:
            for n, ty, de, _ in table_instance.show_columns().rows():
                clmns[n] = (ty, de)
        filter = self.equivalent_condition_to_str(condition, table_instance)
        removeValue = {}
        for k, v in list(new_value.items()):
            if k == "docnm_kwd":
                new_value["docnm"] = self.list2str(v)
            elif k == "title_kwd":
                if not new_value.get("docnm_kwd"):
                    new_value["docnm"] = self.list2str(v)
            elif k == "title_sm_tks":
                if not new_value.get("docnm_kwd"):
                    new_value["docnm"] = v
            elif k == "important_kwd":
                if isinstance(v, list):
                    empty_count = sum(1 for kw in v if kw == "")
                    tokens = [kw for kw in v if kw != ""]
                    new_value["important_keywords"] = self.list2str(tokens, ",")
                    new_value["important_kwd_empty_count"] = empty_count
                else:
                    new_value["important_keywords"] = self.list2str(v, ",")
            elif k == "important_tks":
                if not new_value.get("important_kwd"):
                    new_value["important_keywords"] = v
            elif k == "content_with_weight":
                new_value["content"] = v
            elif k == "content_ltks":
                if not new_value.get("content_with_weight"):
                    new_value["content"] = v
            elif k == "content_sm_ltks":
                if not new_value.get("content_with_weight"):
                    new_value["content"] = v
            elif k == "authors_tks":
                new_value["authors"] = v
            elif k == "authors_sm_tks":
                if not new_value.get("authors_tks"):
                    new_value["authors"] = v
            elif k == "question_kwd":
                new_value["questions"] = "\n".join(v)
            elif k == "question_tks":
                if not new_value.get("question_kwd"):
                    new_value["questions"] = self.list2str(v)
            elif self.field_keyword(k):
                if isinstance(v, list):
                    new_value[k] = "###".join(v)
                else:
                    new_value[k] = v
            elif re.search(r"_feas$", k):
                new_value[k] = json.dumps(v)
            elif k == "kb_id":
                if isinstance(new_value[k], list):
                    new_value[k] = new_value[k][0]  # since d[k] is a list, but we need a str
            elif k == "position_int":
                assert isinstance(v, list)
                arr = [num for row in v for num in row]
                new_value[k] = "_".join(f"{num:08x}" for num in arr)
            elif k in ["page_num_int", "top_int"]:
                assert isinstance(v, list)
                new_value[k] = "_".join(f"{num:08x}" for num in v)
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
        for k in ["docnm_kwd", "title_tks", "title_sm_tks", "important_kwd", "important_tks", "content_with_weight",
                  "content_ltks", "content_sm_ltks", "authors_tks", "authors_sm_tks", "question_kwd", "question_tks"]:
            if k in new_value:
                del new_value[k]

        remove_opt = {}  # "[k,new_value]": [id_to_update, ...]
        if removeValue:
            col_to_remove = list(removeValue.keys())
            row_to_opt = table_instance.output(col_to_remove + ["id"]).filter(filter).to_df()
            self.logger.debug(f"INFINITY search table {str(table_name)}, filter {filter}, result: {str(row_to_opt[0])}")
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

        self.logger.debug(f"INFINITY update table {table_name}, filter {filter}, newValue {new_value}.")
        for update_kv, ids in remove_opt.items():
            k, v = json.loads(update_kv)
            table_instance.update(filter + " AND id in ({0})".format(",".join([f"'{id}'" for id in ids])),
                                  {k: "###".join(v)})

        table_instance.update(filter, new_value)
        self.connPool.release_conn(inf_conn)
        return True

    """
    Helper functions for search result
    """

    def get_fields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        if isinstance(res, tuple):
            res = res[0]
        if not fields:
            return {}
        fields_all = fields.copy()
        fields_all.append("id")
        fields_all = set(fields_all)
        if "docnm" in res.columns:
            for field in ["docnm_kwd", "title_tks", "title_sm_tks"]:
                if field in fields_all:
                    res[field] = res["docnm"]
        if "important_keywords" in res.columns:
            if "important_kwd" in fields_all:
                if "important_kwd_empty_count" in res.columns:
                    base = res["important_keywords"].apply(lambda raw: raw.split(",") if raw else [])
                    counts = res["important_kwd_empty_count"].fillna(0).astype(int)
                    res["important_kwd"] = [
                        tokens + [""] * empty_count
                        for tokens, empty_count in zip(base.tolist(), counts.tolist())
                    ]
                else:
                    res["important_kwd"] = res["important_keywords"].apply(lambda v: v.split(",") if v else [])
            if "important_tks" in fields_all:
                res["important_tks"] = res["important_keywords"]
        if "questions" in res.columns:
            if "question_kwd" in fields_all:
                res["question_kwd"] = res["questions"].apply(lambda v: v.splitlines())
            if "question_tks" in fields_all:
                res["question_tks"] = res["questions"]
        if "content" in res.columns:
            for field in ["content_with_weight", "content_ltks", "content_sm_ltks"]:
                if field in fields_all:
                    res[field] = res["content"]
        if "authors" in res.columns:
            for field in ["authors_tks", "authors_sm_tks"]:
                if field in fields_all:
                    res[field] = res["authors"]

        column_map = {col.lower(): col for col in res.columns}
        matched_columns = {column_map[col.lower()]: col for col in fields_all if col.lower() in column_map}
        none_columns = [col for col in fields_all if col.lower() not in column_map]

        res2 = res[matched_columns.keys()]
        res2 = res2.rename(columns=matched_columns)
        res2.drop_duplicates(subset=["id"], inplace=True)

        for column in list(res2.columns):
            k = column.lower()
            if self.field_keyword(k):
                res2[column] = res2[column].apply(lambda v: [kwd for kwd in v.split("###") if kwd])
            elif re.search(r"_feas$", k):
                res2[column] = res2[column].apply(lambda v: json.loads(v) if v else {})
            elif k == "chunk_data":
                # Parse JSON data back to dict for table parser fields
                res2[column] = res2[column].apply(lambda v: json.loads(v) if v and isinstance(v, str) else v)
            elif k == "position_int":
                def to_position_int(v):
                    if v:
                        arr = [int(hex_val, 16) for hex_val in v.split("_")]
                        v = [arr[i: i + 5] for i in range(0, len(arr), 5)]
                    else:
                        v = []
                    return v

                res2[column] = res2[column].apply(to_position_int)
            elif k in ["page_num_int", "top_int"]:
                res2[column] = res2[column].apply(lambda v: [int(hex_val, 16) for hex_val in v.split("_")] if v else [])
            else:
                pass
        for column in ["docnm", "important_keywords", "questions", "content", "authors"]:
            if column in res2:
                del res2[column]
        for column in none_columns:
            res2[column] = None

        return res2.set_index("id").to_dict(orient="index")
