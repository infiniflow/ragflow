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


_CONFIG_PATH = Path(__file__).parents[3] / "common" / "data_source" / "config.py"


def _load_config():
    spec = spec_from_file_location("config_under_test", _CONFIG_PATH)
    assert spec is not None
    assert spec.loader is not None

    module = module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_blob_storage_size_threshold_uses_default(monkeypatch):
    monkeypatch.delenv("BLOB_STORAGE_SIZE_THRESHOLD", raising=False)

    config = _load_config()

    assert config.BLOB_STORAGE_SIZE_THRESHOLD == 20 * 1024 * 1024


def test_blob_storage_size_threshold_uses_environment_variable(monkeypatch):
    monkeypatch.setenv("BLOB_STORAGE_SIZE_THRESHOLD", "52428800")

    config = _load_config()

    assert config.BLOB_STORAGE_SIZE_THRESHOLD == 52_428_800
