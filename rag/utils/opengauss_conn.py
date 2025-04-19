import logging
import os
import re
import json
import time
import copy
import psycopg2
from psycopg2 import sql
import pandas as pd
import numpy as np
import ast
from rag import settings
from rag.settings import PAGERANK_FLD
from rag.utils import singleton
from api.utils.file_utils import get_project_base_directory
import traceback

from rag.utils.doc_store_conn import (
    DocStoreConnection,
    MatchExpr,
    MatchTextExpr,
    MatchDenseExpr,
    FusionExpr,
    OrderByExpr,
)

logger = logging.getLogger('ragflow.opengauss_conn')

ATTEMPT_TIME = 2

def equivalent_condition_to_str(condition: dict, table_columns: dict = None) -> str:
    assert "_id" not in condition

    def exists(column):
        assert column in table_columns, f"'{column}' should be in '{table_columns}'."
        column_type, default_value = table_columns[column]
        if "char" in column_type.lower():  
            if not default_value:
                default_value = ""
            return f"{column} != '{default_value}'"
        return f"{column} != {default_value}"

    conditions = []
    for key, value in condition.items():
        if not isinstance(key, str) or key == "kb_id" or not value:
            continue

        if isinstance(value, list):
            in_conditions = [f"'{item}'" if isinstance(item, str) else str(item) for item in value]
            if in_conditions:
                str_in_conditions = ", ".join(in_conditions)
                conditions.append(f"{key} IN ({str_in_conditions})")
        elif key == "must_not" and isinstance(value, dict):
            for sub_key, sub_value in value.items():
                if sub_key == "exists":
                    conditions.append(f"NOT ({exists(sub_value)})")
        elif isinstance(value, str):
            conditions.append(f"{key} = '{value}'")
        elif key == "exists":
            conditions.append(exists(value))
        else:
            conditions.append(f"{key} = {value}")

    return " AND ".join(conditions) if conditions else "1=1"


def concat_dataframes(df_list: list[pd.DataFrame], selectFields: list[str]) -> pd.DataFrame:
    df_list2 = [df for df in df_list if not df.empty]
    if df_list2:
        return pd.concat(df_list2, axis=0).reset_index(drop=True)

    schema = []
    for field_name in selectFields:
        if field_name == 'score()':  # Workaround: fix schema is changed to score()
            schema.append('SCORE')
        elif field_name == 'similarity()':  # Workaround: fix schema is changed to similarity()
            schema.append('SIMILARITY')
        else:
            schema.append(field_name)
    return pd.DataFrame(columns=schema)

def get_tsquery(query: str)-> str:
    clean_text = re.sub(r'\^[\d.]+|~[\d.]', '', query) 
    pattern = re.compile(r'[A-Za-z]+|[\u4e00-\u9fff]+|\d+')
    tokens = pattern.findall(clean_text)
    keywords = [token for token in tokens if token.upper() != 'OR']
    seen = set()
    unique_keywords = []
    for token in keywords:
        lower = token.lower()
        if lower not in seen:
            seen.add(lower)
            unique_keywords.append(token)
    result = " | ".join(unique_keywords)
    return result

@singleton
class OpenGaussConnection(DocStoreConnection):
    def __init__(self):
        self.info = {}
        logger.info(f"Use openGauss {settings.OG['host']} as the doc engine.")
        for _ in range(ATTEMPT_TIME):
            try:
                self.conn = psycopg2.connect(
                    host=settings.OG["host"],
                    port=settings.OG["port"],
                    user=settings.OG["user"],
                    password=settings.OG["password"],
                    dbname=settings.OG["database"]
                )
                if self.conn:
                    self.info = self.conn.info
                    break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting openGauss to be healthy.")
                time.sleep(5)
        if not self.conn:
            msg = f"openGauss is unhealthy in 120s."
            logger.error(msg)
            raise Exception(msg)
        logger.info(f"openGauss is healthy.")

    """
    Database operations
    """

    def dbType(self) -> str:
        return "opengauss"

    def health(self) -> dict:
        try:
            with self.conn.cursor() as cursor:
                cursor.execute("SELECT 1")
                result = cursor.fetchone()
                return {"type": "opengauss", "status": "healthy" if result else "unhealthy"}
        except Exception as e:
            logger.error(f"openGauss health check failed: {str(e)}")
            return {"type": "opengauss", "status": "unhealthy"}
    """
    Table operations
    """

    def createIdx(self, indexName: str, knowledgebaseId: str, vectorSize: int):
        table_name = f"{indexName}_{knowledgebaseId}"
        vector_name = f"q_{vectorSize}_vec"

        columns = []
        indices = []

        fp_mapping = os.path.join(
            get_project_base_directory(), "conf", "infinity_mapping.json"
        )
        if not os.path.exists(fp_mapping):
            raise Exception(f"Mapping file not found at {fp_mapping}")
        schema = json.load(open(fp_mapping))
        schema[vector_name] = {"type": f"vector({vectorSize})"}

        for field_name, field_info in schema.items():
            default_value = field_info.get("default")
            if default_value is not None:
                if field_info.get("type").lower() == "varchar":
                    default_clause = f"DEFAULT '{default_value}'"
                else:
                    default_clause = f"DEFAULT {default_value}"
            else:
                default_clause = ""

            column_definition = f"{field_name} {field_info['type']} {default_clause}"
            columns.append(column_definition)

            if field_info.get("analyzer"):
                indices.append({
                    "field": field_name,
                    "analyzer": field_info["analyzer"]
                })
        create_table_sql = sql.SQL("""
                    CREATE TABLE IF NOT EXISTS {} (
                        {}
                    );
                """).format(
            sql.Identifier(table_name),
            sql.SQL(", ").join(sql.SQL(column) for column in columns))
        with self.conn.cursor() as cursor:
            # create table
            cursor.execute(create_table_sql)
            # create vector index
            cursor.execute(sql.SQL("""
                            CREATE INDEX IF NOT EXISTS {} ON {} USING hnsw({} vector_cosine_ops) WITH (m = 16, ef_construction = 64);
                        """).format(
                sql.Identifier(f"q_vec_idx_{knowledgebaseId}"),
                sql.Identifier(table_name),
                sql.Identifier(vector_name)
            ))

            # create fulltext index
            weight_map = {'title_tks': 'C', 'title_sm_tks': 'D', 'important_kwd': 'A', 'important_tks': 'B', 'question_tks': 'B', 'content_ltks' : 'D', 'content_sm_ltks' : 'D'}
            sql_parts = []
            for field, weight in weight_map.items():
                if field == "content_sm_ltks":
                    sql_parts.append(f"to_tsvector('chparser_conf', {field})")
                else:
                    sql_parts.append(f"setweight(to_tsvector('chparser_conf', COALESCE({field}, ' ')), '{weight}')")
            field_sql = " ||\n".join(sql_parts)
            cursor.execute(sql.SQL("""
                                    CREATE INDEX IF NOT EXISTS {} ON {} USING gin(({}));    
                                """).format(
                sql.Identifier(f"text_idx_fulltext_{knowledgebaseId}"),
                sql.Identifier(table_name),
                sql.SQL(field_sql)
            ))
            self.conn.commit()
        logger.info(
            f"openGauss created table {table_name}, vector size {vectorSize}"
        )

    def deleteIdx(self, indexName: str, knowledgebaseId: str):
        table_name = f"{indexName}_{knowledgebaseId}"
        with self.conn.cursor() as cursor:
            cursor.execute(sql.SQL("""
                DROP TABLE IF EXISTS {}
            """).format(
                sql.Identifier(table_name)
            ))
            self.conn.commit()
        logger.info(f"openGauss dropped table {table_name}")

    def indexExist(self, indexName: str, knowledgebaseId: str) -> bool:
        table_name = f"{indexName}_{knowledgebaseId[:-10]}%"
        with self.conn.cursor() as cursor:
            try:
                cursor.execute(sql.SQL("""
                    SELECT EXISTS (
                        SELECT 1 FROM information_schema.tables 
                        WHERE table_name like %s
                    )
                """), (table_name,))
                exists = cursor.fetchone()[0]
                return exists
            except Exception as e:
                print(f"openGauss indexExist error: {str(e)}")
                return False

    """
    CRUD operations
    """    
    def search(
            self, selectFields: list[str],
            highlightFields: list[str],
            condition: dict,
            matchExprs: list[MatchExpr],
            orderBy: OrderByExpr,
            offset: int,
            limit: int,
            indexNames: str | list[str],
            knowledgebaseIds: list[str],
            aggFields: list[str] = [],
            rank_feature: dict | None = None) -> tuple[pd.DataFrame, int]:
        if isinstance(indexNames, str):
            indexNames = indexNames.split(",")
        assert isinstance(indexNames, list) and len(indexNames) > 0
        logger.info(f"[offset, limit]: {offset} {limit}")

        df_list = list()
        table_list = list()
        output = selectFields.copy()
        for essential_field in ["id"]:
            if essential_field not in output:
                output.append(essential_field)

        score_func = ""
        score_column = ""
        text_score_expr = ""
        vector_score_expr = ""
        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                weight_fields = []
                for field in matchExpr.fields:
                    if '^' in field:
                        col, weight = field.split('^')
                        weight_map = {'important_kwd': 'A', 'important_tks': 'B', 'question_tks': 'B', 'title_tks': 'C', 'title_sm_tks': 'D', 'content_ltks' : 'D', 'content_sm_ltks' : 'D'} 
                        weight_fields.append(
                            f"setweight(to_tsvector('chparser_conf', COALESCE({col}, ' ')), '{weight_map[col]}')"  
                        )
                    else:
                        weight_fields.append(f"to_tsvector('chparser_conf', {field})")
                score_func = f"ts_rank({' || '.join(weight_fields)},  %s, 1)"   
                text_score_expr = score_func
                score_column = "SCORE"
            if isinstance(matchExpr, MatchDenseExpr):
                vector_score_expr = f"({matchExpr.vector_column_name} <=> %s::vector) " # cosine 
                if not score_func:
                    score_func = vector_score_expr
                    score_column = "SIMILARITY"
                    

        if matchExprs:
            if PAGERANK_FLD not in output:
                output.append(PAGERANK_FLD)
            output = [f for f in output if f != "_score"]
        logger.info(f"output: {output}")

        # prepare filter conditions
        filter_cond = None
        filter_fulltext = ""
        if condition:
                table_ref = f"{indexNames[0]}_{knowledgebaseIds[0]}"
                table_columns = self.get_columns(table_ref)
                filter_cond = equivalent_condition_to_str(condition, table_columns)

        for matchExpr in matchExprs:
            if isinstance(matchExpr, MatchTextExpr):
                fields = ",".join(matchExpr.fields)
                filter_fulltext = f"({' || '.join(weight_fields)}) @@  %s"
                if filter_cond:
                    filter_fulltext = f"({filter_cond}) AND {filter_fulltext}"
                minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                if isinstance(minimum_should_match, float):
                    str_minimum_should_match = str(int(minimum_should_match * 100)) + "%"
                    matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match
                for k, v in matchExpr.extra_options.items():
                    if not isinstance(v, str):
                        matchExpr.extra_options[k] = str(v)
                logger.debug(f"openGauss search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
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
                logger.debug(f"openGauss search MatchDenseExpr: {json.dumps(matchExpr.__dict__)}")
            elif isinstance(matchExpr, FusionExpr):
                logger.debug(f"openGauss search FusionExpr: {json.dumps(matchExpr.__dict__)}")

        # construct a basic query template
        base_query = """ 
        set enable_seqscan=off;
        WITH combined AS (
            (SELECT {fields}, {text_score} AS score
            FROM {table_name}
            WHERE {condition} AND {fulltext_condition}
            ORDER BY score DESC
            LIMIT {text_topn}
        ) UNION ALL(
            SELECT {fields}, (1 - {vector_score}) AS score
            FROM {table_name}
            WHERE {condition} AND {vector_condition}
            ORDER BY {vector_score}   
            LIMIT {vector_topn})
        )
        SELECT * FROM combined
        ORDER BY score 
        OFFSET %s LIMIT %s;
        """ 

        total_hits_count = 0
        vector_column_name = ''

        for indexName in indexNames:
            for knowledgebaseId in knowledgebaseIds:
                table_name = f"{indexName}_{knowledgebaseId}"
                try:
                    select_clause = ", ".join(output)
                    where_clause = []
                    params = []
                    text_where = []
                    vector_where = []
                    if filter_cond:
                        where_clause.append(filter_cond)
                    for matchExpr in matchExprs:
                        if isinstance(matchExpr, MatchTextExpr):
                            text_where.append(filter_fulltext)
                            tsquery = get_tsquery(matchExpr.matching_text)
                            params.append(tsquery)
                            params.append(tsquery)
                        elif isinstance(matchExpr, MatchDenseExpr):
                            vector_where.append(f"score > {matchExpr.extra_options.get('threshold', 1.0)}")
                            vector = str(matchExpr.embedding_data)
                            params.append(vector)
                            params.append(vector)
                            vector_column_name = matchExpr.vector_column_name

                    if len(matchExprs) == 0:
                        query = f'select {", ".join(output)}, COUNT(*) OVER() AS total_count from {table_name} where {" AND ".join(where_clause) if where_clause else "1=1"} offset {offset} Limit {limit};'
                    elif len(vector_where) == 0 and len(text_where) != 0:
                        query = f"""
                            set enable_seqscan=off;
                            select {", ".join(output)}, COUNT(*) OVER() AS total_count
                            from {table_name}
                            where {" AND ".join(where_clause) if where_clause else "1=1"}
                            AND {filter_fulltext}
                            order by {text_score_expr} DESC
                            offset {offset} Limit {limit};
                            """%(f"'{tsquery}'", f"'{tsquery}'")
                    else:
                        # construct a complete query
                        query = base_query.format(
                            fields=", ".join(output),
                            text_score=text_score_expr,
                            vector_score=vector_score_expr,
                            table_name=table_name,
                            condition=" AND ".join(where_clause) if where_clause else "1=1",
                            fulltext_condition=" AND ".join(text_where) if text_where else "1=1",  
                            vector_condition=" OR ".join(vector_where) if vector_where else "1=1",
                            vector_distance=f"{vector_column_name} <=> %s",
                            text_topn=next((e.topn for e in matchExprs if isinstance(e, MatchTextExpr)), 100),
                            vector_topn=next((e.topn for e in matchExprs if isinstance(e, MatchDenseExpr)), 1024)
                        )

                    # add pagination parameters
                    params.extend([offset, limit])

                    with self.conn.cursor() as cursor:
                        cursor.execute(query, params)
                        results = cursor.fetchall()
                        columns = [desc[0] for desc in cursor.description]
                        df = pd.DataFrame(results, columns=columns).drop_duplicates(subset = ['id']) # remove duplicates
                        if len(vector_where) != 0:
                            df[vector_column_name] = df[vector_column_name].apply(lambda x: list(map(float, ast.literal_eval(x))) if isinstance(x, str) else np.array([]))
                            total_hits_count += len(df)
                        else:
                            total_hits_count += results[0][-1] if results else 0
                        for col in df.select_dtypes(include=['object']).columns:
                            df[col] = df[col].fillna("")
                        df_list.append(df)
                        
                except Exception as e:
                    traceback.print_exc()
                    continue

        # combine results
        final_df = concat_dataframes(df_list, output)

        if matchExprs:
            if "score" not in final_df.columns:
                final_df["score"] = 0
            final_df['total_score'] = final_df["score"] + final_df[PAGERANK_FLD]
            final_df = final_df.sort_values(by='total_score', ascending=False)
            final_df = final_df.head(limit) 
            final_df = final_df.drop(columns=['total_score'])
        logger.info(f"final_df: {len(final_df)}")
        return final_df, total_hits_count

    def get(
            self, chunkId: str, indexName: str, knowledgebaseIds: list[str]
    ) -> dict | None:
        df_list = []
        table_list = []

        for knowledgebaseId in knowledgebaseIds:
            table_name = f"{indexName}_{knowledgebaseId}"
            table_list.append(table_name)

            query = f"SELECT * FROM {table_name} WHERE id = %s"
            logger.debug(f"Executing query on table: {table_name}")
            try:
                with self.conn.cursor() as cursor:
                    cursor.execute(query, (chunkId,))
                    data = cursor.fetchall()
                    columns = [desc[0] for desc in cursor.description]
                    if data:
                        df_list.append(pd.DataFrame(data, columns=columns))
            except psycopg2.Error as e:
                logger.warning(
                    f"Table not found or query failed: {table_name}, error: {str(e)}")
                continue

        if not df_list:
            logger.info(f"No data found for chunkId: {chunkId} in tables: {table_list}")
            return None

        res = concat_dataframes(df_list, ["id"])
        res_fields = self.getFields(res, res.columns.tolist())
        return res_fields.get(chunkId, None)

    def insert(
            self, documents: list[dict], indexName: str, knowledgebaseId: str = None
    ) -> list[str]:
        table_name = f"{indexName}_{knowledgebaseId}"
        try:
            with self.conn.cursor() as cursor:
                cursor.execute(f"SELECT 1 FROM {table_name} LIMIT 1;")
        except psycopg2.Error:
            vector_size = 0
            patt = re.compile(r"q_(?P<vector_size>\d+)_vec")
            for k in documents[0].keys():
                m = patt.match(k)
                if m:
                    vector_size = int(m.group("vector_size"))
                    break
            if vector_size == 0:
                raise ValueError("Cannot infer vector size from documents")
            self.create_table(table_name, vector_size)

        docs = copy.deepcopy(documents)
        for d in docs:
            assert "_id" not in d
            assert "id" in d
            for k, v in d.items():
                if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                    assert isinstance(v, list)
                    d[k] = "###".join(v)
                    logger.info(f"insert_data: {d[k]}")
                elif re.search(r"_feas$", k):
                    d[k] = json.dumps(v)
                elif k == 'kb_id':
                    if isinstance(d[k], list):
                        d[k] = d[k][0]  
                elif k == "position_int":
                    assert isinstance(v, list)
                    arr = [num for row in v for num in row]
                    d[k] = "_".join(f"{num:08x}" for num in arr)
                elif k in ["page_num_int", "top_int"]:
                    assert isinstance(v, list)
                    d[k] = "_".join(f"{num:08x}" for num in v)

        # delete conflicting records
        ids = [f"'{d['id']}'" for d in docs]
        str_ids = ", ".join(ids)
        delete_query = f"DELETE FROM {table_name} WHERE id IN ({str_ids})"
        with self.conn.cursor() as cursor:
            cursor.execute(delete_query)

        insert_query = f"INSERT INTO {table_name} ({', '.join(docs[0].keys())}) VALUES %s"
        values = [
            tuple(d.values())
            for d in docs
        ]
        from psycopg2.extras import execute_values
        with self.conn.cursor() as cursor:
            execute_values(cursor, insert_query, values)
        self.conn.commit()
        logger.debug(f"Inserted into {table_name}: {str_ids}.")
        return []

    def update(
            self, condition: dict, newValue: dict, indexName: str, knowledgebaseId: str
    ) -> bool:
        table_name = f"{indexName}_{knowledgebaseId}"
        table_columns = self.get_columns(table_name)
        filter_cond = equivalent_condition_to_str(condition, table_columns)

        update_set = []
        update_values = []
        for k, v in list(newValue.items()):
            if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                assert isinstance(v, list)
                v = "###".join(v)
            elif re.search(r"_feas$", k):
                v = json.dumps(v)
            elif k.endswith("_kwd") and isinstance(v, list):
                v = " ".join(v)
            elif k == 'kb_id' and isinstance(v, list):
                v = v[0]
            elif k == "position_int":
                assert isinstance(v, list)
                arr = [num for row in v for num in row]
                v = "_".join(f"{num:08x}" for num in arr)
            elif k in ["page_num_int", "top_int"]:
                assert isinstance(v, list)
                v = "_".join(f"{num:08x}" for num in v)
            elif k == "remove":
                del newValue[k]
                if v in ["PAGERANK_FLD"]:
                    newValue[v] = 0
                continue

            update_set.append(f"{k} = %s")
            update_values.append(v)

        update_query = f"""
            UPDATE {table_name}
            SET {', '.join(update_set)}
            WHERE {filter_cond}
            """

        with self.conn.cursor() as cursor:
            try:
                cursor.execute(update_query, update_values)
                self.conn.commit()
                logger.debug(f"openGauss updated table {table_name}, filter {condition}, newValue {newValue}.")
                return True
            except psycopg2.Error as e:
                logger.error(f"Failed to update table {table_name}: {str(e)}")
                self.conn.rollback()
                return False


    def delete(self, condition: dict, indexName: str, knowledgebaseId: str) -> int:
        table_name = f"{indexName}_{knowledgebaseId}"

        if not self.indexExist(indexName, knowledgebaseId):
            logger.warning(f"Skipped deleting from table {table_name} since the table doesn't exist.")
            return 0

        table_columns = self.get_columns(table_name)
        filter_cond = equivalent_condition_to_str(condition, table_columns)

        delete_query = f"DELETE FROM {table_name} WHERE {filter_cond}"
        with self.conn.cursor() as cursor:
            try:
                cursor.execute(delete_query)
                self.conn.commit()
                deleted_rows = cursor.rowcount
                logger.debug(
                    f"openGauss delete table {table_name}, filter {filter_cond}. Deleted rows: {deleted_rows}.")
                return deleted_rows
            except psycopg2.Error as e:
                logger.error(f"Failed to delete from table {table_name}: {str(e)}")
                self.conn.rollback()
                return 0


    """
    Helper functions for search result
    """

    def getTotal(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> int:
        if isinstance(res, tuple):
            return res[1]
        return len(res)

    def getChunkIds(self, res: tuple[pd.DataFrame, int] | pd.DataFrame) -> list[str]:
        if isinstance(res, tuple):
            res = res[0]
        return list(res["id"])

    def getFields(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fields: list[str]) -> dict[str, dict]:
        logging.info(f"res fields : {res} {fields}")
        if isinstance(res, tuple):
            res = res[0]
        if not fields:
            return {}
        fieldsAll = fields.copy()
        fieldsAll.append('id')
        column_map = {col.lower(): col for col in res.columns}
        matched_columns = {column_map[col.lower()]: col for col in set(fieldsAll) if col.lower() in column_map}
        none_columns = [col for col in set(fieldsAll) if col.lower() not in column_map]

        res2 = res[matched_columns.keys()]
        res2 = res2.rename(columns=matched_columns)
        res2.drop_duplicates(subset=['id'], inplace=True)

        for column in res2.columns:
            k = column.lower()
            if k in ["important_kwd", "question_kwd", "entities_kwd", "tag_kwd", "source_id"]:
                res2[column] = res2[column].astype(str).fillna("")
                res2[column] = res2[column].apply(lambda v: [kwd for kwd in v.split("###") if kwd])
            elif k == "position_int":
                def to_position_int(v):
                    if v:
                        arr = [int(hex_val, 16) for hex_val in v.split('_')]
                        v = [arr[i:i + 5] for i in range(0, len(arr), 5)]
                    else:
                        v = []
                    return v

                res2[column] = res2[column].apply(to_position_int)
            elif k in ["page_num_int", "top_int"]:
                res2[column] = res2[column].apply(lambda v: [int(hex_val, 16) for hex_val in v.split('_')] if v else [])
            else:
                pass
        for column in none_columns:
            res2[column] = None

        return res2.set_index("id").to_dict(orient="index")
    
    def get_columns(self, table_name: str):
        table_columns = {}
        try:
            with self.conn.cursor() as cursor:
                cursor.execute(
                    f"SELECT column_name, data_type, column_default FROM information_schema.columns WHERE table_name = %s",
                    (table_name,))
                for row in cursor.fetchall():
                    column_name, data_type, column_default = row
                    table_columns[column_name] = (data_type, column_default)
            return table_columns
        except psycopg2.Error as e:
            logger.error(f"Failed to get columns from table {table_name}: {str(e)}")
            self.conn.rollback()
            return table_columns

    def getHighlight(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, keywords: list[str], fieldnm: str):
        if isinstance(res, tuple):
            res = res[0]
        ans = {}
        num_rows = len(res)
        column_id = res["id"]
        if fieldnm not in res:
            return {}
        sentence_split_pattern = r"[.?!;\n，。！？；]"
        boundary_chars = r"[\s.,?!/;:，。！？；：‘’“”\"'\(\)\[\]{}【】—－-]"
        for i in range(num_rows):
            id = column_id.iloc[i]
            txt = res[fieldnm].iloc[i]
            txt = re.sub(r"[\r\n]", " ", txt, flags=re.IGNORECASE | re.MULTILINE)
            txts = []
            for t in re.split(sentence_split_pattern, txt):
                if not t.strip(): 
                    continue
                for w in keywords:
                    t = re.sub(
                        r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])"
                        % re.escape(w),
                        r"\1<em>\2</em>\3",
                        t,
                        flags=re.IGNORECASE | re.MULTILINE,
                    )
                txts.append(t)
            ans[id] = "...".join(txts)
        return ans

    def getAggregation(self, res: tuple[pd.DataFrame, int] | pd.DataFrame, fieldnm: str):
        """
        TODO
        """
        return list()

    """
    SQL
    """

    def sql(sql: str, fetch_size: int, format: str):
        raise NotImplementedError("Not implemented")