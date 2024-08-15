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
        self.port = 1
        self.password = ""
        self.loop = 5

    def check(self):
        self.check_valid_value(self.db_type, "Choose DB type", ['mysql', 'postgresql', 'mariadb'])
        self.check_empty(self.database, "Database name")
        self.check_empty(self.username, "database username")
        self.check_empty(self.host, "IP Address")
        self.check_positive_integer(self.port, "IP Port")
        self.check_empty(self.password, "Database password")


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
        if not ans:
            return ExeSQL.be_output("")
        if self._param.db_type in ["mysql", "mariadb"]:
            db = MySQLDatabase(self._param.database, user=self._param.username, host=self._param.host,
                               port=self._param.port, password=self._param.password)
        elif self._param.db_type == 'postgresql':
            db = PostgresqlDatabase(self._param.database, user=self._param.username, host=self._param.host,
                                    port=self._param.port, password=self._param.password)

        try:
            db.connect()
            query = db.execute_sql(ans)
            res = "\n".join([str(i) for i in query.fetchall()])
            db.close()
            return ExeSQL.be_output(res)
        except Exception as e:
            return ExeSQL.be_output("**Error**:" + str(e))

