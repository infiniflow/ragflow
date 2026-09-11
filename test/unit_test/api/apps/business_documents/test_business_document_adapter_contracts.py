#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#

"""Behavioral contracts for the narrow Business Documents upstream adapters."""

from __future__ import annotations

import asyncio
import json
from pathlib import Path
import sys
from types import ModuleType

import common
import pytest


if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents import ai as ai_module
from api.apps.business_documents.adapters.storage import BusinessDocumentStorageAdapter, StorageRemovalVerificationError
from api.apps.business_documents.ai import BusinessDocumentAI, RAGFlowLLMAdapter
from api.apps.business_documents.evidence import BusinessDocumentEvidence, RAGFlowDatasetSearchAdapter
from api.apps.business_documents.exports import BusinessDocumentExportService


@pytest.mark.p1
@pytest.mark.parametrize(("task_type", "expected_limit"), [("GENERATE_DRAFT", 8192), ("ASSESS_INTAKE", 4096)])
def test_llm_adapter_preserves_tenant_prompt_payload_and_retry_contract(monkeypatch, task_type, expected_limit):
    lookup_calls = []
    bundle_calls = []
    chat_calls = []
    drain_calls = []
    model_config = {"model_name": "configured-chat"}

    tenant_model_module = ModuleType("api.db.joint_services.tenant_model_service")

    def get_tenant_default_model_by_type(tenant_id, model_type):
        lookup_calls.append((tenant_id, model_type))
        return model_config

    tenant_model_module.get_tenant_default_model_by_type = get_tenant_default_model_by_type
    monkeypatch.setitem(sys.modules, tenant_model_module.__name__, tenant_model_module)

    llm_service_module = ModuleType("api.db.services.llm_service")

    class FakeBundle:
        def __init__(self, *args, **kwargs):
            bundle_calls.append((args, kwargs))

        def __enter__(self):
            return self

        def __exit__(self, *_args):
            return False

        async def async_chat(self, system_prompt, messages, options):
            chat_calls.append((system_prompt, messages, options))
            return "adapter-result"

    llm_service_module.LLMBundle = FakeBundle
    monkeypatch.setitem(sys.modules, llm_service_module.__name__, llm_service_module)

    async def drain_litellm_callbacks():
        drain_calls.append(True)

    monkeypatch.setattr(ai_module, "_drain_litellm_callbacks", drain_litellm_callbacks)

    payload = {"job_input": {"task_type": task_type}, "document": {"title": "Регламент"}}
    result = RAGFlowLLMAdapter().generate("tenant-1", "system contract", payload)

    assert result == "adapter-result"
    assert lookup_calls[0][0] == "tenant-1"
    assert getattr(lookup_calls[0][1], "value", lookup_calls[0][1]) == "chat"
    assert bundle_calls == [(("tenant-1", model_config), {"lang": "Russian", "max_retries": 0})]
    assert len(chat_calls) == 1
    system_prompt, messages, options = chat_calls[0]
    assert system_prompt == "system contract"
    assert messages[0]["role"] == "user"
    assert json.loads(messages[0]["content"]) == payload
    assert options == {"temperature": 0, "top_p": 0.1, "max_completion_tokens": expected_limit}
    assert drain_calls == [True]


@pytest.mark.p1
def test_dataset_search_adapter_forwards_actor_request_and_result(monkeypatch):
    calls = []
    expected = (True, {"chunks": [{"id": "chunk-1"}]})
    service_module = ModuleType("api.apps.services.dataset_api_service")

    async def search_datasets(actor_id, request):
        calls.append((actor_id, request))
        return expected

    service_module.search_datasets = search_datasets
    monkeypatch.setitem(sys.modules, service_module.__name__, service_module)
    request = {"dataset_ids": ["dataset-1"], "question": "Как?"}

    assert RAGFlowDatasetSearchAdapter().search("actor-1", request) == expected
    assert calls == [("actor-1", request)]


@pytest.mark.p1
def test_default_and_injected_adapters_keep_one_production_boundary(monkeypatch):
    custom_llm = object()
    custom_search = object()

    class RawStorage:
        @staticmethod
        def put(_bucket, _key, _content):
            return "stored"

        @staticmethod
        def get(_bucket, _key):
            return b"content"

    raw_storage = RawStorage()
    settings_module = ModuleType("common.settings")
    settings_module.STORAGE_IMPL = raw_storage
    settings_module.STORAGE_IMPL_TYPE = "MINIO"
    monkeypatch.setitem(sys.modules, settings_module.__name__, settings_module)
    monkeypatch.setattr(common, "settings", settings_module)

    assert isinstance(BusinessDocumentAI()._adapter, RAGFlowLLMAdapter)
    assert BusinessDocumentAI(custom_llm)._adapter is custom_llm
    assert isinstance(BusinessDocumentEvidence()._search_adapter, RAGFlowDatasetSearchAdapter)
    assert BusinessDocumentEvidence(custom_search)._search_adapter is custom_search
    storage = BusinessDocumentExportService._default_storage()
    assert isinstance(storage, BusinessDocumentStorageAdapter)
    assert storage.put("bucket", "key", b"content") == "stored"
    assert storage.get("bucket", "key") == b"content"


@pytest.mark.p1
def test_litellm_callback_drain_does_not_stop_or_clear_the_global_worker(monkeypatch):
    calls = []

    class FakeWorker:
        _running_tasks = set()

        async def flush(self):
            calls.append("flush")

        async def stop(self):
            raise AssertionError("the process-global worker must not be stopped")

        async def clear_queue(self):
            raise AssertionError("callbacks from other requests must not be cleared")

    logging_worker = ModuleType("litellm.litellm_core_utils.logging_worker")
    logging_worker.GLOBAL_LOGGING_WORKER = FakeWorker()
    monkeypatch.setitem(sys.modules, logging_worker.__name__, logging_worker)

    asyncio.run(ai_module._drain_litellm_callbacks())

    assert calls == ["flush"]


@pytest.mark.p0
def test_storage_adapter_verifies_supported_backend_absence_without_compatibility_probes():
    class MissingError(RuntimeError):
        def __init__(self, *, code=None, error_code=None, status_code=None, response=None):
            super().__init__("missing")
            self.code = code
            self.error_code = error_code
            self.status_code = status_code
            self.response = response

    class HeadClient:
        def __init__(self, missing_error):
            self.missing_error = missing_error
            self.deleted = []

        def remove_object(self, bucket, key):
            self.deleted.append((bucket, key))

        def stat_object(self, _bucket, _key):
            raise self.missing_error

        def delete_object(self, *, Bucket, Key):
            self.deleted.append((Bucket, Key))

        def head_object(self, *, Bucket, Key):
            raise self.missing_error

    minio_client = HeadClient(MissingError(code="NoSuchKey"))

    class MinioStorage:
        conn = minio_client

        @staticmethod
        def _resolve_bucket_and_path(bucket, key):
            return "physical-minio", f"prefix/{bucket}/{key}"

        def rm(self, *_args):
            raise AssertionError("compatibility rm must not be used")

        def obj_exist(self, *_args):
            raise AssertionError("compatibility obj_exist must not be used")

        def get(self, *_args):
            raise AssertionError("compatibility get must not be used")

        def health(self):
            raise AssertionError("generic health must not be used")

    class EncryptedStorageWrapper:
        def __init__(self, storage_impl):
            self.storage_impl = storage_impl

    EncryptedStorageWrapper.__module__ = "rag.utils.encrypted_storage"
    minio_storage = EncryptedStorageWrapper(MinioStorage())
    minio = BusinessDocumentStorageAdapter(minio_storage, "minio")
    assert minio.remove_and_confirm_absent("tenant", "object") is True
    assert minio_client.deleted == [("physical-minio", "prefix/tenant/object")]

    class DelegateStorage:
        def __init__(self):
            self.objects = {}

        def put(self, bucket, key, content):
            self.objects[(bucket, key)] = content
            return "stored"

        def get(self, bucket, key):
            return self.objects.get((bucket, key))

    delegate_storage = DelegateStorage()
    delegate = BusinessDocumentStorageAdapter(delegate_storage, "UNKNOWN")
    assert delegate.put("tenant", "object", b"content") == "stored"
    assert delegate.get("tenant", "object") == b"content"

    s3_client = HeadClient(MissingError(response={"Error": {"Code": "404"}}))

    class S3Storage:
        conn = [s3_client]

        @staticmethod
        def _resolve_path(bucket, key):
            return "physical-s3", f"prefix/{bucket}/{key}"

    assert BusinessDocumentStorageAdapter(S3Storage(), "AWS_S3").remove_and_confirm_absent("tenant", "object") is True
    assert s3_client.deleted == [("physical-s3", "prefix/tenant/object")]

    oss_client = HeadClient(MissingError(response={"ResponseMetadata": {"HTTPStatusCode": 404}}))

    class OssStorage:
        bucket = "physical-oss"
        prefix_path = "prefix"
        conn = oss_client

    assert BusinessDocumentStorageAdapter(OssStorage(), "OSS").remove_and_confirm_absent("tenant", "object") is True
    assert oss_client.deleted == [("physical-oss", "prefix/object")]

    class BooleanBlob:
        def __init__(self):
            self.deleted = False

        def delete(self):
            self.deleted = True

        def exists(self):
            return False

    blob = BooleanBlob()

    class Bucket:
        @staticmethod
        def blob(path):
            assert path == "tenant/object"
            return blob

    class GcsClient:
        @staticmethod
        def bucket(name):
            assert name == "physical-gcs"
            return Bucket()

    class GcsStorage:
        client = GcsClient()
        bucket_name = "physical-gcs"

        @staticmethod
        def _get_blob_path(bucket, key):
            return f"{bucket}/{key}"

    assert BusinessDocumentStorageAdapter(GcsStorage(), "GCS").remove_and_confirm_absent("tenant", "object") is True
    assert blob.deleted is True

    class MissingProperties:
        def __init__(self, error):
            self.error = error
            self.deleted = False

        def delete_blob(self):
            self.deleted = True

        def get_blob_properties(self):
            raise self.error

        def get_file_properties(self):
            raise self.error

    spn_file = MissingProperties(MissingError(status_code=404))

    class SpnClient:
        deleted = []

        @classmethod
        def delete_file(cls, path):
            cls.deleted.append(path)

        @staticmethod
        def get_file_client(path):
            assert path == "tenant/object"
            return spn_file

    class SpnStorage:
        conn = SpnClient()

    assert BusinessDocumentStorageAdapter(SpnStorage(), "AZURE_SPN").remove_and_confirm_absent("tenant", "object") is True
    assert SpnClient.deleted == ["tenant/object"]

    sas_blob = MissingProperties(MissingError(error_code="BlobNotFound"))

    class SasClient:
        @staticmethod
        def get_blob_client(path):
            assert path == "tenant/object"
            return sas_blob

    class SasStorage:
        conn = SasClient()

    assert BusinessDocumentStorageAdapter(SasStorage(), "AZURE_SAS").remove_and_confirm_absent("tenant", "object") is True
    assert sas_blob.deleted is True

    class Operator:
        deleted = []

        @classmethod
        def delete(cls, path):
            cls.deleted.append(path)

        @staticmethod
        def exists(path):
            assert path == "tenant/object"
            return False

    class OpenDalStorage:
        _operator = Operator()

    assert BusinessDocumentStorageAdapter(OpenDalStorage(), "OPENDAL").remove_and_confirm_absent("tenant", "object") is True
    assert Operator.deleted == ["tenant/object"]


@pytest.mark.p0
def test_storage_adapter_fails_closed_on_unknown_backend_and_storage_errors():
    class BrokenClient:
        @staticmethod
        def remove_object(_bucket, _key):
            raise RuntimeError("storage unavailable")

    class BrokenMinio:
        conn = BrokenClient()

        @staticmethod
        def _resolve_bucket_and_path(bucket, key):
            return bucket, key

    with pytest.raises(RuntimeError, match="storage unavailable"):
        BusinessDocumentStorageAdapter(BrokenMinio(), "MINIO").remove_and_confirm_absent("tenant", "object")

    class PresentClient:
        @staticmethod
        def remove_object(_bucket, _key):
            return None

        @staticmethod
        def stat_object(_bucket, _key):
            return object()

    class PresentMinio:
        conn = PresentClient()

        @staticmethod
        def _resolve_bucket_and_path(bucket, key):
            return bucket, key

    assert BusinessDocumentStorageAdapter(PresentMinio(), "MINIO").remove_and_confirm_absent("tenant", "object") is False
    with pytest.raises(StorageRemovalVerificationError, match="Unsupported storage backend"):
        BusinessDocumentStorageAdapter(object(), "UNKNOWN").remove_and_confirm_absent("tenant", "object")
