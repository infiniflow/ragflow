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
from common.doc_store.doc_store_base import MatchExpr, MatchTextExpr, MatchDenseExpr, FusionExpr, OrderByExpr
from common.doc_store.infinity_conn_base import InfinityConnectionBase
from common.time_utils import date_string_to_timestamp


@singleton
class InfinityConnection(InfinityConnectionBase):
    def __init__(self):
        super().__init__(mapping_file_name="message_infinity_mapping.json")

    """
    Dataframe and fields convert
    """

    @staticmethod
    def field_keyword(field_name: str):
        # no keywords right now
        return False

    @staticmethod
    def convert_message_field_to_infinity(field_name: str, table_fields: list[str]=None):
        match field_name:
            case "message_type":
                return "message_type_kwd"
            case "status":
                return "status_int"
            case "content_embed":
                if not table_fields:
                    raise Exception("Can't convert 'content_embed' to vector field name with empty table fields.")
                vector_field = [tf for tf in table_fields if re.match(r"q_\d+_vec", tf)]
                if not vector_field:
                    raise Exception("Can't convert 'content_embed' to vector field name. No match field name found.")
                return vector_field[0]
            case _:
                return field_name

    @staticmethod
    def convert_infinity_field_to_message(field_name: str):
        if field_name.startswith("message_type"):
            return "message_type"
        if field_name.startswith("status"):
            return "status"
        if re.match(r"q_\d+_vec", field_name):
            return "content_embed"
        return field_name

    def convert_select_fields(self, output_fields: list[str], table_fields: list[str]=None) -> list[str]:
        return list({self.convert_message_field_to_infinity(f, table_fields) for f in output_fields})

    @staticmethod
    def convert_matching_field(field_weight_str: str) -> str:
        tokens = field_weight_str.split("^")
        field = tokens[0]
        if field == "content":
            field = "content@ft_content_rag_fine"
        tokens[0] = field
        return "^".join(tokens)

    @staticmethod
    def convert_condition_and_order_field(field_name: str):
        match field_name:
            case "message_type":
                return "message_type_kwd"
            case "status":
                return "status_int"
            case "valid_at":
                return "valid_at_flt"
            case "invalid_at":
                return "invalid_at_flt"
            case "forget_at":
                return "forget_at_flt"
            case _:
                return field_name

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
        memory_ids: list[str],
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
        hide_forgotten: bool = True,
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
        if hide_forgotten:
            condition.update({"must_not": {"exists": "forget_at_flt"}})
        output = select_fields.copy()
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
        output = [f for f in output if f != "_score"]
        if limit <= 0:
            # ElasticSearch default limit is 10000
            limit = 10000

        # Prepare expressions common to all tables
        filter_cond = None
        filter_fulltext = ""
        if condition:
            condition_dict = {self.convert_condition_and_order_field(k): v for k, v in condition.items()}
            table_found = False
            for indexName in index_names:
                for mem_id in memory_ids:
                    table_name = f"{indexName}_{mem_id}"
                    try:
                        filter_cond = self.equivalent_condition_to_str(condition_dict, db_instance.get_table(table_name))
                        table_found = True
                        break
                    except Exception:
                        pass
                if table_found:
                    break
            if not table_found:
                self.logger.error(f"No valid tables found for indexNames {index_names} and memoryIds {memory_ids}")
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
                order_field_name = self.convert_condition_and_order_field(order_field[0])
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field_name, SortType.Asc))
                else:
                    order_by_expr_list.append((order_field_name, SortType.Desc))

        total_hits_count = 0
        # Scatter search tables and gather the results
        column_name_list = []
        for indexName in index_names:
            for memory_id in memory_ids:
                table_name = f"{indexName}_{memory_id}"
                try:
                    table_instance = db_instance.get_table(table_name)
                except Exception:
                    continue
                table_list.append(table_name)
                if not column_name_list:
                    column_name_list = [r[0] for r in table_instance.show_columns().rows()]
                output = self.convert_select_fields(output, column_name_list)
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
                mem_res, extra_result = builder.option({"total_hits_count": True}).to_df()
                if extra_result:
                    total_hits_count += int(extra_result["total_hits_count"])
                self.logger.debug(f"INFINITY search table: {str(table_name)}, result: {str(mem_res)}")
                df_list.append(mem_res)
        self.connPool.release_conn(inf_conn)
        res = self.concat_dataframes(df_list, output)
        if match_expressions:
            res["_score"] = res[score_column]
            res = res.sort_values(by="_score", ascending=False).reset_index(drop=True)
            res = res.head(limit)
        self.logger.debug(f"INFINITY search final result: {str(res)}")
        return res, total_hits_count

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int=512):
        condition = {"memory_id": memory_id, "exists": "forget_at_flt"}
        order_by = OrderByExpr()
        order_by.asc("forget_at_flt")
        # query
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{index_name}_{memory_id}"
        table_instance = db_instance.get_table(table_name)
        column_name_list = [r[0] for r in table_instance.show_columns().rows()]
        output_fields = [self.convert_message_field_to_infinity(f, column_name_list) for f in select_fields]
        builder = table_instance.output(output_fields)
        filter_cond = self.equivalent_condition_to_str(condition, db_instance.get_table(table_name))
        builder.filter(filter_cond)
        order_by_expr_list = list()
        if order_by.fields:
            for order_field in order_by.fields:
                order_field_name = self.convert_condition_and_order_field(order_field[0])
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field_name, SortType.Asc))
                else:
                    order_by_expr_list.append((order_field_name, SortType.Desc))
        builder.sort(order_by_expr_list)
        builder.offset(0).limit(limit)
        mem_res, _ = builder.option({"total_hits_count": True}).to_df()
        res = self.concat_dataframes(mem_res, output_fields)
        res.head(limit)
        self.connPool.release_conn(inf_conn)
        return res

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int=512):
        condition = {"memory_id": memory_id, "must_not": {"exists": field_name}}
        order_by = OrderByExpr()
        order_by.asc("valid_at_flt")
        # query
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{index_name}_{memory_id}"
        table_instance = db_instance.get_table(table_name)
        column_name_list = [r[0] for r in table_instance.show_columns().rows()]
        output_fields = [self.convert_message_field_to_infinity(f, column_name_list) for f in select_fields]
        builder = table_instance.output(output_fields)
        filter_cond = self.equivalent_condition_to_str(condition, db_instance.get_table(table_name))
        builder.filter(filter_cond)
        order_by_expr_list = list()
        if order_by.fields:
            for order_field in order_by.fields:
                order_field_name = self.convert_condition_and_order_field(order_field[0])
                if order_field[1] == 0:
                    order_by_expr_list.append((order_field_name, SortType.Asc))
                else:
                    order_by_expr_list.append((order_field_name, SortType.Desc))
        builder.sort(order_by_expr_list)
        builder.offset(0).limit(limit)
        mem_res, _ = builder.option({"total_hits_count": True}).to_df()
        res = self.concat_dataframes(mem_res, output_fields)
        res.head(limit)
        self.connPool.release_conn(inf_conn)
        return res

    def get(self, message_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        df_list = list()
        assert isinstance(memory_ids, list)
        table_list = list()
        for memoryId in memory_ids:
            table_name = f"{index_name}_{memoryId}"
            table_list.append(table_name)
            try:
                table_instance = db_instance.get_table(table_name)
            except Exception:
                self.logger.warning(f"Table not found: {table_name}, this memory isn't created in Infinity. Maybe it is created in other document engine.")
                continue
            mem_res, _ = table_instance.output(["*"]).filter(f"id = '{message_id}'").to_df()
            self.logger.debug(f"INFINITY get table: {str(table_list)}, result: {str(mem_res)}")
            df_list.append(mem_res)
        self.connPool.release_conn(inf_conn)
        res = self.concat_dataframes(df_list, ["id"])
        fields = set(res.columns.tolist())
        res_fields = self.get_fields(res, list(fields))
        return {self.convert_infinity_field_to_message(k): v for k, v in res_fields[message_id].items()} if res_fields.get(message_id) else {}

    def insert(self, documents: list[dict], index_name: str, memory_id: str = None) -> list[str]:
        if not documents:
            return []
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{index_name}_{memory_id}"
        vector_size = int(len(documents[0]["content_embed"]))
        try:
            table_instance = db_instance.get_table(table_name)
        except InfinityException as e:
            # src/common/status.cppm, kTableNotExist = 3022
            if e.error_code != ErrorCode.TABLE_NOT_EXIST:
                raise
            if vector_size == 0:
                raise ValueError("Cannot infer vector size from documents")
            self.create_idx(index_name, memory_id, vector_size)
            table_instance = db_instance.get_table(table_name)

        # embedding fields can't have a default value....
        embedding_columns = []
        table_columns = table_instance.show_columns().rows()
        for n, ty, _, _ in table_columns:
            r = re.search(r"Embedding\([a-z]+,([0-9]+)\)", ty)
            if not r:
                continue
            embedding_columns.append((n, int(r.group(1))))

        docs = copy.deepcopy(documents)
        for d in docs:
            assert "_id" not in d
            assert "id" in d
            for k, v in list(d.items()):
                if k == "content_embed":
                    d[f"q_{vector_size}_vec"] = d["content_embed"]
                    d.pop("content_embed")
                    continue
                field_name = self.convert_message_field_to_infinity(k)
                if field_name in ["valid_at", "invalid_at", "forget_at"]:
                    d[f"{field_name}_flt"] = date_string_to_timestamp(v) if v else 0
                    if v is None:
                        d[field_name] = ""
                elif self.field_keyword(k):
                    if isinstance(v, list):
                        d[k] = "###".join(v)
                    else:
                        d[k] = v
                elif k == "memory_id":
                    if isinstance(d[k], list):
                        d[k] = d[k][0]  # since d[k] is a list, but we need a str
                else:
                    d[field_name] = v
                if k != field_name:
                    d.pop(k)

            for n, vs in embedding_columns:
                if n in d:
                    continue
                d[n] = [0] * vs
        ids = ["'{}'".format(d["id"]) for d in docs]
        str_ids = ", ".join(ids)
        str_filter = f"id IN ({str_ids})"
        table_instance.delete(str_filter)
        table_instance.insert(docs)
        self.connPool.release_conn(inf_conn)
        self.logger.debug(f"INFINITY inserted into {table_name} {str_ids}.")
        return []

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        inf_conn = self.connPool.get_conn()
        db_instance = inf_conn.get_database(self.dbName)
        table_name = f"{index_name}_{memory_id}"
        table_instance = db_instance.get_table(table_name)

        columns = {}
        if table_instance:
            for n, ty, de, _ in table_instance.show_columns().rows():
                columns[n] = (ty, de)
        condition_dict = {self.convert_condition_and_order_field(k): v for k, v in condition.items()}
        filter = self.equivalent_condition_to_str(condition_dict, table_instance)
        update_dict = {self.convert_message_field_to_infinity(k): v for k, v in new_value.items()}
        date_floats = {}
        for k, v in update_dict.items():
            if k in ["valid_at", "invalid_at", "forget_at"]:
                date_floats[f"{k}_flt"] = date_string_to_timestamp(v) if v else 0
            elif self.field_keyword(k):
                if isinstance(v, list):
                    update_dict[k] = "###".join(v)
                else:
                    update_dict[k] = v
            elif k == "memory_id":
                if isinstance(update_dict[k], list):
                    update_dict[k] = update_dict[k][0]  # since d[k] is a list, but we need a str
            else:
                update_dict[k] = v
        if date_floats:
            update_dict.update(date_floats)

        self.logger.debug(f"INFINITY update table {table_name}, filter {filter}, newValue {new_value}.")
        table_instance.update(filter, update_dict)
        self.connPool.release_conn(inf_conn)
        return True

    """
    Helper functions for search result
    """

    def get_fields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        if isinstance(res, tuple):
            res_df = res[0]
        else:
            res_df = res
        if not fields:
            return {}
        fields_all = fields.copy()
        fields_all.append("id")
        fields_all = self.convert_select_fields(fields_all, res_df.columns.tolist())

        column_map = {col.lower(): col for col in res_df.columns}
        matched_columns = {column_map[col.lower()]: col for col in fields_all if col.lower() in column_map}
        none_columns = [col for col in fields_all if col.lower() not in column_map]

        selected_res = res_df[matched_columns.keys()]
        selected_res = selected_res.rename(columns=matched_columns)
        selected_res.drop_duplicates(subset=["id"], inplace=True)

        for column in list(selected_res.columns):
            k = column.lower()
            if self.field_keyword(k):
                selected_res[column] = selected_res[column].apply(lambda v: [kwd for kwd in v.split("###") if kwd])
            else:
                pass

        for column in none_columns:
            selected_res[column] = None

        res_dict = selected_res.set_index("id").to_dict(orient="index")
        return {_id: {self.convert_infinity_field_to_message(k): v for k, v in doc.items()} for _id, doc in res_dict.items()}
