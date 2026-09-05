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
from types import SimpleNamespace

import pytest

from api.db.services.managed_resource_service import (
    ManagedResourceConfigurationError,
    ManagedResourceService,
)


@pytest.mark.parametrize(
    ("blueprint", "endpoint", "method", "expected"),
    [
        (None, None, "GET", True),
        ("unknown_api", "unknown_api.update", "POST", True),
        ("dataset_api", "dataset_api.list_datasets", "GET", True),
        ("dataset_api", "dataset_api.create", "POST", False),
        ("dataset_api", "dataset_api.search", "POST", True),
        ("dataset_api", "dataset_api.search_datasets", "POST", True),
        ("dataset_api", "dataset_api.update_dataset", "PUT", False),
        ("dataset_api", "dataset_api.delete_datasets", "DELETE", False),
        ("dataset_api", "dataset_api.update_metadata", "PATCH", False),
        ("dataset_api", "dataset_api.create", "post", False),
        ("document_api", "document_api.list_documents", "GET", True),
        ("document_api", "document_api.upload_document", "POST", False),
        ("document_api", "document_api.delete_documents", "DELETE", False),
        ("file_commit_api", "file_commit_api.create_commit_1", "POST", False),
        ("file_commit_api", "file_commit_api.list_commits_1", "GET", True),
        ("task_api", "task_api.cancel_task", "POST", False),
        ("task_api", "task_api.patch_task", "PATCH", False),
        ("chunk_api", "chunk_api.list_chunks", "GET", True),
        ("chunk_api", "chunk_api.retrieval_test", "POST", True),
        ("chunk_api", "chunk_api.create_chunk", "POST", False),
        ("chunk_api", "chunk_api.update_chunk", "PATCH", False),
        ("provider_api", "provider_api.list_providers", "GET", True),
        ("provider_api", "provider_api.add_provider", "PUT", False),
        ("provider_api", "provider_api.chat_to_model", "POST", True),
        ("models_api", "models_api.get_added_models", "GET", True),
        ("models_api", "models_api.set_default_models", "PATCH", True),
        ("llm", "llm.my_llms", "GET", True),
        ("llm", "llm.add_llm", "POST", False),
        ("chat_channel_api", "chat_channel_api.list_chat_channel", "GET", False),
        ("chat_channel_api", "chat_channel_api.create_chat_channel", "POST", False),
        ("connector_api", "connector_api.list_connectors", "GET", False),
        ("connector_api", "connector_api.create_connector", "POST", False),
        ("mcp_api", "mcp_api.list_mcp", "GET", False),
        ("mcp_api", "mcp_api.create", "POST", False),
        ("tenant_api", "tenant_api.tenant_list", "GET", True),
        ("tenant_api", "tenant_api.create", "POST", False),
        ("tenant_api", "tenant_api.agree", "PATCH", False),
        ("tenant_api", "tenant_api.rm", "DELETE", False),
        ("system_api", "system_api.token_list", "GET", False),
        ("system_api", "system_api.new_token", "POST", False),
        ("system_api", "system_api.rm", "DELETE", False),
        ("system_api", "system_api.get_config", "GET", True),
        ("user_api", "user_api.user_profile", "GET", True),
        ("user_api", "user_api.setting_user", "PATCH", True),
        ("user_api", "user_api.list_eva_user_credentials", "GET", True),
        ("user_api", "user_api.put_eva_user_credential", "PUT", True),
        ("user_api", "user_api.delete_eva_user_credential", "DELETE", True),
        ("user_api", "user_api.set_tenant_info", "PATCH", False),
        ("user_api", "other_prefix.set_tenant_info", "PATCH", False),
    ],
)
def test_regular_user_policy(blueprint, endpoint, method, expected):
    user = SimpleNamespace(is_superuser=False)
    assert ManagedResourceService.request_allowed(user, blueprint, endpoint, method) is expected


def test_superuser_policy_allows_managed_resource_changes():
    user = SimpleNamespace(is_superuser=True)
    assert ManagedResourceService.request_allowed(user, "dataset_api", "dataset_api.create", "POST") is True
    assert ManagedResourceService.request_allowed(user, "chat_channel_api", "chat_channel_api.list_chat_channel", "GET") is True
    assert ManagedResourceService.request_allowed(user, "mcp_api", "mcp_api.create", "POST") is True
    assert ManagedResourceService.request_allowed(user, "system_api", "system_api.new_token", "POST") is True


def test_owner_id_falls_back_when_managed_mode_is_not_configured(monkeypatch):
    monkeypatch.setattr(ManagedResourceService, "_configured_owner", staticmethod(lambda: ""))
    assert ManagedResourceService.owner_id("tenant-1") == "tenant-1"


def test_owner_id_requires_fallback_when_managed_mode_is_not_configured(monkeypatch):
    monkeypatch.setattr(ManagedResourceService, "_configured_owner", staticmethod(lambda: ""))

    with pytest.raises(ManagedResourceConfigurationError):
        ManagedResourceService.owner_id()


def test_owner_id_fails_closed_for_unknown_configured_owner(monkeypatch):
    monkeypatch.setattr(ManagedResourceService, "_configured_owner", staticmethod(lambda: "missing@example.com"))
    monkeypatch.setattr("api.db.services.managed_resource_service.User.get_or_none", lambda *_args, **_kwargs: None)

    with pytest.raises(ManagedResourceConfigurationError):
        ManagedResourceService.owner_id("tenant-1")


def test_owner_id_resolves_configured_account(monkeypatch):
    monkeypatch.setattr(ManagedResourceService, "_configured_owner", staticmethod(lambda: "owner@example.com"))
    monkeypatch.setattr(
        "api.db.services.managed_resource_service.User.get_or_none",
        lambda *_args, **_kwargs: SimpleNamespace(id="managed-owner"),
    )

    assert ManagedResourceService.owner_id("tenant-1") == "managed-owner"


def test_configured_owner_prefers_environment_override_and_trims_whitespace(monkeypatch):
    monkeypatch.setenv("MANAGED_RESOURCE_OWNER_EMAIL", "env-owner@example.test")
    monkeypatch.setattr(
        "api.db.services.managed_resource_service.SystemSettingsService.get_by_name",
        lambda _name: [SimpleNamespace(value="  db-owner@example.test  ")],
    )

    assert ManagedResourceService._configured_owner() == "env-owner@example.test"


def test_configured_owner_uses_environment_when_setting_is_absent(monkeypatch):
    monkeypatch.setenv("MANAGED_RESOURCE_OWNER_EMAIL", " env-owner@example.test ")
    monkeypatch.setattr(
        "api.db.services.managed_resource_service.SystemSettingsService.get_by_name",
        lambda _name: [],
    )

    assert ManagedResourceService._configured_owner() == "env-owner@example.test"


def test_configured_owner_uses_database_when_environment_is_absent(monkeypatch):
    monkeypatch.delenv("MANAGED_RESOURCE_OWNER_EMAIL", raising=False)
    monkeypatch.setattr(
        "api.db.services.managed_resource_service.SystemSettingsService.get_by_name",
        lambda _name: [SimpleNamespace(value=" db-owner@example.test ")],
    )

    assert ManagedResourceService._configured_owner() == "db-owner@example.test"


@pytest.mark.parametrize(
    ("user", "expected"),
    [
        (None, False),
        (SimpleNamespace(is_superuser=False), False),
        (SimpleNamespace(is_superuser=True), True),
    ],
)
def test_user_is_superuser_is_fail_closed(monkeypatch, user, expected):
    monkeypatch.setattr(
        "api.db.services.managed_resource_service.User.get_or_none",
        lambda *_args, **_kwargs: user,
    )

    assert ManagedResourceService.user_is_superuser("user-1") is expected
