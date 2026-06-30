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

import pytest
from unittest.mock import patch
from core.config.app import AppConfig
from core.config.types import DocumentEngineType


# ------------------------
# Defaults
# ------------------------

def test_doc_engine_defaults(monkeypatch):
    """Test default doc engine is Elasticsearch with correct default fields."""
    monkeypatch.delenv("DOC_ENGINE", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    es_cfg = cfg.doc_engine.elasticsearch
    assert cfg.doc_engine.active == DocumentEngineType.ELASTICSEARCH
    assert es_cfg.hosts == ["http://localhost:1200"]
    assert es_cfg.username == "elastic"
    assert es_cfg.password is None
    assert es_cfg.verify_certs is False


def test_elasticsearch_defaults(monkeypatch):
    """Explicitly test Elasticsearch defaults when DOC_ENGINE=elasticsearch."""
    monkeypatch.setenv("DOC_ENGINE", "elasticsearch")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    es_cfg = cfg.doc_engine.elasticsearch
    assert es_cfg.hosts == ["http://localhost:1200"]
    assert es_cfg.username == "elastic"
    assert es_cfg.password is None
    assert es_cfg.verify_certs is False


def test_opensearch_defaults(monkeypatch):
    """Explicitly test OpenSearch defaults when DOC_ENGINE=opensearch."""
    monkeypatch.setenv("DOC_ENGINE", "opensearch")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    os_cfg = cfg.doc_engine.opensearch
    assert os_cfg.hosts == ["http://localhost:1201"]
    assert os_cfg.username == "admin"
    assert os_cfg.password is None


def test_infinity_override(monkeypatch):
    """Test Infinity config loaded from YAML overrides and parses host:port correctly."""
    monkeypatch.delenv("DOC_ENGINE", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {"doc_engine": {"active": "infinity", "infinity": {"uri": "host:9999"}}},
            {}
        ]
        cfg = AppConfig()

    infinity_cfg = cfg.doc_engine.infinity
    assert infinity_cfg.host == "host"
    assert infinity_cfg.http_port == 9999


# ------------------------
# Active type handling
# ------------------------

@pytest.mark.parametrize(
    "engine_type, enum_val",
    [
        ("elasticsearch", DocumentEngineType.ELASTICSEARCH),
        ("opensearch", DocumentEngineType.OPENSEARCH),
        ("infinity", DocumentEngineType.INFINITY),
    ]
)
def test_doc_engine_active_types(engine_type, enum_val, monkeypatch):
    """Test DOC_ENGINE environment variable correctly maps to enum values."""
    monkeypatch.setenv("DOC_ENGINE", engine_type)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    assert cfg.doc_engine.active == enum_val


# ------------------------
# Current property
# ------------------------

def test_doc_engine_current(monkeypatch):
    """Test doc_engine.current returns the correct engine instance."""
    monkeypatch.setenv("DOC_ENGINE", "opensearch")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    current = cfg.doc_engine.current
    assert current == cfg.doc_engine.opensearch


# ------------------------
# Invalid engine
# ------------------------

def test_doc_engine_invalid(monkeypatch):
    """Test that an unknown engine type raises a ValueError."""
    monkeypatch.delenv("DOC_ENGINE", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {"doc_engine": {"active": "unknown_engine"}},
            {}
        ]
        with pytest.raises(ValueError):
            AppConfig()


# ------------------------
# YAML vs Env priority
# ------------------------

def test_yaml_overrides_env(monkeypatch):
    """Test that YAML values take priority over environment variables."""
    monkeypatch.setenv("DOC_ENGINE", "elasticsearch")
    yaml_cfg = {
        "doc_engine": {"active": "opensearch"}  # YAML says OpenSearch
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    # YAML should win over environment variable
    assert cfg.doc_engine.active == DocumentEngineType.OPENSEARCH


def test_env_applied_if_yaml_missing(monkeypatch):
    """Test that environment variable applies only if YAML does not specify the field."""
    monkeypatch.setenv("DOC_ENGINE", "infinity")
    yaml_cfg = {}  # YAML does not specify doc_engine
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    # Env variable should be applied because YAML is missing the field
    assert cfg.doc_engine.active == DocumentEngineType.INFINITY


def test_infinity_host_port_parsing(monkeypatch):
    """Test Infinity URI parsing when both YAML and env are used."""
    monkeypatch.setenv("INFINITY_URI", "envhost:8888")
    yaml_cfg = {"doc_engine": {"active": "infinity", "infinity": {"uri": "yamlhost:9999"}}}
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    # YAML should take priority for uri, host, and http_port
    infinity_cfg = cfg.doc_engine.infinity
    assert infinity_cfg.uri == "yamlhost:9999"
    assert infinity_cfg.host == "yamlhost"
    assert infinity_cfg.http_port == 9999
