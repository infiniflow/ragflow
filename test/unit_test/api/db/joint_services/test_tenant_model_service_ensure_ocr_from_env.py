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

"""Unit tests for OCR env auto-provisioning functions in
api.db.joint_services.tenant_model_service and TenantModelInstanceService.create_instance.

Regression tests for issue #19887:
ensure_paddleocr_from_env and related _ensure_ocr_provider_from_env functions
failed with AttributeError: 'int' object has no attribute 'id' because
TenantModelInstanceService.create_instance returned the row count int from insert()
instead of the created TenantModelInstance object.
"""

import sys
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

for mod in [
    "elastic_transport",
    "elasticsearch",
    "elasticsearch_dsl",
    "quart_auth",
    "xxhash",
    "infinity",
    "infinity.common",
    "infinity.errors",
    "rag.utils.es_conn",
    "rag.utils.infinity_conn",
    "rag.utils.ob_conn",
    "rag.utils.opensearch_conn",
    "rag.utils.gaussdb_conn",
    "rag.utils.azure_sas_conn",
    "rag.utils.azure_spn_conn",
    "rag.utils.gcs_conn",
    "rag.utils.minio_conn",
    "rag.utils.opendal_conn",
    "rag.utils.redis_conn",
    "rag.utils.s3_conn",
    "rag.utils.oss_conn",
    "rag.nlp",
    "rag.nlp.search",
    "memory",
    "memory.utils",
    "memory.utils.es_conn",
    "memory.utils.infinity_conn",
    "memory.utils.ob_conn",
    "memory.utils.gaussdb_conn",
]:
    if mod not in sys.modules:
        m = ModuleType(mod)
        m.ConnectionTimeout = type("ConnectionTimeout", (Exception,), {})
        m.AuthUser = type("AuthUser", (), {})
        m.InfinityException = type("InfinityException", (Exception,), {})
        m.SortType = type("SortType", (), {})
        m.ErrorCode = type("ErrorCode", (), {})
        m.RAGFlowAzureSasBlob = type("RAGFlowAzureSasBlob", (), {})
        m.RAGFlowAzureSpnBlob = type("RAGFlowAzureSpnBlob", (), {})
        m.RAGFlowGCS = type("RAGFlowGCS", (), {})
        m.RAGFlowMinio = type("RAGFlowMinio", (), {})
        m.OpenDALStorage = type("OpenDALStorage", (), {})
        m.REDIS_CONN = None
        m.RAGFlowS3 = type("RAGFlowS3", (), {})
        m.RAGFlowOSS = type("RAGFlowOSS", (), {})
        m.search = None
        m.xxh64_hexdigest = lambda x: "hash"
        sys.modules[mod] = m

sys.modules["memory"].utils = sys.modules["memory.utils"]
sys.modules["memory.utils"].es_conn = sys.modules["memory.utils.es_conn"]
sys.modules["memory.utils"].infinity_conn = sys.modules["memory.utils.infinity_conn"]
sys.modules["memory.utils"].ob_conn = sys.modules["memory.utils.ob_conn"]
sys.modules["memory.utils"].gaussdb_conn = sys.modules["memory.utils.gaussdb_conn"]

import contextlib
import json

import pytest

from common.constants import LLMType, ModelTypeBinary


@pytest.mark.p1
def test_tenant_model_instance_service_create_instance_returns_record(monkeypatch):
    from api.db.db_models import DB
    from api.db.services.tenant_model_instance_service import TenantModelInstanceService

    monkeypatch.setattr(DB, "connect", lambda *a, **k: None)
    monkeypatch.setattr(DB, "_connect", lambda *a, **k: MagicMock())

    created_record = SimpleNamespace(
        id="inst-uuid-123",
        provider_id="prov-1",
        instance_name="paddleocr-from-env",
        api_key='{"PADDLEOCR_BASE_URL": "http://127.0.0.1:8000"}',
        extra="{}",
    )

    inserted_args = {}

    def mock_insert(**kwargs):
        inserted_args.update(kwargs)
        return 1  # peewee save() returns int 1

    monkeypatch.setattr(TenantModelInstanceService, "insert", mock_insert)
    monkeypatch.setattr(
        TenantModelInstanceService.model,
        "get_or_none",
        lambda condition: created_record,
    )
    monkeypatch.setattr(
        "api.db.services.tenant_model_instance_service.duplicate_name",
        lambda query, name_field, provider_id, instance_name: instance_name,
    )

    result = TenantModelInstanceService.create_instance(
        provider_id="prov-1",
        instance_name="paddleocr-from-env",
        api_key='{"PADDLEOCR_BASE_URL": "http://127.0.0.1:8000"}',
        extra="{}",
    )

    assert result is not None
    assert result.id == "inst-uuid-123"
    assert result.instance_name == "paddleocr-from-env"
    assert inserted_args["provider_id"] == "prov-1"
    assert inserted_args["instance_name"] == "paddleocr-from-env"


@pytest.mark.p1
def test_ensure_paddleocr_from_env_auto_provisions_correctly(monkeypatch):
    from api.db.joint_services import tenant_model_service as tms

    mock_provider = SimpleNamespace(id="prov-paddle-1", provider_name="PaddleOCR")
    mock_instance = SimpleNamespace(id="inst-paddle-1", instance_name="paddleocr-from-env")
    mock_model = SimpleNamespace(
        id="model-paddle-1",
        model_name="paddleocr-from-env",
        provider_id="prov-paddle-1",
        instance_id="inst-paddle-1",
    )

    created_provider = None
    created_instance = None
    created_model = None

    # Provider queries
    def mock_get_provider(tenant_id, provider_name):
        return mock_provider if created_provider else None

    def mock_insert_provider(tenant_id, provider_name):
        nonlocal created_provider
        created_provider = mock_provider
        return 1

    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_tenant_id_and_provider_name", mock_get_provider)
    monkeypatch.setattr(tms.TenantModelProviderService, "insert", mock_insert_provider)

    # Instance queries
    def mock_get_instance_by_api_key(provider_id, api_key):
        return mock_instance if created_instance else None

    def mock_create_instance(provider_id, instance_name, api_key, extra):
        nonlocal created_instance
        created_instance = mock_instance
        return mock_instance

    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_provider_id_and_api_key", mock_get_instance_by_api_key)
    monkeypatch.setattr(tms.TenantModelInstanceService, "create_instance", mock_create_instance)

    # Model queries
    def mock_get_model(provider_id, instance_id, model_type, model_name):
        return mock_model if created_model else None

    def mock_insert_model(model_name, provider_id, instance_id, model_type, extra):
        nonlocal created_model
        created_model = mock_model
        return 1

    monkeypatch.setattr(
        tms.TenantModelService,
        "get_by_provider_id_and_instance_id_and_model_type_and_model_name",
        mock_get_model,
    )
    monkeypatch.setattr(tms.TenantModelService, "insert", mock_insert_model)

    monkeypatch.setenv("PADDLEOCR_BASE_URL", "http://paddleocr-server:8000")
    monkeypatch.setenv("PADDLEOCR_ALGORITHM", "PaddleOCR-VL")

    result = tms.ensure_paddleocr_from_env("tenant-123")

    assert result == "paddleocr-from-env@paddleocr-from-env@PaddleOCR"
    assert created_provider is not None
    assert created_instance is not None
    assert created_model is not None


@pytest.mark.p1
def test_ensure_mineru_from_env_auto_provisions_correctly(monkeypatch):
    from api.db.joint_services import tenant_model_service as tms

    mock_provider = SimpleNamespace(id="prov-mineru-1", provider_name="MinerU")
    mock_instance = SimpleNamespace(id="inst-mineru-1", instance_name="mineru-from-env")
    mock_model = SimpleNamespace(
        id="model-mineru-1",
        model_name="mineru-from-env",
        provider_id="prov-mineru-1",
        instance_id="inst-mineru-1",
    )

    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_tenant_id_and_provider_name", lambda *a, **k: mock_provider)
    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_provider_id_and_api_key", lambda *a, **k: None)
    monkeypatch.setattr(tms.TenantModelInstanceService, "create_instance", lambda *a, **k: mock_instance)
    monkeypatch.setattr(
        tms.TenantModelService,
        "get_by_provider_id_and_instance_id_and_model_type_and_model_name",
        lambda *a, **k: mock_model,
    )

    monkeypatch.setenv("MINERU_SERVER_URL", "http://mineru-server:8000")

    result = tms.ensure_mineru_from_env("tenant-123")

    assert result == "mineru-from-env@mineru-from-env@MinerU"


@pytest.mark.p1
def test_ensure_opendataloader_from_env_auto_provisions_correctly(monkeypatch):
    from api.db.joint_services import tenant_model_service as tms

    mock_provider = SimpleNamespace(id="prov-opendata-1", provider_name="OpenDataLoader")
    mock_instance = SimpleNamespace(id="inst-opendata-1", instance_name="opendataloader-from-env")
    mock_model = SimpleNamespace(
        id="model-opendata-1",
        model_name="opendataloader-from-env",
        provider_id="prov-opendata-1",
        instance_id="inst-opendata-1",
    )

    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_tenant_id_and_provider_name", lambda *a, **k: mock_provider)
    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_provider_id_and_api_key", lambda *a, **k: None)
    monkeypatch.setattr(tms.TenantModelInstanceService, "create_instance", lambda *a, **k: mock_instance)
    monkeypatch.setattr(
        tms.TenantModelService,
        "get_by_provider_id_and_instance_id_and_model_type_and_model_name",
        lambda *a, **k: mock_model,
    )

    monkeypatch.setenv("OPENDATALOADER_APISERVER", "http://opendata-server:8000")

    result = tms.ensure_opendataloader_from_env("tenant-123")

    assert result == "opendataloader-from-env@opendataloader-from-env@OpenDataLoader"


@pytest.mark.p1
def test_ensure_ocr_from_env_returns_none_when_no_env_configured(monkeypatch):
    from api.db.joint_services import tenant_model_service as tms

    for key in tms.PADDLEOCR_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)

    for key in tms.MINERU_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)

    for key in tms.OPENDATALOADER_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)

    assert tms.ensure_paddleocr_from_env("tenant-123") is None
    assert tms.ensure_mineru_from_env("tenant-123") is None
    assert tms.ensure_opendataloader_from_env("tenant-123") is None
