#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import os
from pathlib import Path

import pytest


_DOCKER_ENV = Path(__file__).resolve().parents[2] / "docker" / ".env"


def _docker_env_value(name: str) -> str | None:
    if not _DOCKER_ENV.exists():
        return None
    for line in _DOCKER_ENV.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        if key.strip() == name:
            return value.split("#", 1)[0].strip().strip('"').strip("'")
    return None


def _config_value(name: str, default: str | None = None) -> str | None:
    return os.getenv(name) or _docker_env_value(name) or default


API_PROXY_SCHEME = _config_value("API_PROXY_SCHEME", "python")
IS_GO_PROXY = API_PROXY_SCHEME == "go"
SDK_UNAUTHORIZED_ERROR_MESSAGE = "Invalid access token" if IS_GO_PROXY else "<Unauthorized '401: Unauthorized'>"


def _default_host_address() -> str:
    if API_PROXY_SCHEME == "go":
        return f"http://127.0.0.1:{_config_value('GO_HTTP_PORT', '9384')}"
    return "http://127.0.0.1:9380"


HOST_ADDRESS = os.getenv("HOST_ADDRESS") or _default_host_address()
logging.info("Resolved API proxy configuration: scheme=%s, is_go_proxy=%s, host_address=%s", API_PROXY_SCHEME, IS_GO_PROXY, HOST_ADDRESS)
VERSION = "v1"
ZHIPU_AI_API_KEY = os.getenv("ZHIPU_AI_API_KEY")
if ZHIPU_AI_API_KEY is None:
    pytest.exit("Error: Environment variable ZHIPU_AI_API_KEY must be set")

SILICONFLOW_API_KEY = os.getenv("SILICONFLOW_API_KEY")
if SILICONFLOW_API_KEY is None:
    pytest.exit("Error: Environment variable SILICONFLOW_API_KEY must be set")

EMAIL = "qa@infiniflow.org"
# password is "123"
PASSWORD = """ctAseGvejiaSWWZ88T/m4FQVOpQyUvP+x7sXtdv3feqZACiQleuewkUi35E16wSd5C5QcnkkcV9cYc8TKPTRZlxappDuirxghxoOvFcJxFU4ixLsD
fN33jCHRoDUW81IH9zjij/vaw8IbVyb6vuwg6MX6inOEBRRzVbRYxXOu1wkWY6SsI8X70oF9aeLFp/PzQpjoe/YbSqpTq8qqrmHzn9vO+yvyYyvmDsphXe
X8f7fp9c7vUsfOCkM+gHY3PadG+QHa7KI7mzTKgUTZImK6BZtfRBATDTthEUbbaTewY4H0MnWiCeeDhcbeQao6cFy1To8pE3RpmxnGnS8BsBn8w=="""

INVALID_API_TOKEN = "invalid_key_123"
INVALID_ID_32 = "0" * 32
DATASET_NAME_LIMIT = 128
DOCUMENT_NAME_LIMIT = 255
CHAT_ASSISTANT_NAME_LIMIT = 255
SESSION_WITH_CHAT_NAME_LIMIT = 255

DEFAULT_PARSER_CONFIG = {
    "layout_recognize": "DeepDOC",
    "chunk_token_num": 512,
    "delimiter": "\n",
    "auto_keywords": 0,
    "auto_questions": 0,
    "html4excel": False,
    "image_context_size": 0,
    "table_context_size": 0,
    "topn_tags": 3,
    "llm_id": "glm-4-flash@CI@ZHIPU-AI",
    "raptor": {
        "use_raptor": True,
        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
        "max_token": 256,
        "threshold": 0.1,
        "max_cluster": 64,
        "random_seed": 0,
    },
    "graphrag": {
        "use_graphrag": True,
        "entity_types": [
            "organization",
            "person",
            "geo",
            "event",
            "category",
        ],
        "method": "light",
        "batch_chunk_token_size": 4096,
        "retry_attempts": 2,
        "retry_backoff_seconds": 2.0,
        "retry_backoff_max_seconds": 60.0,
        "build_subgraph_timeout_per_chunk_seconds": 300,
        "build_subgraph_min_timeout_seconds": 600,
        "merge_timeout_seconds": 180,
        "resolution_timeout_seconds": 1800,
        "community_timeout_seconds": 1800,
        "lock_acquire_timeout_seconds": 600,
    },
    "parent_child": {
        "use_parent_child": False,
        "children_delimiter": "\n",
    },
    "children_delimiter": "",
}
