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

"""
Core Configuration Module

This module provides the unified configuration system for RAGFlow.

Structure:
----------
config/
    __init__.py               <- This file (module description & usage)
    app.py                    <- Top-level AppConfig, loads all configurations
    env_overrides.py          <- Environment variables for overriding defaults at initialization
    legacy.py                 <- Compatible with the legacy yaml structure
    components/
        base/                 <- System foundational modules
            db.py             <- Database configs (MySQL, Postgres, OceanBase)
            storage.py        <- Storage configs (Minio, S3, Azure, etc.)
            cache.py          <- Redis / caching configuration
            doc_engine.py     <- Document engine configs (ElasticSearch, OpenSearch, Infinity)
        abilities/            <- System capability modules
            rag.py            <- Core RAG capability
            llm.py            <- Default LLM configuration
            security.py       <- Security-related configs
            services.py       <- Services module configs (RAGFlow, Admin, Task-executor, etc.)
        third_party/          <- External service integrations
            oauth.py          <- OAuth configuration
            tcadp.py          <- Tencent ADP configuration
            ...               <- Other third-party service configs

Key Concepts:
-------------
1. AppConfig:
   - Top-level configuration class.
   - Aggregates all unit-level configurations (database, storage, LLM, Ragflow services, etc.).
   - Supports environment variable and YAML file overrides.
   - Example usage:
        from core.config.app import app_config
        print(app_config.database.active)
        print(app_config.redis.host)

2. Service Configs (RagflowConfig, AdminConfig, TaskExecutorConfig):
   - Encapsulate service-specific settings.
   - Configs can be overridden via environment variables (e.g., RAGFLOW_HOST, TASK_EXECUTOR_PORT) or YAML files.

3. Components:
   - Each component (database, storage, cache, llm, doc_engine, etc.) has its own Pydantic model.
   - Supports default values, type validation, and optional env prefix.

4. YAML / Environment Variable Priority:
   - Load order:
       1. `service_conf.yaml`
       2. `local.service_conf.yaml` (overrides service_conf.yaml)
       3. Environment variables (override missing values from YAML)
   - Ensures flexibility for local development and production environments.

5. Providers (External Layer):
   - Providers wrap the configuration with initialization logic, e.g., database connection, storage client, doc engine.
   - Provides singleton instances via `get_providers()` function.
   - Example:
        from core.providers import get_providers
        providers = get_providers()
        db_conn = providers.database.conn
        doc_conn = providers.doc_store.conn
        storage_conn = providers.storage.conn

6. Adding a New Component:
   - Define a Pydantic model in `components/`.
   - Add it to AppConfig with a default factory.
   - Optionally, create a corresponding provider class under `core/providers`.

7. Guidelines:
   - Do not use global variables directly; always access via AppConfig or provider instances.
   - Keep AppConfig as the source of truth for configuration data.
   - Providers manage the runtime initialization of external clients or services.

This setup ensures:
- Type-safe configuration validation.
- Flexible overrides via environment or YAML.
- Clear separation between configuration data (AppConfig) and runtime logic (Providers).
"""

from .app import AppConfig

app_config = AppConfig()
