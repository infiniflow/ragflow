#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import os
import re
from abc import ABC
import pandas as pd
import pymysql
import psycopg2
import pyodbc
from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from api.utils.api_utils import timeout


class ExeSQLParam(ToolParamBase):
    """
    Define the ExeSQL component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "execute_sql",
            "description": "This is a tool that can execute SQL.",
            "parameters": {
                "sql": {
                    "type": "string",
                    "description": "The SQL needs to be executed.",
                    "default": "{sys.query}",
                    "required": True
                }
            }
        }
        super().__init__()
        self.db_type = "mysql"
        self.database = ""
        self.username = ""
        self.host = ""
        self.port = 3306
        self.password = ""
        self.max_records = 1024

    def check(self):
        self.check_valid_value(self.db_type, "Choose DB type", ['mysql', 'postgresql', 'mariadb', 'mssql'])
        self.check_empty(self.database, "Database name")
        self.check_empty(self.username, "database username")
        self.check_empty(self.host, "IP Address")
        self.check_positive_integer(self.port, "IP Port")
        self.check_empty(self.password, "Database password")
        self.check_positive_integer(self.max_records, "Maximum number of records")
        if self.database == "rag_flow":
            if self.host == "ragflow-mysql":
                raise ValueError("For the security reason, it dose not support database named rag_flow.")
            if self.password == "infini_rag_flow":
                raise ValueError("For the security reason, it dose not support database named rag_flow.")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "sql": {
                "name": "SQL",
                "type": "line"
            }
        }


class ExeSQL(ToolBase, ABC):
    component_name = "ExeSQL"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 60))
    def _invoke(self, **kwargs):

        def convert_decimals(obj):
            from decimal import Decimal
            if isinstance(obj, Decimal):
                return float(obj)  # 或 str(obj)
            elif isinstance(obj, dict):
                return {k: convert_decimals(v) for k, v in obj.items()}
            elif isinstance(obj, list):
                return [convert_decimals(item) for item in obj]
            return obj

        sql = kwargs.get("sql")
        if not sql:
            raise Exception("SQL for `ExeSQL` MUST not be empty.")
        sqls = sql.split(";")

        if self._param.db_type in ["mysql", "mariadb"]:
            db = pymysql.connect(db=self._param.database, user=self._param.username, host=self._param.host,
                                 port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'postgresql':
            db = psycopg2.connect(dbname=self._param.database, user=self._param.username, host=self._param.host,
                                  port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'mssql':
            conn_str = (
                    r'DRIVER={ODBC Driver 17 for SQL Server};'
                    r'SERVER=' + self._param.host + ',' + str(self._param.port) + ';'
                    r'DATABASE=' + self._param.database + ';'
                    r'UID=' + self._param.username + ';'
                    r'PWD=' + self._param.password
            )
            db = pyodbc.connect(conn_str)
        try:
            cursor = db.cursor()
        except Exception as e:
            raise Exception("Database Connection Failed! \n" + str(e))

        sql_res = []
        formalized_content = []
        for single_sql in sqls:
            single_sql = single_sql.replace('```','')
            if not single_sql:
                continue
            single_sql = re.sub(r"\[ID:[0-9]+\]", "", single_sql)
            cursor.execute(single_sql)
            if cursor.rowcount == 0:
                sql_res.append({"content": "No record in the database!"})
                break
            if self._param.db_type == 'mssql':
                single_res = pd.DataFrame.from_records(cursor.fetchmany(self._param.max_records),
                                                       columns=[desc[0] for desc in cursor.description])
            else:
                single_res = pd.DataFrame([i for i in cursor.fetchmany(self._param.max_records)])
                single_res.columns = [i[0] for i in cursor.description]

            for col in single_res.columns:
                if pd.api.types.is_datetime64_any_dtype(single_res[col]):
                    single_res[col] = single_res[col].dt.strftime('%Y-%m-%d')

            sql_res.append(convert_decimals(single_res.to_dict(orient='records')))
            formalized_content.append(single_res.to_markdown(index=False, floatfmt=".6f"))

        self.set_output("json", sql_res)
        self.set_output("formalized_content", "\n\n".join(formalized_content))
        return self.output("formalized_content")

    def thoughts(self) -> str:
        return "Query sent—waiting for the data."
