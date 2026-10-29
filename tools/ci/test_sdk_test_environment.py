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

"""Service-free checks for isolated CI provisioning and secret-safe errors."""

from unittest.mock import Mock

import pytest

from tools.ci import sdk_test_environment as setup


@pytest.fixture(autouse=True)
def ci_environment(monkeypatch):
    monkeypatch.setenv("CI", "true")
    monkeypatch.setenv("HOST_ADDRESS", "http://sdk.invalid")
    monkeypatch.setenv("DOC_ENGINE", "infinity")


@pytest.mark.parametrize("key,value", [("CI", "false"), ("HOST_ADDRESS", ""), ("DOC_ENGINE", "unknown")])
def test_refuses_provisioning_without_explicit_ci_stack(monkeypatch, key, value):
    monkeypatch.setenv(key, value)
    tenant = Mock()
    monkeypatch.setattr(setup, "_tenant", tenant)
    with pytest.raises(RuntimeError, match="requires CI=true"):
        setup.prepare_environment("test_agent_management")
    tenant.assert_not_called()


def test_requires_credentials_before_mutating_backend(monkeypatch):
    for name in ("ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY"):
        monkeypatch.delenv(name, raising=False)
        monkeypatch.delenv("SDK_" + name, raising=False)
    tenant = Mock()
    monkeypatch.setattr(setup, "_tenant", tenant)
    with pytest.raises(RuntimeError, match="require credentials"):
        setup.prepare_environment("test_agent_management")
    tenant.assert_not_called()


@pytest.mark.parametrize("folder,pipeline", [("test_agent_management", False), ("test_dataset_mangement", True)])
def test_provisions_fresh_tenants_and_pipeline_only_for_datasets(monkeypatch, folder, pipeline):
    for name in ("ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY"):
        monkeypatch.setenv(name, "runner-key")
        monkeypatch.setenv("SDK_" + name, "secret-key")
    monkeypatch.setenv("HOST_ADDRESS", "http://sdk.invalid/")
    monkeypatch.setenv("DOC_ENGINE", "infinity")
    api, other = Mock(), Mock()
    api.call.side_effect = [({"dsl": {"components": {}}}, {}), ({"id": "pipeline"}, {}), ({"id": "version"}, {})]
    tenant = Mock(side_effect=[(api, "primary", "primary-key"), (other, "other", "other-key")])
    models = Mock()
    monkeypatch.setattr(setup, "_tenant", tenant)
    monkeypatch.setattr(setup, "_models", models)
    env = setup.prepare_environment(folder)
    assert tenant.call_count == 2
    assert env["RAGFLOW_SDK_TENANT_ID"] == "primary"
    assert env["RAGFLOW_SDK_OTHER_TENANT_ID"] == "other"
    assert env["RAGFLOW_SDK_OTHER_API_KEY"] == "other-key"
    assert env["ZHIPU_AI_API_KEY"] == "secret-key"
    assert env["RAGFLOW_SDK_DOC_ENGINE"] == "infinity"
    models.assert_called_once_with(api, {"ZHIPU_AI_API_KEY": "secret-key", "SILICONFLOW_API_KEY": "secret-key"})
    if pipeline:
        assert env["RAGFLOW_SDK_PIPELINE_ID"] == "pipeline"
        assert env["RAGFLOW_SDK_PIPELINE_VERSION_ID"] == "version"
        assert api.call.call_args_list[1].args[2]["canvas_category"] == "dataflow_canvas"
    else:
        assert "RAGFLOW_SDK_PIPELINE_ID" not in env
        api.call.assert_not_called()
    api.close.assert_called_once()
    other.close.assert_called_once()


def test_http_errors_never_include_provider_response(monkeypatch):
    api = setup._API("http://sdk.invalid")
    response = Mock(status_code=400)
    response.json.return_value = {"code": 100, "message": "provider echoed secret-key"}
    monkeypatch.setattr(api.session, "request", Mock(return_value=response))
    with pytest.raises(RuntimeError) as error:
        api.call("POST", "/providers/ZHIPU-AI/instances", {"api_key": "secret-key"})
    assert "secret-key" not in str(error.value)
    assert "HTTP 400, API 100" in str(error.value)
