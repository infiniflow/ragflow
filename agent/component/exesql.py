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
import pandas as pd
from peewee import MySQLDatabase, PostgresqlDatabase
from agent.component.base import ComponentBase, ComponentParamBase


class ExeSQLParam(ComponentParamBase):
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
        self.check_valid_value(self.db_type, "Choose DB type", ['mysql', 'postgresql', 'mariadb'])
        self.check_empty(self.database, "Database name")
        self.check_empty(self.username, "database username")
        self.check_empty(self.host, "IP Address")
        self.check_positive_integer(self.port, "IP Port")
        self.check_empty(self.password, "Database password")
        self.check_positive_integer(self.top_n, "Number of records")


class ExeSQL(ComponentBase, ABC):
    component_name = "ExeSQL"

    def _run(self, history, **kwargs):
        if not hasattr(self, "_loop"):
            setattr(self, "_loop", 0)
        if self._loop >= self._param.loop:
            self._loop = 0
            raise Exception("Maximum loop time exceeds. Can't query the correct data via SQL statement.")
        self._loop += 1

        ans = self.get_input()
        ans = "".join(ans["content"]) if "content" in ans else ""
        ans = re.sub(r'^.*?SELECT ', 'SELECT ', repr(ans), flags=re.IGNORECASE)
        ans = re.sub(r';.*?SELECT ', '; SELECT ', ans, flags=re.IGNORECASE)
        ans = re.sub(r';[^;]*$', r';', ans)
        if not ans:
            raise Exception("SQL statement not found!")

        if self._param.db_type in ["mysql", "mariadb"]:
            db = MySQLDatabase(self._param.database, user=self._param.username, host=self._param.host,
                               port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'postgresql':
            db = PostgresqlDatabase(self._param.database, user=self._param.username, host=self._param.host,
                                    port=self._param.port, password=self._param.password)

        try:
            db.connect()
        except Exception as e:
            raise Exception("Database Connection Failed! \n" + str(e))
        sql_res = []
        for single_sql in re.split(r';', ans.replace(r"\n", " ")):
            if not single_sql:
                continue
            try:
                query = db.execute_sql(single_sql)
                if query.rowcount == 0:
                    sql_res.append({"content": "\nTotal: " + str(query.rowcount) + "\n No record in the database!"})
                    continue
                single_res = pd.DataFrame([i for i in query.fetchmany(size=self._param.top_n)])
                single_res.columns = [i[0] for i in query.description]
                sql_res.append({"content": "\nTotal: " + str(query.rowcount) + "\n" + single_res.to_markdown()})
            except Exception as e:
                sql_res.append({"content": "**Error**:" + str(e) + "\nError SQL Statement:" + single_sql})
                pass
        db.close()

        if not sql_res:
            return ExeSQL.be_output("")

        return pd.DataFrame(sql_res)
