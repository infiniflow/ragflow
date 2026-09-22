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
from unittest.mock import MagicMock, patch

def test_doc_engine_vastbase_flag(monkeypatch):
    monkeypatch.setenv("DOC_ENGINE", "vastbase")
    from common import settings

    assert settings.DOC_ENGINE_VASTBASE is True
    assert settings.DOC_ENGINE_GAUSSDB is False


def test_init_settings_wires_vastbase_doc_and_message_store(monkeypatch):
    monkeypatch.setenv("DOC_ENGINE", "vastbase")
    monkeypatch.setenv("STORAGE_IMPL", "MINIO")

    fake_doc = MagicMock(name="VBConnectionDoc")
    fake_msg = MagicMock(name="VBConnectionMsg")

    with (
        patch("common.settings.get_base_config", return_value={"host": "vb.local", "port": 5432, "db_name": "ragflow"}),
        patch("rag.utils.vastbase_conn.VBConnection", return_value=fake_doc),
        patch("memory.utils.vastbase_conn.VBConnection", return_value=fake_msg),
        patch("common.settings.decrypt_database_config", return_value={}),
        patch("common.settings.StorageFactory.create", return_value=MagicMock()),
    ):
        from common import settings

        settings.init_settings()

    assert settings.docStoreConn is fake_doc
    assert settings.msgStoreConn is fake_msg
    assert settings.VB["host"] == "vb.local"
