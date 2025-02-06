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
from abc import ABC
import re
from copy import deepcopy

import pandas as pd
import pymysql
import psycopg2
from agent.component import GenerateParam, Generate
import pyodbc
import logging


class ExeSQLParam(GenerateParam):
    """
    Define the ExeSQL component parameters.
    """

    def __init__(self):
        super().__init__()
        self.db_type = "mysql"
        self.database = ""
        self.username = ""
        self.host = ""
        self.port = 3306
        self.password = ""
        self.loop = 3
        self.top_n = 30

    def check(self):
        super().check()
        self.check_valid_value(self.db_type, "Choose DB type", ['mysql', 'postgresql', 'mariadb', 'mssql'])
        self.check_empty(self.database, "Database name")
        self.check_empty(self.username, "database username")
        self.check_empty(self.host, "IP Address")
        self.check_positive_integer(self.port, "IP Port")
        self.check_empty(self.password, "Database password")
        self.check_positive_integer(self.top_n, "Number of records")
        if self.database == "rag_flow":
            if self.host == "ragflow-mysql":
                raise ValueError("The host is not accessible.")
            if self.password == "infini_rag_flow":
                raise ValueError("The host is not accessible.")


class ExeSQL(Generate, ABC):
    component_name = "ExeSQL"

    def _refactor(self,ans):
        match = re.search(r"```sql\s*(.*?)\s*```", ans, re.DOTALL)
        if match:
            ans = match.group(1)  # Query content
            return ans
        else:
            print("no markdown")
        ans = re.sub(r'^.*?SELECT ', 'SELECT ', (ans), flags=re.IGNORECASE)
        ans = re.sub(r';.*?SELECT ', '; SELECT ', ans, flags=re.IGNORECASE)
        ans = re.sub(r';[^;]*$', r';', ans)
        if not ans:
            raise Exception("SQL statement not found!")
        return ans

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = "".join([str(a) for a in ans["content"]]) if "content" in ans else ""
        ans = self._refactor(ans)
        logging.info("db_type: ",self._param.db_type)
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
        if not hasattr(self, "_loop"):
            setattr(self, "_loop", 0)
            self._loop += 1
        input_list=re.split(r';', ans.replace(r"\n", " "))
        sql_res = []
        for i in range(len(input_list)):
            single_sql=input_list[i]
            while self._loop <= self._param.loop:
                self._loop+=1
                if not single_sql:
                    break
                try:
                    logging.info("single_sql: ", single_sql)
                    cursor.execute(single_sql)
                    if cursor.rowcount == 0:
                        sql_res.append({"content": "No record in the database!"})
                        break
                    if self._param.db_type == 'mssql':
                        single_res  = pd.DataFrame.from_records(cursor.fetchmany(self._param.top_n),columns = [desc[0] for desc in cursor.description])
                    else:
                        single_res = pd.DataFrame([i for i in cursor.fetchmany(self._param.top_n)])
                        single_res.columns = [i[0] for i in cursor.description]
                    sql_res.append({"content":  single_res.to_markdown()})
                    break
                except Exception as e:
                    single_sql = self._regenerate_sql(single_sql, str(e), **kwargs)
                    single_sql = self._refactor(single_sql)
                    if self._loop > self._param.loop:
                        sql_res.append({"content": "Can't query the correct data via SQL statement."})
                        # raise Exception("Maximum loop time exceeds. Can't query the correct data via SQL statement.")
        db.close()
        if not sql_res:
            return ExeSQL.be_output("")
        return pd.DataFrame(sql_res)

    def _regenerate_sql(self, failed_sql, error_message,**kwargs):
        prompt = f'''
        ## You are the Repair SQL Statement Helper, please modify the original SQL statement based on the SQL query error report.
        ## The original SQL statement is as follows:{failed_sql}.
        ## The contents of the SQL query error report is as follows:{error_message}.
        ## Answer only the modified SQL statement. Please do not give any explanation, just answer the code.
'''
        self._param.prompt=prompt
        kwargs_ = deepcopy(kwargs)
        kwargs_["stream"] = False
        response = Generate._run(self, [], **kwargs_)
        try:
            regenerated_sql = response.loc[0,"content"]
            return regenerated_sql
        except Exception as e:
            logging.error(f"Failed to regenerate SQL: {e}")
            return None

    def debug(self, **kwargs):
        return self._run([], **kwargs)
