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
"""
Unit tests for Azure Blob storage path construction (issue #14159).

Both AzureSpn and AzureSas implementations must prepend the bucket
parameter to file paths so that files with the same name from different
datasets do not overwrite each other in flat blob storage.
"""
import importlib
import sys
import types
from unittest.mock import MagicMock

import pytest


def _install_stubs():
    """Replace heavyweight runtime modules so the connection modules can be
    imported in isolation without the full ragflow runtime or the real
    `azure` SDK being installed."""

    decorator_mod = types.ModuleType("common.decorator")
    decorator_mod.singleton = lambda cls: cls

    settings_mod = types.ModuleType("common.settings")
    settings_mod.AZURE = {
        "account_url": "https://example.dfs.core.windows.net",
        "client_id": "x",
        "secret": "x",
        "tenant_id": "x",
        "container_name": "c",
        "cloud": "public",
        "container_url": "https://example.blob.core.windows.net/c",
        "sas_token": "sig=x",
    }

    common_pkg = types.ModuleType("common")
    common_pkg.decorator = decorator_mod
    common_pkg.settings = settings_mod

    azure_pkg = types.ModuleType("azure")
    azure_identity = types.ModuleType("azure.identity")
    azure_identity.ClientSecretCredential = MagicMock()
    azure_identity.AzureAuthorityHosts = types.SimpleNamespace(
        AZURE_PUBLIC_CLOUD="public",
        AZURE_CHINA="china",
        AZURE_GOVERNMENT="gov",
        AZURE_GERMANY="de",
    )
    azure_storage = types.ModuleType("azure.storage")
    azure_fdl = types.ModuleType("azure.storage.filedatalake")
    azure_fdl.FileSystemClient = MagicMock()
    azure_blob = types.ModuleType("azure.storage.blob")
    azure_blob.ContainerClient = MagicMock()
    azure_pkg.identity = azure_identity
    azure_pkg.storage = azure_storage
    azure_storage.filedatalake = azure_fdl
    azure_storage.blob = azure_blob

    sys.modules.update({
        "common": common_pkg,
        "common.decorator": decorator_mod,
        "common.settings": settings_mod,
        "azure": azure_pkg,
        "azure.identity": azure_identity,
        "azure.storage": azure_storage,
        "azure.storage.filedatalake": azure_fdl,
        "azure.storage.blob": azure_blob,
    })


@pytest.fixture(scope="module")
def spn_module():
    _install_stubs()
    sys.modules.pop("rag.utils.azure_spn_conn", None)
    return importlib.import_module("rag.utils.azure_spn_conn")


@pytest.fixture(scope="module")
def sas_module():
    _install_stubs()
    sys.modules.pop("rag.utils.azure_sas_conn", None)
    return importlib.import_module("rag.utils.azure_sas_conn")


def _make_instance(module, cls_name):
    """Build an instance with a mocked underlying connection, bypassing
    __init__ so we don't need real Azure credentials or connectivity."""
    cls = getattr(module, cls_name)
    inst = cls.__new__(cls)
    inst.conn = MagicMock()
    return inst


class TestAzureSpnBucketPrefix:
    """RAGFlowAzureSpnBlob must include the bucket as a path prefix in all
    operations so that identical filenames from different datasets are
    isolated."""

    def test_put_uses_bucket_prefix(self, spn_module):
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.put("kb_a", "doc.pdf", b"data")
        spn.conn.create_file.assert_called_once_with("kb_a/doc.pdf")

    def test_get_uses_bucket_prefix(self, spn_module):
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.get("kb_a", "doc.pdf")
        spn.conn.get_file_client.assert_called_once_with("kb_a/doc.pdf")

    def test_rm_uses_bucket_prefix(self, spn_module):
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.rm("kb_a", "doc.pdf")
        spn.conn.delete_file.assert_called_once_with("kb_a/doc.pdf")

    def test_obj_exist_uses_bucket_prefix(self, spn_module):
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.obj_exist("kb_a", "doc.pdf")
        spn.conn.get_file_client.assert_called_once_with("kb_a/doc.pdf")

    def test_get_presigned_url_uses_bucket_prefix(self, spn_module):
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.get_presigned_url("kb_a", "doc.pdf", 3600)
        spn.conn.get_presigned_url.assert_called_once_with("GET", "kb_a/doc.pdf", 3600)

    def test_same_filename_in_different_buckets_does_not_collide(self, spn_module):
        """Regression test for issue #14159: two datasets uploading a file
        with the same name must produce two distinct storage paths."""
        spn = _make_instance(spn_module, "RAGFlowAzureSpnBlob")
        spn.put("kb_a", "report.pdf", b"data_a")
        spn.put("kb_b", "report.pdf", b"data_b")
        called_paths = [c.args[0] for c in spn.conn.create_file.call_args_list]
        assert called_paths == ["kb_a/report.pdf", "kb_b/report.pdf"]
        assert called_paths[0] != called_paths[1]


class TestAzureSasBucketPrefix:
    """Same contract for RAGFlowAzureSasBlob."""

    def test_put_uses_bucket_prefix(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.put("kb_a", "doc.pdf", b"data")
        kwargs = sas.conn.upload_blob.call_args.kwargs
        assert kwargs["name"] == "kb_a/doc.pdf"

    def test_get_uses_bucket_prefix(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.get("kb_a", "doc.pdf")
        sas.conn.download_blob.assert_called_once_with("kb_a/doc.pdf")

    def test_rm_uses_bucket_prefix(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.rm("kb_a", "doc.pdf")
        sas.conn.delete_blob.assert_called_once_with("kb_a/doc.pdf")

    def test_obj_exist_uses_bucket_prefix(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.obj_exist("kb_a", "doc.pdf")
        sas.conn.get_blob_client.assert_called_once_with("kb_a/doc.pdf")

    def test_get_presigned_url_uses_bucket_prefix(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.get_presigned_url("kb_a", "doc.pdf", 3600)
        sas.conn.get_presigned_url.assert_called_once_with("GET", "kb_a/doc.pdf", 3600)

    def test_same_filename_in_different_buckets_does_not_collide(self, sas_module):
        sas = _make_instance(sas_module, "RAGFlowAzureSasBlob")
        sas.put("kb_a", "report.pdf", b"data_a")
        sas.put("kb_b", "report.pdf", b"data_b")
        names = [c.kwargs["name"] for c in sas.conn.upload_blob.call_args_list]
        assert names == ["kb_a/report.pdf", "kb_b/report.pdf"]
        assert names[0] != names[1]
