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

from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from types import ModuleType
import pytest


_CONFIG_PATH = Path(__file__).parents[3] / "common" / "data_source" / "config.py"


def _load_config_module() -> ModuleType:
    """Load the config module so each call reads the current environment."""
    spec = spec_from_file_location("config_under_test", _CONFIG_PATH)
    assert spec is not None
    assert spec.loader is not None

    module = module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_blob_storage_size_threshold_defaults_to_20_mib_when_environment_variable_is_unset(
    monkeypatch,
):
    """Use the 20 MiB default when the environment variable is unset."""
    monkeypatch.delenv("BLOB_STORAGE_SIZE_THRESHOLD", raising=False)

    config_module = _load_config_module()

    assert config_module.BLOB_STORAGE_SIZE_THRESHOLD == 20 * 1024 * 1024


def test_blob_storage_size_threshold_uses_environment_variable_when_set(
    monkeypatch,
):
    """Use the configured threshold when the environment variable is set."""
    configured_threshold = 50 * 1024 * 1024
    monkeypatch.setenv(
        "BLOB_STORAGE_SIZE_THRESHOLD",
        str(configured_threshold),
    )

    config_module = _load_config_module()

    assert config_module.BLOB_STORAGE_SIZE_THRESHOLD == configured_threshold


def test_blob_storage_size_threshold_raises_clear_error_for_invalid_environment_value(
    monkeypatch,
):
    """Raise an actionable error when the configured threshold is not an integer."""
    monkeypatch.setenv("BLOB_STORAGE_SIZE_THRESHOLD", "20MB")

    with pytest.raises(
        ValueError,
        match="BLOB_STORAGE_SIZE_THRESHOLD must be an integer number of bytes",
    ):
        _load_config_module()
