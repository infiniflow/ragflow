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
from core.config.types import DatabaseType


# ------------------------
# Default values
# ------------------------

def test_db_defaults(monkeypatch):
    """Test that default database active type and all fields are correct."""
    monkeypatch.delenv("DB_TYPE", raising=False)
    with patch("core.config.app.load_yaml", side_effect=[{}, {}]):
        cfg = AppConfig()

    db = cfg.database
    assert db.active == DatabaseType.MYSQL

    # MySQL defaults
    mysql = db.mysql
    assert mysql.host == "localhost"
    assert mysql.port == 5455
    assert mysql.user == "root"
    assert mysql.password is None
    assert mysql.name == "rag_flow"
    # Ensure DSN does not include password if None
    assert "root@" in str(mysql.dsn)
    assert ":None" not in str(mysql.dsn)

    # Postgres defaults
    pg = db.postgres
    assert pg.host == "localhost"
    assert pg.port == 5432
    assert pg.user == "rag_flow"
    assert pg.password is None
    assert pg.database == "rag_flow"

    # OceanBase defaults
    ob = db.oceanbase
    ob_cfg = ob.config
    assert ob.scheme == "oceanbase"
    assert ob_cfg.user == "root"
    assert ob_cfg.password is None
    assert ob_cfg.host == "localhost"
    assert ob_cfg.port == 2881
    assert ob_cfg.db_name == "test"


# ------------------------
# YAML overrides
# ------------------------

def test_db_yaml_override(monkeypatch):
    """Test that YAML values override default database configuration."""
    monkeypatch.delenv("DB_TYPE", raising=False)
    yaml_cfg = {
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
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
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


def test_yaml_priority_over_env(monkeypatch):
    """Ensure YAML values take precedence over environment variables."""
    monkeypatch.setenv("DB_TYPE", "postgres")
    yaml_cfg = {
        "database": {
            "active": "mysql",
            "mysql": {"host": "10.0.0.1", "port": 3306, "user": "yaml_user"}
        }
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    # YAML active overrides ENV
    assert cfg.database.active == DatabaseType.MYSQL
    assert cfg.database.mysql.host == "10.0.0.1"
    assert cfg.database.mysql.user == "yaml_user"


# ------------------------
# OceanBase special logic
# ------------------------

def test_oceanbase_mysql_override(monkeypatch):
    """OceanBase with schema='mysql' uses MySQL config values."""
    monkeypatch.delenv("DB_TYPE", raising=False)
    yaml_cfg = {
        "database": {
            "active": "mysql",
            "mysql": {"host": "1.2.3.4", "port": 3306, "user": "mysql_user", "password": "mysql_pass", "name": "mysql_db"},
            "oceanbase": {"scheme": "mysql", "config": {"user": "ob_user", "password": "ob_pass", "host": "ob_host", "port": 2882, "db_name": "ob_db"}}
        }
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    ob_cfg = cfg.database.oceanbase.config
    assert cfg.database.oceanbase.scheme == "mysql"
    assert ob_cfg.user == "mysql_user"
    assert ob_cfg.password == "mysql_pass"
    assert ob_cfg.host == "1.2.3.4"
    assert ob_cfg.port == 3306
    assert ob_cfg.db_name == "mysql_db"


def test_oceanbase_native(monkeypatch):
    """OceanBase with schema='oceanbase' uses its own config."""
    monkeypatch.delenv("DB_TYPE", raising=False)
    yaml_cfg = {
        "database": {
            "active": "mysql",
            "mysql": {"host": "1.2.3.4", "port": 3306, "user": "mysql_user", "password": "mysql_pass", "name": "mysql_db"},
            "oceanbase": {"scheme": "oceanbase", "config": {"user": "ob_user", "password": "ob_pass", "host": "ob_host", "port": 2882, "db_name": "ob_db"}}
        }
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    ob_cfg = cfg.database.oceanbase.config
    assert cfg.database.oceanbase.scheme == "oceanbase"
    assert ob_cfg.user == "ob_user"
    assert ob_cfg.password == "ob_pass"
    assert ob_cfg.host == "ob_host"
    assert ob_cfg.port == 2882
    assert ob_cfg.db_name == "ob_db"


# ------------------------
# Current property
# ------------------------

def test_database_current_property(monkeypatch):
    """DatabaseConfig.current returns correct active database config."""
    monkeypatch.delenv("DB_TYPE", raising=False)
    with patch("core.config.app.load_yaml", side_effect=[{}, {}]):
        cfg = AppConfig()

    db = cfg.database
    assert db.current is db.mysql

    db.active = DatabaseType.POSTGRES
    assert db.current is db.postgres

    db.active = DatabaseType.OCEANBASE
    assert db.current is db.oceanbase


# ------------------------
# Optional: test connection_params reflects YAML overrides
# ------------------------

def test_mysql_connection_params_yaml_override():
    """MySQL connection_params reflects YAML username/password."""
    yaml_cfg = {"database": {"mysql": {"user": "yaml_user", "password": "yaml_pass"}}}
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    mysql_cfg = cfg.database.mysql
    conn = {
        "host": mysql_cfg.host,
        "port": mysql_cfg.port,
        "db": mysql_cfg.name,
        "username": mysql_cfg.user,
        "password": mysql_cfg.password
    }
    assert conn["username"] == "yaml_user"
    assert conn["password"] == "yaml_pass"

