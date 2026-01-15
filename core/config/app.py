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
from typing import Any, Dict

from pydantic import Field, model_validator, AliasChoices
from pydantic.fields import FieldInfo
from pydantic_settings import BaseSettings, SettingsConfigDict, PydanticBaseSettingsSource, EnvSettingsSource

from core.config.components.abilities import UserDefaultLLMConfig, RAGConfig, SecurityConfig
from core.config.components.base import CacheConfig, DatabaseConfig, DocumentEngineConfig, StorageConfig
from core.config.components.processes import (
    AdminConfig, APIConfig, SandboxConfig, TaskExecutorConfig, SMTPConfig
)
from core.config.components.third_party import ThirdPartyConfig
from core.config.env_overrides import ENV_OVERRIDES
from core.config.legacy import normalize_legacy_yaml
from core.config.utils.loader import load_yaml, merge_dicts
from core.config.utils.paths import SERVICE_CONF_PATH, LOCAL_SERVICE_CONF_PATH
from core.types.cache import CacheType
from core.types.database import DatabaseType
from core.types.doc_engine import DocumentEngineType
from core.types.storage import ObjectStorageType


class YAMLSource(PydanticBaseSettingsSource):
    """
    A Pydantic Settings source that loads values from a preloaded YAML dict.
    """

    def __init__(self, settings_cls: type[BaseSettings], yaml_dict: Dict[str, Any]):
        super().__init__(settings_cls)
        self.yaml_dict = yaml_dict

    def __call__(self) -> Dict[str, Any]:
        return self.yaml_dict

    def get_field_value(self, field: FieldInfo, field_name: str) -> tuple[Any, str, bool]:
        if field_name in self.yaml_dict:
            return self.yaml_dict[field_name], "yaml", True
        return None, "yaml", False


class EnvOverrideSource(EnvSettingsSource):
    """
    Override from environment variables.

    Define mappings of environment variables and config fields at env_overrides.py.
    """

    def __call__(self) -> dict[str, Any]:
        result = {}
        for model_field, env_name in ENV_OVERRIDES.items():
            value = os.environ.get(env_name)
            if value is not None:
                # Nested dict assignment for "a.b.c" paths
                parts = model_field.split(".")
                d = result
                for p in parts[:-1]:
                    d = d.setdefault(p, {})

                if env_name == "REGISTER_ENABLED":
                    value = True
                d[parts[-1]] = value
        return result

    def prepare_field_value(self, field_name, field, value, value_is_complex):
        return value


class AppConfig(BaseSettings):
    """
    # Top-level AppConfig
    """
    ragflow: APIConfig = Field(default_factory=APIConfig)
    rag: RAGConfig = Field(default_factory=RAGConfig)
    task_executor: TaskExecutorConfig = Field(default_factory=TaskExecutorConfig)
    admin: AdminConfig = Field(default_factory=AdminConfig)
    smtp: SMTPConfig = Field(default_factory=SMTPConfig)
    sandbox: SandboxConfig = Field(default_factory=SandboxConfig)

    user_default_llm: UserDefaultLLMConfig = Field(default_factory=UserDefaultLLMConfig)

    # ----------------------
    # Integrated Configurations
    # ----------------------
    database: DatabaseConfig = Field(
        default_factory=DatabaseConfig, validation_alias=AliasChoices("db", "database"))
    storage: StorageConfig = Field(default_factory=StorageConfig)
    doc_engine: DocumentEngineConfig = Field(default_factory=DocumentEngineConfig)
    cache: CacheConfig = Field(default_factory=CacheConfig)
    security: SecurityConfig = Field(default_factory=SecurityConfig)
    third_party: ThirdPartyConfig = Field(default_factory=ThirdPartyConfig)

    # ----------------------
    # Pydantic Settings
    # ----------------------
    model_config = SettingsConfigDict()

    @classmethod
    def settings_customise_sources(
            cls,
            settings_cls,
            init_settings,
            env_settings,
            dotenv_settings,
            file_secret_settings,
    ):
        service_conf: Dict[str, Any] = merge_dicts(
            load_yaml(SERVICE_CONF_PATH),
            load_yaml(LOCAL_SERVICE_CONF_PATH, allow_missing=True)
        )
        # Compatible with old yaml format
        service_conf = normalize_legacy_yaml(service_conf)
        return (
            YAMLSource(settings_cls, service_conf),
            EnvOverrideSource(settings_cls),
            dotenv_settings,
            file_secret_settings,
        )

    @model_validator(mode="after")
    @classmethod
    def post_init(cls, values):
        # Override active from environment variable if present
        db_type = os.environ.get("DB_TYPE")
        if db_type:
            values.database.active = DatabaseType(db_type.lower())
            values.database.decrypt_passwords(values.security.password)

        storage_impl = os.environ.get("STORAGE_IMPL")
        if storage_impl:
            values.storage.active = ObjectStorageType(storage_impl.lower())
            values.storage.decrypt_password(values.security.password)

        doc_engine_name = os.environ.get("DOC_ENGINE")
        if doc_engine_name:
            values.doc_engine.active = DocumentEngineType(doc_engine_name.lower())

        cache_type = os.environ.get("CACHE_TYPE", "redis")
        values.cache.active = CacheType(cache_type.lower())
        if values.cache.active == CacheType.REDIS:
            values.cache.decrypt_password(values.security.password)

        return values
