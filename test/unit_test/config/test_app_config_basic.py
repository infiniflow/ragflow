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

import pytest

from core.config.app import AppConfig
from core.config.env_overrides import ENV_OVERRIDES
from core.config.utils.pydantic import get_field_value
from core.config.types import CacheType, DatabaseType, DocumentEngineType, ObjectStorageType


@pytest.mark.parametrize(
    "env_name, env_value, expected_attr, expected_value",
    [
        ("DB_TYPE", "postgres", "database.active", DatabaseType.POSTGRES),
        ("STORAGE_IMPL", "s3", "storage.active", ObjectStorageType.S3),
        ("DOC_ENGINE", "infinity", "doc_engine.active", DocumentEngineType.INFINITY),
        ("CACHE_TYPE", "redis", "cache.active", CacheType.REDIS),
    ]
)
def test_env_override_param(monkeypatch, env_name, env_value, expected_attr, expected_value):
    monkeypatch.setenv(env_name, env_value)
    config = AppConfig()
    attr = expected_attr.split(".")
    val = config
    for a in attr:
        val = getattr(val, a)
    assert val == expected_value


def test_yaml_overrides_defaults():
    with patch("core.config.app.load_yaml", return_value={"database": {"active": "postgres"}}):
        config = AppConfig()
    assert config.database.active == DatabaseType.POSTGRES


def test_yaml_priority_over_env(monkeypatch):
    """
    YAML values should take precedence over environment variables.
    """
    monkeypatch.setenv("DB_TYPE", "mysql")  # Env would normally override
    yaml_data = {"database": {"active": "postgres"}}  # YAML should win
    with patch("core.config.app.load_yaml", return_value=yaml_data):
        config = AppConfig()
    assert config.database.active == DatabaseType.POSTGRES


def test_env_overrides_paths_exist():
    """
    Ensure all ENV_OVERRIDES keys point to existing fields in AppConfig.
    """
    cfg = AppConfig()

    for field_path, env_var in ENV_OVERRIDES.items():
        parts = field_path.split(".")
        current_model = cfg
        for i, part in enumerate(parts):
            # Only the last part should be retrieved as a value
            if i == len(parts) - 1:
                # Use get_field_value to support alias
                try:
                    get_field_value(current_model, part)
                except AttributeError:
                    pytest.fail(
                        f"Field '{part}' (from path '{field_path}') not found "
                        f"in {current_model.__class__.__name__}"
                    )
            else:
                # Drill down to nested model
                try:
                    current_model = get_field_value(current_model, part)
                except AttributeError:
                    pytest.fail(
                        f"Nested field '{part}' (from path '{field_path}') not found "
                        f"in {current_model.__class__.__name__}"
                    )
