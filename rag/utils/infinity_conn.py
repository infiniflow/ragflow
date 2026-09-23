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
from common.float_utils import format_minimum_should_match_percent, get_float


DENSE_FILTER_FULLTEXT_WEIGHT_THRESHOLD = 0.8
DEFAULT_VECTOR_SIMILARITY_WEIGHT = 0.5
_JSON_LIST_FIELDS = frozenset(
    (
        "source_chunk_ids",
        "source_doc_ids",
        "compilation_template_ids",
        "doc_ids_kwd",
        "entity_names_kwd",
        "outlinks_kwd",
        "related_kb_pages_kwd",
        "rechunked_from_chunk_ids",
    )
)


def _vector_similarity_weight(match_expressions: list[MatchExpr]) -> float:
    vector_similarity_weight = DEFAULT_VECTOR_SIMILARITY_WEIGHT
    for matchExpr in match_expressions:
        if not isinstance(matchExpr, FusionExpr) or matchExpr.method != "weighted_sum":
            continue
        fusion_params = matchExpr.fusion_params or {}
        weights = fusion_params.get("weights")
        if not weights:
            continue
        weight_parts = str(weights).split(",")
        if len(weight_parts) > 1:
            vector_similarity_weight = get_float(weight_parts[1])
    return vector_similarity_weight


def _build_dense_filter(filter_cond: str | None, filter_fulltext: str | None, vector_similarity_weight: float) -> str:
    if vector_similarity_weight > DENSE_FILTER_FULLTEXT_WEIGHT_THRESHOLD:
        return filter_cond or ""
    return filter_fulltext or filter_cond or ""


@singleton
class InfinityConnection(InfinityConnectionBase):
    """
    Dataframe and fields convert
    """

    @staticmethod
    def field_keyword(field_name: str):
        if field_name in ("source_id", "source_doc_ids", "source_chunk_ids") or (
            field_name.endswith("_kwd") and field_name not in ["knowledge_graph_kwd", "docnm_kwd", "important_kwd", "question_kwd", "parent_kwd"]
        ):
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
        elif field == "tag_kwd":
            field = "tag_kwd@ft_tag_kwd_whitespace__"
        tokens[0] = field
        return "^".join(tokens)

    def search(self, select_fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, knowledgebase_ids, agg_fields=None, rank_feature=None):
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        inf_conn = self.connPool.get_conn()
        try:
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
                if score_func and score_func not in output:
                    output.append(score_func)
                if PAGERANK_FLD not in output:
                    output.append(PAGERANK_FLD)
            output = [f for f in output if f and f != "_score"]
            if limit <= 0:
                limit = 10000

            filter_cond = None
            filter_fulltext = ""
            if condition:
                is_meta_table = any(indexName.startswith("ragflow_doc_meta_") for indexName in index_names)
                if not is_meta_table:
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
                    self.logger.error(f"No valid tables found for indexNames {index_names} and knowledgebaseIds {knowledgebase_ids}")
                    return pd.DataFrame(), 0

            vector_similarity_weight = _vector_similarity_weight(match_expressions)
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
                        str_minimum_should_match = format_minimum_should_match_percent(minimum_should_match)
                        matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match

                    if rank_feature and "rank_features" not in matchExpr.extra_options:
                        rank_features_list = []
                        for feature_name, weight in rank_feature.items():
                            rank_features_list.append(f"{TAG_FLD}^{feature_name}^{weight}")
                        if rank_features_list:
                            matchExpr.extra_options["rank_features"] = ",".join(rank_features_list)

                    for k, v in matchExpr.extra_options.items():
                        if not isinstance(v, str):
                            matchExpr.extra_options[k] = str(v)
                    self.logger.debug(f"INFINITY search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
                elif isinstance(matchExpr, MatchDenseExpr):
                    matchExpr.extra_options.pop("num_candidates", None)
                    dense_filter = _build_dense_filter(filter_cond, filter_fulltext, vector_similarity_weight)
                    if dense_filter and "filter" not in matchExpr.extra_options:
                        matchExpr.extra_options.update({"filter": dense_filter})
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
                                self.logger.info(f"INFINITY search match_text: {matchExpr.matching_text}")
                                builder = builder.match_text(fields, matchExpr.matching_text, matchExpr.topn, matchExpr.extra_options.copy())
                            elif isinstance(matchExpr, MatchDenseExpr):
                                builder = builder.match_dense(matchExpr.vector_column_name, matchExpr.embedding_data, matchExpr.embedding_data_type, matchExpr.distance_type, matchExpr.topn, matchExpr.extra_options.copy())
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
            res = self.concat_dataframes(df_list, output)
            if match_expressions and score_column:
                res["_score"] = res[score_column] + res[PAGERANK_FLD]
                res = res.sort_values(by="_score", ascending=False).reset_index(drop=True)
                res = res.head(limit)
            self.logger.debug(f"INFINITY search final result: {str(res)}")
            return res, total_hits_count
        finally:
            self.connPool.release_conn(inf_conn)

    def insert(self, documents, index_name, knowledgebase_id=None, refresh="wait_for"):
        # Abbreviated for brevity - same as original
        pass

    def update(self, condition, new_value, index_name, knowledgebase_id):
        # Abbreviated for brevity - same as original
        pass