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
import os

import pytest

HOST_ADDRESS = os.getenv("HOST_ADDRESS", "http://127.0.0.1:9380")
VERSION = "v1"
ZHIPU_AI_API_KEY = os.getenv("ZHIPU_AI_API_KEY")
if ZHIPU_AI_API_KEY is None:
    pytest.exit("Error: Environment variable ZHIPU_AI_API_KEY must be set")

EMAIL = "qa@infiniflow.org"
# password is "123"
PASSWORD = """ctAseGvejiaSWWZ88T/m4FQVOpQyUvP+x7sXtdv3feqZACiQleuewkUi35E16wSd5C5QcnkkcV9cYc8TKPTRZlxappDuirxghxoOvFcJxFU4ixLsD
fN33jCHRoDUW81IH9zjij/vaw8IbVyb6vuwg6MX6inOEBRRzVbRYxXOu1wkWY6SsI8X70oF9aeLFp/PzQpjoe/YbSqpTq8qqrmHzn9vO+yvyYyvmDsphXe
X8f7fp9c7vUsfOCkM+gHY3PadG+QHa7KI7mzTKgUTZImK6BZtfRBATDTthEUbbaTewY4H0MnWiCeeDhcbeQao6cFy1To8pE3RpmxnGnS8BsBn8w=="""

INVALID_API_TOKEN = "invalid_key_123"
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
    "topn_tags": 3,
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
    },
}