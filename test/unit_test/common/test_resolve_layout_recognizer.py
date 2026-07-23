#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#

from unittest.mock import MagicMock, patch

import pytest

from common.parser_config_utils import is_tenant_model_id, resolve_layout_recognizer


def test_is_tenant_model_id():
    assert is_tenant_model_id("5a32e0d280f111f1a6f0e50d83a56ab6")
    assert not is_tenant_model_id("MinerU@mineru@MinerU")
    assert not is_tenant_model_id("DeepDOC")


def test_resolve_layout_recognizer_legacy_suffix():
    layout, model_ref = resolve_layout_recognizer(
        None,
        "mineru-from-env@mineru@MinerU",
    )
    assert layout == "MinerU"
    assert model_ref == "mineru-from-env@mineru@MinerU"


@patch("api.db.joint_services.tenant_model_service.TenantModelService.get_by_id")
@patch("api.db.joint_services.tenant_model_service.TenantModelProviderService.get_by_id")
def test_resolve_layout_recognizer_model_id_for_mineru(provider_get_by_id, model_get_by_id):
    model_obj = MagicMock(provider_id="provider-1")
    model_get_by_id.return_value = (True, model_obj)
    provider_obj = MagicMock(provider_name="MinerU")
    provider_get_by_id.return_value = (True, provider_obj)

    layout, model_ref = resolve_layout_recognizer(
        "tenant-1",
        "5a32e0d280f111f1a6f0e50d83a56ab6",
    )
    assert layout == "MinerU"
    assert model_ref == "5a32e0d280f111f1a6f0e50d83a56ab6"


@patch("api.db.joint_services.tenant_model_service.TenantModelService.get_by_id")
@patch("api.db.joint_services.tenant_model_service.TenantModelProviderService.get_by_id")
def test_resolve_layout_recognizer_model_id_for_paddleocr(provider_get_by_id, model_get_by_id):
    model_obj = MagicMock(provider_id="provider-1")
    model_get_by_id.return_value = (True, model_obj)
    provider_obj = MagicMock(provider_name="PaddleOCR")
    provider_get_by_id.return_value = (True, provider_obj)

    layout, model_ref = resolve_layout_recognizer(
        "tenant-1",
        "cb576992867611f19e88cdd8590e17d3",
    )
    assert layout == "PaddleOCR"
    assert model_ref == "cb576992867611f19e88cdd8590e17d3"


@patch("api.db.joint_services.tenant_model_service.TenantModelService.get_by_id")
def test_resolve_layout_recognizer_missing_model_id(model_get_by_id):
    model_get_by_id.return_value = (False, None)
    with pytest.raises(LookupError, match="TenantModel id=.* not found"):
        resolve_layout_recognizer("tenant-1", "5a32e0d280f111f1a6f0e50d83a56ab6")
