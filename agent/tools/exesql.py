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
import json
import os
import re
from abc import ABC
import pandas as pd
import pymysql
import psycopg2
import pyodbc
from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from common.connection_utils import timeout


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
        self.check_valid_value(self.db_type, "Choose DB type", ['mysql', 'postgres', 'mariadb', 'mssql', 'IBM DB2', 'trino'])
        self.check_empty(self.database, "Database name")
        self.check_empty(self.username, "database username")
        self.check_empty(self.host, "IP Address")
        self.check_positive_integer(self.port, "IP Port")
        if self.db_type != "trino":
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

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("ExeSQL processing"):
            return

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

        if self.check_if_canceled("ExeSQL processing"):
            return

        vars = self.get_input_elements_from_text(sql)
        args = {}
        for k, o in vars.items():
            args[k] = o["value"]
            if not isinstance(args[k], str):
                try:
                    args[k] = json.dumps(args[k], ensure_ascii=False)
                except Exception:
                    args[k] = str(args[k])
            self.set_input_value(k, args[k])
        sql = self.string_format(sql, args)

        if self.check_if_canceled("ExeSQL processing"):
            return

        sqls = sql.split(";")
        if self._param.db_type in ["mysql", "mariadb"]:
            db = pymysql.connect(db=self._param.database, user=self._param.username, host=self._param.host,
                                 port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'postgres':
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
        elif self._param.db_type == 'trino':
            try:
                import trino
                from trino.auth import BasicAuthentication
            except Exception:
                raise Exception("Missing dependency 'trino'. Please install: pip install trino")

            def _parse_catalog_schema(db: str):
                if not db:
                    return None, None
                if "." in db:
                    c, s = db.split(".", 1)
                elif "/" in db:
                    c, s = db.split("/", 1)
                else:
                    c, s = db, "default"
                return c, s

            catalog, schema = _parse_catalog_schema(self._param.database)
            if not catalog:
                raise Exception("For Trino, `database` must be 'catalog.schema' or at least 'catalog'.")

            http_scheme = "https" if os.environ.get("TRINO_USE_TLS", "0") == "1" else "http"
            auth = None
            if http_scheme == "https" and self._param.password:
                auth = BasicAuthentication(self._param.username, self._param.password)

            try:
                db = trino.dbapi.connect(
                    host=self._param.host,
                    port=int(self._param.port or 8080),
                    user=self._param.username or "ragflow",
                    catalog=catalog,
                    schema=schema or "default",
                    http_scheme=http_scheme,
                    auth=auth
                )
            except Exception as e:
                raise Exception("Database Connection Failed! \n" + str(e))
        elif self._param.db_type == 'IBM DB2':
            import ibm_db
            conn_str = (
                f"DATABASE={self._param.database};"
                f"HOSTNAME={self._param.host};"
                f"PORT={self._param.port};"
                f"PROTOCOL=TCPIP;"
                f"UID={self._param.username};"
                f"PWD={self._param.password};"
            )
            try:
                conn = ibm_db.connect(conn_str, "", "")
            except Exception as e:
                raise Exception("Database Connection Failed! \n" + str(e))

            sql_res = []
            formalized_content = []
            for single_sql in sqls:
                if self.check_if_canceled("ExeSQL processing"):
                    ibm_db.close(conn)
                    return

                single_sql = single_sql.replace("```", "").strip()
                if not single_sql:
                    continue
                single_sql = re.sub(r"\[ID:[0-9]+\]", "", single_sql)

                stmt = ibm_db.exec_immediate(conn, single_sql)
                rows = []
                row = ibm_db.fetch_assoc(stmt)
                while row and len(rows) < self._param.max_records:
                    if self.check_if_canceled("ExeSQL processing"):
                        ibm_db.close(conn)
                        return
                    rows.append(row)
                    row = ibm_db.fetch_assoc(stmt)

                if not rows:
                    sql_res.append({"content": "No record in the database!"})
                    continue

                df = pd.DataFrame(rows)
                for col in df.columns:
                    if pd.api.types.is_datetime64_any_dtype(df[col]):
                        df[col] = df[col].dt.strftime("%Y-%m-%d")

                df = df.where(pd.notnull(df), None)

                sql_res.append(convert_decimals(df.to_dict(orient="records")))
                formalized_content.append(df.to_markdown(index=False, floatfmt=".6f"))

            ibm_db.close(conn)

            self.set_output("json", sql_res)
            self.set_output("formalized_content", "\n\n".join(formalized_content))
            return self.output("formalized_content")
        try:
            cursor = db.cursor()
        except Exception as e:
            raise Exception("Database Connection Failed! \n" + str(e))

        sql_res = []
        formalized_content = []
        for single_sql in sqls:
            if self.check_if_canceled("ExeSQL processing"):
                cursor.close()
                db.close()
                return

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

            single_res = single_res.where(pd.notnull(single_res), None)

            sql_res.append(convert_decimals(single_res.to_dict(orient='records')))
            formalized_content.append(single_res.to_markdown(index=False, floatfmt=".6f"))

        cursor.close()
        db.close()

        self.set_output("json", sql_res)
        self.set_output("formalized_content", "\n\n".join(formalized_content))
        return self.output("formalized_content")

    def thoughts(self) -> str:
        return "Query sent—waiting for the data."
