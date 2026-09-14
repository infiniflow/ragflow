#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from pathlib import Path
import re
import subprocess
from unittest.mock import patch

from ruamel.yaml import YAML

ROOT = Path(__file__).resolve().parents[3]
SERVICE_CONF_TEMPLATE = ROOT / "docker" / "service_conf.yaml.template"
_PLACEHOLDER_PATTERN = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}")


def _expand_service_conf_template(template_path, output_path, environ):
    def replace_placeholder(match):
        value = environ.get(match.group(1))
        return value if value else (match.group(2) or "")

    rendered = _PLACEHOLDER_PATTERN.sub(replace_placeholder, Path(template_path).read_text(encoding="utf-8"))
    destination = Path(output_path)
    destination.write_text(rendered, encoding="utf-8")
    return destination


def _load_yaml(path):
    yaml = YAML(typ="safe", pure=True)
    return yaml.load(path.read_text(encoding="utf-8"))


def _import_gaussdb_conn_pool(monkeypatch):
    from common import settings

    monkeypatch.setattr(
        settings,
        "GAUSSDB",
        {
            "host": "gaussdb.local",
            "port": "19995",
            "database": "postgres",
            "user": "sqlbuilder",
            "password": "fake-unit-password",
        },
        raising=False,
    )
    with patch("psycopg2.pool.ThreadedConnectionPool", lambda *_args, **_kwargs: object()):
        from common.doc_store import gaussdb_conn_pool

    return gaussdb_conn_pool


def test_tc_cfg_801_docker_template_defines_complete_gaussdb_config(tmp_path):
    entrypoint = (ROOT / "docker" / "entrypoint.sh").read_text(encoding="utf-8")
    template = (ROOT / "docker" / "service_conf.yaml.template").read_text(encoding="utf-8")
    gaussdb_block = template.split("gaussdb:", 1)[1].split("seekdb:", 1)[0]

    assert 'DEF_ENV_VALUE_PATTERN="\\$\\{([^:]+):-([^}]+)\\}"' in entrypoint
    assert 'done < "${TEMPLATE_FILE}"' in entrypoint
    assert "common.service_conf_renderer" not in entrypoint
    assert "host: '${GAUSSDB_HOST}'" in gaussdb_block
    assert "port: ${GAUSSDB_PORT}" in gaussdb_block
    assert "database: '${GAUSSDB_DATABASE:-postgres}'" in gaussdb_block
    assert "user: '${GAUSSDB_USER}'" in gaussdb_block
    assert "password: '${GAUSSDB_PASSWORD}'" in gaussdb_block
    assert "schema: '${GAUSSDB_SCHEMA:-public}'" in gaussdb_block

    rendered_path = _expand_service_conf_template(
        SERVICE_CONF_TEMPLATE,
        tmp_path / "service_conf.yaml",
        {
            "GAUSSDB_HOST": "h",
            "GAUSSDB_PORT": "5432",
            "GAUSSDB_DATABASE": "d",
            "GAUSSDB_USER": "u",
            "GAUSSDB_PASSWORD": "p",
            "GAUSSDB_SCHEMA": "zlw",
        },
    )

    assert rendered_path.parent == tmp_path
    assert _load_yaml(rendered_path)["gaussdb"] == {
        "host": "h",
        "port": 5432,
        "database": "d",
        "user": "u",
        "password": "p",
        "schema": "zlw",
    }


def test_tc_cfg_802_docker_template_defaults_empty_schema_to_public(tmp_path):
    template = (ROOT / "docker" / "service_conf.yaml.template").read_text(encoding="utf-8")
    gaussdb_block = template.split("gaussdb:", 1)[1].split("seekdb:", 1)[0]
    assert "schema: '${GAUSSDB_SCHEMA:-public}'" in gaussdb_block

    rendered_path = _expand_service_conf_template(
        SERVICE_CONF_TEMPLATE,
        tmp_path / "service_conf.yaml",
        {
            "GAUSSDB_HOST": "h",
            "GAUSSDB_PORT": "5432",
            "GAUSSDB_USER": "u",
            "GAUSSDB_PASSWORD": "p",
            "GAUSSDB_SCHEMA": "",
        },
    )

    config = _load_yaml(rendered_path)["gaussdb"]
    assert config["database"] == "postgres"
    assert config["schema"] == "public"


def test_tc_cfg_804_docker_rendered_gaussdb_config_excludes_tuning_fields(tmp_path):
    template = (ROOT / "docker" / "service_conf.yaml.template").read_text(encoding="utf-8")
    gaussdb_block = template.split("gaussdb:", 1)[1].split("seekdb:", 1)[0]
    rendered_path = _expand_service_conf_template(
        SERVICE_CONF_TEMPLATE,
        tmp_path / "service_conf.yaml",
        {
            "GAUSSDB_HOST": "h",
            "GAUSSDB_PORT": "5432",
            "GAUSSDB_DATABASE": "d",
            "GAUSSDB_USER": "u",
            "GAUSSDB_PASSWORD": "p",
            "GAUSSDB_SCHEMA": "zlw",
            "GAUSSDB_SSL": "true",
            "GAUSSDB_TIMEOUT": "30",
            "GAUSSDB_POOL_MIN": "4",
            "GAUSSDB_POOL_MAX": "32",
        },
    )
    gaussdb = _load_yaml(rendered_path)["gaussdb"]

    assert "password: '${GAUSSDB_PASSWORD}'" in gaussdb_block
    assert set(gaussdb) == {"host", "port", "database", "user", "password", "schema"}
    forbidden = {"ssl", "sslmode", "timeout", "connect_timeout", "pool_min", "pool_max", "minconn", "maxconn"}
    assert forbidden.isdisjoint(gaussdb)
    assert all(f"GAUSSDB_{name.upper()}" not in gaussdb_block for name in forbidden)


def test_tc_cfg_807_rendered_service_conf_gaussdb_block_loads_as_gaussdb_config(tmp_path, monkeypatch):
    rendered_path = _expand_service_conf_template(
        SERVICE_CONF_TEMPLATE,
        tmp_path / "service_conf.yaml",
        {
            "GAUSSDB_HOST": "h",
            "GAUSSDB_PORT": "5432",
            "GAUSSDB_DATABASE": "d",
            "GAUSSDB_USER": "u",
            "GAUSSDB_PASSWORD": "p",
            "GAUSSDB_SCHEMA": "zlw",
        },
    )
    rendered = _load_yaml(rendered_path)
    pool_mod = _import_gaussdb_conn_pool(monkeypatch)

    cfg = pool_mod.load_gaussdb_config(rendered["gaussdb"])

    assert cfg.host == "h"
    assert cfg.port == 5432
    assert cfg.database == "d"
    assert cfg.user == "u"
    assert cfg.password == "p"
    assert cfg.schema == "zlw"


def test_tc_cfg_808_rendered_artifacts_stay_under_tmp_path_and_git_status_is_stable(tmp_path):
    before = subprocess.run(
        ["git", "status", "--porcelain=v1", "--untracked-files=all"],
        cwd=ROOT,
        encoding="utf-8",
        capture_output=True,
        check=True,
    ).stdout

    service_conf = _expand_service_conf_template(
        SERVICE_CONF_TEMPLATE,
        tmp_path / "service_conf.yaml",
        {
            "GAUSSDB_HOST": "gaussdb.local",
            "GAUSSDB_PORT": "19995",
            "GAUSSDB_DATABASE": "postgres",
            "GAUSSDB_USER": "ragflow",
            "GAUSSDB_PASSWORD": "secret",
            "GAUSSDB_SCHEMA": "public",
        },
    )
    assert service_conf.is_relative_to(tmp_path)
    assert _load_yaml(service_conf)["gaussdb"]["host"] == "gaussdb.local"

    after = subprocess.run(
        ["git", "status", "--porcelain=v1", "--untracked-files=all"],
        cwd=ROOT,
        encoding="utf-8",
        capture_output=True,
        check=True,
    ).stdout
    assert after == before
