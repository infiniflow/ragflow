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

ENV_OVERRIDES = {
    # Mapping of environment variables to configuration model fields.
    #
    # Usage:
    # 1. Dictionary key: full path of the model field (nested fields separated by '.')
    #    - Must match the **field names defined in YAML or the Pydantic model** exactly.
    #    - Aliases (e.g., `validation_alias`) do NOT apply for environment overrides.
    #    - Example: AppConfig has `api: APIConfig`, ENV override must be 'api.register_enabled',
    #      not the alias 'ragflow.register_enabled'.
    # 2. Dictionary value: the corresponding environment variable name
    # 3. EnvOverrideSource will read these environment variables and construct a nested dict
    # 4. The environment variable values should be compatible with the model field type.
    #    Use helper functions like str_to_bool or str_to_int for safe conversion if needed.

    # Base
    "database.active": "DB_TYPE",
    "storage.active": "STORAGE_IMPL",
    "doc_engine.active": "DOC_ENGINE",
    "cache.active": "CACHE_TYPE",

    # Ragflow / API
    "ragflow.register_enabled": "REGISTER_ENABLED",
    "ragflow.secret_key": "RAGFLOW_SECRET_KEY",
    "ragflow.strong_test_count": "STRONG_TEST_COUNT",
    "ragflow.crypto_enabled": "RAGFLOW_CRYPTO_ENABLED",
    "ragflow.default_superuser_email": "DEFAULT_SUPERUSER_EMAIL",
    "ragflow.default_superuser_password": "DEFAULT_SUPERUSER_PASSWORD",
    "ragflow.default_superuser_nickname": "DEFAULT_SUPERUSER_NICKNAME",

    # RAG
    "rag.embedding_batch_size": "EMBEDDING_BATCH_SIZE",
    "rag.ocr_gpu_mem_limit_mb": "OCR_GPU_MEM_LIMIT_MB",
    "rag.parallel_devices": "PARALLEL_DEVICES",
    "rag.doc_bulk_size": "DOC_BULK_SIZE",
    "rag.doc_maximum_size": "MAX_CONTENT_LENGTH",
    "rag.max_file_num_per_user": "MAX_FILE_NUM_PER_USER",

    # Sandbox
    "sandbox.enabled": "SANDBOX_ENABLED",
    "sandbox.host": "SANDBOX_HOST",
    "sandbox.max_memory": "SANDBOX_MAX_MEMORY",
    "sandbox.timeout": "SANDBOX_TIMEOUT",
    "sandbox.base_python_image": "SANDBOX_BASE_PYTHON_IMAGE",
    "sandbox.base_nodejs_image": "SANDBOX_BASE_NODEJS_IMAGE",
    "sandbox.executor_manager_port": "SANDBOX_EXECUTOR_MANAGER_PORT",
    "sandbox.executor_manager_pool_size": "SANDBOX_EXECUTOR_MANAGER_POOL_SIZE",
    "sandbox.enable_seccomp": "SANDBOX_ENABLE_SECCOMP",
}
