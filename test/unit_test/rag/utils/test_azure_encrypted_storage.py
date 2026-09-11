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

"""Azure adapters must accept the tenant argument forwarded by encrypted storage."""

import importlib.util
import sys
from io import BytesIO
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock

import pytest
from azure.storage.blob import ContainerClient
from azure.storage.filedatalake import FileSystemClient

import common
from rag.utils.encrypted_storage import EncryptedStorageWrapper

pytestmark = pytest.mark.p2


@pytest.fixture(params=[("azure_sas_conn", "RAGFlowAzureSasBlob", ContainerClient), ("azure_spn_conn", "RAGFlowAzureSpnBlob", FileSystemClient)])
def storage(request, monkeypatch):
    """Load a fresh singleton without loading service settings or credentials."""
    name, factory, client_type = request.param
    settings = ModuleType("common.settings")
    settings.AZURE = {}
    monkeypatch.setitem(sys.modules, "common.settings", settings)
    monkeypatch.setattr(common, "settings", settings, raising=False)
    path = Path(__file__).resolve().parents[4] / "rag" / "utils" / (name + ".py")
    spec = importlib.util.spec_from_file_location("test_" + name, path)
    module = importlib.util.module_from_spec(spec)
    # Bypass only singleton/connection setup; keep all adapter method bodies.
    with monkeypatch.context() as load_patch:
        load_patch.setattr("common.decorator.singleton", lambda cls: cls)
        spec.loader.exec_module(module)
    cls = getattr(module, factory)
    adapter = cls.__new__(cls)
    adapter.conn = Mock(spec=client_type)
    return adapter, name


@pytest.mark.parametrize("tenant_id", [None, "tenant-a"])
def test_encrypted_get_decrypts_backend_bytes(storage, tenant_id):
    adapter, name = storage
    wrapper = EncryptedStorageWrapper(adapter, key="test-key")
    encrypted = wrapper.crypto.encrypt(b"document contents")
    if name == "azure_sas_conn":
        adapter.conn.download_blob.return_value = BytesIO(encrypted)
    else:
        adapter.conn.get_file_client.return_value.download_file.return_value = BytesIO(encrypted)
    assert wrapper.get("kb", "file.txt", tenant_id) == b"document contents"


@pytest.mark.parametrize("tenant_id", [None, "tenant-a"])
def test_encrypted_rm_deletes_same_object(storage, tenant_id):
    adapter, name = storage
    wrapper = EncryptedStorageWrapper(adapter, key="test-key")
    wrapper.rm("kb", "file.txt", tenant_id)
    delete = adapter.conn.delete_blob if name == "azure_sas_conn" else adapter.conn.delete_file
    delete.assert_called_once_with("kb/file.txt")


@pytest.mark.parametrize("tenant_id", [None, "tenant-a"])
@pytest.mark.parametrize("exists", [False, True])
def test_encrypted_obj_exist_preserves_backend_result(storage, tenant_id, exists):
    adapter, name = storage
    if name == "azure_sas_conn":
        adapter.conn.get_blob_client.return_value.exists.return_value = exists
    else:
        adapter.conn.get_file_client.return_value.exists.return_value = exists
    # The separate SPN get_blob_client defect remains out of scope: the real
    # FileSystemClient spec rejects it, and the adapter currently returns False.
    expected = adapter.obj_exist("kb", "file.txt")
    wrapper = EncryptedStorageWrapper(adapter, key="test-key")
    assert wrapper.obj_exist("kb", "file.txt", tenant_id) is expected


def test_direct_get_and_rm_remain_callable_without_tenant(storage):
    adapter, name = storage
    if name == "azure_sas_conn":
        adapter.conn.download_blob.return_value = BytesIO(b"plain")
    else:
        adapter.conn.get_file_client.return_value.download_file.return_value = BytesIO(b"plain")
    assert adapter.get("kb", "file.txt") == b"plain"
    adapter.rm("kb", "file.txt")
