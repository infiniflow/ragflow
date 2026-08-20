#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from unittest.mock import patch

from core.config import AppConfig


def test_database_old_yaml():
    return_value = {
        "mysql": {"name": "old_db", "user": "root", "password": "oldpass", "host": "127.0.0.1", "port": 3306},
        "postgres": {"database": "old_pg", "user": "pguser", "password": "pgpass", "host": "127.0.0.1", "port": 5432},
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    mysql_cfg = config.database.mysql
    pg_cfg = config.database.postgres
    assert mysql_cfg.name == "old_db"
    assert pg_cfg.database == "old_pg"

def test_database_new_yaml():
    return_value = {
        "database": {
            "mysql": {"name": "new_db", "user": "root", "password": "newpass", "host": "127.0.0.2", "port": 3306},
            "postgres": {"database": "new_pg", "user": "pguser", "password": "newpgpass", "host": "127.0.0.2", "port": 5432},
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    mysql_cfg = config.database.mysql
    pg_cfg = config.database.postgres
    assert mysql_cfg.name == "new_db"
    assert pg_cfg.database == "new_pg"
