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
import os

from api.db.db_models import User
from api.db.services.system_settings_service import SystemSettingsService
from common.constants import StatusEnum


MANAGED_RESOURCE_OWNER_SETTING = "managed_resources.owner_email"
MANAGED_RESOURCE_OWNER_ENV = "MANAGED_RESOURCE_OWNER_EMAIL"

_ADMIN_ONLY_BLUEPRINTS = {
    "chat_channel_api",
    "connector_api",
    "mcp_api",
}
_ADMIN_ONLY_MUTATION_BLUEPRINTS = {
    "chunk_api",
    "dataset_api",
    "document_api",
    "file_commit_api",
    "file2document_api",
    "llm",
    "provider_api",
    "task_api",
    "tenant_api",
}
_NON_ADMIN_MUTATION_ALLOWLIST = {
    ("chunk_api", "retrieval_test"),
    ("dataset_api", "search"),
    ("dataset_api", "search_datasets"),
    ("provider_api", "chat_to_model"),
}
_ADMIN_ONLY_ENDPOINTS = {
    ("backward_compat", "deprecated_delete_knowledge_graph"),
    ("backward_compat", "deprecated_file_convert"),
    ("backward_compat", "deprecated_file_upload_info"),
    ("backward_compat", "deprecated_run_graphrag"),
    ("backward_compat", "deprecated_run_raptor"),
    ("backward_compat", "deprecated_update_chunk"),
    ("backward_compat", "deprecated_update_document"),
    ("backward_compat_legacy_v1", "deprecated_legacy_document_upload_info"),
    ("system_api", "new_token"),
    ("system_api", "rm"),
    ("system_api", "token_list"),
    ("user_api", "set_tenant_info"),
}
_SAFE_METHODS = {"GET", "HEAD", "OPTIONS"}


class ManagedResourceConfigurationError(RuntimeError):
    pass


class ManagedResourceService:
    """Own the local managed-resource policy and its configured resource account."""

    @staticmethod
    def request_allowed(user, blueprint: str | None, endpoint: str | None, method: str) -> bool:
        if getattr(user, "is_superuser", False):
            return True

        blueprint = blueprint or ""
        endpoint_name = (endpoint or "").rsplit(".", 1)[-1]
        operation = (blueprint, endpoint_name)

        if blueprint in _ADMIN_ONLY_BLUEPRINTS or operation in _ADMIN_ONLY_ENDPOINTS:
            return False
        if method.upper() in _SAFE_METHODS:
            return True
        if operation in _NON_ADMIN_MUTATION_ALLOWLIST:
            return True
        return blueprint not in _ADMIN_ONLY_MUTATION_BLUEPRINTS

    @staticmethod
    def user_is_superuser(user_id: str) -> bool:
        user = User.get_or_none((User.id == user_id) & (User.status == StatusEnum.VALID.value))
        return bool(user and user.is_superuser)

    @staticmethod
    def _configured_owner() -> str:
        environment_owner = os.getenv(MANAGED_RESOURCE_OWNER_ENV, "").strip()
        if environment_owner:
            return environment_owner
        rows = SystemSettingsService.get_by_name(MANAGED_RESOURCE_OWNER_SETTING)
        if rows:
            return str(rows[0].value or "").strip()
        return ""

    @classmethod
    def owner_id(cls, fallback_tenant_id: str | None = None) -> str:
        """Resolve the account that owns centrally managed models, datasets and channels.

        A missing setting preserves upstream-compatible tenant ownership. Once the
        setting is present, an invalid owner fails closed instead of silently
        exposing a user's private model configuration.
        """

        owner = cls._configured_owner()
        if not owner:
            if fallback_tenant_id:
                return fallback_tenant_id
            raise ManagedResourceConfigurationError(f"System setting '{MANAGED_RESOURCE_OWNER_SETTING}' is required.")

        user = User.get_or_none(((User.email == owner) | (User.id == owner)) & (User.status == StatusEnum.VALID.value))
        if user is None:
            raise ManagedResourceConfigurationError(f"Managed resource owner '{owner}' is not an active user.")
        return user.id
