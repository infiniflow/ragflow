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
            raise Exception("Maximum loop time exceeds. Can't query the correct data via sql statement.")
        self._loop += 1

        ans = self.get_input()
        ans = "".join(ans["content"]) if "content" in ans else ""
        ans = re.sub(r'^.*?SELECT ', 'SELECT ', repr(ans), flags=re.IGNORECASE)
        ans = re.sub(r';.*?SELECT ', '; SELECT ', ans)
        ans = re.sub(r';[^;]*$', r';', ans)
        if not ans:
            return ExeSQL.be_output("SQL statement not found!")

        if self._param.db_type in ["mysql", "mariadb"]:
            db = MySQLDatabase(self._param.database, user=self._param.username, host=self._param.host,
                               port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'postgresql':
            db = PostgresqlDatabase(self._param.database, user=self._param.username, host=self._param.host,
                                    port=self._param.port, password=self._param.password)

        try:
            db.connect()
            query = db.execute_sql(ans)
            sql_res = [{"content": rec + "\n"} for rec in [str(i) for i in query.fetchall()]]
            db.close()
        except Exception as e:
            return ExeSQL.be_output("**Error**:" + str(e) + "\nError SQL Statement:" + ans)

        if not sql_res:
            return ExeSQL.be_output("No record in the database!")

        sql_res.insert(0, {"content": "Number of records retrieved from the database is " + str(len(sql_res)) + "\n"})
        df = pd.DataFrame(sql_res[0:self._param.top_n + 1])
        return ExeSQL.be_output(df.to_markdown())
