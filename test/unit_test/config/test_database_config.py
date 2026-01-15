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

from core.config.app import AppConfig
from core.config.components.base import DatabaseConfig
from core.config.components.base.database import OceanBaseConfig, OceanBaseInnerConfig
from core.types.database import DatabaseType

def test_db_defaults(monkeypatch):
    """Test default database active type and default fields"""

    monkeypatch.delenv("DB_TYPE", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    db = cfg.database

    assert db.active == DatabaseType.MYSQL

    mysql = db.mysql
    assert mysql.host == "localhost"
    assert mysql.port == 5455
    assert mysql.user == "root"
    assert mysql.password == ""
    assert mysql.name == "rag_flow"

    pg = db.postgres
    assert pg.host == "localhost"
    assert pg.port == 5432
    assert pg.user == "rag_flow"
    assert pg.password == ""
    assert pg.database == "rag_flow"

    ob = db.oceanbase
    ob_cfg = ob.config
    assert ob.scheme == "oceanbase"
    assert ob_cfg.user == "root"
    assert ob_cfg.password is None
    assert ob_cfg.host == "localhost"
    assert ob_cfg.port == 2881
    assert ob_cfg.db_name == "test"


def test_db_yaml_override(monkeypatch):
    """Test that YAML values override default database configuration"""
    monkeypatch.delenv("DB_TYPE", raising=False)

    return_value = {
        "database": {
            "active": "postgres",
            "mysql": {
                "host": "1.2.3.4",
                "port": 3306,
                "user": "mysql_user",
                "password": "mysql_pass",
                "name": "mysql_db",
            },
            "postgres": {
                "host": "pg_host",
                "port": 5433,
                "user": "pg_user",
                "password": "pg_pass",
                "database": "pg_db",
            },
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        cfg = AppConfig()

    db = cfg.database

    assert db.active == DatabaseType.POSTGRES
    mysql = db.mysql
    assert mysql.host == "1.2.3.4"
    assert mysql.port == 3306
    assert mysql.user == "mysql_user"
    assert mysql.password == "mysql_pass"
    assert mysql.name == "mysql_db"

    pg = db.postgres
    assert pg.host == "pg_host"
    assert pg.port == 5433
    assert pg.user == "pg_user"
    assert pg.password == "pg_pass"
    assert pg.database == "pg_db"


def test_oceanbase_mysql_override(monkeypatch):
    """Test OceanBase schema='mysql' uses MySQL config"""
    monkeypatch.delenv("DB_TYPE", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {
                "database": {
                    "active": "mysql",
                    "mysql": {
                        "host": "1.2.3.4",
                        "port": 3306,
                        "user": "mysql_user",
                        "password": "mysql_pass",
                        "name": "mysql_db",
                    },
                    "oceanbase": {
                        "scheme": "mysql",
                        "config": {
                            "user": "ob_user",
                            "password": "ob_pass",
                            "host": "ob_host",
                            "port": 2882,
                            "db_name": "ob_db",
                        }
                    }
                }
            },
            {}
        ]
        cfg = AppConfig()

    ob_cfg = cfg.database.oceanbase.config

    assert cfg.database.oceanbase.scheme == "mysql"
    assert ob_cfg.user == "mysql_user"
    assert ob_cfg.password == "mysql_pass"
    assert ob_cfg.host == "1.2.3.4"
    assert ob_cfg.port == 3306
    assert ob_cfg.db_name == "mysql_db"


def test_oceanbase_native(monkeypatch):
    """Test OceanBase schema='oceanbase' uses its own config"""
    monkeypatch.delenv("DB_TYPE", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {
                "database": {
                    "active": "mysql",
                    "mysql": {
                        "host": "1.2.3.4",
                        "port": 3306,
                        "user": "mysql_user",
                        "password": "mysql_pass",
                        "name": "mysql_db",
                    },
                    "oceanbase": {
                        "scheme": "oceanbase",
                        "config": {
                            "user": "ob_user",
                            "password": "ob_pass",
                            "host": "ob_host",
                            "port": 2882,
                            "db_name": "ob_db",
                        }
                    }
                }
            },
            {}
        ]
        cfg = AppConfig()

    ob_cfg = cfg.database.oceanbase.config

    assert cfg.database.oceanbase.scheme == "oceanbase"
    assert ob_cfg.user == "ob_user"
    assert ob_cfg.password == "ob_pass"
    assert ob_cfg.host == "ob_host"
    assert ob_cfg.port == 2882
    assert ob_cfg.db_name == "ob_db"


def test_mysql_defaults(monkeypatch):
    """
    Test MySQL default values are used when environment variables are not set.
    """
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {"database": {"active": "mysql"}},       # service_conf.yaml
            {}     # local.service_conf.yaml
        ]
        cfg = AppConfig()

    mysql = cfg.database.mysql

    assert mysql.host == "localhost"
    assert mysql.port == 5455
    assert mysql.user == "root"
    assert mysql.password == ""
    assert mysql.database == "rag_flow"


def test_postgres_defaults(monkeypatch):
    """
    Test Postgres default values are used when environment variables are not set.
    """
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {"database": {"active": "postgres"}},  # service_conf.yaml
            {}  # local.service_conf.yaml
        ]
        cfg = AppConfig()

    pg = cfg.database.postgres
    assert pg.host == "localhost"
    assert pg.port == 5432
    assert pg.user == "rag_flow"
    assert pg.database == "rag_flow"
    assert pg.password == ""


def test_database_current_property(monkeypatch):
    """Test DatabaseConfig.current returns the correct active database config"""
    monkeypatch.delenv("DB_TYPE", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    db = cfg.database

    assert db.current is db.mysql

    db.active = DatabaseType.POSTGRES
    assert db.current is db.postgres

    db.active = DatabaseType.OCEANBASE
    assert db.current is db.oceanbase


def test_normalize_ob_config_on_init(monkeypatch):
    # Patch YAML to be empty
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    # Prepare MySQL config
    cfg.database.mysql.host = "mysqlhost"
    cfg.database.mysql.port = 3306
    cfg.database.mysql.user = "mysqluser"
    cfg.database.mysql.password = "mysqlpass"
    cfg.database.mysql.name = "mysqldb"

    # Re-create DatabaseConfig with scheme='mysql' to trigger validator
    db_cfg = DatabaseConfig(
        active=DatabaseType.MYSQL,
        mysql=cfg.database.mysql,
        oceanbase=OceanBaseConfig(scheme="mysql")
    )

    ob_cfg = db_cfg.oceanbase.config
    assert ob_cfg.host == "mysqlhost"
    assert ob_cfg.port == 3306
    assert ob_cfg.user == "mysqluser"
    assert ob_cfg.password == "mysqlpass"
    assert ob_cfg.db_name == "mysqldb"

    # Scheme = "oceanbase" → not override
    db_cfg = DatabaseConfig(
        active=DatabaseType.MYSQL,
        mysql=cfg.database.mysql,
        oceanbase=OceanBaseConfig(
            scheme="oceanbase",
            config=OceanBaseInnerConfig(user="obuser", password="obpass")
        )
    )
    ob_cfg = db_cfg.oceanbase.config
    assert ob_cfg.user == "obuser"
    assert ob_cfg.password == "obpass"