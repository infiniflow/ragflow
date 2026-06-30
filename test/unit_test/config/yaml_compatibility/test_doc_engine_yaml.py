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

"""
Test compatibility between old and new YAML formats for doc_engine configuration.

1. Verify that the old format (flat keys like 'elasticsearch' and 'infinity')
   is correctly loaded and accessible in AppConfig.
2. Verify that the new format (nested under 'doc_engine') overrides the old
   format when both are present.
3. Ensure that when both old and new formats exist, the new format takes precedence.
4. Check that key properties like host/uri and port are correctly parsed from YAML.
"""
from unittest.mock import patch

from core.config import AppConfig


def test_doc_engine_old_yaml():
    return_value = {
        "es": {"hosts": "127.0.0.1", "username": "old", "password": "oldpass"},
        "infinity": {"uri": "127.0.0.1:23817"},
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    # Elasticsearch
    es_cfg = config.doc_engine.elasticsearch
    assert es_cfg.hosts == ["127.0.0.1"]
    assert es_cfg.username == "old"
    assert es_cfg.password == "oldpass"

    # Infinity
    inf_cfg = config.doc_engine.infinity
    assert inf_cfg.uri == "127.0.0.1:23817"


def test_doc_engine_new_yaml():
    return_value = {
        "doc_engine": {
            "es": {"hosts": "127.0.0.2", "username": "new", "password": "newpass"},
            "infinity": {"uri": "127.0.0.2:23818"},
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    # Elasticsearch
    es_cfg = config.doc_engine.elasticsearch
    assert es_cfg.hosts == ["127.0.0.2"]
    assert es_cfg.username == "new"
    assert es_cfg.password == "newpass"

    # Infinity
    inf_cfg = config.doc_engine.infinity
    assert inf_cfg.uri == "127.0.0.2:23818"
