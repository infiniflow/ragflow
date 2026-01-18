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
from core.config.components.abilities.services import (
    AdminConfig, RAGFlowConfig, SandboxConfig, TaskExecutorConfig, SMTPConfig
)
from core.config.components.third_party import ThirdPartyConfig
from core.config.env_overrides import ENV_OVERRIDES
from core.config.legacy import normalize_legacy_yaml
from core.config.utils.loader import load_yaml, merge_dicts
from core.config.utils.paths import SERVICE_CONF_PATH, LOCAL_SERVICE_CONF_PATH


class YAMLSource(PydanticBaseSettingsSource):
    """
    A Pydantic Settings source that loads values from a preloaded YAML dict.
    """

    def __init__(self, settings_cls: type[BaseSettings], yaml_dict: Dict[str, Any]):
        super().__init__(settings_cls)
        self.yaml_dict = yaml_dict
        self.explicit_paths = set()
        self._collect_paths(yaml_dict)

    def _collect_paths(self, d: dict, prefix: str = ""):
        for k, v in d.items():
            path = f"{prefix}.{k}" if prefix else k
            self.explicit_paths.add(path)
            if isinstance(v, dict):
                self._collect_paths(v, path)

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
    def __init__(self, settings_cls, yaml_source: YAMLSource):
        super().__init__(settings_cls)
        self.yaml_source = yaml_source

    def __call__(self) -> dict[str, Any]:
        result = {}

        for model_field, env_name in ENV_OVERRIDES.items():
            if model_field in self.yaml_source.explicit_paths:
                # Skip env override if it was already defined in yaml
                continue

            value = os.environ.get(env_name)
            if value is None:
                continue

            parts = model_field.split(".")
            d = result
            for p in parts[:-1]:
                d = d.setdefault(p, {})
            d[parts[-1]] = value

        return result


class AppConfig(BaseSettings):
    """
    # Top-level AppConfig
    """
    ragflow: RAGFlowConfig = Field(default_factory=RAGFlowConfig)
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

        yaml_source = YAMLSource(settings_cls, service_conf)
        env_override = EnvOverrideSource(settings_cls, yaml_source)

        return (
            yaml_source,
            env_override,
            dotenv_settings,
            file_secret_settings,
        )

    @model_validator(mode="after")
    def post_init(self):
        self.database.decrypt_passwords(self.security.password)
        self.storage.decrypt_password(self.security.password)
        self.cache.decrypt_password(self.security.password)
        return self
